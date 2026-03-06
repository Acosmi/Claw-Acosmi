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
	"sync"
	"time"
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
							"content": fmt.Sprintf("**授权时长**\n%d 分钟", req.TTLMinutes),
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
						"type": "primary",
						"value": map[string]interface{}{
							"action": "approve",
							"id":     req.EscalationID,
							"ttl":    req.TTLMinutes,
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
		bodyText = fmt.Sprintf("权限提升请求已批准。\n\n"+
			"**授权级别**: %s\n"+
			"**有效时长**: %d 分钟\n\n"+
			"权限到期后将自动降级。",
			result.RequestedLevel, result.TTLMinutes)
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
					"content": fmt.Sprintf("**任务**: %s\n**意图级别**: %s\n\n**执行方案**:%s", brief, req.IntentTier, stepsText),
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
