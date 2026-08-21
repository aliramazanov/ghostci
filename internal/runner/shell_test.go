package runner

import (
	"context"
	"slices"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestEarlyFailureInAMultiLineScriptFails(t *testing.T) {
	res := Run(context.Background(), []config.Check{
		{Name: "two lines", Command: "false\ntrue"},
	}, Options{})

	if res[0].Status != StatusFailed {
		t.Fatalf("status = %v, want FAIL", res[0].Status)
	}
}

func TestShellCarryingFlagsRuns(t *testing.T) {
	res := Run(context.Background(), []config.Check{
		{Name: "composite", Command: "echo hi", Shell: "bash --noprofile --norc -eo pipefail"},
	}, Options{})

	if res[0].Status != StatusPassed {
		t.Fatalf("status = %v, err = %v", res[0].Status, res[0].Err)
	}
}

func TestInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shell    string
		wantName string
		wantArgs []string
	}{
		{"sh -e", "sh", []string{"-e", "-c", "s"}},
		{"bash --noprofile --norc -eo pipefail", "bash", []string{"--noprofile", "--norc", "-eo", "pipefail", "-c", "s"}},
		{"pwsh", "pwsh", []string{"-Command", "s"}},
		{"python", "python", []string{"-c", "s"}},
		{"zsh -c", "zsh", []string{"-c", "s"}},
	}

	for _, tc := range tests {
		name, args := invocation(tc.shell, "s")
		if name != tc.wantName || !slices.Equal(args, tc.wantArgs) {
			t.Errorf("invocation(%q) = %q %q, want %q %q", tc.shell, name, args, tc.wantName, tc.wantArgs)
		}
	}
}

func TestDefaultShellAborts(t *testing.T) {
	t.Parallel()

	name, args := invocation("", "s")
	if name != "bash" && name != "sh" {
		t.Fatalf("default shell = %q", name)
	}
	if !slices.Contains(args, "-e") {
		t.Errorf("default shell args %q carry no -e", args)
	}
}
