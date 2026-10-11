package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func contractSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "schemas", name+"-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	schema, err := c.Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validateContract(t *testing.T, schema *jsonschema.Schema, raw []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("stdout must contain exactly one JSON object: %v\n%s", err, raw)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("response violates the published schema: %v\n%s", err, raw)
	}
	return value
}

// Exercise the executable boundary used by Yuheng: stdout and process exit
// status are separate, and valid non-passing envelopes must still be decoded.
func TestCLIConsumerContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture checks use POSIX shell commands")
	}
	cli := contractCLI{binary: buildCLI(t), schema: contractSchema(t, "result")}
	t.Run("passing", cli.testPassing)
	t.Run("failed-and-skipped", cli.testFailed)
	t.Run("blocked-and-skipped", cli.testUnavailable)
	t.Run("timed-out", cli.testTimeout)
	t.Run("status-stale", cli.testStaleStatus)
	t.Run("verify-stale", cli.testStaleVerify)
	t.Run("no-evidence", cli.testNoEvidence)
	t.Run("config-invalid", cli.testInvalidConfig)
	t.Run("evidence-corrupt", cli.testCorruptEvidence)
	t.Run("legacy-evidence", cli.testLegacyEvidence)
}

type contractCLI struct {
	binary string
	schema *jsonschema.Schema
}

func buildCLI(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "lf")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return binary
}

func invokeCLI(t *testing.T, binary, repo string, wantExit int, args ...string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if code != wantExit {
		t.Fatalf("%v: exit %d, want %d\nstdout: %s\nstderr: %s", args, code, wantExit, &stdout, &stderr)
	}
	return stdout.String(), stderr.String()
}

func (cli contractCLI) invoke(t *testing.T, repo string, wantExit int, command string) map[string]any {
	t.Helper()
	stdout, _ := invokeCLI(t, cli.binary, repo, wantExit, command, "--json")
	return validateContract(t, cli.schema, []byte(stdout))
}

func writeContractFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contractHead(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func assertEnvelope(t *testing.T, got map[string]any, command, state, reason string, wantRun bool) {
	t.Helper()
	if got["command"] != command || got["state"] != state || got["reason"] != reason {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	_, hasRun := got["run"]
	_, hasChecks := got["checks"]
	if hasRun != wantRun || hasChecks != wantRun {
		t.Fatalf("run/check presence must match the outcome: %+v", got)
	}
}

func assertCurrentHead(t *testing.T, got map[string]any, head string) {
	t.Helper()
	current := got["repository"].(map[string]any)
	if current["head"] != head {
		t.Fatalf("repository.head = %v, want full Git ID %s", current["head"], head)
	}
}

func assertRunHead(t *testing.T, got map[string]any, head string) {
	t.Helper()
	run := got["run"].(map[string]any)
	if run["head"] != head {
		t.Fatalf("run.head = %v, want %s", run["head"], head)
	}
}

func assertPassingSnapshot(t *testing.T, got map[string]any, dirty bool) {
	t.Helper()
	current := got["repository"].(map[string]any)
	run := got["run"].(map[string]any)
	if current["fingerprint"] != run["fingerprint"] || run["fingerprint"] != run["final_fingerprint"] {
		t.Fatalf("passing evidence must bind to the current snapshot: %+v", got)
	}
	if current["dirty"] != dirty || run["dirty"] != dirty {
		t.Fatalf("dirty must be an explicit, accurate boolean: %+v", got)
	}
}

const contractPassingConfig = `version: 1
verify:
  commands:
    - id: ok
      run: echo check-output
`

func (cli contractCLI) testPassing(t *testing.T) {
	for _, command := range []string{"verify", "ci"} {
		t.Run(command, func(t *testing.T) {
			repo := newTestRepo(t, contractPassingConfig)
			got := cli.invoke(t, repo, 0, command)
			assertEnvelope(t, got, command, "pass", "checks_passed", true)
			assertCurrentHead(t, got, contractHead(t, repo))
			assertRunHead(t, got, contractHead(t, repo))
			assertPassingSnapshot(t, got, false)
		})
	}
	t.Run("status", func(t *testing.T) {
		repo := newTestRepo(t, contractPassingConfig)
		cli.invoke(t, repo, 0, "verify")
		got := cli.invoke(t, repo, 0, "status")
		assertEnvelope(t, got, "status", "pass", "checks_passed", true)
		assertCurrentHead(t, got, contractHead(t, repo))
		assertRunHead(t, got, contractHead(t, repo))
		assertPassingSnapshot(t, got, false)
	})
	t.Run("dirty", func(t *testing.T) {
		repo := newTestRepo(t, contractPassingConfig)
		writeContractFile(t, filepath.Join(repo, "marker.txt"), "changed\n")
		got := cli.invoke(t, repo, 0, "verify")
		assertEnvelope(t, got, "verify", "pass", "checks_passed", true)
		assertCurrentHead(t, got, contractHead(t, repo))
		assertRunHead(t, got, contractHead(t, repo))
		assertPassingSnapshot(t, got, true)
	})
}

func assertSkippedCheck(t *testing.T, got map[string]any) {
	t.Helper()
	checks := got["checks"].([]any)
	last := checks[len(checks)-1].(map[string]any)
	if last["state"] != "skipped" || last["exit_code"] != float64(-1) {
		t.Fatalf("unexecuted check must be explicit: %+v", last)
	}
}

func (cli contractCLI) testFailed(t *testing.T) {
	repo := newTestRepo(t, failingConfig)
	got := cli.invoke(t, repo, 1, "verify")
	assertEnvelope(t, got, "verify", "fail", "check_failed", true)
	assertCurrentHead(t, got, contractHead(t, repo))
	assertRunHead(t, got, contractHead(t, repo))
	assertSkippedCheck(t, got)
}

func (cli contractCLI) testUnavailable(t *testing.T) {
	repo := newTestRepo(t, `version: 1
verify:
  commands:
    - id: missing
      run: lunarforge_nonexistent_contract_check
    - id: later
      run: echo skipped
`)
	got := cli.invoke(t, repo, 2, "verify")
	assertEnvelope(t, got, "verify", "blocked", "tool_unavailable", true)
	assertCurrentHead(t, got, contractHead(t, repo))
	assertRunHead(t, got, contractHead(t, repo))
	assertSkippedCheck(t, got)
}

func (cli contractCLI) testTimeout(t *testing.T) {
	repo := newTestRepo(t, `version: 1
verify:
  commands:
    - id: slow
      run: exec sleep 5
      timeout_seconds: 1
`)
	got := cli.invoke(t, repo, 2, "verify")
	assertEnvelope(t, got, "verify", "blocked", "timed_out", true)
	assertCurrentHead(t, got, contractHead(t, repo))
	assertRunHead(t, got, contractHead(t, repo))
}

func (cli contractCLI) testStaleStatus(t *testing.T) {
	repo := newTestRepo(t, contractPassingConfig)
	cli.invoke(t, repo, 0, "verify")
	writeContractFile(t, filepath.Join(repo, "marker.txt"), "changed\n")
	got := cli.invoke(t, repo, 1, "status")
	assertEnvelope(t, got, "status", "stale", "snapshot_changed", true)
	assertCurrentHead(t, got, contractHead(t, repo))
	assertRunHead(t, got, contractHead(t, repo))
	current := got["repository"].(map[string]any)
	run := got["run"].(map[string]any)
	if run["state"] != "pass" || current["fingerprint"] == run["fingerprint"] {
		t.Fatal("status must distinguish historical success from current freshness")
	}
}

func (cli contractCLI) testStaleVerify(t *testing.T) {
	repo := newTestRepo(t, `version: 1
verify:
  commands:
    - id: mutate
      run: echo changed >> marker.txt
`)
	got := cli.invoke(t, repo, 1, "verify")
	assertEnvelope(t, got, "verify", "stale", "snapshot_changed", true)
	assertCurrentHead(t, got, contractHead(t, repo))
	assertRunHead(t, got, contractHead(t, repo))
}

func (cli contractCLI) testNoEvidence(t *testing.T) {
	repo := newTestRepo(t, contractPassingConfig)
	got := cli.invoke(t, repo, 2, "status")
	assertEnvelope(t, got, "status", "blocked", "no_evidence", false)
	assertCurrentHead(t, got, contractHead(t, repo))
}

func (cli contractCLI) testInvalidConfig(t *testing.T) {
	repo := newTestRepo(t, "version: 999\n")
	got := cli.invoke(t, repo, 2, "verify")
	assertEnvelope(t, got, "verify", "blocked", "config_invalid", false)
	if got["error"] == nil {
		t.Fatal("early failure must include error detail")
	}
}

func (cli contractCLI) testCorruptEvidence(t *testing.T) {
	repo := newTestRepo(t, contractPassingConfig)
	cli.invoke(t, repo, 0, "verify")
	writeContractFile(t, filepath.Join(latestEvidenceDir(t, repo), "evidence.json"), "{invalid")
	got := cli.invoke(t, repo, 3, "status")
	assertEnvelope(t, got, "status", "error", "evidence_corrupt", false)
	if got["error"] == nil {
		t.Fatal("early failure must include error detail")
	}
}

func (cli contractCLI) testLegacyEvidence(t *testing.T) {
	repo := newTestRepo(t, contractPassingConfig)
	cli.invoke(t, repo, 0, "verify")
	path := filepath.Join(latestEvidenceDir(t, repo), "evidence.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	head := contractHead(t, repo)
	// Previous schema-1 evidence remains readable after upgrading.
	writeContractFile(t, path, strings.ReplaceAll(string(data), head, head[:7]))
	got := cli.invoke(t, repo, 0, "status")
	assertEnvelope(t, got, "status", "pass", "checks_passed", true)
	assertCurrentHead(t, got, head)
	assertRunHead(t, got, head[:7])
	assertPassingSnapshot(t, got, false)
}

func TestPublishedContractFixtures(t *testing.T) {
	schema := contractSchema(t, "result")
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "contracts", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("missing response fixtures: %v", err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			validateContract(t, schema, data)
		})
	}
}

func TestResultSchemaRequiresExplicitEvidence(t *testing.T) {
	schema := contractSchema(t, "result")
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "contracts", "status-pass.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema_version", "run", "checks", "dirty", "exit_code"} {
		t.Run(field, func(t *testing.T) {
			value := validateContract(t, schema, data)
			switch field {
			case "dirty":
				delete(value["repository"].(map[string]any), field)
			case "exit_code":
				delete(value["checks"].([]any)[0].(map[string]any), field)
			default:
				delete(value, field)
			}
			if err := schema.Validate(value); err == nil {
				t.Fatalf("schema accepted passing evidence without %s", field)
			}
		})
	}
	value := validateContract(t, schema, data)
	value["future_field"] = "additive extensions are allowed"
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSchema(t *testing.T) {
	schema := contractSchema(t, "config")
	for _, tc := range []struct {
		name, policy string
		valid        bool
	}{
		{"minimal", passingConfig, true},
		{"zero-timeout", passingConfig + "      timeout_seconds: 0\n", true},
		{"negative-timeout", passingConfig + "      timeout_seconds: -1\n", false},
		{"unknown-field", passingConfig + "      retry: 3\n", false},
		{"invalid-id", strings.ReplaceAll(passingConfig, "id: ok", "id: ../ok"), false},
		{"empty-command", strings.ReplaceAll(passingConfig, `"true"`, `" "`), false},
		{"empty-checks", "version: 1\nverify:\n  commands: []\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			if err := yaml.Unmarshal([]byte(tc.policy), &value); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); (err == nil) != tc.valid {
				t.Fatalf("schema validation = %v, want valid=%t", err, tc.valid)
			}
		})
	}
}
