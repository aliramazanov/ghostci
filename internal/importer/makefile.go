package importer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	makeRule    = regexp.MustCompile(`^([^\s:=#][^:=]*):(?:[^=]|$)`)
	makeInvoked = regexp.MustCompile(`(?:^|[;&|]|\n)\s*make\b([^\n;&|]*)`)
)

const maxRecipeDepth = 2

func makeGoals(command string) (goals []string, dir string) {
	for _, m := range makeInvoked.FindAllStringSubmatch(command, -1) {
		fields := strings.Fields(m[1])
		for i := 0; i < len(fields); i++ {
			f := fields[i]
			switch {
			case f == "-C" || f == "--directory":
				if i+1 < len(fields) {
					dir = fields[i+1]
					i++
				}
			case strings.HasPrefix(f, "-"), strings.Contains(f, "="):
			default:
				goals = append(goals, f)
			}
		}
	}

	return goals, dir
}

func makeRecipe(root, dir string, goals []string) (string, bool) {
	if root == "" {
		return "", false
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "Makefile"))
	if err != nil {
		return "", false
	}

	recipes, first := parseMakefile(string(data))
	if len(goals) == 0 {
		goals = []string{first}
	}

	var out []string
	for _, goal := range goals {
		body, ok := recipes[goal]
		if !ok {
			return "", false
		}

		out = append(out, body)
	}

	return strings.Join(out, "\n"), len(out) > 0
}

func parseMakefile(text string) (recipes map[string]string, first string) {
	recipes = map[string]string{}

	var current []string
	var targets []string

	flush := func() {
		if len(current) == 0 {
			return
		}
		body := strings.Join(current, "\n")
		for _, t := range targets {
			recipes[t] = recipes[t] + "\n" + body
		}
		current = nil
	}

	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "\t") {

			current = append(current, strings.TrimLeft(strings.TrimPrefix(line, "\t"), "-@"))

			continue
		}

		flush()
		targets = nil

		m := makeRule.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		for _, t := range strings.Fields(m[1]) {
			if strings.HasPrefix(t, ".") {

				continue
			}
			targets = append(targets, t)
			if first == "" {
				first = t
			}
		}
	}

	flush()

	return recipes, first
}
