package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aliramazanov/ghostci/internal/runner"
)

func Line(w io.Writer, res runner.Result) {
	switch res.Status {
	case runner.StatusSkipped, runner.StatusCached:
		fmt.Fprintf(w, "  %-8s %-24s (%s)\n", res.Status, res.Name, res.Reason)
	case runner.StatusStartError:
		fmt.Fprintf(w, "  %-8s %-24s %s\n", res.Status, res.Name, res.Err)
	default:
		fmt.Fprintf(w, "  %-8s %-24s %s\n", res.Status, res.Name, dur(res.Duration))
	}
}

type Verdict struct {
	Interrupted bool

	Unavailable []string

	WatchingNothing []string

	Overridden []string

	Stale []string
}

func (v Verdict) NeedsAttention() bool {
	return v.Interrupted ||
		len(v.Stale) > 0 ||
		len(v.Unavailable) > 0 ||
		len(v.WatchingNothing) > 0 ||
		len(v.Overridden) > 0
}

func Summary(w io.Writer, results []runner.Result, elapsed time.Duration, v Verdict) {
	var passed, failed, cached, skipped, cancelled, optional, unavailable int
	for _, r := range results {
		switch {
		case r.Status == runner.StatusPassed:
			passed++
		case r.Status == runner.StatusCached:
			cached++
		case r.Status == runner.StatusSkipped:
			skipped++
		case r.Status == runner.StatusCancelled:
			cancelled++
		case r.Status == runner.StatusUnavailable:
			unavailable++
		case r.Status.CountsAsFailure() && r.Optional:
			optional++
		case r.Status.CountsAsFailure():
			failed++
		}
	}

	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(passed, "ok")
	add(failed, "failed")
	add(cached, "cached")
	add(skipped, "skipped")
	add(cancelled, "cancelled")
	add(unavailable, "could not run")
	add(optional, "failed but optional")
	if len(parts) == 0 {
		parts = append(parts, "nothing to do")
	}

	fmt.Fprintf(w, "\n  %s: %s\t%s\n", counted(len(results), "check"), strings.Join(parts, ", "), dur(elapsed))

	for _, r := range results {
		if r.Status.CountsAsFailure() {
			_, _ = Failure{Name: r.Name, Output: r.Output}.WriteTo(w)
		}
	}

	if len(v.WatchingNothing) > 0 {
		verb, runs := "watches", "it runs"
		if len(v.WatchingNothing) > 1 {
			verb, runs = "watch", "they run"
		}

		fmt.Fprintf(w, "\n  warning: %s %s no file here, so %s every time. Check its inputs.\n",
			namedChecks(v.WatchingNothing), verb, runs)
	}

	if len(v.Overridden) > 0 {
		subject := "this variable"
		if len(v.Overridden) > 1 {
			subject = "these variables"
		}

		fmt.Fprintf(w, "\n  warning: your environment already sets %s.\n", subject)
		for _, line := range v.Overridden {
			fmt.Fprintf(w, "    %s\n", line)
		}
		fmt.Fprintf(w, "  checks saw those values, not the ones for this tree.\n")
	}

	switch {
	case failed > 0:
		fmt.Fprintf(w, "\n  %s. CI would fail.\n", plural(failed, "check"))
	case v.Interrupted:
		fmt.Fprintf(w, "\n  interrupted, so this is not an all clear.\n")
	case len(v.Unavailable) > 0:
		fmt.Fprintf(w, "\n  %s could not run here, so this is not an all clear.\n"+
			"  Something each one needs is missing from this machine rather than from CI.\n"+
			"  The output above names it.\n",
			namedChecks(v.Unavailable))
	case len(v.Stale) > 0:
		fmt.Fprintf(w, "\n  %s changed while the run was in progress, so this is not\n"+
			"  an all clear. A check rewrote them. Run again on the settled tree.\n",
			inputsOf(v.Stale))
	default:
		fmt.Fprintf(w, "\n  all clear.\n")
	}
}

func namedChecks(names []string) string {
	if len(names) == 1 {
		return names[0]
	}

	return strings.Join(names, ", ")
}

func inputsOf(names []string) string {
	if len(names) == 1 {
		return "Files " + names[0] + " depends on"
	}

	return "Files these checks depend on (" + strings.Join(names, ", ") + ")"
}

func counted(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}

	return fmt.Sprintf("%d %ss", n, word)
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s failed", n, word)
	}
	return fmt.Sprintf("%d %ss failed", n, word)
}

func dur(d time.Duration) string {
	switch {
	case d == 0:
		return ""
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}
