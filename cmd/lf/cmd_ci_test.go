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
