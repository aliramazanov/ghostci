package importer

import (
	"fmt"
	"strings"

	"github.com/aliramazanov/ghostci/internal/expr"
)

const shellControl = ";|&`\n"

func checkInjection(run string, ctx expr.Context) []string {
	if !strings.Contains(run, "${{") {
		return nil
	}

	var warnings []string

	for _, raw := range expr.Fragments(run) {
		value, err := expr.EvalString(raw, ctx)
		if err != nil || value == "" {
			continue
		}

		if strings.ContainsAny(value, shellControl) || strings.Contains(value, "$(") {
			warnings = append(warnings, fmt.Sprintf(
				"${{ %s }} resolves to %q, which contains shell control characters "+
					"and will run as more than one command", raw, value))
		}
	}

	return warnings
}
