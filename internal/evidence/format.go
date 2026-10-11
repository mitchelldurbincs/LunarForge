package evidence

import (
	"fmt"
	"path/filepath"
)

// CheckStdoutPath returns the run-relative log path for a validated check ID.
func CheckStdoutPath(id string) string {
	return filepath.Join("checks", id+".stdout.txt")
}

// CheckStderrPath returns the run-relative log path for a validated check ID.
func CheckStderrPath(id string) string {
	return filepath.Join("checks", id+".stderr.txt")
}

// FormatDuration renders milliseconds consistently in CLI and saved summaries.
func FormatDuration(ms int64) string {
	if ms < 100 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}
