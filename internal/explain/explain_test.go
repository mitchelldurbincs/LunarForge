package explain

import (
	"strings"
	"testing"

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
	if !strings.Contains(got, "No verification evidence exists") {
		t.Error("expected no-evidence note")
	}
	if !strings.Contains(got, "(no diff") {
		t.Error("expected empty-diff note")
	}
}
