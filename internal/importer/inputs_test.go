package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aliramazanov/ghostci/internal/selector"
)

func watches(globs []string, file string) bool {
	for _, g := range globs {
		if selector.Match(g, file) {
			return true
		}
	}

	return false
}

func TestALinterConfigChangeReachesTheLinter(t *testing.T) {
	t.Parallel()

	tests := []struct{ command, file string }{
		{"golangci-lint run", ".golangci.yml"},
		{"golangci-lint run ./...", "tools/.golangci.yaml"},
		{"npx eslint .", ".eslintrc.json"},
		{"npx eslint .", "web/.eslintrc.cjs"},
		{"npx prettier --check .", ".prettierrc"},
		{"uv run ruff check .", "ruff.toml"},
		{"poetry run mypy src", "mypy.ini"},
		{"flake8", ".flake8"},
		{"pytest", "pytest.ini"},
		{"cargo clippy", "clippy.toml"},
		{"cargo fmt --check", "rustfmt.toml"},
		{"cargo build", ".cargo/config.toml"},
		{"bundle exec rubocop", ".rubocop.yml"},
		{"shellcheck scripts/*.sh", ".shellcheckrc"},
		{"yamllint .", ".yamllint"},
	}
	for _, tc := range tests {
		t.Run(tc.command+" "+tc.file, func(t *testing.T) {
			t.Parallel()

			if got := inferInputs(tc.command); !watches(got, tc.file) {
				t.Errorf("%q does not watch %s, so changing it would reuse a stale pass: %v", tc.command, tc.file, got)
			}
		})
	}
}

func TestAToolDoesNotWatchAnotherToolsConfig(t *testing.T) {
	t.Parallel()

	tests := []struct{ command, file string }{
		{"go test ./...", ".golangci.yml"},
		{"pytest", "ruff.toml"},
		{"npx tsc --noEmit", ".prettierrc"},
	}
	for _, tc := range tests {
		if got := inferInputs(tc.command); watches(got, tc.file) {
			t.Errorf("%q watches %s, which it never reads: %v", tc.command, tc.file, got)
		}
	}
}

func TestInputsFollowTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	res := importYAML(t, synthWorkflow(
		newJob("web", run("npx eslint .").in("./web")),
		newJob("api", run("go test ./...").in("services/api")),
	))

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want 2", len(res.Checks))
	}

	want := map[string][]string{
		"./web":        {"web/package.json", "web/package-lock.json", "web/tsconfig.json", "package-lock.json"},
		"services/api": {"services/api/go.mod", "services/api/go.sum", "go.work"},
	}

	for _, c := range res.Checks {
		for _, file := range want[c.Dir] {
			if !watches(c.Inputs, file) {
				t.Errorf("%s runs in %s but does not watch %s: %v", c.Name, c.Dir, file, c.Inputs)
			}
		}
	}
}

func TestAMergedJobWatchesEachStepsDirectory(t *testing.T) {
	t.Parallel()

	res := importYAML(t, `
name: t
on: [push]
jobs:
  all:
    runs-on: ubuntu-latest
    steps:
      - run: npm ci
        working-directory: web
      - run: npx eslint .
        working-directory: web
      - run: go test ./...
        working-directory: api
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want the job merged into 1", len(res.Checks))
	}

	for _, file := range []string{"web/package-lock.json", "web/package.json", "api/go.mod", "package-lock.json"} {
		if !watches(res.Checks[0].Inputs, file) {
			t.Errorf("the merged check does not watch %s: %v", file, res.Checks[0].Inputs)
		}
	}
}

func TestMakeDirectoryIsRelativeToTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "web", "tools")

	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(sub, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := inferInputsIn(root, "web", "make -C tools test")

	if !watches(got, "web/tools/go.mod") {
		t.Errorf("make -C tools from web reads web/tools/Makefile, so it must watch web/tools/go.mod: %v", got)
	}
}

func TestOnlyADirectoryInsideTheRepositoryScopesInputs(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"web":            "web",
		"./web/":         "web",
		"services//api":  "services/api",
		"":               "",
		".":              "",
		"..":             "",
		"../sibling":     "",
		"/srv/app":       "",
		"~/project":      "",
		"$HOME/app":      "",
		"${{ env.DIR }}": "",
		"pkg/*":          "",
		`windows\path`:   "",
	}
	for dir, want := range tests {
		got, ok := repoRelative(dir)
		if got != want || ok != (want != "") {
			t.Errorf("repoRelative(%q) = %q, %v; want %q", dir, got, ok, want)
		}
	}

	if got := scoped(nil, "web"); got != nil {
		t.Errorf("scoping no inputs must stay no inputs, so the check still always runs: %v", got)
	}
}

func withManifest(t *testing.T, scripts string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":`+scripts+`}`), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestAScriptRunInAnotherPackageIsNotReadFromTheRoot(t *testing.T) {
	t.Parallel()

	root := withManifest(t, `{"build": "tsc -b", "test": "eslint ."}`)

	for _, command := range []string{
		"npm run build --workspace=packages/foo",
		"npm run build -w packages/foo",
		"npm run build --workspaces",
		"npm --prefix packages/foo run build",
		"pnpm --filter foo build",
		"pnpm -r run build",
		"pnpm -C packages/foo build",
		"yarn --cwd packages/foo build",
		"cd packages/foo && npm test",
	} {
		if got := inferInputsIn(root, "", command); got != nil {
			t.Errorf("%q runs a script the root manifest does not describe, so it must always run: %v", command, got)
		}
	}

	if got := inferInputsIn(root, "", "npm test -- -w"); !watches(got, "src/a.ts") {
		t.Errorf("flags after -- belong to the script, not npm: %v", got)
	}
}

func TestAToolRunAfterCdWatchesItsManifestWhereverItIs(t *testing.T) {
	t.Parallel()

	got := inferInputs("cd web && npx eslint .")

	for _, file := range []string{"web/package-lock.json", "web/package.json", "web/.eslintrc.json", "package-lock.json"} {
		if !watches(got, file) {
			t.Errorf("after cd the check does not watch %s: %v", file, got)
		}
	}
}
