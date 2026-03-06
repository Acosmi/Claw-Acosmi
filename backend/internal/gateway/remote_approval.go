package gateway

// remote_approval.go — P4 远程审批通知器
// 行业对照: ServiceNow Mobile Approval / Slack Approval Workflow
//
// 当智能体请求提权时，通过外部消息平台（飞书/钉钉/企业微信）发送互动卡片，
// 用户在手机端审批/拒绝后通过 HTTP Webhook 回调通知 Gateway。
//
// 设计：
//   - Provider 接口：每个消息平台一个实现
//   - RemoteApprovalNotifier：持有多个 Provider，扇出通知
//   - 配置持久化到 ~/.openacosmi/remote-approval-config.json

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

// ---------- Provider 接口 ----------

// RemoteApprovalProvider 远程审批消息 Provider 接口。
// 每个外部消息平台（飞书/钉钉/企业微信）实现此接口。
type RemoteApprovalProvider interface {
	// Name 返回 Provider 名称（如 "feishu", "dingtalk", "wecom"）。
	Name() string
	// SendApprovalRequest 发送审批请求卡片到外部平台。
	SendApprovalRequest(ctx context.Context, req ApprovalCardRequest) error
	// ValidateConfig 验证当前 Provider 配置是否有效。
	ValidateConfig() error
}

// ApprovalCardRequest 审批卡片请求参数。
type ApprovalCardRequest struct {
	EscalationID   string    `json:"escalationId"`
	RequestedLevel string    `json:"requestedLevel"`
	Reason         string    `json:"reason"`
	RunID          string    `json:"runId,omitempty"`
	SessionID      string    `json:"sessionId,omitempty"`
	TTLMinutes     int       `json:"ttlMinutes"`
	CallbackURL    string    `json:"callbackUrl"`
	RequestedAt    time.Time `json:"requestedAt"`
	// OriginatorChatID 发起操作的群聊 ID（如飞书 chat_id），用于审批卡片群发。
	OriginatorChatID string `json:"originatorChatId,omitempty"`
	// OriginatorUserID 发起远程操作的用户 ID（如飞书 open_id），用于审批卡片私聊。
	OriginatorUserID string `json:"originatorUserId,omitempty"`
}

// ApprovalCallbackPayload 外部平台回调载荷。
type ApprovalCallbackPayload struct {
	EscalationID string `json:"escalationId"`
	Approved     bool   `json:"approved"`
	TTLMinutes   int    `json:"ttlMinutes,omitempty"`
	Provider     string `json:"provider"`
	ApproverID   string `json:"approverId,omitempty"`
	ApproverName string `json:"approverName,omitempty"`
}

// ---------- 远程审批配置 ----------

// RemoteApprovalConfig 远程审批全局配置。
type RemoteApprovalConfig struct {
	Enabled     bool                    `json:"enabled"`
	CallbackURL string                  `json:"callbackUrl"` // 公网可达的回调地址
	Feishu      *FeishuProviderConfig   `json:"feishu,omitempty"`
	DingTalk    *DingTalkProviderConfig `json:"dingtalk,omitempty"`
	WeCom       *WeComProviderConfig    `json:"wecom,omitempty"`
}

// FeishuProviderConfig 飞书 Provider 配置。
type FeishuProviderConfig struct {
	Enabled   bool   `json:"enabled"`
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
	ChatID    string `json:"chatId,omitempty"` // 目标群聊 ID
	UserID    string `json:"userId,omitempty"` // 目标用户 open_id
	// ApprovalChatID: 审批通知专用群聊 ID（固定审批群 fallback）。
	// 当 ChatID/UserID/LastKnown/Originator 均为空时（如 Web UI 发起的审批），
	// 使用此群 ID 作为最终 fallback，确保审批卡片始终有送达目标。
	ApprovalChatID string `json:"approvalChatId,omitempty"`
	// LastKnownChatID/LastKnownUserID: 运行时从飞书消息事件自动学习并持久化。
	// 当静态 ChatID/UserID 为空且 OriginatorChatID/UserID 也为空时，
	// 使用这些值作为 fallback 目标，确保重启后审批通知仍可送达。
	LastKnownChatID string `json:"lastKnownChatId,omitempty"`
	LastKnownUserID string `json:"lastKnownUserId,omitempty"`
}

// DingTalkProviderConfig 钉钉 Provider 配置。
type DingTalkProviderConfig struct {
	Enabled       bool   `json:"enabled"`
	AppKey        string `json:"appKey"`
	AppSecret     string `json:"appSecret"`
	RobotCode     string `json:"robotCode,omitempty"`
	WebhookURL    string `json:"webhookUrl,omitempty"` // 自定义机器人 Webhook
	WebhookSecret string `json:"webhookSecret,omitempty"`
	ApiSecret     string `json:"apiSecret,omitempty"` // Phase 8: 互动卡片回调验签
}

// WeComProviderConfig 企业微信 Provider 配置。
type WeComProviderConfig struct {
	Enabled        bool   `json:"enabled"`
	CorpID         string `json:"corpId"`
	AgentID        int    `json:"agentId"`
	Secret         string `json:"secret"`
	ToUser         string `json:"toUser,omitempty"`         // 接收用户 ID（| 分隔）
	ToParty        string `json:"toParty,omitempty"`        // 接收部门 ID
	Token          string `json:"token,omitempty"`          // Phase 8: 回调签名 Token
	EncodingAESKey string `json:"encodingAESKey,omitempty"` // Phase 8: AES 解密密钥
}

const remoteApprovalConfigFile = "remote-approval-config.json"

// ---------- 远程审批通知管理器 ----------

// RemoteApprovalNotifier 远程审批通知管理器。
// 持有多个 Provider，提权请求时扇出通知到所有已启用的平台。
type RemoteApprovalNotifier struct {
	mu          sync.RWMutex
	config      RemoteApprovalConfig
	providers   []RemoteApprovalProvider
	logger      *slog.Logger
	broadcaster *Broadcaster // 发送失败时广播通知前端
}

// NewRemoteApprovalNotifier 创建远程审批通知管理器。
// 自动加载配置并初始化已启用的 Provider。
func NewRemoteApprovalNotifier(broadcaster *Broadcaster) *RemoteApprovalNotifier {
	n := &RemoteApprovalNotifier{
		logger:      slog.Default().With("component", "remote-approval"),
		broadcaster: broadcaster,
	}
	// 加载配置
	if err := n.loadConfig(); err != nil {
		n.logger.Warn("加载远程审批配置失败，使用默认配置", "error", err)
	}
	n.rebuildProviders()
	return n
}

// InjectChannelFeishuConfig 从频道配置自动补充飞书审批凭据。
// 当 remote-approval-config.json 未配置飞书时，复用频道插件的 AppID/AppSecret。
// approvalChatID: 审批通知专用群 ID（可选，为空则依赖 LastKnown 或 Originator）。
// 由 server.go 在频道插件初始化后调用。
func (n *RemoteApprovalNotifier) InjectChannelFeishuConfig(appID, appSecret, approvalChatID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 如果已有专用飞书审批配置且已启用，不覆盖（保留已持久化的 LastKnown 字段）
	if n.config.Feishu != nil && n.config.Feishu.Enabled && n.config.Feishu.AppID != "" {
		n.logger.Debug("飞书审批已有专用配置，跳过频道凭据注入")
		return
	}

	n.logger.Info("从飞书频道配置自动注入审批凭据",
		"appId", appID[:min(4, len(appID))]+"***",
	)

	// 保留可能已持久化的 LastKnown 值和已有的 ApprovalChatID
	var lastChatID, lastUserID, existingApprovalChatID string
	if n.config.Feishu != nil {
		lastChatID = n.config.Feishu.LastKnownChatID
		lastUserID = n.config.Feishu.LastKnownUserID
		existingApprovalChatID = n.config.Feishu.ApprovalChatID
	}
	// 新注入的 approvalChatID 优先，其次保留已有值
	if approvalChatID == "" {
		approvalChatID = existingApprovalChatID
	}

	n.config.Feishu = &FeishuProviderConfig{
		Enabled:         true,
		AppID:           appID,
		AppSecret:       appSecret,
		ApprovalChatID:  approvalChatID,
		LastKnownChatID: lastChatID,
		LastKnownUserID: lastUserID,
	}
	n.config.Enabled = true
	n.rebuildProviders()
}

// UpdateLastKnownFeishuTarget 当收到飞书消息事件时调用，
// 持久化最近的 chatID/userID 到配置文件，确保重启后审批通知仍可送达。
// 幂等：只在值发生变化时写入磁盘。
func (n *RemoteApprovalNotifier) UpdateLastKnownFeishuTarget(chatID, userID string) {
	if chatID == "" && userID == "" {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.config.Feishu == nil {
		return
	}

	// 检查是否有变化
	changed := false
	if chatID != "" && n.config.Feishu.LastKnownChatID != chatID {
		n.config.Feishu.LastKnownChatID = chatID
		changed = true
	}
	if userID != "" && n.config.Feishu.LastKnownUserID != userID {
		n.config.Feishu.LastKnownUserID = userID
		changed = true
	}

	if !changed {
		return
	}

	n.logger.Info("持久化飞书审批目标",
		"lastKnownChatId", chatID,
		"lastKnownUserId", userID,
	)

	// 重建 provider（让新的 LastKnown 值在 broadcastCard 中生效）
	n.rebuildProviders()

	if err := n.saveConfigLocked(); err != nil {
		n.logger.Error("持久化飞书审批目标失败", "error", err)
	}
}

// NotifyAll 向所有已启用的 Provider 发送审批通知。
// 不阻塞调用方——异步发送，收集错误日志。
func (n *RemoteApprovalNotifier) NotifyAll(req ApprovalCardRequest) {
	n.mu.RLock()
	if !n.config.Enabled || len(n.providers) == 0 {
		n.mu.RUnlock()
		return
	}
	providers := make([]RemoteApprovalProvider, len(n.providers))
	copy(providers, n.providers)
	n.mu.RUnlock()

	// 填充回调 URL
	if req.CallbackURL == "" {
		n.mu.RLock()
		req.CallbackURL = n.config.CallbackURL
		n.mu.RUnlock()
	}

	for _, p := range providers {
		go func(prov RemoteApprovalProvider) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := prov.SendApprovalRequest(ctx, req); err != nil {
				n.logger.Error("远程审批通知发送失败",
					"provider", prov.Name(),
					"escalation_id", req.EscalationID,
					"error", err,
				)
				// 广播发送失败事件，让前端知道远程审批卡片未送达
				if n.broadcaster != nil {
					n.broadcaster.Broadcast("remote.approval.sendFailed", map[string]interface{}{
						"provider":     prov.Name(),
						"escalationId": req.EscalationID,
						"error":        err.Error(),
					}, nil)
				}
			} else {
				n.logger.Info("远程审批通知已发送",
					"provider", prov.Name(),
					"escalation_id", req.EscalationID,
				)
			}
		}(p)
	}
}

// ---------- Phase 8: 审批结果通知 ----------

// ApprovalResultNotification 审批结果通知参数。
type ApprovalResultNotification struct {
	EscalationID   string `json:"escalationId"`
	Approved       bool   `json:"approved"`
	Reason         string `json:"reason"`         // 拒绝原因（超时/手动拒绝）
	RequestedLevel string `json:"requestedLevel"` // 请求的级别
	TTLMinutes     int    `json:"ttlMinutes"`     // 批准时的授权时长
}

// ResultNotifier 可选接口——Provider 实现后可推送审批结果卡片。
type ResultNotifier interface {
	SendResultNotification(ctx context.Context, result ApprovalResultNotification) error
}

// NotifyResult 审批结束后向所有已启用的平台推送结果卡片。
// 使用 type assertion 检测 Provider 是否支持结果通知。
func (n *RemoteApprovalNotifier) NotifyResult(result ApprovalResultNotification) {
	n.mu.RLock()
	if !n.config.Enabled || len(n.providers) == 0 {
		n.mu.RUnlock()
		return
	}
	providers := make([]RemoteApprovalProvider, len(n.providers))
	copy(providers, n.providers)
	n.mu.RUnlock()

	for _, p := range providers {
		rn, ok := p.(ResultNotifier)
		if !ok {
			continue
		}
		go func(notifier ResultNotifier, name string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := notifier.SendResultNotification(ctx, result); err != nil {
				n.logger.Error("审批结果通知发送失败",
					"provider", name,
					"escalation_id", result.EscalationID,
					"error", err,
				)
			} else {
				n.logger.Info("审批结果通知已发送",
					"provider", name,
					"escalation_id", result.EscalationID,
					"approved", result.Approved,
				)
			}
		}(rn, p.Name())
	}
}

// ---------- CoderConfirmation 远程通知 ----------

// CoderConfirmCardRequest 操作确认卡片请求参数。
type CoderConfirmCardRequest struct {
	ConfirmID        string `json:"confirmId"`
	ToolName         string `json:"toolName"`
	Preview          string `json:"preview"`                    // 命令/文件预览文本
	SessionKey       string `json:"sessionKey"`                 // 来源会话标识
	OriginatorChatID string `json:"originatorChatId,omitempty"` // 飞书 chat_id
	OriginatorUserID string `json:"originatorUserId,omitempty"` // D5-F1: 飞书 open_id，用于私聊推送审批卡片
	TTLMinutes       int    `json:"ttlMinutes"`                 // 超时分钟数
}

// CoderConfirmNotifier 可选接口——Provider 实现后可推送操作确认卡片。
type CoderConfirmNotifier interface {
	SendCoderConfirmRequest(ctx context.Context, req CoderConfirmCardRequest) error
	SendCoderConfirmResult(ctx context.Context, id string, approved bool) error
}

// NotifyCoderConfirm 向所有已启用的 Provider 发送操作确认通知。
// 使用 type assertion 检测 Provider 是否支持操作确认。
func (n *RemoteApprovalNotifier) NotifyCoderConfirm(req CoderConfirmCardRequest) {
	n.mu.RLock()
	if !n.config.Enabled || len(n.providers) == 0 {
		n.mu.RUnlock()
		return
	}
	providers := make([]RemoteApprovalProvider, len(n.providers))
	copy(providers, n.providers)
	n.mu.RUnlock()

	for _, p := range providers {
		cn, ok := p.(CoderConfirmNotifier)
		if !ok {
			continue
		}
		go func(notifier CoderConfirmNotifier, name string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := notifier.SendCoderConfirmRequest(ctx, req); err != nil {
				n.logger.Error("操作确认通知发送失败",
					"provider", name,
					"confirmId", req.ConfirmID,
					"error", err,
				)
			} else {
				n.logger.Info("操作确认通知已发送",
					"provider", name,
					"confirmId", req.ConfirmID,
					"tool", req.ToolName,
				)
			}
		}(cn, p.Name())
	}
}

// NotifyCoderConfirmResult 向所有已启用的 Provider 推送操作确认结果。
func (n *RemoteApprovalNotifier) NotifyCoderConfirmResult(id string, approved bool) {
	n.mu.RLock()
	if !n.config.Enabled || len(n.providers) == 0 {
		n.mu.RUnlock()
		return
	}
	providers := make([]RemoteApprovalProvider, len(n.providers))
	copy(providers, n.providers)
	n.mu.RUnlock()

	for _, p := range providers {
		cn, ok := p.(CoderConfirmNotifier)
		if !ok {
			continue
		}
		go func(notifier CoderConfirmNotifier, name string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := notifier.SendCoderConfirmResult(ctx, id, approved); err != nil {
				n.logger.Error("操作确认结果通知失败",
					"provider", name,
					"confirmId", id,
					"error", err,
				)
			}
		}(cn, p.Name())
	}
}

// ---------- PlanConfirmation 方案确认卡片 ----------

// PlanConfirmCardRequest 方案确认卡片请求参数。
type PlanConfirmCardRequest struct {
	ConfirmID        string   `json:"confirmId"`
	TaskBrief        string   `json:"taskBrief"`
	PlanSteps        []string `json:"planSteps"`
	IntentTier       string   `json:"intentTier"`
	SessionKey       string   `json:"sessionKey"`                 // 来源会话标识
	OriginatorChatID string   `json:"originatorChatId,omitempty"` // 飞书 chat_id
	OriginatorUserID string   `json:"originatorUserId,omitempty"` // 飞书 open_id
	TTLMinutes       int      `json:"ttlMinutes"`
}

// PlanConfirmNotifier 可选接口——Provider 实现后可推送方案确认卡片。
type PlanConfirmNotifier interface {
	SendPlanConfirmRequest(ctx context.Context, req PlanConfirmCardRequest) error
	SendPlanConfirmResult(ctx context.Context, id string, decision string) error
}

// NotifyPlanConfirm 向所有已启用的 Provider 发送方案确认通知。
func (n *RemoteApprovalNotifier) NotifyPlanConfirm(req PlanConfirmCardRequest) {
	n.mu.RLock()
	if !n.config.Enabled || len(n.providers) == 0 {
		n.mu.RUnlock()
		return
	}
	providers := make([]RemoteApprovalProvider, len(n.providers))
	copy(providers, n.providers)
	n.mu.RUnlock()

	for _, p := range providers {
		pn, ok := p.(PlanConfirmNotifier)
		if !ok {
			continue
		}
		go func(notifier PlanConfirmNotifier, name string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := notifier.SendPlanConfirmRequest(ctx, req); err != nil {
				n.logger.Error("方案确认通知发送失败",
					"provider", name,
					"confirmId", req.ConfirmID,
					"error", err,
				)
			} else {
				n.logger.Info("方案确认通知已发送",
					"provider", name,
					"confirmId", req.ConfirmID,
					"taskBrief", req.TaskBrief,
				)
			}
		}(pn, p.Name())
	}
}

// NotifyPlanConfirmResult 向所有已启用的 Provider 推送方案确认结果。
func (n *RemoteApprovalNotifier) NotifyPlanConfirmResult(id string, decision string) {
	n.mu.RLock()
	if !n.config.Enabled || len(n.providers) == 0 {
		n.mu.RUnlock()
		return
	}
	providers := make([]RemoteApprovalProvider, len(n.providers))
	copy(providers, n.providers)
	n.mu.RUnlock()

	for _, p := range providers {
		pn, ok := p.(PlanConfirmNotifier)
		if !ok {
			continue
		}
		go func(notifier PlanConfirmNotifier, name string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := notifier.SendPlanConfirmResult(ctx, id, decision); err != nil {
				n.logger.Error("方案确认结果通知失败",
					"provider", name,
					"confirmId", id,
					"error", err,
				)
			}
		}(pn, p.Name())
	}
}

// GetConfig 获取当前配置（脱敏）。
func (n *RemoteApprovalNotifier) GetConfig() RemoteApprovalConfig {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config
}

// GetConfigSanitized 获取脱敏后的配置（隐藏 secret 字段），用于前端展示。
func (n *RemoteApprovalNotifier) GetConfigSanitized() RemoteApprovalConfig {
	n.mu.RLock()
	defer n.mu.RUnlock()

	cfg := n.config

	if cfg.Feishu != nil {
		copy := *cfg.Feishu
		if copy.AppSecret != "" {
			copy.AppSecret = "***"
		}
		cfg.Feishu = &copy
	}
	if cfg.DingTalk != nil {
		copy := *cfg.DingTalk
		if copy.AppSecret != "" {
			copy.AppSecret = "***"
		}
		if copy.WebhookSecret != "" {
			copy.WebhookSecret = "***"
		}
		cfg.DingTalk = &copy
	}
	if cfg.WeCom != nil {
		copy := *cfg.WeCom
		if copy.Secret != "" {
			copy.Secret = "***"
		}
		cfg.WeCom = &copy
	}
	return cfg
}

// UpdateConfig 更新配置，保存到磁盘并重建 Provider。
func (n *RemoteApprovalNotifier) UpdateConfig(cfg RemoteApprovalConfig) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 如果前端传回 "***"，保留原有 secret
	if cfg.Feishu != nil && n.config.Feishu != nil && cfg.Feishu.AppSecret == "***" {
		cfg.Feishu.AppSecret = n.config.Feishu.AppSecret
	}
	if cfg.Feishu != nil && n.config.Feishu != nil {
		// 保留 fallback 路由字段，避免前端未回传时被整体覆盖清空。
		if strings.TrimSpace(cfg.Feishu.ApprovalChatID) == "" {
			cfg.Feishu.ApprovalChatID = n.config.Feishu.ApprovalChatID
		}
		if strings.TrimSpace(cfg.Feishu.LastKnownChatID) == "" {
			cfg.Feishu.LastKnownChatID = n.config.Feishu.LastKnownChatID
		}
		if strings.TrimSpace(cfg.Feishu.LastKnownUserID) == "" {
			cfg.Feishu.LastKnownUserID = n.config.Feishu.LastKnownUserID
		}
	}
	if cfg.DingTalk != nil && n.config.DingTalk != nil {
		if cfg.DingTalk.AppSecret == "***" {
			cfg.DingTalk.AppSecret = n.config.DingTalk.AppSecret
		}
		if cfg.DingTalk.WebhookSecret == "***" {
			cfg.DingTalk.WebhookSecret = n.config.DingTalk.WebhookSecret
		}
	}
	if cfg.WeCom != nil && n.config.WeCom != nil && cfg.WeCom.Secret == "***" {
		cfg.WeCom.Secret = n.config.WeCom.Secret
	}

	n.config = cfg
	if err := n.saveConfigLocked(); err != nil {
		return fmt.Errorf("保存远程审批配置失败: %w", err)
	}
	n.rebuildProviders()
	return nil
}

// TestProvider 测试指定 Provider 是否可正常发送消息。
func (n *RemoteApprovalNotifier) TestProvider(providerName string) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, p := range n.providers {
		if p.Name() == providerName {
			channelID := remoteApprovalProviderChannelID(providerName)
			if err := p.ValidateConfig(); err != nil {
				return channels.NewSendError(channelID, channels.SendErrInvalidRequest,
					"remote approval config invalid: "+err.Error()).
					WithOperation("remote_approval.validate").
					WithDetails(map[string]interface{}{
						"provider": providerName,
					})
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := p.SendApprovalRequest(ctx, ApprovalCardRequest{
				EscalationID:   "test_" + fmt.Sprintf("%d", time.Now().UnixMilli()),
				RequestedLevel: "full",
				Reason:         "测试远程审批连接 / Test remote approval connection",
				TTLMinutes:     30,
				CallbackURL:    n.config.CallbackURL,
				RequestedAt:    time.Now(),
			}); err != nil {
				if _, ok := channels.AsSendError(err); ok {
					return err
				}
				return channels.WrapSendError(channelID, channels.SendErrUpstream,
					"remote_approval.test.send",
					"remote approval test send failed", err).
					WithRetryable(true).
					WithDetails(map[string]interface{}{
						"provider": providerName,
					})
			}
			return nil
		}
	}
	return channels.NewSendError(remoteApprovalProviderChannelID(providerName), channels.SendErrInvalidTarget,
		fmt.Sprintf("provider %q 未启用或不存在", providerName)).
		WithOperation("remote_approval.resolve_provider").
		WithDetails(map[string]interface{}{
			"provider": providerName,
		})
}

func remoteApprovalProviderChannelID(providerName string) channels.ChannelID {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "feishu", "lark":
		return channels.ChannelFeishu
	case "dingtalk":
		return channels.ChannelDingTalk
	case "wecom", "wechatwork", "workweixin":
		return channels.ChannelWeCom
	default:
		if trimmed := strings.TrimSpace(providerName); trimmed != "" {
			return channels.ChannelID(trimmed)
		}
		return channels.ChannelID("remote_approval")
	}
}

// EnabledProviderNames 返回当前已启用的 Provider 名称列表。
func (n *RemoteApprovalNotifier) EnabledProviderNames() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	names := make([]string, 0, len(n.providers))
	for _, p := range n.providers {
		names = append(names, p.Name())
	}
	return names
}

// ---------- 配置持久化 ----------

func (n *RemoteApprovalNotifier) configFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".openacosmi", remoteApprovalConfigFile)
}

func (n *RemoteApprovalNotifier) loadConfig() error {
	data, err := os.ReadFile(n.configFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 无配置文件，使用默认值
		}
		return err
	}
	return json.Unmarshal(data, &n.config)
}

func (n *RemoteApprovalNotifier) saveConfigLocked() error {
	filePath := n.configFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(n.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0o600)
}

// ---------- Provider 构建 ----------

func (n *RemoteApprovalNotifier) rebuildProviders() {
	n.providers = nil

	if n.config.Feishu != nil && n.config.Feishu.Enabled {
		n.providers = append(n.providers, newFeishuProvider(n.config.Feishu))
	}
	if n.config.DingTalk != nil && n.config.DingTalk.Enabled {
		n.providers = append(n.providers, newDingTalkProvider(n.config.DingTalk))
	}
	if n.config.WeCom != nil && n.config.WeCom.Enabled {
		n.providers = append(n.providers, newWeComProvider(n.config.WeCom))
	}
}
