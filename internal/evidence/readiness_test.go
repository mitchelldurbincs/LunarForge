package evidence

import "testing"

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name, current, wantState, wantReason string
		ev                                   *Evidence
	}{
		{"missing", "b", ResultBlocked, ReasonNoEvidence, nil},
		{"fresh pass", "a", ResultPassed, ReasonChecksPassed, &Evidence{RunID: "r", Result: ResultPassed, Reason: ReasonChecksPassed, DiffHash: "a", FinalDiffHash: "a"}},
		{"current changed", "b", ResultStale, ReasonSnapshotChanged, &Evidence{RunID: "r", Result: ResultPassed, Reason: ReasonChecksPassed, DiffHash: "a", FinalDiffHash: "a"}},
		{"changed during run", "b", ResultStale, ReasonSnapshotChanged, &Evidence{RunID: "r", Result: ResultStale, Reason: ReasonSnapshotChanged, DiffHash: "a", FinalDiffHash: "b"}},
		{"fresh fail", "a", ResultFailed, ReasonCheckFailed, &Evidence{RunID: "r", Result: ResultFailed, Reason: ReasonCheckFailed, DiffHash: "a", FinalDiffHash: "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Evaluate(tc.ev, tc.current)
			if r.State() != tc.wantState || r.Reason() != tc.wantReason {
				t.Fatalf("got %s/%s, want %s/%s", r.State(), r.Reason(), tc.wantState, tc.wantReason)
			}
			if r.Ready() != (tc.wantState == ResultPassed) {
				t.Fatalf("Ready = %v", r.Ready())
			}
		})
	}
}
