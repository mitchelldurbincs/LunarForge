package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchelldurbincs/lunarforge/internal/actions"
)

// cmdGenActions writes a GitHub Actions workflow that mirrors the local gate by
// running `lf ci`. It refuses to overwrite an existing workflow unless --force is
// given, and prints the path plus next steps.
func cmdGenActions(args []string) error {
	fs := flag.NewFlagSet("gen-actions", flag.ExitOnError)
	output := fs.String("output", actions.DefaultOutputPath, "path to write the workflow file")
	force := fs.Bool("force", false, "overwrite an existing workflow file")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf gen-actions [--output PATH] [--force]\n\n"+
			"Generates a GitHub Actions workflow that runs `lf ci` (the remote mirror\n"+
			"of your local LunarForge gate). Does not overwrite an existing file unless\n"+
			"--force is passed.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
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

	content := actions.Generate(actions.ParamsFromConfig(l.cfg))
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing workflow: %w", err)
	}

	rel := relPath(l.repoDir, outPath)
	fmt.Println("LunarForge gen-actions")
	fmt.Println()
	fmt.Println("Created:")
	fmt.Printf("%s\n", rel)
	fmt.Println()
	fmt.Println("Next:")
	fmt.Printf("git add %s\n", rel)
	fmt.Println("git commit -m \"add LunarForge CI\"")
	fmt.Println("git push")
	fmt.Println()
	fmt.Println("Note: this workflow is a remote mirror of your verify commands. It does")
	fmt.Println("not install project dependencies — add language setup (Node, Rust, etc.)")
	fmt.Println("or `ci.setup_commands` in .lunarforge.yml as your project needs.")
	return nil
}
