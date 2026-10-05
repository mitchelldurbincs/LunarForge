package runner

import (
	"fmt"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
)

func prepareCommit(cfg *config.Config, sourceDir, commit string) (*evidence.Identity, string, func(), error) {
	if commit != "HEAD" {
		return nil, "", nil, fmt.Errorf("--commit supports HEAD only")
	}
	identity, err := evidence.CaptureIdentity(cfg, sourceDir)
	if err != nil {
		return nil, "", nil, err
	}
	if identity.Subject.Dirty {
		return nil, "", nil, fmt.Errorf("commit verification requires a clean worktree and index")
	}
	isolated, cleanup, err := gitutil.IsolatedCheckout(sourceDir, identity.Subject.Commit)
	if err != nil {
		return nil, "", nil, err
	}
	// Resolve tools in the actual execution checkout as well as the invocation.
	execution, err := captureExecution(cfg, isolated, identity.Repository)
	if err == nil && !sameIdentity(identity, execution) {
		err = fmt.Errorf("isolated checkout identity differs from source")
	}
	if err != nil {
		cleanup()
		return nil, "", nil, err
	}
	return identity, isolated, cleanup, nil
}

func captureExecution(cfg *config.Config, dir, repository string) (*evidence.Identity, error) {
	identity, err := evidence.CaptureIdentity(cfg, dir)
	if err == nil {
		identity.Repository = repository
	}
	return identity, err
}

func sameIdentity(a, b *evidence.Identity) bool {
	return a.SameInputs(*b) && a.Subject == b.Subject
}

func verifySubject(cfg *config.Config, sourceDir, executionDir string, ev *evidence.Evidence) bool {
	var err error
	if cfg.Path() != "" {
		cfg, err = config.Load(cfg.Path())
	}
	if err == nil {
		ev.EndIdentity, err = evidence.CaptureIdentity(cfg, sourceDir)
	}
	if err == nil {
		var execution *evidence.Identity
		execution, err = captureExecution(cfg, executionDir, ev.Identity.Repository)
		if err == nil {
			ev.ExecutionEndIdentity = execution
			ev.SubjectVerified = sameIdentity(ev.Identity, ev.EndIdentity) && sameIdentity(ev.Identity, execution)
		}
	}
	if err != nil {
		ev.SubjectError = err.Error()
	}
	if !ev.SubjectVerified && ev.SubjectError == "" {
		ev.SubjectError = "subject, contract, or platform changed during verification"
	}
	return ev.SubjectVerified
}
