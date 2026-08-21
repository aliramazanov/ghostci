package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoLocalEnvIsStripped(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "."}, {"config", "user.email", "t@t.t"}, {"config", "user.name", "t"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = other
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)

	root, err := Root(dir)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	resolved, _ := filepath.EvalSymlinks(root)
	want, _ := filepath.EvalSymlinks(dir)

	if resolved != want {
		t.Errorf("Root() = %s, want %s: an inherited GIT_DIR redirected the query", resolved, want)
	}

	dirty, err := Dirty(dir)
	if err != nil {
		t.Fatalf("Dirty: %v", err)
	}
	if len(dirty) != 1 || !strings.Contains(dirty[0], "a.txt") {
		t.Errorf("Dirty() = %v, want the untracked file in the target repo", dirty)
	}
}
