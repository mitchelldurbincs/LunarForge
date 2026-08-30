package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func gitInit(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "initial"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestPrePushRequiresCleanCheckedOutHead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executes the POSIX hook")
	}
	dir := gitInit(t)
	res, err := InstallPrePush(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "lf"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitOutput(t, dir, "rev-parse", "HEAD"))
	input := "refs/heads/main " + head + " refs/heads/main 0000000000000000000000000000000000000000\n"
	run := func(input string) error {
		cmd := exec.Command(res.Path, "origin", "unused")
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(input)
		cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		return cmd.Run()
	}
	if err := run(input); err != nil {
		t.Fatalf("clean verified HEAD should pass: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(input); err == nil {
		t.Fatal("dirty worktree should be rejected")
	}
	if err := os.Remove(filepath.Join(dir, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	if err := run("refs/heads/main 1111111111111111111111111111111111111111 refs/heads/main " + head + "\n"); err == nil {
		t.Fatal("unknown/non-HEAD object should be rejected")
	}
}

func TestPrePushAcceptsAnnotatedTagOfHead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test executes the POSIX hook")
	}
	dir := gitInit(t)
	res, err := InstallPrePush(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	gitOutput(t, dir, "tag", "-a", "v1.0.0", "-m", "release")
	tagObject := strings.TrimSpace(gitOutput(t, dir, "rev-parse", "refs/tags/v1.0.0"))
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "lf"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(res.Path, "origin", "unused")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("refs/tags/v1.0.0 " + tagObject + " refs/tags/v1.0.0 0000000000000000000000000000000000000000\n")
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("annotated tag of HEAD should pass: %v\n%s", err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestInstallPrePushFresh(t *testing.T) {
	dir := gitInit(t)
	res, err := InstallPrePush(dir, time.Now())
	if err != nil {
		t.Fatalf("InstallPrePush: %v", err)
	}
	if res.BackupPath != "" {
		t.Errorf("unexpected backup on a fresh install: %s", res.BackupPath)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("reading hook: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, marker) {
		t.Error("hook missing management marker")
	}
	if !strings.Contains(body, "lf status --require-fresh-passing") {
		t.Error("hook should call lf status --require-fresh-passing")
	}
	if !strings.Contains(body, "git status --porcelain") || !strings.Contains(body, "git rev-parse HEAD") {
		t.Error("hook should require a clean checked-out HEAD")
	}
}

func TestInstallPrePushUpdatesManagedHook(t *testing.T) {
	dir := gitInit(t)
	if _, err := InstallPrePush(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	res, err := InstallPrePush(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Replaced {
		t.Error("re-installing over a managed hook should set Replaced")
	}
	if res.BackupPath != "" {
		t.Error("managed hook should be updated in place, not backed up")
	}
}

func TestInstallPrePushBacksUpForeignHook(t *testing.T) {
	dir := gitInit(t)
	hooksDir, err := HooksDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := "#!/bin/sh\necho my custom hook\n"
	hookPath := filepath.Join(hooksDir, "pre-push")
	if err := os.WriteFile(hookPath, []byte(foreign), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := InstallPrePush(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath == "" {
		t.Fatal("foreign hook should have been backed up")
	}
	backup, err := os.ReadFile(res.BackupPath)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(backup) != foreign {
		t.Error("backup should preserve the original foreign hook contents")
	}
	// The new hook is the managed one.
	newHook, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(newHook), marker) {
		t.Error("installed hook should be the managed hook")
	}
}
