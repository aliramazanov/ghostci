package importer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/expr"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

const maxCompositeDepth = 3

type site struct {
	workflow string
	job      string
	matrix   string
}

func (s site) with(action string) site {
	s.job += "/" + action
	return s
}

func (s site) entry(step workflow.Step, i int, o Outcome, reason string) Entry {
	return Entry{
		Workflow: s.workflow,
		Job:      s.job,
		Matrix:   s.matrix,
		Step:     stepName(step, i),
		Outcome:  o,
		Reason:   reason,
	}
}

func (s site) checkName(step workflow.Step, i int) string {
	base := s.job
	if s.workflow != "" && !strings.HasPrefix(s.job, s.workflow) {
		base = s.workflow + "/" + s.job
	}
	if s.matrix != "" {
		base += "[" + slug(s.matrix) + "]"
	}
	return base + "/" + slug(stepName(step, i))
}

func (im *importState) expandComposite(ctx expr.Context, step workflow.Step, at site,
	index, depth int, seen map[string]bool) ([]config.Check, []Entry) {

	refuse := func(o Outcome, reason string) ([]config.Check, []Entry) {
		return nil, []Entry{at.entry(step, index, o, reason)}
	}

	if depth >= maxCompositeDepth {
		return refuse(LocalAction,
			fmt.Sprintf("composite nesting deeper than %d, not expanded (%s)", maxCompositeDepth, step.Uses))
	}

	action, err := workflow.LoadAction(im.root, step.Uses)
	if err != nil {
		if errors.Is(err, workflow.ErrNotAnAction) {
			return refuse(LocalAction, "local action not found on disk ("+step.Uses+")")
		}
		return refuse(LocalAction, "could not read local action: "+err.Error())
	}
	if !action.IsComposite() {
		return refuse(UnsupportedAction,
			fmt.Sprintf("local action uses %q, which has no shell steps to run (%s)", action.Runs.Using, step.Uses))
	}
	if seen[action.Path] {
		return refuse(LocalAction, "composite action calls itself, not expanded ("+step.Uses+")")
	}
	seen[action.Path] = true
	defer delete(seen, action.Path)

	inner := compositeContext(ctx, action, step)

	nested := at.with(actionName(step.Uses))
	var checks []config.Check
	var entries []Entry

	for i, sub := range action.Runs.Steps {
		switch {
		case sub.Uses != "" && sub.Type() == workflow.StepLocalAction:
			c, e := im.expandComposite(inner, sub, nested, i, depth+1, seen)
			checks = append(checks, c...)
			entries = append(entries, e...)

		case sub.Uses != "":
			outcome, reason := classifyUses(sub.Uses)
			entries = append(entries, nested.entry(sub, i, outcome,
				fmt.Sprintf("%s (%s)", reason, sub.Uses)))

		case strings.TrimSpace(sub.Run) == "":
			entries = append(entries, nested.entry(sub, i, UnsupportedFeature,
				"composite step has neither run nor uses"))

		default:
			chk, e := im.compositeRunStep(sub, i, nested, inner)
			entries = append(entries, e)
			if e.Outcome == Extracted {
				checks = append(checks, chk)
			}
		}
	}
	return checks, entries
}

func (im *importState) compositeRunStep(sub workflow.Step, i int, at site, ctx expr.Context) (config.Check, Entry) {
	if strings.TrimSpace(sub.If) != "" {
		ok, err := expr.EvalCondition(sub.If, ctx)
		if err != nil {
			return config.Check{}, at.entry(sub, i, NeedsReview, gateReason(sub.If, err))
		}
		if !ok {
			return config.Check{}, at.entry(sub, i, SkippedByCondition,
				"composite step condition is false locally: "+trim(sub.If))
		}
	}

	command, err := expr.Interpolate(sub.Run, ctx)
	if err != nil {
		return config.Check{}, at.entry(sub, i, NeedsReview, interpolateReason(sub.Run, err))
	}

	chk := config.Check{
		Name:    at.checkName(sub, i),
		Command: command,
		Dir:     sub.WorkingDirectory,
		Shell:   normaliseShell(sub.Shell),
		Env:     mergeEnv(ctx, sub.Env),
		Inputs:  inferInputs(command),
	}
	e := at.entry(sub, i, Extracted, "")
	e.Command = command
	return chk, e
}

func compositeContext(caller expr.Context, action *workflow.Action, step workflow.Step) expr.Context {
	inputs := map[string]any{}
	resolve := func(name, raw string) {
		resolved, err := expr.Interpolate(raw, caller)
		if err != nil {
			inputs[name] = expr.Unknown("inputs." + name)
			return
		}
		inputs[name] = resolved
	}
	for name, def := range action.Defaults() {
		resolve(name, def)
	}
	for name, raw := range step.With {
		resolve(name, raw)
	}

	for _, name := range action.Unsatisfied(step.With) {
		inputs[name] = expr.Unknown("inputs." + name)
	}

	inner := make(expr.Context, len(caller)+1)
	for k, v := range caller {
		inner[k] = v
	}
	inner["inputs"] = inputs
	return inner
}

func actionName(uses string) string {
	trimmed := strings.Trim(strings.TrimPrefix(uses, "./"), "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	if trimmed == "" {
		return "action"
	}
	return trimmed
}

func normaliseShell(shell string) string {
	return githubShell(workflow.CompositeShell(shell))
}
