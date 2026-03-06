// tools/gateway.go — Gateway 工具。
// TS 参考：src/agents/tools/gateway.ts (48L) + gateway-tool.ts (254L)
// 协议：WebSocket 长连接，端口通过 config.ResolveGatewayPort 动态解析。
// 帧格式：RequestFrame → ResponseFrame（根据 id 字段匹配）。
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- Gateway 连接选项 ----------

// GatewayOptions Gateway 连接选项。
type GatewayOptions struct {
	// URL WebSocket 地址，通过 config.ResolveGatewayPort 动态解析。
	URL     string
	Token   string
	Timeout time.Duration
}

// DefaultGatewayOptions 默认 Gateway 选项。
// 端口通过 config.ResolveGatewayPort 解析（env > config > DefaultGatewayPort）。
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

// ---------- WebSocket 帧类型（对齐 backend/internal/gateway/protocol.go）----------

// wsRequestFrame WS 请求帧（客户端 → 服务端）。
type wsRequestFrame struct {
	Type   string      `json:"type"`             // 固定 "req"
	ID     string      `json:"id"`               // 请求 ID，用于关联响应
	Method string      `json:"method"`           // 方法名
	Params interface{} `json:"params,omitempty"` // 可选参数
}

// wsResponseFrame WS 响应帧（服务端 → 客户端）。
type wsResponseFrame struct {
	Type    string      `json:"type"`              // "res" | "event" | "hello-ok" | …
	ID      string      `json:"id"`                // 匹配 RequestFrame.ID
	OK      bool        `json:"ok"`                // 是否成功
	Payload interface{} `json:"payload,omitempty"` // 成功载荷
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"` // 失败错误
}

// wsConnectFrame WS 握手帧（对齐 TS: ConnectParams）。
type wsConnectFrame struct {
	Type        string `json:"type"`        // 固定 "connect"
	MinProtocol int    `json:"minProtocol"` // 支持最低协议版本
	MaxProtocol int    `json:"maxProtocol"` // 支持最高协议版本
	Role        string `json:"role"`        // "operator"
	Client      struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName,omitempty"`
		Version     string `json:"version"`
		Mode        string `json:"mode"` // "backend"
	} `json:"client"`
	Auth *struct {
		Token string `json:"token,omitempty"`
	} `json:"auth,omitempty"`
}

// ---------- WebSocket Gateway 调用 ----------

// CallGateway 通过 WebSocket 调用 Gateway 方法。
// 对齐 TS callGateway(): 建立 WS 连接 → 握手 → 发送 req 帧 → 等待 res 帧 → 断开。
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

	// ---------- 1. 建立 WebSocket 连接 ----------
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	headers := http.Header{}
	if opts.Token != "" {
		headers.Set("Authorization", "Bearer "+opts.Token)
	}

	conn, _, err := dialer.DialContext(ctx, opts.URL, headers)
	if err != nil {
		return nil, fmt.Errorf("gateway ws connect %s: %w", opts.URL, err)
	}
	defer conn.Close()

	// ---------- 2. 读取并处理 connect.challenge 事件（如存在）----------
	// 服务端在连接后立即发送 connect.challenge 事件，然后等待 connect 帧。
	// 对于内部调用，只需读取并丢弃 challenge 消息即可。
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, rawChallenge, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("gateway ws read challenge: %w", err)
	}
	conn.SetReadDeadline(time.Time{}) // 清除 deadline

	// 解析帧类型以确认是 challenge 事件（可选验证）
	var challengeFrame struct {
		Type  string `json:"type"`
		Event string `json:"event"`
	}
	_ = json.Unmarshal(rawChallenge, &challengeFrame)
	// 不验证 challenge 内容，内部后端连接豁免 nonce 验证

	// ---------- 3. 发送 connect 握手帧 ----------
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

	// ---------- 4. 等待 hello-ok 帧 ----------
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

	// ---------- 5. 发送请求帧 ----------
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

	// ---------- 6. 等待响应帧（根据 ID 匹配）----------
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
			continue // 跳过无法解析的帧
		}

		// 只处理类型为 "res" 且 ID 匹配的帧，跳过 event 等其他帧
		if resp.Type != "res" || resp.ID != reqID {
			continue
		}

		if !resp.OK {
			errCode := ""
			errMsg := ""
			if resp.Error != nil {
				errCode = resp.Error.Code
				errMsg = resp.Error.Message
			}
			return nil, fmt.Errorf("gateway method %s failed [%s]: %s", method, errCode, errMsg)
		}

		// 将 payload 转换为 map[string]interface{}
		if resp.Payload == nil {
			return map[string]interface{}{}, nil
		}
		// payload 可能已经是 map，也可能需要重新序列化
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

// ---------- Gateway Tool ----------

// GatewayToolActions 支持的 Gateway 操作。
var GatewayToolActions = []string{
	"restart",
	"config.get",
	"config.apply",
	"config.patch",
	"update.run",
}

// CreateGatewayTool 创建 gateway 工具。
// TS 参考: gateway-tool.ts
func CreateGatewayTool(opts GatewayOptions) *AgentTool {
	return &AgentTool{
		Name:        "gateway",
		Label:       "Gateway",
		Description: "Interact with the Claw Acosmi gateway: restart, get/apply/patch configuration, run updates.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []any{"restart", "config.get", "config.apply", "config.patch", "update.run"},
					"description": "The gateway action to perform",
				},
				"config": map[string]any{
					"type":        "object",
					"description": "Configuration object (for config.apply and config.patch)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Configuration path (for config.get with specific path)",
				},
			},
			"required": []any{"action"},
		},
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (*AgentToolResult, error) {
			action, err := ReadStringParam(args, "action", &StringParamOptions{Required: true})
			if err != nil {
				return nil, err
			}

			switch action {
			case "restart":
				result, err := CallGateway(ctx, opts, "system.restart", nil)
				if err != nil {
					return nil, fmt.Errorf("restart failed: %w", err)
				}
				return JsonResult(map[string]any{
					"action": "restart",
					"status": "initiated",
					"result": result,
				}), nil

			case "config.get":
				path, _ := ReadStringParam(args, "path", nil)
				var params interface{}
				if path != "" {
					params = map[string]interface{}{"path": path}
				}
				result, err := CallGateway(ctx, opts, "config.get", params)
				if err != nil {
					return nil, fmt.Errorf("config.get failed: %w", err)
				}
				return JsonResult(result), nil

			case "config.apply":
				config := args["config"]
				if config == nil {
					return nil, fmt.Errorf("config required for config.apply")
				}
				result, err := CallGateway(ctx, opts, "config.apply", map[string]interface{}{"config": config})
				if err != nil {
					return nil, fmt.Errorf("config.apply failed: %w", err)
				}
				return JsonResult(map[string]any{
					"action": "config.apply",
					"status": "applied",
					"result": result,
				}), nil

			case "config.patch":
				config := args["config"]
				if config == nil {
					return nil, fmt.Errorf("config required for config.patch")
				}
				result, err := CallGateway(ctx, opts, "config.patch", map[string]interface{}{"config": config})
				if err != nil {
					return nil, fmt.Errorf("config.patch failed: %w", err)
				}
				return JsonResult(map[string]any{
					"action": "config.patch",
					"status": "patched",
					"result": result,
				}), nil

			case "update.run":
				result, err := CallGateway(ctx, opts, "update.run", nil)
				if err != nil {
					return nil, fmt.Errorf("update.run failed: %w", err)
				}
				return JsonResult(map[string]any{
					"action": "update.run",
					"status": "initiated",
					"result": result,
				}), nil

			default:
				return nil, fmt.Errorf("unknown gateway action: %s", action)
			}
		},
	}
}
