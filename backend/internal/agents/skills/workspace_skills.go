package skills

// ============================================================================
// 工作区技能快照构建
// 对应 TS: agents/skills/workspace.ts → buildWorkspaceSkillSnapshot() (L191-226)
// ============================================================================

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Acosmi/ClawAcosmi/internal/agents/capabilities"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// Skill 技能定义。
type Skill struct {
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content,omitempty"`
}

// SkillSummary 技能摘要（供 API / prompt 使用）。
type SkillSummary struct {
	Name       string `json:"name"`
	PrimaryEnv string `json:"primaryEnv,omitempty"`
}

// SkillSnapshot 技能快照。
type SkillSnapshot struct {
	Prompt         string         `json:"prompt"`
	Skills         []SkillSummary `json:"skills"`
	ResolvedSkills []Skill        `json:"resolvedSkills"`
	Version        *int           `json:"version,omitempty"`
}

// SkillEntry 技能条目（加载 + 元数据解析后）。
type SkillEntry struct {
	Skill      Skill
	PrimaryEnv string
	Enabled    bool
	// DisableModelInvocation 如果为 true，模型不应主动调用此技能
	DisableModelInvocation bool
	// Metadata OpenAcosmi 元数据（从 frontmatter 解析）
	Metadata *OpenAcosmiSkillMetadata
	// Invocation 调用策略
	Invocation *SkillInvocationPolicy
}

// SkillEligibilityContext 技能适用性上下文。
type SkillEligibilityContext struct {
	RemoteNote string // 远程技能注释
	Remote     *RemoteEligibility
}

// BuildSnapshotParams 构建快照参数。
type BuildSnapshotParams struct {
	WorkspaceDir    string
	Config          *types.OpenAcosmiConfig
	ManagedDir      string
	BundledDir      string
	SkillFilter     []string
	Eligibility     *SkillEligibilityContext
	SnapshotVersion *int
	// Entries 预加载条目（可选，跳过文件系统扫描）
	Entries []SkillEntry
}

// BuildWorkspaceSkillSnapshot 构建工作区技能快照。
// TS 对应: workspace.ts → buildWorkspaceSkillSnapshot() (L191-226)
func BuildWorkspaceSkillSnapshot(params BuildSnapshotParams) SkillSnapshot {
	entries := params.Entries
	if entries == nil {
		entries = LoadSkillEntries(params.WorkspaceDir, params.ManagedDir, params.BundledDir, params.Config)
	}

	eligible := filterSkillEntries(entries, params.Config, params.SkillFilter)
	promptEntries := make([]SkillEntry, 0, len(eligible))
	for _, e := range eligible {
		if !e.DisableModelInvocation {
			promptEntries = append(promptEntries, e)
		}
	}

	resolvedSkills := make([]Skill, 0, len(promptEntries))
	for _, e := range promptEntries {
		resolvedSkills = append(resolvedSkills, e.Skill)
	}

	var parts []string
	if params.Eligibility != nil && strings.TrimSpace(params.Eligibility.RemoteNote) != "" {
		parts = append(parts, strings.TrimSpace(params.Eligibility.RemoteNote))
	}
	if prompt := formatSkillsForPrompt(resolvedSkills); prompt != "" {
		parts = append(parts, prompt)
	}

	summaries := make([]SkillSummary, 0, len(eligible))
	for _, e := range eligible {
		summaries = append(summaries, SkillSummary{
			Name:       e.Skill.Name,
			PrimaryEnv: e.PrimaryEnv,
		})
	}

	return SkillSnapshot{
		Prompt:         strings.Join(parts, "\n"),
		Skills:         summaries,
		ResolvedSkills: resolvedSkills,
		Version:        params.SnapshotVersion,
	}
}

// LoadSkillEntries 从文件系统加载技能条目。
// TS 对应: workspace.ts → loadSkillEntries()
func LoadSkillEntries(workspaceDir, managedDir, bundledDir string, cfg *types.OpenAcosmiConfig) []SkillEntry {
	var entries []SkillEntry

	// 工作区 .agent/skills/
	wsSkillDir := filepath.Join(workspaceDir, ".agent", "skills")
	entries = append(entries, loadSkillsFromDir(wsSkillDir)...)

	// 托管目录
	if managedDir != "" {
		entries = append(entries, loadSkillsFromDir(managedDir)...)
	}

	// 捆绑目录
	if bundledDir != "" {
		bundled := loadSkillsFromDir(bundledDir)
		allowBundled := resolveAllowBundled(cfg)
		for _, b := range bundled {
			if allowBundled == nil || allowBundled[b.Skill.Name] {
				entries = append(entries, b)
			}
		}
	}

	// 额外目录
	if cfg != nil && cfg.Skills != nil && cfg.Skills.Load != nil {
		for _, dir := range cfg.Skills.Load.ExtraDirs {
			entries = append(entries, loadSkillsFromDir(dir)...)
		}
	}

	// 自动扫描: 项目 docs/skills/ 下的分类子目录（tools/, providers/, general/, official/ 等）
	if workspaceDir != "" {
		docsSkillsDir := ResolveDocsSkillsDir(workspaceDir)
		if docsSkillsDir != "" {
			// 扫描 docs/skills/ 自身（平级技能目录）
			entries = append(entries, loadSkillsFromDir(docsSkillsDir)...)
			// 扫描分类子目录（tools/, providers/, general/, official/ 等）
			subDirs, _ := os.ReadDir(docsSkillsDir)
			for _, sd := range subDirs {
				if sd.IsDir() && !strings.HasPrefix(sd.Name(), ".") {
					entries = append(entries, loadSkillsFromDir(filepath.Join(docsSkillsDir, sd.Name()))...)
				}
			}
		}
	}

	return deduplicateEntries(entries)
}

// loadSkillsFromDir 从目录加载技能。
func loadSkillsFromDir(dir string) []SkillEntry {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var entries []SkillEntry
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, de.Name(), "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}
		contentStr := string(content)
		desc := extractDescription(contentStr)

		// 解析 frontmatter 元数据
		fm := ParseFrontmatter(contentStr)
		meta := ResolveOpenAcosmiMetadata(fm)
		inv := ResolveSkillInvocationPolicy(fm)

		// 直接 frontmatter tools 字段（非 metadata JSON 内）
		var directTools []string
		if toolsStr := fm["tools"]; toolsStr != "" {
			for _, t := range strings.Split(toolsStr, ",") {
				if s := strings.TrimSpace(t); s != "" {
					directTools = append(directTools, s)
				}
			}
		}
		// 合并: 直接 frontmatter 优先，metadata 补充
		if meta == nil && len(directTools) > 0 {
			meta = &OpenAcosmiSkillMetadata{Tools: directTools}
		} else if meta != nil && len(directTools) > 0 && len(meta.Tools) == 0 {
			meta.Tools = directTools
		}

		var primaryEnv string
		if meta != nil {
			primaryEnv = meta.PrimaryEnv
		}

		entries = append(entries, SkillEntry{
			Skill: Skill{
				Name:        de.Name(),
				Dir:         filepath.Join(dir, de.Name()),
				Description: desc,
				Content:     contentStr,
			},
			PrimaryEnv:             primaryEnv,
			Enabled:                true,
			Metadata:               meta,
			Invocation:             &inv,
			DisableModelInvocation: inv.DisableModelInvocation,
		})
	}
	return entries
}

// extractDescription 从 SKILL.md frontmatter 提取 description。
func extractDescription(content string) string {
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return ""
	}
	frontmatter := content[3 : 3+end]
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return ""
}

// filterSkillEntries 根据配置和过滤器筛选技能条目。
func filterSkillEntries(entries []SkillEntry, cfg *types.OpenAcosmiConfig, skillFilter []string) []SkillEntry {
	filtered := make([]SkillEntry, 0, len(entries))
	filterSet := make(map[string]bool)
	if len(skillFilter) > 0 {
		for _, name := range skillFilter {
			filterSet[name] = true
		}
	}

	for _, e := range entries {
		if !e.Enabled {
			continue
		}
		if len(filterSet) > 0 && !filterSet[e.Skill.Name] {
			continue
		}
		// 检查配置级禁用
		if cfg != nil && cfg.Skills != nil && cfg.Skills.Entries != nil {
			if sc, ok := cfg.Skills.Entries[e.Skill.Name]; ok && sc != nil {
				if sc.Enabled != nil && !*sc.Enabled {
					continue
				}
			}
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// formatSkillsForPrompt 格式化技能列表为 prompt 字符串。
func formatSkillsForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Available skills:\n")
	for _, s := range skills {
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", s.Name, s.Description))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", s.Name))
		}
	}
	return sb.String()
}

// ResolveDocsSkillsDir 从 workspace 或 CWD 向上查找 docs/skills/ 目录。
//
// 策略:
//  1. 从 workspaceDir 向上遍历 3 层（适用于 workspace 在项目内的场景）
//  2. 从 CWD 向上遍历 4 层（适用于 monorepo 布局：CWD=backend/，docs/skills/ 在上级）
//  3. 环境变量 OPENACOSMI_DOCS_SKILLS_DIR 显式覆盖
func ResolveDocsSkillsDir(workspaceDir string) string {
	// 0. 环境变量显式覆盖
	if override := os.Getenv("OPENACOSMI_DOCS_SKILLS_DIR"); override != "" {
		if info, err := os.Stat(override); err == nil && info.IsDir() {
			return override
		}
	}

	// 1. 从 workspace 向上查找（原有逻辑）
	if workspaceDir != "" {
		current := workspaceDir
		for depth := 0; depth < 3; depth++ {
			candidate := filepath.Join(current, "docs", "skills")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
			next := filepath.Dir(current)
			if next == current {
				break
			}
			current = next
		}
	}

	// 2. 从 CWD 向上查找（monorepo fallback: CWD=backend/ → parent=项目根）
	if cwd, err := os.Getwd(); err == nil {
		current := cwd
		for depth := 0; depth < 4; depth++ {
			candidate := filepath.Join(current, "docs", "skills")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
			next := filepath.Dir(current)
			if next == current {
				break
			}
			current = next
		}
	}

	return ""
}

// deduplicateEntries 去重技能条目（先出现的同名技能优先保留）。
func deduplicateEntries(entries []SkillEntry) []SkillEntry {
	seen := make(map[string]bool)
	result := make([]SkillEntry, 0, len(entries))
	for _, e := range entries {
		if seen[e.Skill.Name] {
			continue
		}
		seen[e.Skill.Name] = true
		result = append(result, e)
	}
	return result
}

// FormatSkillIndex 将技能列表格式化为紧凑索引（名称 + 截断描述）。
// 用于 system prompt 的按需加载模式：prompt 只放索引，LLM 通过 lookup_skill 获取完整内容。
func FormatSkillIndex(resolvedSkills []Skill) string {
	if len(resolvedSkills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	for _, s := range resolvedSkills {
		desc := s.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		if desc != "" {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", s.Name, desc))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", s.Name))
		}
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

// ResolveToolSkillBindings 从技能列表构建 toolName → 技能描述 映射。
// 用于将技能指引注入到工具 Description 中，LLM 读工具定义时自动获得使用指南。
// 验证门控：只有 Registry 中标记为 SkillBindable 的工具才允许绑定。
func ResolveToolSkillBindings(entries []SkillEntry) map[string]string {
	bindings := make(map[string]string)
	for _, e := range entries {
		if e.Metadata == nil || len(e.Metadata.Tools) == 0 {
			continue
		}
		desc := e.Skill.Description
		if len(desc) > 120 {
			desc = desc[:117] + "..."
		}
		if desc == "" {
			continue
		}
		for _, toolName := range e.Metadata.Tools {
			if _, exists := bindings[toolName]; exists {
				continue
			}
			spec := capabilities.LookupByToolName(toolName)
			if spec == nil {
				slog.Warn("skill binds to unknown tool", "skill", e.Skill.Name, "tool", toolName)
				continue
			}
			if !spec.SkillBindable {
				slog.Warn("skill binds to non-bindable tool", "skill", e.Skill.Name, "tool", toolName)
				continue
			}
			bindings[toolName] = desc
		}
	}
	return bindings
}

// resolveAllowBundled 解析捆绑技能白名单。
func resolveAllowBundled(cfg *types.OpenAcosmiConfig) map[string]bool {
	if cfg == nil || cfg.Skills == nil || len(cfg.Skills.AllowBundled) == 0 {
		return nil // nil = allow all
	}
	allowed := make(map[string]bool)
	for _, name := range cfg.Skills.AllowBundled {
		allowed[name] = true
	}
	return allowed
}
