package importer

import (
	"strings"
	"testing"
)

func TestGatedJobInterpolatesUnderThePassingEvent(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: ci
on: [push, pull_request]
jobs:
  only-pr:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - run: ./report --trigger=${{ github.event_name }}
`)

	if len(res.Checks) != 1 {
		t.Fatalf("checks = %+v, want one", res.Checks)
	}
	if got := res.Checks[0].Command; got != "./report --trigger=pull_request" {
		t.Errorf("command = %q, want the pull_request rendering", got)
	}
}

func TestEventDivergentCommandNeedsReview(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: ci
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: ./report --trigger=${{ github.event_name }}
`)

	if len(res.Checks) != 0 {
		t.Fatalf("checks = %+v, want none", res.Checks)
	}

	var reviewed bool
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview && strings.Contains(e.Reason, "differs by triggering event") {
			reviewed = true
		}
	}
	if !reviewed {
		t.Errorf("entries = %+v, want a needs-review for the divergent command", res.Entries)
	}
}

func TestCommandSeesStepLevelEnv(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make ${{ env.TARGET }}
        env:
          TARGET: release
`)

	if len(res.Checks) != 1 {
		t.Fatalf("checks = %+v, want one", res.Checks)
	}
	if got := res.Checks[0].Command; got != "make release" {
		t.Errorf("command = %q, want make release", got)
	}
}
