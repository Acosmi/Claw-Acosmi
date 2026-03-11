package gateway

import (
	"path/filepath"
	"testing"

	internalconfig "github.com/Acosmi/ClawAcosmi/internal/config"
	internalmedia "github.com/Acosmi/ClawAcosmi/internal/media"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }

func newSpecializedConfigLoader(t *testing.T, cfg *types.OpenAcosmiConfig) (*internalconfig.ConfigLoader, *types.ConfigFileSnapshot) {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	loader := internalconfig.NewConfigLoader(internalconfig.WithConfigPath(cfgPath))
	if err := loader.WriteConfigFile(cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	return loader, snapshot
}

func callGatewayMethodForTest(
	t *testing.T,
	handlers map[string]GatewayMethodHandler,
	method string,
	params map[string]interface{},
	ctx *GatewayMethodContext,
) (bool, interface{}, *ErrorShape) {
	t.Helper()

	if params == nil {
		params = map[string]interface{}{}
	}
	if ctx == nil {
		ctx = &GatewayMethodContext{}
	}

	registry := NewMethodRegistry()
	registry.RegisterAll(handlers)

	req := &RequestFrame{Method: method, Params: params}
	var gotOK bool
	var gotPayload interface{}
	var gotErr *ErrorShape
	HandleGatewayRequest(registry, req, nil, ctx, func(ok bool, payload interface{}, err *ErrorShape) {
		gotOK = ok
		gotPayload = payload
		gotErr = err
	})
	return gotOK, gotPayload, gotErr
}

func TestToolsBrowserGetAndSet_UseConfigHashAndPreserveFields(t *testing.T) {
	loader, snapshot := newSpecializedConfigLoader(t, &types.OpenAcosmiConfig{
		Browser: &types.BrowserConfig{
			Enabled: boolPtr(false),
			CdpURL:  "ws://127.0.0.1:9222/devtools/browser/original",
			Profiles: map[string]*types.BrowserProfileConfig{
				"default": {CdpPort: intPtr(9222)},
			},
		},
	})
	ctx := &GatewayMethodContext{ConfigLoader: loader}

	gotOK, gotPayload, gotErr := callGatewayMethodForTest(t, PluginsHandlers(), "tools.browser.get", nil, ctx)
	if !gotOK {
		t.Fatalf("tools.browser.get failed: %+v", gotErr)
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	if gotHash, _ := result["hash"].(string); gotHash != snapshot.Hash {
		t.Fatalf("tools.browser.get hash=%q, want %q", gotHash, snapshot.Hash)
	}

	gotOK, gotPayload, gotErr = callGatewayMethodForTest(t, PluginsHandlers(), "tools.browser.set", map[string]interface{}{
		"baseHash": snapshot.Hash,
		"enabled":  true,
	}, ctx)
	if !gotOK {
		t.Fatalf("tools.browser.set failed: %+v", gotErr)
	}
	writeResult, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	verification, ok := writeResult["verification"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected verification map, got %T", writeResult["verification"])
	}
	if verification["baseHashChecked"] != true {
		t.Fatalf("baseHashChecked=%v, want true", verification["baseHashChecked"])
	}

	freshCfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if freshCfg.Browser == nil || freshCfg.Browser.Enabled == nil || !*freshCfg.Browser.Enabled {
		t.Fatalf("browser enabled not updated: %+v", freshCfg.Browser)
	}
	if freshCfg.Browser.CdpURL != "ws://127.0.0.1:9222/devtools/browser/original" {
		t.Fatalf("browser cdpUrl lost during merge: %q", freshCfg.Browser.CdpURL)
	}
	if freshCfg.Browser.Profiles["default"] == nil || freshCfg.Browser.Profiles["default"].CdpPort == nil || *freshCfg.Browser.Profiles["default"].CdpPort != 9222 {
		t.Fatalf("browser profiles were not preserved: %+v", freshCfg.Browser.Profiles)
	}
}

func TestImageConfigGetAndSet_PreserveSecret(t *testing.T) {
	loader, snapshot := newSpecializedConfigLoader(t, &types.OpenAcosmiConfig{
		ImageUnderstanding: &types.ImageUnderstandingConfig{
			Provider:  "openai",
			APIKey:    "image-secret",
			Model:     "gpt-4o-mini",
			BaseURL:   "https://api.example.com",
			MaxTokens: 256,
		},
	})
	ctx := &GatewayMethodContext{ConfigLoader: loader}

	gotOK, gotPayload, gotErr := callGatewayMethodForTest(t, ImageHandlers(), "image.config.get", nil, ctx)
	if !gotOK {
		t.Fatalf("image.config.get failed: %+v", gotErr)
	}
	result, ok := gotPayload.(ImageConfigGetResult)
	if !ok {
		t.Fatalf("expected ImageConfigGetResult, got %T", gotPayload)
	}
	if result.Hash != snapshot.Hash {
		t.Fatalf("image.config.get hash=%q, want %q", result.Hash, snapshot.Hash)
	}

	gotOK, _, gotErr = callGatewayMethodForTest(t, ImageHandlers(), "image.config.set", map[string]interface{}{
		"baseHash": snapshot.Hash,
		"model":    "gpt-4.1-mini",
	}, ctx)
	if !gotOK {
		t.Fatalf("image.config.set failed: %+v", gotErr)
	}

	freshCfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if freshCfg.ImageUnderstanding == nil {
		t.Fatal("image understanding config missing after write")
	}
	if freshCfg.ImageUnderstanding.APIKey != "image-secret" {
		t.Fatalf("image apiKey should be preserved, got %q", freshCfg.ImageUnderstanding.APIKey)
	}
	if freshCfg.ImageUnderstanding.Model != "gpt-4.1-mini" {
		t.Fatalf("image model=%q, want gpt-4.1-mini", freshCfg.ImageUnderstanding.Model)
	}
}

func TestSTTConfigGetAndSet_PreserveSecret(t *testing.T) {
	loader, snapshot := newSpecializedConfigLoader(t, &types.OpenAcosmiConfig{
		STT: &types.STTConfig{
			Provider: "openai",
			APIKey:   "stt-secret",
			Model:    "whisper-1",
			BaseURL:  "https://api.example.com",
			Language: "zh",
		},
	})
	ctx := &GatewayMethodContext{ConfigLoader: loader}

	gotOK, gotPayload, gotErr := callGatewayMethodForTest(t, STTHandlers(), "stt.config.get", nil, ctx)
	if !gotOK {
		t.Fatalf("stt.config.get failed: %+v", gotErr)
	}
	result, ok := gotPayload.(STTConfigGetResult)
	if !ok {
		t.Fatalf("expected STTConfigGetResult, got %T", gotPayload)
	}
	if result.Hash != snapshot.Hash {
		t.Fatalf("stt.config.get hash=%q, want %q", result.Hash, snapshot.Hash)
	}

	gotOK, _, gotErr = callGatewayMethodForTest(t, STTHandlers(), "stt.config.set", map[string]interface{}{
		"baseHash": snapshot.Hash,
		"language": "en",
	}, ctx)
	if !gotOK {
		t.Fatalf("stt.config.set failed: %+v", gotErr)
	}

	freshCfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if freshCfg.STT == nil {
		t.Fatal("stt config missing after write")
	}
	if freshCfg.STT.APIKey != "stt-secret" {
		t.Fatalf("stt apiKey should be preserved, got %q", freshCfg.STT.APIKey)
	}
	if freshCfg.STT.Language != "en" {
		t.Fatalf("stt language=%q, want en", freshCfg.STT.Language)
	}
}

func TestDocConvConfigGetAndSet_PreserveFields(t *testing.T) {
	loader, snapshot := newSpecializedConfigLoader(t, &types.OpenAcosmiConfig{
		DocConv: &types.DocConvConfig{
			Provider:   "builtin",
			PandocPath: "/usr/local/bin/pandoc",
			UseSandbox: boolPtr(true),
		},
	})
	ctx := &GatewayMethodContext{ConfigLoader: loader}

	gotOK, gotPayload, gotErr := callGatewayMethodForTest(t, DocConvHandlers(), "docconv.config.get", nil, ctx)
	if !gotOK {
		t.Fatalf("docconv.config.get failed: %+v", gotErr)
	}
	result, ok := gotPayload.(DocConvConfigGetResult)
	if !ok {
		t.Fatalf("expected DocConvConfigGetResult, got %T", gotPayload)
	}
	if result.Hash != snapshot.Hash {
		t.Fatalf("docconv.config.get hash=%q, want %q", result.Hash, snapshot.Hash)
	}

	gotOK, _, gotErr = callGatewayMethodForTest(t, DocConvHandlers(), "docconv.config.set", map[string]interface{}{
		"baseHash":     snapshot.Hash,
		"provider":     "mcp",
		"mcpCommand":   "npx -y mcp-pandoc",
		"mcpTransport": "stdio",
	}, ctx)
	if !gotOK {
		t.Fatalf("docconv.config.set failed: %+v", gotErr)
	}

	freshCfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if freshCfg.DocConv == nil {
		t.Fatal("docconv config missing after write")
	}
	if freshCfg.DocConv.PandocPath != "/usr/local/bin/pandoc" {
		t.Fatalf("docconv pandocPath lost during merge: %q", freshCfg.DocConv.PandocPath)
	}
	if freshCfg.DocConv.Provider != "mcp" {
		t.Fatalf("docconv provider=%q, want mcp", freshCfg.DocConv.Provider)
	}
	if freshCfg.DocConv.MCPCommand != "npx -y mcp-pandoc" {
		t.Fatalf("docconv mcpCommand=%q", freshCfg.DocConv.MCPCommand)
	}
}

func TestMediaConfigGetAndUpdate_UseConfigHashAndStructuredVerification(t *testing.T) {
	loader, snapshot := newSpecializedConfigLoader(t, &types.OpenAcosmiConfig{
		SubAgents: &types.SubAgentConfig{
			MediaAgent: &types.MediaAgentSettings{
				AutoSpawnEnabled:    false,
				MaxAutoSpawnsPerDay: 5,
			},
		},
	})
	subsystem, err := internalmedia.NewMediaSubsystem(internalmedia.MediaSubsystemConfig{
		Workspace:      t.TempDir(),
		EnablePublish:  false,
		EnableInteract: false,
		EnabledSources: []string{},
	})
	if err != nil {
		t.Fatalf("new media subsystem: %v", err)
	}
	ctx := &GatewayMethodContext{
		ConfigLoader:   loader,
		MediaSubsystem: subsystem,
	}

	gotOK, gotPayload, gotErr := callGatewayMethodForTest(t, MediaHandlers(), "media.config.get", nil, ctx)
	if !gotOK {
		t.Fatalf("media.config.get failed: %+v", gotErr)
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	if gotHash, _ := result["hash"].(string); gotHash != snapshot.Hash {
		t.Fatalf("media.config.get hash=%q, want %q", gotHash, snapshot.Hash)
	}

	gotOK, gotPayload, gotErr = callGatewayMethodForTest(t, MediaHandlers(), "media.config.update", map[string]interface{}{
		"baseHash":            snapshot.Hash,
		"autoSpawnEnabled":    true,
		"maxAutoSpawnsPerDay": float64(9),
	}, ctx)
	if !gotOK {
		t.Fatalf("media.config.update failed: %+v", gotErr)
	}
	writeResult, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	verification, ok := writeResult["verification"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected verification map, got %T", writeResult["verification"])
	}
	if verification["baseHashChecked"] != true {
		t.Fatalf("baseHashChecked=%v, want true", verification["baseHashChecked"])
	}
	if verification["mediaSubsystemSynced"] != true {
		t.Fatalf("mediaSubsystemSynced=%v, want true", verification["mediaSubsystemSynced"])
	}

	freshCfg, err := loader.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if freshCfg.SubAgents == nil || freshCfg.SubAgents.MediaAgent == nil {
		t.Fatal("media config missing after update")
	}
	if !freshCfg.SubAgents.MediaAgent.AutoSpawnEnabled {
		t.Fatal("media autoSpawnEnabled should be true after update")
	}
	if freshCfg.SubAgents.MediaAgent.MaxAutoSpawnsPerDay != 9 {
		t.Fatalf("media maxAutoSpawnsPerDay=%d, want 9", freshCfg.SubAgents.MediaAgent.MaxAutoSpawnsPerDay)
	}
}

func TestRemoteApprovalConfigGetAndSet_UseDedicatedHashAndPreserveFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	notifier := &RemoteApprovalNotifier{
		config: RemoteApprovalConfig{
			Enabled:     true,
			CallbackURL: "https://example.com/callback",
			Feishu: &FeishuProviderConfig{
				Enabled:        true,
				AppID:          "cli_app",
				AppSecret:      "remote-secret",
				ApprovalChatID: "approval-chat",
			},
		},
	}
	ctx := &GatewayMethodContext{RemoteApprovalNotifier: notifier}

	gotOK, gotPayload, gotErr := callGatewayMethodForTest(t, RemoteApprovalHandlers(), "security.remoteApproval.config.get", nil, ctx)
	if !gotOK {
		t.Fatalf("security.remoteApproval.config.get failed: %+v", gotErr)
	}
	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	baseHash, _ := result["hash"].(string)
	if baseHash == "" {
		t.Fatal("remote approval get should return dedicated hash")
	}
	feishuCfg, ok := result["feishu"].(*FeishuProviderConfig)
	if !ok {
		t.Fatalf("expected sanitized feishu config, got %T", result["feishu"])
	}
	if feishuCfg.AppSecret != "***" {
		t.Fatalf("expected remote approval secret to be masked, got %q", feishuCfg.AppSecret)
	}

	gotOK, gotPayload, gotErr = callGatewayMethodForTest(t, RemoteApprovalHandlers(), "security.remoteApproval.config.set", map[string]interface{}{
		"baseHash":    baseHash,
		"callbackUrl": "https://example.com/new-callback",
		"feishu": map[string]interface{}{
			"chatId": "chat-2",
		},
	}, ctx)
	if !gotOK {
		t.Fatalf("security.remoteApproval.config.set failed: %+v", gotErr)
	}
	writeResult, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}
	verification, ok := writeResult["verification"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected verification map, got %T", writeResult["verification"])
	}
	if verification["baseHashChecked"] != true {
		t.Fatalf("baseHashChecked=%v, want true", verification["baseHashChecked"])
	}

	currentCfg := notifier.GetConfig()
	if currentCfg.CallbackURL != "https://example.com/new-callback" {
		t.Fatalf("callbackUrl=%q", currentCfg.CallbackURL)
	}
	if currentCfg.Feishu == nil {
		t.Fatal("feishu config missing after update")
	}
	if currentCfg.Feishu.AppSecret != "remote-secret" {
		t.Fatalf("remote approval secret should be preserved, got %q", currentCfg.Feishu.AppSecret)
	}
	if currentCfg.Feishu.ApprovalChatID != "approval-chat" {
		t.Fatalf("approvalChatId should be preserved, got %q", currentCfg.Feishu.ApprovalChatID)
	}
	if currentCfg.Feishu.ChatID != "chat-2" {
		t.Fatalf("chatId=%q, want chat-2", currentCfg.Feishu.ChatID)
	}
}

func TestRemoteApprovalConfigSet_RejectsStaleBaseHash(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	notifier := &RemoteApprovalNotifier{
		config: RemoteApprovalConfig{
			Enabled:     true,
			CallbackURL: "https://example.com/callback",
		},
	}
	ctx := &GatewayMethodContext{RemoteApprovalNotifier: notifier}

	gotOK, _, gotErr := callGatewayMethodForTest(t, RemoteApprovalHandlers(), "security.remoteApproval.config.set", map[string]interface{}{
		"baseHash": "stale-hash",
		"enabled":  true,
	}, ctx)
	if gotOK {
		t.Fatal("security.remoteApproval.config.set should reject stale baseHash")
	}
	if gotErr == nil {
		t.Fatal("expected error shape")
	}
	if gotErr.Code != ErrCodeBadRequest {
		t.Fatalf("error code=%q, want %q", gotErr.Code, ErrCodeBadRequest)
	}
	details, ok := gotErr.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("expected details map, got %T", gotErr.Details)
	}
	if details["expectedHash"] != hashJSONValue(notifier.GetConfig()) {
		t.Fatalf("expectedHash=%v", details["expectedHash"])
	}
}
