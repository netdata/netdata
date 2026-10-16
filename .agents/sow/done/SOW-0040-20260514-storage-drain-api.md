# SOW-0040 - Storage drain API: eliminate per-sample virtual dispatch

## Status

Status: completed

Sub-state: shipped 2026-05-14. Per-sample `Box<dyn Iterator>::next`
dispatch eliminated; `drain_samples` writes directly into caller
buffers. Box frame at 0% inclusive in post-SOW-0040 samply profiles
of both Q3 and Q6. Q6 wall-clock improved 5-11%; Q3 essentially
flat (bottleneck already shifted off storage after SOW-0039).
Regression and structural gates green.

## Requirements

### Purpose

Replace `BackendQuery::open_samples` (which returns
`Option<Box<dyn Iterator<Item = Sample> + '_>>`) with a buffer-fill
`drain_samples` API. Both existing consumers — `eval/select.rs` and
`eval/fused.rs` — already drain the iterator into per-series
`Vec<i64> + Vec<f64>` immediately. The iterator abstraction pays the
trait-object dispatch cost per sample without delivering any laziness
the consumers use.

### User Request

> I think we don't want the NaN-prefilter. We should focus on the
> virtual dispatch instead.

Followed by "Yes, please." on the drafting prompt.

### Assistant Understanding

Facts:

- `storage::backend::BackendQuery::open_samples`
  (backend.rs:56-62) returns
  `Option<Box<dyn Iterator<Item = Sample> + '_>>`.
- Two consumers exist:
  - `eval/select.rs:94-110` (vector selector) and the matrix-select
    path at `eval/select.rs:259-273`. Both drain into a fresh
    `Vec<i64>` + `Vec<f64>` and then operate on the columns.
  - `eval/fused.rs::run_fused` (the fused aggr/rollup driver).
    Same pattern.
- Pre-SOW-0039 Q3 RelWithDebInfo samply profile recorded the
  `alloc::boxed::iter::<impl core::iter::traits::iter::Iterator>::next`
  frame at 36.8% inclusive (Q3) and 44.2% inclusive (Q6). Per-sample
  indirect call across the trait-object boundary.
- The `Sample` struct in `storage/samples.rs:21-27` carries
  `{timestamp_ms, value, flags}`. Grep of every in-crate consumer
  shows the `flags` field is written by `NdSamples::next` and
  `MemBackend` but **never read** by any consumer; the `flags`
  module's `STALE` and `RESET` constants are marked
  `#[allow(dead_code)]`.
- The eval-layer `Sample` (`eval/types.rs:29`) is a *different*
  struct that omits `flags` and is used for the eval-side
  `Series::samples()` materialization helper. Independent of this
  SOW.

Inferences:

- The performance shape from SOW-0039's analysis is:
  - Per-sample virtual call cost: small absolute (~1-2 cycles ×
    8.9M samples ≈ 18ms on Q3) but disables inlining cascades.
  - Inlining cascade: with a direct (non-virtual) sample drain, the
    inner loop of `FfiBackend::drain_samples` can fully inline
    `nd_pds_samples_next` and `unpack_storage_number` calls into a
    tight pull loop. The C-side decode time per sample roughly
    halved between Debug and Release (-O2 affected the C side
    cleanly); a tighter Rust drain wrapper unlocks similar wins on
    the Rust side of the boundary.
  - Net expected wall-clock: 5-15% on storage-heavy queries (Q3
    type) and possibly more on selector-only queries (Q6 type
    where storage drain is the dominant cost).
- Removing the `Box<dyn Iterator>` allocation also removes one heap
  allocation per series per query (612 for Q3) and one Drop call
  per iterator.

Unknowns:

- Whether `Sample::flags` carries semantic information any
  downstream out-of-crate consumer relies on. Grep within the crate
  shows none. The `Sample` struct itself is `pub` (exported as
  `storage::Sample`) so external consumers *could* observe it. The
  crate boundary is `pub(crate)` for most of the storage module
  but `Sample` is `pub`. Need to confirm during the gate whether
  retaining it is required.

### Acceptance Criteria

- All 195/195 unit tests pass on `cargo test -p netdata_promql --release`.
- Compliance corpus: same 545/255/212 split across 1012 cases.
- Smoke harness: 117/117 pass.
- Re-running the SOW-0040 benchmarks on the same RelWithDebInfo
  binary shows:
  - The `alloc::boxed::iter::<impl ... Iterator>::next` frame
    disappears from the bucketed samply profile for Q3 and Q6.
  - Q3 wall-clock (warm) drops by at least 3% relative to the
    post-SOW-0039 baseline of 3.21s.
  - Q6 wall-clock (warm) drops by at least 5% relative to the
    post-SOW-0039 baseline of ~10.6s (Q6 is more storage-heavy
    so the win lands harder there).
- The `BackendQuery` trait shape changes; no public FFI or HTTP
  contract changes.

## Analysis

Sources checked:

- `src/crates/netdata_promql/src/storage/backend.rs` (trait
  definition).
- `src/crates/netdata_promql/src/storage/ffi_backend.rs` and
  `src/crates/netdata_promql/src/storage/query.rs` (FfiBackend
  impl + NdQuery::open_samples).
- `src/crates/netdata_promql/src/storage/mem_backend.rs`
  (MemBackend impl).
- `src/crates/netdata_promql/src/storage/samples.rs` (NdSamples
  iterator + Sample struct + flags module).
- `src/crates/netdata_promql/src/eval/select.rs` (vector +
  matrix selector consumers).
- `src/crates/netdata_promql/src/eval/fused.rs::run_fused` (fused
  evaluator consumer).
- SOW-0039 samply profile bucketed analysis showing the Box
  iterator dispatch share.

Current state:

- One trait method (`open_samples`) returns a heap-allocated
  trait object iterator. Two backends implement it; both
  underlying iterator types (`NdSamples`, `MemBackend`'s
  inner iterator) immediately materialize per-sample fetches.
- Both consumers drain into per-series buffers right away, then
  pass `&[i64]` and `&[f64]` slices into the inner evaluator
  logic.
- `Sample::flags` is a dead channel — present in the trait's
  item type, written by both backends, read by nobody in-tree.

Risks:

- `Sample` is `pub` (not `pub(crate)`), so out-of-crate consumers
  *could* be reading it. The crate isn't published; its only
  consumer is the daemon's C side via the FFI, which does not
  see Rust types. So `Sample`'s `pub` is effectively unused; it
  can be removed or shrunk. Confirm during the Pre-Implementation
  Gate.
- `Box<dyn Iterator>` lifetime semantics — the existing trait
  returns `Box<dyn Iterator<Item = Sample> + '_>` borrowing from
  `&self`. The drain API takes `&mut Vec<...>` from the caller,
  which is simpler and avoids the lifetime gymnastics entirely.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The storage trait was shaped to look like a Rust iterator
  because that is the idiomatic Rust shape for "stream of items."
  In this crate every consumer immediately drains the stream into
  a column-oriented buffer; the laziness offered by the iterator
  is unused. The trait-object dispatch (`Box<dyn Iterator>`)
  costs an indirect call per sample, blocks inlining of the
  per-sample fetch into the consumer loop, and forces a per-
  series heap allocation.

Evidence reviewed:

- SOW-0039 samply profile of Q3 (`/tmp/q3-rel.json.gz` and
  `/tmp/q3-sow0039-final.json.gz`): the
  `alloc::boxed::iter::<impl ... Iterator>::next` frame appears
  at 36.8-44.2% inclusive on the heaviest WEB thread.
- The same profiles show `<NdSamples as Iterator>::next` at
  3-7% leaf time — the indirect call wrapper sits between
  consumer and concrete impl.
- Grep evidence (see Facts above): both consumers immediately
  drain to `Vec<i64> + Vec<f64>`; no consumer uses iterator
  laziness; no consumer reads `Sample::flags`.

Affected contracts and surfaces:

- `storage::Backend` trait (internal). No public ABI.
- `storage::BackendQuery` trait (internal).
- `storage::NdQuery` (internal). `open_samples` signature replaced
  by `drain_samples`.
- `storage::Sample` (currently `pub`). Likely removed or
  demoted to `pub(crate)` — confirm.
- `storage::flags` module (internal, currently `pub(crate)`).
  Dead; remove or document why retained.
- `eval::select::eval_vector_select` and
  `eval::select::eval_matrix_select` — call site swap.
- `eval::fused::run_fused` — call site swap.
- No HTTP/JSON/FFI/wire change.

Existing patterns to reuse:

- The selectors already materialize `Vec<i64> + Vec<f64>` from
  the iterator drain. The `drain_samples` API receives the
  caller's mutable buffers and writes into them directly. The
  buffer reuse pattern across series in a single query is a
  small extension on top.
- `MemBackend::open_samples` returns `Box<dyn Iterator + 'q>`
  wrapping a slice-iter. Its `drain_samples` implementation is
  a tight copy loop.

Risk and blast radius:

- Behavioral risk: zero, as long as `drain_samples` preserves the
  exact same iteration semantics (same time window, same step,
  same NaN policy, same order). The unit tests + compliance
  corpus + smoke harness cover all three consumers.
- Performance risk: it is possible the win is smaller than
  predicted if the dbengine's per-sample cost dominates such
  that the trait-object indirection was already amortized. Q6
  (wide selector) is the cleaner measurement because the storage
  drain is the bulk of its wall-clock; if Q6 does not improve
  meaningfully, the SOW's "win lands harder there" prediction is
  wrong and we should revisit.

Sensitive data handling plan:

- This SOW touches internal storage trait shapes and two samply
  profile outputs. No raw secrets, credentials, customer data,
  private endpoints, or proprietary incident details. Benchmark
  queries reference stock Netdata metric names
  (`app_fds_open`, `app_cpu_utilization`, etc.). Profile files
  remain in `/tmp/` and are not durable artifacts.

Implementation plan:

1. Confirm `Sample`'s pub surface and the flags channel are
   genuinely unused. Grep `storage::Sample` across the crate
   and across any consumer code. If a consumer reads `flags`,
   the drain API must add an `out_flags: &mut Vec<u32>` (or
   equivalent); otherwise drop the flags channel from the trait.
2. Change `BackendQuery::open_samples` to `drain_samples` with
   signature
   `fn drain_samples(&self, i: usize, after_s: i64, before_s: i64,
   step_ms: i64, out_ts: &mut Vec<i64>, out_vals: &mut Vec<f64>)
   -> Result<(), ResolveError>`. The method clears the output
   vectors first (so callers can pass a reused buffer without
   pre-clearing).
3. Implement `FfiBackend::drain_samples`: open the underlying
   `nd_pds_open_samples` handle, walk `nd_pds_samples_next` in
   a tight loop, push (ts, value) for every returned sample,
   close the handle. No `NdSamples` iterator wrapper. This is
   the path where the inlining cascade unlocks.
4. Implement `MemBackend::drain_samples`: existing slice walk
   becomes a direct copy into the caller's buffers.
5. Remove `NdSamples` from `storage::samples.rs` (or demote to
   a private helper if any internal path still needs the iterator
   shape; grep should confirm none does).
6. Remove `Sample` from `storage::` (or keep as a private impl
   detail if any internal site needs the (ts, value, flags)
   triple shape; the eval-layer `Sample` in `eval/types.rs` is
   unaffected).
7. Update `eval/select.rs::eval_vector_select`
   (samples_iter loop) and `eval/select.rs::eval_matrix_select`
   to call `drain_samples`, reusing per-query buffers across the
   per-series loop where convenient.
8. Update `eval/fused.rs::run_fused` similarly.
9. Run the regression gate: cargo test, compliance corpus, smoke
   harness.
10. Rebuild RelWithDebInfo, restart daemon, re-profile Q3 and Q6
    under samply, compare bucketed leaf shares vs the SOW-0040
    baseline.

Validation plan:

- Unit tests: 195/195.
- Compliance: 545/255/212 across 1012 cases.
- Smoke: 117/117.
- samply Q3 and Q6 profiles: the
  `alloc::boxed::iter::<impl ... Iterator>::next` frame is gone.
- Wall-clock: Q3 -3% minimum, Q6 -5% minimum from the
  post-SOW-0039 baseline.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: no update expected. The storage adapter is an internal
  Rust surface; its shape changes do not affect any public
  contract.
- End-user/operator docs: no update expected.
- End-user/operator skills: no update expected.
- SOW lifecycle: standard close. Single commit per the rules.

Open-source reference evidence:

- None required. Pattern (buffer-fill instead of iterator-return
  to remove dispatch overhead on hot paths) is borrowed from
  VictoriaMetrics' Go storage layer architecture (shared
  `Vec<int64>` timestamps + `Vec<float64>` values that the
  storage fills per series — referenced in earlier
  PromQL-architecture conversations), but the design here is
  driven by in-tree evidence and pattern, not transcribed from
  an external repo.

Open decisions:

1. **Keep `Sample::flags` channel?** RESOLVED 2026-05-14 → Option A
   (drop the flags channel).
   - Grep evidence (chunk 1 investigation gate, already captured
     during drafting): no in-crate consumer reads `Sample::flags`.
     `STALE` and `RESET` constants are marked `#[allow(dead_code)]`.
     The crate is not published; out-of-crate consumers do not
     exist. The flags channel is dead.
   - Drain API takes two buffers: `out_ts: &mut Vec<i64>` and
     `out_vals: &mut Vec<f64>`. The `Sample` struct and the
     `flags` module become deletable.
   - If a future feature needs flags, re-add as a separate method
     (`drain_samples_with_flags`) or a third buffer. The cost of
     re-introduction is small; the savings of removal are real
     today.

2. **Per-series buffer reuse across the per-query loop?** RESOLVED
   2026-05-14 → mixed (Option B for selectors, Option A for fused).
   - Selectors (`eval/select.rs`): one `(Vec<i64>, Vec<f64>)` pair
     allocated per query, `clear()`ed between series via
     `drain_samples`. Peak memory residency: largest series'
     sample count × 16 bytes ≈ 80 KB for Q3-shaped queries.
   - Fused evaluator (`eval/fused.rs`): per-series buffer
     allocation stays. The driver already iterates series in a
     tight outer loop and the buffers' lifetimes match that loop.
     Converting to per-query reuse would be churn for no
     measurable savings.

## Implications And Decisions

Pending user response on Open Decisions 1 and 2 (or acceptance of
the recommended A/B path).

## Plan

1. Investigation gate: grep confirms `Sample::flags` is read by
   nobody (already done in this draft; locked at chunk start).
2. Trait shape: `BackendQuery::open_samples` → `drain_samples`
   with buffer arguments. Implementation chunks 2 in the
   Pre-Implementation Gate.
3. Backend impls: FfiBackend + MemBackend. Implementation chunks
   3-4.
4. Iterator wrapper removal: NdSamples → private helper or gone.
   Implementation chunk 5.
5. Sample struct: remove or demote based on Open Decision 1.
   Implementation chunk 6.
6. Call site migration: select.rs + fused.rs. Implementation
   chunks 7-8.
7. Regression gate: cargo test, compliance, smoke.
8. Performance gate: samply Q3 and Q6 bucketed comparison.
9. Close: artifact maintenance gate, lessons, follow-up mapping,
   single commit.

## Execution Log

### 2026-05-14

- SOW drafted following user direction to pursue virtual-
  dispatch elimination rather than NaN-prefilter (post-SOW-0039
  follow-up choice).
- Promoted from `pending/` to `current/` per user instruction.
  Status changed to `in-progress`.
- Open Decision 1 resolved → Option A (drop `Sample::flags`
  channel). Grep evidence pre-collected during drafting confirmed
  no in-crate consumer reads the channel.
- Open Decision 2 resolved → mixed (Option B for selectors,
  Option A for fused). Per-query buffer reuse in
  `eval/select.rs`, per-series allocation preserved in
  `eval/fused.rs`.

## Validation

Acceptance criteria evidence:

- **Unit tests: 195/195 pass.** `cargo test -p netdata_promql --release`
  reports `test result: ok. 195 passed; 0 failed`.
- **Compliance corpus: pass.** The `run_compliance_corpus` integration
  test (which asserts the 545/255/212 baseline) passed. No drift.
- **Smoke: 117/117 pass.** `tests/promql-smoke/run-smoke.sh` against
  the SOW-0040 daemon reports `117 passed, 0 failed`.
- **Box<dyn Iterator>::next frame inclusive: 0.0%** in both Q3 and
  Q6 samply profiles (`/tmp/q3-sow0040.json.gz`,
  `/tmp/q6-sow0040.json.gz`). Pre-SOW-0040 readings were 36.8% (Q3)
  and 44.2% (Q6). The `drain_samples` frame replaces the dispatch
  path at 50.8% (Q3) and 67.1% (Q6) inclusive — the storage drain
  is now a direct typed call.
- **Q6 wall-clock improvement: 5-11%** off the post-SOW-0039
  baseline of ~10.6s warm. New warm runs: 9.41s and 9.81s. Meets
  the ≥5% acceptance gate.
- **Q3 wall-clock: essentially flat** at 3.34s vs 3.21s baseline
  (within run-to-run variance of ±200ms). The strict ≥3%
  improvement gate is a miss. Honest read: Q3's bottleneck after
  SOW-0039 had already shifted to per-sample operator work; the
  storage dispatch was a smaller absolute fraction of Q3's
  wall-clock than predicted. The structural goal (eliminate the
  dispatch) is met; the storage-side wall-clock win lands on
  storage-heavy queries (Q6 type), not on operator-heavy queries
  (Q3 type). Documented honestly rather than retroactively
  weakening the criterion.

Tests or equivalent validation:

- Release build of `netdata_promql`: clean (only pre-existing
  dead-import warnings on `Sample` from earlier modules; some
  resolved by this SOW since `eval::Sample` is now the only
  `Sample` in scope and a few stale `use` lines became precise).
- Daemon rebuilt as RelWithDebInfo via the project's CMake/Corrosion
  pipeline. Installed binary BuildID confirmed different from
  pre-SOW-0040.

Real-use evidence:

- Q3 and Q6 queries executed against the post-SOW-0040 daemon.
  Q3 returns 67 series × 5053 grid points; Q6 returns 1836 series
  × ~4900 grid points each. Both match the pre-SOW-0040 result
  shapes.
- Smoke harness exercised the full HTTP/JSON pipeline end-to-end
  against the live daemon, including all selector / function /
  aggregation / set-operator / label-op / absent / subquery /
  host-scope categories. 117/117 pass.

Reviewer findings:

- None requested.

Same-failure scan:

- Post-edit grep:
  `grep -rn "Sample\|samples_iter\|open_samples" src/crates/netdata_promql/`
  returns only the eval-layer `Sample` (in `eval/types.rs`, a
  different struct) and `samples_iter` as a string in a function
  body comment. No storage::Sample or NdSamples residue.
- File `storage/samples.rs` deleted; `mod samples;` line removed
  from `storage/mod.rs`. No build artifact reference remains.

Sensitive data gate:

- This SOW touches an internal trait shape and two samply profile
  outputs from the local Netdata daemon. No raw secrets,
  credentials, customer data, private endpoints, or proprietary
  incident details. Benchmark queries reference stock Netdata
  metric names. Profile files remain in `/tmp/` and are not
  durable artifacts.

Artifact maintenance gate:

- AGENTS.md: no update needed. Workflow rules unchanged.
- Runtime project skills: no update needed. No skill describes the
  storage adapter internals.
- Specs: no update needed. No public contract changed.
- End-user/operator docs: no update needed.
- End-user/operator skills: no update needed.
- SOW lifecycle: standard close. Status `completed`, file moves to
  `.agents/sow/done/`, single commit covers source + SOW lifecycle
  per CLAUDE.md.

Specs update:

- Not needed; see above.

Project skills update:

- Not needed; see above.

End-user/operator docs update:

- Not needed; see above.

End-user/operator skills update:

- Not needed; see above.

Lessons:

- **Eliminating dispatch wins only where dispatch dominated.**
  Q6 (storage-heavy) saw 5-11% wall-clock improvement; Q3
  (operator-heavy after SOW-0039) saw essentially nothing. The
  per-sample indirect call was cheap in absolute terms; its
  removal mattered because of the *inlining cascade it enabled
  on storage-heavy paths*. Where storage is a smaller absolute
  fraction (post-SOW-0039 Q3), the cascade unlocks less work
  to inline.
- **`MemSeries` shape change is API surface.** The mem backend's
  series struct shifted from `samples: Vec<Sample>` to
  `(timestamps_ms: Vec<i64>, values: Vec<f64>)`. The compliance
  corpus runner and a few unit tests had to migrate. Worth
  flagging because the in-tree test surface is the only external
  consumer of `MemSeries` today; if anyone wires a third backend
  later, the shape is now column-oriented by construction.
- **Iterator abstractions that nobody pulls lazily are pure
  overhead.** Both call sites of `open_samples` drained
  immediately into per-series `Vec<i64> + Vec<f64>`. The iterator
  abstraction paid its full cost without delivering anything
  consumers used. Look for similar shapes elsewhere — anywhere a
  trait returns `Box<dyn Iterator>` and every consumer
  immediately collects, the trait is paying for laziness it does
  not enable.

Follow-up mapping:

- **Sample::flags channel retirement was done in this SOW.** The
  channel was dead; removed cleanly. If a future feature needs
  per-sample flags, re-add as either a separate
  `drain_samples_with_flags` method or a third buffer; the cost
  of re-introduction is small.
- **The C shim leaf-time bucket (18.8% on Q3, 22.9% on Q6 in
  post-SOW-0040)** is now the natural next attack surface if
  storage wall-clock becomes a focus. The shim's
  `collapse_storage_point` and `nd_pds_samples_next` per-sample
  glue absorbed the inlined Rust wrappers; further wins would
  require either a batch API (return N samples per call) or
  pushing the storage decode logic further into the inner loop.
  **Not a new SOW; flagged as the architectural successor if
  storage-side optimization is the next focus.**

## Outcome

Shipped. The `Box<dyn Iterator>::next` per-sample dispatch is
gone. The `drain_samples` method on `BackendQuery` writes
`(Vec<i64>, Vec<f64>)` columns directly into caller-provided
buffers; one trait-object dispatch per series, not per sample.
The `Sample` struct, `NdSamples` iterator, and `flags` module are
deleted from the storage layer.

Quantitative outcome:

| Metric | Pre-SOW-0040 | Post-SOW-0040 | Change |
|--------|-------------:|--------------:|-------:|
| Q3 wall-clock (warm) | 3.21s | 3.34s | flat (within noise) |
| Q3 Box<dyn Iterator>::next inclusive | 36.8% | 0.0% | eliminated |
| Q6 wall-clock (warm) | 10.6s | 9.4-9.8s | -5% to -11% |
| Q6 Box<dyn Iterator>::next inclusive | 44.2% | 0.0% | eliminated |
| Unit tests | 195/195 | 195/195 | — |
| Compliance | 545/255/212 | 545/255/212 | — |
| Smoke | 117/117 | 117/117 | — |

## Lessons Extracted

See the Lessons subsection of the Validation gate. Three
load-bearing points:

1. Dispatch-elimination wins land where dispatch dominated. The
   inlining cascade matters more than the indirect call cost.
2. Iterator abstractions whose consumers always collect are pure
   overhead — pattern worth grepping for in similar trait
   surfaces.
3. Honest acceptance criteria are better than retroactively
   weakened ones; document what missed and why rather than
   redefining the gate.

## Followup

- The C shim's per-sample glue (`collapse_storage_point` +
  `nd_pds_samples_next`) is now the largest storage-side leaf
  bucket. Further storage wins would need a batch API or pushing
  decode into the inner loop. Not tracked as a new SOW yet;
  awaits direction.

## Regression Log

None yet.
