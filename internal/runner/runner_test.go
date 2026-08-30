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
	if _, err := exec.LookPath("git"); err != nil || runtime.GOOS == "windows" {
		t.Skip("test needs git and a POSIX shell")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "t"}, {"commit", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func run(t *testing.T, dir string, commands ...config.Command) *Result {
	t.Helper()
	res, err := Run(&config.Config{Version: 1, Verify: config.Verify{Commands: commands}}, Options{
		RepoDir: dir, EvidenceDir: filepath.Join(dir, evidence.DefaultDir), Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRunPassesAndWritesEvidence(t *testing.T) {
	dir := gitRepo(t)
	res := run(t, dir, config.Command{ID: "echo", Run: "echo hello"}, config.Command{ID: "ok", Run: "true"})
	if !res.Evidence.Passed() || len(res.Evidence.Commands) != 2 {
		t.Fatalf("unexpected evidence: %+v", res.Evidence)
	}
	out, err := os.ReadFile(filepath.Join(res.RunDir, "checks", "echo.stdout.txt"))
	if err != nil || string(out) != "hello\n" {
		t.Fatalf("stdout = %q, err=%v", out, err)
	}
	for _, name := range []string{"evidence.json", "summary.md"} {
		if _, err := os.Stat(filepath.Join(res.RunDir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestRunRecordsSkippedChecks(t *testing.T) {
	dir := gitRepo(t)
	res := run(t, dir,
		config.Command{ID: "bad", Run: "exit 7"},
		config.Command{ID: "later", Run: "echo should-not-run"},
	)
	if res.Evidence.Result != evidence.ResultFailed || len(res.Evidence.Commands) != 2 {
		t.Fatalf("unexpected result: %+v", res.Evidence)
	}
	if got := res.Evidence.Commands[1]; got.Result != evidence.ResultSkipped || got.Reason != evidence.ReasonPreviousCheckFailed {
		t.Fatalf("unexpected skipped record: %+v", got)
	}
}

func TestRunTimeoutIsBlocked(t *testing.T) {
	dir := gitRepo(t)
	res := run(t, dir, config.Command{ID: "slow", Run: "sleep 2", TimeoutSeconds: 1})
	if res.Evidence.Result != evidence.ResultBlocked || res.Evidence.Reason != evidence.ReasonTimedOut {
		t.Fatalf("unexpected timeout: %+v", res.Evidence)
	}
}

func TestRunMissingToolIsBlocked(t *testing.T) {
	dir := gitRepo(t)
	res := run(t, dir, config.Command{ID: "missing", Run: "lunarforge-command-that-does-not-exist"})
	if res.Evidence.Result != evidence.ResultBlocked || res.Evidence.Reason != evidence.ReasonToolUnavailable {
		t.Fatalf("unexpected missing-tool result: %+v", res.Evidence)
	}
}

func TestRunDetectsRepositoryMutation(t *testing.T) {
	dir := gitRepo(t)
	res := run(t, dir, config.Command{ID: "mutate", Run: "printf changed > generated.txt"})
	if res.Evidence.Result != evidence.ResultStale || res.Evidence.Reason != evidence.ReasonSnapshotChanged {
		t.Fatalf("mutation should stale evidence: %+v", res.Evidence)
	}
	if res.Evidence.DiffHash == res.Evidence.FinalDiffHash {
		t.Fatal("pre/post fingerprints should differ")
	}
}
