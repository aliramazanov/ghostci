package workflow

import (
	"slices"
	"testing"
)

func TestIncludeIsProcessedAfterExclude(t *testing.T) {
	t.Parallel()
	got := expand(t, `
matrix:
  os: [linux, mac]
  v: [1, 2]
  exclude:
    - os: linux
      v: 1
  include:
    - os: linux
      v: 1
`)

	if len(got) != 4 {
		t.Fatalf("got %d combinations, want the excluded one reinstated: %v", len(got), got)
	}
	if !slices.Contains(got, "os=linux v=1 ") {
		t.Errorf("include did not reinstate the excluded combination: %v", got)
	}
}
