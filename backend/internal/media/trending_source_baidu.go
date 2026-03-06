package media

// ============================================================================
// media/trending_source_baidu.go — 百度热搜数据源
// 使用百度热搜公开 API 拉取实时热搜榜。
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// BaiduTrendingSource 百度热搜数据源。
type BaiduTrendingSource struct {
	client *http.Client
}

// NewBaiduTrendingSource 创建百度热搜源。
func NewBaiduTrendingSource() *BaiduTrendingSource {
	return &BaiduTrendingSource{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (b *BaiduTrendingSource) Name() string { return "baidu" }

// baiduAPIResponse 百度热搜 API 响应结构。
type baiduAPIResponse struct {
	Success int `json:"success"`
	Data    struct {
		Cards []baiduCard `json:"cards"`
	} `json:"data"`
}

type baiduCard struct {
	Content []baiduContentItem `json:"content"`
}

type baiduContentItem struct {
	Word     string `json:"word"`
	HotScore string `json:"hotScore"`
	URL      string `json:"url"`
	Desc     string `json:"desc"`
}

const baiduHotSearchURL = "https://top.baidu.com/api/board?platform=wise&tab=realtime"

// baiduCategoryMap 将标准分类名映射到百度 tab 参数。
var baiduCategoryMap = map[string]string{
	"tech":          "science",
	"science":       "science",
	"finance":       "finance",
	"entertainment": "entertainment",
	"sports":        "sport",
	"game":          "game",
	"car":           "car",
}

func (b *BaiduTrendingSource) Fetch(ctx context.Context, category string, limit int) ([]TrendingTopic, error) {
	apiURL := baiduHotSearchURL
	if category != "" {
		if tab, ok := baiduCategoryMap[category]; ok {
			apiURL = "https://top.baidu.com/api/board?platform=wise&tab=" + tab
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("baidu: create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baidu: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("baidu: read body: %w", err)
	}

	var apiResp baiduAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("baidu: parse JSON: %w", err)
	}

	now := time.Now().UTC()
	var topics []TrendingTopic

	for _, card := range apiResp.Data.Cards {
		for _, item := range card.Content {
			if item.Word == "" {
				continue
			}
			score := parseBaiduHotScore(item.HotScore)
			topics = append(topics, TrendingTopic{
				Title:     item.Word,
				Source:    "baidu",
				URL:       item.URL,
				HeatScore: score,
				Category:  category,
				FetchedAt: now,
			})
		}
	}

	if limit > 0 && len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, nil
}

// parseBaiduHotScore 解析百度热度字符串为数值。
// 支持纯数字（"5678901"）和带"万"后缀（"5.2万"）两种格式。
func parseBaiduHotScore(s string) float64 {
	hasSuffix := strings.Contains(s, "万")

	// 提取数字部分（含小数点）
	var buf strings.Builder
	for _, c := range s {
		if (c >= '0' && c <= '9') || c == '.' {
			buf.WriteRune(c)
		}
	}
	numStr := buf.String()
	if numStr == "" {
		return 0
	}

	v, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	if hasSuffix {
		return v * 10000
	}
	return v
}
