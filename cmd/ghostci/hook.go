package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/aliramazanov/ghostci/internal/hook"
)

func runInstallHook(args []string) int {
	fs := flag.NewFlagSet("ghostci install-hook", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("force", false, "replace an existing pre-push hook that ghostci did not write")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "usage: ghostci install-hook [--force]\n\n"+
			"Writes a git pre-push hook that runs ghostci before every push.\n\nflags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return exitBadUsage
	}

	path, err := hook.Install(".", *force)
	if err != nil {
		var shared *hook.ErrSharedHooksPath
		if errors.As(err, &shared) {
			fmt.Fprintf(os.Stderr,
				"ghostci: core.hooksPath points at %s, which is outside this repository.\n"+
					"Installing there would add this hook to every repository that uses it.\n"+
					"Re-run with --force if that is what you want, or unset it:\n"+
					"  git config --unset core.hooksPath\n", shared.Dir)

			return exitBadUsage
		}

		if errors.Is(err, hook.ErrForeignHook) {
			fmt.Fprintf(os.Stderr, "ghostci: a pre-push hook already exists and ghostci did not write it.\n"+
				"Inspect it, then re-run with --force to replace it.\n")
			return exitBadUsage
		}
		fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
		return exitBadUsage
	}

	fmt.Printf("installed %s\n", path)
	fmt.Printf("checks now run before every push. Skip once with: git push --no-verify\n")
	return exitOK
}

func runUninstallHook(args []string) int {
	fs := flag.NewFlagSet("ghostci uninstall-hook", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitBadUsage
	}

	path, err := hook.Uninstall(".")
	if err != nil {
		switch {
		case errors.Is(err, hook.ErrNotInstalled):
			fmt.Fprintf(os.Stderr, "ghostci: no hook installed\n")
		case errors.Is(err, hook.ErrForeignHook):
			fmt.Fprintf(os.Stderr, "ghostci: the pre-push hook was not written by ghostci, leaving it alone\n")
		default:
			fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
		}
		return exitBadUsage
	}

	fmt.Printf("removed %s\n", path)
	return exitOK
}
