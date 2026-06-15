# SOW-20260614-netflow-query-payload - NetFlow Flow Function Query Payload Cost

## Status

Status: completed

Sub-state: Implemented, validated, externally reviewed, rebased, and pushed to the draft PR branch.

## Requirements

### Purpose

Reduce Flow Function query payload allocation and sorting cost while preserving dashboard/API response contracts.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers query and Flow Function payload costs.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Facet vocabulary payload construction clones and sorts full published value vectors before truncation.
- `IngestMetrics::snapshot()` allocates a `HashMap<String, u64>` for API stats responses.

Inferences:

- Query overhead may dominate dashboard refresh in high-cardinality deployments even after ingest overhead is fixed.

Unknowns:

- Which response fields are required by the dashboard and external consumers.

### Acceptance Criteria

- Add or verify tests for Flow Function table, timeseries, and autocomplete payloads.
- Preserve response schema and ordering unless explicitly approved.
- Reduce avoidable cloning/sorting and stats allocation where tests prove equivalent behavior.
- Validate query behavior with high-cardinality facet values.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/query/facets/cache/payload.rs`
- `src/crates/netflow-plugin/src/api/flows/handler.rs`
- `src/crates/netflow-plugin/src/ingest/metrics.rs`
- Parent inventory SOW.
- `src/crates/netflow-plugin/src/query/tests.rs`
- `src/crates/netflow-plugin/src/main_tests.rs`
- Akvorado console query/filter limiting patterns.

Current state:

- Parent inventory records exact query payload evidence.
- `query/facets/cache/payload.rs:11` clones selected values for each field, `:12` clones the full published field, `:13` clones all published values, `:19` repeats the selected-values clone, and `:20` sorts the full row set before `:25` truncates to `FACET_VALUE_LIMIT`.
- `ingest/metrics.rs:100` builds a fresh `HashMap<String, u64>` stats snapshot; `api/flows/handler.rs:59`, `:101`, and `:147` allocate that full ingest snapshot for autocomplete, timeseries, and table responses before extending query-specific stats.
- Contract tests exist for table, timeseries, and autocomplete Function responses in `main_tests.rs:957`, `main_tests.rs:1221`, and `main_tests.rs:1261`.
- Facet vocabulary tests exist in `query/tests.rs:846` and `query/tests.rs:1020`, but no test currently pins the high-cardinality "many values, return limit" case.

Risks:

- Flow Function payload shape is user-facing.
- Changing ordering or truncation can affect dashboard UX.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Query payload generation can do O(full facet cardinality) clone/sort work even when the response returns a limited subset.
- Root cause 1: `build_facet_vocabulary_payload()` materializes and sorts every published value before truncating to the response limit.
- Root cause 2: `IngestMetrics::snapshot()` eagerly allocates every ingest stats key even when Function responses only need to merge a small query stats map with stable ingest counters.

Evidence reviewed:

- Parent inventory SOW reviewer findings.
- `src/crates/netflow-plugin/src/query/facets/cache/payload.rs:11-25` - duplicate selected-values lookup, full published-field clone, full values clone, full sort, then truncate.
- `src/crates/netflow-plugin/src/ingest/metrics.rs:100-331` - full stats `HashMap<String, u64>` allocation path.
- `src/crates/netflow-plugin/src/api/flows/handler.rs:59-60`, `:101-102`, `:147-148` - all response modes build a metrics snapshot and extend it with query stats.
- `src/crates/netflow-plugin/src/main_tests.rs:957-1078`, `:1221-1258`, `:1261-1320` - existing Function response contract tests for table/autocomplete/timeseries.
- `src/crates/netflow-plugin/src/query/tests.rs:846-917`, `:1020-1065` - existing facet payload ordering and selected-value coverage.
- `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/filter.go:61-67`, `:188-195`, `:232-241` - autocomplete/filter completions carry an explicit limit into the query rather than building an unlimited result in memory.
- `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/query.go:45-67` - graph row selection pushes ordering and `LIMIT` into the bounded query result.
- `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/config.go:34-35`, `:96-109` - returned graph dimensions are bounded by a configured limit.

Affected contracts and surfaces:

- NetFlow Flow Function API responses.
- Dashboard tables/facets/autocomplete.
- API stats payloads.

Clean-end-state target:

- Query payload generation only clones/sorts the data needed for the response while preserving schema and ordering contracts.
- Selected values are looked up once per field, not cloned twice.
- Published facet fields and value vectors are not cloned wholesale just to build a limited response.
- High-cardinality facet vocabulary construction returns the same ordered top `FACET_VALUE_LIMIT` rows while avoiding a full sorted output vector.
- Function stats payload keys and values remain byte-for-byte equivalent at the JSON contract level for existing table, timeseries, and autocomplete tests.
- Removed as redundant (i): duplicate selected-values lookup and full-field clone in facet vocabulary payload construction.
- Excluded coupled items (ii): facet runtime published snapshot storage belongs to facet runtime SOW; query output schema/stat key changes are not part of this SOW.
- Reference search: no shipped response schema, field name, stats key, or ordering contract is being replaced. If implementation requires changing any of those, stop for user approval and run a repository-wide reference search.

Existing patterns to reuse:

- Existing query/facet payload tests.
- Existing presentation helpers.
- Existing `FACET_VALUE_LIMIT` response limit.
- `HashMap::from` for compact query-local stats maps where the set of keys is small.
- Bounded result construction pattern from Akvorado's graph/filter query paths.

Risk and blast radius:

- Medium user-visible API/UX risk.
- Low security risk with synthetic query data.
- High-cardinality synthetic tests are required before implementation because sorting/selection changes can silently alter dashboard option order.
- No public contract change is planned; any schema/key/order change is a user-owned fork.

Sensitive data handling plan:

- Use synthetic query data only. Do not write real flow values in SOW/tests.

Implementation plan:

1. Audit query payload tests and dashboard-facing schema assumptions.
2. Add high-cardinality and selected-value edge tests.
3. Optimize facet vocabulary construction without schema/order/limit changes.
4. Optimize stats merge/allocation without schema/key changes.
5. Validate targeted query tests, Function tests, and full crate tests.

Validation plan:

- Targeted query/facet tests.
- High-cardinality synthetic payload test.
- Existing table/timeseries/autocomplete Function tests.
- Full `netflow-plugin` crate test run.
- `git diff --check`.
- Reference search for changed stats/schema names if any; no search required if keys and schema stay unchanged.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if Flow Function payload invariant is clarified.
- End-user/operator docs: update only if API behavior changes.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- Akvorado console uses explicit limits for graph dimensions and filter completions before result materialization:
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/filter.go:61-67`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/filter.go:188-195`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/query.go:45-67`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/config.go:34-35`

Open decisions:

- None. The user selected this SOW ordering, and this SOW preserves the existing public Function response contract. Any contract/schema/stat-key/order change becomes a user-owned decision and must pause implementation.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

## Plan

1. Test/contract audit.
2. Edge-case tests.
3. Optimization and validation.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Continued this SOW after the decoder packet-path SOW was completed, committed, and pushed.
- Audited current code and tests:
  - table Function response: `main_tests.rs:957-1078`
  - autocomplete Function response: `main_tests.rs:1221-1258`
  - timeseries Function response: `main_tests.rs:1261-1320`
  - facet vocabulary payload: `query/tests.rs:846-917`, `query/tests.rs:1020-1065`
- Filled the Pre-Implementation Gate and set implementation status to in-progress.
- Added contract tests before implementation:
  - high-cardinality facet vocabulary truncation with selected values and duplicate selected values;
  - table/autocomplete/timeseries Function response stats include both ingest counters and query-specific counters;
  - ingest stats map extension preserves old query-over-ingest collision behavior.
- Implemented bounded facet vocabulary construction:
  - selected values are borrowed and ranked once;
  - published fields and published value vectors are no longer cloned wholesale;
  - high-cardinality responses keep a bounded heap of the best `FACET_VALUE_LIMIT` rows instead of sorting the full candidate set.
- Implemented response stats map reuse:
  - Function handlers now reuse the query stats map as the final response stats map;
  - `IngestMetrics::extend_snapshot()` inserts ingest counters into that map while preserving existing query keys;
  - `IngestMetrics::snapshot()` remains test-only for existing resource benchmark tests.
- Removed unrelated `cargo fmt` formatting-only churn in `ingest/rebuild.rs` and `query/scan/direct.rs` to keep this SOW diff focused.

## Validation

Acceptance criteria evidence:

- Flow Function table, timeseries, and autocomplete payload contracts are covered by existing E2E tests plus new stats assertions.
- Facet vocabulary high-cardinality behavior is covered by `facet_vocabularies_keep_selected_values_and_sorted_limited_prefix_for_high_cardinality`.
- Response stats equivalence is covered by Function E2E stats assertions and `ingest_metrics_extend_snapshot_preserves_existing_query_stats_on_key_collision`.

Tests or equivalent validation:

- Passed before implementation:
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml facet_vocabularies -- --nocapture`
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml e2e_flows_function_returns_expected_response_sections -- --nocapture`
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml e2e_flows_function_supports_autocomplete_mode -- --nocapture`
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml e2e_flows_metrics_function_returns_top_n_chart_with_on_disk_tier_fallback -- --nocapture`
- Passed after implementation:
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml -- --nocapture`
  - Result: 531 passed, 25 ignored; `grpc_build` 1 passed.
  - Known pre-existing warning remains: `OpenTierRow` fields are reported as dead code in `src/crates/netflow-plugin/src/tiering/model.rs:78`.
- Passed:
  - `git diff --check`

Real-use evidence:

- Akvorado reference check confirms comparable flow UI paths bound graph/filter/autocomplete results before materialization:
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/filter.go:61-67`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 console/query.go:45-67`

Reviewer findings:

- Parent inventory SOW findings apply.
- External review of this SOW/diff: completed with `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen`.
- Reviewer outcome: no blocking findings; all six reviewers classified the final diff as production-grade or merge-ready.
- Non-blocking notes retained for follow-up consideration:
  - add optional tests for empty-selection high-cardinality heap behavior and selected-rank tie direction;
  - keep `INGEST_STATS_SNAPSHOT_KEY_COUNT` aligned with `extend_snapshot()` insertions;
  - test-only `render.rs` still uses full sort/truncate and is outside this production-path SOW.

Same-failure scan:

- Searched for response stats merge call sites: only `api/flows/handler.rs` used `IngestMetrics::snapshot()` in production.
- Searched for facet vocabulary payload builder call sites: `query/facets/cache/service.rs` is the production caller; direct tests cover the helper.

Sensitive data gate:

- No sensitive data recorded.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: pending outcome.
- End-user/operator docs: pending outcome.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- Not required. No response schema, stat key, or public contract changed.

Project skills update:

- Not required.

End-user/operator docs update:

- Not required. Behavior is contract-preserving.

End-user/operator skills update:

- Pending.

Lessons:

- `cargo fmt` can introduce unrelated formatting-only churn in nearby files; check `git diff --stat` and revert unrelated formatter drift before review/commit.

Follow-up mapping:

- Parent inventory SOW tracks ordering.

## Outcome

Completed. Flow Function facet payload construction now avoids duplicate
selected-value clones, avoids cloning full published facet fields/vectors, and
uses bounded selection for high-cardinality value lists. Function response stats
now reuse the query stats map and extend it with ingest counters while preserving
the existing response schema, stat keys, and ordering behavior.

Pending.

## Lessons Extracted

Pending.

## Follow-up Issues

None yet.
