package importer

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/gitlab"
	"github.com/aliramazanov/ghostci/internal/pipeline"
)

const GitLabFile = ".gitlab-ci.yml"

const gitlabShell = "bash -eo pipefail"

var pipelineSources = []string{"push", "merge_request_event"}

type pendingJob struct {
	file  *gitlab.File
	label string
	name  string
	job   gitlab.Job
}

type gitlabImport struct {
	root string
	res  *Result
	a    Assumptions

	scope   gitlab.Scope
	pending []pendingJob

	defaultBranch string

	seenFile map[string]bool
}

func ImportGitLab(path string, a Assumptions) (*Result, error) {
	file, err := gitlab.Load(path)
	if err != nil {
		return nil, err
	}

	im := &gitlabImport{
		root:          repoRootFor(path),
		res:           &Result{GitLab: true},
		a:             a,
		defaultBranch: a.DefaultBranch,
		seenFile:      map[string]bool{},
	}
	im.seenFile[filepath.Clean(path)] = true

	im.importFile(file, filepath.Base(path))
	dedupeNames(im.res)

	return im.res, nil
}

func repoRootFor(path string) string {
	abs, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "."
	}

	return abs
}

func (im *gitlabImport) importFile(file *gitlab.File, label string) {
	im.scope = gitlab.Scope{}
	im.pending = nil

	im.collect(file, label)

	sources := im.pipelineSources(file, label)
	if len(sources) == 0 {
		im.res.NotTriggered = append(im.res.NotTriggered,
			label+" (its workflow: rules exclude every pipeline ghostci reasons about)")

		return
	}

	for _, p := range im.pending {
		job, err := im.scope.Resolve(p.name, p.job)
		if err != nil {
			im.entry(p.label, p.name, NeedsReview, err.Error())

			continue
		}

		im.importJob(p.file, p.label, p.name, job, sources)
	}
}

func (im *gitlabImport) collect(file *gitlab.File, label string) {
	im.followIncludes(file, label)

	im.scope.Files = append(im.scope.Files, file)

	for _, nj := range file.Jobs {
		im.pending = append(im.pending, pendingJob{
			file: file, label: label, name: nj.Name, job: nj.Job,
		})
	}
}

func (im *gitlabImport) followIncludes(file *gitlab.File, label string) {
	for _, inc := range file.Includes {
		if !inc.IsLocal() {
			im.res.Entries = append(im.res.Entries, Entry{
				Workflow: label, Job: "(include)", Step: inc.Describe(),
				Outcome: UnsupportedFeature,
				Reason:  "include is fetched by GitLab, so its jobs are not visible here (" + inc.Describe() + ")",
			})

			continue
		}

		path := filepath.Join(im.root, filepath.FromSlash(strings.TrimPrefix(inc.Local, "/")))
		if im.seenFile[filepath.Clean(path)] {
			continue
		}
		im.seenFile[filepath.Clean(path)] = true

		included, err := gitlab.Load(path)
		if err != nil {
			im.res.Entries = append(im.res.Entries, Entry{
				Workflow: label, Job: "(include)", Step: inc.Local,
				Outcome: UnsupportedFeature, Reason: "included file not readable: " + err.Error(),
			})

			continue
		}

		im.collect(included, filepath.Base(inc.Local))
	}
}

func (im *gitlabImport) pipelineSources(file *gitlab.File, label string) []string {
	if file.Workflow == nil || len(file.Workflow.Rules) == 0 {
		return pipelineSources
	}

	var out []string

	for _, source := range pipelineSources {
		vars := im.vars(file, gitlab.Job{}, source)

		match, _, err := matchRules(file.Workflow.Rules, vars)
		switch {
		case err != nil:

			out = append(out, source)
		case match == nil:
			continue
		case runsAutomatically(match.When):
			out = append(out, source)
		}
	}

	return out
}

func (im *gitlabImport) importJob(file *gitlab.File, label, name string, job gitlab.Job, sources []string) {
	entry := Entry{Workflow: label, Job: name, Step: "script"}

	if len(job.Script) == 0 {
		if !job.Trigger.IsZero() {
			im.refuse(label, name, pipeline.OpaqueUnit, "triggers a child pipeline, which ghostci does not expand")

			return
		}
		im.refuse(label, name, pipeline.Unreadable, "declares no script")

		return
	}
	if !job.Only.IsZero() || !job.Except.IsZero() {
		im.entry(label, name, NeedsReview,
			"uses the deprecated only/except keywords, which ghostci does not evaluate")

		return
	}
	if !job.Services.IsZero() {
		im.refuse(label, name, pipeline.NeedsContainer, "needs service containers")

		return
	}
	if !strings.EqualFold(job.When, "") && !runsAutomatically(job.When) {
		im.refuse(label, name, pipeline.NotAutomatic, "when: "+job.When+" does not run automatically")

		return
	}

	combos, err := parallelMatrix(job)
	if err != nil {
		im.entry(label, name, NeedsReview, err.Error())

		return
	}

	for _, combo := range combos {
		im.importLeg(file, label, name, job, sources, combo, entry)
	}
}

type gate struct {
	runs   bool
	inputs []string

	undecided error
	skipped   string
	answered  bool
}

func (im *gitlabImport) evaluateRules(file *gitlab.File, job gitlab.Job,
	sources []string, combo gitlab.Variables) gate {

	var g gate

	verdict, _, err := pipeline.Decide(sources, func(source string) (bool, error) {
		vars := im.vars(file, job, source)
		for k, v := range combo {
			vars[k] = pipeline.Known(v)
		}

		rule, ruleInputs, err := matchRules(job.Rules, vars)
		if err != nil {
			return false, err
		}

		g.answered = true

		if rule == nil && len(job.Rules) > 0 {
			g.skipped = "no rule matches locally, so GitLab would not add the job"

			return false, nil
		}

		if rule != nil {
			if !runsAutomatically(rule.When) {
				g.skipped = "the matching rule sets when: " + rule.When

				return false, nil
			}

			if rule.Changes.CompareTo != "" {
				ruleInputs = nil
			}
		}

		g.inputs = append(g.inputs, ruleInputs...)

		return true, nil
	})

	g.runs = verdict == pipeline.Runs
	g.undecided = err

	return g
}

func (im *gitlabImport) importLeg(file *gitlab.File, label, name string, job gitlab.Job,
	sources []string, combo gitlab.Variables, entry Entry) {

	entry.Matrix = comboLabel(combo)

	if name, ok := unsetInput(ruleText(job.Rules)); ok {
		im.entryAt(entry, NeedsReview,
			"a rule tests the input "+name+", which the pipeline declares without a default, "+
				"so its value comes from whoever starts the pipeline")

		return
	}

	g := im.evaluateRules(file, job, sources, combo)

	if !g.runs {
		switch {
		case g.undecided != nil:
			im.entryAt(entry, NeedsReview, ruleReason(ruleText(job.Rules), g.undecided))
		case g.skipped != "":
			im.entryAt(entry, SkippedByCondition, g.skipped)
		case g.answered:
			im.entryAt(entry, SkippedByCondition, "no rule matches locally")
		default:
			im.entryAt(entry, NeedsReview, "no pipeline ghostci reasons about would run this job")
		}

		return
	}

	inputs := g.inputs

	script := append(append([]string{}, job.BeforeScript...), job.Script...)

	for _, line := range script {
		if name, ok := unsetInput(line); ok {
			im.entryAt(entry, NeedsReview,
				"reads the input "+name+", which the pipeline declares without a default, "+
					"so its value comes from whoever starts the pipeline")

			return
		}

		if strings.Contains(line, gitlab.Unresolved) {
			im.entryAt(entry, NeedsReview,
				"uses !reference ["+strings.TrimPrefix(strings.TrimSpace(line), gitlab.Unresolved)+
					"], which is defined outside this repository")

			return
		}
	}

	command := strings.Join(script, "\n")

	env, unresolved := im.scriptEnv(file, job, combo)
	if len(unresolved) > 0 {
		im.entryAt(entry, NeedsReview,
			"the script reads "+strings.Join(unresolved, ", ")+", which only GitLab sets")

		return
	}

	chk := config.Check{
		Name:     gitlabCheckName(name, combo),
		Command:  command,
		Shell:    gitlabShell,
		Env:      env,
		Inputs:   im.inputsFor(inputs, command),
		Optional: allowsFailure(job),
		Timeout:  config.Duration(gitlabTimeout(job.Timeout)),
	}

	entry.Command = command
	entry.Outcome = Extracted
	entry.Heavy = im.route(name, chk)

	im.res.Entries = append(im.res.Entries, entry)

	if len(job.AfterScript) > 0 {
		im.refuse(label, name, pipeline.Unreadable,
			"after_script runs in its own shell and is not imported")
	}

	im.recordImage(job)
}

func (im *gitlabImport) inputsFor(declared []string, command string) []string {
	if len(declared) == 0 {
		return inferInputsIn(im.root, "", command)
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(declared))

	for _, g := range declared {
		if seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, g)
	}
	sort.Strings(out)

	return out
}

func (im *gitlabImport) route(job string, chk config.Check) bool {
	cost, why := classifyCost(job, chk.Command)
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

func (im *gitlabImport) refuse(workflow, job string, r pipeline.Refusal, reason string) {
	im.entry(workflow, job, refusalOutcome(r), reason)
}

func (im *gitlabImport) entry(workflow, job string, outcome Outcome, reason string) {
	im.res.Entries = append(im.res.Entries, Entry{
		Workflow: workflow, Job: job, Step: "script",
		Outcome: outcome, Reason: reason,
	})
}

func (im *gitlabImport) entryAt(entry Entry, outcome Outcome, reason string) {
	entry.Outcome, entry.Reason = outcome, reason
	im.res.Entries = append(im.res.Entries, entry)
}

func matchRules(rules []gitlab.Rule, vars gitlab.Vars) (*gitlab.Rule, []string, error) {
	for i := range rules {
		rule := rules[i]

		if rule.If != "" {
			ok, err := gitlab.EvalRule(rule.If, vars)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				continue
			}
		}

		if len(rule.Exists) > 0 && !anyExists(rule.Exists) {
			continue
		}

		return &rule, rule.Changes.Paths, nil
	}

	return nil, nil, nil
}

func anyExists(globs []string) bool {
	for _, g := range globs {
		matches, err := filepath.Glob(g)
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}

func runsAutomatically(when string) bool {
	switch strings.ToLower(strings.TrimSpace(when)) {
	case "", "on_success", "always", "delayed":
		return true
	}

	return false
}

func allowsFailure(job gitlab.Job) bool {
	return !job.AllowFailure.IsZero() && job.AllowFailure.Value == "true"
}

func gitlabTimeout(text string) time.Duration {
	fields := strings.Fields(strings.ToLower(strings.ReplaceAll(text, ",", " ")))

	var total time.Duration

	for i := 0; i < len(fields); i++ {
		digits := strings.TrimRight(fields[i], "abcdefghijklmnopqrstuvwxyz")
		if digits == "" {
			continue
		}

		n, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}

		unit := strings.TrimPrefix(fields[i], digits)
		if unit == "" && i+1 < len(fields) {
			i++
			unit = fields[i]
		}

		switch {
		case strings.HasPrefix(unit, "h"):
			total += time.Duration(n) * time.Hour
		case strings.HasPrefix(unit, "m"):
			total += time.Duration(n) * time.Minute
		case strings.HasPrefix(unit, "s"):
			total += time.Duration(n) * time.Second
		}
	}

	return total
}

func (im *gitlabImport) recordImage(job gitlab.Job) {
	name := imageName(job.Image)
	if name == "" {
		return
	}

	tool, version := imageToolchain(name)
	if tool == "" {
		return
	}

	for _, t := range im.res.Toolchains {
		if t.Name == tool {
			return
		}
	}

	im.res.Toolchains = append(im.res.Toolchains,
		Toolchain{Name: tool, Version: version, Source: "image: " + name})
}

func ruleReason(cond string, err error) string {
	return pipeline.GateReason("rule", cond, err)
}

func ruleText(rules []gitlab.Rule) string {
	for _, r := range rules {
		if r.If != "" {
			return r.If
		}
	}

	return "rules"
}

func gitlabCheckName(job string, combo gitlab.Variables) string {
	if len(combo) == 0 {
		return job
	}

	return job + " [" + comboLabel(combo) + "]"
}

func comboLabel(combo gitlab.Variables) string {
	if len(combo) == 0 {
		return ""
	}

	keys := make([]string, 0, len(combo))
	for k := range combo {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, combo[k])
	}

	return strings.Join(parts, ", ")
}

func projectName(repository string) string {
	_, name, ok := strings.Cut(repository, "/")
	if !ok {
		return repository
	}

	return name
}
