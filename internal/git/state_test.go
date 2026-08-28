package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append(args[:1:1], args[1:]...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return dir
}

func commit(t *testing.T, dir, message string) string {
	t.Helper()

	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", message}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	sha, err := Run(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	return sha
}

func touch(t *testing.T, dir, name string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedReadsNonASCIIPaths(t *testing.T) {
	dir := repo(t)
	touch(t, dir, "seed.txt")
	base := commit(t, dir, "seed")

	touch(t, dir, "src/café.go")
	touch(t, dir, "src/naïve.go")
	commit(t, dir, "accented")

	got, err := Changed(dir, base)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"src/café.go", "src/naïve.go"} {
		if !slices.Contains(got, want) {
			t.Errorf("Changed = %q, want it to contain %q", got, want)
		}
	}
}

func TestChangedListsCommittedPaths(t *testing.T) {
	dir := repo(t)
	touch(t, dir, "a.go")
	base := commit(t, dir, "seed")

	touch(t, dir, "b.go")
	commit(t, dir, "add b")

	got, err := Changed(dir, base)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"b.go"}) {
		t.Errorf("Changed = %q, want [b.go]", got)
	}
}

func TestDirtyIncludesUntracked(t *testing.T) {
	dir := repo(t)
	touch(t, dir, "a.go")
	commit(t, dir, "seed")

	touch(t, dir, "new file.go")

	got, err := Dirty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "new file.go") {
		t.Errorf("Dirty = %q, want it to contain %q", got, "new file.go")
	}
}

func TestChangedReportsFailureRatherThanNoChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	for _, base := range []string{
		"0000000000000000000000000000000000000000",
		"refs/heads/does-not-exist",
		"--output=/tmp/ghostci-should-never-be-written",
		"-x",
	} {
		files, err := Changed(dir, base)
		if err == nil {
			t.Errorf("Changed(%q) returned %v with no error", base, files)
		}
		if len(files) != 0 {
			t.Errorf("Changed(%q) returned files alongside a failure: %v", base, files)
		}
	}

	if _, err := os.Stat("/tmp/ghostci-should-never-be-written"); err == nil {
		os.Remove("/tmp/ghostci-should-never-be-written")
		t.Fatal("a ref beginning with - reached git as an option and wrote a file")
	}
}

func TestARenameReportsBothPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run("add", "-A")
	run("commit", "-qm", "init")
	run("mv", "src/a.go", "src/b.go")

	got, err := Dirty(dir)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, p := range got {
		seen[p] = true
	}

	if !seen["src/b.go"] {
		t.Errorf("the new path is missing from %v", got)
	}
	if !seen["src/a.go"] {
		t.Errorf("the old path is missing from %v, so a check watching it would be skipped", got)
	}
}
