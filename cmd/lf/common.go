package main

import (
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

	evidenceDir := cfg.EvidenceDir()
	if !filepath.IsAbs(evidenceDir) {
		evidenceDir = filepath.Join(repoDir, evidenceDir)
	}
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

// freshness reports whether ev matches the current diff hash.
func freshness(l *loaded, ev *evidence.Evidence) (bool, string, error) {
	currentHash, err := l.currentDiffHash()
	if err != nil {
		return false, "", err
	}
	return currentHash == ev.DiffHash, currentHash, nil
}
