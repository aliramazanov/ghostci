package importer

import (
	"testing"
	"time"
)

func outcomes(t *testing.T, y string) map[Outcome]int {
	t.Helper()

	res := importYAML(t, y)
	got := map[Outcome]int{}
	for _, e := range res.Entries {
		got[e.Outcome]++
	}

	return got
}

func TestExtractsPlainShellSteps(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(newJob("ci",
		uses("actions/checkout@v4"),
		run("go vet ./..."),
		run("go test ./..."),
	)))

	if len(res.Checks) != 2 {
		t.Fatalf("want 2 checks, got %d: %+v", len(res.Checks), res.Checks)
	}
	if res.Count(DroppedAction) != 1 {
		t.Errorf("want checkout dropped, got %d", res.Count(DroppedAction))
	}
}

func TestClassifiesEveryStepShape(t *testing.T) {
	t.Parallel()

	got := outcomes(t, synthWorkflow(newJob("ci",
		uses("actions/checkout@v4"),
		usesWith("actions/setup-go@v5", map[string]string{"go-version": "'1.26'"}),
		uses("some/marketplace@v1"),
		uses("./.github/actions/missing"),
		run("go test ./..."),
		runIf("false", "never"),
		runIf("needs.build.outputs.ok == 'true'", "gated"),
	)))

	want := map[Outcome]int{
		DroppedAction:      1,
		ToolchainAction:    1,
		UnsupportedAction:  1,
		LocalAction:        1,
		Extracted:          1,
		SkippedByCondition: 1,
		NeedsReview:        1,
	}
	for o, n := range want {
		if got[o] != n {
			t.Errorf("%s: got %d, want %d", o, got[o], n)
		}
	}
}

func TestMatrixProducesOneCheckPerLeg(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(
		newJob("ci", run("go test -tags ${{ matrix.go }} ./...")).
			over("go", "'1.25'", "'1.26'"),
	))

	if len(res.Checks) != 2 {
		t.Fatalf("want one check per matrix leg, got %d", len(res.Checks))
	}
	if res.Checks[0].Name == res.Checks[1].Name {
		t.Errorf("matrix legs share a name: %q", res.Checks[0].Name)
	}
}

func TestSkipsJobsTargetingAnotherPlatform(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(
		newJob("win", run("go test ./...")).on("windows-latest"),
	))

	if len(res.Checks) != 0 {
		t.Fatal("a windows job must not be extracted on linux")
	}
	if res.Count(UnsupportedFeature) != 1 {
		t.Errorf("want one unsupported entry with a reason")
	}
}

func TestPullRequestGatedStepsStillExtract(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(newJob("ci",
		runIf("github.event_name == 'pull_request'", "go test ./..."),
	)))

	if len(res.Checks) != 1 {
		t.Fatal("dropping a pull_request-gated step is a false all-clear")
	}
}

func TestTagGatedStepsExtractOnABranch(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(newJob("ci",
		runIf("!startsWith(github.ref, 'refs/tags/')", "go test ./..."),
		runIf("startsWith(github.ref, 'refs/tags/')", "goreleaser release"),
	)))

	if len(res.Checks) != 1 {
		t.Fatalf("want the non-tag step only, got %d", len(res.Checks))
	}
	if res.Checks[0].Command != "go test ./..." {
		t.Errorf("wrong step extracted: %q", res.Checks[0].Command)
	}
}

func TestUndecidableStepsAreDeferredWithAReason(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(newJob("ci",
		runIf("needs.setup.outputs.go == 'true'", "go test ./..."),
		run("echo ${{ steps.prev.outputs.value }}"),
		run("deploy --token ${{ secrets.TOKEN }}"),
	)))

	if len(res.Checks) != 0 {
		t.Fatal("undecidable steps must never be extracted")
	}
	if res.Count(NeedsReview) != 3 {
		t.Fatalf("want 3 deferred, got %d: %+v", res.Count(NeedsReview), res.Entries)
	}
	for _, e := range res.Entries {
		if e.Reason == "" {
			t.Error("a deferred step must say why")
		}
	}
}

func TestEveryStepProducesExactlyOneEntry(t *testing.T) {
	t.Parallel()

	steps := []step{
		uses("actions/checkout@v4"),
		uses("some/marketplace@v1"),
		run("go test ./..."),
		runIf("false", "never"),
	}
	res := importYAML(t, synthWorkflow(newJob("ci", steps...)))

	if len(res.Entries) != len(steps) {
		t.Fatalf("got %d entries for %d steps: nothing may be dropped silently",
			len(res.Entries), len(steps))
	}
}

func TestPreservesMultilineScripts(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(newJob("ci",
		run("set -e\ngo build ./...\ngo test ./..."),
	)))

	if len(res.Checks) != 1 {
		t.Fatalf("a multiline script is one check, got %d", len(res.Checks))
	}
	if got := res.Checks[0].Command; got != "set -e\ngo build ./...\ngo test ./..." {
		t.Errorf("script altered: %q", got)
	}
}

func TestCarriesWorkingDirectoryAndShell(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(newJob("ci",
		run("pytest").in("./backend").via("pwsh"),
	)))

	c := res.Checks[0]
	if c.Dir != "./backend" || c.Shell != "pwsh" {
		t.Errorf("dir=%q shell=%q", c.Dir, c.Shell)
	}
}

func TestInfersInputsPerEcosystem(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"go test ./...":  "**/*.go",
		"npx eslint src": "**/*.ts",
		"cargo clippy":   "**/*.rs",
		"pytest -q":      "**/*.py",
		"./unknown.sh":   "",
	}
	for cmd, want := range tests {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()

			got := inferInputs(cmd)
			if want == "" {
				if got != nil {
					t.Errorf("an unrecognised command must infer nothing, got %v", got)
				}

				return
			}
			if !contains(got, want) {
				t.Errorf("inferInputs(%q) = %v, want it to include %q", cmd, got, want)
			}
		})
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}

	return false
}

func TestPathFiltersBecomeInputs(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on:
  push:
    paths: ['docs/**', '*.md']
    paths-ignore: ['docs/drafts/**']
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: ./build-docs.sh
`)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks", len(res.Checks))
	}

	c := res.Checks[0]
	if len(c.Inputs) != 2 || c.Inputs[0] != "docs/**" {
		t.Errorf("inputs = %v, want the declared paths", c.Inputs)
	}
	if len(c.Exclude) != 1 || c.Exclude[0] != "docs/drafts/**" {
		t.Errorf("exclude = %v, want paths-ignore", c.Exclude)
	}
}

func TestInferenceRemainsTheFallback(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`)

	if len(res.Checks[0].Inputs) == 0 || res.Checks[0].Inputs[0] != "**/*.go" {
		t.Errorf("inputs = %v, want inferred Go globs", res.Checks[0].Inputs)
	}
}

func TestEnvLayersFromWorkflowToStep(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
env:
  FROM_WORKFLOW: wf
  OVERRIDDEN: wf
jobs:
  ci:
    runs-on: ubuntu-latest
    env:
      FROM_JOB: job
      OVERRIDDEN: job
    steps:
      - run: echo hi
        env:
          FROM_STEP: step
`)

	env := res.Checks[0].Env
	for k, want := range map[string]string{
		"FROM_WORKFLOW": "wf",
		"FROM_JOB":      "job",
		"FROM_STEP":     "step",
		"OVERRIDDEN":    "job",
	} {
		if env[k] != want {
			t.Errorf("%s = %q, want %q", k, env[k], want)
		}
	}
}

func TestContinueOnErrorIsOptional(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: flaky
        continue-on-error: true
      - run: go test ./...
`)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks", len(res.Checks))
	}
	if !res.Checks[0].Optional {
		t.Error("continue-on-error must mark the check optional")
	}
	if res.Checks[1].Optional {
		t.Error("an ordinary step must not be optional")
	}
}

func TestTimeoutMinutesIsCarried(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - run: slow
        timeout-minutes: 2
      - run: uses-job-limit
`)

	if got := time.Duration(res.Checks[0].Timeout); got != 2*time.Minute {
		t.Errorf("step timeout = %v, want 2m", got)
	}
	if got := time.Duration(res.Checks[1].Timeout); got != 5*time.Minute {
		t.Errorf("job timeout should apply when the step has none, got %v", got)
	}
}

func TestEnvExpressionsResolve(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.26']
    env:
      GOVERSION: ${{ matrix.go }}
      UNKNOWABLE: ${{ secrets.TOKEN }}
    steps:
      - run: go test ./...
`)

	env := res.Checks[0].Env
	if env["GOVERSION"] != "1.26" {
		t.Errorf("a resolvable env value must be kept, got %v", env)
	}
	if _, ok := env["UNKNOWABLE"]; ok {
		t.Error("a value only CI can resolve must be dropped, not guessed")
	}
}
