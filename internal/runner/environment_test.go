package runner

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func lookup(env []string, key string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}

	return "", false
}

func nothingSet(string) bool { return false }

func TestContextExportsCI(t *testing.T) {
	ctx := Context{
		Repository: "aliramazanov/ghostci",
		Ref:        "refs/heads/main",
		RefName:    "main",
		Head:       "0123456789abcdef",
		Workspace:  "/repo",
	}

	env := ctx.env(nothingSet)

	for key, want := range map[string]string{
		"CI":                      "true",
		"GITHUB_ACTIONS":          "true",
		"GITHUB_REPOSITORY":       "aliramazanov/ghostci",
		"GITHUB_REPOSITORY_OWNER": "aliramazanov",
		"GITHUB_REF_TYPE":         "branch",
		"GITHUB_SHA":              "0123456789abcdef",
		"GITHUB_WORKSPACE":        "/repo",
	} {
		if got, ok := lookup(env, key); !ok || got != want {
			t.Errorf("%s = %q (present %v), want %q", key, got, ok, want)
		}
	}
}

func TestContextOmitsUnknowns(t *testing.T) {
	env := Context{}.env(nothingSet)

	for _, key := range []string{"GITHUB_REPOSITORY", "GITHUB_REF", "GITHUB_SHA", "GITHUB_REF_TYPE"} {
		if _, ok := lookup(env, key); ok {
			t.Errorf("%s was exported with no repository to read it from", key)
		}
	}
	if _, ok := lookup(env, "CI"); !ok {
		t.Error("CI should be exported even outside a repository")
	}
}

func TestContextDefersToTheEnvironment(t *testing.T) {
	t.Setenv("GITHUB_SHA", "already-set")

	env := Context{Head: "ours"}.Env()

	if got, ok := lookup(env, "GITHUB_SHA"); ok {
		t.Errorf("GITHUB_SHA was overridden with %q", got)
	}
}

func TestTagRefType(t *testing.T) {
	env := Context{Ref: "refs/tags/v1.0.0"}.env(nothingSet)

	if got, _ := lookup(env, "GITHUB_REF_TYPE"); got != "tag" {
		t.Errorf("GITHUB_REF_TYPE = %q, want tag", got)
	}
}

func TestCommandCarriesContext(t *testing.T) {
	t.Setenv("GITHUB_SHA", "restored-after")
	os.Unsetenv("GITHUB_SHA")

	opts := Options{Context: Context{Head: "abc"}}
	opts.applyDefaults()

	cmd := command(config.Check{Name: "hi", Command: "echo hi"}, opts, nil)

	if got, _ := lookup(cmd.Env, "GITHUB_SHA"); got != "abc" {
		t.Fatalf("GITHUB_SHA = %q, want abc", got)
	}
	if !slices.ContainsFunc(cmd.Env, func(kv string) bool { return strings.HasPrefix(kv, "PATH=") }) {
		t.Error("the ambient environment was dropped")
	}
}
