package importer

import (
	"strings"
	"testing"
)

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

func TestSideEffectsNeverRunByDefault(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ job, command string }{
		{"autocommit", "git config user.name bot\ngit commit -am regen\ngit push origin main"},
		{"coverage", "go test -coverprofile=c.out ./...\ngoveralls -coverprofile=c.out"},
		{"coverage", "codecov -f coverage.xml"},
		{"notify", `curl -X POST -d '{"text":"done"}' https://hooks.slack.com/services/X`},
		{"release", "gh release create v1.0.0"},
		{"docs", "aws s3 sync ./site s3://bucket"},
		{"deploy", "kubectl apply -f k8s/"},
		{"publish", "npm publish --access public"},
		{"sync", "rsync -az ./dist user@host:/srv"},
	} {
		cost, why := classifyCost(c.job, c.command)
		if cost != CostHeavy {
			t.Errorf("%s: %q was classified runnable, want held back", c.job, c.command)
		}
		if why == "" {
			t.Errorf("%s: %q held back with no reason", c.job, c.command)
		}
	}
}

func TestOrdinaryChecksStayRunnable(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ job, command string }{
		{"test", "go test ./..."},
		{"lint", "golangci-lint run"},
		{"smoke-local", "curl -X POST -d '{}' http://localhost:8080/api"},
		{"smoke", "curl --data @body.json http://127.0.0.1:3000/health"},
		{"fetch", "curl -sSL https://example.com/schema.json -o schema.json"},
	} {
		if cost, _ := classifyCost(c.job, c.command); cost != CostCheck {
			t.Errorf("%s: %q was held back, want runnable", c.job, c.command)
		}
	}
}

func TestAWriteFlagInOneStatementDoesNotConvictAnother(t *testing.T) {
	t.Parallel()

	script := `sudo install -d -m 0755 /etc/apt/keyrings
wget -qO- https://apt.fury.io/nushell/gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/fury.gpg
make test`

	if postsSomewhere(strings.ToLower(script)) {
		t.Error("a download beside an unrelated -d was read as posting data")
	}

	withPost := script + "\ncurl -X POST -d @report.json https://example.com/ingest"
	if !postsSomewhere(strings.ToLower(withPost)) {
		t.Error("a genuine post went unnoticed")
	}
}

func TestRootAndSystemInstallsAreHeldBack(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		"sudo install -d -m 0755 /etc/apt/keyrings\nmake test",
		"sudo apt-get install --yes tmux",
		"apt-get install -y build-essential",
		"brew install fish",
		"apk add --no-cache git",
		"sudo systemctl restart nginx",
	} {
		cost, why := classifyCost("build", command)
		if cost != CostHeavy {
			t.Errorf("%q was classified runnable, want held back", command)
		}

		if why == "" {
			t.Errorf("%q held back with no reason", command)
		}
	}

	if cost, _ := classifyCost("test", `go test ./... -run TestSudoParsing`); cost != CostCheck {
		t.Error("a test whose name contains sudo was held back")
	}
}

func TestGlobalInstallsAreHeldBackButProjectOnesAreNot(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		"cargo install --locked --path .",
		"cargo install cargo-audit --locked",
		"go install github.com/mattn/goveralls@latest",
		"npm i -g typescript",
		"pipx install ruff",
	} {
		if cost, _ := classifyCost("build", command); cost != CostHeavy {
			t.Errorf("%q was classified runnable, want held back", command)
		}
	}

	for _, command := range []string{
		"npm ci",
		"npm install",
		"pip install -r requirements.txt",
		"poetry install",
		"bundle install",
		"uv sync --frozen",
	} {
		if cost, why := classifyCost("setup", command); cost != CostCheck {
			t.Errorf("%q was held back (%s), want runnable", command, why)
		}
	}
}
