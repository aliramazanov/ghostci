package importer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func inputsForPackage(t *testing.T, manifest, command string) []string {
	t.Helper()

	root := t.TempDir()
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return inferInputsIn(root, "", command)
}

func TestNpmScriptIsInferredFromTheManifest(t *testing.T) {
	t.Parallel()

	got := inputsForPackage(t, `{"scripts":{"test":"vitest run"}}`, "npm test")

	if !slices.Contains(got, "**/*.ts") {
		t.Errorf("inputs = %v, want the files vitest reads", got)
	}
	if !slices.Contains(got, "package.json") {
		t.Errorf("inputs = %v, want package.json, which decides what the script is", got)
	}
}

func TestInstallWatchesTheManifestAlone(t *testing.T) {
	t.Parallel()

	got := inputsForPackage(t, `{"scripts":{}}`, "npm ci")

	if !slices.Contains(got, "package-lock.json") {
		t.Errorf("inputs = %v, want the lockfile", got)
	}
	if slices.Contains(got, "**/*.ts") {
		t.Errorf("inputs = %v, want no source files: an install does not read them", got)
	}
}

func TestOneUnresolvedScriptLeavesTheWholeCheckRunning(t *testing.T) {
	t.Parallel()

	got := inputsForPackage(t, `{"scripts":{"test":"./scripts/everything.sh"}}`, "npm ci\nnpm test")

	if len(got) != 0 {
		t.Errorf("inputs = %v, want none: the test script runs something unrecognised", got)
	}
}

func TestMissingScriptLeavesTheCheckRunning(t *testing.T) {
	t.Parallel()

	if got := inputsForPackage(t, `{"scripts":{"build":"tsc"}}`, "npm test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none: there is no test script", got)
	}
	if got := inputsForPackage(t, "", "npm test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none: there is no package.json", got)
	}
	if got := inputsForPackage(t, "{not json", "npm test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none: the manifest could not be read", got)
	}
}

func TestRunFormsResolveTheSameWay(t *testing.T) {
	t.Parallel()

	manifest := `{"scripts":{"lint":"eslint ."}}`
	for _, command := range []string{"npm run lint", "pnpm lint", "yarn lint", "bun run lint"} {
		got := inputsForPackage(t, manifest, command)
		if !slices.Contains(got, "**/*.js") {
			t.Errorf("%q: inputs = %v, want the files eslint reads", command, got)
		}
	}
}

func TestScriptRecursionIsBounded(t *testing.T) {
	t.Parallel()

	if got := inputsForPackage(t, `{"scripts":{"test":"npm test"}}`, "npm test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none", got)
	}
}
