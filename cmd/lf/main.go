// Command lf is the LunarForge CLI: a local engineering gate for AI-assisted
// coding. It runs your repo's verify commands, records evidence tied to the
// current git diff, and explains the diff.
package main

import (
	"fmt"
	"os"
)

// version is the build version, overridable via -ldflags.
var version = "0.1.0-mvp"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "verify":
		err = cmdVerify(args)
	case "explain":
		err = cmdExplain(args)
	case "repair":
		err = cmdRepair(args)
	case "status":
		err = cmdStatus(args)
	case "install-hooks":
		err = cmdInstallHooks(args)
	case "version", "--version", "-v":
		fmt.Printf("lf %s\n", version)
		return
	case "help", "--help", "-h":
		usage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		// exitError carries an explicit exit code (e.g. verify failures, stale
		// evidence in --strict mode). Everything else is a generic error.
		if ee, ok := err.(*exitError); ok {
			if ee.message != "" {
				fmt.Fprintln(os.Stderr, ee.message)
			}
			os.Exit(ee.code)
		}
		fmt.Fprintf(os.Stderr, "lf %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

// exitError lets a command request a specific process exit code while keeping a
// clean (already-printed) terminal output.
type exitError struct {
	code    int
	message string
}

func (e *exitError) Error() string {
	if e.message != "" {
		return e.message
	}
	return fmt.Sprintf("exit code %d", e.code)
}

func usage(w *os.File) {
	fmt.Fprintf(w, `LunarForge (lf) %s — local engineering gate for AI-assisted coding

Usage:
  lf <command> [flags]

Commands:
  init            Create .lunarforge.yml and .lf/ in the current repo
  verify          Run verify commands and save evidence tied to the current diff
  status          Show whether the latest evidence is fresh and passing
  explain         Explain the current diff using git + the latest evidence
  repair          Ask a configured AI agent to fix failed verification, then reverify
  install-hooks   Install a pre-push hook that requires fresh passing evidence
  version         Print the version
  help            Show this help

Run 'lf <command> -h' for command-specific flags.
`, version)
}
