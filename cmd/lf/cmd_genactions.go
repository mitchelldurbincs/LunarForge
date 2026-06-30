package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchelldurbincs/lunarforge/internal/actions"
)

// cmdGenActions writes a GitHub Actions workflow that mirrors the local gate by
// running `lf ci`. It supports two install stories — building `lf` from source
// (the LunarForge repo itself) and `go install`ing it (a normal consumer repo) —
// auto-detecting which one fits when not told. It refuses to overwrite an
// existing workflow unless --force is given, and prints the path plus next steps.
func cmdGenActions(args []string) error {
	fs := flag.NewFlagSet("gen-actions", flag.ExitOnError)
	output := fs.String("output", actions.DefaultOutputPath, "path to write the workflow file")
	force := fs.Bool("force", false, "overwrite an existing workflow file")
	installMode := fs.String("install-mode", "", "how the workflow gets lf: source | go-install | custom (default: auto-detect)")
	installRef := fs.String("install-ref", "", "version/ref for go-install mode (default: latest)")
	installModule := fs.String("install-module", "", "go install target for go-install mode (default: read from go.mod)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lf gen-actions [--install-mode MODE] [--install-ref REF]\n"+
			"                      [--install-module MODULE] [--output PATH] [--force]\n\n"+
			"Generates a GitHub Actions workflow that runs `lf ci` (the remote mirror\n"+
			"of your local LunarForge gate). It does not overwrite an existing file\n"+
			"unless --force is passed.\n\n"+
			"Install modes (how the workflow obtains the lf binary):\n"+
			"  source      build lf from ./cmd/lf — for the LunarForge repo itself\n"+
			"  go-install  go install lf — for a normal repo that uses LunarForge\n"+
			"  custom      run install_commands from .lunarforge.yml\n\n"+
			"When --install-mode is omitted, the mode is taken from\n"+
			"ci.github_actions.install in .lunarforge.yml, or auto-detected: a repo\n"+
			"containing ./cmd/lf defaults to source, any other repo to go-install.\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	l, err := load()
	if err != nil {
		return err
	}

	params, err := actions.ResolveParams(l.cfg, l.repoDir, actions.Overrides{
		Mode:   *installMode,
		Module: *installModule,
		Ref:    *installRef,
	})
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

	content := actions.Generate(params)
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing workflow: %w", err)
	}

	rel := relPath(l.repoDir, outPath)
	fmt.Println("LunarForge gen-actions")
	fmt.Println()
	fmt.Printf("Install mode: %s\n", params.Install.Mode)
	if params.Install.Mode == actions.InstallGoInstall {
		fmt.Printf("Install:      go install %s@%s\n", params.Install.Module, params.Install.Ref)
	}
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
