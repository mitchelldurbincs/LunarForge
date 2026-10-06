package main

import (
	"github.com/mitchelldurbincs/lunarforge/internal/config"
	"github.com/mitchelldurbincs/lunarforge/internal/evidence"
)

// checkStatus adds live evidence fields without changing declarative row JSON.
type checkStatus struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Fresh      bool   `json:"fresh"`
	Passed     bool   `json:"passed"`
	Ready      bool   `json:"ready"`
	ExitCode   *int   `json:"exit_code"`
	Reason     string `json:"reason"`
	Command    string `json:"command"`
	RunID      string `json:"run_id"`
	ReusedFrom string `json:"reused_from"`
}

func checkContractRow(row config.Contract, ev *evidence.Evidence, r evidence.Readiness) checkStatus {
	out := checkStatus{ID: row.ID, Status: "pending", Command: row.Check, Reason: "no check evidence"}
	if rec := ev.CheckResult(row); rec != nil {
		out.Status = rec.Status
		out.Fresh = r.Fresh && r.Blocked == ""
		out.Passed = rec.Status == evidence.ResultPassed
		out.Ready = out.Fresh && out.Passed
		out.ExitCode = &rec.ExitCode
		out.Reason = rec.Reason
		out.RunID = ev.RunID
		out.ReusedFrom = rec.ReusedFrom
		if !out.Fresh {
			out.Reason = "stale or blocked for current subject; " + out.Reason
		}
	}
	return out
}
