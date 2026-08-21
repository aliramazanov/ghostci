package gitlab

import (
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/pipeline"
)

func FuzzParseAndResolve(f *testing.F) {
	for _, seed := range []string{
		"job:\n  script: echo hi\n",
		"job:\n  script:\n    - a\n    - [b, c]\n",
		".base:\n  script: [x]\njob:\n  extends: .base\n",
		".a:\n  extends: .b\n.b:\n  extends: .a\njob:\n  extends: .a\n",
		"job:\n  extends: .missing\n  script: [x]\n",
		".s:\n  common: [setup]\njob:\n  script:\n    - !reference [.s, common]\n",
		"job:\n  script:\n    - !reference [.nope, missing]\n",
		"job:\n  rules:\n    - if: '$CI_COMMIT_BRANCH == \"main\"'\n      changes: ['**/*.go']\n  script: [x]\n",
		"variables:\n  A:\n    value: '1'\n    description: d\njob:\n  script: [x]\n",
		"include:\n  - local: other.yml\njob:\n  script: [x]\n",
		"workflow:\n  rules:\n    - if: $CI\njob:\n  script: [x]\n",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		file, err := Parse([]byte(src))
		if err != nil || file == nil {
			return
		}

		for _, nj := range file.Jobs {

			job, err := file.Resolve(nj.Name, nj.Job)
			if err != nil {
				continue
			}

			if strings.HasPrefix(nj.Name, ".") {
				t.Errorf("a hidden template was listed as a job: %q", nj.Name)
			}

			for _, rule := range job.Rules {
				if rule.If == "" {
					continue
				}

				_, _ = EvalRule(rule.If, Vars{"CI": pipeline.Known("true")})
			}
		}
	})
}
