# sfsq-cli

Inspect OpenTelemetry logs and traces stored in Netdata's on-disk **WAL/SFST**
files from the terminal, without a running agent. Useful for forensic and
offline inspection. Run with no subcommand, it queries **logs** and prints
NDJSON; the `trace`, `attributes`, `attribute-values`, and `search`
subcommands query **traces** and print plain text.

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

`sfsq-cli trace`, `attributes`, `attribute-values`, and `search` drive the
traces query engine over the **traces** signal's files — on a stock install
the sealed SFSTs under `{base_dir}/traces/index/` and the WALs under
`{base_dir}/traces/wal/`. Unlike the logs mode there is no directory or
config resolution: you name the files. Output is line-based text, not NDJSON.

Common to all four:

- `--sfst FILE` — a sealed traces SFST file, repeatable; `--wal FILE` — a
  traces WAL file, repeatable. At least one of the two is required.
- `--start-ns N` / `--end-ns N` — an optional half-open window in epoch
  nanoseconds; the two flags must be given together (and start before end).
  `search` admits a span when its *start* falls in the window; key
  enumeration prunes at file granularity.
- WAL files are scanned whole as tails — right for shut-down or recovered
  WALs; query an actively written WAL through a live agent instead. A torn
  or truncated tail is clamped to the last complete frame (with a warning);
  a corrupt WAL is skipped with a warning so one bad file cannot abort a
  multi-file query, while a nonexistent or header-less path is a hard error.

### `trace` — reconstruct one trace

```
sfsq-cli trace --trace-id <32 hex chars> (--sfst FILE | --wal FILE)…
               [--span-cap N]
```

Prints a summary line — `trace <id>: N span(s), M root(s), status …, K typed
field(s)` — followed by the span tree, one line per span, indented two
spaces per depth level (a span reached again through a cycle prints once).
`--trace-id` is the 16-byte trace id as hex, case-insensitive. `--span-cap`
overrides the assembly cap (default 65,536; 0 rejected).

### `attributes` / `attribute-values` — enumerate keys and values

```
sfsq-cli attributes       (--sfst FILE | --wal FILE)… [--owner OWNER]
                          [--max-keys N] [--start-ns N --end-ns N]
sfsq-cli attribute-values (--sfst FILE | --wal FILE)… --owner OWNER
                          --key KEY [--max-values N] [--start-ns N --end-ns N]
```

`OWNER` is one of `resource`, `span`, `instrumentation`, `event`, `link`,
`builtin`. `KEY` is the bare attribute name, or — under `--owner builtin` —
one of the builtin-field words: `name`, `kind`, `status`, `status-message`,
`instrumentation-name`, `instrumentation-version`, `event-name`, `duration`,
`span-id`, `parent-span-id`, `trace-id`, `link-span-id`, `link-trace-id`,
`event-time-since-start`, `root-name`, `root-service-name`,
`trace-duration`. `attributes` prints one `<owner> <key>` line per key;
`attribute-values` prints one `<value> kind=<type>` line per value (`none`
when the value carries no type). Both end with a
`N key(s)/value(s), truncated true|false, status …` summary line;
`--max-keys` / `--max-values` cap the list (0 rejected) and set the
`truncated` flag when they cut it.

### `search` — filter whole traces

```
sfsq-cli search (--sfst FILE | --wal FILE)…
                [--where 'TARGET<op>VALUE']…
                [--min-duration-ns N] [--max-duration-ns N]
                [--limit N] [--spans-per-trace N]
                [--start-ns N --end-ns N]
```

- `--where` — repeatable; conditions AND and are evaluated per span: a trace
  matches when one of its spans satisfies all of them. `TARGET` is
  `OWNER.KEY` (the attribute owners above), `.KEY` (any owner: resource ∪
  span), or a builtin-field word. Ops: `=` / `!=` (text), `=~` / `!~`
  (regex anchored to the whole value), `>` `<` `>=` `<=` (integer or decimal
  values). An empty value is rejected.
- `--min-duration-ns` / `--max-duration-ns` — shorthand for `duration`
  `>=`/`<=` conditions (inclusive).
- `--limit` — keep the top-K most recent matching traces (default 20;
  0 rejected). `--spans-per-trace` — matched spans attached per trace
  (default 3, max 128, 0 = none).

Per matching trace, one line
`<trace-id> <root-service> / <root-name> start=<ns> dur=<ns>ns spans=N
errors=N matched=N` — `<no service>` / `<unnamed>` when unset, and an
`[inexact]` suffix when that trace's assembly was capped or degraded (its
summary numbers may undercount) — followed by the attached spans, two-space
indented (`  <name> start=<ns> dur=<ns>ns`). A final
`N trace(s), status …` line reports the count and `complete` or `PARTIAL`
with reasons.

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

# Reconstruct one trace from a sealed SFST plus a recovered WAL
sfsq-cli trace --trace-id 4bf92f3577b34da6a3ce929d0e0e4736 \
  --sfst /var/lib/netdata/otel/traces/index/<SEQ>.sfst \
  --wal  /var/lib/netdata/otel/traces/wal/<SEQ>.wal

# Checkout traces with a span of ≥ 1ms in the last 10 minutes, newest first
# (root-service-name matches the trace's true root; window in epoch ns)
sfsq-cli search --sfst /var/lib/netdata/otel/traces/index/<SEQ>.sfst \
  --where 'root-service-name=checkout' --min-duration-ns 1000000 \
  --start-ns 1769000000000000000 --end-ns 1769000600000000000

# Which values does the span attribute http.method take?
sfsq-cli attribute-values --owner span --key http.method \
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
