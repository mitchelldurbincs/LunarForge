package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/explain"
	"github.com/mitchelldurbincs/lunarforge/internal/runner"
)

// cmdLoop chains the existing local commands into one repeatable sequence:
// verify → repair if needed → explain when verified. It composes the same
// internal code paths the standalone commands use; it never shells out to `lf`,
// and it never declares success on its own — only fresh, passing evidence does.
//
// It deliberately does NOT start new work, take a task description, or implement
// features. It only operates on the current working tree and .lunarforge.yml.
func cmdLoop(args []string) error {
	fs := flag.NewFlagSet("loop", flag.ExitOnError)
	noRepair := fs.Bool("no-repair", false, "do not run repair if verify fails (strict check only)")
	noExplain := fs.Bool("no-explain", false, "do not run explain even when verification passes")
	repairAttempts := fs.Int("repair-attempts", 0, "override repair max attempts for this loop")
	continueOnFailure := fs.Bool("continue-on-failure", false, "forward to verify: run all commands even after a failure")
	dryRun := fs.Bool("dry-run", false, "print the steps that would run without running verify, repair, or explain")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf loop [--no-repair] [--no-explain] [--repair-attempts N]\n"+
			"               [--continue-on-failure] [--dry-run]\n\n"+
			"Runs the standard local sequence: verify → repair if needed → explain when\n"+
			"verified. It does not start new work — it only checks and repairs the current\n"+
			"working tree. The agent never declares success; only LunarForge evidence does.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	l, err := load()
	if err != nil {
		return err
	}

	fmt.Println("LunarForge loop")
	fmt.Println()

	if *dryRun {
		return l.loopDryRun(*noRepair, *noExplain, *repairAttempts)
	}

	started := time.Now()
	sum := loopSummary{
		StartedAt:    started.UTC(),
		RepairMax:    l.cfg.RepairMaxAttempts(),
		FinalResult: loopBlocked, // overwritten once readiness is known
	}
	if *repairAttempts > 0 {
		sum.RepairMax = *repairAttempts
	}

	// --- Step 1/3: verify -------------------------------------------------
	fmt.Println("Step 1/3: verify")
	verifyOpts := runner.Options{
		RepoDir:     l.repoDir,
		EvidenceDir: l.evidenceDir,
		Now:         time.Now(),
		KeepGoing:   *continueOnFailure,
	}
	res, err := runner.Run(l.cfg, verifyOpts)
	if err != nil {
		return err
	}
	verifyPassed := res.Evidence.Passed()
	sum.VerifyResult = res.Evidence.Result
	sum.FinalEvidencePath = relPath(l.repoDir, res.RunDir)
	if verifyPassed {
		fmt.Println("✅ verify passed")
	} else {
		fmt.Println("❌ verify failed")
	}

	// --- Step 2/3: repair -------------------------------------------------
	fmt.Println()
	fmt.Println("Step 2/3: repair")

	repaired := false
	switch {
	case verifyPassed:
		fmt.Println("⏭️  skipped — verification already passed")
	case *noRepair:
		fmt.Println("⏭️  skipped — --no-repair")
	default:
		// loopRepair prints its own skip line when repair can't run (disabled / no
		// agent); either way the loop's verdict comes from re-reading evidence below.
		didRepair, rErr := l.loopRepair(res, *repairAttempts, &sum)
		if rErr != nil {
			return rErr
		}
		repaired = didRepair
	}

	// Re-evaluate the latest evidence after any repair. This is the single source
	// of truth: the loop trusts evidence, not the agent's claims.
	ready, finalRunDir, err := l.evidenceReady()
	if err != nil {
		return err
	}
	if finalRunDir != "" {
		sum.FinalEvidencePath = finalRunDir
	}
	sum.FinalPassing = ready

	// --- Step 3/3: explain ------------------------------------------------
	fmt.Println()
	fmt.Println("Step 3/3: explain")
	switch {
	case !ready:
		fmt.Println("⏭️  skipped — verification is still failing")
	case *noExplain:
		fmt.Println("⏭️  skipped — --no-explain")
	default:
		path, eErr := l.loopExplain()
		if eErr != nil {
			// explain is advisory, not a gate — a failure here does not block the
			// "ready" result, but we surface it and link the saved prompt.
			fmt.Println("⚠️  explanation could not be generated (see message above)")
			sum.ExplainRan = false
			sum.ExplanationPath = path
		} else {
			fmt.Println("✅ explanation saved")
			sum.ExplainRan = true
			sum.ExplanationPath = relPath(l.repoDir, path)
		}
	}

	// --- Result -----------------------------------------------------------
	switch {
	case ready && repaired:
		sum.FinalResult = loopRepaired
	case ready:
		sum.FinalResult = loopReady
	default:
		sum.FinalResult = loopBlocked
	}
	sum.RepairRan = repaired || sum.RepairAttempts > 0
	sum.FinishedAt = time.Now().UTC()

	summaryDir, werr := l.writeLoopSummary(sum)
	if werr != nil {
		// A summary-write failure should not mask the real loop outcome; surface it
		// but continue to print the result and set the exit code from evidence.
		fmt.Fprintf(os.Stderr, "lf loop: could not write loop summary: %v\n", werr)
	}

	printLoopResult(sum, l.repoDir, summaryDir)

	// Exit code mirrors evidence readiness so scripts and `lf status` agree.
	if !ready {
		return &exitError{code: 1}
	}
	return nil
}

// loopRepair runs the repair attempt loop against the failed verify result,
// printing compact per-attempt output. It returns whether the loop ended in a
// repaired state. When repair cannot run (disabled or no agent configured) it
// prints a skip line and returns false. Attempt accounting is recorded into sum.
func (l *loaded) loopRepair(failedRes *runner.Result, attemptsOverride int, sum *loopSummary) (repaired bool, err error) {
	if !l.cfg.RepairEnabled() {
		fmt.Println("⏭️  skipped — repair is disabled (repair.enabled: false)")
		return false, nil
	}
	resolvedName, agent, rerr := l.cfg.ResolveAgent("")
	if rerr != nil {
		fmt.Printf("⏭️  skipped — %v\n", rerr)
		return false, nil
	}

	ev := failedRes.Evidence
	originalRunDir := failedRes.RunDir

	// The verify we just ran is fresh by construction, so repair is never stale.
	maxAttempts := l.cfg.RepairMaxAttempts()
	if attemptsOverride > 0 {
		maxAttempts = attemptsOverride
	}
	verifyAfter := l.cfg.RepairVerifyAfterEach()

	attempts := 0
	hooks := repairHooks{
		beforeAttempt: func(attempt, max int) {
			if attempt > 1 {
				fmt.Println()
			}
			fmt.Printf("Attempt %d/%d\n", attempt, max)
			attempts = attempt
		},
		onAgentError: func(_ int, e error) {
			fmt.Printf("❌ repair agent failed to run: %v\n", e)
		},
		onSkipVerify: func(_ int) {
			fmt.Println("⚠️  verification skipped (repair.verify_after_each_attempt: false)")
		},
		afterVerify: func(_ int, passed bool, _ *evidence.Evidence) {
			if passed {
				fmt.Println("✅ verify passed")
			} else {
				fmt.Println("❌ verify failed")
			}
		},
	}

	final, _, err := l.runRepairAttempts(repairConfig{
		resolvedName:   resolvedName,
		agent:          agent,
		ev:             ev,
		originalRunDir: originalRunDir,
		stale:          false,
		failedCommands: failedCommandSummary(ev),
		maxAttempts:    maxAttempts,
		verifyAfter:    verifyAfter,
	}, hooks)
	if err != nil {
		return false, err
	}
	sum.RepairAttempts = attempts
	return final == "repaired", nil
}

// loopExplain builds the explanation for the (now passing) evidence and invokes
// the configured explain agent, returning the path to the explanation (or the
// saved prompt on failure).
func (l *loaded) loopExplain() (string, error) {
	in, runDir, err := l.explainContext()
	if err != nil {
		return "", err
	}
	prompt := explain.BuildPrompt(in)
	if _, err := explain.Run(l.cfg, l.repoDir, runDir, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "lf loop: %v\n", err)
		// The prompt is preserved by explain.Run; point the caller at it.
		return filepath.Join(runDir, "explain-prompt.md"), err
	}
	return filepath.Join(runDir, "explanation.md"), nil
}

// evidenceReady reports whether the latest evidence is fresh and passing, and the
// repo-relative path of that evidence run. It is the same readiness check
// `lf status --require-fresh-passing` uses, so the loop and the push gate agree.
func (l *loaded) evidenceReady() (bool, string, error) {
	currentHash, err := l.currentDiffHash()
	if err != nil {
		return false, "", err
	}
	var (
		ev     *evidence.Evidence
		runDir string
	)
	if e, dir, lerr := evidence.LoadLatest(l.evidenceDir); lerr == nil {
		ev, runDir = e, dir
	}
	r := evidence.Evaluate(ev, currentHash)
	rel := ""
	if runDir != "" {
		rel = relPath(l.repoDir, runDir)
	}
	return r.Ready(), rel, nil
}

// loopDryRun prints the steps the loop would take without running anything.
func (l *loaded) loopDryRun(noRepair, noExplain bool, attemptsOverride int) error {
	maxAttempts := l.cfg.RepairMaxAttempts()
	if attemptsOverride > 0 {
		maxAttempts = attemptsOverride
	}

	fmt.Println("Dry run — no verify, repair, or explain will be executed.")
	fmt.Println()
	fmt.Println("Step 1/3: verify")
	fmt.Println("  would run lf verify on the current working tree")
	fmt.Println()
	fmt.Println("Step 2/3: repair")
	switch {
	case noRepair:
		fmt.Println("  would be skipped (--no-repair) if verify fails")
	case !l.cfg.RepairEnabled():
		fmt.Println("  would be skipped (repair.enabled: false) if verify fails")
	default:
		name, _, err := l.cfg.ResolveAgent("")
		if err != nil {
			fmt.Printf("  would be skipped (%v) if verify fails\n", err)
		} else {
			fmt.Printf("  if verify fails: would run agent %q for up to %d attempt(s), reverifying after each\n", name, maxAttempts)
		}
	}
	fmt.Println()
	fmt.Println("Step 3/3: explain")
	if noExplain {
		fmt.Println("  would be skipped (--no-explain)")
	} else {
		fmt.Println("  would run lf explain only if final evidence is fresh and passing")
	}
	return nil
}
