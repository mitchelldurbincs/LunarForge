package evidence

import (
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

func TestReadinessNamesFailedContractOnBlockedSubject(t *testing.T) {
	// Arrange.
	r := Readiness{Blocked: "dirty_subject", ContractFailure: "contract format: error"}
	// Act.
	reason := r.Reason()
	// Assert.
	if r.Ready() || !strings.Contains(reason, "dirty_subject") || !strings.Contains(reason, "format") {
		t.Fatalf("lost readiness reason: %s", reason)
	}
}

func TestEvaluateCommitNeverReusesChecksAcrossCommits(t *testing.T) {
	// Arrange.
	original := identityFixture()
	ev := &Evidence{Mode: "commit", Result: ResultPassed, Identity: &original, EndIdentity: &original, SubjectVerified: true, Contracts: []Contract{{ID: "check", Status: ResultPassed}}}
	want := original
	want.Subject.Commit = "rewritten"
	// Act.
	r := EvaluateCommit(ev, want, true)
	// Assert.
	if r.Fresh || r.Ready() || r.ReusedFrom != "" {
		t.Fatalf("check crossed commit identity: %+v", r)
	}
}

func TestRequireChecksRejectsMissingEvidence(t *testing.T) {
	// Arrange: an older passing run has no check record.
	cfg := &config.Config{Status: config.Status{Contracts: []config.Contract{{ID: "new-check", Check: "true"}}}}
	ev := &Evidence{Result: ResultPassed, DiffHash: "same"}
	// Act.
	r := RequireChecks(Evaluate(ev, "same"), cfg, ev)
	// Assert.
	if r.Ready() || r.Reason() != "contract new-check: pending" {
		t.Fatalf("missing check accepted: %+v", r)
	}
}

func TestExecutionConfigPreservesChecksAndIgnoresObservations(t *testing.T) {
	// Arrange.
	cfg := &config.Config{Status: config.Status{Contracts: []config.Contract{{ID: "check", Check: "true"}, {ID: "remote", Status: "enforced-remotely", ADO: &config.ADOObservation{BuildID: 1}}}}}
	original := Digest(ExecutionConfig(cfg))
	// Act.
	cfg.Status.Contracts[1].ADO.BuildID = 2
	observationDigest := Digest(ExecutionConfig(cfg))
	cfg.Status.Contracts[0].Check = "exit 1"
	checkDigest := Digest(ExecutionConfig(cfg))
	// Assert.
	if original != observationDigest || original == checkDigest {
		t.Fatal("execution digest did not distinguish checks from reporting observations")
	}
}
