package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchelldurbincs/lunarforge/internal/actions"
)

func cmdGenActions(args []string) error {
	fs := flag.NewFlagSet("gen-actions", flag.ContinueOnError)
	output := fs.String("output", actions.DefaultOutputPath, "workflow path")
	force := fs.Bool("force", false, "overwrite an existing workflow")
	installRef := fs.String("install-ref", actions.DefaultInstallRef, "LunarForge version or ref to install")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: lf gen-actions [--install-ref VERSION] [--output PATH] [--force]")
	}
	if err := fs.Parse(args); err != nil {
		return &exitError{code: 2}
	}

	l, err := load()
	if err != nil {
		return err
	}
	outPath := *output
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(l.repoDir, outPath)
	}
	if _, err := os.Stat(outPath); err == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", relPath(l.repoDir, outPath))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating workflow directory: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(actions.Generate(*installRef)), 0o644); err != nil {
		return fmt.Errorf("writing workflow: %w", err)
	}

	fmt.Printf("Created %s\n", relPath(l.repoDir, outPath))
	fmt.Println("Commit the workflow after choosing a released --install-ref.")
	return nil
}
