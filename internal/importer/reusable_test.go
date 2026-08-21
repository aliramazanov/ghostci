package importer

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repoWithGit(t *testing.T, files map[string]string) string {
	t.Helper()

	root := repoWith(t, files)

	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	return root
}

func TestLocalReusableWorkflowIsExpanded(t *testing.T) {
	t.Parallel()

	root := repoWithGit(t, map[string]string{
		".github/workflows/ci.yml": `
on: [push]
jobs:
  call:
    uses: ./.github/workflows/shared.yml
`,
		".github/workflows/shared.yml": `
on:
  workflow_call:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`,
	})

	res, err := ImportDir(filepath.Join(root, ".github", "workflows"), DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Checks) == 0 {
		t.Fatalf("reusable workflow not expanded: %+v", res.Entries)
	}
	if res.Checks[0].Command != "go test ./..." {
		t.Errorf("command = %q", res.Checks[0].Command)
	}
}

func TestReusableWorkflowInputsResolveFromWith(t *testing.T) {
	t.Parallel()

	root := repoWithGit(t, map[string]string{
		".github/workflows/ci.yml": `
on: [push]
jobs:
  call:
    uses: ./.github/workflows/shared.yml
    with:
      flags: -race
`,
		".github/workflows/shared.yml": `
on:
  workflow_call:
    inputs:
      flags:
        default: ""
      pkg:
        default: "./..."
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ${{ inputs.flags }} ${{ inputs.pkg }}
`,
	})

	res, err := ImportDir(filepath.Join(root, ".github", "workflows"), DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks: %+v", len(res.Checks), res.Entries)
	}
	if got := res.Checks[0].Command; got != "go test -race ./..." {
		t.Errorf("command = %q, want inputs from with: plus defaults", got)
	}
}

func TestRemoteReusableWorkflowIsRefused(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  call:
    uses: owner/repo/.github/workflows/x.yml@v1
`)

	if len(res.Checks) != 0 {
		t.Fatal("a remote workflow must not be fetched")
	}
	if res.Count(UnsupportedFeature) != 1 || !strings.Contains(res.Entries[0].Reason, "not fetched") {
		t.Errorf("want a clear refusal, got %+v", res.Entries)
	}
}

func TestSelfReferencingWorkflowTerminates(t *testing.T) {
	t.Parallel()

	root := repoWithGit(t, map[string]string{
		".github/workflows/loop.yml": `
on:
  push:
  workflow_call:
jobs:
  call:
    uses: ./.github/workflows/loop.yml
`,
	})

	done := make(chan bool, 1)
	go func() {
		_, err := ImportDir(filepath.Join(root, ".github", "workflows"), DefaultAssumptions())
		done <- err == nil
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("recursive reusable workflow did not terminate")
	}
}
