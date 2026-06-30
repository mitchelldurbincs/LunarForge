package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/explain"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func cmdExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)
	noRun := fs.Bool("no-run", false, "build and save the prompt without invoking the explain agent")
	promptOnly := fs.Bool("prompt-only", false, "alias of --no-run")
	printPrompt := fs.Bool("print-prompt", false, "print the generated prompt to stdout (implies --no-run)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf explain [--no-run] [--print-prompt]\n\nExplains the current diff using git + the latest evidence.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	skipAgent := *noRun || *promptOnly || *printPrompt

	l, err := load()
	if err != nil {
		return err
	}

	gitInfo, err := gitutil.Snapshot(l.repoDir)
	if err != nil {
		return err
	}
	diff, err := gitutil.Diff(l.repoDir)
	if err != nil {
		return err
	}

	// Load latest evidence if any, and determine freshness. explain is
	// non-blocking: it works whether evidence is fresh, stale, failed, or absent.
	in := explain.PromptInput{
		Project: l.cfg.Project.Name,
		Branch:  gitInfo.Branch,
		Head:    gitInfo.Head,
		Status:  gitInfo.StatusPorcelain,
		Diff:    diff,
	}

	var runDir string
	if ev, dir, lerr := evidence.LoadLatest(l.evidenceDir); lerr == nil {
		fresh, _, ferr := freshness(l, ev)
		if ferr != nil {
			return ferr
		}
		in.Evidence = ev
		in.HasEvidence = true
		in.EvidenceFresh = fresh
		runDir = dir
	} else {
		// No evidence yet: create a fresh run dir just to hold the explanation.
		runID := evidence.NewRunID(time.Now())
		runDir = evidence.RunDir(l.evidenceDir, runID)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return fmt.Errorf("creating run dir: %w", err)
		}
	}

	prompt := explain.BuildPrompt(in)

	fmt.Println("LunarForge explain")
	fmt.Println()
	fmt.Printf("Evidence: %s\n", evidenceBadge(in))
	fmt.Println()

	if *printPrompt {
		// Save and print the prompt itself, then stop.
		if _, err := explain.SavePrompt(runDir, prompt); err != nil {
			return err
		}
		fmt.Println(prompt)
		return nil
	}

	if skipAgent {
		promptPath, err := explain.SavePrompt(runDir, prompt)
		if err != nil {
			return err
		}
		fmt.Printf("Prompt saved: %s\n", relPath(l.repoDir, promptPath))
		return nil
	}

	fmt.Printf("Calling %s ...\n\n", l.cfg.Explain.Command)
	explanation, err := explain.Run(l.cfg, l.repoDir, runDir, prompt)
	if err != nil {
		// The prompt is preserved by explain.Run; surface a helpful error but do
		// not fail the overall flow hard — explain is advisory, not a gate.
		fmt.Fprintf(os.Stderr, "lf explain: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nThe generated prompt was saved so you can run your agent manually.")
		return &exitError{code: 1}
	}

	fmt.Println(explanation)
	fmt.Println()
	fmt.Printf("Explanation saved: %s\n", relPath(l.repoDir, filepath.Join(runDir, "explanation.md")))
	return nil
}

func evidenceBadge(in explain.PromptInput) string {
	switch {
	case !in.HasEvidence:
		return "none (run `lf verify` first for verification context)"
	case !in.Evidence.Passed():
		return "❌ failed (run `lf verify` after fixing)"
	case !in.EvidenceFresh:
		return "⚠️ stale (run `lf verify` for current evidence)"
	default:
		return "✅ fresh and passing"
	}
}
