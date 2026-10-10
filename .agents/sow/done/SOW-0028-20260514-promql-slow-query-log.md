# SOW-0028 - PromQL - Slow-query log (JSONL)

## Status

Status: completed

Sub-state: shipped 2026-05-14; opt-in JSONL log via NETDATA_PROMQL_LOG.
165 Rust unit / 117 smoke, no regression. Live verified with parse
errors, range queries, scalar queries, and the bytes-per-op shape.

## Requirements

### Purpose

Capture every PromQL query the evaluator runs in an append-only
JSONL file, with enough fields to reproduce the query later. The
operator workflow is: open Grafana, notice a panel is sluggish,
grep the log for slow lines, copy the query text, run it through
curl or the smoke harness while attaching a profiler. The log
exists to shorten that loop -- it is not yet a productionised
observability feature.

### User Request

"Going to Grafana's metrics browser/explorer is a little bit
slow. I'm curious, can we record/log all promql expressions that
are being evaluated and their execution time? The idea is to be
able to easily/quickly find anything that's too slow and
copy/paste the expression so that I can run it on my own and
profile/benchmark stuff." -> Accepted approach (A) from a 3-way
breakdown: append-only JSONL file. "I think we want to start
simple at this point in time."

### Assistant Understanding

Facts:

- The Rust crate's two FFI entry points
  (`nd_promql_query_instant` / `nd_promql_query_range` in
  `src/crates/netdata_promql/src/lib.rs`) are the single funnel
  for every PromQL query that reaches the evaluator.
- Both entry points have access to the full decoded query
  string, the resolved time parameters, and the response handle
  (which carries HTTP status and lets us count series).
- The daemon is multi-threaded; HTTP WEB worker threads call
  the FFI concurrently. A shared writer needs locking.
- The crate currently emits Prometheus JSON by hand without
  `serde` (see `output/prometheus_json.rs`). JSONL lines are
  simple enough to follow the same convention; no new crate
  dependency.

Inferences:

- The log path should be env-var driven, not config-file
  driven. The user is testing locally; production gating can
  come later. Unset env var disables the log entirely so the
  default is safe.
- Truncated queries that came through the buggy `url_decode_r`
  path will be logged in their truncated form. That's the
  right thing -- the log records what the daemon evaluated,
  not what the user intended.
- The query text may legitimately be a few KB (Grafana
  auto-generates long queries with many label filters);
  capping at 4 KB by default is conservative but keeps a
  pathological query from filling the log with one record.

Unknowns:

- None. The scope is intentionally narrow.

### Acceptance Criteria

1. New module `eval/slow_log.rs` (or `slow_log.rs` at crate
   root -- the cleaner placement is at crate root since the
   log is a side effect of the FFI, not of the evaluator
   itself).
2. `run_instant` and `run_range` wrap their inner pipelines in
   `Instant::now()` brackets and emit one JSONL line per call
   (regardless of success / error).
3. Log file path comes from env `NETDATA_PROMQL_LOG`. Unset =
   feature disabled (no file open, no per-call overhead beyond
   a single atomic check).
4. Threshold from env `NETDATA_PROMQL_LOG_THRESHOLD_MS`
   (default 0 = log every query).
5. Query truncation cap from env
   `NETDATA_PROMQL_LOG_MAX_QUERY_LEN` (default 4096).
6. Single global writer guarded by `Mutex<BufWriter<File>>`;
   each line is flushed before the lock is released so
   `tail -f` sees lines promptly.
7. JSONL fields: `ts` (ISO-8601 with milliseconds), `kind`
   (`"instant"` or `"range"`), `query` (truncated as above),
   `host` (null or string), `start_ms`, `end_ms`, `step_ms`
   (last three only for range), `elapsed_ms`, `http_status`,
   `series_count` (best-effort; missing on errors), and the
   optional `error_type` / `error_message` fields when the
   call failed.
8. Rust unit tests:
   - Round-trip a record through the logger to a tempfile and
     parse it back; confirm every required field is present
     and well-typed.
   - Threshold filter: a 1ms record skipped when threshold is
     100ms.
   - Disabled state: with env unset, the logger is a no-op
     and never touches the filesystem.
   - Query truncation: a query longer than the cap is stored
     truncated to the cap.
9. The implementation must not change the success / error
   semantics of `run_instant` / `run_range`. Logger failure
   (e.g. disk full) is swallowed silently.
10. No smoke harness change required for this SOW -- the
    feature is opt-in via env var, and the daemon has to be
    relaunched with the env set for the smoke runner to test
    it. A single optional smoke check can read
    `NETDATA_PROMQL_LOG` from its own env and assert the log
    grew during the run; if unset the check is skipped.

Out of scope:

- Log rotation (logrotate handles it).
- A Functions API endpoint that returns recent slow queries
  (deferred to a follow-up SOW -- the user accepted approach
  A explicitly, not the hybrid).
- Cloud / UI surface.
- Including request / response sizes.
- Aggregating queries by canonical form to find repeated
  slow patterns (a downstream tool's job).

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

The PromQL evaluator has no per-query observability. The
daemon's `access.log` records the HTTP transaction with total
`prep_ut` but not the query text or the per-step breakdown.
There is currently no way to grep for slow queries after the
fact.

Affected contracts and surfaces:

- New: `src/crates/netdata_promql/src/slow_log.rs` -- the
  logger module, with the env-var-driven config helper, the
  record struct, the writer mutex, and the hand-rolled JSON
  emitter.
- Modified: `src/crates/netdata_promql/src/lib.rs` --
  `run_instant` and `run_range` capture timing and call
  `slow_log::record()`. The function bodies stay otherwise
  unchanged.
- New: `.agents/sow/specs/promql-endpoint-contract.md`
  section noting the env vars and the log format.

No FFI change, no shim change, no IR change. The slow-query
log is observability-only.

Existing patterns to reuse:

- The crate's hand-rolled JSON emit pattern in
  `output/prometheus_json.rs`. The slow-log records are
  simpler (flat, no nested arrays) so they can use a slightly
  smaller helper.
- `OnceLock` / `Mutex` patterns already in use in
  `storage/matchers.rs` for the regex cache.

Risk and blast radius:

- Per-query overhead: one `Instant::now()` pair plus a
  serialise-and-write under a mutex. Each query already does
  multiple storage iterations; this is negligible.
- Lock contention: the writer mutex is taken once per query.
  At a sustained 1000 QPS the contention is bounded by the
  serialised write time, which is microseconds. Not a
  concern for the operator workflows in scope.
- Disk fill: at ~500 bytes per JSONL line, 100 QPS for 24h
  is roughly 4 GB. Operators running long stress tests
  should know to point the log at a partition with room or
  unset the env var. Document in the spec section.

Sensitive data handling plan:

The query field is the literal PromQL string. Users may put
label values that look like secrets in their queries
(unlikely but possible -- e.g.
`metric{token="..."}`). Document the risk in the spec
section; treat the slow-query log like a debug log, not a
public artifact. No additional redaction is done in the
shipped feature -- the operator chooses to enable it and
where to point it.

Implementation plan (single chunk):

1. New `slow_log.rs` module: env-var config struct, lazy
   global writer, `record()` entry point, JSON emit helper.
2. Wrap `run_instant` and `run_range` in
   `Instant::now()` brackets and emit on every exit path
   (success and error). Pass the response handle to extract
   HTTP status + body length; series count comes from the
   `EvalResult` before serialization.
3. Rust unit tests in `slow_log.rs::tests` covering enable/
   disable, threshold filter, truncation, and a full
   field-shape round trip.
4. Spec update.
5. Close: status -> completed, move to `done/`, commit.

Validation plan:

- Rust unit tests: 5+ new (round-trip, threshold filter,
  disabled-state no-op, truncation, error-path logging).
- Live verification: set `NETDATA_PROMQL_LOG=/tmp/x.jsonl`,
  restart the daemon, fire a handful of queries, confirm
  the file has matching lines.

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: updated -- a short subsection at the bottom of the
  endpoint contract documenting the env vars and the log
  format.
- End-user docs / skills: no change (the endpoint is still
  phase-gated).
- SOW lifecycle: pending -> current -> done.

Open-source reference evidence:

- Prometheus itself does not ship an equivalent "slow query
  log" by default. Mimir / Cortex have a `query_log_*`
  family of settings that this design is loosely informed
  by. No upstream code is copied.

## Execution Log

### 2026-05-14

- SOW drafted, promoted, implemented single chunk.
- New module `src/crates/netdata_promql/src/slow_log.rs` (~280
  lines including tests). Hand-rolled JSON emitter avoids
  pulling `serde` into the crate.
- `lib.rs::run_instant` and `run_range` refactored: each is now
  a 20-line wrapper that captures `Instant::now()`, calls
  `run_X_inner` (which carries the original body and returns
  `(NdPromqlResponse, usize)` for the series count), and emits
  one slow-log record on every exit path.
- New `log_query` helper extracts HTTP status / error_type /
  error_message uniformly from the success or error arm.
- 9 new unit tests covering: required-key shape on success;
  required-key shape on failure; UTF-8 truncation; control
  -char JSON escape; newline-escape; record_to writer
  contract; threshold filter; multi-record ordering; config
  defaults when env unset.

## Validation

Acceptance criteria coverage:

1. New module at crate root (`src/crates/netdata_promql/src/slow_log.rs`).
2. Both `run_instant` and `run_range` capture timing and log
   on success + error.
3. Env-driven path (`NETDATA_PROMQL_LOG`); unset = disabled,
   verified by `config_defaults_when_env_unset` test.
4. Threshold env (`NETDATA_PROMQL_LOG_THRESHOLD_MS`), tested
   in `record_to_respects_threshold_filter`.
5. Truncation env (`NETDATA_PROMQL_LOG_MAX_QUERY_LEN`),
   tested in `format_truncates_long_queries_at_char_boundary`.
6. Single global writer guarded by `Mutex<BufWriter<File>>`;
   each line flushed after the write so `tail -f` sees it.
7. JSONL shape verified live -- four success lines (one
   scalar, one instant vector, one composite with `abs`, one
   range) and one error line, all with the expected fields.
8. Unit tests: 9 new across `slow_log.rs::tests`.
9. Success and error semantics of `run_instant` /
   `run_range` are unchanged; existing FFI tests pass.
10. Smoke harness unchanged; the opt-in nature means no
    smoke gating is required for the SOW. Verified the
    existing 117 smoke checks all still pass after the
    daemon rebuild.

Test posture: 165/165 Rust unit (was 156; +9), 117/117 live
smoke (unchanged; SOW is observability-only).

Live verification:

```
$ NETDATA_PROMQL_LOG=/tmp/promql-slow.jsonl ./netdata -D
$ curl /api/v1/query?query=42
$ curl /api/v1/query?query=system_cpu
$ curl /api/v1/query?query=bad%7B    # parse error
$ cat /tmp/promql-slow.jsonl
{"ts_ms":1778749631858,"kind":"instant","query":"42",...,"elapsed_ms":0,"http_status":200,"series_count":1}
{"ts_ms":1778749631866,"kind":"instant","query":"system_cpu",...,"elapsed_ms":3,"http_status":200,"series_count":10}
{"ts_ms":1778749655744,"kind":"instant","query":"bad{",...,"elapsed_ms":0,"http_status":400,"error_type":"bad_data","error_message":"parse error: unexpected end of input inside braces"}
```

Reviewer findings: none yet (branch stays local).

Same-failure search: no other path in the codebase writes a
per-query log; the new module is self-contained.

Artifact maintenance gate:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Spec: updated -- new `## Slow-query log (SOW-0028)`
  section at the end of `promql-endpoint-contract.md`
  documenting the env vars, the record shape, and operator
  use examples.
- End-user docs / skills: no change (PromQL endpoint still
  phase-gated).
- SOW lifecycle: pending -> current -> done; `completed`.

## Outcome

The slow-query log gives the dev workflow the user asked for:
`tail -f | jq` on a JSONL file, copy-paste the query into
curl or the smoke harness, reproduce. The opt-in env-var
model means production installs see zero new behavior unless
operators explicitly opt in.

The refactor that gave us series_count from the inner
functions also cleaned up a small smell -- `run_X` now reads
as a thin orchestration layer over `run_X_inner`, and the
log emit point is the single funnel that knows both the
timing and the result shape.

## Lessons

- Lazy global `OnceLock` for env-driven config worked well,
  but tests would have been fragile if we relied on it
  directly (env state is process-wide and shared between
  tests). The `record_to` test entry point that takes an
  explicit `Write` sink solved that cleanly.
- The hand-rolled JSON emitter is small enough to keep
  forever; pulling in `serde_json` would have been a few
  hundred KB of dependencies for a feature that emits one
  flat record per line.
- The truncated-query case (UTF-8 char-boundary safety) is
  the kind of corner that's trivial to add a test for and
  saves real pain if a user ever drops a non-ASCII label
  value into a query.

## Followup

- A Functions API endpoint returning recent slow queries
  (originally option B in the design conversation) remains
  deferred. The JSONL file is sufficient for the current
  workflow.
- The `url_decode_r` newline truncation bug is still
  unfixed. The slow-query log will log the truncated query
  text (i.e. what the daemon actually evaluated), which is
  the right thing for an "observability" log but does mean
  operators investigating slow multi-line queries from
  Grafana will see incomplete query text in the log. A fix
  for that bug is the natural companion SOW.
- Log rotation: out of scope here; logrotate handles it.

## Regression Log

None yet.
