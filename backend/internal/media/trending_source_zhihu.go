package media

// ============================================================================
// media/trending_source_zhihu.go — 知乎热榜数据源
// 使用知乎公开 API 拉取热榜话题。
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// ZhihuTrendingSource 知乎热榜数据源。
type ZhihuTrendingSource struct {
	client *http.Client
}

// NewZhihuTrendingSource 创建知乎热榜源。
func NewZhihuTrendingSource() *ZhihuTrendingSource {
	return &ZhihuTrendingSource{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (z *ZhihuTrendingSource) Name() string { return "zhihu" }

// zhihuAPIResponse 知乎热榜 API 响应结构。
type zhihuAPIResponse struct {
	Data []zhihuHotItem `json:"data"`
}

type zhihuHotItem struct {
	Target struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"target"`
	DetailText string `json:"detail_text"`
}

const zhihuHotListURL = "https://www.zhihu.com/api/v3/feed/topstory/hot-lists/total"

// zhihuHeatRegex 从 detail_text（如 "1234 万热度"）提取数字。
var zhihuHeatRegex = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*万`)

// zhihuNumRegex 纯数字提取（万匹配失败时的 fallback）。
var zhihuNumRegex = regexp.MustCompile(`(\d+(?:\.\d+)?)`)

// Fetch 拉取知乎热榜。
// 注意: 知乎热榜 API (hot-lists/total) 不支持按 category 过滤，
// category 参数被忽略。分类过滤由上层 TrendingAggregator 处理。
func (z *ZhihuTrendingSource) Fetch(ctx context.Context, _ string, limit int) ([]TrendingTopic, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zhihuHotListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("zhihu: create request: %w", err)
	}
	// 知乎要求 User-Agent，否则返回 403
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := z.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zhihu: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhihu: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("zhihu: read body: %w", err)
	}

	var apiResp zhihuAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("zhihu: parse JSON: %w", err)
	}

	now := time.Now().UTC()
	topics := make([]TrendingTopic, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		title := item.Target.Title
		if title == "" {
			continue
		}
		score := parseZhihuHeatScore(item.DetailText)
		topics = append(topics, TrendingTopic{
			Title:     title,
			Source:    "zhihu",
			URL:       item.Target.URL,
			HeatScore: score,
			Category:  "general",
			FetchedAt: now,
		})
	}

	if limit > 0 && len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, nil
}

// parseZhihuHeatScore 从热度描述提取数值。
// 格式: "1234 万热度" → 12340000, "567 万热度" → 5670000
// 无匹配返回 0。
func parseZhihuHeatScore(text string) float64 {
	matches := zhihuHeatRegex.FindStringSubmatch(text)
	if len(matches) < 2 {
		// 尝试纯数字提取
		m := zhihuNumRegex.FindStringSubmatch(text)
		if len(m) >= 2 {
			v, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				return v
			}
		}
		return 0
	}
	v, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	return v * 10000 // 万 → 实际数值
}
