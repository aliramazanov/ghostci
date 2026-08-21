package workflow

import (
	"slices"
	"strings"
	"testing"
)

func filters(t *testing.T, src string, events ...string) (paths, ignore []string) {
	t.Helper()

	wf, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}

	return wf.PathFilters(events...)
}

func TestUnfilteredTriggerRemovesTheConstraint(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths: ['src/**']
  pull_request:
`, "push", "pull_request")

	if paths != nil || ignore != nil {
		t.Errorf("paths = %v, ignore = %v, want no constraint", paths, ignore)
	}
}

func TestPathsUnionAcrossTriggers(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths: ['src/**']
  pull_request:
    paths: ['test/**']
`, "push", "pull_request")

	if !slices.Equal(paths, []string{"src/**", "test/**"}) {
		t.Errorf("paths = %v, want both lists", paths)
	}
	if ignore != nil {
		t.Errorf("ignore = %v, want none", ignore)
	}
}

func TestIgnoresMustAgree(t *testing.T) {
	t.Parallel()

	_, ignore := filters(t, `
on:
  push:
    paths-ignore: ['docs/**']
  pull_request:
    paths-ignore: ['docs/**']
`, "push", "pull_request")

	if !slices.Equal(ignore, []string{"docs/**"}) {
		t.Errorf("ignore = %v, want the shared list", ignore)
	}

	_, ignore = filters(t, `
on:
  push:
    paths-ignore: ['docs/**']
  pull_request:
    paths-ignore: ['README.md']
`, "push", "pull_request")

	if ignore != nil {
		t.Errorf("ignore = %v, want none when the triggers disagree", ignore)
	}
}

func TestMixedFilterKindsYieldNothing(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths: ['src/**']
  pull_request:
    paths-ignore: ['docs/**']
`, "push", "pull_request")

	if paths != nil || ignore != nil {
		t.Errorf("paths = %v, ignore = %v, want no constraint", paths, ignore)
	}
}

func TestUnrelatedTriggersAreIgnored(t *testing.T) {
	t.Parallel()

	paths, _ := filters(t, `
on:
  push:
    paths: ['src/**']
  schedule:
    - cron: '0 0 * * *'
`, "push", "pull_request")

	if !slices.Equal(paths, []string{"src/**"}) {
		t.Errorf("paths = %v, want src/**", paths)
	}
}

func TestInlineNegationsBecomeExclusions(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths:
      - 'src/**'
      - '!src/vendor/**'
`, "push")

	if !slices.Equal(paths, []string{"src/**"}) {
		t.Errorf("paths = %v, want only the positive entry", paths)
	}
	if !slices.Equal(ignore, []string{"src/vendor/**"}) {
		t.Errorf("ignore = %v, want the negated entry", ignore)
	}
}

func TestNegationOnlyPathsLeaveNoInputs(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths: ['!docs/**']
`, "push")

	if paths != nil {
		t.Errorf("paths = %v, want none so inference takes over", paths)
	}
	if !slices.Equal(ignore, []string{"docs/**"}) {
		t.Errorf("ignore = %v, want docs/**", ignore)
	}
}

func TestDoubleBangIsALiteralPath(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths: ['!!weird/**']
`, "push")

	if !slices.Equal(paths, []string{"!weird/**"}) {
		t.Errorf("paths = %v, want the literal bang kept", paths)
	}
	if ignore != nil {
		t.Errorf("ignore = %v, want none", ignore)
	}
}

func TestPathsFromSeveralTriggersAreNotRepeated(t *testing.T) {
	t.Parallel()

	paths, ignore := filters(t, `
on:
  push:
    paths: ['services/**', 'tools/**', 'Makefile']
  pull_request:
    paths: ['services/**', 'tools/**']
`, "push", "pull_request")

	want := []string{"services/**", "tools/**", "Makefile"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("paths = %v, want %v with no repeats and order kept", paths, want)
	}
	if len(ignore) != 0 {
		t.Errorf("ignore = %v, want none", ignore)
	}
}
