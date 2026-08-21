package git

import "testing"

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want int
	}{
		{"2.22", "2.22", 0},
		{"2.30", "2.22", 1},
		{"2.21", "2.22", -1},
		{"3.0", "2.22", 1},
		{"2.22.1", "2.22", 1},
		{"1.9", "2.22", -1},
	}

	for _, tc := range tests {
		got := compareVersions(tc.a, tc.b)
		if (got < 0) != (tc.want < 0) || (got > 0) != (tc.want > 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheckVersionToleratesUnknown(t *testing.T) {
	t.Parallel()

	if err := CheckVersion(t.TempDir()); err != nil {
		t.Errorf("unknown git version should not be an error, got %v", err)
	}
}
