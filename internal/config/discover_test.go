package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("checks: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindInPlace(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ghostci.yaml"))

	if got := Find(dir); got != "ghostci.yaml" {
		t.Errorf("Find = %q, want ghostci.yaml", got)
	}
}

func TestFindAcceptsYml(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ghostci.yml"))

	if got := Find(dir); got != "ghostci.yml" {
		t.Errorf("Find = %q, want ghostci.yml", got)
	}
}

func TestFindWalksUp(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ghostci.yaml"))

	sub := filepath.Join(dir, "services", "api")

	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join("..", "..", "ghostci.yaml")

	if got := Find(sub); got != want {
		t.Errorf("Find = %q, want %q", got, want)
	}
}

func TestFindStopsAtRepositoryRoot(t *testing.T) {
	outer := t.TempDir()
	write(t, filepath.Join(outer, "ghostci.yaml"))

	inner := filepath.Join(outer, "nested")

	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := Find(inner); got != "ghostci.yaml" {
		t.Errorf("Find = %q, want the unfound default", got)
	}

	if _, err := os.Stat(filepath.Join(inner, "ghostci.yaml")); err == nil {
		t.Fatal("the test fixture is wrong: inner config exists")
	}
}

func TestFindPrefersYaml(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ghostci.yaml"))
	write(t, filepath.Join(dir, "ghostci.yml"))

	if got := Find(dir); got != "ghostci.yaml" {
		t.Errorf("Find = %q, want ghostci.yaml", got)
	}
}
