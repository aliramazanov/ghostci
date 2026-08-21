package report

import (
	"fmt"
	"io"

	"github.com/aliramazanov/ghostci/internal/engine"
)

func Explain(w io.Writer, p engine.Plan) {
	fmt.Fprintf(w, "change detection: %s\n", p.Changes.Reason)
	if p.Changes.Complete {
		fmt.Fprintf(w, "%d changed files\n", len(p.Changes.Files))
	}
	fmt.Fprintln(w)

	for _, d := range p.Decisions {
		fmt.Fprintf(w, "  %-6s %-40s %s\n", verb(d.Action), d.Check.Name, d.Reason)
	}
	fmt.Fprintln(w)
}

func verb(a engine.Action) string {
	switch a {
	case engine.Skip:
		return "skip"
	case engine.Cached:
		return "cache"
	default:
		return "run"
	}
}
