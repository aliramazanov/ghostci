package importer

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func corpusRepos(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatal(err)
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join("testdata", "corpus", e.Name(), ".github", "workflows"))
		}
	}

	if len(out) < 20 {
		t.Fatalf("only %d corpus repositories, too few to mean anything", len(out))
	}

	return out
}

var placeholder = regexp.MustCompile(`\$\{\{|\$\[\[|<<\s*(parameters|pipeline)\.`)

var sideEffect = regexp.MustCompile(
	`(^|[\s;&|(])(sudo\s|git\s+(push|commit|checkout|reset|clean|rebase|fetch|pull|tag)\b` +
		`|cargo\s+install|go\s+install|npm\s+(publish|i\s+-g)|pipx\s+install|gem\s+install` +
		`|goveralls|codecov|coveralls|twine\s+upload|docker\s+push|gh\s+release` +
		`|kubectl\s+apply|aws\s+s3|terraform\s+apply|helm\s+upgrade)`)

func TestEveryRunnableCheckIsSafeAndResolved(t *testing.T) {
	t.Parallel()

	for _, dir := range corpusRepos(t) {
		res, err := ImportDir(dir, DefaultAssumptions())
		if err != nil {
			t.Errorf("%s: %v", dir, err)

			continue
		}

		seen := map[string]bool{}

		for _, c := range res.Checks {
			body := c.Command + "\n" + c.Dir + "\n" + strings.Join(c.Inputs, "\n")
			for k, v := range c.Env {
				body += "\n" + k + "=" + v
			}

			if m := placeholder.FindString(body); m != "" {
				t.Errorf("%s: check %q carries an unexpanded %s:\n%s", dir, c.Name, m, c.Command)
			}
			if m := sideEffect.FindString(body); m != "" {
				t.Errorf("%s: check %q would run %q by default:\n%s", dir, c.Name, strings.TrimSpace(m), c.Command)
			}
			if !utf8.ValidString(c.Name) {
				t.Errorf("%s: check name is not valid UTF-8: %q", dir, c.Name)
			}
			if c.Name == "" {
				t.Errorf("%s: a check has no name", dir)
			}
			if seen[c.Name] {
				t.Errorf("%s: duplicate check name %q", dir, c.Name)
			}
			seen[c.Name] = true
		}
	}
}

func TestImportingTwiceGivesTheSameAnswer(t *testing.T) {
	t.Parallel()

	for _, dir := range corpusRepos(t) {
		first, err := ImportDir(dir, DefaultAssumptions())
		if err != nil {
			continue
		}

		var a, b bytes.Buffer
		if err := first.YAML(&a); err != nil {
			t.Fatal(err)
		}

		for i := 0; i < 3; i++ {
			again, err := ImportDir(dir, DefaultAssumptions())
			if err != nil {
				t.Fatalf("%s: %v", dir, err)
			}

			b.Reset()
			if err := again.YAML(&b); err != nil {
				t.Fatal(err)
			}

			if a.String() != b.String() {
				t.Fatalf("%s: run %d produced different output from the first", dir, i+1)
			}
		}
	}
}

func TestTheTestHelperImportsTheSameWayTheToolDoes(t *testing.T) {
	t.Parallel()

	const workflow = `
name: CI
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.25', '1.26']
        mode: [fast, slow]
    steps:
      - run: make ci
      - run: make lint
`

	viaHelper := importYAML(t, workflow)

	dir := t.TempDir()
	wf := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "ci.yml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}

	viaTool, err := ImportDir(wf, DefaultAssumptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(viaHelper.Checks) != len(viaTool.Checks) {
		t.Fatalf("the helper produced %d checks and the tool produced %d; the tests are exercising a different path",
			len(viaHelper.Checks), len(viaTool.Checks))
	}

	for i := range viaHelper.Checks {
		if viaHelper.Checks[i].Name != viaTool.Checks[i].Name ||
			viaHelper.Checks[i].Command != viaTool.Checks[i].Command {
			t.Errorf("check %d differs between the helper and the tool:\n  helper: %q\n  tool:   %q",
				i, viaHelper.Checks[i].Name, viaTool.Checks[i].Name)
		}
	}
}
