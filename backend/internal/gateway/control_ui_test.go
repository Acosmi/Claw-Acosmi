package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewControlUIHandler_ServesRootAndRouteFallback(t *testing.T) {
	handler, ok := newControlUIHandler("", fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<html>index</html>")},
		"assets/app.js":    &fstest.MapFile{Data: []byte("console.log('ok')")},
		"assets/style.css": &fstest.MapFile{Data: []byte("body{}")},
	}, "")
	if !ok {
		t.Fatal("expected control UI handler to be created")
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "root", path: "/ui/", want: "<html>index</html>"},
		{name: "route fallback", path: "/ui/chat", want: "<html>index</html>"},
		{name: "nested route fallback", path: "/ui/overview/settings", want: "<html>index</html>"},
		{name: "asset", path: "/ui/assets/app.js", want: "console.log('ok')"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}
			if body := rec.Body.String(); body != tc.want {
				t.Fatalf("expected body %q, got %q", tc.want, body)
			}
		})
	}
}

func TestNewControlUIHandler_MissingAssetWithExtensionReturns404(t *testing.T) {
	handler, ok := newControlUIHandler("", fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
	}, "")
	if !ok {
		t.Fatal("expected control UI handler to be created")
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/assets/missing.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestNewControlUIHandler_CustomIndexAndPathCleaning(t *testing.T) {
	handler, ok := newControlUIHandler("", fstest.MapFS{
		"app.html":          &fstest.MapFile{Data: []byte("<html>app</html>")},
		"reports.csv":       &fstest.MapFile{Data: []byte("a,b")},
		"nested/page":       &fstest.MapFile{Data: []byte("plain")},
		"nested/index.html": &fstest.MapFile{Data: []byte("<html>nested</html>")},
	}, "app.html")
	if !ok {
		t.Fatal("expected control UI handler to be created")
	}

	req := httptest.NewRequest(http.MethodGet, "/ui/../ui/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("expected custom index body, got %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/ui/reports.csv", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "a,b") {
		t.Fatalf("expected file body, got %q", rec.Body.String())
	}
}
