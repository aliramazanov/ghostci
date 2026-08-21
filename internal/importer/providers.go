package importer

import (
	"os"
	"path/filepath"

	"github.com/aliramazanov/ghostci/internal/pipeline"
)

func init() {
	pipeline.Register(githubProvider{})
	pipeline.Register(gitlabProvider{})
	pipeline.Register(azureProvider{})
	pipeline.Register(circleProvider{})
	pipeline.Register(bitbucketProvider{})
}

type githubProvider struct{}

func (githubProvider) Name() string { return "GitHub Actions" }

func (githubProvider) Events() []string { return []string{"push", "pull_request"} }

func (githubProvider) Detect(dir string) (string, bool) {
	workflows := filepath.Join(dir, ".github", "workflows")

	matches, err := filepath.Glob(filepath.Join(workflows, "*.y*ml"))
	if err != nil || len(matches) == 0 {
		return "", false
	}

	return workflows, true
}

type gitlabProvider struct{}

func (gitlabProvider) Name() string { return "GitLab CI" }

func (gitlabProvider) Events() []string { return pipelineSources }

func (gitlabProvider) Detect(dir string) (string, bool) {
	path := filepath.Join(dir, GitLabFile)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	return path, true
}

type azureProvider struct{}

func (azureProvider) Name() string { return "Azure Pipelines" }

func (azureProvider) Events() []string { return []string{"push", "pr"} }

func (azureProvider) Detect(dir string) (string, bool) {
	for _, name := range AzureFiles {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return "", false
}

type circleProvider struct{}

func (circleProvider) Name() string { return "CircleCI" }

func (circleProvider) Events() []string { return []string{"push"} }

func (circleProvider) Detect(dir string) (string, bool) {
	path := filepath.Join(dir, filepath.FromSlash(CircleFile))
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	return path, true
}

type bitbucketProvider struct{}

func (bitbucketProvider) Name() string { return "Bitbucket Pipelines" }

func (bitbucketProvider) Events() []string { return []string{"push", "pull_request"} }

func (bitbucketProvider) Detect(dir string) (string, bool) {
	path := filepath.Join(dir, BitbucketFile)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	return path, true
}

func Import(path string, a Assumptions) (*Result, error) {
	base := filepath.Base(path)

	switch {
	case base == GitLabFile:
		return ImportGitLab(path, a)
	case base == BitbucketFile:
		return ImportBitbucket(path, a)
	case base == filepath.Base(CircleFile) && filepath.Base(filepath.Dir(path)) == ".circleci":
		return ImportCircle(path, a)
	case isAzureFile(base):
		return ImportAzure(path, a)
	}

	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return ImportFile(path, a)
	}

	return ImportDir(path, a)
}

func Detect(dir string) (name, path string, found bool) {
	p, path, ok := pipeline.Detect(dir)
	if !ok {
		return "", "", false
	}

	return p.Name(), path, true
}

func refusalOutcome(r pipeline.Refusal) Outcome {
	switch r {
	case pipeline.ServerState:
		return NeedsReview
	case pipeline.NotAutomatic:
		return SkippedByCondition
	}

	return UnsupportedFeature
}

func isAzureFile(base string) bool {
	for _, name := range AzureFiles {
		if base == name {
			return true
		}
	}

	return false
}
