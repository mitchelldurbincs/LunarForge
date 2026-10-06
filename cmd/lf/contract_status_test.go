package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readCheckStatus(t *testing.T, repo string) map[string]any {
	t.Helper()
	out, err := inDir(t, repo, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestStatusCheckReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, command, status string
		ready                 bool
		exit                  float64
	}{
		{"passed", "true", "passed", true, 0},
		{"failed", "echo changes needed >&2; exit 1", "failed", false, 1},
		{"error", "exit 3", "error", false, 3},
		{"missing executable", "./absent-tool", "error", false, 127},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			repo, _, _ := companionRepo(t, "\nstatus:\n  contracts:\n    - id: format\n      check: '"+tc.command+"'\n")
			// Act.
			err := cmdVerify([]string{"--quiet"})
			status := readCheckStatus(t, repo)
			row := status["contracts"].([]any)[1].(map[string]any)
			profile := status["contracts"].([]any)[0].(map[string]any)
			// Assert.
			if profile["status"] != "passed" || profile["ready"] != true {
				t.Fatalf("check result obscured passing profile: %+v", profile)
			}
			if (err == nil) != tc.ready || status["ready"] != tc.ready || status["all_contracts_satisfied"] != tc.ready || row["status"] != tc.status || row["fresh"] != true || row["exit_code"] != tc.exit || row["run_id"] != status["run_id"] || row["command"] != tc.command {
				t.Fatalf("unexpected status: %+v, err=%v", status, err)
			}
			if !tc.ready && !strings.Contains(status["reason"].(string), "format") {
				t.Fatalf("missing ID: %+v", status)
			}
		})
	}
}

func TestStatusCheckContractsJSONGolden(t *testing.T) {
	// Arrange: declarative and executable rows interleave in config order.
	goldenPath, err := filepath.Abs("testdata/check-contracts.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	repo, _, _ := companionRepo(t, `
status:
  contracts:
    - id: remote
      status: enforced-remotely
      authority: ADO
      ado:
        definition_id: 111
    - id: clean
      check: 'true'
    - id: windows
      platform: windows
      status: pending
    - id: format
      check: 'printf "changes needed" >&2; exit 1'
    - id: broken
      check: 'exit 3'
`)
	// Act.
	verify, err := inDir(t, repo, func() error { return cmdVerify([]string{"--quiet"}) })
	if exit, ok := err.(*exitError); !ok || exit.code != 1 {
		t.Fatalf("verify: %v", err)
	}
	status := readCheckStatus(t, repo)
	rows := status["contracts"].([]any)
	for _, row := range rows {
		r := row.(map[string]any)
		if _, ok := r["run_id"]; ok {
			r["run_id"] = "run"
		}
	}
	// The platform is irrelevant to this JSON shape test.
	rows[0].(map[string]any)["id"] = "linux"
	rows[0].(map[string]any)["platform"] = "linux"
	got, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	explain, err := inDir(t, repo, func() error { return cmdExplain([]string{"--no-run"}) })
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Fatalf("JSON rows differ from golden:\n%s", got)
	}
	for _, line := range []string{"Contract clean: passed (exit 0)", "Contract format: failed (exit 1)", "Contract broken: error (exit 3)"} {
		if !strings.Contains(verify, line) || !strings.Contains(explain, line) {
			t.Fatalf("missing %q in verify/explain:\n%s\n%s", line, verify, explain)
		}
	}
	if status["ready"] != false || status["all_contracts_satisfied"] != false {
		t.Fatalf("failed contracts accepted: %+v", status)
	}
}

func TestStatusCheckPendingJSONGolden(t *testing.T) {
	// Arrange.
	goldenPath, err := filepath.Abs("testdata/pending-check.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	repo, _, _ := companionRepo(t, "\nstatus:\n  contracts:\n    - id: format\n      check: 'true'\n")
	// Act.
	status := readCheckStatus(t, repo)
	row := status["contracts"].([]any)[1]
	got, err := json.MarshalIndent(row, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) || status["ready"] != false || status["all_contracts_satisfied"] != false {
		t.Fatalf("pending shape/readiness wrong:\n%s", got)
	}
}

func TestStatusCheckCommitRewriteIsStale(t *testing.T) {
	// Arrange.
	repo, path, _ := companionRepo(t, "\nstatus:\n  contracts:\n    - id: format\n      check: 'true'\n")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "verify:\n", "verify:\n  tree_reuse: true\n", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmdVerify([]string{"--commit", "HEAD", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	// Act.
	cmd := exec.Command("git", "commit", "--amend", "-m", "new commit identity")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("amend: %v %s", err, out)
	}
	status := readCheckStatus(t, repo)
	row := status["contracts"].([]any)[1].(map[string]any)
	explanation, err := inDir(t, repo, func() error { return cmdExplain([]string{"--no-run"}) })
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	if status["ready"] != false || status["fresh"] != false || row["fresh"] != false || row["ready"] != false || row["reused_from"] != "" || !strings.Contains(row["reason"].(string), "stale") || !strings.Contains(explanation, "fresh=false") {
		t.Fatalf("rewrite was accepted: %+v\n%s", status, explanation)
	}
}

func TestStatusCheckDiffChangeIsStale(t *testing.T) {
	// Arrange.
	repo, _, _ := companionRepo(t, "\nstatus:\n  contracts:\n    - id: format\n      check: 'true'\n")
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	// Act.
	if err := os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("changed diff"), 0644); err != nil {
		t.Fatal(err)
	}
	status := readCheckStatus(t, repo)
	row := status["contracts"].([]any)[1].(map[string]any)
	// Assert.
	if row["fresh"] != false || status["ready"] != false || status["all_contracts_satisfied"] != false {
		t.Fatalf("stale check accepted: %+v", status)
	}
}

func TestStatusChangedCheckCannotUseOldEvidence(t *testing.T) {
	for _, mode := range []string{"diff", "commit"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			repo, path, _ := companionRepo(t, "\nstatus:\n  contracts:\n    - id: format\n      check: 'true'\n")
			args := []string{"--quiet"}
			if mode == "commit" {
				args = append(args, "--commit", "HEAD")
			}
			if err := cmdVerify(args); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Act.
			if err := os.WriteFile(path, []byte(strings.Replace(string(data), "check: 'true'", "check: 'exit 1'", 1)), 0600); err != nil {
				t.Fatal(err)
			}
			status := readCheckStatus(t, repo)
			row := status["contracts"].([]any)[1].(map[string]any)
			verifyErr := cmdVerify(args)
			after := readCheckStatus(t, repo)
			// Assert.
			if status["ready"] != false || row["status"] != "pending" || row["fresh"] != false || row["exit_code"] != nil || verifyErr == nil || after["contracts"].([]any)[1].(map[string]any)["status"] != "failed" {
				t.Fatalf("changed command accepted: before=%+v after=%+v", status, after)
			}
		})
	}
}
