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
	promptOnly := fs.Bool("prompt-only", false, "build and save the prompt without invoking the explain agent")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf explain [--prompt-only]\n\nExplains the current diff using git + the latest evidence.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

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

	// Load latest evidence if any, and determine freshness.
	in := explain.PromptInput{
		Project: l.cfg.Project.Name,
		Branch:  gitInfo.Branch,
		Head:    gitInfo.Head,
		Status:  gitInfo.StatusPorcelain,
		Diff:    diff,
	}

	var runDir string
	if ev, dir, lerr := evidence.LoadLatest(l.evidenceDir); lerr == nil {
		fresh, _, ferr := freshness(l.repoDir, ev)
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
	if in.HasEvidence {
		if in.EvidenceFresh {
			fmt.Println("Evidence: ✅ fresh")
		} else {
			fmt.Println("Evidence: ⚠️ stale (run `lf verify` for current evidence)")
		}
	} else {
		fmt.Println("Evidence: none (run `lf verify` first for verification context)")
	}
	fmt.Println()

	if *promptOnly {
		promptPath := filepath.Join(runDir, "explain-prompt.md")
		if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
			return err
		}
		fmt.Printf("Prompt saved: %s\n", relPath(l.repoDir, promptPath))
		return nil
	}

	fmt.Printf("Calling %s ...\n\n", l.cfg.Explain.Command)
	explanation, err := explain.Run(l.cfg, runDir, prompt)
	if err != nil {
		// The prompt is preserved by explain.Run; surface a helpful error.
		return fmt.Errorf("%w\n\nThe generated prompt was saved so you can run your agent manually", err)
	}

	fmt.Println(explanation)
	fmt.Println()
	fmt.Printf("Explanation saved: %s\n", relPath(l.repoDir, filepath.Join(runDir, "explanation.md")))
	return nil
}
