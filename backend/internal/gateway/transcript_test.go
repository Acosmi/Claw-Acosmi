package gateway

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCapArrayByJSONBytes_KeepsLatestOversizedItem(t *testing.T) {
	items := []map[string]interface{}{
		{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "old"},
			},
		},
		{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": strings.Repeat("x", 2048)},
			},
		},
	}

	capped := CapArrayByJSONBytes(items, 128)
	if len(capped) != 1 {
		t.Fatalf("len=%d, want 1", len(capped))
	}
	content, _ := capped[0]["content"].([]interface{})
	block, _ := content[0].(map[string]interface{})
	text, _ := block["text"].(string)
	if len(text) != 2048 {
		t.Fatalf("latest oversized item was not preserved, text len=%d", len(text))
	}
}

// P5 Usage 收敛: AppendAssistantTranscriptMessage 的 usage 字段必须标记 source=injected。
func TestAppendAssistantTranscriptMessage_UsageSourceInjected(t *testing.T) {
	tmpDir := t.TempDir()
	sessionFile := filepath.Join(tmpDir, "test.jsonl")
	result := AppendAssistantTranscriptMessage(AppendTranscriptParams{
		SessionID:       "test-session",
		SessionFile:     sessionFile,
		Message:         "injected reply",
		CreateIfMissing: true,
	})
	if !result.OK {
		t.Fatalf("append failed: %s", result.Error)
	}
	if result.Message == nil {
		t.Fatal("message should not be nil")
	}
	usage, ok := result.Message["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("message should have usage field")
	}
	source, _ := usage["source"].(string)
	if source != "injected" {
		t.Fatalf("usage.source = %q, want 'injected'", source)
	}
	// 确保不含虚假 0 值
	if v, exists := usage["input"]; exists {
		t.Fatalf("usage should not contain input field, got %v", v)
	}
}

func TestStripEnvelopeFromMessages_StripsDowngradedToolDirectiveText(t *testing.T) {
	msgs := []map[string]interface{}{
		{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "[[lookup_skill]]\nname: acosmi-intro",
				},
			},
		},
	}

	got := StripEnvelopeFromMessages(msgs)
	content, _ := got[0]["content"].([]interface{})
	block, _ := content[0].(map[string]interface{})
	text, _ := block["text"].(string)
	if text != "" {
		t.Fatalf("expected downgraded tool directive text to be stripped, got %q", text)
	}
}
