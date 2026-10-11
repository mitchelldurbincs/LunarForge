package evidence

import (
	"strings"
	"testing"
)

func TestValidateRejectsInvalidCheckLogs(t *testing.T) {
	for _, tc := range []struct{ name, state, reason, stdout, stderr, diagnostic string }{
		{"stdout", ResultPassed, ReasonChecksPassed,
			"checks/other.stdout.txt", "checks/test.stderr.txt", "invalid log paths"},
		{"stderr", ResultPassed, ReasonChecksPassed,
			"checks/test.stdout.txt", "checks/other.stderr.txt", "invalid log paths"},
		{"skipped-stdout", ResultSkipped, ReasonPreviousCheckFailed,
			"checks/test.stdout.txt", "", "must not have log paths"},
		{"skipped-stderr", ResultSkipped, ReasonPreviousCheckFailed,
			"", "checks/test.stderr.txt", "must not have log paths"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := validEvidence("run")
			ev.Commands[0].Result, ev.Commands[0].Reason = tc.state, tc.reason
			ev.Commands[0].StdoutPath, ev.Commands[0].StderrPath = tc.stdout, tc.stderr

			err := validate(ev)

			if err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("validation error = %v, want %q", err, tc.diagnostic)
			}
		})
	}
}
