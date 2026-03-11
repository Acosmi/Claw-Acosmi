package gateway

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

type effectiveUHMSLLMConfig struct {
	Provider  string
	Model     string
	BaseURL   string
	APIKey    string
	Inherited bool
}

func resolveEffectiveUHMSLLMConfig(uhmsCfg *types.MemoryUHMSConfig, fullCfg *types.OpenAcosmiConfig) effectiveUHMSLLMConfig {
	cfg := effectiveUHMSLLMConfig{}
	if uhmsCfg != nil {
		cfg.Provider = strings.TrimSpace(uhmsCfg.LLMProvider)
		cfg.Model = strings.TrimSpace(uhmsCfg.LLMModel)
		cfg.BaseURL = strings.TrimSpace(uhmsCfg.LLMBaseURL)
		cfg.APIKey = strings.TrimSpace(uhmsCfg.LLMApiKey)
	}

	defaultRef := models.ResolveConfiguredModelRef(fullCfg, models.DefaultProvider, models.DefaultModel)
	if cfg.Provider == "" {
		cfg.Provider = strings.TrimSpace(defaultRef.Provider)
		cfg.Inherited = cfg.Provider != ""
	}
	if cfg.Model == "" {
		if cfg.Provider != "" && strings.EqualFold(cfg.Provider, defaultRef.Provider) && strings.TrimSpace(defaultRef.Model) != "" {
			cfg.Model = strings.TrimSpace(defaultRef.Model)
		} else if cfg.Provider != "" {
			cfg.Model = defaultModelForProvider(cfg.Provider)
		}
	}

	if fullCfg != nil && fullCfg.Models != nil && fullCfg.Models.Providers != nil && cfg.Provider != "" {
		if pc := fullCfg.Models.Providers[cfg.Provider]; pc != nil {
			if cfg.BaseURL == "" {
				cfg.BaseURL = strings.TrimSpace(pc.BaseURL)
			}
			if cfg.APIKey == "" {
				cfg.APIKey = strings.TrimSpace(pc.APIKey)
			}
		}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURLForProvider(cfg.Provider)
	}

	return cfg
}
