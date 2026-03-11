package gateway

// task_preset_permissions.go — P5 任务级预设权限
// 行业对照: Terraform Sentinel Policy — 任务模板预绑定权限级别
//
// 允许管理员为特定任务类型预设权限级别和自动审批策略。
// 当智能体启动匹配的任务时，自动应用预设权限，无需每次手动审批。
//
// 配置持久化：扩展 exec-approvals.json 新增 taskPresets 字段

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/config"
)

// ---------- 任务预设类型 ----------

// TaskPreset 任务预设权限模板。
type TaskPreset struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`                  // 预设名称
	Pattern     string    `json:"pattern"`               // 任务名匹配模式（glob/前缀）
	Level       string    `json:"level"`                 // 权限级别: "sandbox", "full"
	AutoApprove bool      `json:"autoApprove"`           // 是否自动审批（跳过人工确认）
	MaxTTL      int       `json:"maxTtlMinutes"`         // 最大授权时长（分钟）
	Description string    `json:"description,omitempty"` // 描述
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TaskPresetMatch 任务匹配结果。
type TaskPresetMatch struct {
	Matched   bool        `json:"matched"`
	Preset    *TaskPreset `json:"preset,omitempty"`
	MatchedBy string      `json:"matchedBy,omitempty"` // "exact", "prefix", "glob"
}

// ---------- 管理器 ----------

// TaskPresetManager 任务预设权限管理器。
type TaskPresetManager struct {
	mu      sync.RWMutex
	presets []TaskPreset
}

// NewTaskPresetManager 创建任务预设管理器，自动加载配置。
func NewTaskPresetManager() *TaskPresetManager {
	m := &TaskPresetManager{}
	_ = m.loadFromDisk()
	return m
}

// List 列出所有预设。
func (m *TaskPresetManager) List() []TaskPreset {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TaskPreset, len(m.presets))
	copy(result, m.presets)
	return result
}

// Add 新增预设。
func (m *TaskPresetManager) Add(preset TaskPreset) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if preset.Name == "" {
		return fmt.Errorf("预设名称不能为空")
	}
	if preset.Pattern == "" {
		return fmt.Errorf("匹配模式不能为空")
	}
	if preset.Level != "sandbox" && preset.Level != "full" {
		return fmt.Errorf("无效的权限级别: %s（仅支持 sandbox/full）", preset.Level)
	}
	if preset.MaxTTL <= 0 {
		preset.MaxTTL = 60 // 默认 60 分钟
	}

	// 检查 ID 唯一性
	for _, p := range m.presets {
		if p.ID == preset.ID {
			return fmt.Errorf("预设 ID 已存在: %s", preset.ID)
		}
	}

	now := time.Now()
	preset.CreatedAt = now
	preset.UpdatedAt = now

	m.presets = append(m.presets, preset)
	return m.saveToDiskLocked()
}

// Update 更新预设。
func (m *TaskPresetManager) Update(id string, update TaskPreset) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, p := range m.presets {
		if p.ID == id {
			if update.Name != "" {
				m.presets[i].Name = update.Name
			}
			if update.Pattern != "" {
				m.presets[i].Pattern = update.Pattern
			}
			if update.Level == "sandbox" || update.Level == "full" {
				m.presets[i].Level = update.Level
			}
			m.presets[i].AutoApprove = update.AutoApprove
			if update.MaxTTL > 0 {
				m.presets[i].MaxTTL = update.MaxTTL
			}
			if update.Description != "" {
				m.presets[i].Description = update.Description
			}
			m.presets[i].UpdatedAt = time.Now()
			return m.saveToDiskLocked()
		}
	}
	return fmt.Errorf("预设 ID 不存在: %s", id)
}

// Remove 删除预设。
func (m *TaskPresetManager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, p := range m.presets {
		if p.ID == id {
			m.presets = append(m.presets[:i], m.presets[i+1:]...)
			return m.saveToDiskLocked()
		}
	}
	return fmt.Errorf("预设 ID 不存在: %s", id)
}

// Match 根据任务名匹配预设。
// 匹配优先级：精确匹配 > 前缀匹配 > glob 通配符。
func (m *TaskPresetManager) Match(taskName string) TaskPresetMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()

	taskNameLower := strings.ToLower(taskName)

	// 1. 精确匹配
	for _, p := range m.presets {
		if strings.ToLower(p.Pattern) == taskNameLower {
			return TaskPresetMatch{Matched: true, Preset: &p, MatchedBy: "exact"}
		}
	}

	// 2. 前缀匹配（pattern 以 * 结尾）
	for _, p := range m.presets {
		pattern := p.Pattern
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.ToLower(strings.TrimSuffix(pattern, "*"))
			if strings.HasPrefix(taskNameLower, prefix) {
				return TaskPresetMatch{Matched: true, Preset: &p, MatchedBy: "prefix"}
			}
		}
	}

	// 3. glob 通配符匹配
	for _, p := range m.presets {
		if globMatch(strings.ToLower(p.Pattern), taskNameLower) {
			return TaskPresetMatch{Matched: true, Preset: &p, MatchedBy: "glob"}
		}
	}

	return TaskPresetMatch{Matched: false}
}

// ---------- glob 匹配 ----------

// globMatch 简单 glob 匹配：支持 * (任意字符序列) 和 ? (单个字符)。
func globMatch(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == "" {
		return name == ""
	}

	pi, ni := 0, 0
	starPI, starNI := -1, -1

	for ni < len(name) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == name[ni]) {
			pi++
			ni++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starPI = pi
			starNI = ni
			pi++
		} else if starPI >= 0 {
			pi = starPI + 1
			starNI++
			ni = starNI
		} else {
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// ---------- 配置持久化 ----------

// taskPresetsFile 任务预设存储在 exec-approvals.json 的 taskPresets 字段中。
// 为简化实现，使用独立文件 task-presets.json。
const taskPresetsFile = "task-presets.json"

func taskPresetsFilePath() string {
	return filepath.Join(config.ResolveStateDir(), taskPresetsFile)
}

func (m *TaskPresetManager) loadFromDisk() error {
	data, err := os.ReadFile(taskPresetsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stored struct {
		Presets []TaskPreset `json:"presets"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	m.presets = stored.Presets
	return nil
}

func (m *TaskPresetManager) saveToDiskLocked() error {
	filePath := taskPresetsFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stored := struct {
		Version int          `json:"version"`
		Presets []TaskPreset `json:"presets"`
	}{
		Version: 1,
		Presets: m.presets,
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0o600)
}
