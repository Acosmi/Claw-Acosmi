// Package skills implements the OpenClaw skill executor,
// running multi-step desktop automation sequences.
// Ported from openclaw-skills/core/skill_executor.py.
//
// Critical improvement: all actions execute via in-process InputController
// instead of WebSocket round-trips (eliminating ~2ms latency per step).
package skills

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"Argus-compound/go-sensory/internal/input"
)

// ──────────────────────────────────────────────────────────────
// Skill types (from openclaw-skills/core/models.py)
// ──────────────────────────────────────────────────────────────

// StepType enumerates skill step types.
type StepType string

const (
	StepClick       StepType = "click"
	StepDoubleClick StepType = "double_click"
	StepType_       StepType = "type"
	StepHotkey      StepType = "hotkey"
	StepScroll      StepType = "scroll"
	StepWait        StepType = "wait"
	StepGround      StepType = "ground" // Visual grounding via VLM
	StepAssertText  StepType = "assert_text"
	StepScreenshot  StepType = "screenshot"
)

// SkillStep represents a single step in a skill.
type SkillStep struct {
	Type        StepType       `json:"type"`
	Params      map[string]any `json:"params"`
	Description string         `json:"description,omitempty"`
	TimeoutMs   int            `json:"timeout_ms,omitempty"`
	Retry       int            `json:"retry,omitempty"`
}

// Skill represents a multi-step desktop automation sequence.
type Skill struct {
	Name      string            `json:"name"`
	Steps     []SkillStep       `json:"steps"`
	Variables map[string]string `json:"variables,omitempty"`
}

// StepResult records the outcome of a single step execution.
type StepResult struct {
	StepIndex  int            `json:"step_index"`
	Success    bool           `json:"success"`
	DurationMs float64        `json:"duration_ms"`
	Data       map[string]any `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// SkillResult holds the complete outcome of executing a skill.
type SkillResult struct {
	SkillName       string            `json:"skill_name"`
	Success         bool              `json:"success"`
	TotalDurationMs float64           `json:"total_duration_ms"`
	StepResults     []StepResult      `json:"step_results"`
	Error           string            `json:"error,omitempty"`
	Variables       map[string]string `json:"variables"`
}

// ──────────────────────────────────────────────────────────────
// SkillExecutor
// ──────────────────────────────────────────────────────────────

// UIParserIface abstracts the UIParser for visual grounding.
// This avoids a circular import with the agent package.
type UIParserIface interface {
	// FindElement returns the (x,y) center of the element matching the
	// given natural-language prompt on the provided screenshot, or an
	// error if no match is found.
	FindElement(screenshot []byte, width, height int, prompt string) (x, y int, err error)
}

// ScreenCapturer abstracts frame capture for screenshot steps.
type ScreenCapturer interface {
	LatestJPEG(quality int) ([]byte, error)
}

// SkillExecutor runs skills using in-process InputController and UIParser.
type SkillExecutor struct {
	inputCtrl input.InputController
	uiParser  UIParserIface  // for GROUND steps, nil = disabled
	capturer  ScreenCapturer // for SCREENSHOT steps, nil = disabled
}

// NewSkillExecutor creates an executor with in-process dependencies.
func NewSkillExecutor(
	inputCtrl input.InputController,
	uiParser UIParserIface,
	capturer ScreenCapturer,
) *SkillExecutor {
	return &SkillExecutor{
		inputCtrl: inputCtrl,
		uiParser:  uiParser,
		capturer:  capturer,
	}
}

// Execute runs a complete skill.
func (e *SkillExecutor) Execute(ctx context.Context, skill Skill) (*SkillResult, error) {
	start := time.Now()
	stepResults := make([]StepResult, 0, len(skill.Steps))
	variables := make(map[string]string)
	for k, v := range skill.Variables {
		variables[k] = v
	}
	success := true
	var finalError string

	log.Printf("[SkillExecutor] Starting skill: %s (%d steps)", skill.Name, len(skill.Steps))

	for i, step := range skill.Steps {
		stepStart := time.Now()

		// Resolve variable placeholders
		resolvedParams := resolveVariables(step.Params, variables)

		retry := step.Retry
		if retry < 1 {
			retry = 1
		}
		timeoutMs := step.TimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = 30000 // 30s default
		}

		var stepResult StepResult
		stepResult.StepIndex = i

		for attempt := 1; attempt <= retry; attempt++ {
			stepCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)

			data, err := e.executeStep(stepCtx, SkillStep{
				Type:   step.Type,
				Params: resolvedParams,
			}, variables)

			cancel()

			if err == nil {
				stepResult.Success = true
				stepResult.Data = data
				break
			}

			stepResult.Error = err.Error()
			log.Printf("[SkillExecutor] Step %d failed: %v (attempt %d/%d)",
				i, err, attempt, retry)
		}

		stepResult.DurationMs = float64(time.Since(stepStart).Milliseconds())
		stepResults = append(stepResults, stepResult)

		if !stepResult.Success {
			success = false
			finalError = stepResult.Error
			break
		}
	}

	totalMs := float64(time.Since(start).Milliseconds())
	status := "completed"
	if !success {
		status = "failed"
	}
	log.Printf("[SkillExecutor] Skill '%s' %s in %.0fms", skill.Name, status, totalMs)

	return &SkillResult{
		SkillName:       skill.Name,
		Success:         success,
		TotalDurationMs: totalMs,
		StepResults:     stepResults,
		Error:           finalError,
		Variables:       variables,
	}, nil
}

// executeStep runs a single step via in-process InputController.
func (e *SkillExecutor) executeStep(ctx context.Context, step SkillStep, variables map[string]string) (map[string]any, error) {
	switch step.Type {
	case StepClick:
		x := getInt(step.Params, "x")
		y := getInt(step.Params, "y")
		return nil, e.inputCtrl.Click(x, y, input.MouseLeft)

	case StepDoubleClick:
		x := getInt(step.Params, "x")
		y := getInt(step.Params, "y")
		return nil, e.inputCtrl.DoubleClick(x, y)

	case StepType_:
		text := getString(step.Params, "text")
		return nil, e.inputCtrl.Type(text)

	case StepHotkey:
		keys := getKeyList(step.Params)
		if len(keys) > 0 {
			return nil, e.inputCtrl.Hotkey(keys...)
		}
		return nil, nil

	case StepScroll:
		x := getInt(step.Params, "x")
		y := getInt(step.Params, "y")
		dx := getInt(step.Params, "delta_x")
		dy := getIntDefault(step.Params, "delta_y", -3)
		return nil, e.inputCtrl.Scroll(x, y, dx, dy)

	case StepWait:
		ms := getIntDefault(step.Params, "ms", 1000)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(ms) * time.Millisecond):
		}
		return map[string]any{"waited_ms": ms}, nil

	case StepGround:
		// Visual grounding via UIParser (in-process, replaces HTTP call)
		if e.uiParser == nil {
			return nil, fmt.Errorf("UIParser not configured for grounding")
		}
		prompt := getString(step.Params, "prompt")
		// Would need screenshot here — placeholder for integration
		return map[string]any{"grounded": false, "note": "grounding requires screenshot integration"}, fmt.Errorf("ground step needs screenshot: %s", prompt)

	case StepAssertText:
		// Text assertion — placeholder, needs OCR integration
		expected := getString(step.Params, "text")
		return map[string]any{"note": "assert_text needs OCR integration"}, fmt.Errorf("assert_text not yet integrated: expected '%s'", expected)

	case StepScreenshot:
		// Screenshot capture — would use capturer
		return map[string]any{"note": "screenshot step needs capturer integration"}, nil

	default:
		log.Printf("[SkillExecutor] Unknown step type: %s", step.Type)
		return nil, nil
	}
}

// ──────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────

// resolveVariables replaces {{var}} placeholders in params.
func resolveVariables(params map[string]any, variables map[string]string) map[string]any {
	resolved := make(map[string]any, len(params))
	for k, v := range params {
		if s, ok := v.(string); ok && strings.Contains(s, "{{") {
			for varName, varValue := range variables {
				s = strings.ReplaceAll(s, "{{"+varName+"}}", varValue)
			}
			resolved[k] = s
		} else {
			resolved[k] = v
		}
	}
	return resolved
}

func getInt(params map[string]any, key string) int {
	return getIntDefault(params, key, 0)
}

func getIntDefault(params map[string]any, key string, def int) int {
	v, ok := params[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return def
}

func getString(params map[string]any, key string) string {
	v, ok := params[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func getKeyList(params map[string]any) []input.Key {
	v, ok := params["keys"]
	if !ok {
		return nil
	}
	switch keys := v.(type) {
	case []any:
		result := make([]input.Key, 0, len(keys))
		for _, k := range keys {
			if n, ok := k.(float64); ok {
				result = append(result, input.Key(int(n)))
			}
		}
		return result
	}
	return nil
}
