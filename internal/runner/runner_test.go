package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell commands in test assume a POSIX shell")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestRunPassesAndWritesEvidence(t *testing.T) {
	dir := gitRepo(t)
	cfg := &config.Config{
		Version: 1,
		Project: config.Project{Name: "demo"},
		Verify: config.Verify{Commands: []config.Command{
			{ID: "echo", Run: "echo hello"},
			{ID: "true", Run: "true"},
		}},
	}
	res, err := Run(cfg, Options{
		RepoDir:     dir,
		EvidenceDir: filepath.Join(dir, ".lf", "runs"),
		Now:         time.Now(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Evidence.Passed() {
		t.Fatalf("expected passed, got %s", res.Evidence.Result)
	}
	if len(res.Evidence.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(res.Evidence.Commands))
	}
	// stdout file should contain the echo output.
	out, err := os.ReadFile(filepath.Join(res.RunDir, "commands", "echo.stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hello\n" {
		t.Errorf("stdout = %q", out)
	}
	// summary + evidence + latest pointer exist.
	for _, p := range []string{
		filepath.Join(res.RunDir, "evidence.json"),
		filepath.Join(res.RunDir, "summary.md"),
		filepath.Join(dir, ".lf", "latest"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
}

func TestRunStopsOnFirstFailure(t *testing.T) {
	dir := gitRepo(t)
	cfg := &config.Config{
		Version: 1,
		Project: config.Project{Name: "demo"},
		Verify: config.Verify{Commands: []config.Command{
			{ID: "ok", Run: "true"},
			{ID: "bad", Run: "exit 3"},
			{ID: "never", Run: "echo should-not-run"},
		}},
	}
	res, err := Run(cfg, Options{
		RepoDir:     dir,
		EvidenceDir: filepath.Join(dir, ".lf", "runs"),
		Now:         time.Now(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Evidence.Passed() {
		t.Fatal("expected failure")
	}
	if len(res.Evidence.Commands) != 2 {
		t.Fatalf("expected stop after 2 commands, got %d", len(res.Evidence.Commands))
	}
	last := res.Evidence.Commands[1]
	if last.ID != "bad" || last.Result != evidence.ResultFailed || last.ExitCode != 3 {
		t.Errorf("unexpected failing command record: %+v", last)
	}
	// The third command's output file must not exist.
	if _, err := os.Stat(filepath.Join(res.RunDir, "commands", "never.stdout.txt")); !os.IsNotExist(err) {
		t.Error("third command should not have run")
	}
}
