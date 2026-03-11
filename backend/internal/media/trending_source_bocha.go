package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type BochaTrendingSourceConfig struct {
	APIKey    string
	BaseURL   string
	Freshness string
}

// BochaTrendingSource 使用官方 Web Search API 做热点发现。
// 它不是厂商原生“热搜榜单”接口，而是基于官方搜索 API 的热点发现源。
type BochaTrendingSource struct {
	apiKey    string
	baseURL   string
	freshness string
	client    *http.Client
}

func NewBochaTrendingSource(cfg BochaTrendingSourceConfig) *BochaTrendingSource {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.bochaai.com"
	}
	freshness := normalizeBochaFreshness(cfg.Freshness)
	return &BochaTrendingSource{
		apiKey:    strings.TrimSpace(cfg.APIKey),
		baseURL:   strings.TrimRight(baseURL, "/"),
		freshness: freshness,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (b *BochaTrendingSource) Name() string { return "bocha" }

type bochaTrendingRequest struct {
	Query     string `json:"query"`
	Freshness string `json:"freshness,omitempty"`
	Summary   bool   `json:"summary"`
	Count     int    `json:"count,omitempty"`
}

type bochaTrendingResponse struct {
	WebPages *struct {
		Value []struct {
			Name          string `json:"name"`
			URL           string `json:"url"`
			Summary       string `json:"summary,omitempty"`
			Snippet       string `json:"snippet,omitempty"`
			DatePublished string `json:"datePublished,omitempty"`
		} `json:"value"`
	} `json:"webPages"`
}

func (b *BochaTrendingSource) Fetch(ctx context.Context, category string, limit int) ([]TrendingTopic, error) {
	if strings.TrimSpace(b.apiKey) == "" {
		return nil, fmt.Errorf("bocha: API key not configured")
	}

	count := limit
	if count <= 0 {
		count = 10
	}
	if count > 50 {
		count = 50
	}

	payload := bochaTrendingRequest{
		Query:     buildBochaTrendingQuery(category),
		Freshness: b.freshness,
		Summary:   true,
		Count:     count,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("bocha: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/web-search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bocha: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bocha: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("bocha: API error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed bochaTrendingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("bocha: decode response: %w", err)
	}
	if parsed.WebPages == nil || len(parsed.WebPages.Value) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(parsed.WebPages.Value))
	topics := make([]TrendingTopic, 0, len(parsed.WebPages.Value))
	for i, item := range parsed.WebPages.Value {
		title := strings.TrimSpace(item.Name)
		if title == "" {
			continue
		}
		key := strings.ToLower(title)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		topics = append(topics, TrendingTopic{
			Title:     title,
			Source:    "bocha",
			URL:       strings.TrimSpace(item.URL),
			HeatScore: bochaRankToHeatScore(i, item.DatePublished, now),
			Category:  normalizeTrendingCategory(category),
			FetchedAt: now,
		})
	}

	if limit > 0 && len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, nil
}

func normalizeBochaFreshness(value string) string {
	switch strings.TrimSpace(value) {
	case "", "oneDay":
		return "oneDay"
	case "noLimit", "oneWeek", "oneMonth", "oneYear":
		return strings.TrimSpace(value)
	default:
		return "oneDay"
	}
}

func buildBochaTrendingQuery(category string) string {
	switch strings.TrimSpace(strings.ToLower(category)) {
	case "tech", "science":
		return "今日 科技 AI 热点 新闻 话题"
	case "finance":
		return "今日 财经 热点 新闻 话题"
	case "entertainment":
		return "今日 娱乐 热点 新闻 话题"
	case "sports":
		return "今日 体育 热点 新闻 话题"
	case "game":
		return "今日 游戏 热点 新闻 话题"
	case "car":
		return "今日 汽车 热点 新闻 话题"
	default:
		return "今日 热点 新闻 热搜 话题"
	}
}

func normalizeTrendingCategory(category string) string {
	if strings.TrimSpace(category) == "" {
		return "general"
	}
	return strings.TrimSpace(category)
}

func bochaRankToHeatScore(index int, publishedAt string, now time.Time) float64 {
	score := float64(100000 - index*1000)
	if score < 1000 {
		score = 1000
	}
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(publishedAt)); err == nil {
		age := now.Sub(ts)
		switch {
		case age <= 6*time.Hour:
			score += 2000
		case age <= 24*time.Hour:
			score += 1000
		}
	}
	return score
}
