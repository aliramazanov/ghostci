package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

var version string

var commit string

var pseudo = regexp.MustCompile(`-(0\.)?\d{14}-[0-9a-f]{12}`)

func Version() string {
	if version != "" {
		return version
	}

	if released := moduleVersion(); released != "" {
		return released
	}

	return "dev"
}

func Full() string {
	if commit == "" {
		commit = vcsRevision()
	}
	if commit == "" {
		return Version()
	}

	return Version() + " (" + commit + ")"
}

func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	return released(info.Main.Version)
}

func released(v string) string {
	if v == "" || v == "(devel)" || pseudo.MatchString(v) || strings.HasSuffix(v, "+dirty") {
		return ""
	}

	return strings.TrimPrefix(v, "v")
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
