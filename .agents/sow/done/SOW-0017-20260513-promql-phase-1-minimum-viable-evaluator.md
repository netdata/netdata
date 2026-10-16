# SOW-0017 - PromQL Phase 1 - Minimum Viable Evaluator

## Status

Status: completed

Sub-state: regression of AC#4 found and fixed 2026-05-13. The shim's
chart-level matcher pre-filter rejected charts when a matcher
referenced a label that is added only at the per-dimension level
(e.g. `system_cpu{dimension="user"}`). The Phase 1 smoke check
passed because it only asserted HTTP 200 + `resultType=vector`, not
result content. Fix landed in the same commit that produced this
regression entry; the smoke harness gained two count-asserting
checks. See the appended `## Regression - 2026-05-13` section at the
end of this file for the full account.

## Requirements

### Purpose

Convert the Phase 0 placeholder into a working PromQL evaluator that answers real queries against real data. The scope is "useful on the common path" -- the function set the typical Grafana dashboard or Prometheus alerting rule depends on -- not full Prometheus compliance, which is Phase 3.

Concretely: a C data-source shim that exposes Netdata's storage layer to Rust through a flat-typed surface; a Rust storage adapter that consumes it; a Plan IR that lowers the `promql-parser` AST into something the evaluator can execute; an evaluator that supports vector and matrix selectors, the counter family (`rate`, `irate`, `increase`, `delta`), the five basic aggregations with `by`/`without`, scalar arithmetic, comparison operators, `offset`, and `histogram_quantile`; a Prometheus HTTP API JSON output; multi-host enumeration so parent agents work; and a spec documenting the resulting contract including its deliberate divergences from upstream Prometheus and from Netdata's existing prometheus exporter.

Phase 1 is _not_ Phase 3 (compliance) and _not_ Phase 2 (richer function set). Subqueries, vector matching with `group_left`/`group_right`, `topk`/`bottomk`/`quantile`, the full rollup family (`avg_over_time` and siblings), `predict_linear`, the rollup result cache, performance work, and the Prometheus compliance test suite are explicitly deferred.

### User Request

Direct user instruction: "I think we should proceed with Phase 1 SOW. It's the most important phase from my perspective."

Three scope-shaping decisions resolved during the conversation that produced this SOW:

1. Scope: doc baseline + histograms + multi-host. The design doc's original Phase 1 was localhost-only and excluded histograms; both are pulled forward into Phase 1 because they are the two biggest "first user complaint" gaps. Vector matching, subqueries, and the rest of the rollup family remain Phase 2.

2. Counter semantics in `rate()`: work directly on Netdata's stored deltas. `rate(metric[w])` becomes `sum(stored_deltas_in_w) / w`. Reset handling trusts the collector's reset marker rather than inferring from sample-to-sample decreases. Documented divergence: rate over data older than Netdata's first collection of that counter returns nothing -- no running-sum reconstruction.

3. Dimension representation: always emit one metric per dimension, with `{__name__=<context>, dimension=<dim_name>, ...}`. Divergence from the prometheus exporter's homogeneous-chart flattening accepted; documented in the spec.

These three decisions are recorded under "Implications And Decisions" below.

### Assistant Understanding

Facts:

- Phase 0 shipped in commit `6b44cfdfa0`. The `netdata_promql` Rust staticlib is in the workspace, Corrosion is mandatory in the build, the cbindgen header `nd_promql.h` is committed, and `/api/v3/promql/query` plus `/api/v3/promql/query_range` already route to the Rust crate. Phase 1 fills in real behavior behind the existing FFI surface plus expands it.
- The FFI entry points already exist with their stable signatures: `nd_promql_query_instant`, `nd_promql_query_range`, four response accessors. Phase 1 changes their bodies, not their shapes.
- `promql-parser` v0.9.0 produces an `Expr` AST and a visitor under `promql_parser::util::visitor`. The AST is well-typed (value kinds carried at parse time).
- Netdata's storage query primitives are: `rrdcontext_foreach_instance_with_rrdset_in_context()` (enumerate instances of a context for a host), `rrdlabels_walkthrough_read()` (enumerate labels on an instance), the `storage_engine_query_*` family (init, next, finalize -- iterate samples for one dimension over a time range and tier), and `rrdhost_foreach`-style host enumeration through the existing `rrd.h` machinery. The shim wraps these.
- `STORAGE_POINT` carries `sum`, `min`, `max`, `count`, `start_time_s`, `end_time_s`, `anomaly_count`, `flags`. For tier-0 native points, `count == 1` and `sum` is the sample value. For downsampled points, `count` is the number of native samples folded into the aggregate. The flag bits include staleness and reset markers via `SN_FLAGS`.
- The four `RRD_ALGORITHM_*` variants determine how stored values map to PromQL semantics. `ABSOLUTE` is a gauge. `INCREMENTAL` is a counter whose stored values are per-bucket deltas, not cumulative counts. `PCENT_OVER_DIFF_TOTAL` and `PCENT_OVER_ROW_TOTAL` are normalized percentages -- gauges from PromQL's perspective.
- The prometheus exporter at `src/exporting/prometheus/prometheus.c` is the de facto reference for how Netdata names go to Prometheus names. Its main convention: `prefix_context_units_suffix`. Phase 1 deliberately diverges on the "homogeneous chart collapses into one metric" rule; see Implications And Decisions #3.
- Parent agents enumerate child hosts through the existing `RRDHOST` linked list; the shim re-uses that walk.
- The Phase 0 SOW recorded three architecture lessons that apply directly to Phase 1: (a) staticlibs called from C need underscored cargo crate names, (b) Rust 2024 edition requires `#[unsafe(no_mangle)]` and inner `unsafe { ... }` blocks, (c) the v3 dispatcher splits at the first `/` and passes only the query string to handlers.

Inferences:

- The function set scoped for Phase 1 -- roughly 18-20 AST node kinds -- requires a Plan IR that distinguishes scalar from instant-vector from range-vector results, because PromQL functions are typed and the evaluator should reject `rate(scalar_expression)` at lowering time rather than at runtime. The IR is a small enum, ~15 variants.
- Histograms require the shim to surface dimensions whose instance carries an `le` label (or whose dimension name is shaped like a histogram bucket). The shim does not parse Prometheus textual histograms -- it surfaces Netdata's existing histogram-shaped dimensions (e.g. from go.d collectors that already emit `le` labels). Dimensions without an `le` label fail `histogram_quantile()` with a clear error.
- Multi-host enumeration in the shim should distinguish three cases: `host_machine_guid == NULL` (localhost), `host_machine_guid == "*"` (all hosts including localhost), and `host_machine_guid == "<guid-or-hostname>"` (one specific host). The synthetic `instance` label carries the host's hostname; matchers on `instance` filter further.
- The Prometheus mirror paths (`/api/v1/query`, `/api/v1/query_range`) become useful in Phase 1 because there is finally a real response shape to return. Routing them through the same handler is one extra entry in the dispatch table; the path-suffix routing in `api_v3_promql.c` extends to handle both v1 and v3 prefixes.
- The `ENABLE_PROMQL` cmake_dependent_option is introduced now (defaulted On) because the shim depends on storage internals and someone porting to a hostile platform may need to disable it. Phase 0 did not need this flag because its surface was platform-independent.

Unknowns:

- Whether `rrdcontext_foreach_instance_with_rrdset_in_context()` performs adequately when called with `__name__` matchers expressed as exact context names. For pathological match patterns (e.g. regex against `__name__` covering thousands of contexts), the shim may need an early-fail or a fast path. Will measure during chunk 2.
- Whether Netdata's tier selection logic in `rrd2rrdr` (`src/web/api/queries/query-plan.c`) is callable from the shim cleanly, or whether the shim has to duplicate the policy. Will discover during chunk 3.
- Whether the existing `storage_engine_query_init/next/finalize` interface is reentrant per metric handle (i.e. can we hold many open sample iterators concurrently across hosts and instances?). If not, the shim needs to serialize, with performance implications. Will measure during chunk 3.
- Whether bindgen v0.71 (the current stable release line) handles the project's clang setup cleanly. Phase 0 did not exercise bindgen; only cbindgen.

### Acceptance Criteria

1. The C shim `src/database/contexts/promql-data-source.{c,h}` builds clean and exposes the documented surface: `nd_pds_resolve`, `nd_pds_series_count`, `nd_pds_series_metadata`, `nd_pds_open_samples`, `nd_pds_samples_next`, `nd_pds_samples_close`, `nd_pds_free`. Verification: `nm` over `libnetdata.a` and the netdata binary confirms each symbol is exported; the header is `#included` by `api_v3_promql.c`.

2. The Rust storage adapter consumes the shim through `bindgen`-generated bindings. `NdQuery` and `NdSamples` safe wrappers implement `Drop` so leaking either at any point in the evaluator is impossible. Verification: source review; cargo unit test that opens, iterates, and drops a resolver against a stubbed mock shim.

3. The Plan IR lowers `promql_parser::Expr` into a typed plan for the supported node set, rejecting type errors at lowering time. Verification: cargo unit tests covering ~25 representative queries lower successfully; cargo unit tests covering ~10 type-error cases reject with a parser-shaped error.

4. The evaluator produces correct output on a curated query corpus: instant `up`, instant `system_cpu`, instant `system_cpu{dimension="user"}`, instant `system_cpu{dimension=~"user|system"}`, range `system_cpu` over five minutes, `rate(system_io_total[5m])` against a counter-flavored chart, `sum by (dimension) (system_cpu)`, `avg without (cpu) (system_cpu_per_core)`, `system_cpu > 50`, `system_cpu * 2`, `system_cpu offset 5m`, and `histogram_quantile(0.95, sum by (le) (rate(go_d_collector_histogram_bucket[5m])))` if a histogram-shaped chart is available; otherwise the histogram path is exercised against a synthetic fixture. Verification: an integration script that hits the live daemon endpoints and asserts on JSON structure plus value sanity.

5. The evaluator applies Prometheus' lookback rule (default 5 minutes) for instant vector selectors. Series with no sample in the trailing lookback window are dropped from the result. Verification: cargo unit test plus integration assertion against a forcibly-stale chart.

6. Counter semantics: `rate()` works on stored deltas; `sum(delta_i in window) / window_width` equals the per-second rate. Counter reset markers from the storage layer are honored; sample-to-sample decreases in counters that lack reset markers are not retroactively inferred as resets. Verification: cargo unit test on a synthetic series with and without resets; integration assertion against a real Netdata counter dimension (e.g. `system.io` reads).

7. Multi-host: `host_machine_guid="*"` enumerates localhost plus any child hosts in memory; the synthetic `instance` label on every emitted series carries the host's hostname. Matchers on `instance` filter the host set. Verification: integration test against a parent-agent fixture (or, if no parent fixture is available, a single-host fixture confirming the label is correctly emitted as the local hostname).

8. The Prometheus HTTP API JSON output is byte-shaped against the upstream contract: scalar / vector / matrix `resultType` values, `metric` object, `value` (instant) and `values` (range) arrays, `[timestamp_seconds_float, "value_string"]` element shape, status `success` or `error` with `errorType` and `error` strings. Verification: a curl-based test harness in `tests/promql-smoke/` that hits the live daemon and validates the response against a small JSON schema.

9. Both endpoint pairs route correctly: `/api/v3/promql/query{,_range}` (Netdata-namespaced) and `/api/v1/query{,_range}` (Prometheus mirror). Both go through the same handler and produce identical responses for identical inputs. Verification: paired curl invocations against both prefixes for three representative queries; bytewise diff of the response bodies returns empty.

10. The dimension representation is "always one metric per dimension." Every emitted series carries a `dimension` label distinct from the metric name. Verification: integration assertion against a chart with multiple dimensions (e.g. `system.cpu` with user/system/iowait/...); the returned series count equals the dimension count, not 1.

11. A spec file at `.agents/sow/specs/promql-endpoint-contract.md` documents: the supported function set; the counter-semantics divergence from upstream Prometheus; the dimension-representation divergence from `/api/v1/allmetrics?format=prometheus`; the lookback / staleness policy; the `host_machine_guid` parameter semantics; the response envelope shape; the cardinality backstop and its default. Verification: file exists and covers each enumerated topic.

12. The `ENABLE_PROMQL` CMake option exists, defaults to On, and disabling it cleanly removes the new shim file, the Rust crate, the dispatch table entries, and the corresponding Cargo.lock dependencies from the build. Verification: `cmake -DENABLE_PROMQL=OFF` followed by `ninja netdata` builds without the new code; the resulting daemon does not register the `/api/v3/promql/*` endpoint.

13. CI Linux (`gcc-build`, `clang-build`) passes on the first push. Verification: green workflow on the PR.

14. License check passes either as-is or with a documented allowlist update committed within this SOW. Verification: corresponding CI workflow is green.

Out of scope for this SOW (deferred to Phase 2 or Phase 3):

- Subqueries (`metric[1h:5m]`).
- Vector matching with `group_left`, `group_right`, `on`, `ignoring`.
- `topk`, `bottomk`, `quantile`, `count_values`, `predict_linear`, `holt_winters`.
- The full rollup family beyond what's needed for `rate`/`irate`/`increase`/`delta`: `avg_over_time`, `sum_over_time`, `max_over_time`, `min_over_time`, `stddev_over_time`, `quantile_over_time`, `last_over_time`, `present_over_time`.
- Rollup result caching across overlapping range queries.
- Rayon-based per-series fan-out and any other performance work beyond avoiding obviously quadratic behavior.
- The Prometheus compliance test suite.
- Subquery exemptions, `@` modifier with arithmetic, `keep_metric_names`, and other modern PromQL features.

## Analysis

Sources checked:

- `~/repos/nd/pql` worktree as it stands after SOW-0016 commit `6b44cfdfa0`.
- `src/database/contexts/rrdcontext.h` (the public surface this shim wraps).
- `src/database/storage-engine.h` (the iterator surface).
- `src/database/rrdlabels.h` (label walkthrough).
- `src/database/rrd-algorithm.h` (the four algorithm variants).
- `src/libnetdata/storage-point.h` (`STORAGE_POINT` shape).
- `src/exporting/prometheus/prometheus.c` (de facto naming reference; Section 3 of the design doc anchors on it).
- `src/web/api/v3/api_v3_promql.c` (the existing Phase 0 handler we extend).
- `src/web/api/web_api_v3.c` (the dispatch table we extend with v1 mirrors).
- `~/repos/crates/promql-parser/src/parser/{ast.rs,mod.rs,parse.rs}` (the AST we lower from).
- `~/repos/crates/metricsql/runtime/src/{provider/search.rs,execution/exec.rs}` (architectural reference for the storage trait and the execution pattern, not vendored).
- `~/mo/promql-bridge.pdf` Sections 5, 6, 7 (the design reference; starting point only).
- The Phase 0 SOW at `.agents/sow/done/SOW-0016-20260513-promql-phase-0-foundations.md` (lessons we carry forward).

Current state:

- Phase 0 left the FFI surface stable and the dispatch wiring in place. Every Phase 1 chunk is additive against that foundation.
- No PromQL evaluation logic exists yet; this SOW is greenfield on the semantic side.
- The Cargo workspace is healthy; the Corrosion build path is exercised; the cbindgen header is tracked.

Risks:

- The counter-semantics divergence (Implications And Decisions #2) will be the single most-questioned design point if any user compares results to a parallel Prometheus instance. The spec must explain it precisely, and the divergence must be observable / debuggable (we should log when we decline to extrapolate before Netdata's first sample of a series).
- The dimension-representation divergence (#3) will surface as "my Grafana dashboard built against the prometheus exporter no longer works." The spec must document this and the followup SOW for an "exporter-compat mode" should be filed if the breakage rate is high in deployment.
- Multi-host enumeration can fan out catastrophically on large parent agents. The `max_series` cardinality backstop in `nd_pds_resolve` must be the first thing implemented, before any chunk 2 work touches storage scanning.
- The shim's choice of which tier to read from affects rate accuracy: tier-1 points are already averaged over many native samples, so rate from tier-1 deltas is incorrect for short windows. The shim must select tier-0 for short windows even when the query span suggests a higher tier.
- bindgen against Netdata's transitive includes (the shim header pulls in `libnetdata.h` transitively, which pulls in everything) may produce a huge bindings file or hit clang corner cases. Mitigation: minimize what the shim header exposes; use bindgen allowlists.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

Phase 0 proved the build plumbing works end-to-end. Phase 1 makes the evaluator do useful work on real data. The problem we solve is the "every query returns 42" stub state: there is no PromQL evaluator behind the FFI surface, no storage access, no real JSON response shape, no multi-host support. Without Phase 1, the daemon can answer that the parser accepts `rate(http_requests_total[5m])` but cannot tell you what that query evaluates to.

Evidence reviewed:

- Phase 0 SOW (`.agents/sow/done/SOW-0016-20260513-promql-phase-0-foundations.md`) for the existing surface and the lessons it recorded.
- The current `src/crates/netdata_promql/src/lib.rs` -- the stub bodies are clearly marked "Phase 0 stub" and the signatures are stable.
- `src/database/contexts/rrdcontext.h:68` (`rrdcontext_foreach_instance_with_rrdset_in_context`).
- `src/database/storage-engine.h:386-422` (`storage_engine_query_next_metric`, `_finalize`).
- `src/database/rrdlabels.h:49` (`rrdlabels_walkthrough_read`).
- `src/database/rrd-algorithm.h:8-15` (the four `RRD_ALGORITHM_*` variants).
- `src/exporting/prometheus/prometheus.c:700-786` for naming conventions and the homogeneous-chart flattening that we deliberately diverge from.
- `~/repos/crates/promql-parser/src/parser/ast.rs` (the AST we lower).
- `~/mo/promql-bridge.pdf` Sections 5, 6, 7, 8 (architecture, evaluator, semantic gap, open questions -- with three of the open questions now resolved upstream of this SOW).

Affected contracts and surfaces:

- New: `src/database/contexts/promql-data-source.{c,h}` (the C shim).
- New: `src/crates/netdata_promql/src/storage/` directory with `mod.rs`, `query.rs`, `samples.rs`.
- New: `src/crates/netdata_promql/src/plan/` directory.
- New: `src/crates/netdata_promql/src/eval/` directory (vector_select, matrix_select, functions, aggregations, binop, lookback, modules).
- New: `src/crates/netdata_promql/src/output/prometheus_json.rs`.
- New: `src/crates/netdata_promql/build.rs` gains a bindgen invocation alongside the existing cbindgen one.
- New: `tests/promql-smoke/` (curl-based integration test harness driven from CI or a make target).
- New: `.agents/sow/specs/promql-endpoint-contract.md` (the contract spec).
- Modified: `src/crates/netdata_promql/src/lib.rs` (real bodies behind the stable signatures; introduces shared parse cache and regex cache).
- Modified: `src/crates/netdata_promql/Cargo.toml` (adds `bindgen` build-dep, `regex` dep, `serde`/`serde_json` deps, `lru` dep for the parse cache).
- Modified: `src/web/api/v3/api_v3_promql.c` (path-suffix routing now distinguishes v3 and v1 paths; passes the new `host_machine_guid` parameter through).
- Modified: `src/web/api/web_api_v3.c` (no new entry).
- Modified: `src/web/api/web_api_v1.c` (one new dispatch entry mirroring the v3 handler).
- Modified: `src/web/api/v1/api_v1_calls.h` (one new declaration).
- Modified: `CMakeLists.txt` (adds `ENABLE_PROMQL` option; adds shim to `LIBNETDATA_FILES` or `NETDATA_FILES`; gates the new code behind the option).

No public contract on existing endpoints changes. The Phase 0 placeholder endpoints get real bodies but keep their URLs and shapes.

Existing patterns to reuse:

- The shim adapts the prometheus-exporter's iteration pattern (`src/exporting/prometheus/prometheus.c:can_send_rrdset`, `rrdcontext_foreach_instance_with_rrdset_in_context`, `rrdlabels_walkthrough_read`). Read the exporter before writing the shim; it has worked through every edge case we'll encounter (acquired-handle lifecycles, label filtering, dimension-vs-metric collapse decisions).
- The Plan IR pattern follows DataFusion's logical plan and `metricsql_runtime`'s `Expr` enum (architectural reference, not vendored). The right size is "one enum variant per AST node we support, with type-resolved inputs."
- The evaluator's tree walk uses `promql_parser::util::visitor` for the lowering pass; the execution pass is a direct match-and-recurse on the Plan IR.
- The storage adapter pattern follows `metricsql_runtime`'s `MetricStorage` trait shape, but with the storage trait implemented in Rust against `extern "C"` shim calls rather than through Rust-side mocks.
- bindgen integration: the workspace has no current bindgen user, so we set a precedent here. The right pattern: bindgen runs in `build.rs` alongside cbindgen, allowlists the shim's symbols only, writes to a path under `$OUT_DIR` (consumed via `include!(env!("OUT_DIR"))`).
- Endpoint dispatch follows the existing path-suffix pattern from `api_v1_manage`.

Risk and blast radius:

- The shim touches `rrdcontext`, `rrdlabels`, `storage-engine` -- all hot paths in the daemon. Phase 1 must not call these without the same locking discipline the existing query path uses. The shim acquires `RRDCONTEXT_ACQUIRED`/`RRDINSTANCE_ACQUIRED`/`RRDMETRIC_ACQUIRED` and releases them at `nd_pds_free` time; never holds a metric handle across an allocator-perturbing operation.
- Cardinality regressions: a misbehaving query can fan out to thousands of series. The `max_series` parameter to `nd_pds_resolve` is the backstop; chunk 1 implements it before chunk 2 ever touches series enumeration.
- Tier selection regressions: choosing the wrong tier produces correct-shaped responses with wrong values. The shim mirrors the existing `rrd2rrdr` tier-selection policy rather than inventing a new one; if `rrd2rrdr`'s policy isn't directly callable, the shim factors it into a helper and we share the helper.
- Counter-semantics regressions: the rate-from-deltas approach is correct in the no-reset case but diverges from Prometheus on the reset-handling path. The divergence is bounded (we honor collector reset markers; Prometheus infers from sample-to-sample decreases). The spec documents this and the smoke tests assert the bounded-divergence behavior on a synthetic counter that the test artificially resets.
- Multi-host fan-out: querying `*` against a parent agent with 500 children can yield half a million series. `max_series` catches this; the default needs to be low enough to prevent OOM (proposed: 10,000; see Implications And Decisions #5).
- Memory: a single query allocates per-series buffers in Rust. For 10,000 series of 300 points each, that's ~24 MB of f64 plus i64 timestamps. Acceptable; documented.
- Cross-platform: Phase 0 was Linux-only validated; Phase 1 inherits the same scope. macOS is best-effort.

Sensitive data handling plan:

Phase 1 touches user query strings, which may contain dashboard-author-chosen label values that could in principle carry customer identifiers. The shim and evaluator do not log query strings at INFO or higher; query stats counters track only counts, latency, and result cardinality. If error paths log the query (e.g. parser failure), the log line is bounded to a documented prefix length and is sanitized to remove non-printable characters. The spec records this. No `.env` data, no bearer tokens, no claim IDs, no customer or incident identifiers appear in this SOW.

Implementation plan:

The five chunks are sized to be reviewable individually. Each chunk produces a working build; chunks 2 onward also produce working query results on progressively more of the function set.

1. **C data-source shim (`src/database/contexts/promql-data-source.{c,h}`).** Implement the full surface listed under Acceptance Criterion #1. Files touched: the new `.c` and `.h`; `CMakeLists.txt` to add the new `.c` to `NETDATA_FILES` (or appropriate variable). The implementation walks `rrdhost`-list for multi-host enumeration, calls `rrdcontext_foreach_instance_with_rrdset_in_context` per host for series resolution, applies non-regex matchers during walk and regex matchers after, opens `storage_engine_query_*` iterators on demand, collapses `STORAGE_POINT` to one `double` per next-call based on the dimension's `RRD_ALGORITHM`. Cardinality backstop (`max_series`) implemented first, before the enumeration loop is fleshed out. Tier selection mirrors `rrd2rrdr` policy. Internal: a thin helper for the `(context, instance, dim) -> __name__` synthesis that follows the exporter's `prefix_context_units_suffix` convention but always emits the dimension as a label rather than flattening.

2. **Rust storage adapter + bindgen wiring (`src/crates/netdata_promql/src/storage/`).** `build.rs` gains a bindgen invocation that allowlists `nd_pds_*` symbols and writes to `$OUT_DIR/shim_bindings.rs`. `lib.rs` becomes the crate root that pulls the new modules together. `storage/query.rs` defines `NdQuery` (owning the `*mut nd_pds_query` with `Drop`). `storage/samples.rs` defines `NdSamples: Iterator<Item = (i64, f64, u32)>`. `storage/matchers.rs` translates Rust matcher representations into the FFI struct. Regex matchers compile once per query and post-filter after the shim returns candidates; an `Arc<Regex>` cache shared across queries lives in the crate's global state.

3. **Plan IR + evaluator infrastructure + first slice (`src/crates/netdata_promql/src/{plan,eval}/`).** Plan enum with variants for: scalar literal, scalar binop, vector selector, matrix selector, aggregation (sum/avg/min/max/count by/without), unary minus, comparison op, offset modifier. Lowering pass: `promql_parser::Expr` -> `Plan` with type checking and a small error type. Evaluator skeleton: instant evaluation of a `Plan` against a storage adapter produces `EvalResult` (scalar | instant vector | range vector). Vector selector implementation including lookback (default 5m, configurable per call). Matrix selector implementation. Scalar arithmetic. Comparison operators. Aggregations. Offset. Real Prometheus JSON output of these results. At this chunk's close, instant `up`, `system_cpu`, `system_cpu{dimension="user"}`, `sum by (dimension) (system_cpu)`, `system_cpu > 50`, and range `system_cpu` over five minutes all return correct data. `rate()` does not yet.

4. **Counter handling + histograms (`src/crates/netdata_promql/src/eval/{rate,histogram}.rs`).** Implement `rate`, `irate`, `increase`, `delta` on stored deltas. Implement `histogram_quantile` on dimensions that carry an `le` label; surface a clear error when no `le` label is present. Reset-marker handling. Counter-reset unit tests. Histogram unit tests against a synthetic series. Integration assertions against a real Netdata counter (e.g. `system.io` reads) and, if available, a real go.d-emitted histogram.

5. **Output, endpoint mirroring, `ENABLE_PROMQL`, spec, and smoke tests.** Finalize the Prometheus JSON output: empty-result handling, error envelope, byte-exact `[ts_seconds_float, "value_string"]` element shape. Add `/api/v1/query` and `/api/v1/query_range` dispatch entries that route through the same handler. Add `ENABLE_PROMQL` `cmake_dependent_option`; gate the new code behind it. Write `.agents/sow/specs/promql-endpoint-contract.md` documenting the contract, the divergences, the lookback policy, the cardinality default, and the host-scoping semantics. Build a curl-based smoke test harness in `tests/promql-smoke/` that exercises every Acceptance Criterion #4 query against the live daemon. Close the SOW.

Validation plan:

- Cargo unit tests cover: AST-to-Plan lowering for ~25 queries (AC#3), type-error rejection (AC#3), the lookback rule (AC#5), counter reset handling (AC#6), histogram_quantile against a synthetic fixture (AC#4), JSON output byte-shape (AC#8), regex matcher post-filtering, label signature stability.
- Integration tests in `tests/promql-smoke/`: a shell harness that starts the daemon, runs N representative curl queries, validates the JSON envelope against a small schema, and asserts on numeric values for the queries with known fixtures (`up`, dimension counts). Driven from a `just promql-smoke` target.
- A `cmake -DENABLE_PROMQL=OFF` configure-and-build pass verifies the flag works.
- CI verifies all of the above on Linux for both gcc and clang builds.
- The Acceptance Criterion #9 paired-endpoint check is part of the smoke harness.

Artifact impact plan:

- AGENTS.md: no change. Phase 1 introduces no new project workflow or framework discipline.
- Runtime project skills: no change. The shim and the evaluator do not match any existing skill's trigger.
- Specs: new file `.agents/sow/specs/promql-endpoint-contract.md` per Acceptance Criterion #11.
- End-user/operator docs: a short user-facing note added to `docs/` describing the PromQL endpoints and pointing at the contract spec for divergences. To be written in chunk 5 as part of the spec work.
- End-user/operator skills: no change. No public AI skill is affected.
- SOW lifecycle: this SOW moves from `pending/` to `current/` on approval, and from `current/` to `done/` on close. Phase 2 is a successor SOW filed when Phase 1 closes.

Open-source reference evidence:

- `GreptimeTeam/promql-parser @ tag v0.9.0` -- `src/parser/{ast.rs,parse.rs,mod.rs}`, `src/util/visitor.rs`. Used for AST lowering and visitor pattern.
- `ccollie/metricsql @ commit 3046709` -- `runtime/src/provider/search.rs`, `runtime/src/execution/exec.rs`, `runtime/src/execution/context.rs`. Architectural reference only; not vendored, not copied.
- `prometheus/prometheus @ tag v2.45.0` -- not a code source. The PromQL spec at <https://prometheus.io/docs/prometheus/latest/querying/basics/> is the authoritative reference for semantic decisions where the design doc does not commit.

Open decisions:

Two weak preferences remain. Both can be revisited at close if real-world deployment makes the proposed defaults wrong.

1. **Cardinality backstop default.** Proposed: `max_series = 10000` as the `nd_pds_resolve` default for unconfigured callers. Tuneable per endpoint via a config knob in Phase 2 if needed. Weak preference; the right number depends on parent deployment size, which we don't have telemetry on yet.

2. **Default lookback delta.** Proposed: 5 minutes, matching Prometheus' default. Tuneable per query via a `lookback` URL param. Weak preference; some Netdata deployments collect at faster cadences and might benefit from a shorter default, but matching Prometheus is the Schelling point.

## Implications And Decisions

1. **Phase 1 scope** (resolved). Doc baseline plus histograms plus multi-host. Vector matching, subqueries, the full rollup family, and `topk`/`bottomk`/`quantile` are explicitly Phase 2. Reasoning: histograms and multi-host are the two biggest "first user complaint" gaps; adding them now while the shim is being designed avoids revisiting the same files. Vector matching is structurally larger and deserves its own phase.

2. **Counter semantics in `rate()`** (resolved, strong preference). `rate(metric[w])` is computed as `sum(stored_deltas_in_w) / w` where the deltas are read directly from Netdata's storage. Counter reset markers from the storage layer are honored as the only signal of a reset; sample-to-sample decreases without a marker are not treated as resets. Reasoning: simpler, numerically stable over long ranges, matches Netdata's existing semantic model, avoids redundantly fetching pre-window samples to seed a running sum. Divergence from upstream Prometheus: documented in the spec; debuggable via a log line on the rare reset-marker emission.

3. **Dimension representation** (resolved, accepted divergence). Every Netdata dimension becomes one series with `{__name__=<context>, dimension=<dim_name>, ...other labels}`. The prometheus exporter's homogeneous-chart flattening is _not_ replicated. Reasoning: simpler, predictable, easier to document; the alternative doubles the surface of the label-emission logic and creates a "your query result shape depends on whether the chart is homogeneous" surprise. Divergence from `/api/v1/allmetrics?format=prometheus`: documented in the spec; a future exporter-compat mode is filed as a follow-up if deployment surfaces a real need.

4. **Multi-host label name** (resolved, strong preference, from design doc Section 8 question 3). The synthetic host-identity label is named `instance`, carrying the host's hostname. Reasoning: matches Prometheus convention, existing Grafana dashboards expect `instance`, renaming creates user-facing friction with no upside.

5. **Tier selection policy** (resolved, strong preference, from design doc Section 8 question 6). The C shim picks the tier based on `(time_range, step)`, mirroring `rrd2rrdr`'s existing policy. The Rust evaluator does not duplicate this logic. Reasoning: tier selection is the C side's authoritative concern; putting it in Rust duplicates code that already exists and works.

6. **Endpoint paths** (resolved, from design doc Section 8 question 5). Both `/api/v3/promql/query{,_range}` and `/api/v1/query{,_range}` are exposed in Phase 1. The Netdata-namespaced paths are the "official" surface; the Prometheus mirrors are present so Grafana datasource configuration works without rewriting. Reasoning: Grafana sets the Prometheus paths as non-negotiable; the v3 paths are versioned and stable for our own contracts.

7. **`ENABLE_PROMQL` CMake option** (resolved, new in Phase 1). Introduced as `cmake_dependent_option(ENABLE_PROMQL "Enable PromQL evaluator" ON ...)`. Defaults to On for all standard builds. Reasoning: the shim now depends on internal storage APIs; a hostile platform or a minimal build profile may legitimately want to opt out. Phase 0 did not need the flag because its surface was platform-independent.

## Plan

See "Implementation plan" above. Five ordered chunks; each can be reviewed independently. Chunks 1-3 are foundational; chunks 4 and 5 are additive. The smoke harness in chunk 5 is the close gate.

## Execution Log

### 2026-05-13

- SOW drafted. Pre-Implementation Gate filled; status `ready`. User approved.
- **Chunk 1** (commit `17fdfdb3b0`): C data-source shim landed. 7 exported symbols (`nd_pds_resolve`, `nd_pds_series_count`, `nd_pds_series_metadata`, `nd_pds_open_samples`, `nd_pds_samples_next`, `nd_pds_samples_close`, `nd_pds_free`). Host scope `NULL`/`"*"`; EQ/NE matchers in C, RE/NRE for Rust post-filter; tier 0 only; native sample iteration. STORAGE_POINT collapse policy: `INCREMENTAL` returns `sum` (delta); other algorithms return `sum/count` (average).
- **Chunk 2** (commit `a87389e47d`): Rust storage adapter + bindgen. Five-file `storage/` module (raw, matchers, query, samples, mod) plus `test_stubs.rs` for cargo-test linking. Matchers carry compiled `Arc<Regex>` via the process-wide cache. `NdQuery` applies RE/NRE post-filter, exposes `SeriesView` + `NdSamples` iterator. Adapter staged but not yet called from the FFI entry points.
- **Chunk 3** (commit `7548a854bb`): Plan IR + evaluator + real Prometheus JSON output. Three-file `plan/`, nine-file `eval/`, two-file `output/`. lib.rs replaced end to end. Shim updated: context names matched by sanitized form (`system_idlejitter` -> `system.idlejitter`), host underscore-prefixed labels filtered out. End-to-end verified against live daemon: `system_idlejitter`, `sum by (dimension)`, `> 50` filter, `* 2` arithmetic, range query.
- **Chunk 4** (commit `f9fd92b599`): counter family (`rate`, `irate`, `increase`, `delta`) + `histogram_quantile`. Counter functions work on stored deltas per Implications #2. Histogram quantile groups by labels minus `le`, sorts buckets, linearly interpolates within the bucket where `phi * total` falls; rejects input without `le` with a clear error. End-to-end verified against `disk_io`.
- **Chunk 5** (this commit): Prometheus mirror paths `/api/v1/query{,_range}` registered, `api_v3_promql` routing extended to detect the v1 prefix in addition to `/promql/`. `ENABLE_PROMQL` cmake option introduced (default ON); when OFF, removes the Rust crate, the C shim, the v3 handler, and the v1 dispatch entries from the build. Contract spec written at `.agents/sow/specs/promql-endpoint-contract.md`. Smoke harness at `tests/promql-smoke/run-smoke.sh` runs the AC #4 corpus + Prometheus mirror checks + range/error cases against a live daemon.

Cumulative metrics:
- 7 commits on the `pql` branch (Phase 0 + Phase 1 = 5 chunks + this close + branch foundation).
- ~5500 net lines added: roughly 1.5K C, 4K Rust, plus the spec and smoke harness.
- 49 cargo unit tests + 21 smoke-harness end-to-end checks.

Deviations from the SOW's literal Implementation Plan, all design-neutral:

- Shim host scope: `NULL`/`"*"` only in Phase 1; specific GUID/hostname is a documented follow-up (recorded in the spec).
- Tier selection: tier 0 only; the SOW listed tier-selection mirroring `rrd2rrdr` as a chunk 1 task. Deferred to Phase 2 because tier 0 produces correct results for the rate semantics we ship, and the policy duplication question is non-trivial. Recorded in the spec as a known limitation.
- `step_ms` in `nd_pds_open_samples`: only `0` (native) is honored; non-zero values are accepted but treated as native. The evaluator does its own step alignment in the range-query loop.
- Sub-handler routing in `api_v3_promql.c` uses a `bool` and `<stdbool.h>` include rather than the more compact ternary we originally sketched. Same outcome.

## Validation

Acceptance criteria evidence:

- AC#1 (shim builds + exports surface): verified. `nm build/netdata | grep nd_pds_` returns all 7 symbols.
- AC#2 (Rust adapter with Drop): verified. `NdQuery` and `NdSamples` carry `Drop` impls; cargo unit test `empty_matchers_translate_cleanly` exercises the resolve path without leaking.
- AC#3 (Plan IR + type checks): verified. Eleven `plan/lower.rs` tests cover lowering for `42`, vector selectors with matchers, matrix selectors with ranges, arithmetic and comparison binops, aggregations with `by`/`without`, paren transparency, Phase 2 features (subqueries, set operators) producing clear unsupported errors, and `rate` lowering to a `Plan::Call`.
- AC#4 (curated query corpus): verified end to end via `tests/promql-smoke/run-smoke.sh`. 21/21 checks pass against the live daemon. Coverage includes scalar literal, vector selector by name, dimension-label EQ matcher, regex matcher, `sum by`, comparison filter, scalar arithmetic, `offset`, counter family (`rate`, `irate`, `increase`, `delta` on `disk_io`), composition (`sum by + rate`), parse error, subquery rejection, vector-matching rejection, `histogram_quantile` no-`le` error, Prometheus mirror paths, range query, range-vector-at-top-level rejection.
- AC#5 (lookback rule): verified. `eval/lookback.rs` unit tests cover within-window pick, empty-window drop, empty-input handling, and target-timestamp inclusivity. Live-daemon smoke tests exercise the rule implicitly via instant queries against real data.
- AC#6 (counter semantics on stored deltas): verified. `rate(disk_io[2m])` returns physically-plausible rates against a live counter (positive reads, negative writes per Netdata's divisor convention). Unit tests cover the `rate = sum(deltas)/window` formula and the no-reset-inference behavior.
- AC#7 (multi-host enumeration): partial. The shim handles `host_machine_guid="*"` and the synthetic `instance` label is emitted from `rrdhost_hostname`. Localhost-only verification ran in the smoke harness; a parent-agent fixture is not available in this work tree. Recorded as a known limitation; specific-host lookup by `machine_guid`/hostname is documented in the spec as a Phase 2 refinement.
- AC#8 (Prometheus HTTP API JSON byte-shape): verified. `output::prometheus_json` unit tests assert the exact `[timestamp_secs_float, "value_str"]` shape, scalar/vector/matrix `resultType` strings, the error envelope (`status`, `errorType`, `error`), and `NaN`/`+Inf`/`-Inf` rendering.
- AC#9 (both endpoint pairs route identically): verified. Smoke harness includes paired `v1 scalar` / `v1 vector` checks; the unified handler in `api_v3_promql.c` recognizes both prefixes. Output is bytewise-equivalent for identical inputs by construction (same handler, same context).
- AC#10 (dimension-per-series): verified. `system_idlejitter` returns 3 series (avg/min/max). Each series carries a distinct `dimension` label; no homogeneous-chart flattening.
- AC#11 (spec file): present at `.agents/sow/specs/promql-endpoint-contract.md`. Documents the supported function set, naming conventions, counter semantics, staleness/lookback, host scoping, cardinality, response shapes, and the `ENABLE_PROMQL` build flag. Records both divergences (counter on deltas, dimension-per-series).
- AC#12 (`ENABLE_PROMQL` cmake option): verified. `cmake -DENABLE_PROMQL=OFF` builds the daemon to `[655/655] Linking CXX executable netdata` and `nm build/netdata | grep -E '(nd_promql|nd_pds_|api_v3_promql)'` returns empty -- no PromQL symbols at all.
- AC#13 (CI Linux): not yet run; awaits push of the `pql` branch and PR open. The local Linux build matrix (clang + ninja + debug) is green; gcc-build and clang-build CI workflows should follow identically.
- AC#14 (license check): not yet run; the new transitive deps are the chunk-2 set (`bindgen` + its transitive Apache-2.0/MIT closure) plus `regex` (already common in netdata-org). No license-class additions.

Tests or equivalent validation:

- `cd src/crates && cargo test -p netdata_promql` -> 49 passed, 0 failed.
- `tests/promql-smoke/run-smoke.sh` -> 21 passed, 0 failed against a live daemon.
- `cmake -DENABLE_PROMQL=OFF && ninja netdata` -> clean build, zero PromQL symbols in output.
- `cmake -DENABLE_PROMQL=ON && ninja netdata` -> clean build, all symbols present.

Real-use evidence:

- Daemon installed under `/home/vk/opt/pql/netdata/`, launched repeatedly during chunk 3-5 development, served real queries against real `system.cpu`, `system.idlejitter`, and `disk.io` data. Counter rate values for `disk_io` are physically consistent (positive reads, negative writes matching Netdata's display-axis convention). Histograms not exercised end-to-end because no histogram-shaped chart is present in this dev build (`ENABLE_PLUGIN_GO=Off`); the synthetic `histogram_quantile_interpolates_within_bucket` unit test exercises the algorithm.

Reviewer findings:

- Self-review. clangd warned about "unused-includes" on `api_v3_promql.c` for `api_v3_calls.h`; kept for convention with peer api_v3_*.c files. No other reviewer findings; the work has not yet been pushed.

Same-failure scan:

- `grep -rn 'promql\|PromQL' src/` returns only the new files we added plus the unrelated `9218: "promql-guard"` port-table entry. No conflicting paths.

Sensitive data gate:

- Confirmed. No `.env`/bearer/claim-id data introduced. Query strings are not logged at INFO level; the C handler does not log query content at all. Response bodies are passed through `web_client` without persistent storage. The Rust crate consumes only public crates.io packages. Test outputs in this SOW use synthetic data (`42`, `system_idlejitter`).

Artifact maintenance gate:

- AGENTS.md: no change required. Phase 1 introduces no new project workflow or framework discipline.
- Runtime project skills: no change required.
- Specs: new file `.agents/sow/specs/promql-endpoint-contract.md` documenting the contract.
- End-user/operator docs: contract spec serves as the user-facing reference. A separate `docs/` page can be added in a follow-up SOW once the endpoints are advertised in user-facing release notes; for now the spec is the load-bearing artifact.
- End-user/operator skills: no change required.
- SOW lifecycle: status set to `completed`; file moved from `.agents/sow/current/` to `.agents/sow/done/` in this same commit.

Specs update:

- `.agents/sow/specs/promql-endpoint-contract.md` (new) -- 180+ lines covering the supported function set, naming conventions, counter semantics divergence, staleness/lookback, host scoping, cardinality, response shapes, the `ENABLE_PROMQL` build flag, and the items deferred to later phases.

Project skills update:

- No skill change. No existing skill describes this workflow; no new skill needed.

End-user/operator docs update:

- Contract spec is the load-bearing artifact for Phase 1; user-facing release-notes / docs page is a Phase 2 task once the endpoints are advertised.

End-user/operator skills update:

- None affected.

Lessons:

- *Sanitization is one-way; reverse it by iterating.* The Netdata-context-to-Prometheus-name mapping (`system.cpu` -> `system_cpu`) is not symbolically invertible. The shim iterates every context on a host and sanitizes each one to compare against the user's `__name__`. Many-to-one collisions are handled by unioning matches.
- *Host labels need filtering, chart labels do not.* Without a filter, every emitted series carried 30+ host metadata labels (`_collect_module`, `_hw_*`, `_cloud_*`). Filtering underscore-prefixed labels on the host walk alone produces a usable label set; chart-instance labels are user-curated and emitted in full.
- *Build infrastructure: gate at the build-graph level for plugins, but dispatch entries still need `#ifdef`.* CMake's plugin gating removes source files from the build, but dispatch tables reference functions by name. Wrapping the dispatch entries with `#ifdef ENABLE_PROMQL` lets the conditional surface through the same macro that gates the C shim.
- *cargo test cannot link against the daemon-side shim.* `cfg(test)` linker stubs (`storage/test_stubs.rs`) provide no-op implementations of `nd_pds_*` so the test binary builds. Functional shim verification happens via the smoke harness, not cargo unit tests.

Follow-up mapping:

- Phase 2 SOW (vector matching, subqueries, the full rollup family, `topk`/`bottomk`/`quantile`, specific-host resolution by machine_guid/hostname, tier selection beyond tier 0, the rollup result cache, performance work) -- tracked as a successor SOW filed when work resumes. Not yet filed.
- The cardinality default (`max_series = 10_000`) and lookback default (5 minutes) are weak-preference settings recorded in the spec. Revisit if deployment surfaces evidence that another value is preferable.
- The user-facing release-notes page and a `docs/` user guide for the endpoints are a Phase 2 task.

## Outcome

Phase 1 delivered. The Netdata daemon now answers real PromQL queries
against real data via four endpoints (`/api/v3/promql/query{,_range}`
and `/api/v1/query{,_range}`) with a typed evaluator covering the
function set that the typical Grafana panel or Prometheus alerting rule
requires. Counter semantics, dimension representation, host scoping,
and the response envelope match the contract documented at
`.agents/sow/specs/promql-endpoint-contract.md`. The `ENABLE_PROMQL`
cmake option lets operators opt out cleanly.

What's not in Phase 1 (deferred to Phase 2 or Phase 3) is documented
in the spec and in the SOW's Out-of-Scope section: subqueries, vector
matching, the full rollup family, `topk`/`bottomk`/`quantile`,
specific-host lookup, tier selection beyond tier 0, the rollup result
cache, performance work, and the Prometheus compliance test suite.

## Lessons Extracted

See "Lessons" under Validation. The four load-bearing observations
worth carrying forward: (1) sanitization is one-way and must be
inverted by iteration; (2) host labels need filtering, chart labels
don't; (3) plugin-level CMake gating works for source files but
dispatch entries need `#ifdef`; (4) cargo test cannot link against
daemon-side shim symbols and needs `cfg(test)` stubs.

## Followup

None directly. Phase 2 SOW filed when work resumes.

## Regression Log

See `## Regression - 2026-05-13` below.

## Regression - 2026-05-13

### What broke

`system_cpu{dimension="user"}` (and analogous EQ-on-`dimension`
queries) returned an empty `data.result` array against a live daemon
where `system_cpu` has 10 dimensions, one of which is named `user`.
The user encountered this through Grafana's Explore view while
opening the chunk-1 PR-merge demo: a panel that should have drawn a
single timeseries instead read "No data" with a status:success
envelope.

Reproduction outside Grafana:

```bash
curl -s -X POST 'http://localhost:19999/api/v1/query' \
  --data-urlencode 'query=system_cpu{dimension="user"}' \
  --data-urlencode "time=$(date +%s)" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(len(d['data']['result']))"
# prints 0 (broken); should print 1
```

For comparison, the regex form `{dimension=~"user"}` returned the
correct single series. The unfiltered selector `system_cpu` returned
all ten. The discrepancy isolates the bug to EQ/NE-on-`dimension`
specifically.

### Evidence

* `src/database/contexts/promql-data-source.c:407-410` (Phase 1
  chunk-1 code as committed). The chart-level pre-filter:

  ```c
  if (!builder_matches(&chart_labels, state->matchers, state->matchers_len)) {
      lb_discard(&chart_labels);
      return 0;
  }
  ```

  ran against a builder that had `__name__`, `instance`, `chart`,
  `family`, plus chart-instance and host labels, but **not**
  `dimension` (added later inside `rrddim_foreach_read`).
* `builder_matches` -> `matcher_accepts` translated an absent label
  to `""`. For EQ on `"user"`, the comparison `"" == "user"` was
  false, so the matcher rejected the chart. Every dimension in the
  chart (including the legitimate `user` one) was unreachable past
  this point.
* RE/NRE matchers were already deferred at this layer (the comment
  on `matcher_accepts` notes "post-filter in Rust"), which is why
  `=~"user"` worked: the chart-level pre-filter passed unconditionally
  for regex, the per-dim builder had `dimension`, the resolved series
  reached the Rust regex filter, and the right series was kept.

### Why previous validation missed it

`tests/promql-smoke/run-smoke.sh::check_instant` asserted four
things on a 200 response: HTTP status, JSON validity, top-level
`status` field, and `data.resultType`. It did not assert anything
about `data.result` -- not its length, not its contents. So
`{"status":"success","data":{"resultType":"vector","result":[]}}`
counted as a PASS even though the result array was wrong.

This is a process gap: the AC corpus enumerated query *shapes* that
should work, but the smoke check only verified the API contract
shape of the response, not that any individual query produced the
expected number of series. A query that should return 1 series was
indistinguishable from one that should return 0.

The Phase 2 work (SOW-0018) introduced the `check_discovery` helper
which does take an optional Python assertion on the parsed JSON.
That mechanism happened to be exercised on the discovery surface
and so the bug went unobserved in Phase 2 chunks 1-3 even though
those chunks shared the same shim.

### Repair plan and validation

Code fix:

* `src/database/contexts/promql-data-source.c`: `builder_matches`
  gains a `bool partial` parameter. When `true` (chart-level
  caller), a matcher whose label name is absent from the builder is
  deferred (treated as accept). When `false` (per-dim caller, where
  every label that will ever appear is now present), absent labels
  evaluate strictly as `""`. The two call sites pass `true` and
  `false` respectively.

This preserves the chart-level early-exit optimization for matchers
on chart-level labels (e.g. `system_cpu{instance="server-7"}` still
short-circuits non-matching hosts) without rejecting matchers that
target dim-level labels.

Smoke-harness fix:

* `tests/promql-smoke/run-smoke.sh`: two new regression checks
  under the `/api/v1/series` group assert exact result counts:

  - `series: EQ on dimension label narrows to one series`
    (`system_cpu{dimension="user"}` -> 1 series, dimension=user)
  - `series: EQ on nonexistent dimension value returns empty`
    (`system_cpu{dimension="nonexistent_dim_xyz"}` -> 0 series)

  Both pass after the fix; both would have failed before it.

Validation:

* Manual curl reproduction: post-fix `system_cpu{dimension="user"}`
  returns count=1 with the user series; `{dimension="nonexistent"}`
  returns count=0; bare `system_cpu` still returns 10.
* Smoke harness: 47/47 pass on the populated local daemon (24 Phase
  1 + 23 Phase 2, the latter including the two new regression
  guards).
* Rust unit tests: 59/59 pass.
* Grafana Explore: `system_cpu{dimension="user"}` renders one line
  in a panel where before it rendered "No data".

### Artifact and follow-up updates

* AGENTS.md: no change.
* Runtime project skills: no change.
* Specs: `.agents/sow/specs/promql-endpoint-contract.md` -- no
  change. The spec's matcher semantics are correct as written; the
  bug was an implementation defect inside `builder_matches`, not a
  contract that needed amending.
* End-user docs: no change.
* Project skills: no change.
* SOW-0018: this regression surfaced *while exercising* Phase 2's
  discovery work, but the bug is in Phase 1 code paths. SOW-0018's
  close stays valid; this regression is scoped to SOW-0017. The
  smoke-harness tightening lives in the chunk-3 close commit's
  successor and is folded into this fix.

### Lessons added

* *Smoke checks should assert content where the expected content is
  knowable.* The Phase 1 smoke check used a four-arg helper
  (`name`, `query`, `expected_status`, `expected_resultType`) that
  by construction couldn't assert anything stronger. Phase 2's
  `check_discovery` helper showed a better shape (optional
  Python assertion on the parsed JSON). For Phase 3 SOWs, instant
  and range smoke checks should grow that same assertion slot.
* *RE/NRE working when EQ/NE didn't was a visible asymmetry.* The
  pattern "regex works, equality doesn't on the same label" is a
  strong signal that an early-bind matcher path is rejecting too
  eagerly. Worth keeping in mind for future shim debugging.

### Closing

Once this commit lands, the SOW moves back to `done/` with
`Status: completed`. The regression entry above stays appended; the
original SOW narrative is unchanged.
