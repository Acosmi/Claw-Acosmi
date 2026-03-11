package statepaths

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	newStateDirname           = ".openacosmi"
	compatibilityStateDirname = ".crabclaw"
	configFilename            = "openacosmi.json"
	oauthFilename             = "oauth.json"
	defaultAgentID            = "main"
)

var (
	legacyStateDirnames = []string{".openclaw", ".clawdbot", ".moltbot", ".moldbot"}
	validIDRE           = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	invalidCharsRE      = regexp.MustCompile(`[^a-z0-9_-]+`)
	leadingDashRE       = regexp.MustCompile(`^-+`)
	trailingDashRE      = regexp.MustCompile(`-+$`)
)

func ResolveHomeDir() string {
	if v := envTrimmedCompat("CRABCLAW_HOME", "OPENACOSMI_HOME"); v != "" {
		return expandTilde(v)
	}
	if v := envTrimmed("OPENCLAW_HOME"); v != "" {
		return expandTilde(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func NormalizeProfileName(profile string) string {
	trimmed := strings.TrimSpace(profile)
	if trimmed == "" || strings.EqualFold(trimmed, "default") {
		return ""
	}
	return trimmed
}

func ResolveProfileSuffix() string {
	normalized := NormalizeProfileName(envTrimmedCompat("CRABCLAW_PROFILE", "OPENACOSMI_PROFILE"))
	if normalized == "" {
		return ""
	}
	return "-" + normalized
}

func ResolveStateDir() string {
	if v := envTrimmedCompat("CRABCLAW_STATE_DIR", "OPENACOSMI_STATE_DIR"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("OPENCLAW_STATE_DIR"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("CLAWDBOT_STATE_DIR"); v != "" {
		return resolveUserPath(v)
	}

	home := ResolveHomeDir()
	suffix := ResolveProfileSuffix()
	compatDir := filepath.Join(home, compatibilityStateDirname+suffix)
	newDir := filepath.Join(home, newStateDirname+suffix)

	if stateDirHasManagedContent(compatDir) {
		return compatDir
	}
	if dirExists(newDir) {
		return newDir
	}
	if dirExists(compatDir) {
		return compatDir
	}
	for _, name := range legacyStateDirnames {
		candidate := filepath.Join(home, name+suffix)
		if dirExists(candidate) {
			return candidate
		}
	}
	return newDir
}

func ResolveOAuthDir() string {
	if v := envTrimmedCompat("CRABCLAW_OAUTH_DIR", "OPENACOSMI_OAUTH_DIR"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("OPENCLAW_OAUTH_DIR"); v != "" {
		return resolveUserPath(v)
	}
	return filepath.Join(ResolveStateDir(), "credentials")
}

func ResolveStoreDir() string {
	if v := envTrimmedCompat("CRABCLAW_STORE_PATH", "OPENACOSMI_STORE_PATH"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("OPENCLAW_STORE_PATH"); v != "" {
		return resolveUserPath(v)
	}
	if v := envTrimmed("CLAWDBOT_STORE_PATH"); v != "" {
		return resolveUserPath(v)
	}
	return filepath.Join(ResolveStateDir(), "store")
}

func ResolveRuntimeStateDir() string {
	return filepath.Join(ResolveStateDir(), "state")
}

func ResolveRuntimeAgentsRoot() string {
	return filepath.Join(ResolveRuntimeStateDir(), "agents")
}

func ResolveRuntimeAgentDir(agentID string) string {
	return filepath.Join(ResolveRuntimeAgentsRoot(), NormalizeAgentID(agentID), "agent")
}

func ResolveDefaultRuntimeAgentDir() string {
	return ResolveRuntimeAgentDir(defaultAgentID)
}

func NormalizeAgentID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultAgentID
	}
	lower := strings.ToLower(trimmed)
	if validIDRE.MatchString(lower) {
		return lower
	}
	result := invalidCharsRE.ReplaceAllString(lower, "-")
	result = leadingDashRE.ReplaceAllString(result, "")
	result = trailingDashRE.ReplaceAllString(result, "")
	if len(result) > 64 {
		result = result[:64]
	}
	if result == "" {
		return defaultAgentID
	}
	return result
}

func envTrimmed(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envTrimmedCompat(keys ...string) string {
	for _, key := range keys {
		if v := envTrimmed(key); v != "" {
			return v
		}
	}
	return ""
}

func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

func resolveUserPath(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "~") {
		return filepath.Clean(expandTilde(trimmed))
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return trimmed
	}
	return abs
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func stateDirHasManagedContent(dir string) bool {
	if !dirExists(dir) {
		return false
	}

	markers := []string{
		configFilename,
		"credentials",
		"state",
		"store",
		"sessions",
		"agents",
		"memory",
		"_media",
		"workspace",
		"extensions",
		"logs",
		"exec-approvals.json",
		"gateway-token",
		"relay-token",
		"desktop-update-state.json",
		oauthFilename,
	}
	for _, marker := range markers {
		p := filepath.Join(dir, marker)
		if dirExists(p) || fileExists(p) {
			return true
		}
	}
	return false
}
