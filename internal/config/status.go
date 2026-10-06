package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Status declares local checks and additional reporting requirements.
type Status struct {
	Contracts []Contract `yaml:"contracts"`
}

// Contract describes a command check, pending platform, or remote gate.
type Contract struct {
	ID        string          `yaml:"id" json:"id"`
	Check     string          `yaml:"check" json:"check,omitempty"`
	Platform  string          `yaml:"platform" json:"platform,omitempty"`
	Status    string          `yaml:"status" json:"status"`
	Reason    string          `yaml:"reason" json:"reason,omitempty"`
	Authority string          `yaml:"authority" json:"authority,omitempty"`
	ADO       *ADOObservation `yaml:"ado" json:"ado,omitempty"`
}

// UnmarshalYAML rejects an explicitly empty check and mixed forms, including
// explicitly empty declarative keys that would otherwise disappear on decode.
func (c *Contract) UnmarshalYAML(node *yaml.Node) error {
	type plain Contract
	if err := node.Decode((*plain)(c)); err != nil {
		return err
	}
	var check, declaration bool
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "check":
			check = true
		case "platform", "status", "ado":
			declaration = true
		}
	}
	if check && (strings.TrimSpace(c.Check) == "" || declaration) {
		return fmt.Errorf("status contract %s check must be nonempty and cannot be combined with platform, status, or ado", c.ID)
	}
	return nil
}

// HasChecks reports whether verification includes executable contracts.
func (c *Config) HasChecks() bool {
	for _, row := range c.Status.Contracts {
		if row.Check != "" {
			return true
		}
	}
	return false
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
