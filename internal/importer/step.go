package importer

import (
	"strings"

	"github.com/aliramazanov/ghostci/internal/expr"

	"github.com/aliramazanov/ghostci/internal/workflow"
)

func recordToolchain(step workflow.Step, res *Result, seen map[string]bool) {
	version := toolchainVersion(step.Uses, step.With)
	if version == "" || strings.Contains(version, "${{") {
		return
	}
	name := toolchainName(step.Uses)
	key := name + "@" + version
	if seen[key] {
		return
	}
	seen[key] = true
	res.Toolchains = append(res.Toolchains, Toolchain{Name: name, Version: version, Source: step.Uses})
}

func defaultsRun(layers ...*workflow.Defaults) []*workflow.DefaultsRun {
	out := make([]*workflow.DefaultsRun, 0, len(layers))

	for _, d := range layers {
		if d != nil && d.Run != nil {
			out = append(out, d.Run)
		}
	}

	return out
}

func workingDir(wf, job *workflow.Defaults, step workflow.Step) string {
	if step.WorkingDirectory != "" {
		return step.WorkingDirectory
	}

	for _, run := range defaultsRun(job, wf) {
		if run.WorkingDirectory != "" {
			return run.WorkingDirectory
		}
	}

	return ""
}

func shellFor(wf, job *workflow.Defaults, step workflow.Step) string {
	shell := step.Shell

	for _, run := range defaultsRun(job, wf) {
		if shell != "" {
			break
		}

		shell = run.Shell
	}

	return githubShell(shell)
}

func githubShell(shell string) string {
	switch shell {
	case "":
		return ""
	case "bash":
		return "bash --noprofile --norc -eo pipefail"
	case "sh":
		return "sh -e"
	}

	return shell
}

func mergeEnv(ctx expr.Context, layers ...workflow.Env) map[string]string {
	out := map[string]string{}

	for _, layer := range layers {
		for k, v := range layer {
			out[k] = v
		}
	}

	for k, v := range out {
		if !strings.Contains(v, "${{") {
			continue
		}

		resolved, err := expr.Interpolate(v, ctx)
		if err != nil {
			delete(out, k)

			continue
		}

		out[k] = resolved
	}

	if len(out) == 0 {
		return nil
	}

	return out
}
