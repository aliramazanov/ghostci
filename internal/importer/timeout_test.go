package importer

import (
	"testing"
	"time"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestBitbucketCarriesItsDeclaredTimeout(t *testing.T) {
	t.Parallel()

	path := writeAt(t, t.TempDir(), BitbucketFile, `
pipelines:
  default:
    - step:
        name: slow
        max-time: 5
        script:
          - make build
    - step:
        name: unbounded
        script:
          - make other
`)

	res, err := ImportBitbucket(path, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Checks) != 2 {
		t.Fatalf("imported %d checks, want 2", len(res.Checks))
	}
	if got := time.Duration(res.Checks[0].Timeout); got != 5*time.Minute {
		t.Errorf("timeout = %v, want 5m: max-time is minutes", got)
	}
	if got := time.Duration(res.Checks[1].Timeout); got != 0 {
		t.Errorf("timeout = %v, want none declared", got)
	}
}

func TestAzureCarriesItsDeclaredTimeout(t *testing.T) {
	t.Parallel()

	res := importAzure(t, `
trigger: [main]
jobs:
  - job: build
    timeoutInMinutes: 30
    steps:
      - script: make build
        displayName: inherits the job
      - script: make quick
        displayName: overrides it
        timeoutInMinutes: 2
`)

	byName := map[string]config.Check{}
	for _, c := range res.Checks {
		byName[c.Name] = c
	}

	if got := time.Duration(byName["inherits-the-job"].Timeout); got != 30*time.Minute {
		t.Errorf("timeout = %v, want the job's 30m", got)
	}
	if got := time.Duration(byName["overrides-it"].Timeout); got != 2*time.Minute {
		t.Errorf("timeout = %v, want the step's 2m", got)
	}
}

func TestAZeroTimeoutIsNoLimitNotAnExpiredOne(t *testing.T) {
	t.Parallel()

	res := importAzure(t, `
trigger: [main]
jobs:
  - job: build
    timeoutInMinutes: 0
    steps:
      - script: make build
        displayName: unbounded
        timeoutInMinutes: 0
`)

	if len(res.Checks) != 1 {
		t.Fatalf("imported %d checks, want 1", len(res.Checks))
	}
	if got := time.Duration(res.Checks[0].Timeout); got != 0 {
		t.Errorf("timeout = %v, want none: zero means no limit", got)
	}
}

func TestTimeoutMinutesIsBounded(t *testing.T) {
	t.Parallel()

	for _, n := range []int{-1, 0} {
		if got := timeoutMinutes(n); got != 0 {
			t.Errorf("timeoutMinutes(%d) = %v, want 0", n, got)
		}
	}

	if got := timeoutMinutes(1 << 60); got != maxImportedTimeout {
		t.Errorf("timeoutMinutes(huge) = %v, want the cap", got)
	}
	if got := timeoutMinutes(10); got != 10*time.Minute {
		t.Errorf("timeoutMinutes(10) = %v, want 10m", got)
	}
}
