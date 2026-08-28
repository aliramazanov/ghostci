# ghostci

**Will my push pass CI?** Answered in seconds, before you push.

ghostci reads the CI config you already have, GitHub Actions or GitLab CI, runs the checks your diff can actually affect, and skips the ones whose inputs have not changed.

```console
$ ghostci
ghostci: running 2 of 4 checks

  cached   tsc                      (inputs unchanged)
  skipped  docs-lint                (no changed file matches docs/**)
  ok       eslint            1.2s
  FAIL     test              8.7s

  4 checks: 1 ok, 1 failed, 1 cached, 1 skipped     8.7s

  ── test ────────────────────────────────
  --- FAIL: TestGreet (0.00s)
      main_test.go:7: expected "hello", got "hi"

  1 check failed. CI would fail.
```

## Install

```bash
go install github.com/aliramazanov/ghostci/cmd/ghostci@latest
```

## Quickstart

```bash
cd your-repo
ghostci init          # reads .github/workflows or .gitlab-ci.yml, writes ghostci.yaml
ghostci               # runs the checks
ghostci install-hook  # from now on, runs automatically before every push
```

`init` writes a config you review and commit. It does not run anything, and it does not guess:

```console
$ ghostci init
imported 12 checks from 34 workflow steps
plus 3 long-running steps, written commented out

  extracted              12
  dropped                 9   (checkout, cache, artifacts)
  toolchain               4   (versions recorded)
  needs-review            2

needs-review:
  ci/deploy: condition needs.build.outputs.ok == 'true': depends on needs,
             which is only known inside CI

local assumptions used to resolve conditions:
  github.ref        = refs/heads/feature/my-change
  github.event_name = push, pull_request
  github.repository = you/your-repo

wrote ghostci.yaml
```

## Why it is fast

Because it does less, not because it runs things faster.

**It skips checks your change cannot affect.** Touch a `.md` file and the Go tests do not run.

**It skips checks whose inputs are unchanged.** Content-addressed, keyed on file contents plus command plus toolchain version. Clean files reuse the hash git already computed, so the check costs nothing to evaluate.

Nothing changed since the last run, every tool's own cache already warm,
measured with `hyperfine` on one machine:

| project | its own checks, run directly | ghostci | |
|---|---:|---:|---:|
| JS/TS, 800 files (`eslint`, `tsc --noEmit`) | 1,085 ms ± 77 | **42 ms ± 3** | 26x |
| Python, 364 files (`ruff check`, `ruff format`, `pytest`) | 367 ms ± 20 | **33 ms ± 2** | 11x |
| this repo, Go (`gofmt`, `vet`, `build`, `test -race`) | 777 ms ± 40 | **36 ms ± 1** | 22x |

The gap is not that ghostci runs anything faster: it runs nothing at all,
because every check's inputs hash to a recorded pass. A warm native cache
still pays process startup, config resolution and a file walk before it can
decide there is nothing to do.

Go is the conservative case, since its build and test caches are the best of
the three, and it is the one you can reproduce from this repository. The
Python case is conservative too: `ruff` is a Rust binary that starts in
milliseconds, where a slower linter would widen the gap.

**One honest caveat.** On raw parallel execution ghostci is on par with
[lefthook](https://lefthook.dev), not faster. The win is skipping work, not
running it quicker.

## It will not lie to you

A tool that says "all clear" when CI would have failed is worse than no tool. So:

- A check whose inputs cannot be determined **always runs**, and is never cached.
- A command that **delegates** decides nothing on its own. `make test` is read from the recipe it runs and `npm test` from the script in `package.json`, not from the words `make` and `npm`: a Makefile builds whatever its recipes build, and reading one as a C build skipped `make test` in a Go repository on a change to its Go files. Anything that cannot be read leaves the check running, and one unreadable part leaves the whole check running: resolving `npm ci` while `npm test` stayed unknown would watch the lockfile alone.
- Only passes are cached. Failures re-run at full cost, every time.
- A workflow step that depends on CI-only state is written to your config **commented out with the reason**, never silently dropped.
- A malformed input glob is **rejected at load**, and if one ever reaches matching it **matches**, so a typo cannot hide a check.
- If a check **rewrites files** another check depends on, the run is not an all clear: what passed no longer describes your tree.
- Interrupting a run (Ctrl-C) is **never** an all clear.
- `ghostci --explain` prints why every check ran, was cached, or was skipped.
- A check still running after ten seconds **says so**, and keeps saying so, so
  a slow check is never mistaken for a hung one.
- `ghostci --quiet` says nothing while everything passes, and everything when
  it does not. A failure, an interruption, a check that verified nothing or
  watched nothing, and a CI variable your shell overrode all survive it.

Each of those is held by a test that fails when it stops being true, and the
ones that matter most are checked against the whole corpus of real workflows
rather than a fixture:

- **No runnable check carries an unexpanded placeholder** from any provider, in
  its command, directory, inputs or environment.
- **No runnable check does anything outward-facing** by default: pushing,
  publishing, deploying, installing into the machine, or needing root.
- **Importing the same tree twice gives the same answer**, so an explanation
  does not change between two runs of an unchanged repository.
- **Text that reaches a config or a report stays valid UTF-8**, whatever it was
  cut from. Fuzzed in CI rather than left to a remembered example.
- **The helper the tests import through does what the real entry point does**,
  so a change cannot pass its tests on a path nobody runs.

```console
$ ghostci --explain
change detection: compared against merge-base with origin/main
1 changed files

  run   go-test        src/a.go changed
  skip  docs-lint      no changed file matches docs/**
  run   secrets-scan   no input globs declared, so it cannot be safely skipped
```

## Configuration

`ghostci init` generates this, but it is plain YAML you can write by hand:

```yaml
checks:
  - name: test
    command: go test ./...
    inputs: ["**/*.go", "go.mod"]   # omit to always run
    dir: ./backend                   # optional
    timeout: 5m                      # optional
    env:                             # optional
      CI: "true"
```

Flags: `--all` run everything, `--explain` show reasoning, `--no-cache`, `--fail-fast`, `-j N` parallelism, `--since <ref>` compare against a specific ref, `-f` config path.

Escape hatches: `git push --no-verify`, or `GHOSTCI_SKIP=1 git push`.

### Other providers

Azure Pipelines, CircleCI and Bitbucket Pipelines have initial support: `init`
detects their config and imports plain script steps under the right shell.
Anything else is listed for review rather than guessed, including marketplace
tasks, orbs, pipes, containers, dynamic configs and any step carrying a
condition. Depth lives in GitHub Actions and GitLab CI.

### GitLab CI

`init` finds `.gitlab-ci.yml` on its own when there are no GitHub workflows, or
point at it with `--from .gitlab-ci.yml`. Jobs become checks with
`before_script` prepended and the same `bash -eo pipefail` GitLab runs them
under, `rules:changes` becomes the check's input globs, `extends`, YAML
anchors, `!reference` and local `include:` all resolve, and `parallel:matrix`
expands to one check per leg.

What it refuses rather than guesses: jobs needing `services:`, the deprecated
`only`/`except` keywords, rules resting on variables only GitLab sets, and
scripts reading a `CI_` variable that has no local value. Remote, project and
component includes are named, since their content is never visible here.

## What it does not do

Stated plainly so the scope reads as deliberate:

- **No emulation.** Commands run on your machine, in your shell. It does not reproduce the GitHub runner environment and never will. If you need that, use [act](https://github.com/nektos/act).
- **No containers.** No Docker, no service containers.
- **No remote actions.** `uses: owner/repo@ref` is reported, not fetched. Local composite actions are expanded.
- **No output plumbing.** Steps reading `steps.*` outputs are refused rather than approximated.
- **Linux and macOS.** Both run the same process-group cancellation path, and CI runs the full suite on both. Windows compiles and is checked in CI, but refuses to run and is not shipped in releases: without process-group cancellation a killed check can orphan children that hang the tool, and a hang is worse than a clear refusal. Under the hook it says nothing was verified rather than blocking your push.

## How it works

```
.github/workflows/*.yml
        │  ghostci init, once
        ▼
    ghostci.yaml  (reviewable, editable, committed)
        │
        ▼
git diff ─▶ selector ─▶ cache ─▶ runner ─▶ report ─▶ exit code
               │          │                    ▲
        (no match:     (hash hit:              │
         skipped)       cached) ───────────────┘
```

Exit codes: `0` all clear, `1` a check failed or the tree changed under the run, `2` usage or config error, `130` interrupted before every check finished (never an all clear).


## Contributing

`go test -race ./...` must pass, and the importer's extraction rate against the real-workflow corpus must not regress below 70%.
