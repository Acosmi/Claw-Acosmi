package gateway

// remote_approval_feishu.go — P4 飞书远程审批 Provider
// 通过飞书开放平台 API 发送互动卡片消息。
//
// 实现方式：直接 HTTP API 调用（标准库），不引入 larksuite/oapi-sdk-go
// 原因：减少外部依赖，飞书 API 结构稳定，HTTP 调用足够简单。
//
// API 参考：
//   - 获取 tenant_access_token: POST https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal/
//   - 发送消息: POST https://open.feishu.cn/open-apis/im/v1/messages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/runner"
)

const (
	feishuTokenURL   = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal/"
	feishuMessageURL = "https://open.feishu.cn/open-apis/im/v1/messages"
)

// feishuProvider 飞书远程审批 Provider。
type feishuProvider struct {
	config *FeishuProviderConfig
	client *http.Client
	// token cache
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// newFeishuProvider 创建飞书 Provider。
func newFeishuProvider(cfg *FeishuProviderConfig) *feishuProvider {
	return &feishuProvider{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *feishuProvider) Name() string { return "feishu" }

func (p *feishuProvider) ValidateConfig() error {
	if p.config.AppID == "" {
		return fmt.Errorf("飞书 App ID 不能为空")
	}
	if p.config.AppSecret == "" {
		return fmt.Errorf("飞书 App Secret 不能为空")
	}
	// 接收方校验移至 SendApprovalRequest，因为 OriginatorUserID 是运行时动态填入的
	return nil
}

func (p *feishuProvider) SendApprovalRequest(ctx context.Context, req ApprovalCardRequest) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	// 1. 获取 tenant_access_token
	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	// 2. 构建互动卡片
	card := p.buildApprovalCard(req)

	// 3. 群发到静态配置目标 + 运行时动态 originator 目标
	return p.broadcastCard(ctx, token, card,
		feishuTarget{"chat_id", req.OriginatorChatID},
		feishuTarget{"open_id", req.OriginatorUserID},
	)
}

// getTenantAccessToken 获取飞书 tenant_access_token（带缓存）。
// 飞书 token 有效期 2 小时，提前 5 分钟刷新。
func (p *feishuProvider) getTenantAccessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	// 缓存命中（提前 5 分钟刷新）
	if p.cachedToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.cachedToken, nil
	}

	body, _ := json.Marshal(map[string]string{
		"app_id":     p.config.AppID,
		"app_secret": p.config.AppSecret,
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", feishuTokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("飞书 token API 错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	p.cachedToken = result.TenantAccessToken
	p.tokenExpiry = time.Now().Add(115 * time.Minute) // 2h - 5min buffer
	return p.cachedToken, nil
}

// buildApprovalCard 构建飞书互动卡片 JSON。
func (p *feishuProvider) buildApprovalCard(req ApprovalCardRequest) map[string]interface{} {
	levelLabel := map[string]string{
		"full":      "🔴 L3 — 完全权限 / Full Access",
		"sandboxed": "🟠 L2 — 沙盒执行 / Sandboxed",
		"allowlist": "🟡 L1 — 受限执行 / Allowlist",
	}[req.RequestedLevel]
	if levelLabel == "" {
		levelLabel = req.RequestedLevel
	}

	durationText := fmt.Sprintf("%d 分钟", req.TTLMinutes)
	if req.TaskScoped {
		durationText = "本任务有效 / Current task only"
	} else if isPermanentApprovalLevel(req.RequestedLevel) {
		durationText = "永久授权 / Permanent"
	}

	approveValue := map[string]interface{}{
		"action": "approve",
		"id":     req.EscalationID,
	}
	if !req.TaskScoped && !isPermanentApprovalLevel(req.RequestedLevel) {
		approveValue["ttl"] = req.TTLMinutes
	}

	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "🔐 权限提升审批 / Permission Escalation",
			},
			"template": "red",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"fields": []interface{}{
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**请求级别**\n%s", levelLabel),
						},
					},
					map[string]interface{}{
						"is_short": true,
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**授权模式**\n%s", durationText),
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**原因**: %s", req.Reason),
				},
			},
			map[string]interface{}{
				"tag": "hr",
			},
			map[string]interface{}{
				"tag": "action",
				"actions": []interface{}{
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "✅ 批准 / Approve",
						},
						"type":  "primary",
						"value": approveValue,
					},
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "❌ 拒绝 / Deny",
						},
						"type": "danger",
						"value": map[string]interface{}{
							"action": "deny",
							"id":     req.EscalationID,
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("ID: %s | %s", req.EscalationID, req.RequestedAt.Format(time.RFC3339)),
					},
				},
			},
		},
	}
	return card
}

// feishuTarget 飞书消息发送目标。
type feishuTarget struct {
	idType string // "chat_id" 或 "open_id"
	id     string
}

// broadcastCard 群发飞书卡片到所有已配置目标（群聊 + 私聊）+ 可选额外目标。
// 策略：
// 1) 优先尝试 chat_id 目标；
// 2) 若 chat_id 全失败，则回退 open_id 目标重试；
// 3) 若 chat_id 成功，则不再发送 open_id（避免双卡片）。
func (p *feishuProvider) broadcastCard(ctx context.Context, token string, card map[string]interface{}, extraTargets ...feishuTarget) error {
	seen := make(map[string]bool)
	var chatTargets []feishuTarget
	var userTargets []feishuTarget
	addTarget := func(idType, id string) {
		if id == "" {
			return
		}
		key := idType + ":" + id
		if seen[key] {
			return
		}
		seen[key] = true
		target := feishuTarget{idType, id}
		if idType == "chat_id" {
			chatTargets = append(chatTargets, target)
			return
		}
		if idType == "open_id" {
			userTargets = append(userTargets, target)
		}
	}

	// 收集 chat_id 目标（优先级最高）
	addTarget("chat_id", p.config.ChatID)
	addTarget("chat_id", p.config.ApprovalChatID) // 固定审批群 fallback
	addTarget("chat_id", p.config.LastKnownChatID)
	for _, t := range extraTargets {
		if t.idType == "chat_id" {
			addTarget(t.idType, t.id)
		}
	}

	// 收集 open_id 目标（用于无群目标或群发送失败回退）
	addTarget("open_id", p.config.UserID)
	addTarget("open_id", p.config.LastKnownUserID)
	for _, t := range extraTargets {
		if t.idType == "open_id" {
			addTarget(t.idType, t.id)
		}
	}

	if len(chatTargets) == 0 && len(userTargets) == 0 {
		return fmt.Errorf("飞书消息接收方为空: 需要配置 chatId/userId 或由飞书消息事件自动填充")
	}

	sendBatch := func(targets []feishuTarget) (int, []error) {
		if len(targets) == 0 {
			return 0, nil
		}
		success := 0
		errs := make([]error, 0, len(targets))
		for _, t := range targets {
			if err := p.sendMessageTo(ctx, token, card, t.idType, t.id); err != nil {
				errs = append(errs, err)
			} else {
				success++
			}
		}
		return success, errs
	}

	// 1) 优先尝试群聊目标
	if len(chatTargets) > 0 {
		success, errs := sendBatch(chatTargets)
		if success > 0 {
			return nil
		}
		// 2) 群聊全部失败，回退到私聊目标
		if len(userTargets) > 0 {
			userSuccess, userErrs := sendBatch(userTargets)
			if userSuccess > 0 {
				return nil
			}
			allErrs := append(errs, userErrs...)
			return errors.Join(allErrs...)
		}
		return errors.Join(errs...)
	}

	// 3) 无群聊目标，直接尝试私聊目标
	success, errs := sendBatch(userTargets)
	if success > 0 {
		return nil
	}
	if len(errs) == 0 {
		return fmt.Errorf("飞书消息发送失败: 无可用接收目标")
	}
	return errors.Join(errs...)
}

// sendMessageTo 发送飞书消息到指定接收方。
func (p *feishuProvider) sendMessageTo(ctx context.Context, token string, card map[string]interface{}, receiveIDType, receiveID string) error {
	if receiveID == "" {
		return fmt.Errorf("飞书消息接收方为空: 需要配置 chatId、userId 或由系统自动填充 originatorUserId")
	}

	cardJSON, err := json.Marshal(card)
	if err != nil {
		return err
	}

	msgBody := map[string]interface{}{
		"receive_id": receiveID,
		"msg_type":   "interactive",
		"content":    string(cardJSON),
	}

	body, err := json.Marshal(msgBody)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s?receive_id_type=%s", feishuMessageURL, receiveIDType)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("飞书消息 API 响应解析失败: %s", string(respBody))
	}
	if result.Code != 0 {
		return fmt.Errorf("飞书消息发送失败: code=%d, msg=%s", result.Code, result.Msg)
	}
	return nil
}

// ---------- Phase 8: 审批结果通知 ----------

// SendResultNotification 实现 ResultNotifier 接口，推送审批结果卡片。
func (p *feishuProvider) SendResultNotification(ctx context.Context, result ApprovalResultNotification) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.buildResultCard(result)
	return p.broadcastCard(ctx, token, card)
}

// buildResultCard 构建审批结果互动卡片。
func (p *feishuProvider) buildResultCard(result ApprovalResultNotification) map[string]interface{} {
	var headerTitle, headerTemplate, bodyText string

	if result.Approved {
		headerTitle = "✅ 权限已生效 / Permission Granted"
		headerTemplate = "green"
		if isPermanentApprovalLevel(result.RequestedLevel) {
			bodyText = fmt.Sprintf("权限提升请求已批准。\n\n"+
				"**授权级别**: %s\n"+
				"**授权模式**: 永久授权 / Permanent\n\n"+
				"该权限将持续生效，直到手动改回。",
				result.RequestedLevel)
		} else {
			bodyText = fmt.Sprintf("权限提升请求已批准。\n\n"+
				"**授权级别**: %s\n"+
				"**有效时长**: %d 分钟\n\n"+
				"权限到期后将自动降级。",
				result.RequestedLevel, result.TTLMinutes)
		}
	} else {
		headerTitle = "❌ 权限请求已拒绝 / Permission Denied"
		headerTemplate = "red"
		reason := result.Reason
		if reason == "" {
			reason = "管理员拒绝 / Denied by administrator"
		}
		bodyText = fmt.Sprintf("权限提升请求未通过。\n\n"+
			"**拒绝原因**: %s\n\n"+
			"相关任务已暂停执行。如需继续，请重新发起权限申请。",
			reason)
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": headerTitle,
			},
			"template": headerTemplate,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": bodyText,
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("ID: %s", result.EscalationID),
					},
				},
			},
		},
	}
}

// ---------- CoderConfirmation 操作确认卡片 ----------

// SendCoderConfirmRequest 实现 CoderConfirmNotifier 接口，发送操作确认卡片。
func (p *feishuProvider) SendCoderConfirmRequest(ctx context.Context, req CoderConfirmCardRequest) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.buildCoderConfirmCard(req)
	// D5-F3: 与 SendApprovalRequest 对齐，同时推送到群聊(chat_id)和私聊(open_id)。
	return p.broadcastCard(ctx, token, card,
		feishuTarget{"chat_id", req.OriginatorChatID},
		feishuTarget{"open_id", req.OriginatorUserID},
	)
}

// SendCoderConfirmResult 实现 CoderConfirmNotifier 接口，发送操作确认结果卡片。
func (p *feishuProvider) SendCoderConfirmResult(ctx context.Context, id string, approved bool) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.buildCoderConfirmResultCard(id, approved)
	return p.broadcastCard(ctx, token, card)
}

// buildCoderConfirmCard 构建操作确认互动卡片（黄色主题）。
func (p *feishuProvider) buildCoderConfirmCard(req CoderConfirmCardRequest) map[string]interface{} {
	preview := req.Preview
	if preview == "" {
		preview = req.ToolName
	}
	// 截断预览文本
	previewRunes := []rune(preview)
	if len(previewRunes) > 200 {
		preview = string(previewRunes[:200]) + "..."
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "⚠️ 操作确认 / Action Confirmation",
			},
			"template": "yellow",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**工具**: %s\n**内容预览**:\n```\n%s\n```", req.ToolName, preview),
				},
			},
			map[string]interface{}{
				"tag": "hr",
			},
			map[string]interface{}{
				"tag": "action",
				"actions": []interface{}{
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "✅ 允许 / Allow",
						},
						"type": "primary",
						"value": map[string]interface{}{
							"type":   "coder_confirm",
							"action": "allow",
							"id":     req.ConfirmID,
						},
					},
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "❌ 拒绝 / Deny",
						},
						"type": "danger",
						"value": map[string]interface{}{
							"type":   "coder_confirm",
							"action": "deny",
							"id":     req.ConfirmID,
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("ID: %s | 超时: %d 分钟", req.ConfirmID, req.TTLMinutes),
					},
				},
			},
		},
	}
}

// buildCoderConfirmResultCard 构建操作确认结果卡片。
func (p *feishuProvider) buildCoderConfirmResultCard(id string, approved bool) map[string]interface{} {
	var headerTitle, headerTemplate, bodyText string
	if approved {
		headerTitle = "✅ 操作已批准 / Action Approved"
		headerTemplate = "green"
		bodyText = "操作确认请求已批准，正在执行。"
	} else {
		headerTitle = "❌ 操作已拒绝 / Action Denied"
		headerTemplate = "red"
		bodyText = "操作确认请求已拒绝，任务将跳过此操作。"
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": headerTitle,
			},
			"template": headerTemplate,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": bodyText,
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("ID: %s", id),
					},
				},
			},
		},
	}
}

// ---------- PlanConfirmation 方案确认卡片 ----------

// SendPlanConfirmRequest 实现 PlanConfirmNotifier 接口，发送方案确认卡片。
func (p *feishuProvider) SendPlanConfirmRequest(ctx context.Context, req PlanConfirmCardRequest) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.buildPlanConfirmCard(req)
	return p.broadcastCard(ctx, token, card,
		feishuTarget{"chat_id", req.OriginatorChatID},
		feishuTarget{"open_id", req.OriginatorUserID},
	)
}

// SendPlanConfirmResult 实现 PlanConfirmNotifier 接口，发送方案确认结果卡片。
func (p *feishuProvider) SendPlanConfirmResult(ctx context.Context, id string, decision string) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.buildPlanConfirmResultCard(id, decision)
	return p.broadcastCard(ctx, token, card)
}

// buildPlanConfirmCard 构建方案确认互动卡片（蓝色主题）。
func (p *feishuProvider) buildPlanConfirmCard(req PlanConfirmCardRequest) map[string]interface{} {
	brief := req.TaskBrief
	if len([]rune(brief)) > 200 {
		brief = string([]rune(brief)[:200]) + "..."
	}

	// 格式化方案步骤
	stepsText := ""
	for i, step := range req.PlanSteps {
		if i >= 10 { // 最多展示 10 步
			stepsText += fmt.Sprintf("\n...（共 %d 步）", len(req.PlanSteps))
			break
		}
		stepsText += fmt.Sprintf("\n%d. %s", i+1, step)
	}

	approvalText := ""
	for i, item := range req.ApprovalSummary {
		if i >= 5 {
			approvalText += fmt.Sprintf("\n...（共 %d 项审批）", len(req.ApprovalSummary))
			break
		}
		approvalText += fmt.Sprintf("\n- %s", item)
	}

	content := fmt.Sprintf("**任务**: %s\n**意图级别**: %s", brief, req.IntentTier)
	if approvalText != "" {
		content += fmt.Sprintf("\n\n**审批摘要**:%s", approvalText)
	}
	content += fmt.Sprintf("\n\n**执行方案**:%s", stepsText)

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "📋 方案确认 / Plan Confirmation",
			},
			"template": "blue",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": content,
				},
			},
			map[string]interface{}{
				"tag": "hr",
			},
			map[string]interface{}{
				"tag": "action",
				"actions": []interface{}{
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "✅ 批准 / Approve",
						},
						"type": "primary",
						"value": map[string]interface{}{
							"type":   "plan_confirm",
							"action": "approve",
							"id":     req.ConfirmID,
						},
					},
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"tag":     "plain_text",
							"content": "❌ 拒绝 / Reject",
						},
						"type": "danger",
						"value": map[string]interface{}{
							"type":   "plan_confirm",
							"action": "reject",
							"id":     req.ConfirmID,
						},
					},
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("ID: %s | 超时: %d 分钟", req.ConfirmID, req.TTLMinutes),
					},
				},
			},
		},
	}
}

// buildPlanConfirmResultCard 构建方案确认结果卡片。
func (p *feishuProvider) buildPlanConfirmResultCard(id string, decision string) map[string]interface{} {
	var headerTitle, headerTemplate, bodyText string
	switch decision {
	case "approve":
		headerTitle = "✅ 方案已批准 / Plan Approved"
		headerTemplate = "green"
		bodyText = "方案确认请求已批准，正在执行。"
	case "reject":
		headerTitle = "❌ 方案已拒绝 / Plan Rejected"
		headerTemplate = "red"
		bodyText = "方案确认请求已拒绝，任务已暂停。"
	case "edit":
		headerTitle = "✏️ 方案已修改 / Plan Edited"
		headerTemplate = "orange"
		bodyText = "方案已被编辑，正在按修改后的方案执行。"
	default:
		headerTitle = "📋 方案确认已处理 / Plan Processed"
		headerTemplate = "grey"
		bodyText = fmt.Sprintf("方案确认已处理，决策: %s", decision)
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": headerTitle,
			},
			"template": headerTemplate,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": bodyText,
				},
			},
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "plain_text",
						"content": fmt.Sprintf("ID: %s", id),
					},
				},
			},
		},
	}
}

// ---------- Phase 6: 类型化审批卡片 ----------

// SendTypedApprovalRequest 实现 TypedApprovalNotifier 接口，发送类型化审批卡片。
func (p *feishuProvider) SendTypedApprovalRequest(ctx context.Context, req TypedApprovalRequest) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.renderTypedCard(req)
	return p.broadcastCard(ctx, token, card,
		feishuTarget{"chat_id", req.OriginatorChatID},
		feishuTarget{"open_id", req.OriginatorUserID},
	)
}

// SendTypedApprovalResult 实现 TypedApprovalResultNotifier 接口，发送类型化审批结果卡片。
func (p *feishuProvider) SendTypedApprovalResult(ctx context.Context, result TypedApprovalResultNotification) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	token, err := p.getTenantAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取飞书 access token 失败: %w", err)
	}

	card := p.renderTypedResultCard(result)
	return p.broadcastCard(ctx, token, card)
}

// renderTypedCard 根据审批类型分派到对应的卡片构建器。
func (p *feishuProvider) renderTypedCard(req TypedApprovalRequest) map[string]interface{} {
	switch req.Type {
	case ApprovalTypePlanConfirm:
		return p.buildTypedPlanConfirmCard(req)
	case ApprovalTypeExecEscalation:
		return p.buildTypedExecEscalationCard(req)
	case ApprovalTypeMountAccess:
		return p.buildTypedMountAccessCard(req)
	case ApprovalTypeDataExport:
		return p.buildTypedDataExportCard(req)
	case ApprovalTypeResultReview:
		return p.buildTypedResultReviewCard(req)
	default:
		return p.buildTypedExecEscalationCard(req)
	}
}

// renderTypedResultCard 根据审批类型分派到对应的结果卡片构建器。
func (p *feishuProvider) renderTypedResultCard(result TypedApprovalResultNotification) map[string]interface{} {
	switch result.Type {
	case ApprovalTypeMountAccess:
		return p.buildTypedMountAccessResultCard(result)
	case ApprovalTypeDataExport:
		return p.buildTypedDataExportResultCard(result)
	case ApprovalTypePlanConfirm:
		return p.buildTypedPlanConfirmResultCard(result)
	case ApprovalTypeExecEscalation:
		return p.buildTypedExecEscalationResultCard(result)
	case ApprovalTypeResultReview:
		return p.buildTypedResultReviewResultCard(result)
	default:
		return p.buildTypedExecEscalationResultCard(result)
	}
}

func appendWorkflowElements(elements []interface{}, workflow runner.ApprovalWorkflow, currentType string) []interface{} {
	if workflow.ID == "" || len(workflow.Stages) == 0 {
		return elements
	}
	stage, index, total, ok := workflow.StageInfo(currentType)
	lines := make([]string, 0, len(workflow.Stages)+2)
	if ok {
		lines = append(lines, fmt.Sprintf("**当前阶段**: %d/%d %s", index, total, stage.Summary))
	}
	for i, item := range workflow.Stages {
		status := "待处理"
		switch item.Status {
		case runner.ApprovalStageApproved:
			status = "已批准"
		case runner.ApprovalStageRejected:
			status = "已拒绝"
		case runner.ApprovalStageSkipped:
			status = "已跳过"
		case runner.ApprovalStageEdited:
			status = "已修改"
		case runner.ApprovalStagePending:
			status = "待处理"
		}
		summary := item.Summary
		if summary == "" {
			summary = item.Type
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, status, summary))
		if i >= 4 && len(workflow.Stages) > 5 {
			lines = append(lines, fmt.Sprintf("...（共 %d 个阶段）", len(workflow.Stages)))
			break
		}
	}
	return append(elements, map[string]interface{}{
		"tag": "div",
		"text": map[string]interface{}{
			"tag":     "lark_md",
			"content": "**审批流程**:\n" + strings.Join(lines, "\n"),
		},
	})
}

// P6-3: buildTypedPlanConfirmCard 构建方案确认卡片（蓝色主题，展示 PlanSteps）。
func (p *feishuProvider) buildTypedPlanConfirmCard(req TypedApprovalRequest) map[string]interface{} {
	brief := req.TaskBrief
	if len([]rune(brief)) > 200 {
		brief = string([]rune(brief)[:200]) + "..."
	}

	stepsText := ""
	for i, step := range req.PlanSteps {
		if i >= 10 {
			stepsText += fmt.Sprintf("\n...（共 %d 步）", len(req.PlanSteps))
			break
		}
		stepsText += fmt.Sprintf("\n%d. %s", i+1, step)
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**任务**: %s\n**意图级别**: %s\n\n**执行方案**:%s", brief, req.IntentTier, stepsText),
			},
		},
	}
	elements = appendWorkflowElements(elements, req.Workflow, ApprovalTypePlanConfirm)
	elements = append(elements,
		map[string]interface{}{"tag": "hr"},
		map[string]interface{}{
			"tag": "action",
			"actions": []interface{}{
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "✅ 批准 / Approve"},
					"type": "primary",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "approve",
						"id": req.ID, "approval_type": req.Type,
					},
				},
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "❌ 拒绝 / Reject"},
					"type": "danger",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "reject",
						"id": req.ID, "approval_type": req.Type,
					},
				},
			},
		},
		map[string]interface{}{
			"tag": "note",
			"elements": []interface{}{
				map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s | 超时: %d 分钟", req.ID, req.Type, req.TTLMinutes)},
			},
		},
	)

	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "📋 方案确认 / Plan Confirmation"},
			"template": "blue",
		},
		"elements": elements,
	}
}

// P6-4: buildTypedExecEscalationCard 构建执行提权卡片（红色主题，展示命令 + 风险等级）。
func (p *feishuProvider) buildTypedExecEscalationCard(req TypedApprovalRequest) map[string]interface{} {
	levelLabel := map[string]string{
		"full":      "🔴 L3 — 完全权限 / Full Access",
		"sandboxed": "🟠 L2 — 沙盒执行 / Sandboxed",
		"allowlist": "🟡 L1 — 受限执行 / Allowlist",
	}[req.RequestedLevel]
	if levelLabel == "" {
		levelLabel = req.RequestedLevel
	}

	riskLabel := map[string]string{
		"critical": "🔴 极高 / Critical",
		"high":     "🟠 高 / High",
		"medium":   "🟡 中 / Medium",
		"low":      "🟢 低 / Low",
	}[req.RiskLevel]
	if riskLabel == "" {
		riskLabel = "🟡 中 / Medium"
	}

	bodyContent := fmt.Sprintf("**请求级别**: %s\n**授权时长**: %d 分钟\n**风险等级**: %s",
		levelLabel, req.TTLMinutes, riskLabel)
	if isPermanentApprovalLevel(req.RequestedLevel) {
		bodyContent = fmt.Sprintf("**请求级别**: %s\n**授权模式**: 永久授权 / Permanent\n**风险等级**: %s",
			levelLabel, riskLabel)
	}
	if req.Command != "" {
		cmdPreview := req.Command
		if len([]rune(cmdPreview)) > 200 {
			cmdPreview = string([]rune(cmdPreview)[:200]) + "..."
		}
		bodyContent += fmt.Sprintf("\n\n**命令预览**:\n```\n%s\n```", cmdPreview)
	}
	bodyContent += fmt.Sprintf("\n\n**原因**: %s", req.Reason)

	approveValue := map[string]interface{}{
		"type": "typed_approval", "action": "approve",
		"id": req.ID, "approval_type": req.Type,
	}
	if !isPermanentApprovalLevel(req.RequestedLevel) {
		approveValue["ttl"] = req.TTLMinutes
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyContent},
		},
	}
	elements = appendWorkflowElements(elements, req.Workflow, ApprovalTypeExecEscalation)
	elements = append(elements,
		map[string]interface{}{"tag": "hr"},
		map[string]interface{}{
			"tag": "action",
			"actions": []interface{}{
				map[string]interface{}{
					"tag":   "button",
					"text":  map[string]interface{}{"tag": "plain_text", "content": "✅ 批准 / Approve"},
					"type":  "primary",
					"value": approveValue,
				},
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "❌ 拒绝 / Deny"},
					"type": "danger",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "deny",
						"id": req.ID, "approval_type": req.Type,
					},
				},
			},
		},
		map[string]interface{}{
			"tag": "note",
			"elements": []interface{}{
				map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s | %s", req.ID, req.Type, req.RequestedAt.Format(time.RFC3339))},
			},
		},
	)

	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "🔐 权限提升审批 / Permission Escalation"},
			"template": "red",
		},
		"elements": elements,
	}
}

// P6-5: buildTypedMountAccessCard 构建挂载访问卡片（橙色主题，展示路径 + 读写权限 + 任务范围）。
func (p *feishuProvider) buildTypedMountAccessCard(req TypedApprovalRequest) map[string]interface{} {
	modeLabel := map[string]string{
		"ro": "🔒 只读 / Read-Only",
		"rw": "🔓 读写 / Read-Write",
	}[req.MountMode]
	if modeLabel == "" {
		modeLabel = "🔒 只读 / Read-Only"
	}

	scopeLine := fmt.Sprintf("**授权时长**: %d 分钟", req.TTLMinutes)
	if req.TaskScoped {
		scopeLine = "**授权范围**: 当前任务（任务结束自动回收）"
	}
	bodyContent := fmt.Sprintf("**挂载路径**: `%s`\n**访问模式**: %s\n%s",
		req.MountPath, modeLabel, scopeLine)
	bodyContent += fmt.Sprintf("\n\n**原因**: %s", req.Reason)

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyContent},
		},
	}
	elements = appendWorkflowElements(elements, req.Workflow, ApprovalTypeMountAccess)
	elements = append(elements,
		map[string]interface{}{"tag": "hr"},
		map[string]interface{}{
			"tag": "action",
			"actions": []interface{}{
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "✅ 批准 / Approve"},
					"type": "primary",
					"value": func() map[string]interface{} {
						value := map[string]interface{}{
							"type": "typed_approval", "action": "approve",
							"id": req.ID, "approval_type": req.Type,
						}
						if !req.TaskScoped {
							value["ttl"] = req.TTLMinutes
						}
						return value
					}(),
				},
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "❌ 拒绝 / Deny"},
					"type": "danger",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "deny",
						"id": req.ID, "approval_type": req.Type,
					},
				},
			},
		},
		map[string]interface{}{
			"tag": "note",
			"elements": []interface{}{
				map[string]interface{}{"tag": "plain_text", "content": func() string {
					if req.TaskScoped {
						return fmt.Sprintf("ID: %s | 类型: %s | 范围: 当前任务", req.ID, req.Type)
					}
					return fmt.Sprintf("ID: %s | 类型: %s | 超时: %d 分钟", req.ID, req.Type, req.TTLMinutes)
				}()},
			},
		},
	)

	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "📁 挂载访问审批 / Mount Access"},
			"template": "orange",
		},
		"elements": elements,
	}
}

// P6-6: buildTypedDataExportCard 构建数据导出卡片（紫色主题，展示目标频道 + 文件 + 脱敏）。
func (p *feishuProvider) buildTypedDataExportCard(req TypedApprovalRequest) map[string]interface{} {
	filesText := ""
	for i, f := range req.ExportFiles {
		if i >= 5 {
			filesText += fmt.Sprintf("\n...（共 %d 个文件）", len(req.ExportFiles))
			break
		}
		filesText += fmt.Sprintf("\n• %s", f)
	}
	if filesText == "" {
		filesText = "\n（未指定文件）"
	}

	sanitizedLabel := "❌ 未脱敏 / Not Sanitized"
	if req.Sanitized {
		sanitizedLabel = "✅ 已脱敏 / Sanitized"
	}

	targetLabel := req.TargetChannel
	if targetLabel == "" {
		targetLabel = "未指定"
	}

	bodyContent := fmt.Sprintf("**目标频道**: %s\n**脱敏状态**: %s\n**授权时长**: %d 分钟\n\n**导出文件**:%s",
		targetLabel, sanitizedLabel, req.TTLMinutes, filesText)
	bodyContent += fmt.Sprintf("\n\n**原因**: %s", req.Reason)

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyContent},
		},
	}
	elements = appendWorkflowElements(elements, req.Workflow, ApprovalTypeDataExport)
	elements = append(elements,
		map[string]interface{}{"tag": "hr"},
		map[string]interface{}{
			"tag": "action",
			"actions": []interface{}{
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "✅ 批准 / Approve"},
					"type": "primary",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "approve",
						"id": req.ID, "approval_type": req.Type, "ttl": req.TTLMinutes,
					},
				},
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "❌ 拒绝 / Deny"},
					"type": "danger",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "deny",
						"id": req.ID, "approval_type": req.Type,
					},
				},
			},
		},
		map[string]interface{}{
			"tag": "note",
			"elements": []interface{}{
				map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s | 超时: %d 分钟", req.ID, req.Type, req.TTLMinutes)},
			},
		},
	)

	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "📤 数据导出审批 / Data Export"},
			"template": "purple",
		},
		"elements": elements,
	}
}

func (p *feishuProvider) buildTypedResultReviewCard(req TypedApprovalRequest) map[string]interface{} {
	resultText := req.ResultSummary
	if len([]rune(resultText)) > 220 {
		resultText = string([]rune(resultText)[:220]) + "..."
	}
	bodyContent := fmt.Sprintf("**结果摘要**: %s", resultText)
	if strings.TrimSpace(req.ReviewSummary) != "" {
		bodyContent += fmt.Sprintf("\n\n**审核摘要**: %s", req.ReviewSummary)
	}
	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyContent},
		},
	}
	elements = appendWorkflowElements(elements, req.Workflow, ApprovalTypeResultReview)
	elements = append(elements,
		map[string]interface{}{"tag": "hr"},
		map[string]interface{}{
			"tag": "action",
			"actions": []interface{}{
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "✅ 签收 / Approve"},
					"type": "primary",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "approve",
						"id": req.ID, "approval_type": req.Type,
					},
				},
				map[string]interface{}{
					"tag":  "button",
					"text": map[string]interface{}{"tag": "plain_text", "content": "❌ 退回 / Reject"},
					"type": "danger",
					"value": map[string]interface{}{
						"type": "typed_approval", "action": "reject",
						"id": req.ID, "approval_type": req.Type,
					},
				},
			},
		},
		map[string]interface{}{
			"tag": "note",
			"elements": []interface{}{
				map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s | 超时: %d 分钟", req.ID, req.Type, req.TTLMinutes)},
			},
		},
	)
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": "🧾 结果签收 / Result Review"},
			"template": "blue",
		},
		"elements": elements,
	}
}

func (p *feishuProvider) buildTypedPlanConfirmResultCard(result TypedApprovalResultNotification) map[string]interface{} {
	headerTitle := "📋 方案确认已处理 / Plan Processed"
	headerTemplate := "grey"
	bodyText := "方案确认已处理。"
	if result.Approved {
		headerTitle = "✅ 方案已批准 / Plan Approved"
		headerTemplate = "green"
		bodyText = "方案确认已批准，任务将继续执行。"
	} else if result.Reason != "" {
		bodyText = fmt.Sprintf("方案确认未通过。\n\n**原因**: %s", result.Reason)
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyText},
		},
	}
	elements = appendWorkflowElements(elements, result.Workflow, ApprovalTypePlanConfirm)
	elements = append(elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s", result.ID, result.Type)},
		},
	})
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": headerTitle},
			"template": headerTemplate,
		},
		"elements": elements,
	}
}

func (p *feishuProvider) buildTypedExecEscalationResultCard(result TypedApprovalResultNotification) map[string]interface{} {
	headerTitle := "❌ 权限提升已拒绝 / Permission Denied"
	headerTemplate := "red"
	bodyText := "权限提升请求未通过。"
	if result.Approved {
		headerTitle = "✅ 权限已生效 / Permission Granted"
		headerTemplate = "green"
		if isPermanentApprovalLevel(result.RequestedLevel) {
			bodyText = fmt.Sprintf("权限提升请求已批准。\n\n**授权级别**: %s\n**授权模式**: 永久授权 / Permanent", result.RequestedLevel)
		} else {
			bodyText = fmt.Sprintf("权限提升请求已批准。\n\n**授权级别**: %s\n**有效时长**: %d 分钟", result.RequestedLevel, result.TTLMinutes)
		}
	} else if result.Reason != "" {
		bodyText = fmt.Sprintf("权限提升请求未通过。\n\n**原因**: %s", result.Reason)
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyText},
		},
	}
	elements = appendWorkflowElements(elements, result.Workflow, ApprovalTypeExecEscalation)
	elements = append(elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s", result.ID, result.Type)},
		},
	})
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": headerTitle},
			"template": headerTemplate,
		},
		"elements": elements,
	}
}

func (p *feishuProvider) buildTypedMountAccessResultCard(result TypedApprovalResultNotification) map[string]interface{} {
	modeLabel := map[string]string{
		"ro": "🔒 只读 / Read-Only",
		"rw": "🔓 读写 / Read-Write",
	}[result.MountMode]
	if modeLabel == "" {
		modeLabel = "🔒 只读 / Read-Only"
	}

	headerTitle := "❌ 挂载访问已拒绝 / Mount Access Denied"
	headerTemplate := "red"
	bodyText := fmt.Sprintf("挂载访问请求未通过。\n\n**挂载路径**: `%s`\n**访问模式**: %s", result.MountPath, modeLabel)
	if result.Approved {
		headerTitle = "✅ 挂载访问已生效 / Mount Access Granted"
		headerTemplate = "green"
		scopeLine := fmt.Sprintf("**有效时长**: %d 分钟", result.TTLMinutes)
		if result.TaskScoped {
			scopeLine = "**生效范围**: 当前任务（任务结束自动回收）"
		}
		bodyText = fmt.Sprintf("挂载访问请求已批准。\n\n**挂载路径**: `%s`\n**访问模式**: %s\n%s", result.MountPath, modeLabel, scopeLine)
	} else if result.Reason != "" {
		bodyText += fmt.Sprintf("\n**原因**: %s", result.Reason)
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyText},
		},
	}
	elements = appendWorkflowElements(elements, result.Workflow, ApprovalTypeMountAccess)
	elements = append(elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s", result.ID, result.Type)},
		},
	})
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": headerTitle},
			"template": headerTemplate,
		},
		"elements": elements,
	}
}

func (p *feishuProvider) buildTypedDataExportResultCard(result TypedApprovalResultNotification) map[string]interface{} {
	filesText := ""
	for i, f := range result.ExportFiles {
		if i >= 5 {
			filesText += fmt.Sprintf("\n...（共 %d 个文件）", len(result.ExportFiles))
			break
		}
		filesText += fmt.Sprintf("\n• %s", f)
	}
	if filesText == "" {
		filesText = "\n（未指定文件）"
	}

	targetLabel := result.TargetChannel
	if targetLabel == "" {
		targetLabel = "未指定"
	}

	headerTitle := "❌ 数据导出已拒绝 / Data Export Denied"
	headerTemplate := "red"
	bodyText := fmt.Sprintf("数据导出请求未通过。\n\n**目标频道**: %s\n**导出文件**:%s", targetLabel, filesText)
	if result.Approved {
		headerTitle = "✅ 数据导出已批准 / Data Export Approved"
		headerTemplate = "green"
		bodyText = fmt.Sprintf("数据导出请求已批准。\n\n**目标频道**: %s\n**导出文件**:%s", targetLabel, filesText)
	} else if result.Reason != "" {
		bodyText += fmt.Sprintf("\n\n**原因**: %s", result.Reason)
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyText},
		},
	}
	elements = appendWorkflowElements(elements, result.Workflow, ApprovalTypeDataExport)
	elements = append(elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s", result.ID, result.Type)},
		},
	})
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": headerTitle},
			"template": headerTemplate,
		},
		"elements": elements,
	}
}

func (p *feishuProvider) buildTypedResultReviewResultCard(result TypedApprovalResultNotification) map[string]interface{} {
	headerTitle := "❌ 结果已退回 / Result Rejected"
	headerTemplate := "red"
	bodyText := "最终结果签收未通过。"
	if result.Approved {
		headerTitle = "✅ 结果已签收 / Result Approved"
		headerTemplate = "green"
		bodyText = "最终结果已签收，任务可以视为完成。"
	} else if result.Reason != "" {
		bodyText = fmt.Sprintf("最终结果签收未通过。\n\n**原因**: %s", result.Reason)
	}
	elements := []interface{}{
		map[string]interface{}{
			"tag":  "div",
			"text": map[string]interface{}{"tag": "lark_md", "content": bodyText},
		},
	}
	elements = appendWorkflowElements(elements, result.Workflow, ApprovalTypeResultReview)
	elements = append(elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{"tag": "plain_text", "content": fmt.Sprintf("ID: %s | 类型: %s", result.ID, result.Type)},
		},
	})
	return map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title":    map[string]interface{}{"tag": "plain_text", "content": headerTitle},
			"template": headerTemplate,
		},
		"elements": elements,
	}
}
