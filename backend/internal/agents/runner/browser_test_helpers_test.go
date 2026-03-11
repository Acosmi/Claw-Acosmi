package runner

import (
	"context"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
)

type noopBrowserController struct{}

func (noopBrowserController) Navigate(context.Context, string) error { return nil }

func (noopBrowserController) GetContent(context.Context) (string, error) {
	return "page content", nil
}

func (noopBrowserController) Click(context.Context, string) error { return nil }

func (noopBrowserController) Type(context.Context, string, string) error { return nil }

func (noopBrowserController) Screenshot(context.Context) ([]byte, string, error) {
	return []byte("img"), "image/jpeg", nil
}

func (noopBrowserController) Evaluate(context.Context, string) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (noopBrowserController) WaitForSelector(context.Context, string) error { return nil }

func (noopBrowserController) GoBack(context.Context) error { return nil }

func (noopBrowserController) GoForward(context.Context) error { return nil }

func (noopBrowserController) GetURL(context.Context) (string, error) {
	return "https://example.com", nil
}

func (noopBrowserController) SnapshotAI(context.Context) (map[string]any, error) {
	return map[string]any{
		"snapshot": "tree",
		"refs": map[string]any{
			"e1": "button",
		},
	}, nil
}

func (noopBrowserController) ClickRef(context.Context, string) error { return nil }

func (noopBrowserController) FillRef(context.Context, string, string) error { return nil }

func (noopBrowserController) AIBrowse(context.Context, string) (string, error) {
	return `{"status":"ok"}`, nil
}

func (noopBrowserController) AnnotateSOM(context.Context) ([]byte, string, []browser.SOMAnnotation, error) {
	return []byte("img"), "image/jpeg", []browser.SOMAnnotation{
		{Index: 1, Tag: "button", Role: "button", Text: "OK"},
	}, nil
}

func (noopBrowserController) StartGIFRecording() {}

func (noopBrowserController) StopGIFRecording() ([]byte, int, error) {
	return []byte("gif"), 2, nil
}

func (noopBrowserController) IsGIFRecording() bool { return true }

func (noopBrowserController) ListTabs(context.Context) ([]browser.TabInfo, error) {
	return []browser.TabInfo{{ID: "tab-1", URL: "https://example.com", Title: "Example", Type: "page"}}, nil
}

func (noopBrowserController) CreateTab(context.Context, string) (*browser.TabInfo, error) {
	return &browser.TabInfo{ID: "tab-2", URL: "https://example.com", Title: "Example", Type: "page"}, nil
}

func (noopBrowserController) CloseTab(context.Context, string) error { return nil }

func (noopBrowserController) SwitchTab(context.Context, string) error { return nil }
