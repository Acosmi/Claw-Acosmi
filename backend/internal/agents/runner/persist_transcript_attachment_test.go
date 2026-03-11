package runner

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
)

// TestPersistToTranscript_WithAttachments 验证 persistToTranscript 将附件 blocks 写入 user 消息。
func TestPersistToTranscript_WithAttachments(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-att"
	sessionFile := filepath.Join(tmpDir, sessionID+".jsonl")

	// 写入 header
	mgr := session.NewSessionManager("")
	if _, err := mgr.EnsureSessionFile(sessionID, sessionFile); err != nil {
		t.Fatalf("ensure session file: %v", err)
	}

	r := &EmbeddedAttemptRunner{}
	params := AttemptParams{
		SessionID:   sessionID,
		SessionFile: sessionFile,
		Prompt:      "look at this image",
		Provider:    "anthropic",
		ModelID:     "claude-sonnet-4-5",
		Attachments: []session.ContentBlock{
			{
				Type:     "image",
				FileName: "test.png",
				MimeType: "image/png",
				Source: &session.MediaSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      "iVBORw0KGgo=",
				},
			},
			{
				Type:     "document",
				FileName: "readme.md",
				FileSize: 256,
				MimeType: "text/markdown",
			},
		},
	}

	messages := []llmclient.ChatMessage{
		{Role: "user", Content: []llmclient.ContentBlock{{Type: "text", Text: "look at this image"}}},
		{Role: "assistant", Content: []llmclient.ContentBlock{{Type: "text", Text: "I see the image."}}},
	}

	log := slog.Default()
	r.persistToTranscript(params, messages, nil, llmclient.UsageInfo{
		InputTokens:      120,
		OutputTokens:     45,
		CacheReadTokens:  30,
		CacheWriteTokens: 10,
		TotalTokens:      205,
	}, map[string]int{"view_image": 2}, log)

	// 读取 transcript 验证
	entries, err := mgr.LoadSessionMessages(sessionID, sessionFile)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if len(entries) < 1 {
		t.Fatalf("expected at least 1 entry, got %d", len(entries))
	}

	// 验证 user 消息
	userEntry := entries[0]
	role, _ := userEntry["role"].(string)
	if role != "user" {
		t.Fatalf("expected user role, got %q", role)
	}

	content, ok := userEntry["content"].([]interface{})
	if !ok {
		t.Fatalf("expected content array, got %T", userEntry["content"])
	}

	// 应有 3 个 blocks: text + image + document
	if len(content) != 3 {
		t.Fatalf("expected 3 content blocks (text+image+document), got %d", len(content))
	}

	// 验证第一个 block 是 text
	block0, _ := content[0].(map[string]interface{})
	if blockType, _ := block0["type"].(string); blockType != "text" {
		t.Fatalf("expected first block type=text, got %q", blockType)
	}

	// 验证第二个 block 是 image
	block1, _ := content[1].(map[string]interface{})
	if blockType, _ := block1["type"].(string); blockType != "image" {
		t.Fatalf("expected second block type=image, got %q", blockType)
	}
	if fn, _ := block1["fileName"].(string); fn != "test.png" {
		t.Fatalf("expected fileName=test.png, got %q", fn)
	}

	// 验证第三个 block 是 document
	block2, _ := content[2].(map[string]interface{})
	if blockType, _ := block2["type"].(string); blockType != "document" {
		t.Fatalf("expected third block type=document, got %q", blockType)
	}

	assistantEntry := entries[len(entries)-1]
	if provider, _ := assistantEntry["provider"].(string); provider != "anthropic" {
		t.Fatalf("expected assistant provider=anthropic, got %q", provider)
	}
	if model, _ := assistantEntry["model"].(string); model != "claude-sonnet-4-5" {
		t.Fatalf("expected assistant model=claude-sonnet-4-5, got %q", model)
	}
	usage, ok := assistantEntry["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected assistant usage map, got %T", assistantEntry["usage"])
	}
	if cacheRead, _ := usage["cache_read_input_tokens"].(float64); int(cacheRead) != 30 {
		t.Fatalf("expected assistant cache_read_input_tokens=30, got %v", usage["cache_read_input_tokens"])
	}
	if cacheWrite, _ := usage["cache_creation_input_tokens"].(float64); int(cacheWrite) != 10 {
		t.Fatalf("expected assistant cache_creation_input_tokens=10, got %v", usage["cache_creation_input_tokens"])
	}
	if total, _ := usage["total_tokens"].(float64); int(total) != 205 {
		t.Fatalf("expected assistant total_tokens=205, got %v", usage["total_tokens"])
	}
	toolCalls, ok := assistantEntry["toolCalls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected assistant toolCalls map, got %T", assistantEntry["toolCalls"])
	}
	if count, _ := toolCalls["view_image"].(float64); int(count) != 2 {
		t.Fatalf("expected assistant toolCalls.view_image=2, got %v", toolCalls["view_image"])
	}

	// 清理
	os.Remove(sessionFile)
}

// TestPersistToTranscript_NoAttachments 验证无附件时行为不变。
func TestPersistToTranscript_NoAttachments(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-noatt"
	sessionFile := filepath.Join(tmpDir, sessionID+".jsonl")

	mgr := session.NewSessionManager("")
	if _, err := mgr.EnsureSessionFile(sessionID, sessionFile); err != nil {
		t.Fatalf("ensure session file: %v", err)
	}

	r := &EmbeddedAttemptRunner{}
	params := AttemptParams{
		SessionID:   sessionID,
		SessionFile: sessionFile,
		Prompt:      "hello",
		Provider:    "openai",
		ModelID:     "gpt-5",
	}

	messages := []llmclient.ChatMessage{
		{Role: "user", Content: []llmclient.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []llmclient.ContentBlock{{Type: "text", Text: "hi"}}},
	}

	log := slog.Default()
	r.persistToTranscript(params, messages, nil, llmclient.UsageInfo{
		InputTokens:  11,
		OutputTokens: 7,
	}, nil, log)

	entries, err := mgr.LoadSessionMessages(sessionID, sessionFile)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if len(entries) < 1 {
		t.Fatalf("expected at least 1 entry, got %d", len(entries))
	}

	userEntry := entries[0]
	content, _ := userEntry["content"].([]interface{})
	// 仅 1 个 text block
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
}
