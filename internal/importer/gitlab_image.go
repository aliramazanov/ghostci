package importer

import (
	"fmt"
	"strings"

	"github.com/aliramazanov/ghostci/internal/gitlab"
	"github.com/aliramazanov/ghostci/internal/pipeline"
	"go.yaml.in/yaml/v3"
)

func imageName(node yaml.Node) string {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "name" {
				return node.Content[i+1].Value
			}
		}
	}

	return ""
}

func imageToolchain(image string) (tool, version string) {
	repo, tag, _ := strings.Cut(image, ":")
	tag, _, _ = strings.Cut(tag, "@")

	if slash := strings.LastIndex(repo, "/"); slash >= 0 {
		repo = repo[slash+1:]
	}

	known := map[string]string{
		"golang": "go", "go": "go",
		"node": "node", "nodejs": "node",
		"python": "python", "rust": "rust", "ruby": "ruby",
		"openjdk": "java", "eclipse-temurin": "java", "maven": "java", "gradle": "java",
		"deno": "deno", "bun": "bun",
	}

	tool, ok := known[strings.ToLower(repo)]
	if !ok {
		return "", ""
	}

	version = strings.TrimSuffix(strings.TrimSuffix(tag, "-alpine"), "-slim")
	if version == "latest" {
		version = ""
	}

	return tool, version
}

func matrixNode(node yaml.Node) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "matrix" {
			return node.Content[i+1]
		}
	}

	return nil
}

func matrixAxes(entry *yaml.Node) []pipeline.Axis {
	axes := make([]pipeline.Axis, 0, len(entry.Content)/2)

	for i := 0; i+1 < len(entry.Content); i += 2 {
		var values []any

		switch entry.Content[i+1].Kind {
		case yaml.ScalarNode:
			values = []any{entry.Content[i+1].Value}
		case yaml.SequenceNode:
			for _, v := range entry.Content[i+1].Content {
				values = append(values, v.Value)
			}
		}

		axes = append(axes, pipeline.Axis{Key: entry.Content[i].Value, Values: values})
	}

	return axes
}

func parallelMatrix(job gitlab.Job) ([]gitlab.Variables, error) {
	matrix := matrixNode(job.Parallel)
	if matrix == nil || matrix.Kind != yaml.SequenceNode {
		return []gitlab.Variables{nil}, nil
	}

	var out []gitlab.Variables

	for _, entry := range matrix.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}

		expanded, truncated := pipeline.Cartesian(matrixAxes(entry))
		if truncated {
			return nil, fmt.Errorf("parallel:matrix is too large for ghostci to expand")
		}

		for _, c := range expanded {
			vars := make(gitlab.Variables, len(c))
			for k, v := range c {
				vars[k] = fmt.Sprint(v)
			}

			out = append(out, vars)
		}
	}

	if len(out) == 0 {
		return []gitlab.Variables{nil}, nil
	}

	if len(out) > pipeline.MaxCombinations {
		return nil, fmt.Errorf("parallel:matrix expands to %d jobs, more than ghostci will generate", len(out))
	}

	return out, nil
}
