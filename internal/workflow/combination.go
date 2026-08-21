package workflow

import (
	"fmt"
	"sort"
	"strings"
)

type Combination map[string]any

func matches(c, pattern Combination) bool {
	for k, v := range pattern {
		if !equalValue(c[k], v) {
			return false
		}
	}
	return true
}

func equalValue(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func cloneCombination(c Combination) Combination {
	out := make(Combination, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}

func (c Combination) Label() string {
	if len(c) == 0 {
		return ""
	}
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, label(c[k])...)
	}
	return strings.Join(parts, "-")
}

func (c Combination) AsContext() map[string]any {
	out := make(map[string]any, len(c))
	for k, v := range c {
		out[k] = normalise(v)
	}
	return out
}

func normalise(v any) any {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = normalise(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = normalise(vv)
		}
		return out
	}
	return v
}

func label(v any) []string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		out := make([]string, 0, len(keys))
		for _, k := range keys {
			out = append(out, label(t[k])...)
		}

		return out

	case []any:
		return nil
	}

	s := fmt.Sprint(v)
	if s == "" || strings.ContainsAny(s, "{}[]") {
		return nil
	}

	return []string{s}
}
