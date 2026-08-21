package toolchain

import "testing"

func TestMatchesProperties(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pinned, local string
		want          bool
		why           string
	}{
		{"1.26", "1.26.5", true, "a pin names a prefix, not an exact build"},
		{"1.26.5", "1.26.5", true, "exact"},
		{"1.2", "1.20.0", false, "1.2 must not match 1.20: components compare whole"},
		{"1.20", "1.2", false, "a shorter local version cannot satisfy a longer pin"},
		{"v1.26", "1.26.1", true, "a leading v is noise"},
		{"1.26.x", "1.26.4", true, "an x placeholder means any patch"},
		{"1.26.*", "1.26.4", true, "a star placeholder means any patch"},
		{"stable", "1.26.5", true, "a channel name pins nothing checkable"},
		{"lts", "20.1.0", true, "a channel name pins nothing checkable"},
		{"", "1.26.5", true, "nothing pinned"},
		{"1.26", "", true, "nothing local to compare"},
		{"1.26", "1.27.0", false, "a different minor is a mismatch"},
		{"2", "20.1.0", false, "major 2 must not match major 20"},
		{"not-a-version", "1.26", true, "an unparseable pin cannot be judged"},
	}

	for _, c := range cases {
		if got := Matches(c.pinned, c.local); got != c.want {
			t.Errorf("Matches(%q, %q) = %v, want %v: %s", c.pinned, c.local, got, c.want, c.why)
		}
	}
}

func FuzzMatches(f *testing.F) {
	for _, s := range [][2]string{
		{"1.26", "1.26.5"}, {"", ""}, {"v", "v"}, {"...", "1"},
		{"1.2.3.4.5.6", "1"}, {"999999999999999999999", "1"},
	} {
		f.Add(s[0], s[1])
	}

	f.Fuzz(func(t *testing.T, pinned, local string) {
		got := Matches(pinned, local)

		if (pinned == "" || local == "") && !got {
			t.Errorf("Matches(%q, %q) = false with nothing to compare", pinned, local)
		}

		if local != "" && !Matches(local, local) {
			t.Errorf("Matches(%q, %q) = false: a version must satisfy itself", local, local)
		}
	})
}
