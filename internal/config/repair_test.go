package config

import "testing"

const repairConfig = `version: 1
project:
  name: demo
verify:
  commands:
    - id: contents
      run: ./scripts/verify.sh
repair:
  enabled: true
  max_attempts: 5
  max_log_chars: 1000
  agent: fake_success
agents:
  fake_success:
    backend: fake
    command: ./scripts/fake-repair-success.sh
  no_command:
    backend: fake
`

func loadRepair(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	path := writeConfig(t, dir, repairConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestRepairDefaultsAndOverrides(t *testing.T) {
	cfg := loadRepair(t)
	if !cfg.RepairEnabled() {
		t.Error("repair should be enabled")
	}
	if cfg.RepairMaxAttempts() != 5 {
		t.Errorf("max attempts = %d, want 5", cfg.RepairMaxAttempts())
	}
	if cfg.RepairMaxLogChars() != 1000 {
		t.Errorf("max log chars = %d, want 1000", cfg.RepairMaxLogChars())
	}
	if !cfg.RepairVerifyAfterEach() {
		t.Error("verify_after_each_attempt should default to true when unset")
	}
}

func TestRepairDefaultsWhenAbsent(t *testing.T) {
	cfg := &Config{} // nothing configured
	if !cfg.RepairEnabled() {
		t.Error("repair should default to enabled when unset")
	}
	if cfg.RepairMaxAttempts() != 3 {
		t.Errorf("default max attempts = %d, want 3", cfg.RepairMaxAttempts())
	}
	if cfg.RepairMaxLogChars() != 20000 {
		t.Errorf("default max log chars = %d, want 20000", cfg.RepairMaxLogChars())
	}
}

func TestRepairEnabledFalse(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "version: 1\nproject:\n  name: x\nverify:\n  commands:\n    - id: a\n      run: echo hi\nrepair:\n  enabled: false\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RepairEnabled() {
		t.Error("repair.enabled: false should disable repair")
	}
}

func TestResolveAgent(t *testing.T) {
	cfg := loadRepair(t)

	// Default agent from repair.agent.
	name, a, err := cfg.ResolveAgent("")
	if err != nil {
		t.Fatalf("ResolveAgent default: %v", err)
	}
	if name != "fake_success" || a.Command == "" {
		t.Errorf("unexpected default agent: %s %+v", name, a)
	}

	// Explicit override.
	if _, _, err := cfg.ResolveAgent("no_command"); err == nil {
		t.Error("expected error for agent with no command")
	}
	if _, _, err := cfg.ResolveAgent("missing"); err == nil {
		t.Error("expected error for undefined agent")
	}
}

func TestResolveAgentNoneConfigured(t *testing.T) {
	cfg := &Config{}
	if _, _, err := cfg.ResolveAgent(""); err == nil {
		t.Error("expected error when no agent configured")
	}
}
