package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repairRepo builds a temp git repo wired for repair: a verify command that
// checks src/hello.txt, fake success/noop repair agents, and a config. hello is
// the initial contents of src/hello.txt ("broken" to start failing).
func repairRepo(t *testing.T, hello string) string {
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
repair:
  enabled: true
  max_attempts: 3
  agent: fake_success
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

func mustWrite(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

// inDir runs fn with the process working directory set to dir, capturing
// stdout, and restoring both afterwards.
func inDir(t *testing.T, dir string, fn func() error) (string, error) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = origStdout
	data, _ := io.ReadAll(r)
	return string(data), runErr
}

// seedFailedVerify runs verify once so failed evidence exists.
func seedVerify(t *testing.T, dir string) {
	t.Helper()
	if _, err := inDir(t, dir, func() error { return cmdVerify(nil) }); err != nil {
		if _, ok := err.(*exitError); !ok {
			t.Fatalf("seed verify: %v", err)
		}
	}
}

func TestRepairRefusesWhenNoEvidence(t *testing.T) {
	dir := repairRepo(t, "broken")
	out, err := inDir(t, dir, func() error { return cmdRepair(nil) })
	if _, ok := err.(*exitError); !ok {
		t.Fatalf("expected exitError, got %v", err)
	}
	if !strings.Contains(out, "Nothing to repair") {
		t.Errorf("expected refusal output, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lf", "runs")); err == nil {
		t.Error("repair should not have created evidence when there is none")
	}
}

func TestRepairRefusesWhenPassed(t *testing.T) {
	dir := repairRepo(t, "hello lunarforge")
	seedVerify(t, dir) // passes
	out, err := inDir(t, dir, func() error { return cmdRepair(nil) })
	if err != nil {
		t.Fatalf("expected nil error on passed evidence, got %v", err)
	}
	if !strings.Contains(out, "Nothing to repair") || !strings.Contains(out, "passed") {
		t.Errorf("expected nothing-to-repair output, got:\n%s", out)
	}
	if _, err := os.Stat(repairDirFor(dir)); err == nil {
		t.Error("no repair artifacts should be written when evidence already passed")
	}
}

func TestRepairDisabledRefuses(t *testing.T) {
	dir := repairRepo(t, "broken")
	// Disable repair.
	cfgPath := filepath.Join(dir, ".lunarforge.yml")
	data, _ := os.ReadFile(cfgPath)
	mustWrite(t, cfgPath, strings.Replace(string(data), "enabled: true", "enabled: false", 1), 0o644)
	seedVerify(t, dir)
	out, err := inDir(t, dir, func() error { return cmdRepair(nil) })
	if _, ok := err.(*exitError); !ok {
		t.Fatalf("expected exitError when disabled, got %v", err)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected disabled message, got:\n%s", out)
	}
}

func TestRepairDryRunDoesNotInvokeAgent(t *testing.T) {
	dir := repairRepo(t, "broken")
	seedVerify(t, dir)
	out, err := inDir(t, dir, func() error { return cmdRepair([]string{"--dry-run"}) })
	if err != nil {
		t.Fatalf("dry-run error: %v", err)
	}
	if !strings.Contains(out, "Dry run") || !strings.Contains(out, "fake-repair-success.sh") {
		t.Errorf("expected dry-run plan with agent command, got:\n%s", out)
	}
	// File must be untouched and no attempt dir written.
	if got := readFile(t, filepath.Join(dir, "src", "hello.txt")); !strings.HasPrefix(got, "broken") {
		t.Errorf("dry-run modified the file: %q", got)
	}
	if _, err := os.Stat(filepath.Join(repairDirFor(dir), "attempt-1")); err == nil {
		t.Error("dry-run should not write attempt artifacts")
	}
}

func TestRepairPrintPromptExits(t *testing.T) {
	dir := repairRepo(t, "broken")
	seedVerify(t, dir)
	out, err := inDir(t, dir, func() error { return cmdRepair([]string{"--print-prompt"}) })
	if err != nil {
		t.Fatalf("print-prompt error: %v", err)
	}
	if !strings.Contains(out, "You are repairing a failed LunarForge verification run") {
		t.Errorf("expected prompt printed, got:\n%s", out)
	}
	if strings.HasPrefix(readFile(t, filepath.Join(dir, "src", "hello.txt")), "broken") == false {
		t.Error("print-prompt should not modify files")
	}
}

func TestRepairSucceedsAndReverifies(t *testing.T) {
	dir := repairRepo(t, "broken")
	seedVerify(t, dir)
	out, err := inDir(t, dir, func() error { return cmdRepair(nil) })
	if err != nil {
		t.Fatalf("repair should succeed, got %v\n%s", err, out)
	}
	if !strings.Contains(out, "repaired and locally verified") {
		t.Errorf("expected success result, got:\n%s", out)
	}
	// The agent fixed the file.
	if got := readFile(t, filepath.Join(dir, "src", "hello.txt")); !strings.Contains(got, "hello lunarforge") {
		t.Errorf("file not repaired: %q", got)
	}
	// Artifacts exist for attempt 1.
	for _, name := range []string{"prompt.md", "agent.stdout.txt", "agent.stderr.txt", "result.json"} {
		p := filepath.Join(repairDirFor(dir), "attempt-1", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing artifact %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repairDirFor(dir), "summary.md")); err != nil {
		t.Errorf("missing repair summary: %v", err)
	}
}

func TestRepairExhaustsAttempts(t *testing.T) {
	dir := repairRepo(t, "broken")
	seedVerify(t, dir)
	out, err := inDir(t, dir, func() error {
		return cmdRepair([]string{"--agent", "fake_noop", "--attempts", "2"})
	})
	if _, ok := err.(*exitError); !ok {
		t.Fatalf("expected exitError on exhaustion, got %v", err)
	}
	if !strings.Contains(out, "max attempts reached") {
		t.Errorf("expected exhaustion message, got:\n%s", out)
	}
	// Two attempts recorded.
	for _, n := range []string{"attempt-1", "attempt-2"} {
		if _, err := os.Stat(filepath.Join(repairDirFor(dir), n, "prompt.md")); err != nil {
			t.Errorf("missing %s prompt: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repairDirFor(dir), "attempt-3")); err == nil {
		t.Error("should not have a third attempt with --attempts 2")
	}
}

// repairDirFor returns the repair dir under the single run directory in dir.
func repairDirFor(dir string) string {
	runs := filepath.Join(dir, ".lf", "runs")
	entries, err := os.ReadDir(runs)
	if err != nil || len(entries) == 0 {
		return filepath.Join(runs, "none", "repair")
	}
	// The original (oldest) run holds the repair dir.
	return filepath.Join(runs, entries[0].Name(), "repair")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
