package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestACheckThatRewritesTheTreeIsNotAnAllClear(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: verify
    command: "grep -q GOOD src/data.txt"
    inputs: ["src/**"]
  - name: mutate
    command: "sleep 0.2; echo BAD > src/data.txt"
    inputs: ["trigger/**"]
`)
	r.write("src/data.txt", "GOOD\n")
	r.write("trigger/t", "1\n")
	r.commit("seed")

	got := r.ghostci("", "--all")

	body, err := os.ReadFile(filepath.Join(r.dir, "src/data.txt"))

	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(body), "GOOD") {
		t.Fatal("the mutating check never ran, so this proves nothing")
	}

	if got.code == 0 {
		t.Errorf("exit 0 for a tree that would fail its own check:\n%s", got.out)
	}

	if strings.Contains(got.out, "\n  all clear.\n") {
		t.Errorf("a rewritten tree was reported all clear:\n%s", got.out)
	}

	if !strings.Contains(got.out, "verify") {
		t.Errorf("the invalidated check is not named:\n%s", got.out)
	}
}

func TestStaleIsReportedInJSON(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: verify
    command: "true"
    inputs: ["src/**"]
  - name: mutate
    command: "sleep 0.2; echo CHANGED > src/data.txt"
    inputs: ["trigger/**"]
`)
	r.write("src/data.txt", "GOOD\n")
	r.write("trigger/t", "1\n")
	r.commit("seed")

	got := r.ghostci("", "--all", "--json")

	var doc struct {
		Stale  []string `json:"stale"`
		Failed bool     `json:"failed"`
	}

	if err := json.Unmarshal([]byte(got.out), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, got.out)
	}

	if len(doc.Stale) == 0 {
		t.Fatalf("stale checks are absent from the JSON report:\n%s", got.out)
	}
}

func TestAQuietRunIsStillAllClear(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: reads
    command: "grep -q GOOD src/data.txt"
    inputs: ["src/**"]
  - name: writes-elsewhere
    command: "echo scratch > /dev/null"
    inputs: ["src/**"]
`)
	r.write("src/data.txt", "GOOD\n")
	r.commit("seed")

	got := r.ghostci("", "--all")

	if got.code != 0 {
		t.Fatalf("a run that changed nothing exited %d:\n%s", got.code, got.out)
	}

	if !strings.Contains(got.out, "all clear") {
		t.Fatalf("a clean run was not reported clear:\n%s", got.out)
	}
}

func TestFilesMerelyCreatedDoNotMakeARunStale(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: reads
    command: "grep -q GOOD src/data.txt"
    inputs: ["src/**"]
  - name: writes-artifacts
    command: "mkdir -p src/__pycache__ && echo cache > src/__pycache__/x.pyc"
    inputs: ["src/**"]
`)
	r.write("src/data.txt", "GOOD\n")
	r.commit("seed")

	got := r.ghostci("", "--all")
	if got.code != 0 {
		t.Fatalf("creating build artifacts failed the run: exit %d\n%s", got.code, got.out)
	}
	if !strings.Contains(got.out, "all clear") {
		t.Errorf("a run that only created artifacts was not clear:\n%s", got.out)
	}

	r.write("ghostci.yaml", `
checks:
  - name: reads
    command: "grep -q GOOD src/data.txt"
    inputs: ["src/**"]
  - name: rewrites
    command: "sleep 0.2; echo BAD > src/data.txt"
    inputs: ["trigger/**"]
`)
	r.write("trigger/t", "1")
	r.commit("rewriting check")

	again := r.ghostci("", "--all")
	if again.code == 0 {
		t.Errorf("a check that rewrote another's input was reported clean:\n%s", again.out)
	}
}
