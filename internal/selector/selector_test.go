package selector

import (
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		glob, file string
		want       bool
	}{
		{"**/*.go", "main.go", true},
		{"**/*.go", "internal/runner/runner.go", true},
		{"**/*.go", "README.md", false},
		{"go.mod", "go.mod", true},
		{"go.mod", "sub/go.mod", false},
		{"src/**/*.ts", "src/a/b.ts", true},
		{"src/**/*.ts", "src/a.ts", true},
		{"src/**/*.ts", "lib/a.ts", false},
		{"**/*.{js,ts}", "a/b.ts", true},
		{"**/*.{js,ts}", "a/b.py", false},
		{"docs", "docs/guide.md", true},
		{"docs", "docsite/guide.md", false},
		{"**/Dockerfile*", "build/Dockerfile.prod", true},
		{"./**/*.go", "main.go", true},

		{"[", "anything.go", true},
	}
	for _, tc := range tests {
		t.Run(tc.glob+"~"+tc.file, func(t *testing.T) {
			t.Parallel()
			if got := matchesInput(tc.glob, tc.file); got != tc.want {
				t.Errorf("matchesInput(%q, %q) = %v, want %v", tc.glob, tc.file, got, tc.want)
			}
		})
	}
}

func checks(specs ...any) []config.Check {
	var out []config.Check
	for i := 0; i+1 < len(specs); i += 2 {
		inputs, _ := specs[i+1].([]string)
		out = append(out, config.Check{Name: specs[i].(string), Inputs: inputs})
	}
	return out
}

func TestSelect(t *testing.T) {
	t.Parallel()
	cs := checks(
		"go", []string{"**/*.go", "go.mod"},
		"ts", []string{"src/**/*.ts"},
		"unknown", nil,
	)
	changes := Changes{Files: []string{"internal/a.go"}, Complete: true, Base: "abc"}

	decisions := Select(cs, changes)
	if len(decisions) != 3 {
		t.Fatalf("got %d decisions", len(decisions))
	}
	if !decisions[0].Run {
		t.Error("go check should run: a .go file changed")
	}
	if decisions[1].Run {
		t.Error("ts check should be skipped: no .ts changed")
	}
	if !decisions[2].Run {
		t.Error("a check with no inputs must always run")
	}
	if decisions[1].Globs == nil {
		t.Error("a skipped check must carry the globs that failed to match")
	}
	if !decisions[2].Unknown {
		t.Error("a check with no inputs must be marked unknown, not merely run")
	}
}

func TestIncompleteChangesRunEverything(t *testing.T) {
	t.Parallel()
	cs := checks("go", []string{"**/*.go"}, "ts", []string{"**/*.ts"})
	changes := Changes{Complete: false, Reason: "not a git repository, running every check"}

	for _, d := range Select(cs, changes) {
		if !d.Run {
			t.Errorf("%s was skipped despite an incomplete change set", d.Check.Name)
		}
		if !d.Unknown {
			t.Errorf("%s must be marked unknown when changes are incomplete", d.Check.Name)
		}
	}
}

func TestAlwaysRunsChecksAreNeverSkipped(t *testing.T) {
	t.Parallel()
	cs := checks(
		"no-inputs", nil,
		"empty", []string{},
		"catch-all", []string{"**"},
	)
	changes := Changes{Files: []string{"README.md"}, Complete: true}
	for _, d := range Select(cs, changes) {
		if !d.Run {
			t.Errorf("%s must always run", d.Check.Name)
		}
	}
}

func TestPartition(t *testing.T) {
	t.Parallel()
	cs := checks("a", []string{"**/*.go"}, "b", []string{"**/*.rs"})
	changes := Changes{Files: []string{"x.go"}, Complete: true}

	run, skipped := Partition(Select(cs, changes))
	if len(run) != 1 || run[0].Name != "a" {
		t.Errorf("run = %+v", run)
	}
	if len(skipped) != 1 || skipped[0].Check.Name != "b" {
		t.Errorf("skipped = %+v", skipped)
	}
}

func TestEmptyChangeSetSkipsNarrowChecks(t *testing.T) {
	t.Parallel()
	cs := checks("go", []string{"**/*.go"})
	changes := Changes{Files: nil, Complete: true, Base: "abc"}

	d := Select(cs, changes)[0]
	if d.Run {
		t.Error("nothing changed, so a narrowly-scoped check should skip")
	}
	if d.Matched != "" {
		t.Errorf("a skipped check must not report a match, got %q", d.Matched)
	}
}

func TestExcludeNarrowsInputs(t *testing.T) {
	t.Parallel()

	chk := config.Check{
		Name:    "go",
		Inputs:  []string{"**/*.go"},
		Exclude: []string{"vendor/**", "**/*_generated.go"},
	}

	cases := map[string]bool{
		"internal/a.go":          true,
		"vendor/x/y.go":          false,
		"pkg/thing_generated.go": false,
		"README.md":              false,
	}

	match := Matcher(chk)
	for file, want := range cases {
		if got := match(file); got != want {
			t.Errorf("Matcher(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestExcludedOnlyChangesSkipTheCheck(t *testing.T) {
	t.Parallel()

	cs := []config.Check{{
		Name:    "go",
		Inputs:  []string{"**/*.go"},
		Exclude: []string{"vendor/**"},
	}}

	d := Select(cs, Changes{Files: []string{"vendor/x/y.go"}, Complete: true})[0]
	if d.Run {
		t.Error("a change only to excluded files should not run the check")
	}

	d = Select(cs, Changes{Files: []string{"vendor/x/y.go", "main.go"}, Complete: true})[0]
	if !d.Run || d.Matched != "main.go" {
		t.Errorf("a non-excluded change must still run it, got run=%v matched=%q", d.Run, d.Matched)
	}
}

func TestBrokenGlobBiasesTowardRunning(t *testing.T) {
	t.Parallel()

	if !matchesInput("[", "main.go") {
		t.Error("a broken input glob should match, so the check runs")
	}
	if matchesExclude("[", "main.go") {
		t.Error("a broken exclude glob should match nothing")
	}

	chk := config.Check{Name: "test", Command: "go test ./...",
		Inputs: []string{"**/*.go"}, Exclude: []string{"["}}

	if !Matcher(chk)("main.go") {
		t.Error("a broken exclusion excluded a real input")
	}
}

func TestTheReportedChangeIsStable(t *testing.T) {
	t.Parallel()

	files := map[string]bool{}
	for _, f := range []string{"h.go", "b.go", "e.go", "a.go", "g.go", "c.go", "d.go", "f.go"} {
		files[f] = true
	}

	first := paths(files)
	for i := 0; i < 200; i++ {
		got := paths(files)
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d gave %v, the first gave %v", i, got, first)
			}
		}
	}

	if first[0] != "a.go" {
		t.Errorf("first changed file is %q, want the order to be sorted", first[0])
	}
}
