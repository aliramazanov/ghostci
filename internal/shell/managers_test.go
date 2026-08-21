package shell

import (
	"slices"
	"testing"
)

func TestManagersAreCommands(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"npm test":         {"npm"},
		"npm ci":           {"npm"},
		"yarn install":     {"yarn"},
		"pnpm build":       {"pnpm"},
		"bun install":      {"bun"},
		"bundle install":   {"bundle"},
		"poetry install":   {"poetry"},
		"CI=1 npm test":    {"npm"},
		"sudo npm install": {"npm"},
	}

	for script, want := range tests {
		got := Invoked(script)
		for _, w := range want {
			if !slices.Contains(got, w) {
				t.Errorf("Invoked(%q) = %v, want it to include %q", script, got, w)
			}
		}
	}
}

func TestManagersRunningAnotherToolReportBoth(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"pnpm exec tsc --noEmit": {"pnpm", "tsc"},
		"uv run pytest -q":       {"uv", "pytest"},
		"bundle exec rspec":      {"bundle", "rspec"},
		"yarn dlx cowsay moo":    {"yarn", "cowsay"},
		"npm run build":          {"npm"},
	}

	for script, want := range tests {
		got := Invoked(script)
		for _, w := range want {
			if !slices.Contains(got, w) {
				t.Errorf("Invoked(%q) = %v, want it to include %q", script, got, w)
			}
		}
	}
}
