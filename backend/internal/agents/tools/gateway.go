// tools/gateway.go — Gateway 工具。
package tools

import (
	"context"

	"github.com/Acosmi/ClawAcosmi/internal/agents/gatewayclient"
)

type GatewayOptions = gatewayclient.GatewayOptions
type GatewayCaller = gatewayclient.GatewayCaller
type GatewayCallError = gatewayclient.GatewayCallError

var GatewayToolActions = gatewayclient.GatewayToolActions

func DefaultGatewayOptions() GatewayOptions {
	return gatewayclient.DefaultGatewayOptions()
}

func CallGateway(ctx context.Context, opts GatewayOptions, method string, params interface{}) (map[string]interface{}, error) {
	return gatewayclient.CallGateway(ctx, opts, method, params)
}

func GatewayToolDescription() string {
	return gatewayclient.GatewayToolDescription()
}

func GatewayToolParameters() map[string]any {
	return gatewayclient.GatewayToolParameters()
}

// ExecuteGatewayTool 执行 gateway 工具调用。
func ExecuteGatewayTool(ctx context.Context, opts GatewayOptions, args map[string]any) (*AgentToolResult, error) {
	payload, err := gatewayclient.ExecuteToolAction(ctx, opts, args)
	if err != nil {
		return nil, err
	}
	return JsonResult(payload), nil
}

// CreateGatewayTool 创建 gateway 工具。
func CreateGatewayTool(opts GatewayOptions) *AgentTool {
	return &AgentTool{
		Name:        "gateway",
		Label:       "Gateway",
		Description: GatewayToolDescription(),
		Parameters:  GatewayToolParameters(),
		Execute: func(ctx context.Context, toolCallID string, args map[string]any) (*AgentToolResult, error) {
			return ExecuteGatewayTool(ctx, opts, args)
		},
	}
}
