package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/gateway"
	authstoretypes "github.com/Acosmi/ClawAcosmi/internal/goproviders/types"
	types "github.com/Acosmi/ClawAcosmi/pkg/types"
)

type fakeRuntime struct {
	closeCalls []string
}

func (f *fakeRuntime) Close(reason string) error {
	f.closeCalls = append(f.closeCalls, reason)
	return nil
}

type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "index.html" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }

func withStubbedDesktopAuthStore(t *testing.T, store *authstoretypes.AuthProfileStore) {
	t.Helper()
	previous := loadDesktopAuthProfileStore
	loadDesktopAuthProfileStore = func() *authstoretypes.AuthProfileStore { return store }
	t.Cleanup(func() {
		loadDesktopAuthProfileStore = previous
	})
}

func withStubbedDesktopRuntimeScaffold(t *testing.T) {
	t.Helper()
	previous := ensureDesktopRuntimeScaffold
	ensureDesktopRuntimeScaffold = func() error { return nil }
	t.Cleanup(func() {
		ensureDesktopRuntimeScaffold = previous
	})
}

func withStubbedDesktopBootstrapPersistenceHooks(t *testing.T) (*int, *int, *int) {
	t.Helper()
	updateCalls := 0
	pendingCalls := 0
	handoffCalls := 0

	prevUpdate := syncDesktopBootstrapUpdateState
	prevPending := finalizeDesktopBootstrapPendingUpdate
	prevHandoff := finalizeDesktopBootstrapInstallerHandoff

	syncDesktopBootstrapUpdateState = func(cfg *types.OpenAcosmiConfig) error {
		updateCalls++
		return nil
	}
	finalizeDesktopBootstrapPendingUpdate = func() error {
		pendingCalls++
		return nil
	}
	finalizeDesktopBootstrapInstallerHandoff = func() error {
		handoffCalls++
		return nil
	}
	t.Cleanup(func() {
		syncDesktopBootstrapUpdateState = prevUpdate
		finalizeDesktopBootstrapPendingUpdate = prevPending
		finalizeDesktopBootstrapInstallerHandoff = prevHandoff
	})
	return &updateCalls, &pendingCalls, &handoffCalls
}

func configuredDesktopConfig() *types.OpenAcosmiConfig {
	return &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{
					Primary: "google/gemini-3.1-pro-preview",
				},
			},
		},
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"google": {
					API:    types.ModelAPIGoogleGenerativeAI,
					APIKey: "test-key",
					Models: []types.ModelDefinitionConfig{
						{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro"},
					},
				},
			},
		},
	}
}

func TestResolveDesktopPort(t *testing.T) {
	port := 23456
	cfg := &types.OpenAcosmiConfig{
		Gateway: &types.GatewayConfig{Port: &port},
	}

	if got := resolveDesktopPort(cfg, 45678); got != 45678 {
		t.Fatalf("expected override port 45678, got %d", got)
	}
	if got := resolveDesktopPort(cfg, 0); got != 23456 {
		t.Fatalf("expected config port 23456, got %d", got)
	}
}

func TestBuildDesktopURL(t *testing.T) {
	if got := buildDesktopURL(19001, false); got != "http://127.0.0.1:19001/ui/" {
		t.Fatalf("unexpected desktop url: %s", got)
	}
	if got := buildDesktopURL(19001, true); got != "http://127.0.0.1:19001/ui/?onboarding=true" {
		t.Fatalf("unexpected onboarding url: %s", got)
	}
}

func TestResolveDesktopWindowURL(t *testing.T) {
	bootstrap := &desktopBootstrap{Port: 19001, NeedsOnboarding: true}

	if got := resolveDesktopWindowURL(bootstrap, true); got != "http://127.0.0.1:19001/ui/?onboarding=true" {
		t.Fatalf("force onboarding url = %q", got)
	}

	if got := resolveDesktopWindowURL(bootstrap, false); got != "http://127.0.0.1:19001/ui/" {
		t.Fatalf("default url = %q", got)
	}
}

func TestPrepareDesktopBootstrap_MissingConfigDoesNotForceOnboardingURL(t *testing.T) {
	withStubbedDesktopRuntimeScaffold(t)
	updateCalls, pendingCalls, handoffCalls := withStubbedDesktopBootstrapPersistenceHooks(t)

	fake := &fakeRuntime{}
	waitCalls := 0
	wait := func(port int, timeout time.Duration) bool {
		waitCalls++
		return waitCalls == 2
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		if opts.BootstrapProfile != gateway.GatewayBootstrapProfileReadonlyBootstrap {
			t.Fatalf("expected readonly bootstrap profile, got %q", opts.BootstrapProfile)
		}
		return fake, nil
	}

	bootstrap, err := prepareDesktopBootstrap(
		nil,
		"/tmp/missing.json",
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		wait,
		start,
		nil,
	)
	if err != nil {
		t.Fatalf("prepareDesktopBootstrap returned error: %v", err)
	}
	if !bootstrap.NeedsOnboarding {
		t.Fatal("expected needsOnboarding=true")
	}
	if bootstrap.URL != "http://127.0.0.1:19001/ui/" {
		t.Fatalf("expected base shell url, got %q", bootstrap.URL)
	}
	if *updateCalls != 0 || *pendingCalls != 0 || *handoffCalls != 0 {
		t.Fatalf("readonly bootstrap should skip persistent hooks, got update=%d pending=%d handoff=%d", *updateCalls, *pendingCalls, *handoffCalls)
	}
}

func TestPrepareDesktopBootstrap_ConfiguredRunsPersistentHooks(t *testing.T) {
	withStubbedDesktopRuntimeScaffold(t)
	updateCalls, pendingCalls, handoffCalls := withStubbedDesktopBootstrapPersistenceHooks(t)
	withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{})

	fake := &fakeRuntime{}
	waitCalls := 0
	wait := func(port int, timeout time.Duration) bool {
		waitCalls++
		return waitCalls == 2
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		if opts.BootstrapProfile != gateway.GatewayBootstrapProfileDefault {
			t.Fatalf("expected default bootstrap profile, got %q", opts.BootstrapProfile)
		}
		return fake, nil
	}

	bootstrap, err := prepareDesktopBootstrap(
		configuredDesktopConfig(),
		"/tmp/existing.json",
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		func(string) (os.FileInfo, error) { return fakeFileInfo{}, nil },
		wait,
		start,
		nil,
	)
	if err != nil {
		t.Fatalf("prepareDesktopBootstrap returned error: %v", err)
	}
	if bootstrap == nil {
		t.Fatal("expected bootstrap")
	}
	if *updateCalls != 1 || *pendingCalls != 1 || *handoffCalls != 1 {
		t.Fatalf("expected persistent hooks once, got update=%d pending=%d handoff=%d", *updateCalls, *pendingCalls, *handoffCalls)
	}
}

func TestNeedsOnboarding(t *testing.T) {
	withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{})

	notFound := func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	if !needsOnboarding("/tmp/missing.json", nil, notFound) {
		t.Fatal("expected missing config to require onboarding")
	}

	exists := func(string) (os.FileInfo, error) {
		return nil, nil
	}
	if !needsOnboarding("/tmp/existing.json", &types.OpenAcosmiConfig{}, exists) {
		t.Fatal("expected incomplete existing config to require onboarding")
	}
	if needsOnboarding("/tmp/existing.json", configuredDesktopConfig(), exists) {
		t.Fatal("expected complete existing config to skip onboarding")
	}
}

func TestNeedsOnboarding_AllowsOAuthStoreBackedProvider(t *testing.T) {
	withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{
		Profiles: map[string]map[string]interface{}{
			"google:oauth": {
				"type":     "oauth",
				"provider": "google",
				"access":   "token",
			},
		},
	})

	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "google/gemini-3.1-pro-preview"},
			},
		},
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"google": {
					API:    types.ModelAPIGoogleGenerativeAI,
					Auth:   types.ModelAuthOAuth,
					Models: []types.ModelDefinitionConfig{{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro"}},
				},
			},
		},
	}

	exists := func(string) (os.FileInfo, error) { return nil, nil }
	if needsOnboarding("/tmp/existing.json", cfg, exists) {
		t.Fatal("expected oauth-backed provider to skip onboarding")
	}
}

func TestNeedsOnboarding_AllowsPortalProfilesForRuntimeProviders(t *testing.T) {
	tests := []struct {
		name            string
		modelRef        string
		providerID      string
		profileProvider string
		modelID         string
		api             types.ModelApi
		baseURL         string
	}{
		{
			name:            "qwen",
			modelRef:        "qwen/qwen3.5-plus",
			providerID:      "qwen",
			profileProvider: "qwen-portal",
			modelID:         "qwen3.5-plus",
			api:             types.ModelAPIOpenAICompletions,
			baseURL:         "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
		{
			name:            "minimax",
			modelRef:        "minimax/MiniMax-M2.5",
			providerID:      "minimax",
			profileProvider: "minimax-portal",
			modelID:         "MiniMax-M2.5",
			api:             types.ModelAPIOpenAICompletions,
			baseURL:         "https://api.minimax.io/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{
				Profiles: map[string]map[string]interface{}{
					tt.profileProvider + ":oauth": {
						"type":     "oauth",
						"provider": tt.profileProvider,
						"access":   "token",
					},
				},
			})

			cfg := &types.OpenAcosmiConfig{
				Agents: &types.AgentsConfig{
					Defaults: &types.AgentDefaultsConfig{
						Model: &types.AgentModelListConfig{Primary: tt.modelRef},
					},
				},
				Models: &types.ModelsConfig{
					Providers: map[string]*types.ModelProviderConfig{
						tt.providerID: {
							API:     tt.api,
							BaseURL: tt.baseURL,
							Models:  []types.ModelDefinitionConfig{{ID: tt.modelID, Name: tt.modelID}},
						},
					},
				},
			}

			exists := func(string) (os.FileInfo, error) { return nil, nil }
			if needsOnboarding("/tmp/existing.json", cfg, exists) {
				t.Fatalf("expected portal oauth profile to satisfy runtime provider %s", tt.providerID)
			}
		})
	}
}

func TestNeedsOnboarding_AllowsCredentiallessLocalProvider(t *testing.T) {
	withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{})

	cfg := &types.OpenAcosmiConfig{
		Agents: &types.AgentsConfig{
			Defaults: &types.AgentDefaultsConfig{
				Model: &types.AgentModelListConfig{Primary: "ollama/llama3.3"},
			},
		},
		Models: &types.ModelsConfig{
			Providers: map[string]*types.ModelProviderConfig{
				"ollama": {
					API:     types.ModelAPIOpenAICompletions,
					BaseURL: "http://127.0.0.1:11434/v1",
					Models:  []types.ModelDefinitionConfig{{ID: "llama3.3", Name: "Llama 3.3"}},
				},
			},
		},
	}

	exists := func(string) (os.FileInfo, error) { return nil, nil }
	if needsOnboarding("/tmp/existing.json", cfg, exists) {
		t.Fatal("expected local provider to skip onboarding")
	}
}

func TestPrepareDesktopBootstrap_RejectsExistingGateway(t *testing.T) {
	withStubbedDesktopRuntimeScaffold(t)

	calledStart := false
	wait := func(port int, timeout time.Duration) bool {
		return true
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		calledStart = true
		return nil, nil
	}

	bootstrap, err := prepareDesktopBootstrap(
		&types.OpenAcosmiConfig{},
		"/tmp/missing.json",
		19001,
		defaultDesktopGatewayOptions(""),
		func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		wait,
		start,
		nil,
	)
	if err == nil {
		t.Fatal("expected existing gateway to be rejected")
	}
	if calledStart {
		t.Fatal("expected existing gateway attach to skip start")
	}
	if bootstrap != nil {
		t.Fatal("expected bootstrap to be nil on existing gateway rejection")
	}
}

func TestPrepareDesktopBootstrap_StartsGatewayAndBuildsURL(t *testing.T) {
	withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{})
	withStubbedDesktopRuntimeScaffold(t)

	fake := &fakeRuntime{}
	waitCalls := 0
	wait := func(port int, timeout time.Duration) bool {
		waitCalls++
		return waitCalls == 2
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		if opts.ControlUIIndex != "index.html" {
			t.Fatalf("expected default control UI index, got %q", opts.ControlUIIndex)
		}
		return fake, nil
	}

	bootstrap, err := prepareDesktopBootstrap(
		configuredDesktopConfig(),
		"/tmp/existing.json",
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		func(string) (os.FileInfo, error) { return nil, nil },
		wait,
		start,
		nil,
	)
	if err != nil {
		t.Fatalf("prepareDesktopBootstrap returned error: %v", err)
	}
	if bootstrap.AttachedExisting {
		t.Fatal("expected a newly started runtime")
	}
	if bootstrap.Runtime != fake {
		t.Fatal("expected runtime to be preserved")
	}
	if bootstrap.URL != "http://127.0.0.1:19001/ui/" {
		t.Fatalf("unexpected bootstrap url: %s", bootstrap.URL)
	}
}

func TestStartOrAttachGateway_TimeoutClosesRuntime(t *testing.T) {
	fake := &fakeRuntime{}
	wait := func(port int, timeout time.Duration) bool {
		return false
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		return fake, nil
	}

	runtime, attached, err := startOrAttachGateway(
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		time.Millisecond,
		time.Millisecond,
		wait,
		start,
	)
	if err == nil {
		t.Fatal("expected startup timeout error")
	}
	if attached {
		t.Fatal("expected attachedExisting=false")
	}
	if runtime != nil {
		t.Fatal("expected runtime to be nil on timeout")
	}
	if len(fake.closeCalls) != 1 {
		t.Fatalf("expected runtime.Close to be called once, got %d", len(fake.closeCalls))
	}
}

func TestPrepareDesktopBootstrap_ValidatesCallbacks(t *testing.T) {
	withStubbedDesktopRuntimeScaffold(t)

	_, err := prepareDesktopBootstrap(nil, "", 0, gateway.GatewayServerOptions{}, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when callbacks are missing")
	}
	if err.Error() != "desktop bootstrap requires wait callback" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareDesktopBootstrap_ProbeFailureClosesRuntime(t *testing.T) {
	withStubbedDesktopAuthStore(t, &authstoretypes.AuthProfileStore{})
	withStubbedDesktopRuntimeScaffold(t)

	fake := &fakeRuntime{}
	waitCalls := 0
	wait := func(port int, timeout time.Duration) bool {
		waitCalls++
		return waitCalls == 2
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		return fake, nil
	}
	probe := func(url string, timeout time.Duration) error {
		return os.ErrNotExist
	}

	_, err := prepareDesktopBootstrap(
		configuredDesktopConfig(),
		"/tmp/existing.json",
		19001,
		defaultDesktopGatewayOptions("/tmp/ui"),
		func(string) (os.FileInfo, error) { return nil, nil },
		wait,
		start,
		probe,
	)
	if err == nil {
		t.Fatal("expected probe failure")
	}
	if len(fake.closeCalls) != 1 {
		t.Fatalf("expected runtime.Close to be called once, got %d", len(fake.closeCalls))
	}
}

func TestStartOrAttachGateway_RequiresControlUIWhenStarting(t *testing.T) {
	wait := func(port int, timeout time.Duration) bool {
		return false
	}
	start := func(port int, opts gateway.GatewayServerOptions) (runtimeCloser, error) {
		t.Fatal("start should not be called without a control UI source")
		return nil, nil
	}

	_, _, err := startOrAttachGateway(
		19001,
		gateway.GatewayServerOptions{},
		time.Millisecond,
		time.Millisecond,
		wait,
		start,
	)
	if err == nil {
		t.Fatal("expected missing control UI source to fail")
	}
}

func TestEmbeddedDesktopGatewayOptions(t *testing.T) {
	opts, err := embeddedDesktopGatewayOptions(fstest.MapFS{
		"frontend/dist/index.html":    &fstest.MapFile{Data: []byte("<html>ok</html>")},
		"frontend/dist/assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}, "frontend/dist")
	if err != nil {
		t.Fatalf("embeddedDesktopGatewayOptions returned error: %v", err)
	}
	if opts.ControlUIFS == nil {
		t.Fatal("expected embedded control UI fs to be set")
	}
	if opts.ControlUIIndex != "index.html" {
		t.Fatalf("unexpected control UI index: %q", opts.ControlUIIndex)
	}
}

func TestEmbeddedDesktopGatewayOptions_RequiresExistingSubdir(t *testing.T) {
	_, err := embeddedDesktopGatewayOptions(fstest.MapFS{}, "frontend/dist")
	if err == nil {
		t.Fatal("expected missing embedded subdir to fail")
	}
}

func TestResolveDesktopGatewayOptions_PrefersOverride(t *testing.T) {
	opts := resolveDesktopGatewayOptions(nil, "/tmp/control-ui", nil)
	if opts.ControlUIDir != "/tmp/control-ui" {
		t.Fatalf("expected override control UI dir, got %q", opts.ControlUIDir)
	}
	if opts.ControlUIFS != nil {
		t.Fatal("expected override to skip embedded fs")
	}
}

func TestResolveDesktopGatewayOptions_UsesEmbeddedAssets(t *testing.T) {
	restore := desktopEmbeddedAssetsFunc
	desktopEmbeddedAssetsFunc = func() fs.FS {
		return fstest.MapFS{
			"frontend/dist/index.html": &fstest.MapFile{Data: []byte("<html>ok</html>")},
		}
	}
	defer func() {
		desktopEmbeddedAssetsFunc = restore
	}()

	opts := resolveDesktopGatewayOptions(nil, "", func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	if opts.ControlUIFS == nil {
		t.Fatal("expected embedded control UI fs to be selected")
	}
	if opts.ControlUIDir != "" {
		t.Fatalf("expected no control UI dir when embedded assets are available, got %q", opts.ControlUIDir)
	}
}

func TestResolveDesktopGatewayOptions_UsesConfiguredDiskRoot(t *testing.T) {
	restore := desktopEmbeddedAssetsFunc
	desktopEmbeddedAssetsFunc = func() fs.FS { return nil }
	defer func() {
		desktopEmbeddedAssetsFunc = restore
	}()

	cfg := &types.OpenAcosmiConfig{
		Gateway: &types.GatewayConfig{
			ControlUI: &types.GatewayControlUiConfig{Root: "/workspace/dist/control-ui"},
		},
	}
	opts := resolveDesktopGatewayOptions(cfg, "", func(name string) (os.FileInfo, error) {
		if filepath.Clean(name) == filepath.Clean("/workspace/dist/control-ui/index.html") {
			return fakeFileInfo{}, nil
		}
		return nil, os.ErrNotExist
	})
	if opts.ControlUIDir != "/workspace/dist/control-ui" {
		t.Fatalf("expected configured control UI dir, got %q", opts.ControlUIDir)
	}
}

func TestResolveDesktopGatewayOptions_LeavesSourceEmptyWhenUnavailable(t *testing.T) {
	restore := desktopEmbeddedAssetsFunc
	desktopEmbeddedAssetsFunc = func() fs.FS { return nil }
	defer func() {
		desktopEmbeddedAssetsFunc = restore
	}()

	opts := resolveDesktopGatewayOptions(nil, "", func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	if opts.ControlUIDir != "" || opts.ControlUIFS != nil {
		t.Fatal("expected no control UI source when nothing is available")
	}
	if opts.ControlUIIndex != "index.html" {
		t.Fatalf("unexpected control UI index: %q", opts.ControlUIIndex)
	}
}
