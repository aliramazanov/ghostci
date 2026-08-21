package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheStillServesWhenTheDiffBaseIsUnknown(t *testing.T) {
	r := newRepo(t)
	marker := filepath.Join(t.TempDir(), "runs")

	r.write("ghostci.yaml", fmt.Sprintf(`
checks:
  - name: counted
    command: "echo run >> %s"
    inputs: ["src/**"]
`, marker))
	r.write("src/a.txt", "a")
	r.commit("seed")

	first := r.ghostci("", "--explain")

	if first.code != 0 {
		t.Fatalf("first run: exit %d\n%s", first.code, first.out)
	}

	if !strings.Contains(first.out, "running every check") {
		t.Fatalf("this test needs an unresolvable base to mean anything:\n%s", first.out)
	}

	if got := lines(t, marker); got != 1 {
		t.Fatalf("counted ran %d times, want 1", got)
	}

	second := r.ghostci("", "--explain")

	if got := lines(t, marker); got != 1 {
		t.Errorf("counted ran %d times with unchanged inputs and no diff base, want 1:\n%s",
			got, second.out)
	}

	if !strings.Contains(second.out, "cached") {
		t.Errorf("the second run did not report a cache hit:\n%s", second.out)
	}

	r.write("src/a.txt", "changed")
	r.ghostci("", "--explain")

	if got := lines(t, marker); got != 2 {
		t.Errorf("counted ran %d times after an edit, want 2", got)
	}
}
