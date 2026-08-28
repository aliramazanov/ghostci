package importer

import (
	"strings"
	"testing"
)

func TestIdenticalMatrixLegsCollapseToOne(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: CI
on: [push]
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        python: ['3.11', '3.12', '3.13']
    steps:
      - run: make ci
`)

	if len(res.Checks) != 1 {
		names := make([]string, 0, len(res.Checks))
		for _, c := range res.Checks {
			names = append(names, c.Name)
		}
		t.Fatalf("got %d checks for one command across nine legs: %v", len(res.Checks), names)
	}
	if !strings.Contains(strings.Join(res.Warnings, " "), "matrix legs") {
		t.Errorf("the collapse was not reported: %v", res.Warnings)
	}
}

func TestMatrixLegsThatDifferAreKept(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: CI
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        suite: [unit, integration]
    steps:
      - run: make test
        env:
          SUITE: ${{ matrix.suite }}
`)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want both suites kept", len(res.Checks))
	}
}

func TestGitCommandsThatMoveTheWorkingTreeAreHeldBack(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		"git checkout HEAD^2",
		"git reset --hard origin/main",
		"git clean -fdx",
		"git stash",
		"git rebase main",
	} {
		if cost, why := classifyCost("analyze", command); cost != CostHeavy || why == "" {
			t.Errorf("%q was classified runnable, want held back", command)
		}
	}
}
