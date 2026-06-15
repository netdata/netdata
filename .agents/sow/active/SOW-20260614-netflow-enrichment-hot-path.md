# SOW-20260614-netflow-enrichment-hot-path - NetFlow Enrichment Hot-Path Costs

## Status

Status: completed

Sub-state: Remote network-source prefix lookup optimized, validated, and externally reviewed with no blockers.

## Requirements

### Purpose

Reduce avoidable NetFlow enrichment hot-path work while preserving metadata, routing, network-source, GeoIP, sampling, and classifier semantics.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers enrichment hot-path costs.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- Runtime network sources use a linear scan over records per address.
- Static/dynamic routing paths already use prefix-map/trie-like structures.
- Enrichment allocates exporter IP strings and clones network attributes into flow records.
- Classifier caches use mutexes and may prune while holding locks when classifiers are configured.

Inferences:

- This bucket is workload-dependent but can matter at high flow rates or many network-source prefixes.

Unknowns:

- Typical configured network-source cardinality and classifier usage.

### Acceptance Criteria

- Add or verify tests for network source precedence/merge semantics.
- Add or verify tests for classifier cache behavior if touched.
- Reduce hot-path linear scans/allocation where tests prove equivalent behavior.
- Preserve enrichment output fields exactly unless a user-visible contract decision is approved.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/network_sources/runtime.rs`
- `src/crates/netflow-plugin/src/enrichment/resolve.rs`
- `src/crates/netflow-plugin/src/enrichment/apply.rs`
- `src/crates/netflow-plugin/src/enrichment/data/network/write.rs`
- `src/crates/netflow-plugin/src/enrichment/classify.rs`
- `src/crates/netflow-plugin/src/network_sources/service.rs`
- `src/crates/netflow-plugin/src/enrichment/data/prefix.rs`
- `src/crates/netflow-plugin/src/enrichment/tests.rs`
- `src/crates/netflow-plugin/src/network_sources/tests.rs`
- Parent inventory SOW.
- `ipnet-trie` 0.3.0 local crate source.
- Akvorado network dictionary and routing lookup reference.

Current state:

- Parent inventory records exact enrichment evidence.
- `NetworkSourcesRuntime` stores a raw `Vec<NetworkSourceRecord>` under an `RwLock`.
- `NetworkSourcesRuntime::matching_attributes_ascending()` scans every remote source record for each address, clones all matching attributes, then sorts matches by prefix length.
- `FlowEnricher::resolve_network_attributes()` requires all matching runtime prefixes in ascending prefix length order so less-specific attributes are merged before more-specific attributes.
- At the same prefix length, runtime network-source attributes must merge before static config attributes.
- `publish_source_records()` merges source records in `BTreeMap` source-name order, preserving per-source record order. The current stable sort means same-prefix-length runtime records merge in that merged order.
- The existing static `PrefixMap` uses `ipnet-trie` for longest-prefix match, but its multi-match walk still filters a sorted vector.
- `ipnet_trie::matches()` cannot be used directly for this contract: a local check with prefixes `10.0.0.0/8`, `10.1.0.0/16`, and `10.2.0.0/16` returned the sibling `10.2.0.0/16` for host `10.1.1.1`.

Risks:

- Network source merge order and prefix precedence are correctness-sensitive.
- Classifier cache changes can alter reject/accept behavior if keys are not preserved.
- Non-canonical prefixes are currently accepted by `ipnet` parsing and `IpNet::contains()` still treats them according to their mask, so an index must canonicalize lookup keys without narrowing accepted input behavior.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Remote network-source enrichment contains conditional but real hot-path work that scales with flow rate and configured remote prefix count.
- The root cause selected for this SOW step is the raw vector lookup in `NetworkSourcesRuntime::matching_attributes_ascending()`: every enriched source/destination address scans all refreshed remote source records under a read lock.
- Classifier caches and exporter-name string allocation remain candidates, but they are not the first clean fix because the remote source lookup has clearer evidence, simpler semantics, and existing indexed-prefix patterns nearby.

Evidence reviewed:

- Parent inventory SOW reviewer findings.
- `src/crates/netflow-plugin/src/network_sources/runtime.rs:13-39` - raw vector storage, full scan, clone matching attributes, stable sort by prefix length.
- `src/crates/netflow-plugin/src/enrichment/resolve.rs:25-66` - resolver consumes runtime matches in ascending prefix length and interleaves runtime before static config at equal prefix length.
- `src/crates/netflow-plugin/src/network_sources/service.rs:123-131` - source refreshes merge records in `BTreeMap` source-name order before publishing to the runtime.
- `src/crates/netflow-plugin/src/enrichment/data/prefix.rs:49-106` - existing local prefix abstraction already uses `IpnetTrie` for longest-prefix match while preserving ordered multi-match walks.
- `src/crates/netflow-plugin/src/enrichment/tests.rs:1965` - existing test covers same-prefix runtime/static merge, but not multiple runtime prefix lengths, sibling exclusion, same-length runtime order, or non-canonical prefix behavior.
- `ipnet-trie` 0.3.0 `src/lib.rs:338-365` - crate exposes `matches()` but it returns children under the shortest prefix match, which is not equivalent to "all prefixes containing this host" for this use case.

Affected contracts and surfaces:

- Enriched flow fields.
- Network-source configuration behavior.
- Classifier behavior and cache semantics.
- High-rate ingest performance.

Clean-end-state target:

- Remote network-source runtime lookup avoids unbounded O(total remote records) full scans when the refreshed record count exceeds the address-family prefix-probe budget.
- Small source lists keep the old linear path because scanning at most 33 IPv4 or 129 IPv6 records is cheaper and simpler than forcing trie probes for every tiny configuration.
- Large source lists use an index built when refreshed records are published.
- Matching semantics stay equivalent:
  - all matching runtime prefixes are applied least-specific to most-specific;
  - runtime attributes apply before static config at the same prefix length;
  - same-prefix-length runtime records keep current publish order;
  - sibling prefixes that do not contain the address are not merged;
  - non-canonical remote prefixes retain existing `IpNet::contains()` behavior through canonical index keys.
- Removed/reduced as redundant (i): unbounded raw full-vector scan for remote network-source lookup; tiny bounded scans remain intentionally as the cheaper path.
- Excluded coupled items (ii): classifier cache pruning, exporter-name string allocation, and generic `PrefixMap::matching_entries_ascending()` scan remain outside this step because changing them touches separate semantics and/or lower-priority hot paths; query payload costs belong to query SOW; decoder exporter namespace strings belong to decoder SOW.
- Reference search: no config schema, field names, classifier semantics, response fields, or public contracts are being changed. Search covered `NetworkSourcesRuntime`, `matching_attributes_ascending`, `replace_records`, and `network_sources_runtime` references.

Existing patterns to reuse:

- Existing `PrefixMap`/`IpnetTrie` patterns in static/dynamic routing.
- Existing enrichment tests.
- Existing `NetworkSourcesRuntime::replace_records()` refresh boundary.
- Existing `IpNet::trunc()` canonicalization from the pinned `ipnet` crate for exact lookup keys.

Risk and blast radius:

- Medium correctness risk for metadata attribution.
- Medium performance risk at high rates if over-engineered structures add refresh cost.
- Low public-contract risk because the planned change is internal and should preserve all enriched fields and config behavior.
- Refresh-time cost can increase slightly because the index is rebuilt on every successful remote source publish; this is acceptable only if lookup-time work is reduced and tests prove semantics.

Sensitive data handling plan:

- Use synthetic private/example networks and metadata in tests. Do not record customer IPs or identifiers.

Implementation plan:

1. Add tests that pin remote network-source sibling exclusion, least-specific to most-specific merge order, same-prefix-length publish order, and non-canonical prefix compatibility.
2. Run the targeted tests before implementation to verify the current contract.
3. Replace `NetworkSourcesRuntime` raw Vec-only storage with a refresh-built prefix index while retaining the old linear path for tiny source lists where it is cheaper than fixed prefix probes.
4. Preserve the public `NetworkSourceRecord` and `matching_attributes_ascending()` contracts so resolver code and tests remain focused.
5. Run targeted network-source/enrichment tests, then full `netflow-plugin` tests.

Validation plan:

- Targeted enrichment tests.
- Production-shaped benchmark with enrichment enabled if feasible.
- Targeted network-source runtime tests.
- `git diff --check`.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if enrichment lookup invariants become durable.
- End-user/operator docs: update if config behavior changes.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- Akvorado builds network metadata through a subnet map rather than per-flow raw list scans:
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 orchestrator/clickhouse/networks.go:31`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 orchestrator/clickhouse/networks.go:64`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 orchestrator/clickhouse/networks.go:108`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 orchestrator/clickhouse/networks.go:113`
- Akvorado BMP route lookup uses route/RIB lookup instead of scanning all routes:
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 outlet/routing/provider/bmp/lookup.go:24`
  - `akvorado/akvorado @ eedeef7ec6dc22da9f6e788fd82fb8396983d7e9 outlet/routing/provider/bmp/lookup.go:32`

Open decisions:

- None for the selected internal remote network-source lookup optimization. Any config/schema/field/classifier behavior change remains a user-owned decision and must pause implementation.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

## Plan

1. Test audit.
2. Edge-case tests.
3. Measurement/design.
4. Implementation and validation.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Activated this SOW after completing and pushing the query payload SOW.
- Selected the remote network-source lookup as the first enrichment hot-path fix because it has direct O(remote-prefix-count) per-address evidence and a clear indexed replacement.
- Verified `ipnet_trie::matches()` is not sufficient for this exact contract because it can return sibling prefixes under the shortest matching parent.
- Added tests before implementation for sibling exclusion, indexed-path semantics, same-prefix publish order, non-canonical prefix compatibility, and resolver-level runtime/static merge order.
- Implemented a hybrid runtime lookup:
  - records are still retained in publish order for tiny-list and same-prefix-order semantics;
  - a canonical-prefix `HashMap<IpNet, Vec<usize>>` exact index is rebuilt on `replace_records()`;
  - lookups use the original direct linear scan for same-family source lists below the measured cutoff;
  - lookups scan only matching-family records for mixed-family source lists below the measured cutoff;
  - lookups use exact canonical prefix probes for every possible prefix length when the source count is large enough to beat scanning.
- Initial external review found real gaps:
  - indexed-path tests did not cover every edge;
  - no IPv6 indexed-path coverage existed;
  - resolver-level coverage still used the linear path;
  - the first fixed-probe-count threshold was unmeasured and regressed small/medium cases;
  - using a trie as an exact-prefix map created avoidable rebuild and lookup overhead.
- Addressed review findings:
  - replaced the trie exact lookup with a canonical `HashMap` index and `entry()` rebuild;
  - changed the result prefix length to come from `record.prefix.prefix_len()`;
  - replaced unreachable `Result` continuation with an explicit bounded-prefix `expect()`;
  - added indexed IPv4, indexed IPv6, indexed non-canonical, default-route, stale-replacement, same-prefix-order, and resolver-level indexed-path tests;
  - added an ignored manual lookup benchmark and calibrated conservative cutoffs at 500 IPv4 family records and 2,000 IPv6 family records.
- Addressed final external-review coverage findings:
  - added mixed-family below-cutoff coverage for the family-indexed linear path;
  - added IPv6 indexed default-route coverage;
  - added IPv6 indexed non-canonical prefix coverage;
  - added explicit ascending-prefix-order assertions for runtime lookup tests;
  - extended the manual benchmark with mixed-family below-cutoff and indexed cases.
- Addressed final reviewer cleanup findings:
  - replaced bounded hot-path `IpNet::new(...).expect(...)` with `IpNet::new_assert(...)`;
  - tied indexed test filler counts to the runtime threshold constants with headroom;
  - kept small mixed-family configs on the direct linear path after benchmark evidence showed family-index indirection can regress tiny mixed inputs;
  - retained the family-indexed linear path only once the total record count is large enough to justify skipping cross-family records.

## Validation

Acceptance criteria evidence:

- Network source precedence/merge semantics are covered by existing `network_sources_runtime_enrichment_merges_with_static_networks` and new `network_sources_runtime_enrichment_preserves_prefix_and_publish_order`.
- Runtime lookup semantics are covered by tests for sibling exclusion, indexed lookup, IPv6 indexed lookup, same-prefix publish order, non-canonical prefix compatibility, default-route matching, stale-index replacement, and resolver-level indexed merge order.
- Classifier cache behavior was not touched; existing classifier cache tests still pass in the enrichment suite.
- Enrichment output fields are preserved; the resolver contract and public configuration are unchanged.

Tests or equivalent validation:

- Passed before implementation:
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml network_sources_runtime_ -- --nocapture`
  - Result: 5 passed before the later external-review test expansion.
- Passed after implementation:
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml network_sources_runtime_ -- --nocapture`
  - Result: 16 passed, 1 ignored.
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml network_sources::tests:: -- --nocapture`
  - Result: 30 passed, 1 ignored.
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml enrichment::tests:: -- --nocapture`
  - Result: 87 passed.
  - `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml -- --nocapture`
  - Result: 546 passed, 26 ignored; `grpc_build` 1 passed.
  - `git diff --check`
  - Result: passed.
- Manual benchmark:
  - Command: `cargo test --release -p netflow-plugin --manifest-path src/crates/Cargo.toml network_sources::tests::bench_network_sources_runtime_lookup_matrix -- --ignored --nocapture`
  - Result: passed.
  - Benchmark note: nanosecond-level values varied materially between local runs under workstation load, so the accepted evidence is path behavior and order-of-magnitude scale behavior, not a single fixed timing table.
  - Final representative release run after reviewer-driven cleanup:
    - `ipv4 32`: runtime 192.0 ns, linear 184.5 ns, 0.96x.
    - `ipv4 33`: runtime 338.9 ns, linear 293.8 ns, 0.87x.
    - `ipv4 34`: runtime 204.5 ns, linear 210.9 ns, 1.03x.
    - `ipv4 128`: runtime 419.1 ns, linear 412.9 ns, 0.99x.
    - `ipv4 129`: runtime 425.4 ns, linear 422.6 ns, 0.99x.
    - `ipv4 500`: runtime 764.1 ns, linear 1303.6 ns, 1.71x.
    - `ipv4 2000`: runtime 481.0 ns, linear 5923.7 ns, 12.32x.
    - `ipv4 10000`: runtime 648.7 ns, linear 38590.3 ns, 59.49x.
    - `ipv6 128`: runtime 641.6 ns, linear 567.1 ns, 0.88x.
    - `ipv6 129`: runtime 1022.8 ns, linear 1202.9 ns, 1.18x.
    - `ipv6 130`: runtime 721.7 ns, linear 779.0 ns, 1.08x.
    - `ipv6 500`: runtime 1831.1 ns, linear 1827.5 ns, 1.00x.
    - `ipv6 2000`: runtime 2275.7 ns, linear 6819.6 ns, 3.00x.
    - `ipv6 10000`: runtime 2354.4 ns, linear 34532.7 ns, 14.67x.
    - `mixed-ipv4-linear 128/128`: runtime 609.8 ns, linear 589.5 ns, 0.97x.
    - `mixed-ipv6-linear 128/128`: runtime 778.0 ns, linear 771.2 ns, 0.99x.
    - `mixed-ipv4-indexed 500/500`: runtime 793.5 ns, linear 2771.9 ns, 3.49x.
    - `mixed-ipv6-indexed 500/2000`: runtime 2242.1 ns, linear 7710.3 ns, 3.44x.

Real-use evidence:

- Akvorado reference evidence shows comparable flow network metadata paths use subnet-map/RIB lookup structures rather than per-flow raw list scans.

Reviewer findings:

- Parent inventory SOW findings apply.
- First enrichment review round:
  - Kimi: requested more indexed-path edge tests, use `record.prefix.prefix_len()` in indexed results, remove dead `IpNet::new()` error branch, and avoid `remove()` + `insert()` rebuild overhead.
  - GLM/minimax review: found the first trie-backed implementation correctness-preserving but not production-grade because it lacked performance evidence, had likely bad IPv6 threshold behavior, and missed IPv6/resolver indexed-path tests.
  - Fixes applied before re-review: canonical `HashMap` exact-prefix index, conservative measured cutoffs, expanded indexed tests, resolver-level indexed test, stale replacement test, default-route test, and release benchmark.
- Second enrichment review round:
  - Common reviewer findings: mixed-family linear-index path was untested; IPv6 default-route and IPv6 non-canonical indexed cases were not covered; indexed tests should assert ascending prefix order explicitly; the benchmark should include mixed-family cases.
  - Fixes applied before final re-review: added the missing tests, added ascending-order assertions, and extended the benchmark matrix.
- Third enrichment review round:
  - GLM, Mimo, and DeepSeek found no blockers.
  - Qwen found no blocker but identified microbenchmark wording, threshold-fragile test fillers, and bounded hot-path prefix construction as cleanup opportunities.
  - Follow-up measurement showed tiny mixed-family inputs could regress with family-index indirection, so the dispatch was changed to keep direct linear scanning for small mixed configs.
- Final enrichment review round after cleanup:
  - GLM, Minimax, Kimi, Mimo, DeepSeek, and Qwen found no blockers.
  - Remaining comments were low-severity or procedural: optional IPv6 resolver-level indexed coverage, SOW metadata cleanup, and pre-existing lock-poison behavior.

Same-failure scan:

- `rg` reference scan covered `NetworkSourcesRuntime`, `matching_attributes_ascending`, `replace_records`, `network_sources_runtime`, `IpnetTrie`, `HashMap`, and `matching_entries_ascending`.
- No public config, field-name, classifier, or response contract references needed migration because no public contract was replaced.

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

- Not required. No public config, response schema, field name, or durable operator contract changed.

Project skills update:

- Not required.

End-user/operator docs update:

- Not required. Behavior is contract-preserving.

End-user/operator skills update:

- Not required.

Lessons:

- `ipnet_trie::matches()` is broader than this use case because it can return sibling prefixes under the shortest matching parent; exact canonical prefix probes are safer for "all containing prefixes" semantics.
- A pure index can be worse for tiny source lists; choose the cheaper of bounded scan versus address-family prefix probes.

Follow-up mapping:

- Parent inventory SOW tracks ordering.

## Outcome

Completed. Remote network-source enrichment lookup now avoids scanning every configured remote network-source record for large source lists, while preserving existing prefix precedence, same-prefix publish order, non-canonical prefix compatibility, default-route behavior, and runtime/static merge semantics.

## Lessons Extracted

Recorded above under `Lessons`.

## Follow-up Issues

None required for this SOW.
