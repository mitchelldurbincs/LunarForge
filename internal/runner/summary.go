package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

// writeSummary writes a human-readable summary.md for the run.
func writeSummary(runDir string, ev *evidence.Evidence) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# LunarForge verify — %s\n\n", ev.RunID)
	fmt.Fprintf(&b, "- **Project:** %s\n", ev.Project)
	fmt.Fprintf(&b, "- **Result:** %s\n", resultBadge(ev.Result))
	fmt.Fprintf(&b, "- **Branch:** %s\n", ev.Git.Branch)
	fmt.Fprintf(&b, "- **HEAD:** %s\n", ev.Git.Head)
	fmt.Fprintf(&b, "- **Diff hash:** `%s`\n", ev.DiffHash)
	fmt.Fprintf(&b, "- **Started:** %s\n", ev.StartedAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&b, "- **Finished:** %s\n\n", ev.FinishedAt.Format("2006-01-02 15:04:05 MST"))

	b.WriteString("## Commands\n\n")
	b.WriteString("| ID | Result | Exit | Duration | Command |\n")
	b.WriteString("|----|--------|------|----------|---------|\n")
	for _, c := range ev.Commands {
		fmt.Fprintf(&b, "| %s | %s | %d | %dms | `%s` |\n",
			c.ID, resultBadge(c.Result), c.ExitCode, c.DurationMs, c.Run)
	}
	b.WriteString("\nOutput for each command is stored under `commands/`.\n")

	return os.WriteFile(filepath.Join(runDir, "summary.md"), []byte(b.String()), 0o644)
}

func resultBadge(result string) string {
	if result == evidence.ResultPassed {
		return "✅ passed"
	}
	return "❌ failed"
}
