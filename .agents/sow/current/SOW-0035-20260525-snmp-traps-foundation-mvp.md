# SOW-0035 - SNMP Trap Plugin Foundation + MVP Slice (multi-endpoint listener, shared profile loader, journal writer)

## Status

Status: in-progress

Sub-state: activated on 2026-05-25. The user has fixed the implementation language to Go and clarified the listener/profile lifecycle contract. Implementation is delegated primarily to `deepseek/deepseek-v4-pro`; reviews are delegated to `glm`, `kimi`, `mimo`, `minimax`, and `qwen`. The coordinating assistant remains responsible for architecture, direct edits, unblocking, integration, validation, and final quality.

## Requirements

### Purpose

Ship the minimum vertical slice of Netdata's SNMP trap subsystem under the listener-as-job architecture (spec §5):

- DynCfg-managed jobs where each job = one listener that may bind one or more configured protocol/address/port endpoints + own dedup cache + own writer + own journal directory + own retention.
- Decode SNMPv1/v2c traps, resolve them against the OOB profile pack already shipped under `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/`, render MESSAGE + labels per spec §7.
- Write to per-job systemd-journal directories at `/var/cache/netdata/traps/{job_name}/` with the field universe defined in spec §11.
- Load trap profiles lazily on first runnable trap job creation, share the loaded profile index across all listeners, and release it when no runnable trap jobs remain.

Operator enables the stock `local` job (disabled by default) via DynCfg. Any job-creation failure must be detected before DynCfg apply succeeds: endpoint bind failures, unsupported endpoint protocol, invalid job name, profile-load failure, journal directory creation/open failure, writer initialization failure, and retention configuration failure all return a coded DynCfg error. Users must not see a job reported as started and only later discover the failure in logs.

This SOW lands on a feature branch; it is NOT independently mergeable. The merge gate is SOW-0039 (collector consistency bundle + facets + docs + final merge).

### User Request

User-requested 5-SOW slicing of the full trap subsystem (revised from the original 4-SOW plan after reviewer round 1 surfaced a collector-consistency merge hazard). SOW-0035 is the MVP foundation under the new listener-as-job architecture confirmed by the user during planning.

User clarification recorded on 2026-05-25:

- All job-creation failures must surface synchronously at DynCfg apply time, not later as runtime-only log messages.
- Each listener may support multiple ports and protocols. Multiple listeners are for scaling/isolation, not the only way to accept more than one endpoint.
- Profiles load on first runnable trap job creation, not at permanent `go.d.plugin` startup, so agents that do not use traps do not pay the memory cost.
- Profiles are shared across listeners, not loaded once per listener.
- Implementation language is Go, not Rust.

User implementation workflow decision recorded on 2026-05-25:

- `deepseek/deepseek-v4-pro` is the primary implementation worker.
- DeepSeek must not be run with `--agent code-reviewer`; it needs write access for implementation tasks.
- Reviews use `glm`, `kimi`, `mimo`, `minimax`, and `qwen`.
- The coordinating assistant may make direct edits, perform reviews, and unblock external workers when that improves quality or progress. External models are helpers, not autopilot.

### Assistant Understanding

Facts:

- The OOB profile pack already ships (commit `4056ffac1d`): 50,198 traps across 351 vendor files, MIB-qualified `name:` (required field), file-scoped varbinds tables, zero defects across 8 invariants.
- Spec `.agents/sow/specs/snmp-traps/netdata.md` is the authoritative subsystem design. §5 listener-as-job architecture, §6 reception surface (port 162 + INFORM semantics), §7 profile schema (file-scoped varbinds, MIB-qualified name required), §11 per-job journal storage + retention model, §11b OTLP attribute universe, §17 5-SOW lineup, §18 BER decode limits, §19 TrapWriter interface contract.
- NetFlow plugin (`src/crates/netflow-plugin/plugin_config/types/journal.rs`) defines the per-tier retention config pattern the trap plugin mirrors per-job (`max_size: 10GB` default; `max_duration: null` default; both apply independently).
- SNMP polling plugin profile loader (`src/go/plugin/go.d/collector/snmp/ddsnmp/load.go`) defines the multipath + filename-dedup + extends-chain field-merge pattern the trap loader mirrors.
- go.d framework job orchestration (`src/go/plugin/framework/jobruntime/job_v1.go` + `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`) provides the Add/Update/Enable/Disable/Remove lifecycle the trap plugin reuses; HTTP-422 error coding for bind failures surfaces in DynCfg UI.
- systemd-journal collector facet registration happens via `SYSTEMD_KEYS_INCLUDED_IN_FACETS` in `src/collectors/systemd-journal.plugin/systemd-journal.c` — owned by SOW-0039, NOT this SOW.
- User decision on 2026-05-25: the trap subsystem is implemented in Go; a listener job may own multiple endpoints; profile state is lazy-loaded once per trap subsystem lifetime with active jobs and shared across listeners; all creation-time failures are DynCfg apply failures.

Inferences:

- The cross-plugin enrichment mechanism (in-process struct sharing for Go-in-process, `netipc` otherwise) is decided in M1 but full enrichment implementation is deferred to SOW-0037.

Unknowns:

- Whether the Go trap implementation lives as standard in-process go.d module code or needs any separate process boundary for the journal writer path (resolved in M1).
- Final TrapWriter method signatures, `TrapEntry` shape, batching, flush, and close semantics (resolved in M1).

### Acceptance Criteria

- M1: spec §5 records Go as the implementation language; process model/journal-writer backend resolved; spec §19 TrapWriter interface contract finalized; ADR document under `.agents/sow/specs/snmp-traps/decisions/`. Reviewer consensus across all 7 reviewers.
- M2: per-job multi-endpoint listener with DynCfg orchestration; SNMPv1/v2c decode + RFC 3584 v1→v2c; source identification cascade; per-job replayable pcap test corpus; all endpoint binds and job resources are preflighted at job creation with HTTP-422 surfaced in DynCfg on any failure.
- M3: shared lazy profile YAML loader (multipath, filename-dedup, extends-chain merge) loaded on first runnable job creation and shared across listeners; OID index; 2-tier varbind resolution (profile → raw, NO runtime MIB compilation per §14); template renderer producing MESSAGE + `TRAP_TAG_*` labels per §7 syntax (operator/profile labels emit as `TRAP_TAG_<KEY>`, NOT `TRAP_*` which is reserved for plugin-controlled fields).
- M4: per-job journal writer producing the spec §11 field universe at `/var/cache/netdata/traps/{job_name}/`; per-field text/binary encoding for CWE-117; per-job retention config mirroring the NetFlow plugin retention semantics with **intentional deviation** on `retention.max_duration` default (`null` for the trap plugin vs `7d` for NetFlow — rationale in spec §11); end-to-end test (replay pcap from M2 corpus → entry visible via `journalctl --directory=/var/cache/netdata/traps/test/ TRAP_CATEGORY=security`). Reviewer consensus across all 7 reviewers.

## Milestones

### M1 — Architecture decision + spec finalize

Decide and record in the spec:

- **Language** — Go, per user decision on 2026-05-25.
- **Process model / writer backend** — standard in-process go.d module code vs any justified process boundary for the journal writer path.
- **Cross-plugin state access mechanism** — in-process struct sharing if the Go implementation stays in `go.d.plugin`; otherwise a justified IPC mechanism. Full enrichment deferred to SOW-0037.
- **Shared profile cache lifecycle** — load on first runnable trap job creation, share across listeners, and release when no runnable trap jobs remain.
- **TrapWriter interface contract** — per spec §19. Confirm method signatures + `TrapEntry` shape.

Spec updates applied at M1 close:

- Resolve §5 language/process model with Go as the fixed language and record the remaining process/writer rationale.
- Finalize §19 TrapWriter interface contract.
- Update §13 to record the resolution in the Resolved-by-user-decisions list.

Output: ADR document at `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md`.

Reviewers: `glm`, `kimi`, `mimo`, `minimax`, and `qwen` — consensus-critical. The coordinating assistant also performs its own review and may make direct edits before/after external review.

### M2 — Per-job UDP listener + SNMPv1/v2c decode + source identification + test corpus

- DynCfg-managed job orchestration mirroring `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`:
  - Add/Enable: create job, preflight every required resource, fail with HTTP-422 on any creation-time error.
  - Update: stop old job, recreate with new config.
  - Disable/Remove: stop job, close sockets.
- Creation-time preflight before reporting success:
  - validate job name/path-safe journal identifier;
  - validate endpoint list and protocols;
  - load or acquire the shared profile cache if not already loaded;
  - create/open the per-job journal directory and initialize the writer;
  - validate retention settings;
  - bind every configured endpoint;
  - clean up partial resources on failure.
- Per-job endpoint list bound to configured protocol/address/port tuples — no automatic high-port fallback. EACCES/EADDRINUSE on any configured endpoint makes the job fail with HTTP-422 surfaced in DynCfg (per spec §6); operators choose to grant `CAP_NET_BIND_SERVICE` or reconfigure the endpoint.
- **Job-name validation** at config load: the existing `dyncfg.JobNameRuleStrict` (`src/go/plugin/framework/dyncfg/validate.go`) only rejects whitespace, `:`, and `.` — that is NOT sufficient for the trap plugin because the job name also flows into the per-job journal directory path `/var/cache/netdata/traps/{job_name}/` (path-traversal risk via `/`, `\`, `..`) and into `SYSLOG_IDENTIFIER` (CWE-117 risk via control characters). The trap plugin adds a stricter post-`JobNameRuleStrict` validator at job init: require `^[a-zA-Z0-9][a-zA-Z0-9_-]*$` (alphanumeric start; alphanumeric + `_` + `-` thereafter; max 64 chars; no path separators; no dots). Reject with HTTP-422.
- **HTTP-422 surfacing** for job-init failures: current framework evidence shows gaps in both enable and update paths. `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:88-90` hardcodes `Start()` job-creation failures to HTTP 400, `:93-96` schedules retry and returns a plain error for `Start()` `AutoDetection()` failures, `:108-116` returns plain errors for `Update()` job creation and `AutoDetection()` failures, and `src/go/plugin/framework/dyncfg/handler.go:683` sends update failures as HTTP 200. This SOW must own the framework-side change to preserve `CodedError{code: 422}` for trap creation-time failures and keep existing retry behavior for non-coded collector autodetection errors. Implementation note: this is a small jobmgr + DynCfg handler edit, NOT a trap-plugin-only change.
  - Before changing shared DynCfg behavior, run `rg 'CodedError|codedError|MarkNonDisruptiveUpdate' src/go/plugin` and add handler/jobmgr tests proving plain-error retry behavior remains unchanged while coded trap creation failures surface their HTTP status. Preserve `ErrNonDisruptiveUpdate` rollback as HTTP 200 because the old config remains effective; trap creation-time failures must not use that marker.
- BER decode of v1 + v2c trap PDUs within spec §18 limits (max 8 KiB datagram, 256 varbinds, depth 8, 128-byte OID, 1024-byte OctetString, 1 ms decode budget).
- RFC 3584 v1→v2c conversion (generic-trap + specific-trap → standard varbinds; enterprise OID handling; agent-addr extraction).
- Source identification cascade: valid `snmpTrapAddress.0` varbind → valid v1 `agent-addr` → UDP peer. Malformed or non-IP PDU-provided source values are ignored for identity and `_HOSTNAME` fallback.
- Stock conf shipping default job `local`, disabled, UDP port 162 endpoint (no fallback — operator grants `CAP_NET_BIND_SERVICE` or reconfigures, per spec §6 + §7.5).
- Test corpus: replayable pcap library (Cisco/Juniper/Arista/HP/Aruba/RFC 3584 v1 edge cases); golden-file decode assertions.

Cohort reference: `splunk-sc4snmp.md` §3.4 (listener), `logicmonitor.md` §3.4 (source-ID), `logstash.md` §3 (SNMP4j); gosnmp upstream; `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (job lifecycle).

Reviewers: 3 rotating from `glm`, `kimi`, `mimo`, `minimax`, and `qwen`.

### M3 — Profile loader + OID index + 2-tier varbind resolution + template rendering

- Plugin-wide shared cache, not per-listener state:
  - load on first runnable trap job creation;
  - share the parsed profile index across all listeners;
  - release when no runnable trap jobs remain;
  - any load/validation failure aborts job creation with HTTP-422 and no listener is reported as started.
- Multipath load (operator overrides first, then stock):
  - `/etc/netdata/go.d/snmp.trap-profiles/`
  - `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/`
- Filename-dedup (same filename in higher-priority dir replaces lower-priority entirely).
- Schema validation per `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`:
  - Required: `oid`, `name` (MIB-qualified), `category` (8-slug closed set), `severity` (8-slug closed set).
  - Optional `status` accepted only as `current`, `deprecated`, or `obsolete`; the value is informational in SOW-0035 and does not filter/drop/warn on deprecated or obsolete traps.
  - File-scoped `varbinds:` table; per-trap `varbinds: [name, name, ...]` references.
  - Label key validation: `[a-z][a-z0-9_]*` (syntax only). The `TRAP_TAG_*` namespace structurally prevents collisions with plugin-controlled `TRAP_*` fields, so no reserved-prefix rejection is needed (per spec §7.5 simplification).
  - Label cardinality validation: profile `labels:` templates may reference only bounded-cardinality varbinds; reject unbounded references such as MAC addresses, IPs, usernames, packet contents, or per-event identifiers at profile load with a clear file/trap/label error.
  - Parse + retain `dedup_key_varbinds:` field (used by SOW-0037 dedup; loader stores, does not act on it). **Validate** that every name in `dedup_key_varbinds:` resolves to a varbind entry in the file-scoped `varbinds:` table; reject the trap entry at profile load with a clear error if any reference is dangling.
- Extends-chain field-merge (later `extends:` entries override earlier ones per OID).
- OID index (perfect-hash or radix-trie; choice driven by load-time benchmarks). Supports exact-match for trap OIDs and prefix-match for operator `oid_prefix:` overrides.
- 2-tier varbind resolution: profile inline `varbinds:` table → raw OID fallback. **No runtime MIB compilation tier** (per spec §14 non-goal).
- Description template renderer: `{varname}` (varbind by MIB symbolic name), `{<numeric.oid>}` (varbind by numeric OID fallback), `{ifOperStatus}` (enum substitution), `{ifOperStatus.raw}` (raw numeric value), plus standard journal-field references per spec §7: `{_HOSTNAME}`, `{TRAP_SOURCE_IP}`, `{TRAP_NAME}`, `{TRAP_DEVICE_VENDOR}`, `{TRAP_INTERFACE}`, `{TRAP_NEIGHBORS}`.
- Missing/unresolved handling: `<missing>` for absent varbinds, `<unresolved:varname>` for unrecognized references (`snmp.trap.errors.template_unresolved` counter increment is owned by SOW-0036 M4).
- MESSAGE capped at 512 bytes post-substitution including an ASCII `...` truncation marker.

Cohort reference: `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go` (multipath + filename-dedup + extends-chain merge — the pattern this SOW mirrors); `datadog-agent.md` `dd_traps_db` (file-scoped table pattern); spec §7.

Reviewers: 3 rotating from `glm`, `kimi`, `mimo`, `minimax`, and `qwen`.

### M4 — TrapEntry + TrapWriter + journal writer + CWE-117 + per-job retention

- Internal `TrapEntry` per spec §19 (semantic, backend-agnostic).
- `TrapWriter` interface per spec §19 (`Write`, `Flush`, `Close` — fast queue acceptance, writer-internal batching).
- Journal writer backend producing the per-job field universe per spec §11:
  - One Go writer/backend instance per job, writing to `/var/cache/netdata/traps/{job_name}/`.
  - Journal directory creation/open/writability and writer initialization happen during job creation, before DynCfg apply succeeds.
  - Boot ID and machine ID are read and validated at writer creation; any missing/malformed value is a coded job-creation failure, not a runtime warning.
  - Journal format must match the Rust writer's keyed-hash behavior for new files: SipHash-2-4 for keyed DATA/FIELD hash-table lookups, Jenkins lookup3/hash64 with systemd half-ordering for non-keyed fallback and ENTRY `xor_hash`, with hash-table sizes/header size read from existing files during recovery.
  - Recovery must scan, truncate partial tail objects, rebuild DATA/FIELD hash tables and ENTRY_ARRAY chains, and validate interrupted files with both `journalctl --directory=...` and `journalctl --verify`.
  - Field universe per spec §11: standard systemd fields (`MESSAGE`, `PRIORITY`, `SYSLOG_IDENTIFIER`=job_name, `_HOSTNAME`=source device hostname from enrichment or `SourceIP` fallback, `_MACHINE_ID`=agent/system machine identity exposed by the journal file); existing Netdata fields (`ND_LOG_SOURCE`=snmp-trap, `ND_NIDL_NODE`=source-device vnode); plugin-controlled `TRAP_*` fields (`TRAP_REPORT_TYPE`, `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_PDU_TYPE`, `TRAP_VERSION`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`/`TRAP_NEIGHBORS` may be empty pre-SOW-0037 enrichment, `TRAP_JSON`); profile-defined and operator-defined labels under `TRAP_TAG_*` namespace.
- Per-field text-vs-binary encoding for CWE-117: text-line when ASCII-printable / valid UTF-8 / no newlines / no NULs / no control chars (0x00-0x1F except 0x09/0x20); binary size-prefixed otherwise. `snmp.trap.errors.sanitized` counter increments on binary-encoded fields.
- Per-job retention config mirroring the semantics used by the NetFlow plugin (`src/crates/netflow-plugin/src/plugin_config/types/journal.rs`), with **intentional deviation** on the `max_duration` default (`null` for trap = size-only eviction vs NetFlow's `7d`; rationale per spec §11). The Go implementation may port/reuse those semantics through the journal backend selected in M1. A follow-up SOW (tracked in SOW-0039 Followup) aligns NetFlow's default to match.
  - `retention.max_size` default `10GB`; `retention.max_duration` default `null` (disabled); rotation auto.
  - Both retention thresholds apply independently and inclusively.
- Mock TrapWriter implementation for tests.
- Benchmarks with allocation reporting for `TrapWriter.Write()`, queue drain, and journal `WriteEntry()`; if throughput or allocation behavior misses the tens-of-thousands/sec target, reopen batching/backend design before accepting M4.
- End-to-end test: replay a pcap from M2 corpus through the full pipeline, write to a test journal directory, `journalctl --directory=/var/cache/netdata/traps/test/ TRAP_CATEGORY=security` returns the expected entry.

Cohort reference: existing Netdata systemd-journal writer behavior used by netflow-plugin; spec §11 + §11b + §19.

Reviewers: `glm`, `kimi`, `mimo`, `minimax`, and `qwen` — CWE-117 + field universe are security-critical. The coordinating assistant also performs direct security and integration review.

## Reviewer Protocol

- M1 + M4: `glm`, `kimi`, `mimo`, `minimax`, and `qwen` (consensus / security-critical).
- M2 + M3: 3 rotating reviewers per round drawn from `glm`, `kimi`, `mimo`, `minimax`, and `qwen`.
- Fix-cycle: same reviewers as the round being fixed; iterate until clean per AGENTS.md rerun rule.
- Implementation worker: `deepseek/deepseek-v4-pro` through `opencode run -m deepseek/deepseek-v4-pro --dangerously-skip-permissions --dir .`, without `--agent code-reviewer`.
- Coordination rule: the coordinating assistant owns the final outcome and may directly edit, review, validate, or replace external-agent work when needed.

## Pre-Implementation Gate

Status: passed for activation; M1 begins now.

Problem/root-cause model:

- Netdata needs SNMP traps resolved through the new trap profile pack, but `go.d.plugin` is installed broadly and must not load the large trap profile index for agents that never create trap jobs.
- The user-facing failure boundary must be DynCfg apply/job creation. If a listener cannot bind, profiles cannot load, a journal directory cannot be created/opened, or a writer cannot initialize, reporting the job as started is a product bug.
- The existing go.d DynCfg start path currently risks surfacing some `AutoDetection()` failures as uncoded errors. The SOW must make job-init failures HTTP-422 so the dashboard sees an apply failure.
- The old one-port-per-listener wording was too restrictive. A listener is a policy/writer unit and may own multiple endpoints; multiple listeners are scaling/isolation.

Evidence reviewed:

- `.agents/sow/specs/snmp-traps/netdata.md` §5, §6, §7, §11, §13, §17, §19.
- `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` `Start()` and `Update()` paths: `AutoDetection()` errors are returned without a coded HTTP status today.
- `src/go/plugin/framework/dyncfg/handler.go` enable path: it uses a coded error when available, otherwise falls back to the handler default.
- `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go`: existing SNMP polling profile loader pattern for global cached load, multipath, filename dedup, and profile walking.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` and shipped `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/` pack.
- `src/crates/journal-log-writer/` and NetFlow journal retention config as the reference behavior for direct journal files, rotation, retention, and error surfaces.

Affected contracts and surfaces:

- DynCfg job Add/Enable/Update error semantics for go.d collectors.
- New Go collector/module package for SNMP traps.
- Trap profile loading semantics and validation.
- Per-job endpoint configuration and job-name validation.
- Per-job journal directory/writer contract.
- SNMPv1/v2c decode and RFC 3584 source identity behavior.
- Spec §5, §6, §11, §13, §17, and §19 plus M1 ADR.

Existing patterns to reuse:

- go.d collector registration and job lifecycle patterns.
- `ddsnmp` profile loader multipath and filename-dedup behavior.
- go.d table-driven tests keyed by map names.
- Direct journal writer behavior and retention semantics from the NetFlow journal path, adapted to Go.
- `dyncfg` coded-error pattern for HTTP-422 user-visible apply failures.

Risk and blast radius:

- Framework error-code change can affect other go.d DynCfg-managed jobs; tests must prove only job-init failure status changes and successful starts are unchanged.
- Direct journal writing in Go is high risk for correctness and CWE-117. M1 must either define a very narrow Go-compatible backend or split journal writer work into a safe, testable layer.
- Loading 50,198 trap definitions can consume meaningful memory; profile cache must remain lazy and shared.
- Multi-endpoint binding needs all-or-nothing cleanup to avoid leaked sockets or partially-started jobs.
- SNMP BER parsing is untrusted input; strict decode limits and malformed-PDU tests are required.

Sensitive data handling plan:

- No SNMP communities, v3 auth/priv keys, real customer trap payloads, private endpoints, or production IPs may be written to SOWs, specs, docs, tests, fixtures, prompts, or code comments.
- Test fixtures must be public, synthetic, or sanitized. Any real pcap must have attribution and redaction before becoming durable.
- External-agent prompts must include only repository paths, public spec excerpts, and sanitized requirements.

Implementation plan:

1. M1 ADR: decide Go process/writer backend, shared profile cache lifecycle details, and TrapWriter/TrapEntry contract; update spec §5/§13/§19.
2. M2 code: DynCfg/job preflight, strict job-name validation, multi-endpoint listener binding, SNMPv1/v2c decode, RFC 3584 conversion, source identification, and replay fixtures.
3. M3 code: shared lazy trap profile loader, OID index, validation, varbind resolver, template renderer.
4. M4 code: TrapEntry, TrapWriter, Go-compatible journal writer/backend, CWE-117 encoding, retention semantics, mock writer, and end-to-end replay-to-journal validation.

Validation plan:

- `go test` for affected Go packages, including new trap collector packages and changed DynCfg job manager packages.
- Unit tests for job-name validation, endpoint validation, all-or-nothing bind cleanup, profile cache load/share/release, profile validation failures, OID lookup, template rendering, RFC 3584 v1 conversion, BER limits, source identification, and CWE-117 field encoding.
- End-to-end test: replay pcap or packet fixture through the full pipeline and query the test journal directory with `journalctl --directory=...`.
- Same-failure searches for uncoded job-init failures and unsafe job-name-to-path usage.
- External implementation/review loop: DeepSeek implementation, requested reviewer pool, coordinating assistant review, fixes, repeat until clean.

Artifact impact plan:

- `AGENTS.md`: no expected change unless this SOW discovers a reusable workflow rule gap.
- Runtime project skills: update `.agents/skills/project-snmp-trap-profiles-authoring/` only if profile-loader/profile-format workflow changes.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` and M1 ADR as decisions are finalized.
- End-user/operator docs: deferred to SOW-0039 unless implementation exposes a behavior that must be documented earlier to keep specs accurate.
- End-user/operator skills: deferred to SOW-0039 unless public operator workflow changes before then.
- SOW lifecycle: moved from `pending/` to `current/`, status changed to `in-progress`, external-agent workflow recorded, follow-ups must be mapped before close.

Open decisions:

- M1 accepted the Go-compatible journal writer/backend approach and exact TrapWriter/TrapEntry contract in ADR-0001 after five review attempts, four completed round-5 reviews, and coordinating assistant direct review.
- M1 accepted standard in-process go.d module code with a Go-native journal writer in ADR-0001.
- M2/M4 must prove all creation-time failures are caught before DynCfg apply success.

## Plan

1. M1 (decision + ADR) — produce ADR, run all 7 reviewers, record Go/process/profile-cache/TrapWriter resolution in spec §5 + §13 + §19.
2. M2 — per-job listener + decode + corpus + DynCfg orchestration.
3. M3 — profile loader + resolver + renderer.
4. M4 — TrapWriter + journal writer + retention + end-to-end demo.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup (listener-as-job, per-job retention, no runtime MIB compilation, consistency bundle moved to SOW-0039). Not yet activated.

User clarified the load-bearing SOW-0035 contract: all creation-time failures must surface at DynCfg apply; a listener can own multiple ports/protocols; profiles load lazily on first runnable job creation and are shared across listeners; implementation language is Go, not Rust. SOW and spec updated before implementation.

### 2026-05-25 — M1 Architecture Decision Proposal (ADR-0001)

**ADR created**: `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md` (created as proposed; accepted after round 5 fixes and direct review)

- **Process model**: Standard in-process go.d collector V2 module at `src/go/plugin/go.d/collector/snmp_traps/`, registered as `snmp_traps`. No separate process, no CGo, no subprocess bridge.
- **Journal writer backend**: Go-native write-only systemd journal file writer (~4K-5.5K lines estimated). Write-only subset of the journal binary format — no reader and no cursor, but incremental DATA/FIELD indexes and ENTRY_ARRAY chains are required so active files are queryable. Produces files at `/var/cache/netdata/traps/{job_name}/` queryable via `journalctl --directory=...`.
- **TrapWriter interface**: `Write(entry *TrapEntry) error`, `Flush() error`, `Close() error` — fast bounded-queue acceptance, backend-internal batching. CWE-117 owned by the journal writer backend.
- **TrapEntry shape**: Go struct with all spec §19 fields, string enums for ReportType/PduType/SnmpVersion, receive timestamps, and `SummaryCounts` for dedup summaries.
- **Shared profile cache**: Go package-level state (`sync.Mutex` + refcount). First `AcquireProfileCache()` loads profiles; last `ReleaseProfileCache()` drops them. Agents with no trap jobs never pay the memory.

**Spec updated**:
- `.agents/sow/specs/snmp-traps/netdata.md` §5: Language and process model paragraph updated with proposed resolution and rationale. Journal writer backend proposal recorded.
- `.agents/sow/specs/snmp-traps/netdata.md` §13: Open Question item 1 marked PROPOSED with ADR-0001 reference.
- `.agents/sow/specs/snmp-traps/netdata.md` §19: Status header added with key decisions summary. Interface contract reference updated to point to the actual ADR.

**Alternatives rejected** (recorded in ADR):
- Separate Go process (PLUGINSD): adds IPC for cross-plugin enrichment, profile sharing complexity
- CGo bridge to libsystemd: cannot set `_HOSTNAME` (journald owns trusted fields), adds CGo to pure-Go go.d
- Subprocess Rust bridge: process management complexity, build dependency on Rust toolchain
- Full Rust crate port to Go: ~11K lines of Rust; unnecessary for write-only needs

**Remaining for M1 close**:
- External reviewer re-review consensus (glm, kimi, mimo, minimax, qwen) on ADR-0001 after round-1 fixes
- Coordinating assistant direct review pass
- `audit.sh` clean run
- `git diff --check` clean

### 2026-05-25 — M1 review round 1 fixes

Reviewer round 1 found real design gaps in ADR-0001. Four reviewers completed; one reviewer repeated the same read-only investigation without converging, so the coordinating assistant stopped that specific reviewer process and used only its partial evidence as non-final signal.

**Fixes folded into ADR/spec before re-review**:

- Corrected journal writer estimate from ~1.5-2K lines to ~4K-5.5K lines, based on current Rust write-path evidence including ENTRY_ARRAY support.
- Required incremental DATA/FIELD hash tables and ENTRY_ARRAY chains so active journal files are queryable before rotation/close.
- Added receive timestamps to `TrapEntry` (`ReceivedRealtimeUsec`, `ReceivedMonotonicUsec`) so journal and OTLP backends do not invent write-time timestamps.
- Changed `TrapWriter.Write()` semantics from blocking disk write to fast bounded-queue acceptance; runtime write failure increments `journal_write_failed` and the hot path continues.
- Removed `sync.Once` from the shared profile cache design; failed loads must be retryable and the cache must support release/re-acquire.
- Rewrote DynCfg failure handling requirements to cover `Start()` and `Update()`, `createCollectorJob()` and `AutoDetection()`, and the current HTTP-400/HTTP-200 framework behavior.
- Aligned `SummaryCounts`, removed initial `DisplayHint` emission, and required canonical varbind serialization.
- Updated the operator profile-format documentation to say profiles load on first runnable trap job creation, not permanent plugin enablement.

### 2026-05-25 — M1 review round 2 fixes

Reviewer round 2 confirmed the core M1 architecture and found remaining specification gaps that could create implementation ambiguity.

**Fixes folded into ADR/spec before the next review**:

- Corrected Rust source line counts and raised the Go journal writer estimate to ~4K-5.5K lines.
- Replaced ambiguous `[][]byte` journal fields with explicit `JournalField{Name, Value}` and named the `serializeToJournalFields()` layer.
- Chose regular `write`/`pwrite`/`fdatasync` file I/O over mmap for the Go writer to keep random-access updates explicit, avoid mmap lifetime/page-fault failure modes in `go.d.plugin`, and avoid CGo `msync`.
- Added journal format details: hash bucket counts, hash behavior, header state transitions, `seqnum_id`, `tail_entry_boot_id`, crash recovery, and active-file tests.
- Added queue capacity, drop semantics, flush cadence, `Flush()` blocking behavior, post-`Close()` write behavior, and Linux-only backend behavior.
- Tightened `TrapWriter.Write()` ownership semantics on success vs error.
- Required optional enrichment fields to be omitted when empty, not emitted as empty journal/OTLP fields.
- Added an explicit allowed concrete type set for `VarbindValue.Value`.
- Documented immutable `Labels` / `SummaryCounts.ByTrap` map ownership and dedup map cloning.
- Documented profile-cache acquire/release locking, failed-load retry, underflow recovery, and test reset helper.
- Required trap job creation rollback to release profile-cache refs and close partial bind/writer resources because framework `Cleanup()` is not called when `createCollectorJob()` fails.
- Added M3 label cardinality validation to match `profile-format.md`.

### 2026-05-25 — M1 review round 3 fixes

Reviewer round 3 again confirmed the core architecture. Three reviewers completed with useful findings; two stalled without producing final review output, so the coordinating assistant stopped only those specific reviewer processes and treated their partial/no output as non-evidence.

**Fixes folded into ADR/spec before the next review**:

- Replaced the fixed-header wording with explicit `header_size` handling and recovery-time reading of actual file header/hash-table sizes.
- Corrected the hash contract: keyed DATA/FIELD lookups use SipHash-2-4 with file `file_id`; non-keyed fallback and ENTRY `xor_hash` use Jenkins lookup3/hash64.
- Made boot ID and machine ID writer-creation preflight requirements; missing/malformed IDs are coded job-creation failures.
- Tightened `Flush()` and `Close()` concurrency/error semantics, including stored terminal close errors.
- Clarified `TrapEntry` ownership: map/slice reuse after successful `Write()` is a correctness bug unless deep-copied first.
- Kept `ErrNonDisruptiveUpdate` rollback as HTTP 200 and required coded status handling only for the non-rollback `CmdUpdate` failure path.
- Added the DynCfg same-failure scan/test requirement before shared framework changes.
- Added M4 throughput/allocation benchmark gates for `TrapWriter.Write()`, queue drain, and journal `WriteEntry()`.

### 2026-05-25 — M1 review round 4 fixes

Reviewer round 4 confirmed the architecture and found implementation-ambiguity gaps rather than architecture blockers. Four reviewers completed with useful findings; one stalled after investigation output and was stopped by the coordinating assistant.

**Fixes folded into ADR/spec before the next review**:

- Locked the go.d module path and registration name to `snmp_traps`, following the existing `snmp_topology` style and avoiding dotted names.
- Split journal hash wording into DATA/FIELD keyed SipHash, DATA/FIELD non-keyed Jenkins, and ENTRY `xor_hash` always-Jenkins cases, including the systemd/Rust 32-bit half-ordering warning.
- Made `file_id`, `seqnum_id`, and per-entry sequence numbers explicit internal `JournalWriter` state.
- Specified that `JournalWriter.WriteEntry()` is single-worker-only and not concurrency-safe; concurrency is at the `TrapWriter` queue.
- Added regular-I/O ENTRY_ARRAY rewrite requirements, initial capacity/doubling behavior, recent-DATA cache guidance, and crash-recovery scan/truncate/rebuild algorithm.
- Defined profile cache generation as an explicit `uint64` returned by acquire and passed to release.
- Clarified `Update()` failure limits: the trap factory rolls back only failed-new-job resources; the old job is already stopped by current framework behavior.
- Renamed the semantic `TrapEntry` vnode field to `SourceVnodeID`.
- Clarified config parsing, rotation-without-retention behavior, `_HOSTNAME` fallback without DNS, timestamp non-negative validation, dedup summary OID-keyed `ByTrap`, ASCII truncation marker, reserved `decode_error_summary`, and `profile-format.md` `status` semantics.

### 2026-05-25 — M1 review round 5 fixes

Reviewer round 5 confirmed the architecture with four completed reviews. One reviewer stalled after file traversal and was stopped by the coordinating assistant; its partial output is treated as non-final signal only.

**Fixes folded into ADR/spec before accepting M1**:

- Tightened the DynCfg framework-change contract: creation-time `CodedError` values from trap job creation must be preserved through `Start()` and `Update()` instead of being wrapped as HTTP 400 or returned as HTTP 200.
- Reworked profile-cache generation from "ignore stale release" to exact per-generation holder accounting, so future hot reload can retire old profile indexes without leaking refs while active jobs still use them.
- Added source-IP validation to the source identification cascade; malformed PDU-provided addresses fall back to the kernel-provided UDP peer.
- Moved the exact trap job-name regex and 64-character cap into the main spec, near the per-job filesystem layout.
- Clarified that `serializeToJournalFields()` runs only on the single journal writer worker, not on the decode hot path.
- Clarified `entry_seqnum` vs `seqnum_id`, Linux positioned-I/O implementation expectations, duplicate `TRAP_JSON` key suffixing, `dedup_key_varbinds` missing-value sentinel behavior, and retention behavior when both cleanup thresholds are explicitly disabled.
- Added profile-format validation notes for `status` and dangling `dedup_key_varbinds`.
- Rejected weakening `journalctl --verify` to best-effort: SOW-0035 needs hard journal-file compatibility evidence before accepting the Go writer.

## Validation

Acceptance criteria evidence: M1 accepted in ADR-0001 after round-5 fixes and direct review; M2-M4 implementation evidence pending.
Tests or equivalent validation: `git diff --check` and `.agents/sow/audit.sh` must pass for M1 artifact changes before implementation commit; M2-M4 code tests pending.
Real-use evidence: pending (M4 end-to-end pcap replay is the primary real-use signal).
Reviewer findings: M1 review rounds 1-5 recorded above; four round-5 reviewers completed with no architecture blocker, one stalled and was stopped after partial read-only investigation.
Same-failure scan: M1 review verified current DynCfg source evidence; M2 implementation must run the required `rg 'CodedError|codedError|MarkNonDisruptiveUpdate' src/go/plugin` scan before editing shared framework behavior.
Sensitive data gate:

Pending — no SNMP communities, USM keys, or operator-secret data in any committed artifact. External-agent prompts and generated artifacts must stay sanitized.

Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
