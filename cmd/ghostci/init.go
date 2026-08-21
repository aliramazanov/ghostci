package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aliramazanov/ghostci/internal/git"
	"github.com/aliramazanov/ghostci/internal/importer"
	"github.com/aliramazanov/ghostci/internal/toolchain"
)

func runInit(args []string) int {
	fs := flag.NewFlagSet("ghostci init", flag.ContinueOnError)
	var (
		dir    = fs.String("dir", ".github/workflows", "workflow directory to import from")
		out    = fs.String("o", defaultConfig, "config file to write")
		force  = fs.Bool("force", false, "overwrite an existing config")
		dryRun = fs.Bool("dry-run", false, "print the config instead of writing it")
		from   = fs.String("from", "", "import this file instead of the detected one (.gitlab-ci.yml or a workflow)")
	)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "usage: ghostci init [flags]\n\n"+
			"Reads .github/workflows or .gitlab-ci.yml and writes a reviewable ghostci.yaml.\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitBadUsage
	}

	if _, err := os.Stat(*out); err == nil && !*force && !*dryRun {
		fmt.Fprintf(os.Stderr, "ghostci: %s already exists (use --force to overwrite)\n", *out)
		return exitBadUsage
	}

	a := localAssumptions()
	repo := git.Discover(".")

	path, _ := resolveSource(*dir, *from)

	res, err := importer.Import(path, a)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
		return exitBadUsage
	}
	if len(res.Checks) == 0 && len(res.Heavy) == 0 {
		res.Ledger(os.Stdout, a)
		fmt.Fprintf(os.Stderr, "\nghostci: nothing runnable was extracted from %s\n", describeSource(path))
		return exitBadUsage
	}

	var buf bytes.Buffer
	if err := res.YAML(&buf); err != nil {
		fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)
		return exitBadUsage
	}

	if *dryRun {
		if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
			fmt.Fprintf(os.Stderr, "ghostci: %v\n", err)

			return exitBadUsage
		}

		return exitOK
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "ghostci: writing %s: %v\n", *out, err)
		return exitBadUsage
	}

	res.Ledger(os.Stdout, a)
	reportToolchains(os.Stdout, res)
	if repo.IsRepo {
		ignoreCacheDir(os.Stdout, repo.Root)
	}

	fmt.Printf("\nwrote %s\n", *out)
	fmt.Printf("review it, then run: ghostci\n")
	return exitOK
}

func reportToolchains(w *os.File, res *importer.Result) {
	var mismatches []string
	for _, t := range res.Toolchains {
		local := toolchain.Local(t.Name)
		if local == "" {
			continue
		}
		if !toolchain.Matches(t.Version, local) {
			mismatches = append(mismatches,
				fmt.Sprintf("  %s: CI pins %s, you have %s", t.Name, t.Version, local))
		}
	}
	if len(mismatches) == 0 {
		return
	}
	fmt.Fprintf(w, "\ntoolchain mismatches with CI:\n%s\n", strings.Join(mismatches, "\n"))
	fmt.Fprintf(w, "  checks may pass locally and still fail in CI\n")
}

func ignoreCacheDir(w *os.File, root string) {
	path := filepath.Join(root, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == cacheDir {
			return
		}
	}

	body := string(existing)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += cacheDir + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return
	}
	fmt.Fprintf(w, "\nadded %s to .gitignore\n", cacheDir)
}
