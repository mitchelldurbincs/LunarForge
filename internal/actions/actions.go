// Package actions generates the GitHub Actions workflow that mirrors LunarForge's
// local gate on the remote. The generated workflow is intentionally a thin
// wrapper: it checks out the repo, obtains `lf`, and runs `lf ci`, which executes
// the same verify.commands from .lunarforge.yml. The repo's lint/test/build
// commands are never duplicated into the YAML — .lunarforge.yml stays the single
// source of truth.
//
// How `lf` is obtained depends on the install mode:
//
//   - source mode is for the LunarForge repo itself: the workflow builds `lf`
//     from ./cmd/lf and invokes ./lf ci.
//   - go-install mode is for a normal consumer repo that does not contain
//     LunarForge's source: the workflow runs `go install <module>@<ref>` and
//     invokes `lf ci` (the binary is on PATH).
//   - custom mode runs explicit install commands (e.g. curl a release binary)
//     and invokes `lf ci`.
package actions

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

// Defaults for the generated workflow.
const (
	DefaultWorkflowName   = "LunarForge"
	DefaultRunsOn         = "ubuntu-latest"
	DefaultTimeoutMinutes = 30
	// DefaultOutputPath is where `lf gen-actions` writes the workflow.
	DefaultOutputPath = ".github/workflows/lunarforge.yml"
	// DefaultModule is the canonical `go install` target, used when go.mod cannot
	// be read while generating from outside the LunarForge repo.
	DefaultModule = "github.com/mitchelldurbincs/lunarforge/cmd/lf"
	// DefaultInstallRef is the version appended after @ in go-install mode.
	DefaultInstallRef = "latest"
)

// InstallMode selects how the generated workflow obtains the `lf` binary.
type InstallMode string

const (
	// InstallSource builds lf from ./cmd/lf (the LunarForge repo itself).
	InstallSource InstallMode = "source"
	// InstallGoInstall runs `go install <module>@<ref>` (a consumer repo).
	InstallGoInstall InstallMode = "go-install"
	// InstallCustom runs explicit install commands that put lf on PATH.
	InstallCustom InstallMode = "custom"
)

// ValidModes lists the install modes accepted by --install-mode, for usage text
// and validation.
var ValidModes = []InstallMode{InstallSource, InstallGoInstall, InstallCustom}

// ParseMode validates a mode string and returns the canonical InstallMode. An
// empty string is not valid here — callers resolve the default before parsing.
func ParseMode(s string) (InstallMode, error) {
	switch InstallMode(s) {
	case InstallSource:
		return InstallSource, nil
	case InstallGoInstall:
		return InstallGoInstall, nil
	case InstallCustom:
		return InstallCustom, nil
	default:
		return "", fmt.Errorf("invalid install mode %q (want source, go-install, or custom)", s)
	}
}

// Install is the resolved install behavior baked into Params.
type Install struct {
	Mode     InstallMode
	Module   string   // go-install target (mode go-install)
	Ref      string   // version after @ (mode go-install)
	Commands []string // explicit install steps (mode custom)
}

// Params are the resolved (post-default) inputs to Generate.
type Params struct {
	WorkflowName    string
	RunsOn          string
	TimeoutMinutes  int
	UploadArtifacts bool
	// SetupCommands run before `lf ci` (e.g. "npm ci"). Empty means no setup step.
	SetupCommands []string
	// Install controls how `lf` is obtained before `lf ci` runs.
	Install Install
}

// Overrides carries CLI flag values that take precedence over config. An empty
// field means "not set on the command line".
type Overrides struct {
	Mode   string
	Module string
	Ref    string
}

// ResolveParams resolves every generator parameter from config, CLI overrides,
// and the repo on disk. Precedence for install settings is: CLI overrides >
// config > auto-detected default. repoDir is used both to auto-detect the
// default mode (./cmd/lf present ⇒ source) and to read the module path from
// go.mod for go-install mode.
func ResolveParams(cfg *config.Config, repoDir string, ov Overrides) (Params, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	ga := cfg.CI.GitHubActions

	p := Params{
		WorkflowName:    ga.WorkflowName,
		RunsOn:          ga.RunsOn,
		TimeoutMinutes:  ga.TimeoutMinutes,
		UploadArtifacts: true, // default on; only an explicit false disables it
		SetupCommands:   cfg.CI.SetupCommands,
	}
	if p.WorkflowName == "" {
		p.WorkflowName = DefaultWorkflowName
	}
	if p.RunsOn == "" {
		p.RunsOn = DefaultRunsOn
	}
	if p.TimeoutMinutes <= 0 {
		p.TimeoutMinutes = DefaultTimeoutMinutes
	}
	if ga.UploadArtifacts != nil {
		p.UploadArtifacts = *ga.UploadArtifacts
	}

	inst, err := resolveInstall(cfg, repoDir, ov)
	if err != nil {
		return Params{}, err
	}
	p.Install = inst
	return p, nil
}

// resolveInstall applies the flags > config > auto-default precedence for the
// install behavior.
func resolveInstall(cfg *config.Config, repoDir string, ov Overrides) (Install, error) {
	ci := cfg.CI.GitHubActions.Install

	// Mode: flag, then config, then auto-detect.
	var mode InstallMode
	switch {
	case ov.Mode != "":
		m, err := ParseMode(ov.Mode)
		if err != nil {
			return Install{}, err
		}
		mode = m
	case ci.Mode != "":
		m, err := ParseMode(ci.Mode)
		if err != nil {
			return Install{}, fmt.Errorf("ci.github_actions.install.mode: %w", err)
		}
		mode = m
	default:
		mode = DefaultMode(repoDir)
	}

	inst := Install{Mode: mode}

	switch mode {
	case InstallGoInstall:
		// Module: flag, then config, then go.mod, then canonical default.
		switch {
		case ov.Module != "":
			inst.Module = ov.Module
		case ci.Module != "":
			inst.Module = ci.Module
		default:
			inst.Module = moduleFromGoMod(repoDir)
		}
		// Ref: flag, then config, then latest.
		switch {
		case ov.Ref != "":
			inst.Ref = ov.Ref
		case ci.Ref != "":
			inst.Ref = ci.Ref
		default:
			inst.Ref = DefaultInstallRef
		}
	case InstallCustom:
		inst.Commands = ci.Commands
		if len(trimEmpty(inst.Commands)) == 0 {
			return Install{}, fmt.Errorf("custom install mode requires ci.github_actions.install.install_commands")
		}
	}

	return inst, nil
}

// DefaultMode auto-detects the install mode for a repo: a repo that contains
// ./cmd/lf is LunarForge itself (source mode); anything else is a consumer
// (go-install mode).
func DefaultMode(repoDir string) InstallMode {
	if repoDir == "" {
		return InstallGoInstall
	}
	if info, err := os.Stat(filepath.Join(repoDir, "cmd", "lf")); err == nil && info.IsDir() {
		return InstallSource
	}
	return InstallGoInstall
}

// moduleFromGoMod reads the module path from go.mod in repoDir and returns it
// with "/cmd/lf" appended, so go-install targets the lf command. It falls back
// to the canonical DefaultModule when go.mod is missing or unparseable.
func moduleFromGoMod(repoDir string) string {
	if repoDir == "" {
		return DefaultModule
	}
	f, err := os.Open(filepath.Join(repoDir, "go.mod"))
	if err != nil {
		return DefaultModule
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if path, ok := strings.CutPrefix(line, "module "); ok {
			path = strings.TrimSpace(path)
			if path != "" {
				return path + "/cmd/lf"
			}
		}
	}
	return DefaultModule
}

// Generate renders the workflow YAML for the given parameters. The result runs
// on pull requests and pushes to main, cancels superseded runs via concurrency,
// uses least-privilege permissions (contents: read), obtains `lf` per the
// install mode, and delegates verification to `lf ci`.
func Generate(p Params) string {
	var b strings.Builder

	fmt.Fprintf(&b, "name: %s\n", p.WorkflowName)
	b.WriteString(`
on:
  pull_request:
  push:
    branches:
      - main

concurrency:
  group: lunarforge-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  verify:
    name: Verify
`)
	fmt.Fprintf(&b, "    runs-on: %s\n", p.RunsOn)
	fmt.Fprintf(&b, "    timeout-minutes: %d\n", p.TimeoutMinutes)
	b.WriteString(`
    steps:
      - name: Checkout
        uses: actions/checkout@v4
`)

	// Go is needed to build (source) or `go install` (go-install) lf. Custom
	// mode brings its own binary and may not need a Go toolchain at all.
	if p.Install.Mode != InstallCustom {
		b.WriteString(`
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: stable
`)
	}

	b.WriteString("\n")
	b.WriteString(installStep(p.Install))

	if step := setupStep(p.SetupCommands); step != "" {
		b.WriteString("\n")
		b.WriteString(step)
	}

	fmt.Fprintf(&b, "\n      - name: Run LunarForge CI\n        run: %s ci\n", lfInvocation(p.Install.Mode))

	if p.UploadArtifacts {
		b.WriteString(`
      - name: Upload LunarForge evidence
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: lunarforge-evidence
          path: .lf/runs/**
`)
	}

	return b.String()
}

// lfInvocation returns how `lf` is invoked once installed. Source mode builds a
// local ./lf; the other modes put `lf` on PATH.
func lfInvocation(mode InstallMode) string {
	if mode == InstallSource {
		return "./lf"
	}
	return "lf"
}

// installStep renders the step that obtains the `lf` binary for the chosen mode.
func installStep(inst Install) string {
	var b strings.Builder
	switch inst.Mode {
	case InstallGoInstall:
		ref := inst.Ref
		if ref == "" {
			ref = DefaultInstallRef
		}
		module := inst.Module
		if module == "" {
			module = DefaultModule
		}
		b.WriteString("      - name: Install LunarForge\n")
		b.WriteString("        run: |\n")
		fmt.Fprintf(&b, "          go install %s@%s\n", module, ref)
		// go install drops the binary in $(go env GOPATH)/bin, which is not
		// guaranteed to be on PATH for later steps; add it explicitly.
		b.WriteString("          echo \"$(go env GOPATH)/bin\" >> \"$GITHUB_PATH\"\n")
	case InstallCustom:
		b.WriteString("      - name: Install LunarForge\n")
		cmds := trimEmpty(inst.Commands)
		if len(cmds) == 1 {
			fmt.Fprintf(&b, "        run: %s\n", cmds[0])
		} else {
			b.WriteString("        run: |\n")
			for _, c := range cmds {
				fmt.Fprintf(&b, "          %s\n", c)
			}
		}
	default: // InstallSource
		b.WriteString("      - name: Build LunarForge\n")
		b.WriteString("        run: go build -o lf ./cmd/lf\n")
	}
	return b.String()
}

// setupStep renders the optional "Project setup" step. With a single command it
// uses an inline run; with several it uses a block scalar so each runs in order.
func setupStep(cmds []string) string {
	nonEmpty := trimEmpty(cmds)
	if len(nonEmpty) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("      - name: Project setup\n")
	if len(nonEmpty) == 1 {
		fmt.Fprintf(&b, "        run: %s\n", nonEmpty[0])
		return b.String()
	}
	b.WriteString("        run: |\n")
	for _, c := range nonEmpty {
		fmt.Fprintf(&b, "          %s\n", c)
	}
	return b.String()
}

// trimEmpty drops blank/whitespace-only entries from a command list.
func trimEmpty(cmds []string) []string {
	var out []string
	for _, c := range cmds {
		if strings.TrimSpace(c) != "" {
			out = append(out, c)
		}
	}
	return out
}
