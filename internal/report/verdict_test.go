package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/runner"
)

func TestInterruptedRunIsNotAllClear(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Summary(&buf, []runner.Result{
		{Name: "done", Status: runner.StatusPassed},
		{Name: "killed", Status: runner.StatusCancelled},
	}, time.Second, Verdict{Interrupted: true})

	got := buf.String()
	if strings.Contains(got, "\n  all clear.\n") {
		t.Fatalf("an interrupted run reported all clear:\n%s", got)
	}
	if !strings.Contains(got, "interrupted") {
		t.Fatalf("the interruption is invisible:\n%s", got)
	}
}

func TestFailureOutranksInterruption(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Summary(&buf, []runner.Result{
		{Name: "bad", Status: runner.StatusFailed},
		{Name: "killed", Status: runner.StatusCancelled},
	}, time.Second, Verdict{Interrupted: true})

	if !strings.Contains(buf.String(), "CI would fail") {
		t.Fatalf("a real failure vanished behind the interruption:\n%s", buf.String())
	}
}

func TestSingleCheckReadsGrammatically(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Summary(&buf, []runner.Result{{Name: "only", Status: runner.StatusPassed}}, time.Second, Verdict{})

	if strings.Contains(buf.String(), "1 checks") {
		t.Fatalf("summary says \"1 checks\":\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "1 check:") {
		t.Fatalf("summary lost the count line:\n%s", buf.String())
	}
}

func TestJSONReport(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := JSON(&buf, []runner.Result{
		{Index: 1, Name: "second", Status: runner.StatusFailed, Output: "boom", Duration: 2 * time.Second},
		{Index: 0, Name: "first", Status: runner.StatusCached, Reason: "inputs unchanged"},
		{Index: 2, Name: "soft", Status: runner.StatusFailed, Optional: true, Output: "meh"},
	}, 3*time.Second, Verdict{})
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Reason string `json:"reason"`
			Output string `json:"output"`
		} `json:"checks"`
		Failed      bool `json:"failed"`
		Interrupted bool `json:"interrupted"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}

	if len(got.Checks) != 3 || got.Checks[0].Name != "first" || got.Checks[1].Name != "second" {
		t.Fatalf("checks missing or out of config order: %+v", got.Checks)
	}
	if got.Checks[0].Status != "cached" || got.Checks[0].Reason != "inputs unchanged" {
		t.Errorf("cached check rendered as %+v", got.Checks[0])
	}
	if got.Checks[1].Status != "failed" || got.Checks[1].Output != "boom" {
		t.Errorf("failed check rendered as %+v", got.Checks[1])
	}
	if !got.Failed {
		t.Error("a hard failure did not set failed")
	}

	buf.Reset()
	if err := JSON(&buf, []runner.Result{
		{Name: "soft", Status: runner.StatusFailed, Optional: true},
	}, 0, Verdict{}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Failed {
		t.Error("an optional failure alone must not set failed")
	}
}
