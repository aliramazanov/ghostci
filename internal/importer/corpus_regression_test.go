package importer

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailFastExpressionDoesNotPoisonTheWorkflow(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      fail-fast: ${{ github.event_name == 'merge_group' }}
      matrix:
        v: [1, 2]
    steps:
      - run: echo test ${{ matrix.v }}
`)
	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want one per matrix leg: %+v", len(res.Checks), res.Checks)
	}
}

func TestNestedCheckoutResolvesItsOwnReusableWorkflows(t *testing.T) {
	t.Parallel()
	outer := t.TempDir()
	run := exec.Command("git", "init", "-q", outer)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	wfDir := filepath.Join(outer, "vendor", "proj", ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(wfDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ci.yml", `
on: [push]
jobs:
  build:
    uses: ./.github/workflows/build.yml
`)
	write("build.yml", `
on: [workflow_call]
jobs:
  compile:
    runs-on: ubuntu-latest
    steps:
      - run: echo compile
`)

	res, err := ImportDir(wfDir, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Checks) != 1 || !strings.Contains(res.Checks[0].Command, "echo compile") {
		var buf bytes.Buffer
		res.Ledger(&buf, DefaultAssumptions())
		t.Fatalf("the reusable workflow was not followed:\n%s", buf.String())
	}
}

func TestTagOnlyPushWorkflowIsNotImportedForABranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	release := `
on:
  push:
    tags: ['v*']
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - run: ./publish.sh
`
	both := `
on:
  push:
    tags: ['v*']
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`
	for name, body := range map[string]string{"release.yml": release, "ci.yml": both} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := ImportDir(dir, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	for _, chk := range res.Checks {
		if strings.Contains(chk.Command, "publish.sh") {
			t.Fatalf("a tag-only release step was imported for a branch push: %+v", chk)
		}
	}
	if len(res.Checks) != 1 || !strings.Contains(res.Checks[0].Command, "go test") {
		t.Fatalf("the pull_request workflow should still import: %+v", res.Checks)
	}

	var buf bytes.Buffer
	res.Ledger(&buf, DefaultAssumptions())
	if !strings.Contains(buf.String(), "release.yml") {
		t.Fatalf("the excluded release workflow is invisible in the ledger:\n%s", buf.String())
	}
}

func TestWorkflowsWithoutPushTriggersAreNamedNotDropped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `
on:
  workflow_call:
    inputs:
      plan:
        type: string
jobs:
  bench:
    runs-on: ubuntu-latest
    steps:
      - run: cargo bench
`
	if err := os.WriteFile(filepath.Join(dir, "bench.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ImportDir(dir, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	res.Ledger(&buf, DefaultAssumptions())
	if !strings.Contains(buf.String(), "bench.yml") || !strings.Contains(buf.String(), "workflow_call") {
		t.Fatalf("the skipped workflow is invisible in the ledger:\n%s", buf.String())
	}
}
