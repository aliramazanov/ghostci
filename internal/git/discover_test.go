package git

import (
	"os/exec"
	"testing"
)

func TestParseRemote(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"https://github.com/astral-sh/ruff.git":   "astral-sh/ruff",
		"https://github.com/astral-sh/ruff":       "astral-sh/ruff",
		"git@github.com:astral-sh/ruff.git":       "astral-sh/ruff",
		"ssh://git@github.com/astral-sh/ruff.git": "astral-sh/ruff",
		"https://user@gitlab.com/group/proj.git":  "group/proj",
		"git@gitlab.com:group/proj.git":           "group/proj",
		"https://github.com/owner/repo.git\n":     "owner/repo",
		"not-a-url":                               "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got := parseRemote(in); got != want {
				t.Errorf("parseRemote(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestDiscoverOutsideRepo(t *testing.T) {
	t.Parallel()
	if info := Discover(t.TempDir()); info.IsRepo {
		t.Error("a bare temp dir should not report as a repo")
	}
}

func TestDiscoverUnbornBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q", ".")
	run("config", "user.email", "t@t.t")
	run("config", "user.name", "t")
	run("checkout", "-q", "-b", "feature/x")
	run("remote", "add", "origin", "git@github.com:owner/repo.git")

	info := Discover(dir)
	if !info.IsRepo {
		t.Fatal("should detect a repo")
	}
	if info.Branch != "feature/x" {
		t.Errorf("Branch = %q, want feature/x", info.Branch)
	}
	if info.Ref != "refs/heads/feature/x" {
		t.Errorf("Ref = %q", info.Ref)
	}
	if info.Repository != "owner/repo" {
		t.Errorf("Repository = %q", info.Repository)
	}
}
