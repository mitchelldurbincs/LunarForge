package config

import (
	"strings"
	"testing"
)

func TestLoadCheckContracts(t *testing.T) {
	// Arrange.
	path := writeConfig(t, t.TempDir(), StarterTemplate("demo")+`
status:
  contracts:
    - id: windows-restructure
      check: dotnet checker.dll --no-project --check Parts/
      reason: "exit 0 = clean; exit 1 = changes needed"
    - id: windows
      platform: windows
      status: pending
    - id: coverage
      status: enforced-remotely
      ado:
        definition_id: 111
`)
	// Act.
	cfg, err := Load(path)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Status.Contracts) != 3 || !cfg.HasChecks() || cfg.Status.Contracts[0].Check != "dotnet checker.dll --no-project --check Parts/" || cfg.Status.Contracts[1].Status != "pending" || cfg.Status.Contracts[2].ADO.DefinitionID != 111 {
		t.Fatalf("unexpected contracts: %+v", cfg.Status.Contracts)
	}
}

func TestLoadRejectsInvalidCheckContracts(t *testing.T) {
	for _, tc := range []struct{ name, row string }{
		{"platform", "id: check\n      check: 'true'\n      platform: windows"},
		{"status", "id: check\n      check: 'true'\n      status: pending"},
		{"ado", "id: check\n      check: 'true'\n      ado: {}"},
		{"empty", "id: check\n      check: ''"},
		{"empty with status", "id: check\n      check: ''\n      status: pending"},
		{"empty platform", "id: check\n      check: 'true'\n      platform: ''"},
		{"whitespace", "id: check\n      check: '  '"},
		{"traversal", "id: ../outside\n      check: 'true'"},
		{"backslash", "id: ..\\outside\n      check: 'true'"},
		{"dot", "id: .\n      check: 'true'"},
		{"duplicate", "id: check\n      check: 'true'\n    - id: check\n      check: 'true'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			path := writeConfig(t, t.TempDir(), StarterTemplate("demo")+"\nstatus:\n  contracts:\n    - "+tc.row+"\n")
			// Act.
			_, err := Load(path)
			// Assert.
			if err == nil || !strings.Contains(err.Error(), "contract") {
				t.Fatalf("invalid contract accepted: %v", err)
			}
		})
	}
}
