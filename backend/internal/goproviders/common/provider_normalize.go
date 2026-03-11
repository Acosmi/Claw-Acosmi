// common/provider_normalize.go — Provider ID 规范化工具
// 对应 TS 文件: src/agents/model-selection.ts（仅提取 Window 4 所需函数）
package common

import "strings"

// NormalizeProviderId 规范化 Provider ID。
// 将已知别名映射为标准标识符。
// 对应 TS: normalizeProviderId()
func NormalizeProviderId(provider string) string {
	normalized := strings.TrimSpace(strings.ToLower(provider))
	switch normalized {
	case "z.ai", "z-ai":
		return "zai"
	case "opencode-zen":
		return "opencode"
	case "qwen":
		return "qwen-portal"
	case "kimi-code":
		return "kimi-coding"
	case "bedrock", "aws-bedrock":
		return "amazon-bedrock"
	case "bytedance", "doubao":
		return "volcengine"
	default:
		return normalized
	}
}

// NormalizeProviderIdForAuth 规范化 Provider ID 用于认证查找。
// 编码计划变体与基础共享认证。
// 对应 TS: normalizeProviderIdForAuth()
func NormalizeProviderIdForAuth(provider string) string {
	normalized := NormalizeProviderId(provider)
	switch normalized {
	case "minimax":
		return "minimax-portal"
	case "volcengine-plan":
		return "volcengine"
	case "byteplus-plan":
		return "byteplus"
	default:
		return normalized
	}
}

// FindNormalizedProviderValue 在映射中按规范化 Provider ID 查找值。
// 对应 TS: findNormalizedProviderValue()
func FindNormalizedProviderValue(entries map[string][]string, provider string) []string {
	if entries == nil {
		return nil
	}
	providerKey := NormalizeProviderId(provider)
	for key, value := range entries {
		if NormalizeProviderId(key) == providerKey {
			return value
		}
	}
	return nil
}

// FindNormalizedProviderKey 在映射中按规范化 Provider ID 查找原始键名。
// 对应 TS: findNormalizedProviderKey()
func FindNormalizedProviderKey(entries map[string]interface{}, provider string) string {
	if entries == nil {
		return ""
	}
	providerKey := NormalizeProviderId(provider)
	for key := range entries {
		if NormalizeProviderId(key) == providerKey {
			return key
		}
	}
	return ""
}
