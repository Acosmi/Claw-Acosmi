// onboard/credentials.go — 凭证管理（API Key 设置 + OAuth 凭证写入）
// 对应 TS 文件: src/commands/onboard-auth.credentials.ts
// 包含所有 set*ApiKey 函数和 writeOAuthCredentials 函数。
package onboard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/goproviders/authprofile"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/common"
	"github.com/Acosmi/ClawAcosmi/internal/goproviders/types"
	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
)

// ──────────────────────────────────────────────
// 默认模型引用常量
// ──────────────────────────────────────────────

// ZaiDefaultModelRef Z.AI 默认模型引用。
const ZaiDefaultModelRef = "zai/glm-5"

// XiaomiDefaultModelRef 小米默认模型引用。
const XiaomiDefaultModelRef = "xiaomi/mimo-v2-flash"

// OpenrouterDefaultModelRef OpenRouter 默认模型引用。
const OpenrouterDefaultModelRef = "openrouter/auto"

// HuggingfaceDefaultModelRef Hugging Face 默认模型引用。
const HuggingfaceDefaultModelRef = "huggingface/deepseek-ai/DeepSeek-R1"

// TogetherDefaultModelRef Together 默认模型引用。
const TogetherDefaultModelRef = "together/moonshotai/Kimi-K2.5"

// LitellmDefaultModelRef LiteLLM 默认模型引用。
const LitellmDefaultModelRef = "litellm/claude-opus-4-6"

// VercelAiGatewayDefaultModelRef Vercel AI Gateway 默认模型引用。
const VercelAiGatewayDefaultModelRef = "vercel-ai-gateway/anthropic/claude-opus-4.6"

// CloudflareAiGatewayDefaultModelRef Cloudflare AI Gateway 默认模型引用。
const CloudflareAiGatewayDefaultModelRef = "cloudflare-ai-gateway/anthropic/claude-sonnet-4-20250514"

// KilocodeDefaultModelRef Kilocode 默认模型引用。
var KilocodeDefaultModelRef = "kilocode/" + KilocodeDefaultModelID

// ──────────────────────────────────────────────
// 环境变量引用模式
// ──────────────────────────────────────────────

var envRefPattern = regexp.MustCompile(`^\$\{([A-Z][A-Z0-9_]*)\}$`)

// ApiKeyStorageOptions API 密钥存储选项。
type ApiKeyStorageOptions struct {
	SecretInputMode types.SecretInputMode
}

// ──────────────────────────────────────────────
// 密钥引用构建函数
// ──────────────────────────────────────────────

func buildEnvSecretRef(id string) types.SecretRef {
	return types.SecretRef{
		Source:   types.SecretRefSourceEnv,
		Provider: types.DefaultSecretProviderAlias,
		ID:       id,
	}
}

func parseEnvSecretRef(value string) *types.SecretRef {
	matches := envRefPattern.FindStringSubmatch(value)
	if matches == nil {
		return nil
	}
	ref := buildEnvSecretRef(matches[1])
	return &ref
}

// ProviderEnvVars 提供者对应的环境变量名称映射。
var ProviderEnvVars = map[string][]string{
	"anthropic":             {"ANTHROPIC_API_KEY"},
	"openai":                {"OPENAI_API_KEY"},
	"google":                {"GOOGLE_API_KEY", "GEMINI_API_KEY"},
	"minimax":               {"MINIMAX_API_KEY"},
	"moonshot":              {"MOONSHOT_API_KEY"},
	"kimi-coding":           {"KIMI_CODING_API_KEY"},
	"volcengine":            {"VOLCENGINE_API_KEY", "ARK_API_KEY"},
	"byteplus":              {"BYTEPLUS_API_KEY"},
	"synthetic":             {"SYNTHETIC_API_KEY"},
	"venice":                {"VENICE_API_KEY"},
	"zai":                   {"ZAI_API_KEY", "ZHIPU_API_KEY"},
	"xiaomi":                {"XIAOMI_API_KEY"},
	"openrouter":            {"OPENROUTER_API_KEY"},
	"kilocode":              {"KILOCODE_API_KEY"},
	"litellm":               {"LITELLM_API_KEY"},
	"vercel-ai-gateway":     {"VERCEL_AI_GATEWAY_API_KEY"},
	"cloudflare-ai-gateway": {"CLOUDFLARE_AI_GATEWAY_API_KEY"},
	"opencode":              {"OPENCODE_API_KEY"},
	"together":              {"TOGETHER_API_KEY"},
	"huggingface":           {"HUGGINGFACE_API_KEY", "HF_TOKEN"},
	"qianfan":               {"QIANFAN_API_KEY"},
	"xai":                   {"XAI_API_KEY"},
	"mistral":               {"MISTRAL_API_KEY"},
}

func resolveProviderDefaultEnvSecretRef(provider string) (types.SecretRef, error) {
	envVars, ok := ProviderEnvVars[provider]
	if !ok || len(envVars) == 0 {
		return types.SecretRef{}, fmt.Errorf("Provider \"%s\" 没有默认环境变量映射（secret-input-mode=ref）", provider)
	}
	for _, v := range envVars {
		if strings.TrimSpace(v) != "" {
			return buildEnvSecretRef(v), nil
		}
	}
	return types.SecretRef{}, fmt.Errorf("Provider \"%s\" 没有默认环境变量映射（secret-input-mode=ref）", provider)
}

// resolveApiKeySecretInput 解析 API Key 输入为最终存储形式。
func resolveApiKeySecretInput(provider string, input string, opts *ApiKeyStorageOptions) (string, *types.SecretRef) {
	// 尝试环境变量引用解析
	envRef := parseEnvSecretRef(input)
	if envRef != nil {
		return "", envRef
	}
	// ref 模式
	if opts != nil && opts.SecretInputMode == types.SecretInputModeRef {
		ref, err := resolveProviderDefaultEnvSecretRef(provider)
		if err == nil {
			return "", &ref
		}
	}
	// 明文
	return strings.TrimSpace(input), nil
}

// ApiKeyCredential API Key 凭证结构。
type ApiKeyCredential struct {
	Type     string            `json:"type"`
	Provider string            `json:"provider"`
	Key      string            `json:"key,omitempty"`
	KeyRef   *types.SecretRef  `json:"keyRef,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func buildApiKeyCredential(provider string, input string, metadata map[string]string, opts *ApiKeyStorageOptions) ApiKeyCredential {
	key, ref := resolveApiKeySecretInput(provider, input, opts)
	cred := ApiKeyCredential{
		Type:     "api_key",
		Provider: provider,
	}
	if ref != nil {
		cred.KeyRef = ref
	} else {
		cred.Key = key
	}
	if metadata != nil {
		cred.Metadata = metadata
	}
	return cred
}

// ──────────────────────────────────────────────
// UpsertAuthProfile 占位（调用 authprofile 包）
// 窗口 8 将替换为真实实现
// ──────────────────────────────────────────────

func upsertAuthProfileCredential(profileID string, credential interface{}, agentDir string) {
	store := authprofile.EnsureAuthProfileStore(agentDir, nil)
	if store == nil {
		return
	}
	credMap, ok := credential.(map[string]interface{})
	if !ok {
		// ApiKeyCredential 等结构体转 map
		if apiKeyCred, ok := credential.(ApiKeyCredential); ok {
			credMap = map[string]interface{}{
				"type":     apiKeyCred.Type,
				"provider": apiKeyCred.Provider,
			}
			if apiKeyCred.Key != "" {
				credMap["key"] = apiKeyCred.Key
			}
			if apiKeyCred.KeyRef != nil {
				credMap["keyRef"] = apiKeyCred.KeyRef
			}
			if apiKeyCred.Metadata != nil {
				credMap["metadata"] = apiKeyCred.Metadata
			}
		} else {
			return
		}
	}
	authprofile.UpsertAuthProfile(store, profileID, credMap)
	_ = authprofile.SaveAuthProfileStore(store, agentDir)
}

// resolveAuthAgentDir 解析认证代理目录。
func resolveAuthAgentDir(agentDir string) string {
	if agentDir != "" {
		return agentDir
	}
	return common.ResolveDefaultAgentDir()
}

// ──────────────────────────────────────────────
// WriteOAuthCredentials 写入 OAuth 凭证
// ──────────────────────────────────────────────

// WriteOAuthCredentialsOptions OAuth 凭证写入选项。
type WriteOAuthCredentialsOptions struct {
	SyncSiblingAgents bool
}

// safeRealpath 安全解析真实路径。
func safeRealpath(dir string) string {
	resolved, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return ""
	}
	return resolved
}

// resolveSiblingAgentDirs 解析同级代理目录。
func resolveSiblingAgentDirs(primaryAgentDir string) []string {
	normalized, _ := filepath.Abs(primaryAgentDir)
	parentOfAgent := filepath.Dir(normalized)
	candidateAgentsRoot := filepath.Dir(parentOfAgent)
	looksLikeStandardLayout := filepath.Base(normalized) == "agent" &&
		filepath.Base(candidateAgentsRoot) == "agents"

	var agentsRoot string
	if looksLikeStandardLayout {
		agentsRoot = candidateAgentsRoot
	} else {
		agentsRoot = statepaths.ResolveRuntimeAgentsRoot()
	}

	entries, err := os.ReadDir(agentsRoot)
	if err != nil {
		entries = nil
	}
	var discovered []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			discovered = append(discovered, filepath.Join(agentsRoot, entry.Name(), "agent"))
		}
	}

	seen := make(map[string]bool)
	var result []string
	allDirs := append([]string{normalized}, discovered...)
	for _, dir := range allDirs {
		real := safeRealpath(dir)
		if real != "" && !seen[real] {
			seen[real] = true
			result = append(result, real)
		}
	}
	return result
}

// WriteOAuthCredentials 写入 OAuth 凭证到认证存储。
// 对应 TS: writeOAuthCredentials()
func WriteOAuthCredentials(
	provider string,
	creds map[string]interface{},
	agentDir string,
	options *WriteOAuthCredentialsOptions,
) (string, error) {
	email := "default"
	if e, ok := creds["email"].(string); ok && strings.TrimSpace(e) != "" {
		email = strings.TrimSpace(e)
	}
	profileID := provider + ":" + email
	resolvedAgentDir, _ := filepath.Abs(resolveAuthAgentDir(agentDir))

	credential := make(map[string]interface{})
	credential["type"] = "oauth"
	credential["provider"] = provider
	for k, v := range creds {
		credential[k] = v
	}

	// 主写入
	upsertAuthProfileCredential(profileID, credential, resolvedAgentDir)

	// 同级同步（尽力而为）
	if options != nil && options.SyncSiblingAgents {
		targetDirs := resolveSiblingAgentDirs(resolvedAgentDir)
		primaryReal := safeRealpath(resolvedAgentDir)
		for _, targetDir := range targetDirs {
			targetReal := safeRealpath(targetDir)
			if targetReal != "" && primaryReal != "" && targetReal == primaryReal {
				continue
			}
			upsertAuthProfileCredential(profileID, credential, targetDir)
		}
	}
	return profileID, nil
}

// ──────────────────────────────────────────────
// set*ApiKey 函数
// ──────────────────────────────────────────────

// SetAnthropicApiKey 设置 Anthropic API 密钥。
func SetAnthropicApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("anthropic:default",
		buildApiKeyCredential("anthropic", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetOpenaiApiKey 设置 OpenAI API 密钥。
func SetOpenaiApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("openai:default",
		buildApiKeyCredential("openai", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetGeminiApiKey 设置 Gemini API 密钥。
func SetGeminiApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("google:default",
		buildApiKeyCredential("google", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetMinimaxApiKey 设置 MiniMax API 密钥。
func SetMinimaxApiKey(key string, agentDir string, profileID string, opts *ApiKeyStorageOptions) {
	if profileID == "" {
		profileID = "minimax:default"
	}
	provider := "minimax"
	if parts := strings.SplitN(profileID, ":", 2); len(parts) > 0 && parts[0] != "" {
		provider = parts[0]
	}
	upsertAuthProfileCredential(profileID,
		buildApiKeyCredential(provider, key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetMoonshotApiKey 设置 Moonshot API 密钥。
func SetMoonshotApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("moonshot:default",
		buildApiKeyCredential("moonshot", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetKimiCodingApiKey 设置 Kimi Coding API 密钥。
func SetKimiCodingApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("kimi-coding:default",
		buildApiKeyCredential("kimi-coding", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetVolcengineApiKey 设置火山引擎 API 密钥。
func SetVolcengineApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("volcengine:default",
		buildApiKeyCredential("volcengine", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetByteplusApiKey 设置 BytePlus API 密钥。
func SetByteplusApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("byteplus:default",
		buildApiKeyCredential("byteplus", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetSyntheticApiKey 设置 Synthetic API 密钥。
func SetSyntheticApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("synthetic:default",
		buildApiKeyCredential("synthetic", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetVeniceApiKey 设置 Venice API 密钥。
func SetVeniceApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("venice:default",
		buildApiKeyCredential("venice", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetZaiApiKey 设置 Z.AI API 密钥。
func SetZaiApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("zai:default",
		buildApiKeyCredential("zai", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetXiaomiApiKey 设置小米 API 密钥。
func SetXiaomiApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("xiaomi:default",
		buildApiKeyCredential("xiaomi", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetOpenrouterApiKey 设置 OpenRouter API 密钥。
func SetOpenrouterApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	safeKey := key
	if safeKey == "undefined" {
		safeKey = ""
	}
	upsertAuthProfileCredential("openrouter:default",
		buildApiKeyCredential("openrouter", safeKey, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetCloudflareAiGatewayConfig 设置 Cloudflare AI Gateway 配置。
func SetCloudflareAiGatewayConfig(accountID, gatewayID, apiKey string, agentDir string, opts *ApiKeyStorageOptions) {
	normalizedAccountID := strings.TrimSpace(accountID)
	normalizedGatewayID := strings.TrimSpace(gatewayID)
	metadata := map[string]string{
		"accountId": normalizedAccountID,
		"gatewayId": normalizedGatewayID,
	}
	upsertAuthProfileCredential("cloudflare-ai-gateway:default",
		buildApiKeyCredential("cloudflare-ai-gateway", apiKey, metadata, opts), resolveAuthAgentDir(agentDir))
}

// SetLitellmApiKey 设置 LiteLLM API 密钥。
func SetLitellmApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("litellm:default",
		buildApiKeyCredential("litellm", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetVercelAiGatewayApiKey 设置 Vercel AI Gateway API 密钥。
func SetVercelAiGatewayApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("vercel-ai-gateway:default",
		buildApiKeyCredential("vercel-ai-gateway", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetOpencodeZenApiKey 设置 OpenCode Zen API 密钥。
func SetOpencodeZenApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("opencode:default",
		buildApiKeyCredential("opencode", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetTogetherApiKey 设置 Together API 密钥。
func SetTogetherApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("together:default",
		buildApiKeyCredential("together", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetHuggingfaceApiKey 设置 Hugging Face API 密钥。
func SetHuggingfaceApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("huggingface:default",
		buildApiKeyCredential("huggingface", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetQianfanApiKey 设置千帆 API 密钥。
func SetQianfanApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("qianfan:default",
		buildApiKeyCredential("qianfan", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetXaiApiKey 设置 xAI API 密钥。
func SetXaiApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("xai:default",
		buildApiKeyCredential("xai", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetMistralApiKey 设置 Mistral API 密钥。
func SetMistralApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("mistral:default",
		buildApiKeyCredential("mistral", key, nil, opts), resolveAuthAgentDir(agentDir))
}

// SetKilocodeApiKey 设置 Kilocode API 密钥。
func SetKilocodeApiKey(key string, agentDir string, opts *ApiKeyStorageOptions) {
	upsertAuthProfileCredential("kilocode:default",
		buildApiKeyCredential("kilocode", key, nil, opts), resolveAuthAgentDir(agentDir))
}
