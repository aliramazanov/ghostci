package selector

import (
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/aliramazanov/ghostci/internal/config"
)

type Decision struct {
	Check    config.Check
	Run      bool
	Matched  string
	Globs    []string
	Excluded []string
	Unknown  bool
}

func Select(checks []config.Check, changes Changes) []Decision {
	out := make([]Decision, 0, len(checks))

	for _, chk := range checks {
		switch {
		case !changes.Complete:
			out = append(out, Decision{Check: chk, Run: true, Unknown: true})

		case chk.AlwaysRuns():
			out = append(out, Decision{Check: chk, Run: true, Unknown: true})

		default:
			match, ok := firstMatch(chk, changes.Files)
			out = append(out, Decision{
				Check: chk, Run: ok, Matched: match,
				Globs: chk.Inputs, Excluded: chk.Exclude,
			})
		}
	}

	return out
}

func firstMatch(chk config.Check, files []string) (string, bool) {
	match := Matcher(chk)

	for _, f := range files {
		if match(f) {
			return f, true
		}
	}

	return "", false
}

func Match(glob, file string) bool { return matchesInput(glob, file) }

func matchesInput(glob, file string) bool {
	ok, err := match(glob, file)

	return err != nil || ok
}

func matchesExclude(glob, file string) bool {
	ok, err := match(glob, file)

	return err == nil && ok
}

func match(glob, file string) (bool, error) {
	file = path.Clean(strings.ReplaceAll(file, "\\", "/"))
	glob = segmentDoubleStars(strings.TrimPrefix(path.Clean(glob), "./"))

	ok, err := doublestar.Match(glob, file)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}

	if rest, found := strings.CutPrefix(glob, "**/"); found {
		if ok, err := doublestar.Match(rest, file); err == nil && ok {
			return true, nil
		}
	}

	if !strings.ContainsAny(glob, "*?[") && strings.HasPrefix(file, glob+"/") {
		return true, nil
	}

	return false, nil
}

func segmentDoubleStars(glob string) string {
	var sb strings.Builder

	for i := 0; i < len(glob); i++ {
		if glob[i] != '*' || i+1 >= len(glob) || glob[i+1] != '*' {
			sb.WriteByte(glob[i])

			continue
		}

		atSegmentStart := i == 0 || glob[i-1] == '/'
		trailing := i+2 < len(glob) && glob[i+2] != '/'

		sb.WriteString("**")
		if atSegmentStart && trailing {
			sb.WriteString("/*")
		}

		i++
	}

	return sb.String()
}

func Partition(decisions []Decision) (run []config.Check, skipped []Decision) {
	for _, d := range decisions {
		if d.Run {
			run = append(run, d.Check)
			continue
		}
		skipped = append(skipped, d)
	}
	return run, skipped
}

func Matcher(chk config.Check) func(file string) bool {
	return func(file string) bool {
		if !anyMatch(chk.Inputs, file, matchesInput) {
			return false
		}

		return !anyMatch(chk.Exclude, file, matchesExclude)
	}
}

func anyMatch(globs []string, file string, matches func(glob, file string) bool) bool {
	for _, g := range globs {
		if matches(g, file) {
			return true
		}
	}

	return false
}
