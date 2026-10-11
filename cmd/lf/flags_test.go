package main

import (
	"os"
	"strings"
	"testing"
)

func TestCLIFlagHandling(t *testing.T) {
	binary := buildCLI(t)
	commands := []string{"init", "verify", "ci", "status", "install-hooks", "gen-actions"}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			for _, flag := range []string{"-h", "--help"} {
				t.Run(flag, func(t *testing.T) {
					repo := t.TempDir()
					stdout, stderr := invokeCLI(t, binary, repo, 0, command, flag)
					if stdout != "" || !strings.Contains(stderr, "Usage: lf "+command) {
						t.Fatalf("unexpected help: stdout=%q stderr=%q", stdout, stderr)
					}
					if strings.Contains(stderr, "help requested") {
						t.Fatalf("help was reported as an error: %q", stderr)
					}
					assertEmptyDirectory(t, repo)
				})
			}
			t.Run("unknown-flag", func(t *testing.T) {
				assertFlagError(t, binary, command, "--unknown-flag", "flag provided but not defined")
			})
		})
	}
}

func TestCLIInvalidFlagValues(t *testing.T) {
	binary := buildCLI(t)
	for _, tc := range []struct{ command, flag, diagnostic string }{
		{"init", "--force=invalid", "invalid boolean value"},
		{"verify", "--json=invalid", "invalid boolean value"},
		{"ci", "--quiet=invalid", "invalid boolean value"},
		{"status", "--strict=invalid", "invalid boolean value"},
		{"gen-actions", "--output", "flag needs an argument"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			assertFlagError(t, binary, tc.command, tc.flag, tc.diagnostic)
		})
	}
}

func assertFlagError(t *testing.T, binary, command, flag, diagnostic string) {
	t.Helper()
	repo := t.TempDir()
	stdout, stderr := invokeCLI(t, binary, repo, 2, command, flag)
	if stdout != "" || strings.Count(stderr, diagnostic) != 1 {
		t.Fatalf("want one flag diagnostic: stdout=%q stderr=%q", stdout, stderr)
	}
	if strings.Count(stderr, "Usage: lf "+command) != 1 {
		t.Fatalf("want one usage message: %q", stderr)
	}
	assertEmptyDirectory(t, repo)
}

func assertEmptyDirectory(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("help or invalid flags must not create artifacts: %v", entries)
	}
}
