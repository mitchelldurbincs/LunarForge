// Package hooks installs git hooks that enforce LunarForge locally. The MVP
// installs a pre-push hook that requires fresh passing evidence before a push.
package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// marker identifies hooks managed by LunarForge.
const marker = "# >>> LunarForge managed pre-push hook >>>"

// prePushScript is the pre-push hook body. It runs `lf status --strict`, which
// exits non-zero unless there is fresh passing evidence for the current diff.
const prePushScript = `#!/bin/sh
` + marker + `
# Installed by: lf install-hooks
# Blocks a push unless LunarForge has fresh, passing evidence for the current diff.
# To bypass once:  git push --no-verify

if ! command -v lf >/dev/null 2>&1; then
  echo "LunarForge: 'lf' not found on PATH; skipping pre-push gate." >&2
  exit 0
fi

if ! lf status --strict; then
  echo "" >&2
  echo "LunarForge pre-push gate failed: no fresh passing evidence." >&2
  echo "Run 'lf verify' and try again, or push with --no-verify to bypass." >&2
  exit 1
fi
exit 0
`

// HooksDir returns the git hooks directory for the repo at repoDir, honoring
// core.hooksPath when configured.
func HooksDir(repoDir string) (string, error) {
	if hp, err := gitConfig(repoDir, "core.hooksPath"); err == nil && strings.TrimSpace(hp) != "" {
		path := strings.TrimSpace(hp)
		if !filepath.IsAbs(path) {
			path = filepath.Join(repoDir, path)
		}
		return path, nil
	}
	gitDir, err := gitRevParse(repoDir, "--git-dir")
	if err != nil {
		return "", err
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoDir, gitDir)
	}
	return filepath.Join(gitDir, "hooks"), nil
}

// InstallResult describes what InstallPrePush did.
type InstallResult struct {
	Path       string
	BackupPath string // non-empty if an existing hook was backed up
	Replaced   bool   // true if a previous LunarForge-managed hook was updated
}

// InstallPrePush writes the pre-push hook. It refuses to blindly overwrite an
// existing unmanaged hook: instead it backs it up first. A previously
// LunarForge-managed hook is updated in place.
func InstallPrePush(repoDir string, now time.Time) (*InstallResult, error) {
	dir, err := HooksDir(repoDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating hooks dir: %w", err)
	}
	path := filepath.Join(dir, "pre-push")
	res := &InstallResult{Path: path}

	if existing, err := os.ReadFile(path); err == nil {
		if strings.Contains(string(existing), marker) {
			res.Replaced = true // managed by us; safe to overwrite
		} else {
			backup := fmt.Sprintf("%s.backup-%s", path, now.UTC().Format("20060102T150405"))
			if err := os.WriteFile(backup, existing, 0o755); err != nil {
				return nil, fmt.Errorf("backing up existing hook: %w", err)
			}
			res.BackupPath = backup
		}
	}

	if err := os.WriteFile(path, []byte(prePushScript), 0o755); err != nil {
		return nil, fmt.Errorf("writing pre-push hook: %w", err)
	}
	// Best-effort executable bit (no-op semantics on Windows).
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o755)
	}
	return res, nil
}

func gitConfig(repoDir, key string) (string, error) {
	return gitRun(repoDir, "config", "--get", key)
}

func gitRevParse(repoDir string, args ...string) (string, error) {
	return gitRun(repoDir, append([]string{"rev-parse"}, args...)...)
}

func gitRun(repoDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	return string(out), err
}
