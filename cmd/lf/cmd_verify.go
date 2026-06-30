package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/runner"
)

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	continueOnFailure := fs.Bool("continue-on-failure", false, "run all commands even after a failure")
	keepGoing := fs.Bool("keep-going", false, "alias of --continue-on-failure")
	quiet := fs.Bool("quiet", false, "do not stream command output to the terminal")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf verify [--continue-on-failure] [--quiet]\n\nRuns verify commands and saves evidence tied to the current diff.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	keepRunning := *continueOnFailure || *keepGoing

	l, err := load()
	if err != nil {
		return err
	}

	fmt.Println("LunarForge verify")
	fmt.Println()

	opts := runner.Options{
		RepoDir:     l.repoDir,
		EvidenceDir: l.evidenceDir,
		Now:         time.Now(),
		KeepGoing:   keepRunning,
	}
	if !*quiet {
		opts.Stream = streamWriter()
	}

	res, err := runner.Run(l.cfg, opts)
	if err != nil {
		return err
	}
	ev := res.Evidence

	// Per-command pass/fail lines with durations.
	configured := l.cfg.Verify.Commands
	for _, c := range ev.Commands {
		if c.Result == evidence.ResultPassed {
			fmt.Printf("✅ %s passed\t%s\n", c.ID, fmtDuration(c.DurationMs))
		} else {
			fmt.Printf("❌ %s failed\t%s\n", c.ID, fmtDuration(c.DurationMs))
		}
	}
	// Note commands that were skipped because we stopped on first failure.
	if len(ev.Commands) < len(configured) && !keepRunning {
		for _, c := range configured[len(ev.Commands):] {
			fmt.Printf("⏭️  %s skipped\n", c.ID)
		}
	}

	fmt.Println()
	fmt.Println("Result:")
	if ev.Passed() {
		fmt.Println("✅ ready locally")
	} else {
		fmt.Println("❌ not ready")
	}

	// On failure, point directly at the failing command and its logs.
	if !ev.Passed() {
		if failed := firstFailed(ev); failed != nil {
			fmt.Println()
			fmt.Println("Failed command:")
			fmt.Printf("%s\n", failed.Run)
			fmt.Println()
			fmt.Println("Logs:")
			fmt.Printf("%s\n", relPath(l.repoDir, filepath.Join(res.RunDir, failed.StdoutPath)))
			fmt.Printf("%s\n", relPath(l.repoDir, filepath.Join(res.RunDir, failed.StderrPath)))
		}
	}

	fmt.Println()
	fmt.Println("Evidence:")
	fmt.Printf("%s\n", relPath(l.repoDir, filepath.Join(res.RunDir, "evidence.json")))
	fmt.Println()
	fmt.Println("Diff:")
	fmt.Printf("%s\n", ev.DiffHash)

	if !ev.Passed() {
		// Non-zero exit so scripts and hooks can detect failure. Output already
		// printed, so suppress the default error line.
		return &exitError{code: 1}
	}
	return nil
}

func firstFailed(ev *evidence.Evidence) *evidence.Command {
	for i := range ev.Commands {
		if ev.Commands[i].Result != evidence.ResultPassed {
			return &ev.Commands[i]
		}
	}
	return nil
}

// fmtDuration renders a millisecond duration as a friendly seconds value, e.g.
// "1.2s". Sub-100ms durations show milliseconds to stay informative.
func fmtDuration(ms int64) string {
	if ms < 100 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}

func relPath(base, target string) string {
	if r, err := filepath.Rel(base, target); err == nil {
		return r
	}
	return target
}

// streamWriter returns stdout for mirroring command output.
func streamWriter() *os.File { return os.Stdout }
