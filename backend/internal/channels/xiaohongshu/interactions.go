package xiaohongshu

// ============================================================================
// xiaohongshu/interactions.go — 小红书评论/私信互动管理
// 提供评论列表、回复评论、私信管理等 RPA 互动操作。
//
// Design doc: docs/xinshenji/impl-tracking-media-subagent.md §P3-2
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
)

// ---------- 互动管理接口 ----------

// InteractionManager 小红书互动操作管理器接口。
// media/social_interact_tool.go 通过此接口路由操作。
type InteractionManager interface {
	ListComments(ctx context.Context, noteID string) ([]media.InteractionItem, error)
	ReplyComment(ctx context.Context, noteID, commentID, reply string) error
	ListDMs(ctx context.Context) ([]media.InteractionItem, error)
	ReplyDM(ctx context.Context, userID, message string) error
}

// ---------- RPA 互动管理器 ----------

// maxProcessedEntries 是 processed map 的最大容量。
// 超过此值时清理最早一半的记录，防止长期运行内存无限增长。
const maxProcessedEntries = 10000

// RPAInteractionManager 基于 RPA 的小红书互动管理。
type RPAInteractionManager struct {
	mu             sync.Mutex
	client         *XHSRPAClient
	processed      map[string]struct{} // 已处理的评论/私信 ID，用于去重
	processedOrder []string            // 插入顺序，用于 LRU 淘汰
}

// NewRPAInteractionManager 创建互动管理器。
func NewRPAInteractionManager(client *XHSRPAClient) *RPAInteractionManager {
	return &RPAInteractionManager{
		client:    client,
		processed: make(map[string]struct{}),
	}
}

// ListComments 获取笔记评论列表。
func (m *RPAInteractionManager) ListComments(
	ctx context.Context,
	noteID string,
) ([]media.InteractionItem, error) {
	m.client.mu.Lock()
	browser := m.client.browser
	m.client.mu.Unlock()

	if browser == nil {
		return nil, ErrNotImplemented
	}

	m.client.rateLimit()
	slog.Info("xiaohongshu RPA listing comments", "note_id", noteID)

	// 导航到笔记详情页
	noteURL := fmt.Sprintf("https://www.xiaohongshu.com/explore/%s", noteID)
	if err := browser.Navigate(ctx, noteURL); err != nil {
		return nil, fmt.Errorf("navigate to note: %w", err)
	}

	// 等待评论区加载
	if err := browser.WaitForElement(ctx, ".comment-item, .comments-container", 10000); err != nil {
		slog.Warn("xhs: comment section not found", "error", err)
		return nil, nil // 可能无评论
	}

	// JS 提取评论
	result, err := browser.EvaluateJS(ctx, xhsExtractCommentsJS)
	if err != nil {
		return nil, fmt.Errorf("extract comments: %w", err)
	}

	var rawComments []struct {
		ID      string `json:"id"`
		Author  string `json:"author"`
		Content string `json:"content"`
		Time    string `json:"time"`
	}
	if err := json.Unmarshal([]byte(result), &rawComments); err != nil {
		return nil, fmt.Errorf("parse comments: %w", err)
	}

	items := make([]media.InteractionItem, 0, len(rawComments))
	for _, c := range rawComments {
		items = append(items, media.InteractionItem{
			Type:       media.InteractionComment,
			Platform:   media.PlatformXiaohongshu,
			NoteID:     noteID,
			AuthorName: c.Author,
			Content:    c.Content,
			Timestamp:  time.Now().UTC(),
		})
	}

	return items, nil
}

// ReplyComment 回复指定评论。
func (m *RPAInteractionManager) ReplyComment(
	ctx context.Context,
	noteID, commentID, reply string,
) error {
	m.client.mu.Lock()
	browser := m.client.browser
	m.client.mu.Unlock()

	if browser == nil {
		return ErrNotImplemented
	}

	m.client.rateLimit()
	slog.Info("xiaohongshu RPA replying to comment",
		"note_id", noteID,
		"comment_id", commentID)

	// 导航到笔记（如果不在当前页面）
	noteURL := fmt.Sprintf("https://www.xiaohongshu.com/explore/%s", noteID)
	if err := browser.Navigate(ctx, noteURL); err != nil {
		return fmt.Errorf("navigate to note: %w", err)
	}

	// 等待评论区
	if err := browser.WaitForElement(ctx, ".comment-item, .comments-container", 10000); err != nil {
		return fmt.Errorf("wait for comments: %w", err)
	}

	// 定位目标评论的回复按钮并点击
	replyBtnSelector := fmt.Sprintf("[data-id='%s'] .reply-btn, .comment-item:nth-child(1) .reply-btn", commentID)
	if err := browser.ClickBySelector(ctx, replyBtnSelector); err != nil {
		// 降级：点击第一个回复按钮
		_ = browser.ClickBySelector(ctx, ".reply-btn")
	}

	// 输入回复内容
	replyInputSelectors := []string{".reply-input", "textarea[placeholder*='回复']", ".comment-input textarea"}
	for _, sel := range replyInputSelectors {
		if err := browser.FillBySelector(ctx, sel, reply); err == nil {
			break
		}
	}

	// 点击发送
	sendSelectors := []string{".reply-submit", "button[class*='submit']", ".send-btn"}
	for _, sel := range sendSelectors {
		if err := browser.ClickBySelector(ctx, sel); err == nil {
			break
		}
	}

	m.markProcessed(commentID)
	return nil
}

// ListDMs 获取私信列表。
func (m *RPAInteractionManager) ListDMs(
	ctx context.Context,
) ([]media.InteractionItem, error) {
	m.client.mu.Lock()
	browser := m.client.browser
	m.client.mu.Unlock()

	if browser == nil {
		return nil, ErrNotImplemented
	}

	m.client.rateLimit()
	slog.Info("xiaohongshu RPA listing DMs")

	// 导航到消息中心
	if err := browser.Navigate(ctx, "https://www.xiaohongshu.com/message"); err != nil {
		return nil, fmt.Errorf("navigate to messages: %w", err)
	}

	// 等待私信列表
	if err := browser.WaitForElement(ctx, ".message-item, .chat-list", 10000); err != nil {
		slog.Warn("xhs: DM list not found", "error", err)
		return nil, nil
	}

	// JS 提取私信
	result, err := browser.EvaluateJS(ctx, xhsExtractDMsJS)
	if err != nil {
		return nil, fmt.Errorf("extract DMs: %w", err)
	}

	var rawDMs []struct {
		UserID  string `json:"user_id"`
		Author  string `json:"author"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &rawDMs); err != nil {
		return nil, fmt.Errorf("parse DMs: %w", err)
	}

	items := make([]media.InteractionItem, 0, len(rawDMs))
	for _, dm := range rawDMs {
		items = append(items, media.InteractionItem{
			Type:       media.InteractionDM,
			Platform:   media.PlatformXiaohongshu,
			AuthorName: dm.Author,
			Content:    dm.Content,
			Timestamp:  time.Now().UTC(),
		})
	}

	return items, nil
}

// ReplyDM 回复私信。
func (m *RPAInteractionManager) ReplyDM(
	ctx context.Context,
	userID, message string,
) error {
	m.client.mu.Lock()
	browser := m.client.browser
	m.client.mu.Unlock()

	if browser == nil {
		return ErrNotImplemented
	}

	m.client.rateLimit()
	slog.Info("xiaohongshu RPA replying to DM", "user_id", userID)

	// 导航到消息中心
	if err := browser.Navigate(ctx, "https://www.xiaohongshu.com/message"); err != nil {
		return fmt.Errorf("navigate to messages: %w", err)
	}

	// 等待并打开对话
	if err := browser.WaitForElement(ctx, ".message-item, .chat-list", 10000); err != nil {
		return fmt.Errorf("wait for message list: %w", err)
	}

	// 点击对应的私信对话
	chatSelector := fmt.Sprintf("[data-user='%s'], .message-item:first-child", userID)
	if err := browser.ClickBySelector(ctx, chatSelector); err != nil {
		return fmt.Errorf("open chat: %w", err)
	}

	// 输入消息
	inputSelectors := []string{".chat-input textarea", "textarea[placeholder*='消息']", ".message-input"}
	for _, sel := range inputSelectors {
		if err := browser.FillBySelector(ctx, sel, message); err == nil {
			break
		}
	}

	// 发送
	sendSelectors := []string{".send-btn", "button[class*='send']", ".chat-send"}
	for _, sel := range sendSelectors {
		if err := browser.ClickBySelector(ctx, sel); err == nil {
			break
		}
	}

	m.markProcessed("dm:" + userID)
	return nil
}

// ---------- JS 提取常量 ----------

// ⚠️ CSS 选择器为最佳近似值，需通过实际页面验证。
const xhsExtractCommentsJS = `
(() => {
	const items = document.querySelectorAll('.comment-item, [class*="CommentItem"]');
	const result = [];
	items.forEach((item, i) => {
		const author = (item.querySelector('.author-name, .user-name') || {}).textContent || '';
		const content = (item.querySelector('.comment-text, .content') || {}).textContent || '';
		const id = item.getAttribute('data-id') || String(i);
		result.push({id, author: author.trim(), content: content.trim(), time: ''});
	});
	return JSON.stringify(result);
})()
`

const xhsExtractDMsJS = `
(() => {
	const items = document.querySelectorAll('.message-item, [class*="ChatItem"]');
	const result = [];
	items.forEach((item) => {
		const author = (item.querySelector('.user-name, .nickname') || {}).textContent || '';
		const content = (item.querySelector('.last-message, .preview') || {}).textContent || '';
		const userId = item.getAttribute('data-user') || '';
		result.push({user_id: userId, author: author.trim(), content: content.trim()});
	});
	return JSON.stringify(result);
})()
`

// IsProcessed 检查是否已处理（去重）。
func (m *RPAInteractionManager) IsProcessed(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.processed[id]
	return ok
}

// markProcessed 标记项为已处理，超过容量时淘汰最早一半。
func (m *RPAInteractionManager) markProcessed(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processed[id]; exists {
		return
	}

	m.processed[id] = struct{}{}
	m.processedOrder = append(m.processedOrder, id)

	if len(m.processedOrder) > maxProcessedEntries {
		// 淘汰最早一半
		evictCount := len(m.processedOrder) / 2
		for _, old := range m.processedOrder[:evictCount] {
			delete(m.processed, old)
		}
		m.processedOrder = m.processedOrder[evictCount:]
	}
}

// ProcessedCount 返回已处理项数量。
func (m *RPAInteractionManager) ProcessedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.processed)
}
