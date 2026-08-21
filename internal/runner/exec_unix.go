//go:build unix

package runner

import (
	"context"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	killGrace = 2 * time.Second
	waitDelay = 3 * time.Second
)

type processExecutor struct{}

func Unsupported() error { return nil }

func (processExecutor) Execute(ctx context.Context, c Command, out io.Writer) error {
	name, args := invocation(c.Shell, c.Script)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	cmd.Stdout = out
	cmd.Stderr = out

	cmd.Stdin = nil

	var (
		mu        sync.Mutex
		killTimer *time.Timer
	)

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := -cmd.Process.Pid
		if err := syscall.Kill(pgid, syscall.SIGINT); err != nil {
			return syscall.Kill(pgid, syscall.SIGKILL)
		}

		mu.Lock()
		killTimer = time.AfterFunc(killGrace, func() { _ = syscall.Kill(pgid, syscall.SIGKILL) })
		mu.Unlock()

		return nil
	}

	cmd.WaitDelay = waitDelay

	err := cmd.Run()
	if cmd.Process != nil && ctx.Err() != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	mu.Lock()
	if killTimer != nil {
		killTimer.Stop()
	}
	mu.Unlock()

	return err
}
