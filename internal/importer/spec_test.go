package importer

import (
	"strings"
	"testing"
)

func TestPullRequestRefIsTheMergeRef(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: only-on-pr-merge-ref
        if: startsWith(github.ref, 'refs/pull/')
        run: echo pr-only
      - name: not-on-main
        if: github.ref != 'refs/heads/main'
        run: echo not-main
`)

	var got []string
	for _, c := range res.Checks {
		got = append(got, c.Command)
	}
	joined := strings.Join(got, "|")

	if !strings.Contains(joined, "pr-only") {
		t.Errorf("a step gated on the pull request merge ref was dropped: %v", got)
	}
	if !strings.Contains(joined, "not-main") {
		t.Errorf("a step that runs on every pull request was dropped: %v", got)
	}
}

func TestAGateFalseOnPushButTrueOnPullRequestStillImports(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - if: github.ref_name != 'main'
        run: echo runs-on-pr
`)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks, want the pull request path to import it: %+v", len(res.Checks), res.Checks)
	}
}

func TestEnvIsNotReadableFromAJobLevelGate(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
env:
  DEPLOY: "yes"
jobs:
  build:
    runs-on: ubuntu-latest
    if: env.DEPLOY == 'yes'
    steps:
      - run: echo built
`)

	if len(res.Checks) != 0 {
		t.Fatalf("a job gate reading env was resolved instead of surfaced: %+v", res.Checks)
	}
	if res.Count(NeedsReview) == 0 {
		t.Fatalf("the job gate was not surfaced for review: %+v", res.Entries)
	}

	step := importYAML(t, `
on: [push]
env:
  DEPLOY: "yes"
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - if: env.DEPLOY == 'yes'
        run: echo built
`)
	if len(step.Checks) != 1 {
		t.Fatalf("a step-level env gate should resolve: %+v", step.Entries)
	}
}

func TestAJobThatInstallsAToolIsImportedAsOneCheck(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Install
        run: go install github.com/kyoh86/richgo@latest
      - name: Test
        run: richgo test ./...
      - name: Vet
        run: go vet ./...
`)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks, want the job merged into one: %+v", len(res.Checks), res.Checks)
	}

	got := res.Checks[0]
	for _, want := range []string{"go install", "richgo test", "go vet"} {
		if !strings.Contains(got.Command, want) {
			t.Errorf("the merged command lost %q:\n%s", want, got.Command)
		}
	}
	if !strings.HasPrefix(got.Command, "go install") {
		t.Errorf("the install must come first:\n%s", got.Command)
	}

	if !got.Serial {
		t.Error("a check that installs shared tooling must not run beside another")
	}
}

func TestAnOrdinaryJobKeepsItsStepsSeparate(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: go vet ./...
      - run: go test ./...
`)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want one per step: %+v", len(res.Checks), res.Checks)
	}
	for _, c := range res.Checks {
		if c.Serial {
			t.Errorf("%s was needlessly serialised", c.Name)
		}
	}
}

func TestGitHubPropertiesAreModelledOrUndecidable(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  known:
    runs-on: ubuntu-latest
    if: github.repository_owner == 'local'
    steps:
      - run: echo owner-matches
  other:
    runs-on: ubuntu-latest
    if: github.repository_owner == 'someone-else'
    steps:
      - run: echo owner-differs
  unknowable:
    runs-on: ubuntu-latest
    if: github.run_number > 5
    steps:
      - run: echo needs-the-server
`)

	commands := func() string {
		var out []string
		for _, c := range res.Checks {
			out = append(out, c.Command)
		}
		return strings.Join(out, "|")
	}

	if !strings.Contains(commands(), "owner-matches") {
		t.Errorf("a gate on the real owner did not resolve: %s", commands())
	}
	if strings.Contains(commands(), "owner-differs") {
		t.Errorf("a gate on a different owner was imported anyway: %s", commands())
	}

	if strings.Contains(commands(), "needs-the-server") {
		t.Errorf("a gate on run_number was answered locally: %s", commands())
	}

	var surfaced bool
	for _, e := range res.Entries {
		if e.Outcome == NeedsReview && strings.Contains(e.Reason, "run_number") {
			surfaced = true
		}
	}
	if !surfaced {
		t.Errorf("the undecidable gate was not surfaced: %+v", res.Entries)
	}
}

func TestAJobThatInstallsDependenciesIsImportedAsOneCheck(t *testing.T) {
	t.Parallel()

	for name, install := range map[string]string{
		"npm ci":           "npm ci",
		"npm install":      "npm install --prefix web",
		"yarn":             "yarn install --frozen-lockfile",
		"pnpm":             "pnpm install",
		"pip requirements": "pip install -r requirements.txt",
		"poetry":           "poetry install",
		"bundler":          "bundle install",
		"go modules":       "go mod download",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			res := importYAML(t, `
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Install
        run: `+install+`
      - name: Lint
        run: make lint
      - name: Test
        run: make test
`)

			if len(res.Checks) != 1 {
				t.Fatalf("got %d checks, want the job merged: %+v", len(res.Checks), res.Checks)
			}
			if !res.Checks[0].Serial {
				t.Error("a check that installs into a shared place must not run beside another")
			}
			for _, want := range []string{install, "make lint", "make test"} {
				if !strings.Contains(res.Checks[0].Command, want) {
					t.Errorf("the merged command lost %q:\n%s", want, res.Checks[0].Command)
				}
			}
		})
	}
}

func TestAJobThatExportsStateIsImportedAsOneCheck(t *testing.T) {
	t.Parallel()

	for name, exporting := range map[string]string{
		"GITHUB_ENV":  `echo "BUILD_TAG=nightly" >> $GITHUB_ENV`,
		"GITHUB_PATH": `echo "$PWD/bin" >> $GITHUB_PATH`,
		"braced":      `echo "X=1" >> ${GITHUB_ENV}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			res := importYAML(t, `
on: [push]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - name: Export
        run: `+exporting+`
      - name: Consume
        run: test "$BUILD_TAG" = nightly
`)

			if len(res.Checks) != 1 {
				t.Fatalf("got %d checks, want the job merged: %+v", len(res.Checks), res.Checks)
			}
			if !strings.Contains(res.Checks[0].Command, "Consume") &&
				!strings.Contains(res.Checks[0].Command, "BUILD_TAG") {
				t.Errorf("the consuming step was lost:\n%s", res.Checks[0].Command)
			}
		})
	}
}

func TestMerelyMentioningGitHubEnvDoesNotMergeAJob(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - name: Talk about it
        run: echo "this job does not write GITHUB_ENV at all"
      - name: Read it
        run: cat "$GITHUB_ENV" || true
`)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want per-step granularity kept: %+v", len(res.Checks), res.Checks)
	}
}

func TestMergedJobsResolveWorkingDirectoryExpressions(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Install
        run: go install example.com/tool@latest
      - name: Build
        working-directory: ${{ github.workspace }}/sub
        run: go build ./...
`)

	if len(res.Checks) != 1 {
		t.Fatalf("got %d checks, want the job merged: %+v", len(res.Checks), res.Checks)
	}

	chk := res.Checks[0]
	if strings.Contains(chk.Dir, "${{") || strings.Contains(chk.Command, "${{") {
		t.Errorf("the working directory was left unresolved: dir=%q command=%q", chk.Dir, chk.Command)
	}

	if chk.Dir != "" {
		t.Errorf("dir = %q, want none: the steps do not agree on one", chk.Dir)
	}
	if !strings.Contains(chk.Command, "cd ./sub") && !strings.Contains(chk.Command, "cd sub") {
		t.Errorf("command does not place the build in its own directory:\n%s", chk.Command)
	}
	if !strings.HasPrefix(chk.Command, "go install") {
		t.Errorf("the install no longer runs first, at the repository root:\n%s", chk.Command)
	}
}
