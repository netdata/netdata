# sfsq-cli

Inspect OpenTelemetry logs and traces stored in Netdata's on-disk **WAL/SFST**
files from the terminal, without a running agent. Useful for forensic and
offline inspection. Run with no subcommand, it queries **logs** and prints
NDJSON. The trace subcommands (`trace`, `suggest`, `overview`, `search` and
`explore`) query **traces** and print plain text.

> The logs mode also ships inside the Netdata Agent as `otel-plugin logs`
> (the shipped binary; this standalone `sfsq-cli` is a dev/forensic tool, not
> installed). Both front doors flatten the same `Args` and call the same
> `sfsq_cli::run`, so the logs flags and output below are identical —
> `otel-plugin logs --since -1h …` behaves exactly like `sfsq-cli
> --since -1h …`. The trace subcommands exist only in `sfsq-cli`.

## How it finds the files (logs mode)

It needs the WAL directory and the SFST (index) directory. Each is resolved
independently, first match wins:

1. an explicit `--wal-dir` / `--sfst-dir`,
2. `--config <user otel.yaml>` — derived from `base_dir` as
   `{base_dir}/logs/wal` and `{base_dir}/logs/index`,
3. `--stock-config <stock otel.yaml>` — same `base_dir` derivation.

`otel.yaml` no longer configures per-signal dirs: it sets one `base_dir` and
the plugin derives `{base_dir}/{signal}/{wal,index,catalog}`. This is the logs
mode, so it reads the `logs/` subtree (the trace subcommands instead take
explicit traces files — see below). Logs are read from `{dir}/{tenant}`
(`--tenant`, default `default`). A relative `base_dir` in a `--config` file
resolves against the current working directory (the stock config uses an
absolute path).

## Usage (logs mode)

```
sfsq-cli [--wal-dir DIR | --config FILE | --stock-config FILE] [--sfst-dir DIR]
         [--tenant NAME]
         [--since TIME] [--until TIME]
         [--name NAME [--namespace NS]]
         [--filter 'f=v,g~re'] [--query REGEX]
         [--fields A,B,C] [--limit N] [--reverse]
         [--show-files] [--output ndjson]
```

### Time (`--since` / `--until`)

A window `[since, until)` in epoch seconds. Each accepts:

- `now`
- a relative offset: `-1h`, `+30m` (humantime durations; units are
  **lowercase** — `m` is minutes, `M` would be months)
- epoch seconds: `1718539200`
- a **UTC** datetime: `2026-06-16 10:00:00` (or `…T10:00:00`, optionally with
  a trailing `Z` or `+00:00` — all UTC)

A non-UTC timezone offset (e.g. `+03:00`) is **not** accepted — use epoch
seconds for a non-UTC instant. Sub-second precision is truncated to whole
seconds. `--since` defaults to the beginning; `--until` defaults to
`now + 1s` (so the current second is included). An empty window
(`--since` ≥ `--until`) is an error.

### Stream filter

`--name <service.name>` restricts to one service stream; `--namespace`
(default empty) narrows it further and requires `--name`. An empty/absent
namespace and an empty-string namespace are the same stream (per the OTel
convention), so `--name api` with no `--namespace` matches logs that carry no
`service.namespace`.

### Filtering rows

- `--filter` — comma-separated terms: `field=value` (exact) or `field~regex`
  (anchored to the whole value). Repeating a field ORs its terms; different
  fields AND. Example: `--filter 'level=error,host~web.*'`. Values cannot
  contain a literal comma (the term separator); match such values with
  `--query` instead.
- `--query` — free-text unanchored regex over whole `key=value` pairs.

## Trace subcommands

The trace subcommands drive the traces query engine over the **traces**
signal's files — on a stock install
the sealed SFSTs under `{base_dir}/traces/index/` and the WALs under
`{base_dir}/traces/wal/`. Unlike the logs mode there is no directory or
config resolution: you name the files. Output is line-based text, not NDJSON.

Common to all of them:

- `--sfst FILE` — a sealed traces SFST file, repeatable; `--wal FILE` — a
  traces WAL file, repeatable. At least one of the two is required.
- `--start-ns N` / `--end-ns N` — a half-open window in epoch nanoseconds.
  Give the two flags together, with start before end. `trace` has no
  window, `overview` and `explore` need one (for their grid), and the
  others make it optional. `search` admits a span when its *start* is in the window.
  `suggest` skips whole files outside the window.
- WAL files are scanned whole as tails — right for shut-down or recovered
  WALs; query an actively written WAL through a live agent instead. A torn
  or truncated tail is clamped to the last complete frame (with a warning);
  a corrupt WAL is skipped with a warning so one bad file cannot abort a
  multi-file query, while a nonexistent or header-less path is a hard error.

### Search tokens (`search`, `overview`, `explore`, `trace`)

These flags give the search bar's tokens. A trace matches when it matches
all tokens. Different tokens can match different spans of the trace. All
four flags are repeatable. `explore` applies the tokens to each span
instead: a span matches when it matches all tokens, and a `--not` token
keeps the spans that do not match it. In `explore`, a token on the whole
trace (`root_name`, `root_service_name`, `trace_duration`) is an error.

- `--where COND` — one span must match all `--where` conditions together.
  A condition on the whole trace (`root_name`, `root_service_name`,
  `trace_duration`) or on `trace_id` is a token of its own.
- `--and COND` — a token of its own: a span of the trace matches the
  condition.
- `--not COND` — a token of its own: no span of the trace matches the
  condition (NOT EXISTS). For a condition on the whole trace, the trace
  does not match it.
- `--text TERM` — free text, one token per term. A span matches when the
  term is a part of one of its values, case-insensitive: the span name, the
  status message, the event names, or an attribute value. Keys do not
  match. A term has 3 or more characters.

`COND` is `KEY<op>VALUE`. `KEY` is the OpenTelemetry (OTLP) field path of the
data. These are the same keys that the search of the Netdata Agent uses:

- An attribute: `resource.attributes.NAME`, `scope.attributes.NAME`,
  `attributes.NAME` (a span attribute), `events.attributes.NAME` or
  `links.attributes.NAME`. `NAME` is the attribute name, with its dots:
  `resource.attributes.service.name`.
- A field: `name`, `kind`, `role`, `status.code`, `status.message`, `scope.name`,
  `scope.version`, `events.name`, `duration`, `span_id`, `parent_span_id`,
  `trace_id`, `links.span_id`, `links.trace_id`, `events.time_since_start`,
  `root_name`, `root_service_name`, `trace_duration`.

The ops:

| Op | Meaning |
|---|---|
| `=` / `!=` | Is equal / is not equal to the text |
| `=~` / `!~` | Matches / does not match the regex. The regex must match the whole value. |
| `^=` / `!^=` | Starts / does not start with the text |
| `>` `<` `>=` `<=` | Compares numbers (integer or decimal values) |

An empty value is an error. `!=`, `!~` and `!^=` apply to one span: the
span has the field, and its value does not match. To find the traces in
which no span has a value, use `--not` with `=`, `=~` or `^=`.

### `trace` — reconstruct one trace

```
sfsq-cli trace --trace-id <32 hex chars> (--sfst FILE | --wal FILE)…
               [--span-cap N] [token flags]
```

Prints a summary line — `trace <id>: N span(s), M root(s), status …, K typed
field(s)` — followed by the span tree, one line per span, indented two
spaces per depth level (a span reached again through a cycle prints once).
`--trace-id` is the 16-byte trace id as hex, case-insensitive. `--span-cap`
overrides the assembly cap (default 65,536; 0 rejected).

The token flags mark spans. The summary line then ends with `, N matched`,
and the line of each span that matches a token ends with `[match]`. Only
positive tokens on spans mark spans: `--not` tokens, and tokens on the
whole trace or on `trace_id`, mark no span.

### `suggest` — suggest keys or values

```
sfsq-cli suggest (--sfst FILE | --wal FILE)… [--key KEY | --tier TIER]
                 [--term TEXT] [--max-items N] [--max-files N]
                 [--stale-files N] [--deadline-ms N]
                 [--start-ns N --end-ns N]
```

Without `--key`, `suggest` gives keys. With it, it gives the values of that
key. `KEY` is a key as in `COND` above. Only these fields have stored values:
`name`, `kind`, `role`, `status.code`, `status.message`, `scope.name`,
`scope.version` and `events.name`. If you give another field as `--key`, the
command fails. `suggest` reads the newest
files first. A key or a value matches `--term` when it is equal to the
term, starts with it, contains it, or starts within one edit (terms of 4 or
more characters) or two edits (8 or more) of it. An empty term matches
all.

- `--tier` — give only the keys of this cardinality tier: `low` (fewer
  than 100 different values in a file), `mid` (fewer than 1,000) or
  `high`. A key gets the highest tier that a file gives it. With `--tier`,
  `suggest` gives all of these keys: there is no default maximum, and the
  walk does not stop at files that add no key. `--max-items` and
  `--stale-files` set these limits. You cannot use `--tier` with `--key`.
- `--max-items` — the maximum number of suggestions (default 50; 0 is an
  error).
- `--max-files` — stop after this number of files that have the key
  (default 256; 0 is an error).
- `--stale-files` — stop after this number of files in sequence that add
  no suggestion (default 8; 0 is an error). With `--tier`, a file adds a
  suggestion when it adds a key of any tier.
- `--deadline-ms` — stop after this number of milliseconds.

Each suggestion is one line: `<text> match=<class>`, then `tier=…` for a
key, `truncated` when a value is cut for display (keys are not cut), and
`kinds=…` for an attribute key: the kinds of values that it holds, joined with
`+` (for example `kinds=Int+Str`). The last line gives
the count, the `truncated` flag, the status, `complete after <unix second>`
when a limit stopped the walk (the files that it read hold all spans from
that second), and the time.

### `overview` — count the traces of the heatmap grid

```
sfsq-cli overview (--sfst FILE | --wal FILE)… --start-ns N --end-ns N
                  [--buckets N] [token flags]
```

`overview` builds the grid of the list's heatmap: time buckets by trace
duration bins. `--buckets` sets the number of time buckets (default 60).
The time from `--start-ns` to `--end-ns` must divide equally into the
buckets. If it does not, the command stops with an error.
The token flags filter the traces that it counts. A token on
`trace-duration` is an error, because the duration is the grid's own axis.
The command is for measurements and does not print the grid. It prints one
line: `N trace(s), N span(s), N error(s), status …, <time> ms`.

### `explore` — the span view

```
sfsq-cli explore (--sfst FILE | --wal FILE)… --start-ns N --end-ns N
                 [--buckets N] [--stack KEY] [--limit N] [--column KEY]…
                 [token flags]
```

`explore` counts the spans of the window, one row per stored span, as the
Traces tab's span view does. The token flags select the spans (see
*Search tokens*). `--buckets` sets the number of time buckets (default 60).
The time from `--start-ns` to `--end-ns` must divide equally into the
buckets. If it does not, the command stops with an error.
`--stack` sets the key whose values divide the count (default
`status.code`). `--limit` sets how many of the newest spans it lists
(default 100, at most 200; 0 lists none). Each `--column` adds a key to
each listed span. The keys of `--stack` and `--column` must have stored
values, so `duration` and the ids are errors. A WAL file is built into an
index whole before the query.

It prints one line per listed span, newest first:
`<start-ns> <trace-id> <span-id> dur=<ns>ns KEY=VALUE…` (several values of
one key are joined with commas). Then it prints
`N span(s), N error(s); by KEY: VALUE=N… other=N unset=N`: the values with
the most spans, the spans with other values (`other`), and the spans
without the key (`unset`). A final line gives the status and the time.

### `search` — filter whole traces

```
sfsq-cli search (--sfst FILE | --wal FILE)…
                [token flags]
                [--min-duration-ns N] [--max-duration-ns N]
                [--limit N]
                [--start-ns N --end-ns N]
```

- Token flags — see *Search tokens* above.
- `--min-duration-ns` / `--max-duration-ns` — shorthand for `duration`
  `>=`/`<=` conditions (inclusive), in the `--where` group.
- `--limit` — keep the top-K most recent matching traces (default 20;
  0 rejected).

Per matching trace, one line
`<trace-id> <root-service> / <root-name> start=<ns> dur=<ns>ns spans=N
errors=N rank=<ns>`. `rank` sets the order of the list. For each positive
token on spans, take the start of the newest span that matches it: `rank`
is the oldest of these starts. With one such token, it is the start of the
newest matching span; with none, the start of the newest span in the
window. When a value is not set, the line shows
`<no service>` or `<unnamed>`. The counts include each stored copy of a
resent span. The suffix `[inexact]` shows that a source of that trace
failed, so its numbers can be too low. A final `N trace(s), status …`
line gives the count and `complete`, or
`PARTIAL` with the reasons.

## Examples

```sh
# Last hour of errors for service "checkout", newest first
sfsq-cli --config /etc/netdata/otel.yaml \
  --name checkout --since -1h --filter 'level=error'

# A fixed UTC window, oldest-first, only two fields, as NDJSON.
# --fields selects from the `fields` array; timestamp_ns is always emitted.
sfsq-cli --wal-dir /var/lib/netdata/otel/logs/wal --sfst-dir /var/lib/netdata/otel/logs/index \
  --since '2026-06-16 09:00:00' --until '2026-06-16 10:00:00' \
  --reverse --fields body,host

# Show which files were consulted (to stderr)
sfsq-cli --config /etc/netdata/otel.yaml --name api --show-files

# Reconstruct one trace from a sealed SFST plus a recovered WAL,
# and mark the spans that contain the text "checkout"
sfsq-cli trace --trace-id 4bf92f3577b34da6a3ce929d0e0e4736 \
  --sfst /var/lib/netdata/otel/traces/index/<SEQ>.sfst \
  --wal  /var/lib/netdata/otel/traces/wal/<SEQ>.wal \
  --text checkout

# Checkout traces with a span of ≥ 1ms in the last 10 minutes, newest first
# (root_service_name matches the trace's true root; window in epoch ns)
sfsq-cli search --sfst /var/lib/netdata/otel/traces/index/<SEQ>.sfst \
  --where 'root_service_name=checkout' --min-duration-ns 1000000 \
  --start-ns 1769000000000000000 --end-ns 1769000600000000000

# Traces with a frontend span and a PostgreSQL span, and no error span
sfsq-cli search --sfst /var/lib/netdata/otel/traces/index/<SEQ>.sfst \
  --and 'resource.attributes.service.name=frontend' \
  --and 'attributes.db.system=postgresql' --not 'status.code=ERROR'

# Which values does the span attribute http.method take?
sfsq-cli suggest --key attributes.http.method \
  --sfst /var/lib/netdata/otel/traces/index/<SEQ>.sfst
```

## Output

The logs mode writes NDJSON to stdout — one JSON object per row: a top-level
`timestamp_ns` (always emitted) and `fields`, always an array of
`[key, value]` pairs (stable shape). `--fields` projects only the `fields`
array, never `timestamp_ns`. The engine returns the newest `--limit` rows
(default 50; 0 rejected); `--reverse` flips that page to oldest-first — it
does not page backwards, so it cannot surface the *oldest* N rows of a wide
window; narrow `--since`/`--until` to inspect early events. The trace
subcommands print the line-based text described above, not NDJSON.

Diagnostics go to stderr: a one-line `matched=N returned=N window=a..b`
summary (epoch seconds), `consulted:` file lines under `--show-files`, a
`no WAL/SFST files matched (…)` note instead of an empty run, and any
warnings (e.g. a skipped corrupt file) — so an empty result is
distinguishable from "data was there but unreadable". Warnings honor
`RUST_LOG` (default `warn`).

A zero exit means the query *ran*, not that every file was readable: an
unreadable WAL/SFST dir or a corrupt file is warned about on stderr and
skipped, and the run still exits 0 (a closed downstream pipe, e.g. `| head`,
also exits 0 quietly). Any other failure prints `error: …` and exits 1;
bad arguments exit 2. This mapping is identical for the logs mode and the
trace subcommands.
