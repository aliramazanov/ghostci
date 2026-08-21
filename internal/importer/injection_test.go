package importer

import (
	"strings"
	"testing"
)

func TestInjectionIsReported(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        tag: ["ok", "a; echo PWNED"]
    steps:
      - run: go test -tags ${{ matrix.tag }} ./...
`)

	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "PWNED") {
		t.Fatalf("an injecting value must be reported, warnings were: %v", res.Warnings)
	}
	if !strings.Contains(joined, "shell control characters") {
		t.Errorf("the warning should say why: %v", res.Warnings)
	}

	var found bool
	for _, c := range res.Checks {
		if strings.Contains(c.Command, "a; echo PWNED") {
			found = true
		}
	}
	if !found {
		t.Error("the command must match what CI runs, not an escaped variant")
	}
}

func TestOrdinaryValuesAreNotReported(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.25', '1.26']
    steps:
      - run: go test -tags ${{ matrix.go }} ./...
`)

	if len(res.Warnings) != 0 {
		t.Errorf("ordinary matrix values must not warn: %v", res.Warnings)
	}
}
