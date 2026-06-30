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

// cmdCI is the CI-friendly verification command. It runs the same
// verify.commands as `lf verify`, but unlike the local flow it does NOT care
// about pre-existing fresh evidence: in CI the current checkout is the source of
// truth, so it simply runs the commands and saves evidence (which CI can upload
// as an artifact). It exits non-zero if any required command fails.
func cmdCI(args []string) error {
	fs := flag.NewFlagSet("ci", flag.ExitOnError)
	continueOnFailure := fs.Bool("continue-on-failure", false, "run all commands even after a failure")
	keepGoing := fs.Bool("keep-going", false, "alias of --continue-on-failure")
	quiet := fs.Bool("quiet", false, "do not stream command output to the terminal")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf ci [--continue-on-failure] [--quiet]\n\n"+
			"Runs verify commands against the current checkout and saves evidence.\n"+
			"Intended for CI: it does not require pre-existing fresh local evidence.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	keepRunning := *continueOnFailure || *keepGoing

	l, err := load()
	if err != nil {
		return err
	}

	inActions := os.Getenv("GITHUB_ACTIONS") == "true"

	fmt.Println("LunarForge CI")
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
	if len(ev.Commands) < len(configured) && !keepRunning {
		for _, c := range configured[len(ev.Commands):] {
			fmt.Printf("⏭️  %s skipped\n", c.ID)
		}
	}

	fmt.Println()
	fmt.Println("Result:")
	if ev.Passed() {
		fmt.Println("✅ CI verification passed")
	} else {
		fmt.Println("❌ CI verification failed")
	}

	evidenceRel := relPath(l.repoDir, filepath.Join(res.RunDir, "evidence.json"))

	if !ev.Passed() {
		if failed := firstFailed(ev); failed != nil {
			stdoutRel := relPath(l.repoDir, filepath.Join(res.RunDir, failed.StdoutPath))
			stderrRel := relPath(l.repoDir, filepath.Join(res.RunDir, failed.StderrPath))
			fmt.Println()
			fmt.Println("Failed command:")
			fmt.Printf("%s\n", failed.Run)
			fmt.Println()
			fmt.Println("Logs:")
			fmt.Printf("%s\n", stdoutRel)
			fmt.Printf("%s\n", stderrRel)
			// GitHub Actions annotation so the failure surfaces on the PR/run.
			if inActions {
				fmt.Printf("::error title=LunarForge CI::%s failed (%s)\n", failed.ID, failed.Run)
			}
		}
	}

	fmt.Println()
	fmt.Println("Evidence:")
	fmt.Printf("%s\n", evidenceRel)

	// Mirror a short result into the GitHub Actions job summary when available.
	writeStepSummary(ev, evidenceRel)

	if !ev.Passed() {
		return &exitError{code: 1}
	}
	return nil
}

// writeStepSummary appends a one-line CI result to the GitHub Actions job
// summary file when running under Actions. It is best-effort and never fails the
// command.
func writeStepSummary(ev *evidence.Evidence, evidenceRel string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	status := "✅ CI verification passed"
	if !ev.Passed() {
		status = "❌ CI verification failed"
	}
	fmt.Fprintf(f, "## LunarForge CI\n\n%s\n\n", status)
	fmt.Fprintf(f, "| Command | Result | Duration |\n|---|---|---:|\n")
	for _, c := range ev.Commands {
		mark := "✅ passed"
		if c.Result != evidence.ResultPassed {
			mark = "❌ failed"
		}
		fmt.Fprintf(f, "| %s | %s | %s |\n", c.ID, mark, fmtDuration(c.DurationMs))
	}
	fmt.Fprintf(f, "\nEvidence: `%s`\n", evidenceRel)
}
