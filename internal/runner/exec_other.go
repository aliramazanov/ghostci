//go:build !unix

package runner

import (
	"context"
	"errors"
	"io"
	"runtime"
)

var errUnsupportedPlatform = errors.New(
	"ghostci does not support " + runtime.GOOS + " yet: reliable process-group " +
		"cancellation is unimplemented there, and running without it can hang")

func Unsupported() error { return errUnsupportedPlatform }

type processExecutor struct{}

func (processExecutor) Execute(context.Context, Command, io.Writer) error {
	return errUnsupportedPlatform
}
