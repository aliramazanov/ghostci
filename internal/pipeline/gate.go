package pipeline

import (
	"errors"
	"fmt"
	"strings"
)

type Verdict int

const (
	Runs Verdict = iota

	Skipped

	Undecidable
)

type Gate func(event string) (bool, error)

func Decide[T any](candidates []T, gate func(T) (bool, error)) (verdict Verdict, passing []T, err error) {
	var firstErr error

	for _, event := range candidates {
		ok, evalErr := gate(event)
		if evalErr != nil {
			if firstErr == nil {
				firstErr = evalErr
			}

			continue
		}
		if ok {
			passing = append(passing, event)
		}
	}

	switch {
	case len(passing) > 0:
		return Runs, passing, nil
	case firstErr != nil:
		return Undecidable, nil, firstErr
	}

	return Skipped, nil, nil
}

func GateReason(kind, condition string, err error) string {
	var ue *UndecidableError
	if errors.As(err, &ue) {
		return fmt.Sprintf("%s %s: %s", kind, Trim(condition), ue.Error())
	}

	return fmt.Sprintf("%s %s could not be evaluated: %v", kind, Trim(condition), err)
}

func Trim(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 60 {
		return s
	}

	return s[:57] + "..."
}
