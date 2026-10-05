package config

// Status declares additional reporting requirements; these are never fabricated
// local passes. An orchestrator supplies remote observations independently.
type Status struct {
	Contracts []Contract `yaml:"contracts"`
}

// Contract describes a pending platform or a separately enforced remote gate.
type Contract struct {
	ID        string          `yaml:"id" json:"id"`
	Platform  string          `yaml:"platform" json:"platform,omitempty"`
	Status    string          `yaml:"status" json:"status"`
	Reason    string          `yaml:"reason" json:"reason,omitempty"`
	Authority string          `yaml:"authority" json:"authority,omitempty"`
	ADO       *ADOObservation `yaml:"ado" json:"ado,omitempty"`
}

// ADOObservation is an externally supplied observation, not an LF assertion of
// remote readiness. Source, target, and tested merge identities stay separate.
type ADOObservation struct {
	BuildID      int    `yaml:"build_id" json:"build_id,omitempty"`
	DefinitionID int    `yaml:"definition_id" json:"definition_id,omitempty"`
	SourceCommit string `yaml:"source_commit" json:"source_commit,omitempty"`
	TargetCommit string `yaml:"target_commit" json:"target_commit,omitempty"`
	MergeCommit  string `yaml:"merge_commit" json:"merge_commit,omitempty"`
	Result       string `yaml:"result" json:"result,omitempty"`
	ObservedAt   string `yaml:"observed_at" json:"observed_at,omitempty"`
}
