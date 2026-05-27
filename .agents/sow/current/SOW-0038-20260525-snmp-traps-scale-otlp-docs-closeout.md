# SOW-0038 - SNMP Trap Plugin: Throughput Benchmark + Dynamic engineID (opt-in) + OTLP Exporter

## Status

Status: in-progress

Sub-state: activated on 2026-05-27 after SOW-0037 reached implementation-complete / paused state. M1 throughput benchmark harness is the active milestone. M2 SNMPv3 discovery and M3 OTLP exporter remain sequentially gated inside this SOW and must not start until M1 is validated. NOT independently mergeable — merge gate is SOW-0039.

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

Status: ready for M1 implementation.

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

## Validation

Acceptance criteria evidence: M1 benchmark harness evidence recorded above. Result: harness complete, throughput target not met by the current SDK-backed writer path on this workstation. M2/M3 pending.
Tests or equivalent validation: M1 unit, race, vet, benchmark, and diff validation passed; full SOW validation pending M2/M3.
Real-use evidence: pending — OTLP exporter must be verified against multiple receivers (Netdata OTEL plugin + external collector).
Reviewer findings: M1 review rounds produced benchmark methodology, wording, and queue-backpressure fixes; fixed or explicitly not changed with reason above. Final full-scope reviewer rerun returned `PRODUCTION GRADE` from GLM, Kimi, MiniMax, and Qwen.
Same-failure scan: M1 benchmark helper and methodology issues found and fixed (marshal error handling, benchmark-only profile index restore, synthetic source distribution, production-shaped metrics pointer, one-goroutine-per-job multi-job benchmark, drop reporting, max-varbind decode case, journal drain backpressure retry); M2/M3 pending.
Sensitive data gate:

Pending — OTLP exporter must not expose USM keys, community strings, or operator-secret data in any default attribute.
Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
