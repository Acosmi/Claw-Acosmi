package gatewayclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/Acosmi/ClawAcosmi/internal/agents/configtools"
	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// GatewayOptions Gateway 连接选项。
type GatewayOptions struct {
	URL     string
	Token   string
	Timeout time.Duration
	Caller  GatewayCaller
}

// GatewayCaller Gateway RPC 调用函数。
type GatewayCaller func(ctx context.Context, opts GatewayOptions, method string, params interface{}) (map[string]interface{}, error)

// Enabled 返回 gateway 工具是否已显式配置到当前 runtime。
func (o GatewayOptions) Enabled() bool {
	return strings.TrimSpace(o.URL) != "" || o.Caller != nil
}

// DefaultGatewayOptions 默认 Gateway 选项。
func DefaultGatewayOptions() GatewayOptions {
	url := os.Getenv("OPENACOSMI_GATEWAY_URL")
	if url == "" {
		port := config.ResolveGatewayPort(nil)
		url = fmt.Sprintf("ws://127.0.0.1:%d", port)
	}
	return GatewayOptions{
		URL:     url,
		Timeout: 30 * time.Second,
	}
}

type wsRequestFrame struct {
	Type   string      `json:"type"`
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

type wsResponseFrame struct {
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	OK      bool        `json:"ok"`
	Payload interface{} `json:"payload,omitempty"`
	Error   *struct {
		Code         string      `json:"code"`
		Message      string      `json:"message"`
		Details      interface{} `json:"details,omitempty"`
		Retryable    *bool       `json:"retryable,omitempty"`
		RetryAfterMs *int        `json:"retryAfterMs,omitempty"`
	} `json:"error,omitempty"`
}

// GatewayCallError 保留 gateway 返回的结构化错误信息。
type GatewayCallError struct {
	Method       string
	Code         string
	Message      string
	Details      interface{}
	Retryable    *bool
	RetryAfterMs *int
}

func (e *GatewayCallError) Error() string {
	if e == nil {
		return ""
	}
	msg := fmt.Sprintf("gateway method %s failed", e.Method)
	if e.Code != "" {
		msg += fmt.Sprintf(" [%s]", e.Code)
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Details != nil {
		if data, err := json.Marshal(e.Details); err == nil {
			msg += " details=" + string(data)
		}
	}
	return msg
}

type wsConnectFrame struct {
	Type        string `json:"type"`
	MinProtocol int    `json:"minProtocol"`
	MaxProtocol int    `json:"maxProtocol"`
	Role        string `json:"role"`
	Client      struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName,omitempty"`
		Version     string `json:"version"`
		Mode        string `json:"mode"`
	} `json:"client"`
	Auth *struct {
		Token string `json:"token,omitempty"`
	} `json:"auth,omitempty"`
}

// CallGateway 通过 WebSocket 调用 Gateway 方法。
func CallGateway(ctx context.Context, opts GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
	if opts.URL == "" {
		port := config.ResolveGatewayPort(nil)
		opts.URL = fmt.Sprintf("ws://127.0.0.1:%d", port)
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	headers := http.Header{}
	if opts.Token != "" {
		headers.Set("Authorization", "Bearer "+opts.Token)
	}

	conn, _, err := dialer.DialContext(ctx, opts.URL, headers)
	if err != nil {
		return nil, fmt.Errorf("gateway ws connect %s: %w", opts.URL, err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, rawChallenge, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("gateway ws read challenge: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	var challengeFrame struct {
		Type  string `json:"type"`
		Event string `json:"event"`
	}
	_ = json.Unmarshal(rawChallenge, &challengeFrame)

	connFrame := wsConnectFrame{
		Type:        "connect",
		MinProtocol: 1,
		MaxProtocol: 3,
		Role:        "operator",
	}
	connFrame.Client.ID = uuid.NewString()
	connFrame.Client.DisplayName = "agent"
	connFrame.Client.Version = "dev"
	connFrame.Client.Mode = "backend"
	if opts.Token != "" {
		connFrame.Auth = &struct {
			Token string `json:"token,omitempty"`
		}{Token: opts.Token}
	}

	if err := conn.WriteJSON(connFrame); err != nil {
		return nil, fmt.Errorf("gateway ws send connect: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, rawHello, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("gateway ws read hello-ok: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	var helloFrame struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawHello, &helloFrame); err != nil {
		return nil, fmt.Errorf("gateway ws parse hello-ok: %w", err)
	}
	if helloFrame.Type == "error" {
		var errFrame struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rawHello, &errFrame)
		return nil, fmt.Errorf("gateway ws handshake error %s: %s",
			errFrame.Error.Code, errFrame.Error.Message)
	}
	if helloFrame.Type != "hello-ok" {
		return nil, fmt.Errorf("gateway ws unexpected frame type=%q, expected hello-ok", helloFrame.Type)
	}

	reqID := uuid.NewString()
	reqFrame := wsRequestFrame{
		Type:   "req",
		ID:     reqID,
		Method: method,
		Params: params,
	}
	if err := conn.WriteJSON(reqFrame); err != nil {
		return nil, fmt.Errorf("gateway ws send request: %w", err)
	}

	deadline := time.Now().Add(opts.Timeout)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("gateway ws timeout waiting for response to %s", method)
		}
		conn.SetReadDeadline(deadline)

		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("gateway ws read response: %w", err)
		}

		var resp wsResponseFrame
		if err := json.Unmarshal(rawMsg, &resp); err != nil {
			continue
		}
		if resp.Type != "res" || resp.ID != reqID {
			continue
		}
		if !resp.OK {
			callErr := &GatewayCallError{Method: method}
			if resp.Error != nil {
				callErr.Code = resp.Error.Code
				callErr.Message = resp.Error.Message
				callErr.Details = resp.Error.Details
				callErr.Retryable = resp.Error.Retryable
				callErr.RetryAfterMs = resp.Error.RetryAfterMs
			}
			return nil, callErr
		}
		if resp.Payload == nil {
			return map[string]interface{}{}, nil
		}
		payloadData, err := json.Marshal(resp.Payload)
		if err != nil {
			return map[string]interface{}{"raw": resp.Payload}, nil
		}
		var result map[string]interface{}
		if err := json.Unmarshal(payloadData, &result); err != nil {
			return map[string]interface{}{"raw": resp.Payload}, nil
		}
		return result, nil
	}
}

type gatewayActionSpec struct {
	Action         string
	Method         string
	Status         string
	AllowedParams  []string
	RequiredParams []string
}

var gatewayActionSpecs = buildGatewayActionSpecs()

func buildGatewayActionSpecs() []gatewayActionSpec {
	specs := []gatewayActionSpec{
		{Action: "restart", Method: "system.restart", Status: "initiated", AllowedParams: []string{"reason", "delayMs"}},
		{Action: "config.get", Method: "config.get"},
		{Action: "config.schema", Method: "config.schema"},
		{Action: "config.set", Method: "config.set", Status: "written", AllowedParams: []string{"raw", "baseHash"}, RequiredParams: []string{"raw", "baseHash"}},
		{Action: "config.apply", Method: "config.apply", Status: "applied", AllowedParams: []string{"raw", "baseHash", "sessionKey", "note", "restartDelayMs"}, RequiredParams: []string{"raw", "baseHash"}},
		{Action: "config.patch", Method: "config.patch", Status: "patched", AllowedParams: []string{"raw", "baseHash", "sessionKey", "note", "restartDelayMs"}, RequiredParams: []string{"raw", "baseHash"}},
		{Action: "update.run", Method: "update.run", Status: "initiated"},
	}
	for _, toolSpec := range configtools.ToolSpecs() {
		for _, action := range toolSpec.Actions {
			specs = append(specs, gatewayActionSpec{
				Action:         action.Method,
				Method:         action.Method,
				Status:         action.Status,
				AllowedParams:  append([]string(nil), action.AllowedParams...),
				RequiredParams: append([]string(nil), action.RequiredParams...),
			})
		}
	}
	return specs
}

var GatewayToolActions = func() []string {
	actions := make([]string, 0, len(gatewayActionSpecs))
	for _, spec := range gatewayActionSpecs {
		actions = append(actions, spec.Action)
	}
	return actions
}()

var gatewayActionSpecByName = func() map[string]gatewayActionSpec {
	index := make(map[string]gatewayActionSpec, len(gatewayActionSpecs))
	for _, spec := range gatewayActionSpecs {
		index[spec.Action] = spec
	}
	return index
}()

const gatewayToolDescription = "Interact with the Crab Claw（蟹爪） gateway: generic config.get/config.schema/config.set/config.apply/config.patch, restart, update.run, and specialized config fallback actions."

func gatewayActionEnum() []any {
	values := make([]any, len(GatewayToolActions))
	for i, action := range GatewayToolActions {
		values[i] = action
	}
	return values
}

// GatewayToolDescription 返回 gateway 工具描述。
func GatewayToolDescription() string {
	return gatewayToolDescription
}

// GatewayToolParameters 返回 gateway 工具参数 schema。
func GatewayToolParameters() map[string]any {
	properties := map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        gatewayActionEnum(),
			"description": "Gateway action to perform. Prefer browser_config/remote_approval_config/image_config/stt_config/docconv_config/media_config when those top-level tools are available; use gateway specialized actions as fallback.",
		},
	}
	for _, spec := range gatewayActionSpecs {
		for _, key := range append(append([]string(nil), spec.RequiredParams...), spec.AllowedParams...) {
			if _, exists := properties[key]; exists {
				continue
			}
			if schema, ok := configtools.PropertySchema(key); ok {
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

func gatewayCaller(opts GatewayOptions) GatewayCaller {
	if opts.Caller != nil {
		return opts.Caller
	}
	return CallGateway
}

func readStringArg(args map[string]any, key string) string {
	raw, ok := args[key]
	if !ok {
		return ""
	}
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

func readRequiredStringArg(args map[string]any, key string) (string, error) {
	value := readStringArg(args, key)
	if value == "" {
		return "", fmt.Errorf("%s required", key)
	}
	return value, nil
}

func lookupGatewayActionSpec(action string) (gatewayActionSpec, bool) {
	spec, ok := gatewayActionSpecByName[action]
	return spec, ok
}

func buildGatewayActionParams(args map[string]any, spec gatewayActionSpec) (map[string]interface{}, error) {
	return configtools.BuildActionParams(args, spec.RequiredParams, spec.AllowedParams)
}

// ExecuteToolAction 执行 gateway 工具调用，返回可序列化 payload。
func ExecuteToolAction(ctx context.Context, opts GatewayOptions, args map[string]any) (interface{}, error) {
	action, err := readRequiredStringArg(args, "action")
	if err != nil {
		return nil, err
	}
	call := gatewayCaller(opts)

	spec, ok := lookupGatewayActionSpec(action)
	if !ok {
		return nil, fmt.Errorf("unknown gateway action: %s", action)
	}

	params, err := buildGatewayActionParams(args, spec)
	if err != nil {
		return nil, err
	}

	result, err := call(ctx, opts, spec.Method, params)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", action, err)
	}
	if spec.Status == "" {
		return result, nil
	}
	return map[string]any{
		"action": action,
		"status": spec.Status,
		"result": result,
	}, nil
}
