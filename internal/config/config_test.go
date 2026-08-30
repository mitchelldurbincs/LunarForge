package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStarterTemplateLoads(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(writeConfig(t, dir, StarterTemplate()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || len(cfg.Verify.Commands) != 1 || cfg.Verify.Commands[0].ID != "verify" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Verify.Commands[0].Timeout() != 30*time.Minute {
		t.Fatalf("unexpected default timeout: %s", cfg.Verify.Commands[0].Timeout())
	}
}

func TestLoadValidatesPolicy(t *testing.T) {
	cases := []struct{ name, yaml, want string }{
		{"version", "version: 2\nverify:\n  commands:\n    - id: test\n      run: true\n", "unsupported config version"},
		{"commands", "version: 1\nverify:\n  commands: []\n", "at least one"},
		{"duplicate", "version: 1\nverify:\n  commands:\n    - id: test\n      run: true\n    - id: test\n      run: true\n", "duplicate"},
		{"unsafe id", "version: 1\nverify:\n  commands:\n    - id: ../test\n      run: true\n", "must match"},
		{"negative timeout", "version: 1\nverify:\n  commands:\n    - id: test\n      run: true\n      timeout_seconds: -1\n", "cannot be negative"},
		{"blank run", "version: 1\nverify:\n  commands:\n    - id: test\n      run: '   '\n", "run is required"},
		{"unknown field", "version: 1\nproject: demo\nverify:\n  commands:\n    - id: test\n      run: true\n", "field project not found"},
		{"multiple documents", "version: 1\nverify:\n  commands:\n    - id: test\n      run: true\n---\nversion: 1\n", "multiple YAML documents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, t.TempDir(), tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestFindWalksUp(t *testing.T) {
	dir := t.TempDir()
	want := writeConfig(t, dir, StarterTemplate())
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Find = %q, want %q", got, want)
	}
}
