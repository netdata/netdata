# SOW-0038 - SNMP Trap Plugin: Throughput Benchmark + Dynamic engineID (opt-in) + OTLP Exporter

## Status

Status: open

Sub-state: queued in `.agents/sow/pending/`. Depends on SOW-0037 completion. NOT independently mergeable — merge gate is SOW-0039.

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
- M2: SNMPv3 discovery available as opt-in behavior where appropriate: v3 Trap sender engineID hot-registration available behind a default-off flag; v3 INFORM receiver-local engine ID Report response implemented so senders can discover the job's local engine ID. Operator warnings surfaced in docs and at runtime when an unknown sender engineID is hot-registered.
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
- v3 INFORM receiver-local engine ID: implement the RFC 3414 discovery Report path for empty/unknown authoritative engine ID probes, using the SOW-0036 persisted `local_engine_id` and `snmpEngineBoots` state. This is required for operators who do not want to manually configure Netdata's generated receiver engine ID into INFORM senders.
- Spec §5 lists v3 Trap sender discovery as opt-in due to spoofing surface; the SOW must surface that clearly. The v3 INFORM Report responder is not a sender hot-registration path and must not weaken the Trap sender allowlist.

Cohort reference: `splunk-sc4snmp.md` §3.5; `opennms/opennms` INFORM discovery test pattern; spec §5 v3 USM section.

Reviewers: all 7 — security-sensitive.

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

Reviewers: all 7 — interop-critical (must work with arbitrary OTLP receivers without modification).

## Reviewer Protocol

- M2 + M3: all 7 reviewers (security + interop critical).
- M1: 3 rotating reviewers per round (group A: kimi/qwen/minimax).
- Fix-cycle: same reviewers as the round being fixed.

## Pre-Implementation Gate

Status: blocked

Reason: depends on SOW-0037 completion. Full gate filled at activation — depends on cumulative plugin state from SOW-0035–0037.

## Plan

Sequential M1 → M3. The end-user AI skill, user documentation, SOW-0032 closeout, and the final merge are OWNED BY SOW-0039 and not in this SOW.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup: M1 multi-writer partitioning removed (per-job is the partition); M5 AI skill and M6 SOW-0032 closeout moved to SOW-0039 (final merge SOW). Not yet activated.

## Validation

Acceptance criteria evidence: pending.
Tests or equivalent validation: pending.
Real-use evidence: pending — OTLP exporter must be verified against multiple receivers (Netdata OTEL plugin + external collector).
Reviewer findings: pending.
Same-failure scan: pending.
Sensitive data gate: pending — OTLP exporter must not expose USM keys, community strings, or operator-secret data in any default attribute.
Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
