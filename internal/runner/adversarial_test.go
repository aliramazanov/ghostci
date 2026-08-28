package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestAFailingCheckThatLeaksAChildIsStillAFailure(t *testing.T) {
	results := Run(context.Background(), []config.Check{{
		Name: "leaky-failure",

		Command: "sleep 30 & exit 3",
	}}, Options{Jobs: 1})

	if got := results[0].Status; got != StatusFailed {
		t.Fatalf("status = %v, want failed: a check exiting 3 was not reported as failing", got)
	}
}

func TestAPassingCheckThatLeaksAChildStillPasses(t *testing.T) {
	results := Run(context.Background(), []config.Check{{
		Name:    "leaky-pass",
		Command: "sleep 30 & exit 0",
	}}, Options{Jobs: 1})

	if got := results[0].Status; got != StatusPassed {
		t.Fatalf("status = %v, want passed", got)
	}
}

func TestATimedOutCheckIsNeverAPass(t *testing.T) {
	results := Run(context.Background(), []config.Check{{
		Name:    "slow",
		Command: "sleep 30",
		Timeout: config.Duration(200 * time.Millisecond),
	}}, Options{Jobs: 1})

	if got := results[0].Status; got != StatusTimedOut {
		t.Fatalf("status = %v, want timed out", got)
	}
	if !results[0].Status.CountsAsFailure() {
		t.Error("a timeout must count as a failure")
	}
}

func TestAMissingInterpreterIsNotAPass(t *testing.T) {
	results := Run(context.Background(), []config.Check{{
		Name:    "no-shell",
		Command: "true",
		Shell:   "/nonexistent/shell-that-does-not-exist",
	}}, Options{Jobs: 1})

	if results[0].Status == StatusPassed {
		t.Fatalf("a check whose shell does not exist was reported as passing")
	}
	if !results[0].Status.CountsAsFailure() {
		t.Errorf("status = %v must count as a failure", results[0].Status)
	}
}

func TestALeakedChildDoesNotStallTheRun(t *testing.T) {
	start := time.Now()

	results := Run(context.Background(), []config.Check{{
		Name:    "leaks",
		Command: "sleep 60 & exit 0",
	}}, Options{Jobs: 1})

	elapsed := time.Since(start)

	if results[0].Status != StatusPassed {
		t.Fatalf("status = %v, want passed", results[0].Status)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("the run waited %v on a leaked child; it must give up after the wait delay", elapsed)
	}
}

func TestRunawayOutputIsBoundedAndKeepsTheEnd(t *testing.T) {
	results := Run(context.Background(), []config.Check{{
		Name:    "loud",
		Command: "for i in $(seq 1 200000); do echo padding-line-$i; done; echo FINAL-MARKER; exit 1",
	}}, Options{Jobs: 1, OutputLimit: 8 << 10})

	out := results[0].Output
	if len(out) > 64<<10 {
		t.Fatalf("captured %d bytes from an unbounded writer", len(out))
	}
	if !strings.Contains(out, "FINAL-MARKER") {
		t.Error("the end of the output was dropped, which is where a failure explains itself")
	}
}

func TestDegenerateChecksStillProduceAVerdict(t *testing.T) {
	checks := []config.Check{
		{Name: "empty-ish", Command: "   "},
		{Name: "newlines", Command: "\n\n\n"},
		{Name: "comment", Command: "# nothing here"},
		{Name: "unicode", Command: "echo 🔥 café"},
		{Name: "quotes", Command: `echo "unbalanced`},
		{Name: "null-ish", Command: "printf 'a\\000b'"},
	}

	results := Run(context.Background(), checks, Options{Jobs: 4})

	for i, r := range results {
		if r.Name != checks[i].Name {
			t.Fatalf("results are out of config order: %s at %d", r.Name, i)
		}
		if r.Status == StatusSkipped || r.Status == StatusCached {
			t.Errorf("%s: the runner invented a skip", r.Name)
		}
	}
}

func TestRepeatedRunsLeakNothing(t *testing.T) {
	checks := []config.Check{
		{Name: "quick", Command: "true"},
		{Name: "output", Command: "echo hello"},
		{Name: "failing", Command: "exit 1"},
	}

	Run(context.Background(), checks, Options{Jobs: 3})

	runtime.GC()
	before := runtime.NumGoroutine()
	beforeFDs := openDescriptors(t)

	for i := 0; i < 25; i++ {
		Run(context.Background(), checks, Options{Jobs: 3})
	}

	runtime.GC()
	time.Sleep(200 * time.Millisecond)

	if after := runtime.NumGoroutine(); after > before+5 {
		t.Errorf("goroutines grew from %d to %d over 25 runs", before, after)
	}
	if after := openDescriptors(t); beforeFDs > 0 && after > beforeFDs+10 {
		t.Errorf("open descriptors grew from %d to %d over 25 runs", beforeFDs, after)
	}
}

func TestCancellingARunKillsGrandchildrenThatIgnoreSIGINT(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "still-running")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, []config.Check{{
			Name: "spawner",

			Command: "sh -c 'sleep 20; touch " + marker + "' & sleep 30",
		}}, Options{Jobs: 1})
	}()

	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the run did not return after cancellation")
	}

	time.Sleep(21 * time.Second)

	if _, err := os.Stat(marker); err == nil {
		t.Error("a grandchild survived the cancelled run and kept working")
	}
}

func openDescriptors(t *testing.T) int {
	t.Helper()

	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0
	}

	return len(entries)
}

func TestAMissingToolIsNotAFailingCheck(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(notExecutable, []byte("data, not a program\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results := Run(context.Background(), []config.Check{
		{Name: "needs-tool", Command: "definitely-not-a-real-command-xyz --version"},
		{Name: "not-executable", Command: notExecutable},
		{Name: "real-failure", Command: "exit 1"},
	}, Options{Jobs: 1})

	if results[0].Status != StatusUnavailable {
		t.Errorf("a missing command gave %v (exit %d), want unavailable\noutput: %s",
			results[0].Status, results[0].ExitCode, results[0].Output)
	}
	if results[1].Status != StatusUnavailable {
		t.Errorf("a non-executable file gave %v (exit %d), want unavailable\noutput: %s",
			results[1].Status, results[1].ExitCode, results[1].Output)
	}
	if results[2].Status != StatusFailed {
		t.Errorf("a real failure gave %v, want failed", results[2].Status)
	}

	if results[0].Status.CountsAsFailure() {
		t.Error("a check that never ran was counted as a failure")
	}
	if !results[2].Status.CountsAsFailure() {
		t.Error("a real failure stopped counting")
	}
}

func TestAMissingWorkingDirectoryIsNamed(t *testing.T) {
	results := Run(context.Background(), []config.Check{{
		Name:    "elsewhere",
		Command: "true",
		Dir:     "does/not/exist",
	}}, Options{Jobs: 1, Root: t.TempDir()})

	if results[0].Status != StatusUnavailable {
		t.Fatalf("status = %v, want unavailable", results[0].Status)
	}
	if !strings.Contains(results[0].Output, "does/not/exist") {
		t.Errorf("the report does not name the missing directory: %q", results[0].Output)
	}
	if strings.Contains(results[0].Output, "bash") {
		t.Errorf("the report blames the shell: %q", results[0].Output)
	}
}

func TestANonExecutableFileIsUnavailableOnEveryPlatform(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho built\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Run(context.Background(), []config.Check{{Name: "absolute", Command: path}}, Options{})
	if res[0].Status != StatusUnavailable {
		t.Errorf("absolute path: got %v (exit %d), want unavailable", res[0].Status, res[0].ExitCode)
	}

	if !strings.Contains(res[0].Output, "not executable") {
		t.Errorf("the reason does not say why: %q", res[0].Output)
	}

	rel := Run(context.Background(),
		[]config.Check{{Name: "relative", Command: "./build.sh", Dir: dir}}, Options{Root: dir})
	if rel[0].Status != StatusUnavailable {
		t.Errorf("relative path: got %v (exit %d), want unavailable", rel[0].Status, rel[0].ExitCode)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}

	ok := Run(context.Background(), []config.Check{{Name: "now-executable", Command: path}}, Options{})
	if ok[0].Status != StatusPassed {
		t.Errorf("after chmod: got %v (exit %d), want passed\noutput: %s",
			ok[0].Status, ok[0].ExitCode, ok[0].Output)
	}
}
