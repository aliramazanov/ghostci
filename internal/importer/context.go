package importer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aliramazanov/ghostci/internal/expr"
	"github.com/aliramazanov/ghostci/internal/pipeline"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

func passingContexts(cond string, ctxs []expr.Context) ([]expr.Context, error) {
	return passingContextsVia(cond, ctxs, ctxs)
}

func passingContextsVia(cond string, gate, ctxs []expr.Context) ([]expr.Context, error) {
	if strings.TrimSpace(cond) == "" {
		return ctxs, nil
	}

	_, passing, err := pipeline.Decide(indexes(gate), func(i int) (bool, error) {
		return expr.EvalCondition(cond, gate[i])
	})
	if err != nil {
		return nil, err
	}

	out := make([]expr.Context, 0, len(passing))
	for _, i := range passing {
		out = append(out, ctxs[i])
	}

	return out, nil
}

func indexes[T any](items []T) []int {
	out := make([]int, len(items))
	for i := range items {
		out[i] = i
	}

	return out
}

func interpolateAll(src string, ctxs []expr.Context) (value string, divergent bool, err error) {
	set := false

	for _, ctx := range ctxs {
		v, err := expr.Interpolate(src, ctx)
		if err != nil {
			return "", false, err
		}

		if !set {
			value, set = v, true
		} else if v != value {
			return "", true, nil
		}
	}

	return value, false, nil
}

func layerEnv(ctx expr.Context, extra workflow.Env) expr.Context {
	if len(extra) == 0 {
		return ctx
	}

	env := map[string]any{}
	if base, ok := ctx["env"].(expr.Strict); ok {
		for k, v := range base.Values {
			env[k] = v
		}
	}

	for k, raw := range extra {
		if resolved, err := expr.Interpolate(raw, ctx); err == nil {
			env[k] = resolved
		} else {
			env[k] = expr.Unknown("env." + k)
		}
	}

	next := make(expr.Context, len(ctx))
	for k, v := range ctx {
		next[k] = v
	}
	next["env"] = expr.Strict{Name: "env", Values: env}

	return next
}

func overlayEnv(ctxs []expr.Context, extra workflow.Env) []expr.Context {
	if len(extra) == 0 {
		return ctxs
	}

	out := make([]expr.Context, len(ctxs))
	for i, ctx := range ctxs {
		out[i] = layerEnv(ctx, extra)
	}

	return out
}

func gateReason(cond string, err error) string {
	return pipeline.GateReason("condition", cond, err)
}

func interpolateReason(run string, err error) string {
	var ue *expr.UndecidableError
	if errors.As(err, &ue) {
		return fmt.Sprintf("command %s", ue.Error())
	}
	refs := expr.References(run)
	if len(refs) > 0 {
		return fmt.Sprintf("command uses ${{ }} referencing %s: %v", strings.Join(refs, ", "), err)
	}
	return fmt.Sprintf("command expression could not be resolved: %v", err)
}

const prMergeRef = "refs/pull/0/merge"

func owner(repository string) string {
	name, _, ok := strings.Cut(repository, "/")
	if !ok {
		return ""
	}

	return name
}

func refType(ref string) string {
	switch {
	case strings.HasPrefix(ref, "refs/tags/"):
		return "tag"
	case strings.HasPrefix(ref, "refs/heads/"):
		return "branch"
	}

	return ""
}

func shortRef(ref string) string {
	return strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
}

func buildContext(a Assumptions, event, runnerOS string, combo workflow.Combination, envs ...workflow.Env) expr.Context {

	gh := map[string]any{
		"ref":                 a.Ref,
		"ref_name":            shortRef(a.Ref),
		"ref_type":            refType(a.Ref),
		"ref_protected":       expr.Unknown("github.ref_protected"),
		"event_name":          event,
		"repository":          a.Repository,
		"repository_owner":    owner(a.Repository),
		"repository_id":       expr.Unknown("github.repository_id"),
		"repository_owner_id": expr.Unknown("github.repository_owner_id"),
		"workflow":            expr.Unknown("github.workflow"),
		"workflow_ref":        expr.Unknown("github.workflow_ref"),
		"job":                 expr.Unknown("github.job"),
		"run_id":              expr.Unknown("github.run_id"),
		"run_number":          expr.Unknown("github.run_number"),
		"run_attempt":         expr.Unknown("github.run_attempt"),
		"triggering_actor":    expr.Unknown("github.triggering_actor"),
		"token":               expr.Unknown("github.token"),
		"server_url":          "https://github.com",
		"api_url":             "https://api.github.com",
		"graphql_url":         "https://api.github.com/graphql",
		"workspace":           ".",
		"event":               map[string]any{},
		"actor":               "local",
		"head_ref":            "",
		"base_ref":            "",
	}

	if event == "pull_request" {
		gh["ref"] = prMergeRef
		gh["ref_name"] = strings.TrimPrefix(prMergeRef, "refs/pull/")
		gh["head_ref"] = shortRef(a.Ref)
		gh["base_ref"] = expr.Unknown("github.base_ref")
	}

	ctx := expr.Context{
		"github":   gh,
		"runner":   map[string]any{"os": runnerOS, "arch": "X64", "temp": "/tmp"},
		"matrix":   combo.AsContext(),
		"env":      expr.Strict{Name: "env", Values: map[string]any{}},
		"job":      map[string]any{"status": "success"},
		"strategy": map[string]any{},
	}

	if a.callInputs != nil {
		ctx["inputs"] = a.callInputs
	}

	for _, e := range envs {
		ctx = layerEnv(ctx, e)
	}

	return ctx
}
