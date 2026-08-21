package importer

import "testing"

func TestWorkflowLevelDefaults(t *testing.T) {
	res := importYAML(t, `
name: ci
on: [push]
defaults:
  run:
    working-directory: ./frontend
    shell: bash
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm test
`)

	if len(res.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(res.Checks))
	}
	if got := res.Checks[0].Dir; got != "./frontend" {
		t.Errorf("dir = %q, want ./frontend", got)
	}
	if got := res.Checks[0].Shell; got != "bash --noprofile --norc -eo pipefail" {
		t.Errorf("shell = %q, want the bash invocation GitHub uses", got)
	}
}

func TestJobDefaultsOverrideWorkflowDefaults(t *testing.T) {
	res := importYAML(t, `
name: ci
on: [push]
defaults:
  run:
    working-directory: ./frontend
jobs:
  build:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: ./backend
    steps:
      - run: go test ./...
      - run: go vet ./...
        working-directory: ./cmd
`)

	if len(res.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(res.Checks))
	}
	if got := res.Checks[0].Dir; got != "./backend" {
		t.Errorf("dir = %q, want ./backend", got)
	}
	if got := res.Checks[1].Dir; got != "./cmd" {
		t.Errorf("dir = %q, want ./cmd", got)
	}
}

func TestDefaultShellIsLeftToTheRunner(t *testing.T) {
	res := importYAML(t, `
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make
`)

	if got := res.Checks[0].Shell; got != "" {
		t.Errorf("shell = %q, want it left unset", got)
	}
}

func TestWorkingDirectoryIsInterpolated(t *testing.T) {
	res := importYAML(t, `
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        pkg: [api]
    steps:
      - run: go build ./...
        working-directory: services/${{ matrix.pkg }}
`)

	if len(res.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(res.Checks))
	}
	if got := res.Checks[0].Dir; got != "services/api" {
		t.Errorf("dir = %q, want services/api", got)
	}
}
