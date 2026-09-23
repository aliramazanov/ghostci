package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var managerInvoked = regexp.MustCompile(`(?:^|[;&|]|\n)\s*(npm|pnpm|yarn|bun)\b([^\n;&|]*)`)

var installOnly = map[string]bool{
	"ci": true, "install": true, "i": true, "install-frozen-lockfile": true,
}

var manifestGlobs = []string{
	"package.json", "package-lock.json", "npm-shrinkwrap.json",
	"yarn.lock", "pnpm-lock.yaml", "bun.lockb", "bun.lock",
}

func scriptInputs(root, dir, command string, depth int) ([]string, bool) {
	calls := managerInvoked.FindAllStringSubmatch(command, -1)
	if len(calls) == 0 {
		return nil, false
	}

	scripts, haveManifest := packageScripts(root, dir)

	var out []string
	for _, call := range calls {
		name, ok := scriptName(call[2])
		if !ok {
			return nil, false
		}

		if installOnly[name] {
			out = append(out, scoped(manifestGlobs, dir)...)

			continue
		}

		if !haveManifest {
			return nil, false
		}

		body, ok := scripts[name]
		if !ok {
			return nil, false
		}

		inner := inferAt(root, dir, body, depth+1)
		if len(inner) == 0 {
			return nil, false
		}

		out = append(out, inner...)
		out = append(out, scoped(manifestGlobs, dir)...)
	}

	return out, len(out) > 0
}

var elsewhere = map[string]bool{
	"-w": true, "--workspace": true, "--workspaces": true, "-ws": true,
	"--prefix": true, "-C": true, "--dir": true, "--cwd": true,
	"--filter": true, "-F": true, "-r": true, "--recursive": true,
	"--include-workspace-root": true,
}

func scriptName(args string) (string, bool) {
	fields := strings.Fields(args)

	var words []string
	passthrough := false
	for _, f := range fields {
		if f == "--" {
			passthrough = true
		}
		if flag, _, _ := strings.Cut(f, "="); !passthrough && elsewhere[flag] {
			return "", false
		}
		if strings.HasPrefix(f, "-") || strings.Contains(f, "=") {
			continue
		}
		words = append(words, f)
	}

	switch {
	case len(words) == 0:

		return "install", true
	case words[0] == "run" || words[0] == "run-script" || words[0] == "exec":
		if len(words) < 2 {
			return "", false
		}

		return words[1], true
	case len(words) == 1:
		return words[0], true
	}

	return "", false
}

func packageScripts(root, dir string) (map[string]string, bool) {
	if root == "" {
		return nil, false
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "package.json"))
	if err != nil {
		return nil, false
	}

	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, false
	}

	return manifest.Scripts, true
}
