package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func companionRepo(t *testing.T, extra string) (string, string, string) {
	t.Helper()
	repo := newTestRepo(t, passingConfig)
	state := filepath.Join(t.TempDir(), "runs")
	path := filepath.Join(t.TempDir(), "external.yml")
	body := passingConfig + extra + "\nevidence:\n  dir: " + state + "\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUNARFORGE_CONFIG", path)
	return repo, path, state
}

func TestStatusCommitFreshThenExternalContractChanged(t *testing.T) {
	_, path, _ := companionRepo(t, "")
	if err := cmdVerify([]string{"--commit", "HEAD", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdStatus([]string{"--require-fresh-passing"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\nrepair:\n  enabled: false\n")...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmdStatus([]string{"--require-fresh-passing"}); err == nil {
		t.Fatal("changed external config accepted")
	}
}

func TestStatusRefusesPassingDirtyLegacyEvidence(t *testing.T) {
	repo, _, _ := companionRepo(t, "")
	if err := os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("dirty repair"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdStatus([]string{"--require-fresh-passing"}); err == nil {
		t.Fatal("dirty repair accepted for push")
	}
}

func TestStatusRequiresCommitEvidenceWhenRequested(t *testing.T) {
	companionRepo(t, "")
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdStatus([]string{"--commit", "HEAD", "--require-fresh-passing"}); err == nil {
		t.Fatal("legacy evidence satisfied commit mode")
	}
}

func TestStatusReuseDoesNotRewriteEvidence(t *testing.T) {
	repo, path, state := companionRepo(t, "")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Add tree_reuse under the existing verify section.
	data = []byte(strings.Replace(string(data), "verify:\n", "verify:\n  tree_reuse: true\n", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmdVerify([]string{"--commit", "HEAD", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	before, runDir, err := evidence.LoadLatest(state)
	if err != nil {
		t.Fatal(err)
	}
	bytesBefore, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "commit", "--amend", "-m", "reworded fixture")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("amend: %v %s", err, out)
	}
	if err := cmdStatus([]string{"--commit", "HEAD", "--require-fresh-passing"}); err != nil {
		t.Fatal(err)
	}
	after, _, err := evidence.LoadLatest(state)
	if err != nil {
		t.Fatal(err)
	}
	bytesAfter, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bytesBefore) != string(bytesAfter) || before.RunID != after.RunID {
		t.Fatal("reuse changed execution evidence")
	}
}
