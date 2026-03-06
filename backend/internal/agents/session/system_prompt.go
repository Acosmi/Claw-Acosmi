// system_prompt.go — 动态系统提示词组装 + context pruning。
//
// 桥接 internal/agents/prompt（静态提示词构建器）与运行时上下文文件，
// 在 token 预算内注入 workspace 中的 context files（SOUL.md、TOOLS.md 等）。
//
// TS 参考: src/agents/system-prompt.ts → buildAgentSystemPrompt() (L164-609)
package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Acosmi/ClawAcosmi/internal/agents/prompt"
)

// ---------- Context Files ----------

// ContextFile 从 workspace 注入到系统提示词的上下文文件。
// TS 参考: src/agents/pi-embedded-helpers.ts → EmbeddedContextFile
type ContextFile struct {
	Path     string // 相对于 workspace 的路径
	Content  string // 文件内容
	Priority int    // 注入优先级（越小越重要，0=最高）
}

// 已知 context 文件名及其优先级。
// TS 参考: src/agents/pi-embedded-helpers.ts → CONTEXT_FILE_NAMES
var knownContextFiles = []struct {
	name     string
	priority int
}{
	{"SOUL.md", 0},        // 用户 persona，最高优先级
	{"TOOLS.md", 1},       // 工具使用指南
	{"MEMORY.md", 2},      // 核心记忆
	{"CONTEXT.md", 3},     // 附加上下文
	{"CLAUDE.md", 4},      // Claude 习惯
	{"WORKSPACE.md", 5},   // 工作区说明
	{".openacosmi.md", 6}, // 项目级配置说明
	{".clawdignore", 10},  // 忽略规则（低优先级）
}

// ---------- DynamicPromptParams ----------

// DynamicPromptParams 动态系统提示词构建参数。
type DynamicPromptParams struct {
	prompt.BuildParams               // 嵌入基础提示词参数
	WorkspaceDir       string        // workspace 目录（扫描 context files）
	ContextTokenBudget int           // context 文件的 token 预算（0=无限制）
	AgentID            string        // agent 身份
	IsSubagent         bool          // true = 子智能体会话（过滤掉 MEMORY.md 等主会话专属文件）
	ContextFiles       []ContextFile // 预指定的 context files（优先于扫描）
}

// ---------- BuildDynamicSystemPrompt ----------

// BuildDynamicSystemPrompt 构建完整的动态系统提示词。
// 管线：基础提示词 → context files 注入 → token 预算裁剪。
// TS 参考: system-prompt.ts → buildAgentSystemPrompt() (contextFiles 注入段 L552-569)
func BuildDynamicSystemPrompt(params DynamicPromptParams) string {
	// 1. 构建基础提示词
	basePrompt := prompt.BuildAgentSystemPrompt(params.BuildParams)

	// 2. 解析 context files
	contextFiles := params.ContextFiles
	if len(contextFiles) == 0 && params.WorkspaceDir != "" {
		contextFiles = ResolveContextFiles(params.WorkspaceDir)
	}

	// 2b. 子智能体会话过滤：移除主会话专属 context files
	if params.IsSubagent && len(contextFiles) > 0 {
		contextFiles = filterSubagentContextFiles(contextFiles)
	}

	if len(contextFiles) == 0 {
		return basePrompt
	}

	// 3. 按 token 预算裁剪
	if params.ContextTokenBudget > 0 {
		contextFiles = ApplyContextPruning(contextFiles, params.ContextTokenBudget)
	}

	if len(contextFiles) == 0 {
		return basePrompt
	}

	// 4. 组装 context 段落
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n# Project Context\n\nThe following project context files have been loaded:\n")

	// 检查是否包含 SOUL.md
	hasSoul := false
	for _, f := range contextFiles {
		base := filepath.Base(f.Path)
		if strings.EqualFold(base, "SOUL.md") {
			hasSoul = true
			break
		}
	}
	if hasSoul {
		sb.WriteString("If SOUL.md is present, embody its persona and tone. Avoid stiff, generic replies; follow its guidance unless higher-priority instructions override it.\n")
	}
	sb.WriteString("\n")

	for _, f := range contextFiles {
		sb.WriteString("## ")
		sb.WriteString(f.Path)
		sb.WriteString("\n\n")
		sb.WriteString(f.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// ---------- ResolveContextFiles ----------

// ResolveContextFiles 扫描 workspace 目录下的已知 context 文件。
// 按优先级排序返回（SOUL.md > TOOLS.md > MEMORY.md > ...）。
// TS 参考: pi-embedded-helpers.ts → resolveContextFiles()
func ResolveContextFiles(workspaceDir string) []ContextFile {
	if workspaceDir == "" {
		return nil
	}

	var files []ContextFile
	for _, known := range knownContextFiles {
		fullPath := filepath.Join(workspaceDir, known.name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue // 文件不存在或不可读，跳过
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		files = append(files, ContextFile{
			Path:     known.name,
			Content:  content,
			Priority: known.priority,
		})
	}

	// 按优先级排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].Priority < files[j].Priority
	})

	return files
}

// ---------- ResolveProjectRootContextDir ----------

// ResolveProjectRootContextDir 从 CWD 向上遍历，查找包含已知 context 文件的项目根目录。
// 返回第一个包含 SOUL.md / CLAUDE.md 等文件的目录路径。找不到返回空字符串。
// 用于补充 workspace 目录的 context 文件扫描（workspace 可能是 ~/.openacosmi/workspace/，
// 而 SOUL.md 等文件位于项目源码根目录）。
func ResolveProjectRootContextDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	current := cwd
	for depth := 0; depth < 5; depth++ {
		for _, known := range knownContextFiles {
			candidate := filepath.Join(current, known.name)
			if _, err := os.Stat(candidate); err == nil {
				return current
			}
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	return ""
}

// ---------- ApplyContextPruning ----------

// ApplyContextPruning 按 token 预算裁剪 context 文件列表。
// 高优先级文件优先保留，超出预算时截断低优先级文件的内容。
// TS 参考: system-prompt.ts 隐含的 context pruning 逻辑
func ApplyContextPruning(files []ContextFile, tokenBudget int) []ContextFile {
	if tokenBudget <= 0 || len(files) == 0 {
		return files
	}

	// 确保按优先级排序（小 = 高优先级）
	sorted := make([]ContextFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	var result []ContextFile
	usedTokens := 0

	for _, f := range sorted {
		fileTokens := EstimatePromptTokens(f.Content)
		if usedTokens+fileTokens <= tokenBudget {
			// 完整保留
			result = append(result, f)
			usedTokens += fileTokens
		} else {
			// 剩余预算不足 — 截断内容
			remaining := tokenBudget - usedTokens
			if remaining > 100 { // 至少保留 100 token 才值得截断
				truncated := truncateToTokenBudget(f.Content, remaining)
				if truncated != "" {
					result = append(result, ContextFile{
						Path:     f.Path,
						Content:  truncated + "\n\n[... content truncated due to token budget ...]",
						Priority: f.Priority,
					})
				}
			}
			break // 预算耗尽
		}
	}

	return result
}

// ---------- Token 估算 ----------

// EstimatePromptTokens 估算文本的 token 数。
// 使用简单的启发式规则：英文约 4 字符/token，中文约 1.5 字符/token。
// 这是粗略估算，用于预算分配而非精确计算。
func EstimatePromptTokens(text string) int {
	if text == "" {
		return 0
	}

	// 统计中英文字符
	chineseChars := 0
	totalRunes := 0
	for _, r := range text {
		totalRunes++
		if r >= 0x4E00 && r <= 0x9FFF || // CJK Unified Ideographs
			r >= 0x3400 && r <= 0x4DBF || // CJK Extension A
			r >= 0xF900 && r <= 0xFAFF { // CJK Compatibility
			chineseChars++
		}
	}

	nonChinese := totalRunes - chineseChars
	// 英文约 4 字符/token，中文约 1.5 字符/token
	englishTokens := nonChinese / 4
	chineseTokens := int(float64(chineseChars) / 1.5)

	total := englishTokens + chineseTokens
	if total == 0 && utf8.RuneCountInString(text) > 0 {
		total = 1
	}
	return total
}

// ---------- 内部辅助 ----------

// subagentExcludedFiles 子智能体会话中不注入的 context 文件。
// MEMORY.md 含主会话记忆，注入子智能体会导致 WARM_START 误判和上下文污染。
// SOUL.md 同理——子智能体的系统提示词由 announce_helpers 单独构建。
var subagentExcludedFiles = map[string]bool{
	"MEMORY.md": true,
	"SOUL.md":   true,
}

// filterSubagentContextFiles 过滤掉子智能体不应接收的 context 文件。
func filterSubagentContextFiles(files []ContextFile) []ContextFile {
	var filtered []ContextFile
	for _, f := range files {
		base := filepath.Base(f.Path)
		if subagentExcludedFiles[base] {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}

// truncateToTokenBudget 截断文本到指定 token 预算内。
func truncateToTokenBudget(text string, tokenBudget int) string {
	if tokenBudget <= 0 {
		return ""
	}

	// 按行截断以保持可读性
	lines := strings.Split(text, "\n")
	var result []string
	usedTokens := 0

	for _, line := range lines {
		lineTokens := EstimatePromptTokens(line)
		if usedTokens+lineTokens > tokenBudget {
			break
		}
		result = append(result, line)
		usedTokens += lineTokens
	}

	return strings.Join(result, "\n")
}
