package importer

import (
	"strings"
	"testing"
)

func importAzure(t *testing.T, body string) *Result {
	t.Helper()

	path := writeAt(t, t.TempDir(), AzureFiles[0], body)

	a := DefaultAssumptions()
	a.DefaultBranch = "main"

	res, err := ImportAzure(path, a)
	if err != nil {
		t.Fatalf("ImportAzure: %v", err)
	}

	return res
}

func TestAzureMacroOnlyCIKnowsRefusesTheStep(t *testing.T) {
	t.Parallel()

	res := importAzure(t, `
trigger: [main]
variables:
  buildVer: $(Build.BuildNumber)
steps:
  - script: echo "$(buildVer)"
    displayName: uses a server value
`)

	if len(res.Checks) != 0 {
		t.Fatalf("imported %d checks: %s", len(res.Checks), commandsOf(res))
	}
	if !strings.Contains(reasonsOf(res), "buildVer") {
		t.Errorf("the refusal does not name the macro: %s", reasonsOf(res))
	}
}

func TestAzureResolvesDeclaredVariables(t *testing.T) {
	t.Parallel()

	res := importAzure(t, `
trigger: [main]
variables:
  suite: unit
steps:
  - script: make test-$(suite)
    displayName: declared
`)

	if got := commandsOf(res); got != "make test-unit" {
		t.Errorf("command = %q, want the declared value substituted", got)
	}
}

func TestAzureLeavesShellSubstitutionAlone(t *testing.T) {
	t.Parallel()

	res := importAzure(t, `
trigger: [main]
steps:
  - script: echo "$(pwd)/$(git rev-parse HEAD)"
    displayName: shell
`)

	if got := commandsOf(res); got != `echo "$(pwd)/$(git rev-parse HEAD)"` {
		t.Errorf("command = %q, want the shell substitution untouched", got)
	}
}

func TestAzureDropsVariablesItCannotResolve(t *testing.T) {
	t.Parallel()

	res := importAzure(t, `
trigger: [main]
variables:
  fromServer: $(Build.BuildNumber)
  fromTemplate: ${{ variables.other }}
  plain: hello
steps:
  - script: make build
    displayName: plain
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1", len(res.Checks))
	}

	env := res.Checks[0].Env
	for name, value := range env {
		if strings.Contains(value, "$(") || strings.Contains(value, "${{") {
			t.Errorf("env %s = %q reaches the check unresolved", name, value)
		}
	}
	if env["plain"] != "hello" {
		t.Errorf("plain = %q, want the resolvable variable kept", env["plain"])
	}
}
