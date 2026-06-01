# SOW-0037 - SNMP Trap Plugin: Cross-Plugin Enrichment + Opt-In Dedup + Profile YAML Hot-Reload + Per-OID Operator Metrics

## Status

Status: completed

Sub-state: completed on 2026-06-01 as part of the SOW-0039 final close gate. M1-M4 landed on the feature branch, reverse DNS remains disabled by default and explicitly enabled per job, and final collector consistency/merge readiness was closed through SOW-0039.

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

- M1: trap entries include the enrichment field set. `_HOSTNAME` is **always emitted** with primary source-device identity from SNMP polling state (`sysName`, and explicit vnode hostname if configured), optional reverse-DNS enrichment only when enabled in job configuration, and string form of `TRAP_SOURCE_IP` as the required fallback. Reverse DNS is disabled by default and must not make trap job creation or packet handling depend on DNS availability. `ND_NIDL_NODE`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS` are **omitted entirely** (not emitted, not empty) when SNMP polling / topology plugins have no state for the source device (per spec §13 resolved Q4). Dedup summary entries (cross-multiple-source) omit `_HOSTNAME` and `ND_NIDL_NODE` per spec §10.
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

- Reviewers run on meaningful implementation batches, not on tiny line-level fixes. Each external review prompt must include this SOW filename and the whole changed batch under review.
- All 4 milestones: 3 rotating reviewers per round (alternating groups A/B, excluding unavailable models).
- M2 (dedup correctness) may escalate to all 7 if reviewer round 1 surfaces correctness concerns about per-job partitioning.
- Fix-cycle: same reviewers as the round being fixed.

## Pre-Implementation Gate

Status: ready for implementation; reverse-DNS decision D1 resolved before code changes.

### Problem / root-cause model

SOW-0035 and SOW-0036 established a per-job trap listener that writes one journal entry per accepted PDU and emits self-metrics. SOW-0037 adds enrichment, opt-in suppression, profile reload, and operator metrics. The root cause of the remaining gap is that SOW-0036 intentionally left these higher-level behaviours inert: `validateDeferredConfig()` rejects `dedup.enabled` and `metrics:` entries, the profile cache has single-generation refcounting, and `trapEntryFromPDU()` writes only packet/profile-derived fields.

### Evidence reviewed

- `src/go/plugin/go.d/collector/snmp_traps/init.go:254` rejects deferred SOW-0037 knobs; `init.go:258` rejects `dedup.enabled`; `init.go:261` rejects `metrics:`.
- `src/go/plugin/go.d/collector/snmp_traps/profile.go:211` documents the current single-generation profile cache and says SOW-0037 hot reload must replace it with generation-holder accounting.
- `src/go/plugin/go.d/collector/snmp_traps/load.go:80` loads all trap profile YAMLs into a shared `ProfileIndex`; `load.go:87` uses multipath directories; `load.go:96` implements filename dedup; `load.go:105` fails when no profiles load.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go:366` does trap OID lookup; `collector.go:373` creates the journal entry; `collector.go:377` writes it; no enrichment or dedup gate exists yet.
- `src/go/plugin/go.d/collector/snmp_traps/trapentry.go:49` already carries SOW-0037 fields (`DeviceHostname`, `DeviceVendor`, `SourceVnodeID`, `TopologyInterface`, `TopologyNeighbors`, `SummaryCounts`).
- `src/go/plugin/go.d/collector/snmp_traps/serialize.go:53` falls `_HOSTNAME` back to source IP/peer; `serialize.go:74` emits `ND_NIDL_NODE` only when non-empty; `serialize.go:107` to `serialize.go:115` omit vendor/interface/neighbors when empty; `serialize.go:124` to `serialize.go:129` already serializes dedup-summary counters.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:12` has `profileLoadFailed`; `metrics.go:150` emits events/errors only; dedup suppression and operator metrics are absent.
- `src/go/plugin/go.d/collector/snmp/topology_device_registry.go:42` registers SNMP polling device state into `ddsnmp.DeviceRegistry`; `topology_device_registry.go:61` to `topology_device_registry.go:72` includes sysObjectID, sysName, vendor, vnode GUID, and vnode labels.
- `src/go/plugin/go.d/collector/snmp/ddsnmp/device_registry.go:43` defines the global in-process device registry; `device_registry.go:54` and `device_registry.go:78` deep-copy reference fields and are safe for trap-side reads.
- `src/go/plugin/go.d/collector/snmp_topology/topology_registry.go:35` keeps the topology registry package-private; `topology_registry.go:55` exposes only an unexported snapshot method.
- `src/go/plugin/go.d/collector/snmp_topology/topology_dns.go:15` uses a 50ms reverse-DNS timeout and 10-minute cache; `topology_dns.go:136` documents a cached lookup path that never blocks on network I/O.
- `src/go/plugin/framework/charttpl/spec.go:94` supports `name_from_label`; `src/go/plugin/framework/jobruntime/job_v2.go:382` reads `ChartTemplateYAML()` after `Init()`, so trap jobs can generate chart YAML from validated per-job `metrics:` config.
- `.agents/sow/specs/snmp-traps/netdata.md:395` to `.agents/sow/specs/snmp-traps/netdata.md:399` defines opt-in dedup defaults (`enabled: false`, `window_sec: 5`, `cache_max_entries: 100000`).
- `.agents/sow/specs/snmp-traps/netdata.md:420` to `.agents/sow/specs/snmp-traps/netdata.md:432` defines per-OID operator metrics.
- `.agents/sow/specs/snmp-traps/netdata.md:543` to `.agents/sow/specs/snmp-traps/netdata.md:599` defines opt-in dedup, first-wins semantics, periodic summaries, and no dedup metric when disabled.

### Affected contracts and surfaces

- Journal fields: `_HOSTNAME`, `ND_NIDL_NODE`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS`, `TRAP_REPORT_TYPE`, `TRAP_SUPPRESSED_*`, `TRAP_JSON`.
- DynCfg apply semantics: config validation errors still surface at job creation time with 422, not at runtime.
- Function surface: profile reload is expected via an operator-triggered function because collector DynCfg commands are fixed (`add/update/remove/restart/test/schema/get`) in `src/go/plugin/agent/jobmgr/dyncfg_collector.go:73`.
- Metrics: existing contexts `snmp.trap.events` and `snmp.trap.errors`; new `snmp.trap.dedup_suppressed`; dynamic per-OID operator contexts from job config.
- Shared state: SNMP polling device registry; topology enrichment snapshot; trap profile cache generation holders.

### Existing patterns to reuse

- Reuse `ddsnmp.DeviceRegistry` for sysName/vendor/vnode enrichment instead of introducing external IPC for Go-in-process data.
- Add a narrow shared enrichment snapshot API for topology data rather than importing trap code into `snmp_topology` internals.
- Reuse profile loader multipath/filename-dedup semantics from SOW-0036 and add copy-on-write reload.
- Reuse chart-template `name_from_label` for per-OID dimensions and generate chart YAML from validated job config during `Init()`.
- Reuse the existing trap summary serialization path and systemd-journal binary field encoding for multi-line dedup summaries.

### Risk and blast radius

- Hot-path blocking risk: a synchronous reverse-DNS lookup can add up to 50ms for an uncached source IP, which is incompatible with the SOW-0036 ingestion target and trap listener design.
- Shared-state risk: topology and trap collectors run in the same process but different packages; exported APIs must deep-copy data and avoid exposing mutable internals.
- Reload risk: profile cache reload must not invalidate pointers being used by active listeners. New generations need holder accounting; failed reload must retain the previous generation.
- Dedup risk: enabling dedup intentionally loses individual duplicate PDUs. It must stay opt-in, off by default, and transparent through summary journal entries plus suppression metrics.
- Metric-cardinality risk: `dimension_from_varbind` must use the same bounded-cardinality checker as label validation, or operator metrics can explode series count.
- Documentation/collector-consistency risk: health alerts, metadata, taxonomy, README, and user docs remain owned by SOW-0039, but SOW-0037 must leave enough implementation evidence for that bundle.

Sensitive data handling plan:

No raw trap payloads, SNMP communities, USM keys, private IPs from real environments, customer identifiers, hostnames from real systems, or journal rows with live values will be written to SOWs, docs, skills, tests, or code comments. Tests will use synthetic addresses, synthetic engine IDs, synthetic trap OIDs, and generated profile snippets. Evidence in durable artifacts will cite files, line numbers, and redacted/synthetic examples only.

### Implementation plan

1. M1 enrichment: add a small enrichment resolver for SNMP polling state; add topology enrichment through a copied snapshot API; wire enrichment before message/label rendering so templates can use `_HOSTNAME`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, and `TRAP_NEIGHBORS`.
2. M2 dedup: validate dedup config, implement per-job LRU/TTL fingerprint cache, suppress duplicates only when enabled, emit one periodic summary entry, and add `snmp.trap.dedup_suppressed`.
3. M3 reload: replace profile cache with generation holders; add profile reload Function; atomic-swap only after full parse/validate/index succeeds; retain old generation and increment `profile_load_failed` on failure.
4. M4 operator metrics: validate `metrics:` against loaded profile metadata; generate job-specific chart template entries; emit counters on accepted non-suppressed traps; enforce bounded `dimension_from_varbind`.
5. Review externally after meaningful batches: M1+M2 together if cohesive, M3+M4 together if cohesive, then a closeout review before SOW-0039 merge gate.

### Validation plan

- Unit tests for enrichment fallback, omitted enrichment fields, summary-entry omission of `_HOSTNAME` / `ND_NIDL_NODE`, and template rendering after enrichment.
- Unit tests for dedup default-off, default `(source_device, trap_OID)` fingerprint, profile `dedup_key_varbinds`, missing-varbind sentinel, TTL expiry, LRU cap, summary entry contents, and cleanup timer shutdown.
- Unit tests for profile reload success, broken YAML rejection, duplicate OID rejection, previous-index retention, generation release, concurrent lookup under race detector, and `profile_load_failed` increment.
- Unit tests for `metrics:` validation, bounded/unbounded `dimension_from_varbind`, generated chart YAML, and emitted metric counters/dimensions.
- Focused validation commands: `go test ./plugin/go.d/collector/snmp_traps/...`; `go test ./plugin/go.d/collector/snmp/... ./plugin/go.d/collector/snmp_topology/... ./plugin/go.d/collector/snmp_traps/...`; race tests for the same packages; `go vet` for touched packages; `jq empty` on config schema if changed; `git diff --check`; `.agents/sow/audit.sh`.

### Artifact impact plan

- `AGENTS.md`: no expected workflow changes.
- Runtime project skills: update only if implementation discovers a durable trap-profile or collector workflow rule not already covered.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` for any decided reverse-DNS semantics or reload/API contract clarification.
- End-user/operator docs: owned by SOW-0039; SOW-0037 records implementation facts for that bundle.
- End-user/operator skills: no update expected until SOW-0039 docs/spec finalization.
- SOW lifecycle: SOW-0037 is current/paused; SOW-0035 and SOW-0036 remain paused/implementation-complete; SOW-0038 is current/in-progress and SOW-0039 remains pending.

### Decisions

#### D1 - Reverse DNS on the trap hot path

Evidence:

- Acceptance criterion M1 currently says `_HOSTNAME` falls back `sysName -> reverse-DNS lookup -> TRAP_SOURCE_IP`.
- The topology collector already treats reverse DNS as potentially blocking: `src/go/plugin/go.d/collector/snmp_topology/topology_dns.go:15` sets a 50ms timeout, and `topology_dns.go:136` provides a cached lookup path that "never blocks on network I/O".
- The trap listener writes entries directly from packet handling in `src/go/plugin/go.d/collector/snmp_traps/collector.go:283` to `collector.go:377`; blocking here would delay packet processing and reduce ingestion throughput.

Options considered:

- A. Strict synchronous lookup before source-IP fallback.
  - Pros: literal reading of the current acceptance criterion.
  - Cons: uncached sources can block the packet path up to the DNS timeout.
  - Implication: a DNS slowdown can make trap ingestion appear unhealthy.
  - Risk: contradicts the hot-path performance design.
- B. Non-blocking cached/async lookup.
  - Pros: preserves ingestion throughput; `_HOSTNAME` is always emitted; sysName wins; cached PTR wins when available; otherwise source IP is used while a bounded async resolver warms the cache.
  - Cons: the first trap from a source may use source IP even if PTR would resolve moments later.
  - Implication: reverse DNS becomes best-effort enrichment, not a per-trap blocking dependency.
  - Risk: weaker than the current literal acceptance text, so the spec must be clarified.
- C. No reverse DNS fallback.
  - Pros: simplest and fastest.
  - Cons: removes a documented enrichment tier.
  - Implication: `_HOSTNAME` is sysName or source IP only.
  - Risk: less operator-friendly when SNMP polling is not co-located.

Decision: reverse DNS is optional enrichment, disabled by default, enabled explicitly in job configuration, and never required for trap job creation or packet handling. Primary hostname identity comes from SNMP polling state (`sysName`, and explicit vnode hostname if configured); source IP remains the mandatory fallback when no SNMP/topology identity is available. The SNMP helper currently defaults missing `sysName` to literal `unknown`; trap enrichment must treat empty or `unknown` names as unresolved, not as a valid hostname.

Implementation implication: add a job config knob for reverse DNS, default `false`. When enabled, use only non-blocking cached/async resolution on the trap path; DNS failures must not make a successfully bound/listening trap job unhealthy after creation.

## Plan

Sequential M1 → M4. Health alert templates are NOT in this SOW; they ship in SOW-0039 with the consistency bundle.

## Execution Log

### 2026-05-25

SOW rewritten under the 5-SOW lineup: M2 dedup made opt-in (default disabled) with corrected default key per spec §10; M3 simplified to profile YAML hot-reload (no MIB compilation) per user R22; M5 health alerts removed (moved to SOW-0039). Not yet activated.

### 2026-05-26 — M1 batch 1: cross-plugin enrichment + reverse DNS semantics

Implementation completed (first meaningful M1 batch). Changes:

**Files modified:**

1. `src/go/plugin/go.d/collector/snmp/ddsnmp/device_registry.go` — Added `VnodeHostname` field to `DeviceConnectionInfo` so trap enrichment can prefer explicitly configured vnode hostnames without parsing vnode labels. Added `DeviceByHostname()` backed by a normalized hostname index so trap enrichment avoids an O(n) registry scan on every trap. IP literals are normalized before comparison; DNS names match case-insensitively. Register/unregister maintain the index under the same registry mutex and return deep-copied device records.

2. `src/go/plugin/go.d/collector/snmp/topology_device_registry.go:31-35,72` — Added `vnodeHostname()` method on the SNMP `Collector`; populated `VnodeHostname` in `registerDeviceForTopology` from `c.vnode.Hostname`.

3. `src/go/plugin/go.d/collector/snmp_traps/config.go:52-58,119` — Added `ReverseDNSConfig` type with `Enabled bool`; added `ReverseDNS` field to `Config`; added `reverse_dns` with `enabled` child to `configYAMLSpec`.

4. `src/go/plugin/go.d/collector/snmp_traps/collector.go:52-70,221-226,382` — Added `reverseDNS` and `reverseDNSEnabled` fields to `Collector`; initialised reverse DNS resolver in `Init()` when enabled; wired `enrichTrapEntry()` call into `handlePacket`. Trap template rendering now runs after enrichment so `{_HOSTNAME}`, `{TRAP_DEVICE_VENDOR}`, `{TRAP_INTERFACE}`, and `{TRAP_NEIGHBORS}` see enriched values.

5. `src/go/plugin/go.d/collector/snmp_traps/enrich.go` (NEW) — Enrichment resolver:
   - `reverseDNSResolver`: cached non-blocking PTR lookup (50ms timeout, 10min TTL, 30s negative TTL, 10k-entry hard cap). `lookupCached()` is the hot-path API (zero blocking I/O). `resolveAsync()` fires at most one background goroutine per unresolved IP while a lookup is pending, and uses a resolver-owned cancellable context so `Collector.Cleanup()` stops in-flight lookups.
   - Reverse DNS cache maintenance: expired entries are swept from the collector `Collect()` path and after async results; cache is trimmed to the hard cap so spoofed-source or very large deployments cannot grow memory without bound when reverse DNS is enabled. Hard-cap trimming evicts negative entries first, then oldest positive entries by expiry.
   - `isUnresolvedSysName()`: treats empty strings and literal `"unknown"` (case-insensitive) as unresolved.
   - `resolveDeviceEnrichment()`: looks up source IP with `ddsnmp.DeviceRegistry.DeviceByHostname()`; extracts `VnodeHostname` > `SysName` (first non-unresolved); populates `DeviceVendor` and `VnodeGUID` when available.
   - `enrichTrapEntry()`: applies enrichment to a `TrapEntry`; also queries `snmptopology.TrapEnrichmentForIP()` for topology-matched `SysName`, vendor, vnode ID, interface, and neighbor enrichment. Reverse DNS is consulted only after SNMP registry and topology identity are empty, and only when explicitly enabled.

6. `src/go/plugin/go.d/collector/snmp_topology/topology_trap_enrich.go` (NEW) — Exported `TrapTopologyEnrichment` struct and `TrapEnrichmentForIP()` function that queries the topology registry for topology-matched source device `SysName`, vendor, vnode ID, interface name (by normalized IP match on `ifIndexByIP`), and sorted LLDP/CDP neighbor sysNames. Returns nil when no matching topology state exists. Neighbor enrichment is intentionally device-level context for the source-device cache, not event-interface-specific context, because trap source IP is normally the device source/management address and the trap event interface, when any, is carried in trap varbinds/profile templates.

**Files created:**

7. `src/go/plugin/go.d/collector/snmp_traps/enrich_test.go` (NEW) — 24 focused tests:
   - Hostname priority: VnodeHostname > SysName
   - SNMP registry hostname/VnodeID/vendor priority over topology and reverse DNS, while topology-only interface/neighbor fields still fill from topology
   - "unknown" SysName treated as unresolved (case-insensitive, whitespace-trimmed)
   - Empty SysName skipped
   - No device registry match (fields remain empty)
   - SourceUDPPeer fallback
   - Nil entry safety
   - Reverse DNS default-off (cache not consulted when disabled)
   - Reverse DNS enabled with cached DNS name
   - Reverse DNS disabled skips cache
   - Reverse DNS does not schedule PTR lookup when SNMP/vnode hostname is known
   - Reverse DNS enabled schedules exactly the async lookup path and populates cache via an injected resolver function
   - Reverse DNS sweeps expired entries and enforces the cache cap
   - Reverse DNS hard-cap trimming prefers negative and oldest entries
   - Resolver close prevents later async DNS scheduling
   - Vendor and vnode ID enrichment (set, empty, both)
   - `isUnresolvedSysName` table-driven tests
   - `reverseDNSResolver.lookupCached` / nil resolver / invalid IP / async skip / pending suppression
8. `src/go/plugin/go.d/collector/snmp/ddsnmp/device_registry_test.go` (NEW) — 4 tests validating normalized IP matching, deep-copy isolation, no-match behavior, case-insensitive DNS hostname matching, and index updates on register/unregister for `DeviceByHostname()`.

9. `src/go/plugin/go.d/collector/snmp_topology/topology_trap_enrich_test.go` (NEW) — 5 tests validating topology trap interface/neighbor enrichment, no-match omission, local-management-IP neighbor enrichment without interface match, topology-matched local device identity, and the exported global-registry path. The primary enrichment test now includes neighbors on multiple source-device interfaces and asserts the documented device-level `TRAP_NEIGHBORS` behavior. The exported-path test also verifies IPv4-mapped IPv6 source normalization (`::ffff:192.0.2.20`).

**Spec update:**

10. `.agents/sow/specs/snmp-traps/netdata.md:666-675` — Updated `_HOSTNAME` documentation with complete hostname priority order (SNMP registry VnodeHostname > SNMP registry SysName > topology-matched SysName > reverse DNS cached > SourceIP) and description of the non-blocking reverse DNS mechanism.

11. `.agents/sow/specs/snmp-traps/netdata.md:392` — Added `reverse_dns.enabled` to the plugin configuration example.

12. `.agents/sow/specs/snmp-traps/netdata.md:1135` — Updated `DeviceHostname` row in TrapEntry shape table with hostname priority.

13. `.agents/sow/specs/snmp-traps/netdata.md:768-770` — Documented `TRAP_INTERFACE` as the topology interface owning the trap source IP and `TRAP_NEIGHBORS` as sorted device-level LLDP/CDP neighbor sysNames, not a per-event-interface claim.

14. `src/go/plugin/go.d/collector/snmp_topology/topology_trap_enrich.go` — Allows device-level `TRAP_NEIGHBORS` enrichment when the source IP matches the topology cache's local device management IP / management addresses even if no `ifIndexByIP` interface mapping exists. `TRAP_INTERFACE` remains omitted in that case.

15. `src/go/plugin/go.d/collector/snmp_traps/resolver.go` and `src/go/plugin/go.d/collector/snmp_traps/profile_test.go` — Aligned template `{_HOSTNAME}` fallback with journal serialization: `DeviceHostname` > `SourceIP` > `SourceUDPPeer`; added coverage to `TestRenderMessageHostnameFallback`.

16. `src/go/plugin/go.d/collector/snmp_traps/config_schema.json` and `src/go/plugin/go.d/config/go.d/snmp_traps.conf` — Added the `reverse_dns.enabled` knob to the dashboard schema and stock config comments, defaulting to disabled.

17. `src/go/plugin/go.d/collector/snmp_traps/pipeline.go` and `src/go/plugin/go.d/collector/snmp_traps/pipeline_test.go` — Split trap entry construction from template rendering. `SourceVnodeID` is no longer seeded from the listener/job vnode; it is emitted only when enrichment matches source-device state. Removed the obsolete vnode parameter from `trapEntryFromPDU()`. Added packet-path tests proving enriched template rendering, topology-derived identity before reverse DNS, and omission of listener vnode from `ND_NIDL_NODE`.

**Validation:**

- `go test ./plugin/go.d/collector/snmp_traps/...` — PASS (all 80+ tests)
- `go test ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go test ./plugin/go.d/collector/snmp/...` — PASS (all subpackages)
- `go test -race -count=1 ./plugin/go.d/collector/snmp_traps/...` — PASS
- `go test -race -count=1 ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go vet` on all affected packages — PASS (no issues)
- `git diff --check` — PASS (no whitespace errors)
- `.agents/sow/audit.sh` — PASS structurally; existing non-project skill classification warnings remain unrelated to this SOW.

**Unresolved:**

- `DeviceVendor` / `SourceVnodeID` / `TopologyInterface` / `TopologyNeighbors` are populated only when state exists; fields are omitted from journal output when empty (serializer already handles this).
- Reverse DNS lookups use a 50ms timeout, one in-flight lookup per IP, and collector cleanup cancellation. Cache memory is bounded by TTL sweep plus 10k-entry hard cap.
- Profile reload (M3) and per-OID operator metrics (M4) remain unimplemented — `validateDeferredConfig()` still rejects `metrics:`.

### 2026-05-26 — M2 batch 1: opt-in per-job deduplication

Implementation completed (first M2 batch). Changes:

1. `src/go/plugin/go.d/collector/snmp_traps/dedup.go` (NEW) — Added `trapDeduper`, enabled only when `dedup.enabled: true`. It computes SHA-256 fingerprints from source-device identity, trap OID, and optional key varbinds; default key varbinds are empty so the default fingerprint is `(source_device, trap_OID)`. Source-device identity priority is source vnode ID, source IP, UDP peer, then hostname. Profile `dedup_key_varbinds` override job-level `dedup.key_varbinds`; missing key varbinds use a sentinel distinct from empty string.
2. `dedup.go` — Added TTL expiry with default 5s window, bounded cache with default 100k entries, oldest-entry eviction at cap, and per-period suppression accounting by trap OID and unique fingerprint.
3. `dedup.go` / `collector.go` — Added a per-job timer that emits one `TRAP_REPORT_TYPE=deduplication_summary` journal entry per period only when suppressions occurred. `Collector.Cleanup()` closes the listener first, flushes any pending dedup summary while the writer is still open, then closes the writer.
4. `collector.go` — Wired dedup after enrichment/template rendering and before journal write/event metrics. Pipeline-health counters (`unknown_oid`, `template_unresolved`) increment before the dedup gate so coverage/template failures remain visible at received-PDU volume. First occurrence writes normally; duplicate occurrence returns before journal write and before per-event metric increments. If the first occurrence write fails, the inserted fingerprint is rolled back so later traps are not suppressed behind a failed write.
5. `metrics.go` / `charts.yaml` — Added conditional `snmp_trap_dedup_suppressed` metric with `job_name` label. The metric is emitted only for jobs with dedup enabled; richer hub/source labels remain a SOW-0039 collector-consistency target if a stable identity is available.
6. `init.go`, `config_schema.json`, `snmp_traps.conf` — Added creation-time validation for dedup config, removed `dedup.enabled` from deferred-feature rejection, updated the schema description, and documented the opt-in stock-config block.
7. `src/go/plugin/go.d/collector/snmp_traps/dedup_test.go` (NEW) — 12 focused tests covering validation, default fingerprint suppression, source-device priority, profile and numeric-OID key-varbind narrowing, missing-varbind sentinel behavior including literal `<missing>`, TTL expiry, cache cap eviction, summary entry shape/omission of `_HOSTNAME` and `ND_NIDL_NODE`, cleanup flush/timer stop, periodic timer summary emission, and conditional metric emission.
8. `src/go/plugin/go.d/collector/snmp_traps/pipeline_test.go` — Added packet-path coverage proving dedup suppresses duplicate accepted traps, writes only the first occurrence, increments `snmp_trap_dedup_suppressed`, preserves health/error counters for suppressed duplicates, rolls back fingerprints after first-write failure, and increments the event counter only for journaled first occurrences.
9. `.agents/sow/specs/snmp-traps/netdata.md` — Updated Context 3 label contract to match the implemented SOW-0037 M2 metric surface (`job_name` only), with the richer `hub` label explicitly left to SOW-0039 if available.

Initial validation:

- `go test ./plugin/go.d/collector/snmp_traps/... ./plugin/go.d/collector/snmp_topology/... ./plugin/go.d/collector/snmp/...` — PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/go.d/collector/snmp_topology/... ./plugin/go.d/collector/snmp/...` — PASS
- `python3 -m json.tool plugin/go.d/collector/snmp_traps/config_schema.json >/dev/null` — PASS

## Validation

Acceptance criteria evidence: M1 enrichment implemented and tested. `_HOSTNAME` always emitted with source-IP fallback. SNMP registry VnodeHostname > SNMP registry SysName > topology-matched SysName > optional reverse DNS priority enforced. Empty/unknown sysName treated as unresolved. Reverse DNS disabled by default, non-blocking when enabled. ND_NIDL_NODE, TRAP_DEVICE_VENDOR, TRAP_INTERFACE, TRAP_NEIGHBORS omitted when no state. M2 opt-in dedup implemented and tested: default disabled; enabled jobs write first occurrence immediately; duplicates inside the window are suppressed; profile `dedup_key_varbinds` narrow the fingerprint; periodic summary entries emit `TRAP_REPORT_TYPE=deduplication_summary` and omit `_HOSTNAME`/`ND_NIDL_NODE`; `snmp_trap_dedup_suppressed` emits only for dedup-enabled jobs.

Tests or equivalent validation: 24 focused trap enrichment tests, 12 focused dedup tests, 4 device-registry tests, 5 topology trap-enrichment tests, enriched packet-template rendering coverage, SNMP-over-topology-over-reverse-DNS priority coverage, topology-before-reverse-DNS packet-path coverage, listener-vnode omission coverage, dedup packet-path suppression/health-counter/write-rollback coverage, and template `_HOSTNAME` SourceUDPPeer fallback coverage pass. Full snmp_traps, snmp_topology, snmp, and ddsnmp suites pass. Race detector clean on snmp_traps and snmp_topology. go vet clean. go build clean. git diff --check clean. SOW audit clean except pre-existing skill-classification warnings.

Real-use evidence: enrichment priority and topology fallback are covered by focused unit tests across SNMP registry, topology IP state, optional reverse DNS, and source-IP fallback. Co-located SNMP polling plus trap receipt on a live node remains useful lab evidence and is tracked in `.agents/sow/pending/SOW-0046-20260601-snmp-traps-real-use-validation-lab.md`.

Reviewer findings:
- First review round received from glm, kimi, and qwen. Minimax session was no longer attached before completion; the next review round will rerun minimax with the same full scope.
- Accepted and fixed: reverse DNS cache could grow without bound when enabled. Added TTL sweep, 10k-entry hard cap, tests, and `Collect()` maintenance.
- Accepted and fixed: reverse DNS async lookup was scheduled even when SNMP/vnode hostname was already known. Now PTR lookup is scheduled only when no hostname exists.
- Accepted and fixed: exported topology enrichment path lacked coverage. Added global-registry test.
- Reviewed and rejected as a code bug: qwen requested per-interface filtering for `TRAP_NEIGHBORS`. Evidence: trap source IP maps to the device source/management interface via `ifIndexByIP`, while the trap event interface is carried by trap varbinds/profile templates when present. Filtering neighbors by source-IP-owning interface would often return management-interface neighbors, not the event interface. Spec and tests now explicitly define `TRAP_NEIGHBORS` as device-level co-located topology context.
- Second review round received from glm, kimi, and minimax. Qwen's session detached before final output; rerun is planned after these fixes.
- Accepted and fixed: hard-cap DNS trim was arbitrary. It now evicts negative entries first, then oldest positive entries by expiry.
- Accepted and fixed: in-flight DNS goroutines had no cleanup signal. Resolver now owns a cancellable context and `Collector.Cleanup()` closes it.
- Accepted and fixed: `DeviceByHostname()` was an O(n) hot-path registry scan. The registry now maintains a normalized hostname index under the same mutex.
- Accepted and fixed: template `{_HOSTNAME}` fell back to `SourceIP` but not `SourceUDPPeer`, while journal serialization used both. Template resolution now matches serialization.
- Accepted and fixed: topology device-level neighbor enrichment was gated entirely on `ifIndexByIP`. It now also matches the topology cache's local device management IP/management addresses, emitting neighbors while omitting `TRAP_INTERFACE` when no interface mapping exists.
- Reviewed and rejected as a code bug: claimed `net.ParseIP("::ffff:192.0.2.10").String()` preserves IPv4-mapped IPv6 form. Local Go check and new topology test show it normalizes to `192.0.2.10`, matching the `netip.ParseAddr(...).Unmap()` path.
- Reviewed and rejected as a config bug: claimed `reverse_dns` in stock config was uncommented/top-level. `src/go/plugin/go.d/config/go.d/snmp_traps.conf:33-36` shows the block is fully commented inside the sample job.
- Reviewed and rejected as a data-race bug: claimed `lastSweep` was read outside the resolver mutex. Current `reverseDNSResolver.maybeSweep()` acquires `r.mu.Lock()` before reading `r.lastSweep`; race detector remains clean.
- User decision recorded and implemented: reverse DNS remains opt-in and disabled by default; hostname identity priority is SNMP registry `vnode.hostname`, SNMP registry `sysName`, topology-matched `sysName`, optional reverse DNS, then source IP.
- Accepted and fixed: trap templates were rendered before enrichment, so enriched special variables in `MESSAGE`/labels could see source IP instead of source-device identity. Entry construction and template rendering are now separate, and packet-path tests prove templates render after enrichment.
- Accepted and fixed: `SourceVnodeID` was seeded from the listener/job vnode. It now comes only from matched source-device state, so `ND_NIDL_NODE` is omitted when no source device is known.
- Accepted and fixed: packet-path coverage only proved SNMP-registry enrichment, not topology identity before reverse DNS. Added packet-path coverage using an injected topology enrichment function and a pre-seeded reverse DNS cache to prove topology identity wins over DNS for rendered templates and `SourceVnodeID`.
- Accepted and fixed: the async reverse DNS scheduling path had no deterministic test. `reverseDNSResolver` now accepts an injected `lookupAddr` function and tests verify the pending state and cache fill without live DNS.
- Accepted and fixed: `trapEntryFromPDU()` still accepted the listener vnode argument after `SourceVnodeID` moved to enrichment-only. Removed the obsolete parameter and updated the call site.
- Reviewed and not treated as a blocker: direct SNMP registry lookup is keyed by the configured SNMP target, so DNS-configured SNMP jobs may not match trap source IP unless topology has matching IP state. This is the user-approved reason topology identity is now in the priority chain before reverse DNS. Adding new SNMP-registry aliases would require a resolved target-IP contract from the SNMP client or new DNS work in the SNMP collector; neither exists in the current code path, and no DNS lookup is added to the trap hot path.
- Third review round received from glm and minimax; qwen ran local checks but did not produce a final review after several minutes of silence, so the exact qwen `timeout`/`opencode` PIDs were stopped.
- glm: no blocking issues; noted only residual design observations around lock count, GoSNMP error-message classification, bounded rate-limiter allocation, and global registry use in tests. Reviewed as non-blocking because the hot path uses existing registry/topology read locks, decode-error classification is covered by current tests, rate-limiter memory is capped, and tests do not use `t.Parallel()`.
- minimax: no blocking issues; requested explicit coverage that SNMP registry hostname wins when topology and reverse DNS also have identities. Accepted and fixed with `TestEnrichTrapEntryRegistryHostnameWinsOverTopologyAndReverseDNS`.
- minimax residual risks reviewed and not treated as blockers: unbounded `TRAP_NEIGHBORS` and rendered label value length are journal fields, not metric dimensions, and the spec deliberately allows high-cardinality/free-form journal content while enforcing bounded-cardinality label templates at profile/config load. Reverse DNS negative-entry churn is bounded behind an opt-in feature by one in-flight lookup per IP, short timeout, negative TTL, and hard cache cap.
- M2 review round 1 received from minimax, glm, and kimi. Qwen's first session detached before final output. Accepted and fixed: summary `_HOSTNAME`/`ND_NIDL_NODE` omission is now enforced explicitly by serializer summary gating; periodic timer summary path is tested; packet-path dedup test starts/stops the deduper; config schema documents and accepts the Go `0 means default` convention; `unknown_oid` and `template_unresolved` increment before the dedup gate; fingerprinting uses typed length-prefixed hash parts instead of JSON marshaling; missing-varbind tests cover empty string, missing, and literal `<missing>`.
- M2 review round 2 received from glm, kimi, and minimax. Qwen read files and ran local tests but produced no final review after more than 10 minutes without new output, so the exact qwen `timeout`/`opencode` PIDs were stopped. Accepted and fixed: schema indentation; stale SOW validation wording; write-failure rollback test; numeric-OID key-varbind test; source-device priority test.
- M2 residual risks reviewed and not treated as blockers: dedup source identity can become richer between two traps inside one window, causing a false-negative dedup and one extra journal row rather than false-positive suppression; summary write failure resets the best-effort period counters but increments `journal_write_failed`; spoofed-source cache churn can evict old entries but only reduces dedup effectiveness and remains bounded by cache size/rate-limit policy.

Same-failure scan: no suspicious failures.

Sensitive data gate:
No raw trap payloads, SNMP communities, USM keys, private IPs from real environments, customer identifiers, or hostnames from real systems in SOWs, docs, skills, tests, or code comments. Tests use synthetic addresses (10.x, 172.16.x, 192.168.x), synthetic vnode GUIDs, and synthetic vendor names.

Artifact maintenance gate:
- AGENTS.md: no workflow changes needed.
- Runtime project skills: no durable rules discovered.
- Specs/config artifacts: updated `.agents/sow/specs/snmp-traps/netdata.md`, `config_schema.json`, and `snmp_traps.conf` with hostname priority, reverse DNS semantics, and opt-in dedup metric/config semantics.
- End-user/operator docs: owned by SOW-0039; recorded implementation facts in this SOW log.
- End-user/operator skills: no update expected until SOW-0039.
- SOW lifecycle: M1 enrichment and M2 dedup complete on the feature branch; M3 (profile reload) complete; M4 (per-OID metrics) remains pending.

### 2026-05-26 — M3 batch 1: Profile YAML hot-reload via DynCfg

Implementation completed and corrected for the original memory requirement. Changes:

**Core changes:**

1. `src/go/plugin/go.d/collector/snmp_traps/profile.go` — Replaced the single-generation cache pointer with a shared cache that keeps active-job refcounting and exposes the current immutable `ProfileIndex` through `atomic.Pointer`. `AcquireProfileCache()` lazily loads on first job creation and increments the active reference count. `ReleaseProfileCache()` decrements the count and unloads the shared pointer when the last listener stops, so users without active SNMP trap jobs do not retain profile memory. `ReloadProfileCache()` refuses to load without an active job, parses/validates/builds a full replacement index before swapping, preserves the previous index on failure, and increments `profile_load_failed` for active jobs on load/validation failure.

2. `src/go/plugin/go.d/collector/snmp_traps/collector.go` — Removed per-collector `profileIndex`/generation storage. `Init()` still detects profile load failures at job creation time, releases the profile reference on journal/listener/V3 preflight failures, and records a held reference only after successful setup. `Cleanup()` releases the reference after stopping packet and dedup work. `handlePacket()` reads `CurrentProfileIndex()` for each packet so running listeners see a successful reload without restart.

3. `src/go/plugin/go.d/collector/snmp_traps/dedup.go` — Removed the deduper's cached profile pointer. Dedup summary name rendering reads `CurrentProfileIndex()`, so summaries also use the reloaded profile names.

4. `src/go/plugin/go.d/collector/snmp_traps/metrics.go` — Added `incAllJobsProfileLoadFailed()` to increment the reload failure counter across registered active jobs.

**Function surface:**

5. `src/go/plugin/go.d/collector/snmp_traps/reload.go` (NEW) — Registered the agent-wide `reload-profiles` Function through `collectorapi.Creator.Methods` + `MethodHandler`. Success returns status `200`. Profile parse/validation failures return `422`, retain the previous index, increment `profile_load_failed`, and log the loader error through the dispatching collector; the loader error includes the profile file path and validation/parse detail. Reload without an active job returns `503`.

6. `src/go/plugin/go.d/collector/snmp_traps/collector.go` — Updated collector registration to publish `snmpTrapsMethods` and `snmpTrapsMethodHandler`.

**Tests:**

7. `src/go/plugin/go.d/collector/snmp_traps/reload_test.go` (NEW) — 19 focused tests cover successful reload, broken YAML retain-old-index behavior, duplicate OID retain-old-index behavior, empty directory failure, unknown category handling for newly added OIDs, error metric increment for active jobs, concurrent reload/lookups under the race detector, nil current index before first load, refusal to reload without an active job, last-job-exits-during-reload success/failure races, packet-path use of a reloaded profile by an already-created collector, dedup summary use of a reloaded profile name, handler success/failure/unavailable/unknown-method behavior, method params, and method registration shape.

8. `src/go/plugin/go.d/collector/snmp_traps/profile_test.go` — Updated cache tests for the new two-value `AcquireProfileCache()` signature and no-arg `ReleaseProfileCache()`. Tests now prove last-release unloading, idempotent release, shared cache retention while another collector is active, last-collector cleanup unloading, and bind-failure release.

9. `src/go/plugin/go.d/collector/snmp_traps/pipeline_test.go` — Updated direct packet-path tests to use a test helper that stores an immutable profile index in the shared atomic pointer. Updated dedup test constructor calls for the new `newTrapDeduper()` signature.

10. `src/go/plugin/go.d/collector/snmp_traps/dedup_test.go` — Updated `newTrapDeduper()` calls and the summary test to resolve profile names through the shared current index.

**Spec update:**

- `.agents/sow/specs/snmp-traps/netdata.md` now records the accepted `reload-profiles` Function contract, failure behavior, active-job requirement, lazy memory model, and no-runtime-MIB-compilation constraint.

**Validation:**

- `go test -count=1 ./plugin/go.d/collector/snmp_traps/...` — PASS
- `go test -count=1 ./plugin/go.d/collector/snmp/... ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go test -race -count=1 ./plugin/go.d/collector/snmp_traps/...` — PASS
- `go test -race -count=1 ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/go.d/collector/snmp/... ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go build ./plugin/go.d/collector/snmp_traps/...` — PASS
- `git diff --check` — PASS
- `.agents/sow/audit.sh` — PASS with the existing skill-classification warning for 10 non-project skill directories.

**Residual risks:**
- A successful reload temporarily keeps the old immutable index reachable until any in-flight packet handling or dedup summary render drops its local pointer; Go GC then collects it.
- The `reload-profiles` Function is agent-wide but still dispatches through a running `snmp_traps` job because that is the current function framework contract.
- M4 (per-OID operator metrics) was still pending at M3 close; the following M4 batch removes the `metrics:` deferred rejection.

**Review round 1:**
- External reviewers: glm, kimi, minimax, qwen. No blocker found. All reviewers reported the implementation satisfies M3 and preserves M1/M2 behavior.
- Accepted and fixed from glm/qwen: reload failure after the last active job exits now returns `errNoActiveProfileJobs` without updating stale failure metrics; new deterministic tests cover successful and failed mid-load last-release races.
- Accepted and fixed from glm/qwen: concurrent lookup test now asserts successful lookups instead of only exercising the race detector.
- Accepted and fixed from glm: test helpers that seed `globalProfileCache.current` directly now document that this is a test-only shortcut for packet-path tests that do not run `Collector.Init()`.
- Accepted and fixed from kimi: removed the effectively unreachable `profileCache.loadErr` field.
- Reviewed and not changed: concurrent reload calls are not serialized; latest completed reload wins. This wastes work if an operator triggers duplicate reloads, but it does not corrupt state and is outside the hot path.
- Reviewed and rejected: storing a successfully parsed index when all trap jobs have stopped during reload would violate the explicit memory requirement that profile data is released when no trap jobs are active.

**Review round 2:**
- External reviewers completed: glm, kimi, minimax, qwen.
- No blocker found by completed reviewers. Re-run reviewer validation included `go test -race -count=1 ./plugin/go.d/collector/snmp_traps/...`, focused `snmp_traps` tests, and `go vet ./plugin/go.d/collector/snmp_traps/...`.
- Accepted documentation clarification from glm/kimi: `.agents/sow/specs/snmp-traps/netdata.md` now records that reload without an active `snmp_traps` job returns unavailable and that current agent-wide Function dispatch still routes through one running job instance while the reload itself applies to the shared cache.
- Reviewed and not changed: test-only direct `globalProfileCache.current` seeding remains limited to packet-path tests that intentionally do not run `Collector.Init()`; the tests now explain this local invariant.
- Reviewed and rejected as false positive: failed jobs do not leave reload-incremented metrics in this M3 path because `getJobMetrics(c.jobName)` is reached after all fallible preflight setup, and `profileCacheHeld` is only marked true after successful setup.
- Reviewed and rejected as false positive: first lazy profile load and reload cannot race on an unloaded cache in a way that corrupts state. `AcquireProfileCache()` holds the shared mutex while loading the first index, while `ReloadProfileCache()` refuses to load with `activeRefs == 0` and rechecks active refs before storing.
- Reviewed and rejected as false positive: `incAllJobsProfileLoadFailed()` cannot increment metrics for unrelated go.d collectors because `globalMetrics` is package-local to `collector/snmp_traps`; it only contains SNMP trap listener jobs.

### 2026-05-26 — M4 batch 1: Per-OID operator metric opt-in

Implementation completed. Changes:

**New files:**

1. `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go` — Validation, chart template generation, and runtime counter tracking:
   - `validateMetrics(metrics, profileIndex)`: validates each metric config at job creation time (after profiles loaded, before writer/listener setup). Validates oid required/numeric/in-profile-index (with SMIv1/v2 alternate lookup), rejects duplicate raw or alternate-resolved OIDs, validates context required/starts-with-snmp.trap./sane-suffix/no empty dot segments/no duplicates/no normalized selector collisions, and validates dimension_from_varbind references a known trap varbind by symbolic name and passes bounded-cardinality check. Raw numeric OIDs, unknown names, oversized enums, and unbounded types fail with clear errors naming the metric index, trap OID, and varbind.
   - `operatorMetrics`: thread-safe runtime counters. Single-dimension metrics use `atomic.Uint64`. Dimension-from-varbind metrics use `sync.Mutex`-protected `map[string]*atomic.Uint64`. `inc()` is safe to call from the hot path (nil-safe, OID-lookup via map, lock held only for map insert/lookup on dimension counters) and requires the current packet path to have a known trap definition from the current profile index. `collect()` snapshots dimension counts and observes cumulative values through `metrix.SnapshotCounter.ObserveTotal` with `varbind_value` label for `name_from_label` dimension resolution.
   - Runtime dimension values remain bounded even for malformed sender values or profile-reload metadata drift. Each metric stores a job-creation-time bound, including frozen enum key-to-label mappings. Unknown enum values collapse to `unknown`, numeric values outside the job-creation-time bounded range collapse to `out_of_range`, missing configured varbinds collapse to `<missing>`, and a reloaded varbind definition that no longer matches a bounded type collapses to `unknown`.
   - `buildChartTemplateYAML(metrics)`: decodes the embedded base `charts.yaml` through `charttpl.DecodeYAML`, appends operator metric charts with official `charttpl` types, validates, and marshals to YAML. Each operator metric becomes a chart in the `traps` group with `context` set to the configured suffix after `snmp.trap.`. Single-dim charts use `name: events`; dim-from-varbind charts use `name_from_label: varbind_value`. All charts have `instances.by_labels: [job_name]` for per-job isolation. Passes `collecttest.AssertChartTemplateSchema`.

**Modified files:**

2. `src/go/plugin/go.d/collector/snmp_traps/init.go` — Removed `metrics` rejection from `validateDeferredConfig`. The `dynamic_engine_id_discovery` rejection is preserved.

3. `src/go/plugin/go.d/collector/snmp_traps/collector.go`:
   - Added `operatorMetrics *operatorMetrics` and `dynamicChartYAML string` fields.
   - `Init()`: calls `validateMetrics(c.Metrics, idx)` after profile acquisition. On validation pass with configured metrics, builds chart template YAML via `buildChartTemplateYAML` and stores it. On validation failure, returns 422 before any writer/listener setup.
   - `ChartTemplateYAML()`: returns dynamic template when operator metrics are configured, falls back to embedded static `charts.yaml` otherwise.
   - `handlePacket()`: after successful journal write (not for dedup-suppressed or write-failed traps), increments operator metric counters via `c.operatorMetrics.inc(entry.TrapOID, entry, td)`.
   - `collect()`: calls `c.operatorMetrics.collect(c.store, c.jobName)` after self-metrics.
   - `Init()` and `Cleanup()` reset `operatorMetrics` and dynamic chart YAML so Collector reuse cannot leak stale metric configuration.

4. `src/go/plugin/go.d/collector/snmp_traps/config_schema.json` — Updated `metrics` description from "reserved for later implementation" to describe the active feature and documents the exact 64-value `dimension_from_varbind` cap.

5. `src/go/plugin/go.d/config/go.d/snmp_traps.conf` — Added commented-out `metrics:` example block.

6. `src/go/plugin/go.d/collector/snmp_traps/pipeline_test.go` — Updated `TestConfigValidation` subtest from "deferred per-OID metrics" (expected error) to "implemented per-OID metrics" (expected no error).

**New test file:**

7. `src/go/plugin/go.d/collector/snmp_traps/operator_metric_test.go` — focused tests:
   - `TestValidateMetricsSuccess`: single-dim, enum-backed dim, numeric-range dim, integer-range dim, multiple metrics
   - `TestValidateMetricsFailures`: missing oid, invalid oid, oid not in profiles, missing context, context prefix/suffix/empty validation, duplicate raw/alternate-resolved OIDs, duplicate context, duplicate generated selector, raw numeric OID dim, unknown dim name, oversized enum dim, unbounded octet string dim, unbounded mac address dim
   - `TestValidateMetricsErrorsIncludeIndex`: multi-metric validation error includes correct index
   - `TestValidateMetricsNilIndex`: nil index validation fails
   - `TestBuildChartTemplateYAML*`: no metrics, single-dim, dim-from-varbind, multiple metrics — all pass `collecttest.AssertChartTemplateSchema`
   - `TestOperatorMetricsIncSingleDimension`: increments, no counter for unconfigured OID
   - `TestOperatorMetricsIncMatchesAlternateOIDSpelling`: config using an alternate OID spelling still counts packets using the profile's canonical spelling
   - `TestOperatorMetricsIncRequiresCurrentTrapDefinition`: profile reload removal/unknown OID state does not keep counting solely from stale config
   - `TestOperatorMetricsIncDimensionFromVarbind`: enum resolution, cumulative counting
   - `TestOperatorMetricsDimensionValuesStayBoundedAtRuntime`: malformed enum/range values, enum expansion after reload, numeric range expansion after reload, enum label rename after reload, missing reload varbind definitions, and unbounded reload metadata collapse to bounded sentinel dimensions
   - `TestMetricBoolValue` and `TestMetricIntValueRejectsUint64Overflow`: boolean/truthvalue and integer conversion edge cases
   - `TestOperatorMetricsCollectSingleDimension`: store observes correct cumulative value
   - `TestOperatorMetricsCollectDimensionFromVarbind`: store observes per-dimension values with labels
   - `TestOperatorMetricsNilSafe`: nil receiver doesn't panic
   - `TestCollectorHandlePacketIncrementsOperatorMetric`: packet path increments counter
   - `TestCollectorHandlePacketNoOperatorMetricForDroppedTraps`: no counter on write failure
   - `TestCollectorHandlePacketNoOperatorMetricForUnconfiguredOID`: no counter for non-matching OID
   - `TestCollectorHandlePacketNoOperatorMetricWhenNil`: nil operatorMetrics safe
   - `TestCollectorCollectsOperatorMetrics`: cold path pushes to store
   - `TestCollectorHandlePacketOperatorMetricWithDimFromVarbind`: dim-from-varbind resolution on packet path
   - `TestCollectorNoOperatorMetricOnDedupSuppression`: dedup-suppressed duplicates don't increment

**Validation:**

- `go test -count=1 ./plugin/go.d/collector/snmp_traps/...` — PASS
- `go test -count=1 ./plugin/go.d/collector/snmp/... ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go test -race -count=1 ./plugin/go.d/collector/snmp_traps/...` — PASS
- `go test -race -count=1 ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go vet ./plugin/go.d/collector/snmp_traps/... ./plugin/go.d/collector/snmp/... ./plugin/go.d/collector/snmp_topology/...` — PASS
- `go build ./plugin/go.d/collector/snmp_traps/...` — PASS
- `python3 -m json.tool plugin/go.d/collector/snmp_traps/config_schema.json >/dev/null` — PASS
- `git diff --check` — PASS
- `.agents/sow/audit.sh` — PASS with the existing skill-classification warning for 10 non-project skill directories.

**Review round 1:**

- External reviewers: glm, kimi, minimax, qwen.
- Accepted and fixed:
  - Reloaded profile metadata could weaken the runtime cardinality guarantee if a previously bounded `dimension_from_varbind` became unbounded. Runtime now collapses such values to `unknown`.
  - Missing configured varbind definitions after reload returned the symbolic varbind name. Runtime now consistently emits `<missing>`.
  - Enum-backed dimension varbinds had no explicit cardinality cap. `isBoundedLabelVarbind` now caps enums at 64 values, matching numeric-range cardinality.
  - Contexts that differ textually but normalize to the same metric selector now fail job creation with a clear duplicate-selector error.
  - The collector's dynamic chart-template field was renamed to avoid shadowing the embedded base chart template.
- Accepted and added test coverage:
  - Boolean/truthvalue dimension validation and runtime conversion.
  - Unsigned integer overflow handling for numeric dimensions.
  - Profile-reload dimension fallback cases for missing definitions and unbounded metadata drift.
- Reviewed and rejected as false positives:
  - Profile-cache lifecycle race causing premature release: active jobs hold profile-cache references until cleanup, and listener cleanup waits for read goroutines before clearing collector state.
  - Nil `operatorMetrics.collect` guard as a correctness issue: caller and callee nil guards are intentionally harmless; no functional risk.
  - Varbind ownership ambiguity: `dimension_from_varbind` resolves against the trap definition returned for the configured OID, so validation does not cross trap scopes.

**Review rounds 2 and 3:**

- External reviewers: glm, kimi, minimax, qwen.
- Accepted and fixed from kimi: profile reload enum/range expansion could have expanded runtime dimension cardinality. Operator metrics now freeze the job-creation-time bound and tests cover enum expansion and range expansion after reload.
- Accepted and fixed from glm: profile reload enum label renames could have created new stale dimension buckets. Operator metrics now freeze enum key-to-label mappings at job creation and a test verifies label rename does not create a new dimension.
- Accepted and fixed from kimi/minimax: schema/spec/stock config now explicitly document the 64-value cardinality limit for `dimension_from_varbind`.
- Reviewed and rejected as non-blocking:
  - `dimCounts` pruning or hard caps: current runtime cardinality is bounded by job-creation-time enum/range values plus the three sentinel buckets. Counters are cumulative and clearing/pruning buckets in `collect()` would be more dangerous than leaving bounded buckets resident until job cleanup.
  - `c.operatorMetrics` cleanup/read race: go.d lifecycle follows the existing collector pattern where cleanup is not concurrent with collection for a live job; the same pattern is used by listener, writer, deduper, and resolver fields.
  - Inline varbind numeric constraints: current profile authoring pattern uses file-scoped varbinds for reusable bounded metadata; inline numeric constraints are not required for this M4 operator config path.
  - No explicit limit on total configured `metrics:` entries: operator-controlled configuration, not packet-controlled cardinality. The M4 hard requirement is packet/runtime dimension bounding; aggregate per-job metric-count policy can be revisited if SOW-0039 docs/collector consistency work needs a UX cap.
  - Context dots vs underscores: dots are intentionally allowed in the suffix but generated selectors normalize dots to underscores and duplicate normalized selectors are rejected at job creation.

## Outcome

M1-M4 implementation is complete and the collector consistency bundle has finished in SOW-0039. SOW-0037 is completed and moved to `.agents/sow/done/` with the final closeout.

## Lessons Extracted

- Per-OID metrics need a frozen job-creation-time contract, not only job-creation-time validation. Profile reloads can otherwise alter runtime cardinality after DynCfg apply succeeded.
- Enum labels are metric identity too. Freezing only enum keys prevents value expansion, but freezing key-to-label mappings also prevents label rename reloads from creating stale parallel buckets.
- Cumulative counters should not prune dimension buckets opportunistically; bounded retention until job cleanup is simpler and safer than trying to delete or reset counters during collection.

## Followup

- SOW-0039 owns collector consistency, end-user/operator docs, metadata, README, taxonomy, and health alert templates before merge.
- SOW-0038 owns OTLP export behavior.

## Regression Log

None yet.
