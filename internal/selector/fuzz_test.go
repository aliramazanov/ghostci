package selector

import (
	"testing"

	"github.com/aliramazanov/ghostci/internal/config"
)

func FuzzMatcher(f *testing.F) {
	for _, seed := range []struct{ glob, exclude, file string }{
		{"**/*.go", "", "main.go"},
		{"src/**", "src/vendor/**", "src/vendor/x.go"},
		{"**", "", "anything"},
		{"[bad", "", "x"},
		{"a/*/c", "", "a/b/c"},
		{"", "", ""},
		{"**/*", "**/*", "x"},
		{"./x", "", "x"},
	} {
		f.Add(seed.glob, seed.exclude, seed.file)
	}

	f.Fuzz(func(t *testing.T, glob, exclude, file string) {
		chk := config.Check{Name: "c", Command: "x", Inputs: []string{glob}, Exclude: []string{exclude}}
		matched := Matcher(chk)(file)

		empty := config.Check{Name: "c", Command: "x"}
		if !empty.AlwaysRuns() {
			t.Fatal("a check with no inputs stopped always-running")
		}

		noExclude := config.Check{Name: "c", Command: "x", Inputs: []string{glob}}
		if matched && !Matcher(noExclude)(file) {
			t.Errorf("exclude %q created a match that inputs %q did not have for %q", exclude, glob, file)
		}
	})
}
