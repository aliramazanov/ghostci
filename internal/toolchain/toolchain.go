package toolchain

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/aliramazanov/ghostci/internal/shell"
)

var probes = map[string][]string{
	"go":     {"go", "version"},
	"node":   {"node", "--version"},
	"python": {"python3", "--version"},
	"java":   {"java", "-version"},
	"rust":   {"rustc", "--version"},
	"deno":   {"deno", "--version"},
	"bun":    {"bun", "--version"},
	"ruby":   {"ruby", "--version"},
	"uv":     {"uv", "--version"},
}

var versionPattern = regexp.MustCompile(`\d+(\.\d+)+`)

var pinPattern = regexp.MustCompile(`^\d+(\.\d+)*$`)

func Local(name string) string {
	args, ok := probes[normalise(name)]
	if !ok {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	if err != nil {
		return ""
	}
	return versionPattern.FindString(string(out))
}

func normaliseVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func Matches(pinned, local string) bool {
	if pinned == "" || local == "" {
		return true
	}

	pinned = normaliseVersion(pinned)
	local = normaliseVersion(local)

	pinned = strings.TrimSuffix(pinned, ".x")
	pinned = strings.TrimSuffix(pinned, ".*")

	switch pinned {
	case "stable", "latest", "lts", "nightly", "beta", "":
		return true
	}
	if !pinPattern.MatchString(pinned) {
		return true
	}

	want := strings.Split(pinned, ".")
	got := strings.Split(local, ".")
	if len(got) < len(want) {
		return false
	}
	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}
	return true
}

var commandToolchains = map[string][]string{
	"go": {"go"}, "gofmt": {"go"}, "golangci-lint": {"go"}, "staticcheck": {"go"},
	"cargo": {"rust"}, "rustc": {"rust"}, "clippy-driver": {"rust"},
	"npx": {"node"}, "npm": {"node"}, "pnpm": {"node"}, "yarn": {"node"},
	"node": {"node"}, "eslint": {"node"}, "tsc": {"node"}, "vitest": {"node"}, "jest": {"node"},
	"bun": {"bun"}, "deno": {"deno"},
	"python": {"python"}, "python3": {"python"}, "pytest": {"python"},
	"mypy": {"python"}, "ruff": {"python"}, "pylint": {"python"}, "uv": {"python"},
	"poetry": {"python"}, "pipenv": {"python"}, "hatch": {"python"}, "pdm": {"python"}, "rye": {"python"},
	"mvn": {"java"}, "gradle": {"java"}, "javac": {"java"},
	"ruby": {"ruby"}, "rubocop": {"ruby"}, "rspec": {"ruby"}, "bundle": {"ruby"},
}

func ForCommand(command string) []string {
	seen := map[string]bool{}

	var out []string

	for _, word := range shell.Invoked(command) {
		for _, tc := range commandToolchains[word] {
			if !seen[tc] {
				seen[tc] = true
				out = append(out, tc)
			}
		}
	}

	return out
}

type Resolver struct{ cache map[string]string }

func NewResolver() *Resolver { return &Resolver{cache: map[string]string{}} }

func (r *Resolver) Versions(command string) map[string]string {
	out := map[string]string{}
	for _, name := range ForCommand(command) {
		v, ok := r.cache[name]
		if !ok {
			v = Local(name)
			r.cache[name] = v
		}
		if v != "" {
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalise(name string) string {
	name = strings.ToLower(name)
	if _, known := probes[name]; known {
		return name
	}

	for _, token := range strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if token == "javascript" || token == "js" {
			return "node"
		}
		for _, tc := range []string{"python", "rust", "node", "java", "ruby", "deno", "bun", "uv", "go"} {

			if token == tc || token == tc+"lang" || (len(tc) >= 4 && strings.HasPrefix(token, tc)) {
				return tc
			}
		}
	}

	return name
}
