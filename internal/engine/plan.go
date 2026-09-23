package engine

import (
	"sort"
	"time"

	"github.com/aliramazanov/ghostci/internal/cache"
	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/runner"
	"github.com/aliramazanov/ghostci/internal/selector"
)

type Action int

const (
	Run Action = iota
	Skip
	Cached
)

type Decision struct {
	Check  config.Check
	Action Action
	Reason Reason

	Fingerprint *cache.Fingerprint
}

type Plan struct {
	Decisions []Decision
	Changes   selector.Changes

	cost map[string]time.Duration
}

func (p Plan) Checks() []config.Check {
	out := make([]config.Check, 0, len(p.Decisions))

	for _, d := range p.Decisions {
		if d.Action == Run {
			out = append(out, d.Check)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return p.cost[out[i].Name] < p.cost[out[j].Name]
	})

	return out
}

func (p Plan) Deferred() []runner.Result {
	out := make([]runner.Result, 0, len(p.Decisions))
	for _, d := range p.Decisions {
		switch d.Action {
		case Skip:
			out = append(out, runner.Result{Name: d.Check.Name, Status: runner.StatusSkipped, Reason: d.Reason.String(), ExitCode: -1})
		case Cached:
			out = append(out, runner.Result{Name: d.Check.Name, Status: runner.StatusCached, Reason: d.Reason.String(), ExitCode: -1})
		}
	}

	return out
}

func (p Plan) Count(a Action) int {
	n := 0

	for _, d := range p.Decisions {
		if d.Action == a {
			n++
		}
	}

	return n
}

func (e *Engine) Plan(checks []config.Check) Plan {
	if e.opts.All {
		return e.planAll(checks)
	}

	changes := selector.DetectChanges(e.git, e.opts.Since)
	plan := Plan{Changes: changes, Decisions: make([]Decision, 0, len(checks))}

	tree := e.scanTree()

	for _, d := range selector.Select(checks, changes) {
		plan.Decisions = append(plan.Decisions, e.decide(d, changes, tree))
	}

	plan.cost = e.costs(plan.Decisions)

	return plan
}

func (e *Engine) decide(d selector.Decision, changes selector.Changes, tree *cache.Tree) Decision {
	switch {

	case !changes.Complete:
		return e.cacheDecision(d.Check,
			Reason{Kind: IncompleteChanges, Detail: changes.Reason}, tree)

	case d.Unknown:
		return e.cacheDecision(d.Check, Reason{Kind: UnknownInputs}, tree)

	case !d.Run && tree != nil && !tree.Any(selector.Matcher(d.Check)):
		return e.watchesNothing(d.Check)

	case !d.Run:
		return Decision{Check: d.Check, Action: Skip,
			Reason: Reason{Kind: NoMatch, Globs: d.Globs, Excluded: d.Excluded}}
	}

	return e.cacheDecision(d.Check, Reason{Kind: ChangedFile, File: d.Matched}, tree)
}

func Failed(results []runner.Result) bool {
	for _, r := range results {
		if r.Status.CountsAsFailure() && !r.Optional {
			return true
		}
	}

	return false
}

func Unavailable(results []runner.Result) []string {
	var out []string

	for _, r := range results {
		if r.Status == runner.StatusUnavailable {
			out = append(out, r.Name)
		}
	}

	return out
}

func Cancelled(results []runner.Result) bool {
	for _, r := range results {
		if r.Status == runner.StatusCancelled {
			return true
		}
	}

	return false
}
