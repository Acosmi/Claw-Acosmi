package configtools

import (
	"context"
	"fmt"
)

type MethodCaller func(ctx context.Context, method string, params interface{}) (map[string]interface{}, error)

func ExecuteToolAction(ctx context.Context, spec DomainToolSpec, args map[string]any, call MethodCaller) (interface{}, error) {
	actionName, ok := args["action"].(string)
	if !ok || actionName == "" {
		return nil, fmt.Errorf("action required")
	}

	action, found := FindActionSpec(spec, actionName)
	if !found {
		return nil, fmt.Errorf("unknown %s action: %s", spec.ToolName, actionName)
	}

	params, err := BuildActionParams(args, action.RequiredParams, action.AllowedParams)
	if err != nil {
		return nil, err
	}

	result, err := call(ctx, action.Method, params)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", spec.ToolName, action.Name, err)
	}
	if action.Status == "" {
		return result, nil
	}
	return map[string]any{
		"tool":   spec.ToolName,
		"action": action.Name,
		"status": action.Status,
		"result": result,
	}, nil
}
