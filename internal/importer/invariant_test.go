package importer

import (
	"strings"
	"testing"
)

func TestUndecidableOutranksFalseInEveryProvider(t *testing.T) {
	t.Parallel()

	t.Run("github", func(t *testing.T) {
		t.Parallel()

		res := importYAML(t, `
name: t
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - if: github.event_name == 'push' && vars.DEPLOY == 'yes'
        run: make release
`)

		if len(res.Checks) != 0 {
			t.Fatalf("imported %d checks; an unanswerable gate was read as runnable", len(res.Checks))
		}

		if res.Entries[0].Outcome != NeedsReview {
			t.Fatalf("outcome = %v, want NeedsReview: %s", res.Entries[0].Outcome, reasonsOf(res))
		}
		if !strings.Contains(strings.ToLower(reasonsOf(res)), "vars") {
			t.Errorf("the entry does not say what could not be answered: %s", reasonsOf(res))
		}
	})

	t.Run("gitlab", func(t *testing.T) {
		t.Parallel()

		res := importGitLab(t, `
release:
  script:
    - make release
  rules:
    - if: '$CI_MERGE_REQUEST_TARGET_BRANCH_NAME == "main"'
`)

		if len(res.Checks) != 0 {
			t.Fatalf("imported %d checks; an unanswerable rule was read as a skip", len(res.Checks))
		}
		if res.Entries[0].Outcome != NeedsReview {
			t.Errorf("outcome = %v, want NeedsReview", res.Entries[0].Outcome)
		}
	})
}
