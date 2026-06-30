package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, StarterTemplate("demo"))

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if cfg.Project.Name != "demo" {
		t.Errorf("project name = %q, want demo", cfg.Project.Name)
	}
	if len(cfg.Verify.Commands) != 1 || cfg.Verify.Commands[0].ID != "verify" {
		t.Errorf("unexpected verify commands: %+v", cfg.Verify.Commands)
	}
	if cfg.Explain.Command != "claude" {
		t.Errorf("explain command = %q, want claude", cfg.Explain.Command)
	}
	if cfg.EvidenceDir() != filepath.Join(".lf", "runs") {
		t.Errorf("evidence dir = %q", cfg.EvidenceDir())
	}
	if !cfg.Evidence.RequireFreshDiff {
		t.Errorf("require_fresh_diff should default to true in starter template")
	}
}

func TestLoadRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "version: 2\nproject:\n  name: x\nverify:\n  commands:\n    - id: a\n      run: echo hi\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestLoadRejectsNoCommands(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "version: 1\nproject:\n  name: x\nverify:\n  commands: []\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty verify.commands")
	}
}

func TestLoadRejectsDuplicateID(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "version: 1\nproject:\n  name: x\nverify:\n  commands:\n    - id: a\n      run: echo 1\n    - id: a\n      run: echo 2\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for duplicate command id")
	}
}

func TestFindWalksUp(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, StarterTemplate("demo"))
	nested := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found != filepath.Join(dir, FileName) {
		t.Errorf("Find returned %q", found)
	}
}
