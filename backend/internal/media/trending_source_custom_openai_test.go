package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomOpenAITrendingSource_Name(t *testing.T) {
	src := NewCustomOpenAITrendingSource(CustomOpenAITrendingSourceConfig{})
	if src.Name() != "custom_openai" {
		t.Fatalf("Name() = %q, want custom_openai", src.Name())
	}
}

func TestCustomOpenAITrendingSource_Fetch(t *testing.T) {
	var gotAuth string
	var gotModel string
	var gotExtras bool
	var gotMessages []map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel, _ = body["model"].(string)
		gotExtras, _ = body["web_search"].(bool)
		rawMessages, _ := body["messages"].([]interface{})
		for _, raw := range rawMessages {
			if m, ok := raw.(map[string]interface{}); ok {
				gotMessages = append(gotMessages, m)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": `{"topics":[{"title":"热点一","url":"https://example.com/1","heat_score":12345,"category":"tech"},{"title":"热点二"}]}`,
					},
				},
			},
		})
	}))
	defer srv.Close()

	src := NewCustomOpenAITrendingSource(CustomOpenAITrendingSourceConfig{
		APIKey:        "sk-demo",
		BaseURL:       srv.URL,
		Model:         "sonar-pro",
		RequestExtras: `{"web_search":true}`,
	})
	topics, err := src.Fetch(context.Background(), "tech", 2)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if gotAuth != "Bearer sk-demo" {
		t.Fatalf("Authorization = %q, want Bearer sk-demo", gotAuth)
	}
	if gotModel != "sonar-pro" {
		t.Fatalf("model = %q, want sonar-pro", gotModel)
	}
	if !gotExtras {
		t.Fatal("expected request extras to be merged")
	}
	if len(gotMessages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(gotMessages))
	}
	if !strings.Contains(gotMessages[1]["content"].(string), "今天是") {
		t.Fatalf("user prompt = %q, want date-aware prompt", gotMessages[1]["content"])
	}
	if len(topics) != 2 {
		t.Fatalf("topics len = %d, want 2", len(topics))
	}
	if topics[0].Source != "custom_openai" {
		t.Fatalf("source = %q, want custom_openai", topics[0].Source)
	}
}

func TestCustomOpenAITrendingSource_FetchRequiresFields(t *testing.T) {
	src := NewCustomOpenAITrendingSource(CustomOpenAITrendingSourceConfig{})
	if _, err := src.Fetch(context.Background(), "", 0); err == nil {
		t.Fatal("expected missing API key error")
	}
}
