package expr

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func evalCall(c Call, ctx Context) (any, error) {
	switch c.Name {
	case "success", "always":
		return true, nil
	case "failure", "cancelled":
		return false, nil
	}

	args := make([]any, len(c.Args))

	for i, a := range c.Args {
		v, err := Eval(a, ctx)
		if err != nil {
			return nil, err
		}

		if err := deepDecidable(v); err != nil {
			return nil, err
		}

		args[i] = v
	}

	switch c.Name {
	case "contains":
		if err := arity(c.Name, args, 2); err != nil {
			return nil, err
		}
		return contains(args[0], args[1]), nil

	case "startswith":
		if err := arity(c.Name, args, 2); err != nil {
			return nil, err
		}
		return strings.HasPrefix(lower(toString(args[0])), lower(toString(args[1]))), nil

	case "endswith":
		if err := arity(c.Name, args, 2); err != nil {
			return nil, err
		}
		return strings.HasSuffix(lower(toString(args[0])), lower(toString(args[1]))), nil

	case "format":
		if len(args) == 0 {
			return nil, fmt.Errorf("expr: format() needs a template")
		}

		out, err := format(toString(args[0]), args[1:])
		if err != nil {
			return nil, err
		}

		return out, nil

	case "join":
		if len(args) == 0 {
			return nil, fmt.Errorf("expr: join() needs an array")
		}

		sep := ","
		if len(args) > 1 {
			sep = toString(args[1])
		}

		return join(args[0], sep), nil

	case "tojson":
		if err := arity(c.Name, args, 1); err != nil {
			return nil, err
		}

		b, err := json.MarshalIndent(args[0], "", "  ")
		if err != nil {
			return nil, err
		}

		return string(b), nil

	case "fromjson":
		if err := arity(c.Name, args, 1); err != nil {
			return nil, err
		}

		var out any

		if err := json.Unmarshal([]byte(toString(args[0])), &out); err != nil {
			return nil, fmt.Errorf("expr: fromJSON: %w", err)
		}
		return out, nil

	case "hashfiles":
		return nil, &UndecidableError{Context: "hashFiles()"}
	}

	return nil, fmt.Errorf("expr: unsupported function %s()", c.Name)
}

func arity(name string, args []any, want int) error {
	if len(args) != want {
		return fmt.Errorf("expr: %s() takes %d arguments, got %d", name, want, len(args))
	}

	return nil
}

func contains(haystack, needle any) bool {
	if arr, ok := haystack.([]any); ok {
		for _, item := range arr {
			if looseEqual(item, needle) {
				return true
			}
		}
		return false
	}

	return strings.Contains(lower(toString(haystack)), lower(toString(needle)))
}

func join(v any, sep string) string {
	arr, ok := v.([]any)
	if !ok {
		return toString(v)
	}

	parts := make([]string, len(arr))
	for i, item := range arr {
		parts[i] = toString(item)
	}

	return strings.Join(parts, sep)
}

func format(tmpl string, args []any) (string, error) {
	var sb strings.Builder

	for i := 0; i < len(tmpl); i++ {
		switch c := tmpl[i]; c {
		case '{':
			if i+1 < len(tmpl) && tmpl[i+1] == '{' {
				sb.WriteByte('{')
				i++

				continue
			}

			end := strings.IndexByte(tmpl[i+1:], '}')
			if end < 0 {
				return "", fmt.Errorf("expr: format(): unclosed { in %q", tmpl)
			}

			idx, err := strconv.Atoi(tmpl[i+1 : i+1+end])
			if err != nil || idx < 0 {
				return "", fmt.Errorf("expr: format(): %q is not an argument index", tmpl[i:i+end+2])
			}

			if idx >= len(args) {
				return "", fmt.Errorf("expr: format(): %q refers to argument %d but only %d given",
					tmpl, idx, len(args))
			}

			sb.WriteString(toString(args[idx]))
			i += end + 1

		case '}':
			if i+1 < len(tmpl) && tmpl[i+1] == '}' {
				sb.WriteByte('}')
				i++

				continue
			}

			return "", fmt.Errorf("expr: format(): unmatched } in %q", tmpl)

		default:
			sb.WriteByte(c)
		}
	}

	return sb.String(), nil
}
