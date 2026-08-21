package importer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/pipeline"
	"go.yaml.in/yaml/v3"
)

var AzureFiles = []string{"azure-pipelines.yml", "azure-pipelines.yaml", ".azure-pipelines.yml"}

const azureShell = "bash -e"

type azureFile struct {
	Trigger   azureTrigger   `yaml:"trigger"`
	PR        azureTrigger   `yaml:"pr"`
	Variables azureVariables `yaml:"variables"`
	Steps     []azureStep    `yaml:"steps"`
	Jobs      []azureJob     `yaml:"jobs"`
	Stages    []azureStage   `yaml:"stages"`
	Pool      yaml.Node      `yaml:"pool"`
	Resources yaml.Node      `yaml:"resources"`
	Extends   yaml.Node      `yaml:"extends"`
}

type azureTrigger struct {
	Paths struct {
		Include []string `yaml:"include"`
		Exclude []string `yaml:"exclude"`
	} `yaml:"paths"`
}

func (t *azureTrigger) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}

	type raw azureTrigger

	var out raw
	if err := node.Decode(&out); err != nil {
		return nil
	}
	*t = azureTrigger(out)

	return nil
}

type azureStage struct {
	Stage string     `yaml:"stage"`
	Jobs  []azureJob `yaml:"jobs"`
}

type azureJob struct {
	Job         string         `yaml:"job"`
	DisplayName string         `yaml:"displayName"`
	Condition   string         `yaml:"condition"`
	Strategy    yaml.Node      `yaml:"strategy"`
	Container   yaml.Node      `yaml:"container"`
	Services    yaml.Node      `yaml:"services"`
	Variables   azureVariables `yaml:"variables"`
	Timeout     int            `yaml:"timeoutInMinutes"`
	Steps       []azureStep    `yaml:"steps"`
	Template    string         `yaml:"template"`
}

type azureStep struct {
	Script      string         `yaml:"script"`
	Bash        string         `yaml:"bash"`
	Pwsh        string         `yaml:"pwsh"`
	PowerShell  string         `yaml:"powershell"`
	Task        string         `yaml:"task"`
	Template    string         `yaml:"template"`
	Checkout    string         `yaml:"checkout"`
	DisplayName string         `yaml:"displayName"`
	Condition   string         `yaml:"condition"`
	WorkingDir  string         `yaml:"workingDirectory"`
	Env         azureVariables `yaml:"env"`
	Timeout     int            `yaml:"timeoutInMinutes"`
	ContinueOn  bool           `yaml:"continueOnError"`
}

type azureVariables map[string]string

func (v *azureVariables) UnmarshalYAML(node *yaml.Node) error {
	out := azureVariables{}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i+1].Kind == yaml.ScalarNode {
				out[node.Content[i].Value] = node.Content[i+1].Value
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			var entry struct {
				Name     string `yaml:"name"`
				Value    string `yaml:"value"`
				Template string `yaml:"template"`
				Group    string `yaml:"group"`
			}
			if err := item.Decode(&entry); err == nil && entry.Name != "" {
				out[entry.Name] = entry.Value
			}
		}
	}

	*v = out

	return nil
}

func ImportAzure(path string, a Assumptions) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file azureFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	im := &azureImport{
		ledger: ledger{res: &Result{Azure: true}, label: filepath.Base(path)},
		known:  azureKnown(a),
	}

	if !file.Extends.IsZero() {
		im.refuse("(extends)", pipeline.OpaqueUnit,
			"extends a pipeline template, whose steps are not visible here")
	}

	im.inputs = append(im.inputs, file.Trigger.Paths.Include...)
	im.inputs = append(im.inputs, file.PR.Paths.Include...)
	im.exclude = append(im.exclude, file.Trigger.Paths.Exclude...)
	im.exclude = append(im.exclude, file.PR.Paths.Exclude...)

	im.walkJobs("", []azureJob{{Job: "", Steps: file.Steps}}, file.Variables)
	im.walkJobs("", file.Jobs, file.Variables)
	for _, stage := range file.Stages {
		im.walkJobs(stage.Stage, stage.Jobs, file.Variables)
	}

	dedupeNames(im.res)

	return im.res, nil
}

type azureImport struct {
	ledger
	known   map[string]string
	inputs  []string
	exclude []string
}

func (im *azureImport) walkJobs(stage string, jobs []azureJob, global azureVariables) {
	for _, job := range jobs {
		name := firstNonEmpty(job.Job, job.DisplayName, stage)

		switch {
		case job.Template != "":
			im.refuse(name, pipeline.OpaqueUnit, "the job comes from template "+job.Template)

			continue
		case !job.Services.IsZero():
			im.refuse(name, pipeline.NeedsContainer, "needs service containers")

			continue
		case !job.Container.IsZero():
			im.refuse(name, pipeline.NeedsContainer, "runs in a container")

			continue
		case !job.Strategy.IsZero():
			im.refuse(name, pipeline.ServerState,
				"uses a strategy matrix, which ghostci does not expand for Azure yet")

			continue
		case job.Condition != "":
			im.refuse(name, pipeline.ServerState, "job condition: "+pipeline.Trim(job.Condition))

			continue
		}

		vars := mergeAzureVars(global, job.Variables)
		for i, step := range job.Steps {
			im.importStep(name, i, step, vars, job.Timeout)
		}
	}
}

func (im *azureImport) importStep(job string, index int, step azureStep,
	vars azureVariables, jobTimeout int) {

	name := firstNonEmpty(step.DisplayName, job, "step")

	switch {
	case step.Checkout != "":
		return
	case step.Task != "":
		im.refuse(name, pipeline.OpaqueUnit, "runs the marketplace task "+step.Task)

		return
	case step.Template != "":
		im.refuse(name, pipeline.OpaqueUnit, "comes from template "+step.Template)

		return
	case step.Pwsh != "" || step.PowerShell != "":
		im.refuse(name, pipeline.Unreadable, "is a PowerShell step, which ghostci does not run")

		return
	case step.Condition != "":
		im.refuse(name, pipeline.ServerState, "step condition: "+pipeline.Trim(step.Condition))

		return
	}

	command := firstNonEmpty(step.Script, step.Bash)
	if command == "" {
		return
	}

	if strings.Contains(command, "${{") || strings.Contains(command, "$[") {
		im.refuse(name, pipeline.ServerState,
			"the command contains an Azure expression that resolves inside CI")

		return
	}

	all := mergeAzureVars(vars, step.Env)

	command, unresolved, ok := expandMacros(command, all, im.known)
	if !ok {
		im.refuse(name, pipeline.ServerState,
			"the command reads $("+unresolved+"), which Azure sets when it runs the pipeline")

		return
	}

	dir, _, ok := expandMacros(step.WorkingDir, all, im.known)
	if !ok {
		im.refuse(name, pipeline.ServerState,
			"the working directory is only known inside CI")

		return
	}

	chk := config.Check{
		Name:    azureCheckName(job, step, index),
		Command: command,
		Dir:     dir,
		Shell:   azureShell,
		Env:     resolvedVars(all, im.known),
		Timeout: config.Duration(timeoutMinutes(firstNonZero(step.Timeout, jobTimeout))),
		Inputs:  im.inputsFor(command),
		Exclude: im.exclude,
	}

	entry := Entry{Workflow: im.label, Job: job, Step: name, Outcome: Extracted, Command: command}
	entry.Heavy = routeCheck(im.res, job, chk)
	im.res.Entries = append(im.res.Entries, entry)
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}

	return 0
}

func (im *azureImport) inputsFor(command string) []string {
	if len(im.inputs) > 0 {
		return im.inputs
	}

	return inferInputs(command)
}

func azureCheckName(job string, step azureStep, index int) string {
	if step.DisplayName != "" {
		return slug(step.DisplayName)
	}

	return slug(firstNonEmpty(job, "step")) + "-" + itoa(index)
}

func mergeAzureVars(layers ...azureVariables) map[string]string {
	out := map[string]string{}
	for _, layer := range layers {
		for k, v := range layer {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}

	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}

	return ""
}
