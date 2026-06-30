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
	Version  int              `yaml:"version"`
	Project  Project          `yaml:"project"`
	Verify   Verify           `yaml:"verify"`
	Explain  Explain          `yaml:"explain"`
	Evidence Evidence         `yaml:"evidence"`
	Repair   Repair           `yaml:"repair"`
	Agents   map[string]Agent `yaml:"agents"`

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

// Repair configures `lf repair`: how an AI agent is invoked to attempt the
// smallest fix for a failed verification run. Repair is deliberately narrow —
// it only responds to failed evidence and never declares its own success.
type Repair struct {
	// Enabled gates `lf repair`. It is a pointer so that an unset value defaults
	// to enabled, while an explicit `enabled: false` refuses to run.
	Enabled *bool `yaml:"enabled"`
	// MaxAttempts is the number of agent+verify cycles to try. Defaults to 3.
	MaxAttempts int `yaml:"max_attempts"`
	// VerifyAfterEachAttempt reruns `lf verify` after each agent attempt. It is a
	// pointer so an unset value defaults to true. When false, repair invokes the
	// agent once and cannot confirm a fix (LunarForge is the only thing that can).
	VerifyAfterEachAttempt *bool `yaml:"verify_after_each_attempt"`
	// Agent names the entry in the agents map to use by default. The --agent flag
	// overrides it.
	Agent string `yaml:"agent"`
	// MaxLogChars caps how many characters of each failed command's stdout/stderr
	// are inlined into the prompt. Full logs always remain on disk. Defaults to
	// 20000.
	MaxLogChars int `yaml:"max_log_chars"`
}

// Agent is a single repair backend: an external command plus fixed arguments.
// LunarForge writes the generated repair prompt to the command's stdin, so the
// command must accept a prompt on stdin (e.g. `claude --print` or
// `codex exec -`). The model intentionally stays small: no plugin system.
type Agent struct {
	// Backend is an informational label (e.g. "claude_code" or "codex"). It is
	// recorded in artifacts but does not change how the command is invoked.
	Backend string   `yaml:"backend"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

// RepairEnabled reports whether repair is enabled (default true unless an
// explicit `enabled: false` is set).
func (c *Config) RepairEnabled() bool {
	return c.Repair.Enabled == nil || *c.Repair.Enabled
}

// RepairMaxAttempts returns the configured attempt count, defaulting to 3.
func (c *Config) RepairMaxAttempts() int {
	if c.Repair.MaxAttempts <= 0 {
		return 3
	}
	return c.Repair.MaxAttempts
}

// RepairVerifyAfterEach reports whether verify should run after each attempt
// (default true).
func (c *Config) RepairVerifyAfterEach() bool {
	return c.Repair.VerifyAfterEachAttempt == nil || *c.Repair.VerifyAfterEachAttempt
}

// RepairMaxLogChars returns the per-log truncation limit, defaulting to 20000.
func (c *Config) RepairMaxLogChars() int {
	if c.Repair.MaxLogChars <= 0 {
		return 20000
	}
	return c.Repair.MaxLogChars
}

// ResolveAgent returns the agent config for the given name, falling back to
// repair.agent when name is empty. It errors clearly when no agent is named or
// the named agent is missing/incomplete.
func (c *Config) ResolveAgent(name string) (string, Agent, error) {
	if name == "" {
		name = c.Repair.Agent
	}
	if name == "" {
		return "", Agent{}, fmt.Errorf("no repair agent configured: set repair.agent or pass --agent <name>")
	}
	a, ok := c.Agents[name]
	if !ok {
		return "", Agent{}, fmt.Errorf("repair agent %q is not defined under agents:", name)
	}
	if a.Command == "" {
		return "", Agent{}, fmt.Errorf("repair agent %q has no command", name)
	}
	return name, a, nil
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
