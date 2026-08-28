package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
)

func checks(pairs ...string) []config.Check {
	out := make([]config.Check, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, config.Check{Name: pairs[i], Command: pairs[i+1]})
	}
	return out
}

func TestStatuses(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), checks(
		"pass", "exit 0",
		"fail", "exit 1",
		"output", "echo hello; echo world >&2",
		"missing", "definitely-not-a-real-command-xyz",
	), Options{})

	if got := len(res); got != 4 {
		t.Fatalf("got %d results, want 4", got)
	}

	for i, want := range []Status{StatusPassed, StatusFailed, StatusPassed, StatusUnavailable} {
		if res[i].Status != want {
			t.Errorf("%s: got %v, want %v", res[i].Name, res[i].Status, want)
		}
	}
	if res[0].Index != 0 || res[3].Index != 3 {
		t.Errorf("results not in config order: %+v", res)
	}
	if out := res[2].Output; !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Errorf("combined output missing stdout or stderr: %q", out)
	}
}

func TestRunsInParallel(t *testing.T) {
	t.Parallel()
	start := time.Now()
	res := Run(context.Background(), checks(
		"a", "sleep 0.5 && echo a",
		"b", "sleep 0.5 && echo b",
		"c", "sleep 0.5 && echo c",
		"d", "sleep 0.5 && echo d",
	), Options{Jobs: 4})
	elapsed := time.Since(start)

	for _, r := range res {
		if r.Status != StatusPassed {
			t.Fatalf("%s: %v", r.Name, r.Status)
		}
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("took %v, expected near 0.5s: checks did not run in parallel", elapsed)
	}
}

func TestJobsLimitIsRespected(t *testing.T) {
	t.Parallel()
	start := time.Now()
	Run(context.Background(), checks(
		"a", "sleep 0.4",
		"b", "sleep 0.4",
	), Options{Jobs: 1})
	if elapsed := time.Since(start); elapsed < 800*time.Millisecond {
		t.Errorf("took %v, expected >=0.8s with Jobs=1: limit not enforced", elapsed)
	}
}

func TestFailFastCancelsSiblings(t *testing.T) {
	t.Parallel()
	start := time.Now()
	res := Run(context.Background(), checks(
		"slow", "sleep 30",
		"boom", "exit 1",
	), Options{FailFast: true, Jobs: 4})
	elapsed := time.Since(start)

	if elapsed > 10*time.Second {
		t.Fatalf("took %v: the sleeper was never killed", elapsed)
	}
	if res[1].Status != StatusFailed {
		t.Errorf("boom: got %v, want FAIL", res[1].Status)
	}
	if got := res[0].Status; got != StatusCancelled {
		t.Errorf("slow: got %v, want cancelled", got)
	}
	if res[0].Status.CountsAsFailure() {
		t.Error("a cancelled check must not count as a failure")
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), checks("slow", "sleep 30"), Options{
		Timeout: 300 * time.Millisecond,
	})
	if res[0].Status != StatusTimedOut {
		t.Fatalf("got %v, want TIMEOUT", res[0].Status)
	}
	if !res[0].Status.CountsAsFailure() {
		t.Error("a timeout must count as a failure")
	}
}

func TestPerCheckTimeoutOverridesGlobal(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), []config.Check{
		{Name: "slow", Command: "sleep 30", Timeout: config.Duration(200 * time.Millisecond)},
	}, Options{Timeout: time.Hour})
	if res[0].Status != StatusTimedOut {
		t.Fatalf("got %v, want TIMEOUT", res[0].Status)
	}
}

func TestGrandchildIsKilled(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("process groups are Unix-specific")
	}
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")

	cmd := fmt.Sprintf("sleep 60 & echo $! > %s; wait", pidFile)

	start := time.Now()
	Run(context.Background(), checks("spawner", cmd), Options{
		Timeout: 500 * time.Millisecond,
	})
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("run took %v: Wait blocked on an orphan holding the pipe", elapsed)
	}

	pid := readPID(t, pidFile)
	if pid == 0 {
		t.Fatal("grandchild never recorded its pid")
	}
	if alive := waitForExit(pid, 10*time.Second); alive {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
		t.Fatalf("grandchild %d survived cancellation: process group was not killed", pid)
	}
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	for range 50 {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0
}

func waitForExit(pid int, within time.Duration) (stillAlive bool) {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(nil) != nil {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(nil) == nil
}

func TestOutputIsBounded(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), checks(
		"flood", "for i in $(seq 1 20000); do echo 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'; done",
	), Options{OutputLimit: 2048})

	if res[0].Status != StatusPassed {
		t.Fatalf("got %v", res[0].Status)
	}
	if got := len(res[0].Output); got > 8192 {
		t.Errorf("captured %d bytes, expected bounded near 2x2048", got)
	}
	if !strings.Contains(res[0].Output, "elided") {
		t.Error("bounded output should mark the elision")
	}
}

func TestStdinIsClosedNotInherited(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), checks("reader", "read line; echo got=$line"), Options{
		Timeout: 3 * time.Second,
	})
	if res[0].Status == StatusTimedOut {
		t.Error("check hung waiting on stdin; stdin must be detached")
	}
}

func TestEnvIsPassedThrough(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), []config.Check{
		{Name: "env", Command: "echo $GHOSTCI_TEST_VAR", Env: map[string]string{"GHOSTCI_TEST_VAR": "hello"}},
	}, Options{})
	if !strings.Contains(res[0].Output, "hello") {
		t.Errorf("env not passed: %q", res[0].Output)
	}
}

func TestDirIsRespected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	res := Run(context.Background(), []config.Check{
		{Name: "ls", Command: "ls", Dir: dir},
	}, Options{})
	if !strings.Contains(res[0].Output, "marker.txt") {
		t.Errorf("dir not respected: %q", res[0].Output)
	}
}

func TestExternalCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	res := Run(ctx, checks("slow", "sleep 30"), Options{})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("took %v: external cancellation ignored", elapsed)
	}
	if res[0].Status != StatusCancelled {
		t.Errorf("got %v, want cancelled", res[0].Status)
	}
}

func TestOnResultFiresPerCheck(t *testing.T) {
	t.Parallel()
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}
	seen := map[string]bool{}
	Run(context.Background(), checks("a", "true", "b", "true"), Options{
		OnResult: func(r Result) {
			<-mu
			seen[r.Name] = true
			mu <- struct{}{}
		},
	})
	if len(seen) != 2 {
		t.Errorf("OnResult fired for %d checks, want 2", len(seen))
	}
}

func TestGitHubStepFilesExist(t *testing.T) {
	t.Parallel()

	res := Run(context.Background(), checks(
		"output", `echo "v=1" >> $GITHUB_OUTPUT`,
		"env", `echo "K=V" >> $GITHUB_ENV`,
		"summary", `echo "# hi" >> $GITHUB_STEP_SUMMARY`,
		"path", `echo /opt/bin >> $GITHUB_PATH`,
	), Options{})

	for _, r := range res {
		if r.Status != StatusPassed {
			t.Errorf("%s: %v\n%s", r.Name, r.Status, r.Output)
		}
	}
}

func TestGitHubStepFilesAreScratch(t *testing.T) {
	t.Parallel()

	res := Run(context.Background(), checks(
		"writer", `echo leaked >> $GITHUB_ENV`,
		"reader", `test ! -s "$GITHUB_ENV"`,
	), Options{Jobs: 1})

	for _, r := range res {
		if r.Status != StatusPassed {
			t.Errorf("%s: %v — each check must get its own empty files", r.Name, r.Status)
		}
	}
}

func TestAFailedExecIsNeverReportedAsAPass(t *testing.T) {
	t.Parallel()

	res := Run(context.Background(), checks("exec-missing", "exec /nonexistent/binary/xyz"), Options{})

	if res[0].Status == StatusPassed {
		t.Fatalf("a check whose program never started was reported as passed\noutput: %s", res[0].Output)
	}

	t.Logf("failed exec reported as %v (unavailable on Linux; recorded here so a "+
		"platform difference is visible rather than assumed)", res[0].Status)
}

func TestResultKeepsTheExitCode(t *testing.T) {
	t.Parallel()

	res := Run(context.Background(), checks(
		"pass", "exit 0",
		"fail", "exit 7",
		"missing", "definitely-not-a-real-command-xyz",
	), Options{})

	if got := res[0].ExitCode; got != 0 {
		t.Errorf("a passing check reported exit %d, want 0", got)
	}
	if got := res[1].ExitCode; got != 7 {
		t.Errorf("a check exiting 7 reported exit %d", got)
	}
	if got := res[2].ExitCode; got != 127 {
		t.Errorf("a missing command reported exit %d, want 127", got)
	}
}

func TestTheSameCommandInTheSameDirectoryTakesTurns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	same := []config.Check{
		{Name: "leg-1", Command: "sleep 0.4", Dir: dir, Env: map[string]string{"TAGS": "a"}},
		{Name: "leg-2", Command: "sleep 0.4", Dir: dir, Env: map[string]string{"TAGS": "b"}},
		{Name: "leg-3", Command: "sleep 0.4", Dir: dir, Env: map[string]string{"TAGS": "c"}},
	}

	start := time.Now()
	for _, r := range Run(context.Background(), same, Options{Jobs: 4}) {
		if r.Status != StatusPassed {
			t.Fatalf("%s: %v", r.Name, r.Status)
		}
	}

	if elapsed := time.Since(start); elapsed < 1100*time.Millisecond {
		t.Errorf("three legs of the same command took %v, so they overlapped", elapsed)
	}
}

func TestTheSameCommandInDifferentDirectoriesStillRunsAtOnce(t *testing.T) {
	t.Parallel()

	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	checks := []config.Check{
		{Name: "a", Command: "sleep 0.5", Dir: a},
		{Name: "b", Command: "sleep 0.5", Dir: b},
		{Name: "c", Command: "sleep 0.5", Dir: c},
	}

	start := time.Now()
	Run(context.Background(), checks, Options{Jobs: 4})

	if elapsed := time.Since(start); elapsed > 1200*time.Millisecond {
		t.Errorf("took %v: separate directories were serialised needlessly", elapsed)
	}
}
