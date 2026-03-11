package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------- ResolveSessionTranscriptCandidates ----------

func TestResolveSessionTranscriptCandidates_WithSessionFile(t *testing.T) {
	candidates := ResolveSessionTranscriptCandidates("sess1", "", "/custom/path.jsonl", "")
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	if candidates[0] != "/custom/path.jsonl" {
		t.Errorf("first candidate should be sessionFile, got %q", candidates[0])
	}
}

func TestResolveSessionTranscriptCandidates_WithStorePath(t *testing.T) {
	candidates := ResolveSessionTranscriptCandidates("sess1", "/data/store.json", "", "")
	found := false
	for _, c := range candidates {
		if filepath.Base(c) == "sess1.jsonl" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sess1.jsonl candidate, got %v", candidates)
	}
}

func TestResolveSessionTranscriptCandidates_UsesResolvedStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CRABCLAW_STATE_DIR", stateDir)
	t.Setenv("OPENACOSMI_STATE_DIR", "")

	candidates := ResolveSessionTranscriptCandidates("sess1", "", "", "agent-a")
	wantAgent := filepath.Join(stateDir, "agents", "agent-a", "sessions", "sess1.jsonl")
	wantGlobal := filepath.Join(stateDir, "sessions", "sess1.jsonl")
	foundAgent := false
	foundGlobal := false
	for _, candidate := range candidates {
		if candidate == wantAgent {
			foundAgent = true
		}
		if candidate == wantGlobal {
			foundGlobal = true
		}
	}
	if !foundAgent || !foundGlobal {
		t.Fatalf("candidates = %v, want %q and %q", candidates, wantAgent, wantGlobal)
	}
}

// ---------- ReadFirstUserMessageFromTranscript ----------

func TestReadFirstUserMessageFromTranscript(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"message":{"role":"system","content":"You are helpful."}}`,
		`{"message":{"role":"user","content":"Hello world"}}`,
		`{"message":{"role":"assistant","content":"Hi!"}}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := ReadFirstUserMessageFromTranscript("test", "", fp, "")
	if got != "Hello world" {
		t.Errorf("got %q, want %q", got, "Hello world")
	}
}

func TestReadFirstUserMessageFromTranscript_Empty(t *testing.T) {
	got := ReadFirstUserMessageFromTranscript("nonexistent", "", "", "")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ---------- ReadLastMessagePreviewFromTranscript ----------

func TestReadLastMessagePreviewFromTranscript(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"message":{"role":"user","content":"First"}}`,
		`{"message":{"role":"assistant","content":"Response one"}}`,
		`{"message":{"role":"user","content":"Second question"}}`,
		`{"message":{"role":"assistant","content":"Final answer"}}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := ReadLastMessagePreviewFromTranscript("test", "", fp, "")
	if got != "Final answer" {
		t.Errorf("got %q, want %q", got, "Final answer")
	}
}

// ---------- ArchiveFileOnDisk ----------

func TestArchiveFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(fp, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	archived, err := ArchiveFileOnDisk(fp, "reset")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("archived file should exist: %v", err)
	}
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("original file should be gone")
	}
}

// ---------- CapArrayByJsonBytes ----------

func TestCapArrayByJsonBytes_UnderLimit(t *testing.T) {
	items := []json.RawMessage{[]byte(`"a"`), []byte(`"b"`)}
	result, _ := CapArrayByJsonBytes(items, 1000)
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}
}

func TestCapArrayByJsonBytes_OverLimit(t *testing.T) {
	items := []json.RawMessage{
		[]byte(`"aaaaaaaaaa"`),
		[]byte(`"bbbbbbbbbb"`),
		[]byte(`"cc"`),
	}
	result, _ := CapArrayByJsonBytes(items, 20)
	if len(result) >= 3 {
		t.Errorf("expected fewer items, got %d", len(result))
	}
}

// ---------- ReadSessionPreviewItemsFromTranscript ----------

func TestReadSessionPreviewItemsFromTranscript(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"message":{"role":"user","content":"Question?"}}`,
		`{"message":{"role":"assistant","content":"Answer!"}}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	items := ReadSessionPreviewItemsFromTranscript("test", "", fp, "", 10, 500)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Role != "user" || items[0].Text != "Question?" {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[1].Role != "assistant" || items[1].Text != "Answer!" {
		t.Errorf("item[1] = %+v", items[1])
	}
}
