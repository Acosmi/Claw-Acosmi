package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBaiduTrendingSource_Name(t *testing.T) {
	src := NewBaiduTrendingSource()
	if src.Name() != "baidu" {
		t.Errorf("Name() = %q, want %q", src.Name(), "baidu")
	}
}

func TestBaiduTrendingSource_Fetch(t *testing.T) {
	mockResp := baiduAPIResponse{Success: 1}
	mockResp.Data.Cards = []baiduCard{
		{Content: []baiduContentItem{
			{Word: "百度热搜1", HotScore: "5678901", URL: "https://www.baidu.com/s?wd=test1", Desc: "描述1"},
			{Word: "百度热搜2", HotScore: "1234567", URL: "https://www.baidu.com/s?wd=test2", Desc: "描述2"},
			{Word: "", HotScore: "100", URL: "", Desc: ""},
		}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockResp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	src := NewBaiduTrendingSource()
	src.client = srv.Client()
	src.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	topics, err := src.Fetch(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if len(topics) != 2 {
		t.Fatalf("Fetch() returned %d topics, want 2", len(topics))
	}

	if topics[0].Title != "百度热搜1" {
		t.Errorf("topics[0].Title = %q, want %q", topics[0].Title, "百度热搜1")
	}
	if topics[0].Source != "baidu" {
		t.Errorf("topics[0].Source = %q, want %q", topics[0].Source, "baidu")
	}
	if topics[0].HeatScore != 5678901 {
		t.Errorf("topics[0].HeatScore = %f, want 5678901", topics[0].HeatScore)
	}
	if topics[0].URL != "https://www.baidu.com/s?wd=test1" {
		t.Errorf("topics[0].URL = %q", topics[0].URL)
	}
}

func TestBaiduTrendingSource_FetchWithLimit(t *testing.T) {
	mockResp := baiduAPIResponse{Success: 1}
	mockResp.Data.Cards = []baiduCard{
		{Content: []baiduContentItem{
			{Word: "A", HotScore: "300"},
			{Word: "B", HotScore: "200"},
			{Word: "C", HotScore: "100"},
		}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockResp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	src := NewBaiduTrendingSource()
	src.client = srv.Client()
	src.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	topics, err := src.Fetch(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("Fetch() with limit=1 returned %d, want 1", len(topics))
	}
}

func TestBaiduTrendingSource_CategoryMapping(t *testing.T) {
	var receivedTab string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTab = r.URL.Query().Get("tab")
		w.Header().Set("Content-Type", "application/json")
		resp := baiduAPIResponse{Success: 1}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	src := NewBaiduTrendingSource()
	src.client = srv.Client()
	src.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	_, err := src.Fetch(context.Background(), "tech", 0)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if receivedTab != "science" {
		t.Errorf("category tech → tab %q, want %q", receivedTab, "science")
	}
}

func TestParseBaiduHotScore(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1234567", 1234567},
		{"0", 0},
		{"", 0},
		{"12abc34", 1234},
		{"5.2万", 52000},
		{"100万", 1000000},
		{"1.5万热度", 15000},
	}
	for _, tt := range tests {
		got := parseBaiduHotScore(tt.input)
		if got != tt.want {
			t.Errorf("parseBaiduHotScore(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
