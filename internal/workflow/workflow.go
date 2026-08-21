package workflow

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Workflow struct {
	Path string
	Name string    `yaml:"name"`
	On   yaml.Node `yaml:"on"`

	CallInputs map[string]ActionInput `yaml:"-"`
	Env        Env                    `yaml:"env"`
	Defaults   *Defaults              `yaml:"defaults"`
	Jobs       Jobs                   `yaml:"jobs"`
}

type Jobs []NamedJob

type NamedJob struct {
	ID  string
	Job Job
}

type Job struct {
	TimeoutMinutes yaml.Node            `yaml:"timeout-minutes"`
	Name           string               `yaml:"name"`
	RunsOn         yaml.Node            `yaml:"runs-on"`
	Needs          yaml.Node            `yaml:"needs"`
	If             string               `yaml:"if"`
	Env            Env                  `yaml:"env"`
	Strategy       *Strategy            `yaml:"strategy"`
	Steps          []Step               `yaml:"steps"`
	Services       map[string]yaml.Node `yaml:"services"`
	Container      yaml.Node            `yaml:"container"`
	Defaults       *Defaults            `yaml:"defaults"`
	Uses           string               `yaml:"uses"`
	With           Env                  `yaml:"with"`
}

type Step struct {
	Name             string    `yaml:"name"`
	ID               string    `yaml:"id"`
	If               string    `yaml:"if"`
	Run              string    `yaml:"run"`
	Uses             string    `yaml:"uses"`
	With             Env       `yaml:"with"`
	Env              Env       `yaml:"env"`
	Shell            string    `yaml:"shell"`
	WorkingDirectory string    `yaml:"working-directory"`
	ContinueOnError  yaml.Node `yaml:"continue-on-error"`
	TimeoutMinutes   yaml.Node `yaml:"timeout-minutes"`
}

type Strategy struct {
	Matrix yaml.Node `yaml:"matrix"`
}

type Defaults struct {
	Run *DefaultsRun `yaml:"run"`
}

type DefaultsRun struct {
	Shell            string `yaml:"shell"`
	WorkingDirectory string `yaml:"working-directory"`
}

type Env map[string]string

func (e *Env) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	out := make(Env, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		out[k] = node.Content[i+1].Value
	}
	*e = out
	return nil
}

func (j *Jobs) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	out := make(Jobs, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		var job Job
		if err := node.Content[i+1].Decode(&job); err != nil {
			return fmt.Errorf("workflow: job %q: %w", node.Content[i].Value, err)
		}
		out = append(out, NamedJob{ID: node.Content[i].Value, Job: job})
	}
	*j = out
	return nil
}

func Load(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workflow: reading %s: %w", path, err)
	}
	wf, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("workflow: %s: %w", path, err)
	}
	wf.Path = path
	wf.CallInputs = wf.callInputs()

	return wf, nil
}

func Parse(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func (w *Workflow) TriggersOn(events ...string) bool {
	want := make(map[string]bool, len(events))
	for _, e := range events {
		want[e] = true
	}
	for _, got := range w.triggers() {
		if want[got] {
			return true
		}
	}
	return false
}

func (w *Workflow) Triggers() []string { return w.triggers() }

func (w *Workflow) triggers() []string {
	switch w.On.Kind {
	case yaml.ScalarNode:
		return []string{w.On.Value}
	case yaml.SequenceNode:
		out := make([]string, 0, len(w.On.Content))
		for _, n := range w.On.Content {
			out = append(out, n.Value)
		}
		return out
	case yaml.MappingNode:
		out := make([]string, 0, len(w.On.Content)/2)
		for i := 0; i < len(w.On.Content); i += 2 {
			out = append(out, w.On.Content[i].Value)
		}
		return out
	}
	return nil
}

func (j Job) RunsOnLabels() []string {
	switch j.RunsOn.Kind {
	case yaml.ScalarNode:
		return []string{j.RunsOn.Value}
	case yaml.SequenceNode:
		out := make([]string, 0, len(j.RunsOn.Content))
		for _, n := range j.RunsOn.Content {
			out = append(out, n.Value)
		}
		return out
	}
	return nil
}

func RunnerOS(labels []string) string {
	for _, l := range labels {
		switch {
		case strings.Contains(strings.ToLower(l), "windows"):
			return "Windows"
		case strings.Contains(strings.ToLower(l), "macos"):
			return "macOS"
		case strings.Contains(strings.ToLower(l), "ubuntu"), strings.Contains(strings.ToLower(l), "linux"):
			return "Linux"
		}
	}
	return "Linux"
}

func (w *Workflow) callInputs() map[string]ActionInput {
	if w.On.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(w.On.Content); i += 2 {
		if w.On.Content[i].Value != "workflow_call" {
			continue
		}

		var call struct {
			Inputs map[string]ActionInput `yaml:"inputs"`
		}

		if err := w.On.Content[i+1].Decode(&call); err != nil {
			return nil
		}

		return call.Inputs
	}

	return nil
}

func (w *Workflow) IsReusable() bool { return w.TriggersOn("workflow_call") }

func (w *Workflow) PathFilters(events ...string) (paths, ignore []string) {
	filters, ok := eventFilters(w.On, events)
	if !ok || len(filters) == 0 {
		return nil, nil
	}

	for _, f := range filters {
		if len(f.Paths) == 0 && len(f.PathsIgnore) == 0 {
			return nil, nil
		}
	}

	if len(filters) == 1 {
		return filters[0].Paths, filters[0].PathsIgnore
	}

	if every(filters, func(f pathFilter) bool { return len(f.Paths) > 0 }) {

		seen := map[string]bool{}

		for _, f := range filters {
			for _, glob := range f.Paths {
				if seen[glob] {
					continue
				}

				seen[glob] = true
				paths = append(paths, glob)
			}
		}

		return paths, nil
	}

	first := filters[0].PathsIgnore
	if every(filters, func(f pathFilter) bool { return slices.Equal(f.PathsIgnore, first) }) {
		return nil, first
	}

	return nil, nil
}

type pathFilter struct {
	Paths       []string `yaml:"paths"`
	PathsIgnore []string `yaml:"paths-ignore"`
}

func eventFilters(on yaml.Node, events []string) (filters []pathFilter, ok bool) {
	if on.Kind != yaml.MappingNode {
		return nil, true
	}

	want := make(map[string]bool, len(events))
	for _, e := range events {
		want[e] = true
	}

	for i := 0; i+1 < len(on.Content); i += 2 {
		if !want[on.Content[i].Value] {
			continue
		}

		var filter pathFilter
		if err := on.Content[i+1].Decode(&filter); err != nil {
			return nil, false
		}

		include, exclude := splitNegations(filter.Paths)
		filter.Paths = include
		filter.PathsIgnore = append(filter.PathsIgnore, exclude...)

		filters = append(filters, filter)
	}

	return filters, true
}

func splitNegations(globs []string) (include, exclude []string) {
	for _, g := range globs {
		switch {

		case strings.HasPrefix(g, "!!"):
			include = append(include, g[1:])
		case strings.HasPrefix(g, "!"):
			exclude = append(exclude, g[1:])
		default:
			include = append(include, g)
		}
	}

	return include, exclude
}

func every(filters []pathFilter, pred func(pathFilter) bool) bool {
	for _, f := range filters {
		if !pred(f) {
			return false
		}
	}

	return true
}

func (s Step) Optional() bool {
	return s.ContinueOnError.Kind == yaml.ScalarNode && s.ContinueOnError.Value == "true"
}

func (s Step) Timeout() time.Duration { return minutes(s.TimeoutMinutes) }

func (j Job) Timeout() time.Duration { return minutes(j.TimeoutMinutes) }

const maxTimeout = 30 * 24 * time.Hour

func minutes(n yaml.Node) time.Duration {
	if n.Kind != yaml.ScalarNode {
		return 0
	}

	v, err := strconv.ParseFloat(n.Value, 64)
	if err != nil || math.IsNaN(v) || v <= 0 {
		return 0
	}

	if scaled := v * float64(time.Minute); scaled < float64(maxTimeout) {
		return time.Duration(scaled)
	}

	return maxTimeout
}
