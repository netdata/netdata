# SOW-20260614-netflow-raw-journal-sync - NetFlow Raw Journal Sync Behavior

## Status

Status: completed

Sub-state: Completed, validated, and externally reviewed.

## Requirements

### Purpose

Make raw journal sync behavior reasonable and clearly documented without weakening the durability contract operators expect.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers raw journal sync behavior and configuration cost.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Parent evidence recorded a local non-default `listener.sync_every_entries: 1024`.
- Stock/default behavior is `0`.
- Raw journal sync can add disk sync work at low flow rates.

Inferences:

- This may be mostly configuration/documentation rather than source optimization.

Unknowns:

- None remaining for this SOW. Measurement showed the stock default is already reasonable; only benchmark tooling needed improvement.

### Acceptance Criteria

- Add or verify tests for `sync_every_entries = 0` and positive thresholds.
- Measure low-rate impact of raw journal sync behavior.
- Preserve documented durability semantics.
- Update docs only if evidence shows operator guidance is needed.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/ingest/service/runtime.rs`
- `src/crates/netflow-plugin/src/plugin_config/types/listener.rs`
- Parent inventory SOW.

Current state:

- Parent inventory records local non-default setting and raw sync evidence.

Risks:

- Changing defaults or docs can alter operator expectations about durability and disk I/O.

## Pre-Implementation Gate

Status: completed

Problem / root-cause model:

- Raw journal sync threshold can add avoidable disk sync overhead, but the observed local value was non-default.
- Stock runtime behavior already disables periodic raw journal fsync by default, so the likely gap is measurable evidence for non-default tuning rather than a production default bug.

Evidence reviewed:

- Parent inventory SOW.
- `src/crates/netflow-plugin/src/plugin_config/types/listener.rs:14` documents `sync_every_entries = 0` as disabling periodic fsync while preserving rotation/shutdown sync.
- `src/crates/netflow-plugin/src/plugin_config/types/listener.rs:35` defaults `sync_every_entries` to `0`.
- `src/crates/netflow-plugin/configs/netflow.yaml:20` documents the stock default and warning.
- `docs/network-flows/configuration.md:70` documents the default, durability semantics, and high-rate stall/drop risk.
- `docs/network-flows/sizing-capacity.md:29` states the default hot path does not fsync.
- `docs/network-flows/sizing-capacity.md:34` warns that enabling periodic fsync trades throughput for tighter crash durability.
- `src/crates/netflow-plugin/src/main_tests.rs:2338` verifies disabled periodic sync syncs only at shutdown.
- `src/crates/netflow-plugin/src/main_tests.rs:2373` sets positive sync threshold in the shared fixture helper used by normal ingest e2e tests.
- `src/crates/netflow-plugin/src/ingest_test_support.rs:71` creates production-shaped benchmark services with stock listener defaults.
- `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:44` includes a 30 flows/s production-shaped benchmark case.
- Pre-change `run_plugin_resource_envelope()` had no benchmark override for non-default listener sync settings; current code applies `ListenerSyncBenchConfig::from_env()` before layer configuration.

Affected contracts and surfaces:

- Listener configuration.
- Raw journal durability and I/O behavior.
- Operator docs if changed.
- Resource-envelope benchmark CLI/env surface.

Clean-end-state target:

- Raw sync behavior is tested, measured, and documented accurately.
- Benchmark tooling can compare stock production-shaped sync behavior with explicit non-default `sync_every_entries` values.
- Removed as redundant (i): none identified.
- Excluded coupled items (ii): facet persistence writes, tier writes, and chart sampler work belong to other SOWs.
- Reference search: completed for `sync_every_entries`, `raw_journal_syncs`, `sync_interval`, and `NETFLOW_RESOURCE_BENCH_*SYNC*`. No production default or option-name change is planned.

Existing patterns to reuse:

- Existing sync behavior tests in `main_tests.rs`.
- Existing config validation.
- Existing resource benchmark env parsing and child-process execution pattern in `ingest_resource_bench_tests.rs`.

Risk and blast radius:

- Medium operator contract risk if defaults or docs change.
- Low code risk if measurement/docs only.

Sensitive data handling plan:

- Use aggregate I/O metrics and synthetic data only.

Implementation plan:

1. Audit raw sync tests and config docs.
2. Add benchmark-only env override for listener sync settings, with unit tests.
3. Measure low-rate stock vs non-default sync impact.
4. Decide whether docs/config changes are justified.

Validation plan:

- Targeted tests for sync behavior.
- Low-rate benchmark with sync threshold variants.
- Full `netflow-plugin` test run if source files change.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if durability invariant is clarified.
- End-user/operator docs: likely update only if tuning guidance changes.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- Not checked yet; local durability contract is primary unless default behavior changes are proposed.

Open decisions:

- No user-owned design fork is open. Production defaults and public listener option names are unchanged.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

## Plan

1. Test/docs audit.
2. Measurement.
3. Tests/docs/config changes if justified.
4. Validation.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Audited runtime, docs, stock config, and tests for `sync_every_entries`.
- Added benchmark-only listener sync overrides:
  - `NETFLOW_RESOURCE_BENCH_SYNC_EVERY_ENTRIES`
  - `NETFLOW_RESOURCE_BENCH_SYNC_INTERVAL_MILLIS`
- Added benchmark report fields:
  - `listener_sync_every_entries`
  - `listener_sync_interval_millis`
- Added positive-threshold test coverage:
  - `positive_sync_threshold_accumulates_until_threshold_then_records_raw_sync`
- Updated `src/crates/netflow-plugin/README.md` benchmark env-var list.
- Measured low-rate production-shaped benchmark at 30 flows/s, low-cardinality mixed profile:
  - Stock `sync_every_entries = 0`: raw sync `0.00/s`, sync tick `17.86 usec/s`, CPU `0.13%` of one core, disk write `49,697 B/s`.
  - Non-default `sync_every_entries = 1024`: raw sync `1.00/s`, sync tick `3,031.70 usec/s`, CPU `0.20%` of one core, disk write `215,175 B/s`.
- Interpretation:
  - The local non-default setting adds measurable recurring fsync work even at low flow rates.
  - The stock default already avoids this overhead and preserves rotation/shutdown sync durability semantics.
  - No operator doc/default change is justified; existing docs already warn about the trade-off.

## Validation

Acceptance criteria evidence:

- Tests for disabled sync:
  - Existing `tests::e2e_disabled_periodic_sync_syncs_raw_journal_only_at_shutdown`.
- Tests for positive threshold:
  - New `ingest::resource_bench_tests::positive_sync_threshold_accumulates_until_threshold_then_records_raw_sync`.
- Low-rate measurement:
  - Release resource benchmark compared stock `0` with non-default `1024` at 30 flows/s.
- Durability semantics:
  - Production runtime/config/docs were not changed.
  - Existing docs continue to state that `0` disables periodic fsync while files sync on rotation and shutdown.
- Docs:
  - Operator docs unchanged because they already describe the measured trade-off.
  - Benchmark README updated to document the new measurement knobs.

Tests or equivalent validation:

- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml ingest::resource_bench_tests:: -- --nocapture`
  - Passed: 9 passed, 3 ignored.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml resource_report_serializes_background_overhead_and_listener_sync_metadata -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml benchmark_ingest_helpers_make_sync_shape_explicit -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml e2e_disabled_periodic_sync_syncs_raw_journal_only_at_shutdown -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml -- --nocapture`
  - Passed: 552 passed, 26 ignored; `grpc_build` 1 passed.
- `git diff --check`
  - Passed.

Real-use evidence:

- Release benchmark, production-shaped layer, low-cardinality mixed profile, 30 flows/s:
  - Stock `sync_every_entries = 0`: raw sync `0.00/s`, disk write `49,697 B/s`.
  - Non-default `sync_every_entries = 1024`: raw sync `1.00/s`, disk write `215,175 B/s`.
  - Measurement uses synthetic decoded fixture records and disk-backed journals, not customer data.

Reviewer findings:

- Parent inventory SOW findings apply.
- First external review round:
  - No production blockers.
  - Accepted findings: make stock benchmark output say periodic fsync is disabled, add invalid-input parser tests, strengthen positive-threshold test below/exact boundary, fix SOW line citation.
- Second external review round:
  - No production blockers.
  - Accepted findings: fix stale SOW citations, add invalid interval parser negative test, remove unused derives, and make listener-sync report metadata consistency explicit.
- Final external review round:
  - No production blockers.
  - Consensus: production-grade and PR-ready.
  - Non-blocking optional polish recorded for future consideration: empty env var handling, partial override tests, print-format smoke tests, and complete README coverage for pre-existing pool env vars.

Same-failure scan:

- Searched for `sync_every_entries`, `raw_journal_syncs`, `sync_interval`, `NETFLOW_RESOURCE_BENCH_*SYNC*`.
- Existing production defaults, stock config, and operator docs already aligned.
- The missing piece was benchmark measurement override plumbing.

Sensitive data gate:

- No sensitive data recorded.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: no update needed.
- End-user/operator docs: no update needed; existing docs already describe default and positive-value trade-off.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- None.

Project skills update:

- None.

End-user/operator docs update:

- None.

End-user/operator skills update:

- None.

Lessons:

- Production defaults were already correct; measurement tooling was the real gap.
- Low-rate fsync overhead can be understood from proxy counters (`raw_journal_syncs_per_sec`, sync tick wall time, disk write bytes/s) without absolute process memory measurement.

Follow-up mapping:

- Parent inventory SOW tracks ordering.

## Outcome

Completed, validated, externally reviewed with no code blockers, committed as `121d47501c`, and pushed to draft PR `#22719`.

## Lessons Extracted

- Keep benchmark knobs explicit when they represent non-production behavior; otherwise future measurements can accidentally hide the setting being investigated.

## Follow-up Issues

None yet.
