package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Checks []Check `yaml:"checks"`
}

type Check struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Dir     string            `yaml:"dir,omitempty"`
	Shell   string            `yaml:"shell,omitempty"`
	Timeout Duration          `yaml:"timeout,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`

	Inputs []string `yaml:"inputs,omitempty"`

	Optional bool `yaml:"optional,omitempty"`

	Serial bool `yaml:"serial,omitempty"`

	Exclude []string `yaml:"exclude,omitempty"`
}

func (c Check) AlwaysRuns() bool {
	if len(c.Inputs) == 0 {
		return true
	}

	for _, g := range c.Inputs {
		if g == "**" || g == "**/*" {
			return true
		}
	}

	return false
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string

	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("config: timeout must be a duration string like \"90s\": %w", err)
	}

	parsed, err := time.ParseDuration(s)

	if err != nil {
		return fmt.Errorf("config: invalid timeout %q: %w", s, err)
	}

	if parsed <= 0 {
		return fmt.Errorf("config: invalid timeout %q: must be positive", s)
	}

	*d = Duration(parsed)

	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

var ErrNoChecks = errors.New("config: no checks defined")

type NotFoundError struct {
	Path string
	Err  error
}

func (e *NotFoundError) Error() string {
	return "config: no " + e.Path
}

func (e *NotFoundError) Unwrap() error { return e.Err }

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{Path: path, Err: err}
		}

		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}

	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Checks) == 0 {
		return ErrNoChecks
	}

	seen := make(map[string]int, len(c.Checks))

	for i, chk := range c.Checks {
		switch {
		case chk.Name == "":
			return fmt.Errorf("config: check %d: name is required", i)
		case chk.Command == "":
			return fmt.Errorf("config: check %q: command is required", chk.Name)
		}
		if prev, dup := seen[chk.Name]; dup {
			return fmt.Errorf("config: check %q: duplicate name, already defined at index %d", chk.Name, prev)
		}
		seen[chk.Name] = i

		if err := validateGlobs(chk); err != nil {
			return err
		}
	}

	return nil
}

func validateGlobs(chk Check) error {
	for field, globs := range map[string][]string{"inputs": chk.Inputs, "exclude": chk.Exclude} {
		for _, g := range globs {
			if !doublestar.ValidatePattern(g) {
				return fmt.Errorf("config: check %q: %s: %q is not a valid glob", chk.Name, field, g)
			}
		}
	}

	return nil
}
