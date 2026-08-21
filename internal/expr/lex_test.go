package expr

import "testing"

func TestNumericLiterals(t *testing.T) {
	t.Parallel()

	tests := map[string]float64{
		"0x1f":  31,
		"0b101": 5,
		"0o17":  15,
		"1e-5":  1e-5,
		"1E+3":  1000,
		"-4.5":  -4.5,
		"42":    42,
	}

	for src, want := range tests {
		toks, err := lex(src)

		if err != nil {
			t.Errorf("lex(%q): %v", src, err)
			continue
		}

		if len(toks) != 2 || toks[0].kind != tokNumber {
			t.Errorf("lex(%q) = %d tokens, want one number", src, len(toks)-1)
			continue
		}

		if toks[0].num != want {
			t.Errorf("lex(%q) = %v, want %v", src, toks[0].num, want)
		}
	}
}

func TestHyphenatedProperty(t *testing.T) {
	t.Parallel()

	ctx := Context{"matrix": map[string]any{"node-version": "20"}}

	got, err := EvalString("matrix.node-version", ctx)

	if err != nil {
		t.Fatal(err)
	}

	if got != "20" {
		t.Errorf("got %q, want 20", got)
	}
}
