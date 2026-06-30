package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

// Final result codes for a loop run.
const (
	loopReady    = "ready"    // verify passed without needing repair
	loopRepaired = "repaired" // verify failed, repair fixed it, evidence now passes
	loopBlocked  = "blocked"  // verification is still failing
)

// loopSummary is the data behind a loop run's summary.md and loop.json. It links
// to existing evidence/repair artifacts rather than duplicating logs.
type loopSummary struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`

	VerifyResult string `json:"verify_result"` // "passed" / "failed"

	RepairRan      bool `json:"repair_ran"`
	RepairAttempts int  `json:"repair_attempts"`
	RepairMax      int  `json:"repair_max_attempts"`

	FinalEvidencePath string `json:"final_evidence_path"` // repo-relative run dir
	FinalPassing      bool   `json:"final_passing"`       // latest evidence fresh + passing

	ExplainRan      bool   `json:"explain_ran"`
	ExplanationPath string `json:"explanation_path"` // repo-relative; explanation.md or saved prompt

	FinalResult string `json:"final_result"` // ready / repaired / blocked
}

// loopsDir returns the directory loop summaries live in: a sibling of the
// evidence runs dir (".lf/loops" by default). It is under the same parent
// LunarForge already excludes from the diff hash, so writing a loop summary
// never makes the just-recorded evidence look stale.
func (l *loaded) loopsDir() string {
	return filepath.Join(filepath.Dir(l.evidenceDir), "loops")
}

// writeLoopSummary writes summary.md and loop.json under
// .lf/loops/<timestamp>/ and returns that directory.
func (l *loaded) writeLoopSummary(s loopSummary) (string, error) {
	runID := evidence.NewRunID(s.StartedAt)
	dir := filepath.Join(l.loopsDir(), runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating loop dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling loop summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loop.json"), data, 0o644); err != nil {
		return "", fmt.Errorf("writing loop.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(loopSummaryMarkdown(s)), 0o644); err != nil {
		return "", fmt.Errorf("writing summary.md: %w", err)
	}
	return dir, nil
}

func loopSummaryMarkdown(s loopSummary) string {
	var b strings.Builder
	b.WriteString("# LunarForge Loop Summary\n\n")
	fmt.Fprintf(&b, "- Started: %s\n", s.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Finished: %s\n", s.FinishedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Verify result: %s\n", orDash(s.VerifyResult))
	fmt.Fprintf(&b, "- Repair ran: %t\n", s.RepairRan)
	if s.RepairRan {
		fmt.Fprintf(&b, "- Repair attempts: %d (max %d)\n", s.RepairAttempts, s.RepairMax)
	}
	fmt.Fprintf(&b, "- Final evidence: %s\n", orDash(s.FinalEvidencePath))
	fmt.Fprintf(&b, "- Final evidence fresh & passing: %t\n", s.FinalPassing)
	fmt.Fprintf(&b, "- Explain ran: %t\n", s.ExplainRan)
	if s.ExplanationPath != "" {
		fmt.Fprintf(&b, "- Explanation: %s\n", s.ExplanationPath)
	}
	b.WriteString("\n## Final result\n\n")
	fmt.Fprintf(&b, "%s\n", loopResultText(s.FinalResult))
	return b.String()
}

// loopResultText renders the final result code as a human-readable line.
func loopResultText(final string) string {
	switch final {
	case loopReady:
		return "✅ ready for review"
	case loopRepaired:
		return "✅ repaired and ready for review"
	default:
		return "❌ blocked"
	}
}

// printLoopResult prints the closing result + next-steps block to stdout.
func printLoopResult(s loopSummary, repoDir, summaryDir string) {
	fmt.Println()
	fmt.Println("Result:")
	fmt.Println(loopResultText(s.FinalResult))
	fmt.Println()
	fmt.Println("Next:")
	switch s.FinalResult {
	case loopReady, loopRepaired:
		fmt.Println("git diff")
		fmt.Println("git push")
	default:
		fmt.Println("inspect the latest evidence and repair summary")
		if s.FinalEvidencePath != "" {
			fmt.Println(s.FinalEvidencePath)
		}
	}
	if summaryDir != "" {
		fmt.Println()
		fmt.Println("Loop summary:")
		fmt.Println(relPath(repoDir, summaryDir))
	}
}
