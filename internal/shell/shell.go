package shell

import (
	"regexp"
	"strings"
)

var wrappers = map[string]bool{
	"npx": true, "pnpx": true, "bunx": true, "uvx": true,
	"sudo": true, "time": true, "xvfb-run": true, "env": true,
	"exec": true, "command": true, "--": true, "eval": true,
}

var keywords = map[string]bool{
	"if": true, "elif": true, "while": true, "until": true,
	"then": true, "do": true, "else": true,
}

var headers = map[string]bool{"for": true, "case": true, "select": true, "in": true}

var terminators = map[string]bool{"fi": true, "done": true, "esac": true}

var managers = map[string]bool{
	"npm": true, "pnpm": true, "yarn": true, "bun": true, "uv": true,
	"poetry": true, "pipenv": true, "hatch": true, "pdm": true, "rye": true,
	"bundle": true,
}

var runsAnother = map[string]bool{
	"exec": true, "run": true, "x": true, "dlx": true,
}

var (
	statements    = regexp.MustCompile(`[|;&\n)]+`)
	substitutions = regexp.MustCompile(`\$\(([^()]*)\)|` + "`([^`]*)`")
)

func Invoked(script string) []string {
	seen := map[string]bool{}

	var out []string

	add := func(word string) {
		if word == "" || seen[word] {
			return
		}

		seen[word] = true
		out = append(out, word)
	}

	for _, stmt := range split(script) {
		for _, word := range leading(stmt) {
			add(word)
		}
	}

	return out
}

func split(script string) []string {
	var nested []string

	stripped := substitutions.ReplaceAllStringFunc(script, func(m string) string {
		inner := substitutions.FindStringSubmatch(m)
		nested = append(nested, inner[1]+inner[2])

		return " "
	})

	out := statements.Split(stripped, -1)
	for _, n := range nested {
		out = append(out, statements.Split(n, -1)...)
	}

	return out
}

func leading(stmt string) []string {
	fields := strings.Fields(stmt)

	for i := 0; i < len(fields); i++ {
		word := strings.Trim(fields[i], "\"'`${}[]();|&<>!")
		if strings.HasPrefix(word, "#") {
			return nil
		}

		if word == "" {
			continue
		}

		if strings.HasPrefix(word, "-") || strings.Contains(word, "=") {
			continue
		}

		if j := strings.LastIndex(word, "/"); j >= 0 {
			word = word[j+1:]
		}

		if headers[word] || terminators[word] {
			return nil
		}
		if wrappers[word] || keywords[word] {
			continue
		}

		if managers[word] && i+1 < len(fields) && runsAnother[strings.Trim(fields[i+1], "\"'`")] {
			return append([]string{word}, leading(strings.Join(fields[i+2:], " "))...)
		}

		return []string{word}
	}

	return nil
}
