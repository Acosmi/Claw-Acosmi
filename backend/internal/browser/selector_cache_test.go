package browser

import (
	"log/slog"
	"testing"
)

func TestSelectorCacheKey(t *testing.T) {
	key := selectorCacheKey("https://example.com", "e5")
	if key != "https://example.com|e5" {
		t.Errorf("unexpected key: %q", key)
	}
}

func TestSelectorCache_PutGet(t *testing.T) {
	tools := NewCDPPlaywrightTools("ws://fake:1234", slog.Default())
	url := "https://example.com/page"

	// Empty initially.
	if s := tools.getCachedSelector(url, "e1"); s != "" {
		t.Errorf("expected empty, got %q", s)
	}

	// Put and get.
	tools.putCachedSelector(url, "e1", "div > button:nth-child(2)")
	if s := tools.getCachedSelector(url, "e1"); s != "div > button:nth-child(2)" {
		t.Errorf("expected cached selector, got %q", s)
	}

	// Different ref, same URL.
	if s := tools.getCachedSelector(url, "e2"); s != "" {
		t.Errorf("expected empty for different ref, got %q", s)
	}

	// Different URL invalidates.
	if s := tools.getCachedSelector("https://other.com", "e1"); s != "" {
		t.Errorf("expected empty for different URL, got %q", s)
	}
}

func TestSelectorCache_InvalidateOnNavigate(t *testing.T) {
	tools := NewCDPPlaywrightTools("ws://fake:1234", slog.Default())
	url := "https://example.com/page"

	tools.putCachedSelector(url, "e1", "div > button")
	if s := tools.getCachedSelector(url, "e1"); s == "" {
		t.Fatal("should have cached selector")
	}

	// Invalidate.
	tools.invalidateSelectorCache()

	if s := tools.getCachedSelector(url, "e1"); s != "" {
		t.Errorf("expected empty after invalidation, got %q", s)
	}
}

func TestSelectorCache_URLChange(t *testing.T) {
	tools := NewCDPPlaywrightTools("ws://fake:1234", slog.Default())

	tools.putCachedSelector("https://page1.com", "e1", "button#submit")
	tools.putCachedSelector("https://page1.com", "e2", "input#name")

	// Both cached.
	if tools.getCachedSelector("https://page1.com", "e1") == "" {
		t.Fatal("e1 should be cached")
	}
	if tools.getCachedSelector("https://page1.com", "e2") == "" {
		t.Fatal("e2 should be cached")
	}

	// Putting for a new URL clears old cache.
	tools.putCachedSelector("https://page2.com", "e1", "a#link")
	if tools.getCachedSelector("https://page1.com", "e1") != "" {
		t.Error("old URL entries should be cleared")
	}
	if tools.getCachedSelector("https://page2.com", "e1") != "a#link" {
		t.Error("new URL entry should exist")
	}
}
