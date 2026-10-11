package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

// loaded bundles the config together with resolved repo + evidence paths so
// each command doesn't repeat the same resolution dance.
type loaded struct {
	cfg         *config.Config
	repoDir     string
	evidenceDir string // absolute
}

// load finds and loads the config, confirms we're in a git repo, and resolves
// the evidence directory to an absolute path.
func load() (*loaded, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadFromDir(cwd)
	if err != nil {
		return nil, err
	}
	// The repo root is the directory containing the config file.
	repoDir := filepath.Dir(cfg.Path())

	if !gitutil.IsRepo(repoDir) {
		return nil, fmt.Errorf("%s is not inside a git repository", repoDir)
	}

	evidenceDir := filepath.Join(repoDir, evidence.DefaultDir)
	return &loaded{cfg: cfg, repoDir: repoDir, evidenceDir: evidenceDir}, nil
}

// excludes returns the repo-relative pathspecs to keep out of the diff hash so
// LunarForge's own evidence artifacts never invalidate the evidence.
func (l *loaded) excludes() []string {
	return evidence.ArtifactExcludes(l.repoDir, l.evidenceDir)
}

// currentDiffHash computes the diff hash for the repo, excluding LunarForge
// artifacts. This must match how the runner computes it at verify time.
func (l *loaded) currentDiffHash() (string, error) {
	return gitutil.DiffHash(l.repoDir, l.excludes()...)
}

// parseFlags stops command execution after help or a parse error. FlagSet has
// already printed the usage or diagnostic, so main must not print it again.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return &exitError{code: 0}
		}
		return &exitError{code: 2}
	}
	return nil
}

// commandFailure preserves text-mode errors and emits the shared JSON envelope
// and state-derived exit code for machine-readable commands.
func commandFailure(asJSON bool, command, state, reason string, err error) error {
	if !asJSON {
		return err
	}
	writeJSON(jsonFailure(command, state, reason, err))
	return &exitError{code: exitCodeForState(state)}
}
