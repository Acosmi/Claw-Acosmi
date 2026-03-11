package gateway

// server_methods_media.go — 媒体子系统 RPC 方法
// 提供 media.trending.fetch / media.trending.sources / media.drafts.list / media.drafts.get / media.drafts.delete 方法
// 遵循 server_methods_image.go 模式

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	channelxhs "github.com/Acosmi/ClawAcosmi/internal/channels/xiaohongshu"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

var knownMediaSources = func() []string {
	defs := media.KnownTrendingSourceDefinitions()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}()

// MediaHandlers 返回媒体子系统 RPC 方法处理器。
func MediaHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"media.trending.fetch":        handleMediaTrendingFetch,
		"media.trending.sources":      handleMediaTrendingSources,
		"media.trending.health":       handleMediaTrendingHealth,
		"media.drafts.list":           handleMediaDraftsList,
		"media.drafts.get":            handleMediaDraftsGet,
		"media.drafts.delete":         handleMediaDraftsDelete,
		"media.drafts.update":         handleMediaDraftsUpdate,
		"media.drafts.approve":        handleMediaDraftsApprove,
		"media.publish.list":          handleMediaPublishList,
		"media.publish.get":           handleMediaPublishGet,
		"media.config.get":            handleMediaConfigGet,
		"media.config.update":         handleMediaConfigUpdate,
		"media.publisher.login.start": handleMediaPublisherLoginStart,
		"media.publisher.login.wait":  handleMediaPublisherLoginWait,
		"media.tools.list":            handleMediaToolsList,
		"media.tools.toggle":          handleMediaToolsToggle,
		"media.sources.toggle":        handleMediaSourcesToggle,
	}
}

// appendSharedMediaTools 追加共享 runner 工具到工具列表（去重 webSearch 逻辑）。
func appendSharedMediaTools(tools []map[string]interface{}, cfg *types.OpenAcosmiConfig) []map[string]interface{} {
	webSearchEnabled := false
	if cfg != nil && cfg.Tools != nil && cfg.Tools.Web != nil &&
		cfg.Tools.Web.Search != nil && cfg.Tools.Web.Search.Bocha != nil {
		webSearchEnabled = cfg.Tools.Web.Search.Bocha.Enabled == nil || *cfg.Tools.Web.Search.Bocha.Enabled
	}
	tools = append(tools, map[string]interface{}{
		"name":        "web_search",
		"description": "Search the web for information and references",
		"enabled":     webSearchEnabled,
		"scope":       "shared",
	})
	tools = append(tools, map[string]interface{}{
		"name":        "report_progress",
		"description": "Report intermediate progress to the user",
		"enabled":     true,
		"scope":       "shared",
	})
	return tools
}

func mediaAgentSettings(cfg *types.OpenAcosmiConfig) *types.MediaAgentSettings {
	if cfg == nil || cfg.SubAgents == nil {
		return nil
	}
	return cfg.SubAgents.MediaAgent
}

func mediaChannels(cfg *types.OpenAcosmiConfig) *types.ChannelsConfig {
	if cfg == nil {
		return nil
	}
	return cfg.Channels
}

func mediaLiveConfig(ctx *MethodHandlerContext) *types.OpenAcosmiConfig {
	if ctx == nil || ctx.Context == nil {
		return &types.OpenAcosmiConfig{}
	}
	if ctx.Context.ConfigLoader != nil {
		if fresh, err := ctx.Context.ConfigLoader.LoadConfig(); err == nil && fresh != nil {
			ctx.Context.Config = fresh
			return fresh
		}
	}
	if ctx.Context.Config != nil {
		return ctx.Context.Config
	}
	return &types.OpenAcosmiConfig{}
}

func syncMediaTrendingSources(sub *media.MediaSubsystem, cfg *types.OpenAcosmiConfig) {
	if sub == nil {
		return
	}
	sub.SetTrendingSources(media.BuildTrendingSourcesFromConfig(cfg))
}

func mediaSourceRequiresCredential(name string) bool {
	def, ok := media.TrendingSourceDefinitionByName(name)
	return ok && def.RequiresCredential
}

func unionMediaSourceNames(runtimeNames []string, explicitNames []string) []string {
	names := make([]string, 0, len(knownMediaSources)+len(runtimeNames)+len(explicitNames))
	for _, name := range knownMediaSources {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	for _, name := range runtimeNames {
		if strings.TrimSpace(name) == "" || slices.Contains(names, name) {
			continue
		}
		names = append(names, name)
	}
	for _, name := range explicitNames {
		if strings.TrimSpace(name) == "" || slices.Contains(names, name) {
			continue
		}
		names = append(names, name)
	}
	return names
}

func resolveMediaSourceState(cfg *types.OpenAcosmiConfig, runtimeNames []string) ([]map[string]interface{}, []string, bool) {
	ma := mediaAgentSettings(cfg)
	allNames := unionMediaSourceNames(runtimeNames, nil)
	enabledSourcesConfigured := ma != nil && ma.EnabledSources != nil
	enabledSet := map[string]bool{}

	if enabledSourcesConfigured {
		allNames = unionMediaSourceNames(runtimeNames, ma.EnabledSources)
		for _, name := range ma.EnabledSources {
			if strings.TrimSpace(name) == "" {
				continue
			}
			enabledSet[name] = true
		}
	} else {
		for _, name := range allNames {
			if mediaSourceRequiresCredential(name) {
				enabledSet[name] = media.IsTrendingSourceConfigured(cfg, name)
				continue
			}
			enabledSet[name] = true
		}
	}

	sort.Strings(allNames)

	enabledSources := make([]string, 0, len(allNames))
	sources := make([]map[string]interface{}, 0, len(allNames))
	for _, name := range allNames {
		enabled := enabledSet[name]
		sourceConfigured := media.IsTrendingSourceConfigured(cfg, name)
		status := "disabled"
		switch {
		case enabled && mediaSourceRequiresCredential(name) && !sourceConfigured:
			status = "needs_configuration"
		case enabledSourcesConfigured && enabled:
			status = "configured"
		case !enabledSourcesConfigured && enabled && mediaSourceRequiresCredential(name):
			status = "configured"
		case !enabledSourcesConfigured && enabled:
			status = "default_enabled"
		case !enabled && mediaSourceRequiresCredential(name) && !sourceConfigured:
			status = "needs_configuration"
		}
		if enabled {
			enabledSources = append(enabledSources, name)
		}
		sources = append(sources, map[string]interface{}{
			"name":                name,
			"enabled":             enabled,
			"configured":          enabledSourcesConfigured,
			"status":              status,
			"source_configured":   sourceConfigured,
			"requires_credential": mediaSourceRequiresCredential(name),
		})
	}

	return sources, enabledSources, enabledSourcesConfigured
}

func isWeChatPublisherConfigured(cfg *types.OpenAcosmiConfig) bool {
	ch := mediaChannels(cfg)
	if ch == nil || ch.WeChatMP == nil {
		return false
	}
	return ch.WeChatMP.Enabled &&
		strings.TrimSpace(ch.WeChatMP.AppID) != "" &&
		strings.TrimSpace(ch.WeChatMP.AppSecret) != ""
}

func isXiaohongshuConfigured(cfg *types.OpenAcosmiConfig) bool {
	ch := mediaChannels(cfg)
	if ch == nil || ch.Xiaohongshu == nil {
		return false
	}
	return ch.Xiaohongshu.Enabled &&
		strings.TrimSpace(ch.Xiaohongshu.CookiePath) != ""
}

func isWebsitePublisherConfigured(cfg *types.OpenAcosmiConfig) bool {
	ch := mediaChannels(cfg)
	if ch == nil || ch.Website == nil {
		return false
	}
	return ch.Website.Enabled &&
		strings.TrimSpace(ch.Website.APIURL) != "" &&
		strings.TrimSpace(ch.Website.AuthType) != "" &&
		strings.TrimSpace(ch.Website.AuthToken) != ""
}

func configuredPublishers(cfg *types.OpenAcosmiConfig) []string {
	publishers := make([]string, 0, 3)
	if isWeChatPublisherConfigured(cfg) {
		publishers = append(publishers, string(media.PlatformWeChat))
	}
	if isXiaohongshuConfigured(cfg) {
		publishers = append(publishers, string(media.PlatformXiaohongshu))
	}
	if isWebsitePublisherConfigured(cfg) {
		publishers = append(publishers, string(media.PlatformWebsite))
	}
	return publishers
}

func maskSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > 8 {
		return trimmed[:4] + "****" + trimmed[len(trimmed)-4:]
	}
	return "****"
}

func profileStatus(enabled, configured bool) string {
	switch {
	case !enabled:
		return "disabled"
	case configured:
		return "configured"
	default:
		return "needs_configuration"
	}
}

func nonEmptyFields(values map[string]string) []string {
	missing := make([]string, 0, len(values))
	for field, value := range values {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, field)
		}
	}
	sort.Strings(missing)
	return missing
}

func defaultXiaohongshuCookiePath(accountID string) string {
	normalized := strings.TrimSpace(accountID)
	if normalized == "" {
		normalized = channels.DefaultAccountID
	}
	return filepath.Join(config.ResolveStateDir(), "_media", "xhs", normalized+"-cookies.json")
}

func resolveXiaohongshuAuthState(
	cfg *types.XiaohongshuConfig,
	channelMgr *channels.Manager,
) (string, string, string, bool) {
	browserReady := false
	if channelMgr != nil {
		if plugin, ok := channelMgr.GetPlugin(media.ChannelXiaohongshu).(*channelxhs.XiaohongshuPlugin); ok {
			if client := plugin.GetClient(channels.DefaultAccountID); client != nil {
				auth := client.AuthState()
				updatedAt := ""
				if auth.UpdatedAt != nil {
					updatedAt = auth.UpdatedAt.UTC().Format(time.RFC3339)
				}
				return auth.Status, auth.Message, updatedAt, auth.BrowserReady
			}
		}
	}

	if cfg == nil || strings.TrimSpace(cfg.CookiePath) == "" {
		return "cookie_path_missing", "尚未设置 Cookie 保存路径，首次登录时系统会自动分配默认路径。", "", browserReady
	}
	info, err := os.Stat(cfg.CookiePath)
	if err != nil {
		return "not_logged_in", "尚未采集到小红书登录态。", "", browserReady
	}
	return "cookie_present", "检测到本地 Cookie 文件。首次使用前建议点击“检查并保存登录态”确认仍然有效。", info.ModTime().UTC().Format(time.RFC3339), browserReady
}

func buildMediaTrendingSourceProfiles(cfg *types.OpenAcosmiConfig) map[string]map[string]interface{} {
	bocha := &types.MediaTrendingBochaConfig{
		BaseURL:   "https://api.bochaai.com",
		Freshness: "oneDay",
	}
	if ma := mediaAgentSettings(cfg); ma != nil && ma.TrendingProfiles != nil && ma.TrendingProfiles.Bocha != nil {
		copyCfg := *ma.TrendingProfiles.Bocha
		bocha = &copyCfg
		if strings.TrimSpace(bocha.BaseURL) == "" {
			bocha.BaseURL = "https://api.bochaai.com"
		}
	}
	bocha.Freshness = mediaNormalizeBochaFreshness(bocha.Freshness)
	bochaConfigured := strings.TrimSpace(bocha.APIKey) != ""
	bochaMissing := nonEmptyFields(map[string]string{
		"apiKey": bocha.APIKey,
	})
	customOpenAI := &types.MediaTrendingCustomOpenAIConfig{
		BaseURL: "https://api.openai.com/v1",
	}
	if ma := mediaAgentSettings(cfg); ma != nil && ma.TrendingProfiles != nil && ma.TrendingProfiles.CustomOpenAI != nil {
		copyCfg := *ma.TrendingProfiles.CustomOpenAI
		customOpenAI = &copyCfg
		if strings.TrimSpace(customOpenAI.BaseURL) == "" {
			customOpenAI.BaseURL = "https://api.openai.com/v1"
		}
	}
	customOpenAIMissing := nonEmptyFields(map[string]string{
		"apiKey":  customOpenAI.APIKey,
		"baseUrl": customOpenAI.BaseURL,
		"model":   customOpenAI.Model,
	})
	customOpenAIConfigured := len(customOpenAIMissing) == 0
	return map[string]map[string]interface{}{
		"bocha": {
			"configured": bochaConfigured,
			"status":     profileStatus(true, bochaConfigured),
			"missing":    bochaMissing,
			"authMode":   "api_key",
			"apiKey":     maskSecret(bocha.APIKey),
			"baseUrl":    bocha.BaseURL,
			"freshness":  bocha.Freshness,
		},
		"custom_openai": {
			"configured":    customOpenAIConfigured,
			"status":        profileStatus(true, customOpenAIConfigured),
			"missing":       customOpenAIMissing,
			"authMode":      "api_key",
			"apiKey":        maskSecret(customOpenAI.APIKey),
			"baseUrl":       customOpenAI.BaseURL,
			"model":         customOpenAI.Model,
			"systemPrompt":  customOpenAI.SystemPrompt,
			"requestExtras": customOpenAI.RequestExtras,
		},
	}
}

func buildMediaPublisherProfiles(cfg *types.OpenAcosmiConfig, channelMgr *channels.Manager) map[string]map[string]interface{} {
	ch := mediaChannels(cfg)

	wechat := &types.WeChatMPConfig{}
	if ch != nil && ch.WeChatMP != nil {
		copyCfg := *ch.WeChatMP
		wechat = &copyCfg
	}
	wechatMissing := nonEmptyFields(map[string]string{
		"appId":     wechat.AppID,
		"appSecret": wechat.AppSecret,
	})
	if !wechat.Enabled {
		wechatMissing = nil
	}
	wechatConfigured := wechat.Enabled && len(wechatMissing) == 0

	xhs := &types.XiaohongshuConfig{
		AutoInteractInterval: 30,
		RateLimitSeconds:     5,
		ErrorScreenshotDir:   "_media/xhs/errors",
	}
	if ch != nil && ch.Xiaohongshu != nil {
		copyCfg := *ch.Xiaohongshu
		xhs = &copyCfg
		if xhs.RateLimitSeconds <= 0 {
			xhs.RateLimitSeconds = 5
		}
		if strings.TrimSpace(xhs.ErrorScreenshotDir) == "" {
			xhs.ErrorScreenshotDir = "_media/xhs/errors"
		}
	}
	xhsMissing := nonEmptyFields(map[string]string{
		"cookiePath": xhs.CookiePath,
	})
	if !xhs.Enabled {
		xhsMissing = nil
	}
	xhsConfigured := xhs.Enabled && len(xhsMissing) == 0
	xhsAuthStatus, xhsAuthMessage, xhsAuthUpdatedAt, xhsBrowserReady := resolveXiaohongshuAuthState(xhs, channelMgr)

	websiteCfg := &types.WebsiteConfig{
		AuthType:       "bearer",
		TimeoutSeconds: 30,
		MaxRetries:     3,
	}
	if ch != nil && ch.Website != nil {
		copyCfg := *ch.Website
		websiteCfg = &copyCfg
		if strings.TrimSpace(websiteCfg.AuthType) == "" {
			websiteCfg.AuthType = "bearer"
		}
		if websiteCfg.TimeoutSeconds <= 0 {
			websiteCfg.TimeoutSeconds = 30
		}
	}
	websiteMissing := nonEmptyFields(map[string]string{
		"apiUrl":    websiteCfg.APIURL,
		"authType":  websiteCfg.AuthType,
		"authToken": websiteCfg.AuthToken,
	})
	if !websiteCfg.Enabled {
		websiteMissing = nil
	}
	websiteConfigured := websiteCfg.Enabled && len(websiteMissing) == 0

	return map[string]map[string]interface{}{
		"wechat": {
			"enabled":        wechat.Enabled,
			"configured":     wechatConfigured,
			"status":         profileStatus(wechat.Enabled, wechatConfigured),
			"missing":        wechatMissing,
			"authMode":       "app_secret",
			"accountName":    wechat.AccountName,
			"accountId":      wechat.AccountID,
			"appId":          wechat.AppID,
			"appSecret":      maskSecret(wechat.AppSecret),
			"tokenCachePath": wechat.TokenCachePath,
		},
		"xiaohongshu": {
			"enabled":              xhs.Enabled,
			"configured":           xhsConfigured,
			"status":               profileStatus(xhs.Enabled, xhsConfigured),
			"missing":              xhsMissing,
			"authMode":             "cookie_file",
			"accountName":          xhs.AccountName,
			"accountId":            xhs.AccountID,
			"cookiePath":           xhs.CookiePath,
			"autoInteractInterval": xhs.AutoInteractInterval,
			"rateLimitSeconds":     xhs.RateLimitSeconds,
			"errorScreenshotDir":   xhs.ErrorScreenshotDir,
			"authStatus":           xhsAuthStatus,
			"authMessage":          xhsAuthMessage,
			"authUpdatedAt":        xhsAuthUpdatedAt,
			"browserReady":         xhsBrowserReady,
		},
		"website": {
			"enabled":        websiteCfg.Enabled,
			"configured":     websiteConfigured,
			"status":         profileStatus(websiteCfg.Enabled, websiteConfigured),
			"missing":        websiteMissing,
			"authMode":       "token",
			"siteName":       websiteCfg.SiteName,
			"apiUrl":         websiteCfg.APIURL,
			"authType":       websiteCfg.AuthType,
			"authToken":      maskSecret(websiteCfg.AuthToken),
			"imageUploadUrl": websiteCfg.ImageUploadURL,
			"timeoutSeconds": websiteCfg.TimeoutSeconds,
			"maxRetries":     websiteCfg.MaxRetries,
		},
	}
}

func asConfigPatch(value interface{}) map[string]interface{} {
	if raw, ok := value.(map[string]interface{}); ok {
		return raw
	}
	return nil
}

func sanitizeTextPatch(value string) string {
	return strings.TrimSpace(value)
}

func mediaNormalizeBochaFreshness(value string) string {
	switch strings.TrimSpace(value) {
	case "", "oneDay":
		return "oneDay"
	case "noLimit", "oneWeek", "oneMonth", "oneYear":
		return strings.TrimSpace(value)
	default:
		return "oneDay"
	}
}

func applyTrendingBochaPatch(ma *types.MediaAgentSettings, patch map[string]interface{}) {
	if ma == nil || patch == nil {
		return
	}
	if ma.TrendingProfiles == nil {
		ma.TrendingProfiles = &types.MediaTrendingSourceProfiles{}
	}
	if ma.TrendingProfiles.Bocha == nil {
		ma.TrendingProfiles.Bocha = &types.MediaTrendingBochaConfig{}
	}
	cfg := ma.TrendingProfiles.Bocha
	if v, ok := patch["apiKey"].(string); ok && !strings.Contains(v, "****") {
		cfg.APIKey = sanitizeTextPatch(v)
	}
	if v, ok := patch["baseUrl"].(string); ok {
		cfg.BaseURL = sanitizeTextPatch(v)
	}
	if v, ok := patch["freshness"].(string); ok {
		cfg.Freshness = mediaNormalizeBochaFreshness(v)
	}
}

func applyTrendingCustomOpenAIPatch(ma *types.MediaAgentSettings, patch map[string]interface{}) {
	if ma == nil || patch == nil {
		return
	}
	if ma.TrendingProfiles == nil {
		ma.TrendingProfiles = &types.MediaTrendingSourceProfiles{}
	}
	if ma.TrendingProfiles.CustomOpenAI == nil {
		ma.TrendingProfiles.CustomOpenAI = &types.MediaTrendingCustomOpenAIConfig{}
	}
	cfg := ma.TrendingProfiles.CustomOpenAI
	if v, ok := patch["apiKey"].(string); ok && !strings.Contains(v, "****") {
		cfg.APIKey = sanitizeTextPatch(v)
	}
	if v, ok := patch["baseUrl"].(string); ok {
		cfg.BaseURL = sanitizeTextPatch(v)
	}
	if v, ok := patch["model"].(string); ok {
		cfg.Model = sanitizeTextPatch(v)
	}
	if v, ok := patch["systemPrompt"].(string); ok {
		cfg.SystemPrompt = strings.TrimSpace(v)
	}
	if v, ok := patch["requestExtras"].(string); ok {
		cfg.RequestExtras = strings.TrimSpace(v)
	}
}

func applyWeChatPublisherPatch(cfg *types.WeChatMPConfig, patch map[string]interface{}) {
	if cfg == nil || patch == nil {
		return
	}
	if v, ok := patch["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := patch["accountName"].(string); ok {
		cfg.AccountName = sanitizeTextPatch(v)
	}
	if v, ok := patch["accountId"].(string); ok {
		cfg.AccountID = sanitizeTextPatch(v)
	}
	if v, ok := patch["appId"].(string); ok {
		cfg.AppID = sanitizeTextPatch(v)
	}
	if v, ok := patch["appSecret"].(string); ok && !strings.Contains(v, "****") {
		cfg.AppSecret = sanitizeTextPatch(v)
	}
	if v, ok := patch["tokenCachePath"].(string); ok {
		cfg.TokenCachePath = sanitizeTextPatch(v)
	}
}

func applyXiaohongshuPublisherPatch(cfg *types.XiaohongshuConfig, patch map[string]interface{}) {
	if cfg == nil || patch == nil {
		return
	}
	if v, ok := patch["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := patch["accountName"].(string); ok {
		cfg.AccountName = sanitizeTextPatch(v)
	}
	if v, ok := patch["accountId"].(string); ok {
		cfg.AccountID = sanitizeTextPatch(v)
	}
	if v, ok := patch["cookiePath"].(string); ok {
		cfg.CookiePath = sanitizeTextPatch(v)
	}
	if v, ok := patch["autoInteractInterval"].(float64); ok {
		if v < 0 {
			v = 0
		}
		if v > 1440 {
			v = 1440
		}
		cfg.AutoInteractInterval = int(v)
	}
	if v, ok := patch["rateLimitSeconds"].(float64); ok {
		if v < 3 {
			v = 3
		}
		if v > 300 {
			v = 300
		}
		cfg.RateLimitSeconds = int(v)
	}
	if v, ok := patch["errorScreenshotDir"].(string); ok {
		cfg.ErrorScreenshotDir = sanitizeTextPatch(v)
	}
}

func applyWebsitePublisherPatch(cfg *types.WebsiteConfig, patch map[string]interface{}) {
	if cfg == nil || patch == nil {
		return
	}
	if v, ok := patch["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := patch["siteName"].(string); ok {
		cfg.SiteName = sanitizeTextPatch(v)
	}
	if v, ok := patch["apiUrl"].(string); ok {
		cfg.APIURL = sanitizeTextPatch(v)
	}
	if v, ok := patch["authType"].(string); ok {
		cfg.AuthType = sanitizeTextPatch(v)
	}
	if v, ok := patch["authToken"].(string); ok && !strings.Contains(v, "****") {
		cfg.AuthToken = sanitizeTextPatch(v)
	}
	if v, ok := patch["imageUploadUrl"].(string); ok {
		cfg.ImageUploadURL = sanitizeTextPatch(v)
	}
	if v, ok := patch["timeoutSeconds"].(float64); ok {
		if v < 1 {
			v = 1
		}
		if v > 600 {
			v = 600
		}
		cfg.TimeoutSeconds = int(v)
	}
	if v, ok := patch["maxRetries"].(float64); ok {
		if v < 0 {
			v = 0
		}
		if v > 10 {
			v = 10
		}
		cfg.MaxRetries = int(v)
	}
}

func mediaToolState(name string, enabled bool, cfg *types.OpenAcosmiConfig) (string, bool) {
	switch name {
	case media.ToolTrendingTopics, media.ToolContentCompose:
		return "builtin", false
	case media.ToolMediaPublish:
		if !enabled {
			return "disabled", false
		}
		configured := len(configuredPublishers(cfg)) > 0
		if configured {
			return "configured", true
		}
		return "needs_configuration", false
	case media.ToolSocialInteract:
		if !enabled {
			return "disabled", false
		}
		configured := isXiaohongshuConfigured(cfg)
		if configured {
			return "configured", true
		}
		return "needs_configuration", false
	default:
		if enabled {
			return "enabled", false
		}
		return "disabled", false
	}
}

// ---------- media.trending.fetch ----------

func handleMediaTrendingFetch(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	liveCfg := mediaLiveConfig(ctx)
	syncMediaTrendingSources(sub, liveCfg)

	source, _ := ctx.Params["source"].(string)
	category, _ := ctx.Params["category"].(string)
	limit := 20
	if l, ok := ctx.Params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	fetchCtx, cancel := context.WithTimeout(ctx.Ctx, 15*time.Second)
	defer cancel()

	if source != "" {
		topics, err := sub.Aggregator.FetchBySource(fetchCtx, source, category, limit)
		if err != nil {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "fetch trending: "+err.Error()))
			return
		}
		ctx.Respond(true, map[string]interface{}{
			"source": source,
			"topics": topics,
			"count":  len(topics),
		}, nil)
		return
	}

	topics, results := sub.Aggregator.FetchAll(fetchCtx, category, limit)
	var errors []map[string]string
	for _, r := range results {
		if r.Err != nil {
			errors = append(errors, map[string]string{
				"source": r.Source,
				"error":  r.Err.Error(),
			})
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"topics": topics,
		"count":  len(topics),
		"errors": errors,
	}, nil)
}

// ---------- media.trending.sources ----------

func handleMediaTrendingSources(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	liveCfg := mediaLiveConfig(ctx)
	syncMediaTrendingSources(sub, liveCfg)

	names := sub.Aggregator.SourceNames()
	ctx.Respond(true, map[string]interface{}{
		"sources": names,
	}, nil)
}

// ---------- media.trending.health ----------

func handleMediaTrendingHealth(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	liveCfg := mediaLiveConfig(ctx)
	syncMediaTrendingSources(sub, liveCfg)

	// Probe each source with limit=1 to check health
	probeCtx, cancel := context.WithTimeout(ctx.Ctx, 15*time.Second)
	defer cancel()
	_, results := sub.Aggregator.FetchAll(probeCtx, "", 1)

	type sourceHealth struct {
		Name   string `json:"name"`
		Status string `json:"status"` // "ok" | "error"
		Error  string `json:"error,omitempty"`
		Count  int    `json:"count"`
	}

	sources := make([]sourceHealth, 0, len(results))
	for _, r := range results {
		h := sourceHealth{
			Name:  r.Source,
			Count: len(r.Topics),
		}
		if r.Err != nil {
			h.Status = "error"
			h.Error = r.Err.Error()
		} else {
			h.Status = "ok"
		}
		sources = append(sources, h)
	}

	ctx.Respond(true, map[string]interface{}{
		"sources": sources,
	}, nil)
}

// ---------- media.drafts.list ----------

func handleMediaDraftsList(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	platform, _ := ctx.Params["platform"].(string)
	drafts, err := sub.DraftStore.List(platform)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "list drafts: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"drafts": drafts,
		"count":  len(drafts),
	}, nil)
}

// ---------- media.drafts.get ----------

func handleMediaDraftsGet(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	draft, err := sub.DraftStore.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get draft: "+err.Error()))
		return
	}

	ctx.Respond(true, draft, nil)
}

// ---------- media.drafts.delete ----------

func handleMediaDraftsDelete(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	if err := sub.DraftStore.Delete(id); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "delete draft: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"deleted": true,
		"id":      id,
	}, nil)
}

// ---------- media.drafts.update ----------

func handleMediaDraftsUpdate(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	// 加载现有草稿
	draft, err := sub.DraftStore.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get draft: "+err.Error()))
		return
	}

	// 应用部分更新
	if title, ok := ctx.Params["title"].(string); ok && title != "" {
		draft.Title = title
	}
	if body, ok := ctx.Params["body"].(string); ok {
		draft.Body = body
	}
	if platform, ok := ctx.Params["platform"].(string); ok && platform != "" {
		draft.Platform = media.Platform(platform)
	}
	if tagsRaw, ok := ctx.Params["tags"].([]interface{}); ok {
		tags := make([]string, 0, len(tagsRaw))
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
		draft.Tags = tags
	}

	draft.UpdatedAt = time.Now()

	if err := sub.DraftStore.Save(draft); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "save draft: "+err.Error()))
		return
	}

	ctx.Respond(true, draft, nil)
}

// ---------- media.drafts.approve ----------

func handleMediaDraftsApprove(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing draft id"))
		return
	}

	// 加载草稿以验证当前状态
	draft, err := sub.DraftStore.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get draft: "+err.Error()))
		return
	}

	// 仅允许从 pending_review 状态审批
	if draft.Status != media.DraftStatusPendingReview {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"draft cannot be approved: current status is "+string(draft.Status)+", expected pending_review"))
		return
	}

	if err := sub.DraftStore.UpdateStatus(id, media.DraftStatusApproved); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "approve draft: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":     true,
		"id":     id,
		"status": "approved",
	}, nil)
}

// ---------- media.publish.list ----------

func handleMediaPublishList(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	if sub.PublishHistory == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "publish history not available"))
		return
	}

	var opts *media.PublishListOptions
	limit, hasLimit := ctx.Params["limit"].(float64)
	offset, hasOffset := ctx.Params["offset"].(float64)
	if hasLimit || hasOffset {
		opts = &media.PublishListOptions{}
		if hasLimit && limit > 0 {
			opts.Limit = int(limit)
		}
		if hasOffset && offset > 0 {
			opts.Offset = int(offset)
		}
	}

	records, err := sub.PublishHistory.List(opts)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "list publish history: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"records": records,
		"count":   len(records),
	}, nil)
}

// ---------- media.publish.get ----------

func handleMediaPublishGet(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	if sub.PublishHistory == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "publish history not available"))
		return
	}

	id, _ := ctx.Params["id"].(string)
	if id == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "missing publish record id"))
		return
	}

	record, err := sub.PublishHistory.Get(id)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "get publish record: "+err.Error()))
		return
	}

	ctx.Respond(true, record, nil)
}

// ---------- media.config.get ----------

func handleMediaConfigGet(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	// 热加载最新配置
	liveCfg := mediaLiveConfig(ctx)
	syncMediaTrendingSources(sub, liveCfg)

	// 热点来源
	sources, enabledSources, enabledSourcesConfigured := resolveMediaSourceState(liveCfg, sub.Aggregator.SourceNames())

	// 工具列表 — 使用 DefaultMediaToolDefs 获取 enabled/scope 完整信息
	toolsCfg := media.MediaToolsConfig{
		DraftStore:     sub.DraftStore,
		Aggregator:     sub.Aggregator,
		EnablePublish:  sub.PublishHistory != nil,
		EnableInteract: sub.GetTool(media.ToolSocialInteract) != nil,
	}
	toolDefs := media.DefaultMediaToolDefs(toolsCfg)
	tools := make([]map[string]interface{}, 0, len(toolDefs)+2)
	for _, d := range toolDefs {
		status, configured := mediaToolState(d.Name, d.Enabled, liveCfg)
		tools = append(tools, map[string]interface{}{
			"name":        d.Name,
			"description": d.Description,
			"enabled":     d.Enabled,
			"configured":  configured,
			"scope":       "media",
			"status":      status,
		})
	}
	// 共享 runner 工具
	tools = appendSharedMediaTools(tools, liveCfg)

	// 发布器
	publishers := configuredPublishers(liveCfg)
	publishConfigured := len(publishers) > 0
	publisherProfiles := buildMediaPublisherProfiles(liveCfg, ctx.Context.ChannelMgr)
	trendingSourceProfiles := buildMediaTrendingSourceProfiles(liveCfg)

	// LLM 配置（从 live config 读取）
	llmConfig := map[string]interface{}{
		"provider":            "",
		"model":               "",
		"apiKey":              "",
		"baseUrl":             "",
		"autoSpawnEnabled":    false,
		"maxAutoSpawnsPerDay": 5,
	}
	configStatus := "default"
	if liveCfg != nil && liveCfg.SubAgents != nil && liveCfg.SubAgents.MediaAgent != nil {
		ma := liveCfg.SubAgents.MediaAgent
		llmConfig["provider"] = ma.Provider
		llmConfig["model"] = ma.Model
		llmConfig["apiKey"] = maskSecret(ma.APIKey)
		llmConfig["baseUrl"] = ma.BaseURL
		llmConfig["autoSpawnEnabled"] = ma.AutoSpawnEnabled
		if ma.MaxAutoSpawnsPerDay > 0 {
			llmConfig["maxAutoSpawnsPerDay"] = ma.MaxAutoSpawnsPerDay
		}
		if ma.Provider != "" || ma.Model != "" {
			configStatus = "configured"
		}
	}

	// 高级热点策略配置
	trendingStrategy := map[string]interface{}{
		"hotKeywords":        []string{},
		"monitorIntervalMin": 30,
		"trendingThreshold":  float64(10000),
		"contentCategories":  []string{},
		"autoDraftEnabled":   false,
	}
	// 来源配置状态（区分 nil 未配置 vs [] 显式全禁用）
	if liveCfg != nil && liveCfg.SubAgents != nil && liveCfg.SubAgents.MediaAgent != nil {
		ma := liveCfg.SubAgents.MediaAgent
		enabledSourcesConfigured = ma.EnabledSources != nil
		if len(ma.HotKeywords) > 0 {
			trendingStrategy["hotKeywords"] = ma.HotKeywords
		}
		if ma.MonitorIntervalMin > 0 {
			trendingStrategy["monitorIntervalMin"] = ma.MonitorIntervalMin
		}
		if ma.TrendingThreshold != nil {
			trendingStrategy["trendingThreshold"] = *ma.TrendingThreshold
		}
		if len(ma.ContentCategories) > 0 {
			trendingStrategy["contentCategories"] = ma.ContentCategories
		}
		trendingStrategy["autoDraftEnabled"] = ma.AutoDraftEnabled
	}

	result := map[string]interface{}{
		"agent_id":                   "oa-media",
		"label":                      "媒体运营智能体",
		"status":                     configStatus,
		"trending_sources":           sources,
		"tools":                      tools,
		"publishers":                 publishers,
		"publisher_profiles":         publisherProfiles,
		"trending_source_profiles":   trendingSourceProfiles,
		"publish_enabled":            sub.PublishHistory != nil,
		"publish_configured":         publishConfigured,
		"llm":                        llmConfig,
		"trending_strategy":          trendingStrategy,
		"enabled_sources":            enabledSources,
		"enabled_sources_configured": enabledSourcesConfigured,
	}
	attachConfigHash(ctx.Context.ConfigLoader, result)
	ctx.Respond(true, result, nil)
}

// ---------- media.config.update ----------

func handleMediaConfigUpdate(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}

	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read config: "+err.Error()))
		return
	}
	baseHashChecked, ok := requireConfigBaseHashIfProvided(ctx.Params, snapshot, ctx.Respond)
	if !ok {
		return
	}

	// 加载最新配置
	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	// 确保 SubAgents.MediaAgent 存在
	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.MediaAgent == nil {
		cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
	}
	ma := cfg.SubAgents.MediaAgent

	// 合入参数
	if v, ok := ctx.Params["provider"].(string); ok {
		ma.Provider = sanitizeTextPatch(v)
	}
	if v, ok := ctx.Params["model"].(string); ok {
		ma.Model = sanitizeTextPatch(v)
	}
	if v, ok := ctx.Params["apiKey"].(string); ok && !strings.Contains(v, "****") {
		ma.APIKey = sanitizeTextPatch(v)
	}
	if v, ok := ctx.Params["baseUrl"].(string); ok {
		ma.BaseURL = sanitizeTextPatch(v)
	}
	if v, ok := ctx.Params["autoSpawnEnabled"].(bool); ok {
		ma.AutoSpawnEnabled = v
	}
	if v, ok := ctx.Params["maxAutoSpawnsPerDay"].(float64); ok && v > 0 {
		if v > 100 {
			v = 100
		}
		ma.MaxAutoSpawnsPerDay = int(v)
	}

	// 高级热点策略字段
	if v, ok := ctx.Params["hotKeywords"].([]interface{}); ok {
		keywords := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				keywords = append(keywords, s)
			}
		}
		ma.HotKeywords = keywords
	}
	if v, ok := ctx.Params["monitorIntervalMin"].(float64); ok && v > 0 {
		if v < 5 {
			v = 5 // 最小 5 分钟
		}
		if v > 1440 { // 最大 24 小时
			v = 1440
		}
		ma.MonitorIntervalMin = int(v)
	}
	if v, ok := ctx.Params["trendingThreshold"].(float64); ok && v >= 0 {
		ma.TrendingThreshold = &v
	}
	if v, ok := ctx.Params["contentCategories"].([]interface{}); ok {
		cats := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				cats = append(cats, s)
			}
		}
		ma.ContentCategories = cats
	}
	if v, ok := ctx.Params["autoDraftEnabled"].(bool); ok {
		ma.AutoDraftEnabled = v
	}

	if bochaPatch := asConfigPatch(ctx.Params["trendingBocha"]); bochaPatch != nil {
		applyTrendingBochaPatch(ma, bochaPatch)
	}
	if customOpenAIPatch := asConfigPatch(ctx.Params["trendingCustomOpenAI"]); customOpenAIPatch != nil {
		applyTrendingCustomOpenAIPatch(ma, customOpenAIPatch)
	}

	if wechatPatch := asConfigPatch(ctx.Params["wechat"]); wechatPatch != nil {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.WeChatMP == nil {
			cfg.Channels.WeChatMP = &types.WeChatMPConfig{}
		}
		applyWeChatPublisherPatch(cfg.Channels.WeChatMP, wechatPatch)
	}

	if xhsPatch := asConfigPatch(ctx.Params["xiaohongshu"]); xhsPatch != nil {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.Xiaohongshu == nil {
			cfg.Channels.Xiaohongshu = &types.XiaohongshuConfig{}
		}
		applyXiaohongshuPublisherPatch(cfg.Channels.Xiaohongshu, xhsPatch)
	}

	if websitePatch := asConfigPatch(ctx.Params["website"]); websitePatch != nil {
		if cfg.Channels == nil {
			cfg.Channels = &types.ChannelsConfig{}
		}
		if cfg.Channels.Website == nil {
			cfg.Channels.Website = &types.WebsiteConfig{}
		}
		applyWebsitePublisherPatch(cfg.Channels.Website, websitePatch)
	}

	validationErrs := config.ValidateOpenAcosmiConfig(cfg)
	if len(validationErrs) > 0 {
		ctx.Respond(false, nil, configValidationErrorShape(validationErrs))
		return
	}

	// 写入配置文件
	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}
	loader.ClearCache()
	if freshCfg, err := loader.LoadConfig(); err == nil && freshCfg != nil {
		cfg = freshCfg
	}
	ctx.Context.Config = cfg
	syncMediaTrendingSources(sub, cfg)
	monitorReloaded := reloadChannelMonitor(ctx, loader)

	slog.Info("media config updated",
		"provider", ma.Provider,
		"model", ma.Model,
		"autoSpawn", ma.AutoSpawnEnabled,
	)

	result := buildConfigWriteSuccessResult(loader, "media.config.update", cfg, monitorReloaded, nil, "", nil)
	if verification, ok := result["verification"].(map[string]interface{}); ok {
		verification["baseHashChecked"] = baseHashChecked
		verification["mediaSubsystemSynced"] = true
		if !baseHashChecked {
			verification["legacyUnsafeWrite"] = true
		}
	}
	ctx.Respond(true, result, nil)
}

func xiaohongshuLoginSessionPayload(session *channelxhs.LoginSessionState) map[string]interface{} {
	if session == nil {
		return map[string]interface{}{
			"platform": "xiaohongshu",
			"status":   "not_started",
		}
	}
	payload := map[string]interface{}{
		"platform":     "xiaohongshu",
		"status":       session.Status,
		"message":      session.Message,
		"cookiePath":   session.CookiePath,
		"loginUrl":     session.LoginURL,
		"targetId":     session.TargetID,
		"browserReady": session.BrowserReady,
		"startedAt":    session.StartedAt,
		"updatedAt":    session.UpdatedAt,
		"cookieCount":  session.CookieCount,
	}
	if strings.TrimSpace(session.CurrentURL) != "" {
		payload["currentUrl"] = session.CurrentURL
	}
	if session.SavedAt != nil {
		payload["savedAt"] = *session.SavedAt
	}
	if strings.TrimSpace(session.LastError) != "" {
		payload["lastError"] = session.LastError
	}
	return payload
}

func ensureXiaohongshuLoginClient(ctx *MethodHandlerContext) (*channelxhs.XHSRPAClient, string, error) {
	var liveCfg *types.OpenAcosmiConfig
	loader := ctx.Context.ConfigLoader
	if loader != nil {
		if fresh, err := loader.LoadConfig(); err == nil {
			liveCfg = fresh
		}
	}
	if liveCfg == nil {
		if ctx.Context.Config != nil {
			liveCfg = ctx.Context.Config
		} else {
			liveCfg = &types.OpenAcosmiConfig{}
		}
	}
	if liveCfg.Channels == nil {
		liveCfg.Channels = &types.ChannelsConfig{}
	}
	if liveCfg.Channels.Xiaohongshu == nil {
		liveCfg.Channels.Xiaohongshu = &types.XiaohongshuConfig{}
	}

	xhsCfg := liveCfg.Channels.Xiaohongshu
	changed := false
	if strings.TrimSpace(xhsCfg.CookiePath) == "" {
		xhsCfg.CookiePath = defaultXiaohongshuCookiePath(xhsCfg.AccountID)
		changed = true
	}
	if xhsCfg.AutoInteractInterval <= 0 {
		xhsCfg.AutoInteractInterval = 30
		changed = true
	}
	if xhsCfg.RateLimitSeconds <= 0 {
		xhsCfg.RateLimitSeconds = 5
		changed = true
	}
	if strings.TrimSpace(xhsCfg.ErrorScreenshotDir) == "" {
		xhsCfg.ErrorScreenshotDir = "_media/xhs/errors"
		changed = true
	}
	if changed && loader != nil {
		if err := loader.WriteConfigFile(liveCfg); err != nil {
			return nil, "", fmt.Errorf("save xiaohongshu login defaults: %w", err)
		}
		loader.ClearCache()
	}

	mgr := ctx.Context.ChannelMgr
	if mgr == nil {
		return nil, "", fmt.Errorf("channel manager not available")
	}

	plugin, _ := mgr.GetPlugin(media.ChannelXiaohongshu).(*channelxhs.XiaohongshuPlugin)
	if plugin == nil {
		plugin = channelxhs.NewXiaohongshuPlugin()
		mgr.RegisterPlugin(plugin)
		slog.Info("channel: xiaohongshu plugin registered on demand for login executor")
	}

	client, err := plugin.EnsureAccount(channels.DefaultAccountID, &channelxhs.XiaohongshuConfig{
		Enabled:              xhsCfg.Enabled,
		CookiePath:           xhsCfg.CookiePath,
		AutoInteractInterval: xhsCfg.AutoInteractInterval,
		RateLimitSeconds:     xhsCfg.RateLimitSeconds,
		ErrorScreenshotDir:   xhsCfg.ErrorScreenshotDir,
	})
	if err != nil {
		return nil, "", err
	}

	if ctx.Context.MediaSubsystem != nil && xhsCfg.Enabled {
		ctx.Context.MediaSubsystem.RegisterPublisher(media.PlatformXiaohongshu, client)
		if interactor := plugin.GetInteractionManager(channels.DefaultAccountID); interactor != nil {
			ctx.Context.MediaSubsystem.RegisterInteractor(interactor)
		}
	}
	if err := mgr.StartChannel(media.ChannelXiaohongshu, channels.DefaultAccountID); err != nil {
		slog.Warn("channel: xiaohongshu start for login executor failed (non-fatal)", "error", err)
	}
	return client, xhsCfg.CookiePath, nil
}

// ---------- media.publisher.login.start ----------

func handleMediaPublisherLoginStart(ctx *MethodHandlerContext) {
	platform, _ := ctx.Params["platform"].(string)
	if strings.TrimSpace(platform) == "" {
		platform = "xiaohongshu"
	}
	if platform != "xiaohongshu" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "automatic browser login currently only supports xiaohongshu"))
		return
	}

	client, _, err := ensureXiaohongshuLoginClient(ctx)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "prepare xiaohongshu login executor: "+err.Error()))
		return
	}

	session, err := client.StartLoginFlow(ctx.Ctx, channels.DefaultAccountID)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "start xiaohongshu login flow: "+err.Error()))
		return
	}
	ctx.Respond(true, xiaohongshuLoginSessionPayload(session), nil)
}

// ---------- media.publisher.login.wait ----------

func handleMediaPublisherLoginWait(ctx *MethodHandlerContext) {
	platform, _ := ctx.Params["platform"].(string)
	if strings.TrimSpace(platform) == "" {
		platform = "xiaohongshu"
	}
	if platform != "xiaohongshu" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "automatic browser login currently only supports xiaohongshu"))
		return
	}

	client, _, err := ensureXiaohongshuLoginClient(ctx)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "prepare xiaohongshu login executor: "+err.Error()))
		return
	}

	timeoutMs := 90000
	if raw, ok := ctx.Params["timeoutMs"].(float64); ok && raw > 0 {
		timeoutMs = int(raw)
	}

	session, err := client.WaitLoginFlow(ctx.Ctx, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "wait xiaohongshu login flow: "+err.Error()))
		return
	}
	ctx.Respond(true, xiaohongshuLoginSessionPayload(session), nil)
}

// ---------- media.tools.list ----------

func handleMediaToolsList(ctx *MethodHandlerContext) {
	sub := ctx.Context.MediaSubsystem
	if sub == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "media subsystem not available"))
		return
	}
	liveCfg := mediaLiveConfig(ctx)
	syncMediaTrendingSources(sub, liveCfg)

	cfg := media.MediaToolsConfig{
		DraftStore:     sub.DraftStore,
		Aggregator:     sub.Aggregator,
		EnablePublish:  sub.PublishHistory != nil,
		EnableInteract: sub.GetTool(media.ToolSocialInteract) != nil,
	}
	defs := media.DefaultMediaToolDefs(cfg)

	result := make([]map[string]interface{}, 0, len(defs)+3)
	for _, d := range defs {
		result = append(result, map[string]interface{}{
			"name":        d.Name,
			"description": d.Description,
			"enabled":     d.Enabled,
			"scope":       "media",
		})
	}

	// 共享 runner 工具（媒体子智能体运行时自动获得）— 热加载配置
	result = appendSharedMediaTools(result, liveCfg)

	ctx.Respond(true, map[string]interface{}{
		"tools": result,
		"count": len(result),
	}, nil)
}

// ---------- media.tools.toggle ----------

func handleMediaToolsToggle(ctx *MethodHandlerContext) {
	tool, _ := ctx.Params["tool"].(string)
	enabled, hasEnabled := ctx.Params["enabled"].(bool)
	if tool == "" || !hasEnabled {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "tool and enabled required"))
		return
	}

	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.MediaAgent == nil {
		cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
	}
	ma := cfg.SubAgents.MediaAgent

	switch tool {
	case "media_publish":
		ma.EnablePublish = &enabled
	case "social_interact":
		ma.EnableInteract = &enabled
	default:
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "tool toggle not supported for: "+tool))
		return
	}

	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}

	slog.Info("media tool toggled", "tool", tool, "enabled", enabled)
	ctx.Respond(true, map[string]interface{}{"ok": true, "tool": tool, "enabled": enabled}, nil)
}

// ---------- media.sources.toggle ----------

func handleMediaSourcesToggle(ctx *MethodHandlerContext) {
	source, _ := ctx.Params["source"].(string)
	enabled, hasEnabled := ctx.Params["enabled"].(bool)
	if source == "" || !hasEnabled {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "source and enabled required"))
		return
	}

	validSources := map[string]bool{}
	for _, name := range knownMediaSources {
		validSources[name] = true
	}
	if !validSources[source] {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "unknown source: "+source))
		return
	}

	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	cfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}

	if cfg.SubAgents == nil {
		cfg.SubAgents = &types.SubAgentConfig{}
	}
	if cfg.SubAgents.MediaAgent == nil {
		cfg.SubAgents.MediaAgent = &types.MediaAgentSettings{}
	}
	ma := cfg.SubAgents.MediaAgent

	// nil 语义: nil=全部启用。首次 toggle 时展开为完整列表再操作。
	if ma.EnabledSources == nil {
		_, allSources, _ := resolveMediaSourceState(cfg, nil)
		ma.EnabledSources = allSources
	}

	if enabled {
		// 从 EnabledSources 中确保存在
		found := false
		for _, s := range ma.EnabledSources {
			if s == source {
				found = true
				break
			}
		}
		if !found {
			ma.EnabledSources = append(ma.EnabledSources, source)
		}
	} else {
		// 从 EnabledSources 中移除
		filtered := make([]string, 0, len(ma.EnabledSources))
		for _, s := range ma.EnabledSources {
			if s != source {
				filtered = append(filtered, s)
			}
		}
		ma.EnabledSources = filtered
	}

	if err := loader.WriteConfigFile(cfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}
	ctx.Context.Config = cfg
	syncMediaTrendingSources(ctx.Context.MediaSubsystem, cfg)

	slog.Info("media source toggled", "source", source, "enabled", enabled)
	ctx.Respond(true, map[string]interface{}{"ok": true, "source": source, "enabled": enabled}, nil)
}
