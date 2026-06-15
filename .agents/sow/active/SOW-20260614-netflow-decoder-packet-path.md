# SOW-20260614-netflow-decoder-packet-path - NetFlow Decoder Packet-Path Duplicate Work

## Status

Status: completed

Sub-state: Implemented, validated, externally reviewed, and pushed-ready. Active SOW remains local working memory only.

## Requirements

### Purpose

Reduce duplicate packet-path decoder work while preserving NetFlow v9/IPFIX template state persistence, hydration, datalink decoding, and restart behavior.

### User Request

The user selected autonomous SOWs per improvement bucket. This SOW covers decoder packet-path duplicate work.

Parent inventory: `.agents/sow/active/SOW-20260614-netflow-overheads.md`

### Assistant Understanding

Facts:

- v9/IPFIX payloads are observed for decoder state before normal decode.
- Namespace key generation parses template scope and allocates exporter IP strings.
- Special datalink decode is enabled by global protocol presence, not necessarily current packet exporter/domain.
- Decoder scope snapshot is recomputed per packet.

Inferences:

- Some work may be integrated into the normal parse path or gated more narrowly.

Unknowns:

- Whether parser architecture allows safe single-pass template observation without large refactor.

### Acceptance Criteria

- Add or verify tests for v9/IPFIX template persistence and hydration.
- Add or verify tests for special datalink decode behavior.
- Add or verify tests for decoder scope metrics after namespace/hydration changes.
- Reduce duplicate parsing/allocation where tests prove equivalent behavior.
- Preserve restart recovery and template error behavior.

## Analysis

Sources checked:

- `src/crates/netflow-plugin/src/ingest/service/runtime.rs`
- `src/crates/netflow-plugin/src/decoder/state/runtime/decode.rs`
- `src/crates/netflow-plugin/src/decoder/state/runtime/observe.rs`
- `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs`
- `src/crates/netflow-plugin/src/decoder/protocol/entry.rs`
- `src/crates/netflow-plugin/src/decoder/protocol/v9/special.rs`
- `src/crates/netflow-plugin/src/decoder/protocol/ipfix/special/packet.rs`
- `src/crates/netflow-plugin/src/decoder/state/sampling/templates.rs`
- `src/crates/netflow-plugin/src/decoder/tests.rs`
- `src/crates/netflow-plugin/src/ingest_tests.rs`
- `src/crates/netflow-plugin/src/ingest_test_support.rs`
- Parent inventory SOW.
- RFC 3954, NetFlow v9.
- RFC 7011, IPFIX.
- Open-source references listed below.

Current state:

- Parent inventory records exact duplicate decode and namespace allocation evidence.
- `IngestService::handle_received_packet()` calls `prepare_decoder_state_namespace()` before every non-empty packet decode, then calls `FlowDecoders::decode_udp_payload_at()`.
- `prepare_decoder_state_namespace()` computes the normalized parser source and namespace key from the raw packet before decode.
- `decode_udp_payload_at()` immediately calls `observe_decoder_state_from_payload()`, which computes the namespace key again and parses the same version/source-id bytes again.
- `decode_netflow()` normalizes the source again for parser scoping.
- Special v9/IPFIX datalink decode is gated by global `has_any_*_datalink_templates()` predicates, so one datalink template can make unrelated packets pay the raw special decoder scan.
- Existing tests cover persisted v9/IPFIX template restore, datalink restore after restart, source-port churn, and parser scope reuse.

Risks:

- Decoder state persistence is required for restart behavior.
- Narrowing datalink gates incorrectly can drop valid decoded data.
- Hydration must stay before normal decode because persisted templates must be replayed into the parser before data records using those templates are parsed.
- Template scope must remain exporter IP plus observation domain/source ID. Source-port changes must keep reusing the same loaded namespace.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Packet-path decoder work includes duplicate packet header interpretation, duplicate namespace-key allocation, broad special decode gates, and per-packet metrics recomputation.
- The required ordering is: derive packet template scope, hydrate any preloaded persisted namespace, observe new templates/options/sampling from the current packet, then run normal decode.
- The waste is not the hydration step itself. The waste is computing the same scope and source information independently in hydration, observation, parser scoping, and special datalink gating.
- Special datalink decoding should be attempted only when the current packet's exporter IP and observation domain/source ID have a matching datalink-capable template for the current protocol.

Evidence reviewed:

- `src/crates/netflow-plugin/src/ingest/service/runtime.rs:137`: packet path calls `prepare_decoder_state_namespace(source, payload)`.
- `src/crates/netflow-plugin/src/ingest/service/runtime.rs:139`: same packet then calls `decode_udp_payload_at(source, payload, receive_time_usec)`.
- `src/crates/netflow-plugin/src/ingest/persistence.rs:94`: `prepare_decoder_state_namespace()` computes `normalize_template_scope_source(source)`.
- `src/crates/netflow-plugin/src/ingest/persistence.rs:95`: same function computes `FlowDecoders::decoder_state_namespace_key(source, payload)`.
- `src/crates/netflow-plugin/src/decoder/state/runtime/decode.rs:20`: decode calls `observe_decoder_state_from_payload(source, payload)` before normal decode.
- `src/crates/netflow-plugin/src/decoder/state/runtime/observe.rs:13`: observation recomputes `decoder_state_namespace_key(source, payload)`.
- `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs:22`: namespace key parsing calls `template_scope(payload)`.
- `src/crates/netflow-plugin/src/decoder/state/runtime/namespace.rs:24`: namespace key allocates `canonicalize_ip_addr(source.ip()).to_string()`.
- `src/crates/netflow-plugin/src/decoder/protocol/entry.rs:69`: v9 special decode is gated by global `has_any_v9_datalink_templates()`.
- `src/crates/netflow-plugin/src/decoder/protocol/entry.rs:82`: IPFIX special decode is gated by global `has_any_ipfix_datalink_templates()`.
- `src/crates/netflow-plugin/src/decoder/state/sampling/templates.rs:74`: v9 datalink templates are retrieved by exporter IP, observation domain, and template ID.
- `src/crates/netflow-plugin/src/decoder/state/sampling/templates.rs:112`: IPFIX datalink templates are retrieved by exporter IP, observation domain, and template ID.
- `src/crates/netflow-plugin/src/decoder/tests.rs:2710`: persisted v9 templates and sampling restore after restart.
- `src/crates/netflow-plugin/src/decoder/tests.rs:2754`: persisted v9 datalink templates restore after restart.
- `src/crates/netflow-plugin/src/decoder/tests.rs:2797`: persisted IPFIX templates restore after restart.
- `src/crates/netflow-plugin/src/decoder/tests.rs:2877`: parser scope reuses templates across source-port churn.
- `src/crates/netflow-plugin/src/decoder/tests.rs:2901`: loaded namespace reuse across source-port churn.
- RFC 3954 records that template ID uniqueness is local to the Observation Domain and data FlowSet IDs map to previous Template IDs.
- RFC 3954 records that collectors must use exporter source IP plus Source ID to separate v9 export streams.
- RFC 7011 records that IPFIX Template IDs are local to the Transport Session and Observation Domain and that different Observation Domains may reuse the same Template ID for different templates.

Affected contracts and surfaces:

- NetFlow v9/IPFIX template handling.
- Datalink fields.
- Decoder state files.
- Decoder internal charts.
- Internal Rust decoder APIs between ingest, decoder state, and protocol modules.

Clean-end-state target:

- Decoder packet path computes v9/IPFIX packet scope once per packet and reuses it for persisted namespace hydration, state observation, normal parser scoping, and special datalink gate decisions.
- Special datalink raw scans run only for packets whose current exporter IP plus observation domain/source ID has datalink-capable templates for that protocol.
- Persisted decoder state file format, filenames, decoded flow field names, chart IDs, and user configuration remain unchanged.
- Removed as redundant (i): duplicate packet-scope parsing and duplicate source normalization/key construction between ingest hydration and decoder observation; global datalink-template gate predicates in the production decode path.
- Excluded coupled items (ii): persisted decoder-state format changes are excluded because this SOW targets runtime overhead and there is no evidence a format change is needed; decoder scope chart display changes belong to the completed chart sampler SOW; enrichment-path optimizations belong to the enrichment hot path SOW.
- Reference search: `rg -n "decoder_state_namespace_key|normalize_template_scope_source|has_any_.*datalink|decode_udp_payload_at|decode_netflow\\(" src/crates/netflow-plugin/src` was used to identify affected internal call sites. No public metric/config/field/file contract replacement is planned, so no external contract migration is required.

Existing patterns to reuse:

- Existing decoder protocol tests.
- Existing decoder state namespace tests.
- Existing special datalink tests.
- Existing synthetic v9 datalink packet helpers in `decoder/tests.rs`.
- Existing ingestion restart tests in `ingest_tests.rs`.
- Existing `SamplingState` exporter/domain/template-keyed storage model.

Risk and blast radius:

- Medium/high correctness risk across v9/IPFIX deployments.
- Low security risk with synthetic packets.
- Implementation blast radius should stay inside `src/crates/netflow-plugin/src/decoder/**`, `src/crates/netflow-plugin/src/ingest/**`, and targeted Rust tests.
- No sensitive data or production packet data will be used.

Sensitive data handling plan:

- Use existing fixtures or synthetic packet payloads. Do not store production packet data in durable artifacts.

Implementation plan:

1. Add/strengthen tests first:
   - v9 scoped datalink gate does not decode when exporter IP or source ID differs.
   - IPFIX scoped datalink gate does not decode when exporter IP or observation domain differs.
   - Ingest/service path continues to hydrate persisted state before decoding data-only packets.
   - Decoder scope metrics still reflect namespaces and hydrated sources after context reuse.
2. Add an internal packet context for v9/IPFIX:
   - version.
   - exporter IP.
   - observation domain/source ID.
   - normalized parser source.
   - decoder namespace key.
3. Make ingest hydration return the packet context and pass it into decode.
4. Make decoder observation and normal parse reuse the packet context when present.
5. Replace production global special datalink gates with scoped current-packet predicates.
6. Keep compatibility wrappers for tests/benchmarks that call `decode_udp_payload_at()` directly.
7. Run targeted tests, then full `netflow-plugin` tests.

Validation plan:

- Targeted decoder tests.
- Targeted ingest tests for persisted state hydration.
- Production-shaped benchmark with v9/IPFIX protocol scenario if code changes affect the benchmark path.
- Full `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`.
- `git diff --check`.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: update if decoder state contract changes.
- End-user/operator docs: no update expected unless config/behavior changes.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Open-source reference evidence:

- `netsampler/goflow2 @ 6dee964c38ee5f6b04a38681d069427c28ee5cb3`
  - `decoders/netflow/template_store.go:8`: decoder-facing template store is keyed by router, version, observation domain, and template ID.
  - `utils/store/templates/store.go:13`: concrete store key includes router key, version, observation domain ID, and template ID.
- `phaag/nfdump @ 635bfdfd6c5ec7d88406c74b4689c583417db89b`
  - `src/netflow/netflow_v9.h:58`: Source ID identifies exporter observation domain.
  - `src/netflow/netflow_v9.h:60`: collector should use source IP plus Source ID to separate streams.
  - `src/netflow/netflow_v9.c:1717`: v9 packet processing reads source ID before looking up exporter state.
  - `src/netflow/netflow_v9.c:1814`: data flowset processing resolves template ID inside the selected exporter state.
- `cloudflare/goflow @ e5a8a4eabd73883a5b44c41ad8aa1f798d778f96`
  - `decoders/netflow/netflow.go:328`: IPFIX observation domain ID is decoded from the packet header.
  - `decoders/netflow/netflow.go:332`: decoded observation domain is used as packet scope.

Open decisions:

- None. This SOW can proceed without a new user-owned design decision because it keeps public contracts and persisted format unchanged.

## Implications And Decisions

1. User decision: autonomous SOW split and test-first requirement.
   - Selected.
   - Recommendation classification: long-term-best.

## Plan

1. Add/strengthen scoped datalink and hydration/scope tests.
2. Implement internal packet context reuse.
3. Narrow special datalink gates to current packet scope.
4. Run targeted validation and benchmark if available.
5. Run full crate validation and external reviewers before push.

## Execution Log

### 2026-06-14

- Created autonomous child SOW.

### 2026-06-15

- Rebased branch onto `upstream/master`; branch was already up to date.
- Pushed `origin/netflow-overheads`; remote was already up to date.
- Loaded project collector skill for NetFlow hot-path work.
- Audited decoder hydration, observation, namespace, sampling, and datalink paths.
- Checked RFC 3954, RFC 7011, and open-source flow collector implementations for template scoping.
- Completed pre-implementation gate.
- Added test-first coverage for scoped datalink template predicates and decoder packet context reuse.
- Implemented `DecoderPacketContext` for one v9/IPFIX packet-scope parse reused by ingest hydration, decoder observation, parser scoping, and datalink gate decisions.
- Replaced production global datalink gates with scoped current-packet exporter IP plus observation domain/source ID predicates.
- Updated ingest test helper to exercise the same context-aware hydration/decode flow as production.
- Ran first external review round. Reviewers found no runtime correctness or security regression; main actionable gap was missing end-to-end scoped v9 datalink negative coverage.
- Added `v9_datalink_decode_is_scoped_by_exporter_and_source_id` to prove v9 datalink output does not cross exporter IP or source ID scopes.
- Added a debug assertion for the invariant that template-state changes require a packet context, and documented that `DecoderPacketContext` carries derived hot-path scope fields.
- Added `ipfix_datalink_decode_is_scoped_by_exporter_and_observation_domain` after second-round review identified the missing symmetric IPFIX end-to-end scoped datalink coverage.

## Validation

Acceptance criteria evidence:

- Add or verify tests for v9/IPFIX template persistence and hydration:
  - Existing tests verified:
    - `persisted_decoder_state_restores_v9_templates_and_sampling_after_restart`
    - `persisted_decoder_state_restores_v9_datalink_templates_after_restart`
    - `persisted_decoder_state_restores_ipfix_templates_after_restart`
    - `ingest_service_restores_decoder_state_from_disk_after_restart`
- Add or verify tests for special datalink decode behavior:
  - Existing fixture/synthetic tests verified:
    - `akvorado_ipfix_datalink_fixture_matches_expected_projection`
    - `akvorado_ipfix_datalink_fixture_vxlan_mode_drops_non_encapsulated_record`
    - `synthetic_v9_datalink_special_decoder_matches_expected_projection`
    - `synthetic_v9_datalink_special_decoder_vxlan_mode_drops_non_encap_record`
  - Added `datalink_template_presence_is_scoped_by_exporter_and_observation_domain`.
  - Added `v9_datalink_decode_is_scoped_by_exporter_and_source_id`.
  - Added `ipfix_datalink_decode_is_scoped_by_exporter_and_observation_domain`.
- Add or verify tests for decoder scope metrics after namespace/hydration changes:
  - Existing source-port churn tests verified.
  - Added `decoder_packet_context_reuses_scope_and_normalized_source`.
  - Added `decoder_scope_snapshot_tracks_context_reused_namespace_and_hydration`.
- Reduce duplicate parsing/allocation where tests prove equivalent behavior:
  - `prepare_decoder_state_namespace()` now returns the packet context it already derived.
  - Production ingest path passes that context to `decode_udp_payload_at_with_context()`.
  - Decoder observation and parser scoping reuse the context instead of recomputing namespace key and normalized parser source.
  - Production datalink gates use scoped predicates instead of global `has_any_*` predicates.
- Preserve restart recovery and template error behavior:
  - Full crate tests and targeted persisted-state tests passed.

Tests or equivalent validation:

- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml datalink_template_presence_is_scoped_by_exporter_and_observation_domain -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml decoder_packet_context_reuses_scope_and_normalized_source -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml decoder_scope_snapshot_tracks_context_reused_namespace_and_hydration -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml v9_datalink_decode_is_scoped_by_exporter_and_source_id -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml ipfix_datalink_decode_is_scoped_by_exporter_and_observation_domain -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml persisted_decoder_state -- --nocapture`
  - Passed: 10 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml datalink -- --nocapture`
  - Passed before adding the IPFIX end-to-end scoped test: 11 passed.
  - Passed after adding the IPFIX end-to-end scoped test: 12 passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml source_port -- --nocapture`
  - Passed: 2 passed, 1 ignored manual profile.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml ingest_service_restores_decoder_state_from_disk_after_restart -- --nocapture`
  - Passed.
- `cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml`
  - Passed before first review fixes: 522 passed, 25 ignored; `tests/grpc_build.rs`: 1 passed.
  - Passed after first review fixes: 523 passed, 25 ignored; `tests/grpc_build.rs`: 1 passed.
  - Passed after adding the IPFIX end-to-end scoped test: 524 passed, 25 ignored; `tests/grpc_build.rs`: 1 passed.
- `NETFLOW_INGEST_BENCH_ROUNDS=1000 NETFLOW_INGEST_BENCH_WARMUP_ROUNDS=100 cargo test -p netflow-plugin --manifest-path src/crates/Cargo.toml bench_ingestion_protocol_matrix -- --ignored --nocapture`
  - Passed.
  - Directional benchmark results:
    - netflow-v5 decode only: 1,442,688 flows/s.
    - netflow-v9 decode only: 425,352 flows/s.
    - ipfix decode only: 499,498 flows/s.
    - sflow decode only: 947,425 flows/s.
- `cargo fmt --manifest-path src/crates/Cargo.toml -p netflow-plugin -- src/crates/netflow-plugin/src/decoder/tests.rs`
  - Passed after adding the IPFIX end-to-end scoped test.
- `cargo fmt --manifest-path src/crates/Cargo.toml -p netflow-plugin -- --check`
  - Passed before reverting unrelated formatter-only churn outside this SOW.
- `git diff --check`
  - Passed.
- Note: test runs still report the pre-existing `OpenTierRow` dead-code warning in `src/crates/netflow-plugin/src/tiering/model.rs:78`; this SOW does not change that warning.

Real-use evidence:

- Existing production-shaped pcap fixtures and synthetic packet tests cover v5, v9, IPFIX, sFlow, datalink, restart hydration, and source-port churn.
- No production packet data or sensitive data was used.

Reviewer findings:

- First review round:
  - `glm`: production-ready; recommended an end-to-end scoped datalink negative test and noted benchmark numbers are conservative because the decode-only benchmark does not measure the production prepare-to-decode context handoff.
  - `minimax`: no correctness/security regression; recommended the same scoped datalink negative test, SOW completion cleanup, and future wrapper cleanup.
  - `kimi`: production-ready; recommended tracking benchmark/context-handoff measurement and wrapper cleanup as follow-up, no blocking correctness issue.
  - `mimo`: production-ready with minor improvements; recommended the scoped datalink negative test and a defensive invariant assertion.
  - `deepseek` and `qwen`: originally launched, but final outputs were not recoverable after context compaction; these reviewers must be rerun in the second review round.
- Action taken from first review round:
  - Added `v9_datalink_decode_is_scoped_by_exporter_and_source_id`.
  - Added `debug_assert!(packet_context.is_some())` when template state changes.
  - Added a short `DecoderPacketContext` derived-scope comment.
- Second review round before IPFIX test addition:
  - `glm`: production-ready, no blockers; findings were low/info items around assertion redundancy, naming, compatibility wrappers, and optional IPFIX end-to-end coverage.
  - `mimo`: production-ready, no issues; noted the decode-only benchmark and wrapper cleanup as follow-ups.
  - `kimi`: production-grade; flagged non-blocking maintainability/test-symmetry items.
  - `minimax`: production-ready but recommended adding the symmetric IPFIX end-to-end scoped datalink test.
- Action taken from second review round:
  - Added `ipfix_datalink_decode_is_scoped_by_exporter_and_observation_domain`.
  - Reran focused datalink validation and full crate validation.
- Final external review round after IPFIX test addition:
  - `glm`: production-ready; no correctness, security, or regression blockers; low cleanup notes only.
  - `minimax`: production-ready; no blocking correctness, security, or performance regression; low cleanup notes around test-only wrappers, assertion comment, and formatter churn.
  - `kimi`: production-ready; no issues; noted benchmark/context handoff and wrapper cleanup as non-blocking follow-ups.
  - `mimo`: production-grade; no blockers; low/info observations only.
  - `deepseek`: production-grade; no blocking correctness or security issues; code-clarity notes only.
  - `qwen`: production-ready; no issues found; pre-existing `OpenTierRow` warning remains unrelated.
- Items intentionally not changed in this SOW:
  - Compatibility wrappers remain because tests/benchmarks still use them and removing them is cleanup, not required for the runtime overhead fix.
  - The decode-only benchmark remains decode-only; the SOW records its numbers as directional/conservative, not as a complete before/after production-path measurement.

Same-failure scan:

- `rg -n "decoder_state_namespace_key|normalize_template_scope_source|has_any_.*datalink|decode_udp_payload_at|decode_netflow\\(" src/crates/netflow-plugin/src`
  - Checked affected call sites.
- `git diff --stat`
  - Confirmed diff scope is limited to decoder/ingest packet-path files after reverting unrelated formatter churn.

Sensitive data gate:

- No sensitive data recorded.

## Artifact Maintenance Gate

- AGENTS.md: no update expected.
- Runtime project skills: no update expected.
- Specs: no update needed; no persisted or public contract changed.
- End-user/operator docs: no update needed; no user-visible config or behavior changed.
- End-user/operator skills: no update expected.
- SOW lifecycle: active child SOW must not merge to `master`.

Specs update:

- None needed.

Project skills update:

- None needed.

End-user/operator docs update:

- None needed.

End-user/operator skills update:

- None needed.

Lessons:

- Flow decoder hot-path benchmarks that bypass ingest preparation can be conservative when the optimization is a prepare-to-decode handoff.
- Scoped predicate tests are not enough for hot-path gate changes; add at least one end-to-end negative test for the visible decode behavior.
- The old global `has_any_*_datalink_templates()` helpers were broader than production now needs and are retained only for tests.

Follow-up mapping:

- Parent inventory SOW tracks ordering.

## Outcome

Implemented, locally validated, and externally reviewed. Final local step is committing and pushing the 12 intended Rust files; this active SOW remains untracked/local and must not be staged.

## Lessons Extracted

- Keep packet-scope derivation centralized for v9/IPFIX so hydration, observation, parser scoping, and scoped special decode gates cannot drift.
- Treat source-port normalization as part of the packet context so source-port churn does not create extra parser sources or hydrated sources.
- For scoped hot-path gate changes, pair predicate unit tests with end-to-end mismatch tests.

## Follow-up Issues

None yet.
