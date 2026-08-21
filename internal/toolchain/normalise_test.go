package toolchain

import "testing"

func TestNormaliseMatchesWholeTokens(t *testing.T) {
	t.Parallel()

	positive := map[string]string{
		"setup-go":       "go",
		"setup-golang":   "go",
		"rust-toolchain": "rust",
		"setup-node":     "node",
		"setup-nodejs":   "node",
		"setup-python":   "python",
		"setup-python3":  "python",
		"setup-java":     "java",
		"setup-bun":      "bun",
		"uv":             "uv",
	}
	for in, want := range positive {
		if got := normalise(in); got != want {
			t.Errorf("normalise(%q) = %q, want %q", in, got, want)
		}
	}

	for _, in := range []string{"setup-mongo", "setup-mongodb", "setup-django", "install-cargo-deny"} {
		if got := normalise(in); got == "go" || got == "java" {
			t.Errorf("normalise(%q) = %q: substring leak", in, got)
		}
	}
}
