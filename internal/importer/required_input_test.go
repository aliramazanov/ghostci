package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAction(t *testing.T, root, dir, body string) {
	t.Helper()

	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "action.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRequiredCompositeInputNeedsReview(t *testing.T) {
	root := t.TempDir()
	writeAction(t, root, ".github/actions/build", `
name: build
inputs:
  target:
    description: what to build
    required: true
runs:
  using: composite
  steps:
    - run: make ${{ inputs.target }}
      shell: bash
`)

	res := importInRepo(t, root, `
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/build
`)

	if len(res.Checks) != 0 {
		t.Fatalf("checks = %+v, want none: the input was never supplied", res.Checks)
	}

	var reviewed bool
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview && strings.Contains(e.Reason, "inputs.target") {
			reviewed = true
		}
	}
	if !reviewed {
		t.Errorf("entries = %+v, want one needing review for inputs.target", res.Entries)
	}
}

func TestSuppliedRequiredCompositeInputResolves(t *testing.T) {
	root := t.TempDir()
	writeAction(t, root, ".github/actions/build", `
name: build
inputs:
  target:
    description: what to build
    required: true
runs:
  using: composite
  steps:
    - run: make ${{ inputs.target }}
      shell: bash
`)

	res := importInRepo(t, root, `
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/build
        with:
          target: release
`)

	if len(res.Checks) != 1 {
		t.Fatalf("checks = %+v, want one", res.Checks)
	}
	if got := res.Checks[0].Command; got != "make release" {
		t.Errorf("command = %q, want make release", got)
	}
}
