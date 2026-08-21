package workflow

import (
	"fmt"
	"strings"

	"github.com/aliramazanov/ghostci/internal/pipeline"
	"go.yaml.in/yaml/v3"
)

var ErrDynamicMatrix = fmt.Errorf("workflow: matrix is computed at run time")

const MaxCombinations = pipeline.MaxCombinations

func ExpandMatrix(s *Strategy) ([]Combination, bool, error) {
	if s == nil || s.Matrix.IsZero() {
		return []Combination{{}}, false, nil
	}
	if s.Matrix.Kind == yaml.ScalarNode && strings.Contains(s.Matrix.Value, "${{") {
		return nil, false, ErrDynamicMatrix
	}
	if s.Matrix.Kind != yaml.MappingNode {
		return nil, false, ErrDynamicMatrix
	}

	axes, includes, excludes, err := splitMatrix(&s.Matrix)
	if err != nil {
		return nil, false, err
	}

	axisKeys := make(map[string]bool, len(axes))
	for _, a := range axes {
		axisKeys[a.key] = true
	}

	combos, truncated := expandAxes(axes)
	combos = applyExcludes(combos, excludes)
	combos = applyIncludes(combos, includes, axisKeys)

	if len(combos) == 0 {
		combos = []Combination{{}}
	}
	if len(combos) > MaxCombinations {
		combos = combos[:MaxCombinations]
		truncated = true
	}
	return combos, truncated, nil
}

type axis struct {
	key    string
	values []any
}

func expandAxes(axes []axis) ([]Combination, bool) {
	shared := make([]pipeline.Axis, 0, len(axes))
	for _, a := range axes {
		shared = append(shared, pipeline.Axis{Key: a.key, Values: a.values})
	}

	expanded, truncated := pipeline.Cartesian(shared)

	combos := make([]Combination, 0, len(expanded))
	for _, c := range expanded {
		combos = append(combos, Combination(c))
	}

	return combos, truncated
}

func splitMatrix(node *yaml.Node) (axes []axis, includes, excludes []Combination, err error) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		if val.Kind == yaml.ScalarNode && strings.Contains(val.Value, "${{") {
			return nil, nil, nil, ErrDynamicMatrix
		}

		switch key {
		case "include":
			includes, err = decodeCombinations(val)
		case "exclude":
			excludes, err = decodeCombinations(val)
		default:
			if val.Kind != yaml.SequenceNode {
				return nil, nil, nil, ErrDynamicMatrix
			}
			var vals []any
			if err := val.Decode(&vals); err != nil {
				return nil, nil, nil, err
			}
			axes = append(axes, axis{key: key, values: vals})
		}
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return axes, includes, excludes, nil
}

func decodeCombinations(node *yaml.Node) ([]Combination, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, nil
	}
	var raw []map[string]any
	if err := node.Decode(&raw); err != nil {
		return nil, fmt.Errorf("workflow: decoding matrix entries: %w", err)
	}
	out := make([]Combination, 0, len(raw))
	for _, m := range raw {
		out = append(out, Combination(m))
	}
	return out, nil
}

func applyExcludes(combos []Combination, excludes []Combination) []Combination {
	if len(excludes) == 0 {
		return combos
	}
	out := combos[:0:0]
	for _, c := range combos {
		drop := false
		for _, ex := range excludes {
			if matches(c, ex) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, c)
		}
	}
	return out
}

func applyIncludes(combos []Combination, includes []Combination, axisKeys map[string]bool) []Combination {

	if len(axisKeys) == 0 {
		out := make([]Combination, 0, len(includes))
		for _, inc := range includes {
			out = append(out, cloneCombination(inc))
		}

		if len(out) == 0 {
			return combos
		}

		return out
	}

	base := len(combos)

	for _, inc := range includes {
		matched := false
		for i := 0; i < base; i++ {
			if !compatible(combos[i], inc, axisKeys) {
				continue
			}
			matched = true
			for k, v := range inc {
				if !axisKeys[k] {
					combos[i][k] = v
				}
			}
		}
		if !matched {
			combos = append(combos, cloneCombination(inc))
		}
	}
	return combos
}

func compatible(c, inc Combination, axisKeys map[string]bool) bool {
	for k, v := range inc {
		if !axisKeys[k] {
			continue
		}
		if !equalValue(c[k], v) {
			return false
		}
	}
	return true
}
