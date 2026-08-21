package selector

import "testing"

func TestDoubleStarCrossesDirectories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		glob, file string
		want       bool
	}{
		{"**.js", "src/app.js", true},
		{"**.js", "app.js", true},
		{"**.js", "src/app.ts", false},
		{"src/**.js", "src/deep/app.js", true},
		{"src/**.js", "other/app.js", false},
		{"**/*.js", "src/app.js", true},
		{"**", "a/b/c", true},

		{"*.js", "src/app.js", false},
		{"src/*", "src/deep/app.js", false},
	}

	for _, tc := range tests {
		if got := matchesInput(tc.glob, tc.file); got != tc.want {
			t.Errorf("matchesInput(%q, %q) = %v, want %v", tc.glob, tc.file, got, tc.want)
		}
	}
}

func TestSegmentDoubleStars(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"**.js":      "**/*.js",
		"src/**.js":  "src/**/*.js",
		"**":         "**",
		"**/*.js":    "**/*.js",
		"src/**":     "src/**",
		"docs/**/*":  "docs/**/*",
		"plain/path": "plain/path",
	}

	for in, want := range tests {
		if got := segmentDoubleStars(in); got != want {
			t.Errorf("segmentDoubleStars(%q) = %q, want %q", in, got, want)
		}
	}
}
