package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/engine"
	"github.com/aliramazanov/ghostci/internal/git"
	"github.com/aliramazanov/ghostci/internal/hook"
	"github.com/aliramazanov/ghostci/internal/report"
	"github.com/aliramazanov/ghostci/internal/runner"
)

type options struct {
	file     string
	jobs     int
	failFast bool
	timeout  time.Duration
	all      bool
	explain  bool
	noCache  bool
	since    string
	hookMode bool
	jsonOut  bool
}

func parseRunFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("ghostci", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var o options

	fs.StringVar(&o.file, "f", "", "path to config file (default: the nearest "+defaultConfig+")")
	fs.IntVar(&o.jobs, "j", 0, "max checks to run at once (default: CPU count)")
	fs.BoolVar(&o.failFast, "fail-fast", false, "stop the remaining checks on first failure")
	fs.DurationVar(&o.timeout, "timeout", 0, "default per-check timeout (0 = none)")
	fs.BoolVar(&o.all, "all", false, "run every check, ignoring which files changed")
	fs.BoolVar(&o.explain, "explain", false, "print why each check ran, was cached, or was skipped")
	fs.BoolVar(&o.noCache, "no-cache", false,
		"re-run everything selected, ignoring recorded passes (results are still recorded)")
	fs.StringVar(&o.since, "since", "", "compare against this ref instead of the upstream branch")
	fs.BoolVar(&o.hookMode, "hook", false, "running as a git pre-push hook")
	fs.BoolVar(&o.jsonOut, "json", false, "print one JSON document instead of the live report")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ghostci [flags]\n\nRuns the checks in %s in parallel.\n\nflags:\n",
			defaultConfig)
		fs.PrintDefaults()
	}

	return o, fs.Parse(args)
}

func runChecks(args []string) int {
	opts, err := parseRunFlags(args)
	if err != nil {
		return exitBadUsage
	}

	var pushed []hook.Ref
	if opts.hookMode {
		pushed = hook.ReadRefs(os.Stdin)
	}

	if err := runner.Unsupported(); err != nil {
		return reportUnsupported(os.Stderr, err, opts.hookMode)
	}

	if runner.Disabled() {
		if !opts.hookMode {
			fmt.Fprintf(os.Stderr, "ghostci: %s is set to off, running nothing\n", runner.Disable)
		}

		return exitOK
	}

	path := opts.file
	if path == "" {
		path = config.Find(".")
	}

	cfg, err := config.Load(path)
	if err != nil {
		return reportConfigError(err, path, opts.hookMode)
	}

	if opts.hookMode && onlyDeletions(pushed) {
		fmt.Println("ghostci: only deletions pushed, nothing to check")

		return exitOK
	}

	base, ok := diffBase(opts, pushed)
	if !ok {
		return exitOK
	}

	eng := engine.New(engine.Options{
		Root:     filepath.Dir(path),
		All:      opts.all,
		NoCache:  opts.noCache,
		Since:    base,
		Jobs:     opts.jobs,
		FailFast: opts.failFast,
		Timeout:  opts.timeout,
	})

	plan := eng.Plan(cfg.Checks)

	if opts.explain && !opts.jsonOut {
		report.Explain(os.Stdout, plan)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if plan.Count(engine.Run) == 0 {
		return reportNothingToRun(plan, opts)
	}

	return runPlan(ctx, eng, plan, cfg.Checks, opts)
}

func diffBase(opts options, pushed []hook.Ref) (string, bool) {
	if opts.since != "" || !opts.hookMode {
		return opts.since, true
	}

	resolved, ok, unchecked := pushBase(pushed, headSHA(), commitOf)
	if !ok {
		fmt.Fprintln(os.Stderr, "ghostci: the refs being pushed are not what is checked out, "+
			"so there is nothing here to check them against. Nothing was verified.")

		return "", false
	}

	if len(unchecked) > 0 {
		fmt.Fprintf(os.Stderr, "ghostci: checking %s only; not checked: %s\n",
			currentRefName(), strings.Join(unchecked, ", "))
	}

	return resolved, true
}

func reportNothingToRun(plan engine.Plan, opts options) int {
	deferred := plan.Deferred()

	if opts.jsonOut {
		if err := report.JSON(os.Stdout, deferred, 0, report.Verdict{}); err != nil {
			return exitBadUsage
		}

		return exitOK
	}

	for _, r := range deferred {
		report.Line(os.Stdout, r)
	}

	report.Summary(os.Stdout, deferred, 0, report.Verdict{})

	return exitOK
}

func runPlan(ctx context.Context, eng *engine.Engine, plan engine.Plan,
	checks []config.Check, opts options) int {

	onResult := func(runner.Result) {}

	if !opts.jsonOut {
		fmt.Printf("ghostci: running %d of %d checks\n\n", plan.Count(engine.Run), len(checks))

		var mu sync.Mutex

		onResult = func(res runner.Result) {
			mu.Lock()
			defer mu.Unlock()
			report.Line(os.Stdout, res)
		}
	}

	start := time.Now()
	results := eng.Execute(ctx, plan, onResult)

	verdict := report.Verdict{
		Interrupted:     ctx.Err() != nil && engine.Cancelled(results),
		Stale:           eng.Stale(plan, results),
		Unavailable:     engine.Unavailable(results),
		WatchingNothing: eng.WatchingNothing(),
	}

	if opts.jsonOut {
		if err := report.JSON(os.Stdout, results, time.Since(start), verdict); err != nil {
			return exitBadUsage
		}
	} else {
		report.Summary(os.Stdout, results, time.Since(start), verdict)
	}

	return verdictExit(results, verdict, opts.hookMode)
}

func verdictExit(results []runner.Result, verdict report.Verdict, hookMode bool) int {
	switch {
	case engine.Failed(results):
		return exitFailed
	case verdict.Interrupted:
		return exitInterrupted
	case len(verdict.Stale) > 0:
		return exitFailed

	case len(verdict.Unavailable) > 0 && !hookMode:
		return exitFailed
	}

	return exitOK
}

func reportUnsupported(w io.Writer, err error, hookMode bool) int {
	fmt.Fprintf(w, "ghostci: %v\n", err)

	if hookMode {
		_, _ = fmt.Fprintln(w, "ghostci: nothing was verified, and the push was not checked.")

		return exitOK
	}

	return exitBadUsage
}

func reportConfigError(err error, file string, hookMode bool) int {
	switch {
	case errors.Is(err, config.ErrNoChecks):
		fmt.Fprintf(os.Stderr, "ghostci: %s defines no checks\n", file)
	case os.IsNotExist(errors.Unwrap(err)):
		if hookMode {
			fmt.Fprintf(os.Stderr, "ghostci: no %s, skipping pre-push checks\n", file)
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "ghostci: no %s here. Run \"ghostci init\" to create one.\n", file)
	default:
		fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
	}

	return exitBadUsage
}

func pushBase(refs []hook.Ref, head string, commitOf func(string) string) (base string, ok bool, unchecked []string) {
	var live, atHead []hook.Ref

	for _, r := range refs {
		if r.Deleted() {
			continue
		}

		live = append(live, r)

		if head != "" && commitOf(r.LocalSHA) == head {
			atHead = append(atHead, r)
		}
	}

	if len(live) == 0 {
		return "", true, nil
	}

	if len(atHead) == 0 {
		return "", false, nil
	}

	for _, r := range live {
		if head == "" || commitOf(r.LocalSHA) != head {
			unchecked = append(unchecked, r.LocalRef)
		}
	}

	for _, r := range atHead {
		if r.NewBranch() || r.RemoteSHA != atHead[0].RemoteSHA {
			return "", true, unchecked
		}
	}

	return atHead[0].RemoteSHA, true, unchecked
}

func currentRefName() string {
	if b := git.Discover(".").Branch; b != "" {
		return b
	}

	return "what is checked out"
}

func commitOf(sha string) string {
	if sha == "" {
		return ""
	}

	if commit, err := git.Resolve(".", sha); err == nil && commit != "" {
		return commit
	}

	return sha
}

func headSHA() string {
	sha, err := git.Resolve(".", "HEAD")

	if err != nil {
		return ""
	}

	return sha
}

func onlyDeletions(refs []hook.Ref) bool {
	if len(refs) == 0 {
		return false
	}

	for _, r := range refs {
		if !r.Deleted() {
			return false
		}
	}

	return true
}
