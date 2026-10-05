// Package runner executes the configured verify commands, streams their output
// to the terminal, captures stdout/stderr to per-command files, and assembles
// the evidence record for the run.
package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

// Options controls a verify run.
type Options struct {
	// RepoDir is the repository root (working directory for commands).
	RepoDir string
	// Commit selects isolated, clean HEAD verification; empty preserves diff mode.
	Commit string
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
	ev, err := captureRun(cfg, opts)
	if err != nil {
		return nil, err
	}

	var identity *evidence.Identity
	sourceDir := opts.RepoDir
	if opts.Commit != "" {
		var cleanup func()
		identity, opts.RepoDir, cleanup, err = prepareCommit(cfg, sourceDir, opts.Commit)
		if err != nil {
			return nil, err
		}
		defer cleanup()
	}

	ev.RunID = evidence.NewRunID(ev.StartedAt)
	runDir := evidence.RunDir(opts.EvidenceDir, ev.RunID)
	cmdDir, err := createRunDir(opts.EvidenceDir, runDir)
	if err != nil {
		return nil, err
	}

	if identity != nil {
		ev.Git.Head = identity.Subject.Commit
		ev.Git.StatusPorcelain = ""
		ev.Mode = "commit"
		ev.Identity = identity
		ev.ExecutionDir = opts.RepoDir
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

	if identity != nil && !verifySubject(cfg, sourceDir, opts.RepoDir, ev) {
		overall = evidence.ResultFailed
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

func createRunDir(evidenceDir, runDir string) (string, error) {
	cmdDir := filepath.Join(runDir, "commands")
	if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
		return "", err
	}
	if err := os.Mkdir(runDir, 0o700); err != nil {
		return "", err
	}
	if err := os.Mkdir(cmdDir, 0o700); err != nil {
		return "", fmt.Errorf("creating run dir: %w", err)
	}
	return cmdDir, nil
}

func captureRun(cfg *config.Config, opts Options) (*evidence.Evidence, error) {
	start := opts.Now
	if start.IsZero() {
		start = time.Now()
	}
	start = start.UTC()

	gitInfo, err := gitutil.Snapshot(opts.RepoDir)
	if err != nil {
		return nil, err
	}
	excludes := evidence.ArtifactExcludes(opts.RepoDir, opts.EvidenceDir)
	diffHash, err := gitutil.DiffHash(opts.RepoDir, excludes...)
	if err != nil {
		return nil, err
	}

	return &evidence.Evidence{
		Version:   evidence.SchemaVersion,
		Project:   cfg.Project.Name,
		StartedAt: start,
		DiffHash:  diffHash,
		Git: evidence.Git{
			Branch:          gitInfo.Branch,
			Head:            gitInfo.Head,
			StatusPorcelain: gitInfo.StatusPorcelain,
		},
	}, nil
}
