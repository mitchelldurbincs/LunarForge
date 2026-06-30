package main

import (
	"os"
	"path/filepath"
	"testing"
)

// latestEvidenceDir returns the single run directory under .lf/runs, failing if
// there is not exactly one.
func latestEvidenceDir(t *testing.T, repoDir string) string {
	t.Helper()
	runsDir := filepath.Join(repoDir, ".lf", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		t.Fatalf("reading runs dir: %v", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 1 {
		t.Fatalf("expected exactly one run dir, got %v", dirs)
	}
	return filepath.Join(runsDir, dirs[0])
}

func TestCIPassesAndWritesEvidence(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	if err := cmdCI(nil); err != nil {
		t.Fatalf("lf ci returned error on passing config: %v", err)
	}

	runDir := latestEvidenceDir(t, repo)
	if _, err := os.Stat(filepath.Join(runDir, "evidence.json")); err != nil {
		t.Errorf("evidence.json missing: %v", err)
	}
}

func TestCIFailsAndStillWritesEvidence(t *testing.T) {
	repo := newTestRepo(t, failingConfig)

	err := cmdCI(nil)
	if err == nil {
		t.Fatal("expected non-zero exit on failing config")
	}
	ee, ok := err.(*exitError)
	if !ok || ee.code == 0 {
		t.Fatalf("expected non-zero exitError, got %v", err)
	}

	// Failure evidence must still be saved.
	runDir := latestEvidenceDir(t, repo)
	if _, err := os.Stat(filepath.Join(runDir, "evidence.json")); err != nil {
		t.Errorf("failure evidence.json missing: %v", err)
	}
}

// TestCIIgnoresStaleLocalEvidence confirms `lf ci` runs regardless of any
// pre-existing local evidence freshness — the checkout is the source of truth.
func TestCIIgnoresStaleLocalEvidence(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	// First run produces evidence.
	if err := cmdCI(nil); err != nil {
		t.Fatalf("first lf ci: %v", err)
	}
	// Mutate a tracked file so any freshness check would consider evidence stale.
	if err := os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// lf ci should still succeed (it does not gate on freshness).
	if err := cmdCI(nil); err != nil {
		t.Fatalf("lf ci should not gate on stale local evidence: %v", err)
	}
}
