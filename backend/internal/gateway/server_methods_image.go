package gateway

// server_methods_image.go — 图片理解 Fallback RPC 方法（Phase E 新增）
// 提供 image.config.get / image.config.set / image.test / image.models / image.ollama.models 方法
// 遵循 server_methods_stt.go 完全相同的模式

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ImageHandlers 返回图片理解 RPC 方法处理器。
func ImageHandlers() map[string]GatewayMethodHandler {
	return map[string]GatewayMethodHandler{
		"image.config.get":    handleImageConfigGet,
		"image.config.set":    handleImageConfigSet,
		"image.test":          handleImageTest,
		"image.models":        handleImageModels,
		"image.ollama.models": handleImageOllamaModels,
	}
}

// ---------- image.config.get ----------

// ImageConfigGetResult image.config.get 响应
type ImageConfigGetResult struct {
	Configured   bool                `json:"configured"`
	Hash         string              `json:"hash,omitempty"`
	Provider     string              `json:"provider,omitempty"`
	Model        string              `json:"model,omitempty"`
	BaseURL      string              `json:"baseUrl,omitempty"`
	Prompt       string              `json:"prompt,omitempty"`
	MaxTokens    int                 `json:"maxTokens,omitempty"`
	HasAPIKey    bool                `json:"hasApiKey"`
	Providers    []ImageProviderInfo `json:"providers"`
	OllamaOnline bool                `json:"ollamaOnline"`
}

// ImageProviderInfo 可选图片理解 Provider 描述
type ImageProviderInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

func buildImageConfigGetResult(cfg *types.ImageUnderstandingConfig) ImageConfigGetResult {
	ollamaOnline := probeOllama()

	ollamaHint := "本地 Ollama 视觉模型（llava 等）"
	if ollamaOnline {
		ollamaHint = "已检测到本地 Ollama 运行中 ✓"
	} else {
		ollamaHint = "未检测到 Ollama（localhost:11434）"
	}

	result := ImageConfigGetResult{
		OllamaOnline: ollamaOnline,
		Providers: []ImageProviderInfo{
			{ID: "qwen-vl", Label: "通义千问 Qwen-VL", Hint: "DashScope API，中文优化"},
			{ID: "openai", Label: "OpenAI GPT-4o", Hint: "gpt-4o-mini / gpt-4o"},
			{ID: "ollama", Label: "本地 Ollama", Hint: ollamaHint},
			{ID: "google", Label: "Google Gemini", Hint: "gemini-2.0-flash / gemini-1.5-pro"},
			{ID: "anthropic", Label: "Anthropic Claude", Hint: "claude-3-haiku / claude-3.5-sonnet"},
			{ID: "", Label: "禁用", Hint: "不使用图片理解 Fallback"},
		},
	}

	if cfg != nil && cfg.Provider != "" {
		result.Configured = true
		result.Provider = cfg.Provider
		result.Model = cfg.Model
		result.BaseURL = cfg.BaseURL
		result.Prompt = cfg.Prompt
		result.MaxTokens = cfg.MaxTokens
		result.HasAPIKey = cfg.APIKey != ""
	}

	return result
}

func handleImageConfigGet(ctx *MethodHandlerContext) {
	result := buildImageConfigGetResult(loadImageConfigFromCtx(ctx))
	if loader := ctx.Context.ConfigLoader; loader != nil {
		if snapshot, err := loader.ReadConfigFileSnapshot(); err == nil && snapshot != nil {
			result.Hash = snapshot.Hash
		}
	}

	ctx.Respond(true, result, nil)
}

// ---------- image.config.set ----------

func handleImageConfigSet(ctx *MethodHandlerContext) {
	executeConfigMutation(ctx, configMutationOptions{
		Action: "image.config.set",
		Mutate: func(currentCfg *types.OpenAcosmiConfig) error {
			if currentCfg.ImageUnderstanding == nil {
				currentCfg.ImageUnderstanding = &types.ImageUnderstandingConfig{}
			}
			current := currentCfg.ImageUnderstanding

			if provider, ok := readTrimmedStringParam(ctx.Params, "provider"); ok {
				current.Provider = provider
			}
			if apiKeyRaw, ok := ctx.Params["apiKey"].(string); ok {
				trimmed := strings.TrimSpace(apiKeyRaw)
				if !strings.HasPrefix(trimmed, "••") {
					current.APIKey = trimmed
				}
			}
			if model, ok := readTrimmedStringParam(ctx.Params, "model"); ok {
				current.Model = model
			}
			if baseURL, ok := readTrimmedStringParam(ctx.Params, "baseUrl"); ok {
				current.BaseURL = baseURL
			}
			if prompt, ok := readTrimmedStringParam(ctx.Params, "prompt"); ok {
				current.Prompt = prompt
			}
			if maxTokens, found, err := readOptionalIntParam(ctx.Params, "maxTokens"); err != nil {
				return err
			} else if found {
				current.MaxTokens = maxTokens
			}
			return nil
		},
		AfterWrite: func(_ *MethodHandlerContext, cfg *types.OpenAcosmiConfig) map[string]interface{} {
			result := buildImageConfigGetResult(cfg.ImageUnderstanding)
			return map[string]interface{}{
				"image": result,
			}
		},
	})
}

// ---------- image.test ----------

func handleImageTest(ctx *MethodHandlerContext) {
	cfg := loadImageConfigFromCtx(ctx)
	if cfg == nil || cfg.Provider == "" {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeBadRequest, "image understanding not configured"))
		return
	}

	describer, err := media.NewImageDescriber(cfg)
	if err != nil {
		ctx.Respond(false, nil, NewErrorShape(ErrCodeInternalError, "create provider: "+err.Error()))
		return
	}

	testCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := describer.TestConnection(testCtx); err != nil {
		ctx.Respond(true, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil)
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"success":  true,
		"provider": describer.Name(),
	}, nil)
}

// ---------- image.models ----------

func handleImageModels(ctx *MethodHandlerContext) {
	provider, _ := ctx.Params["provider"].(string)

	models := media.DefaultImageModels(provider)
	ctx.Respond(true, map[string]interface{}{
		"provider": provider,
		"models":   models,
	}, nil)
}

// ---------- image.ollama.models ----------

// handleImageOllamaModels 探测本地 Ollama 可用的视觉模型
func handleImageOllamaModels(ctx *MethodHandlerContext) {
	models, err := probeOllamaVisionModels()
	if err != nil {
		ctx.Respond(true, map[string]interface{}{
			"online": false,
			"error":  err.Error(),
			"models": []string{},
		}, nil)
		return
	}

	ctx.Respond(true, map[string]interface{}{
		"online": true,
		"models": models,
	}, nil)
}

// ---------- helpers ----------

func loadImageConfigFromCtx(ctx *MethodHandlerContext) *types.ImageUnderstandingConfig {
	cfgLoader := ctx.Context.ConfigLoader
	if cfgLoader == nil {
		return nil
	}
	cfg, err := cfgLoader.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.ImageUnderstanding
}

// probeOllamaVisionModels 探测本地 Ollama 可用的视觉模型。
// GET http://localhost:11434/api/tags → 按前缀匹配已知视觉模型。
func probeOllamaVisionModels() ([]string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, err
	}

	// 已知视觉模型前缀
	visionPrefixes := []string{
		"llava", "bakllava", "moondream", "llama3.2-vision",
		"cogvlm", "minicpm-v", "internvl",
	}

	var visionModels []string
	for _, m := range tagsResp.Models {
		name := strings.ToLower(m.Name)
		for _, prefix := range visionPrefixes {
			if strings.HasPrefix(name, prefix) {
				visionModels = append(visionModels, m.Name)
				break
			}
		}
	}

	if len(visionModels) == 0 {
		slog.Debug("image: no vision models found in Ollama", "total_models", len(tagsResp.Models))
	}

	return visionModels, nil
}
