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

// freshness reports whether ev matches the current diff hash for repoDir.
func freshness(repoDir string, ev *evidence.Evidence) (bool, string, error) {
	currentHash, err := gitutil.DiffHash(repoDir)
	if err != nil {
		return false, "", err
	}
	return currentHash == ev.DiffHash, currentHash, nil
}
