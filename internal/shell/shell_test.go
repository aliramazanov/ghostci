package shell

import (
	"slices"
	"testing"
)

func TestInvoked(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"go test ./...":                 {"go"},
		"npx eslint src":                {"eslint"},
		"pnpm exec tsc --noEmit":        {"tsc"},
		"uv run pytest -q":              {"pytest"},
		"CI=true go vet ./...":          {"go"},
		"go build ./... && npx tsc":     {"go", "tsc"},
		"cargo clippy -- -D warnings":   {"cargo"},
		"./scripts/build.sh":            {"build.sh"},
		"set -e\ngo test ./...":         {"set", "go"},
		"if [ -f x ]; then go test; fi": {"go"},

		`test -z "$(gofmt -l .)"`:      {"test", "gofmt"},
		"VERSION=$(git describe) make": {"git", "make"},
		"echo `date`":                  {"echo", "date"},
	}

	for script, want := range tests {
		t.Run(script, func(t *testing.T) {
			t.Parallel()

			got := Invoked(script)
			for _, w := range want {
				if !slices.Contains(got, w) {
					t.Errorf("Invoked(%q) = %v, want it to include %q", script, got, w)
				}
			}
		})
	}
}

func TestArgumentsAreNotCommands(t *testing.T) {
	t.Parallel()

	for _, script := range []string{"echo hi", "go test ./...", "rm -rf build"} {
		if got := Invoked(script); len(got) != 1 {
			t.Errorf("Invoked(%q) = %v, want exactly the leading command", script, got)
		}
	}
}

func TestCommandSubstitutionIsSeen(t *testing.T) {
	t.Parallel()

	if got := Invoked(`test -z "$(gofmt -l .)"`); !slices.Contains(got, "gofmt") {
		t.Fatalf("got %v, want gofmt", got)
	}
}
