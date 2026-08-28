package pipeline

import (
	"testing"
	"unicode/utf8"
)

func FuzzTrimStaysValidText(f *testing.F) {
	for _, s := range []string{
		"", "plain ascii", "héllo wörld", "日本語のテキスト",
		"🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂🙂",
		"a very long condition that goes well past the limit and keeps going and going",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		got := Trim(s)

		if utf8.ValidString(s) && !utf8.ValidString(got) {
			t.Errorf("Trim(%q) = %q, which is not valid UTF-8", s, got)
		}
		if len(got) > 60 {
			t.Errorf("Trim returned %d bytes, want at most 60", len(got))
		}
	})
}
