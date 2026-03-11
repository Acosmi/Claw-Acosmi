package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type CustomOpenAITrendingSourceConfig struct {
	APIKey        string
	BaseURL       string
	Model         string
	SystemPrompt  string
	RequestExtras string
}

type CustomOpenAITrendingSource struct {
	apiKey        string
	baseURL       string
	model         string
	systemPrompt  string
	requestExtras string
	client        *http.Client
}

func NewCustomOpenAITrendingSource(cfg CustomOpenAITrendingSourceConfig) *CustomOpenAITrendingSource {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &CustomOpenAITrendingSource{
		apiKey:        strings.TrimSpace(cfg.APIKey),
		baseURL:       strings.TrimRight(baseURL, "/"),
		model:         strings.TrimSpace(cfg.Model),
		systemPrompt:  strings.TrimSpace(cfg.SystemPrompt),
		requestExtras: strings.TrimSpace(cfg.RequestExtras),
		client:        &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *CustomOpenAITrendingSource) Name() string { return "custom_openai" }

type customOpenAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type customOpenAIChatChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type customOpenAIChatResponse struct {
	Choices []customOpenAIChatChoice `json:"choices"`
}

type customOpenAITrendingPayload struct {
	Topics []struct {
		Title     string      `json:"title"`
		URL       string      `json:"url,omitempty"`
		HeatScore interface{} `json:"heat_score,omitempty"`
		Category  string      `json:"category,omitempty"`
	} `json:"topics"`
}

func (c *CustomOpenAITrendingSource) Fetch(ctx context.Context, category string, limit int) ([]TrendingTopic, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("custom_openai: API key not configured")
	}
	if c.model == "" {
		return nil, fmt.Errorf("custom_openai: model not configured")
	}

	count := limit
	if count <= 0 {
		count = 10
	}
	if count > 50 {
		count = 50
	}

	systemPrompt := c.systemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultCustomOpenAITrendingPrompt()
	}

	body := map[string]interface{}{
		"model":  c.model,
		"stream": false,
		"messages": []customOpenAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildCustomOpenAITrendingUserPrompt(category, count)},
		},
		"temperature": 0.2,
	}

	if c.requestExtras != "" {
		var extras map[string]interface{}
		if err := json.Unmarshal([]byte(c.requestExtras), &extras); err != nil {
			return nil, fmt.Errorf("custom_openai: requestExtras must be valid JSON object: %w", err)
		}
		for key, value := range extras {
			body[key] = value
		}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("custom_openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("custom_openai: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom_openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("custom_openai: API error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed customOpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("custom_openai: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("custom_openai: empty choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	payload, err := parseCustomOpenAITrendingPayload(content)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	topics := make([]TrendingTopic, 0, len(payload.Topics))
	for i, item := range payload.Topics {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		heatScore := customOpenAIHeatScore(item.HeatScore, i)
		itemCategory := strings.TrimSpace(item.Category)
		if itemCategory == "" {
			itemCategory = normalizeTrendingCategory(category)
		}
		topics = append(topics, TrendingTopic{
			Title:     title,
			Source:    "custom_openai",
			URL:       strings.TrimSpace(item.URL),
			HeatScore: heatScore,
			Category:  itemCategory,
			FetchedAt: now,
		})
	}

	if limit > 0 && len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, nil
}

func defaultCustomOpenAITrendingPrompt() string {
	return strings.Join([]string{
		"你是媒体运营系统的热点发现器。",
		"如果上游模型具备联网或搜索能力，优先使用实时网络信息；如果没有联网能力，不要假装实时检索。",
		"始终只输出 JSON，不要输出 markdown，不要解释。",
		"JSON 格式必须是 {\"topics\":[{\"title\":\"\",\"url\":\"\",\"heat_score\":12345,\"category\":\"\"}]}.",
		"最多输出用户要求的条数，按热度从高到低排序。",
	}, " ")
}

func buildCustomOpenAITrendingUserPrompt(category string, limit int) string {
	today := time.Now().Format("2006-01-02")
	categoryText := "综合热点"
	if strings.TrimSpace(category) != "" {
		categoryText = strings.TrimSpace(category)
	}
	return fmt.Sprintf(
		"今天是 %s。请输出 %d 条与“%s”最相关的近期热点候选。如果你不能访问实时信息，请返回空数组 topics。只输出 JSON。",
		today,
		limit,
		categoryText,
	)
}

func parseCustomOpenAITrendingPayload(content string) (*customOpenAITrendingPayload, error) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, fmt.Errorf("custom_openai: empty content")
	}
	jsonText := extractJSONObject(raw)
	if jsonText == "" {
		return nil, fmt.Errorf("custom_openai: response did not contain JSON object")
	}
	var payload customOpenAITrendingPayload
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return nil, fmt.Errorf("custom_openai: parse JSON payload: %w", err)
	}
	return &payload, nil
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return ""
}

func customOpenAIHeatScore(value interface{}, index int) float64 {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return v
		}
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	score := float64(90000 - index*1000)
	if score < 1000 {
		return 1000
	}
	return score
}
