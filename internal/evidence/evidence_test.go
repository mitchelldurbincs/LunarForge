package evidence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRunID(t *testing.T) {
	tm := time.Date(2026, 6, 30, 14, 22, 10, 0, time.UTC)
	if got := NewRunID(tm); got != "2026-06-30T14-22-10" {
		t.Errorf("NewRunID = %q", got)
	}
}

func TestWriteLoadLatest(t *testing.T) {
	root := t.TempDir()
	evidenceDir := filepath.Join(root, ".lf", "runs")

	tm := time.Date(2026, 6, 30, 14, 22, 10, 0, time.UTC)
	runID := NewRunID(tm)
	runDir := RunDir(evidenceDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ev := &Evidence{
		Version:  SchemaVersion,
		Project:  "demo",
		RunID:    runID,
		Result:   ResultPassed,
		DiffHash: "sha256:abc",
		Commands: []Command{{ID: "test", Run: "go test", Result: ResultPassed}},
	}
	if err := Write(evidenceDir, runDir, ev); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// latest pointer should exist next to runs/.
	if _, err := os.Stat(filepath.Join(root, ".lf", "latest")); err != nil {
		t.Fatalf("latest pointer missing: %v", err)
	}

	loaded, dir, err := LoadLatest(evidenceDir)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if dir != runDir {
		t.Errorf("run dir = %q, want %q", dir, runDir)
	}
	if loaded.DiffHash != "sha256:abc" || !loaded.Passed() {
		t.Errorf("loaded evidence mismatch: %+v", loaded)
	}
}

func TestArtifactExcludes(t *testing.T) {
	repo := filepath.FromSlash("/repo")
	if got := ArtifactExcludes(repo, filepath.Join(repo, ".lf", "runs")); len(got) != 1 || got[0] != ".lf" {
		t.Errorf("default layout: got %v, want [.lf]", got)
	}
	// Evidence dir directly under repo -> exclude it (parent is repo root).
	if got := ArtifactExcludes(repo, filepath.Join(repo, "evidence")); len(got) != 1 || got[0] != "evidence" {
		t.Errorf("flat layout: got %v, want [evidence]", got)
	}
	// Evidence dir == repo root -> no safe exclude.
	if got := ArtifactExcludes(repo, repo); got != nil {
		t.Errorf("repo-root evidence dir: got %v, want nil", got)
	}
}

func TestLatestRunIDFallbackScan(t *testing.T) {
	evidenceDir := t.TempDir()
	for _, id := range []string{"2026-06-30T10-00-00", "2026-06-30T12-00-00"} {
		if err := os.MkdirAll(RunDir(evidenceDir, id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// No latest pointer -> should pick the lexically-newest dir.
	got, err := LatestRunID(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-06-30T12-00-00" {
		t.Errorf("LatestRunID = %q", got)
	}
}
