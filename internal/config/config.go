// Package config loads and validates the .lunarforge.yml file that lives at the
// root of a repository. The config describes how to verify the repo and how to
// generate an explanation of the current diff.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the name of the config file LunarForge looks for.
const FileName = ".lunarforge.yml"

// Config is the top-level shape of .lunarforge.yml.
type Config struct {
	Version  int      `yaml:"version"`
	Project  Project  `yaml:"project"`
	Verify   Verify   `yaml:"verify"`
	Explain  Explain  `yaml:"explain"`
	Evidence Evidence `yaml:"evidence"`

	// path is the absolute path the config was loaded from. It is not part of
	// the serialized YAML.
	path string `yaml:"-"`
}

// Project holds project-level metadata.
type Project struct {
	Name string `yaml:"name"`
}

// Verify holds the list of commands that make up the repo's verification
// ritual. They run in order during `lf verify`.
type Verify struct {
	Commands []Command `yaml:"commands"`
}

// Command is a single verify step.
type Command struct {
	ID  string `yaml:"id"`
	Run string `yaml:"run"`
}

// Explain configures the external agent command used by `lf explain`.
type Explain struct {
	Agent   string   `yaml:"agent"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// Evidence configures where run evidence is stored.
type Evidence struct {
	Dir              string `yaml:"dir"`
	RequireFreshDiff bool   `yaml:"require_fresh_diff"`
}

// Path returns the absolute path the config was loaded from.
func (c *Config) Path() string { return c.path }

// EvidenceDir returns the evidence directory, defaulting to ".lf/runs" when not
// configured.
func (c *Config) EvidenceDir() string {
	if c.Evidence.Dir == "" {
		return filepath.Join(".lf", "runs")
	}
	return c.Evidence.Dir
}

// Load reads and validates the config from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	cfg.path = abs

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Find locates .lunarforge.yml by walking up from startDir to the filesystem
// root. It returns the path to the config file, or an error if none is found.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found in %s or any parent directory (run `lf init`)", FileName, startDir)
		}
		dir = parent
	}
}

// LoadFromDir finds and loads the config starting at startDir.
func LoadFromDir(startDir string) (*Config, error) {
	path, err := Find(startDir)
	if err != nil {
		return nil, err
	}
	return Load(path)
}

func (c *Config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d (expected 1)", c.Version)
	}
	if len(c.Verify.Commands) == 0 {
		return fmt.Errorf("verify.commands must contain at least one command")
	}
	seen := map[string]bool{}
	for i, cmd := range c.Verify.Commands {
		if cmd.ID == "" {
			return fmt.Errorf("verify.commands[%d].id is required", i)
		}
		if cmd.Run == "" {
			return fmt.Errorf("verify.commands[%d] (%s).run is required", i, cmd.ID)
		}
		if seen[cmd.ID] {
			return fmt.Errorf("duplicate verify command id %q", cmd.ID)
		}
		seen[cmd.ID] = true
	}
	return nil
}
