package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/runner"
)

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()

	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(string(out))
}

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-q", "-b", "main", "."},
		{"config", "user.email", "t@t.t"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	write(t, dir, "a.go", "package a\n")
	write(t, dir, "README.md", "# hi\n")
	commit(t, dir)

	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir string) {
	t.Helper()

	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "x"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func checks() []config.Check {
	return []config.Check{
		{Name: "go", Command: "true", Inputs: []string{"**/*.go"}},
		{Name: "docs", Command: "true", Inputs: []string{"**/*.md"}},
		{Name: "always", Command: "true"},
	}
}

func actionOf(p Plan, name string) (Action, Reason) {
	for _, d := range p.Decisions {
		if d.Check.Name == name {
			return d.Action, d.Reason
		}
	}

	return -1, Reason{}
}

func TestPlanCoversEveryCheckWithAReason(t *testing.T) {
	t.Parallel()
	p := New(Options{Root: repo(t)}).Plan(checks())

	if len(p.Decisions) != 3 {
		t.Fatalf("got %d decisions for 3 checks", len(p.Decisions))
	}

	for _, d := range p.Decisions {
		if d.Reason.String() == "" {
			t.Errorf("%s has no reason; a plan must always explain itself", d.Check.Name)
		}
	}
}

func TestPlanSelectsByChangedFiles(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	base := headSHA(t, dir)
	write(t, dir, "a.go", "package a\n\nfunc X() {}\n")

	p := New(Options{Root: dir, Since: base}).Plan(checks())

	if a, _ := actionOf(p, "go"); a != Run {
		t.Errorf("go check should run after a .go edit, got %v", a)
	}

	if a, r := actionOf(p, "docs"); a != Skip || r.Kind != NoMatch {
		t.Errorf("docs check should skip with NoMatch, got %v (%v)", a, r.Kind)
	}

	if a, r := actionOf(p, "always"); a != Run || r.Kind != UnknownInputs {
		t.Errorf("a check with no inputs must run with UnknownInputs, got %v (%v)", a, r.Kind)
	}
}

func TestAllRunsEverything(t *testing.T) {
	t.Parallel()
	p := New(Options{Root: repo(t), All: true}).Plan(checks())

	if p.Count(Run) != 3 {
		t.Fatalf("--all should run every check, got %d", p.Count(Run))
	}

	if p.Count(Skip) != 0 {
		t.Error("--all must not skip anything")
	}
}

func TestChecksAndDeferredPartitionThePlan(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	base := headSHA(t, dir)

	write(t, dir, "a.go", "package a\n\nfunc X() {}\n")
	p := New(Options{Root: dir, Since: base}).Plan(checks())

	if got := len(p.Checks()) + len(p.Deferred()); got != len(p.Decisions) {
		t.Errorf("Checks()+Deferred() = %d, want %d: a check went missing", got, len(p.Decisions))
	}
}

func TestUnknownInputsAreNeverCached(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	only := []config.Check{{Name: "always", Command: "true"}}

	eng := New(Options{Root: dir, All: true})
	p := eng.Plan(only)

	if p.Decisions[0].Fingerprint != nil {
		t.Fatal("a check with no inputs must not get a fingerprint")
	}

	eng.Execute(context.Background(), p, Observer{})

	if a, _ := actionOf(New(Options{Root: dir, All: true}).Plan(only), "always"); a != Run {
		t.Error("a check with unknown inputs must run every time")
	}
}

func TestCacheHitOnSecondRun(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	only := []config.Check{{Name: "go", Command: "true", Inputs: []string{"**/*.go"}}}

	eng := New(Options{Root: dir, All: true})
	results := eng.Execute(context.Background(), eng.Plan(only), Observer{})

	if results[0].Status != runner.StatusPassed {
		t.Fatalf("first run: %v", results[0].Status)
	}

	if a, r := actionOf(New(Options{Root: dir, All: true}).Plan(only), "go"); a != Cached ||
		r.Kind != InputsUnchanged {
		t.Fatalf("second run should hit cache, got %v (%v)", a, r.Kind)
	}
}

func TestEditingAnInputInvalidatesTheCache(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	only := []config.Check{{Name: "go", Command: "true", Inputs: []string{"**/*.go"}}}

	eng := New(Options{Root: dir, All: true})
	eng.Execute(context.Background(), eng.Plan(only), Observer{})

	write(t, dir, "a.go", "package a\n\nfunc Changed() {}\n")

	a, reason := actionOf(New(Options{Root: dir, All: true}).Plan(only), "go")

	if a != Run {
		t.Fatalf("editing an input must invalidate the cache, got %v", a)
	}

	if reason.Kind != FingerprintChanged {
		t.Fatalf("want FingerprintChanged, got %v", reason.Kind)
	}

	if !strings.Contains(strings.Join(reason.Diffs, " "), "a.go") {
		t.Errorf("the miss should name what changed, got %v", reason.Diffs)
	}
}

func TestFailuresAreNeverCached(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	only := []config.Check{{Name: "boom", Command: "exit 1", Inputs: []string{"**/*.go"}}}

	eng := New(Options{Root: dir, All: true})
	results := eng.Execute(context.Background(), eng.Plan(only), Observer{})

	if results[0].Status != runner.StatusFailed {
		t.Fatalf("expected a failure, got %v", results[0].Status)
	}

	if a, _ := actionOf(New(Options{Root: dir, All: true}).Plan(only), "boom"); a != Run {
		t.Error("a failing check must re-run at full cost every time")
	}
}

func TestNoCacheBypassesRecordedPasses(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	only := []config.Check{{Name: "go", Command: "true", Inputs: []string{"**/*.go"}}}

	eng := New(Options{Root: dir, All: true})
	eng.Execute(context.Background(), eng.Plan(only), Observer{})

	if a, _ := actionOf(New(Options{Root: dir, All: true, NoCache: true}).Plan(only), "go"); a != Run {
		t.Error("--no-cache must ignore recorded passes")
	}
}

func TestOutsideAGitRepoEverythingRuns(t *testing.T) {
	t.Parallel()
	p := New(Options{Root: t.TempDir()}).Plan(checks())

	if p.Count(Run) != 3 {
		t.Fatalf("outside a repo every check must run, got %d", p.Count(Run))
	}

	for _, d := range p.Decisions {
		if d.Fingerprint != nil {
			t.Errorf("%s must not be cacheable without a repo", d.Check.Name)
		}
	}
}

func TestExecuteReturnsOneResultPerCheck(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	base := headSHA(t, dir)

	write(t, dir, "a.go", "package a\n\nfunc X() {}\n")

	eng := New(Options{Root: dir, Since: base})
	p := eng.Plan(checks())
	results := eng.Execute(context.Background(), p, Observer{})

	if len(results) != 3 {
		t.Fatalf("got %d results for 3 checks: skipped ones must still be reported", len(results))
	}
}

func TestFailed(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		status runner.Status
		want   bool
	}{
		"passed":    {runner.StatusPassed, false},
		"failed":    {runner.StatusFailed, true},
		"timed out": {runner.StatusTimedOut, true},
		"cancelled": {runner.StatusCancelled, false},
		"skipped":   {runner.StatusSkipped, false},
		"cached":    {runner.StatusCached, false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Failed([]runner.Result{{Status: tc.status}}); got != tc.want {
				t.Errorf("Failed(%v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestChecksRunCheapestFirst(t *testing.T) {
	t.Parallel()

	p := Plan{
		Decisions: []Decision{
			{Check: config.Check{Name: "slow"}, Action: Run},
			{Check: config.Check{Name: "fast"}, Action: Run},
			{Check: config.Check{Name: "unmeasured"}, Action: Run},
			{Check: config.Check{Name: "skipped"}, Action: Skip},
		},

		cost: map[string]time.Duration{
			"slow": 9 * time.Second,
			"fast": 100 * time.Millisecond,
		},
	}

	got := p.Checks()
	want := []string{"unmeasured", "fast", "slow"}

	if len(got) != len(want) {
		t.Fatalf("got %d checks, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("position %d = %s, want %s (order: %v)", i, got[i].Name, want[i], names(got))
		}
	}
}

func names(checks []config.Check) []string {
	out := make([]string, len(checks))
	for i, c := range checks {
		out[i] = c.Name
	}

	return out
}

func TestOrderingPreservesMembership(t *testing.T) {
	t.Parallel()

	dir := repo(t)
	eng := New(Options{Root: dir, All: true})
	p := eng.Plan(checks())

	if len(p.Checks()) != p.Count(Run) {
		t.Errorf("ordering changed the run set: %d vs %d", len(p.Checks()), p.Count(Run))
	}
}

func TestNoCacheStillRecords(t *testing.T) {
	t.Parallel()

	dir := repo(t)
	only := []config.Check{{Name: "go", Command: "true", Inputs: []string{"**/*.go"}}}

	bypass := New(Options{Root: dir, All: true, NoCache: true})
	bypass.Execute(context.Background(), bypass.Plan(only), Observer{})

	if a, _ := actionOf(New(Options{Root: dir, All: true}).Plan(only), "go"); a != Cached {
		t.Error("a --no-cache run should still record, so the next run can hit")
	}
}

func TestOptionalFailureDoesNotFailTheRun(t *testing.T) {
	t.Parallel()

	results := []runner.Result{
		{Name: "flaky", Status: runner.StatusFailed, Optional: true},
		{Name: "test", Status: runner.StatusPassed},
	}

	if Failed(results) {
		t.Error("an optional failure must not fail the run")
	}

	results[1].Status = runner.StatusFailed

	if !Failed(results) {
		t.Error("a real failure must still fail the run")
	}
}

func TestChecksWatchingNothingAreNamed(t *testing.T) {
	dir := repo(t)

	eng := New(Options{Root: dir, All: true})
	plan := eng.Plan([]config.Check{
		{Name: "typo", Command: "true", Inputs: []string{"nowhere/**"}},
		{Name: "fine", Command: "true", Inputs: []string{"**/*.go"}},
		{Name: "everything", Command: "true"},
	})

	if len(plan.Decisions) != 3 {
		t.Fatalf("got %d decisions", len(plan.Decisions))
	}

	got := eng.WatchingNothing()
	if len(got) != 1 || got[0] != "typo" {
		t.Errorf("WatchingNothing() = %v, want only the typo'd check", got)
	}
}
