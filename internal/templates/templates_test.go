package templates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnInstallPathIsNeverRunAsShell(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, `we'ird "$(touch marker)" dir`)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(dir, "ghostci")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho \"ran with $1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	script, err := PrePush(PrePushParams{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}

	hook := filepath.Join(root, "pre-push")
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", hook)
	cmd.Dir = root
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdin = strings.NewReader("")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook failed: %v\n%s", err, out)
	}

	if !strings.Contains(string(out), "ran with --hook") {
		t.Errorf("the hook did not run the binary at its recorded path:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(root, "marker")); err == nil {
		t.Error("text in the install path was executed by the shell")
	}
}
