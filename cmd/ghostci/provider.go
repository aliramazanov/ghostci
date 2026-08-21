package main

import (
	"path/filepath"

	"github.com/aliramazanov/ghostci/internal/git"
	"github.com/aliramazanov/ghostci/internal/importer"
)

func resolveSource(dir, override string) (path string, detected bool) {
	if override != "" {
		return override, true
	}

	if _, path, ok := importer.Detect("."); ok {
		return path, true
	}

	return dir, false
}

func localAssumptions() importer.Assumptions {
	a := importer.DefaultAssumptions()

	repo := git.Discover(".")

	if repo.Ref != "" {
		a.Ref = repo.Ref
	}

	if repo.Repository != "" {
		a.Repository = repo.Repository
	}

	a.DefaultBranch = repo.DefaultBranch

	return a
}

func describeSource(path string) string { return filepath.Clean(path) }
