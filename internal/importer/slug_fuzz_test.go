package importer

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzSlugMakesAUsableName(f *testing.F) {
	for _, s := range []string{
		"", "Run tests", "Проверка кода", "テスト", "build ✅",
		strings.Repeat("very long step name ", 20),
		"../../etc/passwd", "name with\nnewline", "  ",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		got := slug(s)

		if got == "" {
			t.Errorf("slug(%q) is empty; every check needs a name", s)
		}

		if !utf8.ValidString(got) {
			t.Errorf("slug(%q) = %q, which is not valid UTF-8", s, got)
		}

		if strings.ContainsAny(got, "\n\r\x00") {
			t.Errorf("slug(%q) = %q, which cannot go on one line", s, got)
		}

		if len(got) > 48 {
			t.Errorf("slug(%q) returned %d bytes, want at most 48", s, len(got))
		}
	})
}
