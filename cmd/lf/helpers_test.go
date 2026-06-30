package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// newTestRepo creates a git repo in a temp dir containing a .lunarforge.yml with
// the given verify command and a committed source file, then changes the test's
// working directory into it. It skips on Windows / when git is missing, matching
// the runner tests' POSIX-shell assumptions.
func newTestRepo(t *testing.T, configYAML string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell commands in test assume a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lunarforge.yml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "t"},
		{"add", "."},
		{"commit", "-m", "init"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Chdir(dir)
	return dir
}

const passingConfig = `version: 1
project:
  name: ci-test
verify:
  commands:
    - id: ok
      run: "true"
`

const failingConfig = `version: 1
project:
  name: ci-test
verify:
  commands:
    - id: ok
      run: "true"
    - id: bad
      run: "exit 7"
`
