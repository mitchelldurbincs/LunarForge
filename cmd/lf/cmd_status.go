package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	strict := fs.Bool("strict", false, "exit non-zero unless evidence is fresh AND passing (used by pre-push hook)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf status [--strict]\n\nReports whether the latest evidence is fresh and passing.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	l, err := load()
	if err != nil {
		return err
	}

	fmt.Println("LunarForge status")
	fmt.Println()

	ev, runDir, err := evidence.LoadLatest(l.evidenceDir)
	if err != nil {
		fmt.Println("No evidence found.")
		fmt.Println()
		fmt.Println("Run:")
		fmt.Println("  lf verify")
		if *strict {
			return &exitError{code: 1}
		}
		return nil
	}

	currentHash, err := gitutil.DiffHash(l.repoDir)
	if err != nil {
		return err
	}
	fresh := currentHash == ev.DiffHash

	fmt.Println("Latest run:")
	fmt.Printf("  %s\n", relPath(l.repoDir, runDir))
	fmt.Println()
	fmt.Println("Result:")
	if ev.Passed() {
		fmt.Println("  ✅ passed")
	} else {
		fmt.Println("  ❌ failed")
	}
	fmt.Println()
	fmt.Println("Evidence:")
	if fresh {
		fmt.Println("  ✅ fresh for current diff")
	} else {
		fmt.Println("  ⚠️ stale — current diff changed after verification")
		fmt.Println()
		fmt.Println("Run:")
		fmt.Println("  lf verify")
	}

	if *strict && (!fresh || !ev.Passed()) {
		return &exitError{code: 1}
	}
	return nil
}
