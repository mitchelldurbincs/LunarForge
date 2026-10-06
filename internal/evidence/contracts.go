package evidence

import "github.com/mitchelldurbincs/lunarforge/internal/config"

// Contract is the result of one executable contract. Its subject and freshness
// are inherited from the containing run; tails are limited to 4096 bytes each.
type Contract struct {
	ID         string `json:"id"`
	Command    string `json:"command"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Reason     string `json:"reason"`
	StdoutTail string `json:"stdout_tail"`
	StderrTail string `json:"stderr_tail"`
	ReusedFrom string `json:"reused_from"`
}

// CheckResult finds evidence for the configured command, never a different
// command that happened to use the same contract ID.
func (e *Evidence) CheckResult(row config.Contract) *Contract {
	if e != nil {
		for i := range e.Contracts {
			if e.Contracts[i].ID == row.ID && e.Contracts[i].Command == row.Check {
				return &e.Contracts[i]
			}
		}
	}
	return nil
}

// CheckConfigMatches also invalidates diff evidence when an external execution
// config changes or checks are added to a run made by an older LF version.
func CheckConfigMatches(cfg *config.Config, ev *Evidence) bool {
	return !cfg.HasChecks() || (ev != nil && ev.CheckDigest == Digest(ExecutionConfig(cfg)))
}

// RequireChecks fails closed for missing, failed, erroneous, or stale checks.
// Declarative requirements retain their existing reporting-only behavior.
func RequireChecks(r Readiness, cfg *config.Config, ev *Evidence) Readiness {
	for _, row := range cfg.Status.Contracts {
		if row.Check == "" {
			continue
		}
		result := ev.CheckResult(row)
		state := "pending"
		if result != nil {
			state = result.Status
			if !r.Fresh || !CheckConfigMatches(cfg, ev) {
				state = "stale"
			}
		}
		if state != ResultPassed && r.ContractFailure == "" {
			r.ContractFailure = "contract " + row.ID + ": " + state
		}
	}
	return r
}
