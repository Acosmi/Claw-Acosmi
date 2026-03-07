package hooks

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// --- Soul Evil Override ---
// 对应 TS: soul-evil.ts

const DefaultSoulEvilFilename = "SOUL_EVIL.md"

// SoulEvilConfig Soul Evil 配置
// 对应 TS: soul-evil.ts SoulEvilConfig
type SoulEvilConfig struct {
	File   string         `json:"file,omitempty"`
	Chance *float64       `json:"chance,omitempty"`
	Purge  *SoulEvilPurge `json:"purge,omitempty"`
}

// SoulEvilPurge 清洗窗口配置
type SoulEvilPurge struct {
	At       string `json:"at,omitempty"`       // HH:mm
	Duration string `json:"duration,omitempty"` // e.g. "30s", "10m", "1h"
}

// SoulEvilDecision 决策结果
type SoulEvilDecision struct {
	UseEvil  bool   `json:"useEvil"`
	Reason   string `json:"reason,omitempty"` // "purge"|"chance"
	FileName string `json:"fileName"`
}

// SoulEvilCheckParams 检查参数
type SoulEvilCheckParams struct {
	Config       *SoulEvilConfig
	UserTimezone string
	Now          time.Time
	Random       func() float64
}

// SoulEvilLog 日志接口
type SoulEvilLog struct {
	Debug func(string)
	Warn  func(string)
}

// ResolveSoulEvilConfigFromHook 从 hook 条目解析 Soul Evil 配置
// 对应 TS: soul-evil.ts resolveSoulEvilConfigFromHook
func ResolveSoulEvilConfigFromHook(entry map[string]interface{}, log *SoulEvilLog) *SoulEvilConfig {
	if entry == nil {
		return nil
	}

	var fileStr string
	if f, ok := entry["file"].(string); ok {
		fileStr = f
	} else if entry["file"] != nil && log != nil && log.Warn != nil {
		log.Warn("soul-evil config: file must be a string")
	}

	var chance *float64
	if c, ok := entry["chance"].(float64); ok && math.IsInf(c, 0) == false && !math.IsNaN(c) {
		chance = &c
	} else if entry["chance"] != nil && log != nil && log.Warn != nil {
		log.Warn("soul-evil config: chance must be a number")
	}

	var purge *SoulEvilPurge
	if p, ok := entry["purge"].(map[string]interface{}); ok {
		purge = &SoulEvilPurge{}
		if at, ok := p["at"].(string); ok {
			purge.At = at
		} else if p["at"] != nil && log != nil && log.Warn != nil {
			log.Warn("soul-evil config: purge.at must be a string")
		}
		if dur, ok := p["duration"].(string); ok {
			purge.Duration = dur
		} else if p["duration"] != nil && log != nil && log.Warn != nil {
			log.Warn("soul-evil config: purge.duration must be a string")
		}
	} else if entry["purge"] != nil && log != nil && log.Warn != nil {
		log.Warn("soul-evil config: purge must be an object")
	}

	if fileStr == "" && chance == nil && purge == nil {
		return nil
	}
	return &SoulEvilConfig{File: fileStr, Chance: chance, Purge: purge}
}

// DecideSoulEvil 决定是否使用 Soul Evil
// 对应 TS: soul-evil.ts decideSoulEvil
func DecideSoulEvil(params SoulEvilCheckParams) SoulEvilDecision {
	evil := params.Config
	fileName := DefaultSoulEvilFilename
	if evil != nil && strings.TrimSpace(evil.File) != "" {
		fileName = strings.TrimSpace(evil.File)
	}
	if evil == nil {
		return SoulEvilDecision{UseEvil: false, FileName: fileName}
	}

	tz := resolveTimezone(params.UserTimezone)
	now := params.Now
	if now.IsZero() {
		now = time.Now()
	}

	if evil.Purge != nil {
		inPurge := isWithinDailyPurgeWindow(evil.Purge.At, evil.Purge.Duration, now, tz)
		if inPurge {
			return SoulEvilDecision{UseEvil: true, Reason: "purge", FileName: fileName}
		}
	}

	chance := clampChance(evil.Chance)
	if chance > 0 {
		randomFn := params.Random
		if randomFn == nil {
			randomFn = rand.Float64
		}
		if randomFn() < chance {
			return SoulEvilDecision{UseEvil: true, Reason: "chance", FileName: fileName}
		}
	}

	return SoulEvilDecision{UseEvil: false, FileName: fileName}
}

// WorkspaceBootstrapFile workspace 引导文件
type WorkspaceBootstrapFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Missing bool   `json:"missing,omitempty"`
}

// ApplySoulEvilOverride 应用 Soul Evil 替换
// 对应 TS: soul-evil.ts applySoulEvilOverride
func ApplySoulEvilOverride(params ApplySoulEvilParams) []WorkspaceBootstrapFile {
	decision := DecideSoulEvil(SoulEvilCheckParams{
		Config:       params.Config,
		UserTimezone: params.UserTimezone,
		Now:          params.Now,
		Random:       params.Random,
	})
	if !decision.UseEvil {
		return params.Files
	}

	workspaceDir := params.WorkspaceDir
	evilPath := filepath.Join(workspaceDir, decision.FileName)
	evilContent, err := os.ReadFile(evilPath)
	if err != nil {
		if params.Log != nil && params.Log.Warn != nil {
			params.Log.Warn("SOUL_EVIL active (" + decision.Reason + ") but file missing: " + evilPath)
		}
		return params.Files
	}

	if strings.TrimSpace(string(evilContent)) == "" {
		if params.Log != nil && params.Log.Warn != nil {
			params.Log.Warn("SOUL_EVIL active (" + decision.Reason + ") but file empty: " + evilPath)
		}
		return params.Files
	}

	hasSoul := false
	for _, f := range params.Files {
		if f.Name == "SOUL.md" {
			hasSoul = true
			break
		}
	}
	if !hasSoul {
		if params.Log != nil && params.Log.Warn != nil {
			params.Log.Warn("SOUL_EVIL active (" + decision.Reason + ") but SOUL.md not in bootstrap files")
		}
		return params.Files
	}

	replaced := false
	result := make([]WorkspaceBootstrapFile, len(params.Files))
	for i, f := range params.Files {
		if f.Name == "SOUL.md" {
			result[i] = WorkspaceBootstrapFile{
				Name:    f.Name,
				Content: string(evilContent),
				Missing: false,
			}
			replaced = true
		} else {
			result[i] = f
		}
	}
	if !replaced {
		return params.Files
	}

	if params.Log != nil && params.Log.Debug != nil {
		params.Log.Debug("SOUL_EVIL active (" + decision.Reason + ") using " + decision.FileName)
	}

	return result
}

// ApplySoulEvilParams 应用参数
type ApplySoulEvilParams struct {
	Files        []WorkspaceBootstrapFile
	WorkspaceDir string
	Config       *SoulEvilConfig
	UserTimezone string
	Now          time.Time
	Random       func() float64
	Log          *SoulEvilLog
}

// --- 辅助函数 ---

func clampChance(value *float64) float64 {
	if value == nil || math.IsInf(*value, 0) || math.IsNaN(*value) {
		return 0
	}
	return math.Min(1, math.Max(0, *value))
}

var purgeAtRe = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

func parsePurgeAt(raw string) (minutes int, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	m := purgeAtRe.FindStringSubmatch(trimmed)
	if m == nil {
		return 0, false
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	return hour*60 + minute, true
}

func parseDurationMs(raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	// 支持常见格式: 30s, 10m, 1h, 500ms
	if strings.HasSuffix(trimmed, "ms") {
		num := strings.TrimSuffix(trimmed, "ms")
		if v, err := strconv.ParseFloat(num, 64); err == nil && v > 0 {
			return int64(v), true
		}
		return 0, false
	}

	unit := trimmed[len(trimmed)-1:]
	numStr := trimmed[:len(trimmed)-1]
	v, err := strconv.ParseFloat(numStr, 64)
	if err != nil || v <= 0 {
		return 0, false
	}

	switch unit {
	case "s":
		return int64(v * 1000), true
	case "m":
		return int64(v * 60 * 1000), true
	case "h":
		return int64(v * 3600 * 1000), true
	case "d":
		return int64(v * 86400 * 1000), true
	default:
		// 默认单位分钟
		if v2, err := strconv.ParseFloat(trimmed, 64); err == nil && v2 > 0 {
			return int64(v2 * 60 * 1000), true
		}
		return 0, false
	}
}

func timeOfDayMsInTimezone(t time.Time, tz string) (int64, bool) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return 0, false
	}
	local := t.In(loc)
	hour, minute, second := local.Clock()
	ms := int64(hour)*3600000 + int64(minute)*60000 + int64(second)*1000 + int64(local.Nanosecond()/1e6)
	return ms, true
}

func isWithinDailyPurgeWindow(at, duration string, now time.Time, tz string) bool {
	if at == "" || duration == "" {
		return false
	}
	startMinutes, ok := parsePurgeAt(at)
	if !ok {
		return false
	}
	durationMs, ok := parseDurationMs(duration)
	if !ok || durationMs <= 0 {
		return false
	}

	const dayMs = 24 * 60 * 60 * 1000
	if durationMs >= dayMs {
		return true
	}

	nowMs, ok := timeOfDayMsInTimezone(now, tz)
	if !ok {
		return false
	}

	startMs := int64(startMinutes) * 60 * 1000
	endMs := startMs + durationMs
	if endMs < dayMs {
		return nowMs >= startMs && nowMs < endMs
	}
	wrappedEnd := endMs % dayMs
	return nowMs >= startMs || nowMs < wrappedEnd
}

func resolveTimezone(userTZ string) string {
	if userTZ != "" {
		return userTZ
	}
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	return "UTC"
}
