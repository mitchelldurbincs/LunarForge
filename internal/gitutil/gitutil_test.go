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

// TestDiffHashTrackedChanges mirrors the freshness invariants the gate relies
// on: once a file is tracked, both unstaged and staged content changes flip the
// hash (and therefore evidence freshness).
func TestDiffHashTrackedChanges(t *testing.T) {
	dir := gitInit(t)

	// Commit a tracked file so we start from a clean working tree.
	path := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(path, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "tracked.txt")
	mustGit(t, dir, "commit", "-m", "add tracked")

	clean, err := DiffHash(dir)
	if err != nil {
		t.Fatal(err)
	}

	// 1. No changes -> hash stable (fresh).
	again, _ := DiffHash(dir)
	if clean != again {
		t.Error("clean tree hash should be stable")
	}

	// 2. Unstaged tracked change -> hash changes (stale).
	if err := os.WriteFile(path, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unstaged, _ := DiffHash(dir)
	if unstaged == clean {
		t.Error("unstaged tracked change should change the hash")
	}

	// 3. Staging that change -> hash changes again (moves from worktree to index).
	mustGit(t, dir, "add", "tracked.txt")
	staged, _ := DiffHash(dir)
	if staged == unstaged {
		t.Error("staging should change the hash")
	}

	// 4. A status-only change (new untracked file) -> hash changes.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withUntracked, _ := DiffHash(dir)
	if withUntracked == staged {
		t.Error("a new untracked file should change the porcelain status and the hash")
	}
}

// TestDiffHashTracksHead ensures two distinct clean trees do not collide: a new
// commit must change the hash even though both working trees are clean. This is
// what stops a freshly-committed change from inheriting older evidence.
func TestDiffHashTracksHead(t *testing.T) {
	dir := gitInit(t)
	path := filepath.Join(dir, "f.txt")

	if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "f.txt")
	mustGit(t, dir, "commit", "-m", "a")
	hashA, _ := DiffHash(dir)

	if err := os.WriteFile(path, []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "f.txt")
	mustGit(t, dir, "commit", "-m", "b")
	hashB, _ := DiffHash(dir)

	// Both trees are clean (empty diff), but the commits differ.
	if hashA == hashB {
		t.Error("a new commit on a clean tree must change the diff hash")
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
