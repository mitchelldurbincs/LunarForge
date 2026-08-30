package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

// writeSummary writes a human-readable summary.md for the run. It is meant to be
// readable on its own, without opening evidence.json.
func writeSummary(runDir string, ev *evidence.Evidence) error {
	var b strings.Builder
	b.WriteString("# LunarForge Verification Summary\n\n")
	fmt.Fprintf(&b, "State: %s\n", ev.Result)
	fmt.Fprintf(&b, "Reason: %s\n\n", ev.Reason)

	b.WriteString("## Git\n\n")
	fmt.Fprintf(&b, "- Branch: %s\n", ev.Git.Branch)
	fmt.Fprintf(&b, "- HEAD: %s\n", ev.Git.Head)
	fmt.Fprintf(&b, "- Fingerprint before checks: %s\n", ev.DiffHash)
	fmt.Fprintf(&b, "- Fingerprint after checks: %s\n", ev.FinalDiffHash)
	fmt.Fprintf(&b, "- Dirty at start: %t\n\n", ev.Git.Dirty)

	b.WriteString("## Commands\n\n")
	b.WriteString("| Command | Result | Duration | Logs |\n")
	b.WriteString("|---|---|---:|---|\n")
	for _, c := range ev.Commands {
		logs := "—"
		if c.StdoutPath != "" {
			logs = fmt.Sprintf("[stdout](%s) / [stderr](%s)", c.StdoutPath, c.StderrPath)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			c.ID, plainResult(c.Result), fmtSeconds(c.DurationMs), logs)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Run id: %s  \n", ev.RunID)
	fmt.Fprintf(&b, "Started: %s  \n", ev.StartedAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&b, "Finished: %s\n", ev.FinishedAt.Format("2006-01-02 15:04:05 MST"))

	return os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(b.String()), 0o644)
}

func plainResult(result string) string {
	switch result {
	case evidence.ResultPassed:
		return "passed"
	case evidence.ResultFailed:
		return "failed"
	case evidence.ResultBlocked:
		return "blocked"
	case evidence.ResultSkipped:
		return "skipped"
	case evidence.ResultStale:
		return "stale"
	default:
		return result
	}
}

func fmtSeconds(ms int64) string {
	if ms < 100 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}
