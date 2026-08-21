package report

import (
	"bytes"
	"strings"
	"testing"
)

func render(t *testing.T, name, output string) string {
	t.Helper()

	var buf bytes.Buffer
	if _, err := (Failure{Name: name, Output: output}).WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	return buf.String()
}

func TestFailureTrimsPadding(t *testing.T) {
	t.Parallel()

	got := render(t, "test", "\n\n\n  FAIL: bad  \n\n\n\nsecond\n\n\n")

	if strings.Contains(got, "\n\n\n") {
		t.Errorf("repeated blank lines survived:\n%q", got)
	}
	if !strings.Contains(got, "FAIL: bad") || !strings.Contains(got, "second") {
		t.Errorf("content lost:\n%s", got)
	}
	if strings.HasSuffix(got, "\n\n") {
		t.Errorf("trailing blank line survived:\n%q", got)
	}
}

func TestFailureWithNoOutputRendersNothing(t *testing.T) {
	t.Parallel()

	for _, out := range []string{"", "   ", "\n\n\n"} {
		if got := render(t, "quiet", out); got != "" {
			t.Errorf("empty output should render nothing, got %q", got)
		}
	}
}

func TestFailureNamesTheCheck(t *testing.T) {
	t.Parallel()

	if got := render(t, "ci/lint", "boom"); !strings.Contains(got, "ci/lint") {
		t.Errorf("check name missing:\n%s", got)
	}
}
