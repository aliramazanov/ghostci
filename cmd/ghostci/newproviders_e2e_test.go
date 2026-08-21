package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProvidersEndToEnd(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		config  func(marker string) string
		refused string
	}{
		{
			name: "azure",
			file: "azure-pipelines.yml",
			config: func(marker string) string {
				return fmt.Sprintf(`
trigger:
  paths:
    include: ["src/**"]
variables:
  GREETING: hello
steps:
  - checkout: self
  - script: echo $GREETING >> %s
    displayName: Greet
  - task: PublishBuildArtifacts@1
    displayName: Publish
`, marker)
			},
			refused: "PublishBuildArtifacts",
		},
		{
			name: "circleci",
			file: ".circleci/config.yml",
			config: func(marker string) string {
				return fmt.Sprintf(`
version: 2.1
orbs:
  node: circleci/node@5.0
jobs:
  build:
    docker:
      - image: cimg/base:current
    environment:
      GREETING: hello
    steps:
      - checkout
      - run:
          name: Greet
          command: echo $GREETING >> %s
`, marker)
			},
			refused: "orb",
		},
		{
			name: "bitbucket",
			file: "bitbucket-pipelines.yml",
			config: func(marker string) string {
				return fmt.Sprintf(`
pipelines:
  default:
    - step:
        name: Greet
        script:
          - echo hello >> %s
    - step:
        name: Notify
        script:
          - pipe: atlassian/slack-notify:2.0.0
`, marker)
			},
			refused: "atlassian/slack-notify",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRepo(t)
			marker := filepath.Join(t.TempDir(), "ran")

			r.write(c.file, c.config(marker))
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

			if !strings.Contains(string(body), filepath.Base(c.file)) {
				t.Errorf("the config does not name where it came from:\n%s", body)
			}

			if !strings.Contains(got.out, c.refused) && !strings.Contains(string(body), c.refused) {
				t.Errorf("%q was dropped without a trace\nledger:\n%s\nconfig:\n%s",
					c.refused, got.out, body)
			}

			run := r.ghostci("", "--all")

			if run.code != 0 {
				t.Fatalf("running the generated config: exit %d\n%s", run.code, run.out)
			}

			if lines(t, marker) != 1 {
				t.Fatalf("the imported check never executed:\n%s", run.out)
			}
		})
	}
}

func TestEveryProviderFailsOnAFailingCommand(t *testing.T) {
	cases := []struct{ name, file, config string }{
		{"azure", "azure-pipelines.yml", "steps:\n  - script: exit 3\n    displayName: Boom\n"},
		{
			"circleci", ".circleci/config.yml",
			"version: 2.1\njobs:\n  build:\n    docker: [{image: 'cimg/base:current'}]\n    steps:\n      - run: exit 3\n",
		},
		{
			"bitbucket", "bitbucket-pipelines.yml",
			"pipelines:\n  default:\n    - step:\n        name: Boom\n        script: [exit 3]\n",
		},
		{"gitlab", ".gitlab-ci.yml", "boom:\n  script:\n    - exit 3\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRepo(t)
			r.write(c.file, c.config)
			r.write("src/a.txt", "a")
			r.commit("seed")

			if got := r.ghostci("", "init"); got.code != 0 {
				t.Fatalf("init: exit %d\n%s", got.code, got.out)
			}

			run := r.ghostci("", "--all")
			if run.code == 0 {
				t.Fatalf("a failing command was reported as passing:\n%s", run.out)
			}
			if strings.Contains(run.out, "\n  all clear.\n") {
				t.Errorf("a failing check still printed all clear:\n%s", run.out)
			}
		})
	}
}
