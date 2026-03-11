package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAgentSystemPrompt_Full(t *testing.T) {
	result := BuildAgentSystemPrompt(BuildParams{
		Mode:              PromptModeFull,
		WorkspaceDir:      "/home/user/project",
		ExtraSystemPrompt: "Custom instructions here.",
		OwnerLine:         "User: Alice",
		SkillsPrompt:      "You have access to shell tools.",
		RuntimeInfo: &RuntimeInfo{
			Host:  "myhost",
			OS:    "linux",
			Arch:  "amd64",
			Model: "claude-3.5",
		},
		UserTimezone: "Asia/Shanghai",
		ThinkLevel:   "high",
	})

	// 验证包含各段落
	if !strings.Contains(result, "Crab Claw（蟹爪）") {
		t.Error("missing identity line")
	}
	if !strings.Contains(result, "User: Alice") {
		t.Error("missing user identity section")
	}
	if !strings.Contains(result, "## Workspace") {
		t.Error("missing workspace section")
	}
	if !strings.Contains(result, "Skills") {
		t.Error("missing skills section")
	}
	if !strings.Contains(result, "Custom instructions") {
		t.Error("missing extra prompt")
	}
	if !strings.Contains(result, "Thinking: high") {
		t.Error("missing thinking level in runtime")
	}
}

func TestBuildAgentSystemPrompt_Minimal(t *testing.T) {
	result := BuildAgentSystemPrompt(BuildParams{
		Mode:         PromptModeMinimal,
		OwnerLine:    "User: Bob", // Should be excluded in minimal
		SkillsPrompt: "Shell tools",
		RuntimeInfo:  &RuntimeInfo{Model: "claude-3"},
	})

	// 身份行应存在
	if !strings.Contains(result, "Crab Claw（蟹爪）") {
		t.Error("missing identity")
	}
	// 用户身份在 minimal 模式下被排除
	if strings.Contains(result, "Bob") {
		t.Error("user identity should be excluded in minimal mode")
	}
	// 技能应使用 Summary 标题
	if !strings.Contains(result, "Skills") {
		t.Error("missing skills section")
	}
}

func TestBuildAgentSystemPrompt_SpecializedConfigToolTriggersSelfUpdateSection(t *testing.T) {
	result := BuildAgentSystemPrompt(BuildParams{
		Mode:      PromptModeFull,
		ToolNames: []string{"browser_config"},
		RuntimeInfo: &RuntimeInfo{
			Model: "claude-3.5",
		},
	})

	if !strings.Contains(result, "## Crab Claw（蟹爪） Self-Update") {
		t.Fatal("expected self-update section when specialized config tools are available")
	}
	if !strings.Contains(result, "browser_config") {
		t.Fatal("self-update section should mention specialized config tools")
	}
}

func TestBuildAgentSystemPrompt_ColdStartWithoutLookupSkillAvoidsInternalDirectivePrompt(t *testing.T) {
	result := BuildAgentSystemPrompt(BuildParams{
		Mode:         PromptModeFull,
		SessionState: SessionColdStart,
		ToolNames:    nil,
	})

	if strings.Contains(result, "使用 `lookup_skill` 查找 `acosmi-intro`") {
		t.Fatal("cold start prompt should not require lookup_skill when the tool is unavailable")
	}
	if !strings.Contains(result, "禁止输出任何内部工具/技能查找指令") {
		t.Fatal("cold start prompt should explicitly forbid leaking internal directives without lookup_skill")
	}
}

func TestBuildAgentSystemPrompt_ColdStartWithLookupSkillKeepsSkillGuidance(t *testing.T) {
	result := BuildAgentSystemPrompt(BuildParams{
		Mode:         PromptModeFull,
		SessionState: SessionColdStart,
		ToolNames:    []string{"lookup_skill"},
	})

	if !strings.Contains(result, "使用 `lookup_skill` 查找 `acosmi-intro`") {
		t.Fatal("cold start prompt should keep lookup_skill guidance when the tool is available")
	}
}

func TestBuildAgentSystemPrompt_None(t *testing.T) {
	result := BuildAgentSystemPrompt(BuildParams{
		Mode: PromptModeNone,
	})

	if !strings.Contains(result, "Crab Claw（蟹爪）") {
		t.Error("missing identity")
	}
	// 不应包含段落
	if strings.Contains(result, "## Runtime") {
		t.Error("should not contain Runtime section in none mode")
	}
}

func TestFindGitRoot(t *testing.T) {
	// 创建临时 git 目录
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 从子目录查找
	subDir := filepath.Join(dir, "src", "lib")
	os.MkdirAll(subDir, 0o755)

	root := FindGitRoot(subDir)
	if root != dir {
		t.Errorf("FindGitRoot(%q) = %q, want %q", subDir, root, dir)
	}

	// 无 git 目录
	noGit := t.TempDir()
	root = FindGitRoot(noGit)
	if root != "" {
		t.Errorf("FindGitRoot(no-git) = %q, want empty", root)
	}
}

func TestDefaultRuntimeInfo(t *testing.T) {
	info := DefaultRuntimeInfo()
	if info.OS == "" {
		t.Error("OS should not be empty")
	}
	if info.Arch == "" {
		t.Error("Arch should not be empty")
	}
	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
}

func TestBuildRuntimeLine(t *testing.T) {
	rt := &RuntimeInfo{
		Host:  "test-host",
		OS:    "darwin",
		Arch:  "arm64",
		Model: "claude-3",
	}
	line := buildRuntimeLine(rt, "off")
	if !strings.Contains(line, "test-host") {
		t.Error("missing host")
	}
	if strings.Contains(line, "Thinking") {
		t.Error("thinking:off should not appear")
	}

	line2 := buildRuntimeLine(rt, "high")
	if !strings.Contains(line2, "Thinking: high") {
		t.Error("missing thinking level")
	}
}
