package importer

import (
	"fmt"
	"sort"
	"strings"
)

const testRunner = "ubuntu-latest"

type step struct {
	name  string
	run   string
	uses  string
	cond  string
	with  map[string]string
	shell string
	dir   string
}

func run(cmd string) step { return step{run: cmd} }

func runIf(cond, cmd string) step { return step{cond: cond, run: cmd} }

func uses(action string) step { return step{uses: action} }

func usesWith(action string, with map[string]string) step {
	return step{uses: action, with: with}
}

func (s step) in(dir string) step { s.dir = dir; return s }

func (s step) via(shell string) step { s.shell = shell; return s }

func (s step) yaml(indent string) string {
	var b strings.Builder

	b.WriteString(indent + "- ")
	if s.name != "" {
		b.WriteString("name: " + s.name + "\n" + indent + "  ")
	}
	if s.cond != "" {
		fmt.Fprintf(&b, "if: %s\n%s  ", quote(s.cond), indent)
	}
	if s.uses != "" {
		b.WriteString("uses: " + s.uses + "\n")
	} else {
		b.WriteString("run: " + block(s.run, indent+"      ") + "\n")
	}
	if s.shell != "" {
		b.WriteString(indent + "  shell: " + s.shell + "\n")
	}
	if s.dir != "" {
		b.WriteString(indent + "  working-directory: " + s.dir + "\n")
	}
	for k, v := range s.with {
		b.WriteString(indent + "  with:\n" + indent + "    " + k + ": " + v + "\n")
	}

	return b.String()
}

func quote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func block(cmd, indent string) string {
	if !strings.Contains(cmd, "\n") {
		return quote(cmd)
	}

	out := "|-\n"
	for _, line := range strings.Split(strings.TrimRight(cmd, "\n"), "\n") {
		out += indent + line + "\n"
	}

	return strings.TrimRight(out, "\n")
}

type job struct {
	id     string
	runsOn string
	cond   string
	matrix map[string][]string
	extra  string
	steps  []step
}

func newJob(id string, steps ...step) job {
	return job{id: id, runsOn: testRunner, steps: steps}
}

func (j job) on(runner string) job { j.runsOn = runner; return j }

func (j job) over(key string, values ...string) job {
	if j.matrix == nil {
		j.matrix = map[string][]string{}
	}
	j.matrix[key] = values

	return j
}

func (j job) yaml() string {
	out := "  " + j.id + ":\n    runs-on: " + j.runsOn + "\n"
	if j.cond != "" {
		out += "    if: " + j.cond + "\n"
	}
	if len(j.matrix) > 0 {
		out += "    strategy:\n      matrix:\n"
		keys := make([]string, 0, len(j.matrix))
		for k := range j.matrix {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out += "        " + k + ": [" + strings.Join(j.matrix[k], ", ") + "]\n"
		}
	}
	out += j.extra
	out += "    steps:\n"
	for _, s := range j.steps {
		out += s.yaml("      ")
	}

	return out
}

func synthWorkflow(jobs ...job) string {
	out := "on: [push, pull_request]\njobs:\n"
	for _, j := range jobs {
		out += j.yaml()
	}

	return out
}
