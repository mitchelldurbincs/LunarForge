package gitutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSubjectIgnoresArtifacts(t *testing.T) {
	// Arrange: no root ignore file; porcelain contains only LF artifacts.
	dir := gitInit(t)
	for _, path := range []string{".lf/.gitignore", ".lf/runs/record", ".lf/latest", ".lf/loops/summary", "state/runs/record"} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("artifact"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Act.
	subject, err := ReadSubject(dir, "state")
	// Assert.
	if err != nil || subject.Dirty {
		t.Fatalf("subject = %+v, err = %v", subject, err)
	}
}

func TestPorcelainDirty(t *testing.T) {
	cases := []struct {
		name, status string
		dirty        bool
	}{
		{"legacy directory", "?? .lf/\n", false},
		{"artifact files", "?? .lf/.gitignore\n?? .lf/loops/a\n M .lf/latest\n?? state/runs/a\n", false},
		{"quoted artifact", "?? \".lf/space name\"\n", false},
		{"artifact rename", "R  .lf/old -> .lf/new\n", false},
		{"source rename into artifacts", "R  source -> .lf/new\n", true},
		{"source rename out of artifacts", "R  .lf/old -> source\n", true},
		{"source", " M source\n?? .lf/\n", true},
		{"similar prefix", "?? .lf-source\n", true},
		{"malformed", "bad", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PorcelainDirty(c.status, "state"); got != c.dirty {
				t.Fatalf("dirty = %v, want %v", got, c.dirty)
			}
		})
	}
}
