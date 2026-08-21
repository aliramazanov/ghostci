package config

import (
	"os"
	"path/filepath"
)

var Names = []string{"ghostci.yaml", "ghostci.yml"}

func Find(dir string) string {
	start, err := filepath.Abs(dir)

	if err != nil {
		return Names[0]
	}

	for at := start; ; {
		for _, name := range Names {
			path := filepath.Join(at, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return display(start, path)
			}
		}

		parent := filepath.Dir(at)

		if isRepoRoot(at) || parent == at {
			return Names[0]
		}

		at = parent
	}
}

func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))

	return err == nil
}

func display(from, path string) string {
	rel, err := filepath.Rel(from, path)

	if err != nil {
		return path
	}

	return rel
}
