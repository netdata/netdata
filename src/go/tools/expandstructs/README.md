# expandstructs

`expandstructs` forces keyed struct composite literals to one field per line —
the second stage of this repository's recommended Go formatting pipeline
(see `src/go/AGENTS.md` → "Go Formatting").

## Pipeline

```sh
golines -m 120 -t 4 -w <paths>      # split >120 cols; SKIP if golines not installed
go run ./tools/expandstructs <paths>
goimports -w <paths>
```

`expandstructs` formats source in-process with `go/format` after each pass, so
it leaves files gofmt-clean without invoking an external formatter.

## Usage

```sh
go run ./tools/expandstructs <path> [path ...]
```

A path may be a Go file or a directory; directories are walked for `*.go`
(skipping `vendor`, `testdata`, and dot directories). Example:

```sh
go run ./tools/expandstructs ./plugin/agent/jobmgr
```

## What it does / skips

- Expands only keyed struct literals of a named type (`T{...}`, `pkg.T{...}`,
  `T[X]{...}`, optionally behind `&`) — one field per line with a trailing comma.
- Skips map/slice/array literals, positional (unkeyed) literals, empty `T{}`,
  elided-type nested literals, and any literal with a comment inside its braces
  (so element comments are never dropped).
- AST-based: it never changes tokens, only line layout. Runs to a fixpoint so
  nested struct literals expand too.

This style is recommended, not CI-enforced. `gofmt`/`goimports` cannot express
it, so re-run the pipeline if code drifts.
