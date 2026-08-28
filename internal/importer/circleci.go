package importer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/pipeline"
	"go.yaml.in/yaml/v3"
)

const CircleFile = ".circleci/config.yml"

const circleShell = "/bin/bash -eo pipefail"

type circleConfig struct {
	Version   yaml.Node            `yaml:"version"`
	Setup     bool                 `yaml:"setup"`
	Orbs      map[string]yaml.Node `yaml:"orbs"`
	Jobs      map[string]circleJob `yaml:"jobs"`
	Workflows map[string]yaml.Node `yaml:"workflows"`
	Commands  map[string]yaml.Node `yaml:"commands"`
	Params    map[string]yaml.Node `yaml:"parameters"`
	Executors map[string]yaml.Node `yaml:"executors"`
}

type circleJob struct {
	Docker      []circleImage     `yaml:"docker"`
	Machine     yaml.Node         `yaml:"machine"`
	Executor    yaml.Node         `yaml:"executor"`
	Parallelism int               `yaml:"parallelism"`
	Environment map[string]string `yaml:"environment"`
	Steps       []yaml.Node       `yaml:"steps"`
}

type circleImage struct {
	Image string `yaml:"image"`
}

type circleRun struct {
	Name             string            `yaml:"name"`
	Command          string            `yaml:"command"`
	Shell            string            `yaml:"shell"`
	WorkingDirectory string            `yaml:"working_directory"`
	Environment      map[string]string `yaml:"environment"`
	When             string            `yaml:"when"`

	NoOutputTimeout string `yaml:"no_output_timeout"`
}

func ImportCircle(path string, a Assumptions) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg circleConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	im := &circleImport{ledger{res: &Result{Circle: true}, label: filepath.Base(path)}}

	if cfg.Setup {
		im.refuse("(setup)", pipeline.ServerState,
			"this is a dynamic setup config; the pipeline it generates does not exist until CI runs it")

		return im.res, nil
	}

	for name := range cfg.Orbs {
		im.refuse(name, pipeline.OpaqueUnit,
			"orb "+name+" is resolved from CircleCI's registry, so its commands are not visible here")
	}

	for _, name := range sortedKeys(cfg.Jobs) {
		im.importJob(name, cfg.Jobs[name])
	}

	finish(im.res)

	return im.res, nil
}

type circleImport struct {
	ledger
}

func (im *circleImport) importJob(name string, job circleJob) {

	if len(job.Docker) > 1 {
		im.refuse(name, pipeline.NeedsContainer, "needs service containers")

		return
	}
	if !job.Machine.IsZero() {
		im.refuse(name, pipeline.NeedsContainer, "needs a machine executor")

		return
	}
	if job.Parallelism > 1 {

		im.note(name, "parallelism splits this job across runners in CI; the whole job runs here")
	}

	if len(job.Docker) == 1 {
		im.recordToolchainFrom(job.Docker[0].Image, "docker image ")
	}

	for i, step := range job.Steps {
		im.importStep(name, i, step, job.Environment)
	}
}

func (im *circleImport) importStep(job string, index int, node yaml.Node, jobEnv map[string]string) {

	if node.Kind == yaml.ScalarNode {
		switch node.Value {
		case "checkout", "setup_remote_docker":
			return
		}
		im.refuse(job, pipeline.OpaqueUnit, "step "+node.Value+" is defined outside this config")

		return
	}
	if node.Kind != yaml.MappingNode || len(node.Content) < 2 {
		return
	}

	key := node.Content[0].Value
	value := node.Content[1]

	switch key {
	case "save_cache", "restore_cache", "persist_to_workspace", "attach_workspace",
		"store_artifacts", "store_test_results", "add_ssh_keys":
		return
	case "run":
	default:
		im.refuse(job, pipeline.OpaqueUnit, "step "+key+" is defined outside this config")

		return
	}

	var run circleRun
	switch value.Kind {
	case yaml.ScalarNode:
		run.Command = value.Value
	case yaml.MappingNode:
		if err := value.Decode(&run); err != nil {
			im.refuse(job, pipeline.Unreadable, "a run step could not be read: "+err.Error())

			return
		}
	}

	if run.Command == "" {
		return
	}
	if strings.Contains(run.Command, "<<") {
		im.refuse(job, pipeline.ServerState,
			"the command contains a pipeline parameter that resolves when CircleCI compiles the config")

		return
	}
	if run.When != "" && run.When != "always" && run.When != "on_success" {
		im.refuse(job, pipeline.NotAutomatic, "the step runs only when: "+run.When)

		return
	}

	shell := run.Shell
	if shell == "" {
		shell = circleShell
	}

	chk := config.Check{
		Name:    circleCheckName(job, run, index),
		Command: run.Command,
		Dir:     run.WorkingDirectory,
		Shell:   shell,
		Env:     mergeStringMaps(jobEnv, run.Environment),
		Inputs:  inferInputs(run.Command),
	}

	entry := Entry{
		Workflow: im.label, Job: job, Step: firstNonEmpty(run.Name, "run"),
		Outcome: Extracted, Command: run.Command,
	}
	entry.Heavy = routeCheck(im.res, job, chk)
	im.res.Entries = append(im.res.Entries, entry)
}

func circleCheckName(job string, run circleRun, index int) string {
	if run.Name != "" {
		return slug(job) + "-" + slug(run.Name)
	}

	return slug(job) + "-" + itoa(index)
}
