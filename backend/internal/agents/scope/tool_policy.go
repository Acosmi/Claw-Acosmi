package scope

import (
	"math/rand"
	"strings"
)

// ---------- 工具策略 ----------

// TS 参考: src/agents/tool-policy.ts (292 行)

// ToolProfileID 工具配置文件标识。
type ToolProfileID string

const (
	ToolProfileMinimal   ToolProfileID = "minimal"
	ToolProfileCoding    ToolProfileID = "coding"
	ToolProfileMessaging ToolProfileID = "messaging"
	ToolProfileFull      ToolProfileID = "full"
)

// ToolPolicy 工具访问策略。
type ToolPolicy struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// toolNameAliases 工具名称别名（旧名→真实名）。
// NOTE: 主链路真实名来自 capabilities.Registry。
// 此处保留向后兼容别名，让旧配置文件中的旧名仍能正确映射。
var toolNameAliases = map[string]string{
	"exec":        "bash",
	"read":        "read_file",
	"write":       "write_file",
	"ls":          "list_dir",
	"apply-patch": "apply_patch",
}

// ToolGroups 工具分组定义（名称与 capabilities.Registry 对齐）。
var ToolGroups = map[string][]string{
	"group:memory":     {"memory_search", "memory_get"},
	"group:web":        {"web_search", "web_fetch"},
	"group:fs":         {"read_file", "write_file", "list_dir"},
	"group:runtime":    {"bash"},
	"group:sessions":   {"sessions_list", "sessions_history", "sessions_send", "sessions_spawn", "session_status"},
	"group:ui":         {"browser", "canvas"},
	"group:automation": {"cron", "gateway"},
	"group:messaging":  {"message"},
	"group:nodes":      {"nodes"},
	"group:openacosmi": {
		"browser", "canvas", "nodes", "cron", "message", "gateway",
		"agents_list", "sessions_list", "sessions_history", "sessions_send",
		"sessions_spawn", "session_status", "memory_search", "memory_get",
		"web_search", "web_fetch", "image",
	},
}

// ownerOnlyToolNames 仅所有者可用的工具。
var ownerOnlyToolNames = map[string]bool{
	"whatsapp_login": true,
}

// toolProfiles 预定义工具配置文件。
var toolProfiles = map[ToolProfileID]*ToolPolicy{
	ToolProfileMinimal:   {Allow: []string{"session_status"}},
	ToolProfileCoding:    {Allow: []string{"group:fs", "group:runtime", "group:sessions", "group:memory", "image"}},
	ToolProfileMessaging: {Allow: []string{"group:messaging", "sessions_list", "sessions_history", "sessions_send", "session_status"}},
	ToolProfileFull:      nil, // no restrictions
}

// NormalizeToolName 规范化工具名称。
func NormalizeToolName(name string) string {
	normalized := strings.TrimSpace(strings.ToLower(name))
	if alias, ok := toolNameAliases[normalized]; ok {
		return alias
	}
	return normalized
}

// IsOwnerOnlyTool 检查是否为仅所有者可用的工具。
func IsOwnerOnlyTool(name string) bool {
	return ownerOnlyToolNames[NormalizeToolName(name)]
}

// NormalizeToolList 规范化工具名称列表。
func NormalizeToolList(list []string) []string {
	result := make([]string, 0, len(list))
	for _, name := range list {
		normalized := NormalizeToolName(name)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

// ExpandToolGroups 展开工具组引用为具体工具名。
func ExpandToolGroups(list []string) []string {
	normalized := NormalizeToolList(list)
	seen := make(map[string]bool)
	var expanded []string
	for _, name := range normalized {
		if group, ok := ToolGroups[name]; ok {
			for _, tool := range group {
				if !seen[tool] {
					seen[tool] = true
					expanded = append(expanded, tool)
				}
			}
		} else if !seen[name] {
			seen[name] = true
			expanded = append(expanded, name)
		}
	}
	return expanded
}

// CollectExplicitAllowlist 从多级策略收集允许列表。
func CollectExplicitAllowlist(policies []*ToolPolicy) []string {
	var entries []string
	for _, p := range policies {
		if p == nil || len(p.Allow) == 0 {
			continue
		}
		for _, name := range p.Allow {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				entries = append(entries, trimmed)
			}
		}
	}
	return entries
}

// ResolveToolProfilePolicy 解析工具配置文件策略。
func ResolveToolProfilePolicy(profile string) *ToolPolicy {
	if profile == "" {
		return nil
	}
	id := ToolProfileID(strings.TrimSpace(strings.ToLower(profile)))
	policy, ok := toolProfiles[id]
	if !ok {
		return nil
	}
	if policy == nil {
		return nil
	}
	// 返回副本
	result := &ToolPolicy{}
	if policy.Allow != nil {
		result.Allow = make([]string, len(policy.Allow))
		copy(result.Allow, policy.Allow)
	}
	if policy.Deny != nil {
		result.Deny = make([]string, len(policy.Deny))
		copy(result.Deny, policy.Deny)
	}
	return result
}

// ---------- 插件工具组 ----------
// TS 参考: tool-policy.ts L124-274

// PluginToolGroups 插件工具分组。
type PluginToolGroups struct {
	All      []string            // 所有插件工具名
	ByPlugin map[string][]string // 按 pluginId 分组
}

// AllowlistResolution 允许列表解析结果。
type AllowlistResolution struct {
	Policy            *ToolPolicy
	UnknownAllowlist  []string
	StrippedAllowlist bool
}

// ToolMeta 工具元数据（用于 BuildPluginToolGroups）。
type ToolMeta struct {
	PluginID string
}

// BuildPluginToolGroups 从工具列表构建插件分组。
func BuildPluginToolGroups(tools []string, getMeta func(name string) *ToolMeta) PluginToolGroups {
	result := PluginToolGroups{
		ByPlugin: make(map[string][]string),
	}
	for _, name := range tools {
		meta := getMeta(name)
		if meta == nil {
			continue
		}
		normalized := NormalizeToolName(name)
		result.All = append(result.All, normalized)
		pluginId := strings.ToLower(meta.PluginID)
		result.ByPlugin[pluginId] = append(result.ByPlugin[pluginId], normalized)
	}
	return result
}

// ExpandPluginGroups 展开插件组引用 (group:plugins 和 pluginId)。
func ExpandPluginGroups(list []string, groups PluginToolGroups) []string {
	if len(list) == 0 {
		return list
	}
	seen := make(map[string]bool)
	var expanded []string
	for _, entry := range list {
		normalized := NormalizeToolName(entry)
		if normalized == "group:plugins" {
			if len(groups.All) > 0 {
				for _, t := range groups.All {
					if !seen[t] {
						seen[t] = true
						expanded = append(expanded, t)
					}
				}
			} else {
				if !seen[normalized] {
					seen[normalized] = true
					expanded = append(expanded, normalized)
				}
			}
			continue
		}
		if tools, ok := groups.ByPlugin[normalized]; ok && len(tools) > 0 {
			for _, t := range tools {
				if !seen[t] {
					seen[t] = true
					expanded = append(expanded, t)
				}
			}
			continue
		}
		if !seen[normalized] {
			seen[normalized] = true
			expanded = append(expanded, normalized)
		}
	}
	return expanded
}

// ExpandPolicyWithPluginGroups 展开策略中的插件组引用。
func ExpandPolicyWithPluginGroups(policy *ToolPolicy, groups PluginToolGroups) *ToolPolicy {
	if policy == nil {
		return nil
	}
	return &ToolPolicy{
		Allow: ExpandPluginGroups(policy.Allow, groups),
		Deny:  ExpandPluginGroups(policy.Deny, groups),
	}
}

// StripPluginOnlyAllowlist 剥离仅包含插件工具的 allowlist。
// 当 allowlist 只有插件工具时，移除 allow 以避免意外禁用核心工具。
func StripPluginOnlyAllowlist(policy *ToolPolicy, groups PluginToolGroups, coreTools map[string]bool) AllowlistResolution {
	if policy == nil || len(policy.Allow) == 0 {
		return AllowlistResolution{Policy: policy}
	}
	normalized := NormalizeToolList(policy.Allow)
	if len(normalized) == 0 {
		return AllowlistResolution{Policy: policy}
	}

	pluginIds := make(map[string]bool)
	for k := range groups.ByPlugin {
		pluginIds[k] = true
	}
	pluginTools := make(map[string]bool)
	for _, t := range groups.All {
		pluginTools[t] = true
	}

	var unknownAllowlist []string
	hasCoreEntry := false

	for _, entry := range normalized {
		if entry == "*" {
			hasCoreEntry = true
			continue
		}
		isPluginEntry := entry == "group:plugins" || pluginIds[entry] || pluginTools[entry]
		expanded := ExpandToolGroups([]string{entry})
		isCoreEntry := false
		for _, tool := range expanded {
			if coreTools[tool] {
				isCoreEntry = true
				break
			}
		}
		if isCoreEntry {
			hasCoreEntry = true
		}
		if !isCoreEntry && !isPluginEntry {
			unknownAllowlist = append(unknownAllowlist, entry)
		}
	}

	strippedAllowlist := !hasCoreEntry
	resultPolicy := policy
	if strippedAllowlist {
		resultPolicy = &ToolPolicy{Deny: policy.Deny}
	}

	// 去重 unknownAllowlist
	seen := make(map[string]bool)
	var deduped []string
	for _, u := range unknownAllowlist {
		if !seen[u] {
			seen[u] = true
			deduped = append(deduped, u)
		}
	}

	return AllowlistResolution{
		Policy:            resultPolicy,
		UnknownAllowlist:  deduped,
		StrippedAllowlist: strippedAllowlist,
	}
}

// ---------- Session Slug ----------

// TS 参考: src/agents/session-slug.ts (144 行)

var slugAdjectives = []string{
	"amber", "briny", "brisk", "calm", "clear", "cool", "crisp", "dawn",
	"delta", "ember", "faint", "fast", "fresh", "gentle", "glow", "good",
	"grand", "keen", "kind", "lucky", "marine", "mellow", "mild", "neat",
	"nimble", "nova", "oceanic", "plaid", "quick", "quiet", "rapid", "salty",
	"sharp", "swift", "tender", "tidal", "tidy", "tide", "vivid", "warm",
	"wild", "young",
}

var slugNouns = []string{
	"atlas", "basil", "bison", "bloom", "breeze", "canyon", "cedar", "claw",
	"cloud", "comet", "coral", "cove", "crest", "crustacean", "daisy", "dune",
	"ember", "falcon", "fjord", "forest", "glade", "gulf", "harbor", "haven",
	"kelp", "lagoon", "lobster", "meadow", "mist", "nudibranch", "nexus",
	"ocean", "orbit", "otter", "pine", "prairie", "reef", "ridge", "river",
	"rook", "sable", "sage", "seaslug", "shell", "shoal", "shore", "slug",
	"summit", "tidepool", "trail", "valley", "wharf", "willow", "zephyr",
}

func randomChoice(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[rand.Intn(len(values))]
}

func createSlugBase(words int) string {
	parts := []string{
		randomChoice(slugAdjectives, "steady"),
		randomChoice(slugNouns, "harbor"),
	}
	if words > 2 {
		parts = append(parts, randomChoice(slugNouns, "reef"))
	}
	return strings.Join(parts, "-")
}

// CreateSessionSlug 创建会话 slug（人类友好的短 ID）。
func CreateSessionSlug(isTaken func(string) bool) string {
	if isTaken == nil {
		isTaken = func(string) bool { return false }
	}
	// 2-word slugs
	for attempt := 0; attempt < 12; attempt++ {
		base := createSlugBase(2)
		if !isTaken(base) {
			return base
		}
		for i := 2; i <= 12; i++ {
			candidate := base + "-" + strings.Replace(string(rune('0'+i)), "\x00", "", -1)
			if !isTaken(candidate) {
				return candidate
			}
		}
	}
	// 3-word slugs
	for attempt := 0; attempt < 12; attempt++ {
		base := createSlugBase(3)
		if !isTaken(base) {
			return base
		}
	}
	// Final fallback
	return createSlugBase(3) + "-" + randomSuffix()
}

func randomSuffix() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 3)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
