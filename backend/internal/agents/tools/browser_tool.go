// tools/browser_tool.go — 浏览器控制工具。
// TS 参考：src/agents/tools/browser-tool.ts (724L) + browser-tool.schema.ts (112L)
package tools

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/Acosmi/ClawAcosmi/internal/browser"
)

// BrowserController is an alias for the canonical interface in browser package.
type BrowserController = browser.BrowserController

// formatSOMAnnotations formats SOM annotations as a text table for the LLM.
func formatSOMAnnotations(annotations []browser.SOMAnnotation) string {
	if len(annotations) == 0 {
		return "(no interactive elements found)"
	}
	var lines []string
	for _, a := range annotations {
		text := a.Text
		if len(text) > 40 {
			text = text[:40] + "..."
		}
		lines = append(lines, fmt.Sprintf("[%d] %s (role=%s) %q", a.Index, a.Tag, a.Role, text))
	}
	return fmt.Sprintf("%s", joinLines(lines))
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

// CreateBrowserTool 创建浏览器工具。
// TS 参考: browser-tool.ts
func CreateBrowserTool(controller BrowserController) *AgentTool {
	return &AgentTool{
		Name:        "browser",
		Label:       "Browser",
		Description: "Control a browser: navigate, observe (ARIA tree), click_ref/fill_ref (by ref), screenshot, ai_browse (intent-level), and more. Recommended workflow: observe → click_ref/fill_ref → screenshot.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []any{
						"navigate", "get_content", "observe", "annotate_som",
						"click", "click_ref", "type", "fill_ref",
						"screenshot", "evaluate", "wait_for",
						"go_back", "go_forward", "get_url",
						"ai_browse",
						"start_gif_recording", "stop_gif_recording",
						"list_tabs", "create_tab", "close_tab", "switch_tab",
					},
					"description": "Browser action. Recommended: observe → click_ref/fill_ref → screenshot. annotate_som: visual screenshot with numbered interactive elements. start/stop_gif_recording: record multi-step actions as animated GIF. Use ai_browse for multi-step intent-level tasks. Tab management: list_tabs/create_tab/close_tab/switch_tab.",
				},
				"url":      map[string]any{"type": "string", "description": "URL to navigate to (for navigate action)"},
				"selector": map[string]any{"type": "string", "description": "CSS selector (for click/type/wait_for actions)"},
				"text":     map[string]any{"type": "string", "description": "Text to type (for type/fill_ref actions)"},
				"script":   map[string]any{"type": "string", "description": "JavaScript to evaluate (for evaluate action)"},
				"ref":      map[string]any{"type": "string", "description": "ARIA element ref from observe (e.g. \"e1\") for click_ref/fill_ref actions"},
				"goal":      map[string]any{"type": "string", "description": "Natural language goal for ai_browse (e.g. \"Search for MacBook Pro on jd.com\")"},
				"target_id": map[string]any{"type": "string", "description": "Tab/target ID for close_tab/switch_tab actions"},
			},
			"required": []any{"action"},
		},
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (*AgentToolResult, error) {
			// softText returns a text-only tool result (soft error or status).
			softText := func(text string) *AgentToolResult {
				return &AgentToolResult{Content: []ContentBlock{{Type: "text", Text: text}}}
			}

			action, err := ReadStringParam(args, "action", &StringParamOptions{Required: true})
			if err != nil {
				return softText(fmt.Sprintf("[Browser error: %s]", err)), nil
			}
			if controller == nil {
				return softText("[Browser tool is not configured. Enable browser in config and ensure CDP is running.]"), nil
			}

			// browserErr returns a soft-error result (consistent with tool_executor.go).
			// Soft errors let the LLM tool loop continue and self-correct.
			browserErr := func(action string, err error) (*AgentToolResult, error) {
				return softText(fmt.Sprintf("[Browser %s error: %s]", action, err)), nil
			}

			switch action {
			case "navigate":
				url, err := ReadStringParam(args, "url", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("navigate", err)
				}
				if err := controller.Navigate(ctx, url); err != nil {
					return browserErr("navigate", err)
				}
				return JsonResult(map[string]any{"status": "navigated", "url": url}), nil

			case "get_content":
				content, err := controller.GetContent(ctx)
				if err != nil {
					return browserErr("get_content", err)
				}
				return JsonResult(map[string]any{"content": truncateString(content, 50000)}), nil

			case "click":
				selector, err := ReadStringParam(args, "selector", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("click", err)
				}
				if err := controller.Click(ctx, selector); err != nil {
					return browserErr("click", err)
				}
				return JsonResult(map[string]any{"status": "clicked", "selector": selector}), nil

			case "type":
				selector, err := ReadStringParam(args, "selector", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("type", err)
				}
				text, err := ReadStringParam(args, "text", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("type", err)
				}
				if err := controller.Type(ctx, selector, text); err != nil {
					return browserErr("type", err)
				}
				return JsonResult(map[string]any{"status": "typed", "selector": selector}), nil

			case "screenshot":
				data, mimeType, err := controller.Screenshot(ctx)
				if err != nil {
					return browserErr("screenshot", err)
				}
				return JsonResult(map[string]any{"status": "captured", "mimeType": mimeType, "size": len(data)}), nil

			case "evaluate":
				script, err := ReadStringParam(args, "script", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("evaluate", err)
				}
				result, err := controller.Evaluate(ctx, script)
				if err != nil {
					return browserErr("evaluate", err)
				}
				return JsonResult(map[string]any{"result": result}), nil

			case "wait_for":
				selector, err := ReadStringParam(args, "selector", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("wait_for", err)
				}
				if err := controller.WaitForSelector(ctx, selector); err != nil {
					return browserErr("wait_for", err)
				}
				return JsonResult(map[string]any{"status": "found", "selector": selector}), nil

			case "go_back":
				if err := controller.GoBack(ctx); err != nil {
					return browserErr("go_back", err)
				}
				return JsonResult(map[string]any{"status": "navigated_back"}), nil

			case "go_forward":
				if err := controller.GoForward(ctx); err != nil {
					return browserErr("go_forward", err)
				}
				return JsonResult(map[string]any{"status": "navigated_forward"}), nil

			case "get_url":
				url, err := controller.GetURL(ctx)
				if err != nil {
					return browserErr("get_url", err)
				}
				return JsonResult(map[string]any{"url": url}), nil

			case "observe":
				snapshot, err := controller.SnapshotAI(ctx)
				if err != nil {
					return browserErr("observe", err)
				}
				return JsonResult(snapshot), nil

			case "annotate_som":
				screenshot, mimeType, annotations, err := controller.AnnotateSOM(ctx)
				if err != nil {
					return browserErr("annotate_som", err)
				}
				return &AgentToolResult{Content: []ContentBlock{
					{Type: "image", MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(screenshot)},
					{Type: "text", Text: fmt.Sprintf("SOM annotations (%d elements):\n%s",
						len(annotations), formatSOMAnnotations(annotations))},
				}}, nil

			case "click_ref":
				ref, err := ReadStringParam(args, "ref", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("click_ref", err)
				}
				if err := controller.ClickRef(ctx, ref); err != nil {
					return browserErr("click_ref", err)
				}
				return JsonResult(map[string]any{"status": "clicked_ref", "ref": ref}), nil

			case "fill_ref":
				ref, err := ReadStringParam(args, "ref", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("fill_ref", err)
				}
				text, err := ReadStringParam(args, "text", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("fill_ref", err)
				}
				if err := controller.FillRef(ctx, ref, text); err != nil {
					return browserErr("fill_ref", err)
				}
				return JsonResult(map[string]any{"status": "filled_ref", "ref": ref}), nil

			case "ai_browse":
				goal, err := ReadStringParam(args, "goal", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("ai_browse", err)
				}
				result, err := controller.AIBrowse(ctx, goal)
				if err != nil {
					return browserErr("ai_browse", err)
				}
				return JsonResult(map[string]any{"status": "completed", "result": result}), nil

			case "start_gif_recording":
				if controller.IsGIFRecording() {
					return softText("[GIF recording already in progress]"), nil
				}
				controller.StartGIFRecording()
				return JsonResult(map[string]any{"status": "recording_started"}), nil

			case "stop_gif_recording":
				if !controller.IsGIFRecording() {
					return softText("[No GIF recording in progress]"), nil
				}
				gifData, frameCount, err := controller.StopGIFRecording()
				if err != nil {
					return browserErr("stop_gif_recording", err)
				}
				return &AgentToolResult{Content: []ContentBlock{
					{Type: "text", Text: fmt.Sprintf("GIF recording complete: %d frames, %d bytes", frameCount, len(gifData))},
					{Type: "image", MimeType: "image/gif", Data: base64.StdEncoding.EncodeToString(gifData)},
				}}, nil

			case "list_tabs":
				tabs, err := controller.ListTabs(ctx)
				if err != nil {
					return browserErr("list_tabs", err)
				}
				return JsonResult(map[string]any{"tabs": tabs}), nil

			case "create_tab":
				url, _ := ReadStringParam(args, "url", nil)
				tab, err := controller.CreateTab(ctx, url)
				if err != nil {
					return browserErr("create_tab", err)
				}
				return JsonResult(map[string]any{"status": "created", "tab": tab}), nil

			case "close_tab":
				targetID, err := ReadStringParam(args, "target_id", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("close_tab", err)
				}
				if err := controller.CloseTab(ctx, targetID); err != nil {
					return browserErr("close_tab", err)
				}
				return JsonResult(map[string]any{"status": "closed", "target_id": targetID}), nil

			case "switch_tab":
				targetID, err := ReadStringParam(args, "target_id", &StringParamOptions{Required: true})
				if err != nil {
					return browserErr("switch_tab", err)
				}
				if err := controller.SwitchTab(ctx, targetID); err != nil {
					return browserErr("switch_tab", err)
				}
				return JsonResult(map[string]any{"status": "switched", "target_id": targetID}), nil

			default:
				return softText(fmt.Sprintf("[Unknown browser action: %s]", action)), nil
			}
		},
	}
}
