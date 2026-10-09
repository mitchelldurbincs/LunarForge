// Package gitutil wraps the small set of git operations LunarForge needs:
// confirming we are in a repo, reading status/diff, and computing a
// deterministic hash of the current working-tree changes.
package gitutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Info captures a snapshot of basic repository state.
type Info struct {
	Branch          string
	Head            string
	Dirty           bool
	StatusPorcelain string
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Snapshot collects branch, HEAD, and porcelain status for evidence records.
func Snapshot(dir string) (Info, error) {
	if !IsRepo(dir) {
		return Info{}, fmt.Errorf("not inside a git repository")
	}
	branch, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// A repo with no commits yet has no HEAD; treat as unborn branch.
		branch = "(unborn)"
	}
	// Consumers compare this value with Git and other providers. Abbreviations
	// depend on local Git configuration and cannot serve as an exact object ID.
	head, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		head = "(none)"
	}
	status, err := run(dir, "status", "--porcelain")
	if err != nil {
		return Info{}, fmt.Errorf("git status: %w", err)
	}
	return Info{
		Branch:          strings.TrimSpace(branch),
		Head:            strings.TrimSpace(head),
		Dirty:           strings.TrimSpace(status) != "",
		StatusPorcelain: status,
	}, nil
}

// DiffHash computes a deterministic fingerprint of HEAD plus tracked, staged,
// and untracked working-tree content.
// It combines:
//
//	git rev-parse HEAD
//	git diff --binary
//	git diff --cached --binary
//	git status --porcelain
//	contents of every untracked, non-ignored file
//
// If HEAD advances, or tracked/staged changes change after `lf verify`, the
// hash changes and evidence becomes stale. The returned value is prefixed with
// "sha256:".
//
// excludes are repo-relative pathspecs (e.g. ".lf") that are removed from every
// section. This is how LunarForge keeps its own evidence artifacts from
// invalidating the hash they are recorded under. The same excludes must be used
// at verify time and at status time for hashes to match.
func DiffHash(dir string, excludes ...string) (string, error) {
	if !IsRepo(dir) {
		return "", fmt.Errorf("not inside a git repository")
	}
	h := sha256.New()

	// Bind evidence to the exact commit being pushed. Without this, any two
	// clean working trees produce identical (empty) diffs and would share a
	// hash, so committing new work over previously-verified clean evidence would
	// look "fresh". On an unborn branch (no commits) there is no HEAD.
	head, err := runBytes(dir, "rev-parse", "HEAD")
	if err != nil {
		head = []byte("(no-head)")
	}
	fmt.Fprintf(h, "HEAD:%d:", len(head))
	h.Write(head)

	parts := [][]string{
		{"diff", "--binary"},
		{"diff", "--cached", "--binary"},
		{"status", "--porcelain"},
	}
	for _, args := range parts {
		full := append([]string{}, args...)
		if len(excludes) > 0 {
			full = append(full, "--", ".")
			for _, e := range excludes {
				full = append(full, ":(exclude)"+e)
			}
		}
		out, err := runBytes(dir, full...)
		if err != nil {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		// Length-prefix each section, labeled by the base args (not the
		// pathspec), so the layout stays stable.
		fmt.Fprintf(h, "%s:%d:", strings.Join(args, " "), len(out))
		h.Write(out)
	}
	if err := hashUntracked(h, dir, excludes); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// hashUntracked closes the freshness hole left by porcelain status: status
// includes an untracked path but not its bytes, so editing the same untracked
// file used to leave evidence looking fresh.
func hashUntracked(h interface{ Write([]byte) (int, error) }, dir string, excludes []string) error {
	args := []string{"ls-files", "--others", "--exclude-standard", "-z"}
	if len(excludes) > 0 {
		args = append(args, "--", ".")
		for _, e := range excludes {
			args = append(args, ":(exclude)"+e)
		}
	}
	out, err := runBytes(dir, args...)
	if err != nil {
		return fmt.Errorf("git ls-files --others: %w", err)
	}
	var paths []string
	for _, raw := range bytes.Split(out, []byte{0}) {
		if len(raw) > 0 {
			paths = append(paths, string(raw))
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		info, err := os.Lstat(full)
		if err != nil {
			return fmt.Errorf("reading untracked file %s: %w", path, err)
		}
		var data []byte
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(full)
			if err != nil {
				return fmt.Errorf("reading untracked symlink %s: %w", path, err)
			}
			data = []byte(target)
		} else {
			data, err = os.ReadFile(full)
			if err != nil {
				return fmt.Errorf("reading untracked file %s: %w", path, err)
			}
		}
		fmt.Fprintf(h, "untracked:%d:%s:mode:%s:size:%d:", len(path), path, info.Mode(), len(data))
		h.Write(data)
	}
	return nil
}

func run(dir string, args ...string) (string, error) {
	out, err := runBytes(dir, args...)
	return string(out), err
}

func runBytes(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
