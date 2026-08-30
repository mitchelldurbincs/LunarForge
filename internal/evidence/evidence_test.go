package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validEvidence(runID string) *Evidence {
	return &Evidence{
		Version: SchemaVersion, RunID: runID,
		Result: ResultPassed, Reason: ReasonChecksPassed,
		DiffHash: "sha256:abc", FinalDiffHash: "sha256:abc",
		Commands: []Command{{
			ID: "test", Result: ResultPassed, Reason: ReasonChecksPassed,
			StdoutPath: "checks/test.stdout.txt", StderrPath: "checks/test.stderr.txt",
		}},
	}
}

func writeLogs(t *testing.T, runDir string) {
	t.Helper()
	dir := filepath.Join(runDir, "checks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"test.stdout.txt", "test.stderr.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewRunIDIsUniqueAtSameInstant(t *testing.T) {
	now := time.Date(2026, 6, 30, 14, 22, 10, 0, time.UTC)
	a, b := NewRunID(now), NewRunID(now)
	if a == b {
		t.Fatalf("run IDs collided: %q", a)
	}
	if !validRunID(a) || !validRunID(b) {
		t.Fatalf("invalid run IDs: %q %q", a, b)
	}
}

func TestWriteAndLoadLatest(t *testing.T) {
	evidenceDir := filepath.Join(t.TempDir(), ".lf", "runs")
	runID := NewRunID(time.Now())
	runDir := RunDir(evidenceDir, runID)
	writeLogs(t, runDir)
	if err := Write(evidenceDir, runDir, validEvidence(runID)); err != nil {
		t.Fatal(err)
	}
	loaded, gotDir, err := LoadLatest(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != runDir || loaded.DiffHash != "sha256:abc" || !loaded.Passed() {
		t.Fatalf("loaded mismatch: dir=%q evidence=%+v", gotDir, loaded)
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(evidenceDir), "latest")); err != nil || string(data) != runID+"\n" {
		t.Fatalf("latest pointer = %q, %v", data, err)
	}
}

func TestLoadLatestDistinguishesMissingAndCorrupt(t *testing.T) {
	evidenceDir := filepath.Join(t.TempDir(), ".lf", "runs")
	if _, _, err := LoadLatest(evidenceDir); !errors.Is(err, ErrNoEvidence) {
		t.Fatalf("missing error = %v", err)
	}
	runID := NewRunID(time.Now())
	if err := os.MkdirAll(RunDir(evidenceDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(evidenceDir), "latest"), []byte(runID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(RunDir(evidenceDir, runID), "evidence.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadLatest(evidenceDir); err == nil || errors.Is(err, ErrNoEvidence) {
		t.Fatalf("corrupt evidence error = %v", err)
	}
}

func TestArtifactExcludes(t *testing.T) {
	repo := filepath.FromSlash("/repo")
	if got := ArtifactExcludes(repo, filepath.Join(repo, ".lf", "runs")); len(got) != 1 || got[0] != ".lf" {
		t.Fatalf("got %v, want [.lf]", got)
	}
}

func TestLoadRejectsUnsafeEvidencePaths(t *testing.T) {
	evidenceDir := filepath.Join(t.TempDir(), ".lf", "runs")
	runID := NewRunID(time.Now())
	ev := validEvidence(runID)
	ev.Commands[0].StdoutPath = "../../secret"
	if err := Write(evidenceDir, RunDir(evidenceDir, runID), ev); err == nil {
		t.Fatal("expected unsafe log path to be rejected")
	}
	if validRunID("..") {
		t.Fatal("parent directory must not be a valid run ID")
	}
}

func TestLoadRejectsMissingLogs(t *testing.T) {
	evidenceDir := filepath.Join(t.TempDir(), ".lf", "runs")
	runID := NewRunID(time.Now())
	runDir := RunDir(evidenceDir, runID)
	writeLogs(t, runDir)
	if err := Write(evidenceDir, runDir, validEvidence(runID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, "checks", "test.stdout.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(runDir); err == nil {
		t.Fatal("expected missing log to make evidence corrupt")
	}
}

func TestWriteRejectsInconsistentStateAndChecks(t *testing.T) {
	evidenceDir := filepath.Join(t.TempDir(), ".lf", "runs")
	runID := NewRunID(time.Now())
	ev := validEvidence(runID)
	ev.Commands[0].Result = ResultFailed
	ev.Commands[0].Reason = ReasonCheckFailed
	if err := Write(evidenceDir, RunDir(evidenceDir, runID), ev); err == nil {
		t.Fatal("passing evidence must not contain a failed check")
	}
}
