// Package runner executes the configured verify commands, streams their output
// to the terminal, captures stdout/stderr to per-command files, and assembles
// the evidence record for the run.
package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

// Options controls a verify run.
type Options struct {
	// RepoDir is the repository root (working directory for commands).
	RepoDir string
	// EvidenceDir is the resolved absolute evidence directory.
	EvidenceDir string
	// Now is the run's start time (UTC recommended). Injected for testability.
	Now time.Time
	// KeepGoing runs all commands even after a failure. Default behavior stops
	// on the first failure.
	KeepGoing bool
	// Stream, when set, mirrors command output to the terminal as it runs.
	Stream io.Writer
}

// Result bundles the produced evidence and where it was written.
type Result struct {
	Evidence *evidence.Evidence
	RunDir   string
}

// Run executes the verify commands described by cfg and writes evidence.
func Run(cfg *config.Config, opts Options) (*Result, error) {
	start := opts.Now
	if start.IsZero() {
		start = time.Now()
	}
	start = start.UTC()

	gitInfo, err := gitutil.Snapshot(opts.RepoDir)
	if err != nil {
		return nil, err
	}
	diffHash, err := gitutil.DiffHash(opts.RepoDir)
	if err != nil {
		return nil, err
	}

	runID := evidence.NewRunID(start)
	runDir := evidence.RunDir(opts.EvidenceDir, runID)
	cmdDir := filepath.Join(runDir, "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating run dir: %w", err)
	}

	ev := &evidence.Evidence{
		Version:   evidence.SchemaVersion,
		Project:   cfg.Project.Name,
		RunID:     runID,
		StartedAt: start,
		DiffHash:  diffHash,
		Git: evidence.Git{
			Branch:          gitInfo.Branch,
			Head:            gitInfo.Head,
			StatusPorcelain: gitInfo.StatusPorcelain,
		},
	}

	overall := evidence.ResultPassed
	for _, c := range cfg.Verify.Commands {
		rec := runOne(opts, cmdDir, c)
		ev.Commands = append(ev.Commands, rec)
		if rec.Result != evidence.ResultPassed {
			overall = evidence.ResultFailed
			if !opts.KeepGoing {
				break
			}
		}
	}

	ev.FinishedAt = time.Now().UTC()
	ev.Result = overall

	if err := writeSummary(runDir, ev); err != nil {
		return nil, err
	}
	if err := evidence.Write(opts.EvidenceDir, runDir, ev); err != nil {
		return nil, err
	}
	return &Result{Evidence: ev, RunDir: runDir}, nil
}

func runOne(opts Options, cmdDir string, c config.Command) evidence.Command {
	started := time.Now().UTC()
	rec := evidence.Command{
		ID:         c.ID,
		Run:        c.Run,
		StartedAt:  started,
		StdoutPath: filepath.Join("commands", c.ID+".stdout.txt"),
		StderrPath: filepath.Join("commands", c.ID+".stderr.txt"),
	}

	stdoutFile, err := os.Create(filepath.Join(cmdDir, c.ID+".stdout.txt"))
	if err != nil {
		return failRecord(rec, started, -1, fmt.Sprintf("could not create stdout file: %v", err))
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(cmdDir, c.ID+".stderr.txt"))
	if err != nil {
		return failRecord(rec, started, -1, fmt.Sprintf("could not create stderr file: %v", err))
	}
	defer stderrFile.Close()

	cmd := shellCommand(c.Run)
	cmd.Dir = opts.RepoDir
	if opts.Stream != nil {
		cmd.Stdout = io.MultiWriter(stdoutFile, opts.Stream)
		cmd.Stderr = io.MultiWriter(stderrFile, opts.Stream)
	} else {
		cmd.Stdout = stdoutFile
		cmd.Stderr = stderrFile
	}

	runErr := cmd.Run()
	finished := time.Now().UTC()
	rec.FinishedAt = finished
	rec.DurationMs = finished.Sub(started).Milliseconds()
	rec.ExitCode = exitCode(runErr)
	if runErr == nil {
		rec.Result = evidence.ResultPassed
	} else {
		rec.Result = evidence.ResultFailed
	}
	return rec
}

func failRecord(rec evidence.Command, started time.Time, code int, msg string) evidence.Command {
	finished := time.Now().UTC()
	rec.FinishedAt = finished
	rec.DurationMs = finished.Sub(started).Milliseconds()
	rec.ExitCode = code
	rec.Result = evidence.ResultFailed
	_ = msg
	return rec
}

// shellCommand wraps a command string in the platform shell so that constructs
// like "npm run lint" or "./scripts/verify.sh" work as written in config.
func shellCommand(run string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", run)
	}
	return exec.Command("sh", "-c", run)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func asExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}
