package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// repairSummary is the data behind .lf/runs/<run>/repair/summary.md.
type repairSummary struct {
	OriginalRun    string
	FailedCommands []string
	Agent          string
	Stale          bool
	Attempts       []attemptResult
	Final          string
}

// attemptResult records what happened in a single repair attempt.
type attemptResult struct {
	Attempt      int
	AgentExit    int
	VerifyResult string // "passed", "failed", "skipped"
	VerifyRun    string // run id of the verify rerun, if any
	Note         string
}

func writeRepairSummary(repairDir string, s repairSummary) error {
	var b strings.Builder
	b.WriteString("# LunarForge Repair Summary\n\n")
	fmt.Fprintf(&b, "Original failed run: %s\n", s.OriginalRun)
	fmt.Fprintf(&b, "Failed commands: %s\n", joinOrNone(s.FailedCommands))
	fmt.Fprintf(&b, "Agent: %s\n", s.Agent)
	if s.Stale {
		b.WriteString("Note: the failed evidence was stale relative to the working tree.\n")
	}
	b.WriteString("\n## Attempts\n\n")
	b.WriteString("| Attempt | Agent exit | Verify result | Verify run |\n")
	b.WriteString("|---|---:|---|---|\n")
	for _, a := range s.Attempts {
		vr := a.VerifyResult
		if a.Note != "" {
			vr = a.Note
		}
		fmt.Fprintf(&b, "| %d | %d | %s | %s |\n", a.Attempt, a.AgentExit, orDash(vr), orDash(a.VerifyRun))
	}
	b.WriteString("\n## Final result\n\n")
	fmt.Fprintf(&b, "%s\n\n", finalResultText(s.Final))
	b.WriteString("## Next\n\n")
	if s.Final == "repaired" {
		b.WriteString("lf explain\n")
	} else {
		b.WriteString("lf verify\n")
	}

	return os.WriteFile(filepath.Join(repairDir, "summary.md"), []byte(b.String()), 0o644)
}

func finalResultText(final string) string {
	switch final {
	case "repaired":
		return "repaired and locally verified"
	case "agent_ran_unverified":
		return "agent ran, verification skipped"
	case "agent_error":
		return "repair agent failed to run"
	default:
		return "not repaired — max attempts reached without passing verification"
	}
}

func joinOrNone(parts []string) string {
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
