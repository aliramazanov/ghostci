package runner

import (
	"os"
	"runtime"
	"strings"
)

type Context struct {
	Repository string
	Ref        string
	RefName    string
	Head       string
	Workspace  string
}

func (c Context) Env() []string {
	return c.env(func(key string) bool {
		_, set := os.LookupEnv(key)

		return set
	})
}

func (c Context) env(alreadySet func(string) bool) []string {
	vars := [][2]string{
		{"CI", "true"},
		{"GITHUB_ACTIONS", "true"},
		{"GITHUB_EVENT_NAME", "push"},
		{"GITHUB_REPOSITORY", c.Repository},
		{"GITHUB_REPOSITORY_OWNER", owner(c.Repository)},
		{"GITHUB_REF", c.Ref},
		{"GITHUB_REF_NAME", c.RefName},
		{"GITHUB_REF_TYPE", refType(c.Ref)},
		{"GITHUB_SHA", c.Head},
		{"GITHUB_WORKSPACE", c.Workspace},
		{"RUNNER_OS", runnerOS()},
		{"RUNNER_ARCH", runnerArch()},
		{"RUNNER_TEMP", os.TempDir()},
	}

	out := make([]string, 0, len(vars))

	for _, kv := range vars {
		if kv[1] == "" || alreadySet(kv[0]) {
			continue
		}

		out = append(out, kv[0]+"="+kv[1])
	}

	return out
}

func owner(repository string) string {
	name, _, ok := strings.Cut(repository, "/")
	if !ok {
		return ""
	}

	return name
}

func refType(ref string) string {
	switch {
	case strings.HasPrefix(ref, "refs/tags/"):
		return "tag"
	case strings.HasPrefix(ref, "refs/heads/"):
		return "branch"
	}

	return ""
}

func runnerOS() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}

	return ""
}

func runnerArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "X64"
	case "arm64":
		return "ARM64"
	case "386":
		return "X86"
	case "arm":
		return "ARM"
	}

	return ""
}
