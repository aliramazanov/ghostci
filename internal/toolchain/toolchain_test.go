package toolchain

import "testing"

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
	first := r.Versions("go test ./...")
	second := r.Versions("go vet ./...")
	if first["go"] == "" {
		t.Fatal("expected to resolve the local go version")
	}
	if first["go"] != second["go"] {
		t.Error("resolver returned inconsistent versions")
	}
	if len(r.cache) != 1 {
		t.Errorf("probed %d toolchains, want 1", len(r.cache))
	}
	if got := r.Versions("./scripts/custom.sh"); got != nil {
		t.Errorf("unknown command should imply no toolchain, got %v", got)
	}
}
