package importer

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func inputsForMakefile(t *testing.T, makefile, command string) []string {
	t.Helper()

	root := t.TempDir()
	if makefile != "" {
		if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte(makefile), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return inferInputsIn(root, "", command)
}

func TestMakeIsInferredFromItsRecipe(t *testing.T) {
	t.Parallel()

	got := inputsForMakefile(t, "test:\n\tgo test ./...\n", "make test")

	if !slices.Contains(got, "**/*.go") {
		t.Errorf("inputs = %v, want the Go files the recipe tests", got)
	}
	if slices.Contains(got, "**/*.c") {
		t.Errorf("inputs = %v, want no C files: nothing here builds C", got)
	}
	if !slices.Contains(got, "**/Makefile") {
		t.Errorf("inputs = %v, want the Makefile itself, which decides what runs", got)
	}
}

func TestMakeWithNoGoalUsesTheFirstRule(t *testing.T) {
	t.Parallel()

	got := inputsForMakefile(t, ".PHONY: all\nall:\n\tcargo build\n\ndocs:\n\tmdformat .\n", "make")

	if !slices.Contains(got, "**/*.rs") {
		t.Errorf("inputs = %v, want the first rule's Rust files", got)
	}
	if slices.Contains(got, "**/*.md") {
		t.Errorf("inputs = %v, want nothing from the docs rule, which make would not run", got)
	}
}

func TestUnreadableMakefileLeavesTheCheckRunning(t *testing.T) {
	t.Parallel()

	if got := inputsForMakefile(t, "", "make test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none: there is no Makefile to read", got)
	}

	if got := inputsForMakefile(t, "build:\n\tgo build ./...\n", "make test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none: the Makefile has no test rule", got)
	}

	if got := inputsForMakefile(t, "test:\n\t./scripts/run-everything.sh\n", "make test"); len(got) != 0 {
		t.Errorf("inputs = %v, want none: the recipe runs something unrecognised", got)
	}
}

func TestMakeRecipeRecursionIsBounded(t *testing.T) {
	t.Parallel()

	got := inputsForMakefile(t, "test:\n\tmake test\n", "make test")

	if len(got) != 0 {
		t.Errorf("inputs = %v, want none", got)
	}
}

func TestMakeDirectoryFlagIsFollowed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "backend")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Makefile"), []byte("check:\n\tpytest\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := inferInputsIn(root, "", "make -C backend check")

	if !slices.Contains(got, "**/*.py") {
		t.Errorf("inputs = %v, want the Python files the recipe in backend tests", got)
	}
}
