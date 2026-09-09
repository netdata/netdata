# sfsq-cli

Inspect OpenTelemetry logs stored in Netdata's on-disk **WAL/SFST** files from the
terminal, without a running agent. Useful for forensic and offline inspection: it
reads the same files the live `otel-logs` Function serves, through the same query
engine, and prints NDJSON.

> The same capability ships inside the Netdata Agent as `otel-plugin logs` (the
> shipped binary; this standalone `sfsq-cli` is a dev/forensic tool, not
> installed). Both front doors flatten the same `Args` and call the same
> `sfsq_cli::run`, so the flags and output below are identical — `otel-plugin
> logs --since -1h …` behaves exactly like `sfsq-cli --since -1h …`.

## How it finds the files

It needs the WAL directory and the SFST (index) directory. Each is resolved
independently, first match wins:

1. an explicit `--wal-dir` / `--sfst-dir`,
2. `--config <user otel.yaml>` — derived from `base_dir` as
   `{base_dir}/logs/wal` and `{base_dir}/logs/index`,
3. `--stock-config <stock otel.yaml>` — same `base_dir` derivation.

`otel.yaml` no longer configures per-signal dirs: it sets one `base_dir` and the
plugin derives `{base_dir}/{signal}/{wal,index,catalog}`. This is a logs tool, so
it reads the `logs/` subtree. Logs are read from `{dir}/{tenant}` (`--tenant`,
default `default`). A relative `base_dir` in a `--config` file resolves against
the current working directory (the stock config uses an absolute path).

## Usage

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
- a relative offset: `-1h`, `+30m` (humantime durations; units are **lowercase** —
  `m` is minutes, `M` would be months)
- epoch seconds: `1718539200`
- a **UTC** datetime: `2026-06-16 10:00:00` (or `…T10:00:00`, optionally with a
  trailing `Z` or `+00:00` — all UTC)

A non-UTC timezone offset (e.g. `+03:00`) is **not** accepted — use epoch seconds
for a non-UTC instant. Sub-second precision is truncated to whole seconds.
`--since` defaults to the beginning; `--until` defaults to `now + 1s` (so the
current second is included).

### Stream filter

`--name <service.name>` restricts to one service stream; `--namespace`
(default empty) narrows it further. An empty/absent namespace and an
empty-string namespace are the same stream (per the OTel convention), so
`--name api` with no `--namespace` matches logs that carry no `service.namespace`.

### Filtering rows

- `--filter` — comma-separated terms: `field=value` (exact) or `field~regex`
  (anchored to the whole value). Repeating a field ORs its terms; different
  fields AND. Example: `--filter 'level=error,host~web.*'`. Values cannot contain
  a literal comma (the term separator); match such values with `--query` instead.
- `--query` — free-text unanchored regex over whole `key=value` pairs.

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
```

## Output

NDJSON on stdout — one JSON object per row: a top-level `timestamp_ns` (always
emitted) and `fields`, always an array of `[key, value]` pairs (stable shape).
`--fields` projects only the `fields` array, never `timestamp_ns`. A one-line
`matched=…/returned=…` summary and any warnings (e.g. a skipped corrupt file) go
to stderr, so an empty result is distinguishable from "data was there but
unreadable". v1 emits NDJSON only.

`--reverse` reorders the returned page (the newest `--limit` rows) to
oldest-first; it does not page backwards, so it cannot surface the *oldest* N
rows of a wide window — narrow `--since`/`--until` to inspect early events.

A zero exit means the query *ran*, not that every file was readable: an
unreadable WAL/SFST dir or a corrupt file is reported as a `WARN` on stderr and
skipped, and the run still exits `0`. Check stderr (or run with `RUST_LOG=warn`,
the default) when completeness matters.
