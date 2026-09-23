package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func (r *repo) ghostciWith(env []string, args ...string) result {
	r.t.Helper()

	cmd := exec.Command(binary(r.t), args...)
	cmd.Dir = r.dir
	cmd.Stdin = strings.NewReader("")
	cmd.Env = append(hermeticEnv(), env...)

	out, err := cmd.CombinedOutput()
	code := 0
	var exit *exec.ExitError

	if err != nil {
		if !asExit(err, &exit) {
			r.t.Fatalf("running ghostci: %v\n%s", err, out)
		}
		code = exit.ExitCode()
	}

	return result{out: string(out), code: code}
}

func TestAnOverriddenVariableIsReportedWhenEverythingIsCached(t *testing.T) {
	t.Parallel()

	r := seeded(t)

	if res := r.ghostci("", "--all"); res.code != 0 {
		t.Fatalf("first run: exit %d\n%s", res.code, res.out)
	}

	res := r.ghostciWith([]string{"GITHUB_REF=refs/heads/stale"}, "--all", "--quiet")

	if !strings.Contains(res.out, "GITHUB_REF=refs/heads/stale") {
		t.Errorf("an all-cached run hid the overridden variable:\n%s", res.out)
	}
}

func TestACheckWatchingNothingRunsEveryTime(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.write("ghostci.yaml", "checks:\n  - name: typo\n    command: \"touch ran-typo\"\n    inputs: [\"docz/**\"]\n")
	r.write("docs/guide.md", "hi")
	r.commit("seed")
	r.run("git", "push", "-q", "-u", "origin", "main")

	for run := 1; run <= 2; run++ {
		_ = os.Remove(filepath.Join(r.dir, "ran-typo"))

		res := r.ghostci("")

		if !r.exists("ran-typo") {
			t.Errorf("run %d: a check whose inputs match nothing was not run:\n%s", run, res.out)
		}

		if !strings.Contains(res.out, "watches no file here") {
			t.Errorf("run %d: the misconfigured inputs were not reported:\n%s", run, res.out)
		}
	}
}

func TestTurningGhostciOffIsNeverSilentUnderTheHook(t *testing.T) {
	t.Parallel()

	r := seeded(t)
	res := r.ghostciWith([]string{"GHOSTCI=0"}, "--hook")

	if res.code != 0 {
		t.Errorf("exit = %d, want 0", res.code)
	}

	if !strings.Contains(res.out, "set to off") {
		t.Errorf("the hook skipped every check without saying so:\n%s", res.out)
	}
}

func TestJSONNeverCallsAnUnverifiedRunAPass(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.write("ghostci.yaml", "checks:\n  - name: broken\n    command: \"true\"\n    dir: ./missing\n")
	r.commit("seed")

	res := r.ghostci("", "--json", "--all")

	if res.code != 1 {
		t.Errorf("exit = %d, want 1", res.code)
	}

	if !strings.Contains(res.out, `"failed": true`) {
		t.Errorf("JSON reported a pass for a run that verified nothing:\n%s", res.out)
	}

	if strings.Contains(res.out, `"exit_code"`) {
		t.Errorf("a check that never started reported an exit code:\n%s", res.out)
	}
}
