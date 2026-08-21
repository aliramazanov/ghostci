package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInterruptIsNotAnAllClear(t *testing.T) {
	r := newRepo(t)
	started := filepath.Join(t.TempDir(), "started")

	r.write("ghostci.yaml", `
checks:
  - name: slow
    command: "touch `+started+`; sleep 30"
    inputs: ["src/**"]
`)
	r.write("src/a.txt", "a")
	r.commit("seed")

	cmd := exec.Command(binary(t), "--all")
	cmd.Dir = r.dir
	cmd.Env = hermeticEnv()
	out := &strings.Builder{}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the check never started")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	err := cmd.Wait()
	if err == nil {
		t.Fatalf("an interrupted run exited 0:\n%s", out.String())
	}
	var exit *exec.ExitError
	if !asExit(err, &exit) {
		t.Fatal(err)
	}
	if exit.ExitCode() != 130 {
		t.Errorf("exit = %d, want 130\n%s", exit.ExitCode(), out.String())
	}
	if strings.Contains(out.String(), "\n  all clear.\n") {
		t.Errorf("an interrupted run claimed all clear:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "interrupted") {
		t.Errorf("the interruption is invisible:\n%s", out.String())
	}
}

func TestPushWithTagsStillChecksTheBranch(t *testing.T) {
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

	r.write("src/bad", "boom")
	bad := r.commit("breaking change")

	r.run("git", "tag", "-a", "v1.0.0", "-m", "release")

	out, code := r.tryRun("git", "push", "--follow-tags", "origin", "main")
	if code == 0 {
		t.Fatalf("a failing check did not block a push carrying a tag:\n%s", out)
	}
	if r.remoteHead() == bad {
		t.Fatal("the blocked commit reached the remote anyway")
	}
}

func TestJSONOutputIsParseableAndHonest(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: good
    command: "true"
    inputs: ["src/**"]
  - name: bad
    command: "echo the-details; false"
    inputs: ["src/**"]
`)
	r.write("src/a.txt", "a")
	r.commit("seed")

	got := r.ghostci("", "--all", "--json")
	if got.code != 1 {
		t.Fatalf("exit = %d, want 1\n%s", got.code, got.out)
	}

	var doc struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Output string `json:"output"`
		} `json:"checks"`
		Failed bool `json:"failed"`
	}
	if err := json.Unmarshal([]byte(got.out), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, got.out)
	}
	if len(doc.Checks) != 2 || !doc.Failed {
		t.Fatalf("unexpected document: %+v", doc)
	}
	for _, c := range doc.Checks {
		if c.Name == "bad" && !strings.Contains(c.Output, "the-details") {
			t.Errorf("failure output missing from JSON: %+v", c)
		}
	}
}
