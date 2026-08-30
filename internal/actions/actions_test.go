package actions

import (
	"strings"
	"testing"
)

func TestGenerateCanonicalWorkflow(t *testing.T) {
	out := Generate("v0.2.0")
	for _, want := range []string{
		"pull_request:",
		"branches: [main]",
		"permissions:\n  contents: read",
		"uses: actions/checkout@v7",
		"uses: actions/setup-go@v7",
		"uses: actions/upload-artifact@v7",
		"go install " + DefaultModule + "@v0.2.0",
		"run: lf verify --json",
		"path: .lf/runs/**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated workflow missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "lf ci") || strings.Contains(out, "./cmd/lf") {
		t.Errorf("consumer workflow contains legacy/source behavior\n%s", out)
	}
}

func TestGenerateDefaultsToLatest(t *testing.T) {
	if out := Generate(""); !strings.Contains(out, "@latest") {
		t.Errorf("default workflow must install latest\n%s", out)
	}
}
