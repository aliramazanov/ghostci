package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Action struct {
	Path   string
	Name   string                 `yaml:"name"`
	Inputs map[string]ActionInput `yaml:"inputs"`
	Runs   ActionRuns             `yaml:"runs"`
}

type ActionInput struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

type ActionRuns struct {
	Using string `yaml:"using"`
	Steps []Step `yaml:"steps"`
}

func (a *Action) IsComposite() bool {
	return strings.EqualFold(a.Runs.Using, "composite")
}

func (a *Action) Defaults() map[string]string {
	out := make(map[string]string, len(a.Inputs))
	for name, in := range a.Inputs {
		out[name] = in.Default
	}
	return out
}

func (a *Action) Unsatisfied(with map[string]string) []string {
	var out []string

	for name, in := range a.Inputs {
		if !in.Required || in.Default != "" {
			continue
		}
		if _, given := with[name]; !given {
			out = append(out, name)
		}
	}

	sort.Strings(out)

	return out
}

var ErrNotAnAction = fmt.Errorf("workflow: no action.yml")

func LoadAction(root, ref string) (*Action, error) {
	rel := strings.TrimPrefix(strings.TrimPrefix(ref, "./"), ".\\")
	dir := filepath.Join(root, filepath.FromSlash(rel))

	for _, name := range []string{"action.yml", "action.yaml"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var a Action
		if err := yaml.Unmarshal(data, &a); err != nil {
			return nil, fmt.Errorf("workflow: %s: %w", path, err)
		}
		a.Path = path
		return &a, nil
	}
	return nil, fmt.Errorf("workflow: %s: %w", dir, ErrNotAnAction)
}

func CompositeShell(shell string) string {
	shell = strings.TrimSpace(strings.ReplaceAll(shell, "{0}", ""))
	return strings.TrimSpace(shell)
}
