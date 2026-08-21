package runner

import (
	"context"
	"io"
)

type Command struct {
	Shell  string
	Script string
	Dir    string
	Env    []string
}

type Executor interface {
	Execute(ctx context.Context, cmd Command, out io.Writer) error
}

func NewExecutor() Executor { return processExecutor{} }
