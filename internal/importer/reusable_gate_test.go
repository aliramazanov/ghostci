package importer

import (
	"strings"
	"testing"
)

const sharedWorkflow = `
name: shared
on: [workflow_call]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`

func TestReusableCallHonoursItsGate(t *testing.T) {
	t.Parallel()

	root := repoWith(t, map[string]string{
		".github/workflows/shared.yml": sharedWorkflow,
	})

	res := importInRepo(t, root, `
name: ci
on: [push]
jobs:
  release:
    if: startsWith(github.ref, 'refs/tags/')
    uses: ./.github/workflows/shared.yml
`)

	if len(res.Checks) != 0 {
		t.Fatalf("checks = %+v, want none: the gate is false on a branch", res.Checks)
	}

	var skipped bool
	for _, e := range res.Entries {
		if e.Outcome == SkippedByCondition {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("entries = %+v, want the call reported as skipped by condition", res.Entries)
	}
}

func TestReusableCallWithUndecidableGateNeedsReview(t *testing.T) {
	t.Parallel()

	root := repoWith(t, map[string]string{
		".github/workflows/shared.yml": sharedWorkflow,
	})

	res := importInRepo(t, root, `
name: ci
on: [push]
jobs:
  gated:
    if: needs.decide.outputs.go == 'yes'
    uses: ./.github/workflows/shared.yml
`)

	if len(res.Checks) != 0 {
		t.Fatalf("checks = %+v, want none", res.Checks)
	}

	var reviewed bool
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview {
			reviewed = true
		}
	}
	if !reviewed {
		t.Errorf("entries = %+v, want needs-review for the undecidable gate", res.Entries)
	}
}

func TestMatrixOverReusableCallNeedsReview(t *testing.T) {
	t.Parallel()

	root := repoWith(t, map[string]string{
		".github/workflows/shared.yml": `
name: shared
on:
  workflow_call:
    inputs:
      version:
        type: string
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test -tags v${{ inputs.version }} ./...
`,
	})

	res := importInRepo(t, root, `
name: ci
on: [push]
jobs:
  call:
    strategy:
      matrix:
        version: ["1", "2"]
    uses: ./.github/workflows/shared.yml
    with:
      version: ${{ matrix.version }}
`)

	if len(res.Checks) != 0 {
		t.Fatalf("checks = %+v, want none: one leg cannot stand in for the matrix", res.Checks)
	}

	var reviewed bool
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview && strings.Contains(e.Reason, "matrix over a reusable workflow") {
			reviewed = true
		}
	}
	if !reviewed {
		t.Errorf("entries = %+v, want needs-review for the matrix call", res.Entries)
	}
}
