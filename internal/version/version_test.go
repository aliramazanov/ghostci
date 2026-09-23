package version

import "testing"

func TestOnlyAReleaseVersionIsReported(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"v0.2.0":                               "0.2.0",
		"v1.4.3-rc.1":                          "1.4.3-rc.1",
		"(devel)":                              "",
		"":                                     "",
		"v0.0.0-20260828135455-64bc9ce07ca7":   "",
		"v0.2.1-0.20260828135455-64bc9ce07ca7": "",
		"v0.0.0-20260828135455-64bc9ce07ca7+dirty": "",
		"v0.2.0+dirty": "",
	} {
		if got := released(in); got != want {
			t.Errorf("released(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAStampedVersionWins(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	version = "9.9.9"

	if got := Version(); got != "9.9.9" {
		t.Errorf("Version() = %q, want the stamped 9.9.9", got)
	}
}
