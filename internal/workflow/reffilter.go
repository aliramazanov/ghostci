package workflow

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

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
	if hasNegation(ignore) {
		return true
	}

	name := strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")

	if len(ignore) > 0 {
		matched, ok := filterMatch(ignore, ref, name)
		return !ok || !matched
	}

	matched, ok := filterMatch(patterns, ref, name)

	return !ok || matched
}

func filterMatch(patterns []string, ref, name string) (matched, ok bool) {
	for _, p := range patterns {
		negate := strings.HasPrefix(p, "!")
		if negate {
			p = p[1:]
		}

		re, err := filterRegexp(p)
		if err != nil {
			return false, false
		}

		if re.MatchString(name) || re.MatchString(ref) {
			matched = !negate
		}
	}

	return matched, true
}

func filterRegexp(pattern string) (*regexp.Regexp, error) {
	runes := []rune(pattern)

	var b strings.Builder
	b.WriteString("^")

	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; {
		case r == '*' && i+1 < len(runes) && runes[i+1] == '*':
			b.WriteString(".*")
			i++
		case r == '*':
			b.WriteString("[^/]*")
		case (r == '?' || r == '+') && i == 0:
			return nil, fmt.Errorf("workflow: %q starts with a quantifier", pattern)
		case r == '?' || r == '+':
			b.WriteRune(r)
		case r == '[':
			end := slices.Index(runes[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("workflow: unterminated [ in %q", pattern)
			}
			b.WriteString(string(runes[i : i+end+1]))
			i += end
		case r == '\\' && i+1 < len(runes):
			b.WriteString(regexp.QuoteMeta(string(runes[i+1])))
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}

	b.WriteString("$")

	return regexp.Compile(b.String())
}

func hasNegation(globs []string) bool {
	for _, g := range globs {
		if strings.HasPrefix(g, "!") && !strings.HasPrefix(g, "!!") {
			return true
		}
	}

	return false
}
