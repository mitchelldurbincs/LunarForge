package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
	"github.com/mitchelldurbincs/lunarforge/internal/repair"
	"github.com/mitchelldurbincs/lunarforge/internal/runner"
)

func cmdRepair(args []string) error {
	fs := flag.NewFlagSet("repair", flag.ExitOnError)
	attempts := fs.Int("attempts", 0, "max agent+verify cycles (overrides repair.max_attempts)")
	agentName := fs.String("agent", "", "name of the agent under agents: to use (overrides repair.agent)")
	fromLatestFailed := fs.Bool("from-latest-failed", false, "repair the most recent failed run even if a newer passing run exists")
	noVerify := fs.Bool("no-verify", false, "invoke the agent without rerunning lf verify (cannot confirm a fix)")
	dryRun := fs.Bool("dry-run", false, "show the plan and prompt location without invoking the agent or verifying")
	printPrompt := fs.Bool("print-prompt", false, "print the generated repair prompt and exit")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf repair [--attempts N] [--agent NAME] [--from-latest-failed]\n"+
			"                 [--no-verify] [--dry-run] [--print-prompt]\n\n"+
			"When verification has failed, asks a configured AI agent for the smallest\n"+
			"safe fix and reruns lf verify. The agent never declares success — only\n"+
			"lf verify can.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	l, err := load()
	if err != nil {
		return err
	}

	fmt.Println("LunarForge repair")
	fmt.Println()

	if !l.cfg.RepairEnabled() {
		fmt.Println("Repair is disabled (repair.enabled: false).")
		return &exitError{code: 1}
	}

	// Resolve which agent to use. This only reads config (not PATH), so it works
	// in --dry-run even when the agent binary is not installed.
	resolvedName, agent, err := l.cfg.ResolveAgent(*agentName)
	if err != nil {
		return err
	}

	// Select the failed evidence to repair.
	ev, originalRunDir, err := selectFailedEvidence(l, *fromLatestFailed)
	if err != nil {
		return err
	}

	fmt.Println("Latest evidence:")
	if ev == nil {
		fmt.Println("❌ none found")
		fmt.Println()
		fmt.Println("Nothing to repair (run `lf verify` first).")
		return &exitError{code: 1}
	}
	if ev.Passed() {
		fmt.Println("✅ passed")
		fmt.Println()
		fmt.Println("Nothing to repair.")
		return nil
	}
	fmt.Println("❌ failed")

	// Stale check: did the working tree change since this failed run was recorded?
	fresh, _, err := freshness(l, ev)
	if err != nil {
		return err
	}
	stale := !fresh
	if stale {
		fmt.Println()
		fmt.Println("⚠️ latest failed evidence is stale relative to the current diff.")
		fmt.Println("Repair will use the failed logs, but the working tree has changed since that run.")
	}

	failedCmds := failedCommandSummary(ev)
	fmt.Println()
	fmt.Println("Failed command:")
	fmt.Println(strings.Join(failedCmds, ", "))

	// Build the prompt from the failed evidence we are repairing.
	prompt, err := l.buildRepairPrompt(ev, originalRunDir, stale)
	if err != nil {
		return err
	}

	repairDir := filepath.Join(originalRunDir, "repair")

	// --print-prompt (without --dry-run): print the prompt and exit.
	if *printPrompt && !*dryRun {
		fmt.Println()
		fmt.Println(prompt)
		return nil
	}

	// --dry-run: describe the plan and stop. No agent, no verify, no files written.
	if *dryRun {
		fmt.Println()
		fmt.Println("Dry run — no agent invoked, no verification run, no files written.")
		fmt.Println()
		fmt.Printf("Agent: %s (backend: %s)\n", resolvedName, orUnset(agent.Backend))
		fmt.Printf("Command: %s\n", agentCommandLine(agent))
		fmt.Println()
		fmt.Println("Would save artifacts under:")
		fmt.Printf("%s\n", relPath(l.repoDir, filepath.Join(repairDir, "attempt-1")))
		fmt.Printf("  prompt.md, agent.stdout.txt, agent.stderr.txt, result.json\n")
		fmt.Printf("%s\n", relPath(l.repoDir, filepath.Join(repairDir, "summary.md")))
		if *printPrompt {
			fmt.Println()
			fmt.Println("Prompt:")
			fmt.Println()
			fmt.Println(prompt)
		}
		return nil
	}

	// Real run.
	maxAttempts := l.cfg.RepairMaxAttempts()
	if *attempts > 0 {
		maxAttempts = *attempts
	}
	verifyAfter := l.cfg.RepairVerifyAfterEach() && !*noVerify

	// Print hooks reproduce `lf repair`'s per-attempt output. `lf loop` passes its
	// own hooks for a more compact format; the mechanics are identical.
	hooks := repairHooks{
		beforeAttempt: func(attempt, max int) {
			fmt.Println()
			fmt.Printf("Attempt %d/%d:\n", attempt, max)
			fmt.Printf("🤖 running repair agent: %s\n", resolvedName)
		},
		onAgentError: func(_ int, err error) {
			fmt.Fprintf(os.Stderr, "lf repair: %v\n", err)
		},
		onSkipVerify: func(_ int) {
			fmt.Println("(verification skipped: --no-verify)")
		},
		beforeVerify: func(_ int) {
			fmt.Println("🔁 running lf verify")
		},
		afterVerify: func(_ int, passed bool, ev *evidence.Evidence) {
			if passed {
				fmt.Println("✅ verify passed")
				return
			}
			if failed := firstFailed(ev); failed != nil {
				fmt.Printf("❌ %s failed\n", failed.ID)
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
		stale:          stale,
		failedCommands: failedCmds,
		maxAttempts:    maxAttempts,
		verifyAfter:    verifyAfter,
	}, hooks)
	if err != nil {
		return err
	}

	return l.printRepairResult(final, repairDir)
}

// repairConfig is the resolved input to runRepairAttempts.
type repairConfig struct {
	resolvedName   string
	agent          config.Agent
	ev             *evidence.Evidence // the failed evidence being repaired
	originalRunDir string             // run dir of ev (repair artifacts live under it)
	stale          bool               // ev was stale relative to the working tree
	failedCommands []string           // ids of failed commands in ev
	maxAttempts    int
	verifyAfter    bool // rerun lf verify after each agent attempt
}

// repairHooks lets callers stream progress without baking output format into the
// attempt loop. Every field is optional (nil = no output for that event).
type repairHooks struct {
	beforeAttempt func(attempt, max int)
	onAgentError  func(attempt int, err error)
	onSkipVerify  func(attempt int)
	beforeVerify  func(attempt int)
	afterVerify   func(attempt int, passed bool, ev *evidence.Evidence)
}

// runRepairAttempts runs the agent+verify loop, writing per-attempt artifacts and
// a repair summary under <originalRunDir>/repair. It returns the final state
// ("repaired", "exhausted", "agent_error", or "agent_ran_unverified") and the
// repair directory. Each verify rerun produces its own normal evidence run and
// advances the latest pointer, so the agent never decides success — only the
// reverify does. This is the single shared mechanism behind `lf repair` and the
// repair step of `lf loop`.
func (l *loaded) runRepairAttempts(rc repairConfig, hooks repairHooks) (string, string, error) {
	repairDir := filepath.Join(rc.originalRunDir, "repair")
	if err := os.MkdirAll(repairDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating repair dir: %w", err)
	}

	sum := repairSummary{
		OriginalRun:    rc.ev.RunID,
		FailedCommands: rc.failedCommands,
		Agent:          rc.resolvedName,
		Stale:          rc.stale,
	}

	// curEv/curRunDir track the failure being repaired. After a failed verify
	// rerun they advance to the new evidence so the next prompt uses fresh logs.
	curEv, curRunDir := rc.ev, rc.originalRunDir
	final := "exhausted"

	for attempt := 1; attempt <= rc.maxAttempts; attempt++ {
		if hooks.beforeAttempt != nil {
			hooks.beforeAttempt(attempt, rc.maxAttempts)
		}

		attemptDir := filepath.Join(repairDir, fmt.Sprintf("attempt-%d", attempt))
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			return "", "", fmt.Errorf("creating attempt dir: %w", err)
		}

		attemptPrompt, perr := l.buildRepairPrompt(curEv, curRunDir, attempt == 1 && rc.stale)
		if perr != nil {
			return "", "", perr
		}
		if err := os.WriteFile(filepath.Join(attemptDir, "prompt.md"), []byte(attemptPrompt), 0o644); err != nil {
			return "", "", fmt.Errorf("saving prompt: %w", err)
		}

		ar, runErr := repair.RunAgent(rc.resolvedName, rc.agent, l.repoDir, attemptPrompt)
		saveAgentArtifacts(attemptDir, ar)

		attemptRec := attemptResult{Attempt: attempt, AgentExit: ar.ExitCode}
		if runErr != nil {
			// Could not start the agent at all — surface and stop; nothing was edited.
			if hooks.onAgentError != nil {
				hooks.onAgentError(attempt, runErr)
			}
			attemptRec.Note = "agent failed to start"
			sum.Attempts = append(sum.Attempts, attemptRec)
			final = "agent_error"
			break
		}

		if !rc.verifyAfter {
			if hooks.onSkipVerify != nil {
				hooks.onSkipVerify(attempt)
			}
			attemptRec.VerifyResult = "skipped"
			sum.Attempts = append(sum.Attempts, attemptRec)
			final = "agent_ran_unverified"
			break
		}

		if hooks.beforeVerify != nil {
			hooks.beforeVerify(attempt)
		}
		res, verr := runner.Run(l.cfg, runner.Options{
			RepoDir:     l.repoDir,
			EvidenceDir: l.evidenceDir,
			Now:         time.Now(),
		})
		if verr != nil {
			return "", "", verr
		}
		attemptRec.VerifyResult = res.Evidence.Result
		attemptRec.VerifyRun = res.Evidence.RunID
		sum.Attempts = append(sum.Attempts, attemptRec)

		passed := res.Evidence.Passed()
		if hooks.afterVerify != nil {
			hooks.afterVerify(attempt, passed, res.Evidence)
		}
		if passed {
			final = "repaired"
			break
		}
		// Advance to the fresh failure so the next attempt's prompt is current.
		curEv, curRunDir = res.Evidence, res.RunDir
	}

	sum.Final = final
	if err := writeRepairSummary(repairDir, sum); err != nil {
		return "", "", err
	}
	return final, repairDir, nil
}

// selectFailedEvidence returns the evidence to repair and its run directory.
// With fromLatestFailed it returns the most recent failed run even when a newer
// passing run exists; otherwise it returns the latest run (which the caller then
// checks for pass/fail). A nil evidence with nil error means none exists.
func selectFailedEvidence(l *loaded, fromLatestFailed bool) (*evidence.Evidence, string, error) {
	if fromLatestFailed {
		return latestFailedEvidence(l.evidenceDir)
	}
	ev, dir, err := evidence.LoadLatest(l.evidenceDir)
	if err != nil {
		// No evidence at all is a valid (refuse) state, not a hard error.
		return nil, "", nil
	}
	return ev, dir, nil
}

// latestFailedEvidence scans the evidence dir for the newest run whose result is
// failed. Run ids sort lexically by time, so the largest matching name wins.
func latestFailedEvidence(evidenceDir string) (*evidence.Evidence, string, error) {
	entries, err := os.ReadDir(evidenceDir)
	if err != nil {
		return nil, "", nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	// Iterate from newest to oldest.
	for i := len(names) - 1; i >= 0; i-- {
		runDir := evidence.RunDir(evidenceDir, names[i])
		ev, lerr := evidence.Load(runDir)
		if lerr != nil {
			continue
		}
		if !ev.Passed() {
			return ev, runDir, nil
		}
	}
	return nil, "", nil
}

// buildRepairPrompt assembles the prompt for a given failed evidence record,
// reading and truncating its command logs from runDir.
func (l *loaded) buildRepairPrompt(ev *evidence.Evidence, runDir string, stale bool) (string, error) {
	gitInfo, err := gitutil.Snapshot(l.repoDir)
	if err != nil {
		return "", err
	}
	diff, err := gitutil.Diff(l.repoDir)
	if err != nil {
		return "", err
	}
	maxChars := l.cfg.RepairMaxLogChars()
	diff, _ = repair.Truncate(diff, maxChars)

	var verifyCmds []string
	for _, c := range l.cfg.Verify.Commands {
		verifyCmds = append(verifyCmds, c.Run)
	}

	var failed []repair.FailedCommand
	for _, c := range ev.Commands {
		if c.Result == evidence.ResultPassed {
			continue
		}
		fc := repair.FailedCommand{
			ID:         c.ID,
			Run:        c.Run,
			ExitCode:   c.ExitCode,
			StdoutPath: relPath(l.repoDir, filepath.Join(runDir, c.StdoutPath)),
			StderrPath: relPath(l.repoDir, filepath.Join(runDir, c.StderrPath)),
		}
		fc.StdoutExcerpt = readLogExcerpt(filepath.Join(runDir, c.StdoutPath), maxChars)
		fc.StderrExcerpt = readLogExcerpt(filepath.Join(runDir, c.StderrPath), maxChars)
		failed = append(failed, fc)
	}

	return repair.BuildPrompt(repair.PromptInput{
		Project:        l.cfg.Project.Name,
		Branch:         gitInfo.Branch,
		Head:           gitInfo.Head,
		Status:         gitInfo.StatusPorcelain,
		Diff:           diff,
		VerifyCommands: verifyCmds,
		Failed:         failed,
		Stale:          stale,
	}), nil
}

// readLogExcerpt reads a log file and truncates it for inclusion in the prompt.
// A missing/unreadable file yields an empty excerpt (the path is still listed).
func readLogExcerpt(path string, maxChars int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	out, _ := repair.Truncate(string(data), maxChars)
	return out
}

func failedCommandSummary(ev *evidence.Evidence) []string {
	var out []string
	for _, c := range ev.Commands {
		if c.Result != evidence.ResultPassed {
			out = append(out, c.ID)
		}
	}
	return out
}

func saveAgentArtifacts(attemptDir string, ar repair.AgentResult) {
	_ = os.WriteFile(filepath.Join(attemptDir, "agent.stdout.txt"), []byte(ar.Stdout), 0o644)
	_ = os.WriteFile(filepath.Join(attemptDir, "agent.stderr.txt"), []byte(ar.Stderr), 0o644)
	if data, err := json.MarshalIndent(ar, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(attemptDir, "result.json"), data, 0o644)
	}
}

func (l *loaded) printRepairResult(final, repairDir string) error {
	fmt.Println()
	fmt.Println("Result:")
	switch final {
	case "repaired":
		fmt.Println("✅ repaired and locally verified")
		fmt.Println()
		fmt.Println("Next:")
		fmt.Println("lf explain")
	case "agent_ran_unverified":
		fmt.Println("⚠️ agent ran, but verification was skipped — run `lf verify` to confirm")
	case "agent_error":
		fmt.Println("❌ repair agent failed to run")
	default: // exhausted
		fmt.Println("❌ not repaired — max attempts reached without passing verification")
	}
	fmt.Println()
	fmt.Println("Repair artifacts:")
	fmt.Printf("%s\n", relPath(l.repoDir, repairDir))

	if final != "repaired" {
		return &exitError{code: 1}
	}
	return nil
}

func agentCommandLine(a config.Agent) string {
	parts := append([]string{a.Command}, a.Args...)
	return strings.Join(parts, " ")
}

func orUnset(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(unset)"
	}
	return s
}
