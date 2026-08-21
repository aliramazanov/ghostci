package shell

import (
	"slices"
	"testing"
)

func TestKeywordsAreNotCommands(t *testing.T) {
	t.Parallel()

	cases := []struct {
		script string
		want   []string
	}{
		{"if go test ./...; then echo ok; fi", []string{"go", "echo"}},
		{"until pytest; do break; done", []string{"pytest", "break"}},
		{"while ! cargo check; do sleep 1; done", []string{"cargo", "sleep"}},
		{"case $x in a) ruff check;; esac", []string{"ruff"}},
		{"{ go vet ./...; }", []string{"go"}},
		{"! grep -q TODO src", []string{"grep"}},
		{"for d in */; do (cd $d && cargo build); done", []string{"cd", "cargo"}},
	}

	for _, c := range cases {
		if got := Invoked(c.script); !slices.Equal(got, c.want) {
			t.Errorf("Invoked(%q) = %v, want %v", c.script, got, c.want)
		}
	}
}

func TestAKeywordDoesNotHideAToolFromTheOthersInputs(t *testing.T) {
	t.Parallel()

	got := Invoked("cd web && npm ci\nif go build ./...; then :; fi")

	if !slices.Contains(got, "go") {
		t.Errorf("Invoked = %v, want go among them; its inputs would narrow to npm's", got)
	}
	if !slices.Contains(got, "npm") {
		t.Errorf("Invoked = %v, want npm among them", got)
	}
}
