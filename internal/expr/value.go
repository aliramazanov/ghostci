package expr

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Strict struct {
	Name   string
	Values map[string]any
}

func property(target any, name string) any {
	switch t := target.(type) {
	case Strict:
		for k, v := range t.Values {
			if strings.EqualFold(k, name) {
				return v
			}
		}

		return Unknown(t.Name + "." + name)
	case map[string]any:
		for k, v := range t {
			if strings.EqualFold(k, name) {
				return v
			}
		}
	case Context:
		for k, v := range t {
			if strings.EqualFold(k, name) {
				return v
			}
		}
	}
	return nil
}

func index(target, key any) any {
	switch t := target.(type) {
	case []any:
		i := int(toNumber(key))
		if i >= 0 && i < len(t) {
			return t[i]
		}
	case map[string]any:
		return property(t, toString(key))
	}
	return nil
}

func splat(target any, name string) any {
	pick := func(out []any, item any) []any {
		if name == "" {
			return append(out, item)
		}
		if v := property(item, name); v != nil {
			return append(out, v)
		}
		return out
	}

	switch t := target.(type) {
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = pick(out, item)
		}
		return out

	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		out := make([]any, 0, len(t))
		for _, k := range keys {
			out = pick(out, t[k])
		}
		return out
	}

	return []any{}
}

func looseEqual(a, b any) bool {
	as, aIsStr := a.(string)
	bs, bIsStr := b.(string)
	if aIsStr && bIsStr {
		return strings.EqualFold(as, bs)
	}
	if a == nil && b == nil {
		return true
	}
	an, bn := toNumber(a), toNumber(b)
	if math.IsNaN(an) || math.IsNaN(bn) {
		return false
	}
	return an == bn
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0 && !math.IsNaN(t)
	case string:
		return t != ""
	case []any:
		return true
	case map[string]any:
		return true
	}
	return v != nil
}

func toNumber(v any) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case bool:
		if t {
			return 1
		}
		return 0
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		return parseNumber(t)
	}
	return math.NaN()
}

func parseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if base, digits, ok := radix(s); ok {
		n, err := strconv.ParseUint(digits, base, 64)
		if err != nil {
			return math.NaN()
		}

		return float64(n)
	}

	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}

	if strings.ContainsAny(s, "_xXpPnNiI") {
		return math.NaN()
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}

	return f
}

func radix(s string) (base int, digits string, ok bool) {
	if len(s) < 3 || s[0] != '0' {
		return 0, "", false
	}

	switch s[1] {
	case 'x', 'X':
		return 16, s[2:], true
	case 'o', 'O':
		return 8, s[2:], true
	case 'b', 'B':
		return 2, s[2:], true
	}

	return 0, "", false
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case string:
		return t
	case []any:
		return "Array"
	case map[string]any:
		return "Object"
	}

	return fmt.Sprint(v)
}

func lower(s string) string { return strings.ToLower(s) }
