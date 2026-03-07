package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mockBrowserController implements browser.BrowserController for testing.
type mockBrowserController struct {
	navigateURL    string
	navigateErr    error
	content        string
	contentErr     error
	clickSelector  string
	clickErr       error
	clickRefRef    string
	clickRefErr    error
	fillRefRef     string
	fillRefText    string
	fillRefErr     error
	typeSelector   string
	typeText       string
	typeErr        error
	screenshotData []byte
	screenshotMime string
	screenshotErr  error
	evalResult     any
	evalErr        error
	waitErr        error
	backErr        error
	forwardErr     error
	url            string
	urlErr         error
	snapshot       map[string]any
	snapshotErr    error
	aiBrowseResult string
	aiBrowseErr    error
	tabs           []browser.TabInfo
	tabsErr        error
	createdTab     *browser.TabInfo
	createTabErr   error
	closeTabErr      error
	switchTabErr     error
	somScreenshot    []byte
	somMime          string
	somAnnotations   []browser.SOMAnnotation
	somErr           error
}

func (m *mockBrowserController) Navigate(_ context.Context, url string) error {
	m.navigateURL = url
	return m.navigateErr
}
func (m *mockBrowserController) GetContent(_ context.Context) (string, error) {
	return m.content, m.contentErr
}
func (m *mockBrowserController) Click(_ context.Context, sel string) error {
	m.clickSelector = sel
	return m.clickErr
}
func (m *mockBrowserController) Type(_ context.Context, sel, text string) error {
	m.typeSelector = sel
	m.typeText = text
	return m.typeErr
}
func (m *mockBrowserController) Screenshot(_ context.Context) ([]byte, string, error) {
	return m.screenshotData, m.screenshotMime, m.screenshotErr
}
func (m *mockBrowserController) Evaluate(_ context.Context, _ string) (any, error) {
	return m.evalResult, m.evalErr
}
func (m *mockBrowserController) WaitForSelector(_ context.Context, _ string) error {
	return m.waitErr
}
func (m *mockBrowserController) GoBack(_ context.Context) error       { return m.backErr }
func (m *mockBrowserController) GoForward(_ context.Context) error    { return m.forwardErr }
func (m *mockBrowserController) GetURL(_ context.Context) (string, error) {
	return m.url, m.urlErr
}
func (m *mockBrowserController) SnapshotAI(_ context.Context) (map[string]any, error) {
	return m.snapshot, m.snapshotErr
}
func (m *mockBrowserController) ClickRef(_ context.Context, ref string) error {
	m.clickRefRef = ref
	return m.clickRefErr
}
func (m *mockBrowserController) FillRef(_ context.Context, ref, text string) error {
	m.fillRefRef = ref
	m.fillRefText = text
	return m.fillRefErr
}
func (m *mockBrowserController) AIBrowse(_ context.Context, _ string) (string, error) {
	return m.aiBrowseResult, m.aiBrowseErr
}
func (m *mockBrowserController) ListTabs(_ context.Context) ([]browser.TabInfo, error) {
	return m.tabs, m.tabsErr
}
func (m *mockBrowserController) CreateTab(_ context.Context, _ string) (*browser.TabInfo, error) {
	return m.createdTab, m.createTabErr
}
func (m *mockBrowserController) CloseTab(_ context.Context, _ string) error {
	return m.closeTabErr
}
func (m *mockBrowserController) SwitchTab(_ context.Context, _ string) error {
	return m.switchTabErr
}
func (m *mockBrowserController) AnnotateSOM(_ context.Context) ([]byte, string, []browser.SOMAnnotation, error) {
	return m.somScreenshot, m.somMime, m.somAnnotations, m.somErr
}
func (m *mockBrowserController) StartGIFRecording()            {}
func (m *mockBrowserController) StopGIFRecording() ([]byte, int, error) { return nil, 0, nil }
func (m *mockBrowserController) IsGIFRecording() bool          { return false }

func TestNewServer(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.Inner() == nil {
		t.Fatal("Inner() returned nil")
	}
}

func TestToolNavigate(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolNavigate(context.Background(), &sdkmcp.CallToolRequest{}, NavigateInput{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if mock.navigateURL != "https://example.com" {
		t.Fatalf("expected URL https://example.com, got %s", mock.navigateURL)
	}
	text := extractText(t, result)
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

func TestToolNavigateError(t *testing.T) {
	mock := &mockBrowserController{navigateErr: context.DeadlineExceeded}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolNavigate(context.Background(), &sdkmcp.CallToolRequest{}, NavigateInput{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
}

func TestToolSnapshot(t *testing.T) {
	mock := &mockBrowserController{
		snapshot: map[string]any{"role": "WebArea", "children": []any{}},
	}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolSnapshot(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	text := extractText(t, result)
	if text == "" {
		t.Fatal("expected non-empty snapshot")
	}
}

func TestToolClickRef(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolClick(context.Background(), &sdkmcp.CallToolRequest{}, ClickInput{Ref: "e1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if mock.clickRefRef != "e1" {
		t.Fatalf("expected ref e1, got %s", mock.clickRefRef)
	}
}

func TestToolClickSelector(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolClick(context.Background(), &sdkmcp.CallToolRequest{}, ClickInput{Selector: "#btn"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if mock.clickSelector != "#btn" {
		t.Fatalf("expected selector #btn, got %s", mock.clickSelector)
	}
}

func TestToolClickNoInput(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolClick(context.Background(), &sdkmcp.CallToolRequest{}, ClickInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing ref/selector")
	}
}

func TestToolFill(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolFill(context.Background(), &sdkmcp.CallToolRequest{}, FillInput{Ref: "e2", Value: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if mock.fillRefRef != "e2" || mock.fillRefText != "hello" {
		t.Fatalf("expected ref=e2 text=hello, got ref=%s text=%s", mock.fillRefRef, mock.fillRefText)
	}
}

func TestToolScreenshot(t *testing.T) {
	mock := &mockBrowserController{
		screenshotData: []byte{0x89, 0x50, 0x4E, 0x47},
		screenshotMime: "image/png",
	}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolScreenshot(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
}

func TestToolGetURL(t *testing.T) {
	mock := &mockBrowserController{url: "https://example.com/page"}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolGetURL(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := extractText(t, result)
	if text != "https://example.com/page" {
		t.Fatalf("expected URL, got %s", text)
	}
}

func TestToolGetContent(t *testing.T) {
	mock := &mockBrowserController{content: "page text content"}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolGetContent(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := extractText(t, result)
	if text != "page text content" {
		t.Fatalf("expected content, got %s", text)
	}
}

func TestToolAIBrowse(t *testing.T) {
	mock := &mockBrowserController{aiBrowseResult: `{"status":"done"}`}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolAIBrowse(context.Background(), &sdkmcp.CallToolRequest{}, AIBrowseInput{Goal: "find product"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolListTabs(t *testing.T) {
	mock := &mockBrowserController{
		tabs: []browser.TabInfo{
			{ID: "t1", URL: "https://a.com", Title: "A", Type: "page"},
			{ID: "t2", URL: "https://b.com", Title: "B", Type: "page"},
		},
	}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolListTabs(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := extractText(t, result)
	var tabs []browser.TabInfo
	if err := json.Unmarshal([]byte(text), &tabs); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(tabs))
	}
}

func TestToolCreateTab(t *testing.T) {
	mock := &mockBrowserController{
		createdTab: &browser.TabInfo{ID: "t3", URL: "https://new.com"},
	}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolCreateTab(context.Background(), &sdkmcp.CallToolRequest{}, CreateTabInput{URL: "https://new.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolCloseTab(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolCloseTab(context.Background(), &sdkmcp.CallToolRequest{}, TabIDInput{TargetID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolSwitchTab(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolSwitchTab(context.Background(), &sdkmcp.CallToolRequest{}, TabIDInput{TargetID: "t2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolType(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolType(context.Background(), &sdkmcp.CallToolRequest{}, TypeInput{Selector: "#input", Text: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if mock.typeSelector != "#input" || mock.typeText != "hello" {
		t.Fatalf("expected selector=#input text=hello, got %s %s", mock.typeSelector, mock.typeText)
	}
}

func TestToolEvaluate(t *testing.T) {
	mock := &mockBrowserController{evalResult: "42"}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolEvaluate(context.Background(), &sdkmcp.CallToolRequest{}, EvaluateInput{Script: "1+1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolWait(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolWait(context.Background(), &sdkmcp.CallToolRequest{}, WaitInput{Selector: ".loaded"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolBack(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolBack(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

func TestToolForward(t *testing.T) {
	mock := &mockBrowserController{}
	srv := NewServer(mock, nil)

	result, _, err := srv.toolForward(context.Background(), &sdkmcp.CallToolRequest{}, EmptyInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
}

// extractText returns the text content from the first content block.
func extractText(t *testing.T, result *sdkmcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("no content blocks")
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}
