package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalconfig "github.com/Acosmi/ClawAcosmi/internal/config"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestConfigSet_ReturnsSnapshotAndVerification(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	if err := os.WriteFile(cfgPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := internalconfig.NewConfigLoader(internalconfig.WithConfigPath(cfgPath))
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	registry := NewMethodRegistry()
	registry.RegisterAll(ConfigHandlers())

	req := &RequestFrame{
		Method: "config.set",
		Params: map[string]interface{}{
			"raw":      `{"gateway":{"port":26223}}`,
			"baseHash": snapshot.Hash,
		},
	}

	var gotOK bool
	var gotErr *ErrorShape
	var gotPayload interface{}
	HandleGatewayRequest(registry, req, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, payload interface{}, err *ErrorShape) {
		gotOK = ok
		gotPayload = payload
		gotErr = err
	})

	if !gotOK {
		t.Fatalf("config.set failed: %+v", gotErr)
	}

	result, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", gotPayload)
	}

	validation, ok := result["validation"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected validation map, got %T", result["validation"])
	}
	if validation["ok"] != true {
		t.Fatalf("validation should succeed, got %#v", validation)
	}
	if validation["issueCount"] != 0 {
		t.Fatalf("validation issueCount=%v, want 0", validation["issueCount"])
	}

	verification, ok := result["verification"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected verification map, got %T", result["verification"])
	}
	if verification["configWritten"] != true {
		t.Fatalf("configWritten=%v, want true", verification["configWritten"])
	}
	if verification["runtimeEffect"] != "written_only" {
		t.Fatalf("runtimeEffect=%v, want written_only", verification["runtimeEffect"])
	}
	if verification["restartScheduled"] != false {
		t.Fatalf("restartScheduled=%v, want false", verification["restartScheduled"])
	}

	redactedSnapshot, ok := result["snapshot"].(*types.ConfigFileSnapshot)
	if !ok {
		t.Fatalf("expected redacted snapshot, got %T", result["snapshot"])
	}
	if redactedSnapshot.Hash == "" {
		t.Fatal("snapshot hash should not be empty")
	}
	if gotHash, _ := result["hash"].(string); gotHash != redactedSnapshot.Hash {
		t.Fatalf("result hash=%q, snapshot hash=%q", gotHash, redactedSnapshot.Hash)
	}
}

func TestConfigPatch_ReturnsValidationDetails(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	if err := os.WriteFile(cfgPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := internalconfig.NewConfigLoader(internalconfig.WithConfigPath(cfgPath))
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	registry := NewMethodRegistry()
	registry.RegisterAll(ConfigHandlers())

	req := &RequestFrame{
		Method: "config.patch",
		Params: map[string]interface{}{
			"raw":      `{"browser":{"profiles":{"work":{}}}}`,
			"baseHash": snapshot.Hash,
		},
	}

	var gotOK bool
	var gotErr *ErrorShape
	HandleGatewayRequest(registry, req, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	})

	if gotOK {
		t.Fatal("config.patch should fail for invalid browser profile")
	}
	if gotErr == nil {
		t.Fatal("expected error shape")
	}
	if gotErr.Code != ErrCodeConfigInvalid {
		t.Fatalf("error code=%q, want %q", gotErr.Code, ErrCodeConfigInvalid)
	}
	if !strings.Contains(gotErr.Message, "browser.profiles.work") {
		t.Fatalf("error message should include field path, got %q", gotErr.Message)
	}

	details, ok := gotErr.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("expected details map, got %T", gotErr.Details)
	}
	if details["ok"] != false {
		t.Fatalf("details ok=%v, want false", details["ok"])
	}
	if details["issueCount"] == 0 {
		t.Fatalf("issueCount=%v, want >0", details["issueCount"])
	}
	if !strings.Contains(details["summary"].(string), "browser.profiles.work") {
		t.Fatalf("summary should include invalid path, got %q", details["summary"])
	}
}

func TestConfigApply_RequiresBaseHashWhenSnapshotExists(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "openacosmi.json")
	if err := os.WriteFile(cfgPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader := internalconfig.NewConfigLoader(internalconfig.WithConfigPath(cfgPath))
	snapshot, err := loader.ReadConfigFileSnapshot()
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	registry := NewMethodRegistry()
	registry.RegisterAll(ConfigHandlers())

	req := &RequestFrame{
		Method: "config.apply",
		Params: map[string]interface{}{
			"raw": `{"gateway":{"port":26223}}`,
		},
	}

	var gotOK bool
	var gotErr *ErrorShape
	HandleGatewayRequest(registry, req, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, _ interface{}, err *ErrorShape) {
		gotOK = ok
		gotErr = err
	})

	if gotOK {
		t.Fatal("config.apply should require baseHash when config exists")
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
	if details["expectedHash"] != snapshot.Hash {
		t.Fatalf("expectedHash=%v, want %q", details["expectedHash"], snapshot.Hash)
	}
}
