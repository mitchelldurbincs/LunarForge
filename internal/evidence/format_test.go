package evidence

import (
	"path/filepath"
	"testing"
)

func TestCheckLogPaths(t *testing.T) {
	const id = "Build_1.test-2"
	if got := filepath.ToSlash(CheckStdoutPath(id)); got != "checks/Build_1.test-2.stdout.txt" {
		t.Fatalf("stdout path = %q", got)
	}
	if got := filepath.ToSlash(CheckStderrPath(id)); got != "checks/Build_1.test-2.stderr.txt" {
		t.Fatalf("stderr path = %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	for _, tc := range []struct {
		ms   int64
		want string
	}{
		{0, "0ms"},
		{99, "99ms"},
		{100, "0.1s"},
		{999, "1.0s"},
		{1250, "1.2s"},
		{60000, "60.0s"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := FormatDuration(tc.ms); got != tc.want {
				t.Fatalf("FormatDuration(%d) = %q, want %q", tc.ms, got, tc.want)
			}
		})
	}
}
