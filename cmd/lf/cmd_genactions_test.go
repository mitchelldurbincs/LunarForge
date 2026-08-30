package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenActionsWritesCanonicalWorkflow(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	if err := cmdGenActions([]string{"--install-ref", "v0.2.0"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".github", "workflows", "lunarforge.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "@v0.2.0") || !strings.Contains(out, "lf verify --json") {
		t.Fatalf("unexpected workflow:\n%s", out)
	}
	if err := cmdGenActions(nil); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	if err := cmdGenActions([]string{"--force"}); err != nil {
		t.Fatalf("force overwrite: %v", err)
	}
}

func TestGenActionsCustomOutput(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	path := filepath.Join("automation", "verify.yml")
	if err := cmdGenActions([]string{"--output", path}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, path)); err != nil {
		t.Fatal(err)
	}
}
