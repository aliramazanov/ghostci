package hook

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return dir
}

func setConfig(t *testing.T, dir, key, value string) {
	t.Helper()

	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config %s: %v\n%s", key, err, out)
	}
}

func TestEmptyHooksPathIsRefused(t *testing.T) {
	dir := gitRepo(t)
	setConfig(t, dir, "core.hooksPath", "")

	if _, err := Dir(dir); !errors.Is(err, ErrEmptyHooksPath) {
		t.Fatalf("Dir = %v, want ErrEmptyHooksPath", err)
	}
	if _, err := Install(dir, true); !errors.Is(err, ErrEmptyHooksPath) {
		t.Fatalf("Install = %v, want ErrEmptyHooksPath even with --force", err)
	}
	if _, err := os.Stat(filepath.Join(dir, Name)); err == nil {
		t.Error("a pre-push file was written into the working tree")
	}
}

func TestRelativeHooksPathResolvesAgainstTheWorkTree(t *testing.T) {
	dir := gitRepo(t)
	setConfig(t, dir, "core.hooksPath", ".githooks")

	got, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := realPath(filepath.Join(dir, ".githooks"))
	if got := realPath(got); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
}
