package evidence

// Readiness is the push-gate decision derived from the latest evidence and the
// current repo diff hash. It is the single source of truth for whether the repo
// is ready to push, shared by `lf status` and the pre-push hook.
type Readiness struct {
	HasEvidence bool   // latest evidence exists and could be loaded
	Passed      bool   // latest evidence result == passed
	Fresh       bool   // evidence diff hash matches the current diff hash
	EvidenceID  string // run id of the evaluated evidence (empty if none)
	EvidenceDir string // run dir of the evaluated evidence (empty if none)
	WantHash    string // current diff hash
	HaveHash    string // diff hash recorded in evidence (empty if none)
}

// Ready reports whether the repo is ready to push: fresh, passing evidence
// exists. This is the invariant the pre-push gate enforces.
func (r Readiness) Ready() bool {
	return r.HasEvidence && r.Passed && r.Fresh
}

// Reason returns a short machine-stable reason code for the current state.
func (r Readiness) Reason() string {
	switch {
	case !r.HasEvidence:
		return "no_evidence"
	case !r.Passed:
		return "failed"
	case !r.Fresh:
		return "stale"
	default:
		return "ready"
	}
}

// Evaluate derives a Readiness from the latest evidence (which may be nil when
// none exists) and the current diff hash.
func Evaluate(ev *Evidence, currentHash string) Readiness {
	r := Readiness{WantHash: currentHash}
	if ev == nil {
		return r
	}
	r.HasEvidence = true
	r.Passed = ev.Passed()
	r.HaveHash = ev.DiffHash
	r.Fresh = ev.DiffHash == currentHash
	r.EvidenceID = ev.RunID
	return r
}
