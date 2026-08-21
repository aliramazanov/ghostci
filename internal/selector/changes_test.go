package selector

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/git"
)

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main", ".")
	run(t, dir, "config", "user.email", "t@t.t")
	run(t, dir, "config", "user.name", "t")
	write(t, dir, "a.go", "package a\n")
	write(t, dir, "README.md", "# hi\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "init")
	return dir
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectChangesOutsideRepo(t *testing.T) {
	t.Parallel()
	c := DetectChanges(git.NewSession(t.TempDir()), "")
	if c.Complete {
		t.Error("outside a repo the change set cannot be complete")
	}
	if !strings.Contains(c.Reason, "running every check") {
		t.Errorf("reason = %q", c.Reason)
	}
}

func TestDetectChangesAgainstExplicitBase(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	base := run(t, dir, "rev-parse", "HEAD")

	write(t, dir, "b.go", "package a\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "second")

	c := DetectChanges(git.NewSession(dir), base)
	if !c.Complete {
		t.Fatalf("change set incomplete: %s", c.Reason)
	}
	if !slices.Contains(c.Files, "b.go") {
		t.Errorf("committed change missing: %v", c.Files)
	}
	if slices.Contains(c.Files, "a.go") {
		t.Errorf("unchanged file reported as changed: %v", c.Files)
	}
}

func TestUncommittedAndUntrackedCount(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	base := run(t, dir, "rev-parse", "HEAD")

	write(t, dir, "a.go", "package a\n\nfunc X() {}\n")
	write(t, dir, "brand-new.go", "package a\n")

	c := DetectChanges(git.NewSession(dir), base)
	if !c.Complete {
		t.Fatalf("incomplete: %s", c.Reason)
	}
	for _, want := range []string{"a.go", "brand-new.go"} {
		if !slices.Contains(c.Files, want) {
			t.Errorf("%s missing from %v", want, c.Files)
		}
	}
}

func TestUnresolvableBaseRunsEverything(t *testing.T) {
	t.Parallel()
	c := DetectChanges(git.NewSession(repo(t)), "0000000000000000000000000000000000000000")
	if c.Complete {
		t.Error("an unknown base must not produce a complete change set")
	}
	if !strings.Contains(c.Reason, "running every check") {
		t.Errorf("reason = %q", c.Reason)
	}
}

func TestNoUpstreamFallsBack(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	c := DetectChanges(git.NewSession(dir), "")
	if c.Complete && len(c.Files) == 0 {
		t.Error("without an upstream, an empty complete change set would skip everything")
	}
}

func TestMergeBaseWithDefaultBranch(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "feature.go", "package a\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "feature work")

	c := DetectChanges(git.NewSession(dir), "")
	if !c.Complete {
		t.Fatalf("should resolve via merge-base with main: %s", c.Reason)
	}
	if !slices.Contains(c.Files, "feature.go") {
		t.Errorf("feature commit missing: %v", c.Files)
	}
	if !strings.Contains(c.Reason, "merge-base") {
		t.Errorf("reason should name the fallback: %q", c.Reason)
	}
}

func TestARefThatLooksLikeAnOptionRunsEverything(t *testing.T) {
	t.Parallel()
	dir := repo(t)

	for _, base := range []string{"-x", "--output=/tmp/ghostci-selector-should-not-write"} {
		c := DetectChanges(git.NewSession(dir), base)

		if c.Complete {
			t.Errorf("DetectChanges(%q) reported a complete change set", base)
		}
		if len(c.Files) != 0 {
			t.Errorf("DetectChanges(%q) claimed to know the changed files: %v", base, c.Files)
		}
	}

	if _, err := os.Stat("/tmp/ghostci-selector-should-not-write"); err == nil {
		os.Remove("/tmp/ghostci-selector-should-not-write")
		t.Fatal("the ref reached git as an option and wrote a file")
	}
}
