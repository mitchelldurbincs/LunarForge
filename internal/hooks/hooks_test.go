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
	if !strings.Contains(body, "lf status --require-fresh-passing") {
		t.Error("hook should call lf status --require-fresh-passing")
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
