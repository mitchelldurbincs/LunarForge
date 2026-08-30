package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runErr := fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), runErr
}

func decodeResult(t *testing.T, raw string) JSONResult {
	t.Helper()
	var got JSONResult
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("invalid JSON stdout: %v\n%s", err, raw)
	}
	return got
}

func TestVerifyJSONContract(t *testing.T) {
	newTestRepo(t, passingConfig)
	raw, err := captureStdout(t, func() error { return cmdVerify([]string{"--json", "--quiet"}) })
	if err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, raw)
	if got.SchemaVersion != 1 || got.Command != "verify" || got.State != evidence.ResultPassed {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Run == nil || got.Run.ID == "" || got.Run.Fingerprint == "" || len(got.Checks) != 1 {
		t.Fatalf("missing run/check detail: %+v", got)
	}
	if got.Repository.Fingerprint != got.Run.FinalFingerprint {
		t.Fatalf("current and final fingerprints differ: %+v", got)
	}
}

func TestVerifyJSONFailureIncludesSkippedCheck(t *testing.T) {
	newTestRepo(t, failingConfig)
	raw, err := captureStdout(t, func() error { return cmdVerify([]string{"--json", "--quiet"}) })
	ee, ok := err.(*exitError)
	if !ok || ee.code != 1 {
		t.Fatalf("error = %#v, want exit 1", err)
	}
	got := decodeResult(t, raw)
	if got.State != evidence.ResultFailed || got.Reason != evidence.ReasonCheckFailed || len(got.Checks) != 3 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Checks[2].State != evidence.ResultSkipped || got.Checks[2].Reason != evidence.ReasonPreviousCheckFailed {
		t.Fatalf("missing skipped record: %+v", got.Checks[2])
	}
}

func TestStatusJSONReportsStaleCurrentSnapshot(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repo+"/marker.txt", []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := captureStdout(t, func() error { return cmdStatus([]string{"--json"}) })
	ee, ok := err.(*exitError)
	if !ok || ee.code != 1 {
		t.Fatalf("error = %#v, want exit 1", err)
	}
	got := decodeResult(t, raw)
	if got.State != evidence.ResultStale || got.Reason != evidence.ReasonSnapshotChanged {
		t.Fatalf("unexpected status: %+v", got)
	}
	if got.Run == nil || got.Repository.Fingerprint == got.Run.FinalFingerprint {
		t.Fatalf("status did not expose current vs verified snapshot: %+v", got)
	}
}

func TestStatusJSONDistinguishesNoEvidence(t *testing.T) {
	newTestRepo(t, passingConfig)
	raw, err := captureStdout(t, func() error { return cmdStatus([]string{"--json"}) })
	ee, ok := err.(*exitError)
	if !ok || ee.code != 2 {
		t.Fatalf("error = %#v, want exit 2", err)
	}
	got := decodeResult(t, raw)
	if got.State != evidence.ResultBlocked || got.Reason != evidence.ReasonNoEvidence || got.Run != nil {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestStatusJSONReportsCorruptEvidence(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	if err := cmdVerify([]string{"--quiet"}); err != nil {
		t.Fatal(err)
	}
	runDir := latestEvidenceDir(t, repo)
	if err := os.WriteFile(runDir+"/evidence.json", []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := captureStdout(t, func() error { return cmdStatus([]string{"--json"}) })
	ee, ok := err.(*exitError)
	if !ok || ee.code != 3 {
		t.Fatalf("error = %#v, want exit 3", err)
	}
	got := decodeResult(t, raw)
	if got.State != evidence.ResultError || got.Reason != evidence.ReasonEvidenceCorrupt {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestVerifyJSONReportsInvalidConfig(t *testing.T) {
	const invalid = `version: 1
unknown: value
verify:
  commands:
    - id: ok
      run: "true"
`
	newTestRepo(t, invalid)
	raw, err := captureStdout(t, func() error { return cmdVerify([]string{"--json", "--quiet"}) })
	ee, ok := err.(*exitError)
	if !ok || ee.code != 2 {
		t.Fatalf("error = %#v, want exit 2", err)
	}
	got := decodeResult(t, raw)
	if got.State != evidence.ResultBlocked || got.Reason != evidence.ReasonConfigInvalid || got.Error == nil {
		t.Fatalf("unexpected result: %+v", got)
	}
}
