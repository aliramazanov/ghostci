package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importGitLab(t *testing.T, body string, extra ...string) *Result {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, GitLabFile)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		name := filepath.Join(dir, extra[i])
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(extra[i+1]), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := DefaultAssumptions()
	a.DefaultBranch = "main"

	res, err := ImportGitLab(path, a)
	if err != nil {
		t.Fatalf("ImportGitLab: %v", err)
	}

	return res
}

func checkNamed(t *testing.T, res *Result, name string) (int, bool) {
	t.Helper()
	for i, c := range res.Checks {
		if c.Name == name {
			return i, true
		}
	}

	return 0, false
}

func TestGitLabExtractsJobs(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
default:
  before_script:
    - setup.sh
variables:
  GLOBAL: "1"
test:
  variables:
    LOCAL: "2"
  script:
    - make test
`)

	i, ok := checkNamed(t, res, "test")
	if !ok {
		t.Fatalf("job not imported: %+v", res.Entries)
	}

	got := res.Checks[i]
	if got.Command != "setup.sh\nmake test" {
		t.Errorf("before_script was not prepended: %q", got.Command)
	}
	if got.Env["GLOBAL"] != "1" || got.Env["LOCAL"] != "2" {
		t.Errorf("variables did not reach the check: %v", got.Env)
	}
}

func TestGitLabChecksRunWithPipefail(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
test:
  script:
    - make test
`)

	if len(res.Checks) != 1 {
		t.Fatalf("want one check: %+v", res.Entries)
	}
	if !strings.Contains(res.Checks[0].Shell, "pipefail") {
		t.Errorf("shell = %q, want pipefail", res.Checks[0].Shell)
	}
	if !strings.Contains(res.Checks[0].Shell, "-e") {
		t.Errorf("shell = %q, want errexit", res.Checks[0].Shell)
	}
}

func TestGitLabChangesBecomeInputs(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
test:
  script: [make test]
  rules:
    - changes: ["src/**/*.go", "go.mod"]
`)

	i, ok := checkNamed(t, res, "test")
	if !ok {
		t.Fatalf("job not imported: %+v", res.Entries)
	}
	want := []string{"go.mod", "src/**/*.go"}
	if strings.Join(res.Checks[i].Inputs, ",") != strings.Join(want, ",") {
		t.Errorf("inputs = %v, want %v", res.Checks[i].Inputs, want)
	}
}

func TestGitLabSkipsAndSurfaces(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
tagged:
  script: [make release]
  rules:
    - if: $CI_COMMIT_TAG
manual-deploy:
  script: [make deploy]
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
      when: manual
server-gate:
  script: [make thing]
  rules:
    - if: $CI_MERGE_REQUEST_TARGET_BRANCH_NAME == "main"
legacy:
  script: [make old]
  only: [main]
with-db:
  services: [postgres:16]
  script: [make itest]
`)

	if len(res.Checks) != 0 {
		t.Fatalf("nothing here should import: %+v", res.Checks)
	}

	reasons := map[string]Outcome{}
	for _, e := range res.Entries {
		reasons[e.Job] = e.Outcome
	}

	for job, want := range map[string]Outcome{
		"tagged":        SkippedByCondition,
		"manual-deploy": SkippedByCondition,
		"server-gate":   NeedsReview,
		"legacy":        NeedsReview,
		"with-db":       UnsupportedFeature,
	} {
		if reasons[job] != want {
			t.Errorf("%s outcome = %v, want %v", job, reasons[job], want)
		}
	}
}

func TestGitLabRefusesScriptsNeedingServerVariables(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
publish:
  script:
    - docker push $CI_REGISTRY_IMAGE:latest
`)

	if len(res.Checks) != 0 {
		t.Fatalf("a job needing a server variable was imported: %+v", res.Checks)
	}
	if res.Count(NeedsReview) != 1 {
		t.Fatalf("it was not surfaced: %+v", res.Entries)
	}
	if !strings.Contains(res.Entries[0].Reason, "CI_REGISTRY_IMAGE") {
		t.Errorf("reason does not name the variable: %q", res.Entries[0].Reason)
	}
}

func TestGitLabWorkflowRulesGateEverything(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
nightly:
  script: [make nightly]
`)

	if len(res.Checks) != 0 {
		t.Fatalf("workflow rules exclude every local pipeline: %+v", res.Checks)
	}
	if len(res.NotTriggered) == 0 {
		t.Fatalf("the excluded pipeline was not named: %+v", res)
	}
}

func TestGitLabExtendsAndReference(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
.base:
  variables:
    BASE: "yes"
  before_script:
    - base-setup.sh
.scripts:
  common:
    - common.sh
test:
  extends: .base
  script:
    - !reference [.scripts, common]
    - make test
`)

	i, ok := checkNamed(t, res, "test")
	if !ok {
		t.Fatalf("job not imported: %+v", res.Entries)
	}
	got := res.Checks[i]
	if !strings.Contains(got.Command, "common.sh") {
		t.Errorf("!reference did not resolve: %q", got.Command)
	}
	if !strings.Contains(got.Command, "base-setup.sh") {
		t.Errorf("extends did not carry before_script: %q", got.Command)
	}
	if got.Env["BASE"] != "yes" {
		t.Errorf("extends did not carry variables: %v", got.Env)
	}

	for _, c := range res.Checks {
		if strings.HasPrefix(c.Name, ".") {
			t.Errorf("a hidden template was imported as a job: %s", c.Name)
		}
	}
}

func TestGitLabLocalIncludeIsFollowedAndRemoteIsNamed(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
include:
  - local: ci/extra.yml
  - remote: https://example.com/remote.yml
main-job:
  script: [make main]
`, "ci/extra.yml", `
included-job:
  script: [make included]
`)

	if _, ok := checkNamed(t, res, "included-job"); !ok {
		t.Errorf("a local include was not followed: %+v", res.Checks)
	}
	if _, ok := checkNamed(t, res, "main-job"); !ok {
		t.Errorf("the including file's own jobs went missing: %+v", res.Checks)
	}

	var named bool
	for _, e := range res.Entries {
		if strings.Contains(e.Reason, "example.com") {
			named = true
		}
	}
	if !named {
		t.Errorf("the remote include was not surfaced: %+v", res.Entries)
	}
}

func TestGitLabParallelMatrixExpands(t *testing.T) {
	t.Parallel()
	res := importGitLab(t, `
test:
  script: [make test]
  parallel:
    matrix:
      - GO: ["1.21", "1.22"]
        OS: [linux]
`)

	if len(res.Checks) != 2 {
		t.Fatalf("got %d checks, want one per matrix leg: %+v", len(res.Checks), res.Checks)
	}
	for _, c := range res.Checks {
		if c.Env["GO"] == "" || c.Env["OS"] != "linux" {
			t.Errorf("matrix values did not reach the leg: %v", c.Env)
		}
	}
}

func TestExtendsReachesTemplatesFromTheIncludingFile(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
include:
  - local: ci/test.yml

.go-cache:
  variables:
    GOPATH: /cache/go
  before_script:
    - mkdir -p .go
`, "ci/test.yml", `
unit:
  extends: .go-cache
  script:
    - go test ./...
`)

	i, ok := checkNamed(t, res, "unit")
	if !ok {
		t.Fatalf("the job was not imported: %+v", res.Entries)
	}

	got := res.Checks[i]
	if !strings.Contains(got.Command, "mkdir -p .go") {
		t.Errorf("before_script from the template was lost:\n%s", got.Command)
	}
	if got.Env["GOPATH"] != "/cache/go" {
		t.Errorf("variables from the template were lost: %v", got.Env)
	}
}

func TestExtendsReachesTemplatesFromAnIncludedFile(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
include:
  - local: ci/templates.yml

unit:
  extends: .base
  script:
    - go test ./...
`, "ci/templates.yml", `
.base:
  variables:
    MODE: ci
`)

	i, ok := checkNamed(t, res, "unit")
	if !ok {
		t.Fatalf("the job was not imported: %+v", res.Entries)
	}
	if res.Checks[i].Env["MODE"] != "ci" {
		t.Errorf("variables from the included template were lost: %v", res.Checks[i].Env)
	}
}

func TestSpecHeaderDoesNotHideThePipeline(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
spec:
  inputs:
    image_version:
      type: string
      default: latest
---
variables:
  MODE: ci

unit:
  script:
    - go test ./...
`)

	if _, ok := checkNamed(t, res, "unit"); !ok {
		t.Fatalf("the pipeline after the spec header was lost: %+v", res.Entries)
	}
	if _, ok := checkNamed(t, res, "spec"); ok {
		t.Error("the spec header was imported as a job")
	}
}

func TestAnUnresolvableReferenceRefusesOnlyItsOwnJob(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
include:
  - remote: https://example.com/shared.yml

needs-remote:
  script:
    - !reference [.remote-template, before_script]
    - make deploy

stands-alone:
  script:
    - make test
`)

	if _, ok := checkNamed(t, res, "stands-alone"); !ok {
		t.Fatalf("a job that resolves was lost: %+v", res.Entries)
	}
	if _, ok := checkNamed(t, res, "needs-remote"); ok {
		t.Error("a job using an unresolvable reference was imported anyway")
	}

	var named bool
	for _, e := range res.Entries {
		if e.Job == "needs-remote" && strings.Contains(e.Reason, "remote-template") {
			named = true
		}
	}
	if !named {
		t.Errorf("the unresolvable reference was not named: %+v", res.Entries)
	}
}

func TestHiddenKeysThatAreNotJobsDoNotRejectTheFile(t *testing.T) {
	t.Parallel()

	res := importGitLab(t, `
.code-patterns: &code-patterns
  - "**/*.go"
  - "Makefile"

.a-scalar: "just a value"

.real-template:
  variables:
    MODE: ci

unit:
  extends: .real-template
  script:
    - go test ./...
  rules:
    - changes: *code-patterns
`)

	i, ok := checkNamed(t, res, "unit")
	if !ok {
		t.Fatalf("the file was rejected over a hidden anchor: %+v", res.Entries)
	}
	if res.Checks[i].Env["MODE"] != "ci" {
		t.Errorf("the real template was lost: %v", res.Checks[i].Env)
	}
	if strings.Join(res.Checks[i].Inputs, ",") != "**/*.go,Makefile" {
		t.Errorf("the anchored changes list did not become inputs: %v", res.Checks[i].Inputs)
	}
}
