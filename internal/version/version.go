package version

import "runtime/debug"

var version = "0.1.0"

var commit string

func Version() string { return version }

func Full() string {
	if commit == "" {
		commit = vcsRevision()
	}
	if commit == "" {
		return version
	}

	return version + " (" + commit + ")"
}

func vcsRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}

	return ""
}
