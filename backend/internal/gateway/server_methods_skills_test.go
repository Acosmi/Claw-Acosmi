package gateway

import (
	"os"
	"path/filepath"
	"testing"

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
}
