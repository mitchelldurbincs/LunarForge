package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func commitConfig(command string) *config.Config {
	return &config.Config{Version: 1, Verify: config.Verify{Commands: []config.Command{{ID: "gate", Run: command}}}}
}

func commitFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", name}, {"commit", "-m", "fixture"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
}

func TestCommitRunIsolatesIgnoredInputs(t *testing.T) {
	repo := gitRepo(t)
	commitFile(t, repo, ".gitignore", "ignored.txt\n")
	if err := os.WriteFile(filepath.Join(repo, "ignored.txt"), []byte("poison"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(commitConfig("test ! -e ignored.txt"), Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	ev := res.Evidence
	if !ev.Passed() || !ev.SubjectVerified || ev.Identity.Subject.Dirty || len(ev.Identity.Subject.Commit) != 40 || len(ev.Identity.Subject.Tree) != 40 {
		t.Fatalf("bad commit evidence: %+v", ev)
	}
	if ev.Git.Head != ev.Identity.Subject.Commit {
		t.Fatal("git head must be full SHA")
	}
	if _, err := os.Stat(ev.ExecutionDir); !os.IsNotExist(err) {
		t.Fatal("disposable checkout was not removed")
	}
	if _, err := os.Stat(filepath.Join(repo, "ignored.txt")); err != nil {
		t.Fatal("source ignored input changed")
	}
}

func TestCommitRunRefusesDirtySource(t *testing.T) {
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "untracked"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(commitConfig("true"), Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"})
	if err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("got %v", err)
	}
}

func TestCommitRunFailsWhenCommandChangesTrackedSubject(t *testing.T) {
	repo := gitRepo(t)
	commitFile(t, repo, "source", "before")
	res, err := Run(commitConfig("echo after > source"), Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Evidence.Passed() || res.Evidence.SubjectVerified || res.Evidence.SubjectError == "" {
		t.Fatal("changed execution subject accepted")
	}
	if res.Evidence.ExecutionEndIdentity == nil || !res.Evidence.ExecutionEndIdentity.Subject.Dirty {
		t.Fatal("execution end subject not recorded")
	}
	data, err := os.ReadFile(filepath.Join(repo, "source"))
	if err != nil || string(data) != "before" {
		t.Fatal("source repository changed")
	}
}

func TestCommitRunFailsWhenSourceChangesDuringExecution(t *testing.T) {
	repo := gitRepo(t)
	commitFile(t, repo, "source", "before")
	res, err := Run(commitConfig(fmt.Sprintf("echo after > '%s'", filepath.Join(repo, "source"))), Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Evidence.Passed() || res.Evidence.SubjectVerified {
		t.Fatal("source changed during execution was accepted")
	}
}

func TestRunsAtSameTimeKeepSeparateLogs(t *testing.T) {
	repo := gitRepo(t)
	dir := filepath.Join(t.TempDir(), "runs")
	now := time.Now()
	opts := Options{RepoDir: repo, EvidenceDir: dir, Now: now}
	first, err := Run(commitConfig("echo first"), opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Run(commitConfig("echo second"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunDir == second.RunDir {
		t.Fatal("run ID collision")
	}
	data, err := os.ReadFile(filepath.Join(first.RunDir, "commands", "gate.stdout.txt"))
	if err != nil || string(data) != "first\n" {
		t.Fatal("first log overwritten")
	}
}

func TestConcurrentRunsPublishCompleteIndependentRecords(t *testing.T) {
	repo := gitRepo(t)
	dir := filepath.Join(t.TempDir(), "runs")
	now := time.Now()
	type outcome struct {
		result *Result
		err    error
	}
	completed := make(chan outcome, 8)
	for i := 0; i < 8; i++ {
		go func() {
			result, err := Run(commitConfig("echo complete"), Options{RepoDir: repo, EvidenceDir: dir, Now: now})
			completed <- outcome{result, err}
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		got := <-completed
		if got.err != nil {
			t.Fatal(got.err)
		}
		if seen[got.result.RunDir] {
			t.Fatal("concurrent collision")
		}
		seen[got.result.RunDir] = true
		saved, err := evidence.Load(got.result.RunDir)
		if err != nil || !saved.Passed() {
			t.Fatalf("partial published record: %v", err)
		}
	}
	latest, _, err := evidence.LoadLatest(dir)
	if err != nil || !latest.Passed() {
		t.Fatalf("partial latest pointer: %v", err)
	}
}

func TestCommitRunIgnoresGitEnvironmentOverrides(t *testing.T) {
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY"} {
		t.Run(key, func(t *testing.T) {
			// Arrange: an inherited override must not redirect source or clone commands.
			repo := gitRepo(t)
			commitFile(t, repo, "source", "expected")
			t.Setenv(key, filepath.Join(t.TempDir(), "somewhere-else"))
			t.Setenv("LF_ENV_SENTINEL", "preserved")
			// Act: Git must see the isolated checkout, and other environment survives.
			res, err := Run(commitConfig(`test "$(git rev-parse --show-toplevel)" = "$PWD" && test "$(git show HEAD:source)" = expected && test "$LF_ENV_SENTINEL" = preserved`), Options{RepoDir: repo, EvidenceDir: filepath.Join(t.TempDir(), "runs"), Commit: "HEAD"})
			// Assert.
			if err != nil {
				t.Fatal(err)
			}
			if !res.Evidence.Passed() || !res.Evidence.SubjectVerified {
				t.Fatalf("override %s affected verification: %+v", key, res.Evidence)
			}
		})
	}
}
