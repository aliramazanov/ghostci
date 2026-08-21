package workflow

import "testing"

func FuzzParseAndExpand(f *testing.F) {
	for _, seed := range []string{
		"on: [push]\njobs:\n  a:\n    steps:\n      - run: go test\n",
		"on: push\njobs:\n  a:\n    strategy:\n      matrix:\n        os: [linux, mac]\n        v: [1, 2]\n        include:\n          - os: linux\n            extra: yes\n        exclude:\n          - v: 1\n    steps:\n      - run: echo ${{ matrix.os }}\n",
		"on:\n  push:\n    paths: ['src/**']\njobs:\n  a:\n    strategy:\n      matrix: ${{ fromJSON(needs.x.outputs.m) }}\n    steps: []\n",
		"jobs:\n  a:\n    strategy:\n      matrix:\n        include:\n          - lone: true\n",
		"on: [push]\njobs:\n  a:\n    timeout-minutes: 999999999999999999\n    steps:\n      - run: sleep 1\n        timeout-minutes: -3\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		wf, err := Parse([]byte(src))
		if err != nil {
			return
		}
		wf.CallInputs = wf.callInputs()
		wf.Triggers()
		wf.PathFilters("push", "pull_request")
		for _, nj := range wf.Jobs {
			nj.Job.RunsOnLabels()
			nj.Job.Timeout()
			ExpandMatrix(nj.Job.Strategy)
			for _, s := range nj.Job.Steps {
				s.Optional()
				s.Timeout()
			}
		}
	})
}
