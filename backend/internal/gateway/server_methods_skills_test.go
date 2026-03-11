package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentSkills "github.com/Acosmi/ClawAcosmi/internal/agents/skills"
	"github.com/Acosmi/ClawAcosmi/internal/argus"
	"github.com/Acosmi/ClawAcosmi/internal/config"
)

func TestNormalizeSkillStatusSource(t *testing.T) {
	t.Parallel()

	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	docsSkillsDir := filepath.Join(workspaceDir, "docs", "skills")
	paths := skillStatusPaths{
		WorkspaceDir:  workspaceDir,
		DocsSkillsDir: docsSkillsDir,
		SyncedDir:     filepath.Join(docsSkillsDir, "synced"),
		BundledDir:    filepath.Join(workspaceDir, "bundled"),
		ExtraDirs:     []string{filepath.Join(workspaceDir, "vendor-skills")},
	}

	tests := []struct {
		name       string
		sourceHint string
		skillDir   string
		want       string
	}{
		{
			name:     "agent workspace skill",
			skillDir: filepath.Join(workspaceDir, ".agent", "skills", "local"),
			want:     skillStatusSourceWorkspace,
		},
		{
			name:     "docs skill",
			skillDir: filepath.Join(docsSkillsDir, "tools", "channels-ops"),
			want:     skillStatusSourceWorkspace,
		},
		{
			name:       "synced skill",
			sourceHint: "synced",
			skillDir:   filepath.Join(paths.SyncedDir, "browser"),
			want:       skillStatusSourceManaged,
		},
		{
			name:       "bundled skill",
			sourceHint: "bundled",
			skillDir:   filepath.Join(paths.BundledDir, "exec"),
			want:       skillStatusSourceBundled,
		},
		{
			name:     "extra skill",
			skillDir: filepath.Join(paths.ExtraDirs[0], "vendor-tool"),
			want:     skillStatusSourceExtra,
		},
		{
			name:       "argus docs-backed skill",
			sourceHint: "argus",
			skillDir:   filepath.Join(docsSkillsDir, "tools", "argus-screen-reading"),
			want:       skillStatusSourceWorkspace,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeSkillStatusSource(tc.sourceHint, tc.skillDir, paths)
			if got != tc.want {
				t.Fatalf("source = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAppendArgusSkillStatusEntries_NormalizesSource(t *testing.T) {
	t.Parallel()

	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	docsSkillsDir := filepath.Join(workspaceDir, "docs", "skills")
	paths := skillStatusPaths{
		WorkspaceDir:  workspaceDir,
		DocsSkillsDir: docsSkillsDir,
		SyncedDir:     filepath.Join(docsSkillsDir, "synced"),
	}

	entries := appendArgusSkillStatusEntries(nil, []argus.ArgusSkillEntry{
		{
			Name:        "argus.read_text",
			Description: "Argus read text",
			Source:      "argus",
			FilePath:    filepath.Join(docsSkillsDir, "tools", "argus-screen-reading", "SKILL.md"),
			BaseDir:     filepath.Join(docsSkillsDir, "tools", "argus-screen-reading"),
			SkillKey:    "argus.read_text",
			Eligible:    true,
			Requirements: map[string][]string{
				"bins": {}, "env": {}, "config": {}, "os": {},
			},
			Missing: map[string][]string{
				"bins": {}, "env": {}, "config": {}, "os": {},
			},
		},
	}, paths)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Source != skillStatusSourceWorkspace {
		t.Fatalf("source = %q, want %q", entries[0].Source, skillStatusSourceWorkspace)
	}
}

func TestHandleSkillsStatus_UsesNormalizedSources(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	writeSkill := func(dir, name, description string) {
		t.Helper()
		skillDir := filepath.Join(dir, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: " + description + "\n---\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSkill(filepath.Join(workspaceDir, ".agent", "skills"), "local-skill", "local")
	writeSkill(filepath.Join(workspaceDir, "docs", "skills", "tools"), "docs-skill", "docs")
	writeSkill(filepath.Join(workspaceDir, "docs", "skills", "synced"), "managed-skill", "managed")

	configPath := filepath.Join(t.TempDir(), "config.json")
	configJSON := `{"agents":{"defaults":{"workspace":"` + workspaceDir + `"}}}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := config.NewConfigLoader(config.WithConfigPath(configPath))

	registry := NewMethodRegistry()
	registry.RegisterAll(SkillsHandlers())

	var gotOK bool
	var gotPayload interface{}
	HandleGatewayRequest(registry, &RequestFrame{Method: "skills.status", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, payload interface{}, err *ErrorShape) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gotOK = ok
		gotPayload = payload
	})

	if !gotOK {
		t.Fatal("skills.status should succeed")
	}

	report, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("payload type = %T", gotPayload)
	}
	if got, _ := report["managedSkillsDir"].(string); got != filepath.Join(workspaceDir, "docs", "skills", "synced") {
		t.Fatalf("managedSkillsDir = %q", got)
	}

	rawSkills, ok := report["skills"].([]skillStatusEntry)
	if !ok {
		t.Fatalf("skills type = %T", report["skills"])
	}

	sources := make(map[string]string, len(rawSkills))
	for _, skill := range rawSkills {
		sources[skill.Name] = skill.Source
	}
	if sources["local-skill"] != skillStatusSourceWorkspace {
		t.Fatalf("local-skill source = %q", sources["local-skill"])
	}
	if sources["docs-skill"] != skillStatusSourceWorkspace {
		t.Fatalf("docs-skill source = %q", sources["docs-skill"])
	}
	if sources["managed-skill"] != skillStatusSourceManaged {
		t.Fatalf("managed-skill source = %q", sources["managed-skill"])
	}

	rawBindings, ok := report["toolBindings"].([]skillToolBindingEntry)
	if !ok {
		t.Fatalf("toolBindings type = %T", report["toolBindings"])
	}
	if rawBindings == nil {
		t.Fatal("toolBindings should be present")
	}
}

func TestSkillsStatusReportsRuntimeToolBindings(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	writeSkill := func(root, name, content string) {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSkill(filepath.Join(workspaceDir, ".agent", "skills"), "active-send", `
---
description: active send
tools: send_media
---
# active-send
`)
	writeSkill(filepath.Join(workspaceDir, ".agent", "skills"), "manual-bash", `
---
description: manual bash
tools: bash
disable-model-invocation: true
---
# manual-bash
`)
	writeSkill(filepath.Join(workspaceDir, ".agent", "skills"), "disabled-bash", `
---
description: disabled bash
tools: bash
---
# disabled-bash
`)

	configPath := filepath.Join(t.TempDir(), "config.json")
	configJSON := `{"agents":{"defaults":{"workspace":"` + workspaceDir + `"}},"skills":{"entries":{"disabled-bash":{"enabled":false}}}}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := config.NewConfigLoader(config.WithConfigPath(configPath))

	registry := NewMethodRegistry()
	registry.RegisterAll(SkillsHandlers())

	var gotPayload interface{}
	HandleGatewayRequest(registry, &RequestFrame{Method: "skills.status", Params: map[string]interface{}{}}, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, payload interface{}, err *ErrorShape) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("skills.status should succeed")
		}
		gotPayload = payload
	})

	report, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("payload type = %T", gotPayload)
	}
	rawBindings, ok := report["toolBindings"].([]skillToolBindingEntry)
	if !ok {
		t.Fatalf("toolBindings type = %T", report["toolBindings"])
	}
	var sendMediaBinding *skillToolBindingEntry
	for i := range rawBindings {
		if rawBindings[i].ToolName == "send_media" {
			sendMediaBinding = &rawBindings[i]
			break
		}
	}
	if sendMediaBinding == nil {
		t.Fatal("send_media binding not found")
	}
	if !containsSkillName(sendMediaBinding.Skills, "active-send") {
		t.Fatalf("send_media binding skills = %v, want to contain active-send", sendMediaBinding.Skills)
	}
	for _, binding := range rawBindings {
		if containsSkillName(binding.Skills, "manual-bash") {
			t.Fatalf("manual-only skill leaked into runtime bindings: %+v", binding)
		}
		if containsSkillName(binding.Skills, "disabled-bash") {
			t.Fatalf("disabled skill leaked into runtime bindings: %+v", binding)
		}
	}
}

func TestSkillsStatus_AppliesAgentSkillFilterToBindings(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	writeSkill := func(root, name, content string) {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSkill(filepath.Join(workspaceDir, ".agent", "skills"), "browser-ops", `
---
description: browser guidance
tools: browser
---
# browser-ops
`)
	writeSkill(filepath.Join(workspaceDir, ".agent", "skills"), "bash-ops", `
---
description: bash guidance
tools: bash
---
# bash-ops
`)

	configPath := filepath.Join(t.TempDir(), "config.json")
	configJSON := `{
  "agents": {
    "defaults": {"workspace":"` + workspaceDir + `"},
    "list": [
      {"id":"browser-agent","workspace":"` + workspaceDir + `","skills":["browser-ops"]}
    ]
  }
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := config.NewConfigLoader(config.WithConfigPath(configPath))

	registry := NewMethodRegistry()
	registry.RegisterAll(SkillsHandlers())

	var gotPayload interface{}
	HandleGatewayRequest(registry, &RequestFrame{
		Method: "skills.status",
		Params: map[string]interface{}{"agentId": "browser-agent"},
	}, nil, &GatewayMethodContext{
		ConfigLoader: loader,
	}, func(ok bool, payload interface{}, err *ErrorShape) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("skills.status should succeed")
		}
		gotPayload = payload
	})

	report, ok := gotPayload.(map[string]interface{})
	if !ok {
		t.Fatalf("payload type = %T", gotPayload)
	}
	rawBindings, ok := report["toolBindings"].([]skillToolBindingEntry)
	if !ok {
		t.Fatalf("toolBindings type = %T", report["toolBindings"])
	}

	var browserBinding *skillToolBindingEntry
	for i := range rawBindings {
		switch rawBindings[i].ToolName {
		case "browser":
			browserBinding = &rawBindings[i]
		case "bash":
			t.Fatalf("bash binding should be filtered out, got %+v", rawBindings[i])
		}
	}
	if browserBinding == nil {
		t.Fatal("browser binding not found")
	}
	if !containsSkillName(browserBinding.Skills, "browser-ops") {
		t.Fatalf("browser binding skills = %v, want to contain browser-ops", browserBinding.Skills)
	}
}

func containsSkillName(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestShouldExposeSkillStatusEntry_HidesBundledManualOnlySkills(t *testing.T) {
	t.Parallel()

	bundledDir := filepath.Join(t.TempDir(), "bundled")

	tests := []struct {
		name   string
		entry  agentSkills.SkillEntry
		wantOK bool
	}{
		{
			name: "bundled manual-only skill hidden",
			entry: agentSkills.SkillEntry{
				Skill:                  agentSkills.Skill{Dir: filepath.Join(bundledDir, "claude", "code-audit")},
				DisableModelInvocation: true,
			},
			wantOK: false,
		},
		{
			name: "workspace manual-only skill kept",
			entry: agentSkills.SkillEntry{
				Skill:                  agentSkills.Skill{Dir: filepath.Join(t.TempDir(), "workspace", "skill-creator")},
				DisableModelInvocation: true,
			},
			wantOK: true,
		},
		{
			name: "bundled normal skill kept",
			entry: agentSkills.SkillEntry{
				Skill:                  agentSkills.Skill{Dir: filepath.Join(bundledDir, "tools", "runtime", "bash")},
				DisableModelInvocation: false,
			},
			wantOK: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldExposeSkillStatusEntry(tc.entry, bundledDir); got != tc.wantOK {
				t.Fatalf("shouldExposeSkillStatusEntry() = %v, want %v", got, tc.wantOK)
			}
		})
	}
}
