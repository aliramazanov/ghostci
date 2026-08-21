package expr

import (
	"fmt"
	"strings"
)

func EvalCondition(src string, ctx Context) (bool, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return true, nil
	}

	if inner, ok := unwrap(src); ok {
		src = inner
	}

	n, err := Parse(src)

	if err != nil {
		return false, err
	}

	v, err := Eval(n, ctx)

	if err != nil {
		return false, err
	}

	return truthy(v), nil
}

func Interpolate(src string, ctx Context) (string, error) {
	if !strings.Contains(src, "${{") {
		return src, nil
	}

	var sb strings.Builder
	rest := src

	for {
		start := strings.Index(rest, "${{")

		if start < 0 {
			sb.WriteString(rest)
			return sb.String(), nil
		}

		end := findClose(rest, start+3)

		if end < 0 {
			return "", fmt.Errorf("expr: unclosed ${{ in %q", src)
		}

		sb.WriteString(rest[:start])
		inner := rest[start+3 : end]

		n, err := Parse(strings.TrimSpace(inner))
		if err != nil {
			return "", err
		}

		v, err := Eval(n, ctx)
		if err != nil {
			return "", err
		}

		sb.WriteString(toString(v))
		rest = rest[end+2:]
	}
}

func References(src string) []string {
	seen := map[string]bool{}
	var out []string
	for _, frag := range fragments(src) {
		n, err := Parse(frag)
		if err != nil {
			continue
		}

		walk(n, func(node Node) {
			id, ok := node.(Ident)
			if !ok {
				return
			}

			name := lower(id.Name)
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		})
	}
	return out
}

func Fragments(src string) []string { return fragments(src) }

func EvalString(src string, ctx Context) (string, error) {
	n, err := Parse(strings.TrimSpace(src))
	if err != nil {
		return "", err
	}

	v, err := Eval(n, ctx)
	if err != nil {
		return "", err
	}

	return toString(v), nil
}

func fragments(src string) []string {
	src = strings.TrimSpace(src)
	if inner, ok := unwrap(src); ok {
		return []string{inner}
	}

	if !strings.Contains(src, "${{") {
		return []string{src}
	}

	var out []string
	rest := src

	for {
		start := strings.Index(rest, "${{")
		if start < 0 {
			return out
		}

		end := findClose(rest, start+3)
		if end < 0 {
			return out
		}

		out = append(out, strings.TrimSpace(rest[start+3:end]))
		rest = rest[end+2:]
	}
}

func unwrap(s string) (string, bool) {
	if !strings.HasPrefix(s, "${{") || !strings.HasSuffix(s, "}}") {
		return "", false
	}

	inner := s[3 : len(s)-2]
	if strings.Contains(inner, "${{") {
		return "", false
	}

	return strings.TrimSpace(inner), true
}

func walk(n Node, fn func(Node)) {
	if n == nil {
		return
	}
	fn(n)
	switch v := n.(type) {
	case Property:
		walk(v.Target, fn)
	case Index:
		walk(v.Target, fn)
		walk(v.Key, fn)
	case Splat:
		walk(v.Target, fn)
	case Unary:
		walk(v.Operand, fn)
	case Binary:
		walk(v.Left, fn)
		walk(v.Right, fn)
	case Call:
		for _, a := range v.Args {
			walk(a, fn)
		}
	}
}

func findClose(s string, from int) int {
	inQuote := false

	for i := from; i < len(s); i++ {
		switch {
		case s[i] == '\'':

			if inQuote && i+1 < len(s) && s[i+1] == '\'' {
				i++

				continue
			}

			inQuote = !inQuote

		case !inQuote && s[i] == '}' && i+1 < len(s) && s[i+1] == '}':
			return i
		}
	}

	return -1
}
