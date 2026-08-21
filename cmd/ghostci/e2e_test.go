package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var buildOnce struct {
	sync.Once
	path string
	err  error
}

func binary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ghostci-e2e-")

		if err != nil {
			buildOnce.err = err

			return
		}

		buildOnce.path = filepath.Join(dir, "ghostci")
		out, err := exec.Command("go", "build", "-o", buildOnce.path, ".").CombinedOutput()

		if err != nil {
			buildOnce.err = err
			t.Logf("build: %s", out)
		}
	})

	if buildOnce.err != nil {
		t.Fatalf("building ghostci: %v", buildOnce.err)
	}

	return buildOnce.path
}

type repo struct {
	t    *testing.T
	dir  string
	bare string
}

func newRepo(t *testing.T) *repo {
	t.Helper()

	r := &repo{t: t, dir: t.TempDir(), bare: t.TempDir()}
	r.run("git", "init", "-q", "-b", "main", ".")
	r.run("git", "config", "user.email", "t@example.com")
	r.run("git", "config", "user.name", "t")
	r.run("git", "init", "-q", "--bare", r.bare)
	r.run("git", "remote", "add", "origin", r.bare)

	return r
}

func hermeticEnv() []string {
	var out []string

	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GHOSTCI") {
			out = append(out, kv)
		}
	}

	return out
}

func (r *repo) run(args ...string) string {
	r.t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = r.dir
	cmd.Env = hermeticEnv()
	out, err := cmd.CombinedOutput()

	if err != nil {
		r.t.Fatalf("%v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

func (r *repo) write(rel, body string) {
	r.t.Helper()

	path := filepath.Join(r.dir, rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(message string) string {
	r.t.Helper()

	r.run("git", "add", "-A")
	r.run("git", "commit", "-q", "-m", message)

	return r.run("git", "rev-parse", "HEAD")
}

func (r *repo) existsAt(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

func (r *repo) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(r.dir, rel))

	return err == nil
}

type result struct {
	out  string
	code int
}

func (r *repo) ghostci(stdin string, args ...string) result {
	r.t.Helper()

	cmd := exec.Command(binary(r.t), args...)
	cmd.Dir = r.dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = hermeticEnv()

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

func asExit(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)

	if ok {
		*target = e
	}

	return ok
}

const markerConfig = `
checks:
  - name: touched
    command: "touch ran-touched"
    inputs: ["src/**"]
  - name: elsewhere
    command: "touch ran-elsewhere"
    inputs: ["docs/**"]
`

func seeded(t *testing.T) *repo {
	r := newRepo(t)
	r.write("ghostci.yaml", markerConfig)
	r.write("src/a.txt", "a")
	r.write("docs/b.txt", "b")
	r.commit("seed")
	r.run("git", "push", "-q", "-u", "origin", "main")

	return r
}

func TestPushingABranchThatIsNotCheckedOutIsNotReportedClean(t *testing.T) {
	r := seeded(t)

	r.run("git", "checkout", "-q", "-b", "feature")
	r.write("src/new.txt", "new")
	head := r.commit("work on feature")
	r.run("git", "checkout", "-q", "main")

	stdin := "refs/heads/feature " + head + " refs/heads/feature " + strings.Repeat("0", 40) + "\n"
	got := r.ghostci(stdin, "--hook")

	if strings.Contains(got.out, "all clear") {
		t.Errorf("reported all clear for a branch it never looked at:\n%s", got.out)
	}

	if r.exists("ran-touched") {
		t.Error("a check ran against the wrong branch's contents")
	}
}

func TestPushingTheCheckedOutBranchRunsTheAffectedCheck(t *testing.T) {
	r := seeded(t)

	r.write("src/new.txt", "new")
	head := r.commit("change src")

	before := r.run("git", "rev-parse", "HEAD~1")
	stdin := "refs/heads/main " + head + " refs/heads/main " + before + "\n"
	got := r.ghostci(stdin, "--hook")

	if !r.exists("ran-touched") {
		t.Errorf("the check watching src/** never ran:\n%s", got.out)
	}

	if r.exists("ran-elsewhere") {
		t.Errorf("a check watching docs/** ran for a src change:\n%s", got.out)
	}

	if got.code != 0 {
		t.Errorf("exit = %d, want 0\n%s", got.code, got.out)
	}
}

func TestAFailingCheckFailsTheRun(t *testing.T) {
	r := newRepo(t)
	r.write("ghostci.yaml", `
checks:
  - name: doomed
    command: "echo the-actual-reason >&2; exit 3"
`)
	r.commit("seed")

	got := r.ghostci("", "--all")

	if got.code != 1 {
		t.Errorf("exit = %d, want 1", got.code)
	}

	if !strings.Contains(got.out, "the-actual-reason") {
		t.Errorf("the failing command's output was not shown:\n%s", got.out)
	}

	if strings.Contains(got.out, "all clear") {
		t.Errorf("reported all clear despite a failure:\n%s", got.out)
	}
}

func TestEarlyFailureInAMultiLineCheckFailsTheRun(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: two-lines
    command: |
      ls /definitely/not/here
      true
`)
	r.commit("seed")

	got := r.ghostci("", "--all")

	if got.code != 1 {
		t.Errorf("exit = %d, want 1: the first line failed\n%s", got.code, got.out)
	}
}

func TestAnUnreadableWorkingTreeRunsEverything(t *testing.T) {
	r := seeded(t)
	r.write("src/new.txt", "new")
	r.commit("change src")

	if err := os.WriteFile(filepath.Join(r.dir, ".git", "index"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := r.ghostci("", "--explain")

	if r.exists("ran-touched") && r.exists("ran-elsewhere") {
		return
	}

	t.Errorf("checks were skipped while the working tree was unreadable:\n%s", got.out)
}

func TestACheckWithNoInputsAlwaysRuns(t *testing.T) {
	r := seeded(t)
	r.write("ghostci.yaml", `
checks:
  - name: unknown-inputs
    command: "touch ran-unknown"
`)
	r.write("docs/b.txt", "changed docs only")
	r.commit("docs")

	got := r.ghostci("", "--explain")

	if !r.exists("ran-unknown") {
		t.Errorf("a check with no inputs was skipped:\n%s", got.out)
	}
}
