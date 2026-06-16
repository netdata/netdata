# Offline WAL/SFST OTel-log query contract (`sfsq-cli`)

Scope: how `sfsq-cli` reads and queries OpenTelemetry logs directly from on-disk
WAL and SFST files, with no running agent. It complements the live `otel-logs`
Function for terminal and forensic inspection.

## Crate

`src/crates/sfsq-cli` — a thin offline front-end over the wire-neutral `sfsq`
query engine. It does not open sockets, contact an agent, or read the remote
object store; it reads local files only.

## Directory resolution

- Two directories are needed: the WAL dir (`logs.wal.dir`) and the SFST/index dir
  (`logs.index.dir`).
- Per-dir precedence, resolved independently for each: an explicit
  `--wal-dir`/`--sfst-dir` flag wins, else `--config` (user `otel.yaml`), else
  `--stock-config` (stock `otel.yaml`) — mirroring the agent's stock→user order.
- `otel.yaml` is parsed with a minimal struct that extracts only those two dirs
  and ignores all other fields (the agent's config is typically partial).
- Files are read from `{dir}/{tenant}`; tenant defaults to `default`. The tenant
  must be a single benign path segment (not empty, `.`, `..`, or containing
  `/`, `\`, NUL) so a typo cannot point at a sibling directory.
- An unresolved dir is a hard error naming the ways to set it.

## Source discovery

- **SFST (sealed):** enumerated from the SFST dir; time-pruned via each file's
  cheap summary `[min,max]` against the query window.
- **WAL (unindexed tail):** WAL files carry no on-disk timestamp index, so each is
  enumerated and its whole intact frame prefix is handed to the engine as a
  row-scanned tail; the engine applies the window and filters during the scan.
- **Dedup — SFST wins by sequence:** a WAL whose sequence is already sealed into
  an SFST is skipped (the SFST is authoritative), matching the live query planner
  and avoiding double-counting.
- **Stream filter** mirrors the live planner and the
  [stream-identity contract](otel-stream-identity.md):
  - SFST: exact `ServiceStream` equality against the summary stream.
  - WAL: `FileId.ns_hash` compared against the query stream's
    `ServiceStream::ns_hash()` (empty field == absent). The raw
    `compute_ns_hash(Some(&ns), …)` MUST NOT be used here — it would hash an
    absent namespace as `Some("")` and miss every absent-namespace WAL file.
  - A stream needs a `--name`; `--namespace` defaults to empty and requires
    `--name` (a namespace alone cannot identify a stream).

## Query and output

- Built on the `sfsq` engine via a single-bucket grid spanning the window.
- Window is `[--since, --until)` — inclusive lower, exclusive upper, epoch
  seconds. `--since` defaults to `0` (from the beginning); `--until` defaults to
  `now + 1s` so events in the current second are included. Time specs accept
  `now`, `-1h`/`+30m` relative (lowercase units), epoch seconds, or a UTC datetime
  `YYYY-MM-DD HH:MM:SS` (a trailing `Z`/`+00:00` is accepted as UTC; a non-UTC
  offset like `+03:00` is rejected — use epoch seconds for a non-UTC instant;
  sub-second precision is truncated to whole seconds). A datetime or relative
  offset past the u32 epoch range (post-2106) is an error, not a silent
  truncation.
- Results are the newest `--limit` rows, presented newest-first; `--reverse` flips
  the returned page to oldest-first (a presentation flip, not a pagination
  change).
- `--filter` is comma-separated `field=value` (exact) or `field~regex`
  (full-value-anchored); repeating a field ORs, different fields AND. `--query` is
  a free-text unanchored regex over whole `key=value` pairs. Both are validated up
  front so a bad pattern is a clean global error, not a silent empty result.
- Output is NDJSON (v1), with `fields` always an array of `[key, value]` pairs for
  a stable shape; `--fields` projects a subset. A one-line summary and warnings go
  to stderr (an empty result must be distinguishable from "data was unreadable").

## Relationship to the live path

`sfsq-cli` and the live `otel-logs` Function share the `sfsq` engine and the same
source-selection rules (SFST-wins-by-seq dedup, stream-identity collapse). The CLI
adds offline file discovery and a terminal/NDJSON surface; it does not change any
on-disk format or ingestion behavior.
