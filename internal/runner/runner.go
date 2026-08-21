package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/aliramazanov/ghostci/internal/config"
)

const defaultOutputLimit = 64 << 10

type Options struct {
	Root string

	Jobs        int
	FailFast    bool
	Timeout     time.Duration
	OutputLimit int
	Shell       string
	Context     Context
	OnResult    func(Result)
	Executor    Executor
}

func (o *Options) applyDefaults() {
	if o.Jobs <= 0 {
		o.Jobs = runtime.NumCPU()
	}
	if o.OutputLimit <= 0 {
		o.OutputLimit = defaultOutputLimit
	}
	if o.Shell == "" {
		o.Shell = DefaultShell()
	}
	if o.Executor == nil {
		o.Executor = NewExecutor()
	}
}

var errFailFast = errors.New("runner: fail-fast triggered")

func Run(ctx context.Context, checks []config.Check, opts Options) []Result {
	opts.applyDefaults()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]Result, len(checks))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Jobs)

	var serial sync.Mutex

	for i, chk := range checks {
		g.Go(func() error {
			if chk.Serial {
				serial.Lock()
				defer serial.Unlock()
			}

			res := runOne(gctx, i, chk, opts)
			results[i] = res

			if opts.OnResult != nil {
				opts.OnResult(res)
			}
			if opts.FailFast && res.Status.CountsAsFailure() {
				return errFailFast
			}

			return nil
		})
	}
	_ = g.Wait()

	return results
}

func runOne(ctx context.Context, index int, chk config.Check, opts Options) Result {
	res := Result{Index: index, Name: chk.Name, Optional: chk.Optional}

	if err := ctx.Err(); err != nil {
		res.Status = StatusCancelled

		return res
	}

	if timeout := checkTimeout(chk, opts); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := command(chk, opts, nil)
	if info, err := os.Stat(cmd.Dir); cmd.Dir != "" && (err != nil || !info.IsDir()) {
		res.Status = StatusUnavailable
		res.Err = fmt.Errorf("working directory %s does not exist here", cmd.Dir)
		res.Output = res.Err.Error()

		return res
	}

	scratch, cleanup := scratchEnv()
	defer cleanup()

	buf := newBoundedBuffer(opts.OutputLimit)
	start := time.Now()
	err := opts.Executor.Execute(ctx, command(chk, opts, scratch), buf)

	res.Duration = time.Since(start)
	res.Output = buf.String()
	res.Status, res.Err = classify(ctx, err)

	return res
}

func command(chk config.Check, opts Options, extra []string) Command {
	shell := opts.Shell
	if chk.Shell != "" {
		shell = chk.Shell
	}

	return Command{
		Shell:  shell,
		Script: chk.Command,
		Dir:    workdir(opts.Root, chk.Dir),
		Env:    env(chk.Env, opts.Context.Env(), extra, neutralise(shell, chk.Env)),
	}
}

func workdir(root, dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}

	return filepath.Join(root, dir)
}

func checkTimeout(chk config.Check, opts Options) time.Duration {
	if chk.Timeout > 0 {
		return time.Duration(chk.Timeout)
	}

	return opts.Timeout
}

func classify(ctx context.Context, err error) (Status, error) {
	if err == nil {
		return StatusPassed, nil
	}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return StatusTimedOut, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return StatusCancelled, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {

		switch exitErr.ExitCode() {
		case 126, 127:
			return StatusUnavailable, nil
		}

		return StatusFailed, nil
	}

	if errors.Is(err, exec.ErrWaitDelay) {
		return StatusPassed, nil
	}

	return StatusStartError, err
}

func env(declared map[string]string, layers ...[]string) []string {
	out := os.Environ()

	for _, layer := range layers {
		out = append(out, layer...)
	}

	for k, v := range declared {
		out = append(out, k+"="+v)
	}

	return append(out, Disable+"=0")
}
