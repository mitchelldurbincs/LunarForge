package evidence

import (
	"fmt"
	"path/filepath"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

func validate(e *Evidence) error {
	if e.Version != SchemaVersion {
		return fmt.Errorf("unsupported evidence schema version %d (expected %d)", e.Version, SchemaVersion)
	}
	if !validRunID(e.RunID) {
		return fmt.Errorf("invalid evidence run id %q", e.RunID)
	}
	if !validResult(e.Result, false) {
		return fmt.Errorf("invalid evidence state %q", e.Result)
	}
	if !validStateReason(e.Result, e.Reason, false) {
		return fmt.Errorf("invalid evidence state/reason %q/%q", e.Result, e.Reason)
	}
	if e.DiffHash == "" || e.FinalDiffHash == "" {
		return fmt.Errorf("evidence fingerprints are required")
	}
	if len(e.Commands) == 0 {
		return fmt.Errorf("evidence must contain at least one check")
	}
	hasFailed, hasBlocked, allPassed := false, false, true
	for _, c := range e.Commands {
		if err := validateCommand(c); err != nil {
			return err
		}
		hasFailed = hasFailed || c.Result == ResultFailed
		hasBlocked = hasBlocked || c.Result == ResultBlocked
		allPassed = allPassed && c.Result == ResultPassed
	}
	switch e.Result {
	case ResultPassed:
		if !allPassed {
			return fmt.Errorf("passing evidence contains a non-passing check")
		}
	case ResultFailed:
		if !hasFailed || hasBlocked {
			return fmt.Errorf("failed evidence must contain a failure and no blocked check")
		}
	case ResultBlocked:
		if !hasBlocked {
			return fmt.Errorf("blocked evidence has no blocked check")
		}
	}
	return nil
}

func validResult(result string, allowSkipped bool) bool {
	switch result {
	case ResultPassed, ResultFailed, ResultStale, ResultBlocked, ResultError:
		return true
	case ResultSkipped:
		return allowSkipped
	default:
		return false
	}
}

func validReason(reason string) bool {
	switch reason {
	case ReasonChecksPassed, ReasonCheckFailed, ReasonSnapshotChanged,
		ReasonNoEvidence, ReasonConfigInvalid, ReasonToolUnavailable,
		ReasonTimedOut, ReasonEvidenceCorrupt, ReasonPreviousCheckFailed,
		ReasonPreviousCheckBlocked, ReasonInternalError:
		return true
	default:
		return false
	}
}

func validStateReason(state, reason string, check bool) bool {
	if !validReason(reason) {
		return false
	}
	switch state {
	case ResultPassed:
		return reason == ReasonChecksPassed
	case ResultFailed:
		return reason == ReasonCheckFailed
	case ResultStale:
		return !check && reason == ReasonSnapshotChanged
	case ResultBlocked:
		return reason == ReasonToolUnavailable || reason == ReasonTimedOut
	case ResultSkipped:
		return check && (reason == ReasonPreviousCheckFailed || reason == ReasonPreviousCheckBlocked)
	case ResultError:
		return !check && (reason == ReasonInternalError || reason == ReasonEvidenceCorrupt)
	default:
		return false
	}
}

func validateCommand(c Command) error {
	if err := config.ValidateCheckID(c.ID); err != nil {
		return fmt.Errorf("invalid evidence: %w", err)
	}
	if !validResult(c.Result, true) {
		return fmt.Errorf("check %q has invalid state %q", c.ID, c.Result)
	}
	if !validStateReason(c.Result, c.Reason, true) {
		return fmt.Errorf("check %q has invalid state/reason %q/%q", c.ID, c.Result, c.Reason)
	}
	if c.Result == ResultSkipped {
		if c.StdoutPath != "" || c.StderrPath != "" {
			return fmt.Errorf("skipped check %q must not have log paths", c.ID)
		}
		return nil
	}
	wantStdout := filepath.ToSlash(CheckStdoutPath(c.ID))
	wantStderr := filepath.ToSlash(CheckStderrPath(c.ID))
	if filepath.ToSlash(c.StdoutPath) != wantStdout || filepath.ToSlash(c.StderrPath) != wantStderr {
		return fmt.Errorf("check %q has invalid log paths", c.ID)
	}
	return nil
}
