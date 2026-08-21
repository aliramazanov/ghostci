package main

import (
	"errors"
	"strings"
	"testing"
)

func TestUnsupportedPlatformIsReportedOnce(t *testing.T) {
	t.Parallel()

	err := errors.New("ghostci does not support plan9 yet")

	var manual strings.Builder

	if code := reportUnsupported(&manual, err, false); code != exitBadUsage {
		t.Errorf("manual run exit = %d, want %d", code, exitBadUsage)
	}

	if strings.Count(manual.String(), "does not support") != 1 {
		t.Errorf("the reason should appear once:\n%s", manual.String())
	}

	var hook strings.Builder

	if code := reportUnsupported(&hook, err, true); code != exitOK {
		t.Errorf("hook exit = %d, want %d so the push is not blocked by a platform gap", code, exitOK)
	}

	if !strings.Contains(hook.String(), "nothing was verified") {
		t.Errorf("the hook output does not say nothing was checked:\n%s", hook.String())
	}

	if strings.Contains(hook.String(), "all clear") {
		t.Errorf("an unsupported platform must never read as an all clear:\n%s", hook.String())
	}
}

func TestAMissingToolDoesNotBlockAPushButIsNeverAllClear(t *testing.T) {
	r := newRepo(t)

	r.write("ghostci.yaml", `
checks:
  - name: needs-tool
    command: definitely-not-a-real-command-xyz build
`)
	r.commit("seed")

	byHand := r.ghostci("", "--all")
	if byHand.code == 0 {
		t.Errorf("run by hand exited 0 having verified nothing:\n%s", byHand.out)
	}
	if strings.Contains(byHand.out, "\n  all clear.\n") {
		t.Errorf("a check that could not run was reported all clear:\n%s", byHand.out)
	}
	if !strings.Contains(byHand.out, "could not run") {
		t.Errorf("the report does not say the check could not run:\n%s", byHand.out)
	}

	inHook := r.ghostci("", "--all", "--hook")
	if inHook.code != exitOK {
		t.Errorf("hook exit = %d, want %d: a missing local tool must not block a push",
			inHook.code, exitOK)
	}
	if strings.Contains(inHook.out, "\n  all clear.\n") {
		t.Errorf("the hook reported all clear without running the check:\n%s", inHook.out)
	}
}
