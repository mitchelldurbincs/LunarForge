package main

import (
	"encoding/json"
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

func TestStatusJSONSeparatesLocalAndExternalContracts(t *testing.T) {
	repo, path, _ := companionRepo(t, "")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte(`
status:
  contracts:
    - id: windows-restructure
      platform: windows
      status: pending
      reason: platform not available
    - id: coverage-warnings
      status: enforced-remotely
      authority: ADO
      ado:
        definition_id: 111
        build_id: 123
        source_commit: observed-source
        merge_commit: observed-merge
`)...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmdVerify([]string{"--commit", "HEAD", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	// A refreshed remote observation must not invalidate local execution.
	data = []byte(strings.Replace(string(data), "build_id: 123", "build_id: 124", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := inDir(t, repo, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		CurrentDiffHash string `json:"current_diff_hash"`
		Fresh           bool   `json:"fresh"`
		Passed          bool   `json:"passed"`
		All             bool   `json:"all_contracts_satisfied"`
		Subject         struct {
			Commit string `json:"commit"`
		} `json:"current_subject"`
		Contracts []struct {
			Status string `json:"status"`
			ADO    struct {
				BuildID int `json:"build_id"`
			} `json:"ado"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatal(err)
	}
	if status.CurrentDiffHash == "" || !status.Fresh || !status.Passed || status.All || len(status.Subject.Commit) != 40 || len(status.Contracts) != 3 {
		t.Fatalf("bad summary: %s", out)
	}
	if status.Contracts[1].Status != "pending" || status.Contracts[2].Status != "enforced-remotely" || status.Contracts[2].ADO.BuildID != 124 {
		t.Fatalf("external evidence misreported: %s", out)
	}
}

func TestStatusJSONBlocksDirtyLegacySubjectWithoutEnforcement(t *testing.T) {
	// Arrange: a passing diff run whose dirty subject is still hash-fresh.
	repo, _, _ := companionRepo(t, "")
	if err := os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("dirty repair"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	// Act: plain JSON exits successfully even when it reports not ready.
	out, err := inDir(t, repo, func() error { return cmdStatus([]string{"--json"}) })
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Ready     bool   `json:"ready"`
		Fresh     bool   `json:"fresh"`
		Passed    bool   `json:"passed"`
		Reason    string `json:"reason"`
		Contracts []struct {
			Status string `json:"status"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatal(err)
	}
	if status.Ready || !status.Fresh || !status.Passed || status.Reason != "dirty_subject" || len(status.Contracts) != 1 || status.Contracts[0].Status != "dirty_subject" {
		t.Fatalf("incorrect readiness: %s", out)
	}
	strict, err := inDir(t, repo, func() error { return cmdStatus([]string{"--json", "--require-fresh-passing"}) })
	if exit, ok := err.(*exitError); !ok || exit.code != 1 {
		t.Fatalf("strict status error = %v", err)
	}
	if strict != out {
		t.Fatalf("plain and strict JSON differ:\n%s\n%s", out, strict)
	}
}
