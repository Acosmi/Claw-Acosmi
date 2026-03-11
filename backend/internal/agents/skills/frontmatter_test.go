package skills

import "testing"

func TestResolveOpenAcosmiMetadata_SupportsCapabilityTreeMetadata(t *testing.T) {
	fm := ParsedSkillFrontmatter{
		"metadata": `{"tree_id":"ui/browser","tree_group":"ui","min_tier":"task_light","approval_type":"none"}`,
	}

	meta := ResolveOpenAcosmiMetadata(fm)
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta.TreeID != "ui/browser" {
		t.Fatalf("TreeID = %q, want ui/browser", meta.TreeID)
	}
	if meta.TreeGroup != "ui" {
		t.Fatalf("TreeGroup = %q, want ui", meta.TreeGroup)
	}
	if meta.MinTier != "task_light" {
		t.Fatalf("MinTier = %q, want task_light", meta.MinTier)
	}
	if meta.Approval != "none" {
		t.Fatalf("Approval = %q, want none", meta.Approval)
	}
	if len(meta.Tools) != 1 || meta.Tools[0] != "browser" {
		t.Fatalf("Tools = %v, want [browser]", meta.Tools)
	}
}
