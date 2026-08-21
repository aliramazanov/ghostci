package importer

import (
	"testing"
)

func TestCalledWorkflowsOwnPathFilterIsNotAdopted(t *testing.T) {
	t.Parallel()

	root := repoWith(t, map[string]string{
		".github/workflows/shared.yml": `
name: shared
on:
  workflow_call:
  push:
    paths: ['docs/**']
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`,
	})

	res := importInRepo(t, root, `
name: ci
on: [push]
jobs:
  call:
    uses: ./.github/workflows/shared.yml
`)

	if len(res.Checks) != 1 {
		t.Fatalf("checks = %+v, want one", res.Checks)
	}
	for _, in := range res.Checks[0].Inputs {
		if in == "docs/**" {
			t.Errorf("inputs = %v: the called workflow's own paths filter leaked in", res.Checks[0].Inputs)
		}
	}
}

func TestCallerStateSurvivesAReusableCall(t *testing.T) {
	t.Parallel()

	root := repoWith(t, map[string]string{
		".github/workflows/shared.yml": `
name: shared
on:
  workflow_call:
  push:
    paths: ['docs/**']
env:
  ORIGIN: called
jobs:
  inner:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
`,
	})

	res := importInRepo(t, root, `
name: ci
on: [push]
env:
  ORIGIN: caller
jobs:
  call:
    uses: ./.github/workflows/shared.yml
  after:
    runs-on: ubuntu-latest
    steps:
      - run: echo after
`)

	var after *int
	for i, c := range res.Checks {
		if c.Command == "echo after" {
			after = &i
		}
	}
	if after == nil {
		t.Fatalf("checks = %+v, want the caller's second job", res.Checks)
	}

	c := res.Checks[*after]
	if got := c.Env["ORIGIN"]; got != "caller" {
		t.Errorf("env ORIGIN = %q, want the caller's value", got)
	}
	for _, in := range c.Inputs {
		if in == "docs/**" {
			t.Errorf("inputs = %v: the called workflow's paths bled into a later caller job", c.Inputs)
		}
	}
}
