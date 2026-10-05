package gitutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Subject identifies the full committed snapshot and any working-tree changes.
type Subject struct {
	Commit string `json:"commit"`
	Tree   string `json:"tree"`
	Dirty  bool   `json:"dirty"`
}

// ReadSubject captures HEAD and tracked/untracked cleanliness, excluding LF artifacts.
func ReadSubject(dir string, excludes ...string) (Subject, error) {
	head, err := run(dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Subject{}, err
	}
	tree, err := run(dir, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return Subject{}, err
	}
	excludes = append([]string{".lf"}, excludes...)
	status, err := run(dir, excludingPaths([]string{"status", "--porcelain"}, excludes)...)
	if err != nil {
		return Subject{}, err
	}
	return Subject{Commit: strings.TrimSpace(head), Tree: strings.TrimSpace(tree), Dirty: strings.TrimSpace(status) != ""}, nil
}

// IsolatedCheckout copies local Git objects into a disposable checkout. It never
// registers a worktree or changes the source repository. Ignored and untracked
// source files are not copied. Submodules are refused until supported explicitly.
func IsolatedCheckout(repo, commit string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "lunarforge-checkout-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if _, err = run(repo, "clone", "--local", "--no-hardlinks", "--no-checkout", "--", repo, dir); err == nil {
		_, err = run(dir, "-c", "core.hooksPath="+filepath.Join(dir, "disabled-hooks"), "checkout", "--detach", commit)
	}
	if err == nil {
		var entries string
		entries, err = run(dir, "ls-files", "--stage")
		for _, line := range strings.Split(entries, "\n") {
			if strings.HasPrefix(line, "160000 ") {
				err = fmt.Errorf("commit verification does not support submodules")
				break
			}
		}
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

// PorcelainDirty reports source changes in saved porcelain, including legacy
// records containing LF artifacts. Both sides of renames must be excluded.
func PorcelainDirty(status string, excludes ...string) bool {
	excludes = append([]string{".lf"}, excludes...)
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		if len(line) < 4 {
			return true
		}
		paths := []string{line[3:]}
		if line[0] == 'R' || line[0] == 'C' || line[1] == 'R' || line[1] == 'C' {
			paths = strings.Split(line[3:], " -> ")
		}
		for _, path := range paths {
			if strings.HasPrefix(path, "\"") {
				decoded, err := strconv.Unquote(path)
				if err != nil {
					return true
				}
				path = decoded
			}
			excluded := false
			for _, exclude := range excludes {
				if path == exclude || strings.HasPrefix(path, exclude+"/") {
					excluded = true
					break
				}
			}
			if !excluded {
				return true
			}
		}
	}
	return false
}
