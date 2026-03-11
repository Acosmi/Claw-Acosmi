package runner

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/llmclient"
	"github.com/Acosmi/ClawAcosmi/internal/agents/session"
)

// TestPersistToTranscript_TerminalReplyInjection 验证 S1 终端回复注入模式:
// 当 messages 包含合成终端回复 (如熔断/审批拒绝) 时，
// persistToTranscript 正确将其写入 transcript JSONL 的 assistant 条目。
//
// 这验证了 S1 的核心设计: 终端回复注入 messages 后，
// defer persist / 正常 persist 均读到正确内容，无需修改 persistToTranscript 签名。
func TestPersistToTranscript_TerminalReplyInjection(t *testing.T) {
	// 模拟 8 个 S1 终端路径的合成回复
	terminalReplies := []struct {
		name    string
		message string
	}{
		{"circuit_breaker", "⚠️ 工具 bash 已连续调用 5 次未取得进展，自动终止循环。"},
		{"tool_hard_stop", "⚠️ Tool bash failed 3 times. Forced loop termination."},
		{"global_failure_budget", "⚠️ 工具调用累计失败 10 次（主要: bash × 6），自动终止循环。"},
		{"approval_denied", "⚠️ 用户刚刚拒绝了当前高风险操作的授权，本轮已停止。"},
		{"approval_timeout", "⚠️ 权限审批被拒绝或超时。如需执行此操作，请调整安全设置后重试。"},
		{"perm_denied_circuit", "⚠️ 权限不足，无法执行请求的操作。"},
		{"plan_error", "方案确认出错: context deadline exceeded"},
		{"plan_reject", "方案已被拒绝。用户认为方案不合适。"},
	}

	for _, tr := range terminalReplies {
		t.Run(tr.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			sessionID := "test-terminal-" + tr.name
			sessionFile := filepath.Join(tmpDir, sessionID+".jsonl")

			mgr := session.NewSessionManager("")
			if _, err := mgr.EnsureSessionFile(sessionID, sessionFile); err != nil {
				t.Fatalf("ensure session file: %v", err)
			}

			r := &EmbeddedAttemptRunner{}
			params := AttemptParams{
				SessionID:   sessionID,
				SessionFile: sessionFile,
				Prompt:      "执行某个操作",
			}

			// 模拟 S1 注入模式: user message + 合成终端回复 assistant message
			messages := []llmclient.ChatMessage{
				{Role: "user", Content: []llmclient.ContentBlock{{Type: "text", Text: "执行某个操作"}}},
				llmclient.TextMessage("assistant", tr.message),
			}

			log := slog.Default()
			r.persistToTranscript(params, messages, nil, llmclient.UsageInfo{}, nil, log)

			// 验证 transcript 包含终端回复
			entries, err := mgr.LoadSessionMessages(sessionID, sessionFile)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}

			if len(entries) < 2 {
				t.Fatalf("expected at least 2 entries (user+assistant), got %d", len(entries))
			}

			// 最后一条应为 assistant 且包含终端回复文本
			lastEntry := entries[len(entries)-1]
			role, _ := lastEntry["role"].(string)
			if role != "assistant" {
				t.Fatalf("expected last entry role=assistant, got %q", role)
			}

			content, ok := lastEntry["content"].([]interface{})
			if !ok || len(content) == 0 {
				t.Fatalf("expected non-empty assistant content, got %v", lastEntry["content"])
			}

			block, _ := content[0].(map[string]interface{})
			text, _ := block["text"].(string)
			if !strings.Contains(text, tr.message[:20]) {
				t.Errorf("transcript assistant text should contain terminal reply.\nwant (prefix): %q\ngot: %q",
					tr.message[:20], text)
			}
		})
	}
}
