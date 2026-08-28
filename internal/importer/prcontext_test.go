package importer

import (
	"strings"
	"testing"
)

func TestAStepReadingAnUnresolvableVariableIsSurfaced(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: Changelog
on:
  pull_request:
jobs:
  check:
    runs-on: ubuntu-latest
    env:
      PR_BASE: ${{ github.base_ref }}
    steps:
      - run: git diff -U0 "origin/${PR_BASE}" HEAD -- CHANGELOG.md
`)

	if len(res.Checks) != 0 {
		t.Fatalf("imported %d checks; %q would run with the variable unset",
			len(res.Checks), res.Checks[0].Command)
	}
	if !strings.Contains(reasonsOf(res), "PR_BASE") {
		t.Errorf("the reason does not name the variable: %s", reasonsOf(res))
	}
}

func TestTheEventPayloadIsNotReadableLocally(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    env:
      N: ${{ github.event.number }}
    steps:
      - run: curl -sSfL https://api.github.com/repos/o/r/pulls/${N}
`)

	if len(res.Checks) != 0 {
		t.Fatalf("imported %d checks reading the event payload", len(res.Checks))
	}
}

func TestAWorkflowIsOnlyReadUnderEventsItTriggersOn(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on:
  pull_request:
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - run: echo "started by ${{ github.event_name }}"
`)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks, want 1: %s", len(res.Checks), reasonsOf(res))
	}
	if got := res.Checks[0].Command; !strings.Contains(got, "pull_request") {
		t.Errorf("command = %q, want the event this workflow actually starts on", got)
	}
}

func TestAPushIsKnownNotToCarryPullRequestFields(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  check:
    runs-on: ubuntu-latest
    if: github.event.action != 'closed'
    steps:
      - run: make test
`)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks, want 1: %s", len(res.Checks), reasonsOf(res))
	}
}

func TestThePayloadIsStillUnreadableWhereItMayExist(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [pull_request]
jobs:
  check:
    runs-on: ubuntu-latest
    if: github.event.action != 'closed'
    steps:
      - run: make test
`)
	if len(res.Checks) != 0 {
		t.Errorf("imported %d checks; the action is the server's on a pull request", len(res.Checks))
	}

	res = importYAML(t, `
name: t
on: [push]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ github.event.head_commit.message }}
`)
	if len(res.Checks) != 0 {
		t.Errorf("imported %d checks; the commit message was not read from anywhere", len(res.Checks))
	}
}
