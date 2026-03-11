package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/routing"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/google/uuid"
)

// ---------- sessions handler (移植自 server-methods/sessions.ts) ----------

// getSessionDefaults P3-D2: 从配置解析 sessions.list 默认值。
// 对齐 TS: session-utils.ts getSessionDefaults()
func getSessionDefaults(cfg *types.OpenAcosmiConfig) GatewaySessionsDefaults {
	ref := models.ResolveConfiguredModelRef(cfg, models.DefaultProvider, models.DefaultModel)
	provider := ref.Provider
	model := ref.Model

	// contextTokens: 优先 agents.defaults.contextTokens
	var ct *int
	if cfg != nil && cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.ContextTokens != nil {
		ct = cfg.Agents.Defaults.ContextTokens
	}

	return GatewaySessionsDefaults{
		ModelProvider: &provider,
		Model:         &model,
		ContextTokens: ct,
	}
}

// ---------- sessions.list ----------
func SessionsHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"sessions.list":       handleSessionsList,
		"sessions.preview":    handleSessionsPreview,
		"sessions.resolve":    handleSessionsResolve,
		"sessions.patch":      handleSessionsPatch,
		"sessions.reset":      handleSessionsReset,
		"sessions.delete":     handleSessionsDelete,
		"sessions.compact":    handleSessionsCompact,
		"sessions.create":     handleSessionsCreate,
		"sessions.ensureMain": handleSessionsEnsureMain,
	}
}

// ---------- sessions.list ----------

func handleSessionsList(ctx *MethodHandlerContext) {
	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	var params SessionsListParams
	if raw, err := json.Marshal(ctx.Params); err == nil {
		_ = json.Unmarshal(raw, &params)
	}

	entries := store.List()
	agentIdFilter := NormalizeAgentId(params.AgentId)

	var rows []GatewaySessionRow
	for _, entry := range entries {
		key := entry.SessionKey

		// [BUG-1] 过滤 cron run key（与 TS listSessionsFromStore L577 一致）
		if IsCronRunSessionKey(key) {
			continue
		}

		// P1 域隔离: 过滤 task: 前缀条目（task 已迁移到独立 TaskStore）
		if IsTaskSessionKey(key) {
			continue
		}

		kind := ClassifySessionKey(key, entry)

		if kind == "global" && !params.IncludeGlobal {
			continue
		}
		if kind == "unknown" && !params.IncludeUnknown {
			continue
		}

		// [BUG-3] agentId 过滤（与 TS listSessionsFromStore L586-595 一致）
		if agentIdFilter != "" {
			if key == "global" || key == "unknown" {
				continue
			}
			parsed := parseAgentSessionKeySimple(key)
			if parsed == nil {
				continue
			}
			if NormalizeAgentId(parsed.agentId) != agentIdFilter {
				continue
			}
		}

		// SpawnedBy 过滤
		if params.SpawnedBy != "" {
			if key == "unknown" || key == "global" {
				continue
			}
			if entry.SpawnedBy != params.SpawnedBy {
				continue
			}
		}
		// Label 过滤
		if params.Label != "" && entry.Label != params.Label {
			continue
		}

		// totalTokens 计算（与 TS L617 一致）
		totalTokens := entry.TotalTokens
		if totalTokens == 0 {
			totalTokens = entry.InputTokens + entry.OutputTokens
		}

		// channel 回退（与 TS L619 一致）
		channel := entry.Channel
		if channel == "" {
			if parsed := ParseGroupKey(key); parsed != nil {
				channel = parsed.Channel
			}
		}

		// F1: 解析模型引用
		sessionAgentId := ""
		if parsed := parseAgentSessionKeySimple(key); parsed != nil {
			sessionAgentId = parsed.agentId
		}
		resolvedProvider, resolvedModel := ResolveSessionModelRef(ctx.Context.Config, entry, sessionAgentId)

		// F1: 统一投递字段
		deliveryFields := NormalizeSessionDeliveryFields(entry)

		// F1: 群组显示名 — 对齐 TS L626-639 完整回退链:
		// entry.displayName → buildGroupDisplayName (仅 channel 非空时) → entry.label → origin.label
		displayName := entry.DisplayName
		if displayName == "" && channel != "" {
			displayName = BuildGroupDisplayName(channel, entry.Subject, entry.GroupChannel, entry.Space, entry.GroupId, key)
		}
		if displayName == "" {
			displayName = entry.Label
		}
		if displayName == "" && entry.Origin != nil {
			displayName = entry.Origin.Label
		}

		row := GatewaySessionRow{
			Key:             key,
			Kind:            kind,
			Label:           entry.Label,
			DisplayName:     displayName,
			Channel:         channel,
			Subject:         entry.Subject,
			GroupChannel:    entry.GroupChannel,
			Space:           entry.Space,
			ChatType:        entry.ChatType,
			Origin:          entry.Origin,
			SessionId:       entry.SessionId,
			SystemSent:      entry.SystemSent,
			AbortedLastRun:  entry.AbortedLastRun,
			ThinkingLevel:   entry.ThinkingLevel,
			VerboseLevel:    entry.VerboseLevel,
			ReasoningLevel:  entry.ReasoningLevel,
			ElevatedLevel:   entry.ElevatedLevel,
			SendPolicy:      entry.SendPolicy,
			InputTokens:     entry.InputTokens,
			OutputTokens:    entry.OutputTokens,
			TotalTokens:     totalTokens,
			ResponseUsage:   entry.ResponseUsage,
			ModelProvider:   resolvedProvider,
			Model:           resolvedModel,
			ContextTokens:   entry.ContextTokens,
			DeliveryContext: deliveryFields.DeliveryContext,
			LastChannel:     entry.LastChannel,
			LastTo:          deliveryFields.LastTo,
			LastAccountId:   deliveryFields.LastAccountId,
		}

		if entry.UpdatedAt > 0 {
			ua := entry.UpdatedAt
			row.UpdatedAt = &ua
		}

		rows = append(rows, row)
	}

	// [BUG-2] 按 updatedAt 降序排序（与 TS listSessionsFromStore L680 一致）
	sort.Slice(rows, func(i, j int) bool {
		ai := int64(0)
		aj := int64(0)
		if rows[i].UpdatedAt != nil {
			ai = *rows[i].UpdatedAt
		}
		if rows[j].UpdatedAt != nil {
			aj = *rows[j].UpdatedAt
		}
		return ai > aj
	})

	// Search 过滤（排序后，与 TS L682-687 一致）
	if params.Search != "" {
		needle := strings.ToLower(params.Search)
		filtered := rows[:0]
		for _, s := range rows {
			haystack := strings.ToLower(
				s.DisplayName + " " + s.Label + " " + s.Subject + " " + s.SessionId + " " + s.Key,
			)
			if strings.Contains(haystack, needle) {
				filtered = append(filtered, s)
			}
		}
		rows = filtered
	}

	// activeMinutes 过滤（排序后，与 TS L689-692 一致）
	if params.ActiveMinutes != nil && *params.ActiveMinutes > 0 {
		cutoff := time.Now().UnixMilli() - int64(*params.ActiveMinutes)*60*1000
		filtered := rows[:0]
		for _, s := range rows {
			ua := int64(0)
			if s.UpdatedAt != nil {
				ua = *s.UpdatedAt
			}
			if ua >= cutoff {
				filtered = append(filtered, s)
			}
		}
		rows = filtered
	}

	// Limit
	if params.Limit != nil && *params.Limit > 0 && len(rows) > *params.Limit {
		rows = rows[:*params.Limit]
	}

	// [BUG-4] derivedTitle 和 lastMessagePreview（排序+截断后处理，与 TS L699-723 一致）
	for i := range rows {
		if rows[i].SessionId == "" {
			continue
		}
		if params.IncludeDerivedTitles {
			firstMsg := ReadFirstUserMessageFromTranscript(
				rows[i].SessionId, "", "", "",
			)
			rows[i].DerivedTitle = DeriveSessionTitle(
				store.LoadSessionEntry(rows[i].Key), firstMsg,
			)
		}
		if params.IncludeLastMessage {
			if lastMsg := ReadLastMessagePreviewFromTranscript(
				rows[i].SessionId, "", "", "",
			); lastMsg != "" {
				rows[i].LastMessagePreview = lastMsg
			}
		}
	}

	storePath := ""
	if ctx.Context != nil {
		storePath = ctx.Context.StorePath
	}

	// P3-D2: 从配置填充默认值
	defaults := GatewaySessionsDefaults{}
	if ctx.Context != nil && ctx.Context.Config != nil {
		defaults = getSessionDefaults(ctx.Context.Config)
	}

	result := SessionsListResult{
		Ts:       time.Now().UnixMilli(),
		Path:     storePath,
		Count:    len(rows),
		Sessions: rows,
		Defaults: defaults,
	}
	ctx.Respond(true, result, nil)
}

// ---------- sessions.preview ----------

func handleSessionsPreview(ctx *MethodHandlerContext) {
	keys, _ := ctx.Params["keys"].([]interface{})
	if len(keys) == 0 {
		ctx.Respond(true, SessionsPreviewResult{
			Ts:       time.Now().UnixMilli(),
			Previews: []SessionsPreviewEntry{},
		}, nil)
		return
	}

	// 默认值与 TS 对齐: limit=12, maxChars=240, keys 最多 64 个
	maxItems := 12
	maxChars := 240
	if v, ok := ctx.Params["limit"].(float64); ok && v >= 1 {
		maxItems = int(v)
	}
	if v, ok := ctx.Params["maxChars"].(float64); ok && v >= 20 {
		maxChars = int(v)
	}
	if len(keys) > 64 {
		keys = keys[:64]
	}

	var previews []SessionsPreviewEntry
	for _, rawKey := range keys {
		key, _ := rawKey.(string)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		// 查找 sessionId 和 storePath
		var sessionId, storePath, sessionFile, agentId string
		if ctx.Context.SessionStore != nil {
			entry := ctx.Context.SessionStore.LoadSessionEntry(key)
			if entry != nil {
				sessionId = entry.SessionId
				sessionFile = entry.SessionFile
			}
		}
		if sessionId == "" {
			previews = append(previews, SessionsPreviewEntry{
				Key: key, Status: "missing", Items: []SessionPreviewItem{},
			})
			continue
		}

		items := ReadSessionPreviewItemsFromTranscript(
			sessionId, storePath, sessionFile, agentId,
			maxItems, maxChars,
		)

		status := "ok"
		if len(items) == 0 {
			status = "empty"
		}
		previews = append(previews, SessionsPreviewEntry{
			Key:    key,
			Status: status,
			Items:  items,
		})
	}

	ctx.Respond(true, SessionsPreviewResult{
		Ts:       time.Now().UnixMilli(),
		Previews: previews,
	}, nil)
}

// ---------- sessions.resolve ----------

func handleSessionsResolve(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if key == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key required"))
		return
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	mainKey := store.ResolveMainSessionKey(key)
	ctx.Respond(true, map[string]interface{}{
		"ok":  true,
		"key": mainKey,
	}, nil)
}

// ---------- sessions.patch ----------

func handleSessionsPatch(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if key == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key required"))
		return
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	entry := store.LoadSessionEntry(key)
	if entry == nil {
		entry = &SessionEntry{SessionKey: key}
	}

	// 合并 patch 字段
	if dn, ok := ctx.Params["displayName"].(string); ok {
		entry.DisplayName = dn
	}
	if label, ok := ctx.Params["label"].(string); ok {
		entry.Label = label
	}
	if ct, ok := ctx.Params["contextTokens"].(float64); ok {
		v := int(ct)
		entry.ContextTokens = &v
	}
	if mo, ok := ctx.Params["modelOverride"].(string); ok {
		entry.ModelOverride = mo
	}
	if po, ok := ctx.Params["providerOverride"].(string); ok {
		entry.ProviderOverride = po
	}
	if sp, ok := ctx.Params["sendPolicy"].(string); ok {
		entry.SendPolicy = sp
	}
	if tl, ok := ctx.Params["thinkingLevel"].(string); ok {
		entry.ThinkingLevel = tl
	}
	if ru, ok := ctx.Params["responseUsage"].(string); ok {
		entry.ResponseUsage = ru
	}
	if vl, ok := ctx.Params["verboseLevel"].(string); ok {
		entry.VerboseLevel = vl
	}
	if rl, ok := ctx.Params["reasoningLevel"].(string); ok {
		entry.ReasoningLevel = rl
	}
	if el, ok := ctx.Params["elevatedLevel"].(string); ok {
		entry.ElevatedLevel = el
	}
	if ta, ok := ctx.Params["ttsAuto"].(string); ok {
		entry.TtsAuto = ta
	}
	if ga, ok := ctx.Params["groupActivation"].(string); ok {
		entry.GroupActivation = ga
	}
	if subj, ok := ctx.Params["subject"].(string); ok {
		entry.Subject = subj
	}
	if qm, ok := ctx.Params["queueMode"].(string); ok {
		entry.QueueMode = qm
	}

	entry.UpdatedAt = time.Now().UnixMilli()
	store.Save(entry)

	ctx.Respond(true, SessionsPatchResult{
		OK:    true,
		Key:   key,
		Entry: entry,
	}, nil)
}

// ---------- sessions.reset ----------
// [BUG-5] 完整移植 TS sessions.reset 逻辑：
// 生成新 UUID、保留白名单字段、归档旧 transcript。

func handleSessionsReset(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if key == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key required"))
		return
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	entry := store.LoadSessionEntry(key)

	// 归档旧 transcript（best-effort）
	if entry != nil && entry.SessionFile != "" {
		_, _ = ArchiveFileOnDisk(entry.SessionFile, "reset")
	}

	// 构建新 entry，仅保留白名单字段（与 TS L246-267 一致）
	now := time.Now().UnixMilli()
	nextEntry := &SessionEntry{
		SessionKey:     key,
		SessionId:      uuid.New().String(),
		UpdatedAt:      now,
		SystemSent:     false,
		AbortedLastRun: false,
		InputTokens:    0,
		OutputTokens:   0,
		TotalTokens:    0,
	}
	// 保留旧 entry 的配置字段
	if entry != nil {
		nextEntry.ThinkingLevel = entry.ThinkingLevel
		nextEntry.VerboseLevel = entry.VerboseLevel
		nextEntry.ReasoningLevel = entry.ReasoningLevel
		nextEntry.ResponseUsage = entry.ResponseUsage
		nextEntry.ModelOverride = entry.ModelOverride
		nextEntry.ContextTokens = entry.ContextTokens
		nextEntry.SendPolicy = entry.SendPolicy
		nextEntry.Label = entry.Label
		nextEntry.Origin = entry.Origin
		nextEntry.LastChannel = entry.LastChannel
		nextEntry.LastTo = entry.LastTo
		nextEntry.SkillsSnapshot = entry.SkillsSnapshot
	}

	store.Save(nextEntry)

	ctx.Respond(true, map[string]interface{}{
		"ok":    true,
		"key":   key,
		"entry": nextEntry,
	}, nil)
}

// ---------- sessions.delete ----------
// [DIFF-1] 增加主 session 保护 + transcript 归档

func handleSessionsDelete(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if key == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key required"))
		return
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	// P3-D3: 主 session 保护（与 TS L293-302 一致）
	// 保留 key 保护
	if key == "global" || key == "unknown" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "cannot delete reserved session: "+key))
		return
	}
	// 动态主 session key 保护
	if ctx.Context != nil && ctx.Context.Config != nil {
		cfg := ctx.Context.Config
		agentID := routing.DefaultAgentID
		mainKey := routing.DefaultMainKey
		if cfg.Session != nil && cfg.Session.MainKey != "" {
			mainKey = cfg.Session.MainKey
		}
		resolvedMainKey := routing.BuildAgentMainSessionKey(agentID, mainKey)
		if key == resolvedMainKey {
			ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest,
				fmt.Sprintf("Cannot delete the main session (%s).", resolvedMainKey)))
			return
		}
	}

	entry := store.LoadSessionEntry(key)
	existed := entry != nil

	// 删除 store entry
	store.Delete(key)

	// 归档 transcript 文件（与 TS L344-361 一致）
	deleteTranscript := true
	if v, ok := ctx.Params["deleteTranscript"].(bool); ok {
		deleteTranscript = v
	}

	var archived []string
	if deleteTranscript && entry != nil && entry.SessionId != "" {
		candidates := ResolveSessionTranscriptCandidates(
			entry.SessionId, "", entry.SessionFile, "",
		)
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err != nil {
				continue
			}
			if path, err := ArchiveFileOnDisk(candidate, "deleted"); err == nil {
				archived = append(archived, path)
			}
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":       true,
		"key":      key,
		"deleted":  existed,
		"archived": archived,
	}, nil)
}

// ---------- sessions.compact ----------
// [MISSING-1] 新增 sessions.compact handler（移植自 TS sessions.ts L365-481）

func handleSessionsCompact(ctx *MethodHandlerContext) {
	key, _ := ctx.Params["key"].(string)
	if key == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "key required"))
		return
	}

	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	// 默认保留最近 400 行
	maxLines := 400
	if v, ok := ctx.Params["maxLines"].(float64); ok && v >= 1 {
		maxLines = int(v)
	}

	entry := store.LoadSessionEntry(key)
	sessionId := ""
	if entry != nil {
		sessionId = entry.SessionId
	}
	if sessionId == "" {
		ctx.Respond(true, map[string]interface{}{
			"ok":        true,
			"key":       key,
			"compacted": false,
			"reason":    "no sessionId",
		}, nil)
		return
	}

	// 找到 transcript 文件
	candidates := ResolveSessionTranscriptCandidates(
		sessionId, "", entry.SessionFile, "",
	)
	filePath := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			filePath = c
			break
		}
	}
	if filePath == "" {
		ctx.Respond(true, map[string]interface{}{
			"ok":        true,
			"key":       key,
			"compacted": false,
			"reason":    "no transcript",
		}, nil)
		return
	}

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "read transcript: "+err.Error()))
		return
	}

	rawLines := strings.Split(string(data), "\n")
	var lines []string
	for _, l := range rawLines {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}

	if len(lines) <= maxLines {
		ctx.Respond(true, map[string]interface{}{
			"ok":        true,
			"key":       key,
			"compacted": false,
			"kept":      len(lines),
		}, nil)
		return
	}

	// 归档原文件
	archivedPath, err := ArchiveFileOnDisk(filePath, "bak")
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "archive: "+err.Error()))
		return
	}

	// 写入截断后的文件
	keptLines := lines[len(lines)-maxLines:]
	if err := os.WriteFile(filePath, []byte(strings.Join(keptLines, "\n")+"\n"), 0644); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "write: "+err.Error()))
		return
	}

	// 重置 token 计数
	if entry != nil {
		entry.InputTokens = 0
		entry.OutputTokens = 0
		entry.TotalTokens = 0
		entry.UpdatedAt = time.Now().UnixMilli()
		store.Save(entry)
	}

	ctx.Respond(true, map[string]interface{}{
		"ok":        true,
		"key":       key,
		"compacted": true,
		"archived":  archivedPath,
		"kept":      len(keptLines),
	}, nil)
}

// ---------- sessions.create ----------
// P2 身份收敛: 网关接管 session 创建，前端不再本地生成 sessionKey。
// 请求: { channel?: string, label?: string }
// 响应: { sessionKey: string, sessionId: string }

func handleSessionsCreate(ctx *MethodHandlerContext) {
	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	channel, _ := ctx.Params["channel"].(string)
	label, _ := ctx.Params["label"].(string)

	now := time.Now().UnixMilli()
	sessionId := uuid.New().String()
	sessionKey := fmt.Sprintf("user:%s", sessionId)

	entry := &SessionEntry{
		SessionKey: sessionKey,
		SessionId:  sessionId,
		Label:      label,
		Channel:    channel,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	store.Save(entry)

	ctx.Respond(true, map[string]interface{}{
		"sessionKey": sessionKey,
		"sessionId":  sessionId,
	}, nil)
}

// ---------- sessions.ensureMain ----------
// P2 身份收敛: 确保主 session 存在（幂等）。
// 请求: { agentId?: string }
// 响应: { sessionKey: string, sessionId: string, created: bool }

func handleSessionsEnsureMain(ctx *MethodHandlerContext) {
	store := ctx.Context.SessionStore
	if store == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "session store not available"))
		return
	}

	// 解析 agent ID 和 main key（从配置或默认值）
	agentID := routing.DefaultAgentID
	if id, ok := ctx.Params["agentId"].(string); ok && id != "" {
		agentID = id
	}
	mainKey := routing.DefaultMainKey
	if ctx.Context != nil && ctx.Context.Config != nil && ctx.Context.Config.Session != nil {
		if mk := ctx.Context.Config.Session.MainKey; mk != "" {
			mainKey = mk
		}
	}

	sessionKey := routing.BuildAgentMainSessionKey(agentID, mainKey)
	entry := store.LoadSessionEntry(sessionKey)
	if entry != nil {
		// 已存在，直接返回
		ctx.Respond(true, map[string]interface{}{
			"sessionKey": sessionKey,
			"sessionId":  entry.SessionId,
			"created":    false,
		}, nil)
		return
	}

	// 不存在，创建
	now := time.Now().UnixMilli()
	sessionId := uuid.New().String()
	entry = &SessionEntry{
		SessionKey: sessionKey,
		SessionId:  sessionId,
		Label:      "main",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	store.Save(entry)

	ctx.Respond(true, map[string]interface{}{
		"sessionKey": sessionKey,
		"sessionId":  sessionId,
		"created":    true,
	}, nil)
}
