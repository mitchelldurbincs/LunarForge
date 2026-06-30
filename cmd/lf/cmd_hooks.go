package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mitchelldurbincs/lunarforge/internal/hooks"
)

func cmdInstallHooks(args []string) error {
	fs := flag.NewFlagSet("install-hooks", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf install-hooks\n\nInstalls a pre-push hook that requires fresh passing evidence.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	l, err := load()
	if err != nil {
		return err
	}

	res, err := hooks.InstallPrePush(l.repoDir, time.Now())
	if err != nil {
		return err
	}

	fmt.Println("LunarForge install-hooks")
	fmt.Println()
	if res.Replaced {
		fmt.Printf("✅ Updated existing LunarForge pre-push hook:\n   %s\n", relPath(l.repoDir, res.Path))
	} else {
		fmt.Printf("✅ Installed pre-push hook:\n   %s\n", relPath(l.repoDir, res.Path))
	}
	if res.BackupPath != "" {
		fmt.Println()
		fmt.Printf("⚠️  An existing pre-push hook was found and backed up to:\n   %s\n", relPath(l.repoDir, res.BackupPath))
		fmt.Println("   Review it and merge any logic you still need.")
	}
	fmt.Println()
	fmt.Println("The hook runs `lf status --require-fresh-passing` before each push and")
	fmt.Println("blocks the push unless there is fresh, passing evidence for the current")
	fmt.Println("diff. It only reads saved evidence — it does not re-run your tests.")
	fmt.Println("Bypass once with: git push --no-verify")
	return nil
}
