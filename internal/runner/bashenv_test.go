package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestBashEnvIsNeutralised(t *testing.T) {
	dir := t.TempDir()
	startup := dir + "/startup.sh"
	if err := writeFile(startup, "export GHOSTCI_PROBE=leaked\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASH_ENV", startup)

	res := Run(context.Background(), []config.Check{
		{Name: "probe", Command: "printf '%s' \"$GHOSTCI_PROBE\"", Shell: "bash -e"},
	}, Options{})

	if res[0].Output != "" {
		t.Errorf("BASH_ENV startup file ran: output = %q", res[0].Output)
	}
}

func TestDeclaredBashEnvIsKept(t *testing.T) {
	t.Parallel()

	opts := Options{}
	opts.applyDefaults()

	cmd := command(config.Check{
		Name: "x", Command: "true", Shell: "bash",
		Env: map[string]string{"BASH_ENV": "/tmp/mine"},
	}, opts, nil)

	last := ""
	for _, kv := range cmd.Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == "BASH_ENV" {
			last = v
		}
	}
	if last != "/tmp/mine" {
		t.Errorf("BASH_ENV = %q, want the declared value", last)
	}
}

func TestNeutraliseOnlyAppliesToBash(t *testing.T) {
	t.Parallel()

	if got := neutralise("sh -e", nil); got != nil {
		t.Errorf("neutralise for sh = %v, want nothing", got)
	}
	if got := neutralise("bash --noprofile --norc -eo pipefail", nil); len(got) != 1 {
		t.Errorf("neutralise for bash = %v, want BASH_ENV cleared", got)
	}
}
