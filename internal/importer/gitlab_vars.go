package importer

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/aliramazanov/ghostci/internal/gitlab"
	"github.com/aliramazanov/ghostci/internal/pipeline"
)

func unsetInput(text string) (string, bool) {
	i := strings.Index(text, gitlab.UnsetInput+":")
	if i < 0 {
		return "", false
	}

	name := text[i+len(gitlab.UnsetInput)+1:]
	if cut := strings.IndexFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	}); cut >= 0 {
		name = name[:cut]
	}

	return name, true
}

func declared(name, value string) pipeline.Value {
	if _, unset := unsetInput(value); unset {
		return pipeline.Unknown("$" + name)
	}

	return pipeline.Known(value)
}

var ciVarPattern = regexp.MustCompile(`\$\{?((?:CI|GITLAB)_[A-Z0-9_]+)\}?`)

func (im *gitlabImport) scriptEnv(file *gitlab.File, job gitlab.Job, combo gitlab.Variables) (map[string]string, []string) {
	known := im.vars(file, job, "push")
	for k, v := range combo {
		known[k] = pipeline.Known(v)
	}

	env := map[string]string{}
	set := func(name, value string) {
		if _, unset := unsetInput(value); unset {
			return
		}

		env[name] = value
	}

	for name, value := range file.Variables {
		set(name, value)
	}
	for name, value := range job.Variables {
		set(name, value)
	}
	for name, value := range combo {
		set(name, value)
	}
	for name, v := range known {
		if v.Defined && v.Text != "" {
			if _, set := env[name]; !set {
				env[name] = v.Text
			}
		}
	}

	missing := map[string]bool{}
	for _, line := range append(append([]string{}, job.BeforeScript...), job.Script...) {
		for _, m := range ciVarPattern.FindAllStringSubmatch(line, -1) {
			name := m[1]
			if _, set := env[name]; set {
				continue
			}
			if v := known[name]; v.Defined {
				continue
			}
			missing[name] = true
		}
	}

	if len(missing) == 0 {
		return env, nil
	}

	out := make([]string, 0, len(missing))
	for name := range missing {
		out = append(out, "$"+name)
	}
	sort.Strings(out)

	return env, out
}

func (im *gitlabImport) vars(file *gitlab.File, job gitlab.Job, source string) gitlab.Vars {
	branch := shortRef(im.a.Ref)

	vars := gitlab.Vars{
		"CI":                 pipeline.Known("true"),
		"GITLAB_CI":          pipeline.Known("true"),
		"CI_PIPELINE_SOURCE": pipeline.Known(source),
		"CI_PROJECT_DIR":     pipeline.Known("."),
		"CI_COMMIT_REF_NAME": pipeline.Known(branch),
		"CI_COMMIT_REF_SLUG": pipeline.Known(slug(branch)),
		"CI_DEFAULT_BRANCH":  pipeline.Known(im.defaultBranch),
		"CI_PROJECT_PATH":    pipeline.Known(im.a.Repository),
		"CI_PROJECT_NAME":    pipeline.Known(projectName(im.a.Repository)),
	}

	if im.defaultBranch == "" {
		vars["CI_DEFAULT_BRANCH"] = pipeline.Unknown("$CI_DEFAULT_BRANCH")
	}

	switch source {
	case "merge_request_event":

		vars["CI_COMMIT_BRANCH"] = pipeline.Undefined()
		vars["CI_COMMIT_TAG"] = pipeline.Undefined()
		vars["CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"] = pipeline.Known(branch)
		vars["CI_MERGE_REQUEST_TARGET_BRANCH_NAME"] = pipeline.Unknown("$CI_MERGE_REQUEST_TARGET_BRANCH_NAME")
		vars["CI_MERGE_REQUEST_IID"] = pipeline.Unknown("$CI_MERGE_REQUEST_IID")
	default:
		vars["CI_COMMIT_BRANCH"] = pipeline.Known(branch)
		vars["CI_COMMIT_TAG"] = pipeline.Undefined()
		for _, name := range []string{
			"CI_MERGE_REQUEST_IID",
			"CI_MERGE_REQUEST_SOURCE_BRANCH_NAME",
			"CI_MERGE_REQUEST_TARGET_BRANCH_NAME",
		} {
			vars[name] = pipeline.Undefined()
		}
	}

	for name, value := range file.Variables {
		vars[name] = declared(name, value)
	}
	for name, value := range job.Variables {
		vars[name] = declared(name, value)
	}

	return vars
}
