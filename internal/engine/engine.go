package engine

import (
	"context"
	"time"

	"github.com/aliramazanov/ghostci/internal/cache"
	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/git"
	"github.com/aliramazanov/ghostci/internal/runner"
	"github.com/aliramazanov/ghostci/internal/selector"
	"github.com/aliramazanov/ghostci/internal/toolchain"
)

type Options struct {
	Root string

	All     bool
	NoCache bool
	Since   string

	Jobs     int
	FailFast bool
	Timeout  time.Duration
}

type Engine struct {
	opts     Options
	store    *cache.Store
	versions *toolchain.Resolver
	git      *git.Session

	before map[string]map[string]string

	watchingNothing []string
}

func (e *Engine) WatchingNothing() []string { return e.watchingNothing }

func (e *Engine) OverriddenContext() []string { return e.context().Overridden() }

func New(opts Options) *Engine {
	if opts.Root == "" {
		opts.Root = "."
	}

	return &Engine{
		opts:     opts,
		store:    cache.Open(opts.Root, cacheMode(opts)),
		versions: toolchain.NewResolver(),
		git:      git.NewSession(opts.Root),
	}
}

func (e *Engine) planAll(checks []config.Check) Plan {
	plan := Plan{
		Changes:   selector.Changes{Reason: "--all was given"},
		Decisions: make([]Decision, 0, len(checks)),
	}

	tree := e.scanTree()

	for _, chk := range checks {
		plan.Decisions = append(plan.Decisions, e.cacheDecision(chk, Reason{Kind: Forced}, tree))
	}

	plan.cost = e.costs(checks)

	return plan
}

func (e *Engine) cacheDecision(chk config.Check, why Reason, tree *cache.Tree) Decision {
	if chk.AlwaysRuns() || tree == nil {
		return Decision{Check: chk, Action: Run, Reason: why}
	}

	fpChk := chk
	if fpChk.Shell == "" {
		fpChk.Shell = runner.DefaultShell()
	}

	inputs := tree.Hashes(selector.Matcher(chk))

	if len(inputs) == 0 {
		e.watchingNothing = append(e.watchingNothing, chk.Name)
	}

	fp := cache.New(fpChk, inputs, e.versions.Versions(chk.Command))

	if hit, _ := e.store.Lookup(fp); hit {
		return Decision{Check: chk, Action: Cached,
			Reason: Reason{Kind: InputsUnchanged}, Fingerprint: &fp}
	}

	return Decision{Check: chk, Action: Run,
		Reason: e.missReason(chk.Name, fp, why), Fingerprint: &fp}
}

func (e *Engine) missReason(name string, fp cache.Fingerprint, fallback Reason) Reason {
	previous, ok := e.store.LastFor(name)

	if !ok {
		return fallback
	}

	if diffs := fp.Diff(previous); len(diffs) > 0 {
		return Reason{Kind: FingerprintChanged, Diffs: diffs}
	}

	return fallback
}

func (e *Engine) scanTree() *cache.Tree {
	tree, err := cache.ScanTree(e.git)

	if err != nil {
		return nil
	}

	return tree
}

type Observer struct {
	OnStart  func(name string)
	OnResult func(runner.Result)
}

func (e *Engine) Execute(ctx context.Context, p Plan, obs Observer) []runner.Result {
	toRun := p.Checks()

	if len(toRun) == 0 {
		return p.Deferred()
	}

	e.snapshot(p)

	results := runner.Run(ctx, toRun, runner.Options{
		Root:     e.opts.Root,
		Jobs:     e.opts.Jobs,
		FailFast: e.opts.FailFast,
		Timeout:  e.opts.Timeout,
		Context:  e.context(),
		OnStart:  obs.OnStart,
		OnResult: obs.OnResult,
	})

	e.record(p, results)
	return append(results, p.Deferred()...)
}

func (e *Engine) context() runner.Context {
	info := e.git.Info()

	return runner.Context{
		Repository: info.Repository,
		Ref:        info.Ref,
		RefName:    info.Branch,
		Head:       info.Head,
		Workspace:  info.Root,
	}
}

func (e *Engine) record(p Plan, results []runner.Result) {
	prints := make(map[string]*cache.Fingerprint, len(p.Decisions))

	for _, d := range p.Decisions {
		if d.Action == Run && d.Fingerprint != nil {
			prints[d.Check.Name] = d.Fingerprint
		}
	}

	for _, res := range results {
		if res.Status != runner.StatusPassed {
			continue
		}
		if fp, ok := prints[res.Name]; ok {
			e.store.Record(res.Name, *fp, res.Duration)
		}
	}
}

func (e *Engine) Stale(p Plan, results []runner.Result) []string {
	if e.before == nil {
		return nil
	}

	tree, err := cache.ScanTree(git.NewSession(e.opts.Root))

	if err != nil {
		return nil
	}

	verified := make(map[string]bool, len(results))

	for _, r := range results {
		if r.Status == runner.StatusPassed || r.Status == runner.StatusCached {
			verified[r.Name] = true
		}
	}

	var stale []string

	for _, d := range p.Decisions {
		before, snapped := e.before[d.Check.Name]
		if !snapped || !verified[d.Check.Name] {
			continue
		}
		if rewritten(before, tree.Hashes(watched(d.Check))) {
			stale = append(stale, d.Check.Name)
		}
	}

	return stale
}

func (e *Engine) snapshot(p Plan) {
	tree := e.scanTree()

	if tree == nil {
		return
	}

	e.before = make(map[string]map[string]string, len(p.Decisions))

	for _, d := range p.Decisions {
		e.before[d.Check.Name] = tree.Hashes(watched(d.Check))
	}
}

func rewritten(before, after map[string]string) bool {
	for path, hash := range before {
		if now, ok := after[path]; !ok || now != hash {
			return true
		}
	}

	return false
}

func watched(chk config.Check) func(string) bool {
	if chk.AlwaysRuns() {
		return func(string) bool { return true }
	}

	return selector.Matcher(chk)
}

func (e *Engine) costs(checks []config.Check) map[string]time.Duration {
	out := make(map[string]time.Duration, len(checks))

	for _, chk := range checks {
		out[chk.Name] = e.store.LastDuration(chk.Name)
	}

	return out
}

func cacheMode(opts Options) cache.Mode {
	if opts.NoCache {
		return cache.Bypass()
	}

	return cache.ReadWrite()
}
