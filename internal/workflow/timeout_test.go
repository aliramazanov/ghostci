package workflow

import (
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func scalar(v string) yaml.Node { return yaml.Node{Kind: yaml.ScalarNode, Value: v} }

func TestTimeoutNeverGoesNegative(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"1e300", "Inf", "+Inf", "99999999999", "1e19"} {
		if got := minutes(scalar(raw)); got < 0 {
			t.Errorf("timeout-minutes %q = %v, want a positive duration", raw, got)
		} else if got != maxTimeout {
			t.Errorf("timeout-minutes %q = %v, want it clamped to %v", raw, got, maxTimeout)
		}
	}
}

func TestTimeoutOrdinaryValues(t *testing.T) {
	t.Parallel()

	tests := map[string]time.Duration{
		"5":    5 * time.Minute,
		"0.5":  30 * time.Second,
		"360":  6 * time.Hour,
		"0":    0,
		"-1":   0,
		"NaN":  0,
		"":     0,
		"soon": 0,
	}

	for raw, want := range tests {
		if got := minutes(scalar(raw)); got != want {
			t.Errorf("timeout-minutes %q = %v, want %v", raw, got, want)
		}
	}
}
