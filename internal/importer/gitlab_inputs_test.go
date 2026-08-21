package importer

import (
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func onlyCheck(t *testing.T, res *Result) config.Check {
	t.Helper()

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1: %+v", len(res.Checks), res.Checks)
	}

	return res.Checks[0]
}

func TestSpecInputDefaultsAreSubstituted(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
spec:
  inputs:
    stage:
      default: test
    suite:
      default: unit
---
run:
  stage: $[[ inputs.stage ]]
  variables:
    SUITE: $[[ inputs.suite ]]
  script:
    - make $[[ inputs.suite ]]
`)

	chk := onlyCheck(t, res)

	if strings.Contains(chk.Command, "$[[") {
		t.Errorf("command kept the placeholder: %q", chk.Command)
	}
	if chk.Command != "make unit" {
		t.Errorf("command = %q, want the declared default substituted", chk.Command)
	}
	if got := chk.Env["SUITE"]; got != "unit" {
		t.Errorf("SUITE = %q, want unit", got)
	}
}

func TestInputWithoutDefaultSurfacesTheJob(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
spec:
  inputs:
    target:
      description: what to build
---
build:
  script:
    - make $[[ inputs.target ]]
`)

	if len(res.Checks) != 0 {
		t.Fatalf("imported %d checks reading an input with no default: %+v", len(res.Checks), res.Checks)
	}

	entry := res.Entries[0]
	if entry.Outcome != NeedsReview {
		t.Fatalf("outcome = %v, want NeedsReview", entry.Outcome)
	}
	if !strings.Contains(entry.Reason, "target") {
		t.Errorf("reason does not name the input: %q", entry.Reason)
	}
}

func TestUnsetInputNeverReachesCheckEnv(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
spec:
  inputs:
    release:
      description: set when releasing
---
variables:
  RELEASE: $[[ inputs.release ]]
  KEEP: yes
test:
  script:
    - make test
`)

	chk := onlyCheck(t, res)

	for name, value := range chk.Env {
		if strings.Contains(value, "ghostci-unset-input") {
			t.Errorf("env %s = %q leaks the marker into the check", name, value)
		}
	}
	if _, set := chk.Env["RELEASE"]; set {
		t.Errorf("RELEASE was set to %q, want left unset", chk.Env["RELEASE"])
	}
	if chk.Env["KEEP"] != "yes" {
		t.Errorf("KEEP = %q, want the unaffected variable kept", chk.Env["KEEP"])
	}
}
