package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestRunExcludesExistingArtifactsFromGitSnapshot(t *testing.T) {
	dir := gitRepo(t)
	first := run(t, dir, config.Command{ID: "ok", Run: "true"})

	second := run(t, dir, config.Command{ID: "ok", Run: "true"})

	if second.Evidence.Git.Dirty || second.Evidence.Git.StatusPorcelain != "" {
		t.Fatalf("prior artifacts must not dirty the saved snapshot: %+v", second.Evidence.Git)
	}
	if second.Evidence.DiffHash != first.Evidence.DiffHash {
		t.Fatal("prior artifacts must not change the fingerprint")
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

func TestRunRejectsUnsafeCheckIDBeforeSideEffects(t *testing.T) {
	dir := gitRepo(t)
	// Four parent traversals escape checks/<run>/runs/.lf into the repo root.
	const unsafeID = "../../../../escaped"
	cfg := &config.Config{Version: 1, Verify: config.Verify{Commands: []config.Command{
		{ID: "first", Run: "echo ran > executed.txt"},
		{ID: unsafeID, Run: "echo unsafe"},
	}}}

	res, err := Run(cfg, Options{RepoDir: dir, EvidenceDir: filepath.Join(dir, evidence.DefaultDir)})

	if err == nil || !strings.Contains(err.Error(), unsafeID) || !strings.Contains(err.Error(), "must match ^[A-Za-z0-9][A-Za-z0-9._-]*$") || res != nil {
		t.Fatalf("expected unsafe ID rejection, got result=%+v, err=%v", res, err)
	}
	for _, name := range []string{".lf", "executed.txt", "escaped.stdout.txt", "escaped.stderr.txt"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("invalid check ID must not create %s: %v", name, err)
		}
	}
}

func TestRunDoesNotWriteSummaryWhenEvidenceValidationFails(t *testing.T) {
	dir := gitRepo(t)
	evidenceDir := filepath.Join(dir, evidence.DefaultDir)

	_, err := Run(&config.Config{Version: 1}, Options{RepoDir: dir, EvidenceDir: evidenceDir})

	if err == nil || !strings.Contains(err.Error(), "at least one check") {
		t.Fatalf("expected evidence validation failure, got %v", err)
	}
	assertNoSummary(t, evidenceDir)
}

func TestRunDoesNotWriteSummaryWhenEvidenceWriteFails(t *testing.T) {
	dir := gitRepo(t)
	evidenceDir := filepath.Join(dir, evidence.DefaultDir)
	// A nonempty directory at the latest pointer forces evidence.Write to fail even
	// when the evidence record itself is valid and writable.
	if err := os.MkdirAll(filepath.Join(dir, ".lf", "latest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lf", "latest", "blocker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Version: 1, Verify: config.Verify{Commands: []config.Command{{ID: "ok", Run: "true"}}}}

	_, err := Run(cfg, Options{RepoDir: dir, EvidenceDir: evidenceDir})

	if err == nil || !strings.Contains(err.Error(), "writing latest pointer") {
		t.Fatalf("expected evidence write failure, got %v", err)
	}
	assertNoSummary(t, evidenceDir)
}

func assertNoSummary(t *testing.T, evidenceDir string) {
	t.Helper()
	entries, err := os.ReadDir(evidenceDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one attempted run, got %v, err=%v", entries, err)
	}
	path := filepath.Join(evidenceDir, entries[0].Name(), "summary.md")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("summary must not exist after evidence failure: %v", err)
	}
}

func TestRunKeepGoingPreservesBlockedPrecedence(t *testing.T) {
	failed := config.Command{ID: "failed", Run: "exit 7"}
	blocked := config.Command{ID: "blocked", Run: "lunarforge-command-that-does-not-exist"}
	passed := config.Command{ID: "passed", Run: "true"}
	for _, tc := range []struct {
		name     string
		commands []config.Command
	}{
		{"failure-first", []config.Command{failed, blocked, passed}},
		{"blocked-first", []config.Command{blocked, failed, passed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := gitRepo(t)
			cfg := &config.Config{Version: 1, Verify: config.Verify{Commands: tc.commands}}

			res, err := Run(cfg, Options{
				RepoDir: dir, EvidenceDir: filepath.Join(dir, evidence.DefaultDir), KeepGoing: true,
			})

			if err != nil {
				t.Fatal(err)
			}
			if res.Evidence.Result != evidence.ResultBlocked || res.Evidence.Reason != evidence.ReasonToolUnavailable {
				t.Fatalf("blocked must take precedence over failure: %+v", res.Evidence)
			}
			if len(res.Evidence.Commands) != 3 || res.Evidence.Commands[2].Result != evidence.ResultPassed {
				t.Fatalf("keep-going must execute all checks: %+v", res.Evidence.Commands)
			}
			if _, err := evidence.Load(res.RunDir); err != nil {
				t.Fatalf("saved mixed-state evidence must remain readable: %v", err)
			}
		})
	}
}

func TestRunWritesMatchingCheckLogPaths(t *testing.T) {
	dir := gitRepo(t)
	res := run(t, dir, config.Command{ID: "Build_1.test-2", Run: "echo output; echo error >&2"})
	check := res.Evidence.Commands[0]
	if filepath.ToSlash(check.StdoutPath) != "checks/Build_1.test-2.stdout.txt" ||
		filepath.ToSlash(check.StderrPath) != "checks/Build_1.test-2.stderr.txt" {
		t.Fatalf("unexpected log paths: %+v", check)
	}
	for path, want := range map[string]string{check.StdoutPath: "output\n", check.StderrPath: "error\n"} {
		got, err := os.ReadFile(filepath.Join(res.RunDir, path))
		if err != nil || string(got) != want {
			t.Fatalf("log %s = %q, %v; want %q", path, got, err, want)
		}
	}
	if _, err := evidence.Load(res.RunDir); err != nil {
		t.Fatalf("saved log paths must pass evidence validation: %v", err)
	}
}
