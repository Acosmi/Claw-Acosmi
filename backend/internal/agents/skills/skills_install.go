package skills

// skills_install.go — 技能安装管线
// 对应 TS: agents/skills-install.ts (572L)
//
// 实现 InstallSkillFromSpec — 第三方依赖安装 (brew/node/go/uv/download)。
// gateway skills.install 方法调用此函数。

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/statepaths"
	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

// SkillInstallRequest 技能安装请求。
type SkillInstallRequest struct {
	WorkspaceDir string
	SkillName    string
	InstallID    string
	TimeoutMs    int
	Config       *types.OpenAcosmiConfig
}

// SkillInstallResult 技能安装结果。
type SkillInstallResult struct {
	OK       bool     `json:"ok"`
	Message  string   `json:"message"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	Code     *int     `json:"code"`
	Warnings []string `json:"warnings,omitempty"`
}

// InstallSkillFromSpec 执行技能安装。
// 对应 TS: skills-install.ts → installSkill
func InstallSkillFromSpec(req SkillInstallRequest) SkillInstallResult {
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 300_000
	}
	if timeoutMs < 1_000 {
		timeoutMs = 1_000
	}
	if timeoutMs > 900_000 {
		timeoutMs = 900_000
	}

	// 加载技能条目
	entries := LoadSkillEntries(req.WorkspaceDir, "", ResolveBundledSkillsDir(""), req.Config)
	var entry *SkillEntry
	for i := range entries {
		if entries[i].Skill.Name == req.SkillName {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return SkillInstallResult{
			OK:      false,
			Message: fmt.Sprintf("Skill not found: %s", req.SkillName),
		}
	}

	// 查找安装规格
	spec := findInstallSpec(entry, req.InstallID)
	if spec == nil {
		return SkillInstallResult{
			OK:      false,
			Message: fmt.Sprintf("Installer not found: %s", req.InstallID),
		}
	}

	// 按种类安装
	if spec.Kind == "download" {
		return installDownloadSpec(entry, spec, timeoutMs)
	}

	// 构建命令
	prefs := ResolveSkillsInstallPreferences(req.Config)
	argv, cmdErr := buildInstallCommand(spec, prefs)
	if cmdErr != "" {
		return SkillInstallResult{
			OK:      false,
			Message: cmdErr,
		}
	}
	if len(argv) == 0 {
		return SkillInstallResult{
			OK:      false,
			Message: "invalid install command",
		}
	}

	// brew 可执行文件解析
	if spec.Kind == "brew" && !hasBinaryOnPath("brew") {
		return SkillInstallResult{
			OK:      false,
			Message: "brew not installed",
		}
	}

	// uv 检测
	if spec.Kind == "uv" && !hasBinaryOnPath("uv") {
		if hasBinaryOnPath("brew") {
			result := runWithTimeout([]string{"brew", "install", "uv"}, "", nil, timeoutMs)
			if result.Code == nil || *result.Code != 0 {
				return SkillInstallResult{
					OK:      false,
					Message: "Failed to install uv (brew)",
					Stdout:  result.Stdout,
					Stderr:  result.Stderr,
					Code:    result.Code,
				}
			}
		} else {
			return SkillInstallResult{
				OK:      false,
				Message: "uv not installed (install via brew)",
			}
		}
	}

	// go 检测
	if spec.Kind == "go" && !hasBinaryOnPath("go") {
		if hasBinaryOnPath("brew") {
			result := runWithTimeout([]string{"brew", "install", "go"}, "", nil, timeoutMs)
			if result.Code == nil || *result.Code != 0 {
				return SkillInstallResult{
					OK:      false,
					Message: "Failed to install go (brew)",
					Stdout:  result.Stdout,
					Stderr:  result.Stderr,
					Code:    result.Code,
				}
			}
		} else {
			return SkillInstallResult{
				OK:      false,
				Message: "go not installed (install via brew)",
			}
		}
	}

	// 设置环境变量
	var env []string
	if spec.Kind == "go" && hasBinaryOnPath("brew") {
		if brewBin := resolveBrewBinDir(timeoutMs); brewBin != "" {
			env = append(os.Environ(), "GOBIN="+brewBin)
		}
	}

	// 执行安装命令
	result := runWithTimeout(argv, "", env, timeoutMs)
	success := result.Code != nil && *result.Code == 0
	msg := "Installed"
	if !success {
		msg = formatInstallFailureMessage(result)
	}

	return SkillInstallResult{
		OK:      success,
		Message: msg,
		Stdout:  result.Stdout,
		Stderr:  result.Stderr,
		Code:    result.Code,
	}
}

// ---------- 安装规格查找 ----------

// resolveInstallID 解析安装规格 ID。
func resolveInstallID(spec SkillInstallSpec, index int) string {
	if id := strings.TrimSpace(spec.ID); id != "" {
		return id
	}
	return fmt.Sprintf("%s-%d", spec.Kind, index)
}

// findInstallSpec 按 installId 查找安装规格。
func findInstallSpec(entry *SkillEntry, installID string) *SkillInstallSpec {
	if entry.Metadata == nil {
		return nil
	}
	for i, spec := range entry.Metadata.Install {
		if resolveInstallID(spec, i) == installID {
			return &entry.Metadata.Install[i]
		}
	}
	return nil
}

// ---------- 命令构建 ----------

// buildInstallCommand 构建安装命令行。
// 对应 TS: buildInstallCommand
func buildInstallCommand(spec *SkillInstallSpec, prefs SkillsInstallPreferences) ([]string, string) {
	switch spec.Kind {
	case "brew":
		if spec.Formula == "" {
			return nil, "missing brew formula"
		}
		return []string{"brew", "install", spec.Formula}, ""
	case "node":
		if spec.Package == "" {
			return nil, "missing node package"
		}
		return buildNodeInstallCommand(spec.Package, prefs), ""
	case "go":
		if spec.Module == "" {
			return nil, "missing go module"
		}
		return []string{"go", "install", spec.Module}, ""
	case "uv":
		if spec.Package == "" {
			return nil, "missing uv package"
		}
		return []string{"uv", "tool", "install", spec.Package}, ""
	case "download":
		return nil, "download install handled separately"
	default:
		return nil, "unsupported installer"
	}
}

// buildNodeInstallCommand 构建 Node 包管理器安装命令。
func buildNodeInstallCommand(packageName string, prefs SkillsInstallPreferences) []string {
	switch prefs.NodeManager {
	case "pnpm":
		return []string{"pnpm", "add", "-g", packageName}
	case "yarn":
		return []string{"yarn", "global", "add", packageName}
	case "bun":
		return []string{"bun", "add", "-g", packageName}
	default:
		return []string{"npm", "install", "-g", packageName}
	}
}

// ---------- 下载安装 ----------

// installDownloadSpec 处理 download 类型安装。
func installDownloadSpec(entry *SkillEntry, spec *SkillInstallSpec, timeoutMs int) SkillInstallResult {
	urlStr := strings.TrimSpace(spec.URL)
	if urlStr == "" {
		return SkillInstallResult{OK: false, Message: "missing download url"}
	}

	filename := ""
	if parsed, err := url.Parse(urlStr); err == nil {
		filename = filepath.Base(parsed.Path)
	}
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	targetDir := resolveDownloadTargetDir(entry, spec)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return SkillInstallResult{OK: false, Message: fmt.Sprintf("failed to create directory: %v", err)}
	}

	archivePath := filepath.Join(targetDir, filename)
	downloaded, err := downloadFile(urlStr, archivePath, timeoutMs)
	if err != nil {
		return SkillInstallResult{OK: false, Message: err.Error(), Stderr: err.Error()}
	}

	archiveType := resolveArchiveType(spec, filename)
	shouldExtract := false
	if spec.Extract != nil {
		shouldExtract = *spec.Extract
	} else {
		shouldExtract = archiveType != ""
	}

	if !shouldExtract {
		return SkillInstallResult{
			OK:      true,
			Message: fmt.Sprintf("Downloaded to %s", archivePath),
			Stdout:  fmt.Sprintf("downloaded=%d", downloaded),
		}
	}

	if archiveType == "" {
		return SkillInstallResult{
			OK:      false,
			Message: "extract requested but archive type could not be detected",
		}
	}

	result := extractArchive(archivePath, archiveType, targetDir, spec.StripComponents, timeoutMs)
	success := result.Code != nil && *result.Code == 0
	msg := fmt.Sprintf("Downloaded and extracted to %s", targetDir)
	if !success {
		msg = formatInstallFailureMessage(result)
	}

	return SkillInstallResult{
		OK:      success,
		Message: msg,
		Stdout:  result.Stdout,
		Stderr:  result.Stderr,
		Code:    result.Code,
	}
}

// resolveDownloadTargetDir 解析下载目标目录。
func resolveDownloadTargetDir(entry *SkillEntry, spec *SkillInstallSpec) string {
	if dir := strings.TrimSpace(spec.TargetDir); dir != "" {
		if filepath.IsAbs(dir) {
			return dir
		}
		home, err := os.UserHomeDir()
		if err == nil && strings.HasPrefix(dir, "~") {
			return filepath.Join(home, dir[1:])
		}
		return dir
	}
	key := ResolveSkillKey(entry.Skill.Name, entry.Metadata)
	configDir := resolveConfigDir()
	return filepath.Join(configDir, "tools", key)
}

// resolveConfigDir 解析配置目录路径。
func resolveConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "openacosmi")
	}
	return statepaths.ResolveStateDir()
}

// resolveArchiveType 解析 archive 类型。
func resolveArchiveType(spec *SkillInstallSpec, filename string) string {
	if explicit := strings.TrimSpace(strings.ToLower(spec.Archive)); explicit != "" {
		return explicit
	}
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar.gz"
	}
	if strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2") {
		return "tar.bz2"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}
	return ""
}

// ---------- 文件下载 ----------

// downloadFile 下载文件到指定路径。
func downloadFile(urlStr, destPath string, timeoutMs int) (int64, error) {
	timeout := time.Duration(math.Max(float64(timeoutMs), 1000)) * time.Millisecond
	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(urlStr)
	if err != nil {
		return 0, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download failed (%d %s)", resp.StatusCode, resp.Status)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("download write failed: %w", err)
	}
	return n, nil
}

// ---------- Archive 解压 ----------

// extractArchive 解压 archive 文件。
func extractArchive(archivePath, archiveType, targetDir string, stripComponents *int, timeoutMs int) cmdResult {
	if archiveType == "zip" {
		if !hasBinaryOnPath("unzip") {
			return cmdResult{Stderr: "unzip not found on PATH"}
		}
		return runWithTimeout([]string{"unzip", "-q", archivePath, "-d", targetDir}, "", nil, timeoutMs)
	}

	if !hasBinaryOnPath("tar") {
		return cmdResult{Stderr: "tar not found on PATH"}
	}
	argv := []string{"tar", "xf", archivePath, "-C", targetDir}
	if stripComponents != nil && *stripComponents >= 0 {
		argv = append(argv, "--strip-components", fmt.Sprintf("%d", *stripComponents))
	}
	return runWithTimeout(argv, "", nil, timeoutMs)
}

// ---------- 命令执行 ----------

type cmdResult struct {
	Stdout string
	Stderr string
	Code   *int
}

// runWithTimeout 执行外部命令（带超时）。
func runWithTimeout(argv []string, cwd string, env []string, timeoutMs int) cmdResult {
	if len(argv) == 0 {
		return cmdResult{Stderr: "empty command"}
	}

	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if len(env) > 0 {
		cmd.Env = env
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			return cmdResult{
				Stdout: strings.TrimSpace(stdout.String()),
				Stderr: strings.TrimSpace(stderr.String()) + "\n" + err.Error(),
			}
		}
	}

	return cmdResult{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
		Code:   &code,
	}
}

// ---------- Brew 辅助 ----------

// resolveBrewBinDir 解析 Homebrew bin 目录。
func resolveBrewBinDir(timeoutMs int) string {
	brewExe := "brew"
	if !hasBinaryOnPath(brewExe) {
		// 尝试常见路径
		for _, candidate := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if _, err := os.Stat(candidate); err == nil {
				brewExe = candidate
				break
			}
		}
	}

	result := runWithTimeout([]string{brewExe, "--prefix"}, "", nil, min(timeoutMs, 30_000))
	if result.Code != nil && *result.Code == 0 {
		prefix := strings.TrimSpace(result.Stdout)
		if prefix != "" {
			return filepath.Join(prefix, "bin")
		}
	}

	if envPrefix := strings.TrimSpace(os.Getenv("HOMEBREW_PREFIX")); envPrefix != "" {
		return filepath.Join(envPrefix, "bin")
	}

	for _, candidate := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// ---------- 辅助函数 ----------

// hasBinaryOnPath 检查 PATH 中是否存在指定二进制。
// 与 eligibility.go 中的 HasBinary 功能相同，但为避免循环依赖使用独立函数。
func hasBinaryOnPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// summarizeInstallOutput 提取安装输出中的关键信息。
func summarizeInstallOutput(text string) string {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	var filtered []string
	for _, line := range lines {
		if l := strings.TrimSpace(line); l != "" {
			filtered = append(filtered, l)
		}
	}
	if len(filtered) == 0 {
		return ""
	}

	// 优先查找 error 行
	var preferred string
	for _, line := range filtered {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "error") {
			preferred = line
			break
		}
		if preferred == "" && (strings.Contains(lower, "error:") || strings.Contains(lower, "failed")) {
			preferred = line
		}
	}
	if preferred == "" {
		preferred = filtered[len(filtered)-1]
	}

	normalized := strings.Join(strings.Fields(preferred), " ")
	const maxLen = 200
	if len(normalized) > maxLen {
		return normalized[:maxLen-1] + "…"
	}
	return normalized
}

// formatInstallFailureMessage 格式化安装失败消息。
func formatInstallFailureMessage(result cmdResult) string {
	codeStr := "unknown exit"
	if result.Code != nil {
		codeStr = fmt.Sprintf("exit %d", *result.Code)
	}
	summary := summarizeInstallOutput(result.Stderr)
	if summary == "" {
		summary = summarizeInstallOutput(result.Stdout)
	}
	if summary == "" {
		return fmt.Sprintf("Install failed (%s)", codeStr)
	}
	return fmt.Sprintf("Install failed (%s): %s", codeStr, summary)
}

func init() {
	// 避免 linter 报 slog 未使用
	_ = slog.Default()
}
