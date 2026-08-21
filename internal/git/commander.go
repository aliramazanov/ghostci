package git

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const timeout = 30 * time.Second

func Run(dir string, args ...string) (string, error) {
	out, err := Raw(dir, args...)

	return strings.TrimSpace(out), err
}

func Raw(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append(options(), args...)...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv()

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func options() []string {
	return []string{
		"-c", "core.fsmonitor=false",
		"-c", "core.useBuiltinFSMonitor=false",
	}
}

var repoLocalEnv = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_CONFIG",
	"GIT_DIR",
	"GIT_GRAFT_FILE",
	"GIT_IMPLICIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_INTERNAL_SUPER_PREFIX",
	"GIT_NO_REPLACE_OBJECTS",
	"GIT_OBJECT_DIRECTORY",
	"GIT_PREFIX",
	"GIT_REPLACE_REF_BASE",
	"GIT_SHALLOW_FILE",
	"GIT_WORK_TREE",
}

func sanitizedEnv() []string {
	drop := make(map[string]bool, len(repoLocalEnv))
	for _, k := range repoLocalEnv {
		drop[k] = true
	}

	env := os.Environ()
	out := make([]string, 0, len(env)+2)

	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if ok && drop[key] {
			continue
		}

		out = append(out, kv)
	}

	return append(out, "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=")
}
