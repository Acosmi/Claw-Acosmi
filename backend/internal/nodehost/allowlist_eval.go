package nodehost

// allowlist_eval.go — 白名单评估与审批逻辑
// 对应 TS: exec-approvals.ts L538-1464

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Acosmi/ClawAcosmi/internal/infra"
)

// ---------- 模式匹配 ----------

// normalizeMatchTarget 规范化匹配目标路径。
func normalizeMatchTarget(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, `\`, "/"))
}

// matchesPattern 检查 target 是否匹配 pattern（glob 匹配）。
func matchesPattern(pattern, target string) bool {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return false
	}
	expanded := trimmed
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	normalizedPattern := normalizeMatchTarget(expanded)
	normalizedTarget := normalizeMatchTarget(target)
	re := globToRegExp(normalizedPattern)
	if re == nil {
		return false
	}
	return re.MatchString(normalizedTarget)
}

// matchAllowlist 在白名单中查找匹配条目。
func matchAllowlist(entries []infra.ExecAllowlistEntry, resolution *CommandResolution) *infra.ExecAllowlistEntry {
	if len(entries) == 0 || resolution == nil || resolution.ResolvedPath == "" {
		return nil
	}
	for i := range entries {
		pattern := strings.TrimSpace(entries[i].Pattern)
		if pattern == "" {
			continue
		}
		hasPath := strings.ContainsAny(pattern, `/\~`)
		if !hasPath {
			continue
		}
		if matchesPattern(pattern, resolution.ResolvedPath) {
			return &entries[i]
		}
	}
	return nil
}

// ---------- Safe Bins ----------

// ResolveSafeBins 解析安全命令集合。
func ResolveSafeBins(entries []string) map[string]struct{} {
	if entries == nil {
		entries = DefaultSafeBins
	}
	result := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		trimmed := strings.TrimSpace(strings.ToLower(e))
		if trimmed != "" {
			result[trimmed] = struct{}{}
		}
	}
	return result
}

// isSafeBinUsage 检查命令是否属于安全命令使用（只读、无文件路径参数）。
func isSafeBinUsage(argv []string, resolution *CommandResolution, safeBins map[string]struct{}, cwd string) bool {
	if len(safeBins) == 0 || resolution == nil {
		return false
	}
	execName := strings.ToLower(resolution.ExecutableName)
	if execName == "" {
		return false
	}
	if _, ok := safeBins[execName]; !ok {
		return false
	}
	if resolution.ResolvedPath == "" {
		return false
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	for _, token := range argv[1:] {
		if token == "" || token == "-" {
			continue
		}
		if strings.HasPrefix(token, "-") {
			eqIdx := strings.Index(token, "=")
			if eqIdx > 0 {
				value := token[eqIdx+1:]
				if value != "" && isPathLikeToken(value) {
					return false
				}
			}
			continue
		}
		if isPathLikeToken(token) {
			return false
		}
		candidate := filepath.Join(cwd, token)
		if _, err := os.Stat(candidate); err == nil {
			return false
		}
	}
	return true
}

func isPathLikeToken(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "-" {
		return false
	}
	if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "~") {
		return true
	}
	return strings.HasPrefix(trimmed, "/")
}

// ---------- 段评估 ----------

func resolveAllowlistCandidatePath(resolution *CommandResolution, cwd string) string {
	if resolution == nil {
		return ""
	}
	if resolution.ResolvedPath != "" {
		return resolution.ResolvedPath
	}
	raw := strings.TrimSpace(resolution.RawExecutable)
	if raw == "" {
		return ""
	}
	expanded := raw
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	if !strings.Contains(expanded, "/") && !strings.Contains(expanded, `\`) {
		return ""
	}
	if filepath.IsAbs(expanded) {
		return expanded
	}
	base := cwd
	if base == "" {
		base, _ = os.Getwd()
	}
	return filepath.Join(base, expanded)
}

func evaluateSegments(
	segments []ExecCommandSegment,
	allowlist []infra.ExecAllowlistEntry,
	safeBins map[string]struct{},
	cwd string,
	skillBins map[string]struct{},
	autoAllowSkills bool,
) (satisfied bool, matches []infra.ExecAllowlistEntry) {
	allowSkills := autoAllowSkills && len(skillBins) > 0

	allSatisfied := true
	for _, seg := range segments {
		candidatePath := resolveAllowlistCandidatePath(seg.Resolution, cwd)
		candidateRes := seg.Resolution
		if candidatePath != "" && candidateRes != nil {
			cr := *candidateRes
			cr.ResolvedPath = candidatePath
			candidateRes = &cr
		}
		match := matchAllowlist(allowlist, candidateRes)
		if match != nil {
			matches = append(matches, *match)
		}
		safe := isSafeBinUsage(seg.Argv, seg.Resolution, safeBins, cwd)
		skillAllow := false
		if allowSkills && seg.Resolution != nil && seg.Resolution.ExecutableName != "" {
			_, skillAllow = skillBins[seg.Resolution.ExecutableName]
		}
		if match == nil && !safe && !skillAllow {
			allSatisfied = false
		}
	}
	return allSatisfied, matches
}

// ---------- 公开评估 API ----------

// EvaluateExecAllowlist 评估 argv 命令的白名单。
func EvaluateExecAllowlist(
	analysis ExecCommandAnalysis,
	allowlist []infra.ExecAllowlistEntry,
	safeBins map[string]struct{},
	cwd string,
	skillBins map[string]struct{},
	autoAllowSkills bool,
) ExecAllowlistEvaluation {
	if !analysis.OK || len(analysis.Segments) == 0 {
		return ExecAllowlistEvaluation{AllowlistSatisfied: false}
	}
	if analysis.Chains != nil {
		var allMatches []infra.ExecAllowlistEntry
		for _, chain := range analysis.Chains {
			sat, m := evaluateSegments(chain, allowlist, safeBins, cwd, skillBins, autoAllowSkills)
			if !sat {
				return ExecAllowlistEvaluation{AllowlistSatisfied: false}
			}
			allMatches = append(allMatches, m...)
		}
		return ExecAllowlistEvaluation{AllowlistSatisfied: true, AllowlistMatches: allMatches}
	}
	sat, m := evaluateSegments(analysis.Segments, allowlist, safeBins, cwd, skillBins, autoAllowSkills)
	return ExecAllowlistEvaluation{AllowlistSatisfied: sat, AllowlistMatches: m}
}

// EvaluateShellAllowlist 评估 shell 命令的白名单。
// platform 为空时使用当前运行时平台；传入 "win*" 则使用 Windows 分词器。
func EvaluateShellAllowlist(
	command string,
	allowlist []infra.ExecAllowlistEntry,
	safeBins map[string]struct{},
	cwd string,
	env map[string]string,
	skillBins map[string]struct{},
	autoAllowSkills bool,
	platform string,
) ExecAllowlistAnalysis {
	chainParts := splitCommandChain(command)
	if chainParts == nil {
		analysis := AnalyzeShellCommand(command, cwd, env, platform)
		if !analysis.OK {
			return ExecAllowlistAnalysis{AnalysisOk: false}
		}
		eval := EvaluateExecAllowlist(analysis, allowlist, safeBins, cwd, skillBins, autoAllowSkills)
		return ExecAllowlistAnalysis{
			AnalysisOk:         true,
			AllowlistSatisfied: eval.AllowlistSatisfied,
			AllowlistMatches:   eval.AllowlistMatches,
			Segments:           analysis.Segments,
		}
	}
	var allMatches []infra.ExecAllowlistEntry
	var allSegments []ExecCommandSegment
	for _, part := range chainParts {
		analysis := AnalyzeShellCommand(part, cwd, env, platform)
		if !analysis.OK {
			return ExecAllowlistAnalysis{AnalysisOk: false}
		}
		allSegments = append(allSegments, analysis.Segments...)
		eval := EvaluateExecAllowlist(analysis, allowlist, safeBins, cwd, skillBins, autoAllowSkills)
		allMatches = append(allMatches, eval.AllowlistMatches...)
		if !eval.AllowlistSatisfied {
			return ExecAllowlistAnalysis{
				AnalysisOk:         true,
				AllowlistSatisfied: false,
				AllowlistMatches:   allMatches,
				Segments:           allSegments,
			}
		}
	}
	return ExecAllowlistAnalysis{
		AnalysisOk:         true,
		AllowlistSatisfied: true,
		AllowlistMatches:   allMatches,
		Segments:           allSegments,
	}
}

// RequiresExecApproval 判断命令是否需要审批。
func RequiresExecApproval(ask infra.ExecAsk, security infra.ExecSecurity, analysisOk, allowlistSatisfied bool) bool {
	if ask == infra.ExecAskAlways {
		return true
	}
	return ask == infra.ExecAskOnMiss &&
		security == infra.ExecSecurityAllowlist &&
		(!analysisOk || !allowlistSatisfied)
}

// RecordAllowlistUse 记录白名单被使用。
// 使用 exec-approvals 锁保护写入，防止并发丢失。
func RecordAllowlistUse(
	approvals *infra.ExecApprovalsFile,
	agentID string,
	entry infra.ExecAllowlistEntry,
	command, resolvedPath string,
) {
	if agentID == "" {
		agentID = "main"
	}
	if approvals.Agents == nil {
		approvals.Agents = make(map[string]*infra.ExecApprovalsAgent)
	}
	agent := approvals.Agents[agentID]
	if agent == nil {
		agent = &infra.ExecApprovalsAgent{}
		approvals.Agents[agentID] = agent
	}
	now := time.Now().UnixMilli()
	for i := range agent.Allowlist {
		if agent.Allowlist[i].Pattern == entry.Pattern {
			if agent.Allowlist[i].ID == "" {
				agent.Allowlist[i].ID = uuid.NewString()
			}
			agent.Allowlist[i].LastUsedAt = &now
			agent.Allowlist[i].LastUsedCommand = command
			agent.Allowlist[i].LastResolvedPath = resolvedPath
			break
		}
	}
	unlock := infra.AcquireExecApprovalsLock()
	_ = infra.SaveExecApprovals(approvals)
	unlock()
}

// AddAllowlistEntry 添加新白名单条目。
// 使用 exec-approvals 锁保护写入，防止并发丢失。
func AddAllowlistEntry(approvals *infra.ExecApprovalsFile, agentID, pattern string) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return
	}
	if agentID == "" {
		agentID = "main"
	}
	if approvals.Agents == nil {
		approvals.Agents = make(map[string]*infra.ExecApprovalsAgent)
	}
	agent := approvals.Agents[agentID]
	if agent == nil {
		agent = &infra.ExecApprovalsAgent{}
		approvals.Agents[agentID] = agent
	}
	for _, e := range agent.Allowlist {
		if e.Pattern == trimmed {
			return
		}
	}
	now := time.Now().UnixMilli()
	agent.Allowlist = append(agent.Allowlist, infra.ExecAllowlistEntry{
		ID:         uuid.NewString(),
		Pattern:    trimmed,
		LastUsedAt: &now,
	})
	unlock := infra.AcquireExecApprovalsLock()
	_ = infra.SaveExecApprovals(approvals)
	unlock()
}
