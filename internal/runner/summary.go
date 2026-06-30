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
	fmt.Fprintf(&b, "Result: %s\n\n", plainResult(ev.Result))

	b.WriteString("## Git\n\n")
	fmt.Fprintf(&b, "- Branch: %s\n", ev.Git.Branch)
	fmt.Fprintf(&b, "- HEAD: %s\n", ev.Git.Head)
	fmt.Fprintf(&b, "- Diff hash: %s\n\n", ev.DiffHash)

	b.WriteString("## Commands\n\n")
	b.WriteString("| Command | Result | Duration | Logs |\n")
	b.WriteString("|---|---|---:|---|\n")
	for _, c := range ev.Commands {
		logs := fmt.Sprintf("[stdout](%s) / [stderr](%s)", c.StdoutPath, c.StderrPath)
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
	if result == evidence.ResultPassed {
		return "passed"
	}
	return "failed"
}

func fmtSeconds(ms int64) string {
	if ms < 100 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}
