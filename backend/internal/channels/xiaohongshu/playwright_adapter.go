package xiaohongshu

// ============================================================================
// xiaohongshu/playwright_adapter.go — PlaywrightTools → BrowserController 桥接
//
// 将项目已有的 browser.PlaywrightTools 接口适配为
// xiaohongshu.BrowserController（CDPBrowserAdapter 所需的最小接口）。
//
// 注入链: PlaywrightTools → PlaywrightBrowserBridge → CDPBrowserAdapter → XHSRPAClient
//
// Tracking doc: docs/claude/tracking/tracking-media-subagent-upgrade.md §P0-3
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
)

// PlaywrightBrowserBridge 将 browser.PlaywrightTools 桥接为 BrowserController。
// CDPBrowserAdapter 可直接消费此适配器。
type PlaywrightBrowserBridge struct {
	tools  browser.PlaywrightTools
	target browser.PWTargetOpts
}

// NewPlaywrightBrowserBridge 创建桥接适配器。
// cdpURL 为 CDP WebSocket 地址，用于定位页面目标。
func NewPlaywrightBrowserBridge(tools browser.PlaywrightTools, cdpURL string) *PlaywrightBrowserBridge {
	return &PlaywrightBrowserBridge{
		tools: tools,
		target: browser.PWTargetOpts{
			CDPURL: cdpURL,
		},
	}
}

// NavigateToURL 导航到指定 URL。
func (b *PlaywrightBrowserBridge) NavigateToURL(ctx context.Context, url string) error {
	return b.tools.Navigate(ctx, browser.PWNavigateOpts{
		PWTargetOpts: b.target,
		URL:          url,
		WaitUntil:    "domcontentloaded",
		TimeoutMs:    30000,
	})
}

// ExecuteJS 执行 JS 表达式并返回字符串结果。
func (b *PlaywrightBrowserBridge) ExecuteJS(ctx context.Context, expr string) (string, error) {
	raw, err := b.tools.Evaluate(ctx, browser.PWEvaluateOpts{
		PWTargetOpts: b.target,
		Expression:   expr,
	})
	if err != nil {
		return "", err
	}
	// json.RawMessage → string：尝试解引号，否则原样返回
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	return string(raw), nil
}

// ClickElement 通过 CSS 选择器点击元素。
func (b *PlaywrightBrowserBridge) ClickElement(ctx context.Context, selector string) error {
	return b.tools.Click(ctx, browser.PWClickOpts{
		PWTargetOpts: b.target,
		Ref:          selector,
		TimeoutMs:    10000,
	})
}

// FillInput 通过 CSS 选择器填写输入框。
func (b *PlaywrightBrowserBridge) FillInput(ctx context.Context, selector, value string) error {
	return b.tools.Fill(ctx, browser.PWFillOpts{
		PWTargetOpts: b.target,
		Ref:          selector,
		Value:        value,
		TimeoutMs:    10000,
	})
}

// WaitForSelector 等待元素出现。
func (b *PlaywrightBrowserBridge) WaitForSelector(ctx context.Context, selector string, timeoutMs int) error {
	if timeoutMs <= 0 {
		timeoutMs = 10000
	}
	return b.tools.WaitFor(ctx, browser.PWWaitForOpts{
		PWTargetOpts: b.target,
		Text:         selector,
		TimeoutMs:    timeoutMs,
	})
}

// UploadFileToInput 通过 file input 选择器上传文件。
func (b *PlaywrightBrowserBridge) UploadFileToInput(ctx context.Context, selector, filePath string) error {
	return b.tools.SetInputFiles(ctx, browser.PWSetInputFilesOpts{
		PWTargetOpts: b.target,
		Element:      selector,
		Paths:        []string{filePath},
	})
}

// GetPageContent 获取页面文本内容。
func (b *PlaywrightBrowserBridge) GetPageContent(ctx context.Context) (string, error) {
	raw, err := b.tools.Evaluate(ctx, browser.PWEvaluateOpts{
		PWTargetOpts: b.target,
		Expression:   "document.body.innerText",
	})
	if err != nil {
		return "", fmt.Errorf("get page content: %w", err)
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	return string(raw), nil
}

// TakeScreenshot 截取页面截图。
func (b *PlaywrightBrowserBridge) TakeScreenshot(ctx context.Context) ([]byte, error) {
	return b.tools.Screenshot(ctx, b.target)
}

// SetCookies 批量设置 Cookie。
func (b *PlaywrightBrowserBridge) SetCookies(ctx context.Context, cookies []map[string]interface{}) error {
	for _, c := range cookies {
		name, _ := c["name"].(string)
		value, _ := c["value"].(string)
		domain, _ := c["domain"].(string)
		path, _ := c["path"].(string)
		if name == "" {
			continue
		}

		opts := browser.PWCookieSetOpts{
			PWTargetOpts: b.target,
			Name:         name,
			Value:        value,
			Domain:       domain,
			Path:         path,
		}
		if expires, ok := c["expires"].(float64); ok {
			opts.Expires = expires
		} else if expiresInt, ok := c["expires"].(int64); ok {
			opts.Expires = float64(expiresInt)
		}

		if err := b.tools.CookiesSet(ctx, opts); err != nil {
			return fmt.Errorf("set cookie %s: %w", name, err)
		}
	}
	return nil
}

// Verify interface compliance at compile time.
var _ BrowserController = (*PlaywrightBrowserBridge)(nil)

// ---------- 辅助 ----------

// SetBrowserFromPlaywright 便捷方法：创建完整注入链并调用 SetBrowser。
// 注入链: PlaywrightTools → PlaywrightBrowserBridge → CDPBrowserAdapter → XHSRPAClient
func (c *XHSRPAClient) SetBrowserFromPlaywright(tools browser.PlaywrightTools, cdpURL, errDir string) {
	resolvedURL := cdpURL
	if normalized, err := normalizeCDPWebSocketURL(cdpURL); err == nil {
		resolvedURL = normalized
	}
	c.mu.Lock()
	c.pwTools = tools
	c.cdpURL = resolvedURL
	c.mu.Unlock()

	bridge := NewPlaywrightBrowserBridge(tools, resolvedURL)
	adapter := NewCDPBrowserAdapter(bridge, errDir)
	c.SetBrowser(adapter)
}

// LoadCookiesIfAvailable 尝试加载 Cookie，如果文件不存在则跳过。
func (c *XHSRPAClient) LoadCookiesIfAvailable() error {
	if c.cfg == nil || c.cfg.CookiePath == "" {
		return nil
	}
	if _, err := os.Stat(c.cfg.CookiePath); os.IsNotExist(err) {
		return nil
	}
	return c.LoadCookies()
}
