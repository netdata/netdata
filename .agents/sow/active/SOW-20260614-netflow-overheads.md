# SOW-20260614-netflow-overheads - NetFlow Low-Rate Overhead

## Status

Status: completed

Sub-state: Autonomous child SOWs completed; draft PR pushed and CI evaluating.

## Requirements

### Purpose

Make `netflow-plugin` fit for production DevOps/SRE use at both low and high flow rates:

- preserve high-rate ingest throughput;
- preserve useful Flow Function facets and plugin self-observability;
- remove or sharply reduce fixed background overhead that makes low-rate deployments expensive;
- challenge every fixed-cost, periodic, duplicate, or hot-path operation that may
  waste resources; prior design intent is not proof that the cost is acceptable;
- prove the benchmark model matches long-lived production behavior, not only fresh-state throughput.

### User Request

The user asked to create a dedicated worktree at `<repo-root>` and add a SOW so the NetFlow overhead investigation and any later fixes can happen there.

### Assistant Understanding

Facts:

- The live workstation plugin was observed at about `2.7-3.4` UDP packets/s and about `28-36` raw journal entries/s while using about `6%` of one CPU core.
- The live process rewrote `/var/cache/netdata/flows/facet-state.bin` once per second.
- The facet state file was `12,123,806` bytes and repeated SHA256 samples were identical while the mtime advanced once per second.
- A 10 second sample showed about `121 MiB` of process write volume while raw flow logical writes were about `20-25 KiB/s`.
- Perf samples showed significant CPU in `rmp_serde` serialization, roaring bitmap iteration, `/proc/self/smaps` / `/proc/self/smaps_rollup`, `mallinfo2`, filesystem write, rename, `msync`, and `fdatasync`.
- The local `/etc/netdata/netflow.yaml` had `listener.sync_every_entries: 1024`; the stock/default contract is `0`.
- Existing benchmarks focus on max-throughput or paced fresh-state ingest and do not yet prove long-lived low-rate production overhead.

Inferences:

- The high-rate ingest benchmark can be true while low-rate production CPU remains bad, because low-rate cost is dominated by fixed housekeeping and persisted-state size.
- The likely largest issue is facet persistence writing unchanged durable state every second under steady traffic.
- The chart sampler's per-second memory telemetry is a second likely overhead source.
- The local `sync_every_entries` setting is an additional low-rate disk-sync tax, but probably not the main CPU source.

Unknowns:

- Exact CPU and I/O contribution of each bucket after controlled A/B testing.
- Whether the correct durable fix is change-only dirty tracking, persistence throttling, incremental persistence, a state hash/write skip, reduced facet state, or a combination.
- Whether plugin memory self-observability should be always-on, lower cadence, conditional, or split into cheap and expensive charts.
- Whether any benchmark should become a regression test, a manual benchmark, or both.

### Acceptance Criteria

- Produce a measured overhead budget for low-rate `netflow-plugin` operation, separating ingest, facet persistence, chart memory sampling, raw journal sync, tier commits, Tokio/plugin runtime, and disk I/O.
- Prove whether `facet-state.bin` rewrites are unnecessary when the serialized payload is unchanged.
- Prove whether chart memory sampling frequency and `/proc/self/smaps` reads materially contribute to steady CPU.
- Prove whether local `sync_every_entries` materially changes CPU, I/O, and latency at low flow rates.
- Add or update benchmarks so low-rate long-lived production overhead is measured, not only max-throughput fresh-state ingest.
- Present implementation options with risks and a recommendation before code changes.
- After user approval, implement the chosen fix with tests and validation.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/facet_runtime.rs`
- `src/crates/netflow-plugin/src/charts/runtime.rs`
- `src/crates/netflow-plugin/src/charts/process_maps.rs`
- `src/crates/netflow-plugin/src/memory_allocator.rs`
- `src/crates/netflow-plugin/src/ingest/service/runtime.rs`
- `src/crates/netflow-plugin/src/ingest_test_support.rs`
- `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs`
- `src/crates/netflow-plugin/src/ingest_bench_tests.rs`
- `src/crates/netflow-plugin/src/plugin_config/types/listener.rs`
- Live Netdata charts via `http://127.0.0.1:19999/api/v1/data`
- Live process evidence from `/proc/<pid>/io`, `/proc/<pid>/status`, `perf`, and file metadata under `/var/cache/netdata/flows/`

Current state:

- `observe_active_record()` computes whether published facet values changed, but calls `mark_dirty()` unconditionally:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:326`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:341`
- Dirty facet state serializes the full persisted state:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:546`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:978`
- Persistence writes a temporary file and renames it:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:992`
- The chart sampler runs once per second:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:100`
- The chart sampler reads `/proc/self/status`, `/proc/self/smaps_rollup`, `/proc/self/smaps`, and allocator state:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:250`
  - `src/crates/netflow-plugin/src/charts/process_maps.rs:112`
  - `src/crates/netflow-plugin/src/memory_allocator.rs:75`
- The ingest loop runs a sync tick on the configured interval:
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:11`
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:54`
- Raw journal sync happens when `sync_every_entries > 0` and enough records accumulated:
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:345`
  - `src/crates/netflow-plugin/src/plugin_config/types/listener.rs:12`
- Benchmark helpers set `sync_every_entries = usize::MAX` and `sync_interval = 1h`, which suppresses the local production-style 1s raw fsync path:
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:19`
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:45`
- The resource benchmark starts at 5k flows/s, so it does not model 20-30 flows/s:
  - `src/crates/netflow-plugin/src/ingest_resource_bench_tests.rs:43`

Risks:

- Reducing persistence too aggressively can lose facet autocomplete state after crashes or restarts.
- Reducing memory observability can make future memory growth incidents harder to diagnose.
- Optimizing only low-rate overhead can accidentally regress high-rate ingest or query-path facet UX.
- Benchmark fixes can give false confidence if they do not model long-lived state size and periodic background work.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- Working model: low-rate `netflow-plugin` overhead is dominated by background housekeeping, not by NetFlow decode or raw flow ingest.
- The strongest confirmed issue is repeated full facet-state serialization and rewrite of an unchanged 12 MiB payload once per second.
- Secondary confirmed cost is per-second memory telemetry reading expensive `/proc` data and allocator state.
- A local non-default raw journal fsync setting adds further disk sync work.
- The current benchmarks do not cover the low-rate, long-lived, large-facet-state production case.

Evidence reviewed:

- Live chart evidence: about `6%` CPU at about `28-36` raw journal entries/s.
- Live file evidence: `/var/cache/netdata/flows/facet-state.bin` mtime advanced once per second while SHA256 remained unchanged.
- Live process evidence: `/proc/<pid>/io` showed about `121 MiB` writes over 10 seconds during low-rate traffic.
- Perf evidence: samples in `rmp_serde`, roaring bitmap iteration, `/proc/self/smaps`, `/proc/self/smaps_rollup`, `mallinfo2`, write, rename, `msync`, and `fdatasync`.
- Code evidence listed in the Analysis section.

Affected contracts and surfaces:

- NetFlow Flow Function facet autocomplete and allowed facet values.
- Durable facet state file under the configured flows cache directory.
- NetFlow plugin internal charts, especially memory charts.
- Raw journal durability behavior controlled by `listener.sync_every_entries`.
- Benchmark commands and documented performance claims.
- Potential operator-facing documentation for overhead and tuning behavior.

Clean-end-state target:

- Provisional target: a measured and user-approved fix plan that reduces low-rate fixed overhead without weakening required facet UX, crash/restart recovery, or high-rate throughput.
- Removed as redundant (i): none yet; no implementation design has been approved.
- Excluded coupled items (ii): none yet; investigation must decide what is coupled before implementation.
- Reference search: required before implementation if any file path, persisted format, public config, chart identity, or Function contract changes.

Existing patterns to reuse:

- Existing `ingest_resource_bench_tests.rs` resource-envelope reporting.
- Existing `IngestMetrics` and chart machinery for exposing internal counters.
- Existing facet runtime unit tests around persistence, reconcile, and stale paths.
- Existing SOW spec `.agents/sow/specs/netflow-tier-commit-workers.md` for tier commit worker invariants.

Risk and blast radius:

- Medium performance risk: facet runtime and chart sampler affect every enabled NetFlow deployment.
- Medium data/UX risk: facet state powers user-facing Flow Function dropdowns/autocomplete.
- Low security risk if evidence remains aggregate and sanitized.
- Medium benchmark risk: a benchmark that misses production background tasks can hide regressions.

Sensitive data handling plan:

- Do not write raw flow values, IP addresses, customer identifiers, private endpoints, credentials, SNMP communities, bearer tokens, or proprietary traffic details to durable artifacts.
- Use aggregate counts, file sizes, hashes, field names, command names, and repo-relative code references.
- Redact any live traffic values if detailed samples become necessary.

Implementation plan:

1. Finish read-only decomposition of current overhead with controlled measurements and no source changes.
2. Design temporary A/B probes or local test toggles only after the user approves that measurement step.
3. Present implementation options for facet persistence, memory self-observability, raw fsync defaults/config, and benchmark coverage.
4. After user approval, implement the selected design with focused tests and benchmarks.
5. Validate low-rate overhead, high-rate throughput, restart recovery, and Flow Function facet behavior.

Validation plan:

- Baseline live CPU/I/O at low flow rate with current config.
- A/B test raw journal sync behavior with `sync_every_entries: 0` versus current local setting.
- A/B test facet persistence behavior using a temporary branch-local probe or benchmark harness.
- A/B test chart memory sampling cadence or expensive `/proc` sampling.
- Add a low-rate long-lived-state benchmark that starts from a realistic persisted facet state.
- Run targeted unit tests for facet persistence and chart sampler behavior.
- Run high-rate ingest/resource benchmarks to confirm no throughput regression.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected unless the investigation exposes a reusable collector-observability rule.
- Specs: update or add a NetFlow overhead/performance spec if a durable invariant is created.
- End-user/operator docs: likely update NetFlow README/config docs if `sync_every_entries`, memory charts, or persistence behavior changes.
- End-user/operator skills: no update expected unless NetFlow troubleshooting guidance changes.
- SOW lifecycle: active branch-local SOW; durable knowledge must move into tests, docs, specs, or skills before completion; active SOW must not merge to `master`.

Open-source reference evidence:

- Not checked yet. If implementation changes persistence, self-observability cadence, or benchmark methodology, compare with relevant open-source flow collectors and observability agents before final design.

Open decisions:

- No implementation design is approved.
- The next decision is whether to run controlled A/B probes using temporary branch-local changes or first deepen live read-only profiling.

## Implications And Decisions

1. User decision: create an isolated worktree and SOW.
   - Selected: branch `netflow-overheads` at `<repo-root>`.
   - Implication: future investigation and fixes happen away from the dirty main checkout.
   - Risk: active SOW must be removed from git before merge.
   - Recommendation classification: surgical.

2. Implementation-design decision: not selected yet for individual overhead
   buckets.
   - Candidate route A: surgical fixes for confirmed overheads only.
   - Candidate route B: long-term-best redesign where a bucket exposes a broader
     contract problem.
   - Current recommendation: decide per bucket after tests and measurements.
     The overall work strategy is recorded separately below.

3. User decision: challenge all potentially wasteful resource paths.
   - Selected: do not exempt any path merely because it appears intentional or is
     documented in a spec.
   - Implication: tier sync, chart sampling, decoder-state hydration,
     persistence, raw journal sync, enrichment, and query payload work all remain
     reviewable until measurements prove their cost is acceptable for the
     production purpose.
   - Risk: this broadens the investigation and may reveal larger design work
     before implementation can be approved.
   - Recommendation classification: long-term-best.

4. User decision: proceed with the long-term-best measured cleanup strategy.
   - Selected: Work Strategy A from the progress proposal: fix the
     benchmark/measurement model first, then address overhead buckets in impact
     order.
   - Ordered buckets: production-shaped benchmark/measurement harness, facet
     persistence, chart sampling/facet memory walks, tier sync, decoder duplicate
     work, query payloads, and enrichment hot-path costs.
   - Test-first requirement: before changing implementation behavior in any
     bucket, add or verify focused tests and edge cases that prove the current
     contract still works after the optimization.
   - Implication: implementation speed is secondary to preserving correctness,
     restart recovery, Flow Function UX, high-rate ingest, and benchmark
     credibility.
   - Risk: this may require substantial test harness work before the first
     runtime optimization lands.
   - Recommendation classification: long-term-best.

5. User decision: split implementation into autonomous SOWs.
   - Selected: create separate autonomous SOWs per improvement bucket instead of
     implementing all fixes under one umbrella SOW.
   - Implication: each SOW can be accepted, rejected, paused, or expanded
     independently, while this SOW remains the inventory/root-cause umbrella.
   - Risk: separate SOWs can drift unless each one keeps the shared production
     overhead purpose, test-first requirement, and benchmark evidence linked back
     to this inventory.
   - Recommendation classification: long-term-best.

## Plan

1. Maintain this SOW as the umbrella inventory and root-cause model.
2. Execute autonomous child SOWs in priority order, with each child carrying its
   own tests, implementation decision, validation, and artifact maintenance.
3. Reconcile child SOW outcomes back into this inventory before the umbrella is
   completed.

Autonomous child SOWs:

| Priority | SOW | Impact | Difficulty | Status |
|---:|---|---:|---:|---|
| 1 | `.agents/sow/active/SOW-20260614-netflow-benchmark-overhead-budget.md` | 5 | 3 | completed |
| 2 | `.agents/sow/active/SOW-20260614-netflow-facet-persistence.md` | 5 | 3 | completed |
| 3 | `.agents/sow/active/SOW-20260614-netflow-chart-sampler.md` | 4 | 3 | completed |
| 4 | `.agents/sow/active/SOW-20260614-netflow-facet-runtime-cardinality.md` | 4 | 4 | completed |
| 5 | `.agents/sow/active/SOW-20260614-netflow-tier-sync.md` | 3 | 4 | completed |
| 6 | `.agents/sow/active/SOW-20260614-netflow-decoder-packet-path.md` | 3 | 4 | completed |
| 7 | `.agents/sow/active/SOW-20260614-netflow-query-payload.md` | 3 | 3 | completed |
| 8 | `.agents/sow/active/SOW-20260614-netflow-enrichment-hot-path.md` | 3 | 4 | completed |
| 9 | `.agents/sow/active/SOW-20260614-netflow-raw-journal-sync.md` | 2 | 2 | completed |

## Execution Log

### 2026-06-14

- Created worktree `<repo-root>` on branch `netflow-overheads`.
- Added this planning SOW.
- Reviewed `src/crates/netflow-plugin` for fixed-cost or repeated work that can dominate low-rate deployments.
- Ran the requested six-reviewer external review round. Five reviewers completed
  and one reviewer timed out after the 30 minute command guard without a final
  review. Findings below include only items locally verified against code.
- Created nine autonomous child SOWs and linked them from this inventory SOW.
- No implementation files changed.

### 2026-06-15

- Completed priority 3 child SOW `.agents/sow/active/SOW-20260614-netflow-chart-sampler.md`.
- Outcome: default chart sampler keeps cheap count/cardinality signals and absolute memory byte diagnostics are opt-in via `charts.memory_diagnostics`.
- Validation: two external review rounds found no remaining blocker; final production-shaped low-rate benchmark measured chart sampler cost at about `3.03 usec/s`.
- Completed priority 4 child SOW `.agents/sow/active/SOW-20260614-netflow-facet-runtime-cardinality.md`.
- Outcome: facet runtime overhead was reduced while preserving Flow Function behavior.
- Completed priority 5 child SOW `.agents/sow/active/SOW-20260614-netflow-tier-sync.md`.
- Outcome: tier sync overhead was reduced and measured with focused sync buckets.
- Completed priority 6 child SOW `.agents/sow/active/SOW-20260614-netflow-decoder-packet-path.md`.
- Outcome: decoder packet path overhead was reduced without changing decode contracts.
- Completed priority 7 child SOW `.agents/sow/active/SOW-20260614-netflow-query-payload.md`.
- Outcome: Flow Function query payload overhead was reduced while preserving payload semantics.
- Completed priority 8 child SOW `.agents/sow/active/SOW-20260614-netflow-enrichment-hot-path.md`.
- Outcome: network source runtime now uses indexed IPv4 lookups for high-cardinality source data.
- Completed priority 9 child SOW `.agents/sow/active/SOW-20260614-netflow-raw-journal-sync.md`.
- Outcome: stock raw-journal sync defaults were verified as already reasonable; benchmark tooling now measures explicit non-default listener sync settings.

## Validation

Acceptance criteria evidence:

- Child SOWs 1-9 are completed.
- Each source-changing child SOW recorded focused tests before or with implementation, validation, and external-review results.
- The draft PR branch is pushed at `121d47501c`.

Tests or equivalent validation:

- See each completed child SOW for targeted command output and external-review findings.

Real-use evidence:

- Initial live evidence recorded in Analysis and Pre-Implementation Gate.
- Child SOWs record benchmark evidence for production-shaped low-rate behavior and the measured effects of the optimized buckets.

Reviewer findings:

- Confirmed: facet active-observation paths mark persisted state dirty even when
  the published facet contribution did not change:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:304`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:318`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:326`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:341`
  - Impact: any steady traffic can force full persisted-state serialization and
    temp-file rewrite on the next persistence tick.
- Confirmed: facet persistence serializes and writes the full persisted state
  when dirty:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:433`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:534`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:978`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:992`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:1026`
  - Impact: the dirty-bit bug scales with total stored facet state, not with new
    flow volume.
- Confirmed: chart sampling does expensive process memory collection every
  second:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:90`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:112`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:250`
  - `src/crates/netflow-plugin/src/charts/process_maps.rs:112`
  - `src/crates/netflow-plugin/src/memory_allocator.rs:65`
  - `src/crates/netflow-plugin/src/memory_allocator.rs:75`
  - Impact: `/proc/self/smaps`, `/proc/self/smaps_rollup`, allocator sampling,
    and symbol lookup are fixed per-second costs independent of flow rate.
- Strong candidate: v9/IPFIX packets are scanned for decoder-state persistence
  before the normal parser decodes the same payload:
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:137`
  - `src/crates/netflow-plugin/src/decoder/state/runtime/decode.rs:20`
  - `src/crates/netflow-plugin/src/decoder/state/runtime/observe.rs:13`
  - `src/crates/netflow-plugin/src/decoder/protocol/v9/templates.rs:19`
  - `src/crates/netflow-plugin/src/decoder/protocol/ipfix/templates/entry.rs:17`
  - `src/crates/netflow-plugin/src/decoder/protocol/entry.rs:95`
  - Impact: template/state observation likely duplicates parse work for every
    v9/IPFIX packet. This needs measurement before changing because it protects
    decoder restart/hydration behavior.
- Strong candidate: special datalink decoding is enabled by global protocol
  presence, not by the current packet's exporter/domain/template:
  - `src/crates/netflow-plugin/src/decoder/protocol/entry.rs:66`
  - `src/crates/netflow-plugin/src/decoder/state/sampling/templates.rs:125`
  - `src/crates/netflow-plugin/src/decoder/protocol/v9/special.rs:3`
  - `src/crates/netflow-plugin/src/decoder/protocol/ipfix/special/packet.rs:3`
  - Impact: once any datalink template exists, unrelated packets of that
    protocol can pay an extra raw-set scan before normal decode.
- Low-risk candidate: decoder-scope chart state is recomputed after every
  packet, although the values change only when decoder namespaces or hydrated
  sources change:
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:143`
  - `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs:4`
  - `src/crates/netflow-plugin/src/ingest/metrics.rs:87`
  - Impact: small fixed overhead per packet; likely lower priority than
    persistence, chart sampling, and decoder duplicate parsing.
- Conditional candidate: remote network-source enrichment uses a linear scan
  over all published source prefixes for each resolved address:
  - `src/crates/netflow-plugin/src/network_sources/runtime.rs:24`
  - `src/crates/netflow-plugin/src/enrichment/resolve.rs:18`
  - `src/crates/netflow-plugin/src/enrichment/apply.rs:51`
  - Contrast pattern: static and dynamic routing already use prefix maps/tries:
    `src/crates/netflow-plugin/src/enrichment/data/prefix.rs:10`,
    `src/crates/netflow-plugin/src/routing/runtime.rs:171`.
  - Impact: hot-path cost can grow with configured remote network-source size.
- Challenged candidate: the tier sync tick prunes and snapshots open-tier state
  once per second:
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:54`
  - `src/crates/netflow-plugin/src/ingest/service/tiers.rs:109`
  - `src/crates/netflow-plugin/src/ingest/service/tiers.rs:125`
  - `src/crates/netflow-plugin/src/tiering/index/accumulator.rs:128`
  - `src/crates/netflow-plugin/src/tiering/index/accumulator.rs:147`
  - Impact: this is O(open rollup cardinality). The existing spec records the
    design intent, but that does not prove the resource cost is acceptable. This
    path must be included in the low-rate and high-cardinality overhead budget.
- External-review addition, confirmed: the benchmark gap is stronger than the
  initial SOW framing. Production starts the chart sampler, but the benchmark
  helpers construct only an `IngestService`, set `sync_every_entries` to
  `usize::MAX`, and set `sync_interval` to one hour:
  - `src/crates/netflow-plugin/src/main.rs:190`
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:19`
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:26`
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:45`
  - `src/crates/netflow-plugin/src/ingest_test_support.rs:52`
  - Impact: a new low-rate benchmark is still insufficient unless it runs
    production-equivalent background work, including chart sampling and the real
    sync tick path.
- External-review addition, confirmed: chart sampling performs an O(facet-state)
  memory walk under the facet-state mutex every second, separate from `/proc`
  sampling:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:112`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:117`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:158`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:164`
  - `src/crates/netflow-plugin/src/facet_runtime/store.rs:333`
  - `src/crates/netflow-plugin/src/facet_runtime/store.rs:338`
  - Impact: the sampler can contend with ingest on `FacetState` while walking
    archived fields, active contributions, published values, archived paths, and
    roaring stores.
- External-review addition, confirmed: `current_allocator_memory()` performs
  symbol lookup before allocator sampling:
  - `src/crates/netflow-plugin/src/memory_allocator.rs:65`
  - `src/crates/netflow-plugin/src/memory_allocator.rs:75`
  - Impact: the lookup result is process-stable and should be challenged as
    avoidable fixed work even if the measured cost is small.
- External-review addition, confirmed: publishing facet snapshots deep-clones
  the whole published snapshot on each publish:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:952`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:953`
  - Called from active contribution / record paths already listed above.
  - Impact: legitimate new facet values can trigger O(published facet
    cardinality) cloning, independent of packet size.
- External-review addition, confirmed: rebuilding published facet fields clones
  archived stores, merges active stores, collects strings, and allocates field
  names:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:727`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:734`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:738`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:748`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:751`
  - Impact: reconcile, rotation, or deleted-path rebuilds can spike CPU and
    allocation with facet cardinality.
- External-review addition, confirmed: text facet collection sorts on each full
  collection, including persistence/rebuild paths:
  - `src/crates/netflow-plugin/src/facet_runtime/store.rs:292`
  - `src/crates/netflow-plugin/src/facet_runtime/store.rs:294`
  - `src/crates/netflow-plugin/src/facet_runtime/store.rs:500`
  - `src/crates/netflow-plugin/src/facet_runtime/store.rs:504`
  - Impact: even after the dirty-bit bug is fixed, every legitimate full
    persistence of high-cardinality text facets pays O(n log n) sorting.
- External-review addition, confirmed: `build_reconcile_plan()` allocates an
  active-path `BTreeSet` that is never read:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:175`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:181`
  - Impact: pure dead work on every facet reconcile plan build.
- External-review addition, confirmed: each new active facet value checks all
  other active contributions before insertion:
  - `src/crates/netflow-plugin/src/facet_runtime.rs:836`
  - `src/crates/netflow-plugin/src/facet_runtime.rs:1082`
  - Impact: O(active contribution files) work per new value when more than one
    active contribution exists.
- External-review addition, confirmed: Flow Function facet vocabulary payloads
  clone and sort full published value vectors before truncation:
  - `src/crates/netflow-plugin/src/query/facets/cache/payload.rs:11`
  - `src/crates/netflow-plugin/src/query/facets/cache/payload.rs:13`
  - `src/crates/netflow-plugin/src/query/facets/cache/payload.rs:19`
  - `src/crates/netflow-plugin/src/query/facets/cache/payload.rs:20`
  - Impact: query/dashboard refresh cost can scale with full facet value
    cardinality even when the response returns only a limited subset.
- External-review addition, corrected scope: `IngestMetrics::snapshot()` creates
  a `HashMap<String, u64>` and allocates fixed string keys, but it is used by API
  responses and benchmarks, not by the 1s chart sampler:
  - `src/crates/netflow-plugin/src/ingest/metrics.rs:100`
  - `src/crates/netflow-plugin/src/ingest/metrics.rs:103`
  - `src/crates/netflow-plugin/src/api/flows/handler.rs:59`
  - `src/crates/netflow-plugin/src/api/flows/handler.rs:101`
  - `src/crates/netflow-plugin/src/api/flows/handler.rs:147`
  - Impact: this is query/stats-path allocation waste, not background idle
    overhead.
- External-review addition, confirmed: decoder namespace bookkeeping allocates a
  string key and parses template scope on packet paths:
  - `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs:18`
  - `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs:22`
  - `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs:24`
  - `src/crates/netflow-plugin/src/ingest/service/runtime.rs:137`
  - `src/crates/netflow-plugin/src/decoder/state/runtime/decode.rs:20`
  - Impact: this is another concrete part of the duplicate v9/IPFIX state
    observation path and should be measured with that bucket.
- External-review addition, confirmed: enrichment has per-flow allocation and
  clone paths beyond the remote prefix linear scan:
  - `src/crates/netflow-plugin/src/enrichment/apply.rs:55`
  - `src/crates/netflow-plugin/src/enrichment/apply.rs:56`
  - `src/crates/netflow-plugin/src/enrichment/data/network/write.rs:93`
  - `src/crates/netflow-plugin/src/enrichment/data/network/write.rs:98`
  - `src/crates/netflow-plugin/src/enrichment/data/network/write.rs:110`
  - `src/crates/netflow-plugin/src/enrichment/data/network/write.rs:115`
  - Impact: high-rate deployments can pay avoidable exporter-IP string
    allocation and repeated network-attribute string clones per flow.
- External-review addition, conditional: classifier caches take mutexes and may
  prune while holding them when classifiers are configured:
  - `src/crates/netflow-plugin/src/enrichment/classify.rs:25`
  - `src/crates/netflow-plugin/src/enrichment/classify.rs:29`
  - `src/crates/netflow-plugin/src/enrichment/classify.rs:65`
  - `src/crates/netflow-plugin/src/enrichment/classify.rs:70`
  - Impact: not necessarily active in all deployments, but should be challenged
    in high-rate enriched deployments.
- External-review addition, confirmed detail for tier sync: open-tier refresh
  allocates row vectors and prune scans active flow refs before taking a write
  lock:
  - `src/crates/netflow-plugin/src/tiering/index/accumulator.rs:128`
  - `src/crates/netflow-plugin/src/tiering/index/accumulator.rs:136`
  - `src/crates/netflow-plugin/src/tiering/index/accumulator.rs:147`
  - `src/crates/netflow-plugin/src/tiering/index/accumulator.rs:150`
  - `src/crates/netflow-plugin/src/ingest/service/tiers.rs:109`
  - `src/crates/netflow-plugin/src/ingest/service/tiers.rs:120`
  - `src/crates/netflow-plugin/src/ingest/service/tiers.rs:125`
  - `src/crates/netflow-plugin/src/ingest/service/tiers.rs:140`
  - Impact: the tier-sync bucket should be decomposed into open-row snapshot
    allocation, active-hour scan, in-flight-hour scan, and write-lock hold time.
- External-review addition, low-priority confirmed: chart application pushes all
  chart updates every second, regardless of whether values changed:
  - `src/crates/netflow-plugin/src/charts/runtime.rs:127`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:128`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:148`
  - `src/crates/netflow-plugin/src/charts/runtime.rs:155`
  - Impact: likely smaller than the data-gathering cost, but still part of the
    fixed 1s background budget.

Same-failure scan:

- Completed initial scan for fixed background work, duplicate decode passes, and
  hot-path linear scans inside `src/crates/netflow-plugin`.
- Updated review rule after user decision: no path is exempt from challenge only
  because it was intentionally designed or documented.
- External-review synthesis added additional confirmed candidates covering:
  production-mismatched benchmarks, facet-state memory walks, facet snapshot
  cloning/rebuilds, query facet vocabulary cloning, enrichment allocations,
  decoder namespace allocation, classifier cache locks, and detailed tier-sync
  sub-buckets.
- Remaining scan before implementation: check generated/public chart names,
  Flow Function payload contracts, persisted decoder-state format, and docs if a
  proposed fix changes any contract.

Sensitive data gate:

- Current SOW records only aggregate performance evidence, file sizes, hashes, field names, command classes, and repo-relative code paths.

## Artifact Maintenance Gate

- AGENTS.md: no update needed.
- Runtime project skills: no update needed.
- Specs: no update needed.
- End-user/operator docs: no update needed beyond the committed benchmark README updates.
- End-user/operator skills: no update needed.
- SOW lifecycle: active SOW exists only for this branch-local work and must not merge to `master`.

Specs update:

- None.

Project skills update:

- None.

End-user/operator docs update:

- Benchmark README updates are committed with the relevant child SOWs.

End-user/operator skills update:

- None.

Lessons:

- Test-first autonomous child SOWs were useful here because several initial suspects turned out to be measurement or configuration issues rather than production-default bugs.

Follow-up mapping:

- Remaining optional polish from reviewers is not blocking this PR:
  - Treat empty benchmark env vars as absent instead of invalid if benchmark CLI friendliness becomes important.
  - Add benchmark print-format smoke tests if human report wording becomes a stable contract.
  - Complete the pre-existing README env-var list for pool-size benchmark knobs.

## Outcome

All nine autonomous child SOWs were completed and pushed to draft PR `#22719`.

## Lessons Extracted

- Measure before changing production defaults. The raw-journal-sync SOW showed the stock default was already the reasonable behavior and only benchmark tooling needed improvement.
- Keep expensive diagnostics configurable. The chart sampler SOW preserved lightweight real-time proxies while keeping absolute memory byte diagnostics opt-in.
- Separate autonomous SOWs kept each fix independently testable and allowed reviewers to challenge unrelated costs without expanding a single implementation scope.

## Follow-up Issues

None yet.
