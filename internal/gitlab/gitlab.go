package gitlab

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var reserved = map[string]bool{
	"stages":        true,
	"variables":     true,
	"default":       true,
	"workflow":      true,
	"include":       true,
	"image":         true,
	"services":      true,
	"cache":         true,
	"before_script": true,
	"after_script":  true,
	"types":         true,
}

type File struct {
	Path      string
	Variables Variables
	Default   *Default
	Workflow  *Workflow
	Includes  []Include
	Jobs      []NamedJob

	Templates map[string]Job

	Inputs map[string]SpecInput
}

type Default struct {
	Image         yaml.Node `yaml:"image"`
	Services      yaml.Node `yaml:"services"`
	BeforeScript  Script    `yaml:"before_script"`
	AfterScript   Script    `yaml:"after_script"`
	Timeout       string    `yaml:"timeout"`
	Interruptible *bool     `yaml:"interruptible"`
}

type Workflow struct {
	Rules []Rule `yaml:"rules"`
	Name  string `yaml:"name"`
}

type NamedJob struct {
	Name string
	Job  Job
}

type Job struct {
	Stage        string    `yaml:"stage"`
	Script       Script    `yaml:"script"`
	BeforeScript Script    `yaml:"before_script"`
	AfterScript  Script    `yaml:"after_script"`
	Variables    Variables `yaml:"variables"`
	Rules        []Rule    `yaml:"rules"`
	Extends      Strings   `yaml:"extends"`
	Image        yaml.Node `yaml:"image"`
	Services     yaml.Node `yaml:"services"`
	Parallel     yaml.Node `yaml:"parallel"`
	When         string    `yaml:"when"`
	AllowFailure yaml.Node `yaml:"allow_failure"`
	Timeout      string    `yaml:"timeout"`
	Trigger      yaml.Node `yaml:"trigger"`

	Only   yaml.Node `yaml:"only"`
	Except yaml.Node `yaml:"except"`

	Needs        yaml.Node `yaml:"needs"`
	Dependencies yaml.Node `yaml:"dependencies"`
	Artifacts    yaml.Node `yaml:"artifacts"`
	Cache        yaml.Node `yaml:"cache"`
	Environment  yaml.Node `yaml:"environment"`
}

type Rule struct {
	If        string    `yaml:"if"`
	Changes   Changes   `yaml:"changes"`
	Exists    Strings   `yaml:"exists"`
	When      string    `yaml:"when"`
	Variables Variables `yaml:"variables"`

	AllowFailure yaml.Node `yaml:"allow_failure"`
}

type Changes struct {
	Paths     []string
	CompareTo string
}

func (c *Changes) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&c.Paths)
	case yaml.MappingNode:
		var m struct {
			Paths     []string `yaml:"paths"`
			CompareTo string   `yaml:"compare_to"`
		}
		if err := node.Decode(&m); err != nil {
			return err
		}
		c.Paths, c.CompareTo = m.Paths, m.CompareTo
	}

	return nil
}

func (c Changes) IsZero() bool { return len(c.Paths) == 0 }

type Script []string

func (s *Script) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*s = Script{node.Value}
	case yaml.SequenceNode:
		var out []string
		for _, item := range node.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				out = append(out, item.Value)
			case yaml.SequenceNode:
				var nested Script
				if err := nested.UnmarshalYAML(item); err != nil {
					return err
				}
				out = append(out, nested...)
			}
		}
		*s = Script(out)
	}

	return nil
}

type Strings []string

func (s *Strings) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*s = Strings{node.Value}
	case yaml.SequenceNode:
		var out []string
		if err := node.Decode(&out); err != nil {
			return err
		}
		*s = Strings(out)
	}

	return nil
}

type Variables map[string]string

func (v *Variables) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	out := make(Variables, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i].Value, node.Content[i+1]
		switch val.Kind {
		case yaml.ScalarNode:
			out[key] = val.Value
		case yaml.MappingNode:
			var m struct {
				Value string `yaml:"value"`
			}
			if err := val.Decode(&m); err != nil {
				return err
			}
			out[key] = m.Value
		}
	}
	*v = out

	return nil
}

type Include struct {
	Local     string
	Remote    string
	Project   string
	Template  string
	Component string
	Raw       string
}

func (i Include) IsLocal() bool { return i.Local != "" }

func (i Include) Describe() string {
	switch {
	case i.Local != "":
		return "local " + i.Local
	case i.Remote != "":
		return "remote " + i.Remote
	case i.Project != "":
		return "project " + i.Project
	case i.Template != "":
		return "template " + i.Template
	case i.Component != "":
		return "component " + i.Component
	}

	return i.Raw
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gitlab: reading %s: %w", path, err)
	}

	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("gitlab: %s: %w", path, err)
	}
	f.Path = path

	return f, nil
}

func Parse(data []byte) (*File, error) {
	root, inputs, err := configDocument(data)
	if err != nil {
		return nil, err
	}
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("gitlab: not a mapping")
	}

	substituteInputs(root, inputs)

	if err := resolveReferences(root, root); err != nil {
		return nil, err
	}

	f := &File{Templates: map[string]Job{}, Inputs: inputs}

	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i].Value, root.Content[i+1]

		switch {
		case key == "variables":
			if err := val.Decode(&f.Variables); err != nil {
				return nil, err
			}
		case key == "default":
			var d Default
			if err := val.Decode(&d); err != nil {
				return nil, err
			}
			f.Default = &d
		case key == "workflow":
			var w Workflow
			if err := val.Decode(&w); err != nil {
				return nil, err
			}
			f.Workflow = &w
		case key == "include":
			f.Includes = append(f.Includes, decodeIncludes(val)...)
		case reserved[key]:
		case strings.HasPrefix(key, "."):

			if val.Kind != yaml.MappingNode {
				continue
			}

			var job Job
			if err := val.Decode(&job); err != nil {
				continue
			}

			f.Templates[key] = job
		default:
			if val.Kind != yaml.MappingNode {
				continue
			}
			var job Job
			if err := val.Decode(&job); err != nil {
				return nil, fmt.Errorf("gitlab: job %q: %w", key, err)
			}
			f.Jobs = append(f.Jobs, NamedJob{Name: key, Job: job})
		}
	}

	return f, nil
}

func configDocument(data []byte) (*yaml.Node, map[string]SpecInput, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var (
		docs   []*yaml.Node
		inputs map[string]SpecInput
	)

	for {
		var doc yaml.Node

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		docs = append(docs, documentRoot(&doc))
	}

	if len(docs) == 0 {
		return nil, nil, fmt.Errorf("gitlab: empty file")
	}

	if len(docs) > 1 {
		if declared, ok := specInputs(docs[0]); ok {
			return docs[len(docs)-1], declared, nil
		}
	}

	inputs, _ = specInputs(docs[0])

	return docs[0], inputs, nil
}

type SpecInput struct {
	Default     string `yaml:"default"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

func specInputs(root *yaml.Node) (map[string]SpecInput, bool) {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, false
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "spec" {
			continue
		}

		var spec struct {
			Inputs map[string]SpecInput `yaml:"inputs"`
		}

		if err := root.Content[i+1].Decode(&spec); err != nil {
			return nil, true
		}

		return spec.Inputs, true
	}

	return nil, false
}

func documentRoot(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}

	return n
}

func decodeIncludes(node *yaml.Node) []Include {
	switch node.Kind {
	case yaml.ScalarNode:
		return []Include{{Local: node.Value, Raw: node.Value}}
	case yaml.SequenceNode:
		var out []Include
		for _, item := range node.Content {
			out = append(out, decodeIncludes(item)...)
		}
		return out
	case yaml.MappingNode:
		var m struct {
			Local     string    `yaml:"local"`
			Remote    string    `yaml:"remote"`
			Template  string    `yaml:"template"`
			Component string    `yaml:"component"`
			Project   string    `yaml:"project"`
			File      yaml.Node `yaml:"file"`
		}
		if err := node.Decode(&m); err != nil {
			return nil
		}
		return []Include{{
			Local: m.Local, Remote: m.Remote, Template: m.Template,
			Component: m.Component, Project: m.Project,
		}}
	}

	return nil
}

func (f *File) Resolve(name string, job Job) (Job, error) {
	return Scope{Files: []*File{f}}.Resolve(name, job)
}

type Scope struct {
	Files []*File
}

func (s Scope) Resolve(name string, job Job) (Job, error) {
	seen := map[string]bool{name: true}

	return s.resolve(job, seen, 0)
}

func (s Scope) Lookup(name string) (Job, bool) {
	var (
		found Job
		ok    bool
	)

	for _, f := range s.Files {
		if job, hit := f.lookup(name); hit {
			found, ok = job, true
		}
	}

	return found, ok
}

func (s Scope) Defaults() *Default {
	var out *Default

	for _, f := range s.Files {
		if f.Default != nil {
			out = f.Default
		}
	}

	return out
}

func (s Scope) resolve(job Job, seen map[string]bool, depth int) (Job, error) {
	if depth > maxExtendsDepth {
		return job, fmt.Errorf("extends nested deeper than %d levels", maxExtendsDepth)
	}

	out := Job{}

	for _, parentName := range job.Extends {
		if seen[parentName] {
			return job, fmt.Errorf("extends forms a cycle at %q", parentName)
		}

		parent, ok := s.Lookup(parentName)
		if !ok {
			return job, fmt.Errorf("extends %q, which is not defined here", parentName)
		}

		seen[parentName] = true
		resolved, err := s.resolve(parent, seen, depth+1)
		delete(seen, parentName)

		if err != nil {
			return job, err
		}

		out = merge(out, resolved)
	}

	out = merge(out, job)

	if def := s.Defaults(); def != nil {
		if len(out.BeforeScript) == 0 {
			out.BeforeScript = def.BeforeScript
		}
		if len(out.AfterScript) == 0 {
			out.AfterScript = def.AfterScript
		}
		if out.Image.IsZero() {
			out.Image = def.Image
		}
		if out.Services.IsZero() {
			out.Services = def.Services
		}
		if out.Timeout == "" {
			out.Timeout = def.Timeout
		}
	}

	return out, nil
}

const maxExtendsDepth = 11

func (f *File) lookup(name string) (Job, bool) {
	if job, ok := f.Templates[name]; ok {
		return job, true
	}
	for _, nj := range f.Jobs {
		if nj.Name == name {
			return nj.Job, true
		}
	}

	return Job{}, false
}

func merge(base, child Job) Job {
	out := base

	if len(child.Script) > 0 {
		out.Script = child.Script
	}
	if len(child.BeforeScript) > 0 {
		out.BeforeScript = child.BeforeScript
	}
	if len(child.AfterScript) > 0 {
		out.AfterScript = child.AfterScript
	}
	if len(child.Rules) > 0 {
		out.Rules = child.Rules
	}
	if child.Stage != "" {
		out.Stage = child.Stage
	}
	if child.When != "" {
		out.When = child.When
	}
	if child.Timeout != "" {
		out.Timeout = child.Timeout
	}
	if len(child.Variables) > 0 {
		merged := make(Variables, len(out.Variables)+len(child.Variables))
		for k, v := range out.Variables {
			merged[k] = v
		}
		for k, v := range child.Variables {
			merged[k] = v
		}
		out.Variables = merged
	}

	for _, f := range []struct{ dst, src *yaml.Node }{
		{&out.Image, &child.Image},
		{&out.Services, &child.Services},
		{&out.Parallel, &child.Parallel},
		{&out.AllowFailure, &child.AllowFailure},
		{&out.Only, &child.Only},
		{&out.Except, &child.Except},
		{&out.Needs, &child.Needs},
		{&out.Dependencies, &child.Dependencies},
		{&out.Artifacts, &child.Artifacts},
		{&out.Cache, &child.Cache},
		{&out.Environment, &child.Environment},
		{&out.Trigger, &child.Trigger},
	} {
		if !f.src.IsZero() {
			*f.dst = *f.src
		}
	}

	return out
}

const maxReferenceDepth = 10

func resolveReferences(node, root *yaml.Node) error {
	return resolveReferencesAt(node, root, 0, map[*yaml.Node]bool{})
}

func resolveReferencesAt(node, root *yaml.Node, depth int, active map[*yaml.Node]bool) error {
	if node.Tag == "!reference" {
		if depth >= maxReferenceDepth {
			return fmt.Errorf("gitlab: !reference nested deeper than %d levels", maxReferenceDepth)
		}

		target, err := followReference(node, root)
		if err != nil {

			*node = yaml.Node{Kind: yaml.ScalarNode, Value: Unresolved + referencePath(node)}

			return nil
		}
		if active[target] {
			*node = yaml.Node{Kind: yaml.ScalarNode, Value: Unresolved + referencePath(node)}

			return nil
		}

		active[target] = true
		defer delete(active, target)

		*node = *cloneNode(target)

		return resolveReferencesAt(node, root, depth+1, active)
	}

	for _, child := range node.Content {
		if err := resolveReferencesAt(child, root, depth, active); err != nil {
			return err
		}
	}

	return nil
}

func cloneNode(n *yaml.Node) *yaml.Node {
	out := *n
	out.Content = make([]*yaml.Node, len(n.Content))

	for i, child := range n.Content {
		out.Content[i] = cloneNode(child)
	}

	return &out
}

var inputPattern = regexp.MustCompile(`\$\[\[ *inputs\.([A-Za-z0-9_-]+) *\]\]`)

const UnsetInput = "ghostci-unset-input"

func substituteInputs(node *yaml.Node, inputs map[string]SpecInput) {
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, "$[[") {
		node.Value = inputPattern.ReplaceAllStringFunc(node.Value, func(match string) string {
			name := inputPattern.FindStringSubmatch(match)[1]

			if in, ok := inputs[name]; ok && in.Default != "" {
				return in.Default
			}

			return UnsetInput + ":" + name
		})
	}

	for _, child := range node.Content {
		substituteInputs(child, inputs)
	}
}

const Unresolved = "ghostci-unresolved-reference: "

func referencePath(ref *yaml.Node) string {
	path := make([]string, 0, len(ref.Content))
	for _, item := range ref.Content {
		path = append(path, item.Value)
	}

	return strings.Join(path, ", ")
}

func followReference(ref, root *yaml.Node) (*yaml.Node, error) {
	path := make([]string, 0, len(ref.Content))
	for _, item := range ref.Content {
		path = append(path, item.Value)
	}
	if len(path) == 0 {
		return nil, fmt.Errorf("gitlab: empty !reference")
	}

	node := root
	for _, key := range path {
		if node.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("gitlab: !reference [%s] does not resolve", strings.Join(path, ", "))
		}
		found := false
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				node = node.Content[i+1]
				found = true

				break
			}
		}
		if !found {
			return nil, fmt.Errorf("gitlab: !reference [%s] does not resolve", strings.Join(path, ", "))
		}
	}

	return node, nil
}
