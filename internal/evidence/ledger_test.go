package evidence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func identityFixture() Identity {
	return Identity{Repository: "repo", Subject: gitutil.Subject{Commit: "old", Tree: "same"}, Contract: "contract", Platform: Platform{OS: "linux", Arch: "amd64"}}
}

func TestEvaluateCommitReusePreservesExecutionIdentity(t *testing.T) {
	before := identityFixture()
	ev := &Evidence{RunID: "original", Mode: "commit", Result: ResultPassed, Identity: &before, EndIdentity: &before, SubjectVerified: true}
	want := before
	want.Subject.Commit = "rewritten"
	r := EvaluateCommit(ev, want, true)
	if !r.Ready() || r.ReusedFrom != "original" || ev.Identity.Subject.Commit != "old" {
		t.Fatalf("bad reuse: %+v", r)
	}
	if EvaluateCommit(ev, want, false).Ready() {
		t.Fatal("reuse must be explicit")
	}
}

func TestEvaluateCommitInvalidatesInputs(t *testing.T) {
	original := identityFixture()
	ev := &Evidence{Mode: "commit", Result: ResultPassed, Identity: &original, EndIdentity: &original, SubjectVerified: true}
	for _, field := range []string{"tree", "contract", "platform", "dirty", "repository"} {
		t.Run(field, func(t *testing.T) {
			changed := original
			switch field {
			case "tree":
				changed.Subject.Tree = "different"
			case "contract":
				changed.Contract = "different"
			case "platform":
				changed.Platform.OS = "windows"
			case "dirty":
				changed.Subject.Dirty = true
			case "repository":
				changed.Repository = "different"
			}
			if EvaluateCommit(ev, changed, true).Ready() {
				t.Fatal("changed input accepted")
			}
		})
	}
}

func TestLedgerNewerFailureBlocksOlderSuccess(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs")
	identity := identityFixture()
	for i, result := range []string{ResultPassed, ResultFailed} {
		id := NewRunID(time.Now())
		path := RunDir(dir, id)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		ev := &Evidence{RunID: id, Mode: "commit", Result: result, Identity: &identity, EndIdentity: &identity, SubjectVerified: true, StartedAt: time.Unix(int64(i), 0)}
		if err := Write(dir, path, ev); err != nil {
			t.Fatal(err)
		}
	}
	ev, _, err := SelectCommit(dir, identity, true)
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil || ev.Result != ResultFailed || EvaluateCommit(ev, identity, true).Ready() {
		t.Fatal("new failure hidden")
	}
}

func TestWriteRefusesToMutateCompletedEvidence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs")
	path := RunDir(dir, "run")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	ev := &Evidence{RunID: "run", Result: ResultFailed}
	if err := Write(dir, path, ev); err != nil {
		t.Fatal(err)
	}
	ev.Result = ResultPassed
	if err := Write(dir, path, ev); err == nil {
		t.Fatal("completed record overwritten")
	}
	saved, err := Load(path)
	if err != nil || saved.Result != ResultFailed {
		t.Fatal("original failure changed")
	}
}
