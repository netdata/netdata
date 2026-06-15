# SOW-20260614-netflow-chart-sampler - NetFlow Chart Sampler And Memory Observability

## Status

Status: completed

Sub-state: Completed after local validation and two external review rounds.

## Requirements

### Purpose

Keep useful NetFlow self-observability while making chart sampling and memory telemetry reasonable at low flow rates and high state cardinality.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers chart sampler cadence, process memory sampling, facet memory walks, allocator sampling, and chart update cost.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Chart sampler runs once per second.
- It reads `/proc/self/status`, `/proc/self/smaps_rollup`, `/proc/self/smaps`, allocator state, and facet memory breakdown.
- Facet memory breakdown locks facet state and walks large structures.
- Chart application updates all chart handles every sample.
- NetFlow memory chart IDs and dimensions are documented in the local README and user docs.
- Local Netdata `apps.plugin` treats smaps-derived process memory as expensive and samples/budgets it instead of reading it on every iteration.

Inferences:

- Expensive diagnostic memory sampling should be separated from cheap counters.
- The likely clean route is to preserve chart IDs/dimensions and one-second chart updates, but refresh expensive memory diagnostics on a bounded cadence and reuse the last expensive sample between refreshes.

Unknowns:

- None for the implementation route. Remaining details are code-level naming and validation mechanics inside the approved design.

### Acceptance Criteria

- Add or verify tests for sampler snapshot collection and chart payload values.
- Preserve required chart dimensions or explicitly record any public chart contract decision.
- Disable expensive absolute memory diagnostics by default behind a config option.
- Keep or add lightweight real-time proxy charts that do not read `/proc/self/smaps`, call allocator sampling, or perform heap-byte estimation.
- Validate low-rate CPU impact and ensure memory observability remains useful.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/charts/runtime.rs`
- `src/crates/netflow-plugin/src/charts/process_maps.rs`
- `src/crates/netflow-plugin/src/memory_allocator.rs`
- `src/crates/netflow-plugin/src/facet_runtime.rs`
- Parent inventory SOW.

Current state:

- Parent inventory records the exact chart sampler and memory-walk evidence.
- `NetflowCharts::spawn_sampler()` uses a one-second Tokio interval and samples open tiers, tier indexes, facet memory, process memory, allocator memory, and chart updates in one path:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:90`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:100`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:109`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:112`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:117`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:118`
- Full resident mapping breakdown reads `/proc/self/smaps`:
  - `src/crates/netflow-plugin/src/charts/process_maps.rs:112`
  - `src/crates/netflow-plugin/src/charts/process_maps.rs:115`
- Facet memory estimation locks facet runtime and walks stores/snapshots:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:158`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:163`
- Tier-index memory breakdown walks indexes:
  - `src/crates/netflow-plugin/src/tiering/index/store.rs:192`
  - `src/crates/netflow-plugin/src/tiering/index/store.rs:199`
- Memory chart IDs and dimensions are public chart contracts:
  - `src/crates/netflow-plugin/src/charts/metrics.rs:273`
  - `src/crates/netflow-plugin/src/charts/metrics.rs:297`
  - `src/crates/netflow-plugin/src/charts/metrics.rs:329`
  - `src/crates/netflow-plugin/src/charts/metrics.rs:351`
  - `src/crates/netflow-plugin/src/charts/metrics.rs:383`
- User docs describe these charts as memory troubleshooting signals, not as one-second-fresh diagnostic breakdowns:
  - `docs/network-flows/visualization/dashboard-cards.md:33`
  - `docs/network-flows/visualization/dashboard-cards.md:59`
  - `docs/network-flows/visualization/dashboard-cards.md:68`
  - `docs/network-flows/troubleshooting.md:168`
  - `src/crates/netflow-plugin/README.md:286`
- Production-shaped fresh-state benchmark at 30 flows/s measured chart sampler work at about `1094 usec/s` and `1104 usec/sample`, with `3` samples over a `3s` measurement window.

Risks:

- Reducing memory cadence can hide short-lived memory spikes.
- Renaming/removing chart dimensions can break dashboards or alerts.
- Keeping one-second deep diagnostics preserves fidelity but keeps a fixed overhead that is not proportional to flow rate.

## Pre-Implementation Gate

Status: in-progress

Problem / root-cause model:

- Chart sampling mixes cheap per-second counters with expensive diagnostics, making low-rate deployments pay fixed CPU for memory introspection.
- The expensive path is diagnostic, not required for ingest correctness.
- The chart contract is public; removing or renaming charts/dimensions needs explicit product approval.

Evidence reviewed:

- Parent inventory SOW reviewer findings.
- `src/crates/netflow-plugin/src/charts/runtime.rs`
- `src/crates/netflow-plugin/src/charts/process_maps.rs`
- `src/crates/netflow-plugin/src/charts/metrics.rs`
- `src/crates/netflow-plugin/src/facet_runtime.rs`
- `src/crates/netflow-plugin/src/tiering/index/store.rs`
- `docs/network-flows/visualization/dashboard-cards.md`
- `docs/network-flows/troubleshooting.md`
- `src/crates/netflow-plugin/README.md`
- Netdata `apps.plugin` PSS/smaps sampling precedent:
  - `src/collectors/apps.plugin/README.md:278`
  - `src/collectors/apps.plugin/README.md:295`
  - `src/collectors/apps.plugin/README.md:311`
  - `src/collectors/apps.plugin/apps_os_linux.c:191`
  - `src/collectors/apps.plugin/apps_os_linux.c:220`
- Official Linux documentation: `/proc/pid/status` provides easy-to-parse memory status; `/proc/pid/smaps` reports memory per mapping; `/proc/pid/smaps_rollup` is a pre-summed single-entry variant.

Affected contracts and surfaces:

- Netdata charts emitted by `netflow-plugin`.
- Memory troubleshooting UX.
- Benchmark overhead budget.

Clean-end-state target:

- Chart sampler keeps cheap production observability on by default and only registers/samples absolute memory byte diagnostics when explicitly enabled by config.
- The accepted config shape is `charts.memory_diagnostics.enabled: false` plus `charts.memory_diagnostics.interval: 10s`.
- New/default proxy charts use counts/cardinality rather than byte attribution, and they must not call full memory readers or heap estimators.
- Removed as redundant (i): unconditional default registration/sampling of `netflow.memory_*` byte diagnostics.
- Excluded coupled items (ii): benchmark harness belongs to benchmark SOW; facet persistence belongs to facet persistence SOW.
- Reference search: chart names/dimensions are referenced in metrics, tests, README, and docs. If the selected route preserves chart IDs/dimensions, no chart identity migration is needed; if it removes/renames charts, docs and tests must be updated in this SOW.

Existing patterns to reuse:

- Existing chart snapshot structures and chart tests.
- Existing process map tests.

Risk and blast radius:

- Medium user-visible observability risk.
- Low data/security risk with aggregate process metrics.
- Medium performance risk if expensive sampling remains per second on large state.

Sensitive data handling plan:

- Do not record process maps containing private file paths beyond repo-relative code references. Benchmark reports should remain aggregate.

Implementation plan:

1. Audit existing chart tests and chart contract references.
2. Add tests for any cadence/bucket split.
3. Measure sampler sub-costs: process reads, allocator sampling, facet memory walk, chart updates.
4. Record the user-selected cadence route.
5. Add chart config structs/defaults/validation for opt-in memory diagnostics.
6. Make memory byte chart registration and expensive sampling conditional on that config.
7. Add lightweight count/cardinality proxy charts for default production observability.
8. Update README/operator docs and benchmark expectations.

Validation plan:

- Targeted chart/process map tests.
- Production-shaped low-rate benchmark.
- Reference search for chart dimensions if changed.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if chart cadence or budget invariants become durable.
- End-user/operator docs: update if chart behavior changes.
- End-user/operator skills: no update expected unless troubleshooting guidance changes.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- datadog/saluki @ `e5fbc5a24d37b4dd5a659ab6771d8b33d58e5c97`
  - `lib/process-memory/src/lib.rs:7` documents `/proc/self/smaps_rollup` as the most efficient RSS source, `/proc/self/smaps` as detailed mapping data, and `/proc/self/statm` as cheaper but less accurate.
  - `lib/process-memory/src/linux.rs:100` selects `smaps_rollup` before falling back to full `smaps` and then `statm`.
- prometheus/procfs @ `1082e3d8d4c73ed8e6360357001cbd4a82e40635`
  - `proc_smaps.go:57` reads `/proc/[pid]/smaps_rollup` for summed process memory.
  - `proc_smaps.go:89` falls back to reading and summing `/proc/[pid]/smaps` only when `smaps_rollup` is unavailable.

Open decisions:

1. Chart memory sampling route:
   - Option A: preserve chart IDs/dimensions and one-second chart updates; refresh expensive diagnostics every 10 seconds, reusing the last diagnostic sample between refreshes.
     - Pros: large fixed-overhead reduction; no chart identity migration; memory charts remain useful for leak/backpressure diagnosis.
     - Cons: mapping/facet/tier-index/allocator breakdowns can be up to 10 seconds stale.
     - Risks: very short memory spikes may not be visible in detailed breakdown dimensions.
     - Recommendation classification: long-term-best.
   - Option B: add a user-facing config option for diagnostic memory refresh interval, defaulting to 10 seconds.
     - Pros: operators can tune overhead versus freshness.
     - Cons: adds config/docs/support surface and more test matrix for a plugin-internal diagnostic.
     - Risks: bad settings can either restore high overhead or hide diagnostics too much.
     - Recommendation classification: long-term-best.
   - Option C: disable detailed memory diagnostics by default and make them opt-in.
     - Pros: strongest overhead reduction.
     - Cons: meaningful public observability change; docs and troubleshooting workflows change.
     - Risks: harder production memory investigations unless the operator had enabled diagnostics before the incident.
     - Recommendation classification: surgical only if low overhead is more important than default diagnostics.
   - Option D: keep one-second diagnostics and only micro-optimize individual reads/parsers.
     - Pros: no chart behavior change.
     - Cons: does not address the fixed-cost design; cannot eliminate the facet/tier-index walk cost.
     - Risks: likely leaves the main waste in place.
     - Recommendation classification: surgical but incomplete.
   - Selected: user chose the stronger opt-in route, combining Option C's disabled-by-default diagnostics with Option B's config surface and bounded enabled cadence.
   - Accepted config shape: `charts.memory_diagnostics.enabled: false` and `charts.memory_diagnostics.interval: 10s`.
   - Accepted default observability shape: lightweight real-time proxy charts remain enabled by default; absolute memory byte charts are diagnostic-only.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

2. User direction: expensive absolute memory diagnostics should not be enabled by default after development.
   - Selected direction: make absolute process/facet/tier-index byte diagnostics opt-in through config, and keep or add lightweight real-time proxy signals for normal production operation.
   - Evidence: the current sampler reads `/proc/self/smaps`, allocator state, and facet/tier-index byte breakdowns every second; these are diagnostic costs, not ingest correctness requirements.
   - Implication: default chart behavior changes for the existing `netflow.memory_*` byte charts, so docs/tests must make the disabled-by-default behavior explicit.
   - Risk: operators will not have historical absolute byte attribution unless they enabled the diagnostic config before the incident.
   - Accepted detail: use `charts.memory_diagnostics.enabled` and `charts.memory_diagnostics.interval`; default `enabled: false`, default interval `10s`.
   - Accepted first proxy set: facet value/field counts and tier-index entry counts, alongside existing open-tier and decoder-scope counts.
   - Recommendation classification: long-term-best.

## Plan

1. Chart contract/test audit. Completed.
2. Measurement of sampler sub-costs. Completed.
3. Design options. Completed.
4. Tests, implementation, validation. Completed locally; first external review triaged; second external review pending.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Audited chart sampler runtime, memory chart contract, local docs, Netdata `apps.plugin` smaps sampling precedent, Linux proc documentation, and mirrored OSS process-memory patterns.
- Ran production-shaped fresh-state low-rate benchmark at 30 flows/s; chart sampler measured about `1094 usec/s` and `1104 usec/sample`.
- Identified the first implementation decision: expensive memory diagnostics cadence.
- Added `charts.memory_diagnostics.enabled` and `charts.memory_diagnostics.interval` config with defaults `false` and `10s`.
- Made the `netflow.memory_*` byte charts register only when memory diagnostics are enabled.
- Split default chart sampling into cheap one-second state counts and opt-in absolute memory diagnostics.
- Added default-on proxy charts:
  - `netflow.facet_values`
  - `netflow.facet_fields`
  - `netflow.tier_index_entries`
- Added lightweight facet and tier-index cardinality snapshots.
- Updated README and operator docs so default charts and optional byte diagnostics are explicit.
- Reran production-shaped fresh-state low-rate benchmark at 30 flows/s; default chart sampler measured about `2.81 usec/s` and `2.81 usec/sample`.
- Ran the requested external review round with `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen`.
- Removed residual open-tier byte estimation from the disabled default path; open-tier heap byte estimation now runs only inside the enabled diagnostic refresh path.
- Restricted `MemoryDiagnosticsSamplerState::default()` to tests so production code constructs diagnostics state through `new(interval)`.
- Added README guidance that enabled memory diagnostics refresh on the configured interval and may miss short-lived spikes.
- Reran production-shaped fresh-state low-rate benchmark at 30 flows/s; default chart sampler measured about `3.14 usec/s` and `3.15 usec/sample`.
- Ran the second external review round with `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen`.
- Fixed the medium docs inconsistency found in the second review: dashboard-card docs no longer claim every chart updates every second.
- Added focused tests for nested full-plugin YAML parsing, configured diagnostic refresh cadence, non-zero tier-index cardinality, and Linux-only process-memory assertions.
- Reran production-shaped fresh-state low-rate benchmark at 30 flows/s; default chart sampler measured about `3.03 usec/s` and `3.03 usec/sample`.

## Validation

Acceptance criteria evidence:

- Config surface:
  - `src/crates/netflow-plugin/src/plugin_config/types/charts.rs:5` defines `ChartsConfig`.
  - `src/crates/netflow-plugin/src/plugin_config/types/charts.rs:20` defines `MemoryDiagnosticsConfig`.
  - `src/crates/netflow-plugin/src/plugin_config/types/charts.rs:34` defaults diagnostics to disabled.
  - `src/crates/netflow-plugin/src/plugin_config/validation/charts.rs:5` validates enabled diagnostics.
  - `src/crates/netflow-plugin/configs/netflow.yaml:72` documents the stock disabled-by-default setting.
- Chart contract and registration:
  - `src/crates/netflow-plugin/src/charts/metrics.rs:271` adds `netflow.facet_values`.
  - `src/crates/netflow-plugin/src/charts/metrics.rs:287` adds `netflow.facet_fields`.
  - `src/crates/netflow-plugin/src/charts/metrics.rs:303` adds `netflow.tier_index_entries`.
  - `src/crates/netflow-plugin/src/charts/runtime.rs:74` registers the proxy charts by default.
  - `src/crates/netflow-plugin/src/charts/runtime.rs:80` registers `netflow.memory_*` charts only when diagnostics are enabled.
- Sampling behavior:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:123` samples open-tier counts every second.
  - `src/crates/netflow-plugin/src/charts/runtime.rs:124` samples tier-index cardinality every second.
  - `src/crates/netflow-plugin/src/charts/runtime.rs:140` samples facet cardinality from the published snapshot.
  - `src/crates/netflow-plugin/src/charts/runtime.rs:349` gates expensive diagnostics on `enabled`.
  - `src/crates/netflow-plugin/src/charts/runtime.rs:366` runs open-tier heap-byte estimation, tier-index byte walk, facet heap estimate, and process memory reads only on enabled diagnostic refreshes.
- Lightweight proxy sources:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:71` defines `FacetCardinalitySnapshot`.
  - `src/crates/netflow-plugin/src/facet_runtime.rs:166` exposes facet cardinality without a heap-byte walk.
  - `src/crates/netflow-plugin/src/facet_runtime.rs:971` counts populated, total, exposed, and autocomplete facet fields from the published snapshot.
  - `src/crates/netflow-plugin/src/tiering/index/store.rs:50` defines `TierFlowIndexCardinality`.
  - `src/crates/netflow-plugin/src/tiering/index/store.rs:198` counts tier-index hours and flows without byte estimation.
- Tests:
  - `src/crates/netflow-plugin/src/plugin_config_tests.rs:250` checks diagnostics default to disabled and `10s`.
  - `src/crates/netflow-plugin/src/plugin_config_tests.rs:261` checks YAML can enable diagnostics.
  - `src/crates/netflow-plugin/src/plugin_config_tests.rs:281` checks the nested `charts.memory_diagnostics` YAML path in a full `PluginConfig`.
  - `src/crates/netflow-plugin/src/plugin_config_tests.rs:317` rejects subsecond enabled diagnostics.
  - `src/crates/netflow-plugin/src/charts/tests.rs:94` checks byte charts are registered only when enabled.
  - `src/crates/netflow-plugin/src/charts/tests.rs:119` checks proxy and diagnostic snapshot payload values.
  - `src/crates/netflow-plugin/src/charts/tests.rs:403` checks default sampler work collects production proxy inputs and no diagnostics.
  - `src/crates/netflow-plugin/src/charts/tests.rs:423` checks diagnostics are collected only when enabled, with the `/proc/self/status` assertion limited to Linux.
  - `src/crates/netflow-plugin/src/charts/tests.rs:490` checks diagnostics refresh on the configured cadence.
  - `src/crates/netflow-plugin/src/facet_runtime.rs:1418` checks facet cardinality counts including autocomplete promotion.
  - `src/crates/netflow-plugin/src/tiering/tests.rs:145` checks non-zero tier-index hours and flow cardinality.
- Docs:
  - `src/crates/netflow-plugin/README.md:286` lists default lightweight charts.
  - `src/crates/netflow-plugin/README.md:306` documents byte diagnostics as disabled by default.
  - `src/crates/netflow-plugin/README.md:317` documents configured diagnostic refresh interval behavior.
  - `docs/network-flows/visualization/dashboard-cards.md:33` lists default proxy charts.
  - `docs/network-flows/visualization/dashboard-cards.md:68` says memory charts exist only when enabled.
  - `docs/network-flows/visualization/dashboard-cards.md:17` says default charts update every second and optional memory diagnostics update on their configured interval.
  - `docs/network-flows/troubleshooting.md:168` guides users to default state-cardinality charts first.

Tests or equivalent validation:

- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml memory_diagnostics -- --nocapture`
  - Result: 6 passed, 0 failed.
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml charts::tests:: -- --nocapture`
  - Result: 10 passed, 0 failed.
  - Note: the output includes an expected panic line from the existing poison-recovery test.
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml plugin_config::tests:: -- --nocapture`
  - Result: 34 passed, 0 failed.
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml runtime_cardinality_snapshot_counts_published_proxy_state -- --nocapture`
  - Result: 1 passed, 0 failed.
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml tier_flow_index_store_cardinality_counts_hours_and_flows -- --nocapture`
  - Result: 1 passed, 0 failed.
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml production_shaped_resource_layer_accounts_for_chart_sampler_work -- --nocapture`
  - Result: 1 passed, 0 failed.
- PASS: `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml production_shaped_resource_layer_uses_low_rate_and_configured_sync -- --nocapture`
  - Result: 1 passed, 0 failed.
- PASS: short `bench_resource_envelope_child` run in child mode with layer `production-shaped`, profile `low`, target `30 flows/s`, `1s` warmup, and `3s` measurement.
  - Result: 1 passed, 0 failed.
  - Benchmark result after first implementation iteration: achieved `29.999 flows/s`, `chart_sampler_wall_usec_per_sec=2.812918868856345`, `chart_sampler_wall_usec_per_sample=2.813`.
- PASS: short `bench_resource_envelope_child` run in child mode with layer `production-shaped`, profile `low`, target `30 flows/s`, `1s` warmup, and `3s` measurement.
  - Result: 1 passed, 0 failed.
  - Benchmark result after reviewer-driven cleanup: achieved `29.999 flows/s`, `chart_sampler_wall_usec_per_sec=3.144915666989779`, `chart_sampler_wall_usec_per_sample=3.145`.
- PASS: short `bench_resource_envelope_child` run in child mode with layer `production-shaped`, profile `low`, target `30 flows/s`, `1s` warmup, and `3s` measurement.
  - Result: 1 passed, 0 failed.
  - Final benchmark result after second-review fixes: achieved `29.999 flows/s`, `chart_sampler_wall_usec_per_sec=3.0305854590987047`, `chart_sampler_wall_usec_per_sample=3.030666666666667`.
- PASS: `git diff --check`

Real-use evidence:

- Controlled production-shaped child benchmark before this SOW showed chart sampler cost at about `1094 usec/s` and `1104 usec/sample`.
- The same benchmark after the final local change, with default diagnostics disabled, showed chart sampler cost at about `3.03 usec/s` and `3.03 usec/sample`.

Reviewer findings:

- Parent inventory SOW findings apply.
- First external review round:
  - `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen` were launched against this SOW and the full worktree scope.
  - Captured reviewer verdicts were production-grade; no captured finding identified a blocker, security issue, or correctness regression.
  - Harness output for long reviews was partially truncated after completion, so this SOW records findings by triaged issue rather than full transcripts.
- Finding: facet dirty-bit and disk-hook test changes were observed in the same worktree.
  - Disposition: no chart-sampler action. These changes belong to `.agents/sow/active/SOW-20260614-netflow-facet-persistence.md`, where they are tracked and validated.
- Finding: default sampler still computed open-tier heap bytes and discarded them when diagnostics were disabled.
  - Disposition: fixed. `src/crates/netflow-plugin/src/charts/runtime.rs:295` contains byte estimation in `sample_open_tier_bytes()`, and `src/crates/netflow-plugin/src/charts/runtime.rs:367` calls it only inside the enabled diagnostic refresh path.
- Finding: `MemoryDiagnosticsSamplerState::default()` looked like a possible production footgun because it creates a zero refresh interval.
  - Disposition: fixed. The `Default` impl is now test-only at `src/crates/netflow-plugin/src/charts/runtime.rs:240`.
- Finding: docs should state that enabled byte diagnostics refresh at the configured interval and may miss short spikes.
  - Disposition: fixed at `src/crates/netflow-plugin/README.md:317`.
- Finding: formatting-only rustfmt churn existed outside the chart-sampler scope.
  - Disposition: fixed; unrelated formatting-only diffs were removed.
- Finding: cloning `ChartsConfig` into the sampler task is a tiny startup-time immutable config copy.
  - Disposition: no action. It is not repeated sampler work and is not a meaningful resource overhead.
- Finding: facet proxy counts may be zero until the first published facet snapshot.
  - Disposition: accepted behavior. The proxy intentionally reads the published snapshot to avoid locking and walking facet state in the default sampler path.
- Second external review round:
  - `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen` completed.
  - All reviewers judged the SOW production-grade after the already-applied first-review fixes.
  - One reviewer found a medium documentation inconsistency: `dashboard-cards.md` still said all charts update every second. Fixed at `docs/network-flows/visualization/dashboard-cards.md:17`.
  - Reviewers found low test gaps for nested full-config YAML, diagnostic cadence, non-zero tier-index cardinality, and non-Linux `/proc` assertions. Fixed at `src/crates/netflow-plugin/src/plugin_config_tests.rs:281`, `src/crates/netflow-plugin/src/charts/tests.rs:490`, `src/crates/netflow-plugin/src/tiering/tests.rs:145`, and `src/crates/netflow-plugin/src/charts/tests.rs:481`.
  - Remaining low findings were accepted as non-blocking or sibling-SOW work: diagnostic-path poison warning consistency, exact sub-second interval rounding, `spawn_sampler` argument count, diagnostic-path blocking `/proc` reads when explicitly enabled, and benchmark-harness cleanup.
  - No third external review round was run because the post-review changes were docs/tests only and were covered by focused local validation; rerunning six models for this small cleanup would be wasteful under the repository review-frequency guidance.

Same-failure scan:

- PASS: `rg -n "NetflowCharts::new\\(|spawn_sampler\\(|sample_chart_sampler_work_for_test\\(" src/crates/netflow-plugin/src`
  - Remaining constructor/sampler call sites all pass chart config.
- PASS: `rg -n "memory_diagnostics|netflow\\.memory_|memory_resident|memory_accounted|memory_tier_index|memory_allocator|memory_resident_mapping|facet_values|facet_fields|tier_index_entries" src/crates/netflow-plugin docs/network-flows .agents/sow/active/SOW-20260614-netflow-chart-sampler.md`
  - Remaining memory chart documentation either code/tests or explicitly says the charts are optional diagnostics.

Sensitive data gate:

- No sensitive data recorded.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: no durable spec update needed; behavior is covered by code, config, docs, and tests.
- End-user/operator docs: updated.
- End-user/operator skills: pending outcome.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- None.

Project skills update:

- None.

End-user/operator docs update:

- Updated:
  - `src/crates/netflow-plugin/README.md`
  - `docs/network-flows/visualization/dashboard-cards.md`
  - `docs/network-flows/sizing-capacity.md`
  - `docs/network-flows/troubleshooting.md`
  - `docs/network-flows/validation.md`

End-user/operator skills update:

- None.

Lessons:

- Expensive absolute byte diagnostics are useful during investigation, but they should not be part of the normal one-second production path when lighter state-cardinality proxies answer the common operational question.

Follow-up mapping:

- Parent inventory SOW tracks ordering.

## Outcome

Completed. Default chart sampler work now stays on cheap count/cardinality signals, absolute byte diagnostics are opt-in, tests/docs cover the behavior, two external review rounds found no remaining blocker, and the final production-shaped low-rate benchmark measured about `3.03 usec/s`.

## Lessons Extracted

- Split production health signals from diagnostic introspection:
  - always-on charts should be cheap counters/cardinality;
  - expensive memory attribution should be opt-in and bounded by config.

## Follow-up Issues

None yet.
