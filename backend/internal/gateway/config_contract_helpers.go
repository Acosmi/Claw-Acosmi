package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	cfgstore "github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func requireConfigBaseHashIfProvided(
	params map[string]interface{},
	snapshot *types.ConfigFileSnapshot,
	respond RespondFunc,
) (checked bool, ok bool) {
	if resolveBaseHash(params) == "" {
		return false, true
	}
	return true, requireConfigBaseHash(params, snapshot, respond)
}

func attachConfigHash(loader *cfgstore.ConfigLoader, payload map[string]interface{}) {
	if loader == nil || payload == nil {
		return
	}
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil || snapshot == nil || strings.TrimSpace(snapshot.Hash) == "" {
		return
	}
	payload["hash"] = snapshot.Hash
}

func hashJSONValue(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type configMutationOptions struct {
	Action     string
	Mutate     func(cfg *types.OpenAcosmiConfig) error
	AfterWrite func(ctx *MethodHandlerContext, cfg *types.OpenAcosmiConfig) map[string]interface{}
}

func executeConfigMutation(ctx *MethodHandlerContext, opts configMutationOptions) {
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

	currentCfg, err := loader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to load config: "+err.Error()))
		return
	}
	if currentCfg == nil {
		currentCfg = &types.OpenAcosmiConfig{}
	}

	if err := opts.Mutate(currentCfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, err.Error()))
		return
	}

	validationErrs := cfgstore.ValidateOpenAcosmiConfig(currentCfg)
	if len(validationErrs) > 0 {
		ctx.Respond(false, nil, configValidationErrorShape(validationErrs))
		return
	}

	if err := loader.WriteConfigFile(currentCfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to save config: "+err.Error()))
		return
	}
	loader.ClearCache()

	freshCfg := currentCfg
	if loaded, err := loader.LoadConfig(); err == nil && loaded != nil {
		freshCfg = loaded
	}
	if ctx.Context != nil {
		ctx.Context.Config = freshCfg
	}

	monitorReloaded := reloadChannelMonitor(ctx, loader)
	result := buildConfigWriteSuccessResult(loader, opts.Action, freshCfg, monitorReloaded, nil, "", nil)
	if verification, ok := result["verification"].(map[string]interface{}); ok {
		verification["baseHashChecked"] = baseHashChecked
		if !baseHashChecked {
			verification["legacyUnsafeWrite"] = true
		}
	}
	if opts.AfterWrite != nil {
		for key, value := range opts.AfterWrite(ctx, freshCfg) {
			if key == "verification" {
				if extra, ok := value.(map[string]interface{}); ok {
					if verification, ok := result["verification"].(map[string]interface{}); ok {
						for vk, vv := range extra {
							verification[vk] = vv
						}
					}
				}
				continue
			}
			result[key] = value
		}
	}

	ctx.Respond(true, result, nil)
}

func readTrimmedStringParam(params map[string]interface{}, key string) (string, bool) {
	raw, ok := params[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func readOptionalBoolParam(params map[string]interface{}, key string) (bool, bool) {
	raw, ok := params[key]
	if !ok {
		return false, false
	}
	value, ok := raw.(bool)
	return value, ok
}

func readOptionalIntParam(params map[string]interface{}, key string) (int, bool, error) {
	raw, ok := params[key]
	if !ok {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true, nil
	case int:
		return v, true, nil
	case int64:
		return int(v), true, nil
	default:
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
}
