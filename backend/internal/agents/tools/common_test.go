package tools

import (
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/agents/scope"
)

// ---------- schema.go ----------

func TestNormalizeToolParameters_PlainObject(t *testing.T) {
	tool := &AgentTool{
		Name: "test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "minLength": 1},
			},
		},
	}
	result := NormalizeToolParameters(tool)
	params := result.Parameters.(map[string]any)
	// minLength should be cleaned by Gemini cleaner
	props := params["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if _, ok := name["minLength"]; ok {
		t.Error("minLength should be removed")
	}
}

func TestNormalizeToolParameters_AnyOfMerge(t *testing.T) {
	tool := &AgentTool{
		Name: "test",
		Parameters: map[string]any{
			"anyOf": []any{
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{"type": "string", "enum": []any{"get"}},
						"key":    map[string]any{"type": "string"},
					},
					"required": []any{"action", "key"},
				},
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"action": map[string]any{"type": "string", "enum": []any{"set"}},
						"key":    map[string]any{"type": "string"},
						"value":  map[string]any{"type": "string"},
					},
					"required": []any{"action", "key", "value"},
				},
			},
		},
	}
	result := NormalizeToolParameters(tool)
	params := result.Parameters.(map[string]any)
	if params["type"] != "object" {
		t.Errorf("type = %v, want object", params["type"])
	}
	props := params["properties"].(map[string]any)
	if _, ok := props["action"]; !ok {
		t.Error("action should be merged")
	}
	if _, ok := props["key"]; !ok {
		t.Error("key should be merged")
	}
	if _, ok := props["value"]; !ok {
		t.Error("value should be merged")
	}
}

func TestNormalizeToolParameters_Nil(t *testing.T) {
	result := NormalizeToolParameters(nil)
	if result != nil {
		t.Error("nil → nil")
	}
}

// ---------- policy.go ----------

func TestCompilePattern_Exact(t *testing.T) {
	p := CompilePattern("exec")
	if !p.Matches("exec") {
		t.Error("exact match should succeed")
	}
	if p.Matches("execute") {
		t.Error("should not match non-exact")
	}
}

func TestCompilePattern_Wildcard(t *testing.T) {
	p := CompilePattern("cron*")
	if !p.Matches("cron") {
		t.Error("should match cron")
	}
	if !p.Matches("cron_list") {
		t.Error("should match cron_list")
	}
	if p.Matches("acron") {
		t.Error("should not match acron")
	}
}

func TestCompilePattern_Group(t *testing.T) {
	p := CompilePattern("group:memory")
	if !p.Matches("memory_search") {
		t.Error("should match memory_search")
	}
	if !p.Matches("memory_get") {
		t.Error("should match memory_get")
	}
	if p.Matches("exec") {
		t.Error("should not match exec")
	}
}

func TestToolPolicyMatcher(t *testing.T) {
	policy := &scope.ToolPolicy{
		Allow: []string{"group:fs", "exec"},
		Deny:  []string{"write"},
	}
	matcher := MakeToolPolicyMatcher(policy)

	if !matcher.IsAllowed("read") {
		t.Error("read should be allowed (in group:fs)")
	}
	if !matcher.IsAllowed("exec") {
		t.Error("exec should be allowed")
	}
	if matcher.IsAllowed("write") {
		t.Error("write should be denied")
	}
	if matcher.IsAllowed("gateway") {
		t.Error("gateway should not be in allow list")
	}
}

func TestToolPolicyMatcher_AllowAll(t *testing.T) {
	matcher := MakeToolPolicyMatcher(nil)
	if !matcher.IsAllowed("anything") {
		t.Error("nil policy should allow all")
	}
}

func TestFilterToolsByPolicy(t *testing.T) {
	tools := []*AgentTool{
		{Name: "exec"},
		{Name: "read"},
		{Name: "gateway"},
	}
	layers := []PolicyLayer{
		{Name: "test", Matcher: MakeToolPolicyMatcher(&scope.ToolPolicy{
			Allow: []string{"exec", "read"},
		})},
	}
	filtered := FilterToolsByPolicy(tools, layers)
	if len(filtered) != 2 {
		t.Errorf("filtered = %d, want 2", len(filtered))
	}
}

// ---------- display.go ----------

func TestResolveToolDisplay_Known(t *testing.T) {
	d := ResolveToolDisplay("read", map[string]any{"path": "/etc/hosts"})
	if d.Emoji != "📖" {
		t.Errorf("emoji = %q", d.Emoji)
	}
	if d.Detail != "/etc/hosts" {
		t.Errorf("detail = %q", d.Detail)
	}
}

func TestResolveToolDisplay_Unknown(t *testing.T) {
	d := ResolveToolDisplay("my_custom_tool", nil)
	if d.Emoji != "🧩" {
		t.Errorf("emoji = %q", d.Emoji)
	}
	if d.Title != "My Custom Tool" {
		t.Errorf("title = %q", d.Title)
	}
}

func TestFormatToolSummary(t *testing.T) {
	s := FormatToolSummary("read", map[string]any{"path": "/etc/hosts"})
	if s == "" {
		t.Error("summary should not be empty")
	}
}

// ---------- callid.go ----------

func TestSanitizeToolCallID_Strict(t *testing.T) {
	// 有效 → 不变
	id := SanitizeToolCallID("call_123", IDModeStrict)
	if id != "call_123" {
		t.Errorf("valid strict ID changed: %q", id)
	}

	// 含非法字符
	id = SanitizeToolCallID("call:123!@#", IDModeStrict)
	if id == "call:123!@#" {
		t.Error("should sanitize invalid chars")
	}

	// 空 → 生成随机
	id = SanitizeToolCallID("", IDModeStrict)
	if id == "" {
		t.Error("empty should generate random")
	}
}

func TestSanitizeToolCallID_Strict9(t *testing.T) {
	id := SanitizeToolCallID("very_long_tool_call_id", IDModeStrict9)
	if len(id) > 9 {
		t.Errorf("strict9 should be ≤9 chars, got %d: %q", len(id), id)
	}
}

func TestMakeUniqueToolID(t *testing.T) {
	existing := map[string]bool{}
	for i := 0; i < 20; i++ {
		id := MakeUniqueToolID("test", existing, IDModeDefault)
		if existing[id] {
			t.Fatalf("collision at %d: %q", i, id)
		}
		existing[id] = true
	}
}

// ---------- images.go ----------

func TestSanitizeToolResultImages_SizeLimit(t *testing.T) {
	result := &AgentToolResult{
		Content: []ContentBlock{
			{Type: "text", Text: "hello"},
			{Type: "image", Data: "iVBORw==", MimeType: "image/png"},
		},
	}
	sanitized := SanitizeToolResultImages(result, 4096, DefaultMaxImageBytes)
	if len(sanitized.Content) != 2 {
		t.Errorf("should preserve 2 blocks, got %d", len(sanitized.Content))
	}
}

func TestSanitizeToolResultImages_OversizeRemoval(t *testing.T) {
	// 生成一个超大数据块 (模拟超限)
	result := &AgentToolResult{
		Content: []ContentBlock{
			{Type: "image", Data: "abc", MimeType: "image/png"},
		},
	}
	sanitized := SanitizeToolResultImages(result, 4096, 1) // 1 byte limit
	if len(sanitized.Content) != 1 || sanitized.Content[0].Type != "text" {
		t.Errorf("oversize image should be replaced with text, got %+v", sanitized.Content)
	}
}

func TestInferMimeTypeFromBase64(t *testing.T) {
	// PNG 魔数 base64
	mime := InferMimeTypeFromBase64("iVBORw0KGgoAAAANSUhEU")
	if mime != "image/png" {
		t.Errorf("mime = %q", mime)
	}
}
