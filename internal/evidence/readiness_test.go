package evidence

import "testing"

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name        string
		ev          *Evidence
		currentHash string
		wantReady   bool
		wantReason  string
	}{
		{
			name:        "no evidence",
			ev:          nil,
			currentHash: "sha256:aaa",
			wantReady:   false,
			wantReason:  "no_evidence",
		},
		{
			name:        "passed and fresh -> ready",
			ev:          &Evidence{Result: ResultPassed, DiffHash: "sha256:aaa", RunID: "r1"},
			currentHash: "sha256:aaa",
			wantReady:   true,
			wantReason:  "ready",
		},
		{
			name:        "passed but stale -> not ready",
			ev:          &Evidence{Result: ResultPassed, DiffHash: "sha256:aaa", RunID: "r1"},
			currentHash: "sha256:bbb",
			wantReady:   false,
			wantReason:  "stale",
		},
		{
			name:        "failed even if fresh -> not ready",
			ev:          &Evidence{Result: ResultFailed, DiffHash: "sha256:aaa", RunID: "r1"},
			currentHash: "sha256:aaa",
			wantReady:   false,
			wantReason:  "failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Evaluate(tc.ev, tc.currentHash)
			if r.Ready() != tc.wantReady {
				t.Errorf("Ready() = %v, want %v", r.Ready(), tc.wantReady)
			}
			if r.Reason() != tc.wantReason {
				t.Errorf("Reason() = %q, want %q", r.Reason(), tc.wantReason)
			}
		})
	}
}
