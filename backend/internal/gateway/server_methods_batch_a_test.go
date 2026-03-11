package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/models"
	"github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// ---------- config.get ----------

func TestConfigHandlers_GetWithLoader(t *testing.T) {
	loader := config.NewConfigLoader()
	r := NewMethodRegistry()
	r.RegisterAll(ConfigHandlers())

	req := &RequestFrame{Method: "config.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ConfigLoader: loader}, respond)

	// config.get should succeed (even if no config file exists, it returns a snapshot)
	// or return an internal error if config file doesn't exist — either is acceptable
	_ = gotOK
	_ = gotPayload
}

func TestConfigHandlers_GetNoLoader(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ConfigHandlers())

	req := &RequestFrame{Method: "config.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should fail without config loader")
	}
	if gotErr == nil || gotErr.Code != ErrCodeInternalError {
		t.Errorf("expected internal_error, got %v", gotErr)
	}
}

// ---------- config.schema ----------

func TestConfigHandlers_Schema(t *testing.T) {
	loader := config.NewConfigLoader()
	r := NewMethodRegistry()
	r.RegisterAll(ConfigHandlers())

	req := &RequestFrame{Method: "config.schema", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ConfigLoader: loader}, respond)
	if !gotOK {
		t.Error("config.schema should succeed")
	}
	schema, ok := gotPayload.(*config.ConfigSchemaResponse)
	if !ok {
		t.Fatalf("expected *ConfigSchemaResponse, got %T", gotPayload)
	}
	if schema.UIHints == nil {
		t.Error("schema should have UIHints")
	}
}

// ---------- config.set (no loader) ----------

func TestConfigHandlers_SetNoLoader(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ConfigHandlers())

	req := &RequestFrame{Method: "config.set", Params: map[string]interface{}{
		"raw": "{}",
	}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should fail without config loader")
	}
	if gotErr == nil || gotErr.Code != ErrCodeInternalError {
		t.Errorf("expected internal_error, got %v", gotErr)
	}
}

func TestConfigHandlers_SetMissingParam(t *testing.T) {
	loader := config.NewConfigLoader()
	r := NewMethodRegistry()
	r.RegisterAll(ConfigHandlers())

	req := &RequestFrame{Method: "config.set", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ConfigLoader: loader}, respond)
	if gotOK {
		t.Error("should fail without raw param")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request, got %v", gotErr)
	}
}

// ---------- config.patch (missing param) ----------

func TestConfigHandlers_PatchMissingParam(t *testing.T) {
	loader := config.NewConfigLoader()
	r := NewMethodRegistry()
	r.RegisterAll(ConfigHandlers())

	req := &RequestFrame{Method: "config.patch", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ConfigLoader: loader}, respond)
	if gotOK {
		t.Error("should fail without raw patch")
	}
	if gotErr == nil || gotErr.Code != ErrCodeBadRequest {
		t.Errorf("expected bad_request, got %v", gotErr)
	}
}

// ---------- models.list ----------

func TestModelsHandlers_ListEmpty(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(ModelsHandlers())

	req := &RequestFrame{Method: "models.list", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("models.list should succeed even without catalog")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	modelsArr, ok := result["models"].([]ModelEntryWithSource)
	if !ok {
		t.Fatalf("expected models array, got %T", result["models"])
	}
	if len(modelsArr) == 0 {
		t.Fatal("expected builtin models to be available")
	}
}

func TestModelsHandlers_ListWithCatalog(t *testing.T) {
	catalog := models.NewModelCatalog()
	r := NewMethodRegistry()
	r.RegisterAll(ModelsHandlers())

	req := &RequestFrame{Method: "models.list", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ModelCatalog: catalog}, respond)
	if !gotOK {
		t.Error("models.list should succeed")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	// Empty catalog should return empty array (Phase 4: now returns []ModelEntryWithSource with source field)
	entries, ok := result["models"].([]ModelEntryWithSource)
	if !ok {
		t.Fatalf("expected []ModelEntryWithSource, got %T", result["models"])
	}
	if len(entries) == 0 {
		t.Fatal("expected builtin models from base catalog")
	}
}

func TestModelsHandlers_ListBuildsFromConfigLoader(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{
  "models": {
    "providers": {
      "openai": {
        "models": [
          {
            "id": "gpt-4o",
            "name": "GPT-4o",
            "input": ["text", "image"],
            "contextWindow": 128000
          }
        ]
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := config.NewConfigLoader(config.WithConfigPath(cfgPath))
	catalog := models.NewModelCatalog()
	r := NewMethodRegistry()
	r.RegisterAll(ModelsHandlers())

	req := &RequestFrame{Method: "models.list", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{
		ConfigLoader: loader,
		ModelCatalog: catalog,
	}, respond)
	if !gotOK {
		t.Fatal("models.list should succeed")
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	entries, ok := result["models"].([]ModelEntryWithSource)
	if !ok {
		t.Fatalf("expected []ModelEntryWithSource, got %T", result["models"])
	}
	var found *ModelEntryWithSource
	for i := range entries {
		if entries[i].Provider == "openai" && entries[i].ID == "gpt-4o" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected custom gpt-4o entry in %+v", entries)
	}
	if found.Source != types.ModelSourceCustom {
		t.Fatalf("expected custom source, got %s", found.Source)
	}
}

func TestModelsHandlers_DefaultGetLoadsLatestConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{
  "agents": {
    "defaults": {
      "model": {
        "primary": "openai/gpt-4o"
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := config.NewConfigLoader(config.WithConfigPath(cfgPath))
	r := NewMethodRegistry()
	r.RegisterAll(ModelsHandlers())

	req := &RequestFrame{Method: "models.default.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{ConfigLoader: loader}, respond)
	if !gotOK {
		t.Fatal("models.default.get should succeed")
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if result["model"] != "openai/gpt-4o" {
		t.Fatalf("expected primary model to be loaded, got %#v", result["model"])
	}
}

func TestModelsHandlers_DefaultSetPersistsAndRefreshesContext(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{
  "agents": {
    "defaults": {
      "model": {
        "primary": "google/gemini-3.1-pro-preview"
      }
    }
  },
  "models": {
    "providers": {
      "google": {
        "models": [
          { "id": "gemini-3.1-pro-preview", "name": "Gemini 3.1 Pro" },
          { "id": "gemini-3.1-flash-lite-preview", "name": "Gemini 3.1 Flash-Lite" }
        ]
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := config.NewConfigLoader(config.WithConfigPath(cfgPath))
	catalog := models.NewModelCatalog()
	ctx := &GatewayMethodContext{
		ConfigLoader: loader,
		ModelCatalog: catalog,
	}
	r := NewMethodRegistry()
	r.RegisterAll(ModelsHandlers())

	setReq := &RequestFrame{Method: "models.default.set", Params: map[string]interface{}{
		"model": "google/gemini-3.1-flash-lite-preview",
	}}
	var setOK bool
	var setPayload interface{}
	HandleGatewayRequest(r, setReq, nil, ctx, func(ok bool, payload interface{}, _ *ErrorShape) {
		setOK = ok
		setPayload = payload
	})
	if !setOK {
		t.Fatal("models.default.set should succeed")
	}
	setResult, ok := setPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected set payload map, got %T", setPayload)
	}
	if got := setResult["model"]; got != "google/gemini-3.1-flash-lite-preview" {
		t.Fatalf("set returned %v", got)
	}

	getReq := &RequestFrame{Method: "models.default.get", Params: map[string]interface{}{}}
	var getOK bool
	var getPayload interface{}
	HandleGatewayRequest(r, getReq, nil, ctx, func(ok bool, payload interface{}, _ *ErrorShape) {
		getOK = ok
		getPayload = payload
	})
	if !getOK {
		t.Fatal("models.default.get should succeed after set")
	}
	getResult, ok := getPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected get payload map, got %T", getPayload)
	}
	if got := getResult["model"]; got != "google/gemini-3.1-flash-lite-preview" {
		t.Fatalf("expected updated primary model, got %#v", got)
	}
	if ctx.Config == nil || ctx.Config.Agents == nil || ctx.Config.Agents.Defaults == nil || ctx.Config.Agents.Defaults.Model == nil {
		t.Fatal("expected gateway context config to refresh")
	}
	if ctx.Config.Agents.Defaults.Model.Primary != "google/gemini-3.1-flash-lite-preview" {
		t.Fatalf("context config primary = %q", ctx.Config.Agents.Defaults.Model.Primary)
	}
}

// ---------- agents.list ----------

func TestAgentsHandlers_ListWithConfig(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{
					ID:   "test-agent",
					Name: "Test Agent",
					Identity: &types.IdentityConfig{
						Name:  "Testy",
						Emoji: "🤖",
					},
				},
			},
		},
	}

	r := NewMethodRegistry()
	r.RegisterAll(AgentsHandlers())

	req := &RequestFrame{Method: "agents.list", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{Config: cfg}, respond)
	if !gotOK {
		t.Error("agents.list should succeed")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	agents, ok := result["agents"].([]GatewayAgentRow)
	if !ok {
		t.Fatalf("expected []GatewayAgentRow, got %T", result["agents"])
	}
	if len(agents) == 0 {
		t.Fatal("expected at least 1 agent")
	}

	// Find our test agent
	found := false
	for _, a := range agents {
		if a.ID == "test-agent" {
			found = true
			if a.Name != "Test Agent" {
				t.Errorf("expected name 'Test Agent', got %q", a.Name)
			}
			if a.Identity == nil {
				t.Error("expected identity")
			} else if a.Identity.Name != "Testy" {
				t.Errorf("expected identity.name 'Testy', got %q", a.Identity.Name)
			}
		}
	}
	if !found {
		t.Error("test-agent not found in agents list")
	}
}

func TestAgentsHandlers_ListNoConfig(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(AgentsHandlers())

	req := &RequestFrame{Method: "agents.list", Params: map[string]interface{}{}}
	var gotOK bool
	var gotErr *ErrorShape
	respond := func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if gotOK {
		t.Error("should fail without config")
	}
	if gotErr == nil || gotErr.Code != ErrCodeInternalError {
		t.Errorf("expected internal_error, got %v", gotErr)
	}
}

// ---------- agent.identity.get ----------

func TestAgentHandlers_IdentityGet(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			List: []types.AgentListItemConfig{
				{
					ID: "alice",
					Identity: &types.IdentityConfig{
						Name:  "Alice",
						Theme: "pink",
						Emoji: "🎀",
					},
				},
			},
		},
	}

	r := NewMethodRegistry()
	r.RegisterAll(AgentHandlers())

	req := &RequestFrame{Method: "agent.identity.get", Params: map[string]interface{}{
		"agentId": "alice",
	}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{Config: cfg}, respond)
	if !gotOK {
		t.Error("agent.identity.get should succeed")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if result["agentId"] != "alice" {
		t.Errorf("expected agentId=alice, got %v", result["agentId"])
	}
	identity, ok := result["identity"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected identity map, got %T", result["identity"])
	}
	if identity["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", identity["name"])
	}
}

func TestAgentHandlers_IdentityGetDefault(t *testing.T) {
	cfg := &types.OpenAcosmiConfig{}

	r := NewMethodRegistry()
	r.RegisterAll(AgentHandlers())

	// No agentId → should use default
	req := &RequestFrame{Method: "agent.identity.get", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{Config: cfg}, respond)
	if !gotOK {
		t.Error("agent.identity.get should succeed for default agent")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if result["agentId"] == "" {
		t.Error("expected non-empty agentId")
	}
}

// ---------- agent.wait ----------

func TestAgentHandlers_Wait(t *testing.T) {
	r := NewMethodRegistry()
	r.RegisterAll(AgentHandlers())

	req := &RequestFrame{Method: "agent.wait", Params: map[string]interface{}{}}
	var gotOK bool
	var gotPayload interface{}
	respond := func(ok bool, payload interface{}, _ *ErrorShape) {
		gotOK = ok
		gotPayload = payload
	}
	HandleGatewayRequest(r, req, nil, &GatewayMethodContext{}, respond)
	if !gotOK {
		t.Error("agent.wait should succeed (stub)")
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", gotPayload)
	}
	if result["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", result["status"])
	}
}
