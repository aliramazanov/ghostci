package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		yaml    string
		want    int
		wantErr string
	}{
		"minimal": {
			yaml: "checks:\n  - name: vet\n    command: go vet ./...\n",
			want: 1,
		},
		"all fields": {
			yaml: `
checks:
  - name: test
    command: go test ./...
    dir: ./sub
    shell: bash
    timeout: 90s
    inputs: ["**/*.go", "go.mod"]
    env:
      CI: "true"
`,
			want: 1,
		},
		"multiple checks": {
			yaml: "checks:\n  - name: a\n    command: 'true'\n  - name: b\n    command: 'true'\n",
			want: 2,
		},
		"empty checks list":  {yaml: "checks: []\n", wantErr: "no checks defined"},
		"missing checks key": {yaml: "other: 1\n", wantErr: "field other not found"},
		"malformed yaml":     {yaml: "checks:\n  - name: [unclosed\n", wantErr: "parsing config"},
		"missing name":       {yaml: "checks:\n  - command: 'true'\n", wantErr: "name is required"},
		"missing command":    {yaml: "checks:\n  - name: a\n", wantErr: "command is required"},
		"duplicate names": {
			yaml:    "checks:\n  - name: a\n    command: 'true'\n  - name: a\n    command: 'true'\n",
			wantErr: "duplicate name",
		},
		"unknown field": {
			yaml:    "checks:\n  - name: a\n    command: 'true'\n    bogus: 1\n",
			wantErr: "field bogus not found",
		},
		"bad timeout": {
			yaml:    "checks:\n  - name: a\n    command: 'true'\n    timeout: soon\n",
			wantErr: "invalid timeout",
		},
		"negative timeout": {
			yaml:    "checks:\n  - name: a\n    command: 'true'\n    timeout: -5s\n",
			wantErr: "must be positive",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg, err := Parse([]byte(tc.yaml))

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %q", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := len(cfg.Checks); got != tc.want {
				t.Fatalf("got %d checks, want %d", got, tc.want)
			}
		})
	}
}

func TestParseFieldValues(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]byte(`
checks:
  - name: test
    command: go test ./...
    dir: ./sub
    timeout: 90s
    inputs: ["**/*.go"]
    env:
      CI: "true"
`))
	if err != nil {
		t.Fatal(err)
	}

	c := cfg.Checks[0]

	if c.Name != "test" || c.Command != "go test ./..." || c.Dir != "./sub" {
		t.Errorf("bad scalar fields: %+v", c)
	}

	if time.Duration(c.Timeout) != 90*time.Second {
		t.Errorf("timeout = %v, want 90s", time.Duration(c.Timeout))
	}

	if c.Env["CI"] != "true" {
		t.Errorf("env = %v", c.Env)
	}
}

func TestAlwaysRuns(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		inputs []string
		want   bool
	}{
		"no inputs":       {nil, true},
		"empty inputs":    {[]string{}, true},
		"catch-all":       {[]string{"**"}, true},
		"catch-all slash": {[]string{"**/*"}, true},
		"catch-all mixed": {[]string{"**/*.go", "**"}, true},
		"specific":        {[]string{"**/*.go"}, false},
		"several":         {[]string{"src/**/*.ts", "package.json"}, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := (Check{Inputs: tc.inputs}).AlwaysRuns(); got != tc.want {
				t.Errorf("AlwaysRuns() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))

	if err == nil {
		t.Fatal("want error for missing file")
	}

	if !strings.Contains(err.Error(), "nope.yaml") {
		t.Errorf("the error should name the path it looked for: %v", err)
	}
}

func TestErrNoChecksIsIdentifiable(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte("checks: []\n"))

	if !errors.Is(err, ErrNoChecks) {
		t.Errorf("want ErrNoChecks, got %v", err)
	}
}

func TestMissingConfigIsTyped(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))

	var missing *NotFoundError

	if !errors.As(err, &missing) {
		t.Fatalf("want *NotFoundError, got %T: %v", err, err)
	}

	if missing.Path == "" {
		t.Error("the error should name the path it looked for")
	}
}

func TestMalformedConfigIsNotNotFound(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ghostci.yaml")

	if err := os.WriteFile(path, []byte("checks:\n  - name: [bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var missing *NotFoundError

	if _, err := Load(path); errors.As(err, &missing) {
		t.Error("a malformed config must not report as missing")
	}
}
