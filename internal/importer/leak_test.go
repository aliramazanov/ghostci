package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvValueHoldingAnUnknownExpressionIsNotPassedThrough(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  version:
    runs-on: ubuntu-latest
    outputs:
      v: ${{ steps.get.outputs.v }}
    steps:
      - id: get
        run: echo "v=1.0" >> $GITHUB_OUTPUT
  build:
    needs: version
    runs-on: ubuntu-latest
    env:
      version: ${{ needs.version.outputs.v }}
    steps:
      - run: unzip pkg_${{ env.version }}.zip
`)

	for _, chk := range res.Checks {
		if strings.Contains(chk.Command, "${{") {
			t.Errorf("check %s runs a literal expression: %q", chk.Name, chk.Command)
		}
	}

	var reviewed bool
	for _, e := range res.Entries {
		if strings.Contains(e.Step, "unzip") || strings.Contains(e.Command, "unzip") {
			if e.Outcome == Extracted {
				t.Errorf("a step reading an unknowable env value was imported: %q", e.Command)
			}
			reviewed = true
		}
	}
	if !reviewed {
		t.Error("the step was neither imported nor accounted for")
	}
}

func TestNoCheckCarriesAnUnresolvedExpression(t *testing.T) {
	t.Parallel()

	repos, err := os.ReadDir(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range repos {
		if !r.IsDir() {
			continue
		}

		dir := filepath.Join("testdata", "corpus", r.Name(), ".github", "workflows")
		res, err := ImportDir(dir, DefaultAssumptions())
		if err != nil {
			t.Fatalf("%s: %v", r.Name(), err)
		}

		for _, chk := range res.Checks {
			if strings.Contains(chk.Command, "${{") {
				t.Errorf("%s: check %s runs an unresolved expression:\n%s", r.Name(), chk.Name, chk.Command)
			}
			if strings.Contains(chk.Dir, "${{") {
				t.Errorf("%s: check %s runs in an unresolved directory: %q", r.Name(), chk.Name, chk.Dir)
			}
			for name, value := range chk.Env {
				if strings.Contains(value, "${{") {
					t.Errorf("%s: check %s env %s is unresolved: %q", r.Name(), chk.Name, name, value)
				}
			}
		}
	}
}

func TestMergedJobInheritsWorkingDirectory(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  web:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - run: npm ci
      - run: npm test
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want the job merged into 1", len(res.Checks))
	}
	if got := res.Checks[0].Dir; got != "web" {
		t.Errorf("dir = %q, want web; the check would run from the repository root", got)
	}
}

func TestMergedJobInheritsWorkflowWorkingDirectory(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
defaults:
  run:
    working-directory: app
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
      - run: npm run build
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1", len(res.Checks))
	}
	if got := res.Checks[0].Dir; got != "app" {
		t.Errorf("dir = %q, want app", got)
	}
}

func TestEnvNameNoStepDeclaresIsNotReadAsEmpty(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo "TOOL=/usr/bin/llvm-profdata" >> $GITHUB_ENV
      - run: ${{ env.TOOL }} merge -o merged.profdata profdata
`)

	if len(res.Checks) != 0 {
		t.Fatalf("imported %d checks; the first would run without its program: %q",
			len(res.Checks), res.Checks[0].Command)
	}

	if !strings.Contains(reasonsOf(res), "env.TOOL") {
		t.Errorf("the refusal does not name the variable: %s", reasonsOf(res))
	}
}

func TestDeclaredEnvStillResolves(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
env:
  SUITE: unit
jobs:
  test:
    runs-on: ubuntu-latest
    env:
      LEVEL: fast
    steps:
      - run: make ${{ env.SUITE }}-${{ env.LEVEL }}
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1: declared env must still resolve", len(res.Checks))
	}
	if got := res.Checks[0].Command; got != "make unit-fast" {
		t.Errorf("command = %q, want make unit-fast", got)
	}
}
