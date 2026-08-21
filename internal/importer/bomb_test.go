package importer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/gitlab"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

const bomb = `
a: &a ["x","x","x","x","x","x","x","x","x"]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d]
f: &f [*e,*e,*e,*e,*e,*e,*e,*e,*e]
g: &g [*f,*f,*f,*f,*f,*f,*f,*f,*f]
h: &h [*g,*g,*g,*g,*g,*g,*g,*g,*g]
i: &i [*h,*h,*h,*h,*h,*h,*h,*h,*h]
jobs: *i
`

func TestYAMLBombIsRefusedByEveryParser(t *testing.T) {
	for name, parse := range map[string]func() error{
		"workflow": func() error { _, err := workflow.Parse([]byte(bomb)); return err },
		"gitlab":   func() error { _, err := gitlab.Parse([]byte(bomb)); return err },
		"config":   func() error { _, err := config.Parse([]byte(bomb)); return err },
	} {
		done := make(chan error, 1)
		go func() { done <- parse() }()

		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Errorf("%s: parsing never returned on an expanding alias bomb", name)
		}
	}
}

func TestCompositeActionCyclesAreRefused(t *testing.T) {
	dir := t.TempDir()

	writeAt(t, dir, ".github/actions/loop/action.yml", `
name: loop
runs:
  using: composite
  steps:
    - uses: ./.github/actions/loop
`)
	writeAt(t, dir, ".github/actions/ping/action.yml", `
name: ping
runs:
  using: composite
  steps:
    - uses: ./.github/actions/pong
`)
	writeAt(t, dir, ".github/actions/pong/action.yml", `
name: pong
runs:
  using: composite
  steps:
    - uses: ./.github/actions/ping
`)
	writeAt(t, dir, ".github/workflows/ci.yml", `
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/loop
      - uses: ./.github/actions/ping
`)

	done := make(chan *Result, 1)
	go func() {
		res, err := ImportDir(filepath.Join(dir, ".github", "workflows"), DefaultAssumptions())
		if err != nil {
			done <- nil
			return
		}
		done <- res
	}()

	select {
	case res := <-done:
		if res == nil {
			return
		}

		if len(res.Checks) > 0 {
			t.Errorf("a cyclic composite action produced checks: %v", commandsOf(res))
		}
	case <-time.After(20 * time.Second):
		t.Fatal("importing a self-using composite action never returned")
	}
}

func TestReusableWorkflowCyclesAreRefused(t *testing.T) {
	dir := t.TempDir()

	writeAt(t, dir, ".github/workflows/a.yml", `
on: [push, workflow_call]
jobs:
  call:
    uses: ./.github/workflows/b.yml
`)
	writeAt(t, dir, ".github/workflows/b.yml", `
on: [workflow_call]
jobs:
  call:
    uses: ./.github/workflows/a.yml
`)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = ImportDir(filepath.Join(dir, ".github", "workflows"), DefaultAssumptions())
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a reusable workflow cycle never returned")
	}
}
