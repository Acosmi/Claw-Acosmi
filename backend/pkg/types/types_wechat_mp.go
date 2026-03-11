package types

// 微信公众号频道配置类型 — 媒体运营子智能体 Phase 4

// WeChatMPConfig 微信公众号频道配置。
type WeChatMPConfig struct {
	AccountName    string `json:"accountName,omitempty"`
	AccountID      string `json:"accountId,omitempty"`
	Enabled        bool   `json:"enabled,omitempty"`
	AppID          string `json:"appId,omitempty"`
	AppSecret      string `json:"appSecret,omitempty"`
	TokenCachePath string `json:"tokenCachePath,omitempty"`
}
