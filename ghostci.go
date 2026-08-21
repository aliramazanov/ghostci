// Package ghostci answers "will my push pass CI?" without pushing.
//
// It reads the CI configuration a repository already has, turns the commands
// CI would run into checks, and runs only the ones the working tree can
// affect. A check whose inputs are unchanged since it last passed is served
// from a content-addressed cache.
//
// The two halves are usable independently. [Import] reads a repository's CI
// configuration and reports both what it extracted and what it refused to
// extract, with a reason for each refusal. [Load], [PlanChecks] and [Run]
// operate on a ghostci.yaml, whether written by hand or produced by Import.
//
//	checks, err := ghostci.Load("ghostci.yaml")
//	if err != nil {
//		return err
//	}
//
//	results, err := ghostci.Run(ctx, "ghostci.yaml", ghostci.Options{Root: "."})
//	if err != nil {
//		return err
//	}
//	if ghostci.Failed(results) {
//		return errors.New("CI would fail")
//	}
//
// Nothing here reports a check as passed unless it ran and passed, or passed
// earlier over identical inputs. A check whose inputs cannot be determined
// runs every time, and only passes are ever cached.
package ghostci

import (
	"context"
	"os"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/engine"
	"github.com/aliramazanov/ghostci/internal/importer"
	"github.com/aliramazanov/ghostci/internal/runner"
)

// Check is one shell command ghostci runs, and the files it depends on.
// A Check with no Inputs cannot be skipped safely, so it always runs.
type Check = config.Check

// Duration is a check timeout. It reads YAML forms such as "5m" and converts
// to and from [time.Duration].
type Duration = config.Duration

// Options controls a run: which repository, how much to run, and how fast.
// Root is required; the rest have usable zero values.
type Options = engine.Options

// Result is what became of one check.
type Result = runner.Result

// Status is the outcome of one check. It distinguishes "did not run" from
// "ran and failed", because reporting the first as the second, or as a pass,
// is how a tool like this lies.
type Status = runner.Status

// The statuses a [Result] can carry. Only StatusFailed, StatusTimedOut and
// StatusStartError count as failures; see [Status.CountsAsFailure].
const (
	// StatusPassed means the check ran and exited zero.
	StatusPassed = runner.StatusPassed

	// StatusFailed means the check ran and exited non-zero.
	StatusFailed = runner.StatusFailed

	// StatusTimedOut means the check exceeded its timeout and was killed.
	StatusTimedOut = runner.StatusTimedOut

	// StatusCancelled means fail-fast or an interrupt stopped the check
	// before it finished. It is not a failure and not a pass.
	StatusCancelled = runner.StatusCancelled

	// StatusSkipped means no changed file matched the check's inputs.
	StatusSkipped = runner.StatusSkipped

	// StatusCached means the check passed earlier over identical inputs.
	StatusCached = runner.StatusCached

	// StatusStartError means the check could not be started at all.
	StatusStartError = runner.StatusStartError

	// StatusUnavailable means the command is missing or its directory does
	// not exist. Nothing was verified, so it is never reported as a pass and
	// never as a failure.
	StatusUnavailable = runner.StatusUnavailable
)

// Load reads a ghostci.yaml and returns the checks it declares.
func Load(path string) ([]Check, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	return cfg.Checks, nil
}

// Detect reports which CI system configures dir, and the path to its
// configuration. The first match wins rather than the best match: a
// repository holding two configurations is running the one its own CI runs.
func Detect(dir string) (provider, path string, found bool) {
	return importer.Detect(dir)
}

// Refusal is one piece of CI configuration that did not become a check, and
// why. Every refusal is reported rather than dropped, because a check that
// silently disappears is indistinguishable from one that passes.
type Refusal struct {
	// Workflow, Job and Step locate the refusal in the CI configuration.
	Workflow string
	Job      string
	Step     string

	// Outcome is the category, such as "needs-review" or "unsupported".
	Outcome string

	// Reason states what could not be resolved, in the words a user reads.
	Reason string
}

// ImportResult is what a repository's CI configuration yielded.
type ImportResult struct {
	// Checks are the commands ghostci can run locally.
	Checks []Check

	// Refused is everything that did not become a check, with a reason.
	Refused []Refusal

	// Warnings are notes about checks that were imported, such as a job CI
	// splits across runners that runs whole here.
	Warnings []string
}

// Import reads the CI configuration in dir and returns the checks it could
// extract along with everything it refused. It does not write anything.
//
// dir is a repository root, and the configuration inside it is found the same
// way [Detect] finds it. A path to a single configuration file also works.
func Import(dir string) (*ImportResult, error) {
	source := dir
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		if _, path, ok := importer.Detect(dir); ok {
			source = path
		}
	}

	res, err := importer.Import(source, importer.DefaultAssumptions())
	if err != nil {
		return nil, err
	}

	out := &ImportResult{Checks: res.Checks, Warnings: res.Warnings}

	for _, e := range res.Entries {
		if e.Outcome == importer.Extracted {
			continue
		}

		out.Refused = append(out.Refused, Refusal{
			Workflow: e.Workflow,
			Job:      e.Job,
			Step:     e.Step,
			Outcome:  e.Outcome.String(),
			Reason:   e.Reason,
		})
	}

	return out, nil
}

// Planned is what ghostci decided to do with one check, and why. The reason
// is the same text `ghostci --explain` prints.
type Planned struct {
	Check Check

	// Action is "run", "skip" or "cached".
	Action string

	// Reason states why, such as which changed file matched.
	Reason string
}

// PlanChecks decides which checks the working tree requires, without running
// any of them. Anything it cannot decide, it decides to run.
func PlanChecks(checks []Check, opts Options) []Planned {
	plan := engine.New(opts).Plan(checks)

	out := make([]Planned, 0, len(plan.Decisions))
	for _, d := range plan.Decisions {
		out = append(out, Planned{
			Check:  d.Check,
			Action: action(d.Action),
			Reason: d.Reason.String(),
		})
	}

	return out
}

func action(a engine.Action) string {
	switch a {
	case engine.Run:
		return "run"
	case engine.Skip:
		return "skip"
	case engine.Cached:
		return "cached"
	}

	return "unknown"
}

// Run plans and executes the checks in configPath, returning one Result per
// check, including the ones that were skipped or served from cache.
//
// Cancelling ctx stops the run; the checks already finished keep their
// results and the rest report [StatusCancelled], which is not a pass.
func Run(ctx context.Context, configPath string, opts Options) ([]Result, error) {
	checks, err := Load(configPath)
	if err != nil {
		return nil, err
	}

	eng := engine.New(opts)

	return eng.Execute(ctx, eng.Plan(checks), nil), nil
}

// Failed reports whether the run should fail the push. A check marked
// optional is reported but ignored, matching continue-on-error in the
// workflow it came from.
func Failed(results []Result) bool { return engine.Failed(results) }
