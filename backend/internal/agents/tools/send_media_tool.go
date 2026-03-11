// tools/send_media_tool.go — 媒体发送工具（备用）。
// 允许 Agent 主动向频道发送图片/媒体数据。
// 默认不注册，需显式启用 EnableSendMedia。
package tools

import (
	"context"
	"fmt"
)

// MediaSender 媒体发送接口。
// 实现方通过 sessions_send RPC 或直接频道 API 投递媒体。
type MediaSender interface {
	SendMedia(ctx context.Context, target, mediaBase64, mimeType string) error
}

// CreateSendMediaTool 创建媒体发送工具 schema。
// 实际执行由 runner/tool_executor.go 处理，sender 参数保留用于接口兼容。
func CreateSendMediaTool(_ MediaSender) *AgentTool {
	return &AgentTool{
		Name:        "send_media",
		Label:       "Send Media",
		Description: "Send a file or media to the current conversation channel. Do NOT provide 'target' — it defaults to current channel. Use file_path with an ABSOLUTE path. If the user refers to the latest image already attached in this chat, you may omit both file_path and media_base64 and the tool will reuse that attachment automatically. Only use media_base64 if data is already in base64 form.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "ABSOLUTE path to a local file (e.g. '/tmp/screenshot.png'). Preferred method.",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "DO NOT SET unless sending to a different channel. Defaults to current conversation channel.",
				},
				"media_base64": map[string]any{
					"type":        "string",
					"description": "Base64-encoded media data. Only use when data is already in base64 form.",
				},
				"file_name": map[string]any{
					"type":        "string",
					"description": "Optional original filename. Recommended when using media_base64 so channels can preserve the attachment name.",
				},
				"mime_type": map[string]any{
					"type":        "string",
					"description": "MIME type. Auto-detected from file extension when using file_path.",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional text message to accompany the file.",
				},
			},
		},
		// Execute 占位实现。
		// 实际 send_media 执行由 runner/tool_executor.go 的 executeSendMedia() 处理。
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (*AgentToolResult, error) {
			return nil, fmt.Errorf("send_media: execution should be handled by tool_executor.go, not Execute path")
		},
	}
}
