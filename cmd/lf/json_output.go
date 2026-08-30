package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

const resultSchemaVersion = 1

type JSONResult struct {
	SchemaVersion int            `json:"schema_version"`
	Command       string         `json:"command"`
	State         string         `json:"state"`
	Reason        string         `json:"reason"`
	Repository    JSONRepository `json:"repository"`
	Run           *JSONRun       `json:"run,omitempty"`
	Checks        []JSONCheck    `json:"checks,omitempty"`
	Error         *JSONError     `json:"error,omitempty"`
}

type JSONRepository struct {
	Root        string `json:"root,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Head        string `json:"head,omitempty"`
	Dirty       bool   `json:"dirty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type JSONRun struct {
	ID               string    `json:"id"`
	EvidencePath     string    `json:"evidence_path"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
	State            string    `json:"state"`
	Reason           string    `json:"reason"`
	Branch           string    `json:"branch"`
	Head             string    `json:"head"`
	Dirty            bool      `json:"dirty"`
	Fingerprint      string    `json:"fingerprint"`
	FinalFingerprint string    `json:"final_fingerprint"`
}

type JSONCheck struct {
	ID         string `json:"id"`
	Run        string `json:"run"`
	State      string `json:"state"`
	Reason     string `json:"reason"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	TimeoutMs  int64  `json:"timeout_ms"`
	StdoutPath string `json:"stdout_path,omitempty"`
	StderrPath string `json:"stderr_path,omitempty"`
	StdoutTail string `json:"stdout_tail,omitempty"`
	StderrTail string `json:"stderr_tail,omitempty"`
	Error      string `json:"error,omitempty"`
}

type JSONError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func jsonFromEvidence(command string, l *loaded, ev *evidence.Evidence, runDir, currentHash, state, reason string) JSONResult {
	if state == "" {
		state = ev.Result
	}
	if reason == "" {
		reason = ev.Reason
	}
	currentGit := ev.Git
	if info, err := gitutil.Snapshot(l.repoDir); err == nil {
		currentGit = evidence.Git{Branch: info.Branch, Head: info.Head, Dirty: info.Dirty}
	}
	out := JSONResult{
		SchemaVersion: resultSchemaVersion,
		Command:       command,
		State:         state,
		Reason:        reason,
		Repository: JSONRepository{
			Root:        l.repoDir,
			Branch:      currentGit.Branch,
			Head:        currentGit.Head,
			Dirty:       currentGit.Dirty,
			Fingerprint: currentHash,
		},
		Run: &JSONRun{
			ID:               ev.RunID,
			EvidencePath:     relPath(l.repoDir, filepath.Join(runDir, "evidence.json")),
			StartedAt:        ev.StartedAt,
			FinishedAt:       ev.FinishedAt,
			State:            ev.Result,
			Reason:           ev.Reason,
			Branch:           ev.Git.Branch,
			Head:             ev.Git.Head,
			Dirty:            ev.Git.Dirty,
			Fingerprint:      ev.DiffHash,
			FinalFingerprint: ev.FinalDiffHash,
		},
	}
	for _, c := range ev.Commands {
		jc := JSONCheck{
			ID:         c.ID,
			Run:        c.Run,
			State:      c.Result,
			Reason:     c.Reason,
			ExitCode:   c.ExitCode,
			DurationMs: c.DurationMs,
			TimeoutMs:  c.TimeoutMs,
			Error:      c.Error,
		}
		if c.StdoutPath != "" {
			stdout := filepath.Join(runDir, c.StdoutPath)
			stderr := filepath.Join(runDir, c.StderrPath)
			jc.StdoutPath = relPath(l.repoDir, stdout)
			jc.StderrPath = relPath(l.repoDir, stderr)
			if c.Result == evidence.ResultFailed || c.Result == evidence.ResultBlocked {
				jc.StdoutTail = readTail(stdout, 4000)
				jc.StderrTail = readTail(stderr, 4000)
			}
		}
		out.Checks = append(out.Checks, jc)
	}
	return out
}

func jsonFailure(command, state, reason string, err error) JSONResult {
	root, _ := os.Getwd()
	return JSONResult{
		SchemaVersion: resultSchemaVersion,
		Command:       command,
		State:         state,
		Reason:        reason,
		Repository:    JSONRepository{Root: root},
		Error:         &JSONError{Code: reason, Message: err.Error()},
	}
}

func writeJSON(out JSONResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func readTail(path string, limit int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > limit {
		data = data[len(data)-limit:]
	}
	return strings.ToValidUTF8(string(data), "�")
}

func exitCodeForState(state string) int {
	switch state {
	case evidence.ResultPassed:
		return 0
	case evidence.ResultFailed, evidence.ResultStale:
		return 1
	case evidence.ResultBlocked:
		return 2
	default:
		return 3
	}
}
