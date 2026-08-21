package importer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/pipeline"
	"go.yaml.in/yaml/v3"
)

const BitbucketFile = "bitbucket-pipelines.yml"

const bitbucketShell = "bash -e"

type bitbucketFile struct {
	Image     yaml.Node         `yaml:"image"`
	Options   yaml.Node         `yaml:"options"`
	Pipelines bitbucketSections `yaml:"pipelines"`
}

type bitbucketSections struct {
	Default      []yaml.Node            `yaml:"default"`
	Branches     map[string][]yaml.Node `yaml:"branches"`
	PullRequests map[string][]yaml.Node `yaml:"pull-requests"`
	Custom       map[string][]yaml.Node `yaml:"custom"`
	Tags         map[string][]yaml.Node `yaml:"tags"`
}

type bitbucketStep struct {
	Name     string      `yaml:"name"`
	Script   []yaml.Node `yaml:"script"`
	Image    yaml.Node   `yaml:"image"`
	Services []string    `yaml:"services"`
	Caches   []string    `yaml:"caches"`
	Trigger  string      `yaml:"trigger"`
	Deploy   string      `yaml:"deployment"`

	MaxTime int               `yaml:"max-time"`
	Env     map[string]string `yaml:"environment"`
}

func ImportBitbucket(path string, a Assumptions) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file bitbucketFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	im := &bitbucketImport{ledger{res: &Result{Bitbucket: true}, label: filepath.Base(path)}}

	im.walk("default", file.Pipelines.Default)

	for _, pattern := range sortedKeys(file.Pipelines.Branches) {
		im.walk("branches/"+pattern, file.Pipelines.Branches[pattern])
	}
	for _, pattern := range sortedKeys(file.Pipelines.PullRequests) {
		im.walk("pull-requests/"+pattern, file.Pipelines.PullRequests[pattern])
	}

	for _, name := range sortedKeys(file.Pipelines.Custom) {
		im.refuse("custom/"+name, pipeline.NotAutomatic, "custom pipelines are started by hand")
	}
	for _, name := range sortedKeys(file.Pipelines.Tags) {
		im.refuse("tags/"+name, pipeline.NotAutomatic, "runs only when a tag is pushed")
	}

	dedupeNames(im.res)

	return im.res, nil
}

type bitbucketImport struct {
	ledger
}

func (im *bitbucketImport) walk(section string, nodes []yaml.Node) {
	for i, node := range nodes {
		if node.Kind != yaml.MappingNode || len(node.Content) < 2 {
			continue
		}

		key, value := node.Content[0].Value, node.Content[1]

		switch key {
		case "step":
			im.importStep(section, i, *value)
		case "parallel":

			im.walkParallel(section, *value)
		case "stage":
			im.refuse(section, pipeline.Unreadable, "stages are not expanded yet")
		case "variables":
			im.refuse(section, pipeline.NotAutomatic, "the pipeline asks for input when it starts")
		}
	}
}

func (im *bitbucketImport) walkParallel(section string, node yaml.Node) {
	switch node.Kind {
	case yaml.SequenceNode:
		steps := make([]yaml.Node, 0, len(node.Content))
		for _, item := range node.Content {
			steps = append(steps, *item)
		}
		im.walk(section, steps)
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "steps" {
				steps := make([]yaml.Node, 0, len(node.Content[i+1].Content))
				for _, item := range node.Content[i+1].Content {
					steps = append(steps, *item)
				}
				im.walk(section, steps)
			}
		}
	}
}

func (im *bitbucketImport) importStep(section string, index int, node yaml.Node) {
	var step bitbucketStep
	if err := node.Decode(&step); err != nil {
		im.refuse(section, pipeline.Unreadable, "a step could not be read: "+err.Error())

		return
	}

	name := firstNonEmpty(step.Name, section)

	switch {
	case len(step.Services) > 0:
		im.refuse(name, pipeline.NeedsContainer,
			"needs the service containers "+strings.Join(step.Services, ", "))

		return
	case step.Trigger == "manual":
		im.refuse(name, pipeline.NotAutomatic, "the step waits for someone to start it")

		return
	case step.Deploy != "":
		im.refuse(name, pipeline.ServerState,
			"deploys to the "+step.Deploy+" environment, which is configured in Bitbucket")

		return
	}

	var lines []string

	for _, item := range step.Script {
		switch item.Kind {
		case yaml.ScalarNode:
			lines = append(lines, item.Value)
		case yaml.MappingNode:

			for i := 0; i+1 < len(item.Content); i += 2 {
				if item.Content[i].Value == "pipe" {
					im.refuse(name, pipeline.OpaqueUnit,
						"uses the pipe "+item.Content[i+1].Value+", which runs as a published container")

					return
				}
			}
		}
	}

	if len(lines) == 0 {
		return
	}

	command := strings.Join(lines, "\n")

	if imageName(step.Image) != "" {
		im.recordToolchainFrom(imageName(step.Image), "image ")
	}

	chk := config.Check{
		Name:    slug(name) + "-" + itoa(index),
		Command: command,
		Shell:   bitbucketShell,
		Env:     step.Env,
		Inputs:  inferInputs(command),
		Timeout: config.Duration(timeoutMinutes(step.MaxTime)),
	}

	entry := Entry{
		Workflow: im.label, Job: section, Step: name,
		Outcome: Extracted, Command: command,
	}
	entry.Heavy = routeCheck(im.res, section, chk)
	im.res.Entries = append(im.res.Entries, entry)
}
