package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// loopRepo builds a temp git repo wired for `lf loop`: a verify command that
// checks src/hello.txt, fake repair (success + noop) agents, a fake explain
// command, and a config defaulting to the success agent. hello is the initial
// contents of src/hello.txt ("hello lunarforge" passes, anything else fails).
func loopRepo(t *testing.T, hello, repairAgent string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if runtime.GOOS == "windows" {
		t.Skip("fixture scripts use a POSIX shell")
	}
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, ".lunarforge.yml"), `version: 1
project:
  name: t
verify:
  commands:
    - id: contents
      run: ./scripts/verify.sh
explain:
  command: ./scripts/fake-explain.sh
repair:
  enabled: true
  max_attempts: 3
  agent: `+repairAgent+`
agents:
  fake_success:
    backend: fake
    command: ./scripts/fake-repair-success.sh
  fake_noop:
    backend: fake
    command: ./scripts/fake-repair-noop.sh
evidence:
  dir: .lf/runs
`, 0o644)

	mustWrite(t, filepath.Join(dir, "scripts", "verify.sh"),
		"#!/bin/sh\ngrep -q 'hello lunarforge' src/hello.txt || { echo bad >&2; exit 1; }\n", 0o755)
	mustWrite(t, filepath.Join(dir, "scripts", "fake-repair-success.sh"),
		"#!/bin/sh\ncat >/dev/null\necho 'hello lunarforge' > src/hello.txt\n", 0o755)
	mustWrite(t, filepath.Join(dir, "scripts", "fake-repair-noop.sh"),
		"#!/bin/sh\ncat >/dev/null\necho noop\n", 0o755)
	mustWrite(t, filepath.Join(dir, "scripts", "fake-explain.sh"),
		"#!/bin/sh\ncat >/dev/null 2>&1 || true\necho '# explanation (fake)'\n", 0o755)
	mustWrite(t, filepath.Join(dir, "src", "hello.txt"), hello+"\n", 0o644)
	mustWrite(t, filepath.Join(dir, ".gitignore"), ".lf/\n", 0o644)

	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "t"},
		{"add", "-A"},
		{"commit", "-m", "fixture"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// runLoop runs cmdLoop in dir and returns its stdout and error.
func runLoop(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	return inDir(t, dir, func() error { return cmdLoop(args) })
}

// latestRunDir returns the newest .lf/runs/<id> directory in dir.
func latestRunDir(t *testing.T, dir string) string {
	t.Helper()
	runs := filepath.Join(dir, ".lf", "runs")
	entries, err := os.ReadDir(runs)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no run dirs under %s: %v", runs, err)
	}
	return filepath.Join(runs, entries[len(entries)-1].Name())
}

// loopDir returns the single .lf/loops/<id> directory in dir.
func loopDir(t *testing.T, dir string) string {
	t.Helper()
	loops := filepath.Join(dir, ".lf", "loops")
	entries, err := os.ReadDir(loops)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no loop dirs under %s: %v", loops, err)
	}
	return filepath.Join(loops, entries[len(entries)-1].Name())
}

// requireStatusReady asserts `lf status --require-fresh-passing` exit code.
func requireStatusReady(t *testing.T, dir string, wantReady bool) {
	t.Helper()
	_, err := inDir(t, dir, func() error { return cmdStatus([]string{"--require-fresh-passing"}) })
	if wantReady && err != nil {
		t.Fatalf("expected status ready (exit 0), got %v", err)
	}
	if !wantReady {
		if _, ok := err.(*exitError); !ok {
			t.Fatalf("expected status not-ready (exitError), got %v", err)
		}
	}
}

func TestLoopPassesImmediately(t *testing.T) {
	dir := loopRepo(t, "hello lunarforge", "fake_success")
	out, err := runLoop(t, dir)
	if err != nil {
		t.Fatalf("loop should succeed, got %v\n%s", err, out)
	}
	// Repair skipped, explain ran, result ready.
	if !strings.Contains(out, "Step 1/3: verify") || !strings.Contains(out, "✅ verify passed") {
		t.Errorf("expected verify pass, got:\n%s", out)
	}
	if !strings.Contains(out, "skipped — verification already passed") {
		t.Errorf("expected repair skipped, got:\n%s", out)
	}
	if !strings.Contains(out, "✅ explanation saved") {
		t.Errorf("expected explanation saved, got:\n%s", out)
	}
	if !strings.Contains(out, "ready for review") || strings.Contains(out, "repaired and ready") {
		t.Errorf("expected plain ready result, got:\n%s", out)
	}
	requireStatusReady(t, dir, true)
}

func TestLoopRunsRepairThenExplainAfterSuccess(t *testing.T) {
	dir := loopRepo(t, "broken", "fake_success")
	out, err := runLoop(t, dir)
	if err != nil {
		t.Fatalf("loop should succeed after repair, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "❌ verify failed") {
		t.Errorf("expected initial verify failure, got:\n%s", out)
	}
	if !strings.Contains(out, "Step 2/3: repair") || !strings.Contains(out, "Attempt 1/3") {
		t.Errorf("expected repair attempt, got:\n%s", out)
	}
	if !strings.Contains(out, "✅ explanation saved") {
		t.Errorf("expected explanation after repair, got:\n%s", out)
	}
	if !strings.Contains(out, "repaired and ready for review") {
		t.Errorf("expected repaired result, got:\n%s", out)
	}
	// The agent actually fixed the file.
	if got := readFile(t, filepath.Join(dir, "src", "hello.txt")); !strings.Contains(got, "hello lunarforge") {
		t.Errorf("file not repaired: %q", got)
	}
	requireStatusReady(t, dir, true)
}

func TestLoopBlockedWhenRepairNoops(t *testing.T) {
	dir := loopRepo(t, "broken", "fake_noop")
	out, err := runLoop(t, dir, "--repair-attempts", "2")
	if _, ok := err.(*exitError); !ok {
		t.Fatalf("expected exitError on blocked loop, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "Attempt 1/2") || !strings.Contains(out, "Attempt 2/2") {
		t.Errorf("expected two repair attempts, got:\n%s", out)
	}
	if !strings.Contains(out, "skipped — verification is still failing") {
		t.Errorf("expected explain skipped, got:\n%s", out)
	}
	if strings.Contains(out, "explanation saved") {
		t.Errorf("explain should not run when blocked, got:\n%s", out)
	}
	if !strings.Contains(out, "❌ blocked") {
		t.Errorf("expected blocked result, got:\n%s", out)
	}
	requireStatusReady(t, dir, false)
}

func TestLoopNoRepairStopsAfterFailedVerify(t *testing.T) {
	dir := loopRepo(t, "broken", "fake_success")
	out, err := runLoop(t, dir, "--no-repair")
	if _, ok := err.(*exitError); !ok {
		t.Fatalf("expected exitError with --no-repair on failure, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "skipped — --no-repair") {
		t.Errorf("expected repair skipped by flag, got:\n%s", out)
	}
	if strings.Contains(out, "Attempt 1") {
		t.Errorf("repair must not run with --no-repair, got:\n%s", out)
	}
	if !strings.Contains(out, "❌ blocked") {
		t.Errorf("expected blocked result, got:\n%s", out)
	}
	// The agent never touched the file.
	if got := readFile(t, filepath.Join(dir, "src", "hello.txt")); !strings.HasPrefix(got, "broken") {
		t.Errorf("file should be untouched, got %q", got)
	}
	requireStatusReady(t, dir, false)
}

func TestLoopNoExplainSkipsExplainAfterSuccess(t *testing.T) {
	dir := loopRepo(t, "hello lunarforge", "fake_success")
	out, err := runLoop(t, dir, "--no-explain")
	if err != nil {
		t.Fatalf("loop should succeed, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "skipped — --no-explain") {
		t.Errorf("expected explain skipped by flag, got:\n%s", out)
	}
	if strings.Contains(out, "explanation saved") {
		t.Errorf("explain should not run with --no-explain, got:\n%s", out)
	}
	if !strings.Contains(out, "ready for review") {
		t.Errorf("expected ready result, got:\n%s", out)
	}
	// No explanation.md should exist in the latest run dir.
	if _, err := os.Stat(filepath.Join(latestRunDir(t, dir), "explanation.md")); err == nil {
		t.Error("explanation.md should not be written with --no-explain")
	}
	requireStatusReady(t, dir, true)
}

func TestLoopRepairAttemptsOverride(t *testing.T) {
	dir := loopRepo(t, "broken", "fake_noop")
	// Config default is 3; override to 1 and confirm only one attempt runs.
	out, err := runLoop(t, dir, "--repair-attempts", "1")
	if _, ok := err.(*exitError); !ok {
		t.Fatalf("expected exitError on blocked loop, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "Attempt 1/1") {
		t.Errorf("expected a single 1/1 attempt, got:\n%s", out)
	}
	if strings.Contains(out, "Attempt 2/") {
		t.Errorf("override to 1 must not run a second attempt, got:\n%s", out)
	}
}

func TestLoopWritesSummaryArtifact(t *testing.T) {
	dir := loopRepo(t, "hello lunarforge", "fake_success")
	if _, err := runLoop(t, dir); err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	ld := loopDir(t, dir)
	for _, name := range []string{"summary.md", "loop.json"} {
		if _, err := os.Stat(filepath.Join(ld, name)); err != nil {
			t.Errorf("missing loop artifact %s: %v", name, err)
		}
	}
	summary := readFile(t, filepath.Join(ld, "summary.md"))
	if !strings.Contains(summary, "ready for review") {
		t.Errorf("summary missing final result, got:\n%s", summary)
	}
	loopJSON := readFile(t, filepath.Join(ld, "loop.json"))
	for _, key := range []string{"started_at", "finished_at", "verify_result", "final_result", "final_passing"} {
		if !strings.Contains(loopJSON, key) {
			t.Errorf("loop.json missing %q, got:\n%s", key, loopJSON)
		}
	}
}

func TestLoopDryRunRunsNothing(t *testing.T) {
	dir := loopRepo(t, "broken", "fake_success")
	out, err := runLoop(t, dir, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run should not error, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "Dry run") {
		t.Errorf("expected dry-run notice, got:\n%s", out)
	}
	// Nothing executed: no evidence, no loop summary, file untouched.
	if _, err := os.Stat(filepath.Join(dir, ".lf", "runs")); err == nil {
		t.Error("dry-run must not create evidence")
	}
	if _, err := os.Stat(filepath.Join(dir, ".lf", "loops")); err == nil {
		t.Error("dry-run must not write a loop summary")
	}
	if got := readFile(t, filepath.Join(dir, "src", "hello.txt")); !strings.HasPrefix(got, "broken") {
		t.Errorf("dry-run modified the file: %q", got)
	}
}
