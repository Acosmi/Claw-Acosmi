// bridge/bridge.go — go-providers Apply 函数到 typed OpenAcosmiConfig 的桥接层
//
// go-providers 的 Apply 函数在 map[string]interface{} 上操作。
// 本桥接层在空 map 上调用 Apply 函数，提取结果，写入 typed *types.OpenAcosmiConfig。
package bridge

import (
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/goproviders/onboard"
	gptypes "github.com/Acosmi/ClawAcosmi/internal/goproviders/types"
	pkgtypes "github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ApplyOpts 可选配置参数。
type ApplyOpts struct {
	// SetDefaultModel 是否同时设置 agents.defaults.model.primary
	SetDefaultModel bool
	// APIKey 要设置的 API Key（Apply 后写入）
	APIKey string
	// BaseURL 自定义 BaseURL（覆盖 provider 默认值）
	BaseURL string
}

// providerApplyFunc 类型：接收一个 map 配置，返回修改后的 map。
type providerApplyFunc func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig

// providerRegistry provider ID → Apply 函数注册表。
// 每个函数只注入 provider 配置（不设默认模型）。
var providerRegistry = map[string]providerApplyFunc{
	// 有完整 Apply*ProviderConfig 实现的 provider
	"zai": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyZaiProviderConfig(cfg, nil)
	},
	"moonshot": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyMoonshotProviderConfig(cfg)
	},
	"minimax": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyMinimaxHostedProviderConfig(cfg, "")
	},
	"xai": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyXaiProviderConfig(cfg)
	},
	"mistral": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyMistralProviderConfig(cfg)
	},
	"kilocode": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyKilocodeProviderConfig(cfg)
	},
	"openrouter": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyOpenrouterProviderConfig(cfg)
	},
	"together": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyTogetherProviderConfig(cfg)
	},
	"huggingface": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyHuggingfaceProviderConfig(cfg)
	},
	"venice": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyVeniceProviderConfig(cfg)
	},
	"synthetic": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplySyntheticProviderConfig(cfg)
	},
	"xiaomi": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyXiaomiProviderConfig(cfg)
	},
	"litellm": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyLitellmProviderConfig(cfg)
	},
	"qianfan": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyQianfanProviderConfig(cfg)
	},
	"kimi-coding": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyKimiCodeProviderConfig(cfg)
	},
}

// providerWithDefaultRegistry provider ID → Apply 函数注册表（设默认模型版本）。
var providerWithDefaultRegistry = map[string]providerApplyFunc{
	"zai": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyZaiConfig(cfg, nil)
	},
	"moonshot": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyMoonshotConfig(cfg)
	},
	"minimax": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyMinimaxHostedConfig(cfg, "")
	},
	"xai": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyXaiConfig(cfg)
	},
	"mistral": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyMistralConfig(cfg)
	},
	"kilocode": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyKilocodeConfig(cfg)
	},
	"openrouter": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyOpenrouterConfig(cfg)
	},
	"together": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyTogetherConfig(cfg)
	},
	"huggingface": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyHuggingfaceConfig(cfg)
	},
	"venice": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyVeniceConfig(cfg)
	},
	"synthetic": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplySyntheticConfig(cfg)
	},
	"xiaomi": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyXiaomiConfig(cfg)
	},
	"litellm": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyLitellmConfig(cfg)
	},
	"qianfan": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyQianfanConfig(cfg)
	},
	"kimi-coding": func(cfg onboard.OpenClawConfig) onboard.OpenClawConfig {
		return onboard.ApplyKimiCodeConfig(cfg)
	},
}

// defaultModelRefs provider ID → 默认模型 ref（provider/model 格式）。
// 用于没有在 providerRegistry 中注册的 provider（如 anthropic, openai, google 等）
// 这些 provider 的 Apply 实现在 authchoice 中，走 API key 路径不需要 interactive prompter。
var defaultModelRefs = map[string]struct {
	ModelRef      string
	API           string
	BaseURL       string
	DefaultModels []gptypes.ModelDefinitionConfig
}{
	"anthropic": {
		ModelRef: "anthropic/claude-sonnet-4-6",
		API:      "anthropic-messages",
		BaseURL:  "https://api.anthropic.com",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 16384},
			{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000},
			{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 8192},
		},
	},
	"openai": {
		ModelRef: "openai/gpt-4.1",
		API:      "openai-completions",
		BaseURL:  "https://api.openai.com/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "gpt-4.1", Name: "GPT-4.1", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1047576, MaxTokens: 32768},
			{ID: "o4-mini", Name: "o4-mini", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 100000},
			{ID: "gpt-4.1-mini", Name: "GPT-4.1 mini", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1047576, MaxTokens: 16384},
		},
	},
	"google": {
		ModelRef: "google/gemini-3.1-pro-preview",
		API:      "google-generative-ai",
		BaseURL:  "",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash-Lite", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-3-pro-preview", Name: "Gemini 3 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
		},
	},
	"deepseek": {
		ModelRef: "deepseek/deepseek-chat",
		API:      "openai-completions",
		BaseURL:  "https://api.deepseek.com/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "deepseek-chat", Name: "DeepSeek V3", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "deepseek-reasoner", Name: "DeepSeek R1", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"ollama": {
		ModelRef: "ollama/llama3.3",
		API:      "openai-completions",
		BaseURL:  "http://localhost:11434/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "llama3.3", Name: "Llama 3.3", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "qwen3:32b", Name: "Qwen 3 32B", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"volcengine": {
		ModelRef: "volcengine/ark-code-latest",
		API:      "openai-completions",
		BaseURL:  "https://ark.cn-beijing.volces.com/api/v3",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "ark-code-latest", Name: "ARK Code Latest", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "doubao-1-5-pro-32k", Name: "Doubao 1.5 Pro 32K", Reasoning: false, Input: []string{"text"}, ContextWindow: 32768, MaxTokens: 8192},
			{ID: "doubao-1-5-pro-256k", Name: "Doubao 1.5 Pro 256K", Reasoning: false, Input: []string{"text"}, ContextWindow: 262144, MaxTokens: 8192},
		},
	},
	"qwen": {
		ModelRef: "qwen/qwen3.5-plus",
		API:      "openai-completions",
		BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "qwen3.5-plus", Name: "Qwen3.5 Plus (旗舰多模态)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1000000, MaxTokens: 81920},
			{ID: "qwen3-max", Name: "Qwen3 Max (旗舰推理)", Reasoning: true, Input: []string{"text"}, ContextWindow: 256000, MaxTokens: 32000},
			{ID: "qwen3-coder-plus", Name: "Qwen3 Coder Plus (代码)", Reasoning: false, Input: []string{"text"}, ContextWindow: 262000, MaxTokens: 65536},
			{ID: "qwen3-235b-a22b", Name: "Qwen 3 235B (开源旗舰)", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 16384},
			{ID: "qwq-plus", Name: "QwQ Plus (推理专家)", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 16384},
			{ID: "qwen-max", Name: "Qwen Max (经典)", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 16384},
		},
	},
	"github-copilot": {
		ModelRef: "github-copilot/gpt-4o",
		API:      "github-copilot",
		BaseURL:  "https://api.githubcopilot.com",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "claude-sonnet-4.6", Name: "Claude Sonnet 4.6 (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "claude-sonnet-4.5", Name: "Claude Sonnet 4.5 (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "gpt-4o", Name: "GPT-4o (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "gpt-4.1", Name: "GPT-4.1 (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "gpt-4.1-mini", Name: "GPT-4.1 mini (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "gpt-4.1-nano", Name: "GPT-4.1 nano (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "o1", Name: "o1 (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "o1-mini", Name: "o1-mini (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
			{ID: "o3-mini", Name: "o3-mini (Copilot)", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 128000, MaxTokens: 8192},
		},
	},
	"openai-compat": {
		ModelRef: "openai-compat/custom",
		API:      "openai-completions",
		BaseURL:  "",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "custom", Name: "Custom Model", Reasoning: false, Input: []string{"text"}, ContextWindow: 128000, MaxTokens: 8192},
		},
	},
	"google-gemini-cli": {
		ModelRef: "google-gemini-cli/gemini-3.1-pro-preview",
		API:      "google-generative-ai",
		BaseURL:  "",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash-Lite", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-3-pro-preview", Name: "Gemini 3 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
		},
	},
	// --- 以下 8 个从 staticModelFallbacks 收归 ---
	"minimax": {
		ModelRef: "minimax/MiniMax-M2.5",
		API:      "openai-completions",
		BaseURL:  "https://api.minimax.io/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "MiniMax-M2.5", Name: "MiniMax-M2.5", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "MiniMax-M2.5-highspeed", Name: "MiniMax-M2.5 Highspeed", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "MiniMax-M2.5-Lightning", Name: "MiniMax-M2.5 Lightning", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"openrouter": {
		ModelRef: "openrouter/auto",
		API:      "openai-completions",
		BaseURL:  "https://openrouter.ai/api/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "auto", Name: "Auto", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 16384},
			{ID: "openai/gpt-4.1", Name: "GPT-4.1", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1047576, MaxTokens: 32768},
		},
	},
	"together": {
		ModelRef: "together/moonshotai/Kimi-K2.5",
		API:      "openai-completions",
		BaseURL:  "https://api.together.xyz/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "moonshotai/Kimi-K2.5", Name: "Kimi K2.5", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "deepseek-ai/DeepSeek-R1", Name: "DeepSeek R1", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "meta-llama/Llama-3.3-70B-Instruct-Turbo", Name: "Llama 3.3 70B", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"huggingface": {
		ModelRef: "huggingface/deepseek-ai/DeepSeek-R1",
		API:      "openai-completions",
		BaseURL:  "https://router.huggingface.co/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "deepseek-ai/DeepSeek-R1", Name: "DeepSeek R1", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "Qwen/Qwen3-235B-A22B", Name: "Qwen 3 235B", Reasoning: true, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
			{ID: "meta-llama/Llama-3.3-70B-Instruct", Name: "Llama 3.3 70B", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"venice": {
		ModelRef: "venice/llama-3.3-70b",
		API:      "openai-completions",
		BaseURL:  "https://api.venice.ai/api/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "llama-3.3-70b", Name: "Llama 3.3 70B", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"synthetic": {
		ModelRef: "synthetic/MiniMax-M2.5",
		API:      "anthropic-messages",
		BaseURL:  "https://api.synthetic.dev/v1",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "MiniMax-M2.5", Name: "MiniMax-M2.5", Reasoning: false, Input: []string{"text"}, ContextWindow: 131072, MaxTokens: 8192},
		},
	},
	"litellm": {
		ModelRef: "litellm/claude-opus-4-6",
		API:      "openai-completions",
		BaseURL:  "http://localhost:4000",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 32000},
			{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 200000, MaxTokens: 16384},
			{ID: "gpt-4.1", Name: "GPT-4.1", Reasoning: false, Input: []string{"text", "image"}, ContextWindow: 1047576, MaxTokens: 32768},
			{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Reasoning: true, Input: []string{"text", "image"}, ContextWindow: 1048576, MaxTokens: 65536},
		},
	},
	"byteplus": {
		ModelRef: "byteplus/doubao-1-5-pro-32k",
		API:      "openai-completions",
		BaseURL:  "https://ark.ap-southeast.bytepluses.com/api/v3",
		DefaultModels: []gptypes.ModelDefinitionConfig{
			{ID: "doubao-1-5-pro-32k", Name: "Doubao 1.5 Pro 32K", Reasoning: false, Input: []string{"text"}, ContextWindow: 32768, MaxTokens: 8192},
			{ID: "doubao-pro-32k", Name: "Doubao Pro 32K", Reasoning: false, Input: []string{"text"}, ContextWindow: 32768, MaxTokens: 8192},
		},
	},
}

// NormalizeProviderID 规范化前端 provider ID 到运行时 provider ID。
// 这里保留 qwen/minimax 等运行时 provider，不提前切换到 portal auth provider。
func NormalizeProviderID(frontendID string) string {
	id := strings.TrimSpace(strings.ToLower(frontendID))
	switch id {
	case "z.ai", "z-ai", "zhipu":
		return "zai"
	case "bytedance", "doubao":
		return "volcengine"
	case "kimi-code":
		return "kimi-coding"
	case "gemini-cli":
		return "google-gemini-cli"
	default:
		return id
	}
}

// ResolveAuthProviderID 解析认证用 provider ID。
// 某些 provider 在 OAuth 模式下使用不同 ID。
func ResolveAuthProviderID(frontendID, authMode string) string {
	normalized := NormalizeProviderID(frontendID)
	if authMode == "oauth" || authMode == "deviceCode" {
		switch normalized {
		case "minimax":
			return "minimax-portal"
		case "qwen":
			return "qwen-portal"
		}
	}
	return normalized
}

// ApplyProviderByID 对指定 provider 调用 go-providers 的 Apply 函数，
// 然后合并结果到 typed config。这是 wizard-v2 调用的主入口。
func ApplyProviderByID(providerID string, cfg *pkgtypes.OpenAcosmiConfig, opts *ApplyOpts) {
	if opts == nil {
		opts = &ApplyOpts{}
	}
	providerID = NormalizeProviderID(providerID)

	// 优先尝试 providerRegistry（有完整 onboard Apply 实现）
	registry := providerRegistry
	if opts.SetDefaultModel {
		registry = providerWithDefaultRegistry
	}

	if applyFn, ok := registry[providerID]; ok {
		applyViaOnboard(providerID, applyFn, cfg, opts)
		return
	}

	// 回退到 defaultModelRefs（anthropic, openai, google 等）
	if info, ok := defaultModelRefs[providerID]; ok {
		applyFromStaticInfo(providerID, info.API, info.BaseURL, info.DefaultModels, info.ModelRef, cfg, opts)
		return
	}

	// 未知 provider — 创建最小配置
	ensureModelsProviders(cfg)
	if _, exists := cfg.Models.Providers[providerID]; !exists {
		cfg.Models.Providers[providerID] = &pkgtypes.ModelProviderConfig{}
	}
	if opts.APIKey != "" {
		cfg.Models.Providers[providerID].APIKey = opts.APIKey
	}
	if opts.BaseURL != "" {
		cfg.Models.Providers[providerID].BaseURL = opts.BaseURL
	}
}

// GetDefaultModelRef 获取 provider 的默认模型引用（provider/model 格式）。
func GetDefaultModelRef(providerID string) string {
	providerID = NormalizeProviderID(providerID)

	// 检查 defaultModelRefs
	if info, ok := defaultModelRefs[providerID]; ok {
		return info.ModelRef
	}

	// 从 onboard 常量获取（仅 providerRegistry-only 的 provider）
	switch providerID {
	case "zai":
		return onboard.ZaiDefaultModelRef
	case "moonshot":
		return onboard.MoonshotDefaultModelRef
	case "xai":
		return onboard.XaiDefaultModelRef
	case "mistral":
		return onboard.MistralDefaultModelRef
	case "kilocode":
		return "kilocode/" + onboard.KilocodeDefaultModelID
	case "xiaomi":
		return onboard.XiaomiDefaultModelRef
	case "qianfan":
		return onboard.QianfanDefaultModelRef
	case "kimi-coding":
		return onboard.KimiCodingModelRef
	}

	return providerID + "/"
}

// HasProvider 检查 provider 是否在注册表中。
func HasProvider(providerID string) bool {
	providerID = NormalizeProviderID(providerID)
	if _, ok := providerRegistry[providerID]; ok {
		return true
	}
	if _, ok := defaultModelRefs[providerID]; ok {
		return true
	}
	return false
}

// ---------- 内部实现 ----------

// applyViaOnboard 通过 onboard Apply 函数注入 provider 配置。
func applyViaOnboard(providerID string, applyFn providerApplyFunc, cfg *pkgtypes.OpenAcosmiConfig, opts *ApplyOpts) {
	// 1. 在空 map 上调用 Apply 函数
	emptyMap := make(onboard.OpenClawConfig)
	resultMap := applyFn(emptyMap)

	// 2. 从 map 提取 provider 配置
	mergeProviderFromMap(resultMap, providerID, cfg)

	// 3. 提取 agents.defaults.models (aliases)
	mergeModelAliasesFromMap(resultMap, cfg)

	// 4. 提取 agents.defaults.model.primary（如果 SetDefaultModel）
	if opts.SetDefaultModel {
		if primary := extractPrimaryModel(resultMap); primary != "" {
			ensureAgentsDefaults(cfg)
			if cfg.Agents.Defaults.Model == nil {
				cfg.Agents.Defaults.Model = &pkgtypes.AgentModelListConfig{}
			}
			cfg.Agents.Defaults.Model.Primary = primary
		}
	}

	// 5. 设置 API key 和自定义 BaseURL（Apply 函数不处理 API key）
	if provCfg, ok := cfg.Models.Providers[providerID]; ok {
		// MiniMax hosted scaffold uses "minimax" as a placeholder API key.
		// In OAuth/device-code mode we must not persist that placeholder as a real secret.
		if opts.APIKey == "" && providerID == "minimax" && provCfg.APIKey == "minimax" {
			provCfg.APIKey = ""
		}
		if opts.APIKey != "" {
			provCfg.APIKey = opts.APIKey
		}
		if opts.BaseURL != "" {
			provCfg.BaseURL = opts.BaseURL
		}
	}
}

// applyFromStaticInfo 通过静态信息注入 provider 配置。
func applyFromStaticInfo(
	providerID, api, baseURL string,
	defaultModels []gptypes.ModelDefinitionConfig,
	modelRef string,
	cfg *pkgtypes.OpenAcosmiConfig,
	opts *ApplyOpts,
) {
	ensureModelsProviders(cfg)
	provCfg := cfg.Models.Providers[providerID]
	if provCfg == nil {
		provCfg = &pkgtypes.ModelProviderConfig{}
		cfg.Models.Providers[providerID] = provCfg
	}

	// API 类型
	if provCfg.API == "" {
		provCfg.API = pkgtypes.ModelApi(api)
	}

	// BaseURL
	if opts.BaseURL != "" {
		provCfg.BaseURL = opts.BaseURL
	} else if provCfg.BaseURL == "" && baseURL != "" {
		provCfg.BaseURL = baseURL
	}

	// API Key
	if opts.APIKey != "" {
		provCfg.APIKey = opts.APIKey
	}

	// 合并模型列表
	mergeModelDefinitions(provCfg, defaultModels)

	// 设置默认模型
	if opts.SetDefaultModel && modelRef != "" {
		ensureAgentsDefaults(cfg)
		if cfg.Agents.Defaults.Model == nil {
			cfg.Agents.Defaults.Model = &pkgtypes.AgentModelListConfig{}
		}
		cfg.Agents.Defaults.Model.Primary = modelRef
	}
}

// mergeProviderFromMap 从 go-providers Apply 函数的输出 map 中
// 提取 provider 配置，合并到 typed config。
func mergeProviderFromMap(resultMap onboard.OpenClawConfig, providerID string, cfg *pkgtypes.OpenAcosmiConfig) {
	ensureModelsProviders(cfg)

	// 提取 models.providers[providerID]
	modelsMap, _ := resultMap["models"].(map[string]interface{})
	if modelsMap == nil {
		return
	}
	providersMap, _ := modelsMap["providers"].(map[string]interface{})
	if providersMap == nil {
		return
	}
	provMap, _ := providersMap[providerID].(map[string]interface{})
	if provMap == nil {
		return
	}

	provCfg := cfg.Models.Providers[providerID]
	if provCfg == nil {
		provCfg = &pkgtypes.ModelProviderConfig{}
		cfg.Models.Providers[providerID] = provCfg
	}

	// BaseURL
	if baseURL, ok := provMap["baseUrl"].(string); ok && baseURL != "" {
		provCfg.BaseURL = baseURL
	}

	// API 类型
	if api, ok := provMap["api"].(string); ok && api != "" {
		provCfg.API = pkgtypes.ModelApi(api)
	}

	// API Key（如果 map 中有）
	if apiKey, ok := provMap["apiKey"].(string); ok && apiKey != "" {
		provCfg.APIKey = apiKey
	}

	// Models 列表
	if modelsRaw, ok := provMap["models"]; ok {
		if gpModels, ok := modelsRaw.([]gptypes.ModelDefinitionConfig); ok {
			mergeModelDefinitions(provCfg, gpModels)
		}
	}
}

// mergeModelAliasesFromMap 从 map 中提取 agents.defaults.models (model aliases)。
func mergeModelAliasesFromMap(resultMap onboard.OpenClawConfig, cfg *pkgtypes.OpenAcosmiConfig) {
	agentsMap, _ := resultMap["agents"].(map[string]interface{})
	if agentsMap == nil {
		return
	}
	defaultsMap, _ := agentsMap["defaults"].(map[string]interface{})
	if defaultsMap == nil {
		return
	}
	modelsMap, _ := defaultsMap["models"].(map[string]interface{})
	if modelsMap == nil {
		return
	}

	ensureAgentsDefaults(cfg)
	if cfg.Agents.Defaults.Models == nil {
		cfg.Agents.Defaults.Models = make(map[string]*pkgtypes.AgentModelEntryConfig)
	}

	for modelRef, raw := range modelsMap {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		alias, _ := entry["alias"].(string)
		if alias == "" {
			continue
		}
		if existing := cfg.Agents.Defaults.Models[modelRef]; existing != nil {
			if existing.Alias == "" {
				existing.Alias = alias
			}
		} else {
			cfg.Agents.Defaults.Models[modelRef] = &pkgtypes.AgentModelEntryConfig{
				Alias: alias,
			}
		}
	}
}

// extractPrimaryModel 从 map 中提取 agents.defaults.model.primary。
func extractPrimaryModel(resultMap onboard.OpenClawConfig) string {
	agentsMap, _ := resultMap["agents"].(map[string]interface{})
	if agentsMap == nil {
		return ""
	}
	defaultsMap, _ := agentsMap["defaults"].(map[string]interface{})
	if defaultsMap == nil {
		return ""
	}
	modelMap, _ := defaultsMap["model"].(map[string]interface{})
	if modelMap == nil {
		return ""
	}
	primary, _ := modelMap["primary"].(string)
	return primary
}

// mergeModelDefinitions 将 go-providers 的模型列表合并到 typed provider config。
func mergeModelDefinitions(provCfg *pkgtypes.ModelProviderConfig, gpModels []gptypes.ModelDefinitionConfig) {
	existingIDs := make(map[string]bool, len(provCfg.Models))
	for _, m := range provCfg.Models {
		existingIDs[m.ID] = true
	}

	for _, gm := range gpModels {
		if existingIDs[gm.ID] {
			// 更新已有模型的缺失字段
			for i := range provCfg.Models {
				if provCfg.Models[i].ID == gm.ID {
					updateModelFromGP(&provCfg.Models[i], &gm)
					break
				}
			}
			continue
		}
		provCfg.Models = append(provCfg.Models, convertModelDef(gm))
		existingIDs[gm.ID] = true
	}
}

// convertModelDef 将 go-providers ModelDefinitionConfig 转换为 pkg/types ModelDefinitionConfig。
func convertModelDef(gm gptypes.ModelDefinitionConfig) pkgtypes.ModelDefinitionConfig {
	input := make([]pkgtypes.ModelInputType, 0, len(gm.Input))
	for _, s := range gm.Input {
		input = append(input, pkgtypes.ModelInputType(s))
	}

	m := pkgtypes.ModelDefinitionConfig{
		ID:            gm.ID,
		Name:          gm.Name,
		API:           pkgtypes.ModelApi(gm.Api),
		Reasoning:     gm.Reasoning,
		Input:         input,
		ContextWindow: gm.ContextWindow,
		MaxTokens:     gm.MaxTokens,
		Cost: pkgtypes.ModelCostConfig{
			Input:      gm.Cost.Input,
			Output:     gm.Cost.Output,
			CacheRead:  gm.Cost.CacheRead,
			CacheWrite: gm.Cost.CacheWrite,
		},
	}
	if gm.Compat != nil {
		m.Compat = &pkgtypes.ModelCompatConfig{
			SupportsStore:           gm.Compat.SupportsStore,
			SupportsDeveloperRole:   gm.Compat.SupportsDeveloperRole,
			SupportsReasoningEffort: gm.Compat.SupportsReasoningEffort,
			MaxTokensField:          gm.Compat.MaxTokensField,
		}
	}
	return m
}

// updateModelFromGP 用 go-providers 的模型信息补充已有模型的缺失字段。
func updateModelFromGP(existing *pkgtypes.ModelDefinitionConfig, gm *gptypes.ModelDefinitionConfig) {
	if existing.Name == "" || existing.Name == existing.ID {
		existing.Name = gm.Name
	}
	if existing.ContextWindow == 0 && gm.ContextWindow > 0 {
		existing.ContextWindow = gm.ContextWindow
	}
	if existing.MaxTokens == 0 && gm.MaxTokens > 0 {
		existing.MaxTokens = gm.MaxTokens
	}
	if existing.API == "" && gm.Api != "" {
		existing.API = pkgtypes.ModelApi(gm.Api)
	}
}

// ---------- 辅助 ----------

func ensureModelsProviders(cfg *pkgtypes.OpenAcosmiConfig) {
	if cfg.Models == nil {
		cfg.Models = &pkgtypes.ModelsConfig{}
	}
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]*pkgtypes.ModelProviderConfig)
	}
}

func ensureAgentsDefaults(cfg *pkgtypes.OpenAcosmiConfig) {
	if cfg.Agents == nil {
		cfg.Agents = &pkgtypes.AgentsConfig{}
	}
	if cfg.Agents.Defaults == nil {
		cfg.Agents.Defaults = &pkgtypes.AgentDefaultsConfig{}
	}
}
