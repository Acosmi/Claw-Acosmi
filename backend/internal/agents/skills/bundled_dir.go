package skills

// bundled_dir.go — 内置技能目录解析
// 对应 TS: agents/skills/bundled-dir.ts (91L)
//
// 提供 ResolveBundledSkillsDir — 多策略定位 bundled skills 目录：
//   1. 环境变量覆盖 (OPENACOSMI_BUNDLED_SKILLS_DIR)
//   2. 可执行文件同级 skills/ 目录
//   3. 工作目录向上遍历（最多 6 层）

import (
	"os"
	"path/filepath"
	"strings"
)

func hasDirectSkillDirs(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		skillMd := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillMd); err == nil {
			return true
		}
	}
	return false
}

func loadSkillSearchRoots(dir string) []string {
	if dir == "" {
		return nil
	}

	var roots []string
	seen := make(map[string]struct{})

	var walk func(string, int)
	walk = func(current string, depth int) {
		if current == "" || depth < 0 {
			return
		}
		if hasDirectSkillDirs(current) {
			if _, ok := seen[current]; !ok {
				roots = append(roots, current)
				seen[current] = struct{}{}
			}
			return
		}

		entries, err := os.ReadDir(current)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			walk(filepath.Join(current, entry.Name()), depth-1)
		}
	}

	// 允许 docs/skills/tools/ui/browser 这类两级分类根被识别出来。
	walk(dir, 3)
	return roots
}

// looksLikeSkillsDir 检查目录是否看起来像技能目录。
// 对应 TS: bundled-dir.ts → looksLikeSkillsDir
//
// 判据：
//   - 目录自身直接包含技能目录
//   - 或目录下一层分类目录直接包含技能目录
func looksLikeSkillsDir(dir string) bool {
	return len(loadSkillSearchRoots(dir)) > 0
}

func bundleResourcesDir(execPath string) string {
	if strings.TrimSpace(execPath) == "" {
		return ""
	}
	execDir := filepath.Dir(execPath)
	if filepath.Base(execDir) != "MacOS" {
		return ""
	}
	contentsDir := filepath.Dir(execDir)
	if filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	resourcesDir := filepath.Join(contentsDir, "Resources")
	info, err := os.Stat(resourcesDir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return resourcesDir
}

func skillsDirCandidatesForExecPath(execPath string) []string {
	if strings.TrimSpace(execPath) == "" {
		return nil
	}
	execDir := filepath.Dir(execPath)
	candidates := []string{
		filepath.Join(execDir, "skills"),
		filepath.Join(execDir, "docs", "skills"),
	}
	if resourcesDir := bundleResourcesDir(execPath); resourcesDir != "" {
		candidates = append(candidates,
			filepath.Join(resourcesDir, "skills"),
			filepath.Join(resourcesDir, "docs", "skills"),
		)
	}
	return candidates
}

// ResolveBundledSkillsDir 解析捆绑技能目录。
// 对应 TS: bundled-dir.ts → resolveBundledSkillsDir
//
// 多策略定位：
//  1. 环境变量 OPENACOSMI_BUNDLED_SKILLS_DIR
//  2. execPath 同级/Resources 下的 skills 或 docs/skills 目录
//  3. 当前可执行文件同级/Resources 下的 skills 或 docs/skills 目录
//  4. 当前工作目录向上遍历（最多 6 层），寻找 skills/ 或 docs/skills 子目录
func ResolveBundledSkillsDir(execPath string) string {
	// 1) 环境变量覆盖
	override := strings.TrimSpace(os.Getenv("OPENACOSMI_BUNDLED_SKILLS_DIR"))
	if override != "" {
		return override
	}

	// 2) 指定 execPath 相关目录
	if execPath != "" {
		for _, candidate := range skillsDirCandidatesForExecPath(execPath) {
			if looksLikeSkillsDir(candidate) {
				return candidate
			}
		}
	}

	// 3) 当前可执行文件路径
	if ep, err := os.Executable(); err == nil {
		for _, candidate := range skillsDirCandidatesForExecPath(ep) {
			if looksLikeSkillsDir(candidate) {
				return candidate
			}
		}
	}

	// 4) 当前工作目录向上遍历（TS: moduleDir 向上 6 层 + looksLikeSkillsDir 验证）
	if cwd, err := os.Getwd(); err == nil {
		current := cwd
		for depth := 0; depth < 6; depth++ {
			for _, candidate := range []string{
				filepath.Join(current, "skills"),
				filepath.Join(current, "docs", "skills"),
			} {
				if looksLikeSkillsDir(candidate) {
					return candidate
				}
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
