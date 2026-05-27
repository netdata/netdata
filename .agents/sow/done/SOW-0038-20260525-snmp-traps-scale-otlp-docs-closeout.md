# SOW-0038 - SNMP Trap Plugin: Throughput Benchmark + Dynamic engineID (opt-in) + OTLP Exporter

## Status

Status: completed

Sub-state: regression repaired on 2026-05-27 after manual `go.d.plugin` smoke testing found normal job creation rejected framework metadata keys before binding the listener. Previous closeout remains below for history; the regression section records the repair and validation.

## Requirements

### Purpose

Final feature-branch SOW before the SOW-0039 merge gate. Scope:

- Throughput benchmark harness validating spec §9 + §18 claims at scale.
- SNMPv3 dynamic engineID discovery (opt-in, defaults off) for operators with large fleets of v3 devices where static enumeration is impractical.
- Standards-compliant OTLP/gRPC Logs exporter (optional, vendor-neutral) per spec §11b — works with any OTLP-compatible receiver including Netdata's own OTEL plugin without any modification to the OTEL plugin.

Multi-writer partitioning within a single listener is OUT of scope per spec §14: operators scale by adding more listener jobs (each binding a different port or with different community/USM allowlists). The end-user AI skill `query-snmp-traps`, user documentation for the offline MIB conversion workflow, SOW-0032 comparative-analysis.md closeout, and the final merge gate are OWNED BY SOW-0039.

### User Request

Sequential after SOW-0037. Per user: OTLP export must be vendor-neutral ("works on any OTEL-compatible ingester. We should not touch the OTEL plugin for this. We should drive it, not change its code. OTEL export should be optional, if users want it.").

### Assistant Understanding

Facts:

- Spec §11b defines the OTLP attribute universe with standard OTEL semantic conventions (`network.peer.address`, top-level `EventName`, `service.name`) plus Netdata custom namespaces (`snmp.*`, `netdata.*`, `trap.*`).
- OTLP `severity_number` mapping follows OTel Logs Data Model Appendix B (verified: emerg=21, alert=19, crit=18, err=17, warning=13, notice=10, info=9, debug=5).
- Spec §11b documents both `network.peer.address` and `snmp.source.ip` are always emitted (consistent with §11 journal path).
- Netdata's OTEL plugin (`src/crates/netdata-otel/otel-plugin/src/logs_service.rs`) accepts OTLP/gRPC logs and flattens them to systemd-journal entries. The exporter must work against it AND against arbitrary OTLP receivers (e.g., `opentelemetry-collector-contrib`).
- Per spec §5, SNMPv3 dynamic sender engineID discovery for v3 Trap PDUs is opt-in due to spoofing surface and operator-visibility concerns. v3 INFORM is separate: the receiver is authoritative, so SOW-0038 must also add the optional RFC 3414 Report responder that lets senders discover Netdata's receiver-local engine ID generated/configured by SOW-0036.
- Spec §17 confirms multi-writer partitioning is out of scope (operators scale by adding jobs).

Inferences:

- OTLP transport choice (gRPC vs HTTP/protobuf): gRPC is standard for OTEL collectors and matches the Netdata OTEL plugin's `LogsService` gRPC endpoint. Settled at M3 design time.

Unknowns:

- Throughput target validation against spec §9's stated 30k rows/sec/writer claim (confirmed in M1).

### Acceptance Criteria

- M1: synthetic trap generator validates spec §9 throughput targets (>30k rows/sec per per-job writer) and spec §18 BER decode budget; reports CPU/memory per trap.
- M2: SNMPv3 discovery available as opt-in behavior where appropriate: v3 Trap sender engineID hot-registration available behind a default-off flag; dynamic registrations are in-memory, capped by `dynamic_engine_id_max_pairs`, warn/increment `unknown_engine_id` once per first accepted `(engineID, username)` pair, and are mutually exclusive with `engine_id_whitelist`; v3 INFORM receiver-local engine ID Report response emits `usmStatsUnknownEngineIDs` so senders can discover the job's local engine ID. Operator warnings surfaced in schema/stock config and at runtime when an unknown sender engineID is hot-registered. Full end-user docs remain SOW-0039.
- M3: OTLP gRPC exporter ships as opt-in second backend, configured via per-job `otlp:` config block; runs against any OTLP-compliant receiver; verified against Netdata's OTEL plugin (existing flatten path) AND at least one external OTLP receiver (e.g., `opentelemetry-collector-contrib` binary in test fixture); severity mapping per OTel Logs Data Model Appendix B verified.

## Milestones

### M1 — Throughput benchmark harness

- Synthetic trap generator (configurable rate, varbind diversity, source distribution).
- Per-job stress test: drive a single listener at >30k traps/sec and measure writer thread CPU + memory.
- Multi-job stress test: 10 listeners each at 10k traps/sec; verify per-job partitioning holds (no cross-job contention).
- BER decode budget validation per spec §18: synthetic malformed PDUs (oversized, deep nesting, many varbinds) verify limit enforcement at the documented thresholds.
- Reports: throughput/listener, CPU/trap, memory/trap, decode-budget violations.

Cohort reference: spec §9; spec §18.

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M2 — SNMPv3 discovery (Trap sender engine IDs + INFORM receiver engine ID)

- v3 Trap sender engine IDs: subclass the SNMP transport per `splunk-sc4snmp.md` §3.5 / `traps.py:229-258` pattern: peek at raw bytes pre-parse, ASN.1-decode the SNMPv3 header, extract `engineID + username`, hot-register the pair, retry parse.
- v3 Trap sender engine IDs: explicit per-job opt-in flag (defaults off per spec §5).
- v3 Trap sender engine IDs: on first trap from a previously unknown `(engineID, username)` pair, log WARNING with engineID + username + spoofing advisory + `snmp.trap.errors.unknown_engine_id` increment, then accept the trap.
- v3 Trap sender engine IDs: dynamic registrations are in-memory per job, capped by `dynamic_engine_id_max_pairs` (0/unset means default 4096), and not persisted across Agent restart or job reload.
- v3 Trap sender engine IDs: `dynamic_engine_id_discovery` and `engine_id_whitelist` are mutually exclusive at job creation. Static whitelist mode and dynamic discovery mode are separate operator contracts to avoid implying that the whitelist still restricts senders after dynamic discovery is enabled.
- v3 INFORM receiver-local engine ID: implement the RFC 3414 discovery Report path for reportable empty/malformed authoritative engine ID probes, using the SOW-0036 persisted `local_engine_id` and `snmpEngineBoots` state. This responder is automatic for v3 jobs after source allowlist and is required for operators who do not want to manually configure Netdata's generated receiver engine ID into INFORM senders. Valid non-local engine IDs are rejected, not answered as discovery probes.
- Spec §5 lists v3 Trap sender discovery as opt-in due to spoofing surface; the SOW must surface that clearly. The v3 INFORM Report responder is not a sender hot-registration path and must not weaken the Trap sender allowlist.

Cohort reference: `splunk-sc4snmp.md` §3.5; `opennms/opennms` INFORM discovery test pattern; spec §5 v3 USM section.

Reviewers: GLM, Kimi, MiniMax, and Qwen — security-sensitive. Mimo is excluded by user instruction because it is out of quota.

### M3 — OTLP gRPC exporter (optional, vendor-neutral)

- Standards-compliant OTLP/gRPC Logs export — NO modifications to Netdata's OTEL plugin.
- Field naming per spec §11b (OTEL semconv where official: `network.peer.address`, top-level `EventName`, `service.name`; custom Netdata namespaces: `snmp.*`, `netdata.*`, `trap.*`).
- Severity mapping per OTel Logs Data Model Appendix B (per spec §11b corrected table): emerg=21, alert=19, crit=18, err=17, warning=13, notice=10, info=9, debug=5. Verified in test against published OTel reference.
- `body` carries the rendered MESSAGE; the top-level LogRecord `EventName` field (OTLP proto `event_name`) is set by reading `TrapEntry.Category` and producing `"snmp.trap.<slug>"`; both `network.peer.address` and `snmp.source.ip` always emitted (consistent with §11 journal path).
- Operator opt-in per-job: `otlp.enabled: true` + `otlp.endpoint: <gRPC endpoint URL>` in job config (default off).
- Verification against:
  - Netdata's OTEL plugin (gRPC endpoint at `localhost:4317` — verify entries appear in the OTEL plugin's flattened journal output).
  - External OTLP receiver: `opentelemetry-collector-contrib` binary in CI fixture (verify attribute names, severity_number, body, top-level `EventName`).
- `TrapWriter` interface (per spec §19) implementation: OTLP writer batches internally (default 200 ms flush window) and returns from `Write` as soon as the entry is enqueued.

Cohort reference: OpenTelemetry Logs Data Model spec + Appendix B; OTLP/gRPC proto definitions; `src/crates/netdata-otel/otel-plugin/src/logs_service.rs` (existing receiver behavior).

Reviewers: GLM, Kimi, MiniMax, and Qwen — interop-critical (must work with arbitrary OTLP receivers without modification). Mimo is excluded by user instruction because it is out of quota.

## Reviewer Protocol

- M2 + M3: GLM, Kimi, MiniMax, and Qwen reviewers (security + interop critical). Mimo is excluded by user instruction because it is out of quota.
- M1: 3 rotating reviewers per round (group A: kimi/qwen/minimax).
- Fix-cycle: same reviewers as the round being fixed.

## Pre-Implementation Gate

Status: M1 gate completed. M2 gate added on 2026-05-27 and completed after implementation plus reviewer validation. M3 gate added on 2026-05-27 and ready for implementation.

### Problem / Root-Cause Model

SOW-0035 through SOW-0037 delivered the functional SNMP trap listener, journal writer, decode limits, enrichment, dedup, profile reload, and operator metrics. The remaining scale risk is evidence: the design/spec claims high sustained trap throughput and bounded decode cost, but the current benchmark surface only covers isolated writer operations. There is no single harness that measures the whole packet path, the SDK-backed writer drain path, multi-job partitioning, and malformed BER budget behavior in one repeatable report.

M1 must therefore build a benchmark harness, not change product behavior. If the harness proves throughput below the spec target, that is evidence to feed SDK/runtime optimization and SOW-0039 merge-gate decisions; it must not be hidden by weakening the benchmark.

### Evidence Reviewed

- `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go` already has narrow benchmarks for `TrapWriter.Write`, `JournalTrapWriterDrain`, and `JournalWriter.WriteEntry`, but no packet-path, multi-job, source-distribution, or decode-budget report.
- `src/go/plugin/go.d/collector/snmp_traps/decode.go` enforces the BER guardrails M1 must measure: `maxDatagramSize=8192`, `maxVarbinds=256`, `maxNestingDepth=8`, `maxOIDEncodedLen=128`, `maxOctetStringLen=1024`, and `decodeBudgetTarget=1ms`.
- `src/go/plugin/go.d/collector/snmp_traps/decode_test.go` verifies individual BER limit failures and budget error classification, but does not benchmark their cost at scale.
- `.agents/sow/specs/snmp-traps/netdata.md` documents lazy profile loading, per-job listeners, retention-backed journal files, and forensic journal completeness; the benchmark must exercise these surfaces without changing them.
- `.agents/sow/specs/snmp-traps/comparison/netdata-stress-test.md` explicitly called the 30k rows/sec claim unsupported and requested measured evidence before merge.

### Affected Contracts And Surfaces

- Test/benchmark-only Go files under `src/go/plugin/go.d/collector/snmp_traps/`.
- No runtime config, schema, stock configuration, public docs, health alerts, or production listener behavior should change in M1.
- The benchmark may use synthetic packets, synthetic trap entries, and temporary journal directories only.
- Benchmark names and optional environment variables become developer workflow contracts for SOW-0039 closeout evidence.

### Existing Patterns To Reuse

- Existing Go benchmark style in `benchmark_test.go`.
- Existing synthetic packet builders in `decode_test.go` and pcap fixture readers in `pcap_test.go`.
- Existing `mockTrapWriter`, `newJournalTrapWriter`, `NewJournalWriter`, `Collector.handlePacket`, and `DecodeTrap` paths instead of introducing parallel fake pipelines.
- Go standard `testing.B`, `b.ReportAllocs`, `b.ReportMetric`, and `runtime.MemStats` for reproducible benchmark output.

### Risk And Blast Radius

- Benchmarks that use real SDK-backed journal files can be slow and hardware-sensitive; they must be normal Go benchmarks and not regular unit tests.
- Benchmarks must not bind privileged UDP/162 or rely on live network packets; synthetic packet injection avoids workstation/process side effects.
- A hard pass/fail throughput threshold would make CI unstable across hardware. M1 records throughput using `ReportMetric`; release acceptance happens in SOW-0039.
- Multi-job benchmarks must avoid generic process termination or external services.
- Decode-budget benchmarks must avoid pathological data that stalls the test runner.

Sensitive data handling plan:

No raw customer traps, SNMP communities, USM keys, real hostnames, customer IPs, or live journal rows are used. Synthetic packets use test communities, synthetic trap OIDs already present in fixtures, and RFC 5737 / private-range addresses. Benchmark reports in SOWs record aggregate rates/allocations only, not payload values from real environments.

### Implementation Plan

1. Extend the benchmark harness for M1 only:
   - packet-path benchmark using synthetic v2c traps and `Collector.handlePacket` with an in-memory sink writer;
   - decode-only benchmark over generated packets with configurable varbind diversity;
   - SDK-backed writer drain benchmark with explicit rows/sec and bytes/op reporting;
   - multi-job partition benchmark using independent collectors/writers and synthetic source distribution;
   - malformed BER budget benchmark using bounded synthetic malformed PDUs.
2. Keep all M1 code in test/benchmark files unless a small test-only helper extraction is necessary.
3. Make workload knobs deterministic and documented in the SOW execution log if environment variables are added.
4. Run benchmark commands locally and record measured evidence without overstating hardware portability.

### Validation Plan

- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...`
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*BER|Benchmark.*Multi' -benchmem ./plugin/go.d/collector/snmp_traps`
- SDK-backed benchmark on Linux with temporary journal directories; record rows/sec and allocations.
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...`
- `go vet ./plugin/go.d/collector/snmp_traps/...`
- `git diff --check`
- `.agents/sow/audit.sh`

### Artifact Impact Plan

- `AGENTS.md`: no workflow change expected.
- Runtime project skills: no update expected unless benchmark work exposes a reusable collector-performance rule.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` or comparison docs only if measured evidence changes a stated throughput claim.
- End-user/operator docs and public skills: not part of M1; SOW-0039 owns docs and query skill.
- SOW lifecycle: SOW-0038 is current/in-progress for M1; SOW-0037 is paused implementation-complete; SOW-0039 remains pending merge gate.

### Open Decisions

No user decision is required for M1. If measured SDK-backed journal throughput remains materially below the 30k rows/sec target, SOW-0039 must decide whether to block merge for SDK optimization or release with documented lower throughput.

### M2 Pre-Implementation Gate - 2026-05-27

Status: ready for M2 implementation.

#### Problem / Root-Cause Model

SNMPv3 Trap sender engine IDs and v3 INFORM receiver engine IDs are different authoritative-engine cases:

- v3 Trap PDUs are unconfirmed notifications where the sender is authoritative. Static sender engine ID enumeration is secure but operationally heavy for large fleets. Dynamic sender engine ID discovery must therefore be opt-in, must extract `engineID + username` from raw SNMPv3 USM security parameters before normal decode, and must only accept a dynamically registered sender after the decoded packet proves it is a Trap PDU.
- v3 INFORM PDUs are confirmed notifications where the receiver is authoritative. Netdata already persists a per-job receiver-local `local_engine_id` and `snmpEngineBoots`; M2 must add the Report response used by senders to discover that receiver engine ID. This Report path must not become a sender hot-registration path.

#### Evidence Reviewed

- Local code: `src/go/plugin/go.d/collector/snmp_traps/collector.go:104` validates all USM users with the current static-engine rules; `collector.go:138` currently requires `engine_id_whitelist` for every v3 job.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/init.go:131` currently requires every `usm_users[].engine_id`; this conflicts with dynamic discovery because operators should be able to configure a USM credential template without enumerating every device engine ID.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/init.go:254` currently rejects `dynamic_engine_id_discovery` at job creation.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/decode.go:188` can extract only the v3 authoritative engine ID, not the username or reportability needed by M2.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/collector.go:373` decodes v3 packets once; decode errors are counted and dropped without dynamic retry or discovery Report handling.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/collector.go:398` already separates v3 INFORM local-engine validation from v3 Trap sender-engine whitelist validation. M2 must preserve this split.
- Local GoSNMP fork: `github.com/ilyam8/gosnmp @ v0.0.0-20250912202722-388b2cb5192e`, `v3_map.go:26` locks `SnmpV3SecurityParametersTable.Add()` and stores multiple security parameter sets per username; `trap.go:345` builds an authoritative-engine-ID Report; `trap_test.go:1460` verifies discovery with an empty authoritative engine ID.
- Local GoSNMP fork: `marshal.go:101` exports the `Report` PDU type and `marshal.go:517` exposes `(*SnmpPacket).MarshalMsg()`. `marshal.go:112` defines the unexported `usmStatsUnknownEngineIDs` string as `.1.3.6.1.6.3.15.1.1.4.0`, and `marshal_test.go:744` includes a Report packet with that varbind. M2 will construct a `SnmpPacket` Report directly instead of calling the fork's unexported trap-listener helper.
- Open-source reference: `splunk/splunk-connect-for-snmp @ fdd4c74ef3cc8295675039be9f432b00e48b96d8`, `splunk_connect_for_snmp/traps.py:120` decodes SNMPv3 security context from raw bytes; `traps.py:229` subclasses UDP transport to extract engine ID + username before PySNMP parses; `traps.py:201` hot-registers newly observed engine IDs.
- Open-source reference: `pysnmp/pysnmp @ 4891556e7db831a5a9b27d4bad8ff102609b2a2c`, `pysnmp/proto/secmod/rfc3414/service.py:705` handles null/malformed authoritative engine ID as discovery and returns `usmStatsUnknownEngineIDs`; `pysnmp/proto/mpmod/rfc3412.py:61` maps `1.3.6.1.6.3.15.1.1.4.0` to `unknownEngineID`; `pysnmp/proto/mpmod/rfc3412.py:305` builds the Report PDU varbind.
- Standards reference: RFC 3414 section 4 describes authoritative engine ID discovery with zero-length `msgUserName` and `msgAuthoritativeEngineID`; RFC 3412 section 6.4 defines the reportable flag behavior for confirmed and unconfirmed classes.

#### Affected Contracts And Surfaces

- Runtime behavior in `src/go/plugin/go.d/collector/snmp_traps/collector.go`, `decode.go`, `inform.go`, and initialization validation.
- Configuration behavior for `dynamic_engine_id_discovery`, `usm_users[].engine_id`, and `engine_id_whitelist`.
- New configuration behavior for `dynamic_engine_id_max_pairs`.
- Job-creation validation must reject `dynamic_engine_id_discovery: true` together with non-empty `engine_id_whitelist`.
- Operator-facing config surfaces: `src/go/plugin/go.d/collector/snmp_traps/config_schema.json` and `src/go/plugin/go.d/config/go.d/snmp_traps.conf`.
- Tests under `src/go/plugin/go.d/collector/snmp_traps/`.
- No profile format, journal schema, topology, OTLP, or SDK integration changes belong in M2.

#### Existing Patterns To Reuse

- Keep job-creation validation strict and return `dyncfgCodedError` with code 422 for invalid v3 combinations.
- Reuse the existing BER helpers in `decode.go` for raw SNMPv3 security context extraction instead of adding a new ASN.1 dependency.
- Reuse GoSNMP's `SnmpV3SecurityParametersTable` and `UsmSecurityParameters` rather than copying or vendoring SNMP security logic.
- Reuse persisted `LocalEngineID` and `EngineBoots` from SOW-0036 for INFORM discovery Report responses.
- Reuse existing per-job error metric dimensions: `unknown_engine_id`, `usm_failures`, `auth_failures`, and `inform_response_failed`.

#### Risk And Blast Radius

- Security risk: dynamic Trap discovery expands acceptance from static engine IDs to "any engine ID using a configured username and valid USM keys" for that job. This must remain default-off, source allowlist must still run before decode, the dynamic registry must be capped, and the first accepted dynamic pair must produce a warning and an `unknown_engine_id` increment.
- Operator-contract risk: allowing `engine_id_whitelist` and dynamic discovery together would make the whitelist look like a continuing restriction even though dynamic discovery intentionally accepts authenticated sender engine IDs beyond it. M2 rejects that combination at job creation; operators who need different policies use separate jobs.
- DoS risk: dynamic retry adds BER extraction and a second decode/auth attempt on the decode-failure path. The retry path must skip reportable messages, honor existing per-source rate-limit drop decisions before retrying, and avoid adding failed candidates to shared state.
- INFORM risk: using dynamic sender registration for INFORM would weaken the receiver-local engine ID model. M2 must reject decoded INFORMs whose authoritative engine ID is not Netdata's local engine ID, and dynamic registration must not persist a sender engine ID until the retried packet decodes as a Trap.
- Operational risk: if `usm_users[].engine_id` remains required, dynamic discovery is not fit for purpose. M2 must make `engine_id` optional only when `dynamic_engine_id_discovery` is enabled, while preserving strict validation for static v3 jobs.
- Concurrency risk: multiple listener goroutines can observe the same new `(engineID, username)` pair. Dynamic registration and first-warning state need explicit synchronization even though GoSNMP's table `Add()` is internally locked.
- Compatibility risk: config/schema text currently says dynamic discovery is reserved and that static engine IDs are required. M2 must update these surfaces in the same batch.

Sensitive data handling plan:

Runtime warnings may include engine ID and username because the SOW explicitly requires them for operator visibility. They must never include USM auth or privacy keys, communities, packet payload bytes, customer identifiers, or raw trap values. The RFC 3414 Report responder intentionally exposes the receiver-local engine ID and current boots/time to allowed discovery probes; this is protocol behavior, not secret material. SOW evidence records only synthetic test values, sanitized paths, upstream repository references, and aggregate validation results.

#### Implementation Plan

1. Replace the deferred rejection for `dynamic_engine_id_discovery` with strict v3 validation:
   - v3 still requires at least one USM user;
   - static v3 jobs still require `engine_id_whitelist` and each `usm_users[].engine_id`;
   - dynamic v3 jobs must leave `engine_id_whitelist` empty and may use USM users without `engine_id`;
   - if a dynamic-mode `usm_users[].engine_id` is supplied, keep validating it and treat it as a preconfigured known `(engineID, username)` pair for that user.
   - `dynamic_engine_id_max_pairs` is validated at job creation; negative values fail, 0/unset uses default 4096.
   - `validateUSMUsers`, `buildSnmpV3SecurityTable`, and `config_schema.json` must all implement the same conditional `engine_id` contract.
2. Before writing the Report responder, add/verify a minimal unit path that proves the pinned GoSNMP fork can marshal a v3 `Report` `SnmpPacket` with the required `usmStatsUnknownEngineIDs` varbind, local engine ID, and boots/time. Stop and present alternatives if this fails.
3. Add raw SNMPv3 context extraction for authoritative engine ID, username, and message flags/reportability using bounded BER parsing.
4. Add an opt-in dynamic Trap decode path:
   - on unknown-engine decode failure, extract raw `(engineID, username)`;
   - if the message is reportable, skip the Trap dynamic retry path;
   - only use configured users with that username; missing username or no configured username does not create dynamic state and remains a USM/decode failure;
   - honor existing per-source rate-limit drop decisions before attempting the dynamic retry;
   - retry decode with a per-packet temporary security table containing the candidate engine ID and matching configured credentials;
   - discard the temporary table on retry failure and do not add failed candidates to shared state;
   - persist the dynamic registration into the shared table only after the retry authenticates and decodes a v3 Trap, not an INFORM;
   - enforce `dynamic_engine_id_max_pairs` before persisting a new pair;
   - when the cap is full, reject new pairs and increment `unknown_engine_id`;
   - warn and increment `unknown_engine_id` once per new `(engineID, username)` pair for the job lifetime, then accept the Trap. Subsequent traps from the same pair do not repeat that dynamic-registration warning or increment.
5. Add an INFORM discovery Report path for reportable empty/malformed receiver authoritative engine ID probes using the persisted local engine ID and engine boots/time state.
   - the Report PDU must include `usmStatsUnknownEngineIDs` (`1.3.6.1.6.3.15.1.1.4.0`) and the job's local engine ID plus current boots/time.
   - valid non-local engine IDs are rejected by the existing INFORM local-engine validation and are not answered as discovery probes.
6. Update config schema and stock config examples for the new operator contract.
7. Add focused tests for validation, raw context parsing, dynamic Trap acceptance, static default rejection, static whitelist + dynamic rejection, no dynamic INFORM acceptance, discovery Report wire format, duplicate concurrent registration, cap exhaustion, reportable prefilter, rate-limit interaction, retry failure cleanup, and metric/warning behavior where testable.

#### Validation Plan

- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...`
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...`
- `go vet ./plugin/go.d/collector/snmp_traps/...`
- `go test -run 'Test.*Dynamic|Test.*EngineID|Test.*Inform|TestConfigValidation' -count=1 ./plugin/go.d/collector/snmp_traps`
- Include explicit tests for DynCfg/Init-time 422 failures, static-v3 unchanged behavior when dynamic discovery is disabled, static whitelist + dynamic discovery rejection, `usmStatsUnknownEngineIDs` Report wire format, unknown non-empty INFORM engine rejection, no dynamic state after failed retry, duplicate-pair warning deduplication, and dynamic pair cap exhaustion.
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps`
- `git diff --check`
- `.agents/sow/audit.sh`
- Full-scope external review by GLM, Kimi, MiniMax, and Qwen after the M2 batch is implemented.

#### Artifact Impact Plan

- `AGENTS.md`: no workflow change expected.
- Runtime project skills: no update expected unless M2 exposes a reusable SNMPv3 trap authoring/testing rule.
- Specs: `.agents/sow/specs/snmp-traps/netdata.md` updated before implementation with the M2 dynamic-registration cap, in-memory lifecycle, reportable guard, and INFORM Report responder contract.
- End-user/operator docs and public skills: SOW-0039 owns full docs and AI skill; M2 updates stock config and schema only unless the implementation changes published behavior already present in docs.
- SOW lifecycle: keep SOW-0038 current/in-progress for M2; do not start M3 until M2 is validated.

#### Open Decisions

No user decision is required for M2. The user already selected opt-in dynamic Trap discovery, default-off reverse DNS, Go implementation, job-creation-time failure detection, and separate INFORM receiver-local engine ID discovery. The implementation choices recorded here are: the INFORM Report responder is automatic for allowed v3 discovery probes and has no extra config knob; dynamic Trap registrations are in-memory only; default dynamic cap is 4096 `(engineID, username)` pairs per job; `engine_id_whitelist` and dynamic discovery are mutually exclusive at job creation. If tests show GoSNMP cannot marshal the required discovery Report without invasive changes, stop and present implementation alternatives before changing the dependency.

### M3 Pre-Implementation Gate - 2026-05-27

Status: ready for M3 implementation.

#### Problem / Root-Cause Model

The journal-direct writer is Netdata's authoritative/high-fidelity trap store. M3 adds OTLP as an optional second backend, not as a replacement. The core risk is giving operators a false success signal: if `otlp.enabled: true` but the receiver endpoint is invalid, unreachable, or not an OTLP logs receiver, the job must fail at DynCfg apply time. After a job has successfully started, later OTLP receiver outages must not break journal capture; they must be bounded by the OTLP writer queue and counted as OTLP export loss, not as journal write loss.

#### Evidence Reviewed

- Local code: `src/go/plugin/go.d/collector/snmp_traps/collector.go:89` starts all job resources inside `Init()` and returns `dyncfgCodedError` code 422 for creation-time failures.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/collector.go:253` currently creates only the journal writer, and `collector.go:508` treats any `TrapWriter.Write()` error as `journal_write_failed`.
- Local code: `src/go/plugin/go.d/collector/snmp_traps/metrics.go:12` currently has no OTLP-specific failure counter; reusing `journal_write_failed` for OTLP would be inaccurate once there are two backends.
- Local spec: `.agents/sow/specs/snmp-traps/netdata.md:831` defines OTLP as an optional vendor-neutral LogRecord path while the journal-direct path remains high-fidelity.
- Local spec: `.agents/sow/specs/snmp-traps/netdata.md:837` through `:908` defines the OTLP LogRecord field and attribute universe, severity mapping, dedup summary shape, and protobuf injection-safety model.
- Local spec: `.agents/sow/specs/snmp-traps/netdata.md:1101` through `:1117` defines the `TrapWriter` fast-enqueue interface and backend-internal batching contract.
- Netdata OTEL receiver: `src/crates/netdata-otel/otel-plugin/src/logs_service.rs:128` implements `LogsService.Export`; `src/crates/netdata-otel/flatten_otel/src/logs.rs:12` flattens resource, scope, LogRecord fields, top-level `event_name`, and log attributes for journal storage.
- Existing go.d secret resolver: `src/go/plugin/agent/secrets/resolver/resolver.go` walks config maps and resolves secret references in string values before the collector receives its configuration. OTLP headers can therefore be ordinary string config values without the collector learning secret-store internals.
- Open-source reference: `open-telemetry/opentelemetry-go @ f593185679130f56e14bed3c337fa7f8f60756b1`, `exporters/otlp/otlplog/otlploggrpc/client.go:13` uses the OTLP logs proto client, `client.go:61` applies metadata headers, `client.go:96` selects TLS or insecure credentials, and `client.go:142` exports `ResourceLogs`.
- Open-source reference: `open-telemetry/opentelemetry-collector @ a17ec5a8fbe424d9a5db4f6036f9fda2b96992f7`, `pdata/plog/plogotlp/grpc.go:18` exposes the OTLP logs gRPC client/server contract and `grpc.go:24` recommends keeping the RPC/client alive for performance.
- Open-source reference: `observiq/blitz @ d37f48527ccd13a75bf28cc314c0e95361ac3de6`, `output/otlp_grpc/otlp_grpc.go:31` defines gRPC queue/batch/request-timeout defaults, `otlp_grpc.go:180` defaults to localhost:4317 with insecure transport, and `otlp_grpc.go:252` starts worker-based batching.

#### Affected Contracts And Surfaces

- Runtime code in `src/go/plugin/go.d/collector/snmp_traps/`: config, validation, Init-time preflight, writer fan-out, OTLP serialization, metrics, and tests.
- Operator config surfaces: `config_schema.json` and stock `snmp_traps.conf`.
- Spec surfaces: `.agents/sow/specs/snmp-traps/netdata.md` updates for the concrete `otlp:` block and `otlp_export_failed` error dimension.
- No changes to `src/crates/netdata-otel/otel-plugin/` are allowed in M3. The trap plugin must drive the existing OTLP logs receiver.
- No profile YAML format changes belong in M3.

#### Existing Patterns To Reuse

- Keep all detectable startup failures in `Collector.Init()` and wrap them as HTTP-422 DynCfg errors, matching endpoint bind, profile load, journal directory, and retention validation behavior.
- Reuse the existing `TrapWriter` interface; add a fan-out writer instead of putting OTLP-specific logic in the packet decode path.
- Reuse the current `TrapEntry` semantic model and backend-local serialization split: journal names stay uppercase/systemd-shaped; OTLP names stay dotted-lowercase per spec.
- Reuse go.d config-schema patterns for nested objects, durations as strings where needed, and `sensitive: true`/password widgets only where values are direct credentials. OTLP header values are ordinary strings because the existing secret resolver resolves secret references before collector init.
- Reuse the proto definitions already present in the Go module: `go.opentelemetry.io/proto/otlp` and `google.golang.org/grpc`; do not add a dependency on the full OTel logs SDK unless direct proto export proves insufficient.

#### Risk And Blast Radius

- Startup-truthfulness risk: if OTLP is enabled but endpoint validation is lazy, DynCfg can show a running job that immediately fails in logs. M3 must connect and perform an OTLP Logs `Export` preflight before starting the listener.
- Journal-loss risk: if fan-out returns the secondary OTLP error to the packet path, the dedup rollback path can make a successfully journaled trap look dropped. M3 makes journal write the primary path; an OTLP enqueue/export failure increments `otlp_export_failed` and does not undo the journal copy.
- Backpressure risk: an unavailable OTLP receiver can fill the OTLP queue. The writer must keep `Write()` non-blocking, drop only the OTLP copy when the bounded queue is full, and continue journal writes.
- Interop risk: OTLP endpoint syntax differs across ecosystems. M3 accepts `http://host:port` for plaintext gRPC, `https://host:port` for TLS with system roots, and bare `host:port` as plaintext for local receivers such as Netdata's OTEL plugin and the OpenTelemetry Collector default.
- Secret-handling risk: OTLP metadata headers may contain bearer tokens or API keys. Header values must not be logged, copied into SOW output, emitted as trap attributes, or included in tests except synthetic placeholders.
- Cardinality/shape risk: labels become `trap.<key>` attributes. Existing label cardinality validation remains the guard; M3 must not add source IP, usernames, raw packet bytes, or secrets as OTLP resource attributes or labels beyond the spec-defined source fields.
- Test-fixture risk: an external collector binary may not exist on this workstation. If unavailable, M3 must still validate with an in-process OTLP gRPC test server and record the gap; SOW-0039 can wire a packaged external receiver fixture if needed.

Sensitive data handling plan:

No raw traps, SNMP communities, USM passphrases, OTLP bearer tokens, customer identifiers, live endpoint URLs, or header values are written into SOWs, specs, docs, code comments, tests, or reviewer prompts. Tests use synthetic headers such as `authorization: Bearer test-token` only against in-process fixtures and assert values are transmitted without logging them. Runtime errors include endpoint host/port and error class, but never metadata header values or trap payload bytes.

#### Implementation Plan

1. Update the spec and config surfaces for a per-job `otlp:` block:
   - `enabled` default false;
   - `endpoint` default `http://127.0.0.1:4317`;
   - optional `headers` map for OTLP metadata;
   - `request_timeout` default 5s;
   - `flush_interval` default 200ms;
   - `batch_size` default 512;
   - `queue_capacity` default 10000.
2. Add strict OTLP config validation:
   - disabled OTLP does not require a receiver and does not allocate an OTLP writer;
   - enabled OTLP requires a parseable endpoint using `http`, `https`, or bare `host:port`;
   - unsupported schemes, paths, empty hosts, negative/zero durations, negative/zero batch size, negative/zero queue capacity, and invalid metadata header names fail job creation.
3. Add an OTLP writer implementing `TrapWriter`:
   - `Write()` only enqueues into a bounded channel;
   - a worker batches by size or `flush_interval`;
   - export uses the OTLP Logs gRPC proto client and keeps a persistent connection;
   - `Flush()` and `Close()` make best-effort export of accepted entries and return backend errors to the caller.
4. Add creation-time preflight when `otlp.enabled: true`:
   - create the gRPC client with endpoint-derived credentials;
   - wait for connection readiness within `request_timeout`;
   - send an empty OTLP Logs `Export` request with configured headers;
   - fail `Init()` with code 422 on connection, TLS, auth, unimplemented service, or other preflight errors.
5. Add a fan-out writer:
   - journal writer is primary and remains authoritative;
   - OTLP writer is secondary;
   - primary `Write()` errors keep the existing `journal_write_failed` behavior and dedup rollback;
   - secondary `Write()` errors increment `otlp_export_failed` and return nil to the packet path.
6. Serialize `TrapEntry` to OTLP `ResourceLogs`:
   - resource attributes `service.name=netdata-snmptrap` and `service.instance.id=<job_name>`;
   - `body`, `severity_number`, `severity_text`, `event_name`, timestamps, and all attributes defined in spec §11b;
   - omit empty optional topology/device/vendor/vnode fields;
   - encode `snmp.varbinds` as nested OTLP key-value lists;
   - encode dedup summary counts per spec.
7. Add tests for config validation, endpoint parsing, preflight failure/success, headers, severity mapping, normal Trap serialization, dedup summary serialization, fan-out journal/OTLP failure isolation, queue-full behavior, Flush/Close behavior, and in-process OTLP gRPC receiver interop.

#### Validation Plan

- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`
- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...`
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...`
- `go vet ./plugin/go.d/collector/snmp_traps/...`
- `go test -run 'Test.*OTLP|Test.*TrapWriter|TestConfigValidation' -count=1 ./plugin/go.d/collector/snmp_traps`
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps`
- If an external OTLP receiver binary is available, run a real receiver interop check and record exact command/result. If unavailable, record that the in-process gRPC fixture covers the OTLP service contract and leave the packaged receiver fixture to SOW-0039.
- `git diff --check`
- `.agents/sow/audit.sh`
- Full-scope external review by GLM, Kimi, MiniMax, and Qwen after the M3 batch is implemented. Mimo remains excluded by user instruction.

#### Artifact Impact Plan

- `AGENTS.md`: no workflow change expected.
- Runtime project skills: no update expected unless M3 exposes a reusable OTLP/log-ingestion authoring rule.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` before implementation with the concrete OTLP config and error-metric contract.
- End-user/operator docs and public skills: full docs and `query-snmp-traps` remain SOW-0039; M3 updates schema and stock config because operators need correct DynCfg and file examples now.
- SOW lifecycle: keep SOW-0038 current/in-progress for M3; do not mark completed until M3 implementation, validation, reviewer rounds, artifact maintenance, and follow-up mapping are complete.

#### Open Decisions

No further user decision is required for M3. The user already selected optional vendor-neutral OTLP export with no OTEL-plugin changes, Go implementation, and creation-time detection for failures that can be detected before runtime. The implementation choices recorded here are: OTLP is a secondary fan-out backend; `otlp_export_failed` is added instead of overloading `journal_write_failed`; endpoint URL scheme controls plaintext/TLS; bare `host:port` is plaintext for local collector compatibility; optional metadata headers are supported through normal go.d string config and existing secret-reference resolution.

## Plan

Sequential M1 → M3. The end-user AI skill, user documentation, SOW-0032 closeout, and the final merge are OWNED BY SOW-0039 and not in this SOW.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup: M1 multi-writer partitioning removed (per-job is the partition); M5 AI skill and M6 SOW-0032 closeout moved to SOW-0039 (final merge SOW). Not yet activated.

### 2026-05-27 — M1 benchmark harness implementation and validation

**Implementation**: `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go` extended with:

1. **BenchmarkDecodeTrap** — decode-path throughput over generated SNMPv2c traps at varbind counts 2/5/10/20/50/256. Reports ns/op, B/op, allocs/op, and traps/s. Representative local runs on the same i9-12900K workstation varied with system load: Varbinds=2 ranged roughly 450k-524k traps/s, Varbinds=50 roughly 45k-55k traps/s, and Varbinds=256 roughly 10.3k-11.2k traps/s. Allocation scales with varbind count (about 58-59 allocs at 2 varbinds, about 2763-2764 allocs at 256 varbinds).

2. **BenchmarkPacketTrap** — end-to-end packet path through `Collector.handlePacket` with a production-shaped per-job metrics pointer and a counting sink writer (no unbounded storage). No UDP sockets, no live network. Representative local runs reported roughly 176k-210k packets/s and entries/s, about 5.2KB/op, 120 allocs/op, and low non-zero decode-budget drop percentages under scheduler/GC load.

3. **BenchmarkMultiJob** — multi-job scale shape at N=1/4/10 independent collectors, one benchmark goroutine per collector, production-shaped per-job metrics pointers, and synthetic per-job source IP distribution. Representative local runs reported aggregate in-memory packet-path throughput around 340k-380k entries/s for N=1, around 0.95M-1.1M for N=4, and around 1.43M-1.6M for N=10. This is a partition-shape benchmark, not a journal-writer throughput claim.

4. **BenchmarkBERRejection** — structural malformed PDU rejection cost for 7 error classes (Oversized, DepthOverLimit, OIDTooLong, OctetStringTooLong, TrailingData, IndefiniteLength, Truncated). Representative local run: rejection latency ranged roughly 19ns-159ns. This measures the BER rejection cost directly; the 1ms decode-budget timer is in `DecodeTrap` and is represented by packet-path `drop_pct`, not forced by this benchmark.

5. **Existing journal benchmarks enhanced** — `BenchmarkTrapWriterWrite`, `BenchmarkJournalTrapWriterDrain`, `BenchmarkJournalWriterWriteEntry` now also report `entries/s` via `b.ReportMetric`.

**Helpers added**: `buildBenchV2cTrap` (packet generator with marshal error checks), `countingWriter` (atomic-counting in-memory sink), `setBenchProfileIndex` (profile seeding that restores the previous global profile index), `benchMakePDUs` (synthetic varbind factory).

**Throughput finding**:
- In-memory writer queue path: `BenchmarkTrapWriterWrite` reported millions of entries/s in local runs.
- SDK-backed `journalTrapWriter` drain: `BenchmarkJournalTrapWriterDrain` reported about 4.5k-6.1k entries/s in local runs.
- SDK-backed direct `JournalWriter.WriteEntry`: `BenchmarkJournalWriterWriteEntry` reported about 4.9k-5.9k entries/s in local runs.
- This does not satisfy the >30k rows/sec per-writer target on this workstation. The benchmark now exposes the writer/SDK bottleneck for SOW-0039 merge-gate decision and SDK optimization work; it does not hide the miss.
- M1 disposition: harness complete with finding. This is not a throughput pass. M2/M3 remain pending and SOW-0039 must decide whether the SDK writer miss blocks merge or can ship with documented lower throughput after SDK-side work.

**Assumptions**:
- Multi-job benchmarks use one goroutine per collector to mirror per-listener serial packet handling while still exercising concurrent jobs.
- No UDP sockets are bound, and each job is represented by an independently constructed collector/writer pair.
- Packet path benchmark uses a fresh byte copy per iteration to match production UDP buffer ownership semantics.
- Decode-only benchmarks reuse the same generated packet bytes; GoSNMP parse path is read-only on input.
- Multi-job benchmark uses a counting writer, while SDK-backed writer behavior is measured separately by the journal benchmarks.
- Packet-path benchmarks report `drops` and `drop_pct` instead of hard-failing on any drop, because production decode-budget enforcement uses wall-clock time and can be affected by local scheduler/GC load.
- Journal benchmarks remain Linux-only (`requireLinuxJournalBenchmark` skip guard).
- Local benchmark numbers are evidence from this workstation, not portable acceptance thresholds. The benchmark output is the source of truth; this SOW records representative ranges to avoid overstating precision.

**Validation**:
- `go vet ./plugin/go.d/collector/snmp_traps/...` -> clean
- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...` -> PASS (2.193s)
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...` -> PASS (18.079s)
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*BER|Benchmark.*Multi|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps` -> PASS (28.342s)
- `git diff --check` -> clean

**Reviewer findings handled**:
- Kimi: manual benchmark collectors did not seed `c.metrics`, adding artificial global metrics mutex contention. Fixed by giving benchmark collectors `&perJobMetrics{}` like production `Init()`.
- Kimi: multi-job benchmark used `b.RunParallel` and could hit the same collector from multiple goroutines. Fixed by using one goroutine per synthetic job/listener.
- Kimi: packet-path benchmarks did not expose dropped packets. Fixed by reporting `drops` and `drop_pct` metrics.
- Kimi: SOW benchmark numbers needed hardware/load sensitivity. Fixed by recording representative local ranges instead of precise universal claims.
- Kimi: reverse-DNS resolver nil guard noted as a latent production hardening item. Not changed in M1 because M1 is benchmark-only and production `Init()` currently couples `reverseDNSEnabled` with resolver construction; this can be reconsidered in SOW-0039 if hardening is desired.
- MiniMax: max-varbind decode cost was missing. Fixed by adding `Varbinds=256`.
- MiniMax: SOW-0037 is paused, not completed. Kept as paused because the feature branch is not independently mergeable until the SOW-0039 gate.
- GLM: `BenchmarkBERRejection` measured structural BER rejection, not the full decode budget timer. Fixed in SOW wording.
- GLM: M1 needed an explicit disposition. Fixed by recording "harness complete with finding" and pointing the throughput decision to SOW-0039.
- Qwen: `BenchmarkJournalTrapWriterDrain` should retry `errQueueFull` consistently with the queue-only writer benchmark. Fixed by sharing a backpressure-aware write helper.
- Qwen: comments should make the benchmark-only profile-index shortcut and minimal multi-job packet explicit. Fixed in benchmark comments.
- Qwen first runs were technically unusable due command-loop behavior; the final no-shell review produced no blockers and only the minor fixes listed above.
- Final full-scope reviewer rerun: GLM, Kimi, MiniMax, and Qwen all concluded `PRODUCTION GRADE`. Kimi's final hygiene note found stale SOW-0037 "current/in-progress" wording; fixed to `current/paused`.

**No production behavior changed**. No config, schema, stock configuration, docs, health alerts, or listener runtime modified. M2/M3 implementation not started.

### 2026-05-27 — M2 SNMPv3 discovery implementation and local validation

**Implementation**:

1. **Job-creation validation**:
   - `dynamic_engine_id_discovery` is now implemented instead of rejected as deferred.
   - Static v3 jobs still require `engine_id_whitelist` and `usm_users[].engine_id`.
   - Dynamic v3 jobs reject non-empty `engine_id_whitelist`, may omit `usm_users[].engine_id`, and validate any supplied `engine_id` as a preconfigured known pair.
   - `dynamic_engine_id_max_pairs` is added; negative values fail job creation and 0/unset uses the default 4096.

2. **Dynamic v3 Trap sender engine ID discovery**:
   - Added bounded raw SNMPv3 context extraction for authoritative engine ID, username, reportable flag, and message ID.
   - Dynamic retry is default-off and only runs for non-reportable v3 messages.
   - The retry path builds a per-packet temporary USM table from configured credentials matching the raw username.
   - A candidate pair is persisted to the shared v3 security table only after the retry decodes a v3 Trap, not an INFORM.
   - Dynamic registrations are synchronized, in-memory, per-job, capped by `dynamic_engine_id_max_pairs`, and warn/increment `unknown_engine_id` once per first accepted dynamic `(engineID, username)` pair.
   - A shared-table read/write guard was added because GoSNMP's `Get()` returns a slice after releasing its internal lock; dynamic registration must not mutate the table while another goroutine is decoding from it.
   - Successful decodes also enforce the dynamic registry/cap. This closes the GoSNMP edge case where a noAuth/noPriv v3 Trap with the same username can decode while GoSNMP mutates the copied security parameters to the packet engine ID.

3. **v3 INFORM receiver-local discovery Report**:
   - Reportable empty/malformed authoritative engine ID probes now receive a v3 Report using the persisted `local_engine_id` and `snmpEngineBoots` state.
   - The Report contains `usmStatsUnknownEngineIDs` (`1.3.6.1.6.3.15.1.1.4.0`).
   - Valid non-local INFORM engine IDs remain rejected by the receiver-local engine ID validation path and are not treated as discovery probes.

4. **Operator config surfaces**:
   - `config_schema.json` now documents conditional `usm_users[].engine_id`, `dynamic_engine_id_discovery`, mutual exclusion with `engine_id_whitelist`, and `dynamic_engine_id_max_pairs`.
   - Stock `snmp_traps.conf` now shows the dynamic discovery contract and cap.
   - Spec §5 and §22 updated with the dynamic Trap and INFORM Report contracts.

5. **Tests added/updated**:
   - Raw v3 context extraction for non-reportable Trap and reportable discovery probe.
   - Dynamic Trap registration accepts subsequent traps without repeated warning/counting.
   - Dynamic cap rejects new pairs after the cap is full.
   - Reportable Trap messages do not enter dynamic registration.
   - Concurrent duplicate registration creates one known pair and one visibility increment.
   - Dynamic INFORM retry produces no dynamic registration.
   - Unknown username produces no dynamic state.
   - Rate-limit drop mode skips dynamic retry; sample mode allows retry while incrementing `rate_limited`.
   - Discovery Report wire format includes local engine ID, boots, message ID, and `usmStatsUnknownEngineIDs`.
   - Config validation covers implemented dynamic mode, static whitelist mutual exclusion, negative cap rejection, static USM engine ID requirement, and dynamic omitted-engine-ID allowance.

**Local validation**:

- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...` -> PASS (final M2 run 1.907s)
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...` -> PASS (final M2 run 17.360s)
- `jq empty plugin/go.d/collector/snmp_traps/config_schema.json` -> clean
- `go vet ./plugin/go.d/collector/snmp_traps/...` -> clean
- `go test -run 'Test.*Dynamic|Test.*EngineID|Test.*Inform|Test.*Discovery|TestConfigValidation|Test.*RateLimit' -count=1 ./plugin/go.d/collector/snmp_traps` -> PASS (final M2 run 0.100s)
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps` -> PASS (final M2 run 18.353s); `BenchmarkJournalTrapWriterDrain` measured 5511 entries/s and `BenchmarkJournalWriterWriteEntry` measured 5721 entries/s, still in the SDK-limited range already recorded for M1.
- `git diff --check` -> clean
- `.agents/sow/audit.sh` -> exit 0; pre-existing skill classification warnings remain, no new structural or sensitive-data failure reported.

**Reviewer round 1**:

- GLM: `PRODUCTION GRADE`; non-blocking notes on `validateDeferredConfig` naming, `unknown_engine_id` dual semantics, missing explicit tests for dynamic INFORM/rate-limit/unknown-username paths, and dynamic-mode read-lock cost.
- Kimi: `PRODUCTION GRADE`; non-blocking notes on missing upper bound for `dynamic_engine_id_max_pairs`, missing explicit tests for INFORM rejection, retry cleanup, rate-limit interaction, strict Report counter value, and legacy validation function name.
- MiniMax: `PRODUCTION GRADE`; non-blocking note on redundant idempotent dynamic registration check after retry and missing explicit positive static-v3 test.
- Qwen: `NOT PRODUCTION GRADE`; findings rejected as false positives by code evidence:
  - Qwen said sample-mode dynamic retry drops packets. Actual `allowDynamicRetry` returns `mode != rateLimitModeDrop`, so sample mode returns allowed=true and continues. Added explicit sample/drop dynamic-rate-limit tests.
  - Qwen said discovery Reports are gated by `dynamic_engine_id_discovery`. Actual code uses `if c.dynamicEngineID && !rawCtx.reportable { ... } else if rawCtx.discoveryProbe() { ... }`, so static v3 jobs also enter the discovery Report branch. No code change.
  - Qwen said first dynamic sender double-increments `unknown_engine_id`. Actual retry success sets `err = nil`, so the original decode-error increment is skipped; only `registerDynamicEngineID(... isNew=true)` increments once. No code change.
  - Qwen said cap-full decoded Traps should still be journaled. This contradicts the recorded M2 contract: cap exhaustion rejects new pairs and increments `unknown_engine_id`, while existing pairs continue to work. No code change.

**Fixes after reviewer round 1**:

- Dynamic registration and temporary retry table now validate raw engine IDs with `parseEngineIDHex`, matching static validation for 5-32 byte length and all-zero/all-0xff rejection. This closes an implementation gap found during follow-up test design.
- Added focused tests for dynamic INFORM retry producing no registration, unknown username producing no dynamic state, rate-limit drop mode skipping dynamic retry, and rate-limit sample mode allowing dynamic retry while incrementing `rate_limited`.

**Reviewer status**:

Round 2 full-scope M2 implementation review completed. Mimo remained excluded by user instruction.

- GLM: `PRODUCTION GRADE`; non-blocking notes on `validateDeferredConfig` naming, `unknown_engine_id` dual semantics, dynamic-mode RLock cost, and missing edge-case tests. No code change after round 2: tests already cover the required M2 acceptance paths; naming/metric granularity/RLock cost are not blockers.
- Kimi: `PRODUCTION GRADE`; non-blocking notes on adding a hard upper bound for `dynamic_engine_id_max_pairs`, renaming `validateDeferredConfig`, adding a positive static-v3 acceptance test, splitting discovery Report send failures from `inform_response_failed`, and removing the `dynamic` counter. No code change after round 2:
  - hard upper bound is a product/config contract decision outside M2; current behavior follows nearby config-cap patterns by rejecting negatives and defaulting unset/0 while letting the operator choose the cap;
  - `dynamic` is not redundant because preconfigured known pairs are seeded into `known` but do not consume the dynamic-registration cap;
  - static v3 positive behavior is covered indirectly by existing v3 paths and static rejection tests; no accepted M2 blocker;
  - discovery Report send failure shares the existing UDP response-failure dimension by design for M2.
- MiniMax: `PRODUCTION GRADE`; non-blocking notes on the same naming/counter/test hygiene themes. Its rate-limit double-token concern is not real in current code because `collector.go` skips the main post-decode limiter when `rateLimitChecked` is true.
- Qwen: first round-2 attempt timed out after 30 minutes without a verdict; rerun with the same full scope returned `PRODUCTION GRADE` and confirmed the round-1 false-positive clarifications.

M2 reviewer gate is closed. M3 may start only after the M3 pre-implementation gate is added.

### 2026-05-27 — M3 OTLP exporter implementation and local validation

**Implementation management**:

- DeepSeek v4 pro was launched as the requested implementation assistant with the SOW filename in the prompt and stdin disabled via `</dev/null`.
- That run inspected context but did not produce a useful implementation diff. After it stalled, only the specific timeout/opencode PIDs from that run were terminated.
- Implementation was completed locally to keep the SOW moving, which is allowed by the recorded user instruction that the coordinator may unblock work directly when external assistants are stuck or confused.

**Implementation**:

1. **Config and job-creation validation**:
   - Added per-job `otlp:` config with `enabled`, `endpoint`, `headers`, `request_timeout`, `flush_interval`, `batch_size`, and `queue_capacity`.
   - Disabled OTLP allocates no writer and does not require a receiver.
   - Enabled OTLP validates endpoint syntax, durations, queue/batch limits, and metadata header names at job creation.
   - Endpoint syntax is strict: bare `host:port` and `http://host:port` use plaintext gRPC; `https://host:port` uses TLS; schemes, paths, query strings, and fragments are rejected.
   - `Collector.Init()` creates the OTLP client and performs an empty OTLP Logs `Export` preflight before starting the listener, so unreachable or non-OTLP receivers fail DynCfg apply with code 422.

2. **Fan-out writer**:
   - Journal writer remains primary and authoritative.
   - OTLP writer is secondary. Secondary enqueue/export/flush/close failures increment `otlp_export_failed` and do not report a packet-path writer failure.
   - Primary journal write failures preserve existing `journal_write_failed` behavior and dedup rollback.
   - The fan-out wrapper forwards journal `SanitizedFields()` so OTLP-enabled jobs keep publishing the existing sanitized-field metric.

3. **OTLP writer**:
   - Uses the OTLP Logs gRPC proto client directly, without adding the full OTel SDK.
   - Maintains a persistent gRPC connection.
   - `Write()` only enqueues into a bounded channel and returns `errQueueFull` if the OTLP queue is full.
   - Worker exports batches by `batch_size` or `flush_interval`.
   - Failed export batches remain pending for retry; `otlp_export_failed` counts newly affected accepted records without re-counting the same stuck batch on every tick.
   - `Flush()` and `Close()` drain accepted entries best-effort.

4. **OTLP serialization**:
   - Resource attributes: `service.name=netdata-snmptrap`, `service.instance.id=<job_name>`.
   - LogRecord body is the rendered trap message.
   - Top-level `event_name` is `snmp.trap.<category>` or `snmp.trap.deduplication_summary`.
   - Severity mapping follows the spec table: emerg=FATAL/21, alert=ERROR3/19, crit=ERROR2/18, err=ERROR/17, warning=WARN/13, notice=INFO2/10, info=INFO/9, debug=DEBUG/5.
   - Attributes include the spec-defined `network.peer.address`, `snmp.*`, `netdata.*`, and `trap.*` fields, with empty optional topology/device/vendor/vnode fields omitted.
   - `snmp.varbinds` is encoded as nested OTLP key-value lists; dedup summaries use the same nested structure for summary counts.

5. **Operator surfaces**:
   - Updated `config_schema.json` for DynCfg.
   - Updated stock `snmp_traps.conf` with a commented disabled OTLP example.
   - Updated spec `.agents/sow/specs/snmp-traps/netdata.md` with concrete OTLP config, error metric, and hot-path behavior.

6. **Tests added/updated**:
   - Endpoint parsing and strict rejection of unsupported schemes/paths.
   - OTLP config defaults and invalid durations/headers.
   - Normal trap serialization.
   - Dedup summary serialization.
   - Preflight success with metadata headers.
   - Preflight failure at job creation.
   - Queue-full behavior.
   - Fan-out primary/secondary failure isolation for write, flush, and close.
   - Fan-out sanitized metric forwarding.
   - `otlp_export_failed` metric collection.
   - Optional external receiver test gated by `NETDATA_TEST_SNMP_TRAPS_OTLP_ENDPOINT`.

**Local validation**:

- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json` -> clean.
- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...` -> PASS (final M3 run 2.231s).
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...` -> PASS (final M3 run 18.207s).
- `go vet ./plugin/go.d/collector/snmp_traps/...` -> clean.
- `go test -run 'Test.*OTLP|Test.*FanoutTrapWriter|TestConfigValidation|TestCollectMetricsEmitsCounters' -count=1 ./plugin/go.d/collector/snmp_traps` -> PASS (final M3 run 0.083s).
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps` -> PASS (final M3 run 16.068s); `BenchmarkJournalTrapWriterDrain` measured 5259 entries/s and `BenchmarkJournalWriterWriteEntry` measured 5675 entries/s, still in the SDK-limited range already recorded for M1.
- `git diff --check` -> clean.
- `.agents/sow/audit.sh` -> exit 0; pre-existing skill classification warnings remain, no new structural or sensitive-data failure reported.
- `command -v otelcol-contrib` and `command -v otelcol` -> not found on this workstation.
- Built the in-tree Netdata OTEL plugin with `cargo build -p otel-plugin --bin otel-plugin` -> PASS (32.94s).
- Ran the optional external receiver test against the built Netdata OTEL plugin at a temporary `127.0.0.1:14317` endpoint -> PASS.
- Queried the OTEL plugin's temporary journal with `journalctl --directory=<temporary plugin journal dir> -o json --no-pager`; the flattened data row included `log.event_name=snmp.trap.state_change`, `resource.attributes.service.name=netdata-snmptrap`, `resource.attributes.service.instance.id=external`, `log.body=External receiver interop test`, `log.severity_number=13`, `log.severity_text=WARN`, `log.attributes.snmp.source.ip=[RFC5737_TEST_IP]`, and `log.attributes.snmp.trap.oid=[SNMP_TRAP_OID]`.
- The foreground validation script killed only the specific OTEL plugin and stdin-keeper PIDs it started; follow-up `pgrep` checks found no remaining `otel-plugin` or `tail -f /dev/null` validation processes.

**External receiver gap**:

- A separate OpenTelemetry Collector / Collector Contrib binary is not installed on this workstation. M3 has in-process OTLP gRPC fixture coverage plus Netdata OTEL plugin real-receiver coverage; packaged OpenTelemetry Collector fixture coverage remains a SOW-0039 merge-gate follow-up unless reviewers require it here.

**Reviewer round 1**:

- GLM: `PRODUCTION GRADE`; non-blocking notes on double OTLP config validation, aliasing OTLP queue capacity to the journal queue capacity, and the missing standalone `otelcol` binary fixture.
- MiniMax: `PRODUCTION GRADE`; non-blocking notes on `Flush()`/`Close()` metric granularity and redundant validation branches. Its `Close()` metric finding is accepted only as an observability nuance, not a correctness blocker, because `Close()` errors happen during teardown and the OTLP writer itself counts per accepted record when export failures occur.
- Qwen: `NOT PRODUCTION GRADE`; accepted blocker: `otlpTrapWriter.Flush()` could race with `Close()` and block on the unbuffered `flushCh` if the worker exited between the `closed` check and the send. Qwen also noted a spec-code inconsistency where normal trap OTLP records conditionally omitted `network.peer.address` and `snmp.source.ip` even though the spec marks them always emitted.
- Kimi: technical stall. It read files and ran spot checks for several minutes but did not return a verdict. Only its specific timeout/opencode PIDs were terminated. Kimi will be rerun with the same full scope after the accepted fixes.

**Fixes after reviewer round 1**:

- Fixed `otlpTrapWriter.Flush()`/`Close()` synchronization by holding the writer mutex across the channel-handshake select and by selecting on `doneCh`. This prevents `Flush()` from sending to a worker that has exited and prevents `Close()` from racing past an in-flight `Flush()` check.
- Added `TestOTLPTrapWriterFlushAfterCloseReturns` to verify `Flush()` after `Close()` returns `errWriterClosed` instead of blocking.
- Made normal trap OTLP records always include `network.peer.address` and `snmp.source.ip` attributes, matching spec wording. In real packet paths these values are populated from the UDP peer; the change also makes synthetic `TrapEntry` serialization obey the same contract.

**Post-fix local validation**:

- `go test -run 'Test.*OTLP|Test.*FanoutTrapWriter|TestConfigValidation|TestCollectMetricsEmitsCounters' -count=1 ./plugin/go.d/collector/snmp_traps` -> PASS (0.098s).
- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...` -> PASS (2.196s).
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...` -> PASS (17.951s).
- `go vet ./plugin/go.d/collector/snmp_traps/...` -> clean.
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json` -> clean.
- `git diff --check` -> clean.
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps` -> PASS (15.955s); `BenchmarkJournalTrapWriterDrain` measured 4428 entries/s and `BenchmarkJournalWriterWriteEntry` measured 5497 entries/s, still in the SDK-limited range already recorded for M1.
- Reran the optional external receiver test against the built Netdata OTEL plugin with `NETDATA_TEST_SNMP_TRAPS_OTLP_ENDPOINT=http://127.0.0.1:14317` -> PASS (0.044s). The flattened journal row still included `log.event_name=snmp.trap.state_change`, `resource.attributes.service.name=netdata-snmptrap`, `resource.attributes.service.instance.id=external`, `log.body=External receiver interop test`, `log.severity_number=13`, `log.severity_text=WARN`, `log.attributes.snmp.source.ip=[RFC5737_TEST_IP]`, `log.attributes.network.peer.address=[RFC5737_TEST_IP]`, and `log.attributes.snmp.trap.oid=[SNMP_TRAP_OID]`.
- Follow-up `pgrep` checks found no remaining `otel-plugin` or `tail -f /dev/null` validation processes.

**Reviewer round 2**:

- GLM: `PRODUCTION GRADE`. Verified the round-1 Flush/Close fix, always-emitted `network.peer.address` / `snmp.source.ip`, creation-time OTLP preflight, secondary-backend isolation, metadata header handling, bounded queue behavior, OTLP attribute shape, and Netdata OTEL plugin interop. Non-blocking notes: redundant validation branches, `context.Background()` inside worker export bounded by request timeout, secondary `Close()` delaying primary close during teardown, and no configured upper bound for very large OTLP queue/batch values.
- Kimi: `PRODUCTION GRADE`. Verified SOW/spec/code/schema/config consistency, Init-time 422 handling, OTLP fan-out semantics, dynamic SNMPv3 behavior, and no sensitive-data exposure.
- MiniMax: `PRODUCTION GRADE`. Non-blocking finding about `notice` severity was rejected by source evidence: `otlpSeverity()` maps the default/notice path to `SEVERITY_NUMBER_INFO2`, `"INFO2"`, `"notice"`, matching spec §11b. Other non-blocking notes covered secondary `Close()` error masking and test-hygiene observations.
- Qwen: `PRODUCTION GRADE`. Verified the accepted round-1 blocker fix at `otlpTrapWriter.Flush()` / `Close()`, severity mapping, fan-out isolation, job-creation preflight, queue backpressure, header security, and Netdata OTEL plugin interop. Non-blocking notes: `fanoutTrapWriter.Flush()` / `Close()` increments `otlp_export_failed` by one for secondary teardown failures, and standalone `otelcol` binary coverage remains unavailable on this workstation.

No round-2 blocker was accepted. No code change was made after round 2. The external reviewer gate for M3 is closed.

## Validation

Acceptance criteria evidence: M1 benchmark harness evidence recorded above. Result: harness complete, throughput target not met by the current SDK-backed writer path on this workstation. M2 local implementation and reviewer evidence recorded above. M3 local implementation, Netdata OTEL-plugin interop evidence, and external reviewer evidence recorded above. Separate OpenTelemetry Collector binary coverage remains tracked by SOW-0039 merge-gate work; M3 has in-process OTLP gRPC fixture coverage plus Netdata OTEL receiver evidence.
Tests or equivalent validation: M1 unit, race, vet, benchmark, and diff validation passed. M2 unit, race, vet, targeted, schema, diff, benchmark, SOW audit, and external reviewer validation passed. M3 schema, unit, targeted, race, vet, benchmark, diff, SOW audit, and Netdata OTEL-plugin real receiver validation passed.
Final closeout validation after M3 reviewer gate:
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json` -> clean.
- `git diff --check` -> clean.
- `go vet ./plugin/go.d/collector/snmp_traps/...` -> clean.
- `go test -run 'Test.*OTLP|Test.*FanoutTrapWriter|TestConfigValidation|TestCollectMetricsEmitsCounters' -count=1 ./plugin/go.d/collector/snmp_traps` -> PASS (0.085s).
- `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...` -> PASS (2.852s).
- `go test -race -count=1 -timeout 120s ./plugin/go.d/collector/snmp_traps/...` -> PASS (18.410s).
- `go test -run '^$' -bench 'Benchmark.*Trap|Benchmark.*Decode|Benchmark.*Journal' -benchmem ./plugin/go.d/collector/snmp_traps` -> PASS (14.017s); `BenchmarkJournalTrapWriterDrain` measured 4319 entries/s and `BenchmarkJournalWriterWriteEntry` measured 5705 entries/s, still in the SDK-limited range recorded for M1.
- `.agents/sow/audit.sh` -> exit 0 after redacting synthetic RFC 5737 source IP and numeric trap OID evidence from the SOW; pre-existing skill classification warnings remain.
Real-use evidence: M3 was verified against the built in-tree Netdata OTEL plugin over OTLP/gRPC with the plugin writing the trap to its temporary flattened journal. `otelcol` / `otelcol-contrib` was not installed, so separate OpenTelemetry Collector binary validation remains represented by SOW-0039.
Reviewer findings: M1 review rounds produced benchmark methodology, wording, and queue-backpressure fixes; fixed or explicitly not changed with reason above. Final full-scope M1 reviewer rerun returned `PRODUCTION GRADE` from GLM, Kimi, MiniMax, and Qwen. M2 reviewer round 1 produced no accepted blockers; false-positive blockers and accepted fixes are recorded above. M2 round 2 returned `PRODUCTION GRADE` from GLM, Kimi, MiniMax, and Qwen after one Qwen timeout rerun. M3 round 1 produced one accepted blocker from Qwen; fixed and validated. M3 round 2 returned `PRODUCTION GRADE` from GLM, Kimi, MiniMax, and Qwen. MiniMax's `notice` severity concern was rejected by source evidence at `otlpSeverity()`, which maps notice/default to OTel `INFO2` / 10.
Same-failure scan: M1 benchmark helper and methodology issues found and fixed (marshal error handling, benchmark-only profile index restore, synthetic source distribution, production-shaped metrics pointer, one-goroutine-per-job multi-job benchmark, drop reporting, max-varbind decode case, journal drain backpressure retry). M2 same-failure search covered static validation, reportable-message prefilter, cap exhaustion, concurrent duplicate registration, successful-decode cap enforcement, raw engine ID validation, unknown username cleanup, dynamic INFORM skip, rate-limit drop/sample behavior, and discovery Report wire format. M3 same-failure search covered endpoint validation, creation-time preflight, secondary backend isolation, fan-out metric forwarding, OTLP queue-full handling, metric collection, synthetic secret/header handling, and Netdata OTEL flattened journal output.
Sensitive data gate:

M2 runtime warning intentionally includes synthetic-safe `engineID` and `username` only; it does not include USM keys, communities, packet payload bytes, varbind values, customer identifiers, or raw trap bytes. M3 OTLP attributes do not include USM keys, community strings, raw packet bytes, or configured OTLP header values; tests use synthetic RFC 5737 source IPs and synthetic header values only. SOW evidence uses placeholders (`[RFC5737_TEST_IP]`, `[SNMP_TRAP_OID]`) instead of raw public-looking values.

Artifact maintenance gate:
- `AGENTS.md`: no workflow or repository guardrail update required. Existing SOW, secret-handling, external-reviewer, and collector consistency rules covered this work.
- Runtime project skills: `.agents/skills/project-writing-collectors/` was loaded and remains accurate; no reusable new collector-authoring rule was exposed beyond existing hot-path, config, and validation guidance.
- Specs: `.agents/sow/specs/snmp-traps/netdata.md` was updated for M2 dynamic engine ID discovery and M3 OTLP config/error-metric contracts.
- End-user/operator docs: full operator docs remain intentionally owned by SOW-0039. This SOW updated operator-facing DynCfg schema and stock config examples that are required before docs.
- End-user/operator skills: `query-snmp-traps` remains owned by SOW-0039; no public AI skill was changed in SOW-0038.
- SOW lifecycle: SOW-0038 is completed and moved to `done/` with the implementation commit; SOW-0039 is the tracked merge gate for bundle facets, docs, public skill, external receiver fixture follow-up, and final merge decision.

## Outcome

M1, M2, and M3 are implementation-complete and reviewer-validated.

- M1 delivered a benchmark harness and exposed a real SDK-backed journal-writer throughput miss on this workstation.
- M2 delivered opt-in SNMPv3 dynamic Trap sender engine ID discovery, capped in-memory registrations, and v3 INFORM receiver-local discovery Reports.
- M3 delivered optional vendor-neutral OTLP/gRPC Logs export as a secondary fan-out backend with creation-time preflight, bounded runtime backpressure, `otlp_export_failed` observability, and Netdata OTEL-plugin interop.

This SOW is completed. It is not independently mergeable; SOW-0039 remains the final merge gate.

## Lessons Extracted

- Reviewer batch size matters: Qwen found a real Flush/Close race only when reviewing the full M3 implementation batch; narrow line-by-line reviews would likely have missed the lifecycle interaction.
- SOW evidence must avoid raw public-looking dotted numeric values even for synthetic RFC 5737 IPs or SNMP OIDs, because durable artifact audits cannot distinguish them from sensitive data.
- Direct OTLP proto usage is sufficient for this collector; the full OpenTelemetry SDK is unnecessary for a simple secondary exporter and would add avoidable memory/dependency surface.
- The systemd journal SDK writer remains the measured throughput limiter. The benchmark harness now makes that visible instead of burying it under in-memory packet-path numbers.

## Followup

- SOW-0039 owns the final merge gate, end-user/operator documentation, public `query-snmp-traps` AI skill, bundle facets, and any packaged standalone `otelcol` / `otelcol-contrib` fixture coverage.
- SOW-0039 owns the merge decision around the measured SDK-backed writer throughput miss unless SDK-side work resolves it first.

## Regression Log

### Regression - 2026-05-27 - go.d Framework Metadata Rejected Before Listener Bind

What broke:

- Direct collector e2e validation passed, but the normal `go.d.plugin -d -m snmp_traps -j local` path failed before binding the configured UDP listener.
- The job factory passes framework-owned metadata keys (`name`, `module`, `autodetection_retry`, `priority`, `__provider__`, `__source__`, `__source_type__`) into the collector config map. `snmp_traps` strict YAML validation rejected the first internal key as an unknown operator config key, so job creation failed and the listener never started.

Evidence:

- `src/go/plugin/framework/confgroup/config.go` defines the framework config keys and internal metadata keys.
- `src/go/plugin/go.d/collector/snmp_traps/config.go` strict unknown-key validation did not whitelist those framework-owned keys.
- Manual smoke test evidence: local debug run reported `failed to apply config for snmp_traps[local] job: unknown config key "__provider__"` and `CONFIG go.d:collector:snmp_traps:local status failed`.
- Control evidence: `go test -run TestCollectorReplayPcapThroughListenerToJournal -count=1 -v ./plugin/go.d/collector/snmp_traps` passed, proving the listener/journal packet path works when the collector is instantiated directly.

Why previous validation missed it:

- Prior tests exercised direct `Collector.Init()` and package-level config unmarshalling, but not the full `go.d.plugin` job factory path where framework metadata is injected before collector unmarshalling.

Repair plan:

- Allow the known go.d framework-owned top-level keys in the strict YAML key spec without relaxing nested operator configuration validation.
- Add regression test coverage showing framework metadata is accepted while unrelated top-level and nested unknown keys remain rejected.
- Rerun the direct e2e test and a manual `go.d.plugin` smoke test that sends a synthetic v2c coldStart trap to a high UDP port and queries the generated journal file.

Validation updates required:

- Direct e2e pass recorded: `go test -run TestCollectorReplayPcapThroughListenerToJournal -count=1 -v ./plugin/go.d/collector/snmp_traps` -> PASS.
- Config regression coverage recorded: `go test -run 'TestConfigValidation|TestCollectorReplayPcapThroughListenerToJournal' -count=1 -v ./plugin/go.d/collector/snmp_traps` -> PASS, including new `framework_metadata_keys` case while `unknown_config_key` and `unknown_nested_config_key` still pass.
- Manual `go.d.plugin` smoke test recorded: built `go.d.plugin` with a temporary cache dir, configured one `snmp_traps[local]` job on a high UDP port, ran under terminal debug mode, sent a synthetic v2c coldStart with `snmptrap`, and queried the generated journal directory. Evidence returned `MESSAGE="Device reinitializing and configuration may have been altered on [LOOPBACK_IP]."`, `TRAP_OID=[COLDSTART_TRAP_OID]`, `TRAP_NAME=SNMPv2-MIB::coldStart`, `TRAP_CATEGORY=state_change`, `TRAP_SEVERITY=notice`, `TRAP_VERSION=v2c`, and `TRAP_SOURCE_IP=[LOOPBACK_IP]`. The debug run logged `check success` and `CONFIG go.d:collector:snmp_traps:local status running`.
- Full package validation recorded: `go test -count=1 -timeout 60s ./plugin/go.d/collector/snmp_traps/...` -> PASS (2.680s).
- Static validation recorded: `go vet ./plugin/go.d/collector/snmp_traps/...` -> clean.
- Whitespace validation recorded: `git diff --check` -> clean.
- Reviewer findings: no external reviewer rerun was used for this narrow regression repair. The accepted finding came from manual runtime smoke testing, and the repair was self-reviewed against framework key definitions in `src/go/plugin/framework/confgroup/config.go` plus the retained unknown-key regression tests.
- Artifact maintenance: specs and operator docs are unchanged because this repair preserves existing behavior and only accepts framework-owned metadata that normal go.d job creation already injects. SOW lifecycle is updated by reopening and reclosing SOW-0038 with this regression log.
