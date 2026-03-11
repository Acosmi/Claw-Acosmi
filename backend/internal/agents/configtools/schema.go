package configtools

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type ParamKind string

const (
	ParamString      ParamKind = "string"
	ParamBoolean     ParamKind = "boolean"
	ParamInteger     ParamKind = "integer"
	ParamNumber      ParamKind = "number"
	ParamStringArray ParamKind = "string_array"
	ParamObject      ParamKind = "object"
)

type ParamSpec struct {
	Kind        ParamKind
	Description string
	Minimum     *float64
}

var paramCatalog = map[string]ParamSpec{
	"raw":                  {Kind: ParamString, Description: "JSON5 string payload. For config.set/config.apply this must be a full document; for config.patch it must be a JSON5 merge patch object."},
	"baseHash":             {Kind: ParamString, Description: "Base hash from the latest matching get action. Required by safe write actions."},
	"sessionKey":           {Kind: ParamString, Description: "Optional session key recorded in restart sentinel metadata for config.apply/config.patch."},
	"note":                 {Kind: ParamString, Description: "Optional audit note recorded in restart sentinel metadata for config.apply/config.patch."},
	"restartDelayMs":       {Kind: ParamInteger, Description: "Optional restart delay in milliseconds for config.apply/config.patch.", Minimum: floatPtr(0)},
	"reason":               {Kind: ParamString, Description: "Optional human-readable restart reason for restart."},
	"delayMs":              {Kind: ParamInteger, Description: "Optional restart delay in milliseconds for restart.", Minimum: floatPtr(0)},
	"enabled":              {Kind: ParamBoolean, Description: "Boolean toggle used by browser or remote approval config."},
	"cdpUrl":               {Kind: ParamString, Description: "Browser CDP websocket/http URL."},
	"evaluateEnabled":      {Kind: ParamBoolean, Description: "Whether browser evaluate support is enabled."},
	"headless":             {Kind: ParamBoolean, Description: "Whether browser auto-launch uses headless mode."},
	"callbackUrl":          {Kind: ParamString, Description: "Remote approval callback URL."},
	"feishu":               {Kind: ParamObject, Description: "Remote approval Feishu config patch object."},
	"dingtalk":             {Kind: ParamObject, Description: "Remote approval DingTalk config patch object."},
	"wecom":                {Kind: ParamObject, Description: "Remote approval WeCom config patch object."},
	"provider":             {Kind: ParamString, Description: "Provider identifier for the target config domain or verification action."},
	"apiKey":               {Kind: ParamString, Description: "API key or secret value. Omit to preserve the existing secret."},
	"model":                {Kind: ParamString, Description: "Model identifier for the selected config domain."},
	"baseUrl":              {Kind: ParamString, Description: "Base URL or endpoint for the selected config domain."},
	"prompt":               {Kind: ParamString, Description: "Custom prompt for image understanding config."},
	"maxTokens":            {Kind: ParamInteger, Description: "Maximum output tokens for image understanding config.", Minimum: floatPtr(0)},
	"binaryPath":           {Kind: ParamString, Description: "Binary path for local-whisper STT mode."},
	"modelPath":            {Kind: ParamString, Description: "Model path for local-whisper STT mode."},
	"language":             {Kind: ParamString, Description: "Preferred language code for STT."},
	"mcpServerName":        {Kind: ParamString, Description: "DocConv MCP server preset name."},
	"mcpTransport":         {Kind: ParamString, Description: "DocConv MCP transport (stdio or sse)."},
	"mcpCommand":           {Kind: ParamString, Description: "DocConv MCP command when transport=stdio."},
	"mcpUrl":               {Kind: ParamString, Description: "DocConv MCP URL when transport=sse."},
	"pandocPath":           {Kind: ParamString, Description: "Pandoc executable path for builtin DocConv."},
	"useSandbox":           {Kind: ParamBoolean, Description: "Whether DocConv builtin mode should use sandbox execution."},
	"autoSpawnEnabled":     {Kind: ParamBoolean, Description: "Whether media auto-spawn is enabled."},
	"maxAutoSpawnsPerDay":  {Kind: ParamInteger, Description: "Maximum media auto-spawns per day.", Minimum: floatPtr(0)},
	"hotKeywords":          {Kind: ParamStringArray, Description: "Media hot keywords list."},
	"monitorIntervalMin":   {Kind: ParamInteger, Description: "Media monitor interval in minutes.", Minimum: floatPtr(0)},
	"trendingThreshold":    {Kind: ParamNumber, Description: "Media trending threshold.", Minimum: floatPtr(0)},
	"contentCategories":    {Kind: ParamStringArray, Description: "Preferred media content categories."},
	"autoDraftEnabled":     {Kind: ParamBoolean, Description: "Whether media auto-draft is enabled."},
	"trendingBocha":        {Kind: ParamObject, Description: "Media Bocha trending source patch."},
	"trendingCustomOpenAI": {Kind: ParamObject, Description: "Media custom OpenAI trending source patch."},
	"wechat":               {Kind: ParamObject, Description: "Media WeChat publisher patch."},
	"xiaohongshu":          {Kind: ParamObject, Description: "Media Xiaohongshu publisher patch."},
	"website":              {Kind: ParamObject, Description: "Media website publisher patch."},
}

func floatPtr(v float64) *float64 {
	return &v
}

func PropertySchema(name string) (map[string]any, bool) {
	spec, ok := paramCatalog[strings.TrimSpace(name)]
	if !ok {
		return nil, false
	}
	schema := map[string]any{
		"description": spec.Description,
	}
	switch spec.Kind {
	case ParamString:
		schema["type"] = "string"
	case ParamBoolean:
		schema["type"] = "boolean"
	case ParamInteger:
		schema["type"] = "integer"
	case ParamNumber:
		schema["type"] = "number"
	case ParamStringArray:
		schema["type"] = "array"
		schema["items"] = map[string]any{"type": "string"}
	case ParamObject:
		schema["type"] = "object"
		schema["additionalProperties"] = true
	default:
		return nil, false
	}
	if spec.Minimum != nil {
		schema["minimum"] = *spec.Minimum
	}
	return schema, true
}

func ToolParameters(spec DomainToolSpec) map[string]any {
	properties := map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        actionEnum(spec),
			"description": fmt.Sprintf("%s action to perform.", spec.ToolName),
		},
	}
	for _, action := range spec.Actions {
		for _, key := range allActionParams(action) {
			if _, exists := properties[key]; exists {
				continue
			}
			if schema, ok := PropertySchema(key); ok {
				properties[key] = schema
			}
		}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   []any{"action"},
	}
}

func actionEnum(spec DomainToolSpec) []any {
	values := make([]any, 0, len(spec.Actions))
	for _, action := range spec.Actions {
		values = append(values, action.Name)
	}
	return values
}

func AllowedParams(spec DomainToolSpec) []string {
	names := make([]string, 0)
	seen := make(map[string]bool)
	for _, action := range spec.Actions {
		for _, key := range allActionParams(action) {
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			names = append(names, key)
		}
	}
	sort.Strings(names)
	return names
}

func BuildActionParams(args map[string]any, requiredParams, allowedParams []string) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	for _, key := range requiredParams {
		value, err := readRequiredParam(args, key)
		if err != nil {
			return nil, err
		}
		params[key] = value
	}

	for _, key := range allowedParams {
		if _, exists := params[key]; exists {
			continue
		}
		value, found, err := readOptionalParam(args, key)
		if err != nil {
			return nil, err
		}
		if found {
			params[key] = value
		}
	}

	if len(params) == 0 {
		return nil, nil
	}
	return params, nil
}

func readRequiredParam(args map[string]any, key string) (interface{}, error) {
	value, found, err := readOptionalParam(args, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s required", key)
	}
	return value, nil
}

func readOptionalParam(args map[string]any, key string) (interface{}, bool, error) {
	raw, ok := args[key]
	if !ok {
		return nil, false, nil
	}
	value, err := CoerceParamValue(key, raw)
	if err != nil {
		return nil, false, err
	}
	if value == nil {
		return nil, false, nil
	}
	return value, true, nil
}

func CoerceParamValue(key string, raw any) (interface{}, error) {
	spec, ok := paramCatalog[strings.TrimSpace(key)]
	if !ok {
		return raw, nil
	}
	switch spec.Kind {
	case ParamString:
		value, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be a string", key)
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, nil
		}
		return trimmed, nil
	case ParamBoolean:
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				return nil, nil
			}
			parsed, err := strconv.ParseBool(trimmed)
			if err != nil {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			return parsed, nil
		default:
			return nil, fmt.Errorf("%s must be a boolean", key)
		}
	case ParamInteger:
		value, ok, err := coerceInteger(raw)
		if err != nil {
			return nil, fmt.Errorf("%s %w", key, err)
		}
		if !ok {
			return nil, nil
		}
		return value, nil
	case ParamNumber:
		value, ok, err := coerceNumber(raw)
		if err != nil {
			return nil, fmt.Errorf("%s %w", key, err)
		}
		if !ok {
			return nil, nil
		}
		return value, nil
	case ParamStringArray:
		value, ok := coerceStringArray(raw)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", key)
		}
		if len(value) == 0 {
			return nil, nil
		}
		return value, nil
	case ParamObject:
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return value, nil
	default:
		return raw, nil
	}
}

func coerceInteger(raw any) (int, bool, error) {
	switch v := raw.(type) {
	case float64:
		if v < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return int(v), true, nil
	case int:
		if v < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return v, true, nil
	case int64:
		if v < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return int(v), true, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false, nil
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false, fmt.Errorf("must be an integer")
		}
		if parsed < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return parsed, true, nil
	default:
		return 0, false, fmt.Errorf("must be an integer")
	}
}

func coerceNumber(raw any) (float64, bool, error) {
	switch v := raw.(type) {
	case float64:
		if v < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return v, true, nil
	case int:
		if v < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return float64(v), true, nil
	case int64:
		if v < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return float64(v), true, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false, nil
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false, fmt.Errorf("must be a number")
		}
		if parsed < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return parsed, true, nil
	default:
		return 0, false, fmt.Errorf("must be a number")
	}
}

func coerceStringArray(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		if len(v) == 0 {
			return nil, true
		}
		return slices.Clone(v), true
	case []any:
		values := make([]string, 0, len(v))
		for _, entry := range v {
			str, ok := entry.(string)
			if !ok {
				return nil, false
			}
			trimmed := strings.TrimSpace(str)
			if trimmed == "" {
				continue
			}
			values = append(values, trimmed)
		}
		return values, true
	default:
		return nil, false
	}
}

func allActionParams(action DomainActionSpec) []string {
	params := make([]string, 0, len(action.RequiredParams)+len(action.AllowedParams))
	seen := make(map[string]bool)
	for _, key := range append(slices.Clone(action.RequiredParams), action.AllowedParams...) {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		params = append(params, trimmed)
	}
	return params
}
