package expr

import (
	"fmt"
	"math"
	"strings"

	"github.com/aliramazanov/ghostci/internal/pipeline"
)

type Context map[string]any

type UndecidableError = pipeline.UndecidableError

var undecidable = map[string]bool{
	"needs":   true,
	"steps":   true,
	"secrets": true,
	"inputs":  true,
	"jobs":    true,

	"vars": true,
}

type unknown struct{ value pipeline.Value }

func Unknown(source string) any { return unknown{value: pipeline.Unknown(source)} }

func decidable(v any) (any, error) {
	if u, ok := v.(unknown); ok {
		return nil, u.value.Err()
	}
	return v, nil
}

func deepDecidable(v any) error {
	switch t := v.(type) {
	case unknown:
		return t.value.Err()
	case map[string]any:
		for _, item := range t {
			if err := deepDecidable(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range t {
			if err := deepDecidable(item); err != nil {
				return err
			}
		}
	}

	return nil
}

func Eval(n Node, ctx Context) (any, error) {
	switch v := n.(type) {
	case Literal:
		return v.Value, nil

	case Ident:
		if got, ok := ctx[v.Name]; ok {
			return decidable(got)
		}
		if undecidable[lower(v.Name)] {
			return nil, &UndecidableError{Context: lower(v.Name)}
		}

		return nil, &UndecidableError{Context: lower(v.Name)}

	case Property:
		target, err := Eval(v.Target, ctx)
		if err != nil {
			return nil, err
		}
		return decidable(property(target, v.Name))

	case Index:
		target, err := Eval(v.Target, ctx)
		if err != nil {
			return nil, err
		}
		key, err := Eval(v.Key, ctx)
		if err != nil {
			return nil, err
		}
		return decidable(index(target, key))

	case Splat:
		target, err := Eval(v.Target, ctx)
		if err != nil {
			return nil, err
		}
		return splat(target, v.Name), nil

	case Unary:
		operand, err := Eval(v.Operand, ctx)
		if err != nil {
			return nil, err
		}
		return !truthy(operand), nil

	case Binary:
		return evalBinary(v, ctx)

	case Call:
		return evalCall(v, ctx)
	}

	return nil, fmt.Errorf("expr: unsupported expression node %T", n)
}

func evalBinary(v Binary, ctx Context) (any, error) {
	left, err := Eval(v.Left, ctx)

	if err != nil {
		return nil, err
	}

	switch v.Op {
	case tokAnd:
		if !truthy(left) {
			return left, nil
		}
		return Eval(v.Right, ctx)
	case tokOr:
		if truthy(left) {
			return left, nil
		}
		return Eval(v.Right, ctx)
	}

	right, err := Eval(v.Right, ctx)

	if err != nil {
		return nil, err
	}

	switch v.Op {
	case tokEq:
		return looseEqual(left, right), nil
	case tokNeq:
		return !looseEqual(left, right), nil
	case tokLt, tokLte, tokGt, tokGte:
		return order(v.Op, left, right), nil
	}
	return nil, fmt.Errorf("expr: unsupported operator")
}

func order(op tokenKind, left, right any) bool {
	ls, lok := left.(string)
	rs, rok := right.(string)

	if lok && rok {
		return compare(op, strings.Compare(lower(ls), lower(rs)), 0)
	}

	l, r := toNumber(left), toNumber(right)

	if math.IsNaN(l) || math.IsNaN(r) {
		return false
	}

	switch {
	case l < r:
		return compare(op, -1, 0)
	case l > r:
		return compare(op, 1, 0)
	}

	return compare(op, 0, 0)
}

func compare(op tokenKind, got, want int) bool {
	switch op {
	case tokLt:
		return got < want
	case tokLte:
		return got <= want
	case tokGt:
		return got > want
	case tokGte:
		return got >= want
	}

	return false
}
