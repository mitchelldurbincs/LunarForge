// Package actions generates the GitHub Actions workflow that mirrors LunarForge's
// local gate on the remote. The generated workflow is intentionally a thin
// wrapper: it checks out the repo, builds `lf`, and runs `lf ci`, which executes
// the same verify.commands from .lunarforge.yml. The repo's lint/test/build
// commands are never duplicated into the YAML — .lunarforge.yml stays the single
// source of truth.
package actions

import (
	"fmt"
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
)

// Params are the resolved (post-default) inputs to Generate.
type Params struct {
	WorkflowName    string
	RunsOn          string
	TimeoutMinutes  int
	UploadArtifacts bool
	// SetupCommands run before `lf ci` (e.g. "npm ci"). Empty means no setup step.
	SetupCommands []string
}

// ParamsFromConfig resolves the generator parameters from config, applying
// defaults for any unset field. The ci: section is optional.
func ParamsFromConfig(cfg *config.Config) Params {
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
	return p
}

// Generate renders the workflow YAML for the given parameters. The result runs
// on pull requests and pushes to main, cancels superseded runs via concurrency,
// uses least-privilege permissions (contents: read), builds `lf` from this repo,
// and delegates verification to `lf ci`.
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

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Build LunarForge
        run: go build -o lf ./cmd/lf
`)

	if step := setupStep(p.SetupCommands); step != "" {
		b.WriteString("\n")
		b.WriteString(step)
	}

	b.WriteString(`
      - name: Run LunarForge CI
        run: ./lf ci
`)

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

// setupStep renders the optional "Project setup" step. With a single command it
// uses an inline run; with several it uses a block scalar so each runs in order.
func setupStep(cmds []string) string {
	var nonEmpty []string
	for _, c := range cmds {
		if strings.TrimSpace(c) != "" {
			nonEmpty = append(nonEmpty, c)
		}
	}
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
