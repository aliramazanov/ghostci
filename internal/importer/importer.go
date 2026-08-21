package importer

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aliramazanov/ghostci/internal/git"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

type importState struct {
	paths       []string
	pathsIgnore []string

	env workflow.Env

	defaults *workflow.Defaults

	root         string
	res          *Result
	seenTool     map[string]bool
	seenWorkflow map[string]bool
}

func repoRoot(workflowDir string) string {
	abs, err := filepath.Abs(workflowDir)
	if err != nil {
		return "."
	}
	if filepath.Base(abs) == "workflows" && filepath.Base(filepath.Dir(abs)) == ".github" {
		return filepath.Dir(filepath.Dir(abs))
	}

	if root, err := git.Root(workflowDir); err == nil && root != "" {
		return root
	}

	return filepath.Dir(filepath.Dir(abs))
}

func ImportDir(dir string, a Assumptions) (*Result, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return nil, fmt.Errorf("importer: listing workflows in %s: %w", dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("importer: no workflow files in %s", dir)
	}
	sort.Strings(paths)

	im := &importState{root: repoRoot(dir), res: &Result{}, seenTool: map[string]bool{}, seenWorkflow: map[string]bool{}}
	for _, p := range paths {
		wf, err := workflow.Load(p)
		if err != nil {
			im.res.Warnings = append(im.res.Warnings, err.Error())
			continue
		}
		if !wf.TriggersOn("push", "pull_request") {
			im.res.NotTriggered = append(im.res.NotTriggered,
				fmt.Sprintf("%s (runs on: %s)", filepath.Base(p), triggerList(wf)))
			continue
		}
		if !wf.TriggersOn("pull_request") && !wf.PushMatchesRef(a.Ref) {
			im.res.NotTriggered = append(im.res.NotTriggered,
				fmt.Sprintf("%s (its push filter does not match %s)", filepath.Base(p), a.Ref))
			continue
		}
		im.importWorkflow(wf, a)
	}
	dedupeNames(im.res)
	return im.res, nil
}

func ImportFile(path string, a Assumptions) (*Result, error) {
	wf, err := workflow.Load(path)
	if err != nil {
		return nil, err
	}
	im := &importState{
		root:         repoRoot(filepath.Dir(path)),
		res:          &Result{},
		seenTool:     map[string]bool{},
		seenWorkflow: map[string]bool{},
	}
	im.importWorkflow(wf, a)
	dedupeNames(im.res)
	return im.res, nil
}

func triggerList(wf *workflow.Workflow) string {
	ts := wf.Triggers()
	if len(ts) == 0 {
		return "nothing"
	}

	return strings.Join(ts, ", ")
}

func (im *importState) inputsFor(command, dir string) []string {
	if len(im.paths) > 0 {
		return im.paths
	}

	return inferInputsIn(im.root, dir, command)
}
