package selector

import (
	"strings"

	"github.com/aliramazanov/ghostci/internal/git"
)

type Changes struct {
	Files    []string
	Base     string
	Complete bool
	Reason   string
}

func DetectChanges(sess *git.Session, base string) Changes {
	if !sess.IsRepo() {
		return Changes{Reason: "not a git repository, running every check"}
	}

	resolved, reason, inferred := resolveBase(sess, base)
	if resolved == "" {
		return Changes{Reason: reason}
	}
	if inferred && sess.SameCommit(resolved, "HEAD") {
		return Changes{Reason: "no remote state to compare against (" + reason +
			" is HEAD), running every check"}
	}

	files := map[string]bool{}

	changed, err := sess.Changed(resolved)
	if err != nil {
		return Changes{Reason: "could not diff against " + resolved + ", running every check"}
	}
	for _, p := range changed {
		files[p] = true
	}

	dirty, err := sess.Dirty()
	if err != nil {
		return Changes{Reason: "could not read the working tree, running every check"}
	}

	for _, p := range dirty {
		files[p] = true
	}

	return Changes{
		Files:    paths(files),
		Base:     resolved,
		Complete: true,
		Reason:   "compared against " + reason,
	}
}

func resolveBase(sess *git.Session, base string) (resolved, reason string, inferred bool) {
	if base != "" {

		if strings.HasPrefix(base, "-") {
			return "", "the given ref is not a revision, running every check", false
		}
		if _, err := sess.Verify(base + "^{commit}"); err == nil {
			return base, base, false
		}
		return "", "the pushed ref is not available locally, running every check", false
	}

	if up, err := sess.Upstream(); err == nil && up != "" {
		return up, up, false
	}

	for _, candidate := range defaultBranches(sess) {
		mb, err := sess.MergeBase("HEAD", candidate)
		if err != nil || mb == "" {
			continue
		}
		return mb, "merge-base with " + candidate, !isRemoteTracking(sess, candidate)
	}
	return "", "no upstream or default branch to compare against, running every check", false
}

func isRemoteTracking(sess *git.Session, ref string) bool {
	_, err := sess.Verify("refs/remotes/" + ref)
	return err == nil
}

func defaultBranches(sess *git.Session) []string {
	var out []string
	if ref, err := sess.OriginHead(); err == nil && ref != "" {
		out = append(out, strings.TrimPrefix(ref, "refs/remotes/"))
	}
	return append(out, "origin/main", "origin/master", "main", "master")
}

func paths(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
