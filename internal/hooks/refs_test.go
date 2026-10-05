package hooks

import (
	"strings"
	"testing"
)

func TestValidatePrePush(t *testing.T) {
	head := strings.Repeat("a", 40)
	other := strings.Repeat("b", 40)
	zero := strings.Repeat("0", 40)
	valid := "refs/heads/main " + head + " refs/heads/main " + zero + "\n"
	cases := []struct{ name, input, want string }{
		{"HEAD branch", valid, ""},
		{"HEAD source", "HEAD " + head + " refs/heads/x " + other, ""},
		{"HEAD wrong object", "HEAD " + other + " refs/heads/x " + zero, "pre-push: local object is not HEAD"},
		{"HEAD multiple", "HEAD " + head + " refs/heads/x " + zero + "\n" + valid, "pre-push: multiple ref updates are not supported"},
		{"HEAD tag destination", "HEAD " + head + " refs/tags/v1 " + zero, "pre-push: tag updates are not supported"},
		{"unrelated source", "refs/notes/x " + head + " refs/heads/x " + zero, "pre-push: only branch updates are supported"},
		{"non-HEAD", "refs/heads/other " + other + " refs/heads/other " + zero, "pre-push: local object is not HEAD"},
		{"multiple", valid + valid, "pre-push: multiple ref updates are not supported"},
		{"delete", "(delete) " + zero + " refs/heads/main " + other, "pre-push: deletion updates are not supported"},
		{"tag", "refs/tags/v1 " + head + " refs/tags/v1 " + zero, "pre-push: tag updates are not supported"},
		{"other namespace", "refs/heads/main " + head + " refs/notes/foo " + zero, "pre-push: only branch updates are supported"},
		{"empty", "", "pre-push: no ref updates supplied"},
		{"malformed", "refs/heads/main\n", "pre-push: malformed ref input"},
		{"invalid OID", "refs/heads/main nope refs/heads/main " + zero, "pre-push: malformed object ID"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePrePush(strings.NewReader(c.input), head)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}
