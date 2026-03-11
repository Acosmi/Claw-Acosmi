//go:build darwin

package daemon

import (
	"path/filepath"
	"testing"
)

func TestResolveGatewayLogPathsDarwin_UsesProfileAwareFallback(t *testing.T) {
	env := map[string]string{
		"HOME":               "/Users/tester",
		"OPENACOSMI_PROFILE": "staging",
	}

	stdoutPath, stderrPath := ResolveGatewayLogPathsDarwin(env)
	wantDir := filepath.Join("/Users/tester", ".openacosmi-staging", "logs")
	if stdoutPath != filepath.Join(wantDir, "gateway.stdout.log") {
		t.Fatalf("stdoutPath = %q", stdoutPath)
	}
	if stderrPath != filepath.Join(wantDir, "gateway.stderr.log") {
		t.Fatalf("stderrPath = %q", stderrPath)
	}
}
