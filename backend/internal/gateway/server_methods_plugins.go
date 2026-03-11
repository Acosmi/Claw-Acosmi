package gateway

// server_methods_plugins.go — plugins.list / plugins.config.set
// 插件中心：渠道插件状态 + 联网搜索插件配置。

import (
	"log/slog"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

// PluginConfigField 插件配置字段描述
type PluginConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "string"
	Sensitive   bool   `json:"sensitive"`
	Placeholder string `json:"placeholder,omitempty"`
}

// PluginInfo 插件信息
type PluginInfo struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Category     string              `json:"category"` // "channel" | "search"
	Icon         string              `json:"icon"`
	Enabled      bool                `json:"enabled"`
	Configured   bool                `json:"configured"`
	Running      bool                `json:"running"`
	ConfigFields []PluginConfigField `json:"configFields,omitempty"`
	ConfigValues map[string]string   `json:"configValues,omitempty"`
}

// ToolItem 内置工具信息
type ToolItem struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	Builtin     bool   `json:"builtin"`
}

// PluginsHandlers 返回 plugins.* 方法处理器映射。
func PluginsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"plugins.list":       handlePluginsList,
		"plugins.config.set": handlePluginsConfigSet,
		"tools.list":         handleToolsList,
		"tools.browser.get":  handleToolsBrowserGet,
		"tools.browser.set":  handleToolsBrowserSet,
	}
}

// ---------- 渠道插件元数据 ----------

type channelPluginMeta struct {
	id          string
	name        string
	description string
	icon        string
}

var channelPlugins = []channelPluginMeta{
	{"feishu", "飞书", "飞书 / Lark 企业即时通讯", "feishu"},
	{"dingtalk", "钉钉", "阿里钉钉企业协作平台", "dingtalk"},
	{"wecom", "企业微信", "腾讯企业微信", "wecom"},
	{"telegram", "Telegram", "Telegram Bot API 消息频道", "telegram"},
	{"discord", "Discord", "Discord Bot 服务器频道", "discord"},
	{"slack", "Slack", "Slack Workspace Bot 集成", "slack"},
	{"whatsapp", "WhatsApp", "WhatsApp Business 消息", "whatsapp"},
	{"signal", "Signal", "Signal 加密消息频道", "signal"},
	{"imessage", "iMessage", "Apple iMessage 频道（macOS）", "imessage"},
	{"googlechat", "Google Chat", "Google Workspace Chat Bot", "googlechat"},
	{"msteams", "MS Teams", "Microsoft Teams Bot 集成", "msteams"},
	{"web", "Web Chat", "内嵌 Web 聊天组件", "web"},
}

// ---------- plugins.list ----------

func handlePluginsList(ctx *MethodHandlerContext) {
	var cfg *types.OpenAcosmiConfig
	if loader := ctx.Context.ConfigLoader; loader != nil {
		if loaded, err := loader.LoadConfig(); err == nil {
			cfg = loaded
		}
	}
	if cfg == nil {
		cfg = ctx.Context.Config
	}

	// 从 ChannelManager 获取运行时快照
	var runtimeSnap *channels.RuntimeSnapshot
	if mgr := ctx.Context.ChannelMgr; mgr != nil {
		runtimeSnap = mgr.GetSnapshot()
	}

	plugins := make([]PluginInfo, 0, len(channelPlugins)+2)

	// 渠道插件
	for _, meta := range channelPlugins {
		configured := isChannelConfigured(cfg, meta.id)
		running := false
		if runtimeSnap != nil {
			if snap, ok := runtimeSnap.Channels[channels.ChannelID(meta.id)]; ok {
				running = snap.Status == "running"
			}
		}
		plugins = append(plugins, PluginInfo{
			ID:          meta.id,
			Name:        meta.name,
			Description: meta.description,
			Category:    "channel",
			Icon:        meta.icon,
			Enabled:     configured,
			Configured:  configured,
			Running:     running,
		})
	}

	// 博查搜索插件
	bochaEnabled := false
	bochaConfigured := false
	bochaValues := map[string]string{"apiKey": ""}
	if cfg != nil && cfg.Tools != nil && cfg.Tools.Web != nil && cfg.Tools.Web.Search != nil {
		s := cfg.Tools.Web.Search
		if s.Bocha != nil {
			bochaEnabled = s.Bocha.Enabled != nil && *s.Bocha.Enabled
			bochaConfigured = s.Bocha.APIKey != ""
			bochaValues["apiKey"] = s.Bocha.APIKey
		}
	}
	plugins = append(plugins, PluginInfo{
		ID:          "bocha",
		Name:        "博查搜索",
		Description: "博查 AI 联网搜索，提供中文互联网实时搜索能力",
		Category:    "search",
		Icon:        "search",
		Enabled:     bochaEnabled,
		Configured:  bochaConfigured,
		Running:     bochaEnabled && bochaConfigured,
		ConfigFields: []PluginConfigField{
			{Key: "apiKey", Label: "API Key", Type: "string", Sensitive: true, Placeholder: "bocha-xxxxxxxx"},
			{Key: "enabled", Label: "启用", Type: "boolean", Sensitive: false},
		},
		ConfigValues: bochaValues,
	})

	// Google 搜索插件
	googleEnabled := false
	googleConfigured := false
	googleValues := map[string]string{"apiKey": "", "searchEngineId": ""}
	if cfg != nil && cfg.Tools != nil && cfg.Tools.Web != nil && cfg.Tools.Web.Search != nil {
		s := cfg.Tools.Web.Search
		if s.Google != nil {
			googleEnabled = s.Google.Enabled != nil && *s.Google.Enabled
			googleConfigured = s.Google.APIKey != "" && s.Google.SearchEngineID != ""
			googleValues["apiKey"] = s.Google.APIKey
			googleValues["searchEngineId"] = s.Google.SearchEngineID
		}
	}
	plugins = append(plugins, PluginInfo{
		ID:          "google",
		Name:        "Google Search",
		Description: "Google Custom Search API，提供全球网页实时搜索能力",
		Category:    "search",
		Icon:        "search",
		Enabled:     googleEnabled,
		Configured:  googleConfigured,
		Running:     googleEnabled && googleConfigured,
		ConfigFields: []PluginConfigField{
			{Key: "apiKey", Label: "API Key", Type: "string", Sensitive: true, Placeholder: "AIzaXXXXXX"},
			{Key: "searchEngineId", Label: "Search Engine ID", Type: "string", Sensitive: false, Placeholder: "xxxxxxxxxxxxxxx"},
			{Key: "enabled", Label: "启用", Type: "boolean", Sensitive: false},
		},
		ConfigValues: googleValues,
	})

	ctx.Respond(true, map[string]interface{}{
		"plugins": plugins,
	}, nil)
}

// isChannelConfigured 检查频道是否已配置凭证。
func isChannelConfigured(cfg *types.OpenAcosmiConfig, channelID string) bool {
	if cfg == nil || cfg.Channels == nil {
		return false
	}
	switch channelID {
	case "telegram":
		return cfg.Channels.Telegram != nil && cfg.Channels.Telegram.BotToken != ""
	case "discord":
		return cfg.Channels.Discord != nil && cfg.Channels.Discord.DiscordAccountConfigToken() != ""
	case "slack":
		return cfg.Channels.Slack != nil && cfg.Channels.Slack.BotToken != ""
	case "whatsapp":
		return cfg.Channels.WhatsApp != nil
	case "feishu":
		return cfg.Channels.Feishu != nil && cfg.Channels.Feishu.AppID != "" && cfg.Channels.Feishu.AppSecret != ""
	case "dingtalk":
		return cfg.Channels.DingTalk != nil && cfg.Channels.DingTalk.AppKey != "" && cfg.Channels.DingTalk.AppSecret != ""
	case "wecom":
		return cfg.Channels.WeCom != nil && cfg.Channels.WeCom.CorpID != "" && cfg.Channels.WeCom.Secret != ""
	default:
		return false
	}
}

// ---------- plugins.config.set ----------

func handlePluginsConfigSet(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	pluginID := readString(ctx.Params, "pluginId")
	if pluginID != "bocha" && pluginID != "google" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "plugins.config.set: only search plugins (bocha, google) are configurable via this RPC"))
		return
	}

	configRaw, ok := ctx.Params["config"].(map[string]interface{})
	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "config object is required"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// 确保 tools.web.search 层级存在
	if cfg.Tools == nil {
		cfg.Tools = &types.ToolsConfig{}
	}
	if cfg.Tools.Web == nil {
		cfg.Tools.Web = &types.WebToolsConfig{}
	}
	if cfg.Tools.Web.Search == nil {
		cfg.Tools.Web.Search = &types.WebSearchConfig{}
	}

	rs := func(key string) string {
		v, _ := configRaw[key].(string)
		return strings.TrimSpace(v)
	}
	rb := func(key string) *bool {
		v, ok := configRaw[key]
		if !ok {
			return nil
		}
		switch val := v.(type) {
		case bool:
			b := val
			return &b
		case string:
			b := val == "true" || val == "1" || val == "yes"
			return &b
		}
		return nil
	}

	switch pluginID {
	case "bocha":
		if cfg.Tools.Web.Search.Bocha == nil {
			cfg.Tools.Web.Search.Bocha = &types.WebSearchBochaConfig{}
		}
		if apiKey := rs("apiKey"); apiKey != "" {
			cfg.Tools.Web.Search.Bocha.APIKey = apiKey
		}
		if enabled := rb("enabled"); enabled != nil {
			cfg.Tools.Web.Search.Bocha.Enabled = enabled
		}
	case "google":
		if cfg.Tools.Web.Search.Google == nil {
			cfg.Tools.Web.Search.Google = &types.WebSearchGoogleConfig{}
		}
		if apiKey := rs("apiKey"); apiKey != "" {
			cfg.Tools.Web.Search.Google.APIKey = apiKey
		}
		if seID := rs("searchEngineId"); seID != "" {
			cfg.Tools.Web.Search.Google.SearchEngineID = seID
		}
		if enabled := rb("enabled"); enabled != nil {
			cfg.Tools.Web.Search.Google.Enabled = enabled
		}
	}

	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}
	loader.ClearCache()

	slog.Info("plugins.config.set: saved", "pluginId", pluginID)
	ctx.Respond(true, map[string]interface{}{"ok": true, "pluginId": pluginID}, nil)
}

// ---------- tools.list ----------

// builtinTools 系统内置工具元数据，分类展示。
var builtinTools = []ToolItem{
	// 文件操作
	{Name: "read_file", Label: "Read File", Description: "读取文件内容，支持行范围和偏移量", Category: "file", Icon: "file", Builtin: true},
	{Name: "write_file", Label: "Write File", Description: "创建或覆盖文件内容", Category: "file", Icon: "file", Builtin: true},
	{Name: "apply_patch", Label: "Apply Patch", Description: "应用多文件补丁（unified diff 格式）", Category: "file", Icon: "edit", Builtin: true},
	{Name: "list_dir", Label: "List Directory", Description: "列出目录内容及元信息", Category: "file", Icon: "folder", Builtin: true},

	// 命令执行
	{Name: "bash", Label: "Shell Exec", Description: "在沙箱或本地执行 Shell 命令", Category: "exec", Icon: "terminal", Builtin: true},
	{Name: "process", Label: "Process Manager", Description: "管理后台进程会话（启动/停止/查看输出）", Category: "exec", Icon: "terminal", Builtin: true},

	// 网络与浏览
	{Name: "web_search", Label: "Web Search", Description: "联网搜索（博查/Google），获取实时网页结果", Category: "web", Icon: "search", Builtin: true},
	{Name: "web_fetch", Label: "Web Fetch", Description: "抓取 URL 内容并提取可读文本", Category: "web", Icon: "link", Builtin: true},
	{Name: "browser", Label: "Browser", Description: "控制 Web 浏览器进行自动化操作", Category: "web", Icon: "monitor", Builtin: true},

	// 系统控制
	{Name: "canvas", Label: "Canvas", Description: "展示、评估和快照 Canvas 画布", Category: "system", Icon: "monitor", Builtin: true},
	{Name: "nodes", Label: "Nodes", Description: "列出/描述/通知/控制已配对节点设备", Category: "system", Icon: "monitor", Builtin: true},
	{Name: "cron", Label: "Cron", Description: "管理定时任务和唤醒事件", Category: "system", Icon: "loader", Builtin: true},
	{Name: "message", Label: "Message", Description: "发送消息和频道操作", Category: "system", Icon: "messageSquare", Builtin: true},
	{Name: "gateway", Label: "Gateway", Description: "重启、应用配置或运行更新", Category: "system", Icon: "settings", Builtin: true},

	// 会话管理
	{Name: "agents_list", Label: "Agents List", Description: "列出可用于 sessions_spawn 的 Agent ID", Category: "session", Icon: "folder", Builtin: true},
	{Name: "sessions_list", Label: "Sessions List", Description: "列出其他会话（支持过滤和分页）", Category: "session", Icon: "radio", Builtin: true},
	{Name: "sessions_history", Label: "Sessions History", Description: "获取另一个会话或子 Agent 的历史记录", Category: "session", Icon: "scrollText", Builtin: true},
	{Name: "sessions_send", Label: "Sessions Send", Description: "向另一个会话或子 Agent 发送消息", Category: "session", Icon: "messageSquare", Builtin: true},
	{Name: "sessions_spawn", Label: "Sessions Spawn", Description: "创建新的子 Agent 会话", Category: "session", Icon: "zap", Builtin: true},
	{Name: "session_status", Label: "Session Status", Description: "显示会话状态卡片（用量 + 时间 + 模式）", Category: "session", Icon: "barChart", Builtin: true},

	// AI 能力
	{Name: "image", Label: "Image Analysis", Description: "使用配置的图像模型分析图片", Category: "ai", Icon: "monitor", Builtin: true},

	// 记忆系统
	{Name: "memory_search", Label: "Memory Search", Description: "搜索太虚永忆记忆文件", Category: "memory", Icon: "memoryChip", Builtin: true},
	{Name: "memory_get", Label: "Memory Get", Description: "获取记忆文件特定行内容", Category: "memory", Icon: "memoryChip", Builtin: true},
}

func handleToolsList(ctx *MethodHandlerContext) {
	ctx.Respond(true, map[string]interface{}{
		"tools": builtinTools,
	}, nil)
}

// ---------- tools.browser.get / tools.browser.set ----------

func buildBrowserConfigPayload(cfg *types.OpenAcosmiConfig) map[string]interface{} {
	enabled := true
	evaluateEnabled := true
	headless := false
	cdpUrl := ""

	if cfg != nil && cfg.Browser != nil {
		b := cfg.Browser
		if b.Enabled != nil {
			enabled = *b.Enabled
		}
		if b.EvaluateEnabled != nil {
			evaluateEnabled = *b.EvaluateEnabled
		}
		if b.Headless != nil {
			headless = *b.Headless
		}
		cdpUrl = b.CdpURL
	}

	// 桌面端支持缺省 auto-discovery / auto-launch，未显式填写 cdpUrl 不应再被视为“未配置”。
	configured := enabled

	return map[string]interface{}{
		"enabled":         enabled,
		"cdpUrl":          cdpUrl,
		"evaluateEnabled": evaluateEnabled,
		"headless":        headless,
		"configured":      configured,
	}
}

func handleToolsBrowserGet(ctx *MethodHandlerContext) {
	var cfg *types.OpenAcosmiConfig
	if loader := ctx.Context.ConfigLoader; loader != nil {
		if loaded, err := loader.LoadConfig(); err == nil {
			cfg = loaded
		}
	}
	if cfg == nil {
		cfg = ctx.Context.Config
	}

	result := buildBrowserConfigPayload(cfg)
	attachConfigHash(ctx.Context.ConfigLoader, result)
	ctx.Respond(true, result, nil)
}

func handleToolsBrowserSet(ctx *MethodHandlerContext) {
	executeConfigMutation(ctx, configMutationOptions{
		Action: "tools.browser.set",
		Mutate: func(cfg *types.OpenAcosmiConfig) error {
			if cfg.Browser == nil {
				cfg.Browser = &types.BrowserConfig{}
			}

			if v, ok := ctx.Params["enabled"]; ok {
				b := toBool(v)
				cfg.Browser.Enabled = &b
			}
			if s, ok := readTrimmedStringParam(ctx.Params, "cdpUrl"); ok {
				cfg.Browser.CdpURL = s
			}
			if v, ok := ctx.Params["evaluateEnabled"]; ok {
				b := toBool(v)
				cfg.Browser.EvaluateEnabled = &b
			}
			if v, ok := ctx.Params["headless"]; ok {
				b := toBool(v)
				cfg.Browser.Headless = &b
			}
			return nil
		},
		AfterWrite: func(_ *MethodHandlerContext, cfg *types.OpenAcosmiConfig) map[string]interface{} {
			slog.Info("tools.browser.set: saved")
			return map[string]interface{}{
				"browser": buildBrowserConfigPayload(cfg),
			}
		},
	})
}

// toBool 将 interface{} 转换为 bool。
func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "1" || val == "yes"
	case float64:
		return val != 0
	}
	return false
}
