package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func checkConfig(command string) *config.Config {
	cfg := commitConfig("true")
	cfg.Status.Contracts = []config.Contract{{ID: "format", Check: command, Reason: "exit 0 = clean; exit 1 = changes needed"}}
	return cfg
}

func TestRunContractResults(t *testing.T) {
	for _, tc := range []struct {
		name, command, status, reason string
		exit                          int
		ready                         bool
	}{
		{"clean", "echo clean", "passed", "check passed", 0, true},
		{"changes", "echo '7 files need changes' >&2; exit 1", "failed", "7 files need changes", 1, false},
		{"error", "echo 'tool error' >&2; exit 3", "error", "tool error", 3, false},
		{"missing", "./missing-check-executable", "error", "missing-check-executable", 127, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			repo := gitRepo(t)
			cfg := checkConfig(tc.command)
			// Act.
			res, err := Run(cfg, Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs")})
			// Assert.
			if err != nil {
				t.Fatal(err)
			}
			ev, err := evidence.Load(res.RunDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(ev.Contracts) != 1 {
				t.Fatalf("contracts = %+v", ev.Contracts)
			}
			rec := ev.Contracts[0]
			if rec.Status != tc.status || rec.ExitCode != tc.exit || rec.Command != tc.command || !strings.Contains(rec.Reason, tc.reason) {
				t.Fatalf("result = %+v", rec)
			}
			r := evidence.RequireChecks(evidence.Evaluate(ev, ev.DiffHash), cfg, ev)
			if r.Ready() != tc.ready || !r.Fresh {
				t.Fatalf("readiness = %+v", r)
			}
			if !tc.ready && !strings.Contains(r.Reason(), "format") {
				t.Fatalf("missing contract ID: %s", r.Reason())
			}
			for _, name := range []string{"evidence.json", "stdout.txt", "stderr.txt"} {
				if _, err := os.Stat(filepath.Join(res.RunDir, "contracts", "format", name)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRunContractSpawnFailure(t *testing.T) {
	// Arrange: a valid shell cannot start in a nonexistent working directory.
	repo := gitRepo(t)
	opts := Options{RepoDir: filepath.Join(repo, "missing")}
	// Act.
	rec := runContract(opts, t.TempDir(), config.Contract{ID: "spawn", Check: "true"})
	// Assert.
	if rec.Status != evidence.ResultError || rec.ExitCode != -1 || !strings.Contains(rec.Reason, "could not execute") {
		t.Fatalf("spawn result = %+v", rec)
	}
}

func TestRunContractNotExecutable(t *testing.T) {
	// Arrange.
	repo := gitRepo(t)
	commitFile(t, repo, "not-executable", "#!/bin/sh\nexit 0\n")
	// Act.
	res, err := Run(checkConfig("./not-executable"), Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs")})
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Evidence.Contracts[0]
	if rec.Status != evidence.ResultError || rec.ExitCode != 126 || !strings.Contains(strings.ToLower(rec.Reason), "permission denied") {
		t.Fatalf("result = %+v", rec)
	}
}

func TestRunContractTailsAreBounded(t *testing.T) {
	// Arrange.
	repo := gitRepo(t)
	cfg := checkConfig("head -c 10000 /dev/zero | tr '\\0' x; printf STDOUT-END; head -c 10000 /dev/zero | tr '\\0' y >&2; printf STDERR-END >&2; exit 1")
	// Act.
	res, err := Run(cfg, Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs")})
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Evidence.Contracts[0]
	if len(rec.StdoutTail) != contractTailBytes || len(rec.StderrTail) != contractTailBytes || !strings.HasSuffix(rec.StdoutTail, "STDOUT-END") || !strings.HasSuffix(rec.StderrTail, "STDERR-END") || !strings.Contains(rec.Reason, "STDOUT-END") || !strings.Contains(rec.Reason, "STDERR-END") {
		t.Fatalf("tails lost or unbounded: %+v", rec)
	}
	for _, name := range []string{"stdout.txt", "stderr.txt"} {
		stat, err := os.Stat(filepath.Join(res.RunDir, "contracts", "format", name))
		if err != nil {
			t.Fatal(err)
		}
		if stat.Size() != contractTailBytes {
			t.Fatalf("%s size = %d", name, stat.Size())
		}
	}
}

func TestRunContractsAfterProfileFailure(t *testing.T) {
	// Arrange.
	repo := gitRepo(t)
	cfg := checkConfig("echo checked; exit 1")
	cfg.Verify.Commands = []config.Command{{ID: "stop", Run: "exit 1"}, {ID: "skipped", Run: "true"}}
	cfg.Status.Contracts = append(cfg.Status.Contracts, config.Contract{ID: "second", Check: "true"})
	// Act.
	res, err := Run(cfg, Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs")})
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence.Commands) != 1 || len(res.Evidence.Contracts) != 2 || res.Evidence.Contracts[1].Status != "passed" {
		t.Fatalf("stopped before contracts: %+v", res.Evidence)
	}
}

func TestRunContractEnvironmentMatchesProfile(t *testing.T) {
	// Arrange: a dotnet fixture on PATH proves both launch paths preserve it.
	repo := gitRepo(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "dotnet"), []byte("#!/bin/sh\nprintf '%s\\n' \"$LF_ENV_SENTINEL\" \"$LUNARFORGE_RUN_DIR\" \"$PWD\"\ntest -z \"$GIT_DIR\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_DIR", filepath.Join(bin, "poison"))
	t.Setenv("LF_ENV_SENTINEL", "preserved")
	cfg := checkConfig("dotnet")
	cfg.Verify.Commands[0].Run = "dotnet"
	// Act.
	res, err := Run(cfg, Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"})
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := os.ReadFile(filepath.Join(res.RunDir, res.Evidence.Commands[0].StdoutPath))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Evidence.Passed() || string(stdout) != res.Evidence.Contracts[0].StdoutTail || !strings.Contains(string(stdout), "preserved\n"+res.RunDir+"\n"+res.Evidence.ExecutionDir) {
		t.Fatalf("environment mismatch: profile %q contract %+v", stdout, res.Evidence.Contracts[0])
	}
}

func TestRunContractDiffReuseAndChange(t *testing.T) {
	// Arrange: count outside the repo so counting cannot invalidate the diff.
	repo := gitRepo(t)
	count := filepath.Join(t.TempDir(), "count")
	cfg := checkConfig(fmt.Sprintf("echo run >> '%s'", count))
	opts := Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs")}
	first, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Act: unchanged input should reuse, a changed diff must execute again.
	second, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "changed"), []byte("new diff"), 0644); err != nil {
		t.Fatal(err)
	}
	third, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	runs, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if string(runs) != "run\nrun\n" || second.Evidence.Contracts[0].ReusedFrom != first.Evidence.RunID || third.Evidence.Contracts[0].ReusedFrom != "" {
		t.Fatalf("reuse wrong: count=%q second=%+v third=%+v", runs, second.Evidence.Contracts, third.Evidence.Contracts)
	}
	if evidence.Evaluate(first.Evidence, third.Evidence.DiffHash).Fresh {
		t.Fatal("old diff remains fresh")
	}
}

func TestRunContractCommitReuseAndRewrite(t *testing.T) {
	// Arrange: tree reuse must not allow a check to cross commit identities.
	repo := gitRepo(t)
	count := filepath.Join(t.TempDir(), "count")
	cfg := checkConfig(fmt.Sprintf("echo run >> '%s'", count))
	cfg.Verify.TreeReuse = true
	opts := Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"}
	first, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	cmd := exec.Command("git", "commit", "--amend", "--allow-empty", "-m", "rewritten identity")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("amend: %v %s", err, out)
	}
	want, err := evidence.CaptureIdentity(cfg, repo)
	if err != nil {
		t.Fatal(err)
	}
	stale := evidence.EvaluateCommit(second.Evidence, *want, cfg.Verify.TreeReuse)
	third, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	runs, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Fresh || stale.Ready() || !third.Evidence.SubjectVerified || first.Evidence.Identity.Subject.Commit == third.Evidence.Identity.Subject.Commit || string(runs) != "run\nrun\n" || second.Evidence.Contracts[0].ReusedFrom != first.Evidence.RunID || third.Evidence.Contracts[0].ReusedFrom != "" {
		t.Fatalf("rewrite reused check: stale=%+v runs=%q", stale, runs)
	}
}

func TestRunReusesFailedCheckWithoutLosingReason(t *testing.T) {
	// Arrange.
	repo := gitRepo(t)
	count := filepath.Join(t.TempDir(), "count")
	cfg := checkConfig(fmt.Sprintf("echo run >> '%s'; echo changes-needed >&2; exit 1", count))
	opts := Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"}
	first, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	second, err := Run(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	runs, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	rec := second.Evidence.Contracts[0]
	if string(runs) != "run\n" || rec.Status != evidence.ResultFailed || rec.ReusedFrom != first.Evidence.RunID || rec.Reason != first.Evidence.Contracts[0].Reason || second.Evidence.Passed() {
		t.Fatalf("failed reuse lost result: count=%q result=%+v", runs, rec)
	}
}
