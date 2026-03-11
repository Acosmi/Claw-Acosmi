package media

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

type TrendingSourceDefinition struct {
	Name               string
	RequiresCredential bool
}

var knownTrendingSourceDefinitions = []TrendingSourceDefinition{
	{Name: "weibo"},
	{Name: "baidu"},
	{Name: "zhihu"},
	{Name: "bocha", RequiresCredential: true},
	{Name: "custom_openai", RequiresCredential: true},
}

func KnownTrendingSourceDefinitions() []TrendingSourceDefinition {
	defs := make([]TrendingSourceDefinition, len(knownTrendingSourceDefinitions))
	copy(defs, knownTrendingSourceDefinitions)
	return defs
}

func TrendingSourceDefinitionByName(name string) (TrendingSourceDefinition, bool) {
	for _, def := range knownTrendingSourceDefinitions {
		if def.Name == strings.TrimSpace(name) {
			return def, true
		}
	}
	return TrendingSourceDefinition{}, false
}

func IsTrendingSourceConfigured(cfg *types.OpenAcosmiConfig, name string) bool {
	switch strings.TrimSpace(name) {
	case "bocha":
		bocha := mediaBochaTrendingConfig(cfg)
		return bocha != nil && strings.TrimSpace(bocha.APIKey) != ""
	case "custom_openai":
		openai := mediaCustomOpenAITrendingConfig(cfg)
		return openai != nil &&
			strings.TrimSpace(openai.APIKey) != "" &&
			strings.TrimSpace(openai.Model) != ""
	default:
		return true
	}
}

func BuildTrendingSourcesFromConfig(cfg *types.OpenAcosmiConfig) []TrendingSource {
	defs := KnownTrendingSourceDefinitions()
	enabledExplicitly := map[string]bool{}
	ma := mediaAgentConfig(cfg)
	enabledConfigured := ma != nil && ma.EnabledSources != nil
	if enabledConfigured {
		for _, name := range ma.EnabledSources {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				enabledExplicitly[trimmed] = true
			}
		}
	}

	sources := make([]TrendingSource, 0, len(defs))
	for _, def := range defs {
		enabled := false
		if enabledConfigured {
			enabled = enabledExplicitly[def.Name]
		} else if def.RequiresCredential {
			enabled = IsTrendingSourceConfigured(cfg, def.Name)
		} else {
			enabled = true
		}
		if !enabled {
			continue
		}
		if src := buildTrendingSource(def.Name, cfg); src != nil {
			sources = append(sources, src)
		}
	}
	return sources
}

func mediaAgentConfig(cfg *types.OpenAcosmiConfig) *types.MediaAgentSettings {
	if cfg == nil || cfg.SubAgents == nil {
		return nil
	}
	return cfg.SubAgents.MediaAgent
}

func mediaBochaTrendingConfig(cfg *types.OpenAcosmiConfig) *types.MediaTrendingBochaConfig {
	ma := mediaAgentConfig(cfg)
	if ma == nil || ma.TrendingProfiles == nil {
		return nil
	}
	return ma.TrendingProfiles.Bocha
}

func mediaCustomOpenAITrendingConfig(cfg *types.OpenAcosmiConfig) *types.MediaTrendingCustomOpenAIConfig {
	ma := mediaAgentConfig(cfg)
	if ma == nil || ma.TrendingProfiles == nil {
		return nil
	}
	return ma.TrendingProfiles.CustomOpenAI
}

func buildTrendingSource(name string, cfg *types.OpenAcosmiConfig) TrendingSource {
	switch name {
	case "weibo":
		return NewWeiboTrendingSource()
	case "baidu":
		return NewBaiduTrendingSource()
	case "zhihu":
		return NewZhihuTrendingSource()
	case "bocha":
		bocha := mediaBochaTrendingConfig(cfg)
		if bocha == nil {
			bocha = &types.MediaTrendingBochaConfig{}
		}
		return NewBochaTrendingSource(BochaTrendingSourceConfig{
			APIKey:    bocha.APIKey,
			BaseURL:   bocha.BaseURL,
			Freshness: bocha.Freshness,
		})
	case "custom_openai":
		custom := mediaCustomOpenAITrendingConfig(cfg)
		if custom == nil {
			custom = &types.MediaTrendingCustomOpenAIConfig{}
		}
		return NewCustomOpenAITrendingSource(CustomOpenAITrendingSourceConfig{
			APIKey:        custom.APIKey,
			BaseURL:       custom.BaseURL,
			Model:         custom.Model,
			SystemPrompt:  custom.SystemPrompt,
			RequestExtras: custom.RequestExtras,
		})
	default:
		return nil
	}
}
