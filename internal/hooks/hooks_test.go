package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
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
	c := exec.Command("git", "init")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
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
	if !strings.Contains(body, `lf pre-push "$@"`) {
		t.Error("hook should forward stdin and arguments to lf pre-push")
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

func TestInstalledHookForwardsStdin(t *testing.T) {
	dir := gitInit(t)
	bin := t.TempDir()
	captured := filepath.Join(bin, "captured")
	fakeLF := "#!/bin/sh\n[ \"$1\" = pre-push ] || exit 2\ncat > \"$CAPTURE_REFS\"\nexit 7\n"
	if err := os.WriteFile(filepath.Join(bin, "lf"), []byte(fakeLF), 0755); err != nil {
		t.Fatal(err)
	}
	hook, err := InstallPrePush(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", hook.Path, "origin", "unused")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "CAPTURE_REFS="+captured)
	input := "refs/heads/main local refs/heads/main remote\n"
	cmd.Stdin = strings.NewReader(input)
	if err := cmd.Run(); err == nil {
		t.Fatal("hook must propagate rejection")
	}
	data, err := os.ReadFile(captured)
	if err != nil || string(data) != input {
		t.Fatalf("stdin lost: %q %v", data, err)
	}
}
