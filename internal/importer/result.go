package importer

import (
	"github.com/aliramazanov/ghostci/internal/config"
	"strings"
)

type Assumptions struct {
	Ref        string
	Repository string
	RunnerOS   string

	DefaultBranch string

	Events []string

	callInputs map[string]any
}

func DefaultAssumptions() Assumptions {
	return Assumptions{
		Ref:        "refs/heads/main",
		Repository: "local/repo",
		RunnerOS:   "Linux",
		Events:     []string{"push", "pull_request"},
	}
}

func (a Assumptions) events() []string {
	if len(a.Events) == 0 {
		return []string{"push"}
	}
	return a.Events
}

type Entry struct {
	Workflow string
	Job      string
	Step     string
	Matrix   string
	Outcome  Outcome
	Reason   string
	Command  string
	Heavy    bool
}

type Toolchain struct {
	Name    string
	Version string
	Source  string
}

type Result struct {
	Checks []config.Check

	Heavy       []config.Check
	HeavyReason map[string]string

	Entries    []Entry
	Toolchains []Toolchain
	Warnings   []string

	NotTriggered []string

	GitLab    bool
	Azure     bool
	Circle    bool
	Bitbucket bool
}

func (r *Result) Origin() string {
	switch {
	case r.GitLab:
		return GitLabFile
	case r.Azure:
		return "azure-pipelines.yml"
	case r.Circle:
		return CircleFile
	case r.Bitbucket:
		return BitbucketFile
	}

	return ".github/workflows"
}

func (r *Result) Unit() string {
	if r.GitLab || r.Bitbucket {
		return "jobs"
	}

	return "steps"
}

func (r *Result) UnitCount(n int) string {
	unit := strings.TrimSuffix(r.Unit(), "s")

	return countOf(n, unit)
}

func (r *Result) Count(o Outcome) int {
	n := 0
	for _, e := range r.Entries {
		if e.Outcome == o {
			n++
		}
	}
	return n
}
