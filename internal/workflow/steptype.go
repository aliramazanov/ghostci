package workflow

import "strings"

type StepType int

const (
	StepRun StepType = iota

	StepLocalAction

	StepRemoteAction

	StepDockerAction

	StepLocalReusableWorkflow

	StepInvalid
)

func (t StepType) String() string {
	switch t {
	case StepRun:
		return "run"
	case StepLocalAction:
		return "local action"
	case StepRemoteAction:
		return "remote action"
	case StepDockerAction:
		return "docker action"
	case StepLocalReusableWorkflow:
		return "local reusable workflow"
	}

	return "invalid"
}

func (s Step) Type() StepType {
	switch {
	case strings.TrimSpace(s.Run) != "":
		return StepRun
	case s.Uses == "":
		return StepInvalid
	case strings.HasPrefix(s.Uses, "docker://"):
		return StepDockerAction
	case isLocal(s.Uses):
		if strings.Contains(s.Uses, "/.github/workflows/") {
			return StepLocalReusableWorkflow
		}

		return StepLocalAction
	}

	return StepRemoteAction
}

func isLocal(uses string) bool {
	return strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, ".\\")
}
