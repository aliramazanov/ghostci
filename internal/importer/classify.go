package importer

import "strings"

type Outcome int

const (
	Extracted Outcome = iota
	SkippedByCondition
	DroppedAction
	ToolchainAction
	LocalAction
	UnsupportedAction
	NeedsReview
	UnsupportedFeature
)

func (o Outcome) String() string {
	switch o {
	case Extracted:
		return "extracted"
	case SkippedByCondition:
		return "skipped"
	case DroppedAction:
		return "dropped"
	case ToolchainAction:
		return "toolchain"
	case LocalAction:
		return "local-action"
	case UnsupportedAction:
		return "unsupported-action"
	case NeedsReview:
		return "needs-review"
	case UnsupportedFeature:
		return "unsupported"
	}
	return "unknown"
}

var droppedActions = map[string]string{
	"actions/checkout":              "the working tree is already checked out",
	"actions/cache":                 "caching is a CI concern",
	"actions/cache/restore":         "caching is a CI concern",
	"actions/cache/save":            "caching is a CI concern",
	"actions/upload-artifact":       "artifacts are a CI concern",
	"actions/download-artifact":     "artifacts are a CI concern",
	"swatinem/rust-cache":           "caching is a CI concern",
	"actions/upload-pages-artifact": "artifacts are a CI concern",
	"actions/configure-pages":       "deployment is a CI concern",
	"actions/deploy-pages":          "deployment is a CI concern",
}

func classifyUses(uses string) (Outcome, string) {
	name := strings.ToLower(strings.SplitN(uses, "@", 2)[0])

	if strings.HasPrefix(name, "./") || strings.HasPrefix(name, ".\\") {
		return LocalAction, "local composite action, not expanded"
	}
	if reason, ok := droppedActions[name]; ok {
		return DroppedAction, reason
	}
	if isToolchain(name) {
		return ToolchainAction, "toolchain setup, version recorded"
	}
	return UnsupportedAction, "marketplace action has no local equivalent"
}

func isToolchain(name string) bool {
	for _, marker := range []string{"setup-", "/setup", "toolchain", "install-action", "-version"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

var refToolchains = map[string]bool{
	"dtolnay/rust-toolchain": true,
	"actions-rs/toolchain":   true,
}

func toolchainVersion(uses string, with map[string]string) string {
	for _, k := range []string{
		"go-version", "node-version", "python-version", "java-version",
		"toolchain", "version", "deno-version", "bun-version", "ruby-version",
	} {
		if v, ok := with[k]; ok && v != "" {
			return v
		}
	}

	if repo, ref, ok := strings.Cut(uses, "@"); ok && refToolchains[strings.ToLower(repo)] {
		return ref
	}

	return ""
}

func toolchainName(uses string) string {
	name := strings.SplitN(uses, "@", 2)[0]
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimPrefix(name, "setup-")
}
