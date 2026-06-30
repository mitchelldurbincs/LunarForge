package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenActionsCreatesWorkflow(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	if err := cmdGenActions(nil); err != nil {
		t.Fatalf("lf gen-actions: %v", err)
	}

	wfPath := filepath.Join(repo, ".github", "workflows", "lunarforge.yml")
	data, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("workflow not created: %v", err)
	}
	content := string(data)

	for _, frag := range []string{
		"pull_request:",
		"push:",
		"- main",
		"concurrency:",
		"permissions:\n  contents: read",
		"uses: actions/checkout@v4",
		"run: go build -o lf ./cmd/lf",
		"run: ./lf ci",
		"uses: actions/upload-artifact@v4",
	} {
		if !strings.Contains(content, frag) {
			t.Errorf("generated workflow missing %q", frag)
		}
	}
}

func TestGenActionsRefusesOverwriteWithoutForce(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	if err := cmdGenActions(nil); err != nil {
		t.Fatalf("first gen-actions: %v", err)
	}
	// Second run without --force must refuse.
	if err := cmdGenActions(nil); err == nil {
		t.Fatal("expected gen-actions to refuse overwriting existing workflow")
	}
	// With --force it should overwrite successfully.
	if err := cmdGenActions([]string{"--force"}); err != nil {
		t.Fatalf("gen-actions --force should overwrite: %v", err)
	}

	wfPath := filepath.Join(repo, ".github", "workflows", "lunarforge.yml")
	if _, err := os.Stat(wfPath); err != nil {
		t.Errorf("workflow should still exist after --force: %v", err)
	}
}

func TestGenActionsCustomOutput(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	out := filepath.Join(".github", "workflows", "custom.yml")
	if err := cmdGenActions([]string{"--output", out}); err != nil {
		t.Fatalf("gen-actions --output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, out)); err != nil {
		t.Errorf("custom workflow not created: %v", err)
	}
}
