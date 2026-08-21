package importer

import (
	"regexp"
	"strings"
)

var azureMacro = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_.]*)\)`)

var azureBuiltIn = regexp.MustCompile(`^(Build|System|Agent|Pipeline|Release|Environment|Checks|Strategy|Deployment)\.`)

func azureKnown(a Assumptions) map[string]string {
	return map[string]string{
		"Build.SourcesDirectory":         ".",
		"Build.Repository.LocalPath":     ".",
		"System.DefaultWorkingDirectory": ".",
		"Build.Repository.Name":          a.Repository,
		"Build.SourceBranch":             a.Ref,
		"Build.SourceBranchName":         shortRef(a.Ref),
		"Agent.OS":                       "Linux",
	}
}

func expandMacros(text string, vars azureVariables, known map[string]string) (string, string, bool) {
	var unresolved string

	out := azureMacro.ReplaceAllStringFunc(text, func(match string) string {
		name := azureMacro.FindStringSubmatch(match)[1]

		if value, ok := vars[name]; ok {

			if strings.Contains(value, "$(") || strings.Contains(value, "${{") {
				unresolved = firstNonEmpty(unresolved, name)

				return match
			}

			return value
		}

		if value, ok := known[name]; ok {
			return value
		}

		if azureBuiltIn.MatchString(name) {
			unresolved = firstNonEmpty(unresolved, name)
		}

		return match
	})

	return out, unresolved, unresolved == ""
}

func resolvedVars(vars azureVariables, known map[string]string) map[string]string {
	out := map[string]string{}

	for name, value := range vars {
		if strings.Contains(value, "${{") || strings.Contains(value, "$[") {
			continue
		}

		resolved, _, ok := expandMacros(value, vars, known)
		if !ok {
			continue
		}

		out[name] = resolved
	}

	if len(out) == 0 {
		return nil
	}

	return out
}
