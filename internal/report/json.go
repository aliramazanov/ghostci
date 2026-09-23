package report

import (
	"encoding/json"
	"io"
	"sort"
	"time"

	"github.com/aliramazanov/ghostci/internal/runner"
)

type jsonCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Optional   bool   `json:"optional,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Output     string `json:"output,omitempty"`

	ExitCode *int `json:"exit_code,omitempty"`
}

type jsonReport struct {
	Checks      []jsonCheck `json:"checks"`
	Failed      bool        `json:"failed"`
	Interrupted bool        `json:"interrupted"`
	Stale       []string    `json:"stale,omitempty"`
	Unavailable []string    `json:"unavailable,omitempty"`
	ElapsedMS   int64       `json:"elapsed_ms"`
}

func JSON(w io.Writer, results []runner.Result, elapsed time.Duration, v Verdict) error {
	sorted := make([]runner.Result, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })

	out := jsonReport{
		Checks:      make([]jsonCheck, 0, len(sorted)),
		Interrupted: v.Interrupted,
		Stale:       v.Stale,
		Unavailable: v.Unavailable,
		ElapsedMS:   elapsed.Milliseconds(),
	}
	for _, r := range sorted {
		c := jsonCheck{
			Name:       r.Name,
			Status:     statusKey(r.Status),
			DurationMS: r.Duration.Milliseconds(),
			Optional:   r.Optional,
			Reason:     r.Reason,
		}
		if r.ExitCode >= 0 {
			code := r.ExitCode
			c.ExitCode = &code
		}
		if r.Status.CountsAsFailure() {
			c.Output = r.Output
			if r.Err != nil && c.Output == "" {
				c.Output = r.Err.Error()
			}
			if !r.Optional {
				out.Failed = true
			}
		}
		out.Checks = append(out.Checks, c)
	}

	if len(v.Stale) > 0 || len(v.Unavailable) > 0 {
		out.Failed = true
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(out)
}

func statusKey(s runner.Status) string {
	switch s {
	case runner.StatusPassed:
		return "passed"
	case runner.StatusFailed:
		return "failed"
	case runner.StatusTimedOut:
		return "timed_out"
	case runner.StatusCancelled:
		return "cancelled"
	case runner.StatusSkipped:
		return "skipped"
	case runner.StatusCached:
		return "cached"
	case runner.StatusStartError:
		return "start_error"
	case runner.StatusUnavailable:
		return "unavailable"
	}

	return "unknown"
}
