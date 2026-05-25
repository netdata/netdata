# SOW-0035 - SNMP Trap Plugin Foundation + MVP Slice (multi-endpoint listener, shared profile loader, journal writer)

## Status

Status: open

Sub-state: queued in `.agents/sow/pending/`. The user has fixed the implementation language to Go and clarified the listener/profile lifecycle contract. Activation still requires the remaining M1 architecture decisions (process model, journal-writer backend, TrapWriter interface contract) to be made and reviewed; the full Pre-Implementation Gate is filled at activation so it reflects the freshest code/spec state.

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

Reviewers: all 7 (glm, mimo, kimi, qwen, minimax, deepseek, codex) — consensus-critical.

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
- **HTTP-422 surfacing** for job-init failures: the existing collector `Start()` callback at `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:93` returns plain `fmt.Errorf(...)` for `AutoDetection()` failures (no `Code()` wrapper), and the framework's `handler.go:501` defaults non-coded errors to HTTP-200 — meaning bind failures would silently appear as success in the DynCfg UI. This SOW must own a framework-side change: extend `dyncfg_collector_callbacks.go` to wrap `AutoDetection()` errors as `codedError{code: 422}` (or introduce a typed `BindError` that the framework recognizes). Implementation note: this is a small jobmgr edit, NOT a trap-plugin-only change.
- BER decode of v1 + v2c trap PDUs within spec §18 limits (max 8 KiB datagram, 256 varbinds, depth 8, 128-byte OID, 1024-byte OctetString, 1 ms decode budget).
- RFC 3584 v1→v2c conversion (generic-trap + specific-trap → standard varbinds; enterprise OID handling; agent-addr extraction).
- Source identification cascade: `snmpTrapAddress.0` varbind → v1 `agent-addr` → UDP peer.
- Stock conf shipping default job `local`, disabled, UDP port 162 endpoint (no fallback — operator grants `CAP_NET_BIND_SERVICE` or reconfigures, per spec §6 + §7.5).
- Test corpus: replayable pcap library (Cisco/Juniper/Arista/HP/Aruba/RFC 3584 v1 edge cases); golden-file decode assertions.

Cohort reference: `splunk-sc4snmp.md` §3.4 (listener), `logicmonitor.md` §3.4 (source-ID), `logstash.md` §3 (SNMP4j); gosnmp upstream; `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (job lifecycle).

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

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
  - File-scoped `varbinds:` table; per-trap `varbinds: [name, name, ...]` references.
  - Label key validation: `[a-z][a-z0-9_]*` (syntax only). The `TRAP_TAG_*` namespace structurally prevents collisions with plugin-controlled `TRAP_*` fields, so no reserved-prefix rejection is needed (per spec §7.5 simplification).
  - Parse + retain `dedup_key_varbinds:` field (used by SOW-0037 dedup; loader stores, does not act on it). **Validate** that every name in `dedup_key_varbinds:` resolves to a varbind entry in the file-scoped `varbinds:` table; reject the trap entry at profile load with a clear error if any reference is dangling.
- Extends-chain field-merge (later `extends:` entries override earlier ones per OID).
- OID index (perfect-hash or radix-trie; choice driven by load-time benchmarks). Supports exact-match for trap OIDs and prefix-match for operator `oid_prefix:` overrides.
- 2-tier varbind resolution: profile inline `varbinds:` table → raw OID fallback. **No runtime MIB compilation tier** (per spec §14 non-goal).
- Description template renderer: `{varname}` (varbind by MIB symbolic name), `{<numeric.oid>}` (varbind by numeric OID fallback), `{ifOperStatus}` (enum substitution), `{ifOperStatus.raw}` (raw numeric value), plus standard journal-field references per spec §7: `{_HOSTNAME}`, `{TRAP_SOURCE_IP}`, `{TRAP_NAME}`, `{TRAP_DEVICE_VENDOR}`, `{TRAP_INTERFACE}`, `{TRAP_NEIGHBORS}`.
- Missing/unresolved handling: `<missing>` for absent varbinds, `<unresolved:varname>` for unrecognized references (`snmp.trap.errors.template_unresolved` counter increment is owned by SOW-0036 M4).
- MESSAGE capped at 512 bytes post-substitution; truncated with `…` marker.

Cohort reference: `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go` (multipath + filename-dedup + extends-chain merge — the pattern this SOW mirrors); `datadog-agent.md` `dd_traps_db` (file-scoped table pattern); spec §7.

Reviewers: 3 rotating (group B: glm/mimo/deepseek).

### M4 — TrapEntry + TrapWriter + journal writer + CWE-117 + per-job retention

- Internal `TrapEntry` per spec §19 (semantic, backend-agnostic).
- `TrapWriter` interface per spec §19 (`Write`, `Flush`, `Close` — synchronous, writer-internal batching).
- Journal writer backend producing the per-job field universe per spec §11:
  - One Go writer/backend instance per job, writing to `/var/cache/netdata/traps/{job_name}/`.
  - Journal directory creation/open/writability and writer initialization happen during job creation, before DynCfg apply succeeds.
  - Field universe per spec §11: standard systemd fields (`MESSAGE`, `PRIORITY`, `SYSLOG_IDENTIFIER`=job_name, `_HOSTNAME`=source device hostname); existing Netdata fields (`ND_LOG_SOURCE`=snmp-trap, `ND_NIDL_NODE`=source-device vnode); plugin-controlled `TRAP_*` fields (`TRAP_REPORT_TYPE`, `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_PDU_TYPE`, `TRAP_VERSION`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`/`TRAP_NEIGHBORS` may be empty pre-SOW-0037 enrichment, `TRAP_JSON`); profile-defined and operator-defined labels under `TRAP_TAG_*` namespace.
- Per-field text-vs-binary encoding for CWE-117: text-line when ASCII-printable / valid UTF-8 / no newlines / no NULs / no control chars (0x00-0x1F except 0x09/0x20); binary size-prefixed otherwise. `snmp.trap.errors.sanitized` counter increments on binary-encoded fields.
- Per-job retention config mirroring the semantics used by the NetFlow plugin (`src/crates/netflow-plugin/src/plugin_config/types/journal.rs`), with **intentional deviation** on the `max_duration` default (`null` for trap = size-only eviction vs NetFlow's `7d`; rationale per spec §11). The Go implementation may port/reuse those semantics through the journal backend selected in M1. A follow-up SOW (tracked in SOW-0039 Followup) aligns NetFlow's default to match.
  - `retention.max_size` default `10GB`; `retention.max_duration` default `null` (disabled); rotation auto.
  - Both retention thresholds apply independently and inclusively.
- Mock TrapWriter implementation for tests.
- End-to-end test: replay a pcap from M2 corpus through the full pipeline, write to a test journal directory, `journalctl --directory=/var/cache/netdata/traps/test/ TRAP_CATEGORY=security` returns the expected entry.

Cohort reference: existing Netdata systemd-journal writer behavior used by netflow-plugin; spec §11 + §11b + §19.

Reviewers: all 7 — CWE-117 + field universe are security-critical.

## Reviewer Protocol

- M1 + M4: all 7 reviewers (consensus / security-critical).
- M2 + M3: 3 rotating reviewers per round (groups A/B).
- Fix-cycle: same reviewers as the round being fixed; iterate until clean per AGENTS.md rerun rule.

## Pre-Implementation Gate

Status: pending-activation

This SOW remains in `pending/` until M1 is approved by reviewer consensus on process model, journal-writer backend, shared profile cache lifecycle details, and TrapWriter interface. The user has already decided the implementation language is Go. The full gate is filled at activation — deferred deliberately so the gate captures the freshest code/spec state at start-of-work rather than at SOW authoring.

Open decisions for M1:

1. **Process model / writer backend** — standard in-process go.d module code with a Go journal writer/port vs any justified process boundary or bridge.
2. **Cross-plugin state access** — in-process struct sharing if the Go implementation stays in `go.d.plugin`; otherwise a justified IPC mechanism.
3. **TrapWriter contract** — method signatures, `TrapEntry` fields, batching, flush, close, and error semantics.

## Plan

1. M1 (decision + ADR) — produce ADR, run all 7 reviewers, record Go/process/profile-cache/TrapWriter resolution in spec §5 + §13 + §19.
2. M2 — per-job listener + decode + corpus + DynCfg orchestration.
3. M3 — profile loader + resolver + renderer.
4. M4 — TrapWriter + journal writer + retention + end-to-end demo.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup (listener-as-job, per-job retention, no runtime MIB compilation, consistency bundle moved to SOW-0039). Not yet activated.

User clarified the load-bearing SOW-0035 contract: all creation-time failures must surface at DynCfg apply; a listener can own multiple ports/protocols; profiles load lazily on first runnable job creation and are shared across listeners; implementation language is Go, not Rust. SOW and spec updated before implementation.

## Validation

Acceptance criteria evidence: pending.
Tests or equivalent validation: pending.
Real-use evidence: pending (M4 end-to-end pcap replay is the primary real-use signal).
Reviewer findings: pending.
Same-failure scan: pending.
Sensitive data gate: pending — no SNMP communities, USM keys, or operator-secret data in any committed artifact.
Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
