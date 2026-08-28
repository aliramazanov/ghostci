package gitlab

import (
	"os"
	"path/filepath"
	"testing"
)

func TestASplicedRuleSequenceIsFlattened(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".gitlab-ci.yml")
	if err := os.WriteFile(path, []byte(`
.on_merge: &on_merge
  - if: $CI_PROJECT_PATH != "org/repo"
    when: manual
  - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH

test:
  script:
    - make test
  rules:
    - if: $CI_MERGE_REQUEST_LABELS =~ /FIPS/
    - *on_merge
`), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := Load(path)
	if err != nil {
		t.Fatalf("the file did not load: %v", err)
	}
	if len(f.Jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(f.Jobs))
	}

	if got := len(f.Jobs[0].Job.Rules); got != 3 {
		t.Errorf("got %d rules, want 3 with the spliced ones flattened", got)
	}
	for i, r := range f.Jobs[0].Job.Rules {
		if r.If == "" {
			t.Errorf("rule %d has no condition, so the splice was lost", i)
		}
	}
}

func TestAnUnreadableJobDoesNotTakeTheFileWithIt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".gitlab-ci.yml")
	if err := os.WriteFile(path, []byte(`
good:
  script:
    - make test

broken:
  script:
    - make other
  rules:
    if: not-a-list

also-good:
  script:
    - make lint
`), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := Load(path)
	if err != nil {
		t.Fatalf("one unreadable job failed the whole file: %v", err)
	}
	if len(f.Jobs) != 2 {
		t.Errorf("got %d readable jobs, want the two that are fine", len(f.Jobs))
	}
	if len(f.Unreadable) != 1 {
		t.Fatalf("got %d unreadable jobs recorded, want 1", len(f.Unreadable))
	}
	if f.Unreadable[0].Name != "broken" {
		t.Errorf("recorded %q as unreadable", f.Unreadable[0].Name)
	}
}
