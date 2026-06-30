package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

// sourceParams is a convenience for tests that want a default source-mode
// workflow without touching the filesystem.
func sourceParams() Params {
	return Params{
		WorkflowName:    DefaultWorkflowName,
		RunsOn:          DefaultRunsOn,
		TimeoutMinutes:  DefaultTimeoutMinutes,
		UploadArtifacts: true,
		Install:         Install{Mode: InstallSource},
	}
}

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
}

func TestGenerateContainsRequiredFragments(t *testing.T) {
	out := Generate(sourceParams())
	for _, frag := range requiredFragments {
		if !strings.Contains(out, frag) {
			t.Errorf("generated workflow missing %q\n---\n%s", frag, out)
		}
	}
	// Source mode builds lf and runs the local binary.
	if !strings.Contains(out, "run: go build -o lf ./cmd/lf") {
		t.Errorf("expected source build step\n%s", out)
	}
	if !strings.Contains(out, "run: ./lf ci") {
		t.Errorf("expected ./lf ci delegation\n%s", out)
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
	p := sourceParams()
	p.UploadArtifacts = false
	out := Generate(p)
	if strings.Contains(out, "upload-artifact") {
		t.Errorf("did not expect upload step when disabled\n%s", out)
	}
	// The core delegation must still be present.
	if !strings.Contains(out, "run: ./lf ci") {
		t.Errorf("expected lf ci step even without artifacts\n%s", out)
	}
}

func TestGenerateRespectsConfigOverrides(t *testing.T) {
	p := sourceParams()
	p.WorkflowName = "CustomName"
	p.RunsOn = "windows-latest"
	p.TimeoutMinutes = 45
	out := Generate(p)
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

func TestGenerateSourceMode(t *testing.T) {
	out := Generate(sourceParams())
	if !strings.Contains(out, "run: go build -o lf ./cmd/lf") {
		t.Errorf("source mode must build from ./cmd/lf\n%s", out)
	}
	if !strings.Contains(out, "run: ./lf ci") {
		t.Errorf("source mode must run ./lf ci\n%s", out)
	}
}

func TestGenerateGoInstallMode(t *testing.T) {
	p := sourceParams()
	p.Install = Install{
		Mode:   InstallGoInstall,
		Module: "github.com/mitchelldurbincs/lunarforge/cmd/lf",
		Ref:    "latest",
	}
	out := Generate(p)
	if !strings.Contains(out, "go install github.com/mitchelldurbincs/lunarforge/cmd/lf@latest") {
		t.Errorf("go-install mode must `go install ...@latest`\n%s", out)
	}
	// Must invoke the on-PATH binary, not ./lf.
	if !strings.Contains(out, "run: lf ci") {
		t.Errorf("go-install mode must run `lf ci`\n%s", out)
	}
	if strings.Contains(out, "./lf ci") {
		t.Errorf("go-install mode must NOT run ./lf ci\n%s", out)
	}
	// A consumer repo does not contain LunarForge source.
	if strings.Contains(out, "./cmd/lf") {
		t.Errorf("go-install mode must NOT reference ./cmd/lf\n%s", out)
	}
	if strings.Contains(out, "go build -o lf ./cmd/lf") {
		t.Errorf("go-install mode must NOT build from source\n%s", out)
	}
}

func TestGenerateGoInstallRef(t *testing.T) {
	p := sourceParams()
	p.Install = Install{Mode: InstallGoInstall, Module: "example.com/x/cmd/lf", Ref: "v0.1.0"}
	out := Generate(p)
	if !strings.Contains(out, "go install example.com/x/cmd/lf@v0.1.0") {
		t.Errorf("expected pinned ref in go install\n%s", out)
	}
}

func TestGenerateCustomMode(t *testing.T) {
	p := sourceParams()
	p.Install = Install{
		Mode:     InstallCustom,
		Commands: []string{"curl -L https://example.com/lf -o lf", "chmod +x lf", "sudo mv lf /usr/local/bin/lf"},
	}
	out := Generate(p)
	for _, want := range []string{
		"curl -L https://example.com/lf -o lf",
		"chmod +x lf",
		"sudo mv lf /usr/local/bin/lf",
		"run: lf ci",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("custom mode missing %q\n%s", want, out)
		}
	}
	// Custom mode supplies its own binary; no Go setup, no source build.
	if strings.Contains(out, "go build -o lf ./cmd/lf") {
		t.Errorf("custom mode must NOT build from source\n%s", out)
	}
	if strings.Contains(out, "actions/setup-go") {
		t.Errorf("custom mode should not set up Go\n%s", out)
	}
}

func TestGenerateSingleSetupCommand(t *testing.T) {
	p := sourceParams()
	p.SetupCommands = []string{"npm ci"}
	out := Generate(p)
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
	p := sourceParams()
	p.SetupCommands = []string{"npm ci", "npm run build:deps"}
	out := Generate(p)
	if !strings.Contains(out, "run: |") {
		t.Errorf("expected block scalar for multiple commands\n%s", out)
	}
	for _, c := range []string{"npm ci", "npm run build:deps"} {
		if !strings.Contains(out, c) {
			t.Errorf("missing setup command %q\n%s", c, out)
		}
	}
}

func TestSetupPrecedesGoInstallCI(t *testing.T) {
	p := sourceParams()
	p.Install = Install{Mode: InstallGoInstall, Module: DefaultModule, Ref: "latest"}
	p.SetupCommands = []string{"npm ci"}
	out := Generate(p)
	ciIdx := strings.Index(out, "run: lf ci")
	setupIdx := strings.Index(out, "Project setup")
	if setupIdx < 0 || ciIdx < 0 || setupIdx > ciIdx {
		t.Errorf("setup_commands must be inserted before `lf ci`\n%s", out)
	}
}

func TestResolveParamsDefaultsToGoInstallOutsideLunarForge(t *testing.T) {
	dir := t.TempDir() // no cmd/lf, no go.mod
	p, err := ResolveParams(&config.Config{}, dir, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Install.Mode != InstallGoInstall {
		t.Errorf("expected go-install default outside LunarForge, got %q", p.Install.Mode)
	}
	if p.Install.Module != DefaultModule {
		t.Errorf("expected canonical module fallback, got %q", p.Install.Module)
	}
	if p.Install.Ref != DefaultInstallRef {
		t.Errorf("expected default ref latest, got %q", p.Install.Ref)
	}
}

func TestResolveParamsDefaultsToSourceInLunarForge(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "lf"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := ResolveParams(&config.Config{}, dir, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Install.Mode != InstallSource {
		t.Errorf("expected source default when ./cmd/lf exists, got %q", p.Install.Mode)
	}
}

func TestResolveParamsReadsModuleFromGoMod(t *testing.T) {
	dir := t.TempDir()
	goMod := "module example.com/myrepo\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := ResolveParams(&config.Config{}, dir, Overrides{Mode: "go-install"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Install.Module != "example.com/myrepo/cmd/lf" {
		t.Errorf("module from go.mod = %q, want example.com/myrepo/cmd/lf", p.Install.Module)
	}
}

func TestResolveParamsFlagOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		CI: config.CI{
			GitHubActions: config.GitHubActions{
				Install: config.Install{Mode: "source"},
			},
		},
	}
	p, err := ResolveParams(cfg, dir, Overrides{Mode: "go-install", Ref: "v1.2.3", Module: "example.com/x/cmd/lf"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Install.Mode != InstallGoInstall {
		t.Errorf("flag should override config mode, got %q", p.Install.Mode)
	}
	if p.Install.Ref != "v1.2.3" || p.Install.Module != "example.com/x/cmd/lf" {
		t.Errorf("flag ref/module not applied: %+v", p.Install)
	}
}

func TestResolveParamsConfigInstall(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		CI: config.CI{
			GitHubActions: config.GitHubActions{
				Install: config.Install{Mode: "go-install", Module: "example.com/x/cmd/lf", Ref: "v0.2.0"},
			},
		},
	}
	p, err := ResolveParams(cfg, dir, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Install.Mode != InstallGoInstall || p.Install.Module != "example.com/x/cmd/lf" || p.Install.Ref != "v0.2.0" {
		t.Errorf("config install not applied: %+v", p.Install)
	}
}

func TestResolveParamsInvalidMode(t *testing.T) {
	if _, err := ResolveParams(&config.Config{}, t.TempDir(), Overrides{Mode: "nonsense"}); err == nil {
		t.Error("expected error for invalid install mode")
	}
}

func TestResolveParamsCustomRequiresCommands(t *testing.T) {
	if _, err := ResolveParams(&config.Config{}, t.TempDir(), Overrides{Mode: "custom"}); err == nil {
		t.Error("custom mode without install_commands should error")
	}
	cfg := &config.Config{
		CI: config.CI{
			GitHubActions: config.GitHubActions{
				Install: config.Install{Mode: "custom", Commands: []string{"echo hi"}},
			},
		},
	}
	if _, err := ResolveParams(cfg, t.TempDir(), Overrides{}); err != nil {
		t.Errorf("custom mode with commands should succeed: %v", err)
	}
}

func TestResolveParamsNonInstallDefaults(t *testing.T) {
	p, err := ResolveParams(&config.Config{}, t.TempDir(), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
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

func TestResolveParamsUploadDisable(t *testing.T) {
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
	p, err := ResolveParams(cfg, t.TempDir(), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
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
