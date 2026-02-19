# Phase 4: MySQL Collector Migration Plan (V1 -> V2)

## 1) TL;DR

- MySQL is a good next migration target: it covers static charts, dynamic charts, feature-gated charts, and function handlers in one collector.
- Current design is heavily chart-coupled (runtime chart mutation, `sync.Once`, dynamic key suffixing), so migration should first normalize metrics + labels, then move chart lifecycle to template+engine.
- Recommended rollout is staged and parity-first: keep chart IDs/contexts stable, migrate collection internals to `metrix`, then swap registration to V2-only.

## 2) Requirements (User Verbatim)

- "I think next collector to migrate should be MySQL - it is less complex than postgres but quite complex: it monitors MySQL/MariaDB/Percona so has a bit different set of charts depends on monitored instance, has functions, has charts with labels."
- "Firts I need you to analyze current mysql collector and come up with a step by step plan how we will migrate it."
- "I think mysql charts.yaml is wrong. You creates on group when we should make multuple. Please check mysql.json (in the repo root) - this is mysql metrics split into group - config from our UI - we need to do the same - groups and families"
- "we need to preserve chart context, title, units and dimension"
- "keep id the same is not required and if we can't do it just rely on the auto id generation"
- "i see we have a \"problem\" with replication - mysql creates charts for empty connection and for connections with the same context - we need to split it - should be different charts. When we have connection it should be a label and intsnce by this label and context mysql.slave_behind => mysql.conn_slave_behind."
- "I find current mx (collector metrix) with sinks very hard to follow. simplify it. Instead of iterating over slices with metrics and creating them make it manually (metrics as string literals) and in data collection do switch on metric name and set appropriate."
- "Undeclared metric write panics today - lets remove that."
- "wsrep_local_state and wsrep_cluster_status are statesets"
- "it is not really technically requires but lets use counters (not gagues) for incremental metrics."
- "ok, i think now it is time to fix tests. I see we still build legacy maps in tests. Lets discuss first what we need to test. I think:
  - metrics
  - somehow we need to test that charts from templates created (and dimensions for them set) + some charts needs to be expluded from that check (should be configurable). We need to build I guess some helper test functions so all collectors can call them."

## 3) Facts (Current Code Evidence)

- Collector is still V1-registered (`Create`, not `CreateV2`).
  - Evidence: `src/go/plugin/go.d/collector/mysql/collector.go:26-33`
- Collector returns `map[string]int64` and owns chart state/mutation.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collector.go:221-231`
  - Evidence: `src/go/plugin/go.d/collector/mysql/collector.go:134-141`
- Collection path mutates chart set during runtime based on observed metrics/flags.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collect.go:43-61`
  - Evidence: `src/go/plugin/go.d/collector/mysql/collect.go:82-87`
- Dynamic chart lifecycle is manual and split between collection and chart helpers.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collect_slave_status.go:50-57`
  - Evidence: `src/go/plugin/go.d/collector/mysql/collect_user_statistics.go:24-28`
  - Evidence: `src/go/plugin/go.d/collector/mysql/charts.go:1214-1283`
- Dynamic IDs are compatibility-shaped today (lowercasing/suffixing).
  - Replica connection IDs/dims lowercased: `src/go/plugin/go.d/collector/mysql/charts.go:1011-1018`
  - User IDs/dims lowercased: `src/go/plugin/go.d/collector/mysql/charts.go:1024-1033`
- Vendor/version gates affect what gets collected.
  - Version detection/vendor flags: `src/go/plugin/go.d/collector/mysql/collect_version.go:57-59`
  - Replica query selection by vendor/version: `src/go/plugin/go.d/collector/mysql/collect_slave_status.go:22-28`
  - User-statistics enablement: `src/go/plugin/go.d/collector/mysql/collect.go:27-29`
- MySQL function handlers already use runtime job seam and collector struct access.
  - Evidence: `src/go/plugin/go.d/collector/mysql/func_router.go:62-68`
- Test surface is large and already captures many vendor/feature variants; this is useful parity baseline.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collector_test.go:294`
  - Evidence: `src/go/plugin/go.d/collector/mysql/collector_test.go:1943-1945`

## 4) User Made Decisions (Relevant Carryover)

- V2 direction is metrics + chart templates + chartengine (collector should not manage chart lifecycle manually).
- No backward-compat adapters as a design goal when moving forward (keep code clean and refactor if needed).
- Functions are required for migrated dynamic collectors (MySQL/Postgres path).
- Phase-4 MySQL migration decisions (2026-02-16):
  - `1A`: preserve chart IDs/contexts exactly where possible.
  - `2A`: use a single `charts.yaml` with selector-driven/lazy materialization for vendor-specific parts.
  - `3A`: use template lifecycle expiry for dynamic entities (with conservative values).
  - `4A`: replace MySQL with V2-only registration (no temporary dual path).
  - `5A`: migrate metric writes directly to typed metrix instruments (no temporary map->store bridge).
  - `6A`: use collector-owned explicit `mx` instrument handles pattern (created in `New()` from store) as migration baseline.
  - `7A`: split MySQL chart template into multiple groups/families aligned with `mysql.json` UI grouping model (instead of one flat group).

## 5) Implied Decisions (Needed for Coherent Migration)

- Keep collector externally stable for users where possible:
  - preserve contexts and chart IDs unless explicitly changed,
  - preserve function method IDs/behavior.
- Convert dynamic key suffix model to labeled metric series in store (engine materializes chart instances/dims).
- Replace runtime chart mutation (`Charts().Add`, `sync.Once`) with template-driven lazy materialization.

## 6) Resolved Decisions

1. Chart identity compatibility scope
- Context:
  - Current IDs are explicit and in some places transformed (`lower`) at runtime (`charts.go:1011`, `charts.go:1024`).
- Options:
  - A) Preserve chart IDs/contexts exactly where possible.
    - Pros: minimal dashboard breakage, easiest rollout validation.
    - Cons: template may need transforms and some legacy naming quirks retained.
  - B) Normalize/rework IDs during migration.
    - Pros: cleaner naming long-term.
    - Cons: breaking change risk.
- Outcome: `A` (user-selected on 2026-02-16).

2. Template organization for vendor-specific charts
- Context:
  - MySQL/MariaDB/Percona have partially divergent metric sets and charts.
- Options:
  - A) Single `charts.yaml` using selector-driven/lazy materialization for optional parts.
    - Pros: one source of truth, simpler runtime wiring.
    - Cons: larger template file.
  - B) Multiple templates selected by vendor/version at runtime.
    - Pros: each template smaller.
    - Cons: more compile/load paths and branching.
- Outcome: `A` (user-selected on 2026-02-16).

3. Dynamic entity expiry policy (user stats, replication channels)
- Context:
  - V1 mostly adds dynamic charts but does not centrally expire by policy.
- Options:
  - A) Use template lifecycle expiry (default phase-1 behavior).
    - Pros: aligned with engine model, bounded cardinality.
    - Cons: entities can disappear from charts after absence.
  - B) Disable/extend expiry aggressively for these families.
    - Pros: stable chart presence for intermittent entities.
    - Cons: stale chart buildup.
- Outcome: `A` (user-selected on 2026-02-16), with conservative lifecycle values.

4. Migration switch style
- Context:
  - MySQL currently registers only V1 `Create`.
- Options:
  - A) Replace with V2-only registration in one migration PR.
    - Pros: clean cut, no dual logic.
    - Cons: requires parity confidence before merge.
  - B) Temporary dual registration and feature flag.
    - Pros: fallback path.
    - Cons: added complexity/debt.
- Outcome: `A` (user-selected on 2026-02-16).

5. Metric-write migration style
- Context:
  - MySQL currently writes through `map[string]int64` and key strings, then charts consume that map.
- Options:
  - A) Direct rewrite to typed metrix writes now (no temporary map->store bridge).
    - Pros: cleaner end state immediately, no transient adapter/debt.
    - Cons: larger migration diff.
  - B) Temporary map->store bridge first, rewrite later.
    - Pros: smaller first step.
    - Cons: temporary complexity and double migration.
- Outcome: `A` (user-selected on 2026-02-17).

6. Collector metrics structure pattern
- Context:
  - We need a consistent migration pattern for MySQL and future collectors.
- Options:
  - A) Explicit collector-owned `mx` struct with instrument handles initialized in `New()`.
    - Pros: explicit, discoverable, reusable pattern for future migrations.
    - Cons: larger struct surface.
  - B) Ad-hoc meter lookups in collect paths.
    - Pros: less upfront struct code.
    - Cons: less explicit and harder to review/standardize.
- Outcome: `A` (user-selected on 2026-02-17).

7. MySQL template grouping model
- Context:
  - Current generated `charts.yaml` is one flat group and does not reflect UI grouping hierarchy from `mysql.json`.
- Options:
  - A) Keep flat group for simpler template maintenance.
    - Pros: simpler file structure.
    - Cons: diverges from UI grouping/family model requested by user.
  - B) Split into hierarchical groups aligned with `mysql.json` sections.
    - Pros: matches UI mental model and requested organization.
    - Cons: more verbose template structure.
- Outcome: `B` (user-selected on 2026-02-18; tracked as `7A` decision ID in this plan).

8. MySQL charts.yaml formatting and annotation policy
- Context:
  - User requested style normalization for `charts.yaml` while keeping the new grouped layout.
- Options:
  - A) Keep generated formatting and no chart comments.
    - Pros: no extra maintenance.
    - Cons: diverges from requested style and reduces readability for instance/vendor-specific charts.
  - B) Normalize style and annotate special chart scopes.
    - Pros: consistent file style and clearer maintenance for MySQL/MariaDB/Percona/Galera specific chart blocks.
    - Cons: slightly more verbose YAML.
- Outcome: `B` (user-selected on 2026-02-18): use 2-space indentation, remove `priority`, add blank lines between top-level groups, add comments for instance/vendor-specific charts.

9. Compatibility priority for MySQL migration
- Context:
  - V1 chart IDs are partly dynamic and legacy-shaped, but user priorities for migration are chart semantics and UI continuity.
- Options:
  - A) Preserve IDs strictly.
    - Pros: exact ID continuity.
    - Cons: constrains template evolution and may require legacy-specific naming hacks.
  - B) Preserve chart `context`, `title`, `units`, and dimensions as primary compatibility contract; keep ID when practical, otherwise allow auto-generated ID.
    - Pros: aligns with user-visible continuity while keeping V2 template cleaner.
    - Cons: chart ID continuity is not guaranteed in all cases.
- Outcome: `B` (user-selected on 2026-02-18).

10. Replication chart context split policy
- Context:
  - Replication metrics can produce both unnamed/default connection rows and named connection rows.
  - A single context for both shapes causes semantic mixing in UI.
- Options:
  - A) Keep one context and rely only on labels.
    - Pros: fewer chart templates.
    - Cons: mixes default and per-connection semantics in same context.
  - B) Split replication charts into two contexts:
    - default/empty connection context (`mysql.slave_behind`, `mysql.slave_status`)
    - per-connection context (`mysql.conn_slave_behind`, `mysql.conn_slave_status`) with `connection` label and instance expansion.
    - Pros: clean semantic separation and clearer dashboards.
    - Cons: more chart blocks in template.
- Outcome: `A` (updated by user on 2026-02-18): keep existing replication metric/chart naming and rely on `connection` label + instance expansion in template (no context split).

## Pending Decision

11. MySQL `Check()` mandatory query set for V2 (legacy removal step)
- Context:
  - `Check()` currently runs full collect path (`collect()`), including optional branches and legacy chart code.
  - User direction is to make `Check()` lighter; likely mandatory checks are `SHOW GLOBAL STATUS` and `SHOW GLOBAL VARIABLES`.
- Evidence:
  - Full collect in `Check()`: `src/go/plugin/go.d/collector/mysql/collector.go:218-225`, `src/go/plugin/go.d/collector/mysql/collect.go:17-23`.
  - Optional branches currently run in check path too: `src/go/plugin/go.d/collector/mysql/collect.go:109-125`.
  - Version is currently required by collect path (`collectVersion` gate): `src/go/plugin/go.d/collector/mysql/collect.go:35-41`.
- Options:
  - A) Mandatory in `Check()`: `version + global status + global vars`.
    - Pros: avoids first-cycle failure due version/feature detection problems; still much lighter than full collect.
    - Cons: one extra query compared to strict status/vars-only.
  - B) Mandatory in `Check()`: `global status + global vars` only (no version).
    - Pros: minimal mandatory set.
    - Cons: can pass check but fail first collect if version query fails.
- Recommendation:
  - `A` (more robust startup signal while staying lightweight).
- Outcome:
  - `A` (user-selected on 2026-02-18): `Check()` uses mandatory lightweight path with `version + global status + global vars`; optional probes remain collect-only.

12. Context propagation policy for MySQL collector runtime paths
- Context:
  - `Check(ctx)` and `Collect(ctx)` receive runtime context; collector should use it for DB operations/cancellation propagation.
- Options:
  - A) Keep internal `context.Background()` usage.
    - Pros: no refactor.
    - Cons: ignores runtime cancellation/deadline signals.
  - B) Propagate `ctx` through collector query/open paths used by `Check` and `Collect`.
    - Pros: correct cancellation semantics, consistent with runtime contract.
    - Cons: broader signature updates in helper methods.
- Outcome:
  - `B` (user-selected on 2026-02-18): propagate runtime `ctx` through MySQL collection/check helpers and function bootstrap connection calls.

13. Final MySQL legacy strip scope
- Context:
  - Runtime collection legacy branches are removed; remaining legacy is collector-owned chart state/API and `charts.go`.
- Options:
  - A) Complete full legacy strip now: remove `charts` state + `Charts()` method + legacy chart fields/maps, delete `charts.go`, and migrate remaining tests to store/template assertions only.
    - Pros: clean V2-only collector with no dead legacy internals.
    - Cons: larger test refactor in one step.
  - B) Keep structural legacy temporarily and remove later.
    - Pros: smaller immediate diff.
    - Cons: leaves debt and dual mental model.
- Outcome:
  - `A` (user-selected on 2026-02-18): complete the remaining legacy removal now.

14. MySQL collector `mx`/sink simplification shape
- Context:
  - Current `collectorMetrics` uses name->instrument maps and `metricSink` indirection (`set`, `setReplication`, `setUser`) with bulk declaration loops.
  - User requested more explicit, easier-to-follow manual wiring with literal metric names and switch-based setting.
- Options:
  - A) Keep current map+sinks approach.
    - Pros: compact code and low churn.
    - Cons: indirect flow; harder to follow/debug.
  - B) Remove sink/mapping indirection and move to explicit metric registration + explicit switch-based assignment by metric name.
    - Pros: very explicit control flow and easier code tracing.
    - Cons: more verbose, larger switch blocks.
  - C) Hybrid: remove `metricSink` and keep map-backed `mx`.
    - Pros: moderate simplification with smaller diff.
    - Cons: still map indirection in hot path.
- Recommendation:
  - `B` if readability/debuggability is the top priority (as requested).
- Outcome:
  - `B` (user-selected on 2026-02-18): remove sink indirection and loop-based metric declaration; use explicit metric registration (string literals) and switch-based assignment flow.

15. MySQL instrument declaration source (`charts.yaml` vs collector code)
- Context:
  - `charts.yaml` already declares metric names per group (`metrics:`) and selectors per dimension.
  - Collector currently declares instruments explicitly in `newCollectorMetrics` and panics on undeclared writes.
- Evidence:
  - Template metric declarations and selectors: `src/go/plugin/go.d/collector/mysql/charts.yaml:7`, `src/go/plugin/go.d/collector/mysql/charts.yaml:28`.
  - Explicit collector declaration list: `src/go/plugin/go.d/collector/mysql/metrics.go:45`.
  - Write-time undeclared panic guard: `src/go/plugin/go.d/collector/mysql/metrics.go:225`.
- Options:
  - A) Keep explicit collector declarations as source of truth.
    - Pros: simple runtime, no new coupling between collector and template compiler.
    - Cons: duplicate metric inventory maintenance (`charts.yaml` + `metrics.go`).
  - B) Runtime-generate instrument declarations from parsed `charts.yaml`.
    - Pros: single source of truth at runtime.
    - Cons: adds collector<->template parsing coupling and startup complexity; harder failure mode on template parse/compile mismatch.
  - C) Build-time generate `metrics_generated.go` from `charts.yaml` (go:generate/tool).
    - Pros: single source of truth while keeping runtime simple; explicit generated code remains easy to debug.
    - Cons: needs generator tooling + CI/workflow guard to keep generated code fresh.
- Recommendation:
  - `C` for medium term; `A` for now if you want to keep momentum and avoid toolchain churn in this migration step.

16. Undeclared metric writes in MySQL collector
- Context:
  - `collectorMetrics.set*` currently panic for unknown metric names.
- Evidence:
  - Fixed metric panic path: `src/go/plugin/go.d/collector/mysql/metrics.go:225`.
  - Replication metric panic path: `src/go/plugin/go.d/collector/mysql/metrics.go:241`.
  - User metric panic path: `src/go/plugin/go.d/collector/mysql/metrics.go:285`.
- Outcome:
  - `A` (user-selected on 2026-02-18): do not panic on undeclared writes; ignore/drop unknown metric names.

17. Galera status modeling
- Context:
  - `wsrep_local_state` and `wsrep_cluster_status` are currently exploded into multiple synthetic gauge metrics.
- Evidence:
  - Collected as split booleans in `collect_global_status.go`.
  - Template uses `wsrep_local_state_*` and `wsrep_cluster_status_*` selectors in `charts.yaml`.
- Outcome:
  - `A` (user-selected on 2026-02-18): model both as `StateSet` metrics in metrix, and adapt charts/selectors accordingly.

18. Counter-vs-gauge policy for MySQL metrics
- Context:
  - User asked to use counters for incremental metrics.
- Evidence:
  - Chart template explicitly marks chart algorithms (`incremental`/`absolute`) and dimensions/selectors.
- Outcome:
  - `A` (user-selected on 2026-02-18): metrics used by incremental charts are declared as counters where applicable; absolute charts remain gauges/statesets.

19. StateSet chart dimensions in templates
- Context:
  - Galera charts currently enumerate one selector per state (`...{metric="state"}`), but stateset flattening can expose all states from a single selector.
- Evidence:
  - StateSet flatten emits one scalar series per declared state with label key equal to metric family name:
    - `src/go/pkg/metrix/reader.go:388-431`
  - Planner can infer dimension names from stateset flattened metadata when dimension naming is omitted:
    - `src/go/plugin/go.d/agent/chartengine/planner.go:456-504`
    - `src/go/plugin/go.d/agent/chartengine/compiler.go:208-244`
- Outcome:
  - `A` (user-selected on 2026-02-19): for stateset charts, use a single dimension selector (`selector: <stateset_metric>`) and rely on inferred dimensions.

20. Galera local-state naming normalization
- Context:
  - With inferred stateset dimensions, rendered dimension names come directly from stateset state values.
  - Existing wsrep local-state mapping uses `joiner`, while UI naming previously used `joining`.
- Outcome:
  - `A` (user-selected on 2026-02-19): rename wsrep local-state value from `joiner` to `joining` in stateset modeling.

21. collectorMetrics layout normalization
- Context:
  - Current `collectorMetrics` mixes a map-based global metrics model with explicit replication/userstats vec fields and large switch dispatch blocks.
- Evidence:
  - Mixed struct layout and switches in `src/go/plugin/go.d/collector/mysql/metrics.go`.
- Outcome:
  - `A` (user-selected on 2026-02-19): use a nested `globalStatus` struct (`gauges`/`counters` maps) and map-based containers for `replication` and `userstats` vec instruments.

## Pending Decisions (Testing Refactor)

22. Metrics assertion scope for MySQL V2 tests
- Context:
  - Current tests still compare legacy key maps (`collectLegacyMetricsMap`) instead of native metric names+labels.
- Options:
  - A) Full parity: assert complete metric set as `(name, labels, value)` points per scenario.
  - B) Smoke parity: assert only selected critical metrics.
- Recommendation:
  - `A` for migration confidence; keep optional ignore list for known volatile points.
- Outcome:
  - `A` (user-selected on 2026-02-19): assert complete metric sets as native `(name, labels, value)` points per scenario.

23. Chart-materialization assertion level
- Context:
  - Need to verify template-driven chart creation + dimensions.
- Options:
  - A) Assert chartengine `Plan` actions (`CreateChartAction`, `CreateDimensionAction`, `UpdateChartAction`) directly.
  - B) Assert emitted wire protocol (`CHART`, `DIMENSION`, `BEGIN/SET/END`) only.
  - C) Both: Plan assertions as primary, one smoke wire assertion path.
- Recommendation:
  - `C` for robust semantics checks plus one end-to-end sanity guard.
- Outcome:
  - `C` (user-selected on 2026-02-19): use plan assertions as primary plus one wire-protocol smoke assertion path.

24. Expected-chart strategy in tests
- Context:
  - Optional DB features can make some charts absent by design.
- Options:
  - A) Fully explicit expected chart contexts+dims per scenario.
  - B) Auto-derived from template+observed metrics only.
  - C) Hybrid: explicit must-have contexts+dims + configurable exclude/allow-missing contexts.
- Recommendation:
  - `C` to keep tests readable and resilient to optional feature branches.
- Outcome:
  - `C` (user-selected on 2026-02-19): hybrid expected-chart checks with explicit must-have sets + configurable excludes/allow-missing.

25. Shared helper location for V2 collector tests
- Context:
  - Need reusable helpers for MySQL and future V2 collector migrations.
- Options:
  - A) `src/go/plugin/go.d/pkg/collecttest`
  - B) `src/go/plugin/go.d/agent/module` test helpers only
  - C) Collector-local helpers duplicated per collector
- Recommendation:
  - `A` for reuse without coupling to one collector or module package internals.
- Outcome:
  - `A` (user-selected on 2026-02-19): add shared helpers under `src/go/plugin/go.d/pkg/collecttest`.

26. Chart exclusion configuration key
- Context:
  - Dynamic chart IDs vary by labels; contexts are more stable.
- Options:
  - A) Exclude by chart context only.
  - B) Exclude by chart ID only.
  - C) Support both context and ID filters.
- Recommendation:
  - `C`; use context-first in practice, keep ID escape hatch.
- Outcome:
  - `C` (user-selected on 2026-02-19): support exclusions by both chart context and chart ID.

27. Location of chart materialization assertions in MySQL tests
- Context:
  - Materialization is currently validated in a standalone Galera-only test, while scenario matrix checks focus on metrics.
- Options:
  - A) Move chart-materialization assertions into selected `TestCollector_Collect` scenarios and remove standalone materialization test.
  - B) Keep both standalone and scenario-coupled assertions.
  - C) Keep standalone only.
- Outcome:
  - `A` (user-selected on 2026-02-19): move materialization assertions into selected collect scenarios and remove `TestCollector_ChartTemplateMaterialization`.

28. Template coverage strategy for chart materialization assertions
- Context:
  - Current helper primarily checks explicit `RequiredContexts`, which can miss template drift outside listed contexts.
  - User requested template-driven coverage (derive expected contexts/dimensions from template) with scenario excludes.
- Options:
  - A) Keep explicit required-only checks.
  - B) Template-wide checks (all template contexts + explicit dimension names) with optional targeted required checks for inferred dimensions.
- Outcome:
  - `B` (user-selected on 2026-02-19): assert template-wide coverage and keep optional explicit required checks for inferred-dimension scenarios.

29. Exclusion matching mode for chart materialization assertions
- Context:
  - Exclusions are needed per scenario for optional families (replication/userstats/galera, etc.).
  - User requested glob support and confirmed glob already covers exact/prefix/suffix matches.
- Options:
  - A) Exact matches only.
  - B) Glob-only patterns.
  - C) Exact + glob.
- Outcome:
  - `B` (user-selected on 2026-02-19): use glob-only exclusion patterns (`ExcludeContextPatterns`).

30. Shared chart-materialization helper API shape
- Context:
  - Current MySQL test contains reusable chart/template coverage logic that should move to `collecttest`.
- Options:
  - A) Assert-style helper API (`*testing.T` + internal assertions).
  - B) Data-style helper API (return coverage/validation data + errors; collector tests assert).
- Outcome:
  - `B` (user-selected on 2026-02-19): implement data-style helper API in `collecttest`.

31. Shared helper exclusion scope
- Context:
  - Exclusions are needed by scenario in coverage checks.
- Options:
  - A) Context-glob excludes only.
  - B) Context-glob + chart-ID-glob excludes.
- Outcome:
  - `A` (user-selected on 2026-02-19): support context-glob excludes only.

32. collecttest scalar-collection helper input shape
- Context:
  - Current helper call sites pass `MetricStore()` and `Collect` separately, which is verbose and repeats collector internals at each test site.
- Options:
  - A) Add collector-based helper (`CollectScalarSeriesFromCollector`) and migrate call sites to pass collector directly.
  - B) Keep store+collectFn helper only.
- Outcome:
  - `A` (user-selected on 2026-02-19): add collector-based helper and use collector directly at call sites.

33. collecttest API surface policy
- Context:
  - `collecttest` had many exported helpers that were implementation details and not used by collector tests directly.
- Options:
  - A) Keep broad exported helper set for potential future reuse.
  - B) Restrict exported API to collector-facing helpers only; keep internals package-private.
- Outcome:
  - `B` (user-selected on 2026-02-19): keep only collector-facing exports public and make internal helpers private.

34. collecttest helper API naming/shape cleanup
- Context:
  - User requested simpler collector-facing signatures and less redundant helper layering.
- Outcome:
  - `A` (user-selected on 2026-02-19): rename `CollectScalarSeriesFromCollector` to `CollectScalarSeries`, inline/remove intermediate helper, and change `AssertChartCoverage` to accept a collector-like interface instead of `(templateYAML, store)` arguments.

35. collecttest collector-first assertion seam
- Context:
  - Follow-up request to remove remaining call-site verbosity and keep collector tests on collector-first helpers only.
- Outcome:
  - `A` (user-selected on 2026-02-19): `AssertChartCoverage` accepts collector interface (`MetricStore()`, `ChartTemplateYAML()`), and all MySQL/collecttest call sites use the new helper signature.

## 7) Plan (Step-by-Step)

1. Inventory + mapping freeze
- Build a machine-readable mapping from current charts/dims to source metric keys and expected algorithm/type.
- Extract current compatibility invariants (chart ID, context, dim names, labels) from `charts.go`.
- Artifact (phase-4 step1): `src/go/plugin/go.d/TODO-codex-godv2/phase4-mysql-chart-matrix.md`.

2. V2 collector skeleton
- In `mysql` collector:
  - add `CreateV2`, remove V1-only `Create` path for this collector,
  - add `MetricStore() metrix.CollectorStore`,
  - add `ChartTemplateYAML() string` via embedded `charts.yaml`.
- Keep function registration (`Methods`/`MethodHandler`) unchanged.

3. Metrics model refactor (internal)
- Replace `map[string]int64` emission with store writes (`SnapshotMeter`/`Vec`) directly.
- Introduce `Collector.mx` typed instrument handles (initialized in `New()` from `CollectorStore`).
- Keep SQL query code paths and parsing intact initially to reduce migration risk.
- Introduce labels for dynamic entities:
  - replication channel label (instead of key suffix),
  - user label (instead of `userstats_<user>_*` key expansion).

4. Template authoring (parity-first)
- Translate static chart set from `baseCharts` to compact DSL.
- Translate optional feature charts (InnoDB OS log variants, qcache, galera, myisam, binlog, etc.) to lazy/materialized-by-data templates.
- Translate dynamic replication/user-stat charts using chart ID placeholders and dimension selectors.
- Apply placeholder transforms needed for compatibility (e.g., lowercase where currently required).

5. Remove collector-side chart lifecycle logic
- Delete `sync.Once` chart-add fields and runtime `add*Charts()` calls once template parity is proven.
- Remove `charts` field and `Charts()` reliance for MySQL V2 path.

6. Check + collect behavior parity hardening
- Ensure `Check()` still fails on no data/critical query failures and tolerates non-critical branches as before.
- Preserve current toggle/error semantics for optional branches (slave status/user statistics/process list).

7. Function regression pass
- Verify methods still resolve and execute through `RuntimeJob` with V2 collector.
- Ensure function handlers that read collector internals remain valid after refactor.

8. Final cutover
- Remove leftover V1-only MySQL artifacts once tests pass and parity criteria are met.

### Legacy-Removal Checklist (Current Status)

The collector is V2-registered, and recent cleanup removed part of legacy internals:

- Legacy chart ownership still exists in collector state and constructor:
  - `src/go/plugin/go.d/collector/mysql/collector.go:54-61`
  - `src/go/plugin/go.d/collector/mysql/collector.go:143-150`
  - `src/go/plugin/go.d/collector/mysql/collector.go:229-231`
- Legacy map collection path has been removed from collector runtime code:
  - `Check()` uses mandatory lightweight path, no map sink.
  - `Collect()` writes store-only.
- `legacyCharts` branching and chart mutation calls were removed from collect path.
- Legacy chart definitions/helpers are still present:
  - `src/go/plugin/go.d/collector/mysql/charts.go:87`
  - `src/go/plugin/go.d/collector/mysql/charts.go:1214-1283`
- Tests are still map/charts-based:
  - `src/go/plugin/go.d/collector/mysql/collector_test.go:175-177`
  - `src/go/plugin/go.d/collector/mysql/collector_test.go:327-333`
  - `src/go/plugin/go.d/collector/mysql/collector_test.go:1963-1974`

Recommended removal sequence:

1. Remove `legacyCharts` branching and chart mutation calls from collection path.
  - Status: completed on 2026-02-18.
2. Remove legacy sink/map path (`collect()`, `legacyMapMetricSink`), keep store-only write path.
  - Status: completed on 2026-02-18.
3. Update `Check()` to V2 semantics (collect to store + verify non-empty via reader).
  - Status: completed on 2026-02-18 with mandatory lightweight check (`version + global status + global vars`).
4. Drop `charts` state + `Charts()` method + legacy chart fields/maps from collector struct.
  - Status: completed on 2026-02-18.
5. Delete legacy `charts.go` and any now-unused helpers (`slaveMetricSuffix`, dynamic add* helpers).
  - Status: completed on 2026-02-18.
6. Rework tests from `map[string]int64` + `module.Charts` assertions to store-reader assertions + chart template compile/runtime assertions.
  - Status: in progress (2026-02-18). `module.Charts` assertions removed; parity assertions currently still compare legacy-key maps generated from store snapshots.

## 8) Testing Requirements

- Unit tests:
  - registration and module contract tests for MySQL V2 seams,
  - template compile/validate tests for MySQL `charts.yaml`.
- Collector parity tests:
  - reuse current SQL mock matrix (MySQL/MariaDB/Percona variants),
  - assert expected chart actions/IDs/contexts/dims via chartengine output,
  - assert metric values parity for representative scenarios.
- Function tests:
  - keep existing function tests green (`func_top_queries`, `func_deadlock_info`, `func_error_info`).
- Race safety:
  - run collector and metrix tests with `-race`.

## 9) Documentation Updates Required

- `src/go/plugin/go.d/collector/mysql/integrations/*.md`
  - note V2 template-driven chart lifecycle and any behavior changes.
- Developer docs under `src/go/plugin/go.d/TODO-codex-godv2/`
  - add MySQL migration notes and resolved compatibility decisions.
- If any chart IDs/contexts must change, explicitly document migration impact.

## 10) MySQL Collector Code-Smell / Readability Audit (2026-02-19)

### Findings (evidence-based)

1. InnoDB status parser can fail silently on large payloads.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/collect_engine_innodb_status.go:22-35`
- Facts:
  - Uses `bufio.Scanner` with default token limit and does not check `scanner.Err()`.
  - On long `SHOW ENGINE INNODB STATUS` lines, scanner can stop with token-too-long and metrics can remain stale without explicit error signal.
- Improvement:
  - Check `scanner.Err()` after scan loop and either return/log parsing failure.
  - Consider `bufio.Reader`-based parsing for long lines.

2. Mandatory check path still performs heavy global-status parsing.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/collect.go:41-47`
  - `src/go/plugin/go.d/collector/mysql/collect_global_status.go:18-57`
- Facts:
  - `checkMandatory` invokes `collectGlobalStatus(..., false)`, but parser still iterates full result set.
  - This duplicates expensive read/parse work relative to normal collect path.
- Improvement:
  - Split probe-vs-collect responsibilities: lightweight check query/probe for `Check`, full parse for collect.

3. Metric definition source is duplicated (registration vs column whitelist).
- Evidence:
  - Registration: `src/go/plugin/go.d/collector/mysql/metrics.go:30-174`
  - Whitelist: `src/go/plugin/go.d/collector/mysql/collect_global_status.go:87-215`
- Facts:
  - Two manually maintained lists represent the same metric universe.
  - Drift risk: metric can be whitelisted but not instrumented (or vice versa).
- Improvement:
  - Introduce one table-driven definition source (`column`, `metric`, `kind`) and derive both paths.

4. Slave-status per-row state handling is implicit and easy to misread.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/collect_slave_status.go:31-72`
- Facts:
  - State struct reused across callback events, reset lifecycle is not explicit at row boundaries.
  - Readability cost and regression risk when query columns evolve.
- Improvement:
  - Make row-state lifecycle explicit (new/reset state per row before emission).

5. User-statistics CPU division condition is hard to parse.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/collect_user_statistics.go:24-35`
- Facts:
  - Mixed `&&`/`||` expression without explicit grouping harms readability and review confidence.
- Improvement:
  - Extract predicate into a named helper or parenthesize grouped condition explicitly.

6. Function handlers duplicate performance-schema readiness logic.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/func_top_queries.go:213-248`
  - `src/go/plugin/go.d/collector/mysql/func_error_info.go:216-240`
- Facts:
  - Same locking/cache/readiness checks are copied in multiple handlers.
  - Higher maintenance cost and divergence risk.
- Improvement:
  - Extract shared helper on router/collector for performance-schema readiness/cache check.

7. Function file layout is heavy and data+logic are mixed in giant files.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/func_top_queries.go` (~634 lines)
  - `src/go/plugin/go.d/collector/mysql/func_deadlock_info.go` (~764 lines)
  - `src/go/plugin/go.d/collector/mysql/func_error_info.go` (~616 lines)
- Facts:
  - Large metadata blocks and runtime logic live together, raising cognitive load.
- Improvement:
  - Split static metadata (columns/label maps/chart groups) into dedicated files; keep handler logic files focused.

8. Coverage gaps in function tests vs parsing complexity.
- Evidence:
  - `src/go/plugin/go.d/collector/mysql/func_top_queries_test.go:26-55`
  - large parsing logic in `src/go/plugin/go.d/collector/mysql/func_deadlock_info.go`
- Facts:
  - Tests mainly validate routing/shape; parser-heavy flows have weaker direct coverage.
- Improvement:
  - Add focused parser tests for deadlock/error/top-query parsing branches.

9. Collector test suite has large repeated expected maps and broad coverage excludes.
- Evidence:
  - Large repeated blocks: `src/go/plugin/go.d/collector/mysql/collector_test.go:246-940`
  - Broad exclude patterns in chart coverage checks:
    - `src/go/plugin/go.d/collector/mysql/collector_test.go:577`
    - `src/go/plugin/go.d/collector/mysql/collector_test.go:960`
    - `src/go/plugin/go.d/collector/mysql/collector_test.go:1539`
- Facts:
  - High duplication and brittleness.
  - Excluded chart families reduce signal for template-drift regressions.
- Improvement:
  - Normalize scenario assertions around smaller focused groups and add dedicated coverage cases for excluded families.

### Recommended Pattern Directions

- Table-driven metric schema as single source of truth (instrument registration + collectors).
- Probe/collect split pattern for check path (fast health check, heavy parse only in collect).
- Shared capability guard helper for function handlers (performance-schema readiness).
- File-by-responsibility split:
  - metadata/static definitions
  - parser/extractor logic
  - handler orchestration

## 11) Concrete Refactor Plan (post-audit)

### Phase A: Safe, low-risk readability/safety fixes

Scope:
- Switch DSN debug log to sanitized DSN only.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collector.go:190`
- Harden InnoDB status parsing:
  - check `scanner.Err()`,
  - avoid silent failures on oversized scanner tokens.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collect_engine_innodb_status.go:23-35`
- Clarify user-statistics special-case predicate via explicit helper/parentheses.
  - Evidence: `src/go/plugin/go.d/collector/mysql/collect_user_statistics.go:27-29`
- Remove/confirm dead fields (`varDisabledStorageEngine`, `varLogBin`) if truly unused.
  - Evidence: writes in `src/go/plugin/go.d/collector/mysql/collect_global_vars.go:34,40`; no collector-path reads.

Deliverables:
- Small targeted diffs only.
- No behavior/model changes beyond safety/readability.

Tests:
- `go test -race ./plugin/go.d/collector/mysql`

### Phase B: Metric schema dedupe and collect-path clarity

Scope:
- Introduce one metric definition table for global status metrics:
  - DB column name
  - internal metric name
  - metric kind (gauge/counter/special stateset hook)
- Use it to derive:
  - instrument registration in `metrics.go`,
  - accepted global-status key set in `collect_global_status.go`.
- Make slave-status row handling explicit (row-local state per record).

Evidence:
- Duplicate schema sources:
  - `src/go/plugin/go.d/collector/mysql/metrics.go:30-174`
  - `src/go/plugin/go.d/collector/mysql/collect_global_status.go:87-215`
- Implicit row state:
  - `src/go/plugin/go.d/collector/mysql/collect_slave_status.go:31-53`

Deliverables:
- Reduced duplication.
- Clearer parsing/data-flow code.

Tests:
- `go test -race ./plugin/go.d/collector/mysql`
- Keep existing metrics coverage assertions green.

### Phase C: Function refactor and test hardening

Scope:
- Extract shared performance-schema availability helper and reuse in top-queries/error-info.
  - Evidence:
    - `src/go/plugin/go.d/collector/mysql/func_top_queries.go:393-422`
    - `src/go/plugin/go.d/collector/mysql/func_error_info.go:216-240`
- Split large function files by responsibility:
  - static metadata definitions (`*_meta.go`)
  - parse/transform logic (`*_parse.go`)
  - handler orchestration (`*_handler.go`)
- Add focused unit tests for parser-heavy function paths not currently directly asserted.

Deliverables:
- Lower cognitive load per file.
- Less duplicated function logic.
- Better parser regression coverage.

Tests:
- `go test -race ./plugin/go.d/collector/mysql`

## 12) Pending Decisions (for implementation start)

36. `Check()` strategy after refactor
- Context:
  - `checkMandatory` currently runs full global-status scan path with `writeMetrics=false`.
  - Evidence:
    - `src/go/plugin/go.d/collector/mysql/collect.go:39`
    - `src/go/plugin/go.d/collector/mysql/collect_global_status.go:19-59`
- Options:
  - A) Keep current query/parse path, only readability cleanup.
    - Pros: no semantic risk.
    - Cons: keeps double heavy scan overhead.
  - B) Add dedicated lightweight probe path for `Check()` (fast query / minimal parsing).
    - Pros: lower DB and collector overhead, cleaner semantics.
    - Cons: small behavior/path divergence from collect.
- Recommendation:
  - `B`.
- Outcome:
  - `B` (user-selected on 2026-02-19): add dedicated lightweight probe path for `Check()`.

37. Optional-family disable policy on collection errors
- Context:
  - Current logic can permanently disable family after non-timeout error by setting flag false.
  - Evidence:
    - `src/go/plugin/go.d/collector/mysql/collect.go:98`
    - `src/go/plugin/go.d/collector/mysql/collect.go:105`
- Options:
  - A) Keep current behavior.
    - Pros: existing behavior unchanged.
    - Cons: transient failure can disable feature until restart.
  - B) Keep retrying every cycle (do not permanently disable on transient non-timeout errors).
    - Pros: self-recovery after transient faults.
    - Cons: possible repeated log noise.
  - C) Backoff retry policy (disable temporarily, then retry after cooldown).
    - Pros: controlled logs + recovery.
    - Cons: more state/complexity.
- Recommendation:
  - `C` for balanced resilience/readability.
- Outcome:
  - `A` (user-selected on 2026-02-19): keep current permanent-disable behavior on non-timeout errors for optional families.

38. Function file split depth
- Context:
  - `func_top_queries.go`, `func_deadlock_info.go`, `func_error_info.go` are large mixed files.
  - Evidence:
    - `src/go/plugin/go.d/collector/mysql/func_top_queries.go`
    - `src/go/plugin/go.d/collector/mysql/func_deadlock_info.go`
    - `src/go/plugin/go.d/collector/mysql/func_error_info.go`
- Options:
  - A) Minimal split: only move static metadata blocks out.
    - Pros: smaller diff.
    - Cons: parser/handler density remains high.
  - B) Full split (meta + parse + handler files).
    - Pros: clearest long-term structure.
    - Cons: larger refactor diff.
- Recommendation:
  - `B`.
- Outcome:
  - `B` (user-selected on 2026-02-19): full split by responsibility (metadata + parser + handler files).

39. Collector test cleanup scope in this refactor pass
- Context:
  - Existing test matrix has repeated large expected maps and broad excludes for chart coverage.
  - Evidence:
    - repeated expected maps: `src/go/plugin/go.d/collector/mysql/collector_test.go:270`, `:455`, `:605`, etc.
    - excludes: `src/go/plugin/go.d/collector/mysql/collector_test.go:578`, `:961`, `:1540`
- Options:
  - A) Defer test-structure cleanup to separate pass; keep current assertions.
    - Pros: lower immediate risk.
    - Cons: keeps readability/brittleness debt.
  - B) Include targeted cleanup now (shared fixtures/helpers for repeated expected subsets + tighter coverage cases).
    - Pros: improves maintainability immediately.
    - Cons: larger review surface in same PR.
- Recommendation:
  - `A` for focus and safer phased delivery.
- Outcome:
  - `A` (user-selected on 2026-02-19): defer structural test cleanup to a separate pass.

40. Phase-B simplification for global-status filtering
- Context:
  - Original Phase-B plan proposed a single shared definition table for global-status metric filtering/registration.
  - User requested a simpler path: remove `globalStatusKeys` whitelist and rely on `mx.set(...)` to ignore undeclared metrics.
- Outcome:
  - `A` (user-selected on 2026-02-19): remove `globalStatusKeys` and use `mx.set` as the runtime filter.

## 13) Implementation Progress Snapshot

- 2026-02-19:
  - Phase A completed and committed:
    - `58b57d78cd` (`go.d/mysql: apply phase-a readability and safety fixes`)
  - Phase B simplified variant completed and committed:
    - `d4b3278460` (`go.d/mysql: simplify global-status filtering path`)
  - Phase C step 1 (`36.B`) implemented (not committed yet):
    - `Check()` now uses lightweight `probeGlobalStatus()` path instead of full `collectGlobalStatus(..., false)` parsing.
    - Probe query: `SHOW GLOBAL STATUS LIKE 'Threads_connected';`.
    - MySQL collector tests updated and passing with `-race`.
