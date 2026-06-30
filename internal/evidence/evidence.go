// Package evidence defines the on-disk record produced by `lf verify` and the
// helpers used to write and read it. Each verify run gets its own directory
// under the configured evidence dir, plus a pointer file (.lf/latest) naming
// the most recent run.
package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion is the version of the evidence.json shape.
const SchemaVersion = 1

// Result values.
const (
	ResultPassed = "passed"
	ResultFailed = "failed"
)

// Evidence is the top-level evidence.json document.
type Evidence struct {
	Version    int       `json:"version"`
	Project    string    `json:"project"`
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Result     string    `json:"result"`
	DiffHash   string    `json:"diff_hash"`
	Git        Git       `json:"git"`
	Commands   []Command `json:"commands"`
}

// Git is the captured repository state at verify time.
type Git struct {
	Branch          string `json:"branch"`
	Head            string `json:"head"`
	StatusPorcelain string `json:"status_porcelain"`
}

// Command is the record of a single verify command.
type Command struct {
	ID         string    `json:"id"`
	Run        string    `json:"run"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMs int64     `json:"duration_ms"`
	ExitCode   int       `json:"exit_code"`
	StdoutPath string    `json:"stdout_path"`
	StderrPath string    `json:"stderr_path"`
	Result     string    `json:"result"`
}

// Passed reports whether the overall run passed.
func (e *Evidence) Passed() bool { return e.Result == ResultPassed }

// NewRunID returns a filesystem-safe run id based on the given UTC time, e.g.
// "2026-06-30T14-22-10".
func NewRunID(t time.Time) string {
	return t.UTC().Format("2006-01-02T15-04-05")
}

// RunDir returns the directory for a given run id under the evidence dir.
func RunDir(evidenceDir, runID string) string {
	return filepath.Join(evidenceDir, runID)
}

// latestPointer returns the path of the .lf/latest pointer file. It is placed
// in the parent of the evidence dir (i.e. .lf/latest alongside .lf/runs).
func latestPointer(evidenceDir string) string {
	return filepath.Join(filepath.Dir(evidenceDir), "latest")
}

// Write serializes the evidence to <runDir>/evidence.json and updates the
// latest pointer. The run directory must already exist.
func Write(evidenceDir, runDir string, e *Evidence) error {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling evidence: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "evidence.json"), data, 0o644); err != nil {
		return fmt.Errorf("writing evidence.json: %w", err)
	}
	if err := os.WriteFile(latestPointer(evidenceDir), []byte(e.RunID+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing latest pointer: %w", err)
	}
	return nil
}

// Load reads the evidence.json from a specific run directory.
func Load(runDir string) (*Evidence, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		return nil, err
	}
	var e Evidence
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parsing evidence.json: %w", err)
	}
	return &e, nil
}

// LatestRunID reads the run id stored in the latest pointer. It falls back to
// scanning the evidence dir for the newest run directory if the pointer is
// missing.
func LatestRunID(evidenceDir string) (string, error) {
	data, err := os.ReadFile(latestPointer(evidenceDir))
	if err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}
	// Fallback: scan for the newest directory name (run ids sort lexically by
	// time because of the fixed-width timestamp format).
	entries, derr := os.ReadDir(evidenceDir)
	if derr != nil {
		return "", fmt.Errorf("no evidence found (run `lf verify`)")
	}
	var newest string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() > newest {
			newest = entry.Name()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no evidence found (run `lf verify`)")
	}
	return newest, nil
}

// LoadLatest loads the most recent evidence record and returns it together with
// its run directory.
func LoadLatest(evidenceDir string) (*Evidence, string, error) {
	id, err := LatestRunID(evidenceDir)
	if err != nil {
		return nil, "", err
	}
	runDir := RunDir(evidenceDir, id)
	e, err := Load(runDir)
	if err != nil {
		return nil, "", err
	}
	return e, runDir, nil
}
