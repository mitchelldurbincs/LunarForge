package evidence

// Readiness is the decision derived from saved evidence and the current source
// fingerprint. Loading/corruption errors are handled before evaluation.
type Readiness struct {
	HasEvidence bool
	Passed      bool
	Fresh       bool
	EvidenceID  string
	EvidenceDir string
	WantHash    string
	HaveHash    string
	StateCode   string
	ReasonCode  string
}

func (r Readiness) Ready() bool    { return r.StateCode == ResultPassed }
func (r Readiness) State() string  { return r.StateCode }
func (r Readiness) Reason() string { return r.ReasonCode }

func Evaluate(ev *Evidence, currentHash string) Readiness {
	r := Readiness{
		WantHash:   currentHash,
		StateCode:  ResultBlocked,
		ReasonCode: ReasonNoEvidence,
	}
	if ev == nil {
		return r
	}
	r.HasEvidence = true
	r.Passed = ev.Passed()
	r.HaveHash = ev.DiffHash
	r.Fresh = ev.DiffHash == currentHash && ev.FinalDiffHash == currentHash
	r.EvidenceID = ev.RunID
	if !r.Fresh || ev.Result == ResultStale {
		r.StateCode = ResultStale
		r.ReasonCode = ReasonSnapshotChanged
		return r
	}
	r.StateCode = ev.Result
	r.ReasonCode = ev.Reason
	return r
}
