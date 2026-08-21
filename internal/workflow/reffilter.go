package workflow

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.yaml.in/yaml/v3"
)

type refFilter struct {
	Branches       []string `yaml:"branches"`
	BranchesIgnore []string `yaml:"branches-ignore"`
	Tags           []string `yaml:"tags"`
	TagsIgnore     []string `yaml:"tags-ignore"`
}

func (w *Workflow) PushMatchesRef(ref string) bool {
	if ref == "" || w.On.Kind != yaml.MappingNode {
		return true
	}

	for i := 0; i+1 < len(w.On.Content); i += 2 {
		if w.On.Content[i].Value != "push" {
			continue
		}

		var f refFilter
		if err := w.On.Content[i+1].Decode(&f); err != nil {
			return true
		}

		return refMatches(ref, f)
	}

	return true
}

func refMatches(ref string, f refFilter) bool {
	patterns, ignore := f.Branches, f.BranchesIgnore
	otherKind := len(f.Tags) > 0 || len(f.TagsIgnore) > 0
	if strings.HasPrefix(ref, "refs/tags/") {
		patterns, ignore = f.Tags, f.TagsIgnore
		otherKind = len(f.Branches) > 0 || len(f.BranchesIgnore) > 0
	}

	if len(patterns) == 0 && len(ignore) == 0 {
		return !otherKind
	}

	if len(patterns) > 0 && len(ignore) > 0 {
		return true
	}
	if hasNegation(patterns) || hasNegation(ignore) {
		return true
	}

	name := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")

	if len(ignore) > 0 {
		return !confidentMatch(ignore, ref, name)
	}

	for _, p := range patterns {
		if !confident(p) {
			return true
		}
	}

	return confidentMatch(patterns, ref, name)
}

func confident(pattern string) bool {
	return !strings.ContainsAny(pattern, "?+")
}

func confidentMatch(patterns []string, ref, name string) bool {
	for _, p := range patterns {
		if !confident(p) {
			continue
		}
		if ok, err := doublestar.Match(p, name); err == nil && ok {
			return true
		}
		if ok, err := doublestar.Match(p, ref); err == nil && ok {
			return true
		}
	}

	return false
}

func hasNegation(globs []string) bool {
	for _, g := range globs {
		if strings.HasPrefix(g, "!") && !strings.HasPrefix(g, "!!") {
			return true
		}
	}

	return false
}
