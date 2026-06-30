package actions

import (
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

// requiredFragments are the structural elements every generated workflow must
// contain, regardless of config. They map directly to the acceptance criteria.
var requiredFragments = []string{
	"pull_request:",                  // PR trigger
	"push:",                          // push trigger
	"branches:\n      - main",        // push to main
	"concurrency:",                   // cancel stale runs
	"cancel-in-progress: true",       // ...
	"permissions:\n  contents: read", // least privilege
	"uses: actions/checkout@v4",      // checkout
	"run: go build -o lf ./cmd/lf",   // build/install lf
	"run: ./lf ci",                   // delegate to lf ci
}

func TestGenerateContainsRequiredFragments(t *testing.T) {
	out := Generate(Params{
		WorkflowName:    DefaultWorkflowName,
		RunsOn:          DefaultRunsOn,
		TimeoutMinutes:  DefaultTimeoutMinutes,
		UploadArtifacts: true,
	})
	for _, frag := range requiredFragments {
		if !strings.Contains(out, frag) {
			t.Errorf("generated workflow missing %q\n---\n%s", frag, out)
		}
	}
	// Upload step present when enabled.
	if !strings.Contains(out, "uses: actions/upload-artifact@v4") {
		t.Errorf("expected upload-artifact step\n%s", out)
	}
	if !strings.Contains(out, "path: .lf/runs/**") {
		t.Errorf("expected evidence artifact path\n%s", out)
	}
}

func TestGenerateUploadArtifactsDisabled(t *testing.T) {
	out := Generate(Params{
		WorkflowName:    DefaultWorkflowName,
		RunsOn:          DefaultRunsOn,
		TimeoutMinutes:  DefaultTimeoutMinutes,
		UploadArtifacts: false,
	})
	if strings.Contains(out, "upload-artifact") {
		t.Errorf("did not expect upload step when disabled\n%s", out)
	}
	// The core delegation must still be present.
	if !strings.Contains(out, "run: ./lf ci") {
		t.Errorf("expected lf ci step even without artifacts\n%s", out)
	}
}

func TestGenerateRespectsConfigOverrides(t *testing.T) {
	out := Generate(Params{
		WorkflowName:    "CustomName",
		RunsOn:          "windows-latest",
		TimeoutMinutes:  45,
		UploadArtifacts: true,
	})
	for _, want := range []string{
		"name: CustomName",
		"runs-on: windows-latest",
		"timeout-minutes: 45",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output\n%s", want, out)
		}
	}
}

func TestGenerateSingleSetupCommand(t *testing.T) {
	out := Generate(Params{
		WorkflowName:    DefaultWorkflowName,
		RunsOn:          DefaultRunsOn,
		TimeoutMinutes:  DefaultTimeoutMinutes,
		UploadArtifacts: true,
		SetupCommands:   []string{"npm ci"},
	})
	if !strings.Contains(out, "- name: Project setup") {
		t.Errorf("expected Project setup step\n%s", out)
	}
	if !strings.Contains(out, "run: npm ci") {
		t.Errorf("expected inline setup command\n%s", out)
	}
	// Setup must appear before the lf ci step.
	if strings.Index(out, "Project setup") > strings.Index(out, "run: ./lf ci") {
		t.Errorf("setup step must precede lf ci\n%s", out)
	}
}

func TestGenerateMultipleSetupCommands(t *testing.T) {
	out := Generate(Params{
		WorkflowName:    DefaultWorkflowName,
		RunsOn:          DefaultRunsOn,
		TimeoutMinutes:  DefaultTimeoutMinutes,
		UploadArtifacts: true,
		SetupCommands:   []string{"npm ci", "npm run build:deps"},
	})
	if !strings.Contains(out, "run: |") {
		t.Errorf("expected block scalar for multiple commands\n%s", out)
	}
	for _, c := range []string{"npm ci", "npm run build:deps"} {
		if !strings.Contains(out, c) {
			t.Errorf("missing setup command %q\n%s", c, out)
		}
	}
}

func TestParamsFromConfigDefaults(t *testing.T) {
	p := ParamsFromConfig(&config.Config{})
	if p.WorkflowName != DefaultWorkflowName {
		t.Errorf("workflow name = %q, want default", p.WorkflowName)
	}
	if p.RunsOn != DefaultRunsOn {
		t.Errorf("runs-on = %q, want default", p.RunsOn)
	}
	if p.TimeoutMinutes != DefaultTimeoutMinutes {
		t.Errorf("timeout = %d, want default", p.TimeoutMinutes)
	}
	if !p.UploadArtifacts {
		t.Error("upload artifacts should default to true")
	}
}

func TestParamsFromConfigOverridesAndDisable(t *testing.T) {
	disable := false
	cfg := &config.Config{
		CI: config.CI{
			GitHubActions: config.GitHubActions{
				WorkflowName:    "Mirror",
				RunsOn:          "macos-latest",
				TimeoutMinutes:  10,
				UploadArtifacts: &disable,
			},
			SetupCommands: []string{"npm ci"},
		},
	}
	p := ParamsFromConfig(cfg)
	if p.WorkflowName != "Mirror" || p.RunsOn != "macos-latest" || p.TimeoutMinutes != 10 {
		t.Errorf("overrides not applied: %+v", p)
	}
	if p.UploadArtifacts {
		t.Error("explicit false should disable upload")
	}
	if len(p.SetupCommands) != 1 || p.SetupCommands[0] != "npm ci" {
		t.Errorf("setup commands not carried: %+v", p.SetupCommands)
	}
}
