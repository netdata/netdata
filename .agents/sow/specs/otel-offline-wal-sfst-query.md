# Offline WAL/SFST OTel-log query contract (`sfsq-cli` / `otel-plugin logs`)

Scope: how the offline OTel-log query reads and queries OpenTelemetry logs
directly from on-disk WAL and SFST files, with no running agent. It complements
the live `otel-logs` Function for terminal and forensic inspection. The capability
has two front doors over one shared code path (see below).

## Code location and front doors

- `src/crates/sfsq-cli` — a thin offline front-end over the wire-neutral `sfsq`
  query engine. It does not open sockets, contact an agent, or read the remote
  object store; it reads local files only.
- The query surface lives in the **`sfsq_cli` library**: the `clap::Args` struct,
  `run(&Args, &mut impl Write)`, `init_tracing()`, and `is_broken_pipe()`. Two
  front doors flatten the same `Args` and call the same `run`, so flags, behavior,
  and output are identical:
  - **`sfsq-cli` binary** — a dev/forensic tool. It is a workspace member but is
    NOT shipped: it is absent from corrosion's `CRATES` list in `CMakeLists.txt`,
    so CMake never builds or installs it.
  - **`otel-plugin logs` subcommand** — the shipped path. `otel-plugin` depends on
    the `sfsq_cli` lib (the lib is compiled as a dependency; the `sfsq-cli` binary
    is still not installed). `CliCommand::Logs(Box<sfsq_cli::Args>)` flattens `Args`
    (boxed to keep the enum's largest variant small; `Box<T: Args>: Args` in clap).
- clap dispatch: the `logs` token coexists with the agent's hidden numeric
  `_update_every` positional exactly as the internal `worker` subcommand does — a
  non-numeric token routes to the subcommand, a number to the positional; no extra
  clap attribute is required.
- Tracing/output discipline: the `logs` arm installs the stderr-`warn` subscriber
  instead of the daemon subscriber, runs the synchronous query, and exits before
  any supervisor setup. The global tracing subscriber is set at most once per
  process — the `logs` arm and the daemon arms are mutually exclusive. stdout NDJSON
  is safe regardless of subscriber (the daemon subscriber targets journald or
  stderr, never stdout); the offline arm picks the `warn`/stderr subscriber because
  the daemon subscriber defaults to `info`, emits a "tracing initialized" preamble,
  formats for journald, and would try to connect to journald — all wrong for an
  operator's terminal.

## Directory resolution

- Two directories are needed: the WAL dir and the SFST/index dir. `otel.yaml` no
  longer configures per-signal dirs — it sets one `base_dir`, and this logs query
  tool derives `{base_dir}/logs/wal` and `{base_dir}/logs/index` (matching
  `PluginConfig::lifecycle_for(Signal::Logs)`).
- Per-dir precedence, resolved independently for each: an explicit
  `--wal-dir`/`--sfst-dir` flag wins, else the dir derived from `--config` (user
  `otel.yaml`) `base_dir`, else from `--stock-config` (stock `otel.yaml`)
  `base_dir` — mirroring the agent's stock→user order.
- `otel.yaml` is parsed with a minimal struct that extracts only `base_dir` and
  ignores all other fields (the agent's config is typically partial).
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
  - Both tiers filter identically by **`FileId.part_key` membership**: each
    candidate's `id.part_key` (parsed from the filename) is tested against the
    query's `partition_keys` set via `Query::matches_partition`. There is no
    content-derived `ServiceStream` equality — the substrate is content-agnostic
    and routes purely by the opaque partition key in the filename.
  - The CLI derives the one filter key from `--namespace`/`--name` via
    `otel_logs_identity::part_key` (i.e. `ServiceStream::ns_hash`, empty field ==
    absent). The raw `compute_ns_hash(Some(&ns), …)` MUST NOT be used here — it
    would hash an absent namespace as `Some("")` and miss every absent-namespace
    file.
  - A stream needs a `--name`; `--namespace` defaults to empty and requires
    `--name` (a namespace alone cannot identify a stream).

## Query and output

- Built on the `sfsq` engine via a single-bucket grid spanning the window.
- Window is `[--since, --until)` — inclusive lower, exclusive upper, epoch
  seconds. `--since` defaults to `0` (from the beginning); `--until` defaults to
  `now + 1s` so events in the current second are included. Time specs accept
  `now`, `-1h`/`+30m` relative (lowercase units; `--since`/`--until` set
  `allow_hyphen_values` so the leading-dash form parses without `=`), epoch
  seconds, or a UTC datetime
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

The offline query (`sfsq-cli` and `otel-plugin logs`) and the live `otel-logs`
Function share the `sfsq` engine and the same source-selection rules
(SFST-wins-by-seq dedup, stream-identity collapse). The offline path adds local
file discovery and a terminal/NDJSON surface; it does not change any on-disk
format or ingestion behavior.
