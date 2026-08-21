package gitlab

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/pipeline"
)

func branchVars() Vars {
	return Vars{
		"CI_COMMIT_BRANCH":   pipeline.Known("main"),
		"CI_PIPELINE_SOURCE": pipeline.Known("push"),
		"CI_COMMIT_TAG":      pipeline.Undefined(),
		"CI_DEFAULT_BRANCH":  pipeline.Known("main"),
		"EMPTY":              pipeline.Known(""),
	}
}

func TestEvalRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		expr string
		want bool
	}{
		{`$CI_COMMIT_BRANCH == "main"`, true},
		{`$CI_COMMIT_BRANCH == 'main'`, true},
		{`$CI_COMMIT_BRANCH != "main"`, false},
		{`$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH`, true},
		{`${CI_COMMIT_BRANCH} == "main"`, true},
		{`$CI_COMMIT_BRANCH`, true},
		{`$CI_COMMIT_TAG`, false},
		{`$EMPTY`, false},
		{`$CI_COMMIT_TAG == null`, true},
		{`$CI_COMMIT_BRANCH == null`, false},
		{`$CI_COMMIT_BRANCH =~ /^ma/`, true},
		{`$CI_COMMIT_BRANCH =~ /^MA/i`, true},
		{`$CI_COMMIT_BRANCH =~ /^MA/`, false},
		{`$CI_COMMIT_BRANCH !~ /^feature/`, true},
		{`$CI_COMMIT_TAG =~ /^v/`, false},
		{`$CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == "main"`, true},
		{`$CI_PIPELINE_SOURCE == "schedule" && $CI_COMMIT_BRANCH == "main"`, false},
		{`$CI_PIPELINE_SOURCE == "schedule" || $CI_COMMIT_BRANCH == "main"`, true},
		{`($CI_PIPELINE_SOURCE == "schedule" || $CI_COMMIT_BRANCH == "main") && $CI_COMMIT_TAG == null`, true},
	}

	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			t.Parallel()
			got, err := EvalRule(c.expr, branchVars())
			if err != nil {
				t.Fatalf("EvalRule(%q): %v", c.expr, err)
			}
			if got != c.want {
				t.Errorf("EvalRule(%q) = %v, want %v", c.expr, got, c.want)
			}
		})
	}
}

func TestServerOnlyVariablesAreUndecidable(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`$CI_MERGE_REQUEST_IID`,
		`$CI_MERGE_REQUEST_TARGET_BRANCH_NAME == "main"`,
		`$CI_COMMIT_BRANCH == "main" && $CI_REGISTRY_IMAGE != null`,
		`$GITLAB_USER_LOGIN == "someone"`,
	} {
		_, err := EvalRule(src, branchVars())

		var ue *pipeline.UndecidableError
		if !errors.As(err, &ue) {
			t.Errorf("EvalRule(%q) err = %v, want undecidable", src, err)
		}
	}
}

func TestShortCircuitKeepsRulesDecidable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		expr string
		want bool
	}{
		{`$CI_COMMIT_BRANCH == "main" || $CI_MERGE_REQUEST_IID`, true},
		{`$CI_COMMIT_BRANCH == "nope" && $CI_MERGE_REQUEST_IID`, false},
	}

	for _, c := range cases {
		got, err := EvalRule(c.expr, branchVars())
		if err != nil {
			t.Errorf("EvalRule(%q): %v", c.expr, err)

			continue
		}
		if got != c.want {
			t.Errorf("EvalRule(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestMalformedRulesAreRefused(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`$CI_COMMIT_BRANCH ==`,
		`$CI_COMMIT_BRANCH == "unterminated`,
		`$CI_COMMIT_BRANCH =~ /unterminated`,
		`($CI_COMMIT_BRANCH == "main"`,
		`$`,
	} {
		if _, err := EvalRule(src, branchVars()); err == nil {
			t.Errorf("EvalRule(%q) accepted a malformed rule", src)
		}
	}
}

func TestSelfReferencingReferenceIsMarkedNotFatal(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		".a:\n  s: [!reference [.a, s]]\njob:\n  script: [!reference [.a, s]]\n",
		".a:\n  s: [!reference [.b, s]]\n.b:\n  s: [!reference [.a, s]]\njob:\n  script: [!reference [.a, s]]\n",
	} {
		done := make(chan *File, 1)

		go func() {
			f, err := Parse([]byte(src))
			if err != nil {
				done <- nil

				return
			}
			done <- f
		}()

		select {
		case f := <-done:
			if f == nil {
				continue
			}

			job, ok := f.lookup("job")
			if !ok {
				t.Errorf("the job disappeared:\n%s", src)

				continue
			}

			var marked bool
			for _, line := range job.Script {
				if strings.Contains(line, Unresolved) {
					marked = true
				}
			}

			if !marked {
				t.Errorf("a self-referencing !reference was accepted as content:\n%s", src)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("Parse never returned:\n%s", src)
		}
	}
}

func TestNestedReferenceResolvesToItsContent(t *testing.T) {
	t.Parallel()

	file, err := Parse([]byte(".base:\n  s: [setup.sh]\n.mid:\n  s: [!reference [.base, s]]\njob:\n  script: [!reference [.mid, s]]\n"))
	if err != nil {
		t.Fatal(err)
	}

	job, err := file.Resolve("job", file.Jobs[0].Job)
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Script) != 1 || job.Script[0] != "setup.sh" {
		t.Errorf("nested reference resolved to %v, want [setup.sh]", job.Script)
	}
}
