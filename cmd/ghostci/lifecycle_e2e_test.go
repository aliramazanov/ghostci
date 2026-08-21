package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func (r *repo) tryRun(args ...string) (string, int) {
	r.t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = r.dir
	cmd.Env = hermeticEnv()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}

	var exit *exec.ExitError
	if !asExit(err, &exit) {
		r.t.Fatalf("%v: %v\n%s", args, err, out)
	}

	return string(out), exit.ExitCode()
}

func (r *repo) remoteHead() string {
	r.t.Helper()

	cmd := exec.Command("git", "rev-parse", "main")
	cmd.Dir = r.bare
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

func lines(t *testing.T, path string) int {
	t.Helper()

	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}

	return len(strings.Split(strings.TrimRight(string(body), "\n"), "\n"))
}

func TestInstalledHookBlocksAFailingPush(t *testing.T) {
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

	if got := r.ghostci("", "install-hook"); got.code != 0 {
		t.Fatalf("install-hook: exit %d\n%s", got.code, got.out)
	}

	r.write("src/more.txt", "fine")
	good := r.commit("clean change")
	if out, code := r.tryRun("git", "push", "origin", "main"); code != 0 {
		t.Fatalf("a passing push was blocked: exit %d\n%s", code, out)
	}
	if r.remoteHead() != good {
		t.Fatal("the passing push did not reach the remote")
	}

	r.write("src/bad", "boom")
	bad := r.commit("breaking change")
	out, code := r.tryRun("git", "push", "origin", "main")
	if code == 0 {
		t.Fatalf("a failing check did not block the push:\n%s", out)
	}
	if r.remoteHead() == bad {
		t.Fatal("the blocked commit reached the remote anyway")
	}

	if out, code := r.tryRun("git", "push", "--no-verify", "origin", "main"); code != 0 {
		t.Fatalf("--no-verify was blocked: exit %d\n%s", code, out)
	}
	if r.remoteHead() != bad {
		t.Fatal("--no-verify did not push")
	}

	r.run("git", "update-ref", "refs/heads/hold", bad)
	r.write("src/bad2", "boom")
	skipped := r.commit("second breaking change")
	cmd := exec.Command("git", "push", "origin", "main")
	cmd.Dir = r.dir
	cmd.Env = append(hermeticEnv(), "GHOSTCI_SKIP=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("GHOSTCI_SKIP push failed: %v\n%s", err, out)
	}
	if r.remoteHead() != skipped {
		t.Fatal("GHOSTCI_SKIP did not push")
	}

	if got := r.ghostci("", "uninstall-hook"); got.code != 0 {
		t.Fatalf("uninstall-hook: exit %d\n%s", got.code, got.out)
	}
	r.write("src/bad3", "boom")
	last := r.commit("third breaking change")
	if out, code := r.tryRun("git", "push", "origin", "main"); code != 0 {
		t.Fatalf("push after uninstall was blocked: exit %d\n%s", code, out)
	}
	if r.remoteHead() != last {
		t.Fatal("push after uninstall did not land")
	}
}

func TestCacheLifecycle(t *testing.T) {
	r := newRepo(t)
	marker := filepath.Join(t.TempDir(), "runs")
	failing := filepath.Join(t.TempDir(), "fails")

	r.write("ghostci.yaml", fmt.Sprintf(`
checks:
  - name: counted
    command: "echo run >> %s"
    inputs: ["src/**"]
  - name: doomed
    command: "echo run >> %s; exit 1"
    inputs: ["src/**"]
`, marker, failing))
	r.write("src/a.txt", "a")
	r.commit("seed")

	first := r.ghostci("", "--all")
	if first.code != 1 {
		t.Fatalf("first run: exit %d, want 1 for the doomed check\n%s", first.code, first.out)
	}
	if got := lines(t, marker); got != 1 {
		t.Fatalf("counted ran %d times, want 1", got)
	}

	second := r.ghostci("", "--all")
	if got := lines(t, marker); got != 1 {
		t.Errorf("counted ran again with unchanged inputs (%d runs):\n%s", got, second.out)
	}
	if got := lines(t, failing); got != 2 {
		t.Errorf("doomed ran %d times, want 2: a failure must never be cached", got)
	}
	if !strings.Contains(second.out, "cached") {
		t.Errorf("second run does not report the cached check:\n%s", second.out)
	}

	r.write("src/a.txt", "changed")
	r.ghostci("", "--all")
	if got := lines(t, marker); got != 2 {
		t.Errorf("counted ran %d times after an input edit, want 2", got)
	}

	r.ghostci("", "--all", "--no-cache")
	if got := lines(t, marker); got != 3 {
		t.Errorf("counted ran %d times under --no-cache, want 3", got)
	}
	r.ghostci("", "--all")
	if got := lines(t, marker); got != 3 {
		t.Errorf("counted ran %d times, want 3: the --no-cache pass should have been recorded", got)
	}

	if err := os.Remove(filepath.Join(r.dir, "src/a.txt")); err != nil {
		t.Fatal(err)
	}
	r.ghostci("", "--all")
	if got := lines(t, marker); got != 4 {
		t.Errorf("counted ran %d times after an input was deleted, want 4", got)
	}
}

func TestInitProducesARunnableConfig(t *testing.T) {
	r := newRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")

	r.write(".github/workflows/ci.yml", fmt.Sprintf(`
name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo ok >> %s
      - run: ./notify
        if: needs.deploy.result == 'success'
`, marker))
	r.commit("seed")

	got := r.ghostci("", "init")
	if got.code != 0 {
		t.Fatalf("init: exit %d\n%s", got.code, got.out)
	}
	if !r.exists("ghostci.yaml") {
		t.Fatal("init wrote no config")
	}

	run := r.ghostci("", "--all")
	if run.code != 0 {
		t.Fatalf("running the generated config: exit %d\n%s", run.code, run.out)
	}
	if lines(t, marker) != 1 {
		t.Error("the imported check never executed")
	}

	body, err := os.ReadFile(filepath.Join(r.dir, "ghostci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	surfaced := strings.Contains(string(body), "notify") || strings.Contains(got.out, "notify")
	if !surfaced {
		t.Errorf("the needs-gated step vanished without a trace\nconfig:\n%s\ninit output:\n%s", body, got.out)
	}
}
