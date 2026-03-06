package argus

// skills.go — MCP 工具到技能条目转换
//
// BuildArgusSkillEntries 将 MCP tools/list 返回的工具列表转为
// 与 server_methods_skills.go 中 skillStatusEntry 兼容的格式，
// 以便在 skills.status 响应中合并展示。

import (
	"os"
	"path/filepath"
	"strings"

	agentskills "github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/mcpclient"
)

// ArgusSkillEntry 与 gateway skillStatusEntry JSON 格式兼容的 Argus 技能条目。
type ArgusSkillEntry struct {
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	Source             string                   `json:"source"`
	FilePath           string                   `json:"filePath"`
	BaseDir            string                   `json:"baseDir"`
	SkillKey           string                   `json:"skillKey"`
	Bundled            bool                     `json:"bundled,omitempty"`
	PrimaryEnv         string                   `json:"primaryEnv,omitempty"`
	Emoji              string                   `json:"emoji,omitempty"`
	Homepage           string                   `json:"homepage,omitempty"`
	Always             bool                     `json:"always"`
	Disabled           bool                     `json:"disabled"`
	BlockedByAllowlist bool                     `json:"blockedByAllowlist"`
	Eligible           bool                     `json:"eligible"`
	Category           string                   `json:"category"`
	Risk               string                   `json:"risk"`
	Requirements       map[string][]string      `json:"requirements"`
	Missing            map[string][]string      `json:"missing"`
	ConfigChecks       []map[string]interface{} `json:"configChecks"`
	Install            []map[string]interface{} `json:"install"`
}

// 工具分类映射。
var toolCategoryMap = map[string]string{
	// Perception
	"capture_screen":   "perception",
	"describe_scene":   "perception",
	"locate_element":   "perception",
	"read_text":        "perception",
	"detect_dialog":    "perception",
	"watch_for_change": "perception",
	// Action
	"click":          "action",
	"double_click":   "action",
	"type_text":      "action",
	"press_key":      "action",
	"hotkey":         "action",
	"scroll":         "action",
	"mouse_position": "action",
	// Shell
	"run_shell": "shell",
	// macOS
	"macos_shortcut": "macos",
	"open_url":       "macos",
}

// 工具风险等级映射。
var toolRiskMap = map[string]string{
	"capture_screen":   "low",
	"describe_scene":   "low",
	"locate_element":   "low",
	"read_text":        "low",
	"detect_dialog":    "low",
	"watch_for_change": "low",
	"click":            "medium",
	"double_click":     "medium",
	"type_text":        "medium",
	"press_key":        "medium",
	"hotkey":           "medium",
	"scroll":           "low",
	"mouse_position":   "low",
	"run_shell":        "high",
	"macos_shortcut":   "medium",
	"open_url":         "medium",
}

type argusToolBinding struct {
	Description string
	FilePath    string
	BaseDir     string
}

// BuildArgusSkillEntries 将 MCP 工具列表转为技能条目列表。
func BuildArgusSkillEntries(tools []mcpclient.MCPToolDef) []ArgusSkillEntry {
	docBindings := loadArgusToolBindings()
	entries := make([]ArgusSkillEntry, 0, len(tools))
	for _, t := range tools {
		category := toolCategoryMap[t.Name]
		if category == "" {
			category = "unknown"
		}
		risk := toolRiskMap[t.Name]
		if risk == "" {
			risk = "medium"
		}

		description := t.Description
		filePath := ""
		baseDir := ""
		if binding, ok := docBindings["argus_"+t.Name]; ok {
			if strings.TrimSpace(binding.Description) != "" {
				description = binding.Description
			}
			filePath = binding.FilePath
			baseDir = binding.BaseDir
		}

		entries = append(entries, ArgusSkillEntry{
			Name:        "argus." + t.Name,
			Description: description,
			Source:      "argus",
			FilePath:    filePath,
			BaseDir:     baseDir,
			SkillKey:    "argus." + t.Name,
			Bundled:     false,
			PrimaryEnv:  "",
			Emoji:       emojiForCategory(category),
			Always:      false,
			Disabled:    false,
			Eligible:    true,
			Category:    category,
			Risk:        risk,
			Requirements: map[string][]string{
				"bins":   {},
				"env":    {},
				"config": {},
				"os":     {},
			},
			Missing: map[string][]string{
				"bins":   {},
				"env":    {},
				"config": {},
				"os":     {},
			},
			ConfigChecks: []map[string]interface{}{},
			Install:      []map[string]interface{}{},
		})
	}
	return entries
}

func loadArgusToolBindings() map[string]argusToolBinding {
	docsSkillsDir := agentskills.ResolveDocsSkillsDir("")
	if docsSkillsDir == "" {
		return nil
	}

	bindings := make(map[string]argusToolBinding)
	loadArgusToolBindingsFromDir(bindings, docsSkillsDir)

	categoryDirs, err := os.ReadDir(docsSkillsDir)
	if err != nil {
		return bindings
	}
	for _, de := range categoryDirs {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		loadArgusToolBindingsFromDir(bindings, filepath.Join(docsSkillsDir, de.Name()))
	}
	return bindings
}

func loadArgusToolBindingsFromDir(bindings map[string]argusToolBinding, dir string) {
	skillDirs, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, de := range skillDirs {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		skillFile := filepath.Join(dir, de.Name(), "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		fm := agentskills.ParseFrontmatter(string(content))
		description := strings.TrimSpace(fm["description"])
		if description == "" {
			continue
		}
		for _, toolName := range parseSkillTools(fm["tools"]) {
			if !strings.HasPrefix(toolName, "argus_") {
				continue
			}
			if _, exists := bindings[toolName]; exists {
				continue
			}
			bindings[toolName] = argusToolBinding{
				Description: description,
				FilePath:    skillFile,
				BaseDir:     filepath.Dir(skillFile),
			}
		}
	}
}

func parseSkillTools(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tools := make([]string, 0, len(parts))
	for _, part := range parts {
		toolName := strings.TrimSpace(part)
		if toolName != "" {
			tools = append(tools, toolName)
		}
	}
	return tools
}

// emojiForCategory 根据分类返回 emoji 标识。
func emojiForCategory(category string) string {
	switch strings.ToLower(category) {
	case "perception":
		return "eye"
	case "action":
		return "pointer"
	case "shell":
		return "terminal"
	case "macos":
		return "apple"
	default:
		return "tool"
	}
}
