package xiaohongshu

// ============================================================================
// xiaohongshu/rpa_client.go — 小红书 RPA 浏览器自动化客户端
// 基于 Cookie 登录机制 + 操作频率控制的 RPA 客户端框架。
// 具体的浏览器操作（Rod/Playwright）在集成阶段实现。
//
// Design doc: docs/xinshenji/impl-tracking-media-subagent.md §P3-1
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/media"
)

// ErrNotImplemented 表示 RPA 浏览器操作尚未集成。
var ErrNotImplemented = fmt.Errorf(
	"xiaohongshu RPA: browser automation not yet integrated")

// ---------- Cookie 管理 ----------

// CookieEntry 持久化的 Cookie 条目。
type CookieEntry struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Domain  string `json:"domain"`
	Path    string `json:"path,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

// ---------- RPA 客户端 ----------

// XHSRPAClient 小红书 RPA 客户端。
type XHSRPAClient struct {
	mu  sync.Mutex
	cfg *XiaohongshuConfig

	cookies    []CookieEntry
	lastAction time.Time
	errShotDir string
	browser    BrowserDriver // Phase 6A: 注入的浏览器驱动
}

// NewXHSRPAClient 创建 RPA 客户端。
func NewXHSRPAClient(cfg *XiaohongshuConfig) *XHSRPAClient {
	errDir := cfg.ErrorScreenshotDir
	if errDir == "" {
		errDir = "_media/xhs/errors"
	}
	return &XHSRPAClient{
		cfg:        cfg,
		errShotDir: errDir,
	}
}

// SetBrowser 注入浏览器驱动。
func (c *XHSRPAClient) SetBrowser(b BrowserDriver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.browser = b
}

// LoadCookies 从文件加载持久化 Cookie。
func (c *XHSRPAClient) LoadCookies() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.cfg.CookiePath)
	if err != nil {
		return fmt.Errorf("load cookies: %w", err)
	}

	var cookies []CookieEntry
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("parse cookies: %w", err)
	}

	c.cookies = cookies
	slog.Info("xiaohongshu cookies loaded", "count", len(cookies))
	return nil
}

// CheckCookieValid 检查 Cookie 是否有效（未过期）。
func (c *XHSRPAClient) CheckCookieValid() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.cookies) == 0 {
		return false
	}

	now := time.Now().Unix()
	for _, cookie := range c.cookies {
		if cookie.Expires > 0 && cookie.Expires < now {
			slog.Warn("xiaohongshu cookie expired",
				"name", cookie.Name)
			return false
		}
	}
	return true
}

// ---------- 发布笔记 ----------

// PublishNote 通过 RPA 发布小红书笔记。
// 实现 media.MediaPublisher 接口。
func (c *XHSRPAClient) Publish(
	ctx context.Context,
	draft *media.ContentDraft,
) (*media.PublishResult, error) {
	if draft == nil {
		return nil, fmt.Errorf("draft is nil")
	}

	c.mu.Lock()
	browser := c.browser
	cookies := make([]CookieEntry, len(c.cookies))
	copy(cookies, c.cookies)
	c.mu.Unlock()

	if browser == nil {
		return nil, fmt.Errorf("xiaohongshu RPA: browser driver not configured, call SetBrowser() first")
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("xiaohongshu RPA: no cookies loaded, call LoadCookies() first")
	}

	// 频率控制。
	c.rateLimit()

	slog.Info("xiaohongshu RPA publish requested",
		"title", draft.Title,
		"tags", len(draft.Tags),
		"images", len(draft.Images))

	// Step 1: 导航到创作者中心发布页
	const publishURL = "https://creator.xiaohongshu.com/publish/publish"
	if err := browser.Navigate(ctx, publishURL); err != nil {
		return nil, fmt.Errorf("navigate to publish page: %w", err)
	}

	// Step 2: 注入 Cookies 并刷新页面
	if err := browser.SetCookies(ctx, cookies); err != nil {
		return nil, fmt.Errorf("set cookies: %w", err)
	}
	if err := browser.Navigate(ctx, publishURL); err != nil {
		return nil, fmt.Errorf("refresh after cookie injection: %w", err)
	}

	// Step 3: 等待编辑区加载
	// ⚠️ CSS 选择器为最佳近似值，生产环境需实际验证
	if err := browser.WaitForElement(ctx, ".ql-editor, [contenteditable='true']", 10000); err != nil {
		return nil, fmt.Errorf("wait for editor: %w", err)
	}

	// Step 4: 上传封面图（如果有图片）
	if len(draft.Images) > 0 {
		for _, imgPath := range draft.Images {
			if err := browser.UploadFile(ctx, "input[type='file']", imgPath); err != nil {
				slog.Warn("xhs publish: upload image failed", "path", imgPath, "error", err)
			}
		}
		// 等待上传处理
		if err := browser.WaitForElement(ctx, ".upload-success, .image-item", 15000); err != nil {
			slog.Warn("xhs publish: image upload confirmation timeout", "error", err)
		}
	}

	// Step 5: 填写标题
	titleSelectors := []string{"#title", "[placeholder*='标题']", ".title-input"}
	for _, sel := range titleSelectors {
		if err := browser.FillBySelector(ctx, sel, draft.Title); err == nil {
			break
		}
	}

	// Step 6: 填写正文
	bodySelectors := []string{".ql-editor", "[contenteditable='true']"}
	for _, sel := range bodySelectors {
		if err := browser.FillBySelector(ctx, sel, draft.Body); err == nil {
			break
		}
	}

	// Step 7: 添加标签
	for _, tag := range draft.Tags {
		// 输入 # + 标签名触发标签联想
		tagInput := "#" + tag
		_ = browser.FillBySelector(ctx, ".tag-input, .hashtag-input", tagInput)
		_ = browser.ClickBySelector(ctx, ".tag-suggestion:first-child, .hashtag-item:first-child")
	}

	// Step 8: 点击发布按钮
	publishSelectors := []string{".publish-btn", "button[class*='publish']", ".btn-publish"}
	var publishErr error
	for _, sel := range publishSelectors {
		if err := browser.ClickBySelector(ctx, sel); err == nil {
			publishErr = nil
			break
		} else {
			publishErr = err
		}
	}
	if publishErr != nil {
		return nil, fmt.Errorf("click publish button: %w", publishErr)
	}

	// Step 9: 等待发布成功
	if err := browser.WaitForElement(ctx, ".publish-success, .success-page", 20000); err != nil {
		slog.Warn("xhs publish: success confirmation timeout", "error", err)
		// 不直接返回错误 — 可能已发布但 DOM 选择器不精确
	}

	// Step 10: 提取发布结果
	noteURL := c.extractPublishedURL(ctx, browser)
	noteID := c.extractNoteID(noteURL)

	result := &media.PublishResult{
		Platform:    media.PlatformXiaohongshu,
		PostID:      noteID,
		URL:         noteURL,
		Status:      "published",
		PublishedAt: time.Now().UTC(),
	}

	slog.Info("xiaohongshu RPA publish completed",
		"note_id", noteID,
		"url", noteURL)

	return result, nil
}

// extractPublishedURL 从页面提取发布后的 URL。
func (c *XHSRPAClient) extractPublishedURL(ctx context.Context, browser BrowserDriver) string {
	result, err := browser.EvaluateJS(ctx, `
		(() => {
			const link = document.querySelector('.publish-success a, .note-link, a[href*="/explore/"]');
			return link ? link.href : window.location.href;
		})()
	`)
	if err != nil {
		slog.Warn("xhs: extract published URL failed", "error", err)
		return ""
	}
	return result
}

// extractNoteID 从 URL 提取笔记 ID。
// URL 格式: https://www.xiaohongshu.com/explore/<noteID> 或带尾部斜杠。
func (c *XHSRPAClient) extractNoteID(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	// 去除尾部斜杠和查询参数
	s := rawURL
	if idx := strings.IndexByte(s, '?'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimRight(s, "/")
	// 取最后一段路径
	if idx := strings.LastIndexByte(s, '/'); idx >= 0 {
		return s[idx+1:]
	}
	return ""
}

// ---------- 频率控制 ----------

// rateLimit 操作间隔控制（≥ 配置秒数 + 随机延迟）。
// 锁仅用于读写 lastAction，sleep 在锁外执行，避免阻塞并发调用。
func (c *XHSRPAClient) rateLimit() {
	c.mu.Lock()
	minInterval := time.Duration(c.cfg.RateLimitSeconds) * time.Second
	if minInterval < 5*time.Second {
		minInterval = 5 * time.Second
	}
	elapsed := time.Since(c.lastAction)
	c.mu.Unlock()

	if elapsed < minInterval {
		time.Sleep(minInterval - elapsed)
	}

	// 随机延迟 0~2 秒，模拟人工操作。
	jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
	time.Sleep(jitter)

	c.mu.Lock()
	c.lastAction = time.Now()
	c.mu.Unlock()
}
