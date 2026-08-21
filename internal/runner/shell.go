package runner

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var DefaultShell = sync.OnceValue(func() string {
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash -e"
	}

	return "sh -e"
})

func program(shell string) string {
	fields := strings.Fields(shell)
	if len(fields) == 0 {
		fields = strings.Fields(DefaultShell())
	}

	return strings.TrimSuffix(filepath.Base(fields[0]), ".exe")
}

func neutralise(shell string, declared map[string]string) []string {
	if _, set := declared["BASH_ENV"]; set || program(shell) != "bash" {
		return nil
	}

	return []string{"BASH_ENV="}
}

func invocation(shell, script string) (name string, args []string) {
	fields := strings.Fields(shell)
	if len(fields) == 0 {
		fields = strings.Fields(DefaultShell())
	}

	args = make([]string, 0, len(fields)+1)
	args = append(args, fields[1:]...)

	if !carriesScript(args) {
		args = append(args, scriptFlag(fields[0]))
	}

	return fields[0], append(args, script)
}

func carriesScript(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(a) {
		case "-c", "-command":
			return true
		}
	}

	return false
}

func scriptFlag(program string) string {
	switch strings.TrimSuffix(filepath.Base(program), ".exe") {
	case "pwsh", "powershell":
		return "-Command"
	}

	return "-c"
}
