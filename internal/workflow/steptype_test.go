package workflow

import "testing"

func TestStepType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		step Step
		want StepType
	}{
		{Step{Run: "go test ./..."}, StepRun},
		{Step{Uses: "./.github/actions/verify"}, StepLocalAction},
		{Step{Uses: "./.github/workflows/build.yml"}, StepLocalReusableWorkflow},
		{Step{Uses: "actions/checkout@v4"}, StepRemoteAction},
		{Step{Uses: "docker://alpine:3"}, StepDockerAction},
		{Step{}, StepInvalid},

		{Step{Run: "echo", Uses: "actions/checkout@v4"}, StepRun},
	}

	for _, tc := range tests {
		if got := tc.step.Type(); got != tc.want {
			t.Errorf("Type(%+v) = %v, want %v", tc.step, got, tc.want)
		}
	}
}
