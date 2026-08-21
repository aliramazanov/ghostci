package importer

import (
	"strings"
	"testing"
)

func TestAMergedJobMixingToleratedStepsIsSurfaced(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
      - run: npm run flaky
        continue-on-error: true
`)

	if len(res.Checks) != 0 {
		t.Fatalf("imported %d checks: %q would fail the push on a step CI forgives",
			len(res.Checks), res.Checks[0].Command)
	}
	if res.Entries[0].Outcome != NeedsReview {
		t.Errorf("outcome = %v, want NeedsReview", res.Entries[0].Outcome)
	}
	if !strings.Contains(res.Entries[0].Reason, "continue-on-error") {
		t.Errorf("reason does not explain itself: %q", res.Entries[0].Reason)
	}
}

func TestAMergedJobOfOnlyToleratedStepsIsOptional(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
        continue-on-error: true
      - run: npm run flaky
        continue-on-error: true
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1", len(res.Checks))
	}
	if !res.Checks[0].Optional {
		t.Error("the check is required, but CI forgives every step in it")
	}
}

func TestAMergedJobWithNoToleratedStepsStaysRequired(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
      - run: npm test
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1", len(res.Checks))
	}
	if res.Checks[0].Optional {
		t.Error("the check is optional, but CI forgives nothing in it")
	}
	if !res.Checks[0].Serial {
		t.Error("the merged check lost its ordering")
	}
}

func TestMergedStepsKeepTheirOwnDirectories(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
        working-directory: web
      - run: go test ./...
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want the job merged", len(res.Checks))
	}

	chk := res.Checks[0]
	if chk.Dir != "" {
		t.Errorf("dir = %q, want none: the steps do not agree on one", chk.Dir)
	}
	if !strings.Contains(chk.Command, "cd web") {
		t.Errorf("the install lost its directory:\n%s", chk.Command)
	}

	after := chk.Command[strings.Index(chk.Command, "go test"):]
	if strings.Contains(after, "cd ") {
		t.Errorf("a step with no directory was placed in one:\n%s", chk.Command)
	}
}

func TestMergedStepsAgreeingOnADirectoryUseItDirectly(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - run: npm ci
      - run: npm test
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1", len(res.Checks))
	}
	if res.Checks[0].Dir != "web" {
		t.Errorf("dir = %q, want web", res.Checks[0].Dir)
	}
	if strings.Contains(res.Checks[0].Command, "cd ") {
		t.Errorf("the command was rewritten needlessly:\n%s", res.Checks[0].Command)
	}
}
