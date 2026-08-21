package toolchain

import (
	"slices"
	"testing"
)

func TestEverydayManagerCommandsCarryTheirToolchain(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"npm test":               "node",
		"npm ci":                 "node",
		"npm run build":          "node",
		"yarn install":           "node",
		"pnpm exec tsc --noEmit": "node",
		"bun test":               "bun",
		"uv run pytest":          "python",
		"poetry run pytest":      "python",
		"bundle exec rspec":      "ruby",
	}

	for command, want := range tests {
		if got := ForCommand(command); !slices.Contains(got, want) {
			t.Errorf("ForCommand(%q) = %v, want it to include %q", command, got, want)
		}
	}
}
