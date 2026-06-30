// Package explain builds a prompt describing the current diff and verification
// evidence, then invokes the configured agent command to produce a
// human-readable explanation.
package explain

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

// PromptInput is everything needed to build the explanation prompt.
type PromptInput struct {
	Project       string
	Branch        string
	Head          string
	Status        string
	Diff          string
	Evidence      *evidence.Evidence // may be nil if no run exists
	EvidenceFresh bool
	HasEvidence   bool
}

// BuildPrompt assembles the prompt text sent to the explanation agent.
func BuildPrompt(in PromptInput) string {
	var b strings.Builder
	b.WriteString("You are reviewing a local code change for an engineer who is about to review it themselves.\n")
	b.WriteString("Using ONLY the git status, git diff, and verification evidence below, write a concise review.\n\n")
	b.WriteString("Produce these sections, in this order:\n")
	b.WriteString("1. Summary — a concise plain-language summary of the change.\n")
	b.WriteString("2. Files changed — list each changed file.\n")
	b.WriteString("3. Why each file changed — one short explanation per file.\n")
	b.WriteString("4. Verification evidence — what was run and whether it passed.\n")
	b.WriteString("5. Evidence freshness — state clearly whether the evidence is FRESH, STALE, FAILED, or MISSING for the current diff, and what that means.\n")
	b.WriteString("6. Risks — what could go wrong or deserves scrutiny.\n")
	b.WriteString("7. Manual review suggestions — concrete things the human should check by hand.\n\n")
	b.WriteString("Do not invent changes that are not in the diff. If the diff is empty, say so.\n\n")

	b.WriteString("=== CONTEXT ===\n")
	fmt.Fprintf(&b, "Project: %s\n", orNone(in.Project))
	fmt.Fprintf(&b, "Branch: %s\n", orNone(in.Branch))
	fmt.Fprintf(&b, "HEAD: %s\n\n", orNone(in.Head))

	b.WriteString("=== GIT STATUS (porcelain) ===\n")
	if strings.TrimSpace(in.Status) == "" {
		b.WriteString("(working tree clean)\n")
	} else {
		b.WriteString(in.Status)
		if !strings.HasSuffix(in.Status, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	b.WriteString("=== VERIFICATION EVIDENCE ===\n")
	if !in.HasEvidence || in.Evidence == nil {
		b.WriteString("Status: MISSING — no verification evidence exists. Run `lf verify` first.\n")
	} else {
		ev := in.Evidence
		fmt.Fprintf(&b, "Status: %s\n", evidenceStatus(in))
		fmt.Fprintf(&b, "Run: %s\n", ev.RunID)
		fmt.Fprintf(&b, "Overall result: %s\n", ev.Result)
		fmt.Fprintf(&b, "Evidence diff hash: %s\n", ev.DiffHash)
		if in.EvidenceFresh {
			b.WriteString("Freshness: FRESH — evidence matches the current diff.\n")
		} else {
			b.WriteString("Freshness: STALE — the diff changed after verification; evidence may not reflect the current code.\n")
		}
		b.WriteString("Commands:\n")
		for _, c := range ev.Commands {
			fmt.Fprintf(&b, "  - %s (%s): exit %d, %dms — `%s`\n", c.ID, c.Result, c.ExitCode, c.DurationMs, c.Run)
		}
	}
	b.WriteString("\n")

	b.WriteString("=== GIT DIFF ===\n")
	if strings.TrimSpace(in.Diff) == "" {
		b.WriteString("(no diff — nothing changed)\n")
	} else {
		b.WriteString(in.Diff)
		if !strings.HasSuffix(in.Diff, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

// evidenceStatus reduces the evidence state to a single word for the prompt.
func evidenceStatus(in PromptInput) string {
	switch {
	case !in.HasEvidence || in.Evidence == nil:
		return "MISSING"
	case !in.Evidence.Passed():
		return "FAILED"
	case !in.EvidenceFresh:
		return "STALE"
	default:
		return "FRESH (passing)"
	}
}

// SavePrompt writes the prompt to <runDir>/explain-prompt.md and returns its
// path. It is always safe to call, even when the explain command is unavailable.
func SavePrompt(runDir, prompt string) (string, error) {
	promptPath := filepath.Join(runDir, "explain-prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return "", fmt.Errorf("saving prompt: %w", err)
	}
	return promptPath, nil
}

// Run invokes the configured explain command with the prompt as the final
// argument, using an exec-style argument array (no shell). It returns the
// explanation text. The prompt is always written to <runDir>/explain-prompt.md
// first, so it is preserved even if the command fails or is not installed.
//
// The command runs with the working directory set to repoDir so that relative
// command paths and arguments resolve against the repo, not the run directory.
func Run(cfg *config.Config, repoDir, runDir, prompt string) (string, error) {
	promptPath, err := SavePrompt(runDir, prompt)
	if err != nil {
		return "", err
	}

	command := cfg.Explain.Command
	if command == "" {
		return "", fmt.Errorf("explain.command is not configured; prompt saved to %s", promptPath)
	}
	resolved, err := resolveCommand(repoDir, command)
	if err != nil {
		return "", fmt.Errorf("%w (prompt saved to %s)", err, promptPath)
	}

	args := append(append([]string{}, cfg.Explain.Args...), prompt)
	cmd := exec.Command(resolved, args...)
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("explain command failed: %w\n%s\n(prompt saved to %s)",
			err, strings.TrimSpace(stderr.String()), promptPath)
	}

	explanation := stdout.String()
	if err := os.WriteFile(filepath.Join(runDir, "explanation.md"), []byte(explanation), 0o644); err != nil {
		return "", fmt.Errorf("saving explanation: %w", err)
	}
	return explanation, nil
}

// resolveCommand turns the configured command into an executable path. A command
// containing a path separator is treated as repo-relative (e.g.
// "./scripts/fake-explain.sh"); a bare name is looked up on PATH (e.g. "claude").
func resolveCommand(repoDir, command string) (string, error) {
	if strings.ContainsAny(command, "/\\") && !filepath.IsAbs(command) {
		candidate := filepath.Join(repoDir, command)
		if _, err := os.Stat(candidate); err != nil {
			return "", fmt.Errorf("explain command %q not found at %s: %w", command, candidate, err)
		}
		return candidate, nil
	}
	if _, err := exec.LookPath(command); err != nil {
		return "", fmt.Errorf("explain command %q not found on PATH: %w", command, err)
	}
	return command, nil
}
