package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readWorkflow runs gen-actions with the given args and returns the generated
// workflow content from the default path.
func readWorkflow(t *testing.T, repo string, args ...string) string {
	t.Helper()
	if err := cmdGenActions(args); err != nil {
		t.Fatalf("lf gen-actions %v: %v", args, err)
	}
	wfPath := filepath.Join(repo, ".github", "workflows", "lunarforge.yml")
	data, err := os.ReadFile(wfPath)
	if err != nil {
		t.Fatalf("workflow not created: %v", err)
	}
	return string(data)
}

// makeCmdLF creates a ./cmd/lf directory so the repo auto-detects as source mode.
func makeCmdLF(t *testing.T, repo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "lf"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// commonFragments must appear in every generated workflow regardless of mode.
var commonFragments = []string{
	"pull_request:",
	"push:",
	"- main",
	"concurrency:",
	"permissions:\n  contents: read",
	"uses: actions/checkout@v4",
	"uses: actions/upload-artifact@v4",
}

func TestGenActionsSourceMode(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	makeCmdLF(t, repo) // makes auto-detect choose source, but we also pass the flag

	content := readWorkflow(t, repo, "--install-mode", "source")
	for _, frag := range commonFragments {
		if !strings.Contains(content, frag) {
			t.Errorf("generated workflow missing %q", frag)
		}
	}
	if !strings.Contains(content, "run: go build -o lf ./cmd/lf") {
		t.Errorf("source mode must build from ./cmd/lf:\n%s", content)
	}
	if !strings.Contains(content, "run: ./lf ci") {
		t.Errorf("source mode must run ./lf ci:\n%s", content)
	}
}

func TestGenActionsAutoDetectsSourceWhenCmdLFPresent(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	makeCmdLF(t, repo)

	content := readWorkflow(t, repo) // no --install-mode
	if !strings.Contains(content, "run: go build -o lf ./cmd/lf") {
		t.Errorf("auto-detect should pick source mode when ./cmd/lf exists:\n%s", content)
	}
}

func TestGenActionsGoInstallMode(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	// No cmd/lf: a consumer repo.

	content := readWorkflow(t, repo, "--install-mode", "go-install", "--install-ref", "latest")
	if !strings.Contains(content, "go install github.com/mitchelldurbincs/lunarforge/cmd/lf@latest") {
		t.Errorf("go-install mode must `go install ...@latest`:\n%s", content)
	}
	if !strings.Contains(content, "run: lf ci") {
		t.Errorf("go-install mode must run `lf ci`:\n%s", content)
	}
	if strings.Contains(content, "./lf ci") {
		t.Errorf("go-install mode must NOT run ./lf ci:\n%s", content)
	}
	// Consumer-safety: the generated workflow must not assume the repo contains
	// LunarForge source.
	if strings.Contains(content, "./cmd/lf") {
		t.Errorf("consumer workflow must NOT reference ./cmd/lf:\n%s", content)
	}
}

func TestGenActionsAutoDetectsGoInstallWithoutCmdLF(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	content := readWorkflow(t, repo) // no flag, no cmd/lf
	if !strings.Contains(content, "go install ") {
		t.Errorf("auto-detect should pick go-install for a consumer repo:\n%s", content)
	}
	if strings.Contains(content, "./cmd/lf") {
		t.Errorf("consumer workflow must NOT reference ./cmd/lf:\n%s", content)
	}
}

func TestGenActionsGoInstallPinnedRef(t *testing.T) {
	repo := newTestRepo(t, passingConfig)
	content := readWorkflow(t, repo, "--install-mode", "go-install", "--install-ref", "v0.1.0")
	if !strings.Contains(content, "@v0.1.0") {
		t.Errorf("expected pinned ref v0.1.0:\n%s", content)
	}
}

func TestGenActionsSetupCommandsBeforeCI(t *testing.T) {
	repo := newTestRepo(t, setupConfig)
	content := readWorkflow(t, repo, "--install-mode", "go-install")
	setupIdx := strings.Index(content, "Project setup")
	ciIdx := strings.Index(content, "run: lf ci")
	if setupIdx < 0 || ciIdx < 0 {
		t.Fatalf("missing setup and/or ci step:\n%s", content)
	}
	if setupIdx > ciIdx {
		t.Errorf("setup_commands must be inserted before `lf ci`:\n%s", content)
	}
	if !strings.Contains(content, "npm ci") {
		t.Errorf("expected setup command npm ci:\n%s", content)
	}
}

func TestGenActionsRefusesOverwriteWithoutForce(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	if err := cmdGenActions(nil); err != nil {
		t.Fatalf("first gen-actions: %v", err)
	}
	// Second run without --force must refuse.
	if err := cmdGenActions(nil); err == nil {
		t.Fatal("expected gen-actions to refuse overwriting existing workflow")
	}
	// With --force it should overwrite successfully.
	if err := cmdGenActions([]string{"--force"}); err != nil {
		t.Fatalf("gen-actions --force should overwrite: %v", err)
	}

	wfPath := filepath.Join(repo, ".github", "workflows", "lunarforge.yml")
	if _, err := os.Stat(wfPath); err != nil {
		t.Errorf("workflow should still exist after --force: %v", err)
	}
}

func TestGenActionsCustomOutput(t *testing.T) {
	repo := newTestRepo(t, passingConfig)

	out := filepath.Join(".github", "workflows", "custom.yml")
	if err := cmdGenActions([]string{"--output", out}); err != nil {
		t.Fatalf("gen-actions --output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, out)); err != nil {
		t.Errorf("custom workflow not created: %v", err)
	}
}

func TestGenActionsInvalidMode(t *testing.T) {
	newTestRepo(t, passingConfig)
	if err := cmdGenActions([]string{"--install-mode", "bogus"}); err == nil {
		t.Error("expected error for invalid --install-mode")
	}
}
