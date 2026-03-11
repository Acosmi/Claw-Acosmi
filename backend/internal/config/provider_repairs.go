package config

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/goproviders/bridge"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// repairRuntimeProviderConfigs patches incomplete runtime provider scaffolds that
// were produced by older wizard recovery paths after state deletion.
func repairRuntimeProviderConfigs(cfg *types.OpenAcosmiConfig) []string {
	if cfg == nil {
		return nil
	}
	ensureModelsProvidersConfig(cfg)

	var changes []string
	if repairLegacyAPIKeyProvider(cfg, "qwen-portal", "qwen") {
		changes = append(changes, "Repaired incomplete models.providers.qwen-portal using qwen runtime defaults.")
	}
	if mirrorLegacyRuntimeProvider(cfg, "qwen-portal", "qwen") {
		changes = append(changes, "Mirrored legacy qwen-portal API-key config into models.providers.qwen for runtime compatibility.")
	}
	if ensureReferencedRuntimeProvider(cfg, "minimax") {
		changes = append(changes, "Restored missing or incomplete models.providers.minimax from configured model references.")
	}
	return changes
}

func repairLegacyAPIKeyProvider(cfg *types.OpenAcosmiConfig, targetProviderID, templateProviderID string) bool {
	provCfg := cfg.Models.Providers[targetProviderID]
	if provCfg == nil {
		return false
	}
	if provCfg.Auth == types.ModelAuthOAuth {
		return false
	}

	needsAPI := strings.TrimSpace(string(provCfg.API)) == ""
	needsBaseURL := strings.TrimSpace(provCfg.BaseURL) == ""
	needsModels := len(provCfg.Models) == 0
	if !needsAPI && !needsBaseURL && !needsModels {
		return false
	}

	return applyProviderTemplate(provCfg, templateProviderID)
}

func mirrorLegacyRuntimeProvider(cfg *types.OpenAcosmiConfig, sourceProviderID, targetProviderID string) bool {
	source := cfg.Models.Providers[sourceProviderID]
	if source == nil || source.Auth == types.ModelAuthOAuth {
		return false
	}

	target := cfg.Models.Providers[targetProviderID]
	created := false
	if target == nil {
		target = &types.ModelProviderConfig{}
		cfg.Models.Providers[targetProviderID] = target
		created = true
	}

	changed := created
	if strings.TrimSpace(target.APIKey) == "" && strings.TrimSpace(source.APIKey) != "" {
		target.APIKey = source.APIKey
		changed = true
	}
	if target.Auth == "" && source.Auth != "" {
		target.Auth = source.Auth
		changed = true
	}
	if strings.TrimSpace(string(target.API)) == "" && strings.TrimSpace(string(source.API)) != "" {
		target.API = source.API
		changed = true
	}
	if strings.TrimSpace(target.BaseURL) == "" && strings.TrimSpace(source.BaseURL) != "" {
		target.BaseURL = source.BaseURL
		changed = true
	}
	if len(target.Models) == 0 && len(source.Models) > 0 {
		target.Models = append([]types.ModelDefinitionConfig(nil), source.Models...)
		changed = true
	}
	if target.Headers == nil && len(source.Headers) > 0 {
		target.Headers = copyStringMap(source.Headers)
		changed = true
	}
	if target.AuthHeader == nil && source.AuthHeader != nil {
		value := *source.AuthHeader
		target.AuthHeader = &value
		changed = true
	}

	if applyProviderTemplate(target, targetProviderID) {
		changed = true
	}

	return changed
}

func ensureReferencedRuntimeProvider(cfg *types.OpenAcosmiConfig, providerID string) bool {
	if !configReferencesProvider(cfg, providerID) {
		return false
	}

	provCfg := cfg.Models.Providers[providerID]
	if provCfg == nil {
		provCfg = &types.ModelProviderConfig{}
		cfg.Models.Providers[providerID] = provCfg
	}

	return applyProviderTemplate(provCfg, providerID)
}

func configReferencesProvider(cfg *types.OpenAcosmiConfig, providerID string) bool {
	if cfg == nil || cfg.Agents == nil || cfg.Agents.Defaults == nil {
		return false
	}

	modelCfg := cfg.Agents.Defaults.Model
	if modelCfg != nil {
		if ref := ParseModelRef(modelCfg.Primary, "anthropic"); ref != nil && ref.Provider == providerID {
			return true
		}
		if modelCfg.Fallbacks != nil {
			for _, raw := range *modelCfg.Fallbacks {
				if ref := ParseModelRef(raw, "anthropic"); ref != nil && ref.Provider == providerID {
					return true
				}
			}
		}
	}

	for raw := range cfg.Agents.Defaults.Models {
		if ref := ParseModelRef(raw, "anthropic"); ref != nil && ref.Provider == providerID {
			return true
		}
	}

	return false
}

func applyProviderTemplate(provCfg *types.ModelProviderConfig, templateProviderID string) bool {
	if provCfg == nil {
		return false
	}
	needsAPI := strings.TrimSpace(string(provCfg.API)) == ""
	needsBaseURL := strings.TrimSpace(provCfg.BaseURL) == ""
	needsModels := len(provCfg.Models) == 0

	templateCfg := &types.OpenAcosmiConfig{}
	bridge.ApplyProviderByID(templateProviderID, templateCfg, &bridge.ApplyOpts{})
	if templateCfg.Models == nil || templateCfg.Models.Providers == nil {
		return false
	}
	template := templateCfg.Models.Providers[templateProviderID]
	if template == nil {
		return false
	}

	changed := false
	if needsAPI {
		provCfg.API = template.API
		changed = true
	}
	if needsBaseURL {
		provCfg.BaseURL = template.BaseURL
		changed = true
	}
	if needsModels {
		provCfg.Models = append([]types.ModelDefinitionConfig(nil), template.Models...)
		changed = true
	}
	if provCfg.Headers == nil && len(template.Headers) > 0 {
		provCfg.Headers = copyStringMap(template.Headers)
		changed = true
	}
	if provCfg.AuthHeader == nil && template.AuthHeader != nil {
		value := *template.AuthHeader
		provCfg.AuthHeader = &value
		changed = true
	}

	return changed
}

func ensureModelsProvidersConfig(cfg *types.OpenAcosmiConfig) {
	if cfg.Models == nil {
		cfg.Models = &types.ModelsConfig{}
	}
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]*types.ModelProviderConfig)
	}
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
