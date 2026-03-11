package xiaohongshu

// ============================================================================
// xiaohongshu/plugin.go — 小红书频道插件
// 实现 channels.Plugin 接口，参照 wechat_mp/plugin.go 模式。
//
// Design doc: docs/xinshenji/impl-tracking-media-subagent.md §P3-5
// ============================================================================

import (
	"log/slog"
	"sync"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
	"github.com/Acosmi/ClawAcosmi/internal/media"
)

// XiaohongshuPlugin 小红书频道插件。
type XiaohongshuPlugin struct {
	mu           sync.Mutex
	clients      map[string]*XHSRPAClient
	interactions map[string]*RPAInteractionManager

	browserResolver BrowserRuntimeResolver
	onBrowserLaunch BrowserLaunchHook
}

// NewXiaohongshuPlugin 创建小红书插件。
func NewXiaohongshuPlugin() *XiaohongshuPlugin {
	return &XiaohongshuPlugin{
		clients:      make(map[string]*XHSRPAClient),
		interactions: make(map[string]*RPAInteractionManager),
	}
}

// ID 返回频道标识。
func (p *XiaohongshuPlugin) ID() channels.ChannelID {
	return media.ChannelXiaohongshu
}

// Start 启动小红书频道。
func (p *XiaohongshuPlugin) Start(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	if _, ok := p.clients[accountID]; ok {
		slog.Info("xiaohongshu account already started",
			"account", accountID)
		return nil
	}

	slog.Warn("xiaohongshu: no client configured for account, "+
		"use ConfigureAccount() before Start()",
		"account", accountID)
	return nil
}

// ConfigureAccount 配置并初始化指定账号。
func (p *XiaohongshuPlugin) ConfigureAccount(
	accountID string,
	cfg *XiaohongshuConfig,
) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	prevClient := p.clients[accountID]
	client := NewXHSRPAClient(cfg)
	if p.browserResolver != nil {
		client.SetBrowserRuntimeResolver(p.browserResolver, p.onBrowserLaunch)
	}
	if prevClient != nil && prevClient.pwTools != nil && prevClient.cdpURL != "" {
		client.SetBrowserFromPlaywright(prevClient.pwTools, prevClient.cdpURL, cfg.ErrorScreenshotDir)
		client.loginSession = cloneLoginSession(prevClient.loginSession)
	}
	p.clients[accountID] = client
	p.interactions[accountID] = NewRPAInteractionManager(client)

	slog.Info("xiaohongshu account configured",
		"account", accountID,
		"cookie_path", cfg.CookiePath)
	return nil
}

// Stop 停止小红书频道。
func (p *XiaohongshuPlugin) Stop(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	delete(p.clients, accountID)
	delete(p.interactions, accountID)

	slog.Info("xiaohongshu channel stopped", "account", accountID)
	return nil
}

// GetClient 获取指定账号的 RPA 客户端。
func (p *XiaohongshuPlugin) GetClient(
	accountID string,
) *XHSRPAClient {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.clients[accountID]
}

// AllClients 返回所有已配置的 RPA 客户端。
// 用于 Gateway 批量注入 BrowserDriver。
func (p *XiaohongshuPlugin) AllClients() []*XHSRPAClient {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]*XHSRPAClient, 0, len(p.clients))
	for _, client := range p.clients {
		result = append(result, client)
	}
	return result
}

// GetInteractionManager 获取互动管理器。
func (p *XiaohongshuPlugin) GetInteractionManager(
	accountID string,
) *RPAInteractionManager {
	p.mu.Lock()
	defer p.mu.Unlock()

	if accountID == "" {
		accountID = channels.DefaultAccountID
	}
	return p.interactions[accountID]
}

// EnsureAccount returns an existing client or configures one lazily.
func (p *XiaohongshuPlugin) EnsureAccount(
	accountID string,
	cfg *XiaohongshuConfig,
) (*XHSRPAClient, error) {
	if accountID == "" {
		accountID = channels.DefaultAccountID
	}

	p.mu.Lock()
	client := p.clients[accountID]
	p.mu.Unlock()
	if client != nil {
		return client, nil
	}
	if err := p.ConfigureAccount(accountID, cfg); err != nil {
		return nil, err
	}
	return p.GetClient(accountID), nil
}

// SetBrowserRuntimeResolver propagates the gateway browser runtime resolver to all current/future clients.
func (p *XiaohongshuPlugin) SetBrowserRuntimeResolver(
	resolver BrowserRuntimeResolver,
	onLaunch BrowserLaunchHook,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.browserResolver = resolver
	p.onBrowserLaunch = onLaunch
	for _, client := range p.clients {
		client.SetBrowserRuntimeResolver(resolver, onLaunch)
	}
}
