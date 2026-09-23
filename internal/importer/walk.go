package importer

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/expr"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

func (im *importState) importWorkflow(wf *workflow.Workflow, a Assumptions) {
	im.importWorkflowAt(wf, a, 0, "")
}

func (im *importState) importWorkflowAt(wf *workflow.Workflow, a Assumptions, depth int, prefix string) {
	wfName := strings.TrimSuffix(filepath.Base(wf.Path), filepath.Ext(wf.Path))

	if depth == 0 {
		im.paths, im.pathsIgnore = wf.PathFilters(a.events()...)
	}
	im.env = wf.Env
	im.defaults = wf.Defaults
	if prefix != "" {
		wfName = prefix + "/" + wfName
	}

	for _, nj := range wf.Jobs {
		job := nj.Job

		if job.Uses != "" {
			im.expandReusable(wfName, nj, a, depth)

			continue
		}
		if len(job.Services) > 0 {
			im.res.Entries = append(im.res.Entries, Entry{
				Workflow: wfName, Job: nj.ID, Step: "(job)",
				Outcome: UnsupportedFeature,
				Reason:  "needs service containers",
			})
			continue
		}

		combos, truncated, err := workflow.ExpandMatrix(job.Strategy)
		if err != nil {
			im.res.Entries = append(im.res.Entries, Entry{
				Workflow: wfName, Job: nj.ID, Step: "(job)",
				Outcome: NeedsReview,
				Reason:  "matrix is computed at run time and cannot be expanded locally",
			})
			continue
		}
		if truncated {
			im.res.Warnings = append(im.res.Warnings, fmt.Sprintf(
				"%s/%s: matrix truncated to %d combinations", wfName, nj.ID, workflow.MaxCombinations))
		}

		labels := job.RunsOnLabels()
		perLeg := slices.ContainsFunc(labels, func(l string) bool { return strings.Contains(l, "${{") })

		runnerOS := workflow.RunnerOS(labels)
		if !perLeg && runnerOS != a.RunnerOS {
			im.res.Entries = append(im.res.Entries, Entry{
				Workflow: wfName, Job: nj.ID, Step: "(job)",
				Outcome: UnsupportedFeature,
				Reason:  fmt.Sprintf("targets %s, host is %s", runnerOS, a.RunnerOS),
			})
			continue
		}

		for _, combo := range combos {
			legOS := runnerOS
			if perLeg {
				legOS = legRunnerOS(labels, buildContext(a, a.events()[0], a.RunnerOS, combo, wf.Env, job.Env))
			}

			if legOS != a.RunnerOS {
				im.res.Entries = append(im.res.Entries, Entry{
					Workflow: wfName, Job: nj.ID, Matrix: combo.Label(), Step: "(job)",
					Outcome: UnsupportedFeature,
					Reason:  fmt.Sprintf("targets %s, host is %s", legOS, a.RunnerOS),
				})
				continue
			}

			if ctxs := legContexts(wf, job, combo, a, legOS, depth); len(ctxs) > 0 {
				im.importJob(wfName, nj.ID, job, combo, ctxs)
			}
		}
	}
}

func legContexts(wf *workflow.Workflow, job workflow.Job, combo workflow.Combination,
	a Assumptions, runnerOS string, depth int) []expr.Context {

	ctxs := make([]expr.Context, 0, len(a.events()))

	for _, event := range a.events() {
		if depth == 0 && !wf.TriggersOn(event) {
			continue
		}

		ctxs = append(ctxs, buildContext(a, event, runnerOS, combo, wf.Env, job.Env))
	}

	return ctxs
}

func legRunnerOS(labels []string, ctx expr.Context) string {
	resolved := make([]string, 0, len(labels))

	for _, l := range labels {
		if v, err := expr.Interpolate(l, ctx); err == nil {
			l = v
		}

		resolved = append(resolved, l)
	}

	return workflow.RunnerOS(resolved)
}

func jobGateContexts(ctxs []expr.Context) []expr.Context {
	out := make([]expr.Context, 0, len(ctxs))

	for _, ctx := range ctxs {
		clone := make(expr.Context, len(ctx))
		maps.Copy(clone, ctx)
		clone["env"] = expr.Unknown("env, which a job-level if: cannot read")
		out = append(out, clone)
	}

	return out
}

func (im *importState) importJob(wfName, jobID string, job workflow.Job, combo workflow.Combination,
	ctxs []expr.Context) {

	jobCtxs, err := passingContextsVia(job.If, jobGateContexts(ctxs), ctxs)

	if err != nil {
		im.res.Entries = append(im.res.Entries, Entry{
			Workflow: wfName, Job: jobID, Step: "(job if:)", Matrix: combo.Label(),
			Outcome: NeedsReview, Reason: gateReason(job.If, err),
		})
		return
	}

	if len(jobCtxs) == 0 {
		im.res.Entries = append(im.res.Entries, Entry{
			Workflow: wfName, Job: jobID, Step: "(job if:)", Matrix: combo.Label(),
			Outcome: SkippedByCondition,
			Reason:  fmt.Sprintf("job condition is false locally: %s", trim(job.If)),
		})
		return
	}

	ctxs = jobCtxs

	if sharesStateAcrossSteps(job) {
		im.importJobAsOne(wfName, jobID, job, combo, ctxs)

		return
	}

	for i, step := range job.Steps {
		im.importStep(wfName, jobID, job, combo, ctxs, step, i)
	}
}

func (im *importState) importStep(wfName, jobID string, job workflow.Job,
	combo workflow.Combination, ctxs []expr.Context, step workflow.Step, i int) {

	entry := Entry{Workflow: wfName, Job: jobID, Matrix: combo.Label(), Step: stepName(step, i)}

	record := func(outcome Outcome, reason string) {
		entry.Outcome, entry.Reason = outcome, reason
		im.res.Entries = append(im.res.Entries, entry)
	}

	if step.Uses != "" {
		im.importUses(wfName, jobID, combo, ctxs, step, i, entry)

		return
	}

	if strings.TrimSpace(step.Run) == "" {
		record(UnsupportedFeature, "step has neither run nor uses")

		return
	}

	stepCtxs := overlayEnv(ctxs, step.Env)

	passing, err := passingContexts(step.If, stepCtxs)
	if err != nil {
		record(NeedsReview, gateReason(step.If, err))

		return
	}

	if len(passing) == 0 {
		record(SkippedByCondition, fmt.Sprintf("condition is false locally: %s", trim(step.If)))

		return
	}

	command, divergent, err := interpolateAll(step.Run, passing)
	if err != nil {
		record(NeedsReview, interpolateReason(step.Run, err))

		return
	}

	if divergent {
		record(NeedsReview, "command differs by triggering event, so no single local run stands in for CI")

		return
	}

	declared := workingDir(im.defaults, job.Defaults, step)

	dir, divergent, err := interpolateAll(declared, passing)

	if err != nil {
		record(NeedsReview, interpolateReason(declared, err))

		return
	}

	if divergent {
		record(NeedsReview, "working-directory differs by triggering event")

		return
	}

	for _, w := range checkInjection(step.Run, passing[0]) {
		im.res.Warnings = append(im.res.Warnings, fmt.Sprintf("%s/%s: %s", wfName, jobID, w))
	}

	env, dropped := mergeEnv(passing[0], im.env, job.Env, step.Env)

	if name, reads := readsAny(command, dropped); reads {
		record(NeedsReview, "the command reads $"+name+", whose value only CI knows")

		return
	}

	chk := config.Check{
		Name:     checkName(wfName, jobID, step, i, combo),
		Command:  command,
		Dir:      dir,
		Shell:    shellFor(im.defaults, job.Defaults, step),
		Env:      env,
		Inputs:   im.inputsFor(command, dir),
		Exclude:  im.pathsIgnore,
		Optional: step.Optional(),
		Timeout:  config.Duration(stepTimeout(job, step)),
	}

	entry.Outcome, entry.Command = Extracted, command
	entry.Heavy = im.routeCheck(jobID, chk)
	im.res.Entries = append(im.res.Entries, entry)
}

func (im *importState) importUses(wfName, jobID string, combo workflow.Combination,
	ctxs []expr.Context, step workflow.Step, i int, entry Entry) {

	if step.Type() == workflow.StepLocalAction {
		at := site{workflow: wfName, job: jobID, matrix: combo.Label()}

		checks, entries := im.expandComposite(ctxs[0], step, at, i, 0, map[string]bool{})
		im.res.Entries = append(im.res.Entries, entries...)

		for _, c := range checks {
			im.routeCheck(jobID, c)
		}

		return
	}

	outcome, reason := classifyUses(step.Uses)
	entry.Outcome, entry.Reason = outcome, fmt.Sprintf("%s (%s)", reason, step.Uses)

	if outcome == ToolchainAction {
		recordToolchain(step, im.res, im.seenTool)
	}

	im.res.Entries = append(im.res.Entries, entry)
}

func (im *importState) routeCheck(jobID string, chk config.Check) bool {
	cost, why := classifyCost(jobID, chk.Command)

	if cost != CostHeavy {
		im.res.Checks = append(im.res.Checks, chk)
		return false
	}

	if im.res.HeavyReason == nil {
		im.res.HeavyReason = map[string]string{}
	}

	im.res.HeavyReason[chk.Name] = why
	im.res.Heavy = append(im.res.Heavy, chk)

	return true
}

func stepTimeout(job workflow.Job, step workflow.Step) time.Duration {
	if d := step.Timeout(); d > 0 {
		return d
	}

	return job.Timeout()
}

var installerPattern = regexp.MustCompile(
	`(?m)(^|[;&|]\s*|\s)(` +

		`go install|go get|cargo install|npm i(nstall)? -g|npm i -g|` +
		`pipx install|gem install|apt-get install|brew install|` +
		`curl [^|]*\| *(ba)?sh|` +

		`npm ci|npm i(nstall)?( |$)|yarn( install)?( |$)|pnpm i(nstall)?( |$)|` +
		`bun install|pip3? install|poetry install|pipenv install|uv sync|uv pip install|` +
		`bundle install|composer install|go mod download|dotnet restore|mix deps.get` +
		`)`)

var exportPattern = regexp.MustCompile(`\$\{?GITHUB_(ENV|PATH)\}?`)

func sharesStateAcrossSteps(job workflow.Job) bool {
	for _, step := range job.Steps {
		if step.Run == "" {
			continue
		}

		if installerPattern.MatchString(step.Run) || exportsState(step.Run) {
			return true
		}
	}

	return false
}

func exportsState(run string) bool {
	for _, line := range strings.Split(run, "\n") {
		if !exportPattern.MatchString(line) {
			continue
		}
		if strings.Contains(line, ">>") || strings.Contains(line, ">") {
			return true
		}
	}

	return false
}

func mergeCommands(commands, dirs []string) (script, dir string) {
	same := true
	for _, d := range dirs {
		if d != dirs[0] {
			same = false

			break
		}
	}

	if same {
		return strings.Join(commands, "\n"), dirs[0]
	}

	parts := make([]string, 0, len(commands))
	for i, cmd := range commands {
		if dirs[i] == "" {
			parts = append(parts, cmd)

			continue
		}

		parts = append(parts, "(\ncd "+shellQuote(dirs[i])+"\n"+cmd+"\n)")
	}

	return strings.Join(parts, "\n"), ""
}

func shellQuote(path string) string {
	if path != "" && !strings.ContainsAny(path, " \t\n'\"\\$`&|;<>()*?[]{}!#~") {
		return path
	}

	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func optionalRunSteps(job workflow.Job) (all, mixed bool) {
	var opt, req int

	for _, step := range job.Steps {
		if step.Run == "" {
			continue
		}

		if step.Optional() {
			opt++
		} else {
			req++
		}
	}

	return opt > 0 && req == 0, opt > 0 && req > 0
}

func (im *importState) importJobAsOne(wfName, jobID string, job workflow.Job,
	combo workflow.Combination, ctxs []expr.Context) {

	entry := Entry{
		Workflow: wfName, Job: jobID, Matrix: combo.Label(),
		Step: "(job, merged: a step installs a tool the others need)",
	}

	var (
		commands []string
		dirs     []string
	)

	optional, mixed := optionalRunSteps(job)
	if mixed {
		im.deferJob(entry, "some steps in this job set continue-on-error and others do not, "+
			"so merging them would change which failures count")

		return
	}

	for i, step := range job.Steps {
		if step.Run == "" {

			if step.Uses != "" {
				outcome, reason := classifyUses(step.Uses)
				if outcome == ToolchainAction {
					recordToolchain(step, im.res, im.seenTool)
				}
				im.res.Entries = append(im.res.Entries, Entry{
					Workflow: wfName, Job: jobID, Matrix: combo.Label(),
					Step:    stepName(step, i),
					Outcome: outcome, Reason: fmt.Sprintf("%s (%s)", reason, step.Uses),
				})
			}

			continue
		}

		stepCtxs := overlayEnv(ctxs, step.Env)

		passing, err := passingContexts(step.If, stepCtxs)

		if err != nil {
			im.deferJob(entry, gateReason(step.If, err))

			return
		}

		if len(passing) == 0 {
			continue
		}

		command, divergent, err := interpolateAll(step.Run, passing)

		if err != nil {
			im.deferJob(entry, interpolateReason(step.Run, err))

			return
		}

		if divergent {
			im.deferJob(entry, "a command in this job differs by triggering event")

			return
		}

		stepDir := workingDir(im.defaults, job.Defaults, step)

		if stepDir != "" {
			resolved, divergent, err := interpolateAll(stepDir, passing)

			if err != nil {
				im.deferJob(entry, interpolateReason(stepDir, err))

				return
			}

			if divergent {
				im.deferJob(entry, "working-directory differs by triggering event")

				return
			}

			stepDir = resolved
		}

		dirs = append(dirs, stepDir)
		commands = append(commands, command)
	}

	if len(commands) == 0 {
		return
	}

	command, dir := mergeCommands(commands, dirs)

	env, dropped := mergeEnv(ctxs[0], im.env, job.Env, nil)
	if name, reads := readsAny(command, dropped); reads {
		im.deferJob(entry, "the command reads $"+name+", whose value only CI knows")

		return
	}

	chk := config.Check{
		Name:     mergedName(wfName, jobID, combo),
		Command:  command,
		Dir:      dir,
		Shell:    shellFor(im.defaults, job.Defaults, workflow.Step{}),
		Env:      env,
		Inputs:   im.mergedInputsFor(command, dir, dirs),
		Optional: optional,
		Exclude:  im.pathsIgnore,
		Timeout:  config.Duration(job.Timeout()),

		Serial: true,
	}

	entry.Outcome, entry.Command = Extracted, command
	entry.Heavy = im.routeCheck(jobID, chk)
	im.res.Entries = append(im.res.Entries, entry)
}

func (im *importState) mergedInputsFor(command, dir string, stepDirs []string) []string {
	if dir != "" || len(im.paths) > 0 {
		return im.inputsFor(command, dir)
	}

	return scoped(im.inputsFor(command, dir), stepDirs...)
}

func (im *importState) deferJob(entry Entry, reason string) {
	entry.Outcome, entry.Reason = NeedsReview, reason
	im.res.Entries = append(im.res.Entries, entry)
}

func mergedName(wfName, jobID string, combo workflow.Combination) string {
	name := slug(wfName + "/" + jobID)
	if label := combo.Label(); label != "" {
		name += " [" + label + "]"
	}

	return name
}
