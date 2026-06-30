package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchelldurbincs/lunarforge/internal/config"
)

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite an existing .lunarforge.yml")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf init [--force]\n\nCreates .lunarforge.yml and .lf/ in the current directory.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, config.FileName)
	if _, err := os.Stat(configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", config.FileName)
	}

	projectName := filepath.Base(cwd)
	if err := os.WriteFile(configPath, []byte(config.StarterTemplate(projectName)), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.FileName, err)
	}

	// Create the evidence dir and a .gitignore inside .lf so run artifacts are
	// not accidentally committed.
	lfDir := filepath.Join(cwd, ".lf")
	if err := os.MkdirAll(filepath.Join(lfDir, "runs"), 0o755); err != nil {
		return fmt.Errorf("creating .lf: %w", err)
	}
	gitignore := filepath.Join(lfDir, ".gitignore")
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		_ = os.WriteFile(gitignore, []byte("# LunarForge run evidence is local-only by default.\nruns/\nlatest\n"), 0o644)
	}

	fmt.Println("LunarForge init")
	fmt.Println()
	fmt.Printf("✅ Created %s\n", config.FileName)
	fmt.Println("✅ Created .lf/ (evidence directory)")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Create your verify script, e.g.:")
	fmt.Println("       scripts/verify.sh   (macOS/Linux)")
	fmt.Println("       scripts/verify.ps1  (Windows)")
	fmt.Println("     and make it run your real lint/build/test ritual.")
	fmt.Println("  2. Edit .lunarforge.yml to match your project.")
	fmt.Println("  3. Run: lf verify")
	return nil
}
