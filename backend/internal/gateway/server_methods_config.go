package gateway

// config.* 方法处理器 — 对应 src/gateway/server-methods/config.ts
//
// 提供配置的查询、设置、应用、补丁、schema 查询功能。
// 依赖: ConfigLoader (config.get/set/apply/patch), config.RedactConfigSnapshot,
//       config.RestoreRedactedValues, hujson (JSON5 解析)

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/tailscale/hujson"
)

// ---------- baseHash 辅助 ----------
// 对应 TS config.ts:L39-L92

// resolveBaseHash 从参数中提取 baseHash。
func resolveBaseHash(params map[string]interface{}) string {
	raw, ok := params["baseHash"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(raw)
}

// requireConfigBaseHash 验证 baseHash 是否匹配当前快照。
// 返回 true 表示通过验证；返回 false 表示已发送错误响应。
func requireConfigBaseHash(
	params map[string]interface{},
	snapshot *types.ConfigFileSnapshot,
	respond RespondFunc,
) bool {
	if !snapshot.Exists {
		return true
	}
	snapshotHash := snapshot.Hash
	if snapshotHash == "" {
		respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"config base hash unavailable; re-run config.get and retry").WithDetails(map[string]interface{}{
			"exists": snapshot.Exists,
		}))
		return false
	}
	baseHash := resolveBaseHash(params)
	if baseHash == "" {
		respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"config base hash required; re-run config.get and retry").WithDetails(map[string]interface{}{
			"expectedHash": snapshotHash,
		}))
		return false
	}
	if baseHash != snapshotHash {
		respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"config changed since last load; re-run config.get and retry").WithDetails(map[string]interface{}{
			"expectedHash": snapshotHash,
			"baseHash":     baseHash,
		}))
		return false
	}
	return true
}

// parseJson5Raw 将 JSON5 字符串解析为 Go 值。
// 使用 tailscale/hujson 标准化后再用 encoding/json 解析。
func parseJson5Raw(raw string) (interface{}, error) {
	standardized, err := hujson.Standardize([]byte(raw))
	if err != nil {
		return nil, err
	}
	var result interface{}
	if err := json.Unmarshal(standardized, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func summarizeConfigValidationErrors(errs []config.ValidationError) string {
	if len(errs) == 0 {
		return "ok"
	}
	limit := 3
	if len(errs) < limit {
		limit = len(errs)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		parts = append(parts, errs[i].Error())
	}
	summary := strings.Join(parts, "; ")
	if len(errs) > limit {
		summary += fmt.Sprintf(" (and %d more)", len(errs)-limit)
	}
	return summary
}

func configValidationDetails(errs []config.ValidationError) map[string]interface{} {
	return map[string]interface{}{
		"ok":         len(errs) == 0,
		"issueCount": len(errs),
		"issues":     errs,
		"summary":    summarizeConfigValidationErrors(errs),
	}
}

func configValidationErrorShape(errs []config.ValidationError) *ErrorShape {
	return NewErrorShape(ErrCodeConfigInvalid, "invalid config: "+summarizeConfigValidationErrors(errs)).
		WithDetails(configValidationDetails(errs))
}

func configValidationSuccess() map[string]interface{} {
	return configValidationDetails([]config.ValidationError{})
}

func reloadChannelMonitor(ctx *MethodHandlerContext, loader *config.ConfigLoader) bool {
	if mgr := ctx.Context.ChannelMonitorMgr; mgr != nil {
		if newCfg, err := loader.LoadConfig(); err == nil && newCfg != nil {
			mgr.Reload(newCfg)
			return true
		}
	}
	return false
}

func buildConfigWriteSuccessResult(
	loader *config.ConfigLoader,
	action string,
	configObject interface{},
	monitorReloaded bool,
	restartResult *GatewayRestartResult,
	sentinelPath string,
	sentinelPayload *RestartSentinelPayload,
) map[string]interface{} {
	runtimeEffect := "written_only"
	if action == "config.apply" || action == "config.patch" {
		runtimeEffect = "restart_scheduled"
	}

	result := map[string]interface{}{
		"ok":         true,
		"path":       loader.ConfigPath(),
		"config":     config.RedactConfigObject(configObject),
		"validation": configValidationSuccess(),
		"verification": map[string]interface{}{
			"action":           action,
			"configWritten":    true,
			"monitorReloaded":  monitorReloaded,
			"runtimeEffect":    runtimeEffect,
			"restartScheduled": restartResult != nil && restartResult.Scheduled,
			"sentinelWritten":  sentinelPayload != nil,
		},
	}

	if snapshot, err := loader.ReadConfigFileSnapshot(); err == nil && snapshot != nil {
		redacted := config.RedactConfigSnapshot(snapshot)
		result["snapshot"] = redacted
		result["config"] = redacted.Config
		if snapshot.Hash != "" {
			result["hash"] = snapshot.Hash
		}
	}
	if restartResult != nil {
		result["restart"] = restartResult
	}
	if sentinelPayload != nil {
		result["sentinel"] = map[string]interface{}{
			"path":    sentinelPath,
			"payload": sentinelPayload,
		}
	}
	return result
}

// ConfigHandlers 返回 config.* 方法处理器映射。
func ConfigHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"config.get":    handleConfigGet,
		"config.schema": handleConfigSchema,
		"config.set":    handleConfigSet,
		"config.apply":  handleConfigApply,
		"config.patch":  handleConfigPatch,
	}
}

// ---------- config.get ----------
// 对应 TS config.ts:L32-L88
// 返回脱敏后的配置快照（包含 raw、parsed、config、baseHash）。

func handleConfigGet(ctx *MethodHandlerContext) {
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

	// 脱敏
	redacted := config.RedactConfigSnapshot(snapshot)
	ctx.Respond(true, redacted, nil)
}

// ---------- config.schema ----------
// 对应 TS config.ts:L90-L118
// 返回配置 schema + UI hints。

func handleConfigSchema(ctx *MethodHandlerContext) {
	version := ""
	if loader := ctx.Context.ConfigLoader; loader != nil {
		version = config.BuildVersion
	}
	schema := config.NewSchemaResponse(version)
	ctx.Respond(true, schema, nil)
}

// ---------- config.set ----------
// 对应 TS config.ts:L152-L217
// 接收 raw (JSON5 字符串) 配置，还原脱敏值后写入文件。

func handleConfigSet(ctx *MethodHandlerContext) {
	loader := ctx.Context.ConfigLoader
	if loader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	// 读取当前快照
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read config: "+err.Error()))
		return
	}

	// baseHash 验证（乐观并发控制）
	if !requireConfigBaseHash(ctx.Params, snapshot, ctx.Respond) {
		return
	}

	// 从 raw 参数解析 JSON5（对应 TS parseConfigJson5(rawValue)）
	rawValue, _ := ctx.Params["raw"].(string)
	if rawValue == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"invalid config.set params: raw (string) required"))
		return
	}
	parsed, parseErr := parseJson5Raw(rawValue)
	if parseErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, parseErr.Error()))
		return
	}

	// 验证配置结构
	parsedJSON, _ := json.Marshal(parsed)
	var candidateConfig types.OpenAcosmiConfig
	if err := json.Unmarshal(parsedJSON, &candidateConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid config format: "+err.Error()))
		return
	}
	validationErrs := config.ValidateOpenAcosmiConfig(&candidateConfig)
	if len(validationErrs) > 0 {
		ctx.Respond(false, nil, configValidationErrorShape(validationErrs))
		return
	}

	// 还原脱敏值（对应 TS restoreRedactedValues(validated.config, snapshot.config)）
	var originalParsed interface{}
	originalJSON, _ := json.Marshal(snapshot.Config)
	json.Unmarshal(originalJSON, &originalParsed)

	restored, restoreErr := config.RestoreRedactedValues(parsed, originalParsed)
	if restoreErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, restoreErr.Error()))
		return
	}

	// 写入配置文件
	restoredJSON, _ := json.Marshal(restored)
	var finalConfig types.OpenAcosmiConfig
	if err := json.Unmarshal(restoredJSON, &finalConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to serialize config: "+err.Error()))
		return
	}
	if err := loader.WriteConfigFile(&finalConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write config: "+err.Error()))
		return
	}

	loader.ClearCache()
	monitorReloaded := reloadChannelMonitor(ctx, loader)
	ctx.Respond(true, buildConfigWriteSuccessResult(
		loader,
		"config.set",
		restored,
		monitorReloaded,
		nil,
		"",
		nil,
	), nil)
}

// ---------- config.apply ----------
// 对应 TS config.ts:L349-L459
// 验证并应用配置（写入 restart sentinel + 触发网关重启）。

func handleConfigApply(ctx *MethodHandlerContext) {
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

	// 读取当前快照
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read config: "+err.Error()))
		return
	}

	// baseHash 验证
	if !requireConfigBaseHash(ctx.Params, snapshot, ctx.Respond) {
		return
	}

	// 从 raw 参数解析 JSON5
	rawValue, _ := ctx.Params["raw"].(string)
	if rawValue == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"invalid config.apply params: raw (string) required"))
		return
	}
	parsed, parseErr := parseJson5Raw(rawValue)
	if parseErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, parseErr.Error()))
		return
	}

	// 验证配置结构
	parsedJSON, _ := json.Marshal(parsed)
	var candidateConfig types.OpenAcosmiConfig
	if err := json.Unmarshal(parsedJSON, &candidateConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "invalid config format: "+err.Error()))
		return
	}
	validationErrs := config.ValidateOpenAcosmiConfig(&candidateConfig)
	if len(validationErrs) > 0 {
		ctx.Respond(false, nil, configValidationErrorShape(validationErrs))
		return
	}

	// 还原脱敏值
	var originalParsed interface{}
	originalJSON, _ := json.Marshal(snapshot.Config)
	json.Unmarshal(originalJSON, &originalParsed)

	restored, restoreErr := config.RestoreRedactedValues(parsed, originalParsed)
	if restoreErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, restoreErr.Error()))
		return
	}

	// 写入配置文件
	restoredJSON, _ := json.Marshal(restored)
	var finalConfig types.OpenAcosmiConfig
	if err := json.Unmarshal(restoredJSON, &finalConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to serialize config: "+err.Error()))
		return
	}
	if err := loader.WriteConfigFile(&finalConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write config: "+err.Error()))
		return
	}
	loader.ClearCache()
	monitorReloaded := reloadChannelMonitor(ctx, loader)

	sessionKey, _ := ctx.Params["sessionKey"].(string)
	sessionKey = strings.TrimSpace(sessionKey)
	note, _ := ctx.Params["note"].(string)
	note = strings.TrimSpace(note)
	var restartDelayMs *int
	if v, ok := ctx.Params["restartDelayMs"].(float64); ok && v >= 0 {
		ms := int(v)
		restartDelayMs = &ms
	}

	// 写入 restart sentinel（对应 TS writeRestartSentinel）
	var sentinelPath string
	var sentinelPayload *RestartSentinelPayload
	if sw := ctx.Context.RestartSentinel; sw != nil {
		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		sentinelPayload = &RestartSentinelPayload{
			Kind:       "config-apply",
			Status:     "ok",
			Ts:         time.Now().UnixMilli(),
			SessionKey: sessionKey,
			Message:    notePtr,
			DoctorHint: sw.FormatDoctorNonInteractiveHint(),
			Stats: map[string]interface{}{
				"mode": "config.apply",
				"root": loader.ConfigPath(),
			},
		}
		sentinelPath, _ = sw.WriteRestartSentinel(sentinelPayload)
	}

	// 调度网关重启（对应 TS scheduleGatewaySigusr1Restart）
	var restartResult *GatewayRestartResult
	if gr := ctx.Context.GatewayRestarter; gr != nil {
		restartResult = gr.ScheduleRestart(GatewayRestartPlan{
			DelayMs:  restartDelayMs,
			Reason:   "config.apply",
			Rollback: rollback,
		})
	}

	// 构建响应
	ctx.Respond(true, buildConfigWriteSuccessResult(
		loader,
		"config.apply",
		restored,
		monitorReloaded,
		restartResult,
		sentinelPath,
		sentinelPayload,
	), nil)
}

// ---------- config.patch ----------
// 对应 TS config.ts:L218-L348
// 接收 raw (JSON5 merge-patch 字符串) 与现有配置合并后写入。
// 合并后执行 legacy migration、验证、restart sentinel、重启调度。

func handleConfigPatch(ctx *MethodHandlerContext) {
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

	// 读取当前快照
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to read config: "+err.Error()))
		return
	}

	// baseHash 验证
	if !requireConfigBaseHash(ctx.Params, snapshot, ctx.Respond) {
		return
	}

	// 要求快照有效（对应 TS L234-L241）
	if !snapshot.Valid {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"invalid config; fix before patching"))
		return
	}

	// 从 raw 参数解析 JSON5 merge-patch
	rawValue, _ := ctx.Params["raw"].(string)
	if rawValue == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"invalid config.patch params: raw (string) required"))
		return
	}
	parsed, parseErr := parseJson5Raw(rawValue)
	if parseErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, parseErr.Error()))
		return
	}

	// 确认 patch 是对象（对应 TS L259-L270）
	patchMap, ok := parsed.(map[string]interface{})
	if !ok {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
			"config.patch raw must be an object"))
		return
	}

	// 合并 patch 到当前配置（RFC 7396 JSON Merge Patch，对应 TS applyMergePatch）
	var currentMap map[string]interface{}
	currentJSON, _ := json.Marshal(snapshot.Config)
	json.Unmarshal(currentJSON, &currentMap)
	mergedRaw := config.ApplyMergePatch(currentMap, patchMap)
	currentMap, _ = mergedRaw.(map[string]interface{})

	// 还原脱敏值（对应 TS restoreRedactedValues(merged, snapshot.config)）
	var originalParsed interface{}
	originalJSON, _ := json.Marshal(snapshot.Config)
	json.Unmarshal(originalJSON, &originalParsed)

	restored, restoreErr := config.RestoreRedactedValues(currentMap, originalParsed)
	if restoreErr != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, restoreErr.Error()))
		return
	}

	// Legacy migration（对应 TS applyLegacyMigrations(restoredMerge)）
	resolved := restored
	if lm := ctx.Context.LegacyMigrator; lm != nil {
		migrated := lm.ApplyLegacyMigrations(restored)
		if migrated != nil && migrated.Applied && migrated.Next != nil {
			resolved = migrated.Next
		}
	}

	// 验证合并后配置
	resolvedJSON, _ := json.Marshal(resolved)
	var mergedConfig types.OpenAcosmiConfig
	if err := json.Unmarshal(resolvedJSON, &mergedConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "merged config invalid: "+err.Error()))
		return
	}
	validationErrs := config.ValidateOpenAcosmiConfig(&mergedConfig)
	if len(validationErrs) > 0 {
		ctx.Respond(false, nil, configValidationErrorShape(validationErrs))
		return
	}

	// 写入配置文件
	if err := loader.WriteConfigFile(&mergedConfig); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "failed to write config: "+err.Error()))
		return
	}
	loader.ClearCache()
	monitorReloaded := reloadChannelMonitor(ctx, loader)

	sessionKey, _ := ctx.Params["sessionKey"].(string)
	sessionKey = strings.TrimSpace(sessionKey)
	note, _ := ctx.Params["note"].(string)
	note = strings.TrimSpace(note)
	var restartDelayMs *int
	if v, ok := ctx.Params["restartDelayMs"].(float64); ok && v >= 0 {
		ms := int(v)
		restartDelayMs = &ms
	}

	// 写入 restart sentinel（对应 TS writeRestartSentinel）
	var sentinelPath string
	var sentinelPayload *RestartSentinelPayload
	if sw := ctx.Context.RestartSentinel; sw != nil {
		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		sentinelPayload = &RestartSentinelPayload{
			Kind:       "config-apply",
			Status:     "ok",
			Ts:         time.Now().UnixMilli(),
			SessionKey: sessionKey,
			Message:    notePtr,
			DoctorHint: sw.FormatDoctorNonInteractiveHint(),
			Stats: map[string]interface{}{
				"mode": "config.patch",
				"root": loader.ConfigPath(),
			},
		}
		sentinelPath, _ = sw.WriteRestartSentinel(sentinelPayload)
	}

	// 调度网关重启
	var restartResult *GatewayRestartResult
	if gr := ctx.Context.GatewayRestarter; gr != nil {
		restartResult = gr.ScheduleRestart(GatewayRestartPlan{
			DelayMs:  restartDelayMs,
			Reason:   "config.patch",
			Rollback: rollback,
		})
	}

	// 构建响应（对应 TS L334-L347）
	ctx.Respond(true, buildConfigWriteSuccessResult(
		loader,
		"config.patch",
		resolved,
		monitorReloaded,
		restartResult,
		sentinelPath,
		sentinelPayload,
	), nil)
}
