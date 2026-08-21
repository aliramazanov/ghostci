package workflow

import (
	"fmt"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func strategyOf(t *testing.T, src string) *Strategy {
	t.Helper()

	var s Strategy
	if err := yaml.Unmarshal([]byte(src), &s); err != nil {
		t.Fatal(err)
	}

	return &s
}

func TestOverflowBeforeExcludesIsStillReportedTruncated(t *testing.T) {
	t.Parallel()

	var a, b, ex strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&a, "    - a%d\n", i)
		fmt.Fprintf(&b, "    - b%d\n", i)
		if i < 17 {
			fmt.Fprintf(&ex, "    - alpha: a%d\n", i)
		}
	}

	s := strategyOf(t, "matrix:\n  alpha:\n"+a.String()+"  beta:\n"+b.String()+
		"  exclude:\n"+ex.String())

	combos, truncated, err := ExpandMatrix(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(combos) > MaxCombinations {
		t.Fatalf("test construction is wrong: %d combinations survive the excludes", len(combos))
	}
	if !truncated {
		t.Errorf("400 combinations were cut to %d and reported complete", len(combos))
	}
}

func TestOverflowKeepsEveryAxisKey(t *testing.T) {
	t.Parallel()

	var a, b strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&a, "    - a%d\n", i)
		fmt.Fprintf(&b, "    - b%d\n", i)
	}

	s := strategyOf(t, "matrix:\n  alpha:\n"+a.String()+"  beta:\n"+b.String()+
		"  gamma:\n    - g0\n    - g1\n")

	combos, truncated, err := ExpandMatrix(s)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("800 combinations must report truncated")
	}
	for _, c := range combos {
		for _, key := range []string{"alpha", "beta", "gamma"} {
			if _, ok := c[key]; !ok {
				t.Fatalf("combination %v is missing axis %q", c, key)
			}
		}
	}
}
