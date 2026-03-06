package media

// stt.go — 语音转文本（Speech-to-Text）Provider 接口（Phase C 新增）
// 提供统一的 STT 抽象，支持多种后端（OpenAI/Groq/Azure/本地 whisper）
// 可通过配置向导独立配置和切换

import (
	"context"
	"fmt"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// STTProvider 语音转文本 Provider 接口
type STTProvider interface {
	// Transcribe 将音频数据转录为文本
	// audioData: 原始音频二进制数据
	// mimeType: 音频 MIME 类型 (audio/opus, audio/mp3, audio/wav 等)
	// Returns: 转录文本, 错误
	Transcribe(ctx context.Context, audioData []byte, mimeType string) (string, error)

	// Name 返回 Provider 名称
	Name() string

	// TestConnection 测试连接（用空音频或 API 验证）
	TestConnection(ctx context.Context) error
}

// NewSTTProvider 根据配置创建 STT Provider（工厂方法）
func NewSTTProvider(cfg *types.STTConfig) (STTProvider, error) {
	if cfg == nil || cfg.Provider == "" {
		return nil, fmt.Errorf("stt: provider not configured")
	}

	switch cfg.Provider {
	case "openai":
		return NewOpenAISTT(cfg), nil
	case "groq":
		// Groq 使用 OpenAI 兼容 API，只是 BaseURL 不同
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.groq.com/openai/v1"
		}
		if cfg.Model == "" {
			cfg.Model = "whisper-large-v3"
		}
		return NewOpenAISTT(cfg), nil
	case "azure":
		return NewOpenAISTT(cfg), nil // Azure 也走 OpenAI 兼容 API
	case "qwen":
		// 通义千问 DashScope — 使用 WebSocket 实时语音识别 API。
		// DashScope 原生批量转录 API 仅接受 HTTP(S) URL，OpenAI 兼容端点不支持 /audio/transcriptions，
		// 因此使用 WebSocket API（wss://dashscope.aliyuncs.com/api-ws/v1/inference/）直接传入音频字节。
		return NewDashScopeSTT(cfg), nil
	case "ollama":
		// 本地 Ollama OpenAI 兼容 API
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:11434/v1"
		}
		return NewOpenAISTT(cfg), nil
	case "local-whisper":
		return NewLocalWhisperSTT(cfg), nil
	default:
		return nil, fmt.Errorf("stt: unknown provider: %s", cfg.Provider)
	}
}

// DefaultSTTModels 返回各 Provider 可用的模型列表
func DefaultSTTModels(provider string) []string {
	switch provider {
	case "openai":
		return []string{"whisper-1", "gpt-4o-transcribe", "gpt-4o-mini-transcribe"}
	case "groq":
		return []string{"whisper-large-v3", "whisper-large-v3-turbo", "distil-whisper-large-v3-en"}
	case "azure":
		return []string{"whisper-1"}
	case "qwen":
		return []string{"sensevoice-v1", "paraformer-realtime-v2", "paraformer-v2"}
	case "ollama":
		return []string{"whisper"}
	case "local-whisper":
		return []string{"ggml-base", "ggml-small", "ggml-medium", "ggml-large-v3"}
	default:
		return nil
	}
}
