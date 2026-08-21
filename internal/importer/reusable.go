package importer

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/expr"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

const maxWorkflowDepth = 3

func (im *importState) expandReusable(wfName string, nj workflow.NamedJob, a Assumptions, depth int) bool {
	job := nj.Job

	report := func(o Outcome, reason string) bool {
		im.res.Entries = append(im.res.Entries, Entry{
			Workflow: wfName, Job: nj.ID, Step: "(reusable workflow)",
			Outcome: o, Reason: reason,
		})

		return true
	}

	if !strings.HasPrefix(job.Uses, "./") {
		return report(UnsupportedFeature,
			"calls a remote reusable workflow, which is not fetched ("+job.Uses+")")
	}
	if depth >= maxWorkflowDepth {
		return report(UnsupportedFeature,
			fmt.Sprintf("reusable workflows nested deeper than %d (%s)", maxWorkflowDepth, job.Uses))
	}

	ctxs := make([]expr.Context, 0, len(a.events()))
	for _, event := range a.events() {
		ctxs = append(ctxs, buildContext(a, event, a.RunnerOS, nil, im.env))
	}

	passing, err := passingContexts(job.If, ctxs)
	if err != nil {
		return report(NeedsReview, gateReason(job.If, err))
	}
	if len(passing) == 0 {
		return report(SkippedByCondition,
			"job condition is false locally: "+trim(job.If))
	}

	if job.Strategy != nil && !job.Strategy.Matrix.IsZero() {
		return report(NeedsReview,
			"a matrix over a reusable workflow call is not expanded; review "+job.Uses+" by hand")
	}

	path := filepath.Join(im.root, filepath.FromSlash(strings.TrimPrefix(job.Uses, "./")))

	called, err := workflow.Load(path)
	if err != nil {
		return report(UnsupportedFeature, "reusable workflow not readable: "+err.Error())
	}
	if !called.IsReusable() {
		return report(UnsupportedFeature,
			job.Uses+" is not callable: it has no workflow_call trigger")
	}
	if im.seenWorkflow[called.Path] {
		return report(UnsupportedFeature, "reusable workflow calls itself ("+job.Uses+")")
	}

	im.seenWorkflow[called.Path] = true
	defer delete(im.seenWorkflow, called.Path)

	savedEnv, savedDefaults := im.env, im.defaults
	savedPaths, savedIgnore := im.paths, im.pathsIgnore
	defer func() {
		im.env, im.defaults = savedEnv, savedDefaults
		im.paths, im.pathsIgnore = savedPaths, savedIgnore
	}()

	inner := a
	inner.callInputs = callInputs(called, job, passing)

	im.importWorkflowAt(called, inner, depth+1, wfName+"/"+nj.ID)

	return true
}

func callInputs(called *workflow.Workflow, job workflow.Job, ctxs []expr.Context) map[string]any {
	inputs := map[string]any{}

	resolve := func(name, raw string) {
		value, divergent, err := interpolateAll(raw, ctxs)
		if err != nil || divergent {
			inputs[name] = expr.Unknown("inputs." + name)

			return
		}

		inputs[name] = value
	}

	for name, in := range called.CallInputs {
		resolve(name, in.Default)
	}

	for name, raw := range job.With {
		resolve(name, raw)
	}

	return inputs
}
