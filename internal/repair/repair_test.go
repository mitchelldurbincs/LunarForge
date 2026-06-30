package repair

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

func TestBuildPromptContainsContext(t *testing.T) {
	got := BuildPrompt(PromptInput{
		Project:        "demo",
		Branch:         "main",
		Head:           "abc123",
		Status:         " M src/hello.txt\n",
		Diff:           "diff --git a/src/hello.txt b/src/hello.txt\n",
		VerifyCommands: []string{"./scripts/verify.sh"},
		Failed: []FailedCommand{{
			ID:            "contents",
			Run:           "./scripts/verify.sh",
			ExitCode:      1,
			StdoutPath:    ".lf/runs/r1/commands/contents.stdout.txt",
			StderrPath:    ".lf/runs/r1/commands/contents.stderr.txt",
			StderrExcerpt: "does not contain expected text",
		}},
	})
	for _, want := range []string{
		"You are repairing a failed LunarForge verification run.",
		"smallest safe diff",
		"Do not push, commit, or create branches.",
		"do not claim success",
		"contents",                                 // failed command id
		"./scripts/verify.sh",                      // failed command string + verify command
		".lf/runs/r1/commands/contents.stderr.txt", // log path
		"does not contain expected text",           // log excerpt
		"diff --git",                               // diff
		"Project: demo",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildPromptStaleNote(t *testing.T) {
	got := BuildPrompt(PromptInput{Project: "demo", Stale: true})
	if !strings.Contains(got, "working tree has changed") {
		t.Error("expected stale note in prompt")
	}
}

func TestTruncateKeepsTailAndMarks(t *testing.T) {
	s := strings.Repeat("a", 50) + "TAIL"
	out, truncated := Truncate(s, 10)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if !strings.HasSuffix(out, "TAIL") {
		t.Errorf("expected tail kept, got %q", out)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation marker, got %q", out)
	}
	// Short input is returned untouched.
	if out2, tr := Truncate("short", 100); tr || out2 != "short" {
		t.Errorf("short input should be unchanged, got %q (truncated=%v)", out2, tr)
	}
}

func TestRunAgentReadsStdinAndCaptures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script uses a POSIX shebang")
	}
	repo := t.TempDir()
	// Agent echoes a marker and the prompt it read on stdin, proving stdin wiring.
	script := "#!/bin/sh\nread line\necho \"AGENT-SAW: $line\"\n"
	if err := os.WriteFile(filepath.Join(repo, "agent.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := config.Agent{Backend: "fake", Command: "./agent.sh"}
	res, err := RunAgent("fake", a, repo, "PROMPT-LINE\n")
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "AGENT-SAW: PROMPT-LINE") {
		t.Errorf("agent did not receive prompt on stdin: %q", res.Stdout)
	}
}

func TestRunAgentMissingCommand(t *testing.T) {
	repo := t.TempDir()
	a := config.Agent{Command: "./does-not-exist.sh"}
	res, err := RunAgent("x", a, repo, "p")
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if res.Err == "" {
		t.Error("expected res.Err to be populated")
	}
}

func TestRunAgentNonZeroExitIsNotError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script uses a POSIX shebang")
	}
	repo := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\nexit 3\n"
	if err := os.WriteFile(filepath.Join(repo, "fail.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := RunAgent("fail", config.Agent{Command: "./fail.sh"}, repo, "p")
	if err != nil {
		t.Fatalf("non-zero exit should not be a Go error: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}
