package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestChecksRunWithGhostciDisabled(t *testing.T) {
	res := Run(context.Background(), []config.Check{
		{Name: "echo", Command: "printf '%s' \"$GHOSTCI\""},
	}, Options{})

	if res[0].Output != "0" {
		t.Errorf("GHOSTCI in the check environment = %q, want 0", res[0].Output)
	}
}

func TestDeclaredEnvCannotReEnable(t *testing.T) {
	opts := Options{}
	opts.applyDefaults()

	cmd := command(config.Check{
		Name: "x", Command: "true", Env: map[string]string{Disable: "1"},
	}, opts, nil)

	last := ""
	for _, kv := range cmd.Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == Disable {
			last = v
		}
	}
	if last != "0" {
		t.Errorf("effective %s = %q, want 0", Disable, last)
	}
}

func TestDisabled(t *testing.T) {
	for _, value := range []string{"0", "false"} {
		t.Setenv(Disable, value)
		if !Disabled() {
			t.Errorf("%s=%q should disable", Disable, value)
		}
	}

	for _, value := range []string{"", "1", "true", "yes"} {
		t.Setenv(Disable, value)
		if Disabled() {
			t.Errorf("%s=%q should not disable", Disable, value)
		}
	}
}
