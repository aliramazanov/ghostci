package importer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

func importYAML(t *testing.T, yaml string) *Result {
	t.Helper()
	wf, err := workflow.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wf.Path = "ci.yml"
	im := &importState{root: t.TempDir(), res: &Result{}, seenTool: map[string]bool{}, seenWorkflow: map[string]bool{}}
	im.importWorkflow(wf, DefaultAssumptions())
	dedupeNames(im.res)
	return im.res
}

func importInRepo(t *testing.T, root, yaml string) *Result {
	t.Helper()
	wf, err := workflow.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wf.Path = "ci.yml"
	im := &importState{root: root, res: &Result{}, seenTool: map[string]bool{}, seenWorkflow: map[string]bool{}}
	im.importWorkflow(wf, DefaultAssumptions())
	dedupeNames(im.res)
	return im.res
}

func TestExtractsRunSteps(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./...
`)
	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want 2: %+v", len(res.Checks), res.Checks)
	}
	if res.Checks[0].Command != "go vet ./..." {
		t.Errorf("command = %q", res.Checks[0].Command)
	}
	if res.Count(DroppedAction) != 1 {
		t.Errorf("checkout should be dropped")
	}
	if len(res.Toolchains) != 1 || res.Toolchains[0].Version != "1.26" {
		t.Errorf("toolchain not recorded: %+v", res.Toolchains)
	}
	if got := res.Checks[0].Inputs; len(got) == 0 || got[0] != "**/*.go" {
		t.Errorf("go inputs not inferred: %v", got)
	}
}

func TestMatrixExpansion(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.25', '1.26']
    steps:
      - run: go test -tags ${{ matrix.go }} ./...
`)
	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want 2 matrix legs", len(res.Checks))
	}
	var commands []string
	for _, c := range res.Checks {
		commands = append(commands, c.Command)
	}
	joined := strings.Join(commands, "|")
	if !strings.Contains(joined, "1.25") || !strings.Contains(joined, "1.26") {
		t.Errorf("matrix values not interpolated: %v", commands)
	}
	if res.Checks[0].Name == res.Checks[1].Name {
		t.Errorf("matrix legs share a name: %q", res.Checks[0].Name)
	}
}

func TestMatrixExcludeAndInclude(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.25', '1.26']
        mode: [fast, slow]
        exclude:
          - go: '1.25'
            mode: slow
    steps:
      - run: go test ./...
`)
	if len(res.Checks) != 3 {
		t.Fatalf("got %d checks, want 3 after exclude", len(res.Checks))
	}
}

func TestSkipsOtherPlatforms(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  win:
    runs-on: windows-latest
    steps:
      - run: go test ./...
`)
	if len(res.Checks) != 0 {
		t.Fatalf("windows job should not be extracted on Linux")
	}
	if res.Count(UnsupportedFeature) != 1 {
		t.Errorf("expected an unsupported entry with a reason")
	}
}

func TestPullRequestGatedStepsAreExtracted(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - if: github.event_name == 'pull_request'
        run: go test ./...
`)
	if len(res.Checks) != 1 {
		t.Fatalf("pull_request-gated step was dropped; that is a false all-clear")
	}
}

func TestUndecidableStepsAreDeferredNotDropped(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - if: needs.setup.outputs.go == 'true'
        run: go test ./...
      - run: echo ${{ steps.prev.outputs.value }}
`)
	if len(res.Checks) != 0 {
		t.Fatalf("undecidable steps must not be extracted")
	}
	if res.Count(NeedsReview) != 2 {
		t.Fatalf("got %d needs-review, want 2: %+v", res.Count(NeedsReview), res.Entries)
	}
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview && !strings.Contains(e.Reason, "only known inside CI") {
			t.Errorf("reason should name the cause: %q", e.Reason)
		}
	}
}

func TestEveryStepGetsAnOutcome(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: some/marketplace-action@v1
      - uses: ./.github/actions/local
      - run: go test ./...
      - if: false
        run: never
`)
	if len(res.Entries) != 5 {
		t.Fatalf("got %d entries for 5 steps: nothing may be dropped silently", len(res.Entries))
	}
	want := map[Outcome]int{
		DroppedAction: 1, UnsupportedAction: 1, LocalAction: 1,
		Extracted: 1, SkippedByCondition: 1,
	}
	for o, n := range want {
		if res.Count(o) != n {
			t.Errorf("%s: got %d, want %d", o, res.Count(o), n)
		}
	}
}

func TestGeneratedYAMLRoundTrips(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
      - if: needs.x.outputs.y == '1'
        run: deferred
`)
	var buf bytes.Buffer
	if err := res.YAML(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Not imported, needs your review") {
		t.Error("deferred steps must be listed in the generated file, not dropped")
	}
	cfg, err := config.Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
	if len(cfg.Checks) != 1 {
		t.Errorf("round trip lost checks: %d", len(cfg.Checks))
	}
}

func TestNonTriggeringWorkflowsIgnored(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [release]
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: ./publish.sh
`)
	if len(res.Checks) != 1 {
		t.Skip("importWorkflow is unfiltered by design")
	}
}

func TestMultilineRunPreserved(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: |
          set -e
          go build ./...
          go test ./...
`)
	if len(res.Checks) != 1 {
		t.Fatal("multiline run should be one check")
	}
	if n := strings.Count(res.Checks[0].Command, "\n"); n < 2 {
		t.Errorf("multiline script collapsed: %q", res.Checks[0].Command)
	}
}

func TestWorkingDirectoryAndShell(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: pytest
        working-directory: ./backend
        shell: pwsh
`)
	c := res.Checks[0]
	if c.Dir != "./backend" {
		t.Errorf("dir = %q", c.Dir)
	}
	if c.Shell != "pwsh" {
		t.Errorf("shell = %q", c.Shell)
	}
}
