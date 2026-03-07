package media

// ============================================================================
// media/social_interact_tool.go — 社交互动 LLM 工具
// 为子智能体提供 social_interact 工具接口，路由到平台互动管理器。
//
// Design doc: docs/xinshenji/impl-tracking-media-subagent.md §P3-3
// ============================================================================

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ---------- 互动管理接口 ----------

// SocialInteractor 社交互动能力抽象。
// xiaohongshu.RPAInteractionManager 实现此接口。
type SocialInteractor interface {
	ListComments(ctx context.Context, noteID string) ([]InteractionItem, error)
	ReplyComment(ctx context.Context, noteID, commentID, reply string) error
	ListDMs(ctx context.Context) ([]InteractionItem, error)
	ReplyDM(ctx context.Context, userID, message string) error
}

// ---------- Action 常量 ----------

// InteractAction 互动工具操作类型。
type InteractAction string

const (
	InteractActionListComments InteractAction = "list_comments"
	InteractActionReplyComment InteractAction = "reply_comment"
	InteractActionListDMs      InteractAction = "list_dms"
	InteractActionReplyDM      InteractAction = "reply_dm"
)

// ---------- 速率限制 ----------

const interactMinInterval = 5 * time.Second

var (
	interactLastWrite   time.Time
	interactLastWriteMu sync.Mutex
)

// enforceInteractRateLimit 确保写操作之间间隔 ≥5 秒，防止触发平台反爬。
// 支持 context 取消，避免阻塞整个 goroutine。
func enforceInteractRateLimit(ctx context.Context) error {
	interactLastWriteMu.Lock()
	elapsed := time.Since(interactLastWrite)
	interactLastWriteMu.Unlock()

	if elapsed < interactMinInterval {
		timer := time.NewTimer(interactMinInterval - elapsed)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	interactLastWriteMu.Lock()
	interactLastWrite = time.Now()
	interactLastWriteMu.Unlock()
	return nil
}

// ---------- 工具构造器 ----------

// CreateSocialInteractTool 创建社交互动工具。
// interactor 为 nil 时工具仍可构造，但执行时返回错误。
func CreateSocialInteractTool(
	interactor SocialInteractor,
) *MediaTool {
	return &MediaTool{
		ToolName:  ToolSocialInteract,
		ToolLabel: "Social Interact",
		ToolDesc: "Manage social interactions on Xiaohongshu. " +
			"List and reply to comments and direct messages.",
		ToolParams: socialInteractSchema(),
		ToolExecute: func(
			ctx context.Context,
			toolCallID string,
			args map[string]any,
		) (*MediaToolResult, error) {
			action, err := readStringArg(args, "action", true)
			if err != nil {
				return nil, err
			}

			switch InteractAction(action) {
			case InteractActionListComments:
				return executeListComments(ctx, interactor, args)
			case InteractActionReplyComment:
				return executeReplyComment(ctx, interactor, args)
			case InteractActionListDMs:
				return executeListDMs(ctx, interactor)
			case InteractActionReplyDM:
				return executeReplyDM(ctx, interactor, args)
			default:
				return nil, fmt.Errorf("unknown interact action: %s", action)
			}
		},
	}
}

// socialInteractSchema 返回 social_interact 工具的 JSON Schema。
func socialInteractSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []any{
					"list_comments", "reply_comment",
					"list_dms", "reply_dm",
				},
				"description": "The interaction action to perform",
			},
			"note_id": map[string]any{
				"type":        "string",
				"description": "Note ID for comment operations",
			},
			"comment_id": map[string]any{
				"type":        "string",
				"description": "Comment ID to reply to",
			},
			"user_id": map[string]any{
				"type":        "string",
				"description": "User ID for DM operations",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Reply message content",
			},
		},
		"required": []any{"action"},
	}
}

// ---------- Action 实现 ----------

// executeListComments 列出笔记评论。
func executeListComments(
	ctx context.Context,
	interactor SocialInteractor,
	args map[string]any,
) (*MediaToolResult, error) {
	if interactor == nil {
		return nil, fmt.Errorf("social interactor not configured")
	}

	noteID, err := readStringArg(args, "note_id", true)
	if err != nil {
		return nil, err
	}

	comments, err := interactor.ListComments(ctx, noteID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}

	return jsonMediaResult(map[string]any{
		"note_id": noteID,
		"count":   len(comments),
		"items":   comments,
	}), nil
}

// executeReplyComment 回复指定评论。
func executeReplyComment(
	ctx context.Context,
	interactor SocialInteractor,
	args map[string]any,
) (*MediaToolResult, error) {
	if interactor == nil {
		return nil, fmt.Errorf("social interactor not configured")
	}

	noteID, err := readStringArg(args, "note_id", true)
	if err != nil {
		return nil, err
	}
	commentID, err := readStringArg(args, "comment_id", true)
	if err != nil {
		return nil, err
	}
	message, err := readStringArg(args, "message", true)
	if err != nil {
		return nil, err
	}

	if err := enforceInteractRateLimit(ctx); err != nil {
		return nil, err
	}
	if err := interactor.ReplyComment(ctx, noteID, commentID, message); err != nil {
		return nil, fmt.Errorf("reply comment: %w", err)
	}

	return jsonMediaResult(map[string]any{
		"status":     "replied",
		"note_id":    noteID,
		"comment_id": commentID,
	}), nil
}

// executeListDMs 列出私信。
func executeListDMs(
	ctx context.Context,
	interactor SocialInteractor,
) (*MediaToolResult, error) {
	if interactor == nil {
		return nil, fmt.Errorf("social interactor not configured")
	}

	dms, err := interactor.ListDMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list DMs: %w", err)
	}

	return jsonMediaResult(map[string]any{
		"count": len(dms),
		"items": dms,
	}), nil
}

// executeReplyDM 回复私信。
func executeReplyDM(
	ctx context.Context,
	interactor SocialInteractor,
	args map[string]any,
) (*MediaToolResult, error) {
	if interactor == nil {
		return nil, fmt.Errorf("social interactor not configured")
	}

	userID, err := readStringArg(args, "user_id", true)
	if err != nil {
		return nil, err
	}
	message, err := readStringArg(args, "message", true)
	if err != nil {
		return nil, err
	}

	if err := enforceInteractRateLimit(ctx); err != nil {
		return nil, err
	}
	if err := interactor.ReplyDM(ctx, userID, message); err != nil {
		return nil, fmt.Errorf("reply DM: %w", err)
	}

	return jsonMediaResult(map[string]any{
		"status":  "replied",
		"user_id": userID,
	}), nil
}
