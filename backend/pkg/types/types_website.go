package types

// 自有网站频道配置类型 — 媒体运营子智能体 Phase 4

// WebsiteConfig 自有网站频道配置。
type WebsiteConfig struct {
	SiteName       string `json:"siteName,omitempty"`
	Enabled        bool   `json:"enabled,omitempty"`
	APIURL         string `json:"apiUrl,omitempty"`
	AuthType       string `json:"authType,omitempty"` // "bearer" | "api_key" | "basic"
	AuthToken      string `json:"authToken,omitempty"`
	ImageUploadURL string `json:"imageUploadUrl,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"` // 默认 30
	MaxRetries     int    `json:"maxRetries,omitempty"`     // 默认 3
}
