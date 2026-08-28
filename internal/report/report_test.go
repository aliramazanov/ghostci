package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/runner"
)

func TestSummaryCounts(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		results  []runner.Result
		contains []string
		absent   []string
	}{
		"all pass": {
			results: []runner.Result{
				{Name: "a", Status: runner.StatusPassed},
				{Name: "b", Status: runner.StatusPassed},
			},
			contains: []string{"2 checks", "2 ok", "all clear"},
			absent:   []string{"CI would fail"},
		},
		"one fails": {
			results: []runner.Result{
				{Name: "a", Status: runner.StatusPassed},
				{Name: "b", Status: runner.StatusFailed, Output: "boom"},
			},
			contains: []string{"1 ok", "1 failed", "1 check failed", "CI would fail", "boom"},
			absent:   []string{"all clear"},
		},
		"two fail pluralises": {
			results: []runner.Result{
				{Name: "a", Status: runner.StatusFailed},
				{Name: "b", Status: runner.StatusFailed},
			},
			contains: []string{"2 checks failed"},
		},
		"cancelled is not a failure": {
			results: []runner.Result{
				{Name: "a", Status: runner.StatusFailed},
				{Name: "b", Status: runner.StatusCancelled},
			},
			contains: []string{"1 failed", "1 cancelled", "1 check failed"},
			absent:   []string{"2 checks failed"},
		},
		"cached and skipped": {
			results: []runner.Result{
				{Name: "a", Status: runner.StatusCached, Reason: "inputs unchanged"},
				{Name: "b", Status: runner.StatusSkipped, Reason: "no matching changes"},
			},
			contains: []string{"1 cached", "1 skipped", "all clear"},
		},
		"timeout counts as failure": {
			results:  []runner.Result{{Name: "a", Status: runner.StatusTimedOut}},
			contains: []string{"1 failed", "CI would fail"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			Summary(&buf, tc.results, 1500*time.Millisecond, Verdict{})
			got := buf.String()
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
			for _, no := range tc.absent {
				if strings.Contains(got, no) {
					t.Errorf("output should not contain %q:\n%s", no, got)
				}
			}
		})
	}
}

func TestRendererNeverRewritesTerminal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	results := []runner.Result{
		{Name: "a", Status: runner.StatusPassed, Duration: time.Second},
		{Name: "b", Status: runner.StatusFailed, Output: "bad"},
		{Name: "c", Status: runner.StatusCached, Reason: "inputs unchanged"},
	}
	for _, r := range results {
		Line(&buf, r)
	}
	Summary(&buf, results, time.Second, Verdict{})

	for _, seq := range []string{"\x1b[", "\r", "\x1b[2K", "\x1b[A"} {
		if strings.Contains(buf.String(), seq) {
			t.Errorf("output contains terminal control sequence %q, which can destroy scrollback", seq)
		}
	}
}

func TestFailureOutputOnlyShownForFailures(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Summary(&buf, []runner.Result{
		{Name: "quiet", Status: runner.StatusPassed, Output: "should-not-appear"},
		{Name: "loud", Status: runner.StatusFailed, Output: "should-appear"},
	}, time.Second, Verdict{})

	got := buf.String()
	if strings.Contains(got, "should-not-appear") {
		t.Error("output of a passing check must not be dumped")
	}
	if !strings.Contains(got, "should-appear") {
		t.Error("output of a failing check must be dumped")
	}
}

func TestLineShowsReasonForSkippedAndCached(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Line(&buf, runner.Result{Name: "docs", Status: runner.StatusSkipped, Reason: "no matching changes"})
	if got := buf.String(); !strings.Contains(got, "no matching changes") {
		t.Errorf("reason not surfaced: %q", got)
	}
}

func TestDurationFormat(t *testing.T) {
	t.Parallel()
	tests := map[time.Duration]string{
		0:                       "",
		400 * time.Millisecond:  "400ms",
		1500 * time.Millisecond: "1.5s",
		8700 * time.Millisecond: "8.7s",
	}
	for in, want := range tests {
		if got := dur(in); got != want {
			t.Errorf("dur(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestJSONCarriesTheExitCode(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := JSON(&buf, []runner.Result{
		{Index: 0, Name: "ok", Status: runner.StatusPassed, ExitCode: 0},
		{Index: 1, Name: "bad", Status: runner.StatusFailed, ExitCode: 7},
		{Index: 2, Name: "skipped", Status: runner.StatusSkipped, ExitCode: -1},
	}, time.Second, Verdict{})
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		Checks []struct {
			Name     string `json:"name"`
			ExitCode *int   `json:"exit_code"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	by := map[string]*int{}
	for _, c := range got.Checks {
		by[c.Name] = c.ExitCode
	}

	if by["ok"] == nil || *by["ok"] != 0 {
		t.Errorf("passed check: exit_code = %v, want 0", by["ok"])
	}
	if by["bad"] == nil || *by["bad"] != 7 {
		t.Errorf("failed check: exit_code = %v, want 7", by["bad"])
	}
	if by["skipped"] != nil {
		t.Errorf("skipped check: exit_code = %v, want absent", *by["skipped"])
	}
}

func TestNeedsAttentionCoversEveryWarning(t *testing.T) {
	t.Parallel()

	if (Verdict{}).NeedsAttention() {
		t.Error("a clean run wants attention")
	}

	for name, v := range map[string]Verdict{
		"interrupted":      {Interrupted: true},
		"stale":            {Stale: []string{"a"}},
		"could not run":    {Unavailable: []string{"a"}},
		"watching nothing": {WatchingNothing: []string{"a"}},
		"overridden env":   {Overridden: []string{"GITHUB_REF=x, not y"}},
	} {
		if !v.NeedsAttention() {
			t.Errorf("%s would be swallowed by quiet mode", name)
		}
	}
}
