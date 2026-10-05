package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	requireFresh := fs.Bool("require-fresh-passing", false, "exit non-zero unless fresh, passing evidence exists (used by the pre-push hook)")
	strict := fs.Bool("strict", false, "alias of --require-fresh-passing")
	commit := fs.String("commit", "", "require evidence for clean HEAD")
	asJSON := fs.Bool("json", false, "print machine-readable JSON instead of text")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf status [--require-fresh-passing] [--json]\n\n"+
			"Reports whether the latest evidence is fresh and passing.\n"+
			"With --require-fresh-passing, exits non-zero unless the repo is ready to push.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	enforce := *requireFresh || *strict

	l, err := load()
	if err != nil {
		return err
	}

	currentHash, err := l.currentDiffHash()
	if err != nil {
		return err
	}

	// Tolerate "no evidence" — that is a valid (not-ready) state, not an error.
	var (
		ev     *evidence.Evidence
		runDir string
	)
	if e, dir, lerr := evidence.LoadLatest(l.evidenceDir); lerr == nil {
		ev, runDir = e, dir
	}
	r := evidence.Evaluate(ev, currentHash)
	if *commit != "" && *commit != "HEAD" {
		return fmt.Errorf("--commit supports HEAD only")
	}
	if *commit != "" || (ev != nil && ev.Mode == "commit") {
		want, err := evidence.CaptureIdentity(l.cfg, l.repoDir)
		if err != nil {
			return err
		}
		selected, dir, err := evidence.SelectCommit(l.evidenceDir, *want, l.cfg.Verify.TreeReuse)
		if err != nil {
			return err
		}
		// Retain stale latest evidence for diagnostics when no applicable run exists.
		if selected != nil {
			ev, runDir = selected, dir
		}
		r = evidence.EvaluateCommit(ev, *want, l.cfg.Verify.TreeReuse)
	}
	if enforce {
		subject, err := gitutil.ReadSubject(l.repoDir, l.excludes()...)
		if err != nil {
			return err
		}
		if subject.Dirty || (ev != nil && gitutil.PorcelainDirty(ev.Git.StatusPorcelain, l.excludes()...)) {
			r.Blocked = "dirty_subject"
		}
	}
	if runDir != "" {
		r.EvidenceDir = relPath(l.repoDir, runDir)
	}

	if *asJSON {
		printStatusJSON(r)
	} else {
		printStatusText(r)
	}

	if enforce && !r.Ready() {
		return &exitError{code: 1}
	}
	return nil
}

func printStatusText(r evidence.Readiness) {
	fmt.Println("LunarForge status")
	fmt.Println()

	fmt.Println("Latest evidence:")
	switch {
	case !r.HasEvidence:
		fmt.Println("❌ none found")
	case r.Passed:
		fmt.Println("✅ passed")
	default:
		fmt.Println("❌ failed")
	}

	// Freshness only matters when passing evidence exists.
	if r.HasEvidence && r.Passed {
		fmt.Println()
		fmt.Println("Freshness:")
		if r.Fresh {
			fmt.Println("✅ fresh for current diff")
		} else {
			fmt.Println("⚠️ stale — current diff changed after verification")
		}
	}

	fmt.Println()
	fmt.Println("Result:")
	if r.Ready() {
		fmt.Println("✅ ready to push")
	} else {
		fmt.Println("❌ not ready to push")
		fmt.Println()
		fmt.Println("Run:")
		fmt.Println("lf verify")
	}
}

func printStatusJSON(r evidence.Readiness) {
	out := map[string]any{
		"has_evidence":       r.HasEvidence,
		"passed":             r.Passed,
		"fresh":              r.Fresh,
		"ready":              r.Ready(),
		"reason":             r.Reason(),
		"run_id":             r.EvidenceID,
		"run_dir":            r.EvidenceDir,
		"current_diff_hash":  r.WantHash,
		"evidence_diff_hash": r.HaveHash,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
