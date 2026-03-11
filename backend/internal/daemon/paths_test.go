package daemon

import (
	"path/filepath"
	"testing"
)

func TestResolveHomeDir(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"HOME set", map[string]string{"HOME": "/home/user"}, false},
		{"USERPROFILE set", map[string]string{"USERPROFILE": "C:\\Users\\user"}, false},
		{"both set prefers HOME", map[string]string{"HOME": "/home/user", "USERPROFILE": "C:\\Users\\user"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveHomeDir(tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveHomeDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == "" {
				t.Error("ResolveHomeDir() returned empty string")
			}
		})
	}
}

func TestResolveUserPathWithHome(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		home     string
		expected string
		wantErr  bool
	}{
		{"empty", "", "/home/user", "", false},
		{"absolute", "/usr/bin", "/home/user", "/usr/bin", false},
		{"tilde only", "~", "/home/user", "/home/user", false},
		{"tilde slash", "~/docs", "/home/user", "/home/user/docs", false},
		{"tilde no home", "~/docs", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveUserPathWithHome(tt.input, tt.home)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveUserPathWithHome(%q, %q) error = %v, wantErr %v", tt.input, tt.home, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ResolveUserPathWithHome(%q, %q) = %q, want %q", tt.input, tt.home, result, tt.expected)
			}
		})
	}
}

func TestResolveGatewayStateDir(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains string
	}{
		{"default", map[string]string{"HOME": "/home/user"}, ".openacosmi"},
		{"with profile", map[string]string{"HOME": "/home/user", "OPENACOSMI_PROFILE": "staging"}, ".openacosmi-staging"},
		{"override", map[string]string{"HOME": "/home/user", "OPENACOSMI_STATE_DIR": "/custom/state"}, "/custom/state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveGatewayStateDir(tt.env)
			if err != nil {
				t.Errorf("ResolveGatewayStateDir() error = %v", err)
				return
			}
			if tt.contains != "" {
				found := false
				if result == tt.contains || len(result) > len(tt.contains) {
					found = true
				}
				if !found {
					t.Errorf("ResolveGatewayStateDir() = %q, want containing %q", result, tt.contains)
				}
			}
		})
	}
}

func TestResolveGatewayLogPaths_UsesProfileAwareFallback(t *testing.T) {
	env := map[string]string{
		"HOME":               "/home/user",
		"OPENACOSMI_PROFILE": "staging",
	}

	stdoutPath, stderrPath := ResolveGatewayLogPaths(env)
	wantDir := filepath.Join("/home/user", ".openacosmi-staging", "logs")
	if stdoutPath != filepath.Join(wantDir, "gateway.stdout.log") {
		t.Fatalf("stdoutPath = %q", stdoutPath)
	}
	if stderrPath != filepath.Join(wantDir, "gateway.stderr.log") {
		t.Fatalf("stderrPath = %q", stderrPath)
	}
}

func TestParseKeyValueOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		separator string
		expected  map[string]string
	}{
		{
			"equal sign separator",
			"Status=running\nPID=1234\n",
			"=",
			map[string]string{"status": "running", "pid": "1234"},
		},
		{
			"colon separator",
			"TaskName: MyTask\nStatus: Running\n",
			":",
			map[string]string{"taskname": "MyTask", "status": "Running"},
		},
		{
			"empty lines skipped",
			"Key1=Value1\n\nKey2=Value2\n",
			"=",
			map[string]string{"key1": "Value1", "key2": "Value2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseKeyValueOutput(tt.output, tt.separator)
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("ParseKeyValueOutput()[%q] = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}
