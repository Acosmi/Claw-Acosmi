package gateway

// server_methods_remote_approval.go — P4 远程审批 API 方法处理器
//
// 方法:
//   - security.remoteApproval.config.get  — 获取远程审批配置（脱敏）
//   - security.remoteApproval.config.set  — 更新远程审批配置
//   - security.remoteApproval.test        — 发送测试卡片
//   - security.remoteApproval.callback    — HTTP Webhook 回调（外部平台审批结果）

import (
	"strings"
)

// RemoteApprovalHandlers 返回 security.remoteApproval.* 方法处理器映射。
func RemoteApprovalHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"security.remoteApproval.config.get": handleRemoteApprovalConfigGet,
		"security.remoteApproval.config.set": handleRemoteApprovalConfigSet,
		"security.remoteApproval.test":       handleRemoteApprovalTest,
		"security.remoteApproval.callback":   handleRemoteApprovalCallback,
	}
}

// ---------- security.remoteApproval.config.get ----------

func handleRemoteApprovalConfigGet(ctx *MethodHandlerContext) {
	notifier := ctx.Context.RemoteApprovalNotifier
	if notifier == nil {
		ctx.Respond(true, map[string]interface{}{
			"enabled":   false,
			"providers": []string{},
		}, nil)
		return
	}

	cfg := notifier.GetConfigSanitized()
	ctx.Respond(true, map[string]interface{}{
		"enabled":          cfg.Enabled,
		"callbackUrl":      cfg.CallbackURL,
		"feishu":           cfg.Feishu,
		"dingtalk":         cfg.DingTalk,
		"wecom":            cfg.WeCom,
		"enabledProviders": notifier.EnabledProviderNames(),
	}, nil)
}

// ---------- security.remoteApproval.config.set ----------

func handleRemoteApprovalConfigSet(ctx *MethodHandlerContext) {
	notifier := ctx.Context.RemoteApprovalNotifier
	if notifier == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "remote approval notifier not initialized"))
		return
	}

	var cfg RemoteApprovalConfig

	if v, ok := ctx.Params["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := ctx.Params["callbackUrl"].(string); ok {
		cfg.CallbackURL = strings.TrimSpace(v)
	}

	// 解析各平台配置
	if feishuRaw, ok := ctx.Params["feishu"].(map[string]interface{}); ok {
		cfg.Feishu = parseFeishuConfig(feishuRaw)
	}
	if dingtalkRaw, ok := ctx.Params["dingtalk"].(map[string]interface{}); ok {
		cfg.DingTalk = parseDingTalkConfig(dingtalkRaw)
	}
	if wecomRaw, ok := ctx.Params["wecom"].(map[string]interface{}); ok {
		cfg.WeCom = parseWeComConfig(wecomRaw)
	}

	if err := notifier.UpdateConfig(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"status":           "saved",
		"enabledProviders": notifier.EnabledProviderNames(),
	}, nil)
}

// ---------- security.remoteApproval.test ----------

func handleRemoteApprovalTest(ctx *MethodHandlerContext) {
	notifier := ctx.Context.RemoteApprovalNotifier
	if notifier == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "remote approval notifier not initialized"))
		return
	}

	provider, _ := ctx.Params["provider"].(string)
	provider = strings.TrimSpace(provider)
	if provider == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "provider is required"))
		return
	}

	if err := notifier.TestProvider(provider); err != nil {
		ctx.Respond(false, nil, mapSendErrorToShape(err))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"status":   "success",
		"provider": provider,
	}, nil)
}

// ---------- security.remoteApproval.callback ----------
// 外部平台回调：接收审批结果，调用 EscalationManager.ResolveEscalation()。

func handleRemoteApprovalCallback(ctx *MethodHandlerContext) {
	escalationID, _ := ctx.Params["id"].(string)
	escalationID = strings.TrimSpace(escalationID)
	if escalationID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "escalation id is required"))
		return
	}

	action, _ := ctx.Params["action"].(string)
	action = strings.TrimSpace(strings.ToLower(action))

	approved := action == "approve"

	ttlMinutes := 0
	if ttlRaw, ok := ctx.Params["ttl"].(float64); ok && ttlRaw > 0 {
		ttlMinutes = int(ttlRaw)
	}

	mgr := ctx.Context.EscalationMgr
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "escalation manager not initialized"))
		return
	}

	// Fix 9: 验证 escalation ID 匹配当前 pending 请求
	pendingID := mgr.GetPendingID()
	if pendingID != "" && pendingID != escalationID {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "escalation ID mismatch: expected "+pendingID+", got "+escalationID))
		return
	}

	if err := mgr.ResolveEscalation(approved, ttlMinutes); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, err.Error()))
		return
	}

	status := "denied"
	if approved {
		status = "approved"
	}

	ctx.Respond(true, map[string]interface{}{
		"status":       status,
		"escalationId": escalationID,
		"source":       "remote_callback",
	}, nil)
}

// ---------- 配置解析辅助函数 ----------

func parseFeishuConfig(raw map[string]interface{}) *FeishuProviderConfig {
	cfg := &FeishuProviderConfig{}
	if v, ok := raw["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := raw["appId"].(string); ok {
		cfg.AppID = v
	}
	if v, ok := raw["appSecret"].(string); ok {
		cfg.AppSecret = v
	}
	if v, ok := raw["chatId"].(string); ok {
		cfg.ChatID = v
	}
	if v, ok := raw["userId"].(string); ok {
		cfg.UserID = v
	}
	if v, ok := raw["approvalChatId"].(string); ok {
		cfg.ApprovalChatID = v
	}
	if v, ok := raw["lastKnownChatId"].(string); ok {
		cfg.LastKnownChatID = v
	}
	if v, ok := raw["lastKnownUserId"].(string); ok {
		cfg.LastKnownUserID = v
	}
	return cfg
}

func parseDingTalkConfig(raw map[string]interface{}) *DingTalkProviderConfig {
	cfg := &DingTalkProviderConfig{}
	if v, ok := raw["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := raw["appKey"].(string); ok {
		cfg.AppKey = v
	}
	if v, ok := raw["appSecret"].(string); ok {
		cfg.AppSecret = v
	}
	if v, ok := raw["robotCode"].(string); ok {
		cfg.RobotCode = v
	}
	if v, ok := raw["webhookUrl"].(string); ok {
		cfg.WebhookURL = v
	}
	if v, ok := raw["webhookSecret"].(string); ok {
		cfg.WebhookSecret = v
	}
	return cfg
}

func parseWeComConfig(raw map[string]interface{}) *WeComProviderConfig {
	cfg := &WeComProviderConfig{}
	if v, ok := raw["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := raw["corpId"].(string); ok {
		cfg.CorpID = v
	}
	if v, ok := raw["agentId"].(float64); ok {
		cfg.AgentID = int(v)
	}
	if v, ok := raw["secret"].(string); ok {
		cfg.Secret = v
	}
	if v, ok := raw["toUser"].(string); ok {
		cfg.ToUser = v
	}
	if v, ok := raw["toParty"].(string); ok {
		cfg.ToParty = v
	}
	return cfg
}
