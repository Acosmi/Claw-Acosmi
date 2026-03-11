package skills

// frontmatter.go — SKILL.md frontmatter 解析 + 元数据
// 对应 TS: agents/skills/frontmatter.ts (173L) + types.ts (88L)
//
// 提供 frontmatter 解析、OpenAcosmi 元数据解析、调用策略解析、
// install spec 解析等核心能力。

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/adrg/frontmatter"
)

// SkillInstallSpec 技能安装规格。
type SkillInstallSpec struct {
	Kind            string   `json:"kind"` // "brew"|"node"|"go"|"uv"|"download"
	ID              string   `json:"id,omitempty"`
	Label           string   `json:"label,omitempty"`
	Bins            []string `json:"bins,omitempty"`
	OS              []string `json:"os,omitempty"`
	Formula         string   `json:"formula,omitempty"`
	Package         string   `json:"package,omitempty"`
	Module          string   `json:"module,omitempty"`
	URL             string   `json:"url,omitempty"`
	Archive         string   `json:"archive,omitempty"`
	Extract         *bool    `json:"extract,omitempty"`
	StripComponents *int     `json:"stripComponents,omitempty"`
	TargetDir       string   `json:"targetDir,omitempty"`
}

// SkillRequirements 技能依赖需求。
type SkillRequirements struct {
	Bins    []string `json:"bins,omitempty"`
	AnyBins []string `json:"anyBins,omitempty"`
	Env     []string `json:"env,omitempty"`
	Config  []string `json:"config,omitempty"`
}

// OpenAcosmiSkillMetadata OpenAcosmi 技能元数据。
type OpenAcosmiSkillMetadata struct {
	Always     *bool              `json:"always,omitempty"`
	SkillKey   string             `json:"skillKey,omitempty"`
	PrimaryEnv string             `json:"primaryEnv,omitempty"`
	Emoji      string             `json:"emoji,omitempty"`
	Homepage   string             `json:"homepage,omitempty"`
	TreeID     string             `json:"treeId,omitempty"`
	TreeGroup  string             `json:"treeGroup,omitempty"`
	MinTier    string             `json:"minTier,omitempty"`
	Approval   string             `json:"approvalType,omitempty"`
	OS         []string           `json:"os,omitempty"`
	Requires   *SkillRequirements `json:"requires,omitempty"`
	Install    []SkillInstallSpec `json:"install,omitempty"`
	Tools      []string           `json:"tools,omitempty"` // 绑定的工具名列表
}

// SkillInvocationPolicy 技能调用策略。
type SkillInvocationPolicy struct {
	UserInvocable          bool `json:"userInvocable"`
	DisableModelInvocation bool `json:"disableModelInvocation"`
}

// SkillCommandSpec 技能命令规格。
type SkillCommandSpec struct {
	Name        string `json:"name"`
	SkillName   string `json:"skillName"`
	Description string `json:"description"`
}

// ParsedSkillFrontmatter 解析后的 frontmatter（key/value 映射）。
type ParsedSkillFrontmatter map[string]string

// MANIFEST_KEY OpenAcosmi manifest key。
const MANIFEST_KEY = "openacosmi"

// LEGACY_MANIFEST_KEYS 旧版 manifest keys。
var LEGACY_MANIFEST_KEYS = []string{"pi-ai", "pi"}

// ParseFrontmatter 解析 SKILL.md frontmatter。
// 使用 adrg/frontmatter 库解析完整 YAML/TOML/JSON frontmatter，
// 返回 map[string]string 以保持向后兼容。复杂值（对象/数组）序列化为 JSON 字符串。
func ParseFrontmatter(content string) ParsedSkillFrontmatter {
	result := make(ParsedSkillFrontmatter)
	if !strings.HasPrefix(content, "---") {
		return result
	}

	var raw map[string]interface{}
	_, err := frontmatter.Parse(strings.NewReader(content), &raw)
	if err != nil || raw == nil {
		return result
	}

	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val
		case bool:
			result[k] = fmt.Sprintf("%v", val)
		case int:
			result[k] = fmt.Sprintf("%d", val)
		case float64:
			result[k] = fmt.Sprintf("%g", val)
		default:
			// Objects, arrays, nested YAML → JSON string for backward compat
			if b, jsonErr := json.Marshal(val); jsonErr == nil {
				result[k] = string(b)
			}
		}
	}
	return result
}

// NormalizeStringList 规范化字符串列表。
func NormalizeStringList(input interface{}) []string {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case []interface{}:
		var result []string
		for _, item := range v {
			s := strings.TrimSpace(toString(item))
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	case string:
		var result []string
		for _, part := range strings.Split(v, ",") {
			s := strings.TrimSpace(part)
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// ParseInstallSpec 解析安装规格。
func ParseInstallSpec(input interface{}) *SkillInstallSpec {
	raw, ok := input.(map[string]interface{})
	if !ok || raw == nil {
		return nil
	}

	kind := ""
	if v, ok := raw["kind"].(string); ok {
		kind = strings.ToLower(strings.TrimSpace(v))
	} else if v, ok := raw["type"].(string); ok {
		kind = strings.ToLower(strings.TrimSpace(v))
	}

	validKinds := map[string]bool{"brew": true, "node": true, "go": true, "uv": true, "download": true}
	if !validKinds[kind] {
		return nil
	}

	spec := &SkillInstallSpec{Kind: kind}
	if v, ok := raw["id"].(string); ok {
		spec.ID = v
	}
	if v, ok := raw["label"].(string); ok {
		spec.Label = v
	}
	if bins := NormalizeStringList(raw["bins"]); len(bins) > 0 {
		spec.Bins = bins
	}
	if os := NormalizeStringList(raw["os"]); len(os) > 0 {
		spec.OS = os
	}
	if v, ok := raw["formula"].(string); ok {
		spec.Formula = v
	}
	if v, ok := raw["package"].(string); ok {
		spec.Package = v
	}
	if v, ok := raw["module"].(string); ok {
		spec.Module = v
	}
	if v, ok := raw["url"].(string); ok {
		spec.URL = v
	}
	if v, ok := raw["archive"].(string); ok {
		spec.Archive = v
	}
	if v, ok := raw["extract"].(bool); ok {
		spec.Extract = &v
	}
	if v, ok := raw["stripComponents"].(float64); ok {
		n := int(v)
		spec.StripComponents = &n
	}
	if v, ok := raw["targetDir"].(string); ok {
		spec.TargetDir = v
	}

	return spec
}

// ResolveOpenAcosmiMetadata 从 frontmatter 解析 OpenAcosmi 元数据。
// 对应 TS: resolveOpenAcosmiMetadata
func ResolveOpenAcosmiMetadata(fm ParsedSkillFrontmatter) *OpenAcosmiSkillMetadata {
	raw := fm["metadata"]
	if raw == "" {
		return nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}

	// 查找 manifest key；找不到时回退到 flat metadata，兼容当前能力树格式。
	var metadataObj map[string]interface{}
	candidates := append([]string{MANIFEST_KEY}, LEGACY_MANIFEST_KEYS...)
	for _, key := range candidates {
		if v, ok := parsed[key].(map[string]interface{}); ok {
			metadataObj = v
			break
		}
	}
	if metadataObj == nil {
		metadataObj = parsed
	}

	meta := &OpenAcosmiSkillMetadata{}

	if v, ok := parsed["tree_id"].(string); ok {
		meta.TreeID = strings.TrimSpace(v)
	}
	if v, ok := parsed["tree_group"].(string); ok {
		meta.TreeGroup = strings.TrimSpace(v)
	}
	if v, ok := parsed["min_tier"].(string); ok {
		meta.MinTier = strings.TrimSpace(v)
	}
	if v, ok := parsed["approval_type"].(string); ok {
		meta.Approval = strings.TrimSpace(v)
	}

	if v, ok := metadataObj["always"].(bool); ok {
		meta.Always = &v
	}
	if v, ok := metadataObj["emoji"].(string); ok {
		meta.Emoji = v
	}
	if v, ok := metadataObj["homepage"].(string); ok {
		meta.Homepage = v
	}
	if v, ok := metadataObj["skillKey"].(string); ok {
		meta.SkillKey = v
	}
	if v, ok := metadataObj["primaryEnv"].(string); ok {
		meta.PrimaryEnv = v
	}
	if os := NormalizeStringList(metadataObj["os"]); len(os) > 0 {
		meta.OS = os
	}

	// requires
	if req, ok := metadataObj["requires"].(map[string]interface{}); ok {
		meta.Requires = &SkillRequirements{
			Bins:    NormalizeStringList(req["bins"]),
			AnyBins: NormalizeStringList(req["anyBins"]),
			Env:     NormalizeStringList(req["env"]),
			Config:  NormalizeStringList(req["config"]),
		}
	}

	// install
	if installRaw, ok := metadataObj["install"].([]interface{}); ok {
		for _, entry := range installRaw {
			if spec := ParseInstallSpec(entry); spec != nil {
				meta.Install = append(meta.Install, *spec)
			}
		}
	}

	// tools — 绑定的工具名列表
	if tools := NormalizeStringList(metadataObj["tools"]); len(tools) > 0 {
		meta.Tools = tools
	} else if meta.TreeID != "" {
		if toolName := resolveToolNameFromTreeID(meta.TreeID); toolName != "" {
			meta.Tools = []string{toolName}
		}
	}

	return meta
}

// ResolveSkillInvocationPolicy 从 frontmatter 解析调用策略。
// 对应 TS: resolveSkillInvocationPolicy
func ResolveSkillInvocationPolicy(fm ParsedSkillFrontmatter) SkillInvocationPolicy {
	return SkillInvocationPolicy{
		UserInvocable:          parseFrontmatterBool(fm["user-invocable"], true),
		DisableModelInvocation: parseFrontmatterBool(fm["disable-model-invocation"], false),
	}
}

// ResolveSkillKey 解析 skill key。
func ResolveSkillKey(name string, metadata *OpenAcosmiSkillMetadata) string {
	if metadata != nil && metadata.SkillKey != "" {
		return metadata.SkillKey
	}
	return name
}

// ---------- helpers ----------

func parseFrontmatterBool(value string, fallback bool) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		return fallback
	}
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%g", val), "0"), ".")
	default:
		return ""
	}
}

func resolveToolNameFromTreeID(treeID string) string {
	node := capabilities.DefaultTree().Lookup(strings.TrimSpace(treeID))
	if node == nil {
		return ""
	}
	if node.Kind != capabilities.NodeKindTool && node.Kind != capabilities.NodeKindSubagent {
		return ""
	}
	return strings.TrimSpace(node.Name)
}
