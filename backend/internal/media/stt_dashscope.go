package media

// stt_dashscope.go — 阿里云 DashScope 实时语音识别（WebSocket API）
//
// DashScope 的录音文件识别 API 仅接受 HTTP(S) URL，不支持 data: URL 或内联 base64 数据。
// OpenAI 兼容端点（/compatible-mode/v1）不覆盖 /audio/transcriptions。
//
// 因此本文件使用 DashScope WebSocket 实时语音识别 API：
//   1. 连接 wss://dashscope.aliyuncs.com/api-ws/v1/inference/
//   2. 发送 run-task 指令（含模型、格式参数）
//   3. 等待 task-started 事件
//   4. 发送二进制音频数据
//   5. 发送 finish-task 指令
//   6. 收集 result-generated 事件中的文本
//   7. 等待 task-finished 事件
//
// 参考文档：https://help.aliyun.com/zh/model-studio/websocket-for-paraformer-real-time-service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// DashScopeSTT 阿里云 DashScope WebSocket STT Provider
type DashScopeSTT struct {
	apiKey string
	model  string
	wsURL  string
	lang   string
}

// NewDashScopeSTT 创建 DashScope STT Provider
func NewDashScopeSTT(cfg *types.STTConfig) *DashScopeSTT {
	wsURL := "wss://dashscope.aliyuncs.com/api-ws/v1/inference/"
	if cfg.BaseURL != "" {
		// 允许覆盖 WebSocket URL（测试或私有部署）
		base := strings.TrimSuffix(cfg.BaseURL, "/")
		if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") {
			wsURL = base + "/"
		}
	}
	model := cfg.Model
	if model == "" {
		model = "paraformer-realtime-v2"
	}
	return &DashScopeSTT{
		apiKey: cfg.APIKey,
		model:  model,
		wsURL:  wsURL,
		lang:   cfg.Language,
	}
}

// Name 返回 Provider 名称
func (d *DashScopeSTT) Name() string {
	return "dashscope"
}

// ---------- WebSocket 协议类型 ----------

type wsHeader struct {
	Action       string                 `json:"action,omitempty"`
	TaskID       string                 `json:"task_id"`
	Streaming    string                 `json:"streaming,omitempty"`
	Event        string                 `json:"event,omitempty"`
	ErrorCode    string                 `json:"error_code,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Attributes   map[string]interface{} `json:"attributes,omitempty"`
}

type wsSentence struct {
	BeginTime int64  `json:"begin_time"`
	EndTime   *int64 `json:"end_time"`
	Text      string `json:"text"`
}

type wsOutput struct {
	Sentence wsSentence `json:"sentence"`
}

type wsParams struct {
	Format        string   `json:"format"`
	SampleRate    int      `json:"sample_rate"`
	LanguageHints []string `json:"language_hints,omitempty"`
}

type wsPayload struct {
	TaskGroup  string    `json:"task_group,omitempty"`
	Task       string    `json:"task,omitempty"`
	Function   string    `json:"function,omitempty"`
	Model      string    `json:"model,omitempty"`
	Parameters *wsParams `json:"parameters,omitempty"`
	Input      struct{}  `json:"input"`
	Output     wsOutput  `json:"output,omitempty"`
}

type wsEvent struct {
	Header  wsHeader  `json:"header"`
	Payload wsPayload `json:"payload"`
}

// ---------- STTProvider 接口实现 ----------

// mimeToFormat 将 MIME 类型转换为 DashScope 格式标识
func mimeToFormat(mimeType string) string {
	base := strings.Split(mimeType, ";")[0]
	base = strings.TrimSpace(base)
	switch base {
	case "audio/opus", "audio/ogg":
		return "opus"
	case "audio/wav", "audio/x-wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/mp4", "audio/m4a", "audio/aac":
		return "aac"
	case "audio/flac":
		return "flac"
	case "audio/amr":
		return "amr"
	case "audio/webm":
		return "opus" // WebM/Opus 编码兼容
	default:
		return "pcm"
	}
}

// Transcribe 通过 WebSocket API 将音频数据转录为文本
func (d *DashScopeSTT) Transcribe(ctx context.Context, audioData []byte, mimeType string) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("stt/dashscope: empty audio data")
	}
	if d.apiKey == "" {
		return "", fmt.Errorf("stt/dashscope: API key not set")
	}

	if mimeType == "" {
		mimeType = "audio/opus"
	}
	format := mimeToFormat(mimeType)

	// 1. 建立 WebSocket 连接
	header := make(http.Header)
	header.Set("Authorization", "bearer "+d.apiKey)

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, d.wsURL, header)
	if err != nil {
		return "", fmt.Errorf("stt/dashscope: websocket connect: %w", err)
	}
	defer conn.Close()

	// 2. 发送 run-task 指令
	taskID := uuid.New().String()
	runTask := wsEvent{
		Header: wsHeader{
			Action:    "run-task",
			TaskID:    taskID,
			Streaming: "duplex",
		},
		Payload: wsPayload{
			TaskGroup: "audio",
			Task:      "asr",
			Function:  "recognition",
			Model:     d.model,
			Parameters: &wsParams{
				Format:     format,
				SampleRate: 16000,
			},
			Input: struct{}{},
		},
	}

	if d.lang != "" {
		runTask.Payload.Parameters.LanguageHints = []string{d.lang}
	}

	runTaskJSON, err := json.Marshal(runTask)
	if err != nil {
		return "", fmt.Errorf("stt/dashscope: marshal run-task: %w", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, runTaskJSON); err != nil {
		return "", fmt.Errorf("stt/dashscope: send run-task: %w", err)
	}

	slog.Info("stt/dashscope: task submitted (ws)",
		"task_id", taskID,
		"model", d.model,
		"format", format,
		"audio_size", len(audioData),
	)

	// 3. 启动接收协程
	type wsResult struct {
		text string
		err  error
	}
	resultCh := make(chan wsResult, 1)
	taskStarted := make(chan struct{}, 1)

	go func() {
		var texts []string
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				resultCh <- wsResult{err: fmt.Errorf("read ws: %w", err)}
				return
			}
			var event wsEvent
			if err := json.Unmarshal(message, &event); err != nil {
				resultCh <- wsResult{err: fmt.Errorf("parse ws event: %w", err)}
				return
			}

			switch event.Header.Event {
			case "task-started":
				select {
				case taskStarted <- struct{}{}:
				default:
				}
			case "result-generated":
				if event.Payload.Output.Sentence.Text != "" {
					// 只保留最终结果（EndTime 非 nil 表示句子完成）
					if event.Payload.Output.Sentence.EndTime != nil {
						texts = append(texts, event.Payload.Output.Sentence.Text)
					}
				}
			case "task-finished":
				resultCh <- wsResult{text: strings.Join(texts, "")}
				return
			case "task-failed":
				errMsg := event.Header.ErrorMessage
				if errMsg == "" {
					errMsg = "unknown error"
				}
				resultCh <- wsResult{err: fmt.Errorf("task failed: %s (code: %s)", errMsg, event.Header.ErrorCode)}
				return
			}
		}
	}()

	// 4. 等待 task-started
	select {
	case <-taskStarted:
		// OK
	case result := <-resultCh:
		// 任务在启动前就失败了
		if result.err != nil {
			return "", fmt.Errorf("stt/dashscope: %w", result.err)
		}
		return result.text, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(10 * time.Second):
		return "", fmt.Errorf("stt/dashscope: task-started timeout")
	}

	// 5. 发送音频数据（分块发送，每块 3200 字节）
	const chunkSize = 3200
	for offset := 0; offset < len(audioData); offset += chunkSize {
		end := offset + chunkSize
		if end > len(audioData) {
			end = len(audioData)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, audioData[offset:end]); err != nil {
			return "", fmt.Errorf("stt/dashscope: send audio: %w", err)
		}
	}

	// 6. 发送 finish-task 指令
	finishTask := wsEvent{
		Header: wsHeader{
			Action:    "finish-task",
			TaskID:    taskID,
			Streaming: "duplex",
		},
		Payload: wsPayload{
			Input: struct{}{},
		},
	}
	finishTaskJSON, err := json.Marshal(finishTask)
	if err != nil {
		return "", fmt.Errorf("stt/dashscope: marshal finish-task: %w", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, finishTaskJSON); err != nil {
		return "", fmt.Errorf("stt/dashscope: send finish-task: %w", err)
	}

	// 7. 等待结果
	select {
	case result := <-resultCh:
		if result.err != nil {
			return "", fmt.Errorf("stt/dashscope: %w", result.err)
		}
		slog.Info("stt/dashscope: transcription complete (ws)",
			"task_id", taskID,
			"text_len", len(result.text),
		)
		return result.text, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// TestConnection 测试 API 连接
func (d *DashScopeSTT) TestConnection(ctx context.Context) error {
	if d.apiKey == "" {
		return fmt.Errorf("stt/dashscope: API key not set")
	}

	// 尝试建立 WebSocket 连接验证 API Key
	header := make(http.Header)
	header.Set("Authorization", "bearer "+d.apiKey)

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, d.wsURL, header)
	if err != nil {
		return fmt.Errorf("stt/dashscope: connection test failed: %w", err)
	}
	conn.Close()
	return nil
}
