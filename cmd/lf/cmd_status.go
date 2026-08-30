package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	requireFresh := fs.Bool("require-fresh-passing", false, "exit non-zero unless fresh passing evidence exists")
	strict := fs.Bool("strict", false, "alias of --require-fresh-passing")
	asJSON := fs.Bool("json", false, "print the stable machine-readable result")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: lf status [--json] [--require-fresh-passing]")
	}
	if err := fs.Parse(args); err != nil {
		return &exitError{code: 2, message: err.Error()}
	}

	l, err := load()
	if err != nil {
		if *asJSON {
			writeJSON(jsonFailure("status", evidence.ResultBlocked, evidence.ReasonConfigInvalid, err))
			return &exitError{code: 2}
		}
		return err
	}
	currentHash, err := l.currentDiffHash()
	if err != nil {
		if *asJSON {
			writeJSON(jsonFailure("status", evidence.ResultError, evidence.ReasonInternalError, err))
			return &exitError{code: 3}
		}
		return err
	}

	ev, runDir, loadErr := evidence.LoadLatest(l.evidenceDir)
	if loadErr != nil && !errors.Is(loadErr, evidence.ErrNoEvidence) {
		if *asJSON {
			writeJSON(jsonFailure("status", evidence.ResultError, evidence.ReasonEvidenceCorrupt, loadErr))
			return &exitError{code: 3}
		}
		return loadErr
	}
	if errors.Is(loadErr, evidence.ErrNoEvidence) {
		ev = nil
	}
	r := evidence.Evaluate(ev, currentHash)
	if runDir != "" {
		r.EvidenceDir = relPath(l.repoDir, runDir)
	}

	if *asJSON {
		if ev == nil {
			info, _ := gitutil.Snapshot(l.repoDir)
			out := JSONResult{
				SchemaVersion: resultSchemaVersion,
				Command:       "status",
				State:         r.State(),
				Reason:        r.Reason(),
				Repository: JSONRepository{
					Root:        l.repoDir,
					Branch:      info.Branch,
					Head:        info.Head,
					Dirty:       info.Dirty,
					Fingerprint: currentHash,
				},
			}
			writeJSON(out)
		} else {
			writeJSON(jsonFromEvidence("status", l, ev, runDir, currentHash, r.State(), r.Reason()))
		}
	} else {
		printStatusText(r)
	}

	// JSON is a programmatic contract, so its exit code always mirrors state.
	if *asJSON || *requireFresh || *strict {
		if code := exitCodeForState(r.State()); code != 0 {
			return &exitError{code: code}
		}
	}
	return nil
}

func printStatusText(r evidence.Readiness) {
	fmt.Println("LunarForge status")
	fmt.Println()
	fmt.Printf("State: %s\n", r.State())
	fmt.Printf("Reason: %s\n", r.Reason())
	if r.EvidenceID != "" {
		fmt.Printf("Run: %s\n", r.EvidenceID)
	}
	if r.EvidenceDir != "" {
		fmt.Printf("Evidence: %s\n", r.EvidenceDir)
	}
	if r.Ready() {
		fmt.Println("Result: ready")
	} else {
		fmt.Println("Result: not ready; run `lf verify`")
	}
}
