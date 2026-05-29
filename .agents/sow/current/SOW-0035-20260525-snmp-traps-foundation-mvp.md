# SOW-0035 - SNMP Trap Plugin Foundation + MVP Slice (multi-endpoint listener, shared profile loader, journal writer)

## Status

Status: paused

Sub-state: activated on 2026-05-25. Implementation and validation for the SOW-0035 feature-branch slice are complete and pushed in commit `38964171435f`. Terminal completion is intentionally deferred to SOW-0039 because SOW-0035 through SOW-0039 are not independently mergeable and close together at the final collector-consistency and merge gate. Reopened on 2026-05-27 for a test-isolation repair after local runtime inspection found trap test artifacts under `/var/cache/netdata/traps/`; the repair is implemented and validated, and the SOW is paused again pending SOW-0039. The user has fixed the implementation language to Go and clarified the listener/profile lifecycle contract. Implementation is delegated primarily to `deepseek/deepseek-v4-pro`; remaining reviews are delegated to `glm`, `kimi`, `minimax`, and `qwen` after `mimo` was removed from the reviewer pool due to quota exhaustion. The coordinating assistant remains responsible for architecture, direct edits, unblocking, integration, validation, and final quality.

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
- Reviews initially used `glm`, `kimi`, `mimo`, `minimax`, and `qwen`; on 2026-05-25 the user removed `mimo` from remaining review runs because it is out of quota.
- Every remaining external assistant invocation must close stdin with `</dev/null` so non-interactive questions cannot stall the run.
- The coordinating assistant may make direct edits, perform reviews, and unblock external workers when that improves quality or progress. External models are helpers, not autopilot.

User journal-backend decision recorded on 2026-05-25:

- The long-term journal writer backend should come from the adjacent `systemd-journal-sdk` workspace, where another worker is preparing the journal SDK writer, FSS support, and retention management.
- The custom package-local Go journal writer currently in this branch is provisional and should not be further expanded as the permanent backend.
- SOW35 pivots to an SDK-backed adapter now if the SDK Go writer API is usable; otherwise M4 journal-backend work pauses until the SDK is ready.
- The SNMP trap subsystem must keep the `TrapWriter` abstraction, creation-time writer/directory/profile/listener preflight, lazy shared profile loading, and end-to-end `journalctl --directory` validation so the backend can be re-vendored when the SDK stabilizes.

User ingestion decision recorded on 2026-05-26:

- Pending SOW-0041 (`SOW-0041-20260525-snmp-traps-smiv1-smiv2-oid-tolerance.md`) is folded into this SOW's M3 ingestion/profile lookup scope instead of being implemented as a separate SOW.
- The receiver must tolerate the SMIv1 / SMIv2 trap-OID `.0.` ambiguity because generated profile YAMLs may contain either `enterprise.0.specific` or `enterprise.specific`, while real devices send only one form.
- Exact trap-OID match wins. On primary miss, `ProfileIndex.Lookup` tries one deterministic alternate key by adding or removing a single `.0.` segment immediately before the final OID arc.
- The tolerance is unconditional, implemented in the profile-index lookup path, and applies only to trap OID lookup. Varbind OID resolution remains exact because varbind OIDs do not have this SMIv1 trap encoding ambiguity.
- A full shipped-pack audit/regeneration remains out of scope for SOW-0035; the receiver-side fix absorbs the runtime ingestion failure class.

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
- M3: shared lazy profile YAML loader (multipath, filename-dedup, extends-chain merge) loaded on first runnable job creation and shared across listeners; OID index with exact-match-first SMIv1 / SMIv2 `.0.` trap-OID tolerance on miss; 2-tier varbind resolution (profile → raw, NO runtime MIB compilation per §14); template renderer producing MESSAGE + `TRAP_TAG_*` labels per §7 syntax (operator/profile labels emit as `TRAP_TAG_<KEY>`, NOT `TRAP_*` which is reserved for plugin-controlled fields).
- M4: SDK-backed per-job journal writer producing the spec §11 field universe under configured root `/var/cache/netdata/traps/{job_name}/` and effective SDK machine-id directory `/var/cache/netdata/traps/{job_name}/{machine_id}/`; per-field CWE-117 sanitization counting; per-job retention config mirroring the NetFlow plugin retention semantics with **intentional deviation** on `retention.max_duration` default (`null` for the trap plugin vs `7d` for NetFlow — rationale in spec §11); end-to-end test (replay pcap from M2 corpus → entry visible via `journalctl --directory=<effective-journal-dir> TRAP_CATEGORY=security`). Reviewer consensus across all 7 reviewers.

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

Reviewers: `glm`, `kimi`, `minimax`, and `qwen` for remaining rounds after `mimo` quota exhaustion — consensus-critical. The coordinating assistant also performs its own review and may make direct edits before/after external review.

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
- BER decode of v1 + v2c trap PDUs within spec §18 limits (max 8 KiB datagram, 256 varbinds, constructed BER depth 8, 128-byte OID, 1024-byte OctetString, 1 ms decode budget).
- RFC 3584 v1→v2c conversion (generic-trap + specific-trap → standard varbinds; enterprise OID handling; agent-addr extraction).
- Source identification cascade: valid `snmpTrapAddress.0` varbind → valid v1 `agent-addr` → UDP peer. Malformed or non-IP PDU-provided source values are ignored for identity and `_HOSTNAME` fallback.
- Stock conf shipping default job `local`, disabled, UDP port 162 endpoint (no fallback — operator grants `CAP_NET_BIND_SERVICE` or reconfigures, per spec §6 + §7.5).
- Test corpus: replayable pcap library (Cisco/Juniper/Arista/HP/Aruba/RFC 3584 v1 edge cases); golden-file decode assertions.

Cohort reference: `splunk-sc4snmp.md` §3.4 (listener), `logicmonitor.md` §3.4 (source-ID), `logstash.md` §3 (SNMP4j); gosnmp upstream; `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` (job lifecycle).

Reviewers: 3 or more rotating from `glm`, `kimi`, `minimax`, and `qwen` for remaining rounds.

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
  - Optional `status` accepted as `current`, `deprecated`, `mandatory`, `obsolete`, or `optional`; the value is informational in SOW-0035 and does not filter/drop/warn on non-current traps.
  - File-scoped `varbinds:` table; per-trap `varbinds: [name, name, ...]` references.
  - Label key validation: `[a-z][a-z0-9_]*` (syntax only). The `TRAP_TAG_*` namespace structurally prevents collisions with plugin-controlled `TRAP_*` fields, so no reserved-prefix rejection is needed (per spec §7.5 simplification).
  - Label cardinality validation: profile `labels:` templates may reference only bounded-cardinality varbinds; reject unbounded references such as MAC addresses, IPs, usernames, packet contents, or per-event identifiers at profile load with a clear file/trap/label error.
  - Parse + retain `dedup_key_varbinds:` field (used by SOW-0037 dedup; loader stores, does not act on it). **Validate** that every name in `dedup_key_varbinds:` resolves to a varbind entry in the file-scoped `varbinds:` table; reject the trap entry at profile load with a clear error if any reference is dangling.
- Extends-chain field-merge (later `extends:` entries override earlier ones per OID).
- OID index exact-match for profile trap OIDs, then one SMIv1 / SMIv2 `.0.` alternate lookup on primary miss:
  - decoded `enterprise.0.specific` may match a profile entry stored as `enterprise.specific`;
  - decoded `enterprise.specific` may match a profile entry stored as `enterprise.0.specific`;
  - exact matches take precedence when both forms exist;
  - degenerate/too-short OIDs and true misses still return no profile match;
  - the fallback is trap-OID-only and must not be applied to varbind resolution.
  Operator `oid_prefix:` override matching belongs with the override config surface in SOW-0036.
- 2-tier varbind resolution: profile inline `varbinds:` table → raw OID fallback. **No runtime MIB compilation tier** (per spec §14 non-goal).
- Description template renderer: `{varname}` (varbind by MIB symbolic name), `{<numeric.oid>}` (varbind by numeric OID fallback), `{ifOperStatus}` (enum substitution), `{ifOperStatus.raw}` (raw numeric value), plus standard journal-field references per spec §7: `{_HOSTNAME}`, `{TRAP_SOURCE_IP}`, `{TRAP_NAME}`, `{TRAP_DEVICE_VENDOR}`, `{TRAP_INTERFACE}`, `{TRAP_NEIGHBORS}`.
- Missing/unresolved handling: `<missing>` for absent varbinds, `<unresolved:varname>` for unrecognized references (`snmp.trap.errors.template_unresolved` counter increment is owned by SOW-0036 M4).
- MESSAGE capped at 512 bytes post-substitution including an ASCII `...` truncation marker.

Cohort reference: `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go` (multipath + filename-dedup + extends-chain merge — the pattern this SOW mirrors); `datadog-agent.md` `dd_traps_db` (file-scoped table pattern); spec §7.

Reviewers: 3 or more rotating from `glm`, `kimi`, `minimax`, and `qwen` for remaining rounds.

### M4 — TrapEntry + TrapWriter + journal writer + CWE-117 + per-job retention

- Internal `TrapEntry` per spec §19 (semantic, backend-agnostic).
- `TrapWriter` interface per spec §19 (`Write`, `Flush`, `Close` — fast queue acceptance, writer-internal batching).
- SDK-backed journal writer backend producing the per-job field universe per spec §11:
  - One Go writer/backend adapter per job, configured with root `/var/cache/netdata/traps/{job_name}/`; SDK appends the machine-id child directory, so `journalctl --directory` validation uses `/var/cache/netdata/traps/{job_name}/{machine_id}/`.
  - Journal directory creation/open/writability and writer initialization happen during job creation, before DynCfg apply succeeds.
  - Boot ID and machine ID are read and validated at writer creation; any missing/malformed value is a coded job-creation failure, not a runtime warning.
  - Journal format, writer lock acquisition, rotation, retention, active-file indexing, and existing-chain validation/reopen are delegated to `github.com/netdata/systemd-journal-sdk/go/journal` `go/v0.3.0` via `journal.NewLog`.
  - The adapter uses `LogOpenEager` and `LogIdentityStrict` so resource and identity failures are detected at job creation time.
  - Field universe per spec §11: standard systemd fields (`MESSAGE`, `PRIORITY`, `SYSLOG_IDENTIFIER`=job_name, `_HOSTNAME`=source device hostname from enrichment or `SourceIP` fallback, `_MACHINE_ID`=agent/system machine identity exposed by the journal file); existing Netdata fields (`ND_LOG_SOURCE`=snmp-trap, `ND_NIDL_NODE`=source-device vnode); plugin-controlled `TRAP_*` fields (`TRAP_REPORT_TYPE`, `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_PDU_TYPE`, `TRAP_VERSION`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`/`TRAP_NEIGHBORS` may be empty pre-SOW-0037 enrichment, `TRAP_JSON`); profile-defined and operator-defined labels under `TRAP_TAG_*` namespace.
- Per-field CWE-117 accounting: fields containing newlines, NUL, DEL, unsafe control bytes, or invalid UTF-8 increment the future `snmp.trap.errors.sanitized` counter. The SDK stores field values as journal DATA objects, so embedded newlines cannot inject additional journal fields.
- Per-job retention config mirroring the semantics used by the NetFlow plugin (`src/crates/netflow-plugin/src/plugin_config/types/journal.rs`), with **intentional deviation** on the `max_duration` default (`null` for trap = size-only eviction vs NetFlow's `7d`; rationale per spec §11). The Go implementation may port/reuse those semantics through the journal backend selected in M1. A follow-up SOW (tracked in SOW-0039 Followup) aligns NetFlow's default to match.
  - `retention.max_size` default `10GB`; `retention.max_duration` default `null` (disabled); rotation auto.
  - Both retention thresholds apply independently and inclusively.
- Mock TrapWriter implementation for tests.
- Benchmarks with allocation reporting for `TrapWriter.Write()`, queue drain, and SDK-backed journal `WriteEntry()`; if throughput or allocation behavior misses the tens-of-thousands/sec target, reopen batching/backend design before accepting M4.
- End-to-end test: replay a pcap from M2 corpus through the full pipeline, write to a test journal directory, `journalctl --directory=/var/cache/netdata/traps/test/ TRAP_CATEGORY=security` returns the expected entry.

Cohort reference: existing Netdata systemd-journal writer behavior used by netflow-plugin; spec §11 + §11b + §19.

Reviewers: `glm`, `kimi`, `minimax`, and `qwen` for remaining rounds — CWE-117 + field universe are security-critical. The coordinating assistant also performs direct security and integration review.

## Reviewer Protocol

- M1 + M4: `glm`, `kimi`, `minimax`, and `qwen` for remaining rounds after `mimo` quota exhaustion (consensus / security-critical).
- M2 + M3: 3 or more rotating reviewers per round drawn from `glm`, `kimi`, `minimax`, and `qwen` for remaining rounds after `mimo` quota exhaustion.
- Fix-cycle: same reviewers as the round being fixed; iterate until clean per AGENTS.md rerun rule.
- Implementation worker: `deepseek/deepseek-v4-pro` through `opencode run -m deepseek/deepseek-v4-pro --dangerously-skip-permissions --dir .`, without `--agent code-reviewer`.
- External assistant process rule: run with stdin closed via `</dev/null`.
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
- Trap profile lookup semantics, including exact-match-first SMIv1 / SMIv2 `.0.` tolerance for trap OIDs only.
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
- Unit tests for SMIv1 / SMIv2 trap-OID tolerance: with-`.0.` decoded OID matching without-`.0.` profile entry, reverse case, exact-match precedence when both forms exist, degenerate inputs, and true miss.
- End-to-end test: replay pcap or packet fixture through the full pipeline and query the test journal directory with `journalctl --directory=...`.
- Same-failure searches for uncoded job-init failures and unsafe job-name-to-path usage.
- External implementation/review loop: DeepSeek implementation, requested reviewer pool, coordinating assistant review, fixes, repeat until clean.

Artifact impact plan:

- `AGENTS.md`: no expected change unless this SOW discovers a reusable workflow rule gap.
- Runtime project skills: update `.agents/skills/project-snmp-trap-profiles-authoring/` for the trap-OID `.0.` tolerance authoring rule.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` and M1 ADR as decisions are finalized.
- End-user/operator docs: update `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` for the trap-OID `.0.` tolerance behavior. Other user-facing docs remain deferred to SOW-0039 unless implementation exposes a behavior that must be documented earlier to keep specs accurate.
- End-user/operator skills: deferred to SOW-0039 unless public operator workflow changes before then.
- SOW lifecycle: moved from `pending/` to `current/`, status changed to `in-progress`, external-agent workflow recorded, follow-ups must be mapped before close.

Open decisions:

- M1 accepted the Go-compatible journal writer/backend approach and exact TrapWriter/TrapEntry contract in ADR-0001 after five review attempts, four completed round-5 reviews, and coordinating assistant direct review.
- The 2026-05-25 Go-native journal writer decision was amended on 2026-05-26 after the SDK published `go/v0.1.0`: SOW-0035 now uses standard in-process go.d module code with a thin SDK-backed Go journal adapter.
- M2/M4 must prove all creation-time failures are caught before DynCfg apply success.
- SOW-0041 was consolidated into this SOW on 2026-05-26 by user direction. The decisions are: helper/logic in the profile lookup path, unconditional tolerance, trap-OID-only scope, and pack audit/regeneration deferred.

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

### 2026-05-25 — M2 Implementation

**Status**: M2 implementation completed and review-round fixes applied (uncommitted working tree).

**M2.1 DynCfg/jobmgr creation-time failure surfacing**

Framework changes scoped to `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go` and `src/go/plugin/framework/dyncfg/handler.go`:

- `Start()` now preserves inner `dyncfg.CodedError` from `createCollectorJob` failures instead of always wrapping as HTTP 400. Non-coded errors keep the existing 400 wrapper.
- `Start()` now checks `AutoDetection` errors for the public `dyncfg.CodedError` interface. Coded AutoDetection failures return directly (no retry scheduled). Non-coded AutoDetection failures keep the existing retry behavior.
- `Update()` mirrors `Start()` for both `createCollectorJob` and `AutoDetection` coded error handling.
- `CmdUpdate` at `handler.go:683` honors `CodedError` response codes from `cb.Update()`/`cb.Start()` failures, falling back to 200 for plain errors. The `ErrNonDisruptiveUpdate` rollback path at `handler.go:667-677` remains HTTP 200.
- `job_factory.go`: Added optional `SetJobName(string)` interface check for both V1 and V2 module creators, so collectors can validate the job name before Init.

Tests added:
- `TestCmdUpdate_NonConversion_StartFails_CodedError` — verifies CodedError from Update path surfaces HTTP 422 in the DynCfg response
- `TestCmdUpdate_Conversion_StartFails_CodedError` — verifies CodedError from Start (conversion) path surfaces HTTP 422 in the DynCfg response
- `TestCollectorCallbacks_Start_AutodetectionCodedError_PreservedNoRetry` — verifies a non-jobmgr `dyncfg.CodedError` returns directly, no retry
- `TestCollectorCallbacks_Start_InitCodedError_PreservedNoRetry` — verifies coded Init/preflight failure returns directly, no retry
- `TestCollectorCallbacks_Update_AutodetectionCodedError_PreservedNoRetry` — same for Update path
- `TestCollectorCallbacks_Update_InitCodedError_PreservedNoRetry` — same for Update path Init/preflight failure
- `TestCollectorCallbacks_Update_CreateCollectorJobPlainErrorPreserved` — verifies plain create-job errors in the Update path remain uncoded, preserving existing handler fallback behavior.

Existing tests verified non-regression: `TestCollectorCallbacks_Start`, `TestCollectorCallbacks_Update`, `TestCmdUpdate_NonConversion_StartFails`, `TestCmdUpdate_NonConversion_StartFails_NonDisruptiveRollback`, `TestCmdUpdate_Conversion_StartFails`, `TestCmdEnable_StartFails_RegularError`, `TestCmdEnable_StartFails_CodedError`, `TestCmdRestart_StartFails_CodedError`.

**M2.2 Go collector module skeleton**

New module at `src/go/plugin/go.d/collector/snmp_traps/`:

- `collector.go`: V2 collector registration as `snmp_traps`, struct with embedded `collectorapi.Base` + `Config`, `SetJobName(string)`, `Init`/`Check`/`Collect`/`Cleanup` lifecycle, `dyncfgCodedError` type implementing `Code() int` and `Unwrap() error`.
- `config.go`: `Config` with `Listen.Endpoints`, `Versions`, `Communities`.
- `trapentry.go`: `TrapEntry` struct per ADR-0001 §4 with `ReportType`, `PduType`, `SnmpVersion`, `VarbindValue`, `DedupSummary` types.
- `trapwriter.go`: `TrapWriter` interface (`Write`, `Flush`, `Close`).
- `config_schema.json`: Netdata `jsonSchema` / `uiSchema` wrapped schema for the collector config.
- `src/go/plugin/go.d/config/go.d/snmp_traps.conf`: disabled stock example job `local` listening on UDP/162 with no automatic high-port fallback.

Registered in `src/go/plugin/go.d/collector/init.go` via blank import.

**M2.3 Trap-job creation preflight**

- `init.go`: `validateJobName(name string) error` — matches `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, max 64 chars, rejects empty, path separators (`/`, `\`), dots, colons, spaces, control chars, leading `_`/`-`. Returns descriptive errors.
- `init.go`: `validateEndpoints(endpoints []EndpointConfig) error` — requires at least one endpoint, protocol must be `udp` (M2 only), address/port parseable via `net.ResolveUDPAddr`, and exact duplicate endpoints are rejected before bind. Uses `net.JoinHostPort` for correct IPv6 handling.
- `init.go`: `validateVersions(versions []string) ([]string, error)` — requires at least one SNMP version, accepts only v1/v2c in M2, normalizes case/whitespace, and rejects duplicates before bind.
- `collector.go` / `listener.go`: `Collector.Init()` binds all endpoints via `newListener(jobName, endpoints)`. Binding is all-or-nothing with cleanup on partial failure; each endpoint binds via `net.ListenUDP`, no automatic high-port fallback. `Collector.Check()` is a no-op after creation-time preflight. `Cleanup()` closes all endpoints and is idempotent.

**Linux unsupported-backend check**: Deferred to M4. M2 instantiates only the UDP listener plus BER limit pre-scan / `gosnmp` trap parse path — these are OS-independent. The journal writer (Linux-only) belongs to M4.

**M2.4 BER limit pre-scan + gosnmp parsing + RFC 3584 + source identification**

`decode.go`:
- Uses the existing Netdata `gosnmp` dependency for SNMPv1 Trap-PDU (tag 0xa4), SNMPv2c Trap-PDU (tag 0xa7), and INFORM (tag 0xa6) parsing instead of maintaining a bespoke full BER trap parser.
- Open-source reference checked: `ilyam8/gosnmp @ 388b2cb5192e`, `trap.go` `UnmarshalTrap`, `marshal.go` `unmarshalTrapV1` / `unmarshalVBL`.
- Netdata-owned BER pre-scan enforces spec §18 limits before `gosnmp.UnmarshalTrap`: datagram ≤ 8 KiB, constructed BER nesting depth ≤8, OID encoded length ≤128 bytes, OctetString ≤1024 bytes, definite BER lengths only. Post-parse validation enforces ≤256 varbinds. The elapsed-time decode budget check remains synchronous and does not create a goroutine/timer per packet; decode/parse errors take priority over elapsed-time budget reporting, while valid-but-over-budget packets return the budget error.
- RFC 3584 v1 normalization inserts synthetic `sysUpTime.0`, `snmpTrapOID.0`, `snmpTrapAddress.0`, `snmpTrapCommunity.0`, and `snmpTrapEnterprise.0` varbinds. Generic v1 trap values 0-5 map to the standard `1.3.6.1.6.3.1.1.5.x` notification OIDs; enterprise-specific trap value 6 maps to `{enterprise}.0.{specificTrap}`.
- Source identification cascade via `identifySource()`:
  - `snmpTrapAddress.0` varbind (OID `1.3.6.1.6.3.18.1.3.0`) → `net.ParseIP` → valid IP
  - UDP peer (kernel-provided from `recvfrom()`)
  - Malformed PDU-provided values (non-string, non-IP) are ignored; UDP peer is the final safe fallback.
- `DecodeTrap(data []byte, udpPeer net.IP) (*TrapPDU, error)` public entry point with bounded parsing and elapsed-time decode budget check. The implementation does not create a goroutine/timer per packet.
- SNMPv3 returns clear "not supported in M2" error.

Tests:
- `TestMinimalV2cDecode`, `TestV2cLinkDownDecode` — v2c trap decode with golden OID assertions.
- `TestInformDecode` — INFORM PDU decodes as `PduTypeInform` without implementing ACK in M2.
- `TestDecodeOversized`, `TestDecodeMalformed`, `TestDecodeInvalidVersion`, `TestDecodeRejectsSNMPv2uVersion`, `TestDecodeRejectsSNMPv3InM2`, `TestDecodeRejectsOctetStringOverLimit` — reject invalid, unsupported, or over-limit inputs.
- `TestDecodeWithBudgetPreservesDecodeError` — malformed input returns the root decode error instead of masking it as a budget error.
- `TestDecodeWithBudgetReturnsBudgetOnSlowSuccess` — successful decode followed by an elapsed-time budget violation returns the budget error.
- `TestDecodeRejectsBERLimits`, `TestValidateBERLimitsAcceptsMaxDepth` — BER depth boundary, OID encoded length, trailing data, and indefinite length coverage.
- `TestDecodeTrapRejectsSNMPv3InM2` — full `DecodeTrap()` path rejects SNMPv3 with the M2 unsupported-version error.
- `TestNormalizePDUValueRejectsUnexpectedType`, `TestNormalizePDUValueSupportsOpaqueFloats` — opaque float/double values are normalized explicitly and truly unexpected Go value types are rejected instead of being stringified.
- `TestSourceFromVarbind` / `TestSourceFromVarbindNetIP` — valid IP, `net.IP`, non-string value, non-IP value, no-match.
- `TestIdentifySourceCascade` — varbind → UDP peer → empty.
- `TestV1TrapOID`, `TestV1DecodeRejectsInvalidGenericTrap`, `TestV1DecodeRejectsInvalidEnterpriseSpecificTrap`, `TestV1DecodeConvertsAgentAddressAndSyntheticVarbinds` — v1 trap OID construction, specific-trap bounds, invalid generic/enterprise-specific trap rejection with accurate errors, RFC 3584 synthetic varbinds, and valid agent-addr source identification.
- `TestDecodeTrapFromPcapCorpus` with `testdata/*.pcap.hex` + `testdata/golden.json` — replayable classic pcap fixtures for v2c coldStart, v1 enterpriseSpecific, INFORM request, and PEN-shaped vendor OID examples for Cisco, Juniper, Arista, HP, and Aruba. Fixtures are sanitized/generated Ethernet+IPv4+UDP captures using documentation IP ranges and `public`; golden assertions include OID, source IP, peer IP, community, version, PDU type, and decoded varbind count. `testdata/README.md` records that vendor fixtures exercise OID/PEN shapes only, not real device event semantics.
- `TestDecodeTrapIntegration`, `TestDecodeTrapNilPeer` — full DecodeTrap with peer IP and nil-peer source fallback.
- `TestManagerCreateCollectorJobSetsJobNameV1`, `TestManagerCreateCollectorJobSetsJobNameV2` — optional `SetJobName(string)` handoff reaches both job factory branches before config apply.
- `TestValidateJobName` — 19 cases covering valid, empty, too-long, dots, slashes, control chars, colons, spaces, leading chars.
- `TestValidateEndpoints` — 10 cases covering valid, duplicate, empty, unsupported protocol, missing address, invalid port, invalid address, IPv6.
- `TestValidateVersions` — valid, normalized, empty, unsupported, and duplicate version cases.
- `TestCollectorInit_BindsEndpointsAndCheckIsNoop`, `TestCollectorInit_IdempotentDoubleInit`, `TestCollectorInit_InvalidJobNameIsCodedError`, `TestCollectorInit_InvalidEndpointsIsCodedError`, `TestCollectorInit_BindsMultipleEndpoints`, `TestCollectorInit_BindFailureIsCodedError`, `TestCollectorInit_InvalidVersionIsCodedError`, `TestCollectorInit_PartialBindFailureClosesPriorSockets`, `TestCollectorCleanupIsIdempotent`, `TestCollectorCollectRequiresStartedListener` — endpoint/version/job-name preflight in `Init()`, multi-endpoint happy path, idempotent double-init, HTTP-422 coded validation/bind failure, partial bind cleanup, idempotent cleanup, and collect-before-init guard.

Validation after direct fixes:
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 90s`
- `go test -race ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s`
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...`
- `go build ./plugin/go.d/...`
- `git diff --check`

**M2 review round 1 findings and disposition**

- Completed reviewers: `kimi`, `minimax`, `qwen`. `glm` and `mimo` review commands were started as requested but produced no final review after about 10 minutes; only their partial read-only output was considered and the exact stale PIDs were terminated.
- Fixed: pcap corpus/golden assertion gap, decode-budget error-priority bug, BER depth boundary ambiguity, missing BER limit tests, missing Init/preflight coded-error jobmgr tests, missing partial bind cleanup test, missing `dyncfgCodedError.Unwrap()`, missing `net.IP` source test, missing invalid v1 generic trap test, and missing cleanup idempotency test.
- Rejected with evidence: converting plain `Update()` create-job errors to `codedError{code:400}` would change shared DynCfg behavior; M2 intentionally preserves plain-error fallback behavior and only coded creation-time trap preflight failures override response codes.
- Rejected with evidence: uncommenting the stock `snmp_traps.conf` job would risk automatic UDP/162 binding. The committed file remains a commented disabled example until the full collector consistency bundle in SOW-0039 defines the final operator-facing stock config.
- Deferred by milestone scope: shared profile cache acquisition is M3; journal directory/writer preflight and Linux backend checks are M4; unsupported-varbind telemetry counters are part of SOW-0036 self-metrics / later writer integration.

**M2 review round 2 findings and disposition**

- Completed reviewers: `glm` accepted with no blocking fixes; `minimax` requested fixes; `kimi`, `mimo`, and `qwen` commands were started as requested but produced no final review after about 8 minutes and their exact stale PIDs were terminated.
- Fixed: SNMPv1 enterprise-specific `specificTrap` upper bound, broader invalid generic trap cases, full `DecodeTrap()` SNMPv3 rejection, multi-endpoint listener happy path, and collect-before-init guard test.
- Rejected with evidence: changing `decodeWithBudget()` to report budget errors before root decode errors on slow malformed packets would contradict the accepted M2 behavior and `TestDecodeWithBudgetPreservesDecodeError`; the synchronous check is post-decode by design because the SOW forbids per-packet goroutine/timer cancellation.
- Not implemented: forcing a third-party `gosnmp.UnmarshalTrap` panic for test coverage. The Netdata-owned BER pre-scan is bounds-tested; the `recover()` wrapper remains defense-in-depth for parser regressions.

**M2 review round 3 findings and disposition**

- Completed reviewers: `glm`, `kimi`, `minimax`, and `qwen`. `mimo` was intentionally skipped for this and remaining rounds because the user reported it is out of quota. All commands used stdin closed with `</dev/null` per user instruction.
- Reviewer verdicts after round 3: `glm` accepted with low observations; `minimax` accepted with non-blocking notes; `kimi` accepted with minor fixes; `qwen` accepted with fixes. No reviewer reported a blocking architecture, security, or DynCfg-regression issue.
- Fixed: creation-time SNMP version validation, explicit opaque float/double normalization, rejection of unexpected varbind value types, pcap varbind-count golden assertions, duplicate endpoint validation, ADR-aligned `Category` / `Severity` types, valid-packet decode-budget test, `Init()`-level coded-error tests for invalid job name and invalid endpoint validation, clearer enterprise-specific `specificTrap` error, clearer `update_every` schema text, and removal of a redundant UDP address resolver wrapper.
- Rejected with evidence: changing the synchronous decode budget into a hard cancellation would require the per-packet goroutine/timer pattern already rejected for M2. The current implementation is a bounded pre-scan plus post-decode elapsed-time guard.
- Deferred by milestone scope: receive loop, metrics, rate limiting, INFORM acknowledgement, SNMPv3, profile loader/rendering, journal writer/directory/retention preflight, and IPv6 pcap corpus are owned by later milestones/SOWs.

**M2 review round 4 findings and disposition**

- Completed reviewers: `glm`, `minimax`, and `qwen`. `kimi` was started with stdin closed but spent over 11 minutes in repeated source probing without a final verdict; the exact stale review PIDs were verified and terminated. `mimo` remained skipped due to quota exhaustion.
- Reviewer verdicts after round 4: `glm` accepted with minor fixes; `minimax` accepted with two low-scope clarity notes; `qwen` accepted with fixes. No completed reviewer reported a blocking security, architecture, DynCfg-regression, or M3/M4-compatibility issue.
- Fixed: clearer `update_every` schema wording for event-driven reception, flattened `Collector.Init()` version validation, direct nil-peer `DecodeTrap()` coverage, double-`Init()` idempotency coverage, clearer `TestCollectorCallbacks_Update_CreateCollectorJobPlainErrorPreserved` naming, and an explanatory note for the negative-budget test harness branch.
- Rejected with evidence: the `vnode` field is not dead trap-module state; it follows existing Go collector config shape and is consumed by the shared job factory through `cfg.Vnode()` before runtime job construction. Evidence: `src/go/plugin/agent/jobmgr/job_factory.go:100-159`, plus existing `dyncfg_vnode_test.go` coverage for vnode injection.
- Rejected with evidence: a full handler-to-real-collector coded-error integration test would duplicate already-tested links. Callback preservation is tested in `dyncfg_collector_test.go`; handler coded response propagation is tested in `handler_test.go`; the trap `Init()` coded-error surface is tested in `snmp_traps/init_test.go`.
- Rejected with evidence: DNS re-resolution and multicast/broadcast-specific validation are not M2 blockers. Any bind failure still occurs synchronously during job creation and returns coded 422; explicit policy for hostname/multicast listener support belongs in a later network-surface SOW if supported.
- Deferred by milestone scope: community filtering, receive loop, metrics/charts, richer vendor pcap payload diversity, recover-forced panic tests, and pcap IPv6 fixtures belong to later milestones once the hot path and writer exist.

**Deferred by design, outside M2**:

- SNMPv3, INFORM ack, rate limiting, profile rendering, and journal writer are deferred to later milestones per the SOW scope.

### 2026-05-25 — M3 Implementation

**Status**: M3 implementation completed (uncommitted working tree).

**M3.1 Shared profile cache lifecycle**

`profile.go` adds:
- `VarbindDef` — varbind metadata with OID, MIB type, enum, constraints, and internal raw name
- `TrapDef` — trap entry with OID, name (MIB-qualified), category, severity, description, labels, varbind refs, dedup key varbinds, per-trap shared varbinds map
- `ProfileIndex` — OID→TrapDef map for trap lookup
- `profileCache` — package-level mutex-protected state with refcounting
- `AcquireProfileCache()` — loads profiles on first call, increments refcount, returns index+gen
- `ReleaseProfileCache(generation)` — decrements refcount; releases index at zero
- `resetProfileCacheForTest()` — test isolation helper
- `validateTrapDef()` — validates required fields, numeric OIDs, globally unique MIB-qualified names, closed-set category/severity/status, varbind references, inline varbinds, dedup key references, label keys, and bounded-cardinality label template references
- `buildSharedVarbinds()` — merges file-scoped varbind table with per-trap inline varbinds

**M3.2 Profile loader**

`load.go` adds:
- `getProfileDirs()` — multipath: test override → source-relative (test mode) → executable-relative dev dir → user dirs + stock dir
- `trapProfilesDirFromThisFile()` — resolves stock dir relative to source for test mode
- `loadProfileCache()` — walks multipath dirs, filename-dedup by basename at file granularity (first occurrence wins), validates traps, builds OID index
- `loadProfilesFromDir()` — walks directory, skips non-YAML and `_`-prefixed files
- `loadProfile()` — loads YAML, resolves extends chain with circular detection, deduplicates base traps that are overridden by same-OID current-file entries
- `mergeVarbinds()` — base varbinds merged into target (target wins on collision)

Loaded profile index contains 50,198 traps across 351 stock vendor files.

**M3.3 Template renderer**

`resolver.go` adds:
- `renderMessage(entry, td)` — renders description template, defaults to `{TRAP_NAME} on {_HOSTNAME}.` when no description, caps at 512 bytes with `...` truncation
- `renderLabels(entry, td)` — renders label templates, one per label key
- Template variable references: `{varname}`, `{varname.raw}`, `{numeric.oid}`, `{_HOSTNAME}`, `{TRAP_SOURCE_IP}`, `{TRAP_NAME}`, `{TRAP_DEVICE_VENDOR}`, `{TRAP_INTERFACE}`, `{TRAP_NEIGHBORS}`
- `resolveReference()` — resolves references: special vars → varbind name → numeric OID fallback
- `resolveSpecialVar()` — resolves `_HOSTNAME` (DeviceHostname or SourceIP fallback, no DNS), `TRAP_SOURCE_IP`, `TRAP_NAME`, `TRAP_DEVICE_VENDOR`
- `resolveVarbindByName()` / `resolveVarbindByOID()` — profile-defined varbinds first, then raw PDU fallback
- `varbindDisplayValue()` — renders with enum labels when available
- `varbindRawValue()` — raw string representation for int, uint, float, bool, string, []byte
- `<missing>` for absent varbinds, `<unresolved:varname>` for unknown references

**M3.4 2-tier varbind resolution**

`resolver.go` `resolve2TierVarbind()`:
1. Profile inline varbinds table (OID→VarbindDef with name, type, enum)
2. Raw fallback (OID-keyed, ASN.1-decoded type only)
3. No runtime MIB compilation tier

**M3.5 Collector integration**

`collector.go` updates:
- `Collector` gains `profileGen uint64` and `profileIndex *ProfileIndex`
- `Init()` calls `AcquireProfileCache()` after config validation, before listener creation
- `Init()` releases profile cache reference on bind/listener failure (partial resource cleanup)
- `Cleanup()` releases profile cache reference
- Profile load failures at `Init()` return `dyncfgCodedError{code: 422}`

**M3.6 Tests**

`profile_test.go` adds tests:
- `TestProfileCacheLazyLoad` — cache loads on first acquire
- `TestProfileCacheSharedAcrossCollectors` — same index for multiple acquires
- `TestProfileCacheReleaseAndReacquire` — empty cache after release, re-loaded on next acquire
- `TestProfileCacheReleaseIdempotent` — double release does not panic
- `TestCollectorInitAcquiresProfileCache` — Init acquires; Cleanup releases
- `TestMultipleCollectorsShareSameCache` — two collectors share same index; one cleanup keeps cache alive
- `TestInitBindFailureReleasesProfileRef` — bind failure releases profile ref
- `TestProfileLoadValid` — valid profile loads and resolves by OID
- `TestProfileDirPathBuilders` — profile paths do not double-prefix `go.d`
- `TestProfileLoadEmptyDirFails` — empty profile directories are creation-time failures
- `TestProfileLoadMissingName` — rejection of missing `name` field
- `TestProfileLoadNonMIBQualifiedName` — rejection of non-MIB-qualified name
- `TestProfileLoadInvalidCategory` / `TestProfileLoadInvalidSeverity` / `TestProfileLoadInvalidStatus` — closed-set validation
- `TestProfileLoadInvalidFileVarbind` — file-scoped varbind OID/type validation
- `TestProfileLoadInlineVarbind` / `TestProfileLoadInvalidInlineVarbind` — inline varbind acceptance and creation-time validation
- `TestProfileLoadDanglingVarbind` — rejection of varbind ref not in file table
- `TestProfileLoadDanglingDedupKey` — rejection of dedup key ref not in file table
- `TestProfileLoadInvalidLabelKey` — rejection of label key not matching `^[a-z][a-z0-9_]*$`
- `TestProfileLoadDuplicateOID` — duplicate OID across files is a load error
- `TestProfileLoadDuplicateName` — duplicate MIB-qualified trap names across different OIDs are a load error
- `TestProfileLoadFilenameDedup` — same filename in higher-priority dir replaces lower-priority
- `TestProfileLoadFilenameDedupKeepsAllTrapsFromWinningFile` — file-level dedup preserves every trap from the winning file
- `TestProfileLoadExtendsMerge` — base traps overridden by same-OID current-file entries; `_`-prefixed files excluded from direct loading
- `TestProfileLoadExtendsLaterBaseOverridesEarlier` — later `extends:` entries override earlier base entries for the same OID
- `TestProfileLoadExtendsPartialTrapOverride` — override trap entries inherit omitted fields from their base entry
- `TestProfileLoadRejectsUnsafeExtendsName` — `extends:` entries are filenames only and cannot escape profile directories
- `TestCollectorInit_ProfileLoadFailureIsCodedError` — profile load failures surface as coded job-creation errors
- `TestRenderMessageDefault` — default template when no description
- `TestRenderMessageWithVarbinds` — varbind name substitution with profile metadata
- `TestRenderMessageMissingVarbind` — `<missing>` for known profile varbinds absent from the received PDU
- `TestRenderMessageUnresolvedRef` — `<unresolved:varname>` for unknown references
- `TestRenderMessageNumericOIDRef` — numeric OID reference resolves from raw varbinds
- `TestRenderMessageNumericOIDRawRef` — `{numeric.oid.raw}` resolves from raw varbinds
- `TestRenderMessageMalformedNumericOIDRawRef` — malformed numeric-looking refs do not take the raw-OID path
- `TestRenderMessageEmptyStringVarbindPresent` — an empty string value in a present varbind renders as empty, not missing
- `TestRenderMessageEnumSubstitution` — enum label substitution
- `TestRenderMessageRawEnumValue` — `.raw` suffix returns raw numeric
- `TestRenderMessageTruncation` — 512-byte cap with `...` marker
- `TestRenderMessageTruncationKeepsValidUTF8` — truncation does not split UTF-8 runes
- `TestRenderLabels` — label template rendering
- `TestRenderMessageSpecialVars` — `_HOSTNAME`, `TRAP_SOURCE_IP`, `TRAP_NAME`, `TRAP_DEVICE_VENDOR`
- `TestRenderMessageHostnameFallback` — `_HOSTNAME` falls back to SourceIP
- `TestProfileLoadRejectsUnboundedLabelVarbind` — rejects high-cardinality label references at profile load
- `TestResolve2TierProfileFirst` / `TestResolve2TierRawFallback` / `TestResolve2TierRawFallbackNoName` / `TestResolve2TierEnum` — 2-tier resolution with and without profile metadata
- `TestStockProfileIndexLoads` — stock IETF OIDs verified in live profile load

M3 validation after direct fixes:

- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 90s` — passed
- `go test -race ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 120s` — passed
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...` — passed
- `go build ./plugin/go.d/...` — passed
- `git diff --check` — passed

**M3 known limitations and deferred items**:
- No operator override paths for `TRAP_INTERFACE` / `TRAP_NEIGHBORS` (enrichment deferred to SOW-0037)
- `snmp.trap.errors.template_unresolved` counter increment deferred to SOW-0036 M4
- No hot reload (SOW-0037)
- No dedup_key_varbinds cardinality enforcement in M3 (stored but not acted upon)

**Direct review fixes after DeepSeek M3**:

- Fixed file-level filename-dedup bug: the first DeepSeek implementation deduped by basename per trap and would have kept only the first trap from multi-trap files. Loader now dedups by file and preserves all traps from the winning file.
- Fixed test/runtime stock profile paths: `pluginconfig.Collectors*Dir()` already points at `go.d`, so appending another `go.d` would break installed profile lookup. Added path-builder test coverage.
- Widened accepted `status` values to cover shipped SMIv1 statuses (`mandatory`, `optional`) as well as SMIv2 (`current`, `deprecated`, `obsolete`), and updated profile-format documentation.
- Added bounded-cardinality validation for profile label templates instead of leaving it syntax-only.
- Changed lifecycle/cache tests to use a tiny temp profile pack; only `TestStockProfileIndexLoads` loads the full stock pack. This keeps `go test -race ./plugin/go.d/collector/snmp_traps/...` practical and it now passes.
- Fixed `extends:` semantics to match the spec: later bases override earlier bases, current-file trap entries inherit omitted fields from the selected base, and extends entries are basename-only YAML filenames to prevent profile-dir escape.
- Fixed reviewer-found resolver edge cases: empty string varbind values are present values, numeric OID `.raw` references resolve correctly, UTF-8 message truncation does not split runes, and YAML parser panics are recovered into load errors.
- Added an explicit profile-cache comment that the current single active generation is valid for SOW-0035 because hot reload is out of scope; SOW-0037 must replace it with per-generation holder accounting.

**M3 external review round 1**:

- Completed reviewers: `glm`, `kimi`, `minimax`, and `qwen`. `mimo` was intentionally skipped because the user reported it is out of quota. All external commands used stdin closed with `</dev/null`.
- Fixed blocking/real findings: unsafe `extends:` path traversal, partial config mutation before all `Init()` validation completed, empty string varbind treated as missing, numeric OID `.raw` references, UTF-8 truncation, YAML unmarshal panic handling, and incomplete `extends:` override semantics.
- Fixed quality findings: file-level dedup preserving all traps from the winning file, stock/user profile path construction, release-on-zero-ref documentation, and `extends` merge complexity.
- Rejected with evidence: the test-mode stock-path concern does not apply because source-relative test loading is intentional and covered by `TestStockProfileIndexLoads`; dev-mode executable-relative loading matching existing go.d profile patterns is accepted for local development only.
- Deferred by explicit milestone scope: hot-reload per-generation holder accounting belongs to SOW-0037; template unresolved counters belong to SOW-0036; enrichment fields for interface/neighbors belong to SOW-0037.

### 2026-05-25 — M4 implementation state before external review

**DeepSeek M4 execution**:

- DeepSeek was started as the implementation worker with stdin closed and without `--agent code-reviewer`, per user instruction.
- The command reached the 30-minute timeout before producing a complete, validating M4 result.
- The draft it left contained useful structure (`TrapEntry`, `TrapWriter`, direct journal writer, retention config, tests), but the journal files initially failed `journalctl` end-to-end validation. The coordinating assistant took over implementation directly.

**Direct M4 fixes applied by the coordinating assistant**:

- Fixed systemd journal format compatibility:
  - header size pinned to the systemd minimum header (`208` bytes) so unsupported modern counters are not advertised;
  - DATA object regular payload offset corrected to 64 bytes;
  - ENTRY_ARRAY items corrected to 8-byte offsets;
  - hash table header offsets point to hash-table item payloads, not object headers;
  - `tail_object_offset` now points to the last object header, not EOF;
  - `head_entry_realtime` remains zero until the first entry is written.
- Fixed hash behavior against systemd source evidence:
  - SipHash-2-4 round order corrected and covered by the upstream systemd test vector;
  - Jenkins lookup3/hashlittle2 implemented with systemd's high/low 32-bit half ordering for non-keyed hashes and ENTRY `xor_hash`.
- Added strict creation-time writer checks:
  - machine ID and boot ID are read and parsed at `NewJournalWriter()` creation;
  - malformed/missing IDs are creation-time errors, not placeholder runtime warnings;
  - journal directory creation and writability checks happen before the writer reports success.
- Added journal recovery at writer creation:
  - scans existing `.journal` files in the job directory;
  - truncates partial tail objects;
  - rebuilds DATA/FIELD hash tables;
  - rebuilds global ENTRY_ARRAY chains and per-DATA entry-array chains from valid ENTRY objects;
  - archives recovered interrupted files before opening a new active file;
  - quarantines structurally corrupt files under a non-`.journal` suffix if repair is impossible.
- Added the receive path missing from the draft:
  - one UDP read loop per configured endpoint;
  - listener close waits for read loops before the writer closes;
  - decoded v1/v2c traps flow through profile lookup, 2-tier varbind resolution, template rendering, label rendering, `TrapEntry`, `TrapWriter`, and journal serialization.
- Fixed `TrapWriter.Flush()` correctness:
  - `Flush()` now drains all queued entries before syncing and returning;
  - worker failure is returned without blocking;
  - `Close()` drains remaining entries before finalizing the journal file.
- Added field-name validation at the journal writer boundary to reject invalid journal field names before serialization.
- Added a test-only mock `TrapWriter` implementation for follow-on pipeline tests.

**M4 validation run before external review**:

- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 120s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 120s` — passed.
- `go test -race ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 180s` — passed.
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...` — passed.
- `go build ./plugin/go.d/...` — passed.
- `jq . src/go/plugin/go.d/collector/snmp_traps/config_schema.json >/dev/null` — passed.
- `git diff --check` — passed.
- Benchmark gate:
  - command: `go test ./plugin/go.d/collector/snmp_traps -bench 'Benchmark(TrapWriterWrite|JournalTrapWriterDrain|JournalWriterWriteEntry)' -benchmem -benchtime=30000x -run '^$' -timeout 120s`
  - `BenchmarkTrapWriterWrite-24`: 87.39 ns/op, 0 B/op, 0 allocs/op
  - `BenchmarkJournalTrapWriterDrain-24`: 45432 ns/op, 4602 B/op, 74 allocs/op
  - `BenchmarkJournalWriterWriteEntry-24`: 39727 ns/op, 2184 B/op, 37 allocs/op

**M4 real-use tests added**:

- `TestCollectorReplayPcapThroughListenerToJournal`: replays the M2 `v2c_coldstart.pcap.hex` fixture through a real UDP listener endpoint, resolves it with a test profile, writes the journal entry, and verifies `journalctl --directory=<test-dir> TRAP_CATEGORY=security -o json` returns the expected entry.
- `TestJournalWriterVerify`: writes active and archived journal files and validates both with `journalctl --verify --directory=<test-dir>`.
- `TestJournalWriterRecoveryTruncatesPartialTailAndVerifies`: simulates an interrupted writer with a partial tail object, creates a new writer, verifies recovery, and checks `journalctl --verify` plus query visibility of the pre-crash entry.
- `TestJournalTrapWriterFlushAfterWorkerFailureDoesNotBlock`: proves `Flush()` returns worker failure instead of deadlocking after a permanent writer error.

**M4 review status**:

- External M4 review round 1 started with `glm`, `kimi`, `minimax`, and `qwen`.
- `mimo` remains skipped per user instruction due to quota exhaustion.
- All remaining external review commands must use `</dev/null`.

**M4 external review round 1 findings and disposition**:

- Completed reviewers: `glm`, `kimi`, `minimax`. `qwen` reached the 30-minute command timeout (`timeout 1800`) with stdin closed and produced no review output, so round 1 has no qwen findings to validate.
- `glm` found no blocking issues. Its medium observation on silent packet/drop diagnostics is deferred to SOW-0036 self-metrics, where the full error-counter universe belongs. Its direct `handlePacket` mock-test suggestion was implemented even though the existing UDP listener pcap test already covered that path through the real read loop.
- `minimax` reported two blockers that were rejected after direct evidence review:
  - Rejected: "boot ID overwrites machine ID" — systemd `Header` layout is `file_id` at 24-40, `machine_id` at 40-56, `tail_entry_boot_id` at 56-72, and `seqnum_id` at 72-88. Updating offset 56 writes `tail_entry_boot_id`, not `machine_id`.
  - Rejected: "listener bind errors are not coded" — `newListener()` returns a plain local error, but `Collector.Init()` wraps every listener creation failure in `dyncfgCodedError{code: 422}` before returning to jobmgr/DynCfg. This satisfies the user-visible DynCfg apply contract.
- `kimi` found verified blocking issues:
  - CWE-117 classifier existed but was not wired into the writer path. Fixed by counting unsafe field values at `WriteEntry()` time through `journalFieldNeedsBinary()` and adding `JournalWriter.SanitizedFields()` plus `TestJournalWriterCountsSanitizedFields`.
  - Several journal writer sub-operations set `permanentFailure` or ignored `ReadAt`/`WriteAt` errors while `WriteEntry()` could still return nil. Fixed by checking `ReadAt`/`WriteAt` errors in hash/header update helpers and checking `permanentFailure` / zero offsets after `findOrAddData`, `findOrAddField`, `writeEntryObject`, `appendEntryArray`, `linkDataToEntry`, and `updateAfterEntry`. Added `TestJournalWriterWriteEntryReturnsFileError`.
  - `journalTrapWriter.Close()` did not return a stored terminal error on later calls after a failed first close. Fixed and covered by `TestJournalTrapWriterCloseReturnsStoredTerminalError`.
- Kimi non-blocking findings disposition:
  - Silent decode/filter/write-drop counters deferred to SOW-0036 self-metrics.
  - Periodic retention sweeps beyond rotation-time sweeps deferred to a later retention hardening task; default `rotation_duration: 1h` keeps `max_duration` from waiting indefinitely in the stock configuration.
  - `mockTrapWriter` unused finding fixed by adding `TestCollectorHandlePacketWritesProfileResolvedTrapEntry`.

### 2026-05-26 — SOW-0041 consolidated into M3 ingestion

The user confirmed that pending SOW-0041 must be incorporated into ingestion. SOW-0041 is therefore closed as a standalone pending SOW and its scope is owned by this SOW's M3 profile lookup / ingestion work.

Consolidated requirement:

- `ProfileIndex.Lookup` remains exact-match-first.
- On primary miss only, lookup tries one alternate SMIv1 / SMIv2 trap-OID key by adding or removing a single `.0.` segment immediately before the final OID arc.
- The tolerance applies only to trap OIDs. Varbind OID resolution remains exact.
- The behavior is unconditional. There is no config flag.
- Full pack audit/regeneration is deferred because the receiver-side tolerance fixes the runtime ingestion failure class without blocking on an offline corpus audit.

Acceptance criteria imported from SOW-0041 into SOW-0035:

- A decoded trap OID `1.3.6.1.4.1.14179.2.6.3.0.24` matches a profile entry `1.3.6.1.4.1.14179.2.6.3.24`.
- A decoded trap OID `1.3.6.1.2.1.33.2.1` matches a profile entry `1.3.6.1.2.1.33.2.0.1`.
- Exact match wins when both forms exist.
- Degenerate/too-short/edge OIDs do not create spurious matches.
- True misses still return no profile entry.
- Primary-hit lookup does not pay an alternate lookup; primary miss pays at most one alternate lookup.
- `profile-format.md`, `.agents/sow/specs/snmp-traps/netdata.md`, `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`, and `tools/snmp-traps-profile-gen/README.md` document the behavior.

### 2026-05-26 — SDK-backed journal adapter integrated

The SDK repository published `go/v0.1.0` with `go/API.md`. SOW-0035 journal work now uses the SDK instead of expanding the provisional package-local journal writer.

Implemented changes:

- Added direct Go module dependency `github.com/netdata/systemd-journal-sdk/go v0.1.0`.
- Replaced the package-local binary journal format/hash/recovery code with a thin `JournalWriter` adapter around `journal.NewLog`.
- Adapter uses `LogOpenEager` so directory creation/open, active file creation, writer lock acquisition, rotation/retention validation, and SDK writer options fail during job creation.
- Adapter uses `LogIdentityStrict` with explicit `/etc/machine-id` and `/proc/sys/kernel/random/boot_id` parsing so missing/malformed IDs are coded creation-time failures.
- Kept the local `TrapWriter` queue, `TrapEntry` serializer, `CWE-117` unsafe-field accounting, and `TrapWriter` abstraction.
- Updated collector state to store the SDK effective `JournalDirectory()` because the SDK appends the machine-id child directory under the configured per-job root.
- Fixed an existing `journalTrapWriter.drainAndDiscard()` deadlock where receiving from a closed queue looped forever after a worker failure.
- Updated spec §5 / §11 and ADR-0001 to record the SDK-backed backend and machine-id directory contract.

Validation after SDK integration:

- `go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 180s` — passed.
- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 180s` — passed.
- `go test -race ./plugin/go.d/collector/snmp_traps/... -count=1 -timeout 240s` — passed.
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...` — passed.
- `go build ./plugin/go.d/...` — passed.
- `git diff --check` — passed.
- `.agents/sow/audit.sh` — passed with only pre-existing non-project skill classification warnings.

### 2026-05-26 — Post-SDK review rounds and current disposition

External reviewers were rerun over the full SOW scope after SOW-0041 consolidation, SDK integration, and direct fixes. `mimo` remained skipped by user instruction. All external commands used stdin closed with `</dev/null`.

Reviewer findings and disposition:

- `glm` found a real spec/code mismatch: the implementation and config schema treat an empty `communities` list as accept-all for SOW-0035, while the spec example still said reject-all. Fixed by updating spec §7.5 and the stock config comment. SOW-0036 owns production auth policy.
- `glm` and `qwen` repeated the known hot-path observability gap: decode/filter/write drops are silent. Disposition: deferred to pending SOW-0036, which owns self-metrics and the full error-counter universe. This does not block SOW-0035 because SOW-0035 is the journal-backed ingestion slice.
- `qwen` requested two low-risk cleanups. Fixed: `JournalWriter.JournalDirectory()` now returns the immutable cached path instead of re-reading from the SDK after creation; jobmgr fallback wrappers now use `%w` instead of `%v` so plain-error chains remain inspectable.
- `qwen` requested avoiding hot-path reads from embedded `Base.Vnode`. Fixed by snapshotting `c.Vnode` into `c.vnode` during `Init()` and using the snapshot for packet handling.
- `glm` requested listener read-error hardening and port 162 operator guidance. Fixed: read loop now backs off after unexpected non-close read errors; config schema and stock config mention `CAP_NET_BIND_SERVICE` for UDP/162.
- `glm` requested a real end-to-end listener test. Fixed: `TestCollectorReplayPcapThroughListenerToJournal` sends the M2 pcap fixture through a UDP listener, flushes the writer, and queries the SDK journal directory with `journalctl --directory`.
- `minimax` reported a blocker for a nonexistent `newJournalTrapWriter` error path. Rejected: `newJournalTrapWriter` returns `*journalTrapWriter`, not `(*journalTrapWriter, error)`, so the cited rollback branch cannot occur. Production `Init()` always supplies a non-nil journal writer; the nil-journal path is test/benchmark sink mode and is now commented.
- `minimax` reported SOW-0041 artifact updates missing. Rejected: `.agents/sow/specs/snmp-traps/netdata.md`, `profile-format.md`, `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`, and `tools/snmp-traps-profile-gen/README.md` all document the `.0.` tolerance.
- `kimi` reported a defensive path-traversal concern around `journalRoot`. Rejected as a runtime bug: `journalRoot` is package-local and `Collector.Init()` validates the job name before calling it. Added a precondition comment to keep the invariant obvious.
- `kimi` requested documenting the SDK CWE-117 dependency and several hot-path constants. Fixed with short code comments: SDK DATA objects are length-delimited and the local classifier counts unsafe values for future self-metrics; decode limits now carry rationale comments.

Current benchmark evidence after SDK integration:

- `BenchmarkTrapWriterWrite-16`: 34.32 ns/op, 0 B/op, 0 allocs/op.
- `BenchmarkJournalTrapWriterDrain-16`: 294660 ns/op, 4966 B/op, 75 allocs/op.
- `BenchmarkJournalWriterWriteEntry-16`: 183031 ns/op, 2549 B/op, 38 allocs/op.

Disposition: queue acceptance is excellent, but actual SDK-backed append/drain throughput remains below the original "tens of thousands/sec" M4 target on this workstation. This is a real performance risk, not a correctness blocker for the MVP ingestion slice. It is mapped to SDK pending SOW `.agents/sow/pending/SOW-0009-20260523-benchmark-profile-optimize.md` in the adjacent `systemd-journal-sdk` workspace and to the final Netdata merge gate SOW-0039 for re-benchmarking before merge.

Final validation after review-driven fixes:

- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 240s` — passed.
- `go test -race ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/... -count=1 -timeout 360s` — passed.
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/agent/jobmgr/... ./plugin/framework/dyncfg/...` — passed.
- `go test ./plugin/go.d/... -run '^$' -count=1` — passed.
- `jq empty plugin/go.d/collector/snmp_traps/config_schema.json` — passed.
- `git diff --check` — passed.
- `.agents/sow/audit.sh` — passed with only pre-existing non-project skill classification warnings.

## Validation

Acceptance criteria evidence: M1 accepted in ADR-0001 after round-5 fixes and direct review; M2 implemented and tested; M3 implemented and reviewed; SOW-0041 SMIv1/SMIv2 trap-OID tolerance folded into M3 and tested; M4 uses the SDK-backed journal adapter and validates creation-time journal preflight plus `journalctl --directory` queryability.
Tests or equivalent validation: `go test` passes for all affected packages (jobmgr, dyncfg, snmp_traps). `go test -race` passes for the same affected package set. `go vet`, `go test ./plugin/go.d/... -run '^$'`, `jq` schema validation, `git diff --check`, SDK-backed `journalctl --directory` tests, and the SOW audit all pass except pre-existing non-project skill classification warnings.
Real-use evidence: M2 has replayable pcap fixture decode coverage. M4 has SDK-backed journal write coverage through `journalctl --directory=<effective-journal-dir> TRAP_CATEGORY=security -o json` query visibility and an end-to-end pcap → UDP listener → decode → profile lookup → journal write → `journalctl --directory` test.
Reviewer findings: M1 review rounds 1-5 recorded above; M2 review rounds 1-4 recorded above; M3 external review round 1 recorded above; post-SDK M4 review rounds recorded above. Open reviewer findings are either fixed, rejected with code evidence, or mapped to pending SOW-0036 / SOW-0039 / SDK SOW-0009.
Same-failure scan: `rg 'CodedError|codedError|MarkNonDisruptiveUpdate' src/go/plugin` run before implementation. Framework changes preserve existing plain-error retry behavior while adding coded error propagation.
Sensitive data gate:

- No SNMP communities, USM keys, or operator-secret data in any committed artifact.
- All test fixtures use public/well-known communities ("public", "c") only.
Artifact maintenance gate:

- `AGENTS.md`: no project-wide workflow or responsibility change needed for this SOW.
- Runtime project skills: `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` updated for the SMIv1 / SMIv2 `.0.` trap-OID tolerance.
- Specs: `.agents/sow/specs/snmp-traps/netdata.md` and ADR-0001 updated for Go process model, TrapWriter contract, SDK-backed journal adapter, and `.0.` tolerance.
- End-user/operator docs: trap profile-format documentation and generator README updated for `.0.` tolerance. Full plugin docs, stock config consistency, health alerts, and taxonomy are owned by SOW-0039.
- End-user/operator skills: no public operator skill changed in this SOW. `query-snmp-traps` is owned by SOW-0039.
- SOW lifecycle: SOW-0041 was folded into SOW-0035 and closed. SOW-0035 remains in `current/` as `paused` until SOW-0039 performs the final merge-gate closeout for SOW-0035 through SOW-0039.

## Outcome

Implementation slice complete on the feature branch, including creation-time preflight, multi-endpoint listener, shared lazy profile cache, SMIv1 / SMIv2 `.0.` trap-OID lookup tolerance, SDK-backed journal adapter, and targeted validation. The SOW is paused, not terminal-completed, because final collector consistency and merge readiness are owned by SOW-0039.

## Lessons Extracted

- Creation-time failure surfacing is a cross-cutting contract: trap job failures needed both collector-side coded errors and DynCfg/jobmgr propagation fixes.
- The SDK-backed writer is the right ownership boundary, but current append/drain throughput remains a real merge-gate risk until the SDK optimization SOW is complete and SOW-0039 re-benchmarks it.
- The profile index is the correct place for SMIv1 / SMIv2 trap-OID `.0.` tolerance because it keeps decoder output canonical and avoids changing varbind lookup semantics.

## Followup

- SOW-0036: self-metrics and full error-counter universe, including decode failures, community/version drops, listener read errors, writer queue pressure, unknown OIDs, and sanitized-field counters.
- SOW-0037: hot reload / per-generation profile holder accounting beyond the single-generation SOW-0035 cache.
- SOW-0039: final merge gate must re-check SDK append/drain throughput and decide whether the SDK SOW-0009 optimization is required before merge.
- SDK repository pending SOW-0009 (`systemd-journal-sdk/.agents/sow/pending/SOW-0009-20260523-benchmark-profile-optimize.md`): benchmark/profile/optimize the SDK writers using the deterministic ingestion corpus.

## Test Isolation Repair - 2026-05-27

Facts:

- The running Netdata installation had accumulated many SNMP trap journal files under `/var/cache/netdata/traps/`.
- The root cause in the test suite is that `journalRoot(jobName)` uses `buildinfo.CacheDir`, which defaults to the production cache root, while several `Collector.Init()` tests exercised journal writer creation without replacing `buildinfo.CacheDir`.
- Existing evidence: `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go` builds the journal root from `buildinfo.CacheDir`; `src/go/plugin/go.d/collector/snmp_traps/init_test.go` and `src/go/plugin/go.d/collector/snmp_traps/profile_test.go` contain Init-path tests that did not set a temporary cache root; the e2e test already did this locally and showed the intended pattern.

Plan:

- Stop Netdata, empty `/var/cache/netdata/traps/`, and start Netdata again before code changes.
- Add one package test helper that overrides `buildinfo.CacheDir` to an explicit `/tmp/snmp-traps-test-cache-*` directory and restores the original value with `t.Cleanup`.
- Use the helper in every trap test that reaches `Collector.Init()` and in the `journalRoot` unit test so test journals cannot be written under production paths.
- Run the focused `snmp_traps` package tests and verify the test run does not add or modify `/var/cache/netdata/traps/`. If the live Netdata service recreates its own `local` journal after startup, compare the directory before and after tests instead of treating runtime-created live files as test artifacts.

Sensitive data handling:

- No production trap payloads, community strings, device identifiers, or trap journal contents are copied into repository artifacts. The durable SOW record uses only paths and code references.

Validation:

- `sudo systemctl stop netdata` completed, `/var/cache/netdata/traps/` was emptied, and `sudo systemctl start netdata` restored the service.
- After startup, the live `snmp_traps` job recreated `/var/cache/netdata/traps/local/` and bound UDP/162 through `go.d.plugin`; this is runtime state from the configured listener, not test output.
- Before/after snapshots around the focused test runs had the same four production trap-path entries, so the tests did not add or modify production trap journal files.
- `timeout 240 go test ./plugin/go.d/collector/snmp_traps -count=1` passed.
- `timeout 360 go test -race ./plugin/go.d/collector/snmp_traps -count=1` passed.
- `git diff --check` passed.

## Full Ingestion Throughput Clarification - 2026-05-28

Facts:

- The previously discussed journal numbers were only half of the pipeline.
- `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go:113` `BenchmarkPacketTrap` includes packet decode/profile/rendering through `Collector.handlePacket()`, but its sink is `countingWriter` (`benchmark_test.go:48`), so it does not exercise journal serialization or SDK journal output.
- `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go:269` `BenchmarkJournalTrapWriterDrain` exercises queued trap writer + SDK journal output, but it starts from `benchmarkTrapEntry()` (`benchmark_test.go:278`), so it does not exercise packet input/decode/profile/rendering.
- Any shorthand statement that the SDK journal output benchmark is "ingestion throughput" is therefore incorrect. It is output throughput only.

Temporary full-pipeline benchmark:

- Run against `github.com/netdata/systemd-journal-sdk/go@v0.3.0` with an uncommitted overlay benchmark.
- Path measured: synthetic SNMPv2c packet bytes -> `Collector.handlePacket()` -> decode/profile/template/render -> `journalTrapWriter` queue -> SDK journal append/sync -> `journalctl --directory` row-count verification after the timed section.
- Correctness check: 1,000 packets produced 1,000 queryable journal rows.
- `go test -modfile=<temp-v0.3.0.mod> -mod=mod -overlay=<temp-full-pipeline-overlay.json> ./plugin/go.d/collector/snmp_traps -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -run '^$' -count=3`
  - 30,000 packets/run: 13.35-13.96 us/op, 71.6K-74.9K packets/s, 71.6K-74.9K persisted entries/s, 0-1 drops/run, 12,625-12,626 B/op, 206 allocs/op.
- `go test -modfile=<temp-v0.3.0.mod> -mod=mod -overlay=<temp-full-pipeline-overlay.json> ./plugin/go.d/collector/snmp_traps -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -run '^$' -count=3`
  - 100,000 packets/run: 14.64-17.98 us/op, 55.6K-68.3K packets/s, 55.6K-68.3K persisted entries/s, 1-2 drops/run, 12,580-12,581 B/op, 206 allocs/op.

Current performance interpretation:

- Raw SDK journal append is not the full pipeline.
- Queued journal output from prebuilt `TrapEntry` is not the full pipeline.
- The current full packet-to-journal path on this workstation is approximately 55K-75K persisted traps/sec for the synthetic v2c profile-hit case, depending on run length and local I/O variability.
- The hot-path allocation cost is dominated by full ingestion work, not only the SDK writer, and needs a separate optimization pass if the merge target is materially above this range.

Committed v0.3.0 re-check in SOW-0039:

- The branch now pins `github.com/netdata/systemd-journal-sdk/go v0.3.0` in `src/go/go.mod`.
- The full-pipeline benchmark is committed as `BenchmarkFullPacketToJournal` in `src/go/plugin/go.d/collector/snmp_traps/benchmark_test.go`.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=30000x -count=3 -timeout 120s`
  - 30,000 packets/run: 16.97-18.54 us/op, 53.9K-58.9K packets/sec, 53.9K-58.9K persisted entries/sec, 1-5 dropped packets/run, 12,624-12,625 B/op, 206 allocs/op.
- `go test ./plugin/go.d/collector/snmp_traps -run '^$' -bench '^BenchmarkFullPacketToJournal$' -benchmem -benchtime=100000x -count=3 -timeout 180s`
  - 100,000 packets/run: 16.18-16.55 us/op, 60.4K-61.8K packets/sec, 60.4K-61.8K persisted entries/sec, 1 dropped packet/run, 12,580-12,581 B/op, 206 allocs/op.
- Current release-gate interpretation: the committed full packet-to-journal synthetic v2c profile-hit path is about 54K-62K persisted traps/sec on the workstation. This is acceptable for first-release merge if documented as measured local evidence, not a portable hardware guarantee.

## Regression Log

None yet.
