// Package config loads and validates the repository-local LunarForge policy.
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	FileName              = ".lunarforge.yml"
	DefaultTimeoutSeconds = 30 * 60
)

var commandIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Config deliberately contains only repository quality policy. Agent choice,
// retry behavior, evidence paths, and CI presentation belong to callers.
type Config struct {
	Version int    `yaml:"version"`
	Verify  Verify `yaml:"verify"`

	path string `yaml:"-"`
}

type Verify struct {
	Commands []Command `yaml:"commands"`
}

// Command is one required repository check. TimeoutSeconds defaults to 30
// minutes when omitted.
type Command struct {
	ID             string `yaml:"id"`
	Run            string `yaml:"run"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty"`
}

func (c Command) Timeout() time.Duration {
	seconds := c.TimeoutSeconds
	if seconds == 0 {
		seconds = DefaultTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (c *Config) Path() string { return c.path }

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		return nil, fmt.Errorf("parsing %s: multiple YAML documents are not supported", path)
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
		if !commandIDPattern.MatchString(cmd.ID) {
			return fmt.Errorf("verify.commands[%d].id %q must match %s", i, cmd.ID, commandIDPattern)
		}
		if strings.TrimSpace(cmd.Run) == "" {
			return fmt.Errorf("verify.commands[%d] (%s).run is required", i, cmd.ID)
		}
		if cmd.TimeoutSeconds < 0 {
			return fmt.Errorf("verify.commands[%d] (%s).timeout_seconds cannot be negative", i, cmd.ID)
		}
		if seen[cmd.ID] {
			return fmt.Errorf("duplicate verify command id %q", cmd.ID)
		}
		seen[cmd.ID] = true
	}
	return nil
}
