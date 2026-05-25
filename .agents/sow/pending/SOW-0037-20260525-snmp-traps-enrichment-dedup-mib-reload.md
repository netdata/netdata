# SOW-0037 - SNMP Trap Plugin: Cross-Plugin Enrichment + Opt-In Dedup + Profile YAML Hot-Reload + Per-OID Operator Metrics

## Status

Status: open

Sub-state: queued in `.agents/sow/pending/`. Depends on SOW-0036 completion. NOT independently mergeable — merge gate is SOW-0039.

## Requirements

### Purpose

Add operational depth on top of the per-job plugin shipped through SOW-0035–0036:

- Cross-plugin enrichment from go.d SNMP polling state (sysName → `_HOSTNAME`, vendor → `TRAP_DEVICE_VENDOR`) and topology plugin state (interface → `TRAP_INTERFACE`, neighbors → `TRAP_NEIGHBORS`); plus `ND_NIDL_NODE` from the source device's vnode identity.
- **Opt-in** per-job deduplication (default disabled per spec §10) with periodic summary entries; default key `(source_device, trap_OID)` only; profiles override per-OID via `dedup_key_varbinds:`.
- Profile YAML hot-reload via DynCfg — operators drop a new YAML into `/etc/netdata/go.d/snmp.trap-profiles/` and trigger a reload; NO runtime MIB compilation (operators convert MIBs offline per spec §7 + §14).
- Operator per-OID metric opt-in with bounded-cardinality enforcement.

Default health alert templates are OWNED BY SOW-0039 (collector consistency bundle), not this SOW.

### User Request

Sequential after SOW-0036 per the user-approved 5-SOW lineup. Per user: dedup is opt-in, users enable it and accept the loss (resolves the §2 forensic-store contradiction). Per user R22: copy snmp polling pattern for MIB handling (no runtime compilation).

### Assistant Understanding

Facts:

- Cross-plugin state access mechanism is decided in SOW-0035 M1 (in-process struct sharing for Go in-process with go.d snmp, or `netipc` otherwise).
- The OOB profile pack covers 351 vendor PENs; operators add custom vendor coverage by running `tools/snmp-traps-profile-gen/` offline and dropping the resulting YAML into the override directory.
- Per spec §10, dedup is per-job (source devices route to one job → dedup state stays local; no cross-job sync).
- Per spec §10, default `dedup_key_varbinds: []` means the fingerprint uses `(source_device, trap_OID)` only. Profile-level overrides per OID for narrower fingerprints (e.g., port-security `[macAddress, vlan]`).
- Per spec §10, when dedup is enabled, `snmp.trap.dedup_suppressed` is the per-job suppression-rate metric; when dedup is disabled (default), the metric is not emitted.
- Per spec §7.5, per-OID metric opt-in's `dimension_from_varbind` must reject unbounded-cardinality varbinds at config-load.

Inferences:

- Profile YAML hot-reload via DynCfg keeps the lifecycle simple — operators trigger a reload via DynCfg button or the plugin watches a `dyncfg:` notification, not inotify on the directory (inotify added complexity Phase B flagged as overkill).

Unknowns:

- Default dedup window (current spec suggests 5s; confirmed during validation).

### Acceptance Criteria

- M1: trap entries include the enrichment field set. `_HOSTNAME` is **always emitted** with a 3-tier fallback (sysName from SNMP polling state → reverse-DNS lookup → string form of `TRAP_SOURCE_IP`). `ND_NIDL_NODE`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS` are **omitted entirely** (not emitted, not empty) when SNMP polling / topology plugins have no state for the source device (per spec §13 resolved Q4). Dedup summary entries (cross-multiple-source) omit `_HOSTNAME` and `ND_NIDL_NODE` per spec §10.
- M2: per-job opt-in dedup (default disabled) suppresses flapping traps when enabled; default fingerprint `(source_device, trap_OID)` with profile override per-OID via `dedup_key_varbinds:`; periodic summary entries land in the same per-job journal with `TRAP_REPORT_TYPE=deduplication_summary`; `snmp.trap.dedup_suppressed` metric emits per job only when dedup is active.
- M3: dropping a new profile YAML into `/etc/netdata/go.d/snmp.trap-profiles/` and triggering DynCfg reload extends the OID index within ~5s without restart; newly recognized OIDs default to `category: unknown` until operator overrides; broken YAMLs are rejected at load with clear error + `snmp.trap.errors.profile_load_failed` increment; the previous index state is retained on failure.
- M4: operator can promote a specific OID to its own chart via `metrics:` config block; cardinality discipline rejects `dimension_from_varbind` referencing unbounded varbind at config-load.

## Milestones

### M1 — Cross-plugin enrichment (sysName / vendor / topology)

- Read sysName/vendor from go.d SNMP polling collector state.
- Read interface/neighbors from topology plugin state.
- Populate journal fields `_HOSTNAME` (source device hostname), `ND_NIDL_NODE` (source device vnode), `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS` on the journal path. When the OTLP exporter (SOW-0038) is enabled, populate the equivalent attributes per spec §11b: `snmp.device.hostname`, `snmp.device.vendor`, `netdata.topology.interface`, `netdata.topology.neighbors`, and `netdata.nidl.node`.
- Mechanism per SOW-0035 M1 decision (in-process struct sharing or `netipc`).
- Failure mode: enrichment unavailable → fields absent (per spec §13 Q4 resolution), no error counter (this is normal when polling/topology not co-located).

Cohort reference: spec §5 hot-path step 6 + spec §13 Q4 (resolution); `netdata-existing-snmp.md`; `netdata-existing-netipc.md`; user memory `project_netdata_snmp_hub_architecture`.

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M2 — Opt-in per-job dedup + summary entries

- Per-job in-memory dedup cache (LRU-bounded, default 100k entries, configurable per spec §7.5).
- Default disabled per spec §10. When `dedup.enabled: true` for a job:
  - Fingerprint `hash(source_device, trap_OID, key_varbinds)`.
  - `key_varbinds` default = `[]` → fingerprint uses `(source_device, trap_OID)` only. Profiles override per-OID via `dedup_key_varbinds:` (parsed by SOW-0035 M3 loader).
  - First-occurrence-in-window → emit journal entry normally + insert fingerprint with TTL.
  - Subsequent occurrences → skip journal write + `snmp.trap.dedup_suppressed` increment.
- Window expiry: separate background timer (default = dedup window length) emits one summary entry per period across all expired fingerprints (NOT one entry per fingerprint) with `TRAP_REPORT_TYPE=deduplication_summary` per spec §10.
- Summary entry MESSAGE is multi-line (binary-encoded per spec §11 CWE-117 protection).
- New self-metric: `snmp.trap.dedup_suppressed` (instance: per job; dimension `suppressed`; labels: `job_name`, `hub`) per spec §12 Context 3. Only emitted when at least one job has dedup enabled.

Cohort reference: spec §10; cohort dedup-survey across 16 Phase A systems.

Reviewers: 3 rotating (group B: glm/mimo/deepseek).

### M3 — Profile YAML hot-reload via DynCfg

- Operators drop a new/edited YAML into `/etc/netdata/go.d/snmp.trap-profiles/` and trigger reload via DynCfg button or function call.
- Reload mechanics: parse + validate + build new OID index → atomic swap (copy-on-write per spec §13 resolution Q9). Old index retained until swap completes.
- Failed YAML parse or validation: log error with file path + parse location, retain previous index state, increment `snmp.trap.errors.profile_load_failed`. The plugin continues operating with pre-existing coverage.
- Newly recognized OIDs default to `category: unknown` (per spec §3 + §13 Q2 default); operator overrides category via plugin config per-OID overrides.
- NO runtime MIB compilation (per spec §14 non-goal). Operators convert MIBs to YAMLs offline using `tools/snmp-traps-profile-gen/`; user documentation is owned by SOW-0039.

Cohort reference: `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go` (multipath + dedup pattern reused); spec §7 (Profile loading + Custom MIB workflow sections).

Reviewers: 3 rotating (group A: kimi/qwen/minimax).

### M4 — Per-OID operator metric opt-in

- `metrics:` config block per spec §7.5:
  - `oid` + `context` + optional `dimension_from_varbind`.
  - Single-dim counter fallback when no `dimension_from_varbind`.
- Bounded-cardinality enforcement at config-load: reject `dimension_from_varbind` referencing varbinds marked unbounded by the profile's cardinality table (MAC, IP, username, packet content). Clear error message naming the offending OID + varbind.
- Operator-opted-in metrics emit per spec §12 "Operator-opted-in per-OID metrics" section with naming convention `snmp.trap.<vendor>_<short_name>`.

Cohort reference: spec §7.5; spec §12.

Reviewers: 3 rotating (group B: glm/mimo/deepseek).

## Reviewer Protocol

- All 4 milestones: 3 rotating reviewers per round (alternating groups A/B).
- M2 (dedup correctness) may escalate to all 7 if reviewer round 1 surfaces correctness concerns about per-job partitioning.
- Fix-cycle: same reviewers as the round being fixed.

## Pre-Implementation Gate

Status: blocked

Reason: depends on SOW-0036 completion. Full gate filled at activation — depends on the metric universe + config schema delivered by SOW-0036.

## Plan

Sequential M1 → M4. Health alert templates are NOT in this SOW; they ship in SOW-0039 with the consistency bundle.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup: M2 dedup made opt-in (default disabled) with corrected default key per spec §10; M3 simplified to profile YAML hot-reload (no MIB compilation) per user R22; M5 health alerts removed (moved to SOW-0039). Not yet activated.

## Validation

Acceptance criteria evidence: pending.
Tests or equivalent validation: pending.
Real-use evidence: pending.
Reviewer findings: pending.
Same-failure scan: pending.
Sensitive data gate: pending.
Artifact maintenance gate: pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
