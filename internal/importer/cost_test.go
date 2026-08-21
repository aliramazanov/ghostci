package importer

import "testing"

func TestClassifyCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		job, command string
		want         Cost
	}{
		{"test", "go test ./...", CostCheck},
		{"lint", "golangci-lint run", CostCheck},
		{"ci", "npx eslint src", CostCheck},
		{"ci", "npx tsc --noEmit", CostCheck},
		{"ci", "pytest", CostCheck},
		{"ci", "cargo clippy -- -D warnings", CostCheck},
		{"ci", "go vet ./...", CostCheck},

		{"ci", "docker build -t x .", CostHeavy},
		{"ci", "cargo build --release", CostHeavy},
		{"ci", "npm publish", CostHeavy},
		{"ci", "goreleaser release", CostHeavy},
		{"ci", "python -m build", CostHeavy},
		{"ci", "cargo bench", CostHeavy},

		{"build-binaries", "./scripts/make.sh", CostHeavy},
		{"release", "./ship.sh", CostHeavy},
		{"publish-docker", "./push.sh", CostHeavy},

		{"release", "cargo clippy", CostCheck},
		{"build-binaries", "go test ./...", CostCheck},

		{"ci", "./scripts/verify.sh", CostCheck},
	}

	for _, tc := range tests {
		t.Run(tc.job+"/"+tc.command, func(t *testing.T) {
			t.Parallel()
			got, why := classifyCost(tc.job, tc.command)
			if got != tc.want {
				t.Errorf("classifyCost(%q, %q) = %v, want %v", tc.job, tc.command, got, tc.want)
			}
			if got == CostHeavy && why == "" {
				t.Error("a heavy classification must carry a reason")
			}
		})
	}
}

func TestHeavyStepsAreWrittenNotDropped(t *testing.T) {
	t.Parallel()
	res := importYAML(t, `
on: [push]
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
      - run: docker build -t app .
`)
	if len(res.Checks) != 1 {
		t.Fatalf("got %d enabled checks, want 1", len(res.Checks))
	}
	if len(res.Heavy) != 1 {
		t.Fatalf("got %d heavy checks, want 1", len(res.Heavy))
	}
	if res.HeavyReason[res.Heavy[0].Name] == "" {
		t.Error("heavy check needs a reason")
	}
	if res.Count(Extracted) != 2 {
		t.Errorf("both steps should be extracted, got %d", res.Count(Extracted))
	}
}
