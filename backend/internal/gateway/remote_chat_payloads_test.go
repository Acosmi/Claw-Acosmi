package gateway

import (
	"path/filepath"
	"testing"

	agentsession "github.com/Acosmi/ClawAcosmi/internal/agents/session"
)

func TestBuildRemoteAssistantMessageIncludesTextAndMedia(t *testing.T) {
	message := buildRemoteAssistantMessage(
		"done",
		123,
		[]ReplyMediaItem{{Base64Data: "abc", MimeType: "image/png"}},
		"",
		"",
	)
	if message == nil {
		t.Fatal("expected message")
	}
	if role, _ := message["role"].(string); role != "assistant" {
		t.Fatalf("unexpected role: %v", message["role"])
	}
	if timestamp, _ := message["timestamp"].(int64); timestamp != 123 {
		t.Fatalf("unexpected timestamp: %v", message["timestamp"])
	}
	content, _ := message["content"].([]interface{})
	if len(content) != 2 {
		t.Fatalf("expected text + image blocks, got %d", len(content))
	}
}

// P5 Usage 收敛: 非 LLM 消息不应写虚假 usage 零值。
func TestBuildRemoteAssistantMessage_OmitsUsage(t *testing.T) {
	message := buildRemoteAssistantMessage("hello", 100, nil, "", "")
	if message == nil {
		t.Fatal("expected message")
	}
	if _, hasUsage := message["usage"]; hasUsage {
		t.Fatal("non-LLM message should not include usage field")
	}
}

func TestBuildRemoteAssistantChatPayloadCarriesMediaFields(t *testing.T) {
	payload := buildRemoteAssistantChatPayload(
		"feishu:oc_1",
		"feishu",
		"oc_1",
		"done",
		456,
		[]ReplyMediaItem{{Base64Data: "abc", MimeType: "image/png"}},
		"",
		"",
	)
	if payload == nil {
		t.Fatal("expected payload")
	}
	if payload["sessionKey"] != "feishu:oc_1" || payload["channel"] != "feishu" {
		t.Fatalf("unexpected routing payload: %+v", payload)
	}
	if payload["mediaBase64"] != "abc" || payload["mediaMimeType"] != "image/png" {
		t.Fatalf("unexpected media fields: %+v", payload)
	}
	items, _ := payload["mediaItems"].([]map[string]string)
	if len(items) != 1 {
		t.Fatalf("expected one media item, got %+v", payload["mediaItems"])
	}
}

func TestBuildRemoteAssistantChatPayloadFromMessageUsesTranscriptTimestamp(t *testing.T) {
	payload := buildRemoteAssistantChatPayloadFromMessage(
		"feishu:oc_1",
		"feishu",
		"oc_1",
		map[string]interface{}{
			"role":      "assistant",
			"timestamp": float64(789),
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "done"},
				map[string]interface{}{
					"type": "image",
					"source": map[string]interface{}{
						"type":       "base64",
						"data":       "abc",
						"media_type": "image/png",
					},
				},
			},
		},
	)
	if payload == nil {
		t.Fatal("expected payload")
	}
	if payload["ts"] != int64(789) {
		t.Fatalf("ts=%v, want 789", payload["ts"])
	}
	if payload["text"] != "done" {
		t.Fatalf("text=%v, want done", payload["text"])
	}
	if payload["mediaBase64"] != "abc" {
		t.Fatalf("unexpected mediaBase64: %+v", payload)
	}
}

func TestLoadLatestAssistantTranscriptMessageReturnsNewestAssistant(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "remote.jsonl")
	mgr := agentsession.NewSessionManager("")
	if _, err := mgr.EnsureSessionFile("remote-1", sessionFile); err != nil {
		t.Fatalf("EnsureSessionFile: %v", err)
	}
	if err := mgr.AppendMessage("remote-1", sessionFile, agentsession.TranscriptEntry{
		Role:      "assistant",
		Content:   []agentsession.ContentBlock{agentsession.TextBlock("old")},
		Timestamp: 100,
	}); err != nil {
		t.Fatalf("AppendMessage old: %v", err)
	}
	if err := mgr.AppendMessage("remote-1", sessionFile, agentsession.TranscriptEntry{
		Role:      "user",
		Content:   []agentsession.ContentBlock{agentsession.TextBlock("next")},
		Timestamp: 101,
	}); err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if err := mgr.AppendMessage("remote-1", sessionFile, agentsession.TranscriptEntry{
		Role: "assistant",
		Content: []agentsession.ContentBlock{
			agentsession.TextBlock("latest"),
			{
				Type:     "image",
				MimeType: "image/png",
				Source: &agentsession.MediaSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "abc",
				},
			},
		},
		Timestamp: 102,
	}); err != nil {
		t.Fatalf("AppendMessage latest: %v", err)
	}

	message := loadLatestAssistantTranscriptMessage("remote-1", tmpDir, sessionFile)
	if message == nil {
		t.Fatal("expected assistant message")
	}
	payload := buildRemoteAssistantChatPayloadFromMessage("feishu:oc_1", "feishu", "oc_1", message)
	if payload == nil {
		t.Fatal("expected payload")
	}
	if payload["ts"] != int64(102) {
		t.Fatalf("ts=%v, want 102", payload["ts"])
	}
	if payload["text"] != "latest" {
		t.Fatalf("text=%v, want latest", payload["text"])
	}
	if payload["mediaBase64"] != "abc" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
