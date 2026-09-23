package toolchain

import (
	"os"
	"slices"
	"testing"
)

func TestMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pinned, local string
		want          bool
	}{
		{"1.26", "1.26.5", true},
		{"1.26.5", "1.26.5", true},
		{"1.25", "1.26.5", false},
		{"1.26.4", "1.26.5", false},
		{"22.x", "22.17.0", true},
		{"22.x", "24.17.0", false},
		{"v20", "20.1.0", true},
		{"stable", "1.90.0", true},
		{"latest", "1.90.0", true},
		{"nightly", "1.90.0", true},
		{"lts/*", "22.1.0", true},

		{"", "1.26.5", true},
		{"1.26", "", true},
		{"", "", true},

		{"1.26.5.1", "1.26.5", false},
	}
	for _, tc := range tests {
		t.Run(tc.pinned+"/"+tc.local, func(t *testing.T) {
			t.Parallel()
			if got := Matches(tc.pinned, tc.local); got != tc.want {
				t.Errorf("Matches(%q, %q) = %v, want %v", tc.pinned, tc.local, got, tc.want)
			}
		})
	}
}

func TestNormalise(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"go": "go", "node": "node", "python": "python",
		"rust-toolchain": "rust", "uv": "uv", "deno": "deno",
		"unknown-tool": "unknown-tool",
	}
	for in, want := range tests {
		if got := normalise(in); got != want {
			t.Errorf("normalise(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalGo(t *testing.T) {
	t.Parallel()
	if v := Local("go"); v == "" {
		t.Error("expected to detect the local go version")
	}
	if v := Local("definitely-not-a-toolchain"); v != "" {
		t.Errorf("unknown toolchain should report nothing, got %q", v)
	}
}

func TestForCommand(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"go test ./...":             {"go"},
		"gofmt -l .":                {"go"},
		"npx eslint src":            {"node"},
		"npx tsc --noEmit":          {"node"},
		"cargo clippy":              {"rust"},
		"pytest -q":                 {"python"},
		"./scripts/custom.sh":       nil,
		"echo hello":                nil,
		"go build ./... && npx tsc": {"go", "node"},
		`test -z "$(gofmt -l .)"`:   {"go"},
	}
	for cmd, want := range tests {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			got := ForCommand(cmd)
			if len(got) != len(want) {
				t.Fatalf("ForCommand(%q) = %v, want %v", cmd, got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("ForCommand(%q) = %v, want %v", cmd, got, want)
				}
			}
		})
	}
}

func TestResolverCaches(t *testing.T) {
	t.Parallel()
	r := NewResolver()
	first := r.Versions(For("go test ./...", nil))
	second := r.Versions(For("go vet ./...", nil))
	if first["go"] == "" {
		t.Fatal("expected to resolve the local go version")
	}
	if first["go"] != second["go"] {
		t.Error("resolver returned inconsistent versions")
	}
	if len(r.cache) != 1 {
		t.Errorf("probed %d toolchains, want 1", len(r.cache))
	}
	if got := r.Versions(For("./scripts/custom.sh", nil)); got != nil {
		t.Errorf("unknown command should imply no toolchain, got %v", got)
	}
}

func TestForFiles(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		files []string
		want  []string
	}{
		"go sources":        {[]string{"cmd/main.go", "go.mod"}, []string{"go"}},
		"typescript":        {[]string{"src/app.ts", "package.json"}, []string{"node"}},
		"python manifest":   {[]string{"svc/pyproject.toml"}, []string{"python"}},
		"gradle kotlin dsl": {[]string{"build.gradle.kts"}, []string{"java"}},
		"no toolchain":      {[]string{"Makefile", "README.md", "docs/x.yml"}, nil},
		"mixed":             {[]string{"a.go", "web/b.tsx", "c.go"}, []string{"go", "node"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := ForFiles(slices.Values(tc.files)); !slices.Equal(got, tc.want) {
				t.Errorf("ForFiles(%v) = %v, want %v", tc.files, got, tc.want)
			}
		})
	}
}

func TestACommandThatHidesItsToolchainStillRecordsIt(t *testing.T) {
	t.Parallel()
	r := NewResolver()

	if got := r.Versions(For("make test", nil)); got != nil {
		t.Fatalf("make names no toolchain on its own, got %v", got)
	}

	if got := r.Versions(For("make test", slices.Values([]string{"internal/a.go", "go.mod"}))); got["go"] == "" {
		t.Errorf("a check watching Go sources must record the go version, got %v", got)
	}
}

func withoutToolchainEnv(t *testing.T) {
	t.Helper()

	for _, keys := range toolchainEnv {
		for _, key := range keys {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
}

func TestEnvironmentAToolchainReadsIsRecorded(t *testing.T) {
	withoutToolchainEnv(t)

	if got := Environment([]string{"go", "python"}, nil); got != nil {
		t.Fatalf("nothing is set, got %v", got)
	}

	t.Setenv("GOFLAGS", "-tags=integration")
	t.Setenv("PYTEST_ADDOPTS", "-x")

	got := Environment([]string{"go"}, nil)

	if _, ok := got["GOFLAGS"]; !ok {
		t.Fatalf("GOFLAGS changes what go builds, so it must be recorded: %v", got)
	}

	if got["GOFLAGS"] == "-tags=integration" {
		t.Error("the raw value was recorded; only a digest belongs on disk")
	}

	if _, ok := got["PYTEST_ADDOPTS"]; ok {
		t.Errorf("a go check recorded a python variable: %v", got)
	}

	other := Environment([]string{"go"}, nil)
	t.Setenv("GOFLAGS", "-race")

	if Environment([]string{"go"}, nil)["GOFLAGS"] == other["GOFLAGS"] {
		t.Error("two different values recorded the same digest")
	}
}

func TestAVariableTheCheckSetsItselfIsNotAmbient(t *testing.T) {
	withoutToolchainEnv(t)
	t.Setenv("GOFLAGS", "-tags=integration")

	if got := Environment([]string{"go"}, map[string]string{"GOFLAGS": "-mod=mod"}); got != nil {
		t.Errorf("the check's own env wins over the shell, so the shell value is irrelevant: %v", got)
	}
}
