package runner

import (
	"os"
	"path/filepath"
)

var githubFiles = []string{
	"GITHUB_OUTPUT",
	"GITHUB_ENV",
	"GITHUB_PATH",
	"GITHUB_STATE",
	"GITHUB_STEP_SUMMARY",
}

func scratchEnv() (vars []string, cleanup func()) {
	dir, err := os.MkdirTemp("", "ghostci-step-")
	if err != nil {
		return nil, func() {}
	}

	for _, name := range githubFiles {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			continue
		}

		vars = append(vars, name+"="+path)
	}

	return vars, func() { _ = os.RemoveAll(dir) }
}
