package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBochaTrendingSource_Name(t *testing.T) {
	src := NewBochaTrendingSource(BochaTrendingSourceConfig{})
	if src.Name() != "bocha" {
		t.Fatalf("Name() = %q, want bocha", src.Name())
	}
}

func TestBochaTrendingSource_Fetch(t *testing.T) {
	var gotAuth string
	var gotQuery string
	var gotFreshness string
	var gotCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req bochaTrendingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotQuery = req.Query
		gotFreshness = req.Freshness
		gotCount = req.Count

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"webPages": map[string]any{
				"value": []map[string]any{
					{
						"name":          "热点一",
						"url":           "https://example.com/1",
						"datePublished": "2026-03-09T08:00:00Z",
					},
					{
						"name": "热点二",
						"url":  "https://example.com/2",
					},
				},
			},
		})
	}))
	defer srv.Close()

	src := NewBochaTrendingSource(BochaTrendingSourceConfig{
		APIKey:    "sk-bocha",
		BaseURL:   srv.URL,
		Freshness: "oneWeek",
	})
	topics, err := src.Fetch(context.Background(), "tech", 2)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if gotAuth != "Bearer sk-bocha" {
		t.Fatalf("Authorization = %q, want Bearer sk-bocha", gotAuth)
	}
	if !strings.Contains(gotQuery, "科技") && !strings.Contains(gotQuery, "AI") {
		t.Fatalf("query = %q, want tech-oriented prompt", gotQuery)
	}
	if gotFreshness != "oneWeek" {
		t.Fatalf("freshness = %q, want oneWeek", gotFreshness)
	}
	if gotCount != 2 {
		t.Fatalf("count = %d, want 2", gotCount)
	}
	if len(topics) != 2 {
		t.Fatalf("topics len = %d, want 2", len(topics))
	}
	if topics[0].Source != "bocha" {
		t.Fatalf("topics[0].Source = %q, want bocha", topics[0].Source)
	}
	if topics[0].Category != "tech" {
		t.Fatalf("topics[0].Category = %q, want tech", topics[0].Category)
	}
	if topics[0].URL != "https://example.com/1" {
		t.Fatalf("topics[0].URL = %q, want https://example.com/1", topics[0].URL)
	}
}

func TestBochaTrendingSource_FetchRequiresAPIKey(t *testing.T) {
	src := NewBochaTrendingSource(BochaTrendingSourceConfig{})
	if _, err := src.Fetch(context.Background(), "", 0); err == nil {
		t.Fatal("expected missing API key error")
	}
}
