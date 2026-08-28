package expr

import (
	"math"
	"testing"
)

func TestTruthinessFollowsTheDocumentedRules(t *testing.T) {
	t.Parallel()

	falsy := []any{false, 0.0, math.Copysign(0, -1), "", nil}
	for _, v := range falsy {
		if truthy(v) {
			t.Errorf("%#v is documented as false", v)
		}
	}

	truth := []any{
		true, 1.0, -1.0, 0.5, "a", "false", "0",
		[]any{}, []any{1}, map[string]any{}, map[string]any{"k": 1},
	}
	for _, v := range truth {
		if !truthy(v) {
			t.Errorf("%#v is not one of the documented false values, so it is true", v)
		}
	}
}

func TestNaNComparesFalseEitherWay(t *testing.T) {
	t.Parallel()

	ctx := Context{}

	for _, src := range []string{
		"'abc' < 1", "'abc' > 1", "'abc' <= 1", "'abc' >= 1",
		"1 < 'abc'", "1 > 'abc'", "1 <= 'abc'", "1 >= 'abc'",
	} {
		got, err := EvalCondition(src, ctx)
		if err != nil {
			t.Errorf("%s: %v", src, err)

			continue
		}
		if got {
			t.Errorf("%s = true, want false: a comparison against NaN answers nothing", src)
		}
	}
}

func TestComparisonCoercesTheDocumentedWay(t *testing.T) {
	t.Parallel()

	ctx := Context{"env": Strict{Name: "env", Values: map[string]any{}}}

	for _, c := range []struct {
		src  string
		want bool
	}{
		{"'ABC' == 'abc'", true},
		{"'abc' == 'abd'", false},
		{"'1' == 1", true},
		{"true == 1", true},
		{"false == 0", true},
		{"'abc' == 1", false},
		{"1 < 2", true},
		{"2 < 1", false},
	} {
		got, err := EvalCondition(c.src, ctx)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)

			continue
		}
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.src, got, c.want)
		}
	}
}

func TestNumbersRenderAsAWorkflowExpectsThem(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{22, "22"},
		{3.5, "3.5"},
		{1e20, "1e+20"},
		{-1e20, "-1e+20"},
	} {
		if got := toString(c.in); got != c.want {
			t.Errorf("toString(%v) = %q, want %q", c.in, got, c.want)
		}
	}

	got, err := EvalString("format('go{0}', 1.22)", Context{})

	if err != nil {
		t.Fatal(err)
	}

	if got != "go1.22" {
		t.Errorf("format rendered %q, want go1.22", got)
	}
}
