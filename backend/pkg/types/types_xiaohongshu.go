package types

// 小红书频道配置类型 — 媒体运营子智能体 Phase 4

// XiaohongshuConfig 小红书频道配置。
type XiaohongshuConfig struct {
	AccountName          string `json:"accountName,omitempty"`
	AccountID            string `json:"accountId,omitempty"`
	Enabled              bool   `json:"enabled,omitempty"`
	CookiePath           string `json:"cookiePath,omitempty"`
	AutoInteractInterval int    `json:"autoInteractInterval,omitempty"` // 分钟，0 = 禁用
	RateLimitSeconds     int    `json:"rateLimitSeconds,omitempty"`     // 默认 5
	ErrorScreenshotDir   string `json:"errorScreenshotDir,omitempty"`
}
