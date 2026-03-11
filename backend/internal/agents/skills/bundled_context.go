package skills

// bundled_context.go — 内置技能上下文解析
// 对应 TS: agents/skills/bundled-context.ts (34L)
//
// 解析内置技能目录，加载技能清单，
// 返回 BundledSkillsContext（目录路径 + 技能名称集合）。

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// BundledSkillsContext 内置技能上下文。
type BundledSkillsContext struct {
	Dir   string          `json:"dir,omitempty"`
	Names map[string]bool `json:"names"`
}

var (
	hasWarnedMissingBundledDir bool
	warnOnce                   sync.Once
)

// ResolveBundledSkillsContext 解析内置技能上下文。
// 对应 TS: resolveBundledSkillsContext
func ResolveBundledSkillsContext(customDir string) BundledSkillsContext {
	ctx := BundledSkillsContext{
		Names: make(map[string]bool),
	}

	dir := customDir
	if dir == "" {
		dir = ResolveBundledSkillsDir("")
	}

	if dir == "" {
		warnOnce.Do(func() {
			slog.Warn("Bundled skills directory could not be resolved; built-in skills may be missing.")
			hasWarnedMissingBundledDir = true
		})
		return ctx
	}

	ctx.Dir = dir

	// 加载技能目录中的技能名称（兼容分类根目录）
	for _, root := range loadSkillSearchRoots(dir) {
		skills := LoadSkillsFromDir(root, "openacosmi-bundled")
		for _, skill := range skills {
			name := strings.TrimSpace(skill)
			if name != "" {
				ctx.Names[name] = true
			}
		}
	}

	return ctx
}

// LoadSkillsFromDir 从目录加载技能名称列表。
// 对应 TS: loadSkillsFromDir({ dir, source })
func LoadSkillsFromDir(dir, source string) []string {
	var skills []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("failed to read skills directory",
			"dir", dir,
			"source", source,
			"error", err)
		return skills
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// 检查目录下是否有 SKILL.md
		skillMdPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillMdPath); err == nil {
			skills = append(skills, entry.Name())
		}
	}

	return skills
}
