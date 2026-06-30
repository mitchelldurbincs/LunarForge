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
	keepGoing := fs.Bool("keep-going", false, "run all commands even after a failure")
	quiet := fs.Bool("quiet", false, "do not stream command output to the terminal")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf verify [--keep-going] [--quiet]\n\nRuns verify commands and saves evidence tied to the current diff.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

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
		KeepGoing:   *keepGoing,
	}
	if !*quiet {
		opts.Stream = streamWriter()
	}

	res, err := runner.Run(l.cfg, opts)
	if err != nil {
		return err
	}
	ev := res.Evidence

	// Per-command pass/fail lines.
	configured := l.cfg.Verify.Commands
	for i, c := range ev.Commands {
		_ = i
		if c.Result == evidence.ResultPassed {
			fmt.Printf("✅ %s passed\n", c.ID)
		} else {
			fmt.Printf("❌ %s failed (exit %d)\n", c.ID, c.ExitCode)
		}
	}
	// Note commands that were skipped because we stopped on first failure.
	if len(ev.Commands) < len(configured) && !*keepGoing {
		for _, c := range configured[len(ev.Commands):] {
			fmt.Printf("⏭️  %s skipped\n", c.ID)
		}
	}

	fmt.Println()
	if !ev.Passed() {
		fmt.Println("Not ready.")
		fmt.Println()
	}

	rel := relPath(l.repoDir, filepath.Join(res.RunDir, "evidence.json"))
	fmt.Println("Evidence saved:")
	fmt.Printf("%s\n", rel)
	fmt.Println()
	fmt.Println("Diff hash:")
	fmt.Printf("%s\n", ev.DiffHash)

	if !ev.Passed() {
		// Non-zero exit so scripts and hooks can detect failure. Output already
		// printed, so suppress the default error line.
		return &exitError{code: 1}
	}
	return nil
}

func relPath(base, target string) string {
	if r, err := filepath.Rel(base, target); err == nil {
		return r
	}
	return target
}

// streamWriter returns stdout for mirroring command output.
func streamWriter() *os.File { return os.Stdout }
