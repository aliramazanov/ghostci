package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func TestMissingConfigIsAdviceByHandAndSilenceInAHook(t *testing.T) {
	missing := &config.NotFoundError{Err: os.ErrNotExist}

	if code := reportConfigError(missing, "ghostci.yaml", false); code != exitBadUsage {
		t.Errorf("by hand: exit %d, want %d", code, exitBadUsage)
	}
	if code := reportConfigError(missing, "ghostci.yaml", true); code != exitOK {
		t.Errorf("in a hook: exit %d, want %d so a repo without ghostci can still push", code, exitOK)
	}

	if code := reportConfigError(config.ErrNoChecks, "ghostci.yaml", false); code != exitBadUsage {
		t.Errorf("empty config: exit %d, want %d", code, exitBadUsage)
	}
	if code := reportConfigError(errors.New("boom"), "ghostci.yaml", false); code != exitBadUsage {
		t.Errorf("unreadable config: exit %d, want %d", code, exitBadUsage)
	}
}

func TestMissingConfigMessageSuggestsInit(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	reportConfigError(&config.NotFoundError{Err: os.ErrNotExist}, "ghostci.yaml", false)
	os.Stderr = saved
	w.Close()

	buf := make([]byte, 512)
	n, _ := r.Read(buf)
	if got := string(buf[:n]); !strings.Contains(got, "init") {
		t.Errorf("the message does not tell the user what to do: %q", got)
	}
}
