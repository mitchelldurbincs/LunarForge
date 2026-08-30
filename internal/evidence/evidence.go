// Package evidence defines and persists LunarForge's stable verification record.
package evidence

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

const (
	SchemaVersion = 1
	DefaultDir    = ".lf/runs"

	ResultPassed  = "pass"
	ResultFailed  = "fail"
	ResultStale   = "stale"
	ResultBlocked = "blocked"
	ResultSkipped = "skipped"
	ResultError   = "error"

	ReasonChecksPassed         = "checks_passed"
	ReasonCheckFailed          = "check_failed"
	ReasonSnapshotChanged      = "snapshot_changed"
	ReasonNoEvidence           = "no_evidence"
	ReasonConfigInvalid        = "config_invalid"
	ReasonToolUnavailable      = "tool_unavailable"
	ReasonTimedOut             = "timed_out"
	ReasonEvidenceCorrupt      = "evidence_corrupt"
	ReasonPreviousCheckFailed  = "previous_check_failed"
	ReasonPreviousCheckBlocked = "previous_check_blocked"
	ReasonInternalError        = "internal_error"
)

var (
	ErrNoEvidence  = errors.New("no LunarForge evidence")
	runCounter     atomic.Uint64
	checkIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// Evidence is both the durable run record and the source for JSON CLI output.
// Large command streams remain in per-check files.
type Evidence struct {
	Version       int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	Result        string    `json:"state"`
	Reason        string    `json:"reason"`
	DiffHash      string    `json:"fingerprint"`
	FinalDiffHash string    `json:"final_fingerprint"`
	Git           Git       `json:"git"`
	Commands      []Command `json:"checks"`
}

type Git struct {
	Branch          string `json:"branch"`
	Head            string `json:"head"`
	Dirty           bool   `json:"dirty"`
	StatusPorcelain string `json:"status_porcelain"`
}

type Command struct {
	ID         string    `json:"id"`
	Run        string    `json:"run"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	TimeoutMs  int64     `json:"timeout_ms"`
	ExitCode   int       `json:"exit_code"`
	StdoutPath string    `json:"stdout_path,omitempty"`
	StderrPath string    `json:"stderr_path,omitempty"`
	Result     string    `json:"state"`
	Reason     string    `json:"reason"`
	Error      string    `json:"error,omitempty"`
}

func (e *Evidence) Passed() bool { return e.Result == ResultPassed }

// NewRunID combines a sortable nanosecond timestamp with process-independent
// randomness so rapid and concurrent runs cannot overwrite one another.
func NewRunID(t time.Time) string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		fallback := runCounter.Add(1)
		return fmt.Sprintf("%s-%08x", t.UTC().Format("2006-01-02T15-04-05.000000000Z"), fallback)
	}
	return t.UTC().Format("2006-01-02T15-04-05.000000000Z") + "-" + hex.EncodeToString(suffix[:])
}

func ArtifactExcludes(repoDir, evidenceDir string) []string {
	for _, candidate := range []string{filepath.Dir(evidenceDir), evidenceDir} {
		rel, err := filepath.Rel(repoDir, candidate)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || rel == ".." {
			continue
		}
		return []string{rel}
	}
	return nil
}

func RunDir(evidenceDir, runID string) string { return filepath.Join(evidenceDir, runID) }

func latestPointer(evidenceDir string) string {
	return filepath.Join(filepath.Dir(evidenceDir), "latest")
}

func Write(evidenceDir, runDir string, e *Evidence) error {
	if err := validate(e); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling evidence: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(filepath.Join(runDir, "evidence.json"), data, 0o644); err != nil {
		return fmt.Errorf("writing evidence.json: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(latestPointer(evidenceDir)), 0o755); err != nil {
		return fmt.Errorf("creating evidence metadata dir: %w", err)
	}
	if err := writeAtomic(latestPointer(evidenceDir), []byte(e.RunID+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing latest pointer: %w", err)
	}
	return nil
}

func Load(runDir string) (*Evidence, error) {
	path := filepath.Join(runDir, "evidence.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e Evidence
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parsing evidence.json: %w", err)
	}
	if err := validate(&e); err != nil {
		return nil, err
	}
	for _, c := range e.Commands {
		for _, rel := range []string{c.StdoutPath, c.StderrPath} {
			if rel == "" {
				continue
			}
			info, err := os.Lstat(filepath.Join(runDir, filepath.FromSlash(rel)))
			if err != nil {
				return nil, fmt.Errorf("checking evidence log %s: %w", rel, err)
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("evidence log %s is not a regular file", rel)
			}
		}
	}
	return &e, nil
}

func LatestRunID(evidenceDir string) (string, error) {
	data, err := os.ReadFile(latestPointer(evidenceDir))
	if err == nil {
		id := strings.TrimSpace(string(data))
		if validRunID(id) {
			return id, nil
		}
		return "", fmt.Errorf("invalid latest evidence pointer %q", id)
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading latest evidence pointer: %w", err)
	}

	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoEvidence
		}
		return "", err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.IsDir() && validRunID(entry.Name()) {
			if _, err := os.Stat(filepath.Join(evidenceDir, entry.Name(), "evidence.json")); err == nil {
				return entry.Name(), nil
			}
		}
	}
	return "", ErrNoEvidence
}

func LoadLatest(evidenceDir string) (*Evidence, string, error) {
	id, err := LatestRunID(evidenceDir)
	if err != nil {
		return nil, "", err
	}
	runDir := RunDir(evidenceDir, id)
	e, err := Load(runDir)
	if err != nil {
		return nil, "", fmt.Errorf("loading latest evidence %s: %w", id, err)
	}
	return e, runDir, nil
}

func validate(e *Evidence) error {
	if e.Version != SchemaVersion {
		return fmt.Errorf("unsupported evidence schema version %d (expected %d)", e.Version, SchemaVersion)
	}
	if !validRunID(e.RunID) {
		return fmt.Errorf("invalid evidence run id %q", e.RunID)
	}
	if !validResult(e.Result, false) {
		return fmt.Errorf("invalid evidence state %q", e.Result)
	}
	if !validStateReason(e.Result, e.Reason, false) {
		return fmt.Errorf("invalid evidence state/reason %q/%q", e.Result, e.Reason)
	}
	if e.DiffHash == "" || e.FinalDiffHash == "" {
		return fmt.Errorf("evidence fingerprints are required")
	}
	if len(e.Commands) == 0 {
		return fmt.Errorf("evidence must contain at least one check")
	}
	hasFailed, hasBlocked, allPassed := false, false, true
	for _, c := range e.Commands {
		if !checkIDPattern.MatchString(c.ID) {
			return fmt.Errorf("invalid evidence check id %q", c.ID)
		}
		if !validResult(c.Result, true) {
			return fmt.Errorf("check %q has invalid state %q", c.ID, c.Result)
		}
		if !validStateReason(c.Result, c.Reason, true) {
			return fmt.Errorf("check %q has invalid state/reason %q/%q", c.ID, c.Result, c.Reason)
		}
		hasFailed = hasFailed || c.Result == ResultFailed
		hasBlocked = hasBlocked || c.Result == ResultBlocked
		allPassed = allPassed && c.Result == ResultPassed
		if c.Result == ResultSkipped {
			if c.StdoutPath != "" || c.StderrPath != "" {
				return fmt.Errorf("skipped check %q must not have log paths", c.ID)
			}
			continue
		}
		wantStdout := filepath.ToSlash(filepath.Join("checks", c.ID+".stdout.txt"))
		wantStderr := filepath.ToSlash(filepath.Join("checks", c.ID+".stderr.txt"))
		if filepath.ToSlash(c.StdoutPath) != wantStdout || filepath.ToSlash(c.StderrPath) != wantStderr {
			return fmt.Errorf("check %q has invalid log paths", c.ID)
		}
	}
	switch e.Result {
	case ResultPassed:
		if !allPassed {
			return fmt.Errorf("passing evidence contains a non-passing check")
		}
	case ResultFailed:
		if !hasFailed || hasBlocked {
			return fmt.Errorf("failed evidence must contain a failure and no blocked check")
		}
	case ResultBlocked:
		if !hasBlocked {
			return fmt.Errorf("blocked evidence has no blocked check")
		}
	}
	return nil
}

func validResult(result string, allowSkipped bool) bool {
	switch result {
	case ResultPassed, ResultFailed, ResultStale, ResultBlocked, ResultError:
		return true
	case ResultSkipped:
		return allowSkipped
	default:
		return false
	}
}

func validReason(reason string) bool {
	switch reason {
	case ReasonChecksPassed, ReasonCheckFailed, ReasonSnapshotChanged,
		ReasonNoEvidence, ReasonConfigInvalid, ReasonToolUnavailable,
		ReasonTimedOut, ReasonEvidenceCorrupt, ReasonPreviousCheckFailed,
		ReasonPreviousCheckBlocked, ReasonInternalError:
		return true
	default:
		return false
	}
}

func validStateReason(state, reason string, check bool) bool {
	if !validReason(reason) {
		return false
	}
	switch state {
	case ResultPassed:
		return reason == ReasonChecksPassed
	case ResultFailed:
		return reason == ReasonCheckFailed
	case ResultStale:
		return !check && reason == ReasonSnapshotChanged
	case ResultBlocked:
		return reason == ReasonToolUnavailable || reason == ReasonTimedOut
	case ResultSkipped:
		return check && (reason == ReasonPreviousCheckFailed || reason == ReasonPreviousCheckBlocked)
	case ResultError:
		return !check && (reason == ReasonInternalError || reason == ReasonEvidenceCorrupt)
	default:
		return false
	}
}

func validRunID(id string) bool {
	return id != "" && id != "." && id != ".." && id == filepath.Base(id) && !strings.ContainsAny(id, `/\\`)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".lf-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows does not replace an existing destination with Rename. Keep this
		// compatibility fallback localized; POSIX takes the atomic path above.
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if retryErr := os.Rename(tmp, path); retryErr != nil {
			return retryErr
		}
	}
	return nil
}
