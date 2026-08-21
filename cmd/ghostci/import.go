package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aliramazanov/ghostci/internal/importer"
)

func runImport(args []string) int {
	fs := flag.NewFlagSet("ghostci import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		dir      = fs.String("dir", ".github/workflows", "workflow directory")
		file     = fs.String("file", "", "import this file instead of the detected one")
		showYAML = fs.Bool("yaml", false, "print the generated config instead of the ledger")
		ref      = fs.String("ref", "", "override the assumed github.ref")
		events   = fs.String("events", "push,pull_request", "assumed github.event_name values, comma separated")
	)

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "usage: ghostci import [flags]\n\n"+
			"Prints what init would extract, without writing anything.\n\nflags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return exitBadUsage
	}

	a := localAssumptions()
	a.Events = strings.Split(*events, ",")

	if *ref != "" {
		a.Ref = *ref
	}

	path, _ := resolveSource(*dir, *file)

	res, err := importer.Import(path, a)

	if err != nil {
		fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
		return exitBadUsage
	}

	if *showYAML {
		if err := res.YAML(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
			return exitBadUsage
		}

		return exitOK
	}

	res.Ledger(os.Stdout, a)

	return exitOK
}
