package workflow

import (
	"fmt"
	"sort"
	"testing"

	"go.yaml.in/yaml/v3"
)

func expand(t *testing.T, src string) []string {
	t.Helper()
	var s Strategy
	if err := yaml.Unmarshal([]byte(src), &s); err != nil {
		t.Fatal(err)
	}
	combos, _, err := ExpandMatrix(&s)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	out := make([]string, 0, len(combos))
	for _, c := range combos {
		keys := make([]string, 0, len(c))
		for k := range c {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		s := ""
		for _, k := range keys {
			s += fmt.Sprintf("%s=%v ", k, c[k])
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func assertSet(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %d combinations, want %d:\n got: %q\nwant: %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("combination %d:\n got %q\nwant %q\nall got: %q", i, got[i], want[i], got)
		}
	}
}

func TestIncludeMatchesGitHubDocumentedExample(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  fruit: [apple, pear]
  animal: [cat, dog]
  include:
    - color: green
    - color: pink
      animal: cat
    - fruit: apple
      shape: circle
    - fruit: banana
    - fruit: banana
      animal: cat
`)
	assertSet(t, got, []string{
		"animal=cat color=pink fruit=apple shape=circle ",
		"animal=dog color=green fruit=apple shape=circle ",
		"animal=cat color=pink fruit=pear ",
		"animal=dog color=green fruit=pear ",
		"fruit=banana ",
		"animal=cat fruit=banana ",
	})
}

func TestIncludeWithoutAxisKeysBroadcasts(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  os: [linux, mac]
  include:
    - flags: --verbose
`)
	assertSet(t, got, []string{
		"flags=--verbose os=linux ",
		"flags=--verbose os=mac ",
	})
}

func TestIncludeConditionalOnOneAxis(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  os: [linux, mac]
  go: ['1.25', '1.26']
  include:
    - os: linux
      coverage: true
`)
	assertSet(t, got, []string{
		"coverage=true go=1.25 os=linux ",
		"coverage=true go=1.26 os=linux ",
		"go=1.25 os=mac ",
		"go=1.26 os=mac ",
	})
}

func TestUnmatchedIncludesStayDistinct(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  os: [linux]
  include:
    - os: windows
    - os: windows
      extra: yes
`)
	assertSet(t, got, []string{
		"os=linux ",
		"os=windows ",
		"extra=yes os=windows ",
	})
}

func TestExclude(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  os: [linux, mac]
  go: ['1.25', '1.26']
  exclude:
    - os: mac
      go: '1.25'
`)
	assertSet(t, got, []string{
		"go=1.25 os=linux ",
		"go=1.26 os=linux ",
		"go=1.26 os=mac ",
	})
}

func TestExcludeThenInclude(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  os: [linux, mac]
  exclude:
    - os: mac
  include:
    - tag: fast
`)
	assertSet(t, got, []string{"os=linux tag=fast "})
}

func TestNoMatrixYieldsOneEmptyCombination(t *testing.T) {
	t.Parallel()
	combos, _, err := ExpandMatrix(nil)
	if err != nil || len(combos) != 1 || len(combos[0]) != 0 {
		t.Fatalf("got %v, %v", combos, err)
	}
}

func TestDynamicMatrixIsRefused(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"matrix: ${{ fromJSON(needs.setup.outputs.matrix) }}\n",
		"matrix:\n  os: ${{ fromJSON(needs.x.outputs.os) }}\n",
	} {
		var s Strategy
		if err := yaml.Unmarshal([]byte(src), &s); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ExpandMatrix(&s); err == nil {
			t.Errorf("a run-time matrix must be refused, not guessed: %s", src)
		}
	}
}

func TestCombinationLabel(t *testing.T) {
	t.Parallel()
	c := Combination{"os": "linux", "go": "1.26"}
	if got := c.Label(); got != "1.26-linux" {
		t.Errorf("Label() = %q", got)
	}
	if got := (Combination{}).Label(); got != "" {
		t.Errorf("empty label = %q", got)
	}
}

func TestLargeMatrixIsTruncated(t *testing.T) {
	t.Parallel()
	var s Strategy
	if err := yaml.Unmarshal([]byte(`
matrix:
  a: [1,2,3,4,5,6,7,8,9,10]
  b: [1,2,3,4,5,6,7,8,9,10]
  c: [1,2,3,4,5,6,7,8,9,10]
`), &s); err != nil {
		t.Fatal(err)
	}
	combos, truncated, err := ExpandMatrix(&s)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Error("1000 combinations should report truncation")
	}
	if len(combos) > MaxCombinations {
		t.Errorf("got %d combinations, cap is %d", len(combos), MaxCombinations)
	}
}

func TestIncludeOnlyMatrixIsOneLegEach(t *testing.T) {
	t.Parallel()

	got := expand(t, `
matrix:
  include:
    - go: '1.25'
    - go: '1.26'
`)
	assertSet(t, got, []string{"go=1.25 ", "go=1.26 "})
}

func TestIncludeOnlyMatrixWithSeveralKeys(t *testing.T) {
	t.Parallel()

	got := expand(t, `
matrix:
  include:
    - os: linux
      arch: amd64
    - os: mac
      arch: arm64
`)
	assertSet(t, got, []string{"arch=amd64 os=linux ", "arch=arm64 os=mac "})
}
