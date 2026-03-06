package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
	"github.com/gorilla/websocket"
)

// TestDashScopeSTT_Name 验证 Provider 名称
func TestDashScopeSTT_Name(t *testing.T) {
	stt := NewDashScopeSTT(&types.STTConfig{
		Provider: "qwen",
		APIKey:   "test-key",
	})
	if stt.Name() != "dashscope" {
		t.Errorf("expected name 'dashscope', got %q", stt.Name())
	}
}

// mockWSServer 创建一个模拟 DashScope WebSocket 服务器
func mockWSServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 Authorization
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("ws upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		handler(conn)
	}))
	return srv
}

// wsURL 将 http:// 转换为 ws://
func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

// TestDashScopeSTT_Transcribe_Success 模拟正常转录流程
func TestDashScopeSTT_Transcribe_Success(t *testing.T) {
	srv := mockWSServer(t, func(conn *websocket.Conn) {
		// 1. 读取 run-task 指令
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Logf("read run-task failed: %v", err)
			return
		}
		var event wsEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Logf("parse run-task failed: %v", err)
			return
		}
		taskID := event.Header.TaskID

		// 2. 发送 task-started
		started := wsEvent{
			Header: wsHeader{Event: "task-started", TaskID: taskID},
		}
		startedJSON, _ := json.Marshal(started)
		conn.WriteMessage(websocket.TextMessage, startedJSON)

		// 3. 读取音频数据（忽略内容）
		for {
			msgType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				break // finish-task 指令
			}
		}

		// 4. 发送识别结果
		endTime := int64(5000)
		result := wsEvent{
			Header: wsHeader{Event: "result-generated", TaskID: taskID},
			Payload: wsPayload{
				Output: wsOutput{
					Sentence: wsSentence{
						BeginTime: 0,
						EndTime:   &endTime,
						Text:      "你好世界",
					},
				},
			},
		}
		resultJSON, _ := json.Marshal(result)
		conn.WriteMessage(websocket.TextMessage, resultJSON)

		// 5. 发送 task-finished
		finished := wsEvent{
			Header: wsHeader{Event: "task-finished", TaskID: taskID},
		}
		finishedJSON, _ := json.Marshal(finished)
		conn.WriteMessage(websocket.TextMessage, finishedJSON)
	})
	defer srv.Close()

	stt := NewDashScopeSTT(&types.STTConfig{
		Provider: "qwen",
		APIKey:   "test-key-123",
		BaseURL:  wsURL(srv.URL),
		Model:    "paraformer-realtime-v2",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	text, err := stt.Transcribe(ctx, []byte("fake-audio-data"), "audio/opus")
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if text != "你好世界" {
		t.Errorf("expected '你好世界', got %q", text)
	}
}

// TestDashScopeSTT_Transcribe_EmptyAudio 空音频应报错
func TestDashScopeSTT_Transcribe_EmptyAudio(t *testing.T) {
	stt := NewDashScopeSTT(&types.STTConfig{
		Provider: "qwen",
		APIKey:   "test-key",
	})
	_, err := stt.Transcribe(context.Background(), nil, "audio/opus")
	if err == nil {
		t.Error("expected error for empty audio")
	}
}

// TestDashScopeSTT_Transcribe_NoAPIKey 无 API Key 应报错
func TestDashScopeSTT_Transcribe_NoAPIKey(t *testing.T) {
	stt := NewDashScopeSTT(&types.STTConfig{
		Provider: "qwen",
	})
	_, err := stt.Transcribe(context.Background(), []byte("data"), "audio/opus")
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

// TestDashScopeSTT_Transcribe_TaskFailed 任务失败返回错误
func TestDashScopeSTT_Transcribe_TaskFailed(t *testing.T) {
	srv := mockWSServer(t, func(conn *websocket.Conn) {
		// 读取 run-task
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var event wsEvent
		json.Unmarshal(msg, &event)
		taskID := event.Header.TaskID

		// 发送 task-started
		started := wsEvent{
			Header: wsHeader{Event: "task-started", TaskID: taskID},
		}
		startedJSON, _ := json.Marshal(started)
		conn.WriteMessage(websocket.TextMessage, startedJSON)

		// 读取音频 + finish-task
		for {
			msgType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				break
			}
		}

		// 发送 task-failed
		failed := wsEvent{
			Header: wsHeader{
				Event:        "task-failed",
				TaskID:       taskID,
				ErrorCode:    "InvalidFormat",
				ErrorMessage: "audio format not supported",
			},
		}
		failedJSON, _ := json.Marshal(failed)
		conn.WriteMessage(websocket.TextMessage, failedJSON)
	})
	defer srv.Close()

	stt := NewDashScopeSTT(&types.STTConfig{
		Provider: "qwen",
		APIKey:   "test-key",
		BaseURL:  wsURL(srv.URL),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := stt.Transcribe(ctx, []byte("data"), "audio/opus")
	if err == nil {
		t.Error("expected error for failed task")
	}
	if !strings.Contains(err.Error(), "audio format not supported") {
		t.Errorf("expected error message to contain 'audio format not supported', got: %v", err)
	}
}

// TestDashScopeSTT_TestConnection 测试连接
func TestDashScopeSTT_TestConnection(t *testing.T) {
	srv := mockWSServer(t, func(conn *websocket.Conn) {
		// 连接成功即可，不做其他操作
	})
	defer srv.Close()

	t.Run("valid key", func(t *testing.T) {
		stt := NewDashScopeSTT(&types.STTConfig{
			Provider: "qwen",
			APIKey:   "good-key",
			BaseURL:  wsURL(srv.URL),
		})
		if err := stt.TestConnection(context.Background()); err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("no key", func(t *testing.T) {
		stt := NewDashScopeSTT(&types.STTConfig{Provider: "qwen"})
		err := stt.TestConnection(context.Background())
		if err == nil {
			t.Error("expected error for no key")
		}
	})
}

// TestNewSTTProvider_Qwen 验证工厂方法路由到 DashScopeSTT
func TestNewSTTProvider_Qwen(t *testing.T) {
	provider, err := NewSTTProvider(&types.STTConfig{
		Provider: "qwen",
		APIKey:   "test-key",
	})
	if err != nil {
		t.Fatalf("NewSTTProvider failed: %v", err)
	}
	if provider.Name() != "dashscope" {
		t.Errorf("expected provider name 'dashscope', got %q", provider.Name())
	}
}
