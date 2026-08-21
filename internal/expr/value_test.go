package expr

import (
	"math"
	"testing"
)

func TestNumberCoercionFollowsJavaScript(t *testing.T) {
	t.Parallel()

	tests := map[string]float64{
		"":          0,
		"  12  ":    12,
		"1e3":       1000,
		"0x1f":      31,
		"0X1F":      31,
		"0o17":      15,
		"0b101":     5,
		"-4.5":      -4.5,
		"Infinity":  math.Inf(1),
		"-Infinity": math.Inf(-1),
	}

	for in, want := range tests {
		if got := toNumber(in); got != want {
			t.Errorf("toNumber(%q) = %v, want %v", in, got, want)
		}
	}

	for _, in := range []string{"inf", "nan", "NaN", "INFINITY", "1_000", "0x1p4", "0x", "0o8", "-0x10", "abc"} {
		if got := toNumber(in); !math.IsNaN(got) {
			t.Errorf("toNumber(%q) = %v, want NaN", in, got)
		}
	}
}

func TestHexStringComparesEqualToItsValue(t *testing.T) {
	t.Parallel()

	if !looseEqual("0x1f", 31.0) {
		t.Error(`"0x1f" == 31 should hold, as it does on GitHub`)
	}
}
