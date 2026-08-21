package git

import (
	"os/exec"
	"slices"
	"testing"
)

func TestFsmonitorIsDisabled(t *testing.T) {
	dir := repo(t)
	cmd := exec.Command("git", "config", "core.fsmonitor", "true")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}

	touch(t, dir, "a.go")
	commit(t, dir, "seed")

	if _, err := Dirty(dir); err != nil {
		t.Fatalf("Dirty with fsmonitor enabled: %v", err)
	}
}

func TestOptionsPrecedeTheSubcommand(t *testing.T) {
	t.Parallel()

	opts := options()
	if !slices.Contains(opts, "core.fsmonitor=false") {
		t.Errorf("options = %q, want the fsmonitor override", opts)
	}
	for i := 0; i < len(opts); i += 2 {
		if opts[i] != "-c" {
			t.Fatalf("options[%d] = %q, want -c", i, opts[i])
		}
	}
}
