package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestIsRepo(t *testing.T) {
	dir := gitInit(t)
	if !IsRepo(dir) {
		t.Error("expected IsRepo=true for git dir")
	}
	if IsRepo(t.TempDir()) {
		t.Error("expected IsRepo=false for non-git dir")
	}
}

func TestDiffHashChangesWithWorkingTree(t *testing.T) {
	dir := gitInit(t)

	h0, err := DiffHash(dir)
	if err != nil {
		t.Fatalf("DiffHash: %v", err)
	}

	// Adding an untracked file changes porcelain status -> hash changes.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := DiffHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h0 == h1 {
		t.Error("hash should change after adding an untracked file")
	}

	// Staging the file changes the cached diff -> hash changes again.
	run(dir, "add", "a.txt")
	h2, err := DiffHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("hash should change after staging the file")
	}

	// Identical state should produce identical hash (determinism).
	h2again, err := DiffHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h2 != h2again {
		t.Error("hash should be deterministic for identical state")
	}
}
