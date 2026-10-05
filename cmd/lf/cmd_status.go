package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func cmdStatus(args []string) error {
	return statusForHead(args, "")
}

func statusForHead(args []string, expectedHead string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	requireFresh := fs.Bool("require-fresh-passing", false, "exit non-zero unless fresh, passing evidence exists (used by the pre-push hook)")
	strict := fs.Bool("strict", false, "alias of --require-fresh-passing")
	commit := fs.String("commit", "", "require evidence for clean HEAD")
	asJSON := fs.Bool("json", false, "print machine-readable JSON instead of text")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf status [--commit HEAD] [--require-fresh-passing] [--json]\n\n"+
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

	status, err := evaluateStatus(l, *commit, expectedHead)
	if err != nil {
		return err
	}
	r, ev, want := status.readiness, status.evidence, status.identity

	if *asJSON {
		printStatusJSON(r, l, ev, want)
	} else {
		printStatusText(r)
	}

	if enforce && !r.Ready() {
		return &exitError{code: 1}
	}
	return nil
}

type statusEvaluation struct {
	readiness evidence.Readiness
	evidence  *evidence.Evidence
	identity  *evidence.Identity
}

func evaluateStatus(l *loaded, commit, expectedHead string) (*statusEvaluation, error) {
	currentHash, err := l.currentDiffHash()
	if err != nil {
		return nil, err
	}

	// Tolerate "no evidence" — that is a valid (not-ready) state, not an error.
	var (
		ev     *evidence.Evidence
		runDir string
	)
	if e, dir, lerr := evidence.LoadLatest(l.evidenceDir); lerr == nil {
		ev, runDir = e, dir
	}
	var want *evidence.Identity
	r := evidence.Evaluate(ev, currentHash)
	if commit != "" && commit != "HEAD" {
		return nil, fmt.Errorf("--commit supports HEAD only")
	}
	if commit != "" || (ev != nil && ev.Mode == "commit") {
		want, err = evidence.CaptureIdentity(l.cfg, l.repoDir)
		if err != nil {
			return nil, err
		}
		selected, dir, err := evidence.SelectCommit(l.evidenceDir, *want, l.cfg.Verify.TreeReuse)
		if err != nil {
			return nil, err
		}
		// Retain stale latest evidence for diagnostics when no applicable run exists.
		if selected != nil {
			ev, runDir = selected, dir
		}
		r = evidence.EvaluateCommit(ev, *want, l.cfg.Verify.TreeReuse)
		r.WantHash = currentHash
	}
	subject, err := gitutil.ReadSubject(l.repoDir, l.excludes()...)
	if err != nil {
		return nil, err
	}
	if expectedHead != "" && subject.Commit != expectedHead {
		return nil, fmt.Errorf("pre-push: HEAD changed while evaluating evidence")
	}
	if want != nil && want.Subject != subject {
		r.Blocked = "subject_changed"
	}
	if subject.Dirty || (ev != nil && gitutil.PorcelainDirty(ev.Git.StatusPorcelain, l.excludes()...)) {
		r.Blocked = "dirty_subject"
	}
	if runDir != "" {
		r.EvidenceDir = relPath(l.repoDir, runDir)
	}

	return &statusEvaluation{r, ev, want}, nil
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
			fmt.Println("✅ fresh for current subject and verification mode")
		} else {
			fmt.Println("⚠️ stale — subject or verification inputs changed")
		}
	}

	fmt.Println()
	fmt.Println("Result:")
	if r.Ready() {
		fmt.Println("✅ local gate ready; pushing remains a human step")
	} else {
		fmt.Println("❌ not ready to push")
		fmt.Println()
		fmt.Println("Run:")
		fmt.Println("lf verify --commit HEAD")
		fmt.Printf("Reason: %s\n", r.Reason())
	}
}

func printStatusJSON(r evidence.Readiness, l *loaded, ev *evidence.Evidence, want *evidence.Identity) {
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
	out["reused_from"] = r.ReusedFrom
	out["mode"] = "diff"
	if ev != nil {
		if ev.Mode != "" {
			out["mode"] = ev.Mode
		}
		out["evidence_identity"] = ev.Identity
		out["started_at"] = ev.StartedAt
		out["finished_at"] = ev.FinishedAt
		out["subject_verified"] = ev.SubjectVerified
	}
	if want != nil {
		out["mode"] = "commit"
		out["current_identity"] = want
	}
	if subject, err := gitutil.ReadSubject(l.repoDir, l.excludes()...); err == nil {
		out["current_subject"] = subject
	}
	out["repo_dir"] = l.repoDir
	out["config_path"] = l.cfg.Path()
	out["evidence_root"] = l.evidenceDir
	profile := l.cfg.Verify.Profile
	if profile == "" {
		profile = runtime.GOOS
	}
	rows := []any{map[string]any{"id": profile, "platform": runtime.GOOS, "status": localContractState(r), "fresh": r.Fresh, "passed": r.Passed, "ready": r.Ready(), "run_id": r.EvidenceID, "reused_from": r.ReusedFrom}}
	for _, row := range l.cfg.Status.Contracts {
		rows = append(rows, row)
	}
	out["contracts"] = rows
	out["all_contracts_satisfied"] = r.Ready() && len(l.cfg.Status.Contracts) == 0
	out["push_owner"] = "human"
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func localContractState(r evidence.Readiness) string {
	switch {
	case r.Blocked != "":
		return r.Blocked
	case !r.HasEvidence:
		return "missing"
	case !r.Fresh:
		return "stale"
	case !r.Passed:
		return "failed"
	default:
		return "passed"
	}
}
