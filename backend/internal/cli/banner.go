package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// 彩色拼块 Logo — 创宇太虚 / Claw Acosmi
// 紫色系: 创宇太虚 (中文)  ·  红色系: Claw Acosmi (英文)

var (
	bannerOnce sync.Once
	// BannerEnabled 控制是否输出 banner（可通过环境变量或 flag 禁用）
	BannerEnabled = true
)

// ---------- ANSI 颜色 ----------

const (
	ansiReset = "\033[0m"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"

	ansiPurple       = "\033[35m"
	ansiBrightPurple = "\033[95m"
	ansi256Purple1   = "\033[38;5;141m"
	ansi256Purple2   = "\033[38;5;135m"

	ansiRed       = "\033[31m"
	ansiBrightRed = "\033[91m"
	ansi256Red1   = "\033[38;5;196m"
	ansi256Red2   = "\033[38;5;160m"

	ansi256Dim = "\033[38;5;238m"
)

// ---------- Logo 模板 ----------

var blockLogoTemplate = []string{
	"",
	// ── 中文名: 创 宇 太 虚 (紫色大方框) ──
	"  {BP}╔════════╗ ╔════════╗ ╔════════╗ ╔════════╗{X}",
	"  {BP}║        ║ ║        ║ ║        ║ ║        ║{X}",
	"  {BP}║   创   ║ ║   宇   ║ ║   太   ║ ║   虚   ║{X}",
	"  {BP}║        ║ ║        ║ ║        ║ ║        ║{X}",
	"  {BP}╚════════╝ ╚════════╝ ╚════════╝ ╚════════╝{X}",
	"",
	// ── CLAW Figlet (红色) ──
	"   {BR} ██████╗ ██╗      █████╗ ██╗    ██╗{X}",
	"   {BR}██╔════╝ ██║     ██╔══██╗██║    ██║{X}",
	"   {BR}██║      ██║     ███████║██║ █╗ ██║{X}",
	"   {BR}██║      ██║     ██╔══██║██║███╗██║{X}",
	"   {BR}╚██████╗ ███████╗██║  ██║╚███╔███╔╝{X}",
	"   {BR} ╚═════╝ ╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝{X}",
	"",
	// ── ACOSMI Figlet (红色, 略浅) ──
	"  {R2} █████╗  ██████╗ ██████╗ ███████╗███╗   ███╗██╗{X}",
	"  {R2}██╔══██╗██╔════╝██╔═══██╗██╔════╝████╗ ████║██║{X}",
	"  {R2}███████║██║     ██║   ██║███████╗██╔████╔██║██║{X}",
	"  {R2}██╔══██║██║     ██║   ██║╚════██║██║╚██╔╝██║██║{X}",
	"  {R2}██║  ██║╚██████╗╚██████╔╝███████║██║ ╚═╝ ██║██║{X}",
	"  {R2}╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝╚═╝     ╚═╝╚═╝{X}",
}

// compactBannerTemplate 紧凑版
var compactBannerTemplate = []string{
	"",
	"  {BP}创宇太虚{X}  {D}·{X}  {BR}Claw Acosmi{X}",
	"",
}

// taglines 启动标语
var taglines = []string{
	"Your AI, your rules",
	"Lobster-grade intelligence",
	"Pinch-perfect conversations",
}

// ---------- 颜色检测 ----------

func supportsColor() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if IsTruthyEnv("OPENACOSMI_NO_COLOR") {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	stat, err := os.Stdout.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}

func supports256Color() bool {
	if !supportsColor() {
		return false
	}
	ct := os.Getenv("COLORTERM")
	if ct == "truecolor" || ct == "24bit" {
		return true
	}
	if strings.Contains(os.Getenv("TERM"), "256color") {
		return true
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "Apple_Terminal", "iTerm.app", "vscode", "WarpTerminal":
		return true
	}
	return false
}

// ---------- 渲染 ----------

func colorMap(use256 bool) map[string]string {
	if !use256 {
		return map[string]string{
			"{P1}": ansiBold + ansiPurple,
			"{R1}": ansiBold + ansiRed, "{R2}": ansiRed,
			"{BP}": ansiBold + ansiBrightPurple,
			"{BR}": ansiBold + ansiBrightRed,
			"{D}":  ansiDim, "{X}": ansiReset,
		}
	}
	return map[string]string{
		"{P1}": ansi256Purple1,
		"{R1}": ansi256Red1, "{R2}": ansi256Red2,
		"{BP}": ansiBold + ansi256Purple1,
		"{BR}": ansiBold + ansi256Red1,
		"{D}":  ansi256Dim, "{X}": ansiReset,
	}
}

func noColorMap() map[string]string {
	return map[string]string{
		"{P1}": "", "{R1}": "", "{R2}": "",
		"{BP}": "", "{BR}": "", "{D}": "", "{X}": "",
	}
}

func renderBanner(useCompact bool) string {
	hasColor := supportsColor()
	has256 := supports256Color()

	var cmap map[string]string
	if hasColor {
		cmap = colorMap(has256)
	} else {
		cmap = noColorMap()
	}

	template := blockLogoTemplate
	if useCompact {
		template = compactBannerTemplate
	}

	var sb strings.Builder
	for _, line := range template {
		for tag, code := range cmap {
			line = strings.ReplaceAll(line, tag, code)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	commit := ResolveCommitHash()
	tag := taglines[0]
	if hasColor {
		sb.WriteString(fmt.Sprintf("\n  %s创宇太虚%s v%s (%s) — %s%s%s\n\n",
			cmap["{BP}"], ansiReset, Version, commit, cmap["{BR}"], tag, ansiReset))
	} else {
		sb.WriteString(fmt.Sprintf("\n  创宇太虚 v%s (%s) — %s\n\n", Version, commit, tag))
	}
	return sb.String()
}

// ---------- 公开 API ----------

func FormatBannerLine() string {
	commit := ResolveCommitHash()
	return fmt.Sprintf("🦜 创宇太虚 %s (%s) — %s", Version, commit, taglines[0])
}

func FormatBannerArt() string     { return renderBanner(false) }
func FormatBannerCompact() string { return renderBanner(true) }

func EmitBanner() {
	if !BannerEnabled || IsTruthyEnv("OPENACOSMI_HIDE_BANNER") {
		return
	}
	stat, err := os.Stdout.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice == 0 {
		return
	}
	bannerOnce.Do(func() { fmt.Fprint(os.Stdout, FormatBannerArt()) })
}

func EmitBannerCompact() {
	if !BannerEnabled || IsTruthyEnv("OPENACOSMI_HIDE_BANNER") {
		return
	}
	stat, err := os.Stdout.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice == 0 {
		return
	}
	bannerOnce.Do(func() { fmt.Fprint(os.Stdout, FormatBannerCompact()) })
}
