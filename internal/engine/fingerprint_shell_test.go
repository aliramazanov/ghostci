package engine

import (
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/runner"
)

func TestFingerprintRecordsTheResolvedShell(t *testing.T) {
	root := repo(t)

	eng := New(Options{Root: root, All: true})

	plan := eng.Plan([]config.Check{{
		Name: "shell-less", Command: "true", Inputs: []string{"a.go"},
	}})

	if len(plan.Decisions) != 1 || plan.Decisions[0].Fingerprint == nil {
		t.Fatalf("decisions = %+v, want one with a fingerprint", plan.Decisions)
	}

	if got := plan.Decisions[0].Fingerprint.Shell; got != runner.DefaultShell() {
		t.Errorf("fingerprint shell = %q, want the resolved %q", got, runner.DefaultShell())
	}
}
