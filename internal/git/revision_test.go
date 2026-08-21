package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestChangedRejectsOptionLikeRevisions(t *testing.T) {
	dir := repo(t)
	touch(t, dir, "a.go")
	commit(t, dir, "seed")

	target := filepath.Join(dir, "written-by-git")

	if _, err := Changed(dir, "--output="+target); !errors.Is(err, ErrBadRevision) {
		t.Fatalf("Changed = %v, want ErrBadRevision", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("git wrote a file it was never meant to")
	}
}

func TestRenameReportsBothPaths(t *testing.T) {
	dir := repo(t)
	touch(t, dir, "src/old.go")
	base := commit(t, dir, "seed")

	if err := os.Rename(filepath.Join(dir, "src/old.go"), filepath.Join(dir, "src/new.go")); err != nil {
		t.Fatal(err)
	}
	commit(t, dir, "rename")

	got, err := Changed(dir, base)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{"src/old.go": false, "src/new.go": false}
	for _, p := range got {
		want[p] = true
	}
	for p, seen := range want {
		if !seen {
			t.Errorf("Changed = %q, missing %q", got, p)
		}
	}
}
