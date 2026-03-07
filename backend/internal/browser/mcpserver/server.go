// Package mcpserver — MCP server exposing browser automation tools.
// Registers as a standard MCP tool server for Claude Code, Cursor, VS Code, etc.
// Tool naming aligned with Playwright MCP conventions.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server is the browser MCP server.
type Server struct {
	inner      *sdkmcp.Server
	controller browser.BrowserController
	logger     *slog.Logger
}

// NewServer creates a new browser MCP server with all browser tools registered.
func NewServer(controller browser.BrowserController, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	inner := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "openacosmi-browser",
		Version: "1.0.0",
	}, nil)

	s := &Server{
		inner:      inner,
		controller: controller,
		logger:     logger,
	}

	s.registerTools()
	return s
}

// Inner returns the underlying SDK server for transport integration.
func (s *Server) Inner() *sdkmcp.Server {
	return s.inner
}

// Run runs the MCP server over the given transport.
func (s *Server) Run(ctx context.Context, transport sdkmcp.Transport) error {
	return s.inner.Run(ctx, transport)
}

// ---- Input Types ----

type NavigateInput struct {
	URL string `json:"url" jsonschema_description:"URL to navigate to" jsonschema:"required"`
}

type ClickInput struct {
	Selector string `json:"selector" jsonschema_description:"CSS selector to click"`
	Ref      string `json:"ref" jsonschema_description:"ARIA ref from snapshot (e.g. e1). Preferred over selector."`
}

type FillInput struct {
	Ref   string `json:"ref" jsonschema:"required" jsonschema_description:"ARIA ref from snapshot (e.g. e1)"`
	Value string `json:"value" jsonschema:"required" jsonschema_description:"Text to fill"`
}

type TypeInput struct {
	Selector string `json:"selector" jsonschema:"required" jsonschema_description:"CSS selector"`
	Text     string `json:"text" jsonschema:"required" jsonschema_description:"Text to type"`
}

type EvaluateInput struct {
	Script string `json:"script" jsonschema:"required" jsonschema_description:"JavaScript to execute"`
}

type WaitInput struct {
	Selector string `json:"selector" jsonschema:"required" jsonschema_description:"CSS selector to wait for"`
}

type AIBrowseInput struct {
	Goal string `json:"goal" jsonschema:"required" jsonschema_description:"Natural language goal for multi-step browsing"`
}

type TabIDInput struct {
	TargetID string `json:"target_id" jsonschema:"required" jsonschema_description:"Tab target ID"`
}

type CreateTabInput struct {
	URL string `json:"url" jsonschema_description:"URL for new tab (default: about:blank)"`
}

type EmptyInput struct{}

// ---- Tool Registration ----

func (s *Server) registerTools() {
	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_navigate",
		Description: "Navigate to a URL",
	}, s.toolNavigate)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_snapshot",
		Description: "Get ARIA accessibility tree snapshot with element refs for interaction. Recommended first step.",
	}, s.toolSnapshot)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_click",
		Description: "Click element by ARIA ref (preferred) or CSS selector",
	}, s.toolClick)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_fill",
		Description: "Fill text into element by ARIA ref",
	}, s.toolFill)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_type",
		Description: "Type text into element by CSS selector",
	}, s.toolType)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_screenshot",
		Description: "Capture page screenshot",
	}, s.toolScreenshot)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_evaluate",
		Description: "Execute JavaScript in page context",
	}, s.toolEvaluate)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_wait",
		Description: "Wait for CSS selector to appear (10s timeout)",
	}, s.toolWait)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_back",
		Description: "Navigate back in browser history",
	}, s.toolBack)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_forward",
		Description: "Navigate forward in browser history",
	}, s.toolForward)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_get_url",
		Description: "Get current page URL",
	}, s.toolGetURL)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_get_content",
		Description: "Get page text content",
	}, s.toolGetContent)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_annotate_som",
		Description: "Capture screenshot with numbered visual annotations on all interactive elements (Set-of-Mark). Returns annotated image + element list.",
	}, s.toolAnnotateSOM)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_ai_browse",
		Description: "Multi-step intent-level browsing. Accepts natural language goal, auto-executes observe→plan→act loop.",
	}, s.toolAIBrowse)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_list_tabs",
		Description: "List all browser tabs",
	}, s.toolListTabs)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_create_tab",
		Description: "Create a new browser tab",
	}, s.toolCreateTab)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_close_tab",
		Description: "Close a browser tab by target ID",
	}, s.toolCloseTab)

	sdkmcp.AddTool(s.inner, &sdkmcp.Tool{
		Name:        "browser_switch_tab",
		Description: "Switch to a browser tab by target ID",
	}, s.toolSwitchTab)
}

// ---- Tool Handlers ----

func (s *Server) toolNavigate(ctx context.Context, req *sdkmcp.CallToolRequest, input NavigateInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Debug("browser_navigate", "url", input.URL)
	if err := s.controller.Navigate(ctx, input.URL); err != nil {
		s.logger.Warn("browser_navigate failed", "url", input.URL, "err", err)
		return errorResult("navigate: " + err.Error()), nil, nil
	}
	return textResult(fmt.Sprintf("Navigated to %s", input.URL)), nil, nil
}

func (s *Server) toolSnapshot(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Debug("browser_snapshot")
	snapshot, err := s.controller.SnapshotAI(ctx)
	if err != nil {
		s.logger.Warn("browser_snapshot failed", "err", err)
		return errorResult("snapshot: " + err.Error()), nil, nil
	}
	data, _ := json.MarshalIndent(snapshot, "", "  ")
	return textResult(string(data)), nil, nil
}

func (s *Server) toolClick(ctx context.Context, req *sdkmcp.CallToolRequest, input ClickInput) (*sdkmcp.CallToolResult, any, error) {
	if input.Ref != "" {
		s.logger.Debug("browser_click", "ref", input.Ref)
		if err := s.controller.ClickRef(ctx, input.Ref); err != nil {
			s.logger.Warn("browser_click failed", "ref", input.Ref, "err", err)
			return errorResult("click_ref: " + err.Error()), nil, nil
		}
		return textResult(fmt.Sprintf("Clicked element ref: %s", input.Ref)), nil, nil
	}
	if input.Selector != "" {
		s.logger.Debug("browser_click", "selector", input.Selector)
		if err := s.controller.Click(ctx, input.Selector); err != nil {
			s.logger.Warn("browser_click failed", "selector", input.Selector, "err", err)
			return errorResult("click: " + err.Error()), nil, nil
		}
		return textResult(fmt.Sprintf("Clicked: %s", input.Selector)), nil, nil
	}
	return errorResult("ref or selector is required"), nil, nil
}

func (s *Server) toolFill(ctx context.Context, req *sdkmcp.CallToolRequest, input FillInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Debug("browser_fill", "ref", input.Ref)
	if err := s.controller.FillRef(ctx, input.Ref, input.Value); err != nil {
		s.logger.Warn("browser_fill failed", "ref", input.Ref, "err", err)
		return errorResult("fill: " + err.Error()), nil, nil
	}
	return textResult(fmt.Sprintf("Filled ref %s", input.Ref)), nil, nil
}

func (s *Server) toolType(ctx context.Context, req *sdkmcp.CallToolRequest, input TypeInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.controller.Type(ctx, input.Selector, input.Text); err != nil {
		return errorResult("type: " + err.Error()), nil, nil
	}
	return textResult(fmt.Sprintf("Typed into %s", input.Selector)), nil, nil
}

func (s *Server) toolScreenshot(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Debug("browser_screenshot")
	data, mimeType, err := s.controller.Screenshot(ctx)
	if err != nil {
		s.logger.Warn("browser_screenshot failed", "err", err)
		return errorResult("screenshot: " + err.Error()), nil, nil
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.ImageContent{
				MIMEType: mimeType,
				Data:     data,
			},
		},
	}, nil, nil
}

func (s *Server) toolEvaluate(ctx context.Context, req *sdkmcp.CallToolRequest, input EvaluateInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Debug("browser_evaluate", "scriptLen", len(input.Script))
	result, err := s.controller.Evaluate(ctx, input.Script)
	if err != nil {
		s.logger.Warn("browser_evaluate failed", "err", err)
		return errorResult("evaluate: " + err.Error()), nil, nil
	}
	data, _ := json.Marshal(result)
	return textResult(string(data)), nil, nil
}

func (s *Server) toolWait(ctx context.Context, req *sdkmcp.CallToolRequest, input WaitInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.controller.WaitForSelector(ctx, input.Selector); err != nil {
		return errorResult("wait: " + err.Error()), nil, nil
	}
	return textResult(fmt.Sprintf("Element found: %s", input.Selector)), nil, nil
}

func (s *Server) toolBack(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.controller.GoBack(ctx); err != nil {
		return errorResult("back: " + err.Error()), nil, nil
	}
	return textResult("Navigated back"), nil, nil
}

func (s *Server) toolForward(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.controller.GoForward(ctx); err != nil {
		return errorResult("forward: " + err.Error()), nil, nil
	}
	return textResult("Navigated forward"), nil, nil
}

func (s *Server) toolGetURL(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	url, err := s.controller.GetURL(ctx)
	if err != nil {
		return errorResult("get_url: " + err.Error()), nil, nil
	}
	return textResult(url), nil, nil
}

func (s *Server) toolGetContent(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	content, err := s.controller.GetContent(ctx)
	if err != nil {
		return errorResult("get_content: " + err.Error()), nil, nil
	}
	return textResult(content), nil, nil
}

func (s *Server) toolAnnotateSOM(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Debug("browser_annotate_som")
	screenshot, mimeType, annotations, err := s.controller.AnnotateSOM(ctx)
	if err != nil {
		s.logger.Warn("browser_annotate_som failed", "err", err)
		return errorResult("annotate_som: " + err.Error()), nil, nil
	}

	var annotText string
	annotText = fmt.Sprintf("SOM: %d interactive elements.\n", len(annotations))
	for _, a := range annotations {
		text := a.Text
		if len(text) > 40 {
			text = text[:40] + "..."
		}
		annotText += fmt.Sprintf("[%d] %s (role=%s) %q\n", a.Index, a.Tag, a.Role, text)
	}

	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.ImageContent{
				MIMEType: mimeType,
				Data:     screenshot,
			},
			&sdkmcp.TextContent{Text: annotText},
		},
	}, nil, nil
}

func (s *Server) toolAIBrowse(ctx context.Context, req *sdkmcp.CallToolRequest, input AIBrowseInput) (*sdkmcp.CallToolResult, any, error) {
	s.logger.Info("browser_ai_browse", "goal", input.Goal)
	result, err := s.controller.AIBrowse(ctx, input.Goal)
	if err != nil {
		s.logger.Warn("browser_ai_browse failed", "goal", input.Goal, "err", err)
		return errorResult("ai_browse: " + err.Error()), nil, nil
	}
	return textResult(result), nil, nil
}

func (s *Server) toolListTabs(ctx context.Context, req *sdkmcp.CallToolRequest, input EmptyInput) (*sdkmcp.CallToolResult, any, error) {
	tabs, err := s.controller.ListTabs(ctx)
	if err != nil {
		return errorResult("list_tabs: " + err.Error()), nil, nil
	}
	data, _ := json.MarshalIndent(tabs, "", "  ")
	return textResult(string(data)), nil, nil
}

func (s *Server) toolCreateTab(ctx context.Context, req *sdkmcp.CallToolRequest, input CreateTabInput) (*sdkmcp.CallToolResult, any, error) {
	tab, err := s.controller.CreateTab(ctx, input.URL)
	if err != nil {
		return errorResult("create_tab: " + err.Error()), nil, nil
	}
	data, _ := json.Marshal(tab)
	return textResult(string(data)), nil, nil
}

func (s *Server) toolCloseTab(ctx context.Context, req *sdkmcp.CallToolRequest, input TabIDInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.controller.CloseTab(ctx, input.TargetID); err != nil {
		return errorResult("close_tab: " + err.Error()), nil, nil
	}
	return textResult("Tab closed: " + input.TargetID), nil, nil
}

func (s *Server) toolSwitchTab(ctx context.Context, req *sdkmcp.CallToolRequest, input TabIDInput) (*sdkmcp.CallToolResult, any, error) {
	if err := s.controller.SwitchTab(ctx, input.TargetID); err != nil {
		return errorResult("switch_tab: " + err.Error()), nil, nil
	}
	return textResult("Switched to tab: " + input.TargetID), nil, nil
}

// ---- Helpers ----

func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}

func errorResult(msg string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}
