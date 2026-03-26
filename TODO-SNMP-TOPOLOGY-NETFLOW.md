## 0) Mandatory Install/Reinstall Order (Costa)
- Backend first (Netdata agent + plugins): `cd ~/src/netdata-ktsaou.git && ./install.sh`
- This also installs the default frontend.
- Frontend second (modified UI): `cd ~/src/dashboard/cloud-frontend && sudo ./agent.sh`
- Always run frontend installation only after backend installation completes.

# YOU MUST READ THE WHOLE OF THIS TODO FILE, AND YOU MUST ALWAYS UPDATE IT WITH YOUR PROGRESS - TRY TO KEEP THE TODO FILE AT A MANAGEABLE SIZE, OVERWRITING PREVIOUSLY PENDING ITEMS WITH THEIR COMPLETION AND VERIFICATION. YOU MUST READ THE TODO IN WHOLE, AFTER EACH COMPACTION OF YOUR MEMORY.
# Feature Analysis: SNMP Topology Map & NetFlow/IPFIX/sFlow

## Contract (Costa, 2026-02-03)

1. These are new modules. We will touch some existing code, but the majority of the work is new modules.
2. I expect that will work autonomously until you finish the work completely, delivering 100% production quality code.
3. To assist you, you can run claude, codex, gemini and glm-4.7, as many times as required, for code reviews, for assistance in any dilemma. I grant this permission to you for this TODO, overriding ~/.AGENTS.md rule that prevents it.
4. I expect the new code to have clear separation of concerns, clarity and simplicity. You may perform as many code reviews as necessary to understand existing patterns.
5. Testing is 100% required. Anything that is not being tested cannot be trusted.
6. TESTING MUST BE DONE WITH REAL DEVICE DATA, AVAILABLE FROM THE MULTIPLE SOURCES DEFINED BELOW. TESTING WITH HYPOTHETICAL GENERATED DATA IS NOT ACCEPTED AS TESTING.
7. I expect you will use as many **real-life data** to ensure the code works end-to-end. You can spawn VMs if needed (libvirt), start docker containers, or whatever needed to ensure everything is fully tested.
8. ANY FEATURE IS NOT COMPLETE IF IT IS NOT THOROUGHLY TESTED AGAINST REAL DEVICE DATA.
9. You are not allowed to exclude anything from the scope. Do not change the deliverable.

## TL;DR

Two features designed for **distributed Netdata architecture**:
- Multiple agents collect SNMP/topology/flows independently
- Each agent maintains local metrics, topology, and flow data
- Netdata Cloud queries all agents, aggregates/merges, and presents unified view

TOPOLOGY SHOULD WORK AT LEAST IN 2 LEVELS: LAYER 2 (SNMP) AND LAYER 3 (FLOWS). THE JSON SCHEMA OF THE FUNCTION SHOULD BE COMMON. SO THE ACTORS, THE LINKS, THE FLOWS SHOULD BE AGNOSTIC OF PROTOCOL OR LAYER, SO THAT THE SAME VISUALIZATION CAN PERFECTLY WORK FOR LAYER 2 TOPOLOGY MAPS, LAYER 3 TOPOLOGY MAPS AND ANY OTHER LAYER WE MAY ADD IN THE FUTURE. OF COURSE, IF ANY LAYER REQUIRED ADDITIONAL FIELDS (e.g. port number of a switch), THESE FIELDS SHOULD BE GENERALIZED AND BE AVAILABLE ON ALL LAYERS. For example, the port of switch at layer 2, could be the process name of a host at layer 3, so generally it should be the entity/module/component within an actor. You are allowed and expected to design this schema YOURSELF - once you finish we may polish it together - but do the heavy lifting of defining a multi-layered generic topology schema.

**Key Challenge:** Data must be "aggregatable" - topology must merge, flows must sum without double-counting.

**Permission:** Costa grants permission to run **Claude, Codex, Gemini, and GLM‑4.7** for reviews or dilemmas as needed.

## ✅ COMPLETED WORK — VERIFICATION REQUIRED

The following items are complete and verified via tests:

| Item | Description | Status |
|------|-------------|--------|
| **9** | Expand real device test coverage — add ALL LibreNMS snmprec files (106 LLDP, 15+ CDP), ALL Akvorado pcaps | ✅ DONE |
| **10** | Simulator-based integration testing — snmpsim + NetFlow stress replay workflow | ✅ DONE |
| **11** | Complete LLDP/CDP profiles — add ALL missing MIB fields (management addresses, capabilities, etc.) | ✅ DONE |

**See "Plan" section for details and verification notes.**

---

## Progress (Agent)

- 2026-03-26 (Costa decision): flow queries must not query open in-memory tiers.
  - Design rule:
    - query execution must rely only on the planner spans and their on-disk tier/raw files
    - planner fallback to lower tiers, up to raw, is the only allowed completeness mechanism
    - query code must not merge `open_tiers` / `tier_flow_indexes` rows into flow query results
  - Immediate implication:
    - the current non-raw branch in `query_flows()` that calls `scan_matching_open_tier_grouped_records_projected()` or `open_records_for_spans()` is against the desired design and must be removed or redesigned
  - Verification required before code changes:
    - identify every query path that currently reads `open_tiers`
    - confirm whether any user-visible flow query still depends on in-memory open-tier rows for recent data
    - measure/verify the behavior after removing open-tier query reads
- 2026-03-26: Implemented on-disk-only flow-query execution for grouped and timeseries query paths.
  - Query behavior changes in `src/crates/netdata-netflow/netflow-plugin/src/query.rs`:
    - removed query-time `open_tiers` / `tier_flow_indexes` merging from:
      - grouped projected flow queries
      - grouped compact flow queries
      - timeseries pass 1
      - timeseries pass 2
    - `scan_matching_records()` no longer accepts or appends in-memory `open_records`
  - Planner change:
    - `prepare_query()` now resolves each planned `QueryTierSpan` to on-disk files
    - if a non-raw span has no files, it is re-planned onto lower tiers, recursively, down to raw
    - `query_tier` stats now reflect the actual prepared spans after file-backed fallback
  - Test updates:
    - adjusted end-to-end tests to verify on-disk tier fallback semantics instead of in-memory tier merging
  - Validation:
    - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
    - result: `257 passed; 0 failed; 7 ignored`
  - Residual mismatch with Costa decision:
    - `query_flows()` still builds facet vocabulary through `open_facet_vocabulary()`, which reads `open_tiers`
    - grouped result rows and timeseries data no longer read open in-memory tiers, but the facet vocabulary path still does
- 2026-03-26 (Costa decision): facet vocabulary must also stop reading open in-memory tiers.
  - Requirement:
    - facet vocabulary for flow queries must be derived from on-disk files only
    - no query path, including facet/options payloads, may read `open_tiers` / `tier_flow_indexes`
  - Required analysis:
    - identify why `open_facet_vocabulary()` was added
    - determine whether it covers active on-disk files not included in the archived-file cache
    - replace it with an on-disk active-file vocabulary path if needed
- 2026-03-26: Implemented on-disk-only facet vocabulary and verified the netflow crate again.
  - Why `open_facet_vocabulary()` existed:
    - the old design cached archived-file facet vocabulary and then supplemented it with values from in-memory materialized tiers for recent not-yet-archived data
    - this meant grouped query rows/timeseries and facet vocabulary had different completeness rules
  - What changed in `src/crates/netdata-netflow/netflow-plugin/src/query.rs`:
    - removed `open_facet_vocabulary()` and the `OpenFacetVocabularyCache`
    - `facet_vocabulary_payload()` now combines:
      - cached archived-file vocabulary from `closed_facet_vocabulary()`
      - freshly scanned active on-disk vocabulary from `active_facet_vocabulary()`
    - active vocabulary now comes from registry files with `file.is_active()`, scanned through the same journal-file path as archived vocabulary
  - Consequence:
    - facet/options payloads now follow the same rule as grouped queries and timeseries:
      - planner / registry files only
      - no query-side reads from `open_tiers` / `tier_flow_indexes`
  - Validation:
    - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
    - result: `257 passed; 0 failed; 7 ignored`
  - Cleanup:
    - removed dead open-tier query helpers and dead facet/open-tier query cache types from `query.rs`
    - removed dead `rows_for_tier()` / `rollup_field_supported()` helpers no longer needed by query code
    - `FlowQueryService::new()` no longer depends on `open_tiers` / `tier_flow_indexes`; the query service is now structurally decoupled from query-time in-memory tier reads
- 2026-03-26 (Costa proposal, pending design decision): simplify sampled-flow semantics and remove `RAW_*` / `SAMPLING_RATE` from the hot query path.
  - Proposed direction:
    - scale `BYTES` / `PACKETS` at ingestion time to estimated unsampled values
    - still persist `RAW_BYTES`, `RAW_PACKETS`, and `SAMPLING_RATE` in the journal for debugging / future use
    - never read back or expose `RAW_*` / `SAMPLING_RATE` in normal query paths
    - write `RAW_BYTES`, `RAW_PACKETS`, and `SAMPLING_RATE` as the last journal fields so the hot raw query path reaches useful fields first
  - Required verification before implementation:
    - all current code paths that read or scale `BYTES` / `PACKETS`
    - whether any query/API/UI path still consumes `RAW_*` or `SAMPLING_RATE`
    - whether ingestion-time scaling conflicts with tiering, top-N ranking, time-series, or compatibility with existing stored data
- 2026-03-26: Verified current sampled-counter behavior for the pending ingestion-scaling design.
  - Current journal write order in `FlowRecord::encode_journal_fields()` is:
    - `SAMPLING_RATE`
    - `ETYPE`
    - `PROTOCOL`
    - `BYTES`
    - `PACKETS`
    - `FLOWS`
    - `RAW_BYTES`
    - `RAW_PACKETS`
    - then later fields including `SRC_ADDR` / `DST_ADDR`
    - evidence: `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs`
  - Current semantics:
    - decoder writes `BYTES`, `PACKETS`, `RAW_BYTES`, `RAW_PACKETS`, and `SAMPLING_RATE` separately; it does not renormalize at ingestion time
    - generic raw query path renormalizes at query time in `sampled_metrics_from_fields()`
    - open-tier query path renormalizes at query time in `sampled_metrics_from_open_tier_row()`
    - projected raw grouped fast path parses `BYTES` / `PACKETS` directly and currently bypasses the generic query-time renormalization helpers
  - Current exposure/usage:
    - grouped output serializes only `bytes` / `packets`
    - `RAW_*` is still read in query code and carried in `FlowMetrics`
    - `RAW_*` is still stored in tiered metrics (`tiering.rs`)
    - `SAMPLING_RATE` is currently a normal canonical field, a rollup dimension, and has a presentation label (`Sampling Rate`)
    - `RAW_*` is not groupable, but is currently accepted in some selection paths (`open_tier_selection_field_supported()`)
  - Implication:
    - Costa's proposal is not a small hot-path tweak; it is a semantic change that simplifies query-time math, removes a current fast-path inconsistency, and would also require tiering/open-tier/API cleanup for `RAW_*` and `SAMPLING_RATE`
- 2026-03-26 (Costa decision): adopt ingestion-time canonical scaling with no backward-compatibility handling for existing stored data.
  - Decided semantics:
    - `BYTES` / `PACKETS` are canonical query counters and must be scaled at ingestion time to estimated unsampled values
    - `RAW_BYTES` / `RAW_PACKETS` / `SAMPLING_RATE` remain persisted only for debugging / future use
    - normal query, grouping, selection, tiering, and presentation paths must not read back or expose `RAW_*` / `SAMPLING_RATE`
    - `RAW_BYTES`, `RAW_PACKETS`, and `SAMPLING_RATE` must be encoded at the end of raw journal rows
  - Compatibility decision:
    - ignore old stored data completely
    - do not add compatibility logic
    - do not migrate
    - mixed old/new stores are acceptable during development on this branch; no code should attempt to support both semantics
- 2026-03-26: Implemented the new sampled-counter model in `netflow-plugin` and revalidated the crate.
  - Decoder changes:
    - `FlowRecord::encode_journal_fields()` now writes `RAW_BYTES`, `RAW_PACKETS`, and `SAMPLING_RATE` at the end of the raw row
    - `finalize_record()` now preserves `RAW_*` and scales canonical `BYTES` / `PACKETS` at ingestion time using `sampling_rate.max(1)`
    - `finalize_canonical_flow_fields()` now mirrors the same ingestion-time scaling for `FlowFields`-based decode paths
  - Query / API changes:
    - grouped/query metrics now use only canonical `bytes` / `packets`
    - projected raw grouped paths no longer match or parse `RAW_BYTES` / `RAW_PACKETS`
    - `supported_flow_field_names()` and request validation now reject `RAW_BYTES`, `RAW_PACKETS`, and `SAMPLING_RATE` from normal query/group/selection paths
    - open-tier field lookup and selection support no longer expose `RAW_*`
  - Tiering changes:
    - tiered `FlowMetrics` now store only canonical `bytes` / `packets`
    - `SAMPLING_RATE` was removed from rollup dimensions and presence tracking
  - Regression found and fixed:
    - overlapping normal/special decoder passes were previously deduplicated using visible `BYTES` / `PACKETS`
    - after ingestion-time scaling this broke MPLS flow merging, because special raw decode still had unscaled visible counters while normal decode had scaled counters
    - fix: dedup / merge identity now uses `RAW_BYTES` / `RAW_PACKETS` when present, falling back to visible counters only when raw counters are absent
  - Validation:
    - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
    - result: `257 passed; 0 failed; 7 ignored`
- 2026-03-26: Re-measured the fixed 4-file release benchmark after the ingestion-time counter change (`taskset -c 3`, 3 runs each).
  - Dataset remained the same:
    - `745,969` rows
    - `24,537,589` fields
    - `32.8936` fields/row
  - New release means:
    - plugin `1+2+3`: `1.272924 +/- 0.016515 usec/row`
    - plugin `1+2+3+4`: `1.658046 +/- 0.021297 usec/row`
    - plugin `1+2+3+4+5`: `1.669254 +/- 0.014313 usec/row`
    - plugin `1+2+3+4+5+6`: `1.763245 +/- 0.011751 usec/row`
    - full warm grouped query: `1.888586 +/- 0.017995 usec/row`
  - Derived stage costs now:
    - step `4`: `0.385122 usec/row`
    - step `5`: `0.011208 usec/row`
    - step `6`: `0.093991 usec/row`
    - later grouped work: `0.125341 usec/row`
  - Compared to the previous pre-change branch state:
    - step `4` dropped from about `0.426722` to `0.385122 usec/row`
    - full warm grouped query dropped from about `1.982956` to `1.888586 usec/row`
  - Conclusion:
    - the semantic cleanup produced a real win, but not enough
    - step `4` remains the largest single overhead
- 2026-03-26: Optimized the planned projected matcher to skip 8-byte prefix extraction when the first-byte bucket proves no requested key can match the payload.
  - Change:
    - `apply_projected_payload_planned()` and `benchmark_apply_projected_payload_planned()` now compute `candidates` first and return immediately when it is zero
    - this avoids `projected_prefix_value(payload)` on the dominant non-matching payloads
  - Release re-measurement after this matcher cut (`taskset -c 3`, 3 runs each):
    - match-only: `1.575195 +/- 0.014501 usec/row`
    - `1+2+3+4`: `1.612864 +/- 0.015709 usec/row`
    - `1+2+3+4+5`: `1.608491 +/- 0.005686 usec/row`
    - `1+2+3+4+5+6`: `1.719427 +/- 0.022879 usec/row`
    - full warm grouped query: `1.839996 +/- 0.011029 usec/row`
  - Compared to the immediate pre-change state:
    - match-only delta improved from about `0.341807` to `0.302271 usec/row`
    - step `4` delta improved from about `0.385122` to `0.339940 usec/row`
    - full warm grouped query improved from about `1.888586` to `1.839996 usec/row`
  - Follow-up experiment:
    - tried removing `starts_with()` when the key fits fully in the 8-byte prefix
    - measured result was worse
    - reverted that experiment immediately; it is not part of the current branch state

- 2026-03-26: Verified the 4 current `copilot-pull-request-reviewer` comments on PR `netdata/netdata#21702` and fixed all 4 locally with targeted tests passing:
  - `src/crates/netdata-plugin/rt/src/lib.rs`: removed silent `unwrap_or_default()` JSON serialization fallback for `flows:*`; now log the serialization error and leave payload unset.
  - `src/crates/journal-engine/src/logs/query.rs`: replaced `a.len() + b.len()` with `a.len().saturating_add(b.len())` before capacity calculation.
  - `src/go/pkg/buildinfo/buildinfo.go`: documented that `Info()` output is an extensible `key=value` contract and parsers must ignore unknown keys.
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs`: pre-reserved merged vector capacity before cloning records from all sources.
  - while validating `rt`, also fixed a pre-existing doctest import issue in `src/crates/netdata-plugin/rt/src/lib.rs`; `cargo test --manifest-path src/crates/Cargo.toml -p rt --quiet`, `cargo test --manifest-path src/crates/Cargo.toml -p journal-engine --quiet`, `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`, and `cd src/go && go test ./pkg/buildinfo` all pass locally.
- 2026-03-26: Re-checked the currently failing package jobs before committing, per Costa instruction. Concrete CI evidence from PR `#21702`:
  - `Build (armhf, debian:bullseye, ...)` and `Build (armhf, debian:bookworm, ...)` both fail while building `netflow-plugin`.
  - extracted job logs show the same root error in `netflow-plugin` build script: `Error: Error { os: "linux", arch: "arm" }` after `failed to run custom build command for netflow-plugin v0.1.3`.
  - `src/crates/netdata-netflow/netflow-plugin/build.rs` currently hard-requires `protoc_bin_vendored::protoc_bin_path()?`.
  - upstream crate source `~/.cargo/registry/src/index.crates.io-1949cf8c6b5b557f/protoc-bin-vendored-3.2.0/src/lib.rs` supports only `linux x86`, `linux x86_64`, `linux aarch64`, `linux powerpc64`, and `linux s390x`; it does not support 32-bit `linux arm`, and returns `Err(Error { os, arch })` for unsupported platforms.
  - conclusion: the `armhf` package failures are a real current branch issue and are most likely caused by `protoc_bin_vendored` in `netflow-plugin/build.rs`, not by the 4 Copilot-fix files above.
- 2026-03-26: The third current package failure (`Build (x86_64, quay.io/centos/centos:stream10, ...)`, step `Test Packages`) is different. Extracted logs show RPM install failures because built `netdata-2.9.0-1.el10.x86_64` requires `libbson-1.0.so.0()(64bit)` and `libmongoc-1.0.so.0()(64bit)`, and those providers are not available in the test environment. This failure is separate from the `armhf` `protoc` issue and unrelated to the 4 Copilot-fix files; current evidence is insufficient to attribute it to the changes touched in this slice.
- 2026-03-26: `CodeQL` is still red on PR `#21702`, but current evidence says this is not from the touched files above: check-run `68602388890` reports `2 configurations not found` with missing configs on `master` for `/language:go` and `/language:python`. Current PR code-scanning alerts do not show an open alert in the 4 touched files. `Codacy` is also still `action_required`, but this is an external service gate, not a local compile/test failure.
- 2026-03-26: Applied Costa decision `A` for `netflow-plugin` protobuf generation. `src/crates/netdata-netflow/netflow-plugin/build.rs` now:
  - respects `PROTOC` if already set,
  - otherwise uses vendored `protoc` where supported,
  - otherwise falls back to `protoc` from `PATH`,
  - and fails with a clear message only if neither vendored nor system `protoc` is available.
  Local validation after this change:
  - `command -v protoc && protoc --version` -> `/usr/bin/protoc`, `libprotoc 33.1`
  - `cargo fmt --manifest-path src/crates/Cargo.toml --all`
  - `cargo test --manifest-path src/crates/Cargo.toml -p rt --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-engine --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
  - `cd src/go && go test ./pkg/buildinfo`
  All passed locally.
- 2026-03-26: Re-checked PR `netdata/netdata#21962` after Costa reported a developer comment. The new comment is from `vkalintiris` on the PR discussion, not a code-review thread. Concrete statement:
  - keep only optimization `(1)` in PR `#21962`: collecting data object offsets of an entry object
  - reject promoted `journal-core` extras:
    - changing `RefCell` to `Cell`
    - memory-mapped window lookup changes
    - cached journal-file header state
  - rationale given:
    - `RefCell` runtime checks help debug unsafe code
    - mmap window size can be increased instead of changing lookup logic
    - cached header state breaks live-tail on actively written journal files
    - broader optimizations need measurements plus a `cargo` example that demonstrates the target access pattern
  - implication: the previously promoted minimal `journal-core` subset in PR `#21962` is now explicitly outside the reviewer-accepted scope and likely needs to be removed from that PR if we want it merged cleanly.
- 2026-03-26: New Costa request for PR `#21962`: do not act on the reviewer preference yet. First:
  - validate each of the developer objections technically,
  - measure the real performance contribution of optimizations `2-4`,
  - verify Costa's expected impact numbers for the target access pattern with `35` fields:
    - `(1)` ENTRY offsets caching: about `2x`
    - `(2-4)` extra `journal-core` optimizations: about `2x`
  - target output: evidence-backed decision, ideally with a reproducible benchmark/example for the specific journal access pattern.
- 2026-03-26 (Costa): Measurement plan for PR `#21962` must be incremental and use the real NetFlow database:
  1. use the existing NetFlow DB
  2. read `1M` rows from the raw tier
  3. check out `master` in `/tmp/` and benchmark baseline
  4. apply ENTRY caching and benchmark
  5. apply each of the other changes separately and benchmark
  - goal: concrete per-change evidence, not mixed end-to-end numbers
  - benchmark contract clarified by Costa:
    - measure raw DB read performance only
    - enumerate all fields of every row
    - do not parse, aggregate, normalize, or otherwise process the field values
    - stop after `1M` rows
  - note from Costa: for the cached-header objection, one possible follow-up is caching per query instead of forever, so live tailing may still work
- 2026-03-26: Costa updated PR `netdata/netdata#21962` to current `master`. Implication for benchmarking:
  - current `upstream/master` is now the intended baseline
  - incremental measurements should be performed against a current-master worktree plus the PR changes, not against the older historical merge-base
- 2026-03-26: Built an external raw-tier benchmark harness in `/tmp/journal-raw-bench-1774481723` against the real NetFlow raw DB (`/var/cache/netdata/flows/raw`). Access pattern:
  - open raw journals directly with `journal-core`
  - iterate forward until `1,000,000` rows
  - for each row, call `entry_data_offsets()` and then `data_ref()` for every field offset
  - no parsing, grouping, filtering, normalization, or payload processing beyond counting rows and fields
  - measured dataset shape:
    - `1,000,000` rows
    - `35,321,402` total fields
    - `35.3214` fields per row
- 2026-03-26: Incremental benchmark results for PR `#21962` at small `64 KiB` journal window size (same 1M-row raw scan, median of 3 warm runs):
  - baseline `master`: `7915.666 ms`
  - `(1)` ENTRY offsets caching only: `7812.753 ms`
    - `1.30%` faster vs baseline
  - `(2)` guarded-cell/value-guard changes on top of `(1)`: `7651.415 ms`
    - `2.07%` faster vs `(1)`
    - `3.34%` faster vs baseline
  - `(3)` mmap window-manager changes on top of `(2)`: `7263.296 ms`
    - `5.07%` faster vs `(2)`
    - `8.24%` faster vs baseline
  - `(4)` cached-header-state changes on top of `(3)`: `7190.221 ms`
    - `1.00%` faster vs `(3)`
    - `9.16%` faster vs baseline
  - conclusion from this window size:
    - none of the changes are anywhere near `2x`
    - `(3)` is the biggest contributor at `64 KiB`
- 2026-03-26: Verified that the above `64 KiB` benchmark is not the actual application window used by `journal-session`.
  - evidence:
    - `journal-session` default window size is `8 MiB` in `src/crates/journal-session/src/lib.rs`
  - implication:
    - `64 KiB` numbers are still useful as a low-window micro-benchmark
    - but they are not the best representation of the real netflow/journal-session path
- 2026-03-26: Benchmarked current `master` raw scan with larger window sizes (median of 3 warm runs):
  - `64 KiB`: `7881.163 ms`
  - `256 KiB`: `5804.592 ms`
    - `26.35%` faster vs `64 KiB`
  - `1 MiB`: `4244.364 ms`
    - `46.15%` faster vs `64 KiB`
  - conclusion:
    - reviewer point `(3)` is technically credible: simply increasing the mmap window on current master yields a much larger win than the code changes measured at `64 KiB`
- 2026-03-26: Incremental benchmark results at the application-relevant `8 MiB` journal window size (same 1M-row raw scan, median of 3 warm runs):
  - baseline `master`: `1110.622 ms`
  - `(1)` ENTRY offsets caching only: `1104.814 ms`
    - `0.52%` faster vs baseline
  - `(2)` guarded-cell/value-guard changes on top of `(1)`: `1150.943 ms`
    - `4.18%` slower vs `(1)`
    - `3.63%` slower vs baseline
  - `(3)` mmap window-manager changes on top of `(2)`: `1071.587 ms`
    - `6.90%` faster vs `(2)`
    - `3.51%` faster vs baseline
  - `(4)` cached-header-state changes on top of `(3)`: `925.662 ms`
    - `13.62%` faster vs `(3)`
    - `16.65%` faster vs baseline
  - conclusion from the real `8 MiB` path:
    - `(1)` is tiny
    - `(2)` is negative in this access pattern
    - `(3)` is modestly positive
    - `(4)` is the biggest win
- 2026-03-26: Validated reviewer point `(4)` functionally with a dedicated live-tail repro in the same external benchmark crate (`live_tail_check` binary):
  - on current `master`:
    - open active journal for reading
    - append a second entry via a writable handle after the reader-side `JournalFile` is already open
    - create a fresh reader on the same already-open `JournalFile`
    - result: `rows_seen_after_append=2`, payloads include both `MESSAGE=first` and `MESSAGE=second`
  - on the cached-header variant:
    - same repro
    - result: `ObjectExceedsFileBounds`
  - conclusion:
    - reviewer point `(4)` is valid
    - the cached-header optimization, as currently implemented, breaks live reads on an already-open journal file when the file grows externally
- 2026-03-26 (Costa): Add a second benchmark implementation for the exact same raw-tier access pattern, but using `libsystemd` in C:
  - same raw DB: `/var/cache/netdata/flows/raw`
  - same stop condition: `1,000,000` rows
  - same work: enumerate all fields of every row, without parsing/aggregation
  - purpose: compare the current Rust `journal-core` raw-scan numbers against the reference `libsystemd` reader path
- 2026-03-26: Concrete implementation plan for the `libsystemd` benchmark:
  - build a standalone C harness in `/tmp`, outside the repo, so no product code changes are mixed with this evidence work
  - open `/var/cache/netdata/flows/raw` with `sd_journal_open_directory()`
  - seek to head, iterate exactly `1,000,000` rows with `sd_journal_next()`
  - enumerate every field of every row with `sd_journal_restart_data()` + `sd_journal_enumerate_data()`
  - disable field truncation with `sd_journal_set_data_threshold(..., 0)` so the benchmark matches the Rust harness that reads full field payloads
  - record total rows, total fields, elapsed milliseconds, and run the benchmark three times for a median
  - compare only against the existing Rust raw-tier benchmark for the same workload; do not mix these numbers with the grouped-query `query_flows()` benchmarks in `TODO-netflow-backend.md`
- 2026-03-26: Corrected the `libsystemd` benchmark implementation after validating workload equivalence.
  - first attempt used `sd_journal_open_directory()`, which interleaves all files in the directory automatically according to the official `sd_journal_open_directory(3)` docs
  - that produced a different workload than the Rust harness, which reads raw journals file-by-file in sorted filename order
  - evidence of the mismatch:
    - initial `libsystemd` run still read `1,000,000` rows, but saw only `32,901,435` fields (`32.9014` fields/row)
    - the Rust harness on the same DB/workload shape saw `35,321,402` fields (`35.3214` fields/row)
  - fixed the C benchmark by switching to:
    - sorted raw journal file enumeration
    - `sd_journal_open_files()` with one file at a time
    - same per-file forward scan order as the Rust harness
  - after this correction, the dataset shape matches exactly:
    - `files_opened=11`
    - `rows=1,000,000`
    - `fields=35,321,402`
    - `fields_per_row=35.3214`
- 2026-03-26: Final `libsystemd` vs Rust raw-tier comparison for the exact same 1M-row / all-fields workload.
  - standalone C harness:
    - `/tmp/journal-raw-bench-1774481723/libsystemd_raw_bench.c`
    - built with `gcc -O3 ... $(pkg-config --cflags --libs libsystemd)`
    - uses `sd_journal_open_files()` one file at a time, `sd_journal_set_data_threshold(..., 0)`, `sd_journal_next()`, `sd_journal_restart_data()`, `sd_journal_enumerate_data()`
  - Rust harness:
    - `/tmp/journal-raw-bench-1774481723/src/main.rs`
    - current `upstream/master` `journal-core`
    - `8 MiB` window size, matching `journal-session` default
  - median of 3 warm runs:
    - `libsystemd` C: `1771.428 ms`
    - Rust `journal-core` (`master`, `8 MiB`): `934.210 ms`
  - ratio:
    - Rust is about `1.90x` faster than `libsystemd` on this exact workload
    - equivalently, `libsystemd` is about `89.6%` slower than Rust here
  - important implication:
    - the standalone `journal-core` raw reader on `master` already outperforms the `libsystemd` reference path significantly for this raw-tier sequential scan
- 2026-03-26: Pending Costa decision for PR `netdata/netdata#21962` after the raw-tier evidence pass:
  - question: should the PR remain at all, and if yes, in what narrowed form?
  - evidence now available for the exact target workload (`1,000,000` raw rows, all fields, no parsing/aggregation):
    - `(1)` entry-offset caching only: tiny gain on the application-relevant `8 MiB` path (`1110.622 ms` -> `1104.814 ms`, `0.52%` faster)
    - `(2)` guarded-cell/value-guard changes: negative on the `8 MiB` path (`1150.943 ms`, `3.63%` slower than baseline)
    - `(3)` mmap window-manager changes: modest positive on the `8 MiB` path (`1071.587 ms`, `3.51%` faster than baseline)
    - `(4)` cached-header-state changes: strong gain on the `8 MiB` path (`925.662 ms`, `16.65%` faster than baseline) but functionally invalid because it breaks live reads on an already-open file when the journal grows (`ObjectExceedsFileBounds` in the live-tail repro)
  - reviewer/developer guidance on the PR already matches much of this evidence:
    - keep `(1)` only
    - reject broader changes unless they have clear measurement and safe semantics
  - next response to Costa should present explicit options:
    - close the PR
    - narrow it to `(1)` only
    - split out `(3)` into a new separately justified PR
    - redesign `(4)` with refresh-safe semantics and benchmark again before any upstream proposal
- 2026-03-26 (Costa): Before deciding on `(4)`, test a new variant of cached-header-state:
  - cache the journal file header only for the lifetime of a single query/reader
  - initialize that cache when the query starts
  - discard it when the query ends
  - repeat the live-tail repro and determine the exact semantics:
    - can a fresh query on an already-open journal file see later appended entries?
    - can an in-flight query/tail operation see entries appended after its cache was created?
- 2026-03-26: Completed the query-local cached-header live-tail experiment in a separate `/tmp` worktree based on current `master`.
  - implementation approach:
    - added an experimental `QueryHeaderCache` snapshot to a temporary `journal-core` worktree only
    - the snapshot caches:
      - `header_size`
      - `arena_end`
      - `is_compact`
    - object access in the experiment uses that snapshot only for the lifetime of one query
    - after append, a fresh query takes a fresh snapshot from the same already-open `JournalFile`
  - dedicated binary:
    - `/tmp/journal-raw-bench-1774481723/src/bin/live_tail_check_query_header.rs`
  - results:
    - before append:
      - `saw_first_before_append=true`
      - first row payloads: `["MESSAGE=first"]`
    - same query, after append:
      - `stale_step_after_append=false`
      - the in-flight query does not continue into the new entry
    - fresh query after append on the same already-open `JournalFile`:
      - `fresh_rows_after_append=2`
      - payloads: `[["MESSAGE=first"], ["MESSAGE=second"]]`
  - control run on plain current `master` with no query-local cache:
    - `/tmp/journal-raw-bench-1774481723/src/bin/live_tail_check_running_reader.rs`
    - same behavior:
      - running reader after append does not continue (`running_step_after_append=false`)
      - fresh reader after append sees both rows
  - conclusion:
    - query-local header caching restores the same semantics as current `master` for fresh queries on an already-open file
    - it does **not** provide live-follow/tailing within a single already-running query
    - importantly, it avoids the broken behavior of the per-`JournalFile` cached-header variant, where even a fresh query after append failed with `ObjectExceedsFileBounds`
- 2026-03-26: Tested the more relevant mitigation for cached header state: refresh cached `arena_end` from the live header map and retry once on stale-bounds failure.
  - important correction:
    - the broken cached-header variant does **not** fail at EOF
    - it fails during object access when `end_offset > self.arena_end.get()` in the cached-header `journal_object_ref()` path
    - so "refresh on EOF and retry" is not the right trigger for the observed bug; the meaningful trigger is "refresh on stale `ObjectExceedsFileBounds` and retry once"
  - experimental implementation:
    - temporary patch on the cached-header worktree (`/tmp/netdata-pr21962-header-1774481723`)
    - added `refresh_cached_arena_end()` and retried only when `end_offset` exceeded the cached bound
  - functional result:
    - the original broken fresh-query case is fixed again
    - `live_tail_check` on the refreshed cached-header variant now returns:
      - `rows_seen_after_append=2`
      - payloads: `[["MESSAGE=first"], ["MESSAGE=second"]]`
  - matched performance evidence:
    - simple 3-run median after the patch:
      - refreshed cached-header variant: `952.393 ms`
      - adjacent master rerun: `1010.582 ms`
      - apparent gain: about `5.8%`
    - but a tighter alternating A/B run (5 single-run samples each, alternating `master` and refreshed-cached-header) shows the advantage collapsing to noise:
      - `master` samples: `936.973`, `943.804`, `948.005`, `935.388`, `965.353`
      - `refreshed cached-header` samples: `960.207`, `951.956`, `944.767`, `934.202`, `1098.575`
      - medians:
        - `master`: `943.804 ms`
        - `refreshed cached-header`: `951.956 ms`
      - result:
        - refreshed cached-header is about `0.86%` slower by median in the tighter alternating run
  - conclusion:
    - this mitigation restores fresh-query correctness
    - but it does **not** preserve the earlier `~16%` speedup in a robust way
    - under tighter A/B measurement, the net benefit is effectively gone (noise to slightly negative)
- 2026-03-26 (Costa): Challenge the previous interpretation of the retry experiment:
  - a single extra nested `if` should not plausibly erase a real `~16%` win on its own
  - next evidence step:
    - benchmark the original cached-header variant vs the refreshed-cached-header variant directly
    - use a static snapshot copy of `/var/cache/netdata/flows/raw` to remove live file growth as a confounder
    - then compare both variants against `master` on the same snapshot
- 2026-03-26 (Costa): Replace the broad raw-tier benchmark with a stricter fixed-file benchmark:
  - use exactly these 4 files from `/var/cache/netdata/flows/raw/`:
    - `system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal`
    - `system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal`
    - `system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal`
    - `system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal`
  - read them in whole; roughly `1 GiB` total input
  - timing requirements:
    - use a monotonic clock
    - include everything:
      - opening each file
      - reading all rows
      - closing each file
  - report for each test:
    1. number of rows read
    2. number of fields read
    3. total time in usec
    4. average number of fields per row
    5. time per row in usec
  - repeat each test 3 times
  - tests required:
    1. `libsystemd`
    2. `master`
    3. `ENTRY cache`
    4. guarded-cell/value-guard change
    5. mmap-window-manager change
    6. cached-header change
  - output contract:
    - one single table
    - 6 rows, 5 metric columns
    - timing columns must be reported as `AVERAGE +/- VARIATION`
    - any fluctuation in rows/fields indicates a broken benchmark and invalid results
- 2026-03-26: Completed the fixed-file benchmark exactly as requested, using only the 4 specified raw journal files from `/var/cache/netdata/flows/raw/`.
  - methodology:
    - exact files only, read in whole
    - monotonic clock inside the benchmark harness
    - measured scope includes opening each file, reading all rows, and closing it
    - CPU affinity pinned to core `3` with `taskset -c 3`
    - 3 runs per variant
    - timing variation reported as standard deviation
  - stability check:
    - all 6 variants produced identical counts in all 3 runs
    - `rows = 745,969`
    - `fields = 24,537,589`
    - `fields_per_row = 32.8936`
    - therefore the benchmark is not broken by row/field drift
  - summary table:
    - `libsystemd`
      - `rows`: `745,969`
      - `fields`: `24,537,589`
      - `time_usec`: `1,256,675 +/- 4,725`
      - `fields_per_row`: `32.8936`
      - `usec_per_row`: `1.684620 +/- 0.006335`
    - `master`
      - `rows`: `745,969`
      - `fields`: `24,537,589`
      - `time_usec`: `779,320 +/- 6,703`
      - `fields_per_row`: `32.8936`
      - `usec_per_row`: `1.044709 +/- 0.008986`
    - `ENTRY cache`
      - `rows`: `745,969`
      - `fields`: `24,537,589`
      - `time_usec`: `777,872 +/- 4,010`
      - `fields_per_row`: `32.8936`
      - `usec_per_row`: `1.042767 +/- 0.005375`
    - `guarded-cell/value-guard`
      - `rows`: `745,969`
      - `fields`: `24,537,589`
      - `time_usec`: `770,928 +/- 2,536`
      - `fields_per_row`: `32.8936`
      - `usec_per_row`: `1.033459 +/- 0.003399`
    - `mmap`
      - `rows`: `745,969`
      - `fields`: `24,537,589`
      - `time_usec`: `781,088 +/- 13,640`
      - `fields_per_row`: `32.8936`
      - `usec_per_row`: `1.047079 +/- 0.018285`
    - `cached-header`
      - `rows`: `745,969`
      - `fields`: `24,537,589`
      - `time_usec`: `788,998 +/- 12,377`
      - `fields_per_row`: `32.8936`
      - `usec_per_row`: `1.057682 +/- 0.016591`
  - implications of the fixed-file suite:
    - current `master` is already much faster than `libsystemd` on this workload
    - the broad "2x" claim is not supported
    - on this exact 4-file workload:
      - `ENTRY cache` is a tiny improvement vs `master`
      - `guarded-cell/value-guard` is the fastest of the Rust variants
      - `mmap` and `cached-header` are slower than `master`
- 2026-03-26: Pending Costa decision after the fixed-file suite:
  - question: is PR `netdata/netdata#21962` worth keeping at all?
  - evidence for the exact requested workload:
    - `master`: `779,320 +/- 6,703 usec`
    - `ENTRY cache`: `777,872 +/- 4,010 usec`
      - only about `0.19%` faster than `master`
    - `guarded-cell/value-guard`: `770,928 +/- 2,536 usec`
      - about `1.08%` faster than `master`
    - `mmap`: `781,088 +/- 13,640 usec`
      - slower than `master`
    - `cached-header` (without tail fix): `788,998 +/- 12,377 usec`
      - slower than `master`
  - reviewer/developer stance on the PR already pushes to keep only `(1)` if anything is kept
  - likely decision options to present:
    - close the PR entirely as not worth merge/review cost
    - keep only `(1)` as a very small safe cleanup if maintainers want it despite the tiny gain
- 2026-03-26: Completed the frozen-snapshot control benchmark to answer Costa's objection.
  - snapshot:
    - copied `/var/cache/netdata/flows/raw` (`8.7G`) to `/tmp/netdata-raw-snapshot-1774481723`
    - purpose: remove live file growth and isolate pure branch/codegen cost
  - `8 MiB` window, `1,000,000` rows, all fields, 5 warm runs each
  - refreshed cached-header variant samples:
    - `1030.168`, `1011.352`, `973.550`, `1007.607`, `983.336`
    - median: `1007.607 ms`
  - original cached-header variant samples:
    - `972.532`, `955.080`, `997.102`, `962.332`, `950.054`
    - median: `962.332 ms`
  - current `master` samples:
    - `982.429`, `1003.279`, `1019.581`, `963.833`, `966.732`
    - median: `982.429 ms`
  - conclusions from the frozen snapshot:
    - Costa's objection is valid:
      - the extra retry logic does **not** erase a real `16%` win because the original cached-header variant itself is only about `2.05%` faster than `master` on the frozen snapshot (`982.429 -> 962.332 ms`)
    - refreshed cached-header vs original cached-header:
      - about `4.70%` slower by median (`962.332 -> 1007.607 ms`)
    - refreshed cached-header vs `master`:
      - about `2.56%` slower by median (`982.429 -> 1007.607 ms`)
  - corrected interpretation:
    - the earlier `~16%` gain was not a robust result for the static raw-scan workload
    - once live growth is removed as a confounder, cached-header-only is a small win, not a large one
    - adding retry logic then turns that small win into a small loss
- 2026-03-26: Re-checked PR `netdata/netdata#21702` review state. Current unresolved review threads are `4`, all non-outdated and all from `copilot-pull-request-reviewer` on `2026-03-25`. Affected files:
  - `src/crates/netdata-plugin/rt/src/lib.rs`
  - `src/crates/journal-engine/src/logs/query.rs`
  - `src/go/pkg/buildinfo/buildinfo.go`
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs`
  Planned next step: validate each comment against current code, check current CI failures too, then fix the valid issues together and rerun targeted tests before any commit.
- 2026-03-25: Verified current topology-only remaining backend work before resuming implementation. Current evidence-backed backlog is:
  - `topology:streaming` is live and no longer matches the old stale rewrite TODO; remaining work there is parity/correctness audit and automated validation.
  - `topology:network-connections` is live; remaining work there is dedicated automated validation, not foundational implementation.
  - real NetFlow CI coverage is still missing, but this is a flows/backend integration item rather than topology-specific function work.
  - deferred `network-viewer` backend roadmap (`flows:*`, config, DYNCFG) remains optional future work, not an urgent correctness issue.
  - next topology slice requires Costa choice between validation-first, parity-audit-first, or broader deferred roadmap work.
- 2026-03-25: Re-checked PR `netdata/netdata#21962` after a new review comment arrived. Verified the new comment was valid and narrow in scope: `src/crates/journal-core/src/file/guarded_cell.rs` still had a stale doc example that referred to the old `ValueGuard::new(...)` signature and the wrong error type. Fixed and pushed in both branches:
  - `entry-offset-cache`: `80de0498ca` (`journal-core: fix guarded cell docs`)
  - `topology-flows`: `ecb96333f2` (`journal-core: fix guarded cell docs`)
  - targeted validation: `cargo test --manifest-path src/crates/Cargo.toml -p journal-core --quiet` passed on both branches
  - replied on PR `#21962` thread and resolved it
  - verified unresolved review threads on `#21962`: `0`
- 2026-03-25: Re-verified `topology:streaming` before resuming topology work because the dedicated TODO had become stale. Facts from current `src/web/api/functions/function-streaming.c`: the function already uses `streaming_path` for parent detection and link generation (`542-568`, `1133-1239`), already emits actor types `parent` / `child` / `vnode` / `stale` (`348-432`, `595-603`), and already exposes backend filters `node_type`, `ingest_status`, `stream_status` (`302-323`, `335-340`). Conclusion: the old "IP-based / generic netdata-agent" bug report is no longer current truth; the remaining streaming work must start from a fresh parity/correctness audit instead of the historical TODO claims.
- 2026-03-25: Re-verified the broader topology backend backlog against current code, live function calls, and evidence files. Current live authenticated facts:
  - `topology:streaming` returns `200`, `type=topology`, `actors_total=22`, `links_total=21`, accepted params `node_type`, `ingest_status`, `stream_status`.
  - `topology:network-connections` returns `200`, `type=topology`, `source=network-connections`, `links_total=138`.
  - `.github/workflows/snmp-netflow-sim-tests.yml` still exercises only SNMP paths/steps, so real NetFlow CI coverage remains missing.
  - dedicated automated validation for the C producers (`src/web/api/functions/function-streaming.c`, `src/collectors/network-viewer.plugin/network-viewer.c`) is still not present.
  - `src/go/pkg/topology/engine/parity/evidence/phase2-gap-report.md` and companion parity JSON files currently show Phase 2 direct-port parity pass, so older TODO text claiming that parity is still open is stale unless new live divergence evidence is collected.
- 2026-03-25: Started journal-scope split analysis for PRs `netdata/netdata#21702` (`topology-flows`) and `netdata/netdata#21962` (`entry-offset-cache`). Verified with GitHub PR metadata and local refs that `topology-flows` is rebased on current `upstream/master`, while `entry-offset-cache` still branches from `20f7861eff9a0af18ea9551b72a5eaf1c1b91546` (`git merge-base origin/entry-offset-cache topology-flows` and `git merge-base upstream/master origin/entry-offset-cache` both return `20f7861...`).
- 2026-03-25: Collected concrete journal-only delta that exists in `topology-flows` but not in standalone PR scope yet. Beyond the entry-offset cache work already in `#21962`, the branch contains extra journal commits touching `src/crates/journal-core`, `src/crates/journal-engine`, and `src/crates/journal-log-writer`, notably: `392ed570fb` (`journal: move session back to journal-core`), `5f078d703c` (`netflow: optimize journal payload access`), `7f54485435` (`netflow: speed up journal payload traversal`), `cd69d8b69f` (`journal: reduce payload traversal overhead`), `a1fb2de47a` (`journal-log-writer: add safe timestamp overrides and restart seeding`), and `2d8658bf36` (`review: address copilot suggestions and projection edge cases`).
- 2026-03-02: Started PR `netdata/netdata#21702` triage for Costa request: investigate failing CI jobs and high-volume review comments; gather concrete evidence (failed checks, logs, comment threads), map to repo paths, propose fix sequence, then implement and re-test.
- 2026-03-02: Collected concrete CI failure evidence from latest PR run (`2026-03-02`): `Go toolchain tests` failed on `TestWatcher_Run/change_file` in `src/go/plugin/agent/discovery/file/watch_test.go` (flake-like; local `go test -race -count=20 -run TestWatcher_Run ./plugin/agent/discovery/file` passed), and `Packages` RPM matrix failed consistently with `Installed (but unpackaged) file(s) found: /usr/sbin/topology-ip-intel-downloader` (for example job `65382016464`).
- 2026-03-02: Applied Costa decision that `topology-ip-intel-downloader` belongs to `plugin-netflow`: moved install ownership (binary + cleanup hooks + `topology-ip-intel.yaml`) to `plugin-netflow`, removed it from `plugin-go`, and updated Go toolchain gating so NetFlow builds still configure when `ENABLE_PLUGIN_NETFLOW=On` and `ENABLE_PLUGIN_GO=Off`. Verified generated `cmake_install.cmake` now places downloader assets under `plugin-netflow` only.
- 2026-02-04: **Completed Costa request** — updated `.github/workflows/snmp-netflow-sim-tests.yml` to run only when SNMP/NetFlow/IPFIX/sFlow-related files change; improved snmpsim fixture selection (LLDP `lldpRemSysName` + CDP `cdpCacheDeviceId`) for reliable CI; fixed LLDP/CDP profiles to use valid scalar metrics and ensured `lldpRemEntry`/`cdpCacheEntry` use existing numeric columns so topology rows are emitted. Manual integration tests run locally with snmpsim + stress pcaps: `go test -tags=integration ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow` **PASS**.
- 2026-02-04: Ran `gofmt -w` across all Go files in the repo as requested.
- 2026-02-04: Re-verified implementation completeness; ran unit + integration-tag tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`, `go test -count=1 -tags=integration ./plugin/go.d/collector/netflow ./plugin/go.d/collector/snmp`, and `go test -v -count=1 -tags=integration ./plugin/go.d/collector/snmp`). NetFlow integration replay passed; SNMP integration is env-gated and skipped without `NETDATA_SNMPSIM_ENDPOINT`/`NETDATA_SNMPSIM_COMMUNITIES`. Verified deps `gopacket` and `goflow2/v2` are latest via `go list -m -u ...` (no updates).
- 2026-02-04: Reviewed current state and re-ran unit + integration-tag tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge` and `go test -count=1 -tags=integration ./plugin/go.d/collector/netflow ./plugin/go.d/collector/snmp`); all pass. No code changes required.
- 2026-02-04: Marked Plan items 9-11 as ✅ DONE; re-ran targeted Go tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass.
- 2026-02-04: Completed pending scope: expanded LLDP/CDP profiles (caps + mgmt address tables + stats), extended topology cache/types for mgmt addresses/capabilities, imported **116** LibreNMS snmprec fixtures + **36** Akvorado pcaps with updated attribution, added integration tests (`//go:build integration`) and CI workflow `snmp-netflow-sim-tests.yml` (snmpsim + NetFlow stress pcaps), expanded unit tests to cover all fixtures. Ran `go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge` (pass).
- 2026-02-04: Re-ran targeted Go tests with real-device fixtures (snmp, netflow, jobmgr, funcapi, topology-flow-merge); all pass. No code changes needed.
- 2026-02-04: Reviewed SNMP topology + NetFlow/IPFIX/sFlow modules, schema types, and merge tool; re-ran real-device tests (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass. No code changes required.
- 2026-02-04: Reviewed repo for completeness (docs/config/schema/function types) and re-ran targeted Go tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass. No code changes required.
- 2026-02-04: Re-ran targeted Go tests with real-device fixtures (`go test -count=1 ./plugin/go.d/collector/snmp ./plugin/go.d/collector/netflow ./plugin/go.d/agent/jobmgr ./pkg/funcapi ./tools/topology-flow-merge`); all pass. Verified module versions via `go list -m -u github.com/google/gopacket github.com/netsampler/goflow2/v2` (no updates available).
- 2026-02-04: Completed NetFlow collector (v5/v9/IPFIX/sFlow) + flow aggregation + flows function + charts + docs/config/schema + merge CLI; added real-device testdata (LibreNMS snmprec + Akvorado pcaps) with attribution.
- 2026-02-03: Implemented topology/flows response types + schema support + SNMP topology function/cache + LLDP/CDP profiles + charts + docs + tests.

---

## PR 21702 CI & Comments Triage (2026-03-02)

### TL;DR
- User request: check PR `https://github.com/netdata/netdata/pull/21702` because CI fails and there are many comments.
- Objective: produce a concrete, file-level/actionable triage, then fix what can be fixed in this repo.

### Analysis (in progress)
- Existing branch/work appears to already include topology + netflow code used by PR 21702.
- Need fresh GitHub evidence for:
  - failing CI checks and exact failing jobs/log excerpts,
  - unresolved review comments (especially blocking / requested changes),
  - whether failures are code regressions vs flaky/environmental.
- 2026-03-25: For the journal split specifically:
  - `#21962` currently contains the `jf/journal_file` entry-offset cache optimization and also some mirrored `journal-core` changes (GitHub files list includes `src/crates/journal-core/src/file/file.rs`, `reader.rs`, `writer.rs`).
  - `topology-flows` still carries additional journal work on top of that: payload traversal fast paths in `src/crates/journal-core/src/file/file.rs`, `object.rs`, `mmap.rs`, `guarded_cell.rs`, `value_guard.rs`; session/filter plumbing in `src/crates/journal-core/src/file/reader.rs` and `src/crates/jf/journal_file/src/filter.rs`; projected output field support in `src/crates/journal-engine/src/logs/query.rs`; and timestamp override/restart-seeding work in `src/crates/journal-log-writer/src/log/mod.rs`.
  - This is not a single optimization patch. It is a mixed bundle of: reader optimizations, reader/session API changes, query projection behavior, and writer semantics.

### Decisions
- 2026-03-02 (Costa): `topology-ip-intel-downloader` must be part of the `plugin-netflow` plugin/package (not `netdata` and not `plugin-go`).
- 2026-03-26 (Costa): For `netflow-plugin` protobuf generation, prefer `PROTOC` if set, else use vendored `protoc` where supported, else fall back to `protoc` from `PATH`; only fail with a clear error if neither vendored nor system `protoc` is available.
- If conflicting reviewer asks require design-level tradeoffs, prepare numbered options with implications/risks and ask Costa.
- 2026-03-25: Pending Costa decision: how broad should the standalone journal-reader PR become?
  - Evidence to use for decision:
    - Reader-only/core optimization path: `src/crates/journal-core/src/file/file.rs`, `src/crates/journal-core/src/file/object.rs`, `src/crates/journal-core/src/file/reader.rs`, `src/crates/jf/journal_file/src/file.rs`, `src/crates/jf/journal_file/src/object.rs`, `src/crates/jf/journal_file/src/reader.rs`.
    - Broader mixed-scope path adds: `src/crates/journal-core/src/file/mmap.rs`, `src/crates/journal-core/src/file/guarded_cell.rs`, `src/crates/journal-core/src/file/value_guard.rs`, `src/crates/journal-engine/src/logs/query.rs`, `src/crates/journal-log-writer/src/log/mod.rs`, and related tests.

### Plan
1. Pull PR metadata, check statuses, and failing check details.
2. Pull all issue/review comments and classify by severity and affected files.
3. Reproduce failing checks locally where possible.
4. Implement scoped fixes and rerun targeted tests.
5. Summarize what remains (if any) and recommend merge path.
6. For the journal split, separate:
   - changes that are true standalone journal-reader improvements,
   - changes that are journal infrastructure but not reader-focused,
   - changes that are netflow/query/writer semantics and should stay out unless Costa explicitly widens PR `#21962`.

### Implied Decisions
- Use this existing TODO file (`TODO-SNMP-TOPOLOGY-NETFLOW.md`) as the task tracker because it already tracks the same feature/PR scope.

### Testing Requirements
- Reproduce failing CI tasks locally when practical.
- Run targeted Go tests for touched packages and integration tags where relevant.

### Documentation Updates Required
- Re-check if code fixes alter behavior, APIs, or schemas documented in:
  - `src/go/plugin/go.d/collector/snmp/README.md`
  - `src/go/plugin/go.d/collector/netflow/README.md`
  - `src/plugins.d/FUNCTION_UI_SCHEMA.json` docs/tooling notes

---

## Codebase Analysis (Agent, 2026-02-03)

### Functions infrastructure (facts)

- go.d function responses now resolve `type` from `MethodConfig.ResponseType` or `FunctionResponse.ResponseType` (defaults to `"table"`). Evidence: `src/go/plugin/go.d/agent/jobmgr/funcshandler.go` (resolveResponseType).
- The **Functions UI schema** accepts custom `type` values for topology and flows via dedicated definitions. Evidence: `src/plugins.d/FUNCTION_UI_SCHEMA.json` (topology_response / flows_response).
- `funcapi.FunctionResponse` supports **columns/data + charting** (table) or **custom data payloads** when `ResponseType` is non-table. Evidence: `src/go/pkg/funcapi/response.go`.
- The **functions-validation** tool validates responses against `FUNCTION_UI_SCHEMA.json`. Evidence: `src/go/tools/functions-validation/README.md:67-71`.

### SNMP collector (facts)

- The SNMP module registers **Methods/MethodHandler** and uses a **funcRouter** for function dispatch. Evidence: `src/go/plugin/go.d/collector/snmp/collector.go:26-36`, `src/go/plugin/go.d/collector/snmp/func_router.go:20-64`.
- The existing **SNMP interfaces function** is a good reference for new function handlers (cache → build columns/data → return `FunctionResponse`). Evidence: `src/go/plugin/go.d/collector/snmp/func_interfaces.go:31-113`.
- SNMP collection uses **ddsnmp** profile metrics, and the **interface cache** is populated inside table-metric processing (`collectProfileTableMetrics`). Evidence: `src/go/plugin/go.d/collector/snmp/collect_snmp.go:24-94`.
- SNMP devices are typically **vnodes**; labels include sysinfo + metadata (sys_object_id, name, vendor, model, etc.), which can be used for deterministic correlation. Evidence: `src/go/plugin/go.d/collector/snmp/collect.go:119-190`.
- `snmputils.SysInfo` already provides **sysName/sysDescr/sysObjectID/sysLocation** and metadata. Evidence: `src/go/plugin/go.d/pkg/snmputils/sysinfo.go:20-85`.

### SNMP profiles & tags (facts)

- Profiles support **metric_tags** and cross-table tags; `_std-if-mib.yaml` shows tags derived from **ifTable/ifXTable**. Evidence: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-if-mib.yaml:58-118`.
- Tag processing converts PDU values to **strings** and stores them as tags. This can carry LLDP/CDP string fields. Evidence: `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/tag_processor.go:51-84`.
- LLDP/CDP standard profiles now exist and are used for topology discovery. Evidence: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-lldp-mib.yaml`, `_std-cdp-mib.yaml`.

### NetFlow/IPFIX (facts)

- NetFlow/IPFIX/sFlow collector exists under `src/go/plugin/go.d/collector/netflow` with decoding, aggregation, charts, and flows function responses.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           NETDATA CLOUD                                  │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Aggregation Layer                                               │   │
│  │  - Query all agents for topology/flow data                       │   │
│  │  - Merge topologies (deduplicate nodes, validate links)          │   │
│  │  - Aggregate flows (sum by dimension, normalize sampling)        │   │
│  │  - Present unified visualization                                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │ Query API
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐       ┌───────────────┐       ┌───────────────┐
│   Agent A     │       │   Agent B     │       │   Agent C     │
│ (Data Center) │       │ (Branch Office)│      │ (Cloud Region)│
├───────────────┤       ├───────────────┤       ├───────────────┤
│ SNMP Jobs:    │       │ SNMP Jobs:    │       │ SNMP Jobs:    │
│ - Core Router │       │ - Branch SW   │       │ - Cloud LB    │
│ - DC Switches │       │ - Branch FW   │       │ - Cloud FW    │
│ - Firewalls   │       │               │       │               │
├───────────────┤       ├───────────────┤       ├───────────────┤
│ Local Topology│       │ Local Topology│       │ Local Topology│
│ (partial view)│       │ (partial view)│       │ (partial view)│
├───────────────┤       ├───────────────┤       ├───────────────┤
│ Flow Collector│       │ Flow Collector│       │ Flow Collector│
│ (local flows) │       │ (local flows) │       │ (local flows) │
└───────────────┘       └───────────────┘       └───────────────┘
```

---

## Part 1: SNMP Topology Map (Distributed)

### 1.1 Design Principles

1. **Each agent builds its LOCAL topology** from devices it monitors
2. **Topology data is exposed via API** for Cloud to query
3. **Cloud merges all agent topologies** into unified view
4. **Globally unique identifiers** enable cross-agent correlation

### 1.2 Aggregation Challenges

| Challenge | Problem | Solution |
|-----------|---------|----------|
| **Same device, multiple agents** | Device monitored by 2+ agents | Use chassis ID / sysObjectID as global key, deduplicate |
| **Cross-agent links** | Agent A sees link to device monitored by Agent B | Cloud matches by remote chassis ID |
| **Bidirectional validation** | Link reported by one end, other end on different agent | Cloud validates across agent boundaries |
| **Inconsistent naming** | Same device has different sysName per agent | Canonical ID from chassis ID, aliases tracked |
| **Partial visibility** | Agent sees neighbor but doesn't monitor it | Mark as "discovered" vs "monitored" |

### 1.3 Data Model (Aggregation-Friendly)

```go
// Per-agent topology data (exposed via API)
type AgentTopology struct {
    AgentID       string                  // Netdata agent unique ID
    CollectedAt   time.Time               // Timestamp of collection
    Devices       []TopologyDevice        // Devices this agent monitors
    DiscoveredLinks []TopologyLink        // Links discovered via LLDP/CDP
}

// Device with globally unique identifier
type TopologyDevice struct {
    // === GLOBAL IDENTIFIERS (for cross-agent matching) ===
    ChassisID       string              // LLDP chassis ID (MAC or other)
    ChassisIDType   string              // "mac", "networkAddress", "hostname", etc.
    SysObjectID     string              // SNMP sysObjectID (vendor/model)

    // === LOCAL IDENTIFIERS ===
    AgentJobID      string              // SNMP job ID on this agent
    ManagementIP    string              // IP used to poll this device

    // === DEVICE INFO ===
    SysName         string              // SNMP sysName
    SysDescr        string              // SNMP sysDescr
    SysLocation     string              // SNMP sysLocation
    Capabilities    []string            // "router", "bridge", "station", etc.
    Vendor          string              // Derived from sysObjectID
    Model           string              // Derived from sysDescr or profile

    // === INTERFACES ===
    Interfaces      []TopologyInterface // All interfaces with neighbor info

    // === ROUTING TOPOLOGY ===
    BGPPeers        []BGPPeerInfo       // BGP neighbor relationships
    OSPFNeighbors   []OSPFNeighborInfo  // OSPF adjacencies
}

// Interface with neighbor discovery data
type TopologyInterface struct {
    // === LOCAL INTERFACE ===
    IfIndex         int
    IfName          string              // e.g., "GigabitEthernet0/1"
    IfDescr         string
    IfAlias         string              // User-configured description
    IfType          int                 // IANAifType
    IfSpeed         uint64              // bits/sec
    IfOperStatus    string              // up/down/testing
    IfAdminStatus   string
    IfPhysAddress   string              // MAC address

    // === LLDP NEIGHBOR (if discovered) ===
    LLDPNeighbor    *LLDPNeighborInfo

    // === CDP NEIGHBOR (if discovered) ===
    CDPNeighbor     *CDPNeighborInfo
}

// LLDP neighbor info (enables cross-agent correlation)
type LLDPNeighborInfo struct {
    // === REMOTE DEVICE IDENTIFIERS ===
    RemChassisID      string            // Key for cross-agent matching!
    RemChassisIDType  string
    RemSysName        string
    RemSysDescr       string
    RemSysCapabilities []string

    // === REMOTE PORT ===
    RemPortID         string
    RemPortIDType     string
    RemPortDescr      string

    // === MANAGEMENT ADDRESS ===
    RemManagementAddr string
}

// CDP neighbor info
type CDPNeighborInfo struct {
    RemDeviceID       string            // Cisco device ID
    RemPlatform       string            // e.g., "cisco WS-C3750-48P"
    RemPortID         string            // e.g., "GigabitEthernet1/0/1"
    RemCapabilities   []string
    RemManagementAddr string
    RemNativeVLAN     int
}

// Link representation (for API/Cloud)
type TopologyLink struct {
    // === SOURCE (this agent's device) ===
    SrcChassisID    string              // Global device ID
    SrcIfIndex      int
    SrcIfName       string
    SrcAgentID      string              // Which agent monitors source

    // === TARGET (may be on different agent) ===
    DstChassisID    string              // Global device ID (for matching)
    DstPortID       string              // As reported by LLDP/CDP
    DstSysName      string              // As reported by LLDP/CDP
    DstAgentID      string              // Empty if not monitored by any agent

    // === LINK METADATA ===
    Protocol        string              // "lldp", "cdp", "bgp", "ospf"
    DiscoveredAt    time.Time
    LastSeen        time.Time

    // === VALIDATION (set by Cloud after aggregation) ===
    Bidirectional   bool                // Both ends confirmed
    Validated       bool                // Cloud has matched both ends
}
```

### 1.4 Cloud Aggregation Logic

```
CLOUD TOPOLOGY MERGE ALGORITHM:

1. COLLECT
   - Query /api/v3/topology from all connected agents
   - Each agent returns AgentTopology with its local view

2. BUILD DEVICE INDEX
   FOR each agent's devices:
     key = normalize(ChassisID, ChassisIDType)
     IF key exists in global_devices:
       MERGE device info (prefer newest, keep all management IPs)
       ADD agent to device.MonitoredByAgents[]
     ELSE:
       INSERT into global_devices

3. PROCESS LINKS
   FOR each agent's discovered links:
     src_key = normalize(link.SrcChassisID)
     dst_key = normalize(link.DstChassisID)

     link_key = canonical_link_key(src_key, dst_key)

     IF link_key exists in global_links:
       // Link already known from other direction
       SET link.Bidirectional = true
       SET link.Validated = true
     ELSE:
       INSERT into global_links
       // Check if destination device is monitored
       IF dst_key in global_devices:
         SET link.DstAgentID = device.MonitoredByAgents[0]
       ELSE:
         // Discovered but not monitored - mark as edge device
         CREATE placeholder device (discovered=true, monitored=false)

4. VALIDATE
   FOR each link:
     IF NOT link.Validated:
       // Only one end reported - might be stale or asymmetric
       IF link.LastSeen < (now - LLDP_HOLDTIME):
         MARK as potentially_stale
       ELSE:
         MARK as unidirectional (warning)

5. BUILD GRAPH
   - Create nodes for all devices
   - Create edges for all validated links
   - Annotate with metrics (traffic, status) from agents
```

### 1.5 Agent API Endpoint

```
GET /api/v3/topology

Response:
{
  "agent_id": "abc123",
  "collected_at": "2025-01-15T10:30:00Z",
  "devices": [...],
  "links": [...],
  "stats": {
    "devices_monitored": 15,
    "links_discovered": 42,
    "lldp_neighbors": 38,
    "cdp_neighbors": 12,
    "bgp_peers": 8,
    "ospf_neighbors": 6
  }
}
```

### 1.6 Implementation Components

| Component | Location | Description |
|-----------|----------|-------------|
| LLDP Profile | `snmp.profiles/default/_std-lldp-mib.yaml` | Collect LLDP neighbor data |
| CDP Profile | `snmp.profiles/default/_std-cdp-mib.yaml` | Collect CDP neighbor data |
| Topology Builder | `collector/snmp/topology/` | Build local topology from SNMP data |
| Topology Cache | `collector/snmp/topology/cache.go` | Cache topology between polls |
| API Handler | `collector/snmp/func_topology.go` | Expose topology via function API |
| Cloud Aggregator | Netdata Cloud | Merge agent topologies |

### 1.7 SNMP Profiles Needed

**LLDP Profile (`_std-lldp-mib.yaml`):**
```yaml
# LLDP-MIB (IEEE 802.1AB)
metrics:
  # Local system info
  - MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.3.7  # lldpLocPortTable
      name: lldpLocPortTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.3.7.1.3
        name: lldpLocPortId
      - OID: 1.0.8802.1.1.2.1.3.7.1.4
        name: lldpLocPortDesc
    metric_tags:
      - column:
          OID: 1.0.8802.1.1.2.1.3.7.1.2
          name: lldpLocPortIdSubtype
        tag: port_id_subtype

  # Remote neighbor info (critical for topology)
  - MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.4.1  # lldpRemTable
      name: lldpRemTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.4.1.1.5
        name: lldpRemChassisId
      - OID: 1.0.8802.1.1.2.1.4.1.1.7
        name: lldpRemPortId
      - OID: 1.0.8802.1.1.2.1.4.1.1.8
        name: lldpRemPortDesc
      - OID: 1.0.8802.1.1.2.1.4.1.1.9
        name: lldpRemSysName
      - OID: 1.0.8802.1.1.2.1.4.1.1.10
        name: lldpRemSysDesc
    metric_tags:
      - column:
          OID: 1.0.8802.1.1.2.1.4.1.1.4
          name: lldpRemChassisIdSubtype
        tag: chassis_id_subtype
      - column:
          OID: 1.0.8802.1.1.2.1.4.1.1.6
          name: lldpRemPortIdSubtype
        tag: port_id_subtype
```

---

## Part 2: NetFlow/IPFIX (Distributed)

### 2.1 Design Principles

1. **Each agent runs a flow collector** listening on configured UDP ports
2. **Flows are aggregated locally** into time-bucketed summaries
3. **Flow summaries exposed via API** for Cloud to query
4. **Cloud aggregates across agents** avoiding double-counting

### 2.2 Aggregation Challenges

| Challenge | Problem | Solution |
|-----------|---------|----------|
| **Same flow, multiple exporters** | Transit traffic exported by ingress AND egress routers | Deduplicate by flow key + direction, or use ingress-only |
| **Sampling rate variance** | Router A samples 1:1000, Router B samples 1:100 | Normalize all flows to estimated actual counts |
| **Cross-agent flows** | Flow enters at Agent A's router, exits at Agent B's | Track by flow key, sum bytes/packets, don't double-count |
| **Time alignment** | Agents have different clock skew | Use flow timestamps, align to common buckets |
| **Cardinality explosion** | Too many unique src/dst combinations | Pre-aggregate to top-N per dimension |

### 2.3 Data Model (Aggregation-Friendly)

```go
// Per-agent flow summary (exposed via API)
type AgentFlowSummary struct {
    AgentID       string
    PeriodStart   time.Time           // Start of aggregation window
    PeriodEnd     time.Time           // End of aggregation window
    Exporters     []ExporterInfo      // Flow exporters sending to this agent

    // Pre-aggregated summaries (for Cloud to merge)
    ByProtocol    []ProtocolSummary
    ByASPair      []ASPairSummary     // Top N AS pairs
    ByInterface   []InterfaceSummary
    ByCountry     []CountrySummary    // If GeoIP enabled
    TopTalkers    []TalkerSummary     // Top N by bytes

    // Raw aggregates for flexible Cloud queries
    FlowBuckets   []FlowBucket        // Time-bucketed flow data
}

// Exporter metadata (for Cloud correlation)
type ExporterInfo struct {
    ExporterIP      string
    ExporterName    string            // From SNMP sysName if available
    SamplingRate    uint32            // 1:N, used for normalization
    FlowVersion     string            // "netflow_v5", "netflow_v9", "ipfix", "sflow"
    InterfaceMap    map[uint32]string // ifIndex -> ifName (from SNMP)
}

// Flow bucket (time-aggregated, dimension-grouped)
type FlowBucket struct {
    Timestamp     time.Time           // Bucket start time
    Duration      time.Duration       // Bucket size (e.g., 1 minute)

    // === FLOW KEY (for deduplication across agents) ===
    FlowKey       FlowKey

    // === COUNTERS (normalized for sampling) ===
    Bytes         uint64              // Normalized byte count
    Packets       uint64              // Normalized packet count
    Flows         uint64              // Number of distinct flows

    // === SAMPLING INFO ===
    RawBytes      uint64              // Before normalization
    SamplingRate  uint32              // Rate used for normalization

    // === SOURCE INFO ===
    ExporterIP    string
    AgentID       string
    Direction     string              // "ingress" or "egress"
}

// Flow key for deduplication
type FlowKey struct {
    // 5-tuple (hashed for cardinality control)
    SrcAddrPrefix   string            // /24 for IPv4, /48 for IPv6
    DstAddrPrefix   string
    SrcPort         uint16            // 0 if aggregated
    DstPort         uint16            // 0 if aggregated
    Protocol        uint8

    // Routing info
    SrcAS           uint32
    DstAS           uint32

    // Interface info (for per-interface views)
    InIfIndex       uint32
    OutIfIndex      uint32

    // Exporter (for attribution)
    ExporterIP      string
}

// Pre-computed summaries for common queries
type ProtocolSummary struct {
    Protocol    uint8               // TCP=6, UDP=17, etc.
    Bytes       uint64
    Packets     uint64
    Flows       uint64
}

type ASPairSummary struct {
    SrcAS       uint32
    DstAS       uint32
    Bytes       uint64
    Packets     uint64
}

type InterfaceSummary struct {
    ExporterIP  string
    IfIndex     uint32
    IfName      string              // Enriched from SNMP
    Direction   string              // "in" or "out"
    Bytes       uint64
    Packets     uint64
}

type TalkerSummary struct {
    Address     string              // IP or prefix
    Direction   string              // "src" or "dst"
    Bytes       uint64
    Packets     uint64
    TopPorts    []PortCount         // Top N ports
}
```

### 2.4 Cloud Aggregation Logic

```
CLOUD FLOW AGGREGATION ALGORITHM:

1. COLLECT
   - Query /api/v3/flows?period=5m from all connected agents
   - Each agent returns AgentFlowSummary for the period

2. NORMALIZE
   FOR each agent's flow buckets:
     IF bucket.SamplingRate != 1:
       bucket.Bytes = bucket.RawBytes * bucket.SamplingRate
       bucket.Packets = bucket.RawPackets * bucket.SamplingRate

3. DEDUPLICATE (avoid double-counting transit traffic)

   Option A: Ingress-only accounting
     FOR each bucket:
       IF bucket.Direction == "egress":
         SKIP (only count ingress)

   Option B: Flow-key based deduplication
     flow_seen = {}
     FOR each bucket:
       key = hash(bucket.FlowKey, bucket.Timestamp)
       IF key in flow_seen:
         // Same flow reported by multiple exporters
         // Keep the one with better sampling (lower ratio)
         IF bucket.SamplingRate < flow_seen[key].SamplingRate:
           REPLACE flow_seen[key] with bucket
       ELSE:
         flow_seen[key] = bucket

4. AGGREGATE BY DIMENSION

   // Global protocol breakdown
   protocol_totals = {}
   FOR each bucket:
     protocol_totals[bucket.Protocol] += bucket.Bytes

   // Global AS-pair matrix
   as_matrix = {}
   FOR each bucket:
     as_matrix[(bucket.SrcAS, bucket.DstAS)] += bucket.Bytes

   // Per-interface totals
   interface_totals = {}
   FOR each bucket:
     key = (bucket.ExporterIP, bucket.IfIndex, bucket.Direction)
     interface_totals[key] += bucket.Bytes

5. BUILD VISUALIZATION
   - Sankey diagram from AS-pair matrix
   - Time-series from bucket timestamps
   - Top-N tables from sorted aggregates
```

### 2.5 Agent API Endpoint

```
GET /api/v3/flows?period=5m&dimensions=protocol,as_pair

Response:
{
  "agent_id": "abc123",
  "period_start": "2025-01-15T10:25:00Z",
  "period_end": "2025-01-15T10:30:00Z",
  "exporters": [
    {
      "ip": "10.0.0.1",
      "name": "core-router-01",
      "sampling_rate": 1000,
      "flow_version": "ipfix"
    }
  ],
  "by_protocol": [
    {"protocol": 6, "bytes": 1234567890, "packets": 987654},
    {"protocol": 17, "bytes": 567890123, "packets": 456789}
  ],
  "by_as_pair": [
    {"src_as": 15169, "dst_as": 32934, "bytes": 999999999},
    ...
  ],
  "top_talkers": [...],
  "stats": {
    "total_bytes": 9999999999,
    "total_packets": 8888888,
    "total_flows": 123456,
    "exporters_active": 3
  }
}
```

### 2.6 Implementation Components

| Component | Location | Description |
|-----------|----------|-------------|
| Flow Collector | `collector/netflow/` | UDP listener for NetFlow/IPFIX/sFlow |
| Flow Parser | `collector/netflow/parser/` | Protocol-specific parsers |
| Flow Aggregator | `collector/netflow/aggregator/` | Time-bucket aggregation |
| SNMP Enricher | `collector/netflow/enricher/` | Add interface names from SNMP |
| GeoIP Enricher | `collector/netflow/enricher/` | Add country/ASN info |
| API Handler | `collector/netflow/func_flows.go` | Expose flow data via function API |
| Cloud Aggregator | Netdata Cloud | Merge flow data from agents |

### 2.7 Flow Collector Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     NETDATA AGENT                               │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                   Flow Collector                          │  │
│  │                                                           │  │
│  │  ┌─────────┐   ┌─────────┐   ┌─────────┐                │  │
│  │  │ UDP     │   │ UDP     │   │ UDP     │                │  │
│  │  │ :2055   │   │ :4739   │   │ :6343   │                │  │
│  │  │NetFlow  │   │ IPFIX   │   │ sFlow   │                │  │
│  │  └────┬────┘   └────┬────┘   └────┬────┘                │  │
│  │       │             │             │                      │  │
│  │       └─────────────┼─────────────┘                      │  │
│  │                     ▼                                    │  │
│  │            ┌────────────────┐                            │  │
│  │            │  Flow Parser   │                            │  │
│  │            │  (goflow2 lib) │                            │  │
│  │            └───────┬────────┘                            │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐      ┌─────────────────┐  │  │
│  │            │   Enricher     │◄─────│  SNMP Collector │  │  │
│  │            │ (ifIndex→name) │      │  (interface map)│  │  │
│  │            └───────┬────────┘      └─────────────────┘  │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐      ┌─────────────────┐  │  │
│  │            │   Enricher     │◄─────│  GeoIP DB       │  │  │
│  │            │ (IP→Country)   │      │  (MaxMind)      │  │  │
│  │            └───────┬────────┘      └─────────────────┘  │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐                            │  │
│  │            │  Aggregator    │                            │  │
│  │            │ (time buckets) │                            │  │
│  │            └───────┬────────┘                            │  │
│  │                    ▼                                     │  │
│  │            ┌────────────────┐                            │  │
│  │            │  Flow Cache    │──────► Metrics             │  │
│  │            │ (ring buffer)  │──────► API /api/v3/flows   │  │
│  │            └────────────────┘                            │  │
│  │                                                           │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Part 3: Shared Considerations

### 3.1 Agent API Requirements

Both features need new API endpoints. Options:

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **Function API** | Extend existing `/api/v3/function` | Consistent with SNMP interfaces API | Limited to GET semantics |
| **New API paths** | Add `/api/v3/topology`, `/api/v3/flows` | Clean, RESTful | More API surface |
| **Streaming API** | WebSocket for real-time updates | Low latency for live view | More complex |

**Recommendation:** Function API initially (consistent with existing pattern), dedicated paths later if needed.

### 3.2 Data Retention

| Data Type | Agent Retention | Purpose |
|-----------|-----------------|---------|
| Topology snapshot | Latest only | Cloud queries on-demand |
| Topology history | 24 hours | Detect topology changes |
| Flow buckets | 1 hour (detailed) | Recent high-resolution data |
| Flow summaries | 24 hours | Historical queries |

### 3.3 Cloud Query Patterns

**Topology:**
```
GET /spaces/{space}/topology
  - Cloud queries all agents in parallel
  - Merges responses
  - Caches merged topology (TTL: 1 minute)
  - Returns unified graph
```

**Flows:**
```
GET /spaces/{space}/flows?period=5m&group_by=as_pair
  - Cloud queries all agents for period
  - Aggregates/deduplicates
  - Returns merged summary
```

---

## Part 4: Decisions Required

### Decisions Confirmed (2026-02-03)

- **Scope:** This repo is **agent-only**. No Cloud backend changes in this repo.
- **Agent Functions:** Agent must expose **Netdata Functions** for **Topology** and **Flows**.
- **Time-series:** Agent **may create time-series** with **proper labels** for deterministic correlation later.
- **Function Schemas:** Function JSON outputs must be **concrete and specific**, designed for **cross-agent aggregation/correlation**. If time-series are created, the JSON schema must **link/refer** to them.
- **Aggregation Prototype:** Build a **Go program** that merges **non-overlapping / partial-overlapping / fully-overlapping** Function JSON outputs into a final topology map. This is a **prototype** to validate aggregation logic before Cloud integration.
- **Schema Infrastructure:** Reuse existing **Function schema infrastructure** (table-view schema). Add **common fields** so UI can identify **topology** schema vs table/logs. Versioning is allowed and **schema differences across agents** must be aggregatable (e.g., v1 + v2).
- **Metric Correlation:** SNMP devices are typically **vnodes**; metrics are **namespaced by node**. Constants like node identifiers can be used for deterministic correlation.
- **Overlap Handling:** With **clear identity**, multi-agent overlap should be manageable; treat it similar to a single agent with full visibility.
- **Decision 1:** **B** - Function schema identification should use **new function types** (e.g., `topology`, `flows`) instead of `table/log`.
- **Decision 2:** NetFlow ingestion will be a **separate netflow module with jobs**, and **each job has its own listener port**.
- **Decision 4:** **A** - Aggregation prototype should live under `src/go/tools/` as a CLI tool.
- **Decision 5:** **A** - Require Cloud for Topology/Flows functions.
- **Decision 6:** **B** - Emit time-series **plus** functions, with strict label discipline.
- **Decision 3:** Use **goflow2** as the flow parsing library.
- **Decision 3 (version):** Use module **`github.com/netsampler/goflow2/v2` @ v2.2.6** (verified via `go list -m github.com/netsampler/goflow2/v2@latest`).
- **Decision 7:** **Hybrid** LLDP/CDP enablement: **vendor-profile explicit + configurable autoprobe** with **in-memory missing‑OID learning** only.

### Decisions Confirmed (2026-02-04)

- **Flow Protocol Scope:** Implement **NetFlow v5, NetFlow v9, IPFIX and sFlow**.
- **Aggregation Key (Agent):** Aggregate by **5‑tuple + AS + in/out interface** when present. Missing fields omitted; agent does **no dedup** across exporters.
- **Bucketing & Retention:** **bucket_duration defaults to update_every**; retain **max_buckets** (default 60). Old buckets are evicted.
- **Clock Skew Handling:** When flow timestamps are **too old** for current retention, **re-bucket to arrival time** and increment `records_too_old` (avoid dropping data).
- **Sampling Normalization:** Apply **per‑record sampling rate** when available; fallback to **exporter override** or **config default**. Preserve `raw_*` fields.
- **Flows Response Payload:** Conform to **FUNCTION_UI_SCHEMA flows_response** with `exporters`, `buckets`, `summaries`, `metrics`.
- **Time‑series:** Emit totals for **bytes/s, packets/s, flows/s**, plus **dropped records**.
- **sFlow Support:** Decode **sFlow v5** via `goflow2/v2/decoders/sflow`, detect by 32‑bit version (0x00000005). Map SampledIPv4/IPv6 and SampledHeader into flow records; use sample `sampling_rate`, `input/output` ifIndex. (Exporter IP prefers sFlow agent IP if present.)
- **IPFIX Field Coverage:** Add IPFIX field mappings for bytes/packets/flows, protocol, ports, addresses, prefix lengths, interfaces, AS numbers, direction, and flow start/end (seconds/milliseconds/sysUpTime) where available.

### Decision 1: Topology Identifier Strategy

**Question:** How to uniquely identify devices across agents?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A** | Chassis ID (MAC/LLDP) | Standard, unique | Not all devices support LLDP |
| **B** | Management IP | Always available | IPs can change, NAT issues |
| **C** | sysObjectID + sysName | Works without LLDP | Name collisions possible |
| **D** | Composite (A + B + C) | Most robust | Complex matching logic |

**Recommendation:** D - Use chassis ID when available, fall back to composite

### Decision 2: Flow Deduplication Strategy

**Question:** How to handle same flow seen by multiple exporters?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A** | Ingress-only | Simple, no double-count | Loses egress perspective |
| **B** | Flow-key dedup | Accurate | Complex, needs timestamps |
| **C** | Per-exporter only | No dedup needed | Double-counts transit |
| **D** | User-configurable | Flexible | User must understand |

**Recommendation:** A initially (simple), B later

### Decision 3: Flow Aggregation Granularity

**Question:** What dimensions to aggregate by on agents?

| Option | Description | Storage | Query Flexibility |
|--------|-------------|---------|-------------------|
| **A** | Minimal (protocol only) | Low | Limited |
| **B** | Standard (proto + AS + interface) | Medium | Good for most cases |
| **C** | Full (proto + AS + prefix + port) | High | Maximum flexibility |
| **D** | Configurable per-agent | Varies | Optimal but complex |

**Recommendation:** B - Standard dimensions, configurable top-N limits

### Decision 4: SNMP-Flow Integration

**Question:** How to correlate flow interfaces with SNMP interface names?

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| **A** | Same agent monitors both | Direct correlation | Limits deployment flexibility |
| **B** | Cross-agent lookup via Cloud | Flexible deployment | Latency, complexity |
| **C** | Export mapping via flow exporter config | Simple | Manual config required |
| **D** | Agent-local SNMP query to exporter | Works if exporter allows | Extra SNMP load |

**Recommendation:** A initially, D as enhancement

### Decision 5: Implementation Priority

**Question:** Which to implement first?

| Option | Description |
|--------|-------------|
| **A** | Topology first (LLDP/CDP profiles + API) |
| **B** | Flows first (collector + aggregation + API) |
| **C** | Both in parallel |

**Recommendation:** A - Topology is lower effort and builds on existing SNMP infrastructure

### Decision 6: Function Schema Identification Strategy (Topology/Flows)

**Background:** go.d functions are wrapped with `type: "table"` in the job manager (`src/go/plugin/go.d/agent/jobmgr/funcshandler.go:153-196`). The Functions UI schema only accepts `type: "table"` or `type: "log"` for data responses (`src/plugins.d/FUNCTION_UI_SCHEMA.json:189-195`).  
We still need **common fields** so UI/Cloud can detect **topology** vs **flows** schemas and handle **versioning**.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Keep `type: "table"` and add **extra fields** (e.g., `schema_id`, `schema_version`, `schema_kind`) | No schema change; works with existing validator and wrapper | UI/Cloud must rely on extra fields; schema meaning not enforced |
| **B** | Extend function type to `topology`/`flows` (update wrapper + schema) | Explicit type; clear UI branching | Requires changes in `funcshandler` + `FUNCTION_UI_SCHEMA.json`; risk of tool/UI incompatibility |
| **C** | Keep `type: "table"` and **no extra fields** (encode everything as table only) | Zero infra change | UI cannot distinguish schema; versioning is implicit and fragile |

**Recommendation:** A — minimal infra change, compatible with current schema, still allows explicit schema/version fields.

### Decision 7: NetFlow Collector Job Model

**Background:** go.d modules are **job-based**; functions are bound to job instances (see SNMP router pattern in `src/go/plugin/go.d/collector/snmp/func_router.go:20-64`). There is **no existing UDP listener collector** in go.d, so we must pick a model.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | **One job = one UDP listener** (addr/port per job) | Aligns with go.d model; simple config; multiple listeners allowed | Multiple listeners if many ports; duplication of aggregators |
| **B** | **One listener shared by multiple jobs** (jobs are filters) | Centralized listener | Requires cross-job shared state; complex lifecycle |
| **C** | **Single global listener** (one job only) | Simplest runtime | Limits flexibility; not aligned with multi-job config patterns |

**Recommendation:** A — matches go.d patterns and avoids shared-state complexity.

### Decision 8: Flow Parsing Library

**Background:** There is **no netflow/ipfix/sflow code** in this repo; we must add a parser library and dependencies.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Use `goflow2` (NetFlow v5/v9/IPFIX) + separate sFlow lib if needed | Mature, widely used | New deps; need sFlow coverage |
| **B** | Adapt Akvorado decoder code (AGPL, GPL-compatible) | Broad protocol coverage | Porting effort; license obligations (AGPL) |
| **C** | Write minimal parsers in-house (v5/v9 first) | Full control | High effort; higher bug risk |

**Recommendation:** A initially (fastest to a working collector), then evaluate sFlow coverage.

### Decision 9: Aggregation Prototype Placement

**Background:** This repo already has **Go tools** under `src/go/tools/` (e.g., functions-validation). There are also binaries in `src/go/cmd/`.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Add to `src/go/tools/topology-aggregator/` (CLI tool) | Fits tooling pattern; not part of agent runtime | Not built by default |
| **B** | Add to `src/go/cmd/topology-aggregator/` | Produces binary; easier reuse | Expands build/test matrix |
| **C** | Add under `collector/snmp/` as test helper | Close to SNMP code | Hard to reuse for Cloud later |

**Recommendation:** A — aligns with existing tooling and keeps runtime clean.

### Decision 10: Require Cloud for Topology/Flows Functions

**Background:** `MethodConfig.RequireCloud` controls access flags when functions are registered (`src/go/plugin/go.d/agent/jobmgr/manager.go:148-162`).

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | `RequireCloud = true` | Limits access; aligns with Cloud-centric usage | Local-only UI access blocked |
| **B** | `RequireCloud = false` | Available locally for debugging | Might expose sensitive topology/flow data locally |

**Recommendation:** A for flows (sensitive), **B or A** for topology depending on sensitivity policy.

### Decision 11: Time-series Emission for Topology/Flows

**Background:** Functions can return JSON only, but agents may also emit time-series for correlation. SNMP vnodes provide stable labeling (`src/go/plugin/go.d/collector/snmp/collect.go:140-190`).

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | **Functions only**, no extra charts | Minimal storage/cost | No time-series correlation; less historical context |
| **B** | **Functions + time-series** (key metrics only) | Enables correlation and dashboards | Higher cardinality risk; more design work |

**Recommendation:** B, but **limit** to essential metrics and strong label discipline.

### Decision 12: How to Enable LLDP/CDP Profiles

**Background:** Profiles are matched via selectors; base profiles are applied through `extends` (e.g., `arista-switch.yaml` extends `_std-if-mib.yaml`). LLDP/CDP profiles won’t apply unless explicitly extended.

| Option | Description | Pros | Cons / Risks |
|--------|-------------|------|--------------|
| **A** | Add `_std-lldp-mib.yaml` / `_std-cdp-mib.yaml` and **extend** them in all relevant vendor profiles | Explicit control; predictable | Many profile edits; higher maintenance |
| **B** | Create **generic selectors** for LLDP/CDP profiles so they apply broadly | Minimal profile changes | Risk: collect LLDP/CDP from devices that don’t support it (extra SNMP load) |
| **C** | Add **manual_profiles** guidance only (user opt-in) | No broad changes | Requires user configuration; fewer devices covered |

**Recommendation:** A (explicit) to avoid accidental SNMP load; we can later add B as an opt-in toggle.

---

## Part 5: Implementation Phases

**Note:** This repo is **agent-only**. Cloud aggregation phases are **out of scope** here; we will only build the **aggregation prototype tool** in Go for validation.

### Phase 1: SNMP Topology (Foundation)

1. Create LLDP profile (`_std-lldp-mib.yaml`)
2. Create CDP profile (`_std-cdp-mib.yaml`)
3. Build topology data structures
4. Implement local topology builder
5. Add `/api/v3/function?function=topology` endpoint
6. Document API for Cloud team

**Deliverable:** Each agent exposes local topology via API

### Phase 2: Flow Collection (Foundation)

1. Add flow collector package using goflow2
2. Implement UDP listeners (configurable ports)
3. Build flow aggregator with time buckets
4. Integrate with SNMP for interface enrichment
5. Add `/api/v3/function?function=flows` endpoint
6. Document API for Cloud team

**Deliverable:** Each agent collects and exposes flow data via API

### Phase 3: Cloud Aggregation (Topology)

1. Implement topology query to all agents
2. Build merge algorithm (device dedup, link validation)
3. Create unified topology visualization
4. Add topology diff/change detection

**Deliverable:** Cloud shows merged topology across all agents

### Phase 4: Cloud Aggregation (Flows)

1. Implement flow query to all agents
2. Build deduplication logic
3. Create aggregated views (Sankey, time-series)
4. Add drill-down capabilities

**Deliverable:** Cloud shows aggregated flow data across all agents

---

## References

### SNMP Topology
- LLDP-MIB: IEEE 802.1AB-2016
- CISCO-CDP-MIB: Cisco proprietary
- RFC 2922: PTOPO-MIB (Physical Topology)

### NetFlow/IPFIX
- [goflow2](https://github.com/netsampler/goflow2) - BSD-3 licensed Go library
- RFC 3954: NetFlow v9
- RFC 7011-7015: IPFIX
- RFC 3176: sFlow

### Similar Systems
- [Akvorado](https://github.com/akvorado/akvorado) - Centralized flow collector
- [ntopng](https://www.ntop.org/products/traffic-analysis/ntop/) - Network traffic analysis
- [pmacct](http://www.pmacct.net/) - Network accounting

---

## Part 6: Testing Strategy (E2E & CI)

### 6.1 Current Testing Status

**What exists:**
- Unit tests with gosnmp `MockHandler` (gomock-based)
- 18+ test files covering SNMP collector components
- No integration tests, no simulators, no real device data

**What's needed for topology + flows:**
- SNMP simulator with LLDP/CDP neighbor tables
- NetFlow/IPFIX/sFlow packet generator
- Multi-device topology scenarios
- Flow deduplication testing
- CI integration

### 6.2 Testing Tools Available

#### SNMP Simulation

| Tool | Type | LLDP/CDP Support | CI Ready | Notes |
|------|------|------------------|----------|-------|
| **gosnmp MockHandler** | Go mock | Manual PDU setup | Yes | Already used, fast but manual |
| **[GoSNMPServer](https://github.com/slayercat/GoSNMPServer)** | Go library | Programmable | Yes | Pure Go, can create real agents |
| **[snmpsim](https://github.com/etingof/snmpsim)** | Python daemon | Via .snmprec files | Yes | Can record from real devices |
| **iReasoning SNMP Simulator** | Java | Full MIB support | Limited | Commercial, GUI-focused |

#### NetFlow/IPFIX/sFlow Generation

| Tool | Protocols | CI Ready | Notes |
|------|-----------|----------|-------|
| **[nflow-generator](https://github.com/nerdalert/nflow-generator)** | NetFlow v5 only | Yes (Docker) | Simple, limited to v5 |
| **[FlowTest](https://github.com/CESNET/FlowTest)** | NetFlow/IPFIX | Yes | Complex, research-grade |
| **[softflowd](https://github.com/irino/softflowd)** | NetFlow v1/5/9, IPFIX | Yes | Generates from pcap |
| **[goflow2](https://github.com/netsampler/goflow2)** | NetFlow v5/v9, IPFIX, sFlow | Library only | Can encode packets |
| **Custom Go generator** | Any | Yes | Build using goflow2 encoding |

### 6.3 Recommended Testing Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CI PIPELINE (GitHub Actions)                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        UNIT TESTS (Fast, No I/O)                     │   │
│  │  - gosnmp MockHandler for SNMP PDU responses                        │   │
│  │  - Mock flow packets for parser testing                             │   │
│  │  - Topology merge algorithm tests (pure data structures)            │   │
│  │  - Flow aggregation tests (pure data structures)                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    INTEGRATION TESTS (With Simulators)               │   │
│  │                                                                      │   │
│  │  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────┐  │   │
│  │  │  GoSNMPServer    │    │  GoSNMPServer    │    │ GoSNMPServer │  │   │
│  │  │  "router-a"      │    │  "switch-b"      │    │ "switch-c"   │  │   │
│  │  │  LLDP neighbors: │    │  LLDP neighbors: │    │ LLDP:        │  │   │
│  │  │  - switch-b:g0/1 │    │  - router-a:g0/0 │    │ - switch-b   │  │   │
│  │  │  - switch-c:g0/2 │    │  - switch-c:g0/1 │    │              │  │   │
│  │  └────────┬─────────┘    └────────┬─────────┘    └──────┬───────┘  │   │
│  │           │                       │                      │          │   │
│  │           └───────────────────────┼──────────────────────┘          │   │
│  │                                   ▼                                 │   │
│  │                        ┌──────────────────┐                         │   │
│  │                        │  SNMP Collector  │                         │   │
│  │                        │  (under test)    │                         │   │
│  │                        └────────┬─────────┘                         │   │
│  │                                 │                                   │   │
│  │                                 ▼                                   │   │
│  │                        ┌──────────────────┐                         │   │
│  │                        │ Topology Builder │                         │   │
│  │                        │ Assert: 3 nodes  │                         │   │
│  │                        │ Assert: 3 links  │                         │   │
│  │                        │ Assert: bidir    │                         │   │
│  │                        └──────────────────┘                         │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      FLOW INTEGRATION TESTS                          │   │
│  │                                                                      │   │
│  │  ┌──────────────────┐                    ┌──────────────────────┐   │   │
│  │  │  Flow Generator  │  UDP packets       │   Flow Collector     │   │   │
│  │  │  (Go, in-process)│ ──────────────────►│   (under test)       │   │   │
│  │  │                  │  NetFlow v9/IPFIX  │                      │   │   │
│  │  │  - 100 flows     │  sFlow             │   Assert:            │   │   │
│  │  │  - known values  │                    │   - bytes match      │   │   │
│  │  │  - sampling 1:100│                    │   - packets match    │   │   │
│  │  └──────────────────┘                    │   - sampling norm    │   │   │
│  │                                          └──────────────────────┘   │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.4 Implementation: SNMP Test Fixtures

#### Option A: GoSNMPServer-based (Recommended)

Create Go test fixtures that spin up real SNMP agents:

```go
// testdata/topology_fixtures.go
package testdata

import (
    "github.com/slayercat/GoSNMPServer"
)

// CreateTopologyFixture creates a 3-device topology for testing
func CreateTopologyFixture(t *testing.T) *TopologyFixture {
    // Router A - has LLDP neighbors to Switch B and C
    routerA := NewSNMPAgent(t, "127.0.0.1:10161")
    routerA.SetSysName("router-a")
    routerA.SetSysObjectID("1.3.6.1.4.1.9.1.1")  // Cisco
    routerA.SetChassisID("00:11:22:33:44:01")
    routerA.AddLLDPNeighbor(LLDPNeighbor{
        LocalPort:     "GigabitEthernet0/0",
        RemChassisID:  "00:11:22:33:44:02",  // Switch B
        RemPortID:     "GigabitEthernet0/1",
        RemSysName:    "switch-b",
    })
    routerA.AddLLDPNeighbor(LLDPNeighbor{
        LocalPort:     "GigabitEthernet0/1",
        RemChassisID:  "00:11:22:33:44:03",  // Switch C
        RemPortID:     "GigabitEthernet0/1",
        RemSysName:    "switch-c",
    })

    // Switch B - has LLDP neighbors to Router A and Switch C
    switchB := NewSNMPAgent(t, "127.0.0.1:10162")
    switchB.SetSysName("switch-b")
    switchB.SetChassisID("00:11:22:33:44:02")
    switchB.AddLLDPNeighbor(LLDPNeighbor{
        LocalPort:     "GigabitEthernet0/1",
        RemChassisID:  "00:11:22:33:44:01",  // Router A
        RemPortID:     "GigabitEthernet0/0",
        RemSysName:    "router-a",
    })
    // ... more neighbors

    return &TopologyFixture{
        Agents: []*SNMPAgent{routerA, switchB, switchC},
        ExpectedNodes: 3,
        ExpectedLinks: 3,
        ExpectedBidirectional: 3,
    }
}
```

**Pros:**
- Pure Go, no external dependencies
- Fast startup (in-process or localhost UDP)
- Deterministic, reproducible
- Easy to create complex topologies programmatically

**Cons:**
- Need to implement LLDP-MIB OID handlers
- More initial development effort

#### Option B: snmpsim with Pre-recorded Data

Record from real devices, store in repo:

```
testdata/snmprec/
├── router-a.snmprec       # Recorded from real Cisco router
├── switch-b.snmprec       # Recorded from real switch
├── switch-c.snmprec       # Recorded from real switch
└── topology-scenario.yaml # Describes expected topology
```

**Recording command:**
```bash
snmprec-record-commands \
  --protocol=udp \
  --agent=192.168.1.1 \
  --community=public \
  --output-file=router-a.snmprec \
  --start-oid=1.0.8802.1.1.2  # LLDP-MIB
```

**CI integration:**
```yaml
# .github/workflows/snmp-integration.yml
jobs:
  integration-test:
    runs-on: ubuntu-latest
    services:
      snmpsim:
        image: tandoor/snmpsim:latest
        ports:
          - 10161:161/udp
          - 10162:162/udp
        volumes:
          - ./testdata/snmprec:/usr/share/snmpsim/data
```

**Pros:**
- Uses real device responses
- Mature, battle-tested tool
- Easy to add new device types

**Cons:**
- Python dependency
- Slower startup
- Harder to modify programmatically

### 6.5 Implementation: Flow Test Fixtures

#### Option A: Go-based Flow Generator (Recommended)

Build a simple flow generator using goflow2's encoding:

```go
// testdata/flow_generator.go
package testdata

import (
    "net"
    "github.com/netsampler/goflow2/producer"
)

type FlowGenerator struct {
    conn    *net.UDPConn
    target  *net.UDPAddr
}

func NewFlowGenerator(targetAddr string) *FlowGenerator {
    // Setup UDP connection to collector
}

// SendNetFlowV9 sends a NetFlow v9 packet with specified flows
func (g *FlowGenerator) SendNetFlowV9(flows []TestFlow) error {
    // Encode using goflow2/producer
    // Send via UDP
}

// SendIPFIX sends an IPFIX packet
func (g *FlowGenerator) SendIPFIX(flows []TestFlow) error {
    // Similar encoding
}

// SendSFlow sends an sFlow datagram
func (g *FlowGenerator) SendSFlow(samples []TestSample) error {
    // sFlow encoding
}

// TestFlow represents a flow record for testing
type TestFlow struct {
    SrcAddr     net.IP
    DstAddr     net.IP
    SrcPort     uint16
    DstPort     uint16
    Protocol    uint8
    Bytes       uint64
    Packets     uint64
    StartTime   time.Time
    EndTime     time.Time
    SrcAS       uint32
    DstAS       uint32
    InIfIndex   uint32
    OutIfIndex  uint32
    SamplingRate uint32
}
```

**Test example:**
```go
func TestFlowCollector_NetFlowV9(t *testing.T) {
    // Start collector
    collector := NewFlowCollector(t, ":19995")
    defer collector.Stop()

    // Create generator
    gen := NewFlowGenerator("127.0.0.1:19995")

    // Send known flows
    flows := []TestFlow{
        {SrcAddr: net.ParseIP("10.0.0.1"), DstAddr: net.ParseIP("10.0.0.2"),
         SrcPort: 12345, DstPort: 80, Protocol: 6,
         Bytes: 1000000, Packets: 1000, SamplingRate: 100},
    }
    gen.SendNetFlowV9(flows)

    // Wait and verify
    time.Sleep(100 * time.Millisecond)

    summary := collector.GetSummary()
    // With 1:100 sampling, expect 100M bytes
    assert.Equal(t, uint64(100000000), summary.TotalBytes)
}
```

#### Option B: External Tools (Docker)

Use existing tools in CI:

```yaml
# .github/workflows/flow-integration.yml
jobs:
  flow-test:
    runs-on: ubuntu-latest
    steps:
      - name: Start collector
        run: |
          ./netdata-flow-collector --port 2055 &
          sleep 2

      - name: Generate NetFlow v5 traffic
        run: |
          docker run --rm --net=host networkstatic/nflow-generator \
            -t 127.0.0.1 -p 2055 -c 1000

      - name: Generate NetFlow v9 traffic
        run: |
          # Use softflowd with pre-recorded pcap
          softflowd -i testdata/sample.pcap -n 127.0.0.1:2055 -v 9

      - name: Verify collected data
        run: |
          ./verify-flow-collection --expected-flows 1000
```

### 6.6 Test Scenarios

#### Topology Scenarios

| Scenario | Description | Validates |
|----------|-------------|-----------|
| **T1: Simple chain** | A → B → C | Basic LLDP discovery |
| **T2: Ring** | A → B → C → A | Bidirectional link detection |
| **T3: Partial visibility** | A monitors B, B sees C (not monitored) | "Discovered" node handling |
| **T4: Multi-agent** | Agent1 monitors A, Agent2 monitors B | Cross-agent link merge |
| **T5: CDP + LLDP mixed** | Some devices CDP, some LLDP | Protocol coexistence |
| **T6: Link flap** | Link disappears then reappears | Stale link handling |

#### Flow Scenarios

| Scenario | Description | Validates |
|----------|-------------|-----------|
| **F1: Single exporter** | 1 router sends NetFlow v9 | Basic collection |
| **F2: Multi-protocol** | NetFlow v5 + v9 + IPFIX + sFlow | Protocol parsing |
| **F3: Sampling normalization** | Same flow, 1:100 vs 1:1000 | Normalization math |
| **F4: Transit dedup** | Same flow from ingress + egress | Deduplication logic |
| **F5: Multi-agent** | Agent1 gets ingress, Agent2 gets egress | Cross-agent aggregation |
| **F6: Interface enrichment** | Flows with ifIndex, SNMP has names | SNMP correlation |
| **F7: High cardinality** | 100K unique flows | Top-N aggregation |

### 6.7 CI Pipeline Design

```yaml
# .github/workflows/network-features.yml
name: Network Topology & Flow Tests

on:
  push:
    paths:
      - 'src/go/plugin/go.d/collector/snmp/**'
      - 'src/go/plugin/go.d/collector/netflow/**'
  pull_request:
    paths:
      - 'src/go/plugin/go.d/collector/snmp/**'
      - 'src/go/plugin/go.d/collector/netflow/**'

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run unit tests
        run: |
          cd src/go/plugin/go.d
          go test -v -race ./collector/snmp/...
          go test -v -race ./collector/netflow/...

  snmp-integration:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Run SNMP topology integration tests
        run: |
          cd src/go/plugin/go.d
          go test -v -tags=integration ./collector/snmp/integration/...

  flow-integration:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Run flow collector integration tests
        run: |
          cd src/go/plugin/go.d
          go test -v -tags=integration ./collector/netflow/integration/...

  e2e-topology:
    runs-on: ubuntu-latest
    needs: [snmp-integration, flow-integration]
    steps:
      - uses: actions/checkout@v4
      - name: Build Netdata
        run: ./build.sh
      - name: Start SNMP simulators
        run: |
          docker-compose -f testdata/topology/docker-compose.yml up -d
      - name: Run Netdata with topology
        run: |
          ./netdata -D &
          sleep 10
      - name: Verify topology API
        run: |
          curl -s localhost:19999/api/v3/function?function=topology | \
            jq -e '.devices | length == 3'
          curl -s localhost:19999/api/v3/function?function=topology | \
            jq -e '.links | length == 3'
```

### 6.8 Verdict: Is Full CI Testing Doable?

**YES**, with the following approach:

| Layer | Approach | Effort | CI Time |
|-------|----------|--------|---------|
| **Unit tests** | gosnmp MockHandler + mock flow packets | Low | <30s |
| **SNMP integration** | GoSNMPServer (in-process Go agents) | Medium | <1min |
| **Flow integration** | Go-based flow generator (in-process) | Medium | <1min |
| **E2E topology** | Docker + snmpsim OR GoSNMPServer | Medium | <5min |
| **E2E flows** | Docker + nflow-generator + softflowd | Low | <3min |
| **Multi-agent E2E** | Docker Compose with 2+ Netdata instances | High | <10min |

**Total CI time:** ~15-20 minutes for full test suite

**Recommendation:**
1. **Phase 1:** GoSNMPServer-based integration tests (pure Go, fast, no Docker)
2. **Phase 2:** Add Docker-based E2E for realistic scenarios
3. **Phase 3:** Multi-agent E2E with simulated Cloud aggregation

### 6.9 Testing Infrastructure Components to Build

| Component | Purpose | Location |
|-----------|---------|----------|
| `snmpagent` | Go library to create SNMP agents for tests | `pkg/testutil/snmpagent/` |
| `flowgen` | Go library to generate flow packets | `pkg/testutil/flowgen/` |
| `topofixtures` | Pre-defined topology scenarios | `collector/snmp/testdata/topologies/` |
| `flowfixtures` | Pre-defined flow scenarios | `collector/netflow/testdata/flows/` |
| `e2e/` | Docker Compose + test scripts | `src/go/plugin/go.d/e2e/` |

---

## Part 7: Available Test Data Resources (VERIFIED)

**Verification Date:** 2025-02-03
**Method:** Cloned repositories and inspected actual files

### 7.1 License Compatibility

**Netdata is GPL-3.0+** - All sources below are license-compatible:

| Source | License | Compatible | Notes |
|--------|---------|------------|-------|
| **LibreNMS** | GPL-3.0 | ✅ Yes | Same license family |
| **Akvorado** | AGPL-3.0 | ✅ Yes | AGPL is GPL-compatible |
| **snmpsim-data** | BSD-2-Clause | ✅ Yes | Permissive |
| **goflow2** | BSD-3-Clause | ✅ Yes | Permissive |
| **Wireshark samples** | Various/Public | ✅ Yes | Public domain |
| **tcpreplay samples** | BSD | ✅ Yes | Permissive |

**Conclusion:** No licensing issues. We can freely use code and data from all sources.

### 7.2 SNMP Test Data (VERIFIED)

#### LibreNMS snmprec Collection

**Repository:** https://github.com/librenms/librenms/tree/master/tests/snmpsim
**License:** GPL-3.0 (compatible)

**Verified counts:**
| Metric | Count |
|--------|-------|
| Total snmprec files | **1,872** |
| Files with LLDP data (any) | **137** |
| Files with LLDP remote neighbors (lldpRemTable) | **106** |
| Files with CDP data | **15+** |

**Sample LLDP remote neighbor data (verified):**
```
# From arista_eos.snmprec - lldpRemSysName (1.0.8802.1.1.2.1.4.1.1.9)
1.0.8802.1.1.2.1.4.1.1.9.0.47.2|4|3750E-Estudio1.example.net
1.0.8802.1.1.2.1.4.1.1.9.0.48.1|4|4900M-CORE.example.net

# From arubaos-cx.snmprec - lldpRemSysName
1.0.8802.1.1.2.1.4.1.1.9.5.10.19|4|vmhost-20
1.0.8802.1.1.2.1.4.1.1.9.5.43.16|4|vmhost-30
1.0.8802.1.1.2.1.4.1.1.9.5.44.15|4|vmhost-31
```

**Sample CDP neighbor data (verified):**
```
# From ios_2960x.snmprec - cdpCacheDeviceId (1.3.6.1.4.1.9.9.23.1.2.1.1.6)
1.3.6.1.4.1.9.9.23.1.2.1.1.6.10103.1|4|acme-fr-ap-011
1.3.6.1.4.1.9.9.23.1.2.1.1.6.10152.8|4|acme-fr-s-001

# cdpCacheDevicePort (1.3.6.1.4.1.9.9.23.1.2.1.1.7)
1.3.6.1.4.1.9.9.23.1.2.1.1.7.10103.1|4|GigabitEthernet0
```

**Devices with LLDP neighbor data include:**
- Arista EOS switches
- Aruba OS-CX switches (multiple versions)
- Alcatel-Lucent AOS switches
- Avaya/Extreme BOSS switches
- Various others

**Verdict:** ✅ Production-quality LLDP/CDP test data available and ready to use.

#### snmpsim-data Repository

**Repository:** https://github.com/etingof/snmpsim-data
**License:** BSD-2-Clause (permissive)

**Verified counts:**
| Metric | Count |
|--------|-------|
| Total snmprec files | **116** |
| Categories | UPS, cameras, storage, network, OS, IoT |

**Verdict:** ✅ Smaller but permissive license. Good for general SNMP testing.

### 7.3 NetFlow/IPFIX/sFlow Test Data (VERIFIED)

#### Akvorado Test Data

**Repository:** https://github.com/akvorado/akvorado
**License:** AGPL-3.0 (GPL-compatible)

**Verified NetFlow/IPFIX pcaps:** (`outlet/flow/decoder/netflow/testdata/`)

| File | Size | Content |
|------|------|---------|
| `nfv5.pcap` | 1,498 bytes | NetFlow v5 (29 flow records) |
| `data+templates.pcap` | 1,498 bytes | NetFlow v9 with templates |
| `template.pcap` | 202 bytes | NetFlow v9 templates only |
| `mpls.pcap` | 798 bytes | MPLS traffic flows |
| `nat.pcap` | 594 bytes | NAT translation records |
| `juniper-cpid-data.pcap` | 254 bytes | Juniper-specific fields |
| `juniper-cpid-template.pcap` | 174 bytes | Juniper templates |
| `ipfix-srv6-data.pcap` | 232 bytes | IPFIX with SRv6 |
| `ipfix-srv6-template.pcap` | 126 bytes | IPFIX SRv6 templates |
| `multiplesamplingrates-*.pcap` | Various | Different sampling rates |
| `options-*.pcap` | Various | Options templates |
| `physicalinterfaces.pcap` | 1,430 bytes | Physical interface data |
| `samplingrate-*.pcap` | Various | Sampling rate testing |
| `ipfixprobe-*.pcap` | Various | ipfixprobe format |
| `datalink-*.pcap` | Various | Datalink layer data |
| `icmp-*.pcap` | Various | ICMP flow records |

**Total NetFlow/IPFIX test files:** 25

**Verified sFlow pcaps:** (`outlet/flow/decoder/sflow/testdata/`)

| File | Size | Content |
|------|------|---------|
| `data-sflow-ipv4-data.pcap` | 410 bytes | Basic sFlow IPv4 |
| `data-sflow-raw-ipv4.pcap` | 302 bytes | Raw IPv4 sFlow |
| `data-sflow-expanded-sample.pcap` | 410 bytes | Expanded samples |
| `data-encap-vxlan.pcap` | 326 bytes | VXLAN encapsulation |
| `data-qinq.pcap` | 290 bytes | QinQ VLAN tagging |
| `data-icmpv4.pcap` | 298 bytes | ICMPv4 flows |
| `data-icmpv6.pcap` | 286 bytes | ICMPv6 flows |
| `data-multiple-interfaces.pcap` | 1,290 bytes | Multi-interface |
| `data-discard-interface.pcap` | 1,290 bytes | Discard interface |
| `data-local-interface.pcap` | 1,290 bytes | Local interface |
| `data-1140.pcap` | 1,290 bytes | Additional sFlow data |

**Total sFlow test files:** 11

**NetFlow v5 pcap content verified via tshark:**
```
Frame 1: 1458 bytes
Ethernet II, Src: HuaweiTechno_79:5f:5c
IPv4: 10.19.144.41 → 10.19.144.26
UDP: 40000 → 9990
NetFlow v5 header: version=5, count=29 flows
```

**Verdict:** ✅ Production-quality flow test data covering all major protocols and edge cases.

#### Akvorado Demo Exporter (Flow Generator)

**Location:** `demoexporter/flows/`
**License:** AGPL-3.0 (GPL-compatible)

**Verified files:**
| File | Lines | Purpose |
|------|-------|---------|
| `root.go` | ~80 | Main component, UDP sender |
| `generate.go` | ~100 | Flow generation logic |
| `nfdata.go` | ~80 | NetFlow data structures |
| `nftemplates.go` | ~150 | NetFlow v9 template encoding |
| `config.go` | ~60 | Configuration |
| `packets.go` | ~20 | Packet helpers |
| `*_test.go` | Various | Test files |

**Key features (from code review):**
- Generates NetFlow v9 packets
- Configurable flow rate (flows per second)
- Random IP generation within prefixes using seeded RNG
- Peak hour simulation (traffic patterns)
- Deterministic output for reproducible tests

**Verdict:** ✅ Can adapt this code for Netdata's test flow generator.

### 7.4 Additional Resources (VERIFIED)

#### Wireshark Sample Captures

**URL:** https://wiki.wireshark.org/SampleCaptures

**Available LLDP/CDP files:**
- `lldp.minimal.pcap` - Basic LLDP frames
- `lldp.detailed.pcap` - LLDP with additional TLVs
- `lldpmed_civicloc.pcap` - LLDP-MED with location
- `cdp.pcap` - CDP v2 from Cisco router
- `cdp_v2.pcap` - CDP v2 from Cisco switch

**Note:** These are raw LLDP/CDP Ethernet frames, NOT SNMP walk data. Useful for understanding protocol encoding.

#### Tcpreplay Sample Captures

**URL:** https://tcpreplay.appneta.com/wiki/captures.html

| File | Size | Flows | Applications |
|------|------|-------|--------------|
| `smallFlows.pcap` | 9.4 MB | 1,209 | 28 |
| `bigFlows.pcap` | 368 MB | 40,686 | 132 |
| `test.pcap` | 0.07 MB | 37 | 1 |

**Use with softflowd for high-volume flow generation:**
```bash
softflowd -r bigFlows.pcap -n 127.0.0.1:2055 -v 9
```

### 7.5 Summary: Verified Test Data

| Data Type | Source | Files | Quality | Ready to Use |
|-----------|--------|-------|---------|--------------|
| **SNMP general** | LibreNMS | 1,872 | Real devices | ✅ Yes |
| **SNMP LLDP neighbors** | LibreNMS | 106 | Real topology data | ✅ Yes |
| **SNMP CDP neighbors** | LibreNMS | 15+ | Real Cisco data | ✅ Yes |
| **NetFlow v5** | Akvorado | 1 | Real capture | ✅ Yes |
| **NetFlow v9/IPFIX** | Akvorado | 24 | Real captures + edge cases | ✅ Yes |
| **sFlow** | Akvorado | 11 | Real captures + edge cases | ✅ Yes |
| **Flow generator code** | Akvorado | 6 files | Production code | ✅ Adaptable |
| **High-volume pcaps** | tcpreplay | 3 | 40K+ flows | ✅ Yes |

### 7.6 Recommended Testing Approach

#### For SNMP Topology Testing:

1. **Use LibreNMS snmprec files directly**
   ```bash
   # Clone test data
   git clone --depth 1 --filter=blob:none --sparse \
     https://github.com/librenms/librenms.git /tmp/librenms
   cd /tmp/librenms && git sparse-checkout set tests/snmpsim

   # Find files with LLDP remote neighbors
   grep -l "1\.0\.8802\.1\.1\.2\.1\.4" tests/snmpsim/*.snmprec
   # Returns: arista_eos.snmprec, arubaos-cx.snmprec, aos7.snmprec, etc.
   ```

2. **Run with snmpsim in CI**
   ```yaml
   services:
     snmpsim:
       image: tandoor/snmpsim:latest
       volumes:
         - ./testdata/snmprec:/usr/share/snmpsim/data
   ```

3. **Or use GoSNMPServer for programmatic tests**
   - Load snmprec data into GoSNMPServer
   - Create topology scenarios dynamically

#### For NetFlow/IPFIX/sFlow Testing:

1. **Use Akvorado pcaps directly**
   ```bash
   # Clone test data
   git clone --depth 1 https://github.com/akvorado/akvorado.git /tmp/akvorado

   # Copy relevant pcaps
   cp /tmp/akvorado/outlet/flow/decoder/netflow/testdata/*.pcap ./testdata/flows/
   cp /tmp/akvorado/outlet/flow/decoder/sflow/testdata/*.pcap ./testdata/flows/
   ```

2. **Replay pcaps in tests**
   ```go
   func TestNetFlowV5(t *testing.T) {
       collector := startCollector(t, ":2055")
       replayPcap(t, "testdata/flows/nfv5.pcap", "127.0.0.1:2055")
       // Assert collected flows match expected
   }
   ```

3. **Adapt Akvorado's demoexporter for dynamic generation**
   - Port `generate.go` and `nftemplates.go`
   - Use for fuzz testing and edge cases

### 7.7 Production Confidence Assessment

| Testing Layer | Data Source | Confidence |
|---------------|-------------|------------|
| Unit tests (mocks) | Hand-crafted | 30% |
| Integration (LibreNMS snmprec) | Real device recordings | +25% |
| Integration (Akvorado pcaps) | Real flow captures | +25% |
| E2E (combined) | All sources | +10% |
| Beta testing (real infra) | User networks | +10% |

**Total achievable confidence with available test data: ~90%**

The remaining 10% covers:
- Vendor-specific edge cases not in test data
- Scale issues (memory, CPU at high volume)
- Timing/race conditions in production

---

## Plan (Agent + Aggregator, 2026-02-04)

1. **NetFlow collector core** ✅
   - UDP listener + decoder (NetFlow v5/v9/IPFIX) using goflow2/v2.
   - Aggregation (bucket + key + sampling normalization + retention).
2. **Flows function** ✅
   - Build `flows` Function response payload (schema_version, exporters, buckets, summaries, metrics).
3. **Time‑series charts** ✅
   - Emit totals (bytes/s, packets/s, flows/s) + dropped records.
4. **Docs + config** ✅ (NetFlow/IPFIX only; **sFlow pending**)
5. **Aggregation prototype CLI** ✅
   - Merge topology + flows JSON outputs (non/partial/full overlap).
   - Add fixtures + unit tests.
6. **Remaining work** ✅ (done)
   - Add **sFlow v5** decoding and protocol flag.
   - Expand **IPFIX field decoding** (bytes/packets/ports/AS/prefix/time/direction).
   - Update **docs/config/schema** to mention sFlow.
   - Add **tests** for sFlow mapping and IPFIX fields.
7. **Run targeted tests** ✅ `go test ./plugin/go.d/collector/netflow ./tools/topology-flow-merge`
8. **Add real-data tests (required)** ✅
   - Add LibreNMS snmprec fixtures for LLDP/CDP and tests that build topology from them.
   - Add Akvorado pcaps for NetFlow v5/v9/IPFIX and sFlow v5 and tests that decode/aggregate them.
   - Include attribution files for testdata sources and run targeted tests.
9. **Expand real device test coverage (required)** ✅ DONE
   - SNMP: added all LibreNMS LLDP/CDP snmprec fixtures (116 files) to `src/go/plugin/go.d/collector/snmp/testdata/snmprec/`.
   - NetFlow/IPFIX/sFlow: added all Akvorado pcaps (36 files) to `src/go/plugin/go.d/collector/netflow/testdata/flows/`.
   - Tests: SNMP snmprec tests iterate all fixtures; NetFlow tests cover all pcaps.
   - Attribution updated: `src/go/plugin/go.d/collector/snmp/testdata/ATTRIBUTION.md`, `src/go/plugin/go.d/collector/netflow/testdata/ATTRIBUTION.md`.
   - Stress pcaps are downloaded at test time (CI) from tcpreplay sources.
10. **Simulator-based integration testing (required)** ✅ DONE
    - Integration tests added with `//go:build integration`:
      - SNMP: `src/go/plugin/go.d/collector/snmp/topology_integration_test.go`
      - NetFlow: `src/go/plugin/go.d/collector/netflow/netflow_integration_test.go`
    - CI workflow added: `.github/workflows/snmp-netflow-sim-tests.yml` (snmpsim + pcap replay + optional stress pcaps).
11. **Complete LLDP/CDP profiles — COMPREHENSIVE, NOT MINIMAL (required)** ✅ DONE
    - LLDP profile expanded with caps, management address tables, and stats: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-lldp-mib.yaml`.
    - CDP profile expanded with globals, interface table, and full cache fields: `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_std-cdp-mib.yaml`.
    - Topology cache/types updated for capabilities + management addresses: `src/go/plugin/go.d/collector/snmp/topology_cache.go`, `src/go/plugin/go.d/collector/snmp/topology_types.go`.
    - Tests updated to validate new fields: `src/go/plugin/go.d/collector/snmp/topology_cache_test.go`, `src/go/plugin/go.d/collector/snmp/topology_snmprec_test.go`.

**Status:** Core implementation complete. **Profile completion, expanded testing, and simulator testing DONE** (items 9-11).

## Implied Decisions (implemented)

- Added **LLDP/CDP profiles** and extended vendor profiles under `src/go/plugin/go.d/config/go.d/snmp.profiles/default/`.
- Added **netflow collector** with README/metadata/config and schema.
- Added **function response types** (topology/flows) and schema support.
- Added **goflow2/v2** (flow decoding) and **gopacket** (pcap parsing) dependencies.
- Added **real device testdata** from LibreNMS (snmprec) and Akvorado (pcap) with attribution.

## Testing Requirements

- **Unit tests**
  - Topology cache building from LLDP/CDP tags.
  - Schema generation and versioning fields.
  - Flow aggregation (sampling normalization, bucket rollups).
- **Integration tests**
  - SNMP topology fixtures (Go-based SNMP agent) for LLDP/CDP.
  - Flow packet generation (NetFlow v5/v9/IPFIX; sFlow if supported).
- **Prototype tool tests**
  - Merge non-overlapping, partial-overlap, full-overlap JSON inputs.

## Documentation Updates Required

- `src/go/plugin/go.d/collector/snmp/README.md` (add topology function docs).
- `src/go/plugin/go.d/collector/snmp/metadata.yaml` (expose topology function in docs).
- New NetFlow collector docs:
  - `src/go/plugin/go.d/collector/netflow/README.md`
  - `src/go/plugin/go.d/collector/netflow/metadata.yaml`
  - `src/go/plugin/go.d/collector/netflow/config_schema.json`
  - `src/go/plugin/go.d/config/go.d/netflow.conf`

---

## Additional Test Data Sources (User-Provided, Unverified)

**Status:** These sources are **not yet verified** in this repo.  
Before using them, we must validate **license**, **file availability**, and **fitness** for CI.

**License note:** Netdata is **GPL‑v3+**, so we can use **AGPL**, **GPL**, and all **permissive** licenses.  
Unknown or incompatible licenses must be excluded until verified.

### Proposed Sources (to verify)
- **CESNET FlowTest** (PCAPs for protocol coverage)
- **lldpd/lldpd** (LLDP/CDP fuzzing corpus)
- **pcap_genflow** (load testing PCAP generator)
- **Network Data Repository** (topology graphs)
- **snmpsim-data** (BSD-2-Clause snmprec files)
- **Wireshark SampleCaptures** (LLDP/CDP PCAPs)
- **NetLab / NetReplica templates** (topology inputs)

### Risks / Considerations
- **License unknown** for some repos (cannot import until verified).
- **Data size** may be too large for CI.
- **Security/PII** risks in public PCAPs (need careful selection).
- **Protocol mismatch** (PCAP vs NetFlow export format) may require conversion tooling.

### Action Needed
- Shortlist a **small, safe** subset for CI.
- Verify licenses and download locations before adoption.

---

## 2026-03-26 Progress

- Verified current PR `#21702` review status on GitHub:
  - `4` unresolved, non-outdated review threads remain.
  - All `4` are real issues after code inspection:
    - `src/crates/netdata-plugin/rt/src/lib.rs`
      - `serde_json::to_vec(...).unwrap_or_default()` silently converts a serialization error into an empty payload for `flows:*`.
    - `src/crates/journal-engine/src/logs/query.rs`
      - `a.len() + b.len()` in `merge_log_entries()` can overflow `usize`.
    - `src/go/pkg/buildinfo/buildinfo.go`
      - `Info()` output contract needs forward-compatibility documentation.
    - `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs`
      - merged source publication rebuilds the full vector without reserving total capacity first.

- Verified current CI state on PR `#21702` before any new commit:
  - `Go Tests`
    - `3` failed shards are all the same unrelated failure in:
      - `src/go/plugin/agent/discovery/file/watch_test.go:376`
      - via `src/go/plugin/agent/discovery/file/sim_test.go:54`
    - Facts from logs:
      - expected non-empty `[]*confgroup.Group`
      - actual result is missing groups
    - This does not point to the topology/netflow review-fix files.
  - `CodeQL`
    - Failed check-run id: `68602388890`
    - Check output says:
      - `2 configurations not found`
      - missing on `master`: `/language:go`, `/language:python`
    - Verified code-scanning alerts for `pr=21702`:
      - no open alerts in the files touched by the pending review fixes
      - one topology-related alert on `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs` is already `dismissed`
  - `Codacy Static Code Analysis`
    - `action_required`
    - external service gate, not a compile/test failure
  - `Packages`
    - GitHub currently reports failures on:
      - `Build (armhf, debian:bullseye, ...)`
      - `Build (x86_64, quay.io/centos/centos:stream10, ...)`
    - But `gh run view --log-failed` still reports the package run as in progress, so root-cause logs are not yet available.

- Current implementation plan for this slice:
  - Apply the `4` open review fixes only.
  - Re-run targeted Rust and Go validation locally.
  - Re-check CI/package state again before commit, because package logs are not yet available.

- Local validation status after applying the review fixes:
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-engine --quiet`
    - passed
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
    - passed
  - `go test ./pkg/buildinfo` from `src/go`
    - passed
  - `cargo test --manifest-path src/crates/Cargo.toml -p rt --quiet`
    - uncovered a pre-existing doctest failure in `src/crates/netdata-plugin/rt/src/lib.rs`
    - root cause: example block is missing imports for `FunctionHandler`, `FunctionCallContext`, and `FunctionDeclaration`
  - Because that doctest is in the same file touched by the pending review fix, it should be repaired in the same slice so local validation is green.


## 2026-03-26 Costa decision: drop journal optimization carryover from topology

### TL;DR
- Costa decided PR `#21962` is not worth merging.
- Remove from `topology-flows` all journal optimization changes that were carried for that PR or justified by it.
- Then establish a clean performance baseline for the topology/netflow branch using master-like journal reader behavior, measure full netflow processing overhead on top of raw row scan cost, and identify real optimization targets.

### Purpose
- Keep `topology-flows` focused on topology/netflow functionality, not speculative journal micro-optimizations.
- Measure the actual cost of netflow processing above the known raw journal read baseline.

### User decision
- Remove the journal optimization work from `topology-flows`.
- Rebenchmark after removal and identify where the remaining overhead comes from.

### Plan
1. Identify exactly which journal changes on `topology-flows` came from the closed `#21962` effort.
2. Revert only those changes from the branch, without touching unrelated netflow/topology functionality.
3. Validate the branch still builds/tests.
4. Benchmark the cleaned branch:
   - raw reader baseline if needed for sanity
   - full netflow query path / processing baseline
   - compute overhead above raw row scan cost
5. Identify the hottest remaining processing costs and propose concrete optimization directions.
   - use the same fixed four raw journals for the query-path benchmark too, via a temporary ignored test that links them into a temp `flows/raw` tree and runs `query_flows()` there


## 2026-03-26 Cleaned topology-flows baseline after dropping #21962 carryover

### Cleanup status
- Verified in a disposable worktree first, then applied on `topology-flows`.
- The closed-PR journal micro-optimizations were removed from:
  - `src/crates/jf/journal_file/src/{file,object,reader,writer}.rs`
  - `src/crates/journal-core/src/file/{file,guarded_cell,mmap,object,reader,value_guard,writer}.rs`
- `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet` still passes after the cleanup (`255 passed; 0 failed; 3 ignored`).

### Fixed-file raw reader baseline on cleaned branch
- Dataset: the fixed 4 raw journals Costa selected from `/var/cache/netdata/flows/raw/`
- Tool: `/tmp/journal-raw-bench-1774481723/target/release/journal-raw-bench-1774481723`
- Window size: `8 MiB`
- CPU pinning: `taskset -c 3`
- All 3 runs were stable:
  - rows: `745,969`
  - fields: `24,537,589`
  - fields/row: `32.8936`
- Runs:
  - `747,368 usec` (`1.001875 usec/row`)
  - `743,410 usec` (`0.996570 usec/row`)
  - `741,529 usec` (`0.994048 usec/row`)
- Mean:
  - `744,102 +/- 2,980 usec`
  - `0.997498 +/- 0.003995 usec/row`

### Fixed-file full netflow query baseline on cleaned branch
- Temporary ignored harness in `src/crates/netdata-netflow/netflow-plugin/src/main.rs`
- It copies the same 4 fixed raw journals into a temp `flows/raw` tree and runs the full `query_flows()` path.
- Query shape:
  - `view = table-sankey`
  - `group_by = ["SRC_ADDR", "DST_ADDR", "PROTOCOL"]`
  - `top_n = 25`
  - raw tier forced by raw-only fields
- All 3 runs were stable:
  - streamed/matched rows: `745,969`
  - grouped rows: `23,724`
  - returned rows: `26`
  - other-grouped rows: `23,699`

#### Cold query (first query on fresh service)
- Mean total:
  - `2,995.62 +/- 41.16 ms`
  - `4.015743 +/- 0.055183 usec/row`
- Mean stage breakdown:
  - scan: `2,486.67 +/- 30.62 ms`
  - facets: `483.67 +/- 12.06 ms`
  - build rows: `23.67 +/- 0.58 ms`

#### Warm query (second query on same service)
- Mean total:
  - `2,532.90 +/- 9.65 ms`
  - `3.395449 +/- 0.012933 usec/row`
- Mean stage breakdown:
  - scan: `2,451.67 +/- 13.58 ms`
  - facets: `56.00 +/- 3.61 ms`
  - build rows: `24.00 +/- 1.73 ms`

### Measured processing overhead above raw reading
- Cleaned raw reader baseline:
  - `0.997498 +/- 0.003995 usec/row`
- Warm full query baseline:
  - `3.395449 +/- 0.012933 usec/row`
- Added steady-state processing overhead:
  - `2.397952 +/- 0.013536 usec/row`

### Where the warm-query time currently goes
- `query_group_scan_ms` dominates:
  - `2,451.67 ms`
  - `3.286553 usec/row`
- Warm facets are much smaller once the cache is populated:
  - `56.00 ms`
  - `0.075070 usec/row`
- Final row building is negligible:
  - `24.00 ms`
  - `0.032173 usec/row`

### Evidence-backed optimization targets
1. The projected raw grouped scan closure is the main processing hotspot.
- Evidence:
  - `query_group_scan_ms` dominates warm runtime.
  - `perf report` on the fixed harness shows `netflow_plugin::query::FlowQueryService::scan_matching_grouped_records_projected::{{closure}}` and its inner `memchr` work near the top of user-space CPU time.
- Code:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs` around `scan_matching_grouped_records_projected()`.

2. We still split and inspect every payload in every row, even though the query only needs a tiny subset of fields.
- Evidence:
  - fixed dataset averages `32.8936` fields/row, but this query only needs metrics plus `SRC_ADDR`, `DST_ADDR`, `PROTOCOL`.
  - the scan loop visits every payload and runs `split_payload_bytes()` for each one.
- Code:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs` around the `cursor.visit_payloads(...)` loop.
- Likely win:
  - add an early-stop path once all required fields and metrics have been captured for the row.

3. The warm scan path still pays repeated string/value resolution and compact-index lookups per grouped field.
- Evidence:
  - for each captured grouped value, the scan path calls `find_field_value()` and may later call `get_or_insert_field_value()` / `find_flow_by_field_ids()` / `insert_flow_by_field_ids()`.
- Code:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs` in `accumulate_projected_compact_grouped_record()` and the grouped scan closure.
- Likely win:
  - add faster hot-path handling for common exact fields like `PROTOCOL`, `SRC_ADDR`, `DST_ADDR` so we avoid repeated generic text-index work where possible.

4. Warm facet cost is real but no longer the main target.
- Evidence:
  - warm facets are only `56 ms` vs `2,451.67 ms` scan.
- Implication:
  - after the non-contextual facet work, further facet tuning is lower priority than scan-path tuning.


## 2026-03-26 Audit of remaining `journal-core` / `jf` diffs on `topology-flows`

### TL;DR
- After dropping the closed `#21962` carryover, the remaining delta versus `origin/master` under `jf` / `journal-core/src/file` is down to 6 files.
- Not all of it is required for topology/netflow.
- There are confirmed leftovers still on the branch.

### Remaining files differing from `origin/master`
- `src/crates/jf/journal_file/src/filter.rs`
- `src/crates/jf/journal_file/src/lib.rs`
- `src/crates/journal-core/src/file/file.rs`
- `src/crates/journal-core/src/file/mod.rs`
- `src/crates/journal-core/src/file/object.rs`
- `src/crates/journal-core/src/file/reader.rs`

### Confirmed leftovers
1. `src/crates/jf/journal_file/src/filter.rs`
- This diff is not referenced by `netflow-plugin` or `journal-session`.
- The remaining change is the local `FilterExpr::None` handling in `jf`, which belongs to the closed journal-reader PR path, not to topology/netflow.
- Evidence:
  - `rg -n "build_filter\\(|FilterExpr::None" src/crates`
  - `journal-session` uses `journal_core::JournalReader::build_filter()`, not `jf`.

2. `src/crates/jf/journal_file/src/lib.rs`
- The only remaining diff is `pub use error::JournalError;`
- This is not required by topology/netflow.
- Evidence:
  - `rg -n "journal_file::JournalError|use journal_file::JournalError" src`
  - no topology/netflow caller depends on it.

3. Disposable worktree proof for the two `jf` leftovers
- In `/tmp/topology-journal-audit-2678207`, restoring only:
  - `src/crates/jf/journal_file/src/filter.rs`
  - `src/crates/jf/journal_file/src/lib.rs`
  to `origin/master` still kept:
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-session --quiet`
  green.
- Conclusion:
  - these two `jf` diffs are confirmed leftovers and can be dropped safely from the topology branch.

### Remaining `journal-core` diffs that are required by topology/netflow
1. `src/crates/journal-core/src/file/reader.rs`
- `build_filter()` is required by `journal-session`:
  - `src/crates/journal-session/src/cursor.rs:260`
- This is needed because `journal-session` replays match filters per file and extracts the built `FilterExpr` into its own cursor.

2. `src/crates/journal-core/src/file/file.rs`
- `visit_entry_payloads()` is required by `journal-session`:
  - `src/crates/journal-session/src/cursor.rs:504`
- The helper methods it uses are also required:
  - `object_bytes_in_window()`
  - `parse_object_in_window()`
  - `data_payload_in_window()`

3. `src/crates/journal-core/src/file/object.rs`
- `DataPayloadRef` and the shared decompression helper are required by `visit_entry_payloads()`.

4. `src/crates/journal-core/src/file/mod.rs`
- The re-export of `DataPayloadRef` is required by the public `visit_entry_payloads()` API.
- The re-export of `EntryDataIterator` is required as long as `entry_data_objects()` keeps returning it from the public API.

### Remaining `journal-core` diffs that still look like leftovers
1. `src/crates/journal-core/src/file/file.rs`
- `entry_data_objects()` still precomputes all DATA offsets into a `Vec<NonZeroU64>`, and `EntryDataIterator` now walks that cached vector.
- This is the same entry-offset caching direction that came from the closed `#21962` effort.
- Evidence:
  - compare current `entry_data_objects()` / `EntryDataIterator` with `origin/master`
  - current file differs exactly in that area
  - callers that still use it are:
    - `src/crates/journal-index/src/file_index.rs:386`
    - `src/crates/journal-core/src/file/reader.rs:364`
- This functionality is not required by `journal-session::visit_payloads()`, which uses `visit_entry_payloads()` instead.

2. `src/crates/journal-core/src/file/reader.rs`
- `entry_data_enumerate()` is currently unused outside the file itself:
  - `rg -n "entry_data_enumerate\\(" src/crates`
  - no `journal-session` or `netflow-plugin` caller
- `entry_data_offsets()` is also currently unused outside the file itself:
  - `rg -n "entry_data_offsets\\(" src/crates`
- The new reader fields:
  - `entry_data_offsets`
  - `entry_data_index`
  - `entry_data_ready`
  exist only to support that unused path.
- Conclusion:
  - these look like leftover API and state from the closed entry-offset caching work, not topology/netflow requirements.

### Current reality
- Confirmed safe leftovers:
  - `src/crates/jf/journal_file/src/filter.rs`
  - `src/crates/jf/journal_file/src/lib.rs`
- Likely leftover `journal-core` pieces still embedded in required files:
  - cached `entry_data_objects()` / `EntryDataIterator`
  - unused `JournalReader::entry_data_enumerate()`
  - unused `JournalReader::entry_data_offsets()`
- Required `journal-core` pieces that should stay:
  - `build_filter()`
  - `visit_entry_payloads()`
  - `DataPayloadRef`
  - the minimal helper/export surface supporting those APIs

### Pending cleanup decision
1. Remove only the confirmed leftovers now.
- Files:
  - `src/crates/jf/journal_file/src/filter.rs`
  - `src/crates/jf/journal_file/src/lib.rs`
- Pros:
  - zero controversy
  - already validated in disposable worktree
- Cons:
  - likely leftover `journal-core` pieces remain
- Implications:
  - branch gets cleaner immediately, but not fully minimized
- Risks:
  - none meaningful

2. Do one more surgical cleanup pass on the likely leftover `journal-core` pieces before committing.
- Targets:
  - cached `entry_data_objects()` / `EntryDataIterator`
  - unused `JournalReader::entry_data_enumerate()`
  - unused `JournalReader::entry_data_offsets()`
  - any now-unnecessary export such as `EntryDataIterator`
- Pros:
  - branch becomes materially cleaner
  - journal delta is closer to the exact needs of topology/netflow
- Cons:
  - needs another disposable-worktree validation pass
- Implications:
  - we may end up shrinking the remaining journal delta further
- Risks:
  - if any hidden caller exists, this needs careful compile/test validation

### Recommendation
- Recommendation: do `2`.
- Reason:
  - the `jf` leftovers are already proven removable
  - the remaining `journal-core` leftovers are strong enough to justify one more surgical validation pass before we keep carrying them on the branch

### Costa decision
- `1. B`
- Do one more surgical cleanup pass on the likely leftover `journal-core` pieces before committing anything.

### 2026-03-26 result of the surgical cleanup pass
- Applied on the real `topology-flows` branch after proving it first in `/tmp/topology-journal-audit-2678207`.
- Removed from the branch:
  - `src/crates/jf/journal_file/src/filter.rs`
  - `src/crates/jf/journal_file/src/lib.rs`
  - cached `entry_data_objects()` / `EntryDataIterator` behavior in `src/crates/journal-core/src/file/file.rs`
  - extra reader state for cached entry-data enumeration in `src/crates/journal-core/src/file/reader.rs`
  - extra `EntryDataIterator` re-export in `src/crates/journal-core/src/file/mod.rs`
- Validation on the real branch:
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-session --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-index --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
  - all passed

### Current remaining journal delta versus `origin/master`
- Now down to 4 files:
  - `src/crates/journal-core/src/file/file.rs`
  - `src/crates/journal-core/src/file/mod.rs`
  - `src/crates/journal-core/src/file/object.rs`
  - `src/crates/journal-core/src/file/reader.rs`
- These remaining diffs are tied to the actual `journal-session` functionality used by `netflow-plugin`:
  - `build_filter()`
  - `visit_entry_payloads()`
  - `DataPayloadRef`
  - the minimal helper surface supporting those APIs

## 2026-03-26 PR #21702 remaining live review comments

### Verified live unresolved threads
- Refreshed from GitHub after the journal cleanup commits.
- Exactly 3 unresolved non-outdated review threads remained:
  - `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs`
    - method parsing error text claimed only `GET` / `POST`, but runtime parsing still accepted any valid `reqwest::Method`
  - `src/crates/journal-log-writer/tests/log_writer.rs`
    - tests hard-required external `journalctl`
  - `src/go/pkg/topology/engine/engine_test.go`
    - tests used `require.True(errors.Is(...))` instead of `require.ErrorIs(...)`

### Implemented fixes
- `src/crates/netdata-netflow/netflow-plugin/src/network_sources.rs`
  - added `parse_source_method()` so runtime fetch validation now enforces the same `GET` / `POST` contract already enforced in `plugin_config.rs`
  - added unit coverage for accepted and rejected methods
- `src/crates/journal-log-writer/tests/log_writer.rs`
  - added `journalctl_available()` detection using `journalctl --version`
  - `journalctl`-backed tests now skip cleanly when the external binary is unavailable
- `src/go/pkg/topology/engine/engine_test.go`
  - replaced `require.True(errors.Is(...))` with `require.ErrorIs(...)`

### Validation
- `cargo test --manifest-path src/crates/Cargo.toml -p journal-log-writer --quiet`
- `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
- `cd src/go && go test ./pkg/topology/engine`
- All passed before commit.

## 2026-03-26 journal-core re-audit after the PR #21962 cleanup

### Verified current journal delta versus `origin/master`
- `jf` has no remaining diff at all.
- Remaining journal delta is only:
  - `src/crates/journal-core/src/file/file.rs`
  - `src/crates/journal-core/src/file/mod.rs`
  - `src/crates/journal-core/src/file/object.rs`
  - `src/crates/journal-core/src/file/reader.rs`

### Verified required pieces
- `journal-session` depends on:
  - `JournalReader::build_filter()`
  - `JournalFile::visit_entry_payloads()`
  - `DataPayloadRef`
- Evidence:
  - `src/crates/journal-session/src/cursor.rs` uses `reader.build_filter()` and `jf.visit_entry_payloads(...)`

### Verified leftover still remaining in the branch
- `src/crates/journal-core/src/file/reader.rs`
  - `JournalReader::entry_data_enumerate()`
- Evidence:
  - no in-tree callers in:
    - `journal-core`
    - `journal-session`
    - `journal-index`
    - `netflow-plugin`
  - disposable-worktree proof:
    - removed only `entry_data_enumerate()`
    - `cargo test -p journal-session` passed
    - `cargo test -p journal-index` passed
    - `cargo test -p netflow-plugin` passed

### Conclusion
- The branch was still carrying one last clear `journal-core` leftover after the earlier cleanup.
- This method should be removed from the real branch as part of minimizing the journal delta to only what topology/netflow actually needs.

## 2026-03-26 strict minimization goal for remaining journal-core review surface

### TL;DR
- Costa wants the review surface for netflow to be as small as possible.
- Any remaining `journal-core` change that is not strictly required by the `journal-session` path used by netflow must be removed, even if the leftover is tiny.

### Current verified baseline
- `jf` already has zero remaining affected files in the branch.
- Remaining journal diff is only:
  - `src/crates/journal-core/src/file/file.rs`
  - `src/crates/journal-core/src/file/mod.rs`
  - `src/crates/journal-core/src/file/object.rs`
  - `src/crates/journal-core/src/file/reader.rs`

### Minimization target
- Re-audit every remaining hunk in those 4 files.
- Keep only code that is strictly needed by:
  - `journal-session` integration
  - netflow query processing through `journal-session`
- If a hunk is only a convenience refactor, style cleanup, generic helper reshaping, or an unrelated reuse improvement, remove it.

### Validation requirement
- After every minimization pass, re-run:
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-session --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-index --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`

### 2026-03-26 minimization result
- Further reduced the remaining review surface by moving `journal-session::Cursor::visit_payloads()` back onto existing `journal-core` APIs:
  - `entry_data_object_offsets()`
  - `data_ref()`
  - `DataObject::raw_payload()`
  - `DataObject::decompress()`
- This removed the need for:
  - `DataPayloadRef`
  - `visit_entry_payloads()`
  - helper refactors in `journal-core::file`
  - the corresponding `mod.rs` export
- Validation after the reduction:
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-session --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p journal-index --quiet`
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
  - all passed

### Current minimized journal surface
- `jf`: zero affected files versus `origin/master`
- `journal-core`: only one remaining functional delta versus `origin/master`
  - `src/crates/journal-core/src/file/reader.rs`
  - adds `JournalReader::build_filter()` so `journal-session` can build per-file filters without driving iteration through `JournalReader::step()`
- The rest of the journal-related review surface now lives in `journal-session`, not `journal-core`

## 2026-03-26 benchmark replay request after minimizing journal-core touch points

### TL;DR
- Recreate the fixed 4-file raw benchmark on the current branch and confirm that raw `journal-core` performance still holds.
- Then run the same fixed-dataset benchmark through the netflow query path for `group_by = [SRC_ADDR, DST_ADDR, PROTOCOL]` to measure the processing overhead added by the netflow plugin.

### Fixed dataset
- Use exactly these files from `/var/cache/netdata/flows/raw/`:
  - `system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal`
  - `system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal`
  - `system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal`
  - `system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal`

### Benchmark 1
- Raw `journal-core` performance:
  - open each file
  - read all rows
  - enumerate every field of every row
  - include open/read/close time
  - measure monotonic elapsed time

### Benchmark 2
- Netflow plugin processing overhead on the same raw dataset:
  - answer a grouped query by:
    - `SRC_ADDR`
    - `DST_ADDR`
    - `PROTOCOL`
  - include everything needed by the real plugin/query path to answer that query
  - report time/row so the netflow overhead over the raw reader baseline is explicit

### Output goal
- Reconfirm the raw-reader baseline on the current minimized branch state.
- Then quantify:
  - total query time
  - rows processed
  - per-row cost
  - incremental per-row overhead over raw `journal-core`
## 2026-03-26: current branch baseline on frozen 4-file raw dataset

### Dataset used

- The original live raw paths rotated out of `/var/cache/netdata/flows/raw/`.
- The exact same 4 files still exist in:
  - `/tmp/netdata-raw-snapshot-1774481723/`
- Files:
  - `system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal`
  - `system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal`
  - `system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal`
  - `system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal`

### Raw `journal-core` benchmark

- Purpose:
  - open each file
  - enumerate every row
  - enumerate every field of every row
  - include open/read/close time
- Harness:
  - `/tmp/journal-raw-bench-1774481723/src/main.rs`
- Window:
  - `8 MiB`
- CPU pinning:
  - `taskset -c 3`
- Stable warm results, 3 runs:
  - rows: `745,969`
  - fields: `24,537,589`
  - fields/row: `32.8936`
  - total time: `866,480.667 +/- 7,358.986 usec`
  - time/row: `1.161551 +/- 0.009865 usec`

### Full netflow grouped-query benchmark

- Purpose:
  - run the full netflow query path needed to answer:
    - `group_by = ["SRC_ADDR", "DST_ADDR", "PROTOCOL"]`
- Harness:
  - temporary worktree only
  - `src/crates/netdata-netflow/netflow-plugin/src/main.rs`
  - ignored test `profile_fixed_raw_query_processing_against_local_journals()`
- Notes:
  - temp worktree pointed the harness at the frozen 4-file snapshot
  - no branch code was changed
  - test binary was run directly, pinned with `taskset -c 3`
- Stable results, 3 runs:
  - streamed entries: `745,969`
  - matched entries: `745,969`
  - returned rows: `26`
  - cold elapsed: `3429.737 +/- 28.500 ms`
  - warm elapsed: `3042.260 +/- 14.134 ms`
  - cold time/row: `4.597693 +/- 0.038206 usec`
  - warm time/row: `4.078266 +/- 0.018947 usec`
  - warm group scan: `2960.333 ms`
  - warm facet scan: `56.333 ms`

### Measured overhead over raw `journal-core`

- Warm overhead over raw baseline:
  - `2.916715 +/- 0.021361 usec/row`
- Cold overhead over raw baseline:
  - `3.436142 +/- 0.039459 usec/row`
- Warm slowdown factor versus raw baseline:
  - `3.5111x`
- Cold slowdown factor versus raw baseline:
  - `3.9582x`

### Current interpretation

- Raw `journal-core` performance remains close to the previous ~`1 usec/row` result.
- The full netflow grouped-query path on the same dataset is ~`4.08 usec/row` warm.
- The dominant cost is the grouped scan path, not facets:
  - warm group scan: `3.968440 usec/row`
  - warm facet scan: `0.075517 usec/row`

## 2026-03-26: hard performance budget for netflow processing

### User requirement

- Treat the raw `journal-core` benchmark as the DB work budget.
- Netflow plugin processing on top of that must be at most `10%` of DB work.
- No `journal-core` changes are allowed for this optimization slice.
- Fix the netflow plugin algorithmically.

### Derived target from the current measured baseline

- Current raw DB cost:
  - `1.161551 usec/row`
- Maximum allowed extra netflow processing:
  - `0.116155 usec/row`
- Maximum allowed total warm grouped-query cost on the same benchmark:
  - `1.277706 usec/row`

### Current gap to close

- Current warm grouped-query cost:
  - `4.078266 usec/row`
- Current extra netflow processing:
  - `2.916715 usec/row`
- Required reduction in netflow-only overhead:
  - from `2.916715` to `0.116155 usec/row`
  - about `25.11x` less extra processing than today

### Immediate engineering interpretation

- This cannot be solved by micro-optimizations.
- The DB budget already includes full row and full field enumeration.
- This optimization slice must not try to reduce the DB cost to create room.
- Focus only on netflow-owned processing after payload enumeration.
- The next analysis step is to isolate exactly the CPU work the plugin adds after each payload is already available.

## 2026-03-26: user design correction for the optimization model

### User decision

- The optimization must not assume that all rows contain all fields.
- The optimization must not assume a globally fixed full-field order across rows.
- The viable direction is:
  - learn field positions/order adaptively
  - tolerate missing fields safely
  - provide a very fast lookup path for the few fields the hot grouped query actually needs
- No `journal-core` changes are allowed for this work.

### Evidence backing the decision

- The current benchmark dataset does not have a stable full-field order across rows.
- Direct `journalctl -o export` inspection on the fixed raw snapshot showed different field orders between adjacent rows in the same file.
- Therefore a "hard-coded row layout" optimization would be invalid for this dataset.

### Implementation implication

- The fast path must be implemented entirely in the netflow plugin.
- It must treat row layout as partially predictable at best:
  - use hints / learned positions
  - verify matches cheaply
  - fall back safely when a field is absent or appears elsewhere
- The target query for the first fast path remains:
  - `group_by = ["SRC_ADDR", "DST_ADDR", "PROTOCOL"]`

## 2026-03-26: user-provided fast field lookup direction

### User decision

- For the hot path, replace generic per-payload field dispatch with a very small pending field set.
- For each interesting field, precompute:
  - a scalar prefix from the first up to 8 bytes of the key
  - the full key bytes for final verification
- While scanning payloads in a row:
  - compare payload key prefix against the small pending set
  - verify full key only on prefix hit
  - on match, remove the field from the pending set by swap-removing the matched slot
  - stop doing key comparisons for that field for the rest of the row
- Missing fields remain unresolved at end-of-row without breaking correctness.

### Implementation implication

- The first optimization step is:
  - plugin-only, no `journal-core` changes
  - small-array field matching
  - no fixed field-order assumption
- The tracked query fields must be derived from the request at runtime.
- Do not hardcode a specific `group_by` field set into the fast path.
- If this is still too slow after measurement:
  - consider ordered-prefix search or stronger learned-slot hints as a second step

## 2026-03-26: current branch benchmark after plugin-only matcher work

### Clean repeated benchmark on the fixed 4-file snapshot

- Raw `journal-core` baseline on the current branch, 3 runs, pinned to CPU 3:
  - rows: `745,969`
  - fields: `24,537,589`
  - fields/row: `32.8936`
  - total time: `896,106.667 +/- 6,269.712 usec`
  - time/row: `1.201265 +/- 0.008405 usec`

- Full netflow grouped query on the same dataset, same branch state, 3 clean runs, pinned to CPU 3:
  - query: `group_by = ["SRC_ADDR", "DST_ADDR", "PROTOCOL"]`
  - warm time: `2,901.730 +/- 35.755 ms`
  - warm time/row: `3.889880 +/- 0.047931 usec`
  - cold time: `3,284.187 +/- 31.848 ms`
  - cold time/row: `4.402578 +/- 0.042693 usec`

### Current plugin-owned overhead

- Warm netflow overhead over the raw DB baseline:
  - `2.688615 +/- 0.048663 usec/row`
- Warm slowdown versus raw DB:
  - `3.238152x`

### Important fact

- The current plugin-only work is still nowhere near the required budget.
- The user requirement remains:
  - plugin overhead must be at most `10%` of the DB work
  - with the current raw baseline, that means about `0.120127 usec/row`
- Current warm plugin overhead is still about:
  - `22.38x` above that budget

## 2026-03-26: validated hotspot finding in the current branch

### Evidence

- In `journal-session`, `Cursor::payloads()` copies uncompressed payloads into an internal buffer:
  - `self.buf.clear();`
  - `self.buf.extend_from_slice(guard.raw_payload());`
  - file: `src/crates/journal-session/src/cursor.rs`

- In the same file, `Cursor::visit_payloads()` passes `guard.raw_payload()` directly to the visitor for uncompressed payloads and only decompresses when needed.

- The projected grouped scan in `netflow-plugin` was using `cursor.payloads()` in:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`

### Result

- Switching the projected grouped scan from `cursor.payloads()` to `cursor.visit_payloads()` reduced the clean repeated warm benchmark from about:
  - `4.077024 usec/row`
  - to `3.889880 usec/row`
- This is a real improvement, but only about:
  - `4.59%`
- It is helpful, but not remotely enough.

### Coarse phase timing in the benchmark worktree

- A coarse timing pass on the projected raw grouped scan showed:
  - payload scan: about `2.608212 usec/row`
  - row accumulation: about `0.006388 usec/row`
- So the dominant remaining cost is still in the payload scan path itself.
- The direct tuple/group accumulator is not the bottleneck anymore.

### Engineering implication

- Further optimization should focus on the projected raw payload scan path in the plugin.
- The next serious option, if the current plugin-only matcher path cannot be reduced dramatically further, is:
  - a plugin-owned direct raw `journal-core` scan path for raw-only projected grouped queries
  - no `journal-core` changes
  - avoid `journal-session` overhead entirely for the hot raw benchmark path

### 2026-03-26: corrected optimization direction

- The raw DB budget already includes:
  - opening each file
  - stepping every row
  - enumerating every field
  - `data_ref()` on every field
- So the remaining optimization target is strictly the netflow plugin CPU added after each payload is delivered.
- The current branch still measures about:
  - raw `journal-core`: `1.201265 +/- 0.008405 usec/row`
  - full raw grouped netflow query: `3.889880 +/- 0.047931 usec/row`
  - plugin overhead: `2.688615 +/- 0.048663 usec/row`
- Costa's explicit requirement is:
  - no `journal-core` changes for this slice
  - optimize the netflow plugin only
  - target plugin overhead budget within `10%` of raw DB work
- The next implementation step is therefore:
  - add a plugin-owned direct raw scan path for raw-tier projected grouped queries
  - preserve full field enumeration
  - remove `journal-session` overhead from this hot path only
  - keep the field matcher request-driven, with no hardcoded field-order assumptions

### 2026-03-26: direct raw projected grouped path benchmark

- Implemented a plugin-only direct raw scan path for raw-tier projected grouped queries in:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
- This path:
  - keeps full row and full field enumeration
  - removes `journal-session` from the hot raw grouped path
  - keeps the field matcher request-driven
  - does not assume stable field order
  - does not require `journal-core` changes

### Fixed snapshot benchmark after the direct raw path

- Dataset:
  - the 4 fixed raw files in `/tmp/netdata-raw-snapshot-1774481723`
  - `745,969` rows
  - `24,537,589` fields
  - `32.8936` fields/row

- Raw `journal-core` baseline, 3 runs, pinned to CPU `3`:
  - totals: `992,570`, `960,127`, `941,588 usec`
  - mean: `964,761.667 +/- 25,805.061 usec`
  - per row: `1.293300 +/- 0.034593 usec`

- Full netflow grouped query, 3 runs, same snapshot, same CPU pin:
  - query:
    - `group_by = ["SRC_ADDR", "DST_ADDR", "PROTOCOL"]`
  - warm totals:
    - `2,728,110`, `2,747,910`, `2,794,840 usec`
  - warm mean:
    - `2,756,953.333 +/- 34,271.849 usec`
  - warm per row:
    - `3.695801 +/- 0.045943 usec`
  - cold totals:
    - `3,156,490`, `3,127,640`, `3,107,200 usec`
  - cold mean:
    - `3,130,443.333 +/- 24,764.289 usec`
  - cold per row:
    - `4.196479 +/- 0.033197 usec`

### Measured effect

- Warm plugin overhead over raw baseline:
  - `2.402502 usec/row`
- Cold plugin overhead over raw baseline:
  - `2.903179 usec/row`
- Warm slowdown over raw:
  - `2.857652x`
- This is an improvement over the previous plugin-only matcher result:
  - previous warm:
    - `3.889880 usec/row`
  - current warm:
    - `3.695801 usec/row`
  - improvement:
    - `0.194079 usec/row`
    - about `4.99%`

### Brutal truth

- The direct raw path is better.
- It is still nowhere near Costa's budget of plugin overhead within `10%` of raw DB work.
- The branch still needs more netflow-plugin optimization in the hot projected grouped scan path.

### 2026-03-26: clean remeasurement of the current branch state

- Re-ran the benchmarks sequentially after reverting the slower first-byte bucket matcher experiment.
- Current branch state:
  - direct raw projected grouped path kept
  - duplicate per-row entry lookup removed
  - first-byte bucket matcher reverted because it regressed the warm benchmark

### Current clean baseline

- Raw `journal-core`, 3 sequential runs, pinned to CPU `3`:
  - totals:
    - `906,278`
    - `912,024`
    - `952,796 usec`
  - mean:
    - `923,699.333 +/- 25,361.706 usec`
  - per row:
    - `1.238254 +/- 0.033998 usec`

- Full netflow grouped query on the same fixed snapshot, 3 sequential runs:
  - warm totals:
    - `2,723,270`
    - `2,756,250`
    - `2,741,320 usec`
  - warm mean:
    - `2,740,280.000 +/- 16,514.578 usec`
  - warm per row:
    - `3.673450 +/- 0.022138 usec`

- Cold totals from the same 3 runs:
  - `5,874,880`
  - `3,177,960`
  - `3,352,090 usec`
  - mean:
    - `4,134,976.667 +/- 1,509,313.758 usec`
  - per row:
    - `5.543095 +/- 2.023293 usec`

### Current measured overhead

- Warm plugin overhead over raw baseline:
  - `2.435196 usec/row`
- Warm slowdown over raw:
  - `2.966636x`

### Current conclusion

- The current branch is better than the earlier `journal-session`-based projected path.
- It still exceeds the CPU budget by a large margin.
- The remaining work is in the netflow plugin hot raw grouped scan path, not in `journal-core`.

### 2026-03-26: stage-by-stage measurement objective

- Goal:
  - isolate the cost of each netflow processing stage after raw DB enumeration
  - use the same fixed 4-file raw snapshot
  - keep the same query shape:
    - `group_by = ["SRC_ADDR", "DST_ADDR", "PROTOCOL"]`

- Known baseline:
  - steps `1+2+3` together are the raw DB cost:
    - `1.238254 +/- 0.033998 usec/row`

- Next measurement method:
  - add benchmark modes that stop early after each logical stage
  - run them sequentially on the same dataset
  - compare incremental per-row cost for:
    - `4` keep only interesting extracted fields
    - `5` parse metrics
    - `6` accumulate grouped combinations
    - `7` time-series second-pass bucket filling
    - `8+9+10` top-N, materialization/enrichment, output generation

- Important constraint:
  - these measurements must remain plugin-only
  - no `journal-core` changes

- Immediate next benchmark:
  - measure `1+2+3` from the netflow plugin view itself
  - use the same fixed snapshot and the same raw-reader integration that the later staged measurements will use
  - this will reveal any plugin-side overhead from file/session setup, query planning, or row/field integration even before interesting-field extraction starts

### 2026-03-26: plugin-view `1+2+3` baseline

- Added a test-only scan-only benchmark path in:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/main.rs`
- This benchmark:
  - uses the plugin's raw reader integration
  - uses the same fixed 4-file snapshot
  - does:
    - open files
    - apply raw reader matches
    - step all rows
    - enumerate all row fields
    - `data_ref()` every field object
  - does **not** yet do:
    - interesting-field matching
    - metric parsing
    - grouping
    - ranking
    - enrichment
    - output generation

- Results, 3 runs, pinned to CPU `3`:
  - totals:
    - `1,970,636`
    - `1,966,429`
    - `1,981,749 usec`
  - rows:
    - `745,969`
  - fields:
    - `24,537,589`
  - mean:
    - `1,972,938.000 +/- 7,915.175 usec`
  - per row:
    - `2.644799 +/- 0.010611 usec`

- Comparison to the direct raw `journal-core` harness on the same snapshot:
  - direct raw:
    - `1.238254 +/- 0.033998 usec/row`
  - plugin-view scan-only:
    - `2.644799 +/- 0.010611 usec/row`
  - plugin-side overhead before interesting-field extraction:
    - `1.406545 usec/row`
  - slowdown:
    - `2.135909x`

### Brutal truth

- A large part of the cost is already present before step `4`.
- The plugin-side integration path for `1+2+3` is already much slower than the direct `journal-core` baseline.
- This means the remaining optimization work cannot focus only on interesting-field matching and grouping.

### 2026-03-26: corrected same-profile `1+2+3` comparison

- The previous comparison between:
  - standalone direct raw harness
  - plugin-side scan-only harness
  was invalid.
- Reason:
  - they were not built and run in the same profile/binary setup.

- Added a direct `journal-core` raw scan harness inside the `netflow-plugin` test binary itself:
  - `profile_fixed_raw_direct_journal_core_against_local_journals()`
- Re-ran both harnesses in the same `release` test binary, pinned to CPU `3`.

- Direct `journal-core` in the `netflow-plugin` release test binary:
  - totals:
    - `1,740,637`
    - `1,734,834`
    - `1,841,672 usec`
  - mean:
    - `1,772,381.000 +/- 60,077.872 usec`
  - per row:
    - `2.375945 +/- 0.080537 usec`

- Plugin-view scan-only in the same release test binary:
  - totals:
    - `1,768,206`
    - `1,796,246`
    - `1,755,223 usec`
  - mean:
    - `1,773,225.000 +/- 20,966.984 usec`
  - per row:
    - `2.377076 +/- 0.028107 usec`

- Difference:
  - plugin minus direct:
    - `0.001131 usec/row`
  - ratio:
    - `1.000476x`

### Corrected conclusion

- There is effectively **no plugin integration overhead** in `1+2+3`.
- The earlier claim that plugin-side `1+2+3` was much slower was a benchmark methodology error.
- The real overhead starts after `1+2+3`, in the later netflow processing stages.

### 2026-03-26: release profile decision for this branch

- Verified current workspace profiles in `src/crates/Cargo.toml`:
  - `[profile.release]` uses `opt-level = "z"`
  - `[profile.release-min]` already exists for minimum-size artifacts:
    - `opt-level = "z"`
    - `lto = "fat"`
    - `codegen-units = 1`
    - `strip = true`
    - `panic = "abort"`
- User decision:
  - persist a speed-oriented Rust release configuration in this branch
  - keep `release-min` as the explicit size-oriented profile
- Rationale:
  - the measured `~2.37 usec/row` same-binary `1+2+3` numbers were produced under the workspace `release` profile optimized for size
  - when forcing `CARGO_PROFILE_RELEASE_OPT_LEVEL=3`, the same harnesses dropped to about `~1.2 usec/row`
  - therefore the branch should use a speed-oriented default `release` profile for meaningful performance work and release validation

### 2026-03-26: persisted release profile verification

- Applied:
  - `src/crates/Cargo.toml`
  - changed `[profile.release] opt-level` from `"z"` to `3`
- Kept unchanged:
  - `[profile.release-min]` remains the explicit size-oriented profile

- Re-ran both `1+2+3` harnesses under the persisted `release` profile, 3 runs each:
  - direct `journal-core` in the `netflow-plugin` release test binary:
    - totals:
      - `931,535`
      - `931,169`
      - `974,938 usec`
    - mean:
      - `945,880.667 +/- 25,165.054 usec`
    - per row:
      - `1.267989 +/- 0.033735 usec`
  - plugin scan-only in the same binary:
    - totals:
      - `951,671`
      - `963,551`
      - `969,299 usec`
    - mean:
      - `961,507.000 +/- 8,989.997 usec`
    - per row:
      - `1.288937 +/- 0.012051 usec`

- Validation:
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --release --quiet`
  - result:
    - `256 passed; 0 failed; 6 ignored`

### Current conclusion

- Persisting a speed-oriented `release` profile fixes the misleading `~2.3 usec/row` baseline.
- Direct and plugin `1+2+3` remain effectively aligned in the same persisted profile.
- This is now the correct baseline for measuring the later netflow processing stages.

### 2026-03-26: corrected grouped-query stage breakdown on speed-oriented `release`

- Added test-only cumulative stage benchmarks in `netflow-plugin` for the fixed 4-file raw dataset.
- Stage definitions used for the grouped `SRC_ADDR`, `DST_ADDR`, `PROTOCOL` benchmark:
  - `1+2+3`
    - plugin-side raw scan only
    - open files, apply reader matches, step rows, enumerate field objects, `data_ref()` all payloads
  - `1+2+3+4`
    - `1+2+3` plus interesting-payload matching and value extraction for non-metric interesting fields
    - no metric parsing
    - no group accumulation
  - `1+2+3+4+5`
    - `1+2+3+4` plus metric parsing for `BYTES`, `PACKETS`, `RAW_BYTES`, `RAW_PACKETS`
    - still no group accumulation
  - `1+2+3+4+5+6`
    - current grouped raw direct scan
    - includes group lookup / missing-value handling / row accumulation
  - full grouped query:
    - current `query_flows()` warm path
    - includes ranking, materialization, and output generation

- Repeated 3 times on the same fixed dataset:
  - rows:
    - `745,969`
  - fields:
    - `24,537,589`

- Results (`usec/row`, mean +/- stddev):
  - `1+2+3`:
    - `1.233569 +/- 0.014967`
  - `1+2+3+4`:
    - `1.741017 +/- 0.023142`
  - `1+2+3+4+5`:
    - `1.749772 +/- 0.019734`
  - `1+2+3+4+5+6`:
    - `1.927559 +/- 0.028021`
  - full grouped query warm:
    - `2.026608 +/- 0.021329`

- Derived stage costs (`usec/row`, from adjacent cumulative means):
  - step `4`:
    - `0.507448`
  - step `5`:
    - `0.008755`
  - step `6`:
    - `0.177787`
  - grouped-query work after step `6` (`8+9+10` combined):
    - `0.099049`

### Current interpretation

- The dominant plugin-owned cost is now step `4`:
  - interesting-field matching / extraction
- Metric parsing is almost irrelevant on this workload:
  - step `5` is only about `0.009 usec/row`
- Group accumulation is meaningful but not dominant:
  - step `6` is about `0.178 usec/row`
- Ranking / materialization / output are smaller still:
  - about `0.099 usec/row`

### Brutal truth

- The main optimization target is the interesting-field matcher, not metric parsing.
- The data now supports the user's direction:
  - we need a much faster small-set field lookup path for the projected raw scan.

### 2026-03-26: step 4 alternatives, narrowed to plugin-only work

- Fixed facts from the current code:
  - step `4` is the dominant remaining plugin cost
    - about `0.44 - 0.51 usec/row` on the fixed 4-file dataset
  - the raw grouped path currently always asks for:
    - `BYTES`
    - `PACKETS`
    - `RAW_BYTES`
    - `RAW_PACKETS`
    - all `group_by` fields
    - any captured selection fields
    - `SRC_ADDR` / `DST_ADDR` identity fingerprints
  - the grouped response surface currently serializes only:
    - `metrics.bytes`
    - `metrics.packets`
    - not `raw_bytes` / `raw_packets`
  - the current first-byte bucket matcher only improved step `4` modestly:
    - from about `0.454 usec/row`
    - to about `0.440 usec/row`

- Plugin-only alternatives for step `4`:

  - `A.` Minimize the required interesting-field set per query.
    - Background:
      - the raw grouped path still unconditionally includes `RAW_BYTES` and `RAW_PACKETS`
      - it also computes `SRC_ADDR` / `DST_ADDR` identity hashes even when those fields are already in `group_by`
    - Working direction:
      - only request `RAW_*` when the current query semantics actually require them
      - skip `src_hash` / `dst_hash` work when `SRC_ADDR` / `DST_ADDR` are already grouping keys
      - keep captured fields empty when `request.selections` is empty
    - Pros:
      - attacks step `4` directly
      - small surface area
      - no `journal-core` changes
    - Cons:
      - needs careful semantic audit for sampling and mixed-endpoint rendering
    - Implications:
      - the projected matcher finishes earlier
      - fewer per-row interesting fields remain pending
    - Risks:
      - if `RAW_*` are needed by some grouped/time-series semantics, dropping them blindly would break correctness

  - `B.` Keep the same required fields, but switch step `4` to byte-based extraction with late string conversion.
    - Background:
      - step `4` currently still calls `payload_value()` and works with `&str` / `String` for interesting fields
      - this is expensive and happens before the real group accumulation stage
    - Working direction:
      - keep values as raw bytes during the scan
      - intern / fingerprint / compare on bytes
      - convert to UTF-8 text only when materializing the final top-N rows
    - Pros:
      - removes a large amount of per-row text work from the hot scan
      - fully general
      - does not depend on field order
    - Cons:
      - more invasive than `A`
      - needs byte-key tables or byte-aware projected field storage
    - Implications:
      - group accumulation logic becomes byte-oriented on the hot path
      - text decoding moves to the cold materialization path
    - Risks:
      - more refactoring
      - easy to introduce subtle behavior differences if byte/value normalization is not preserved

  - `C.` Replace the current first-byte matcher with a stronger query-local small-set matcher.
    - Background:
      - the current matcher uses first-byte buckets and full key match
      - the measured gain was real but small
    - Working direction:
      - compile a per-query small array of requested keys
      - store an 8-byte prefix, key length, and the full key
      - on each payload:
        - compare prefix first
        - then compare full key only on prefix match
        - remove satisfied slots from the remaining set
    - Pros:
      - directly matches the current small-request shape
      - cache friendly
      - does not assume field order
    - Cons:
      - still pays one matcher pass per processed payload
      - may not be enough by itself
    - Implications:
      - matcher logic stays generic, but more specialized for tiny query plans
    - Risks:
      - modest gains only, like the current first-byte plan

  - `D.` Combine `A` and `B`.
    - Background:
      - `A` cuts how many interesting fields must be satisfied
      - `B` cuts the per-match cost for the fields that remain
    - Pros:
      - highest probability of a large win
      - still plugin-only
    - Cons:
      - biggest implementation scope
    - Implications:
      - requires a semantic audit first, then a hot-path refactor
    - Risks:
      - larger code churn
      - needs strong benchmark coverage after each sub-step

  - `E.` Do nothing at step `4` and optimize only step `6+`.
    - Brutal truth:
      - the measurements do not support this
    - Risks:
      - low return
      - wrong bottleneck

### 2026-03-26: user decision recorded

- Costa decided:
  - remove grouped endpoint uniqueness tracking for netflow grouped queries
  - remove grouped endpoint aggregation state (`src_hash`, `dst_hash`, `src_mixed`, `dst_mixed`)
  - remove grouped top-level `src` / `dst` exposure from the backend output
- Reason:
  - `src` / `dst` are not part of grouping unless explicitly in `group_by`
  - when they are in `group_by`, extra endpoint-mixing tracking is redundant
  - the current flows UI does not consume grouped top-level `src` / `dst`
- Implementation scope:
  - netflow plugin only
  - no `journal-core` changes

### 2026-03-26: next measurement objective

- User requirement:
  - keep the corrected speed-oriented `release` baseline
  - measure the cumulative query pipeline stages after `1+2+3`
  - specifically establish:
    - `1+2+3`
    - `1+2+3+4`
    - `1+2+3+4+5`
    - `1+2+3+4+5+6`
  - use the same fixed 4-file raw dataset and the same netflow plugin view
- Important constraint:
  - do not attribute any more cost to `journal-core`
  - isolate only the plugin-owned work after raw row/field enumeration
- Engineering note:
  - step `4` and step `5` are partially fused in the hot payload matcher
  - therefore exact stage measurement requires test-only cutoffs inside the current projected raw scan path, not just stopping later in the pipeline

### 2026-03-26: grouped endpoint removal implemented

- Implemented in:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
- Removed:
  - grouped endpoint uniqueness tracking
  - grouped endpoint aggregation state
  - grouped top-level `src` / `dst` emission from grouped flow output
  - representative endpoint snapshots used only for grouped endpoint materialization
- Kept intact:
  - grouping by `key`
  - metrics accumulation
  - top-N ranking
  - grouped `key` / `metrics` output shape
- Validation:
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --release --quiet`
  - result: passed (`256 passed; 0 failed; 7 ignored`)

### 2026-03-26: post-removal benchmark results

- Dataset:
  - same fixed 4-file raw snapshot
  - `745,969` rows
  - `24,537,589` fields
- CPU pinning:
  - `taskset -c 3`
- Measurements repeated 3 times using the ignored local benchmark harnesses in `src/crates/netdata-netflow/netflow-plugin/src/main.rs`

- Plugin `1+2+3` scan-only:
  - runs: `936,543`, `961,266`, `943,002` usec
  - mean: `946,937 +/- 12,823 usec`
  - mean per row: `1.269405 +/- 0.017189 usec/row`

- `1+2+3+4`:
  - runs: `1,250,423`, `1,269,453`, `1,275,899` usec
  - mean: `1,265,258 +/- 13,246 usec`
  - mean per row: `1.696127 +/- 0.017757 usec/row`

- `1+2+3+4+5`:
  - runs: `1,293,310`, `1,260,200`, `1,274,355` usec
  - mean: `1,275,955 +/- 16,613 usec`
  - mean per row: `1.710467 +/- 0.022270 usec/row`

- `1+2+3+4+5+6`:
  - runs: `1,391,151`, `1,378,293`, `1,371,339` usec
  - mean: `1,380,261 +/- 10,052 usec`
  - mean per row: `1.850293 +/- 0.013474 usec/row`

- Full grouped query warm:
  - runs: `1,489,810`, `1,456,530`, `1,491,330` usec
  - mean: `1,479,223 +/- 19,668 usec`
  - mean per row: `1.982956 +/- 0.026365 usec/row`

- Derived stage costs after removal:
  - step `4`: `0.426722 usec/row`
  - step `5`: `0.014339 usec/row`
  - step `6`: `0.139826 usec/row`
  - later grouped work (`8+9+10` combined): `0.132663 usec/row`

- Brutal truth:
  - removing grouped endpoint tracking did not produce a large win
  - the dominant remaining plugin cost is still step `4` field matching / extraction itself

### 2026-03-26: user decision recorded

- Costa decided:
  - `BYTES` / `PACKETS` become canonical counters scaled at ingestion time to estimated unsampled values
  - `RAW_BYTES` / `RAW_PACKETS` / `SAMPLING_RATE` remain persisted only for debugging/future use
  - normal query, grouping, selection, tiering, and presentation paths must not read back or expose `RAW_*` / `SAMPLING_RATE`
  - `RAW_BYTES`, `RAW_PACKETS`, `SAMPLING_RATE` must be encoded at the end of raw journal rows
  - ignore past data completely
  - add no backward compatibility and no migration logic

### 2026-03-26: canonical counter semantics implemented

- Implemented in:
  - `src/crates/netdata-netflow/netflow-plugin/src/decoder.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/tiering.rs`
  - `src/crates/netdata-netflow/netflow-plugin/src/ingest.rs`
- Key changes:
  - ingestion now scales canonical `BYTES` / `PACKETS`
  - `RAW_BYTES` / `RAW_PACKETS` are preserved/backfilled but hidden from normal query paths
  - `SAMPLING_RATE` is no longer groupable or selectable in normal flow queries
  - raw projected scan now matches only canonical metric fields
  - tiering no longer carries `RAW_*` or `SAMPLING_RATE` in normal rollup/query semantics
- Validation:
  - `cargo test --manifest-path src/crates/Cargo.toml -p netflow-plugin --quiet`
  - result: passed (`257 passed; 0 failed; 7 ignored`)

### 2026-03-26: first-byte early reject matcher cut

- Implemented in:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
- Change:
  - the planned matcher now uses `first_byte_masks[payload[0]] & remaining_mask` first
  - when the candidate mask is zero, it returns before computing the payload prefix
- Result:
  - this materially reduced step `4` and the full warm grouped query cost

### 2026-03-26: learned-slot probe experiment failed and was reverted

- Working theory tested:
  - learn likely row positions for requested fields, probe those positions first, then scan the remaining fields
- Reality:
  - this did not reduce the real work
  - measured `processed_fields` stayed at `17,766,506`
  - cumulative times got worse
- Brutal truth:
  - field order is not stable enough in this dataset for the naive learned-slot probe to help
  - the added probe bookkeeping only added overhead
- Status:
  - reverted from `src/crates/netdata-netflow/netflow-plugin/src/query.rs`

### 2026-03-26: current clean benchmark state after reverting learned-slot probing

- Dataset:
  - fixed 4-file raw snapshot
  - `745,969` rows
  - `24,537,589` fields
  - `32.8936` fields/row
- CPU pinning:
  - `taskset -c 3`
- All measurements below are 3-run means from the current branch state after the revert.

- Plugin `1+2+3` scan-only:
  - runs: `961,550`, `938,998`, `950,797` usec
  - mean: `950,448 +/- 11,281 usec`
  - mean per row: `1.274113 +/- 0.015121 usec/row`

- Match-only:
  - runs: `1,241,120`, `1,241,548`, `1,224,688` usec
  - mean: `1,235,785 +/- 9,615 usec`
  - mean per row: `1.656618 +/- 0.012887 usec/row`
  - processed fields: `20,399,739`

- `1+2+3+4`:
  - runs: `1,257,417`, `1,271,594`, `1,261,885` usec
  - mean: `1,263,632 +/- 7,247 usec`
  - mean per row: `1.693947 +/- 0.009716 usec/row`
  - processed fields: `20,399,739`

- `1+2+3+4+5`:
  - runs: `1,299,933`, `1,282,684`, `1,277,047` usec
  - mean: `1,286,555 +/- 11,924 usec`
  - mean per row: `1.724676 +/- 0.015985 usec/row`
  - processed fields: `20,399,739`

- `1+2+3+4+5+6`:
  - runs: `1,376,023`, `1,386,588`, `1,378,754` usec
  - mean: `1,380,455 +/- 5,482 usec`
  - mean per row: `1.850553 +/- 0.007351 usec/row`
  - grouped rows: `23,724`
  - processed fields: `20,399,739`

- Full grouped query warm:
  - runs: `1,466.22`, `1,459.86`, `1,442.72` ms
  - mean: `1,456.27 +/- 12.15 ms`
  - mean per row: `1.952181 +/- 0.016294 usec/row`
  - grouped rows: `23,724`
  - returned rows: `26`

- Derived costs from the current clean state:
  - match-only over `1+2+3`: `0.382505 usec/row`
  - step `4` extraction over match-only: `0.037329 usec/row`
  - step `5`: `0.030729 usec/row`
  - step `6`: `0.125877 usec/row`
  - later grouped work (`8+9+10` combined): `0.101628 usec/row`

- Brutal truth:
  - the dominant remaining cost is still the matching stage itself
  - extraction beyond matching is small
  - grouping and later output work matter, but they are smaller than the matching cost

### 2026-03-26: prefix-only matcher specialization implemented

- Implemented in:
  - `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
- Change:
  - when all requested keys fit in the first 8 bytes, the planned matcher now skips the extra full `starts_with()` check
  - the prefix loader now also has a fast path for payloads at least 8 bytes long
- Important constraint:
  - this is still generic
  - it does not hardcode field names
  - it is enabled only when the query-local projected field set fits the prefix-only condition

### 2026-03-26: benchmark after prefix-only matcher specialization

- Dataset:
  - same fixed 4-file raw snapshot
  - `745,969` rows
  - `24,537,589` fields
  - `32.8936` fields/row
- CPU pinning:
  - `taskset -c 3`
- `1+2+3` baseline is unchanged from the current clean-state scan-only harness:
  - `1.274113 +/- 0.015121 usec/row`

- Match-only:
  - runs: `1.648578`, `1.591155`, `1.562619` usec/row
  - mean: `1.600784 +/- 0.043781 usec/row`
  - delta over scan-only: `0.326671 usec/row`
  - processed fields: `20,399,739`

- `1+2+3+4`:
  - runs: `1.694605`, `1.611004`, `1.614084` usec/row
  - mean: `1.639898 +/- 0.047403 usec/row`
  - delta over scan-only: `0.365785 usec/row`
  - processed fields: `20,399,739`

- `1+2+3+4+5`:
  - runs: `1.676468`, `1.641519`, `1.600033` usec/row
  - mean: `1.639340 +/- 0.038264 usec/row`
  - delta over scan-only: `0.365227 usec/row`
  - Noise note:
    - the tiny negative step-5 delta versus stage 4 is benchmark noise
    - the real conclusion remains that metric parsing is small

- `1+2+3+4+5+6`:
  - runs: `1.803434`, `1.742299`, `1.752381` usec/row
  - mean: `1.766038 +/- 0.032776 usec/row`
  - delta over scan-only: `0.491925 usec/row`
  - grouped rows: `23,724`

- Full grouped query warm:
  - runs: `1,351.27`, `1,364.38`, `1,377.30` ms
  - mean: `1,364.32 +/- 13.02 ms`
  - mean per row: `1.828919 +/- 0.017447 usec/row`
  - delta over scan-only: `0.554806 usec/row`
  - grouped rows: `23,724`
  - returned rows: `26`

- Derived current stage contributions:
  - match-only over `1+2+3`: `0.326671 usec/row`
  - step `4` extraction over match-only: `0.039114 usec/row`
  - step `5`: effectively noise-level in this benchmark
  - step `6`: `0.126698 usec/row`
  - later grouped work (`8+9+10` combined): `0.062881 usec/row`

- Comparison to the prior clean state:
  - previous warm grouped query mean: `1.952181 usec/row`
  - current warm grouped query mean: `1.828919 usec/row`
  - improvement: `0.123262 usec/row`
  - relative improvement: about `6.31%`

- Brutal truth:
  - this matcher cut is real and measurable
  - the dominant remaining cost is still matching, but it is now materially smaller
  - the branch is still above the target budget

### 2026-03-26: field-order experiment considered but not kept

- Working theory:
  - moving the hot canonical fields to the front of journal rows could reduce processed fields before early-stop
- Decision:
  - not kept in the branch
- Reason:
  - the fixed 4-file benchmark snapshot is already written
  - changing encoder field order now would not change the measured dataset
  - keeping an unmeasured optimization in the branch would be dishonest

### 2026-03-26: current bottleneck interpretation and next experiment options

- Hard evidence from the current best measured state:
  - total fields read: `24,537,589`
  - processed fields in matcher path: `20,399,739`
  - processed fields per row: `27.346631`
  - total fields per row: `32.893577`
  - fields skipped after all required fields are found: only `5.546946` per row
  - processed ratio: `83.14%` of all fields
- Interpretation:
  - even after the matcher improvements, the hot query fields are still satisfied very late in the row
  - brute-force matcher tuning has limited remaining headroom if we keep scanning roughly `27.35` fields per row

- Candidate next experiments:
  - `A.` Regenerate journals with hot canonical fields first, then remeasure on a newly written fixed dataset
    - target order: `PROTOCOL`, `BYTES`, `PACKETS`, `FLOWS`, `SRC_ADDR`, `DST_ADDR`, then the rest, with `RAW_*` / `SAMPLING_RATE` last
    - purpose: reduce `processed_fields`, not DB `fields_read`
  - `B.` Keep dataset format unchanged and optimize row-local matching further
    - purpose: reduce CPU per processed field
    - expectation: smaller ceiling than `A`
  - `C.` Attack grouped field resolution / accumulator cost next
    - purpose: reduce stage `6` and later grouped work
    - expectation: worthwhile, but insufficient alone if `processed_fields` stays at `83%`

- Current recommendation:
  - `A` first, then `C`
  - Reason:
    - the measurements now strongly suggest row-local field order is the main remaining structural limiter
    - stage `6` is the second-best target after that

### 2026-03-26: pending user decision for the next step-4 experiment

- Purpose:
  - reduce the plugin-owned step `4` overhead between `1+2+3` and `1+2+3+4`
  - keep the DB budget unchanged
  - keep the work inside the netflow plugin

- Facts from the current code:
  - the step-4 benchmark path only does:
    - row-local reset
    - per-payload matching
    - light extraction for matched non-metric fields
  - it does **not** do:
    - metric parsing
    - accumulation
    - ranking
    - output generation
  - evidence:
    - benchmark stage setup in `src/crates/netdata-netflow/netflow-plugin/src/query.rs`
    - per-row resets at `row_group_field_ids.fill(None)` and `pending_spec_indexes.clear()`
    - matcher call via `benchmark_apply_projected_payload_planned(...)`

- Current hard evidence:
  - `1+2+3`: `1.274113 usec/row`
  - match-only: `1.600784 usec/row`
  - `1+2+3+4`: `1.639898 usec/row`
  - match-only delta: `0.326671 usec/row`
  - extraction delta over matching: `0.039114 usec/row`
  - matcher still processes `20,399,739 / 24,537,589` fields = `83.14%`
  - average processed fields per row: `27.346631`
  - average skipped-after-done fields per row: `5.546946`

- Decisions:
  - `1.` Which experiment should be done next?
    - `A.` Regenerate a fresh benchmark dataset with the hot canonical fields written first
      - Target order:
        - `PROTOCOL`, `BYTES`, `PACKETS`, `FLOWS`, `SRC_ADDR`, `DST_ADDR`
        - rest of the canonical fields
        - `RAW_BYTES`, `RAW_PACKETS`, `SAMPLING_RATE` last
      - Pros:
        - attacks the structural reason step `4` is still large
        - highest potential upside
        - keeps DB work unchanged and reduces `processed_fields`
      - Cons:
        - requires generating a new benchmark snapshot before measurement
      - Implications:
        - performance numbers will be compared on a new fixed dataset written by the new encoder layout
      - Risks:
        - if the hot fields are often absent, gain will be smaller than expected

    - `B.` Keep the dataset format unchanged and do another matcher-only experiment
      - Scope:
        - continue optimizing `benchmark_apply_projected_payload_planned()` / `apply_projected_payload_planned()`
      - Pros:
        - smaller code change
        - easiest to benchmark on the existing fixed dataset
      - Cons:
        - lower ceiling, because we still process `83.14%` of fields
      - Implications:
        - likely only another incremental win
      - Risks:
        - time spent on diminishing returns

    - `C.` Leave step `4` as it is and move to step `6`
      - Pros:
        - stage `6` still costs about `0.126698 usec/row`
      - Cons:
        - does not address the largest remaining cost
      - Implications:
        - full query improves, but step `4` stays structurally expensive
      - Risks:
        - likely not enough to reach the target budget

- Recommendation:
  - `1. A`
  - Reason:
    - the current measurements show the main remaining problem is not fancy matcher logic
    - it is that the required fields are found too late in the row
    - that is best attacked by changing the row layout and measuring it honestly on newly generated data

### 2026-03-26: user direction recorded

- Costa directed the next experiment to focus on the code, not on regenerating or reshaping the benchmark data.
- Implication:
  - the next step-4 experiment must stay on the current fixed dataset
  - no dataset/layout experiment first
  - the goal is to exploit the fact that the flow schema has a fixed, known field set

### 2026-03-26: user decision recorded

- Costa selected:
  - `1. A`
- Meaning:
  - replace step-4 key matching with a code-first fixed canonical field classifier experiment
  - stay on the current fixed benchmark dataset
  - benchmark the new field-id path against the current matcher on the same workload

### 2026-03-26: fixed canonical field classifier experiment result

- Result:
  - failed
  - the code-first fixed canonical field classifier is slower than the current matcher on the same fixed 4-file dataset
- Evidence:
  - release stage-breakdown runs:
    - match-only: `1.768103`, `1.765470`, `1.776260 usec/row`
    - `1+2+3+4`: `1.844661`, `1.844758`, `1.840744 usec/row`
  - compared to the last committed baseline:
    - match-only: `1.600784 +/- 0.043781 usec/row`
    - `1+2+3+4`: `1.639898 +/- 0.047403 usec/row`
  - full grouped query warm run:
    - `1543.63 ms` over `745,969` rows = `2.068009 usec/row`
  - compared to the last committed full grouped-query warm baseline:
    - `1.828919 +/- 0.017447 usec/row`
- Interpretation:
  - replacing the current per-query matcher with a global canonical field classifier adds overhead
  - the extra classification layer is not paying for itself on the current hot path
- Action:
  - revert this experiment from `query.rs`
  - keep the committed matcher path as the current best step-4 baseline

### 2026-03-26: cheap boundary precheck experiment result

- Result:
  - failed
  - reordering the planned matcher to check the candidate `=` boundary before loading the payload prefix did not improve the current fixed-dataset step-4 numbers
- Evidence:
  - release stage-breakdown run:
    - match-only: `1.595305 usec/row`
    - `1+2+3+4`: `1.610397 usec/row`
  - compared to recent revert baseline runs:
    - match-only: `1.557006`, `1.594348 usec/row`
    - `1+2+3+4`: `1.561278`, `1.579019 usec/row`
- Interpretation:
  - the extra branch and lazy-prefix logic do not pay for themselves on the hot path
- Action:
  - revert this experiment from `query.rs`

### 2026-03-26: direct single-bucket matcher experiment result

- Result:
  - failed
  - compiling the single-spec first-byte buckets into a direct path did not produce a stable end-to-end win
- Evidence:
  - release stage-breakdown runs:
    - match-only: `1.569656`, `1.594820`, `1.551483 usec/row`
    - `1+2+3+4`: `1.694521`, `1.580629`, `1.560180 usec/row`
  - full grouped-query warm run:
    - `1401.09 ms / 745,969 rows = 1.877413 usec/row`
  - compared to the committed warm grouped-query baseline:
    - `1.828919 +/- 0.017447 usec/row`
- Interpretation:
  - the direct-path branch can help some pure matching runs, but the effect is unstable and does not survive the full grouped query
- Action:
  - revert this experiment from `query.rs`

### 2026-03-26: step-4 false-positive instrumentation

- The current matcher hot spot is now verified with direct counters from the fixed raw benchmark:
  - processed fields before row completion: `20,399,739`
  - fields with interesting first bytes (`B`, `P`, `S`, `D`): `8,335,358`
  - immediate non-candidate fields before row completion: `12,064,381`
- Match-only attempt/match counts:
  - `BYTES`: attempts `745,969`, matches `745,969`
  - `PACKETS`: attempts `1,249,950`, matches `745,969`
  - `PROTOCOL`: attempts `745,969`, matches `745,969`
  - `SRC_ADDR`: attempts `2,428,257`, matches `745,969`
  - `DST_ADDR`: attempts `3,669,194`, matches `745,969`
- Interpretation:
  - step 4 is dominated by false positives on `SRC_ADDR` and `DST_ADDR`
  - `PACKETS` / `PROTOCOL` collision exists, but it is materially smaller than the two address-field false-positive families

### 2026-03-26: canonical quick-gate experiments

- One-gate per-spec canonical discriminator:
  - evidence:
    - stage run: match-only `1.579617 usec/row`
    - stage run: `1+2+3+4` `1.585884 usec/row`
    - sequential full grouped-query warm run: `1376.30 ms / 745,969 rows = 1.844983 usec/row`
  - interpretation:
    - this is the first code-only matcher tweak that looked directionally promising
    - but the full grouped-query gain versus the existing baseline is too small / too noisy to call it a real win yet

- Two-gate per-spec canonical discriminator:
  - evidence:
    - stage run: match-only `1.645869 usec/row`
    - stage run: `1+2+3+4` `1.651361 usec/row`
  - interpretation:
    - adding a second canonical gate did not help
    - the extra branch cost outweighed the additional filtering benefit on this path

### 2026-03-26: user direction after matcher experiments

- Costa directed:
  - revert the local matcher/instrumentation experiments
  - restore a clear state matching the last committed netflow code
  - explain the exact matcher loop in that clear state only

### 2026-03-26: user direction after failed classifier experiment

- Costa directed:
  - revert the failed canonical-field-classifier experiment
  - proceed with the next step-4 optimization experiment
- Constraints reaffirmed:
  - focus on code, not on regenerating benchmark data
  - keep using the same fixed 4-file dataset for comparisons
