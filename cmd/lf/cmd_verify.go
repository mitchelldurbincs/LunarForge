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

func cmdVerify(args []string) error { return runVerifyCommand("verify", args) }

// ci remains a compatibility alias. GitHub Actions integration is now automatic
// when GITHUB_ACTIONS/GITHUB_STEP_SUMMARY are present.
func cmdCI(args []string) error { return runVerifyCommand("ci", args) }

func runVerifyCommand(command string, args []string) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	continueOnFailure := fs.Bool("continue-on-failure", false, "run all checks even after a failure")
	keepGoing := fs.Bool("keep-going", false, "alias of --continue-on-failure")
	quiet := fs.Bool("quiet", false, "do not stream check output")
	asJSON := fs.Bool("json", false, "print the stable machine-readable result")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf %s [--json] [--continue-on-failure] [--quiet]\n\nRuns repository checks and writes snapshot-bound evidence.\n", command)
	}
	if err := fs.Parse(args); err != nil {
		return &exitError{code: 2, message: err.Error()}
	}

	l, err := load()
	if err != nil {
		if *asJSON {
			writeJSON(jsonFailure(command, evidence.ResultBlocked, evidence.ReasonConfigInvalid, err))
			return &exitError{code: 2}
		}
		return err
	}

	opts := runner.Options{
		RepoDir:     l.repoDir,
		EvidenceDir: l.evidenceDir,
		Now:         time.Now(),
		KeepGoing:   *continueOnFailure || *keepGoing,
	}
	if !*quiet {
		if *asJSON {
			opts.Stream = os.Stderr
		} else {
			opts.Stream = os.Stdout
		}
	}

	res, err := runner.Run(l.cfg, opts)
	if err != nil {
		if *asJSON {
			writeJSON(jsonFailure(command, evidence.ResultError, evidence.ReasonInternalError, err))
			return &exitError{code: 3}
		}
		return err
	}

	if *asJSON {
		writeJSON(jsonFromEvidence(command, l, res.Evidence, res.RunDir, res.Evidence.FinalDiffHash, "", ""))
	} else {
		printVerifyText(command, l, res)
	}
	writeStepSummary(res.Evidence, relPath(l.repoDir, filepath.Join(res.RunDir, "evidence.json")))
	if code := exitCodeForState(res.Evidence.Result); code != 0 {
		return &exitError{code: code}
	}
	return nil
}

func printVerifyText(command string, l *loaded, res *runner.Result) {
	title := "LunarForge verify"
	if command == "ci" {
		title = "LunarForge CI (verify alias)"
	}
	fmt.Println(title)
	fmt.Println()
	for _, c := range res.Evidence.Commands {
		switch c.Result {
		case evidence.ResultPassed:
			fmt.Printf("✅ %s passed\t%s\n", c.ID, fmtDuration(c.DurationMs))
		case evidence.ResultFailed:
			fmt.Printf("❌ %s failed\t%s\n", c.ID, fmtDuration(c.DurationMs))
		case evidence.ResultBlocked:
			fmt.Printf("⛔ %s blocked (%s)\t%s\n", c.ID, c.Reason, fmtDuration(c.DurationMs))
		case evidence.ResultSkipped:
			fmt.Printf("⏭️  %s skipped (%s)\n", c.ID, c.Reason)
		}
	}
	fmt.Println()
	fmt.Printf("State: %s\n", res.Evidence.Result)
	fmt.Printf("Reason: %s\n", res.Evidence.Reason)
	if res.Evidence.Git.Dirty {
		fmt.Println("Repository: dirty (verify again after committing the final change)")
	} else {
		fmt.Println("Repository: clean")
	}
	if failed := firstProblem(res.Evidence); failed != nil && failed.StdoutPath != "" {
		fmt.Println()
		fmt.Printf("Problem check: %s\n", failed.ID)
		fmt.Printf("stdout: %s\n", relPath(l.repoDir, filepath.Join(res.RunDir, failed.StdoutPath)))
		fmt.Printf("stderr: %s\n", relPath(l.repoDir, filepath.Join(res.RunDir, failed.StderrPath)))
	}
	fmt.Println()
	fmt.Printf("Evidence: %s\n", relPath(l.repoDir, filepath.Join(res.RunDir, "evidence.json")))
	fmt.Printf("Fingerprint: %s\n", res.Evidence.DiffHash)
}

func firstProblem(ev *evidence.Evidence) *evidence.Command {
	for i := range ev.Commands {
		if ev.Commands[i].Result == evidence.ResultFailed || ev.Commands[i].Result == evidence.ResultBlocked {
			return &ev.Commands[i]
		}
	}
	return nil
}

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
	fmt.Fprintf(f, "## LunarForge verification\n\nState: **%s** (`%s`)\n\n", ev.Result, ev.Reason)
	fmt.Fprintln(f, "| Check | State | Duration |")
	fmt.Fprintln(f, "|---|---|---:|")
	for _, c := range ev.Commands {
		fmt.Fprintf(f, "| %s | %s | %s |\n", c.ID, c.Result, fmtDuration(c.DurationMs))
	}
	fmt.Fprintf(f, "\nEvidence: `%s`\n", evidenceRel)
}
