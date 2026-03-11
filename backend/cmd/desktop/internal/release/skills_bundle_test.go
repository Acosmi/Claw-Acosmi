package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkillsPackageManifest_DesktopManifestMatchesAuditBaseline(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "build", "skills-package-manifest.txt")
	entries, err := LoadSkillsPackageManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadSkillsPackageManifest() error = %v", err)
	}
	if len(entries) != 40 {
		t.Fatalf("expected 40 manifest entries, got %d", len(entries))
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry, "_archived/") {
			t.Fatalf("manifest must not include archived skill: %s", entry)
		}
		if strings.HasPrefix(entry, "media/") {
			t.Fatalf("manifest must not include non-audited top-level media skill: %s", entry)
		}
	}
}

func TestStageSkillsPackageEntries_CopiesOnlyWhitelistedSkills(t *testing.T) {
	t.Parallel()

	srcRoot := filepath.Join(t.TempDir(), "src")
	dstRoot := filepath.Join(t.TempDir(), "dst")

	writeSkillDir := func(rel string) {
		t.Helper()
		dir := filepath.Join(srcRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "references", "note.txt"), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeSkillDir("tools/runtime/bash")
	writeSkillDir("operations/system-config")
	writeSkillDir("_archived/legacy")
	writeSkillDir("media/media-trending")

	if err := StageSkillsPackageEntries(srcRoot, dstRoot, []string{
		"tools/runtime/bash",
		"operations/system-config",
	}); err != nil {
		t.Fatalf("StageSkillsPackageEntries() error = %v", err)
	}

	mustExist := []string{
		filepath.Join(dstRoot, "tools", "runtime", "bash", "SKILL.md"),
		filepath.Join(dstRoot, "tools", "runtime", "bash", "references", "note.txt"),
		filepath.Join(dstRoot, "operations", "system-config", "SKILL.md"),
	}
	for _, path := range mustExist {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected copied path %s: %v", path, err)
		}
	}

	mustNotExist := []string{
		filepath.Join(dstRoot, "_archived", "legacy", "SKILL.md"),
		filepath.Join(dstRoot, "media", "media-trending", "SKILL.md"),
	}
	for _, path := range mustNotExist {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unexpected copied path %s", path)
		}
	}
}

func TestLoadSkillsPackageManifest_RejectsTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "skills.txt")
	if err := os.WriteFile(manifestPath, []byte("../escape\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillsPackageManifest(manifestPath); err == nil {
		t.Fatal("expected traversal manifest to fail")
	}
}
