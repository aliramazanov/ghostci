package importer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aliramazanov/ghostci/internal/pipeline"

	"github.com/aliramazanov/ghostci/internal/config"
	"github.com/aliramazanov/ghostci/internal/workflow"
)

func stepName(s workflow.Step, i int) string {
	if s.Name != "" {
		return s.Name
	}
	if s.Uses != "" {
		return s.Uses
	}
	if line, _, _ := strings.Cut(strings.TrimSpace(s.Run), "\n"); line != "" {
		return line
	}
	return fmt.Sprintf("step-%d", i+1)
}

func checkName(wfName, jobID string, step workflow.Step, i int, combo workflow.Combination) string {
	name := slug(stepName(step, i))
	base := jobID
	if wfName != "" && wfName != jobID {
		base = wfName + "/" + jobID
	}
	if label := combo.Label(); label != "" {
		base += "[" + slug(label) + "]"
	}
	return base + "/" + name
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			lastDash = false
		case r == '.' || r == '_' || r == '/':
			sb.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && sb.Len() > 0 {
				sb.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(sb.String(), "-")
	if len(out) > 48 {
		out = strings.Trim(out[:48], "-")
	}
	if out == "" {
		return "step"
	}
	return out
}

func finish(res *Result) {
	collapseIdentical(res)
	dedupeNames(res)
}

func collapseIdentical(res *Result) {
	type identity struct {
		command, dir, shell, env, inputs, exclude string
		optional, serial                          bool
		timeout                                   config.Duration
	}

	key := func(c config.Check) identity {
		names := make([]string, 0, len(c.Env))

		for k := range c.Env {
			names = append(names, k)
		}

		sort.Strings(names)

		var env strings.Builder

		for _, k := range names {
			env.WriteString(k)
			env.WriteString("=")
			env.WriteString(c.Env[k])
			env.WriteString("\x00")
		}

		return identity{
			command: c.Command, dir: c.Dir, shell: c.Shell,
			env:      env.String(),
			inputs:   strings.Join(c.Inputs, "\x00"),
			exclude:  strings.Join(c.Exclude, "\x00"),
			optional: c.Optional, serial: c.Serial, timeout: c.Timeout,
		}
	}

	collapse := func(checks []config.Check) ([]config.Check, int) {
		seen := map[identity]bool{}
		out := checks[:0:0]
		dropped := 0

		for _, c := range checks {
			k := key(c)
			if seen[k] {
				dropped++

				continue
			}

			seen[k] = true
			out = append(out, c)
		}

		return out, dropped
	}

	checks, dropped := collapse(res.Checks)
	heavy, heavyDropped := collapse(res.Heavy)

	res.Checks, res.Heavy = checks, heavy
	dropped += heavyDropped

	if dropped > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%d matrix legs run a command already listed here, so one of each is kept", dropped))
	}
}

func dedupeNames(res *Result) {
	seen := map[string]int{}
	rename := func(checks []config.Check) {
		for i := range checks {
			name := checks[i].Name
			if n, dup := seen[name]; dup {
				seen[name] = n + 1
				renamed := fmt.Sprintf("%s-%d", name, n+1)
				if why, ok := res.HeavyReason[name]; ok {
					res.HeavyReason[renamed] = why
				}
				checks[i].Name = renamed
				continue
			}
			seen[name] = 1
		}
	}
	rename(res.Checks)
	rename(res.Heavy)
}

func trim(s string) string { return pipeline.Trim(s) }
