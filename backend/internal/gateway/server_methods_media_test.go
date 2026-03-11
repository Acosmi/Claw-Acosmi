package gateway

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestResolveMediaSourceState_DefaultSourcesAreNotMarkedConfigured(t *testing.T) {
	sources, enabled, explicitlyConfigured := resolveMediaSourceState(&types.OpenAcosmiConfig{}, []string{"weibo", "baidu", "zhihu"})
	if explicitlyConfigured {
		t.Fatal("expected default sources to be treated as not explicitly configured")
	}
	if len(enabled) != 3 {
		t.Fatalf("enabled sources = %d, want 3", len(enabled))
	}

	byName := make(map[string]map[string]interface{}, len(sources))
	for _, source := range sources {
		name, _ := source["name"].(string)
		byName[name] = source
	}
	for _, name := range []string{"weibo", "baidu", "zhihu"} {
		entry := byName[name]
		if entry == nil {
			t.Fatalf("missing source %q", name)
		}
		if entry["status"] != "default_enabled" {
			t.Fatalf("source %q status = %v, want default_enabled", name, entry["status"])
		}
		if configured, _ := entry["configured"].(bool); configured {
			t.Fatalf("source %q configured = true, want false", name)
		}
		if enabled, _ := entry["enabled"].(bool); !enabled {
			t.Fatalf("source %q enabled = false, want true", name)
		}
	}

	bocha := byName["bocha"]
	if bocha == nil {
		t.Fatal("missing bocha source")
	}
	if bocha["status"] != "needs_configuration" {
		t.Fatalf("bocha status = %v, want needs_configuration", bocha["status"])
	}
	if enabled, _ := bocha["enabled"].(bool); enabled {
		t.Fatal("bocha should not be enabled by default without API key")
	}
}

func TestResolveMediaSourceState_BochaBecomesConfiguredWhenAPIKeyPresent(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		SubAgents: &types.SubAgentConfig{
			MediaAgent: &types.MediaAgentSettings{
				TrendingProfiles: &types.MediaTrendingSourceProfiles{
					Bocha: &types.MediaTrendingBochaConfig{
						APIKey: "sk-bocha",
					},
					CustomOpenAI: &types.MediaTrendingCustomOpenAIConfig{
						APIKey:  "sk-openai",
						BaseURL: "https://example.com/v1",
						Model:   "sonar-pro",
					},
				},
			},
		},
	}

	sources, enabled, explicitlyConfigured := resolveMediaSourceState(cfg, []string{"weibo", "baidu", "zhihu", "bocha", "custom_openai"})
	if explicitlyConfigured {
		t.Fatal("expected default source list to remain implicit")
	}
	if len(enabled) != 5 {
		t.Fatalf("enabled sources = %d, want 5", len(enabled))
	}

	byName := make(map[string]map[string]interface{}, len(sources))
	for _, source := range sources {
		name, _ := source["name"].(string)
		byName[name] = source
	}
	if got := byName["bocha"]["status"]; got != "configured" {
		t.Fatalf("bocha status = %v, want configured", got)
	}
	if enabled, _ := byName["bocha"]["enabled"].(bool); !enabled {
		t.Fatal("bocha should be enabled when API key is configured")
	}
	if got := byName["custom_openai"]["status"]; got != "configured" {
		t.Fatalf("custom_openai status = %v, want configured", got)
	}
}

func TestMediaToolState_DoesNotTreatBuiltinToolsAsConfigured(t *testing.T) {
	status, configured := mediaToolState(media.ToolTrendingTopics, true, &types.OpenAcosmiConfig{})
	if status != "builtin" {
		t.Fatalf("status = %q, want builtin", status)
	}
	if configured {
		t.Fatal("expected builtin trending tool to be treated as not explicitly configured")
	}

	status, configured = mediaToolState(media.ToolContentCompose, true, &types.OpenAcosmiConfig{})
	if status != "builtin" {
		t.Fatalf("status = %q, want builtin", status)
	}
	if configured {
		t.Fatal("expected builtin compose tool to be treated as not explicitly configured")
	}
}

func TestMediaToolState_PublishRequiresChannelConfiguration(t *testing.T) {
	status, configured := mediaToolState(media.ToolMediaPublish, true, &types.OpenAcosmiConfig{})
	if status != "needs_configuration" {
		t.Fatalf("status = %q, want needs_configuration", status)
	}
	if configured {
		t.Fatal("expected publish tool to be unconfigured without publisher credentials")
	}

	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Website: &types.WebsiteConfig{
				Enabled:   true,
				APIURL:    "https://example.com/api/posts",
				AuthType:  "bearer",
				AuthToken: "token",
			},
		},
	}
	status, configured = mediaToolState(media.ToolMediaPublish, true, cfg)
	if status != "configured" {
		t.Fatalf("status = %q, want configured", status)
	}
	if !configured {
		t.Fatal("expected publish tool to be configured when website publishing is configured")
	}
}

func TestMediaToolState_InteractRequiresXiaohongshuConfiguration(t *testing.T) {
	status, configured := mediaToolState(media.ToolSocialInteract, true, &types.OpenAcosmiConfig{})
	if status != "needs_configuration" {
		t.Fatalf("status = %q, want needs_configuration", status)
	}
	if configured {
		t.Fatal("expected social interact tool to be unconfigured without xiaohongshu credentials")
	}

	cfg := &types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			Xiaohongshu: &types.XiaohongshuConfig{
				Enabled:    true,
				CookiePath: "/tmp/xhs-cookie.json",
			},
		},
	}
	status, configured = mediaToolState(media.ToolSocialInteract, true, cfg)
	if status != "configured" {
		t.Fatalf("status = %q, want configured", status)
	}
	if !configured {
		t.Fatal("expected social interact tool to be configured when xiaohongshu is configured")
	}
}

func TestBuildMediaPublisherProfiles_ReturnsMaskedCredentials(t *testing.T) {
	profiles := buildMediaPublisherProfiles(&types.OpenAcosmiConfig{
		Channels: &types.ChannelsConfig{
			WeChatMP: &types.WeChatMPConfig{
				Enabled:     true,
				AccountName: "品牌公众号",
				AccountID:   "gh_brand",
				AppID:       "wx12345678",
				AppSecret:   "supersecret1234",
			},
			Xiaohongshu: &types.XiaohongshuConfig{
				Enabled:              true,
				AccountName:          "品牌小红书",
				AccountID:            "xhs_001",
				CookiePath:           "/tmp/xhs-cookie.json",
				AutoInteractInterval: 45,
			},
			Website: &types.WebsiteConfig{
				Enabled:   true,
				SiteName:  "品牌官网",
				APIURL:    "https://example.com/api/posts",
				AuthType:  "bearer",
				AuthToken: "webtoken1234",
			},
		},
	}, nil)

	wechat := profiles["wechat"]
	if wechat == nil {
		t.Fatal("missing wechat profile")
	}
	if got := wechat["status"]; got != "configured" {
		t.Fatalf("wechat status = %v, want configured", got)
	}
	if got := wechat["appSecret"]; got != "supe****1234" {
		t.Fatalf("wechat appSecret = %v, want masked secret", got)
	}
	if got := wechat["accountName"]; got != "品牌公众号" {
		t.Fatalf("wechat accountName = %v, want 品牌公众号", got)
	}

	xhs := profiles["xiaohongshu"]
	if xhs == nil {
		t.Fatal("missing xiaohongshu profile")
	}
	if got := xhs["status"]; got != "configured" {
		t.Fatalf("xiaohongshu status = %v, want configured", got)
	}
	if got := xhs["autoInteractInterval"]; got != 45 {
		t.Fatalf("xiaohongshu autoInteractInterval = %v, want 45", got)
	}
	if got := xhs["rateLimitSeconds"]; got != 5 {
		t.Fatalf("xiaohongshu rateLimitSeconds = %v, want default 5", got)
	}
	if got := xhs["authStatus"]; got != "not_logged_in" {
		t.Fatalf("xiaohongshu authStatus = %v, want not_logged_in", got)
	}

	website := profiles["website"]
	if website == nil {
		t.Fatal("missing website profile")
	}
	if got := website["status"]; got != "configured" {
		t.Fatalf("website status = %v, want configured", got)
	}
	if got := website["authToken"]; got != "webt****1234" {
		t.Fatalf("website authToken = %v, want masked token", got)
	}
	if got := website["siteName"]; got != "品牌官网" {
		t.Fatalf("website siteName = %v, want 品牌官网", got)
	}
}

func TestBuildMediaTrendingSourceProfiles_ReturnsMaskedCredentials(t *testing.T) {
	profiles := buildMediaTrendingSourceProfiles(&types.OpenAcosmiConfig{
		SubAgents: &types.SubAgentConfig{
			MediaAgent: &types.MediaAgentSettings{
				TrendingProfiles: &types.MediaTrendingSourceProfiles{
					Bocha: &types.MediaTrendingBochaConfig{
						APIKey:    "bocha-secret-1234",
						BaseURL:   "https://api.bochaai.com",
						Freshness: "oneWeek",
					},
					CustomOpenAI: &types.MediaTrendingCustomOpenAIConfig{
						APIKey:        "openai-secret-1234",
						BaseURL:       "https://example.com/v1",
						Model:         "sonar-pro",
						SystemPrompt:  "Return JSON only.",
						RequestExtras: "{\"web_search\":true}",
					},
				},
			},
		},
	})

	bocha := profiles["bocha"]
	if bocha == nil {
		t.Fatal("missing bocha profile")
	}
	if got := bocha["status"]; got != "configured" {
		t.Fatalf("bocha status = %v, want configured", got)
	}
	if got := bocha["apiKey"]; got != "boch****1234" {
		t.Fatalf("bocha apiKey = %v, want masked API key", got)
	}
	if got := bocha["freshness"]; got != "oneWeek" {
		t.Fatalf("bocha freshness = %v, want oneWeek", got)
	}

	customOpenAI := profiles["custom_openai"]
	if customOpenAI == nil {
		t.Fatal("missing custom_openai profile")
	}
	if got := customOpenAI["status"]; got != "configured" {
		t.Fatalf("custom_openai status = %v, want configured", got)
	}
	if got := customOpenAI["apiKey"]; got != "open****1234" {
		t.Fatalf("custom_openai apiKey = %v, want masked API key", got)
	}
	if got := customOpenAI["model"]; got != "sonar-pro" {
		t.Fatalf("custom_openai model = %v, want sonar-pro", got)
	}
}

func TestApplyMediaPublisherPatches_SupportMaskedNoopAndClear(t *testing.T) {
	wechat := &types.WeChatMPConfig{
		Enabled:   true,
		AppID:     "wx-old",
		AppSecret: "secret-old",
	}
	applyWeChatPublisherPatch(wechat, map[string]interface{}{
		"accountName": "品牌公众号",
		"appSecret":   "****",
	})
	if wechat.AppSecret != "secret-old" {
		t.Fatalf("wechat AppSecret = %q, want original value", wechat.AppSecret)
	}
	applyWeChatPublisherPatch(wechat, map[string]interface{}{
		"appId":     "wx-new",
		"appSecret": "",
	})
	if wechat.AppID != "wx-new" {
		t.Fatalf("wechat AppID = %q, want wx-new", wechat.AppID)
	}
	if wechat.AppSecret != "" {
		t.Fatalf("wechat AppSecret = %q, want cleared", wechat.AppSecret)
	}

	xhs := &types.XiaohongshuConfig{}
	applyXiaohongshuPublisherPatch(xhs, map[string]interface{}{
		"enabled":              true,
		"accountName":          "品牌小红书",
		"accountId":            "xhs_002",
		"cookiePath":           "/tmp/new-cookie.json",
		"autoInteractInterval": 1500.0,
		"rateLimitSeconds":     1.0,
	})
	if !xhs.Enabled {
		t.Fatal("expected xiaohongshu profile to be enabled")
	}
	if xhs.AccountID != "xhs_002" {
		t.Fatalf("xiaohongshu AccountID = %q, want xhs_002", xhs.AccountID)
	}
	if xhs.AutoInteractInterval != 1440 {
		t.Fatalf("xiaohongshu AutoInteractInterval = %d, want 1440", xhs.AutoInteractInterval)
	}
	if xhs.RateLimitSeconds != 3 {
		t.Fatalf("xiaohongshu RateLimitSeconds = %d, want 3", xhs.RateLimitSeconds)
	}

	website := &types.WebsiteConfig{
		AuthToken: "keepme",
	}
	applyWebsitePublisherPatch(website, map[string]interface{}{
		"siteName":       "品牌官网",
		"authToken":      "****",
		"timeoutSeconds": 700.0,
		"maxRetries":     -2.0,
	})
	if website.AuthToken != "keepme" {
		t.Fatalf("website AuthToken = %q, want preserved", website.AuthToken)
	}
	if website.TimeoutSeconds != 600 {
		t.Fatalf("website TimeoutSeconds = %d, want 600", website.TimeoutSeconds)
	}
	if website.MaxRetries != 0 {
		t.Fatalf("website MaxRetries = %d, want 0", website.MaxRetries)
	}
	applyWebsitePublisherPatch(website, map[string]interface{}{
		"authToken": "",
		"authType":  "basic",
	})
	if website.AuthToken != "" {
		t.Fatalf("website AuthToken = %q, want cleared", website.AuthToken)
	}
	if website.AuthType != "basic" {
		t.Fatalf("website AuthType = %q, want basic", website.AuthType)
	}
}

func TestApplyTrendingBochaPatch_SupportsMaskedNoopAndNormalization(t *testing.T) {
	ma := &types.MediaAgentSettings{
		TrendingProfiles: &types.MediaTrendingSourceProfiles{
			Bocha: &types.MediaTrendingBochaConfig{
				APIKey:    "keepme",
				BaseURL:   "https://api.bochaai.com",
				Freshness: "oneDay",
			},
		},
	}

	applyTrendingBochaPatch(ma, map[string]interface{}{
		"apiKey":    "****",
		"freshness": "invalid-value",
	})
	if ma.TrendingProfiles.Bocha.APIKey != "keepme" {
		t.Fatalf("bocha APIKey = %q, want preserved", ma.TrendingProfiles.Bocha.APIKey)
	}
	if ma.TrendingProfiles.Bocha.Freshness != "oneDay" {
		t.Fatalf("bocha Freshness = %q, want oneDay", ma.TrendingProfiles.Bocha.Freshness)
	}

	applyTrendingBochaPatch(ma, map[string]interface{}{
		"apiKey":    "",
		"baseUrl":   " https://api.bochaai.com ",
		"freshness": "oneMonth",
	})
	if ma.TrendingProfiles.Bocha.APIKey != "" {
		t.Fatalf("bocha APIKey = %q, want cleared", ma.TrendingProfiles.Bocha.APIKey)
	}
	if ma.TrendingProfiles.Bocha.BaseURL != "https://api.bochaai.com" {
		t.Fatalf("bocha BaseURL = %q, want trimmed", ma.TrendingProfiles.Bocha.BaseURL)
	}
	if ma.TrendingProfiles.Bocha.Freshness != "oneMonth" {
		t.Fatalf("bocha Freshness = %q, want oneMonth", ma.TrendingProfiles.Bocha.Freshness)
	}
}

func TestApplyTrendingCustomOpenAIPatch_SupportsMaskedNoopAndTrim(t *testing.T) {
	ma := &types.MediaAgentSettings{
		TrendingProfiles: &types.MediaTrendingSourceProfiles{
			CustomOpenAI: &types.MediaTrendingCustomOpenAIConfig{
				APIKey:        "keepme",
				BaseURL:       "https://example.com/v1",
				Model:         "sonar-pro",
				SystemPrompt:  "hello",
				RequestExtras: "{\"web_search\":true}",
			},
		},
	}

	applyTrendingCustomOpenAIPatch(ma, map[string]interface{}{
		"apiKey": "****",
		"model":  " sonar-max ",
	})
	if ma.TrendingProfiles.CustomOpenAI.APIKey != "keepme" {
		t.Fatalf("custom openai APIKey = %q, want preserved", ma.TrendingProfiles.CustomOpenAI.APIKey)
	}
	if ma.TrendingProfiles.CustomOpenAI.Model != "sonar-max" {
		t.Fatalf("custom openai Model = %q, want trimmed", ma.TrendingProfiles.CustomOpenAI.Model)
	}

	applyTrendingCustomOpenAIPatch(ma, map[string]interface{}{
		"apiKey":        "",
		"baseUrl":       " https://example2.com/v1 ",
		"systemPrompt":  " use live search ",
		"requestExtras": " {\"web_search\":false} ",
	})
	if ma.TrendingProfiles.CustomOpenAI.APIKey != "" {
		t.Fatalf("custom openai APIKey = %q, want cleared", ma.TrendingProfiles.CustomOpenAI.APIKey)
	}
	if ma.TrendingProfiles.CustomOpenAI.BaseURL != "https://example2.com/v1" {
		t.Fatalf("custom openai BaseURL = %q, want trimmed", ma.TrendingProfiles.CustomOpenAI.BaseURL)
	}
}
