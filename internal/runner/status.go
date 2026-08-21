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
}
