package gateway

// server_methods_channels.go — channels.status, channels.logout
// 对应 TS src/gateway/server-methods/channels.ts
//
// Phase 5: 当 ChannelManager 可用时，从运行时快照获取真实状态。
// 否则回退到从 config 检测配置状态。

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ChannelLogoutFunc DI 回调：实际执行频道 logout。
// 返回 (payload, error)。
type ChannelLogoutFunc func(channelID, accountID string) (map[string]interface{}, error)

// ChannelsHandlers 返回 channels.* 方法处理器映射。
func ChannelsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"channels.status": handleChannelsStatus,
		"channels.logout": handleChannelsLogout,
		"channels.save":   handleChannelsSave,
	}
}

// ---------- channels.save ----------
// 接收频道向导的凭证数据，写入配置文件并重启频道。
// 参数: { channelId: "feishu", config: { appId, appSecret, ... } }

func handleChannelsSave(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}
	rollback, rollbackErr := captureGatewayConfigRollback(loader)
	if rollbackErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to snapshot config rollback: "+rollbackErr.Error()))
		return
	}

	channelID := readString(ctx.Params, "channelId")
	if channelID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "channelId is required"))
		return
	}
	channelID = normalizeChannelID(channelID)
	if channelID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unknown channel"))
		return
	}

	configRaw, ok := ctx.Params["config"].(map[string]interface{})
	if !ok || len(configRaw) == 0 {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "config object is required"))
		return
	}

	// 读取当前配置
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// 确保 channels 层级存在
	if cfg.Channels == nil {
		cfg.Channels = &types.ChannelsConfig{}
	}

	// 辅助读取函数
	rs := func(key string) string {
		v, _ := configRaw[key].(string)
		return strings.TrimSpace(v)
	}

	// 按 channelID 映射到对应配置结构
	switch channelID {
	case "feishu":
		cfg.Channels.Feishu = &types.FeishuConfig{
			FeishuAccountConfig: types.FeishuAccountConfig{
				AppID:     rs("appId"),
				AppSecret: rs("appSecret"),
				Domain:    rs("domain"),
			},
		}
	case "dingtalk":
		cfg.Channels.DingTalk = &types.DingTalkConfig{
			DingTalkAccountConfig: types.DingTalkAccountConfig{
				AppKey:    rs("appKey"),
				AppSecret: rs("appSecret"),
				RobotCode: rs("robotCode"),
			},
		}
	case "wecom":
		var agentID *int
		if v := rs("agentId"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				agentID = &n
			}
		}
		cfg.Channels.WeCom = &types.WeComConfig{
			WeComAccountConfig: types.WeComAccountConfig{
				CorpID:  rs("corpId"),
				Secret:  rs("corpSecret"),
				AgentID: agentID,
				Token:   rs("token"),
				AESKey:  rs("encodingAESKey"),
			},
		}
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "channel wizard not supported for: "+channelID))
		return
	}

	// 持久化到磁盘
	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}
	loader.ClearCache()

	// 尝试重启频道：区分首次配置与热重载两条路径。
	// 插件型频道（飞书/钉钉/企微）在启动时通过 server.go 大闭包注册，
	// 若配置时插件未注册（即首次配置），无法热重载，需调度 Gateway 重启。
	// 若插件已注册，通过 ReloadChannel 注入新凭证后 Stop+Start，无需全量重启。
	needsRestart := false
	var restartResult *GatewayRestartResult
	if mgr := ctx.Context.ChannelMgr; mgr != nil {
		if mgr.HasPlugin(channels.ChannelID(channelID)) {
			// 插件已注册（曾经配置过）：注入新凭证 → Stop → Start，实现真正的凭证热重载。
			if err := mgr.ReloadChannel(channels.ChannelID(channelID), cfg, "default"); err != nil {
				slog.Warn("channels.save: hot reload failed", "channel", channelID, "error", err)
				needsRestart = true
				if gr := ctx.Context.GatewayRestarter; gr != nil {
					restartResult = gr.ScheduleRestart(GatewayRestartPlan{
						Reason:   "channels.save: hot reload fallback restart",
						Rollback: rollback,
					})
					slog.Info("channels.save: scheduling gateway restart after hot reload failure", "channel", channelID)
				}
			} else {
				slog.Info("channels.save: hot reload succeeded", "channel", channelID)
			}
		} else {
			// 插件未注册（首次配置），热重载无法工作，需全量重启
			needsRestart = true
			if gr := ctx.Context.GatewayRestarter; gr != nil {
				restartResult = gr.ScheduleRestart(GatewayRestartPlan{
					Reason:   "channels.save: new channel plugin requires gateway restart",
					Rollback: rollback,
				})
				slog.Info("channels.save: scheduling gateway restart for new channel", "channel", channelID)
			}
		}
	}

	slog.Info("channels.save: config saved", "channel", channelID)
	msg := "Channel configuration saved successfully"
	if needsRestart {
		msg = "Channel configuration saved. Gateway is restarting to activate the new channel."
	}
	ctx.Respond(true, map[string]interface{}{
		"ok":              true,
		"channel":         channelID,
		"message":         msg,
		"requiresRestart": needsRestart,
		"restart":         restartResult,
	}, nil)
}

// emailAccountHealthInfo 邮箱账号运行时状态（Phase 9 内部辅助）
type emailAccountHealthInfo struct {
	running   bool
	lastError string
}

// ---------- channels.status ----------
// 对应 TS channels.ts L69-236
// 返回频道状态快照（频道列表、UI 目录、账户状态）。
// 简化实现：从 config 读取已配置的频道列表。

func handleChannelsStatus(ctx *MethodHandlerContext) {
	// 优先从 ConfigLoader 读取最新配置（反映 config.set 的变更）
	var cfg *types.OpenAcosmiConfig
	if loader := ctx.Context.ConfigLoader; loader != nil {
		loaded, err := loader.LoadConfig()
		if err == nil {
			cfg = loaded
		}
	}
	// 回退到启动时的配置
	if cfg == nil {
		cfg = ctx.Context.Config
	}
	if cfg == nil {
		ctx.Respond(true, map[string]interface{}{
			"ts":                      time.Now().UnixMilli(),
			"channels":                map[string]interface{}{},
			"channelAccounts":         map[string]interface{}{},
			"channelDefaultAccountId": map[string]interface{}{},
			"channelOrder":            []string{},
			"channelLabels":           map[string]string{},
			"channelMeta":             []interface{}{},
		}, nil)
		return
	}

	// 构建频道顺序和标签
	channelOrder := []string{
		"email",
		"wecom", "dingtalk", "feishu",
		"telegram", "discord", "slack", "whatsapp", "signal", "imessage",
	}
	channelLabels := map[string]string{
		"telegram": "Telegram",
		"discord":  "Discord",
		"slack":    "Slack",
		"whatsapp": "WhatsApp",
		"signal":   "Signal",
		"imessage": "iMessage",
		"wecom":    "企业微信",
		"dingtalk": "钉钉",
		"feishu":   "飞书",
		"email":    "邮箱",
	}

	// Phase 5: 从 ChannelManager 获取运行时快照
	var runtimeSnap *channels.RuntimeSnapshot
	if mgr := ctx.Context.ChannelMgr; mgr != nil {
		runtimeSnap = mgr.GetSnapshot()
	}

	// 从 config 中检测已配置的频道，并合并运行时状态
	channelsMap := make(map[string]interface{})
	channelAccounts := make(map[string]interface{})
	channelDefaultAccountId := make(map[string]interface{})

	// 辅助函数：构建频道状态并合并运行时数据
	buildStatus := func(chID string, configured bool) {
		status := map[string]interface{}{"configured": configured}
		acctInfo := map[string]interface{}{
			"accountId":  "default",
			"configured": configured,
			"enabled":    true,
		}

		// 合并运行时快照
		if runtimeSnap != nil {
			if snap, ok := runtimeSnap.Channels[channels.ChannelID(chID)]; ok {
				running := snap.Status == "running"
				status["running"] = running
				status["connected"] = running
				acctInfo["running"] = running
				acctInfo["connected"] = running
				if snap.Error != "" {
					status["lastError"] = snap.Error
					acctInfo["lastError"] = snap.Error
				}
			}
		}

		channelsMap[chID] = status
		channelAccounts[chID] = []map[string]interface{}{acctInfo}
		channelDefaultAccountId[chID] = "default"
	}

	if cfg.Channels != nil {
		if cfg.Channels.Telegram != nil {
			buildStatus("telegram", cfg.Channels.Telegram.BotToken != "")
		}
		if cfg.Channels.Discord != nil {
			buildStatus("discord", cfg.Channels.Discord.DiscordAccountConfigToken() != "")
		}
		if cfg.Channels.Slack != nil {
			buildStatus("slack", cfg.Channels.Slack.BotToken != "")
		}
		if cfg.Channels.WhatsApp != nil {
			buildStatus("whatsapp", true)
		}
		if cfg.Channels.Feishu != nil {
			buildStatus("feishu", cfg.Channels.Feishu.AppID != "" && cfg.Channels.Feishu.AppSecret != "")
		}
		if cfg.Channels.DingTalk != nil {
			buildStatus("dingtalk", cfg.Channels.DingTalk.AppKey != "" && cfg.Channels.DingTalk.AppSecret != "")
		}
		if cfg.Channels.WeCom != nil {
			buildStatus("wecom", cfg.Channels.WeCom.CorpID != "" && cfg.Channels.WeCom.Secret != "")
		}

		// Email: 多账号模式 — 每个账号独立状态
		if cfg.Channels.Email != nil && channels.IsAccountEnabled(cfg.Channels.Email.Enabled) {
			emailCfg := cfg.Channels.Email
			configured := emailCfg.Accounts != nil && len(emailCfg.Accounts) > 0
			emailStatus := map[string]interface{}{"configured": configured}

			var emailAcctInfos []map[string]interface{}
			if configured {
				// 从运行时快照读取各账号状态
				var healthMap map[string]*emailAccountHealthInfo
				if runtimeSnap != nil {
					if accts, ok := runtimeSnap.Accounts[channels.ChannelEmail]; ok {
						healthMap = make(map[string]*emailAccountHealthInfo, len(accts))
						for acctID, snap := range accts {
							healthMap[acctID] = &emailAccountHealthInfo{
								running:   snap.Status == "running",
								lastError: snap.Error,
							}
						}
					}
				}

				hasRunning := false
				for acctID, acctCfg := range emailCfg.Accounts {
					info := map[string]interface{}{
						"accountId":  acctID,
						"configured": true,
						"enabled":    channels.IsAccountEnabled(acctCfg.Enabled),
					}
					if acctCfg.Address != "" {
						info["address"] = acctCfg.Address
					}
					if acctCfg.Provider != "" {
						info["provider"] = string(acctCfg.Provider)
					}
					if healthMap != nil {
						if h, ok := healthMap[acctID]; ok {
							info["running"] = h.running
							info["connected"] = h.running
							if h.lastError != "" {
								info["lastError"] = h.lastError
							}
							if h.running {
								hasRunning = true
							}
						}
					}
					emailAcctInfos = append(emailAcctInfos, info)
				}
				emailStatus["running"] = hasRunning
				emailStatus["connected"] = hasRunning
			}
			channelsMap["email"] = emailStatus
			if len(emailAcctInfos) > 0 {
				channelAccounts["email"] = emailAcctInfos
			} else {
				channelAccounts["email"] = []map[string]interface{}{}
			}
			if emailCfg.DefaultAccount != "" {
				channelDefaultAccountId["email"] = emailCfg.DefaultAccount
			}
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"ts":                      time.Now().UnixMilli(),
		"probeAt":                 time.Now().UnixMilli(),
		"channels":                channelsMap,
		"channelAccounts":         channelAccounts,
		"channelDefaultAccountId": channelDefaultAccountId,
		"channelOrder":            channelOrder,
		"channelLabels":           channelLabels,
		"channelDetailLabels":     map[string]interface{}{},
		"channelSystemImages":     map[string]interface{}{},
		"channelMeta":             []interface{}{},
	}, nil)
}

// ---------- channels.logout ----------
// 对应 TS channels.ts L237-291
// 解析 channel/accountId 参数，调用 DI logout 回调。

func handleChannelsLogout(ctx *MethodHandlerContext) {
	channelRaw := readString(ctx.Params, "channel")
	if channelRaw == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid channels.logout channel"))
		return
	}

	channelID := normalizeChannelID(channelRaw)
	if channelID == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid channels.logout channel"))
		return
	}

	accountID := readString(ctx.Params, "accountId")
	if accountID == "" {
		accountID = "default"
	}

	// Phase 5: 优先使用 ChannelManager 停止频道
	if mgr := ctx.Context.ChannelMgr; mgr != nil {
		if err := mgr.StopChannel(channels.ChannelID(channelID), accountID); err != nil {
			slog.Warn("channels.logout: stop failed", "channel", channelID, "error", err)
		} else {
			mgr.MarkLoggedOut(channels.ChannelID(channelID), true, accountID)
		}
		ctx.Respond(true, map[string]interface{}{
			"channel":   channelID,
			"accountId": accountID,
			"cleared":   true,
		}, nil)
		return
	}

	// 回退: DI 回调
	logoutFn := ctx.Context.ChannelLogoutFn
	if logoutFn == nil {
		ctx.Respond(true, map[string]interface{}{
			"channel":   channelID,
			"accountId": accountID,
			"cleared":   false,
			"stub":      true,
			"message":   "channel logout not yet wired to SDK",
		}, nil)
		slog.Warn("channels.logout: no logout function registered",
			"channel", channelID, "accountId", accountID)
		return
	}

	payload, err := logoutFn(channelID, accountID)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, err.Error()))
		return
	}

	ctx.Respond(true, payload, nil)
}

// normalizeChannelID 规范化频道 ID。
func normalizeChannelID(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	known := map[string]bool{
		"telegram": true, "discord": true, "slack": true,
		"whatsapp": true, "signal": true, "imessage": true,
		"webchat": true, "web": true, "cli": true,
		"wecom": true, "dingtalk": true, "feishu": true,
	}
	if known[normalized] {
		return normalized
	}
	// 别名
	aliases := map[string]string{
		"tg": "telegram", "wa": "whatsapp", "ig": "imessage",
		"im": "imessage", "dc": "discord", "slk": "slack",
		"sig": "signal",
		"dd":  "dingtalk", "fs": "feishu", "wx": "wecom",
		"lark": "feishu", "wework": "wecom",
	}
	if alias, ok := aliases[normalized]; ok {
		return alias
	}
	return ""
}
