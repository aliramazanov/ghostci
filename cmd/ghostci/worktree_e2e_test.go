package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookInstalledFromAWorktreeGuardsIt(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: gate
    command: "test ! -f src/bad"
    inputs: ["src/**"]
`)
	r.write("src/ok.txt", "ok")
	r.commit("seed")
	r.run("git", "push", "-q", "-u", "origin", "main")

	tree := filepath.Join(t.TempDir(), "feature")
	r.run("git", "worktree", "add", "-q", tree, "-b", "feature")

	inTree := func(args ...string) (string, int) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tree
		cmd.Env = hermeticEnv()
		out, err := cmd.CombinedOutput()
		if err == nil {
			return string(out), 0
		}
		var exit *exec.ExitError
		if !asExit(err, &exit) {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out), exit.ExitCode()
	}

	if out, code := inTree(binary(t), "install-hook"); code != 0 {
		t.Fatalf("install-hook from a worktree: exit %d\n%s", code, out)
	}

	if err := os.WriteFile(filepath.Join(tree, "src", "bad"), []byte("boom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := inTree("git", "add", "-A"); code != 0 {
		t.Fatal("git add failed")
	}
	if _, code := inTree("git", "commit", "-qm", "breaking"); code != 0 {
		t.Fatal("git commit failed")
	}

	out, code := inTree("git", "push", "origin", "feature")
	if code == 0 {
		t.Fatalf("a failing check did not block a push from a worktree:\n%s", out)
	}

	refs := r.run("git", "ls-remote", r.bare)
	if strings.Contains(refs, "refs/heads/feature") {
		t.Error("the blocked branch reached the remote anyway")
	}
}

func TestInstallRefusesASharedHooksPath(t *testing.T) {
	r := newRepo(t)

	shared := t.TempDir()
	r.run("git", "config", "core.hooksPath", shared)
	r.write("ghostci.yaml", "checks:\n  - name: c\n    command: \"true\"\n")
	r.commit("seed")

	got := r.ghostci("", "install-hook")
	if got.code == 0 {
		t.Fatalf("installing into a shared hooks path was allowed:\n%s", got.out)
	}
	if !strings.Contains(got.out, "core.hooksPath") {
		t.Errorf("the refusal does not name the setting responsible:\n%s", got.out)
	}

	forced := r.ghostci("", "install-hook", "--force")
	if forced.code != 0 {
		t.Fatalf("--force did not install: exit %d\n%s", forced.code, forced.out)
	}
	if !r.existsAt(filepath.Join(shared, "pre-push")) {
		t.Error("--force reported success without writing the hook")
	}
}
