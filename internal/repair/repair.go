// Package repair builds a strict prompt describing a failed LunarForge
// verification run and invokes a configured AI agent to attempt the smallest
// safe fix. The agent never gets to declare success: `lf repair` reruns
// `lf verify`, which is the only thing that can confirm a repair.
//
// The agent abstraction is intentionally tiny — an external command plus fixed
// args (see config.Agent). The generated prompt is delivered on the command's
// stdin, which both `claude --print` and `codex exec -` accept, so a single
// invocation path serves both backends without per-backend branching.
package repair

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

// FailedCommand describes one verify command that failed, with excerpts of its
// captured output and the repo-relative paths to the full logs on disk.
type FailedCommand struct {
	ID            string
	Run           string
	ExitCode      int
	StdoutPath    string // repo-relative
	StderrPath    string // repo-relative
	StdoutExcerpt string
	StderrExcerpt string
}

// PromptInput is everything needed to build the repair prompt.
type PromptInput struct {
	Project        string
	Branch         string
	Head           string
	Status         string
	Diff           string
	VerifyCommands []string // the run strings of every configured verify command
	Failed         []FailedCommand
	Stale          bool // failed evidence no longer matches the current diff
}

// rules is the verbatim constraint block the prompt must always carry.
const rules = `Rules:
- Do not start unrelated refactors.
- Do not weaken or delete tests just to pass.
- Do not skip the failing command.
- Do not change .lunarforge.yml unless the failure is clearly caused by incorrect LunarForge config.
- Do not push, commit, or create branches.
- Do not edit generated/vendor/secrets paths.
- Prefer minimal targeted changes.
- After editing, do not claim success. LunarForge will rerun verification.`

// BuildPrompt assembles the strict repair prompt.
func BuildPrompt(in PromptInput) string {
	var b strings.Builder
	b.WriteString("You are repairing a failed LunarForge verification run.\n\n")
	b.WriteString("Goal:\n")
	b.WriteString("Make the configured verification pass with the smallest safe diff.\n\n")
	b.WriteString(rules)
	b.WriteString("\n\n")

	if in.Stale {
		b.WriteString("Note: the working tree has changed since this failed run was recorded, so the\n")
		b.WriteString("logs below may be slightly out of date relative to the current files.\n\n")
	}

	b.WriteString("=== CONTEXT ===\n")
	fmt.Fprintf(&b, "Project: %s\n", orNone(in.Project))
	fmt.Fprintf(&b, "Branch: %s\n", orNone(in.Branch))
	fmt.Fprintf(&b, "HEAD: %s\n\n", orNone(in.Head))

	b.WriteString("=== VERIFICATION COMMANDS (what must pass) ===\n")
	if len(in.VerifyCommands) == 0 {
		b.WriteString("(none configured)\n")
	} else {
		for _, c := range in.VerifyCommands {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	b.WriteString("\n")

	b.WriteString("=== FAILED COMMAND(S) ===\n")
	if len(in.Failed) == 0 {
		b.WriteString("(none recorded)\n\n")
	}
	for _, f := range in.Failed {
		fmt.Fprintf(&b, "Command id: %s\n", f.ID)
		fmt.Fprintf(&b, "Command: %s\n", f.Run)
		fmt.Fprintf(&b, "Exit code: %d\n", f.ExitCode)
		fmt.Fprintf(&b, "Full stdout log: %s\n", orNone(f.StdoutPath))
		fmt.Fprintf(&b, "Full stderr log: %s\n", orNone(f.StderrPath))
		b.WriteString("--- stdout excerpt ---\n")
		writeExcerpt(&b, f.StdoutExcerpt)
		b.WriteString("--- stderr excerpt ---\n")
		writeExcerpt(&b, f.StderrExcerpt)
		b.WriteString("\n")
	}

	b.WriteString("=== GIT STATUS (porcelain) ===\n")
	if strings.TrimSpace(in.Status) == "" {
		b.WriteString("(working tree clean)\n")
	} else {
		writeBlock(&b, in.Status)
	}
	b.WriteString("\n")

	b.WriteString("=== GIT DIFF ===\n")
	if strings.TrimSpace(in.Diff) == "" {
		b.WriteString("(no diff)\n")
	} else {
		writeBlock(&b, in.Diff)
	}
	b.WriteString("\n")

	b.WriteString("Remember: make the smallest safe change, then stop. Do not claim success —\n")
	b.WriteString("LunarForge will rerun verification to decide.\n")
	return b.String()
}

func writeExcerpt(b *strings.Builder, s string) {
	if strings.TrimSpace(s) == "" {
		b.WriteString("(empty)\n")
		return
	}
	writeBlock(b, s)
}

func writeBlock(b *strings.Builder, s string) {
	b.WriteString(s)
	if !strings.HasSuffix(s, "\n") {
		b.WriteString("\n")
	}
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

// Truncate returns at most max characters of s. When s is longer it keeps the
// tail (where build/test failures usually surface) and prefixes a marker noting
// how much was dropped. It reports whether truncation happened.
func Truncate(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	dropped := len(s) - max
	tail := s[len(s)-max:]
	return fmt.Sprintf("[... truncated %d characters; see full log on disk ...]\n%s", dropped, tail), true
}

// AgentResult records the outcome of invoking a repair agent. ExitCode and the
// captured streams describe how the command ran; Err is set only when the
// command could not be started or resolved.
type AgentResult struct {
	Name     string   `json:"name"`
	Backend  string   `json:"backend"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	ExitCode int      `json:"exit_code"`
	Err      string   `json:"error,omitempty"`
	Stdout   string   `json:"-"`
	Stderr   string   `json:"-"`
}

// RunAgent invokes the agent command with the prompt on stdin, running in
// repoDir, and returns the captured result. The returned error is non-nil only
// when the command could not be resolved or started; a non-zero exit code from
// the agent is reported via AgentResult.ExitCode, not as an error, so callers
// can still persist artifacts.
func RunAgent(name string, a config.Agent, repoDir, prompt string) (AgentResult, error) {
	res := AgentResult{Name: name, Backend: a.Backend, Command: a.Command, Args: a.Args}

	resolved, err := resolveCommand(repoDir, a.Command)
	if err != nil {
		res.Err = err.Error()
		res.ExitCode = -1
		return res, err
	}

	cmd := exec.Command(resolved, a.Args...)
	cmd.Dir = repoDir
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	res.Stdout = stdout.String()
	res.Stderr = stderr.String()
	res.ExitCode = exitCode(runErr)
	if runErr != nil {
		if _, ok := runErr.(*exec.ExitError); !ok {
			// Could not start the process at all (e.g. not executable).
			res.Err = runErr.Error()
			return res, runErr
		}
	}
	return res, nil
}

// resolveCommand mirrors the explain package: a command containing a path
// separator is treated as repo-relative; a bare name is looked up on PATH.
func resolveCommand(repoDir, command string) (string, error) {
	if strings.ContainsAny(command, "/\\") && !filepath.IsAbs(command) {
		candidate := filepath.Join(repoDir, command)
		if _, err := os.Stat(candidate); err != nil {
			return "", fmt.Errorf("repair agent command %q not found at %s: %w", command, candidate, err)
		}
		return candidate, nil
	}
	if _, err := exec.LookPath(command); err != nil {
		return "", fmt.Errorf("repair agent command %q not found on PATH: %w", command, err)
	}
	return command, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
