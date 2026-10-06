package evidence

import (
	"os"
	"path/filepath"
	"strings"
)

// SelectCommit returns the newest applicable execution, including failures. A
// newer failure must never be hidden by reusing an older success. Incomplete run
// directories have no evidence.json and do not count as completed executions.
func SelectCommit(dir string, want Identity, reuse bool) (*Evidence, string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var selected *Evidence
	var selectedDir string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		ev, err := Load(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if ev.Mode != "commit" || ev.Identity == nil || !ev.Identity.SameInputs(want) {
			continue
		}
		if (!reuse || len(ev.Contracts) != 0) && ev.Identity.Subject.Commit != want.Subject.Commit {
			continue
		}
		if selected == nil || ev.StartedAt.After(selected.StartedAt) || (ev.StartedAt.Equal(selected.StartedAt) && ev.RunID > selected.RunID) {
			selected, selectedDir = ev, path
		}
	}
	return selected, selectedDir, nil
}

// EvaluateCommit checks a saved execution against the current clean subject.
func EvaluateCommit(ev *Evidence, want Identity, reuse bool) Readiness {
	r := Evaluate(ev, "")
	r.Fresh = false
	if want.Subject.Dirty {
		r.Blocked = "dirty_subject"
	}
	if ev == nil {
		return r
	}
	if ev.Mode != "commit" || ev.Identity == nil || ev.EndIdentity == nil || !ev.SubjectVerified {
		return r
	}
	if ev.Identity.Subject.Dirty || ev.EndIdentity.Subject.Dirty {
		r.Blocked = "dirty_subject"
		return r
	}
	r.Fresh = ev.Identity.SameInputs(want) && ev.Identity.SameInputs(*ev.EndIdentity) && ev.Identity.Subject == ev.EndIdentity.Subject
	if ev.Identity.Subject.Commit != want.Subject.Commit {
		if reuse && len(ev.Contracts) == 0 && r.Fresh {
			r.ReusedFrom = ev.RunID
		} else {
			r.Fresh = false
		}
	}
	return r
}
