package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Acosmi/ClawAcosmi/pkg/types"
)

func TestBuildWorkspaceSkillSnapshot_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	snap := BuildWorkspaceSkillSnapshot(BuildSnapshotParams{
		WorkspaceDir: tmpDir,
	})
	if len(snap.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(snap.Skills))
	}
	if snap.Prompt != "" {
		t.Errorf("expected empty prompt, got %q", snap.Prompt)
	}
}

func TestBuildWorkspaceSkillSnapshot_WithSkills(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, ".agent", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: A test skill\n---\n# Test Skill\nInstructions here."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	snap := BuildWorkspaceSkillSnapshot(BuildSnapshotParams{
		WorkspaceDir: tmpDir,
	})

	if len(snap.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(snap.Skills))
	}
	if snap.Skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", snap.Skills[0].Name)
	}
	if snap.Prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildWorkspaceSkillSnapshot_WithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		skillDir := filepath.Join(tmpDir, ".agent", "skills", name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: "+name+"\n---\nContent"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	snap := BuildWorkspaceSkillSnapshot(BuildSnapshotParams{
		WorkspaceDir: tmpDir,
		SkillFilter:  []string{"alpha", "gamma"},
	})
	if len(snap.Skills) != 2 {
		t.Fatalf("expected 2 skills after filter, got %d", len(snap.Skills))
	}
}

func TestExtractDescription(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no frontmatter", "# Hello", ""},
		{"with desc", "---\ndescription: My Skill\n---\nBody", "My Skill"},
		{"no desc field", "---\nname: test\n---\nBody", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDescription(tt.content)
			if got != tt.want {
				t.Errorf("extractDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveToolSkillBindings(t *testing.T) {
	entries := []SkillEntry{
		{
			Skill:    Skill{Name: "exec", Description: "Exec tool usage, stdin modes, and TTY support"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
		},
		{
			Skill:    Skill{Name: "web", Description: "Web 搜索和抓取工具使用指南"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"web_search", "browser"}},
		},
		{
			Skill: Skill{Name: "no-tools", Description: "A skill without tool binding"},
			// no Metadata
		},
		{
			Skill:    Skill{Name: "empty-desc", Description: ""},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"read_file"}},
		},
		{
			// SkillBindable: false tools should be rejected
			Skill:    Skill{Name: "skills", Description: "技能系统内部"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"search_skills", "lookup_skill"}},
		},
	}

	bindings := ResolveToolSkillBindings(entries)

	// bash → exec description (registered + bindable)
	if got, ok := bindings["bash"]; !ok || got != "Exec tool usage, stdin modes, and TTY support" {
		t.Errorf("bash binding = %q, ok = %v", got, ok)
	}
	// web_search → web description (registered + bindable)
	if _, ok := bindings["web_search"]; !ok {
		t.Error("web_search binding missing")
	}
	// browser → web description (registered + bindable)
	if _, ok := bindings["browser"]; !ok {
		t.Error("browser binding missing")
	}
	// search_skills should NOT appear (SkillBindable: false)
	if _, ok := bindings["search_skills"]; ok {
		t.Error("search_skills should be rejected (SkillBindable: false)")
	}
	// no-tools and empty-desc should not appear
	if len(bindings) != 3 {
		t.Errorf("expected 3 bindings, got %d: %v", len(bindings), bindings)
	}
}

func TestResolveToolSkillBindings_TruncatesLongDescription(t *testing.T) {
	longDesc := "This is a very long description that exceeds one hundred and twenty characters in total length so it should be truncated by the binding logic."
	entries := []SkillEntry{
		{
			Skill:    Skill{Name: "long", Description: longDesc},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"write_file"}},
		},
	}
	bindings := ResolveToolSkillBindings(entries)
	got := bindings["write_file"]
	if len(got) > 120 {
		t.Errorf("description not truncated: len=%d", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("expected trailing '...', got %q", got[len(got)-5:])
	}
}

func TestResolveToolSkillBindings_FirstWins(t *testing.T) {
	entries := []SkillEntry{
		{
			Skill:    Skill{Name: "first", Description: "First skill"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
		},
		{
			Skill:    Skill{Name: "second", Description: "Second skill"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
		},
	}
	bindings := ResolveToolSkillBindings(entries)
	if bindings["bash"] != "First skill" {
		t.Errorf("expected first-wins, got %q", bindings["bash"])
	}
}

func TestResolveToolSkillBindingSet_TracksAllSkillNames(t *testing.T) {
	entries := []SkillEntry{
		{
			Skill:    Skill{Name: "first", Description: "First skill"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
		},
		{
			Skill:    Skill{Name: "second", Description: "Second skill"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
		},
	}

	bindingSet := ResolveToolSkillBindingSet(entries)
	binding, ok := bindingSet["bash"]
	if !ok {
		t.Fatal("missing bash binding")
	}
	if binding.PrimarySkill != "first" {
		t.Fatalf("PrimarySkill = %q, want first", binding.PrimarySkill)
	}
	if binding.Guidance != "First skill" {
		t.Fatalf("Guidance = %q, want First skill", binding.Guidance)
	}
	if !reflect.DeepEqual(binding.SkillNames, []string{"first", "second"}) {
		t.Fatalf("SkillNames = %v, want [first second]", binding.SkillNames)
	}

	nameMap := ResolveToolSkillNames(entries)
	if !reflect.DeepEqual(nameMap["bash"], []string{"first", "second"}) {
		t.Fatalf("ResolveToolSkillNames(bash) = %v, want [first second]", nameMap["bash"])
	}
}

func TestResolvePromptToolSkillBindingSet_FiltersManualOnlyAndDisabled(t *testing.T) {
	disabled := false
	cfg := &types.OpenAcosmiConfig{
		Skills: &types.SkillsConfig{
			Entries: map[string]*types.SkillConfig{
				"disabled": {Enabled: &disabled},
			},
		},
	}
	entries := []SkillEntry{
		{
			Skill:    Skill{Name: "active", Description: "Active skill"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
			Enabled:  true,
		},
		{
			Skill:                  Skill{Name: "manual-only", Description: "Manual only"},
			Metadata:               &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
			Enabled:                true,
			DisableModelInvocation: true,
		},
		{
			Skill:    Skill{Name: "disabled", Description: "Disabled skill"},
			Metadata: &OpenAcosmiSkillMetadata{Tools: []string{"bash"}},
			Enabled:  true,
		},
	}

	bindingSet := ResolvePromptToolSkillBindingSet(entries, cfg, nil)
	binding, ok := bindingSet["bash"]
	if !ok {
		t.Fatal("missing bash binding")
	}
	if !reflect.DeepEqual(binding.SkillNames, []string{"active"}) {
		t.Fatalf("SkillNames = %v, want [active]", binding.SkillNames)
	}
}

func TestLoadSkillsFromDir_ParsesToolsFromFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "my-exec")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: my-exec\ndescription: \"Run commands\"\ntools: bash, write_file\n---\n# My Exec\nInstructions."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := loadSkillsFromDir(tmpDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Metadata == nil {
		t.Fatal("expected Metadata to be set")
	}
	if len(e.Metadata.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %v", len(e.Metadata.Tools), e.Metadata.Tools)
	}
	if e.Metadata.Tools[0] != "bash" || e.Metadata.Tools[1] != "write_file" {
		t.Errorf("unexpected tools: %v", e.Metadata.Tools)
	}
}

func TestBuildWorkspaceSkillSnapshot_PreloadedEntries(t *testing.T) {
	v := 42
	snap := BuildWorkspaceSkillSnapshot(BuildSnapshotParams{
		WorkspaceDir:    "/nonexistent",
		SnapshotVersion: &v,
		Entries: []SkillEntry{
			{
				Skill:   Skill{Name: "preloaded", Description: "desc"},
				Enabled: true,
			},
		},
	})
	if len(snap.Skills) != 1 || snap.Skills[0].Name != "preloaded" {
		t.Errorf("unexpected skills: %+v", snap.Skills)
	}
	if snap.Version == nil || *snap.Version != 42 {
		t.Error("version not set")
	}
}

func TestLoadSkillsFromRoots_CategorizedSkillsDir(t *testing.T) {
	tmpDir := t.TempDir()
	screenSkillDir := filepath.Join(tmpDir, "tools", "argus-screen-reading")
	if err := os.MkdirAll(screenSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(screenSkillDir, "SKILL.md"), []byte("---\ndescription: read screen\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	providerSkillDir := filepath.Join(tmpDir, "providers", "openai")
	if err := os.MkdirAll(providerSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerSkillDir, "SKILL.md"), []byte("---\ndescription: provider\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := loadSkillsFromRoots(tmpDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from categorized skills dir, got %d", len(entries))
	}
}

func TestLoadSkillsFromRoots_DeepCategorizedSkillsDir(t *testing.T) {
	tmpDir := t.TempDir()
	canvasSkillDir := filepath.Join(tmpDir, "tools", "ui", "canvas")
	if err := os.MkdirAll(canvasSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canvasSkillDir, "SKILL.md"), []byte("---\ndescription: canvas\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	webFetchSkillDir := filepath.Join(tmpDir, "tools", "web", "web-fetch")
	if err := os.MkdirAll(webFetchSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webFetchSkillDir, "SKILL.md"), []byte("---\ndescription: fetch\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries := loadSkillsFromRoots(tmpDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from deep categorized skills dir, got %d", len(entries))
	}
}

func TestResolveDocsSkillsDir_EnvOverrideSupportsCategorizedRoot(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "tools", "argus-screen-reading")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: read screen\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := os.Getenv("OPENACOSMI_DOCS_SKILLS_DIR")
	if err := os.Setenv("OPENACOSMI_DOCS_SKILLS_DIR", tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if prev == "" {
			_ = os.Unsetenv("OPENACOSMI_DOCS_SKILLS_DIR")
			return
		}
		_ = os.Setenv("OPENACOSMI_DOCS_SKILLS_DIR", prev)
	}()

	got := ResolveDocsSkillsDir("")
	if got != tmpDir {
		t.Fatalf("expected env override docs skills dir %q, got %q", tmpDir, got)
	}
}
