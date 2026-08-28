package expr

import (
	"errors"
	"testing"
)

func localCtx() Context {
	return Context{
		"github": map[string]any{
			"ref":        "refs/heads/feature-branch",
			"event_name": "push",
			"repository": "aliramazanov/ghostci",
			"sha":        "abc123",
			"workspace":  "/repo",
			"event":      map[string]any{},
		},
		"runner": map[string]any{"os": "Linux", "arch": "X64"},
		"matrix": map[string]any{"os": "ubuntu-latest", "go": "1.26", "shard_index": 0.0},
		"env":    map[string]any{"CI": "true"},
	}
}

func TestEvalCondition(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		expr string
		want bool
	}{
		"empty means run":       {"", true},
		"bare true":             {"true", true},
		"bare false":            {"false", false},
		"wrapped":               {"${{ true }}", true},
		"string equality":       {"github.event_name == 'push'", true},
		"string inequality":     {"github.event_name != 'push'", false},
		"case insensitive eq":   {"github.event_name == 'PUSH'", true},
		"negation":              {"!startsWith(github.ref, 'refs/tags/')", true},
		"startsWith true":       {"startsWith(github.ref, 'refs/heads/')", true},
		"endsWith":              {"endsWith(github.ref, 'feature-branch')", true},
		"contains string":       {"contains(github.ref, 'feature')", true},
		"and both true":         {"github.event_name == 'push' && runner.os == 'Linux'", true},
		"and one false":         {"github.event_name == 'push' && runner.os == 'Windows'", false},
		"or short circuits":     {"runner.os == 'Windows' || github.event_name == 'push'", true},
		"parens":                {"(runner.os == 'Windows' || github.event_name == 'push') && true", true},
		"matrix access":         {"matrix.os == 'ubuntu-latest'", true},
		"numeric compare":       {"matrix.shard_index == 0", true},
		"less than":             {"matrix.shard_index < 3", true},
		"missing prop is falsy": {"github.nonexistent", false},
		"missing prop equality": {"github.nonexistent == ''", true},
		"success is true":       {"success()", true},
		"always is true":        {"always()", true},
		"failure is false":      {"failure()", false},
		"cancelled is false":    {"cancelled()", false},
		"format":                {"format('{0}-{1}', 'a', 'b') == 'a-b'", true},
		"splat on missing":      {"contains(github.event.pull_request.labels.*.name, 'ci-full')", false},
		"double negation":       {"!(!true)", true},

		"corpus tag gate": {"!startsWith(github.ref, 'refs/tags/')", true},
		"corpus tag gate with matrix": {
			"!startsWith(github.ref, 'refs/tags/') && (matrix.shard_index == 0 || github.event_name == 'pull_request')", true,
		},
		"corpus label gate": {
			"!(!contains(github.event.pull_request.labels.*.name, 'ci-full') && github.event_name == 'pull_request')", true,
		},
		"corpus repo gate": {
			"github.repository == 'denoland/deno' && startsWith(github.ref, 'refs/tags/')", false,
		},
		"corpus pr only": {
			"!startsWith(github.ref, 'refs/tags/') && github.event_name == 'pull_request'", false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := EvalCondition(tc.expr, localCtx())

			if err != nil {
				t.Fatalf("%s: %v", tc.expr, err)
			}

			if got != tc.want {
				t.Errorf("%s = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestUndecidableContextsRefuse(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"needs.build.result == 'success'",
		"steps.foo.outputs.bar == '1'",
		"secrets.TOKEN != ''",
		"inputs.debug == 'true'",
		"github.event_name == 'push' && needs.build.result == 'success'",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			_, err := EvalCondition(src, localCtx())
			var ue *UndecidableError
			if !errors.As(err, &ue) {
				t.Fatalf("want UndecidableError, got %v", err)
			}
		})
	}
}

func TestShortCircuitAvoidsUndecidable(t *testing.T) {
	t.Parallel()
	got, err := EvalCondition("github.event_name == 'release' && needs.build.result == 'success'", localCtx())

	if err != nil {
		t.Fatalf("short circuit should avoid the undecidable branch: %v", err)
	}

	if got {
		t.Error("want false")
	}
}

func TestInterpolate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ in, want string }{
		"no expression":    {"go test ./...", "go test ./..."},
		"whole string":     {"${{ matrix.os }}", "ubuntu-latest"},
		"embedded":         {"go test -tags ${{ matrix.go }} ./...", "go test -tags 1.26 ./..."},
		"two":              {"${{ matrix.os }}-${{ matrix.go }}", "ubuntu-latest-1.26"},
		"numeric":          {"shard ${{ matrix.shard_index }}", "shard 0"},
		"missing is empty": {"x${{ github.nope }}y", "xy"},
		"function":         {"${{ format('{0}!', matrix.os) }}", "ubuntu-latest!"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := Interpolate(tc.in, localCtx())
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Interpolate(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReferences(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"github.ref == 'x'":                        {"github"},
		"matrix.os == 'linux' && github.sha != ''": {"matrix", "github"},
		"${{ needs.build.result }}":                {"needs"},
		"echo ${{ matrix.os }} and ${{ env.CI }}":  {"matrix", "env"},
	}

	for src, want := range tests {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			got := References(src)

			if len(got) != len(want) {
				t.Fatalf("References(%q) = %v, want %v", src, got, want)
			}

			for i := range want {
				if got[i] != want[i] {
					t.Errorf("References(%q) = %v, want %v", src, got, want)
				}
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		"github.ref ==",
		"(unclosed",
		"'unterminated",
		"github..ref",
		"@@@",
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(src); err == nil {
				t.Errorf("want parse error for %q", src)
			}
		})
	}
}

func TestQuoteEscaping(t *testing.T) {
	t.Parallel()

	got, err := EvalCondition("'it''s' == 'it''s'", localCtx())

	if err != nil || !got {
		t.Errorf("doubled quote escape failed: %v %v", got, err)
	}
}

func TestClosingBraceInsideStringLiteral(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		`${{ format('{{Hello {0}!}}', 'Mona') }}`: "{Hello Mona!}",
		`${{ format('{{literal}}') }}`:            "{literal}",
		`${{ format('{0}', 'a') }}`:               "a",
		`${{ format('{1}{0}', 'a', 'b') }}`:       "ba",
	}

	for src, want := range tests {
		got, err := Interpolate(src, localCtx())

		if err != nil {
			t.Errorf("%s: %v", src, err)

			continue
		}

		if got != want {
			t.Errorf("%s\n got %q\nwant %q", src, got, want)
		}
	}
}

func TestFormatRejectsUnsatisfiablePlaceholder(t *testing.T) {
	t.Parallel()

	for _, src := range []string{
		`${{ format('{5}', 'a') }}`,
		`${{ format('{x}', 'a') }}`,
	} {
		if _, err := Interpolate(src, localCtx()); err == nil {
			t.Errorf("%s should be an error, not silent output", src)
		}
	}
}

func TestQuoteAwareScanningSurvivesEscapes(t *testing.T) {
	t.Parallel()

	got, err := Interpolate(`${{ format('it''s {0}', 'fine') }}`, localCtx())
	if err != nil {
		t.Fatal(err)
	}

	if got != "it's fine" {
		t.Errorf("got %q", got)
	}
}

func TestOrderingComparisons(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"'abc' < 'abd'": true,
		"'abd' < 'abc'": false,
		"'b' > 'a'":     true,
		"'a' >= 'a'":    true,
		"'a' <= 'a'":    true,
		"'A' == 'a'":    true,
		"'A' >= 'a'":    true,

		"'10' > '9'":              false,
		"'15' < '9'":              true,
		"'2.5' < '10'":            false,
		"'10' > 9":                true,
		"1 < 2":                   true,
		"2 <= 2":                  true,
		"3 > 10":                  false,
		"matrix.shard_index >= 0": true,
	}

	for src, want := range cases {
		got, err := EvalCondition(src, localCtx())
		if err != nil {
			t.Errorf("%s: %v", src, err)

			continue
		}

		if got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
}

func TestArraysAndObjectsStringifyAsGitHubNamesThem(t *testing.T) {
	t.Parallel()

	ctx := Context{"x": map[string]any{
		"list": []any{"a", "b"},
		"obj":  map[string]any{"k": "v"},
	}}

	for src, want := range map[string]string{
		"${{ x.list }}": "Array",
		"${{ x.obj }}":  "Object",
	} {
		got, err := Interpolate(src, ctx)
		if err != nil {
			t.Fatal(err)
		}

		if got != want {
			t.Errorf("%s = %q, want %q", src, got, want)
		}
	}
}

func TestIndexReadsTheSameAsAProperty(t *testing.T) {
	t.Parallel()

	ctx := Context{"env": Strict{Name: "env", Values: map[string]any{"KNOWN": "yes"}}}

	for _, src := range []string{"env.KNOWN", "env['KNOWN']"} {
		got, err := EvalString(src, ctx)

		if err != nil {
			t.Errorf("%s: %v", src, err)
		}

		if got != "yes" {
			t.Errorf("%s = %q, want yes", src, got)
		}
	}

	for _, src := range []string{"env.MISSING", "env['MISSING']"} {
		if _, err := EvalString(src, ctx); err == nil {
			t.Errorf("%s resolved without error; an absent name is not the empty string", src)
		}
	}
}
