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
	schema := contractSchema(t, "result")
	binary := filepath.Join(t.TempDir(), "lf")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	invoke := func(t *testing.T, repo string, wantExit int, args ...string) map[string]any {
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
		return validateContract(t, schema, stdout.Bytes())
	}
	write := func(t *testing.T, path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name, config, command, state, reason string
		exit                                 int
		prepare                              string
		wantRun                              bool
	}{
		{"verify-pass", "", "verify", "pass", "checks_passed", 0, "", true},
		{"status-pass", "", "status", "pass", "checks_passed", 0, "verify", true},
		{"ci-pass", "", "ci", "pass", "checks_passed", 0, "", true},
		{"dirty-pass", "", "verify", "pass", "checks_passed", 0, "dirty", true},
		{"verify-fail", failingConfig, "verify", "fail", "check_failed", 1, "", true},
		{"status-stale", "", "status", "stale", "snapshot_changed", 1, "stale", true},
		{"verify-stale", "version: 1\nverify:\n  commands:\n    - id: mutate\n      run: echo changed >> marker.txt\n", "verify", "stale", "snapshot_changed", 1, "", true},
		{"no-evidence", "", "status", "blocked", "no_evidence", 2, "", false},
		{"config-invalid", "version: 999\n", "verify", "blocked", "config_invalid", 2, "", false},
		{"tool-unavailable", "version: 1\nverify:\n  commands:\n    - id: missing\n      run: lunarforge_nonexistent_contract_check\n    - id: later\n      run: echo skipped\n", "verify", "blocked", "tool_unavailable", 2, "", true},
		{"timed-out", "version: 1\nverify:\n  commands:\n    - id: slow\n      run: exec sleep 5\n      timeout_seconds: 1\n", "verify", "blocked", "timed_out", 2, "", true},
		{"evidence-corrupt", "", "status", "error", "evidence_corrupt", 3, "corrupt", false},
		{"legacy-evidence", "", "status", "pass", "checks_passed", 0, "legacy", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.config
			if cfg == "" {
				cfg = "version: 1\nverify:\n  commands:\n    - id: ok\n      run: echo check-output\n"
			}
			repo := newTestRepo(t, cfg)
			// Ignore artifacts so clean-HEAD checks match a configured consumer.
			write(t, filepath.Join(repo, ".git", "info", "exclude"), ".lf/\n")
			headCmd := exec.Command("git", "rev-parse", "HEAD")
			headRaw, err := headCmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			head := strings.TrimSpace(string(headRaw))
			switch tc.prepare {
			case "verify", "stale", "corrupt", "legacy":
				invoke(t, repo, 0, "verify", "--json", "--quiet")
			}
			switch tc.prepare {
			case "dirty", "stale":
				write(t, filepath.Join(repo, "marker.txt"), "changed\n")
			case "corrupt":
				write(t, filepath.Join(latestEvidenceDir(t, repo), "evidence.json"), "{invalid")
			case "legacy":
				path := filepath.Join(latestEvidenceDir(t, repo), "evidence.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				// Previous schema-1 evidence remains readable after upgrading.
				write(t, path, strings.ReplaceAll(string(data), head, head[:7]))
			}
			got := invoke(t, repo, tc.exit, tc.command, "--json")
			if got["command"] != tc.command || got["state"] != tc.state || got["reason"] != tc.reason {
				t.Fatalf("unexpected envelope: %+v", got)
			}
			_, hasRun := got["run"]
			_, hasChecks := got["checks"]
			if hasRun != tc.wantRun || hasChecks != tc.wantRun {
				t.Fatalf("run/check presence must match the outcome: %+v", got)
			}
			if tc.reason == "config_invalid" || tc.reason == "evidence_corrupt" {
				if got["error"] == nil {
					t.Fatal("early failure must include error detail")
				}
				return
			}
			current := got["repository"].(map[string]any)
			if current["head"] != head {
				t.Fatalf("repository.head = %v, want full Git ID %s", current["head"], head)
			}
			if !hasRun {
				return
			}
			run := got["run"].(map[string]any)
			wantHead := head
			if tc.prepare == "legacy" {
				wantHead = head[:7]
			}
			if run["head"] != wantHead {
				t.Fatalf("run.head = %v, want %s", run["head"], wantHead)
			}
			if tc.state == "pass" {
				// These equalities and explicit dirty booleans are what consumers
				// need before accepting clean, independently verified task input.
				if current["fingerprint"] != run["fingerprint"] || run["fingerprint"] != run["final_fingerprint"] {
					t.Fatalf("passing evidence must bind to the current snapshot: %+v", got)
				}
				dirty := tc.prepare == "dirty"
				if current["dirty"] != dirty || run["dirty"] != dirty {
					t.Fatalf("dirty must be an explicit, accurate boolean: %+v", got)
				}
			}
			if tc.name == "verify-fail" || tc.name == "tool-unavailable" {
				checks := got["checks"].([]any)
				last := checks[len(checks)-1].(map[string]any)
				if last["state"] != "skipped" || last["exit_code"] != float64(-1) {
					t.Fatalf("unexecuted check must be explicit: %+v", last)
				}
			}
			if tc.name == "status-stale" && (run["state"] != "pass" || current["fingerprint"] == run["fingerprint"]) {
				t.Fatal("status must distinguish historical success from current freshness")
			}
		})
	}
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
