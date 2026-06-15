# SOW-20260614-netflow-benchmark-overhead-budget - NetFlow Production-Shaped Overhead Budget

## Status

Status: completed

Sub-state: Production-shaped benchmark harness completed; runtime optimizations
remain in autonomous follow-up SOWs.

## Requirements

### Purpose

Make `netflow-plugin` overhead measurements fit for production DevOps/SRE use by ensuring benchmarks and tests include the background work that exists in live operation, not only post-decode ingest throughput.

### User Request

The user selected autonomous SOWs per improvement bucket and asked to create them and start. This SOW covers the first bucket: production-shaped benchmark and overhead budget.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Production starts the NetFlow chart sampler.
- Existing benchmark helpers construct only `IngestService`.
- Existing benchmark helpers set `listener.sync_every_entries = usize::MAX`.
- Existing benchmark helpers set `listener.sync_interval = 1h`.
- The resource benchmark already has a partial worker-mode 1s tick for tier handoffs, telemetry mirror, prune, and open-tier refresh.
- Existing benchmark rates start at `5_000` flows/s, not the observed low-rate production range.

Inferences:

- Current benchmarks can miss fixed 1s background costs such as chart sampling, facet memory walks, expensive process memory reads, and production sync tick behavior.
- A benchmark-only change is the safest first step because it establishes evidence before runtime behavior changes.

Unknowns:

- Exact structure needed to start chart sampling in a benchmark without unstable timing or leaking async tasks.
- Whether the production-shaped benchmark should be a new benchmark mode or an extension of existing resource-envelope modes.

### Acceptance Criteria

- Add or update tests/benchmarks so at least one low-rate NetFlow benchmark exercises production-shaped background work.
- The benchmark must include a low-rate case representative of about `20-30` flows/s or a configurable equivalent.
- The benchmark must not hide the 1s sync tick behind `sync_interval = 1h` for the production-shaped mode.
- The benchmark must account for chart-sampler work or explicitly measure the sampler as its own bucket.
- Existing high-rate resource benchmarks must remain available for comparison.
- Validation output must separate at least ingest, sync tick, chart sampler/process memory, facet persistence, tier sync, and disk I/O where practical.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/main.rs`
- `src/crates/netflow-plugin/src/ingest_test_support.rs`
- `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs`
- `src/crates/netflow-plugin/src/charts/runtime.rs`
- `src/crates/netflow-plugin/src/facet_runtime.rs`
- Parent inventory SOW.

Current state:

- Production starts chart sampling at `src/crates/netflow-plugin/src/main.rs:190`.
- Benchmark helper `new_benchmark_ingest_service()` sets `sync_every_entries = usize::MAX` at `src/crates/netflow-plugin/src/ingest_test_support.rs:26`.
- Benchmark helper `new_benchmark_ingest_service()` sets `sync_interval = 1h` at `src/crates/netflow-plugin/src/ingest_test_support.rs:27`.
- Disk benchmark helper repeats those settings at `src/crates/netflow-plugin/src/ingest_test_support.rs:52` and `:53`.
- `BENCHMARK_RATES` starts at `5_000` flows/s in `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:37`.
- Worker-mode resource benchmark calls `handle_sync_tick_for_test()` once per second at `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:604`.
- During implementation, a short production-shaped 30 flows/s child benchmark
  initially reported `achieved_flows_per_sec: 0.0`.
  Root cause: the paced benchmark loop only consumed whole decoded batches, so
  a tick budget smaller than one batch did no work.
- The production-shaped report now exposes sync tick timing, raw sync rate, tier
  sync rate, tier flush rate, decoder-state persistence rate, and chart sampler
  timing.
- Dedicated facet-persistence timing is not added in this benchmark-only SOW
  because it requires runtime instrumentation inside `FacetRuntime` or
  `IngestService`; that work belongs to the facet persistence SOW.

Risks:

- Poorly designed benchmark changes can add noisy or flaky timing.
- Starting background tasks in benchmark children can leak tasks unless shutdown is explicit.
- A benchmark that is too slow may be skipped by developers and fail as a practical guardrail.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The current benchmark model is incomplete for low-rate production overhead because it suppresses or omits important background work.
- Fixing runtime overhead before the benchmark model is credible risks repeating the current false-confidence pattern.

Evidence reviewed:

- Parent inventory SOW reviewer findings.
- Production startup and benchmark helper code listed in Analysis.
- Existing resource-envelope benchmark loop and worker-mode sync-tick code.

Affected contracts and surfaces:

- Manual ignored benchmark commands documented in `src/crates/netflow-plugin/README.md`.
- Test-only benchmark helpers under `src/crates/netflow-plugin/src`.
- No shipped NetFlow runtime behavior should change in this SOW.

Clean-end-state target:

- A production-shaped, low-rate benchmark/test path exists and can observe fixed background costs.
- Existing high-rate benchmark modes remain available.
- Removed as redundant (i): none expected; existing benchmark modes remain useful for isolation.
- Excluded coupled items (ii): runtime optimizations for facet persistence, chart sampler, tier sync, decoder, query, enrichment, and raw journal sync are excluded because they have separate autonomous SOWs.
- Reference search: no public path or runtime contract replacement expected. If benchmark names or documented commands change, run a reference search for those names before completion.

Existing patterns to reuse:

- Existing ignored manual benchmark style in `ingest_resource_bench_tests.rs`.
- Existing child-process benchmark isolation.
- Existing `handle_sync_tick_for_test()` and tier worker test hooks.
- Existing resource report structures in `ingest_resource_bench_support.rs`.

Risk and blast radius:

- Low runtime risk because this SOW should touch test/benchmark code only.
- Medium developer-experience risk if the benchmark becomes too slow or noisy.
- Low security risk if benchmark reports contain only aggregate local resource metrics.

Sensitive data handling plan:

- Do not record raw flow values, IP addresses, customer data, private endpoints, credentials, or secrets in SOWs or docs.
- Use aggregate benchmark metrics and repo-relative code references only.

Implementation plan:

1. Add or adjust benchmark/test support so a production-shaped mode can run with a real 1s sync cadence and explicit background-task accounting.
2. Add a low-rate case or environment-configurable low-rate profile.
3. Ensure background tasks can be shut down deterministically.
4. Update benchmark reporting so fixed overhead buckets are visible.
5. Preserve existing high-rate benchmark modes and commands.

Validation plan:

- Run targeted Rust tests for any new helper behavior.
- Run the ignored benchmark child in a short low-rate configuration if feasible.
- Verify `cargo test` command compiles the touched benchmark/test modules.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: likely update or create a performance/benchmark spec only if durable benchmark invariants are established.
- End-user/operator docs: no update expected unless README benchmark commands change.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`; durable knowledge goes to tests, README/specs if needed.

Open-source reference evidence:

- Not checked for this first harness step because the implementation is constrained by existing local benchmark architecture, not by external protocol semantics. If a new benchmark framework is introduced, check its official docs first.

Open decisions:

- None blocking. The user approved autonomous SOWs and test-first work.

## Implications And Decisions

1. User decision: autonomous SOW split.
   - Selected: this SOW is autonomous and may be completed or abandoned independently.
   - Implication: runtime optimizations are not bundled here.
   - Recommendation classification: long-term-best.

2. User decision: test-first requirement.
   - Selected: benchmark/test coverage must be added or verified before behavior changes.
   - Implication: this SOW is the first dependency for the optimization sequence.
   - Recommendation classification: long-term-best.

## Plan

1. Inspect benchmark helper and resource-envelope code for the smallest production-shaped extension point.
2. Add focused benchmark/test support for low-rate production-shaped overhead.
3. Validate compile/tests and record commands/results.
4. Update durable benchmark docs/specs only if commands or invariants change.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.
- Started implementation for production-shaped benchmark/test harness.
- Added a production-shaped ingest benchmark helper that preserves production
  listener sync defaults instead of forcing the historical benchmark sync
  suppression.
- Added a production-shaped resource benchmark layer with low-rate benchmark
  coverage.
- Added focused tests that lock the benchmark helper sync contract and the
  production-shaped layer's low-rate/configured-sync behavior.
- Removed unrelated formatting churn produced by package-wide formatting; only
  benchmark/test-support code remains changed.

### 2026-06-15

- Added a test-only chart sampler work helper that executes the production
  sampler's expensive inputs without starting an async sampler task:
  - open-tier state sampling
  - tier-flow-index memory sampling
  - facet memory breakdown
  - `/proc/self/status` and `/proc/self/smaps` process memory reads
- Added benchmark report fields for chart sampler sample count and sampler
  wall-time rate.
- Wired the production-shaped benchmark mode to sample chart work at the
  production 1s cadence.
- Added `RecordBatchCursor` so low-rate paced benchmarks can consume partial
  decoded batches instead of starving when the per-tick budget is smaller than
  the decoded batch size.
- Added sync tick timing and existing background I/O counters to the resource
  benchmark report.
- Updated the benchmark README section for the `production-shaped` layer and
  the new fixed-overhead buckets.
- Verified a short 30 flows/s production-shaped child run now achieves about
  30 flows/s and reports chart sampler accounting.

## Validation

Acceptance criteria evidence:

- Production-shaped helper preserves production listener defaults:
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:71`
- Production-shaped benchmark layer includes low-rate coverage and configured
  sync-tick behavior:
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:811`
- Chart sampler accounting is measured as a separate benchmark bucket:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:250`
  - `src/crates/netflow-plugin/src/ingest_resource_bench_support.rs:30`
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:188`
- Sync tick and background I/O accounting are visible in the benchmark report:
  - `src/crates/netflow-plugin/src/ingest_resource_bench_support.rs:30`
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:661`
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:1023`
- Low-rate pacing no longer requires one whole decoded batch per tick:
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:131`
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:719`
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:841`
- README documents the production-shaped layer:
  - `src/crates/netflow-plugin/README.md:366`

Tests or equivalent validation:

- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml benchmark_ingest_helpers_make_sync_shape_explicit -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml production_shaped_resource_layer -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml chart_sampler -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml paced_record_cursor_splits_batches_when_tick_budget_is_smaller_than_batch -- --nocapture`
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml resource_report_serializes_background_overhead_buckets -- --nocapture`
- PASS: short child benchmark:
  `NETFLOW_RESOURCE_BENCH_CHILD=1 NETFLOW_RESOURCE_BENCH_LAYER=production-shaped NETFLOW_RESOURCE_BENCH_PROFILE=low NETFLOW_RESOURCE_BENCH_FLOWS_PER_SEC=30 NETFLOW_RESOURCE_BENCH_WARMUP_SECS=1 NETFLOW_RESOURCE_BENCH_MEASURE_SECS=2 cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml bench_resource_envelope_child -- --ignored --nocapture --test-threads=1`
- PASS: `git diff --check`
- OK - reference search completed:
  `rg -n "production-shaped|plugin-production-shaped|PRODUCTION_SHAPED_RATES|new_production_benchmark_ingest_service" .`
- NOTE: both runs emitted an existing warning that `OpenTierRow` fields are never
  read in `src/crates/netflow-plugin/src/tiering/model.rs`.
- NOTE: `cargo fmt --manifest-path src/crates/Cargo.toml --package netflow-plugin --check`
  currently reports package-wide formatting drift in unrelated files, so the
  formatter was not run. Touched-line formatting was manually adjusted and
  `git diff --check` passes.
- NOTE: `.agents/sow/audit.sh` sensitive-data scan passes. The audit command
  still exits non-zero because durable SNMP trap specs contain pre-existing
  legacy `SOW-NNNN` references unrelated to this benchmark SOW.

Real-use evidence:

- Short production-shaped 30 flows/s benchmark child result:
  - layer: `plugin-production-shaped`
  - requested: `30` flows/s
  - achieved: about `30` flows/s
  - chart sampler samples: `2` during a 2s measurement window
  - sync tick calls: `2` during a 2s measurement window
  - sync tick wall time: about `181 usec/call` on this workstation
  - chart sampler wall time: about `720 usec/sample` on this workstation

Reviewer findings:

- Parent inventory SOW findings apply.

Same-failure scan:

- Current implementation added `plugin-production-shaped`,
  `production-shaped`, `PRODUCTION_SHAPED_RATES`, and
  `new_production_benchmark_ingest_service`.
- Current implementation found and fixed the same low-rate starvation pattern in
  both paced plugin and paced writer loops.
- Dedicated facet-persistence timing remains excluded from this SOW because it
  requires runtime instrumentation and is tracked by the facet persistence SOW.

Sensitive data gate:

- This SOW contains only aggregate performance goals and repo-relative code references.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: no update needed; durable benchmark contract is covered by tests and README.
- End-user/operator docs: README benchmark section updated.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- No update needed.

Project skills update:

- Pending.

End-user/operator docs update:

- Updated `src/crates/netflow-plugin/README.md` benchmark scope for
  `NETFLOW_RESOURCE_BENCH_LAYER=production-shaped`.

End-user/operator skills update:

- Pending.

Lessons:

- Low-rate benchmarks must support partial decoded batches; otherwise a
  per-tick budget smaller than a fixture batch silently measures zero ingest.
- A credible low-rate overhead budget must account for 1s fixed work separately
  from ingest throughput.

Follow-up mapping:

- Related autonomous SOWs are listed in the parent inventory SOW.

## Outcome

Completed. The resource benchmark now has a production-shaped layer that keeps
production listener sync defaults, includes a 30 flows/s low-rate case, runs
tier commits on worker threads, measures chart sampler work, times sync ticks,
reports existing background I/O counters, and preserves existing high-rate
benchmark layers.

## Lessons Extracted

- Low-rate benchmark loops must consume partial batches.
- Benchmark-only overhead accounting can expose fixed background costs before
  runtime optimization SOWs change behavior.

## Follow-up Issues

- Dedicated facet-persistence timing/runtime counters are tracked by
  `.agents/sow/active/SOW-20260614-netflow-facet-persistence.md`.
