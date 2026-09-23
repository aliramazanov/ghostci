package ghostci_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci"
)

func repo(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestLoadReadsTheChecks(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{"ghostci.yaml": `
checks:
  - name: unit
    command: echo hello
    inputs: ["**/*.go"]
`})

	checks, err := ghostci.Load(filepath.Join(dir, "ghostci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 {
		t.Fatalf("got %d checks, want 1", len(checks))
	}
	if checks[0].Name != "unit" || checks[0].Command != "echo hello" {
		t.Errorf("check = %+v", checks[0])
	}
}

func TestLoadReportsAMissingFile(t *testing.T) {
	t.Parallel()

	if _, err := ghostci.Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("a missing config was loaded without error")
	}
}

func TestRunExecutesAndReportsFailure(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{"ghostci.yaml": `
checks:
  - name: passes
    command: "true"
  - name: fails
    command: "exit 3"
`})

	results, err := ghostci.Run(context.Background(), filepath.Join(dir, "ghostci.yaml"),
		ghostci.Options{Root: dir, All: true, NoCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	byName := map[string]ghostci.Result{}
	for _, r := range results {
		byName[r.Name] = r
	}

	if got := byName["passes"].Status; got != ghostci.StatusPassed {
		t.Errorf("passes: status = %v, want StatusPassed", got)
	}
	if got := byName["fails"].Status; got != ghostci.StatusFailed {
		t.Errorf("fails: status = %v, want StatusFailed", got)
	}
	if !ghostci.Failed(results) {
		t.Error("Failed reported no failure on a failing run")
	}
}

func TestFailedIgnoresOptionalChecks(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{"ghostci.yaml": `
checks:
  - name: tolerated
    command: "exit 1"
    optional: true
`})

	results, err := ghostci.Run(context.Background(), filepath.Join(dir, "ghostci.yaml"),
		ghostci.Options{Root: dir, All: true, NoCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if ghostci.Failed(results) {
		t.Error("an optional check failed the run")
	}
}

func TestStatusConstantsAreUsable(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		status ghostci.Status
		fails  bool
	}{
		{ghostci.StatusPassed, false},
		{ghostci.StatusFailed, true},
		{ghostci.StatusTimedOut, true},
		{ghostci.StatusCancelled, false},
		{ghostci.StatusSkipped, false},
		{ghostci.StatusCached, false},
		{ghostci.StatusStartError, true},
		{ghostci.StatusUnavailable, false},
	} {
		if got := c.status.CountsAsFailure(); got != c.fails {
			t.Errorf("%v.CountsAsFailure() = %v, want %v", c.status, got, c.fails)
		}
		if c.status.String() == "" {
			t.Errorf("%v has no name", c.status)
		}
	}
}

func TestPlanChecksExplainsEveryDecision(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{"src/a.go": "package a\n"})

	planned := ghostci.PlanChecks([]ghostci.Check{
		{Name: "go", Command: "go test ./...", Inputs: []string{"**/*.go"}},
		{Name: "docs", Command: "mdformat .", Inputs: []string{"docs/**"}},
	}, ghostci.Options{Root: dir, NoCache: true})

	if len(planned) != 2 {
		t.Fatalf("got %d decisions, want 2", len(planned))
	}

	for _, p := range planned {
		switch p.Action {
		case "run", "skip", "cached":
		default:
			t.Errorf("%s: action = %q, want run, skip or cached", p.Check.Name, p.Action)
		}
		if p.Reason == "" {
			t.Errorf("%s: decided %q with no reason", p.Check.Name, p.Action)
		}
	}
}

func TestDetectFindsTheProvider(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{".github/workflows/ci.yml": `
name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: go build ./...
`})

	provider, path, ok := Detect(t, dir)
	if !ok {
		t.Fatal("no provider detected for a repository with a workflow")
	}
	if !strings.Contains(strings.ToLower(provider), "github") {
		t.Errorf("provider = %q, want GitHub", provider)
	}
	if !strings.Contains(filepath.ToSlash(path), ".github/workflows") {
		t.Errorf("path = %q", path)
	}

	if _, _, ok := Detect(t, t.TempDir()); ok {
		t.Error("a provider was detected in a repository with no CI configuration")
	}
}

func Detect(t *testing.T, dir string) (string, string, bool) {
	t.Helper()

	return ghostci.Detect(dir)
}

func TestImportExtractsAndReportsRefusals(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{".github/workflows/ci.yml": `
name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: go build ./...
  needs-a-service:
    runs-on: ubuntu-latest
    services:
      db:
        image: postgres
    steps:
      - run: go test ./...
`})

	res, err := ghostci.Import(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Checks) == 0 {
		t.Fatal("nothing was extracted from a workflow with a plain run step")
	}
	if len(res.Refused) == 0 {
		t.Fatal("the job needing a service container was not reported as refused")
	}

	for _, r := range res.Refused {
		if r.Reason == "" {
			t.Errorf("refusal %+v carries no reason", r)
		}
		if r.Outcome == "" {
			t.Errorf("refusal %+v carries no outcome", r)
		}
	}
}

func TestImportReportsAnUnreadableDirectory(t *testing.T) {
	t.Parallel()

	if _, err := ghostci.Import(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("importing a directory that does not exist reported no error")
	}
}

func TestFailedCountsACheckThatCouldNotRun(t *testing.T) {
	t.Parallel()

	if !ghostci.Failed([]ghostci.Result{{Name: "lint", Status: ghostci.StatusUnavailable}}) {
		t.Error("a run that verified nothing was reported as not failed")
	}
}

func TestRunSaysWhenACheckRewroteTheTree(t *testing.T) {
	t.Parallel()

	dir := repo(t, map[string]string{
		"a.txt": "one\n",
		"ghostci.yaml": `
checks:
  - name: rewrites
    command: "echo more >> a.txt"
    inputs: ["*.txt"]
`})

	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	results, err := ghostci.Run(context.Background(), filepath.Join(dir, "ghostci.yaml"),
		ghostci.Options{Root: dir, All: true, NoCache: true})

	if !errors.Is(err, ghostci.ErrTreeChanged) {
		t.Fatalf("err = %v, want ErrTreeChanged: the results no longer describe the tree", err)
	}

	if len(results) != 1 {
		t.Errorf("the results were dropped along with the error: %v", results)
	}
}
