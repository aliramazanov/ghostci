package runner

import (
	"time"
)

type Status int

const (
	StatusPassed Status = iota
	StatusFailed
	StatusTimedOut
	StatusCancelled
	StatusSkipped
	StatusCached
	StatusStartError

	StatusUnavailable
)

func (s Status) String() string {
	switch s {
	case StatusPassed:
		return "ok"
	case StatusFailed:
		return "FAIL"
	case StatusTimedOut:
		return "TIMEOUT"
	case StatusCancelled:
		return "cancelled"
	case StatusSkipped:
		return "skipped"
	case StatusCached:
		return "cached"
	case StatusStartError:
		return "ERROR"
	case StatusUnavailable:
		return "no tool"
	}
	return "unknown"
}

func (s Status) CountsAsFailure() bool {
	return s == StatusFailed || s == StatusTimedOut || s == StatusStartError
}

type Result struct {
	Optional bool

	Index    int
	Name     string
	Status   Status
	Duration time.Duration
	Output   string
	Reason   string
	Err      error

	// ExitCode is the status the check's shell returned. It is -1 when no
	// process reached an exit of its own, which covers a check that was
	// skipped, served from cache, killed by a signal, or never started.
	// Knowing the number is what separates a failing test from a shell that
	// could not run the command at all.
	ExitCode int
}
