package main

import (
	"os"

	"github.com/mitchelldurbincs/lunarforge/internal/gitutil"
	"github.com/mitchelldurbincs/lunarforge/internal/hooks"
)

func cmdPrePush() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	subject, err := gitutil.ReadSubject(cwd)
	if err != nil {
		return err
	}
	if err := hooks.ValidatePrePush(os.Stdin, subject.Commit); err != nil {
		return err
	}
	return statusForHead([]string{"--commit", "HEAD", "--require-fresh-passing"}, subject.Commit)
}
