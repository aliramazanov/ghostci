package importer

import (
	"fmt"
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
