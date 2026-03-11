package gateway

import "testing"

func TestAuthz_McpServerList_InReadMethods(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.read"},
	}}
	if err := AuthorizeGatewayMethod("mcp.server.list", client); err != nil {
		t.Errorf("mcp.server.list should be accessible with read scope, got %v", err)
	}
}

func TestAuthz_McpServerStart_InWriteMethods(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.write"},
	}}
	if err := AuthorizeGatewayMethod("mcp.server.start", client); err != nil {
		t.Errorf("mcp.server.start should be accessible with write scope, got %v", err)
	}
}

func TestAuthz_McpServerStart_DeniedWithReadOnly(t *testing.T) {
	client := &GatewayClient{Connect: &ConnectParamsFull{
		Role: "operator", Scopes: []string{"operator.read"},
	}}
	if err := AuthorizeGatewayMethod("mcp.server.start", client); err == nil {
		t.Error("mcp.server.start should be denied with read-only scope")
	}
}
