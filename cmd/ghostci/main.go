package main

import (
	"fmt"
	"os"

	"github.com/aliramazanov/ghostci/internal/version"
)

const (
	exitOK          = 0
	exitFailed      = 1
	exitBadUsage    = 2
	exitInterrupted = 130
)

const (
	defaultConfig = "ghostci.yaml"
	cacheDir      = ".ghostci/"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(runChecks(nil))
	}

	switch args[0] {
	case "init":
		os.Exit(runInit(args[1:]))
	case "import":
		os.Exit(runImport(args[1:]))
	case "install-hook":
		os.Exit(runInstallHook(args[1:]))
	case "uninstall-hook":
		os.Exit(runUninstallHook(args[1:]))
	case "version", "-v", "--version":
		fmt.Println("ghostci " + version.Full())
		os.Exit(exitOK)
	case "help", "-h", "--help":
		usage()
		os.Exit(exitOK)
	}

	os.Exit(runChecks(args))
}

func usage() {
	fmt.Fprint(os.Stderr, `ghostci answers "will my push pass CI?" in seconds.

usage:
  ghostci [flags]      run the checks in ghostci.yaml, in parallel
  ghostci init         read .github/workflows and write ghostci.yaml
  ghostci import       print what init would extract, without writing
  ghostci install-hook run checks automatically before every git push
  ghostci uninstall-hook  remove that hook
  ghostci version      print the version

run "ghostci <command> -h" for the flags of each command.

flags for the run above:
`)

	fs := runFlagSet(&options{})
	fs.SetOutput(os.Stderr)
	fs.PrintDefaults()
}
