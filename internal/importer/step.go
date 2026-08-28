package importer

import (
	"sort"
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

func mergeEnv(ctx expr.Context, layers ...workflow.Env) (env map[string]string, dropped []string) {
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
			dropped = append(dropped, k)

			continue
		}

		out[k] = resolved
	}

	sort.Strings(dropped)

	if len(out) == 0 {
		return nil, dropped
	}

	return out, dropped
}

func readsAny(script string, names []string) (string, bool) {
	for _, name := range names {
		for _, form := range []string{"${" + name + "}", "$" + name} {
			i := strings.Index(script, form)
			if i < 0 {
				continue
			}

			if after := i + len(form); form[len(form)-1] != '}' &&
				after < len(script) && isNameByte(script[after]) {
				continue
			}

			return name, true
		}
	}

	return "", false
}

func isNameByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
