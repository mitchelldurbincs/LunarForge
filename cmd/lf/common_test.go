package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExternalConfigFromNestedInvocation(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	outside := t.TempDir()
	path := filepath.Join(outside, "companion.yml")
	state := filepath.Join(outside, "state", "runs")
	if err := os.WriteFile(path, []byte(passingConfig+"\nevidence:\n  dir: "+state+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv("LUNARFORGE_CONFIG", path)
	l, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if l.repoDir != repo || l.cfg.Path() != path || l.evidenceDir != state {
		t.Fatalf("wrong resolved paths: %+v", l)
	}
}

func TestLoadInvalidExplicitConfigDoesNotFallBack(t *testing.T) {
	newTestRepo(t, passingConfig)
	for _, value := range []string{"", filepath.Join(t.TempDir(), "missing.yml")} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("LUNARFORGE_CONFIG", value)
			if _, err := load(); err == nil {
				t.Fatal("expected explicit config error")
			}
		})
	}
}

func TestLoadExternalConfigRequiresExternalState(t *testing.T) {
	newTestRepo(t, passingConfig)
	path := filepath.Join(t.TempDir(), "companion.yml")
	if err := os.WriteFile(path, []byte(passingConfig), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUNARFORGE_CONFIG", path)
	if _, err := load(); err == nil || !strings.Contains(err.Error(), "absolute evidence.dir") {
		t.Fatalf("got %v", err)
	}
}
