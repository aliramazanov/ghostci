package expr

import "testing"

func TestFormatRejectsMalformedPlaceholders(t *testing.T) {
	t.Parallel()

	for _, tmpl := range []string{"{0abc}", "{ 0 }", "{}", "{-1}", "{0", "}"} {
		if out, err := format(tmpl, []any{"x"}); err == nil {
			t.Errorf("format(%q) = %q, want an error", tmpl, out)
		}
	}
}

func TestFormatEscapesBraces(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"{{Hello {0}!}}": "{Hello world!}",
		"{0}{0}":         "worldworld",
		"no braces":      "no braces",
	}

	for tmpl, want := range tests {
		got, err := format(tmpl, []any{"world"})
		if err != nil {
			t.Errorf("format(%q): %v", tmpl, err)
			continue
		}

		if got != want {
			t.Errorf("format(%q) = %q, want %q", tmpl, got, want)
		}
	}
}

func TestStringFunctionsIgnoreCase(t *testing.T) {
	t.Parallel()

	ctx := Context{}
	for _, expr := range []string{
		"startsWith('Hello world', 'HELLO')",
		"endsWith('Hello world', 'WORLD')",
		"contains('Hello world', 'LO WO')",
	} {
		ok, err := EvalCondition(expr, ctx)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}

		if !ok {
			t.Errorf("%s = false, want true", expr)
		}
	}
}
