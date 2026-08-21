package pipeline

import "fmt"

type Combination map[string]any

type Axis struct {
	Key    string
	Values []any
}

const MaxCombinations = 64

func Cartesian(axes []Axis) (combos []Combination, truncated bool) {
	combos = []Combination{{}}

	for _, axis := range axes {
		next := make([]Combination, 0, len(combos)*len(axis.Values))

		for _, base := range combos {
			for _, v := range axis.Values {
				c := make(Combination, len(base)+1)
				for k, existing := range base {
					c[k] = existing
				}
				c[axis.Key] = v
				next = append(next, c)
			}
		}

		combos = next

		if len(combos) > MaxCombinations*4 {
			combos = combos[:MaxCombinations*4]
			truncated = true
		}
	}

	return combos, truncated
}

func (c Combination) Matches(filter Combination) bool {
	for k, want := range filter {
		got, ok := c[k]
		if !ok || !EqualValue(got, want) {
			return false
		}
	}

	return true
}

func EqualValue(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) }
