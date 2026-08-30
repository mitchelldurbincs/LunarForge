// Package runner executes repository checks and produces durable evidence.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

type Options struct {
	Context     context.Context
	RepoDir     string
	EvidenceDir string
	Now         time.Time
	KeepGoing   bool
	Stream      io.Writer
}

type Result struct {
	Evidence *evidence.Evidence
	RunDir   string
}

func Run(cfg *config.Config, opts Options) (*Result, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
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
	initialHash, err := gitutil.DiffHash(opts.RepoDir, excludes...)
	if err != nil {
		return nil, err
	}

	runID := evidence.NewRunID(start)
	runDir := evidence.RunDir(opts.EvidenceDir, runID)
	cmdDir := filepath.Join(runDir, "checks")
	if err := os.MkdirAll(opts.EvidenceDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating evidence dir: %w", err)
	}
	if err := os.Mkdir(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating run dir: %w", err)
	}
	if err := os.Mkdir(cmdDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating checks dir: %w", err)
	}

	ev := &evidence.Evidence{
		Version:       evidence.SchemaVersion,
		RunID:         runID,
		StartedAt:     start,
		Result:        evidence.ResultPassed,
		Reason:        evidence.ReasonChecksPassed,
		DiffHash:      initialHash,
		FinalDiffHash: initialHash,
		Git: evidence.Git{
			Branch:          gitInfo.Branch,
			Head:            gitInfo.Head,
			Dirty:           gitInfo.Dirty,
			StatusPorcelain: gitInfo.StatusPorcelain,
		},
	}

	stopReason := ""
	for _, c := range cfg.Verify.Commands {
		if stopReason != "" {
			ev.Commands = append(ev.Commands, skipped(c, stopReason))
			continue
		}
		rec, err := runOne(ctx, opts, cmdDir, c)
		if err != nil {
			return nil, err
		}
		ev.Commands = append(ev.Commands, rec)
		switch rec.Result {
		case evidence.ResultBlocked:
			ev.Result = evidence.ResultBlocked
			ev.Reason = rec.Reason
			if !opts.KeepGoing {
				stopReason = evidence.ReasonPreviousCheckBlocked
			}
		case evidence.ResultFailed:
			if ev.Result != evidence.ResultBlocked {
				ev.Result = evidence.ResultFailed
				ev.Reason = evidence.ReasonCheckFailed
			}
			if !opts.KeepGoing {
				stopReason = evidence.ReasonPreviousCheckFailed
			}
		}
	}

	finalHash, err := gitutil.DiffHash(opts.RepoDir, excludes...)
	if err != nil {
		return nil, err
	}
	ev.FinalDiffHash = finalHash
	if finalHash != initialHash {
		ev.Result = evidence.ResultStale
		ev.Reason = evidence.ReasonSnapshotChanged
	}
	ev.FinishedAt = time.Now().UTC()

	if err := writeSummary(runDir, ev); err != nil {
		return nil, err
	}
	if err := evidence.Write(opts.EvidenceDir, runDir, ev); err != nil {
		return nil, err
	}
	return &Result{Evidence: ev, RunDir: runDir}, nil
}

func runOne(parent context.Context, opts Options, cmdDir string, c config.Command) (evidence.Command, error) {
	started := time.Now().UTC()
	timeout := c.Timeout()
	rec := evidence.Command{
		ID:         c.ID,
		Run:        c.Run,
		StartedAt:  started,
		TimeoutMs:  timeout.Milliseconds(),
		ExitCode:   -1,
		StdoutPath: filepath.Join("checks", c.ID+".stdout.txt"),
		StderrPath: filepath.Join("checks", c.ID+".stderr.txt"),
	}

	stdoutFile, err := os.Create(filepath.Join(cmdDir, c.ID+".stdout.txt"))
	if err != nil {
		return rec, fmt.Errorf("creating stdout log for %s: %w", c.ID, err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(cmdDir, c.ID+".stderr.txt"))
	if err != nil {
		return rec, fmt.Errorf("creating stderr log for %s: %w", c.ID, err)
	}
	defer stderrFile.Close()

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := shellCommandContext(ctx, c.Run)
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
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		rec.Result = evidence.ResultBlocked
		rec.Reason = evidence.ReasonTimedOut
		rec.Error = fmt.Sprintf("check exceeded %s timeout", timeout)
	case runErr == nil:
		rec.Result = evidence.ResultPassed
		rec.Reason = evidence.ReasonChecksPassed
	case rec.ExitCode == 127 || rec.ExitCode == 9009:
		rec.Result = evidence.ResultBlocked
		rec.Reason = evidence.ReasonToolUnavailable
		rec.Error = runErr.Error()
	case isStartError(runErr):
		rec.Result = evidence.ResultBlocked
		rec.Reason = evidence.ReasonToolUnavailable
		rec.Error = runErr.Error()
	default:
		rec.Result = evidence.ResultFailed
		rec.Reason = evidence.ReasonCheckFailed
		rec.Error = runErr.Error()
	}
	return rec, nil
}

func skipped(c config.Command, reason string) evidence.Command {
	return evidence.Command{
		ID:        c.ID,
		Run:       c.Run,
		TimeoutMs: c.Timeout().Milliseconds(),
		ExitCode:  -1,
		Result:    evidence.ResultSkipped,
		Reason:    reason,
	}
}

func shellCommandContext(ctx context.Context, run string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", run)
	}
	return exec.CommandContext(ctx, "sh", "-c", run)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func isStartError(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	return !errors.As(err, &exitErr) && !strings.Contains(err.Error(), "signal: killed")
}
