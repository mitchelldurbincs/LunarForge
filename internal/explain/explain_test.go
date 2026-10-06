package explain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func TestBuildPromptFresh(t *testing.T) {
	ev := &evidence.Evidence{
		RunID:    "2026-06-30T14-22-10",
		Result:   evidence.ResultPassed,
		DiffHash: "sha256:abc",
		Commands: []evidence.Command{{ID: "test", Run: "go test", Result: "passed", ExitCode: 0}},
	}
	got := BuildPrompt(PromptInput{
		Project:       "demo",
		Branch:        "main",
		Status:        " M file.go\n",
		Diff:          "diff --git a/file.go b/file.go\n",
		Evidence:      ev,
		HasEvidence:   true,
		EvidenceFresh: true,
	})
	for _, want := range []string{
		"Manual review suggestions",
		"FRESH",
		"go test",
		"diff --git",
		"Project: demo",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildPromptStaleNoEvidence(t *testing.T) {
	got := BuildPrompt(PromptInput{
		Project:     "demo",
		Diff:        "",
		HasEvidence: false,
	})
	if !strings.Contains(got, "MISSING") {
		t.Error("expected MISSING status for no evidence")
	}
	if !strings.Contains(got, "(no diff") {
		t.Error("expected empty-diff note")
	}
}

func TestRunWithRelativeFakeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script uses a POSIX shebang")
	}
	repo := t.TempDir()
	// The fake explain command echoes a fixed line plus the prompt it received,
	// proving args are passed through and the relative path resolves vs repoDir.
	script := "#!/bin/sh\nprintf 'EXPLANATION for: %s\\n' \"$1\"\n"
	if err := os.WriteFile(filepath.Join(repo, "fake-explain.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(repo, ".lf", "runs", "r1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Explain: config.Explain{Command: "./fake-explain.sh"}}
	out, err := Run(cfg, repo, runDir, "PROMPT-BODY")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "EXPLANATION for: PROMPT-BODY") {
		t.Errorf("unexpected explanation output: %q", out)
	}
	// Both prompt and explanation should be persisted.
	if _, err := os.Stat(filepath.Join(runDir, "explain-prompt.md")); err != nil {
		t.Errorf("prompt not saved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "explanation.md")); err != nil {
		t.Errorf("explanation not saved: %v", err)
	}
}

func TestRunMissingCommandPreservesPrompt(t *testing.T) {
	repo := t.TempDir()
	runDir := filepath.Join(repo, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Explain: config.Explain{Command: "./does-not-exist.sh"}}
	if _, err := Run(cfg, repo, runDir, "PROMPT"); err == nil {
		t.Fatal("expected error for missing command")
	}
	// Even on failure the prompt must be preserved.
	if _, err := os.Stat(filepath.Join(runDir, "explain-prompt.md")); err != nil {
		t.Errorf("prompt should be saved even when command is missing: %v", err)
	}
}

func TestBuildPromptIncludesCheckResults(t *testing.T) {
	// Arrange.
	ev := &evidence.Evidence{Result: evidence.ResultFailed, Contracts: []evidence.Contract{
		{ID: "format", Status: "failed", ExitCode: 1, Reason: "changes needed\n7 files"},
		{ID: "tool", Status: "error", ExitCode: 3, Reason: "tool unavailable"},
	}}
	// Act.
	got := BuildPrompt(PromptInput{Evidence: ev, HasEvidence: true, EvidenceFresh: true})
	// Assert.
	for _, want := range []string{"Contract format: failed (exit 1), fresh=true — changes needed 7 files", "Contract tool: error (exit 3), fresh=true — tool unavailable"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
