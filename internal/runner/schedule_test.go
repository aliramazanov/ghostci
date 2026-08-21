package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
)

type fakeExecutor struct {
	mu      sync.Mutex
	live    int
	peak    int
	started []string
	delay   time.Duration
	fail    map[string]error
}

func (f *fakeExecutor) Execute(ctx context.Context, c Command, out io.Writer) error {
	f.mu.Lock()
	f.live++
	if f.live > f.peak {
		f.peak = f.live
	}
	f.started = append(f.started, c.Script)
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.live--
		f.mu.Unlock()
	}()

	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return ctx.Err()
	}

	fmt.Fprintf(out, "ran %s", c.Script)

	return f.fail[c.Script]
}

func specs(names ...string) []config.Check {
	out := make([]config.Check, len(names))
	for i, n := range names {
		out[i] = config.Check{Name: n, Command: n}
	}

	return out
}

func TestJobsLimitCapsConcurrency(t *testing.T) {
	t.Parallel()

	fake := &fakeExecutor{delay: 20 * time.Millisecond}
	Run(context.Background(), specs("a", "b", "c", "d", "e", "f"),
		Options{Jobs: 2, Executor: fake})

	if fake.peak > 2 {
		t.Errorf("peak concurrency %d, limit was 2", fake.peak)
	}
}

func TestResultsStayInConfigOrder(t *testing.T) {
	t.Parallel()

	fake := &fakeExecutor{}
	got := Run(context.Background(), specs("a", "b", "c"), Options{Executor: fake})

	for i, want := range []string{"a", "b", "c"} {
		if got[i].Name != want || got[i].Index != i {
			t.Errorf("result %d = %s (index %d), want %s", i, got[i].Name, got[i].Index, want)
		}
	}
}

func TestFailFastStopsLaterChecks(t *testing.T) {
	t.Parallel()

	fake := &fakeExecutor{
		delay: 50 * time.Millisecond,
		fail:  map[string]error{"boom": errors.New("nope")},
	}
	results := Run(context.Background(), specs("boom", "a", "b", "c", "d"),
		Options{Jobs: 1, FailFast: true, Executor: fake})

	if results[0].Status != StatusStartError {
		t.Fatalf("boom: %v", results[0].Status)
	}
	if len(fake.started) == len(results) {
		t.Error("fail-fast should have prevented later checks from starting")
	}
}

func TestOnResultFiresOncePerCheck(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	Run(context.Background(), specs("a", "b", "c"), Options{
		Executor: &fakeExecutor{},
		OnResult: func(Result) { calls.Add(1) },
	})

	if calls.Load() != 3 {
		t.Errorf("OnResult fired %d times, want 3", calls.Load())
	}
}

func TestCancelledContextRunsNothing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fake := &fakeExecutor{}
	results := Run(ctx, specs("a", "b"), Options{Executor: fake})

	if len(fake.started) != 0 {
		t.Errorf("started %v despite a cancelled context", fake.started)
	}
	for _, r := range results {
		if r.Status != StatusCancelled {
			t.Errorf("%s = %v, want cancelled", r.Name, r.Status)
		}
	}
}

func TestOutputIsCaptured(t *testing.T) {
	t.Parallel()

	got := Run(context.Background(), specs("hello"), Options{Executor: &fakeExecutor{}})
	if got[0].Output != "ran hello" {
		t.Errorf("output = %q", got[0].Output)
	}
}

func TestPerCheckShellOverridesDefault(t *testing.T) {
	t.Parallel()

	var seen Command
	Run(context.Background(), []config.Check{{Name: "x", Command: "x", Shell: "zsh"}},
		Options{Executor: executorFunc(func(_ context.Context, c Command, _ io.Writer) error {
			seen = c

			return nil
		})})

	if seen.Shell != "zsh" {
		t.Errorf("shell = %q, want zsh", seen.Shell)
	}
}

type executorFunc func(context.Context, Command, io.Writer) error

func (f executorFunc) Execute(ctx context.Context, c Command, out io.Writer) error {
	return f(ctx, c, out)
}
