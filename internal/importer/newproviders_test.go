package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAt(t *testing.T, dir, rel, body string) string {
	t.Helper()

	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func commandsOf(res *Result) string {
	var out []string
	for _, c := range res.Checks {
		out = append(out, c.Command)
	}

	return strings.Join(out, "|")
}

func reasonsOf(res *Result) string {
	var out []string
	for _, e := range res.Entries {
		out = append(out, e.Reason)
	}

	return strings.Join(out, "|")
}

func TestAzureImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, "azure-pipelines.yml", `
trigger:
  paths:
    include: ["src/**"]
    exclude: ["docs/**"]
variables:
  BUILD_CONFIG: Release
steps:
  - checkout: self
  - script: dotnet build
    displayName: Build
  - bash: dotnet test
    displayName: Test
  - task: PublishBuildArtifacts@1
    displayName: Publish
  - script: echo ${{ parameters.thing }}
    displayName: Templated
  - script: ./deploy.sh
    displayName: Deploy
    condition: succeeded()
`)

	res, err := ImportAzure(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want build and test only: %s", len(res.Checks), commandsOf(res))
	}
	if !strings.Contains(commandsOf(res), "dotnet build") || !strings.Contains(commandsOf(res), "dotnet test") {
		t.Errorf("script and bash steps did not both import: %s", commandsOf(res))
	}

	if got := strings.Join(res.Checks[0].Inputs, ","); got != "src/**" {
		t.Errorf("inputs = %q, want the pipeline's own trigger paths", got)
	}
	if got := strings.Join(res.Checks[0].Exclude, ","); got != "docs/**" {
		t.Errorf("exclude = %q, want the pipeline's own exclude paths", got)
	}
	if res.Checks[0].Env["BUILD_CONFIG"] != "Release" {
		t.Errorf("variables did not reach the check: %v", res.Checks[0].Env)
	}

	reasons := reasonsOf(res)
	for _, want := range []string{"PublishBuildArtifacts", "Azure expression", "condition"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("no refusal mentioning %q: %s", want, reasons)
		}
	}
}

func TestAzureRunsWithErrexit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, "azure-pipelines.yml", "steps:\n  - script: make test\n")

	res, err := ImportAzure(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Checks) != 1 || !strings.Contains(res.Checks[0].Shell, "-e") {
		t.Fatalf("shell = %q, want errexit", res.Checks[0].Shell)
	}
}

func TestCircleImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, ".circleci/config.yml", `
version: 2.1
orbs:
  node: circleci/node@5.0
jobs:
  build:
    docker:
      - image: cimg/go:1.22
    environment:
      CGO_ENABLED: "0"
    steps:
      - checkout
      - run: go build ./...
      - run:
          name: Unit tests
          command: go test ./...
      - save_cache:
          paths: [/home/circleci/go/pkg/mod]
      - node/install
  itest:
    docker:
      - image: cimg/go:1.22
      - image: postgres:16
    steps:
      - run: make itest
`)

	res, err := ImportCircle(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want the two run steps: %s", len(res.Checks), commandsOf(res))
	}

	for _, c := range res.Checks {
		if !strings.Contains(c.Shell, "pipefail") || !strings.Contains(c.Shell, "-e") {
			t.Errorf("shell = %q, want the documented -eo pipefail", c.Shell)
		}
	}
	if res.Checks[0].Env["CGO_ENABLED"] != "0" {
		t.Errorf("job environment did not reach the check: %v", res.Checks[0].Env)
	}

	reasons := reasonsOf(res)
	if !strings.Contains(reasons, "orb") {
		t.Errorf("the orb was not surfaced: %s", reasons)
	}
	if !strings.Contains(reasons, "service containers") {
		t.Errorf("the job with a second image was not refused: %s", reasons)
	}
	if len(res.Toolchains) == 0 || res.Toolchains[0].Name != "go" {
		t.Errorf("the docker image did not record a toolchain: %+v", res.Toolchains)
	}
}

func TestCircleDynamicConfigIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, ".circleci/config.yml", `
version: 2.1
setup: true
jobs:
  plan:
    docker: [{image: cimg/base:current}]
    steps:
      - run: ./generate-config.sh
`)

	res, err := ImportCircle(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Checks) != 0 {
		t.Fatalf("a setup config must not import checks: %s", commandsOf(res))
	}
	if !strings.Contains(reasonsOf(res), "does not exist until CI runs it") {
		t.Errorf("the reason does not explain why: %s", reasonsOf(res))
	}
}

func TestBitbucketImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, "bitbucket-pipelines.yml", `
image: node:20
pipelines:
  default:
    - step:
        name: Build
        script:
          - npm ci
          - npm run build
    - parallel:
        - step:
            name: Lint
            script: [npm run lint]
        - step:
            name: Test
            script: [npm test]
    - step:
        name: Notify
        script:
          - pipe: atlassian/slack-notify:2.0.0
    - step:
        name: Integration
        services: [postgres]
        script: [make itest]
    - step:
        name: Ship
        deployment: production
        script: [make deploy]
  custom:
    nightly:
      - step:
          script: [make nightly]
`)

	res, err := ImportBitbucket(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Checks) != 3 {
		t.Fatalf("got %d checks, want build plus the two parallel steps: %s",
			len(res.Checks), commandsOf(res))
	}
	if !strings.Contains(commandsOf(res), "npm ci\nnpm run build") {
		t.Errorf("multi-line script was not joined: %s", commandsOf(res))
	}
	if !strings.Contains(commandsOf(res), "npm run lint") || !strings.Contains(commandsOf(res), "npm test") {
		t.Errorf("parallel steps did not both import: %s", commandsOf(res))
	}

	reasons := reasonsOf(res)

	for _, want := range []string{"atlassian/slack-notify", "service containers", "production", "started by hand"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("no refusal mentioning %q: %s", want, reasons)
		}
	}
}

func TestNewProvidersAreDetected(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ file, want string }{
		{"azure-pipelines.yml", "Azure Pipelines"},
		{".circleci/config.yml", "CircleCI"},
		{"bitbucket-pipelines.yml", "Bitbucket Pipelines"},
	} {
		dir := t.TempDir()
		writeAt(t, dir, c.file, "steps: []\n")

		name, path, ok := Detect(dir)
		if !ok {
			t.Errorf("%s was not detected", c.file)

			continue
		}
		if name != c.want {
			t.Errorf("%s detected as %q, want %q", c.file, name, c.want)
		}
		if !strings.HasSuffix(filepath.ToSlash(path), c.file) {
			t.Errorf("%s resolved to %q", c.file, path)
		}
	}
}

func TestBitbucketManualStepIsNotImported(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, "bitbucket-pipelines.yml", `
pipelines:
  default:
    - step:
        name: Build
        script: [make build]
    - step:
        name: Release
        trigger: manual
        script: [make release]
`)

	res, err := ImportBitbucket(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(commandsOf(res), "make release") {
		t.Fatalf("a manual step was imported: %s", commandsOf(res))
	}
	if !strings.Contains(commandsOf(res), "make build") {
		t.Fatalf("the automatic step was lost: %s", commandsOf(res))
	}
	if !strings.Contains(reasonsOf(res), "waits for someone") {
		t.Errorf("the manual step was dropped without a reason: %s", reasonsOf(res))
	}
}

func TestAzureJobsNeedingContainersAreRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAt(t, dir, "azure-pipelines.yml", `
jobs:
  - job: unit
    steps:
      - script: dotnet test
  - job: integration
    services:
      db: postgres
    steps:
      - script: dotnet test --filter Integration
  - job: contained
    container: mcr.microsoft.com/dotnet/sdk:8.0
    steps:
      - script: dotnet build
`)

	res, err := ImportAzure(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(commandsOf(res), "Integration") {
		t.Errorf("a job needing service containers was imported: %s", commandsOf(res))
	}
	if strings.Contains(commandsOf(res), "dotnet build") {
		t.Errorf("a job running in a container was imported: %s", commandsOf(res))
	}
	if !strings.Contains(commandsOf(res), "dotnet test") {
		t.Errorf("the ordinary job was lost: %s", commandsOf(res))
	}

	reasons := reasonsOf(res)
	for _, want := range []string{"service containers", "container"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("no refusal mentioning %q: %s", want, reasons)
		}
	}
}
