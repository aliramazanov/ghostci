package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitLabInitProducesARunnableConfig(t *testing.T) {
	r := newRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")

	r.write(".gitlab-ci.yml", `
variables:
  GREETING: "hello"

unit:
  script:
    - echo $GREETING >> `+marker+`
  rules:
    - changes: ["src/**"]

piped:
  script:
    - "false | cat"
`)
	r.write("src/a.txt", "a")
	r.commit("seed")

	got := r.ghostci("", "init")

	if got.code != 0 {
		t.Fatalf("init: exit %d\n%s", got.code, got.out)
	}

	body, err := os.ReadFile(filepath.Join(r.dir, "ghostci.yaml"))

	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(body), ".gitlab-ci.yml") {
		t.Errorf("the generated config does not say where it came from:\n%s", body)
	}

	run := r.ghostci("", "--all")

	if run.code == 0 {
		t.Fatalf("a job whose pipeline starts with a failing command passed:\n%s", run.out)
	}

	if !strings.Contains(run.out, "piped") {
		t.Errorf("the failing job is not named:\n%s", run.out)
	}

	content, err := os.ReadFile(marker)

	if err != nil {
		t.Fatalf("the imported job never executed: %v", err)
	}

	if strings.TrimSpace(string(content)) != "hello" {
		t.Errorf("variables did not reach the command: %q", content)
	}
}

func TestWorkflowsWinOverGitLabWhenBothExist(t *testing.T) {
	r := newRepo(t)

	r.write(".github/workflows/ci.yml", `
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo from-github
`)
	r.write(".gitlab-ci.yml", `
job:
  script: [echo from-gitlab]
`)
	r.commit("seed")

	got := r.ghostci("", "import")

	if !strings.Contains(got.out, "github.ref") {
		t.Errorf("the GitHub workflow was not preferred:\n%s", got.out)
	}

	forced := r.ghostci("", "import", "-file", ".gitlab-ci.yml")

	if !strings.Contains(forced.out, "CI_PIPELINE_SOURCE") {
		t.Errorf("naming the GitLab file did not import it:\n%s", forced.out)
	}
}
