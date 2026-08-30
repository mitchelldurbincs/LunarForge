// Package actions renders the one supported GitHub Actions integration.
package actions

import (
	"fmt"
	"strings"
)

const (
	DefaultOutputPath = ".github/workflows/lunarforge.yml"
	DefaultModule     = "github.com/mitchelldurbincs/lunarforge/cmd/lf"
	DefaultInstallRef = "latest"
)

// Generate returns a deliberately boring consumer workflow. Repository setup
// belongs in verify.commands, keeping .lunarforge.yml as the single definition
// of what must pass.
func Generate(installRef string) string {
	if strings.TrimSpace(installRef) == "" {
		installRef = DefaultInstallRef
	}
	return fmt.Sprintf(`name: LunarForge

on:
  pull_request:
  push:
    branches: [main]

concurrency:
  group: lunarforge-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  verify:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - name: Install LunarForge
        run: go install %s@%s
      - name: Verify
        run: lf verify --json
      - name: Upload evidence
        if: always()
        uses: actions/upload-artifact@v7
        with:
          name: lunarforge-evidence
          path: .lf/runs/**
`, DefaultModule, installRef)
}
