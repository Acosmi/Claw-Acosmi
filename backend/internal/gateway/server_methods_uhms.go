package gateway

// server_methods_uhms.go — memory.uhms.* RPC 方法
//
// 静态方法:
//   memory.uhms.status    — UHMS 子系统状态
//   memory.uhms.search    — 手动搜索记忆
//   memory.uhms.add       — 手动添加记忆
//   memory.uhms.llm.get   — 获取 UHMS LLM 配置
//   memory.uhms.llm.set   — 设置 UHMS 独立 LLM 配置

import (
	"context"
	"log/slog"
	"strings"

	"github.com/anthropic/open-acosmi/internal/memory/uhms"
	"github.com/anthropic/open-acosmi/pkg/types"
)

// UHMSHandlers 返回 memory.uhms.* 静态方法映射。
func UHMSHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"memory.uhms.status":  handleUHMSStatus,
		"memory.uhms.search":  handleUHMSSearch,
		"memory.uhms.add":     handleUHMSAdd,
		"memory.uhms.llm.get": handleUHMSLLMGet,
		"memory.uhms.llm.set": handleUHMSLLMSet,
	}
}

// ---------- memory.uhms.status ----------

func handleUHMSStatus(ctx *MethodHandlerContext) {
	mgr := ctx.Context.UHMSManager
	if mgr == nil {
		ctx.Respond(true, map[string]interface{}{
			"enabled": false,
			"message": "UHMS memory system not enabled (set memory.uhms.enabled=true in config)",
		}, nil)
		return
	}

	status := mgr.Status()
	ctx.Respond(true, map[string]interface{}{
		"enabled":     status.Enabled,
		"vectorMode":  string(status.VectorMode),
		"vectorReady": status.VectorReady,
		"dbPath":      status.DBPath,
		"vfsPath":     status.VFSPath,
		"memoryCount": status.MemoryCount,
		"diskUsage":   status.DiskUsage,
	}, nil)
}

// ---------- memory.uhms.search ----------

func handleUHMSSearch(ctx *MethodHandlerContext) {
	mgr := ctx.Context.UHMSManager
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "UHMS not enabled"))
		return
	}

	query, _ := ctx.Params["query"].(string)
	if query == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "query is required"))
		return
	}

	userID, _ := ctx.Params["userId"].(string)
	if userID == "" {
		userID = "default"
	}

	topK := 20
	if raw, ok := ctx.Params["topK"].(float64); ok && raw > 0 {
		topK = int(raw)
	}

	results, err := mgr.SearchMemories(context.Background(), userID, query, uhms.SearchOptions{
		TopK:          topK,
		IncludeVector: true,
	})
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "search failed: "+err.Error()))
		return
	}

	// 构建响应
	items := make([]map[string]interface{}, len(results))
	for i, r := range results {
		items[i] = map[string]interface{}{
			"id":       r.Memory.ID,
			"content":  r.Memory.Content,
			"type":     string(r.Memory.MemoryType),
			"category": string(r.Memory.Category),
			"score":    r.Score,
			"source":   r.Source,
		}
	}

	ctx.Respond(true, map[string]interface{}{
		"results": items,
		"count":   len(items),
	}, nil)
}

// ---------- memory.uhms.add ----------

func handleUHMSAdd(ctx *MethodHandlerContext) {
	mgr := ctx.Context.UHMSManager
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "UHMS not enabled"))
		return
	}

	content, _ := ctx.Params["content"].(string)
	if content == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "content is required"))
		return
	}

	userID, _ := ctx.Params["userId"].(string)
	if userID == "" {
		userID = "default"
	}

	memType := uhms.MemoryType("")
	if raw, ok := ctx.Params["type"].(string); ok {
		memType = uhms.MemoryType(raw)
	}

	category := uhms.MemoryCategory("")
	if raw, ok := ctx.Params["category"].(string); ok {
		category = uhms.MemoryCategory(raw)
	}

	mem, err := mgr.AddMemory(context.Background(), userID, content, memType, category)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "add failed: "+err.Error()))
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"id":       mem.ID,
		"type":     string(mem.MemoryType),
		"category": string(mem.Category),
		"vfsPath":  mem.VFSPath,
	}, nil)
}

// ---------- memory.uhms.llm.get ----------

// UHMSLLMProvider describes an available LLM provider for the frontend selector.
type UHMSLLMProvider struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	HasAPIKey      bool   `json:"hasApiKey"`
	DefaultModel   string `json:"defaultModel"`
	DefaultBaseURL string `json:"defaultBaseUrl"`
}

func handleUHMSLLMGet(ctx *MethodHandlerContext) {
	cfgLoader := ctx.Context.ConfigLoader
	if cfgLoader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	loadedCfg, err := cfgLoader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "load config: "+err.Error()))
		return
	}
	if loadedCfg == nil {
		loadedCfg = &types.OpenAcosmiConfig{}
	}

	// 当前 LLM 信息 (直接从配置读取，无 fallback)
	currentProvider := ""
	currentModel := ""

	if loadedCfg.Memory != nil && loadedCfg.Memory.UHMS != nil && loadedCfg.Memory.UHMS.LLMProvider != "" {
		currentProvider = loadedCfg.Memory.UHMS.LLMProvider
		currentModel = loadedCfg.Memory.UHMS.LLMModel
		if currentModel == "" {
			currentModel = defaultModelForProvider(currentProvider)
		}
	}

	baseURL := ""
	if loadedCfg.Memory != nil && loadedCfg.Memory.UHMS != nil {
		baseURL = loadedCfg.Memory.UHMS.LLMBaseURL
	}

	// 收集可用 provider 列表
	providers := collectUHMSLLMProviders(loadedCfg)

	// 检查当前 provider 是否有 API key
	hasAPIKey := false
	for _, p := range providers {
		if strings.EqualFold(p.ID, currentProvider) {
			hasAPIKey = p.HasAPIKey
			break
		}
	}

	hasOwnApiKey := false
	if loadedCfg.Memory != nil && loadedCfg.Memory.UHMS != nil {
		hasOwnApiKey = loadedCfg.Memory.UHMS.LLMApiKey != ""
	}

	ctx.Respond(true, map[string]interface{}{
		"provider":     currentProvider,
		"model":        currentModel,
		"baseUrl":      baseURL,
		"hasApiKey":    hasAPIKey,
		"providers":    providers,
		"hasOwnApiKey": hasOwnApiKey,
	}, nil)
}

// collectUHMSLLMProviders builds the list of available LLM providers from config.
// defaultBaseURLForProvider returns the official API base URL for known LLM providers.
func defaultBaseURLForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai":
		return "https://api.openai.com/v1"
	case "deepseek":
		return "https://api.deepseek.com"
	case "google":
		return "https://generativelanguage.googleapis.com/v1beta"
	case "ollama":
		return "http://localhost:11434"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	case "together":
		return "https://api.together.xyz/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
}

func collectUHMSLLMProviders(cfg *types.OpenAcosmiConfig) []UHMSLLMProvider {
	known := []struct {
		id, label string
	}{
		{"anthropic", "Anthropic"},
		{"openai", "OpenAI"},
		{"deepseek", "DeepSeek"},
		{"google", "Google Gemini"},
		{"ollama", "Ollama"},
		{"groq", "Groq"},
		{"mistral", "Mistral"},
		{"together", "Together AI"},
		{"openrouter", "OpenRouter"},
	}

	// 从配置中收集有 API key 的 provider
	hasKey := make(map[string]bool)
	if cfg != nil && cfg.Models != nil && cfg.Models.Providers != nil {
		for name, pc := range cfg.Models.Providers {
			if pc != nil && pc.APIKey != "" {
				hasKey[strings.ToLower(name)] = true
			}
		}
	}

	var result []UHMSLLMProvider
	seen := make(map[string]bool)
	// 先添加已知 provider（保证顺序）
	for _, k := range known {
		result = append(result, UHMSLLMProvider{
			ID:             k.id,
			Label:          k.label,
			HasAPIKey:      hasKey[k.id],
			DefaultModel:   defaultModelForProvider(k.id),
			DefaultBaseURL: defaultBaseURLForProvider(k.id),
		})
		seen[k.id] = true
	}
	// 追加配置中存在但不在已知列表中的 provider
	if cfg != nil && cfg.Models != nil && cfg.Models.Providers != nil {
		for name := range cfg.Models.Providers {
			lower := strings.ToLower(name)
			if !seen[lower] {
				result = append(result, UHMSLLMProvider{
					ID:             lower,
					Label:          name,
					HasAPIKey:      hasKey[lower],
					DefaultModel:   defaultModelForProvider(lower),
					DefaultBaseURL: defaultBaseURLForProvider(lower),
				})
				seen[lower] = true
			}
		}
	}

	return result
}

// ---------- memory.uhms.llm.set ----------

func handleUHMSLLMSet(ctx *MethodHandlerContext) {
	cfgLoader := ctx.Context.ConfigLoader
	if cfgLoader == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "config loader not available"))
		return
	}

	mgr := ctx.Context.UHMSManager
	if mgr == nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeServiceUnavailable, "UHMS not enabled"))
		return
	}

	// 读取参数
	provider, _ := ctx.Params["provider"].(string)
	model, _ := ctx.Params["model"].(string)
	baseURL, _ := ctx.Params["baseUrl"].(string)

	// apiKey 仅在显式传入时更新（避免 provider/model 变更时误清 key）
	_, apiKeyProvided := ctx.Params["apiKey"]
	apiKey, _ := ctx.Params["apiKey"].(string)

	// 读取当前配置
	currentCfg, err := cfgLoader.LoadConfig()
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "load config: "+err.Error()))
		return
	}
	if currentCfg == nil {
		currentCfg = &types.OpenAcosmiConfig{}
	}
	if currentCfg.Memory == nil {
		currentCfg.Memory = &types.MemoryConfig{}
	}
	if currentCfg.Memory.UHMS == nil {
		currentCfg.Memory.UHMS = &types.MemoryUHMSConfig{Enabled: true}
	}

	// 更新 LLM 配置
	currentCfg.Memory.UHMS.LLMProvider = provider
	currentCfg.Memory.UHMS.LLMModel = model
	currentCfg.Memory.UHMS.LLMBaseURL = baseURL
	if apiKeyProvided {
		currentCfg.Memory.UHMS.LLMApiKey = apiKey
	}

	// 持久化
	if err := cfgLoader.WriteConfigFile(currentCfg); err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "save config: "+err.Error()))
		return
	}

	// 构建新 adapter 并热替换
	newAdapter := buildUHMSLLMAdapter(currentCfg.Memory.UHMS, currentCfg)
	if newAdapter != nil {
		mgr.SetLLMProvider(newAdapter)
		slog.Info("gateway: UHMS LLM provider hot-swapped",
			"provider", provider, "model", model)

		// 如果新 provider 是 anthropic，更新 CompactionClient
		if adapter, ok := newAdapter.(*uhms.LLMClientAdapter); ok && strings.ToLower(adapter.Provider) == "anthropic" {
			mgr.SetCompactionClient(&uhms.AnthropicCompactionClient{
				APIKey: adapter.APIKey,
			})
		} else {
			mgr.SetCompactionClient(nil)
		}
	} else if provider == "" {
		// 清空 provider: UHMS LLM 功能降级（不提取/不压缩）
		mgr.SetLLMProvider(nil)
		mgr.SetCompactionClient(nil)
		slog.Info("gateway: UHMS LLM provider cleared, memory extraction disabled")
	}

	// 读取最终状态返回
	finalProvider, finalModel := mgr.LLMInfo()
	ctx.Respond(true, map[string]interface{}{
		"saved":    true,
		"provider": finalProvider,
		"model":    finalModel,
	}, nil)
}
