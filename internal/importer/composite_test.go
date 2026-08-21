package importer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func repoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const callsLocal = `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/verify
`

func TestCompositeStepsAreExtracted(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
runs:
  using: composite
  steps:
    - name: Vet
      run: go vet ./...
      shell: bash
    - name: Test
      run: go test ./...
      shell: bash
`,
	})
	res := importInRepo(t, root, callsLocal)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want 2: %+v", len(res.Checks), res.Checks)
	}
	if res.Checks[0].Command != "go vet ./..." {
		t.Errorf("command = %q", res.Checks[0].Command)
	}
	if !strings.Contains(res.Checks[0].Name, "verify") {
		t.Errorf("name should name the action: %q", res.Checks[0].Name)
	}
	if len(res.Checks[0].Inputs) == 0 {
		t.Error("inference should still run on composite steps")
	}
}

func TestCompositeInputsResolveFromWith(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
inputs:
  pkg:
    default: ./...
  flags:
    default: ""
runs:
  using: composite
  steps:
    - run: go test ${{ inputs.flags }} ${{ inputs.pkg }}
      shell: bash
`,
	})
	res := importInRepo(t, root, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/verify
        with:
          flags: -race
`)
	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks: %+v", len(res.Checks), res.Entries)
	}
	if got := res.Checks[0].Command; got != "go test -race ./..." {
		t.Errorf("command = %q, want inputs resolved from with: plus defaults", got)
	}
}

func TestCompositeStepConditionsAreHonoured(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
inputs:
  coverage:
    default: "false"
runs:
  using: composite
  steps:
    - run: go test ./...
      shell: bash
    - if: inputs.coverage == 'true'
      run: go test -cover ./...
      shell: bash
    - if: runner.os == 'Windows'
      run: echo windows-only
      shell: bash
`,
	})
	res := importInRepo(t, root, callsLocal)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks, want 1 (two steps gated off): %+v", len(res.Checks), res.Checks)
	}
	if res.Count(SkippedByCondition) != 2 {
		t.Errorf("both false conditions should be reported as skipped, got %d", res.Count(SkippedByCondition))
	}
}

func TestCompositeCustomShellPlaceholderStripped(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
runs:
  using: composite
  steps:
    - run: echo hi
      shell: bash -e {0}
    - run: echo there
      shell: python
`,
	})
	res := importInRepo(t, root, callsLocal)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks", len(res.Checks))
	}
	for _, c := range res.Checks {
		if strings.Contains(c.Shell, "{0}") {
			t.Errorf("shell still carries the script placeholder: %q", c.Shell)
		}
	}
	if res.Checks[0].Shell != "bash -e" {
		t.Errorf("shell = %q, want %q", res.Checks[0].Shell, "bash -e")
	}
}

func TestCompositeOutputStepsAreRefused(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
runs:
  using: composite
  steps:
    - id: detect
      run: echo "value=1" >> $GITHUB_OUTPUT
      shell: bash
    - run: echo ${{ steps.detect.outputs.value }}
      shell: bash
`,
	})
	res := importInRepo(t, root, callsLocal)

	if res.Count(NeedsReview) != 1 {
		t.Fatalf("a step reading steps.* must be deferred, got %d: %+v", res.Count(NeedsReview), res.Entries)
	}
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview && !strings.Contains(e.Reason, "only known inside CI") {
			t.Errorf("reason should name the cause: %q", e.Reason)
		}
	}
}

func TestNonCompositeActionIsUnsupported(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
runs:
  using: node20
  main: index.js
`,
	})
	res := importInRepo(t, root, callsLocal)

	if len(res.Checks) != 0 {
		t.Fatal("a JavaScript action has no shell steps to lift")
	}
	if res.Count(UnsupportedAction) != 1 {
		t.Fatalf("want one unsupported entry, got %+v", res.Entries)
	}
	if !strings.Contains(res.Entries[0].Reason, "node20") {
		t.Errorf("reason should name the action type: %q", res.Entries[0].Reason)
	}
}

func TestMissingLocalActionIsReported(t *testing.T) {
	t.Parallel()
	res := importInRepo(t, t.TempDir(), callsLocal)
	if res.Count(LocalAction) != 1 {
		t.Fatalf("want one local-action entry, got %+v", res.Entries)
	}
	if !strings.Contains(res.Entries[0].Reason, "not found") {
		t.Errorf("reason = %q", res.Entries[0].Reason)
	}
}

func TestNestedCompositesExpand(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/outer/action.yml": `
name: Outer
runs:
  using: composite
  steps:
    - run: echo outer
      shell: bash
    - uses: ./.github/actions/inner
`,
		".github/actions/inner/action.yml": `
name: Inner
runs:
  using: composite
  steps:
    - run: echo inner
      shell: bash
`,
	})
	res := importInRepo(t, root, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/outer
`)
	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want outer and inner: %+v", len(res.Checks), res.Checks)
	}
}

func TestSelfReferencingCompositeTerminates(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/loop/action.yml": `
name: Loop
runs:
  using: composite
  steps:
    - run: echo before
      shell: bash
    - uses: ./.github/actions/loop
`,
	})
	done := make(chan *Result, 1)
	go func() {
		done <- importInRepo(t, root, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/loop
`)
	}()

	select {
	case res := <-done:
		if res.Count(LocalAction) == 0 {
			t.Errorf("the recursive call should be reported, got %+v", res.Entries)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("composite recursion did not terminate")
	}
}

func TestOneUnknownInputDoesNotSinkTheAction(t *testing.T) {
	t.Parallel()
	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
name: Verify
inputs:
  token:
    default: ""
  pkg:
    default: ./...
runs:
  using: composite
  steps:
    - run: go test ${{ inputs.pkg }}
      shell: bash
    - run: go vet ${{ inputs.pkg }}
      shell: bash
    - run: publish --token ${{ inputs.token }}
      shell: bash
`,
	})
	res := importInRepo(t, root, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/verify
        with:
          token: ${{ secrets.PUBLISH_TOKEN }}
`)
	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want the 2 that do not read the unknown input: %+v", len(res.Checks), res.Entries)
	}
	for _, c := range res.Checks {
		if strings.Contains(c.Command, "publish") {
			t.Error("the step reading the secret-derived input must not be extracted")
		}
		if !strings.Contains(c.Command, "./...") {
			t.Errorf("resolvable input was not substituted: %q", c.Command)
		}
	}
	if res.Count(NeedsReview) != 1 {
		t.Errorf("the step reading the unknown input should be deferred, got %d", res.Count(NeedsReview))
	}
}

func TestLocalActionsResolveFromAnyWorkflowLocation(t *testing.T) {
	t.Parallel()

	root := repoWith(t, map[string]string{
		".github/actions/verify/action.yml": `
runs:
  using: composite
  steps:
    - run: go test ./...
      shell: bash
`,
		"deep/nested/wf/ci.yml": `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/verify
`,
	})
	gitInit(t, root)

	res, err := ImportDir(filepath.Join(root, "deep", "nested", "wf"), DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Checks) != 1 {
		t.Fatalf("composite not resolved from a nested workflow dir: %+v", res.Entries)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}
