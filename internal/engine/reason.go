package engine

import (
	"fmt"
	"strings"
)

type ReasonKind int

const (
	ChangedFile ReasonKind = iota

	NoMatch

	UnknownInputs

	IncompleteChanges

	Forced

	InputsUnchanged

	FingerprintChanged

	NeverPassed

	WatchesNothing
)

type Reason struct {
	Kind ReasonKind

	File string

	Globs []string

	Excluded []string

	Diffs []string

	Detail string
}

func (r Reason) String() string {
	switch r.Kind {
	case ChangedFile:
		return r.File + " changed"
	case NoMatch:
		out := "no changed file matches " + strings.Join(r.Globs, ", ")
		if len(r.Excluded) > 0 {
			out += " outside " + strings.Join(r.Excluded, ", ")
		}

		return out
	case UnknownInputs:
		return "no input globs declared, so it cannot be safely skipped"
	case IncompleteChanges:
		return r.Detail
	case Forced:
		return "--all was given"
	case InputsUnchanged:
		return "inputs unchanged since it passed"
	case FingerprintChanged:
		return strings.Join(r.Diffs, "; ")
	case NeverPassed:
		return "no previous passing run"
	case WatchesNothing:
		return "its inputs match no file here, so nothing can show it is unaffected"
	}

	return fmt.Sprintf("unknown reason %d", r.Kind)
}
