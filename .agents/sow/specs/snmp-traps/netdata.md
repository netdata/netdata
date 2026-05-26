# Netdata — SNMP Trap Support: Design Proposal (Pipeline + Storage Scope)

## 0. Document Metadata

- **System**: Netdata Agent — proposed SNMP trap reception, decode, enrichment, metric generation, and journal storage subsystem.
- **Scope of this document**: the **ingestion-through-storage pipeline only**. Alerting (existing Netdata alert engine), notifications (existing channels), forensics UI (existing Logs UI), secret storage (existing Secrets), topology integration (existing topology), and user/RBAC (existing Cloud spaces/rooms) are explicitly out of scope as they are handled by existing Netdata subsystems. This document deliberately does NOT redesign any of those.
- **Architectural premise**: distributed by design — one Netdata Agent per site is appointed as that site's SNMP hub. The hub consolidates polling, discovery, topology, traps, NetFlow, and syslog. There is no central correlation tier; correlation happens locally on each hub. This is a deliberate design choice for unbounded horizontal scalability. All decisions below assume it.
- **Author**: assistant.
- **Status**: design complete after Phase A (cohort-naïve matrix) and Phase B (stress-test); resolved user decisions and 4 rounds of reviewer iteration documented inline (see §13 Resolved-by-user-decisions list).
- **Citation convention for cohort evidence**: `<system>.md §<section>` referring to the per-system specs already produced under `.agents/sow/specs/snmp-traps/`. Cross-system summaries cite the Phase A artefacts in `.agents/sow/specs/snmp-traps/comparison/`.

## 1. The Six Design Rules (verbatim)

These rules drive every choice that follows.

1. **Fast ingestion**: zero allocation strategy, prebuilt indexes for lookups, synchronous enrichment. Target: dozens of thousands of events/sec, sufficient for any reasonable site size.
2. **Simpler**: for the operator this must be the simplest possible engine.
3. **Powerful**: choices must be made consciously. We do not optimize one axis at the cost of others. Each cohort axis must score at least competitive; several must score best.
4. **Coverage**: ultra-wide coverage of profiles — operator should not have to discover or probe devices before traps decode correctly. Ship comprehensive vendor packs OOB.
5. **Features**: the audience is the network teams of modern enterprises. Device-centric, port-centric, vendor-OID-centric, topology-aware.
6. **Alerting**: real-time (1-second from PDU arrival to alert evaluation).

## 2. Trap-as-X Primitive — the foundational choice

The cohort exhibits six distinct trap-as-X primitives (see `comparison/design-forks.md` Fork 1).

| Primitive | Cohort examples | Fit for our rules |
|---|---|---|
| Event-row + alarm engine | OpenNMS, Zenoss, CheckMK | Heavy storage tier. Violates Rule 2 — operators must learn new alarm-engine semantics. |
| Passive-check submission | Centreon, Nagios+SNMPTT | Requires polling engine downstream. Doesn't fit hub-local design. |
| Metric/item value | Zabbix, Telegraf | Loses event identity. High-cardinality varbinds leak into TSDB labels. |
| Log document | Datadog, Dynatrace, Splunk, SolarWinds (current), LogicMonitor, Logstash | Clean separation: search/forensics on the document; alerts elsewhere. SaaS-cohort convergence. |
| Eventlog + handler relational update | LibreNMS | Operator-built SQL alerts; not OOB. |
| Pure pass-through | Cribl | No storage, no alerting in-product. |

**The choice — hybrid, but the journal is the universal store**:

1. **Every trap is captured in the journal — by default.** In the default configuration (dedup disabled), the plugin writes one journal entry per received trap, no exceptions. Operators can opt-in to deduplication (§10) for flap-heavy environments; when opt-in dedup is enabled, repeated identical traps within a configurable window are collapsed and surfaced periodically through a dedup-summary entry. Operators who enable dedup accept that suppressed individual trap PDUs are summarized rather than persisted in full.
2. **Plugin-self metrics** (two NIDL contexts always emitted plus an opt-in third for dedup — see §12) for plugin health monitoring.
3. **Per-OID metrics for alerting are opt-in** — operator selects which OIDs get a dedicated metric via plugin configuration (NOT via profile YAML). See §7.

The journal is the foundation. Metrics are a derived signal for alerting on specific traps the operator cares about.

**Why this choice**:

- Reuses existing infrastructure: systemd-journal storage, alert engine, Logs UI.
- Datadog and Dynatrace converged on functionally the same model.
- Rule 6 satisfied trivially: counters update on every trap; alert engine runs at metric-update cadence.
- High-cardinality varbinds live in the journal (always), never in metric labels — avoiding the SolarWinds 90-min-DELETE pain.
- Profiles stay vendor-curated knowledge; operator-specific choices live in plugin configuration.

## 3. Trap categorization — for journal tagging, NOT automatic metric emission

Every trap is tagged with a category in the journal. **Categories do NOT automatically produce per-category metrics.** Categories serve two purposes:

1. **Journal field** (`TRAP_CATEGORY`) — operators query journals by category.
2. **Dimension of the plugin-self metric `snmp.trap.events`** (§12) — operators get a per-category trap-rate view per device, useful for alerting on broad trends without per-OID setup.

Many traps from network devices have metric-equivalents already polled by Netdata. For those, the polled metric is the right alert source — it carries the current state and supports hysteresis/ML — while the trap journal remains valuable for varbind detail (reason codes, peer identity, change actor) that the polled metric does not carry.

| Slug | What it carries | Cohort evidence |
|---|---|---|
| **`config_change`** | Configuration change audit — `ccmCLIRunningConfigChanged` and analogues — who/what/when/from-where | OpenNMS event catalogue; SolarWinds Trap Rules; LibreNMS handler families |
| **`security`** | Security violations with per-event detail — port-security MAC violations, DHCP-snooping drops, DAI drops, ACL hits, IPS hits | F5/Palo/Fortinet MIBs in vendor packs across cohort |
| **`auth`** | Authentication events with source identity — `authenticationFailure` with source IP, user attempt | Cohort-universal |
| **`license`** | License / compliance events — expired / violated / feature unlocked | Cisco, Juniper MIBs |
| **`mobility`** | MAC mobility / topology events with the actor — `macAddressMoved`, STP `newRoot` | LibreNMS handler classes; vendor MIBs |
| **`state_change`** | Interface/port state, system lifecycle, routing protocol state, environmental state transitions — `linkDown`/`linkUp`, `coldStart`/`warmStart`, BGP transitions, fan/PSU/temp | Cohort-universal |
| **`diagnostic`** | Vendor diagnostic events with device-determined context — reboot reasons, module insertion, RAID array, optical transceiver | Cisco/Juniper/Aruba diagnostic MIBs |
| **`unknown`** | No profile coverage — default for OIDs not in the catalogue. Also used for vendor MIBs that reserve user-defined trap slots (e.g. JANITZA `userTrap*`, SITEBOSS `s550notificationsUserTrap*`, NetApp `userDefined`), whose semantics are operator-determined at runtime, not in the profile | n/a |

The category slug is assigned by the **profile** for known OIDs. For OIDs without profile coverage, the slug defaults to `unknown`. Operator can override the slug per-OID in plugin configuration if needed.

**Category set is closed** — the 8 slugs above are the canonical taxonomy. Operators cannot extend this set; new slugs are added via Netdata releases when genuinely new content types emerge. For cross-cutting concerns (compliance scope, tenant, datacenter, change-window classification, etc.) operators use **labels** — see §7 (profile baseline labels) and §7.5 (plugin operator overrides). Labels are multi-valued per trap, free-form, and don't expand the metric dimension count. Operator-authored OIDs default to `unknown` and the operator overrides category/severity/labels in plugin config — there is no separate "custom" category slug.

PDU type (TRAP / INFORM / v1) is **not** a category — it is recorded separately in the journal field `TRAP_PDU_TYPE`. The same OID can arrive as either TRAP or INFORM with identical meaning; the content category is independent of how it was delivered.

**Note on traps with polled metric equivalents** — several `state_change` traps (e.g., `linkDown`, BGP transitions, fan/PSU/temp) have Netdata polled-metric equivalents (`ifOperStatus`, `bgpPeerState`, env sensors). Operators should alert on the polled metric, which carries the **current** state and supports hysteresis/ML, not on the trap counter. The trap journal still has forensic detail (reason codes, peer identity, change actor) that the polled metric does not carry. A future release may add a `metric_filter` profile field that explicitly links a trap to its polled equivalent so the Logs UI can show the metric time-series next to the trap detail.

## 4. The Cardinality Discipline Rule

Cardinality discipline applies **only** to metric labels and dimensions. The journal — including templated MESSAGE content — has no cardinality constraint and is the proper home for high-cardinality detail (MAC, IP, username, packet content, RAID slot, etc.).

| Surface | Cardinality rule | Why |
|---|---|---|
| Description template → `MESSAGE` field | **No restriction** — use any varbind | MESSAGE is per-row free-form text. 10k distinct MAC values = 10k journal entries with different MESSAGEs. That's normal journal behavior; it is **the** place high-cardinality detail belongs. |
| Varbind capture → `TRAP_JSON` field | **No restriction** — all varbinds always | Single structured field; per-row JSON. No per-OID journal-field-name pollution. |
| Label template → metric labels | **Bounded cardinality only** | Labels propagate to time-series storage. 10k distinct label values = 10k time-series = cardinality explosion. |
| Profile dimension declaration → metric dimensions | **Bounded enum only** | Same reasoning. |

**Allowed as metric labels** (bounded cardinality):
- Device identifier (hub-local universe — bounded by site size)
- OID family (bounded by profile catalogue)
- Severity (bounded enum: one of the 8 syslog levels per §11)
- Reason code (bounded enum where the MIB defines it)
- Feature name (license events — bounded per vendor)
- Interface name when bounded per device

**Forbidden as metric labels** (unbounded cardinality — journal-only):
- MAC address
- Source IP attempting auth
- Username attempting auth
- Specific packet contents
- RAID array element ID
- Any per-event identifier

The forensic question "which MAC violated port-security on switch X today" is a journal query (Logs UI), not an alert. The metric "port-security violations on switch X" fires the alert; the operator clicks through to the journal — where the MAC is in the templated MESSAGE field and in the `TRAP_JSON` structured varbind payload.

Plugin enforces this structurally: label templates with unbounded-cardinality varbind references are **rejected at config-load** with a clear error. Description templates have no such check.

## 5. Plugin Architecture

### Listener-as-Job architecture (load-bearing)

The plugin is a **DynCfg-managed jobs orchestrator**. Each **job is one listener** with one or more configured protocol/address/port endpoints, one auth context, one allowlist, one rate limit, one dedup cache, one writer, one journal directory, one retention policy, and (for SNMPv3) one receiver-local engine ID plus one `snmpEngineBoots` counter. Operators add multiple listeners only for scaling, isolation, or materially different auth/rate-limit/retention policy; multiple jobs are not required just to accept multiple ports or protocols.

Job lifecycle mirrors the established go.d pattern (`src/go/plugin/framework/jobruntime/job_v1.go`; orchestration in `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go`):

- **Add / Enable** — create job and synchronously preflight every required resource. Endpoint bind failures (EACCES for privileged port, EADDRINUSE for port collision, etc.), unsupported endpoint protocol, invalid job name, invalid v3 USM passphrases, profile-load failure, journal directory create/open failure, writer initialization failure, and retention configuration failure all return coded errors (HTTP 422) surfaced in the DynCfg UI. The job is not reported as started when creation fails.
- **Update** — stop the running job, recreate from new config, and preflight every required resource. Atomic restart, no plugin-wide restart.
- **Disable / Remove** — stop the job, close all sockets, retain the journal directory (for forensics) but stop writing.

Creation success is all-or-nothing: partial resources created during a failed preflight are cleaned up, and users see the error during DynCfg apply rather than discovering a runtime-only log failure later.

Trap job names are path and journal identifiers. The trap plugin applies an additional validator after the shared DynCfg `JobNameRuleStrict`: `^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, maximum 64 characters, no dots, no path separators, and no control characters. Violations fail job creation with HTTP-422 before any bind or journal directory creation.

**Stock default job: `local`** — disabled, UDP port 162 endpoint, no community whitelist, no v3 USM users (must be configured before enabling). Operators enable explicitly via DynCfg; the stock conf does not run a listener out of the box. Per spec §6, the job binds the configured endpoint(s) or fails (no automatic high-port fallback) — operators either grant `CAP_NET_BIND_SERVICE` to the binary or reconfigure the endpoint port explicitly.

**Per-job filesystem layout:**

| Path | Purpose |
|---|---|
| `/var/cache/netdata/traps/{job_name}/` | Per-job journal directory (one journal file family per job) |
| `/var/lib/netdata/snmp-trap/{job_name}/engine-boots` | Per-job `snmpEngineBoots` counter for SNMPv3 INFORM correctness (see §6) |
| `/var/lib/netdata/snmp-trap/{job_name}/local-engine-id` | Per-job receiver-local SNMPv3 engine ID used for INFORM authentication/Responses when `local_engine_id` is omitted |

### Language and process model

The implementation language is **Go** (user decision recorded on 2026-05-25). This keeps the listener/job lifecycle aligned with the existing go.d framework and avoids making all `go.d.plugin` users pay SNMP trap profile memory unless they create trap jobs.

**Process model** (accepted by SOW-0035 M1 ADR-0001): The trap plugin is a **standard in-process go.d collector V2 module** at `src/go/plugin/go.d/collector/snmp_traps/`, registered as `snmp_traps`. No separate process, no CGo, no subprocess bridge.

**Journal writer backend**: The trap collector uses the Go systemd journal SDK module `github.com/netdata/systemd-journal-sdk/go/journal` at `go/v0.1.0` behind the local `TrapWriter` abstraction. `NewJournalWriter()` constructs `journal.NewLog()` with `LogOpenEager` and `LogIdentityStrict`, so journal directory creation/open, active file creation, writer lock acquisition, machine ID parsing, boot ID parsing, rotation policy validation, and retention policy validation are proven during job creation before DynCfg apply succeeds.

The configured per-job root remains `/var/cache/netdata/traps/{job_name}/`. The SDK appends the machine-id child directory, so the effective query directory is `/var/cache/netdata/traps/{job_name}/{machine_id}/`. Use the SDK-backed writer's effective `JournalDirectory()` for `journalctl --directory` validation.

**Rationale**: In-process module code means no IPC for cross-plugin enrichment (SOW-0037), trivially shared profile cache (Go package-level state + refcount), and the well-understood go.d job lifecycle. A CGo bridge to libsystemd cannot set `_HOSTNAME` (journald owns trusted fields); a subprocess Rust bridge adds process management complexity. The SDK-backed writer preserves direct journal file control, creation-time failure detection, and `journalctl` compatibility without maintaining a package-local copy of the journal binary format.

**Reference**: `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md`

### Concurrency model

- **Hot path** (per-packet decode + enrich + counter increment): one receive loop per endpoint with reusable buffers; no per-packet heap allocation target; shared per-listener decoder/resolver/writer pipeline. One job = one listener = one or more endpoints = one writer. Multiple endpoint receive loops in the same job fan into one concurrency-safe bounded writer queue; one worker drains that queue and writes journal entries sequentially.
- **Journal write**: per-job writer thread, one journal file family per job (under `/var/cache/netdata/traps/{job_name}/`). Cap is ~30k rows/sec per writer. To exceed that for a single high-volume listener, operators add more jobs for scaling/isolation. Multi-writer partitioning **within a single listener** is out of scope for SOW-0035–SOW-0038; if it becomes necessary it joins a future SOW.

### Process model

SOW-0035 M1 finalizes the exact process/writer boundary for the Go implementation. The default target is standard go.d job orchestration unless M1 evidence justifies another boundary for the journal writer path. **Job lifecycle is independent** of plugin lifecycle — DynCfg add/update/enable/disable/remove operates per-job without restarting the plugin process. Plugin process restart cycles all jobs.

### Hot path (executes per trap, per job)

1. Receive from a configured endpoint into a reusable buffer.
2. Pre-decode allowlist (source IP, CIDR match). Drop if disallowed (no decode cost).
3. BER limit pre-scan, then SNMP trap decode with the chosen parser, within bounded limits (see §18) — drop and increment counter on limit violation.
4. Community / USM auth check.
5. Per-job rate-limit (token bucket). Drop or sample if over budget (operator chooses).
6. Identify source: PDU-based first (v1 `agent-addr`; v2c/v3 `snmpTrapAddress.0` varbind per RFC 3584); fall back to UDP peer. Candidate source addresses are accepted only after IP parsing/normalization. Malformed or non-IP `snmpTrapAddress.0` values are ignored for source identity and `_HOSTNAME` fallback; the kernel-provided UDP peer remains the final safe fallback.
7. OID lookup against the prebuilt OID index (perfect-hash or radix-trie at scale). Lookup is exact-match-first; on primary miss, the receiver tries one SMIv1 / SMIv2 `.0.` alternate trap-OID key by adding or removing a single `.0.` segment immediately before the final OID arc. If neither key matches a profile entry, set `category: unknown`, `severity: notice`, `name: ""`, increment `snmp.trap.errors.unknown_oid`, and continue — the trap still emits to journal with the raw OID + varbinds.
8. Apply profile entry (or unknown defaults from step 7): category tag, severity default, symbolic name.
9. Enrich: device identity (sysName, vendor); topology position if co-located; recent polling state if available. Go in-process access is preferred; any alternate boundary must be justified by SOW-0035 M1.
10. **(Opt-in dedup, default off — see §10)** If dedup is enabled for this job: check `(source_device, trap_OID, key_varbinds)` fingerprint. If hit, increment in-memory suppression counter and skip steps 11-12. If miss or dedup disabled, continue.
11. Atomic increment of in-memory counters for `snmp.trap.events` (per device, per category, per severity), with `job_name` as a label.
12. Build journal entry; one `TrapWriter.Write()` call (see §19). The journal-direct writer serializes the entry directly into SDK-managed per-job journal files (NOT via `sd_journal_send()` — journald is bypassed so the writer can set `_HOSTNAME` to the source device, see §11).
13. Return.

### Cold path (per Netdata collection tick, default 1Hz)

Walk per-job counter maps → emit PLUGINSD `BEGIN`/`SET`/`END` lines on stdout → flush. Standard Netdata pattern.

This decoupling means the hot path is not blocked by stdout back-pressure; if the pipe stalls, traps still ingest, journal still writes, counters still increment, metrics catch up on next tick.

### v3 USM engine-ID discovery (opt-in, SOW-0038)

Dynamic discovery for v3 Trap sender engine IDs follows the Splunk SC4SNMP pattern (`splunk-sc4snmp.md` §3.5; `traps.py:229-258`). Subclass the SNMP transport to peek at the raw bytes pre-parse, ASN.1-decode the SNMPv3 header to extract `engineID`+`username`, hot-register the pair, retry parse.

**This feature is opt-in (disabled by default)** because hot-registering arbitrary `(engineID, username)` pairs at runtime has security/correctness concerns:

1. **Spoofing surface**: a malicious source can present arbitrary `engineID` values to the listener; without operator-curated whitelist, the listener will hot-register and trust the pair on subsequent v3 Trap PDUs.
2. **Operator visibility**: every first-seen dynamic engine ID must be logged and counted so operators can detect unexpected senders.

Operators who genuinely need dynamic v3 Trap discovery enable it explicitly in plugin config; operators with a known set of devices enumerate sender engine IDs in plugin config (the safer default).

This is separate from v3 INFORM receiver-local engine ID discovery. For confirmed-class INFORM messages, the receiver is authoritative; the sender authenticates against the receiver's local engine ID. SOW-0036 implements a configured-or-generated persisted `local_engine_id`. SOW-0038 owns the optional RFC 3414 Report responder that lets senders discover that local engine ID automatically.

## 6. Reception Surface

Reception is **per-job** (§5). Each listener (job) opens all configured endpoints, applies one auth context, one allowlist, one rate limit, and writes to one journal directory. Multiple listeners are for scaling/isolation or materially different policy, not the only mechanism for accepting multiple ports or protocols.

### Endpoint binding

- **UDP/162** is the standard SNMP trap endpoint. Privileged ports (<1024) require `CAP_NET_BIND_SERVICE` on the binary or process.
- **Stock default job `local`** binds a UDP port 162 endpoint. If the binary lacks `CAP_NET_BIND_SERVICE` the bind returns `EACCES` and the job fails to start — no automatic fallback to a high port. Operators choose one of: (a) grant the capability (`setcap CAP_NET_BIND_SERVICE=eip <binary>` or systemd `AmbientCapabilities=CAP_NET_BIND_SERVICE`), or (b) reconfigure the job to a non-privileged port (e.g., 3162, 4162) and configure the sending devices accordingly.
- **Any endpoint bind failure is fatal for the job** — the job init returns an HTTP-422 coded error to DynCfg; the operator sees the error in the dashboard. No silent degradation, no "try a random free port" behavior. This makes listener behavior predictable for operators: every configured endpoint binds exactly as requested or the job is not started.
- **Unsupported endpoint protocols fail at job creation** — the endpoint schema is protocol-aware. A protocol listed in config but not supported by the implementation returns HTTP-422 during DynCfg apply; unsupported endpoints are never ignored.

### Protocols and crypto

- SNMPv1, SNMPv2c, SNMPv3-USM with full HMAC-SHA-2 family (SHA-224 / SHA-256 / SHA-384 / SHA-512) and AES-128 / AES-192 / AES-256 priv (per `logstash.md` §3.6 verification of SNMP4j coverage). The Go implementation selected in SOW-0035 M1 must reach this parity.
- Multiple v3 USM users per job (gosnmp/equivalent's `TrapSecurityParametersTable` semantics).
- IPv4 and IPv6.

### Allowlist (first filter — pre-auth, pre-decode)

- Source-IP CIDR allowlist matched on the UDP packet's source address before any BER decode.
- Community-string allowlist matched after partial decode (community is part of v1/v2c PDU header; v3 has no community).
- v3 USM user/engineID allowlist matched after USM auth.
- Each tier increments its own `snmp.trap.errors.*` counter on drop.

### INFORM acknowledgement semantics

`InformRequest-PDU` (v2c/v3) requires the receiver to send an `Response-PDU` back with matching `request-id`, otherwise the sender retransmits.

- **Response sent synchronously from the receive socket**, immediately after BER decode + auth (before journal write). This guarantees the sender stops retransmitting as quickly as possible.
- **Same UDP socket as receive** — guarantees correct source-port and source-IP, including under NAT. Required per RFC 3414 §3.
- **Retransmits handled by idempotency** — if the sender retransmits the same InformRequest (same `request-id`), the receiver re-emits the same Response. The plugin does not track in-flight informs.
- **Response send failure** is logged + `snmp.trap.errors.inform_response_failed` counter increment; the trap itself is still processed (the sender's retransmit per RFC 3414 will eventually succeed or the operator's monitoring will catch the persistent failure).
- **Receiver-local engine identity for v3 INFORM** — per RFC 3414 confirmed-class message semantics, the receiver is authoritative for INFORM authentication and Response PDUs. The job uses `local_engine_id` when configured; when omitted, Netdata generates a stable per-job value at job creation and persists it at `/var/lib/netdata/snmp-trap/{job_name}/local-engine-id`. Remote sender engine IDs in `engine_id_whitelist` remain the v3 Trap allowlist and are not used as the receiver-local INFORM engine ID.
- **`snmpEngineBoots` persistence for v3 INFORM** — per §5, the receiver-local boot counter is persisted at `/var/lib/netdata/snmp-trap/{job_name}/engine-boots`. Without persistence, devices that cached the old boot counter reject our INFORM Response PDUs as replay attacks after every Netdata restart.
- **Creation-time failure semantics** — failure to create, read, validate, write, fsync, or rename either `local-engine-id` or `engine-boots` fails job creation/DynCfg apply. The job is not reported as started and does not rely on runtime-only log errors for these state files.

### Out of scope (deliberate, this design pass)

- **DTLS / TLS-TM** — Phase A finding: zero cohort systems support this (universal gap). Defer until production demand surfaces and Rust/Go libraries mature. Listed in §14 Non-Goals.

## 7. Profile YAML — vendor knowledge only

Profile YAML defines vendor-curated knowledge ONLY. It does NOT define journal field names (the journal always captures all varbinds — see §11), and it does NOT define metric emission (that lives in plugin configuration — see §7.5).

The authoritative profile schema is `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` (shipped with the OOB pack). The example below is illustrative of the schema; the schema doc is the ground truth.

```yaml
# File-scoped varbinds table — deduplicated definitions referenced by name from each trap.
# This pattern keeps disk + loaded-memory linear in the number of distinct varbinds per
# vendor, not in the number of traps that use them.
varbinds:
  cpsIfViolationMacAddress:
    oid: 1.3.6.1.4.1.9.9.315.1.2.1.1.1
    type: OctetString
    # display_hint: "1x:"   # reserved future field — see profile-format.md
  cpsIfViolationVlan:
    oid: 1.3.6.1.4.1.9.9.315.1.2.1.1.2
    type: INTEGER
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
  ifDescr:
    oid: 1.3.6.1.2.1.31.1.1.1.1
    type: OctetString

traps:
  - oid: 1.3.6.1.4.1.9.9.315.0.1
    name: CISCO-PORT-SECURITY-MIB::ciscoPsmTrapSrvUnauthorized   # REQUIRED, MIB-qualified, globally unique
    category: security
    severity: warning

    # Description template — references varbinds from the file-scoped table by name.
    # Becomes the journal MESSAGE / OTLP body. Free cardinality (§4).
    description: "Port-security violation: MAC {cpsIfViolationMacAddress} on {ifDescr} (VLAN {cpsIfViolationVlan}, ifIndex={ifIndex}) on {_HOSTNAME}"

    # Labels — bounded-cardinality varbinds only (§4). Templates allowed.
    # Each label emits as journal field TRAP_TAG_<KEY_UPPERCASE> and OTLP attribute trap.<key>.
    labels:
      interface: "{ifDescr}"           # OK: bounded per device → TRAP_TAG_INTERFACE
      vlan: "{cpsIfViolationVlan}"     # OK: bounded set per device → TRAP_TAG_VLAN
      # Note: the `interface` label above renders as TRAP_TAG_INTERFACE. The
      # plugin-controlled TRAP_INTERFACE field (from SOW-0037 cross-plugin
      # topology enrichment) lives in a separate namespace; both can co-exist
      # on the same trap entry without conflict.
      # mac: "{cpsIfViolationMacAddress}"   # REJECTED at config-load (unbounded)

    # Per-trap varbind reference list — names from the file-scoped varbinds table above.
    varbinds: [cpsIfViolationMacAddress, cpsIfViolationVlan, ifIndex, ifDescr]

    # Dedup fingerprint key varbinds (only used when opt-in dedup is enabled; see §10).
    # Default key when not specified: (source_device, trap_OID).
    dedup_key_varbinds: [cpsIfViolationMacAddress, cpsIfViolationVlan]
```

**Required per trap entry**: `oid`, `name` (MIB-qualified `<MIB-MODULE>::<symbol>` — vendors reuse bare symbolic names across product-line MIBs; the bare symbol is not globally unique), `category`, `severity`. `description` is optional (defaults to `"{TRAP_NAME} on {_HOSTNAME}."`). `labels`, `varbinds`, and `dedup_key_varbinds` are optional. The plugin loader rejects entries missing required fields at startup with a clear error naming the file + offending entry.

### Trap OID lookup — SMIv1 / SMIv2 `.0.` tolerance

SMIv1 `TRAP-TYPE` notifications map to SMIv2 notification OIDs with the RFC 3584 form `enterprise.0.specific`. SMIv2 `NOTIFICATION-TYPE` notifications commonly use `parent.specific` without the inserted `.0.` segment. MIB tooling can emit either form for the same trap family, so the receiver is tolerant at ingestion time.

The lookup contract is:

1. Exact profile `oid:` match wins.
2. If exact lookup misses, the receiver tries one alternate trap-OID key:
   - `enterprise.0.specific` -> `enterprise.specific`
   - `enterprise.specific` -> `enterprise.0.specific`
3. Degenerate or too-short OIDs do not produce alternate matches.
4. The tolerance applies only to trap-OID lookup. Varbind OID resolution is exact because varbind OIDs do not have this SMIv1 trap encoding ambiguity.

Operators and generated profiles may therefore use the canonical OID form produced by their MIB tooling. The receiver still matches traps sent by devices that use the alternate SMI form. Exact-match precedence prevents the fallback from overriding an explicitly-authored profile entry when both forms exist.

### Varbind resolution — 2-tier

The plugin resolves each varbind in the PDU in this order:

1. **Profile file-scoped `varbinds:` table** — if the profile defines this varbind by OID, use its declared name, type, and (future) `display_hint` directly. The shipped OOB pack covers 437 vendor PENs / 71,787 traps with file-scoped varbind metadata — operators get rich decoding for top vendors without installing any MIB file.
2. **Raw fallback** — varbind not in any loaded profile. Render as OID-keyed entry with the ASN.1-decoded type only. The varbind still lands in `TRAP_JSON` (§11) with its OID and value; just without a symbolic name.

There is **no runtime MIB compilation tier**. The plugin does not parse SMIv1/v2 MIB files at runtime; there is no `pysmi`/`gosmi`/Rust-MIB-crate dependency. Operators who need coverage for a vendor MIB not in the shipped OOB pack convert their MIB files to profile YAMLs **offline** using the shipped helper `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` and drop the resulting YAML into `/etc/netdata/go.d/snmp.trap-profiles/` (per the SNMP polling plugin pattern — see §15 and the user-facing documentation shipped with the plugin).

### Description template syntax

`{varname}` substitutions. Recognized references:

| Reference | Resolved to |
|---|---|
| `{ifDescr}`, `{ifIndex}`, `{cpsIfViolationMacAddress}`, … | Varbind by MIB symbolic name |
| `{1.3.6.1.2.1.31.1.1.1.1}` | Varbind by numeric OID (fallback when no MIB name) |
| `{_HOSTNAME}`, `{TRAP_SOURCE_IP}`, `{TRAP_NAME}`, `{TRAP_DEVICE_VENDOR}` | Standard journal fields |
| `{TRAP_INTERFACE}`, `{TRAP_NEIGHBORS}` | Topology fields when co-located |
| `{ifOperStatus}` | MIB enum value, symbolic (e.g., `down`) by default |
| `{ifOperStatus.raw}` | MIB enum value, raw numeric (e.g., `2`) |

If a reference cannot be resolved (varbind absent, MIB not loaded, etc.):
- Missing varbind → `<missing>`
- Unrecognized variable name → `<unresolved:varname>` plus `snmp.trap.errors.template_unresolved` increment so the operator notices

If `description` is absent, default template is:
```
{TRAP_NAME} on {_HOSTNAME}.
```

Templates are compiled at profile-load (tokenize → segment list). Hot-path substitution is bounded-size buffer fill. MESSAGE capped at 512 bytes (post-substitution); truncated with ASCII `...` marker if exceeded. The 512-byte cap includes the marker bytes. Full forensic data remains in `TRAP_JSON`.

### No `journal_fields:` list, no `metric:` block

The journal captures every varbind always (§11). The plugin emits its own self-metrics always (§12). Operator-defined per-OID metrics are configured separately (§7.5).

### Profile loading — lazy shared cache, multipath, filename-dedup, field-merge on extends-chain

The loader is plugin-wide shared state, not per-listener state. It loads on first runnable trap job creation, is shared by all listeners, and is released when no runnable trap jobs remain. Agents with all trap jobs disabled (or no trap jobs configured) never pay the profile memory footprint. A profile load or validation failure is a job-creation failure and returns HTTP-422 through DynCfg before any listener is reported as started.

The loader mirrors the established SNMP polling pattern (`src/go/plugin/go.d/collector/snmp/ddsnmp/load.go`):

1. **Multipath load** — operator overrides first, then stock: `/etc/netdata/go.d/snmp.trap-profiles/` → `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/`.
2. **Filename dedup** — same filename in a higher-priority directory replaces the lower-priority one entirely. Operator override file `ciscosystems.yaml` fully replaces stock `ciscosystems.yaml`; operators copy + edit to customize a single vendor file.
3. **Field-level merge via `extends:` chain** — when a profile YAML lists `extends: [_base1.yaml, _base2.yaml]`, the loader merges trap entries; later `extends` entries override earlier ones on a per-OID basis. Within a single profile entry, field-level (the override file's fields win for the fields it specifies; unspecified fields inherit from the extended base).
4. **Directory ordering** — within a single directory, files are loaded in `filepath.WalkDir()` lexical order (Go contract). If two files in the same directory define the same OID via `extends`, the alphabetically-later file wins.

### Custom MIB workflow — offline conversion, NOT runtime compilation

Operators who need coverage for a vendor MIB not in the shipped OOB pack:

1. Place MIB files under `~/mibs/` (or any working directory).
2. Run `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen generate --source-dir ~/mibs --all --out-dir ./snmp-trap-profile-gen-output` to produce profile YAMLs. The helper uses default category/severity/description values unless LLM classification is explicitly enabled for stock regeneration. It also writes review artifacts under `--out-dir`, including `conflicts.json` for duplicate trap OIDs and `source-conflicts.json` when multiple MIB files define the same module name.
3. Drop the generated `.yaml` files from `./snmp-trap-profile-gen-output/profiles/` into `/etc/netdata/go.d/snmp.trap-profiles/`.
4. The plugin picks it up on profile YAML hot-reload (or next plugin restart) — see §13.

This mirrors the SNMP polling plugin's model: stock + operator-override YAMLs only, no runtime MIB compilation. The conversion helper is shipped with Netdata in `plugins.d` and uses the bundled IANA PEN snapshot at `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/iana-enterprise-numbers.txt` unless `--refresh-pen` is passed.

Newly added profile entries with `category: unknown` (operator-authored or auto-generated for uncovered OIDs) get the default `unknown` category until the operator sets it in plugin config or in the YAML directly.

## 7.5 Plugin Configuration — per-job listener config + per-OID overrides

The plugin's own configuration (`/etc/netdata/go.d/snmp.trap.conf`, DynCfg-editable) is **per-job** (one job = one listener with one or more endpoints — see §5).

```yaml
# Global settings (apply to all jobs)
update_every: 1   # seconds

jobs:
  - name: local                           # job name → /var/cache/netdata/traps/local/
    enabled: false                        # stock default = disabled; operator enables
    listen:
      endpoints:
        - protocol: udp
          address: "0.0.0.0"
          port: 162                       # job fails to start if any endpoint cannot bind — no automatic fallback
    versions: [v1, v2c, v3]               # which SNMP versions this listener accepts
    communities: []                       # v1/v2c allowlist; empty = accept all
    usm_users: []                         # v3 USM users (each refs Netdata Secrets; passphrases min 8 chars)
    engine_id_whitelist: []               # v3 Trap sender engine IDs (hex)
    # local_engine_id: "0123456789abcdef01234567"  # optional v3 INFORM receiver engine ID; omitted = generated/persisted per job
    dynamic_engine_id_discovery: false    # v3 Trap sender engine ID discovery (opt-in, SOW-0038; see §5)
    reverse_dns:
      enabled: false                       # reverse DNS enrichment (opt-in); cached/non-blocking only
    allowlist:
      source_cidrs: ["0.0.0.0/0", "::/0"] # source IP allowlist (pre-decode)
    rate_limit:
      enabled: false                      # off by default
      per_source_pps: 1000                # token-bucket per source IP
      mode: drop                          # drop | sample (operator choice)
    dedup:                                # OPT-IN, default off — see §10
      enabled: false
      window_sec: 5
      cache_max_entries: 100000
      # key_varbinds default = [] meaning use (source_device, trap_OID) only
    retention:                            # per-job journal retention; see §11
      max_size: 10GB                      # default 10 GB total
      max_duration: null                  # null = no time-based eviction
      rotation_size: null                 # null = auto (max_size / 20, clamped)
      rotation_duration: 1h
    # Per-OID overrides (rare; profile defaults are normally enough)
    overrides:
      - oid: 1.3.6.1.4.1.9.9.43.2.0.1
        category: config_change
        severity: notice
        labels:
          compliance: pci
          tenant: acme
    # Per-OID metric opt-in
    metrics:
      - oid: 1.3.6.1.4.1.9.9.43.2.0.1
        context: snmp.trap.cisco_config_changes
        dimension_from_varbind: ccmHistoryEventTerminalType
```

### Per-OID metric opt-in

Explicit list of OIDs the operator wants emitted as their own metric chart for finer-grained alerting:

```yaml
metrics:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1     # Cisco config change
    context: snmp.trap.cisco_config_changes
    dimension_from_varbind: ccmHistoryEventTerminalType   # MUST be bounded-cardinality
  - oid: 1.3.6.1.4.1.9.9.46.2.0.1     # Cisco port-security violation
    context: snmp.trap.cisco_port_security
    # no dimension_from_varbind → single-dimension counter
```

### Per-OID overrides and labels

Operators can apply per-OID overrides on top of profile baseline. The `overrides:` block in the job config (see example above) supports per-OID category/severity changes and per-OID label additions; `oid_prefix:` matches a whole subtree.

### Label naming rules

Labels are free-form key-value pairs. Keys must match `[a-z][a-z0-9_]*` (lowercase plugin convention; uppercased and prefixed with `TRAP_TAG_` when written to journal as `TRAP_TAG_<KEY>=<VALUE>`, emitted as OTLP attribute `trap.<key>`). Static values are arbitrary strings. Dynamic template references must be bounded-cardinality at profile/config load time; the MVP accepts static strings, `TRAP_NAME`, `TRAP_DEVICE_VENDOR`, enum-backed varbinds, booleans, and small numeric ranges, and rejects unbounded values such as hostnames, source IPs, interface descriptions, MAC addresses, usernames, packet contents, and raw numeric OID references without profile metadata. Labels are slicing metadata on the journal and on the metric-instance, not metric dimensions — they don't expand the chart dimension count.

**Why the `TRAP_TAG_*` namespace?** Labels come from two sources — profile YAMLs (vendor-curated) and operator per-job config. Rejecting labels at config-load would lose vendor data (in the case of profile labels) or annoy operators (in the case of operator labels). Putting all labels in a dedicated `TRAP_TAG_*` sub-namespace means they cannot collide with any current or future plugin-controlled `TRAP_*` field, no rejection logic needed.

**Reserved-name check (post-`TRAP_TAG_*` prefixing)**: the only collision risk now is operator label keys whose uppercased form starts with reserved prefixes inside the `TRAP_TAG_*` namespace, or which would collide with standard systemd fields. The plugin still rejects:

- Any key whose uppercased form would start with `_` (the systemd-reserved trusted-field prefix) when prefixed — this never happens with the lowercase rule above.
- Any key that uppercases to a standard systemd field name (`MESSAGE`, `PRIORITY`, etc.) — again impossible under the `TRAP_TAG_*` prefix.
- In practice, the `TRAP_TAG_*` prefix structurally prevents collisions. The only remaining validation is the `[a-z][a-z0-9_]*` syntax check.

Operators do NOT copy entire profiles to enable metrics or add labels. The profile remains the vendor's curated knowledge; per-installation choices are surgical edits in plugin config.

If `dimension_from_varbind` references a varbind that can take unbounded values (MAC, IP, username, packet content), the plugin REJECTS the config at load with a clear error. Cardinality discipline is structurally enforced.

## 8. OOB Catalog Strategy

Phase A surfaced (`comparison/profile-inventory.md`):

- Datadog Agent: claims "11,000+ MIBs" in public marketing; verified count via a copy of `dd_traps_db.json.gz` is **3,652 MIBs** (67,680 trap definitions, 40,617 varbind definitions). The compiled artifact is closed (Omnibus build) but the **compiler is Apache-2.0** (`datadog/integrations-core :: datadog_checks_dev/datadog_checks/dev/tooling/commands/meta/snmp/generate_traps_db.py`) and the **input MIBs are public** (pysnmp mirror + integrations-core's own MIB tree).
- LibreNMS: 4,770 MIB files / 2,245 with notifications (license per the project's `composer.json` — historically GPL-3.0-or-later; subject to legal review before any redistribution).
- OpenNMS: 230 `.events.xml` files (AGPL-3.0; categorization knowledge — transformation use only).
- Zenoss: ZenPacks per-vendor (various licenses).
- LogicMonitor: closed EventSource catalogue.
- Centreon: catalog of ~214 traps (DB-driven, GPL-2.0).
- CheckMK, Zabbix, Sensu, Telegraf, Logstash, Nagios+SNMPTT: zero vendor MIBs shipped.

### Verified local mirror (production-ready, open-licensed)

We have mirrored sufficient open MIB sources to bootstrap our catalogue end-to-end without depending on any closed cohort artefact:

| Source | Files | Coverage scope |
|---|---|---|
| pysnmp/mibs (canonical pysnmp collection) | 5,087 | Cross-vendor, the same source Datadog's compiler reads from |
| cisco/cisco-mibs (vendor-canonical) | 3,536 | Cisco-published, authoritative |
| Poil/MIBs (community archive) | 4,087 | Community-curated, multi-vendor |
| kcsinclair/mibs | 2,747 | Community archive |
| hsnodgrass/snmp_mib_archive | 2,325 | Community archive |
| kmalinich/snmp-mibs | 620 | Community archive |
| LibreNMS mibs | 4,770 | NMS-curated |
| netdisco-mibs | 5,059 | NMS-curated, vendor-organized directory structure |
| **Total raw files** | **~28,200** | **Deduped: estimated 8,000-12,000 unique MIB modules** |
| Plus the toolchain | — | `pysnmp`, `pysmi`, `pyasn1`, `pyasn1-modules`, `pysnmpcrypto`, `asn1ate` — full compilation chain (all BSD-2-Clause / Apache-2.0) |

This comfortably exceeds the Datadog reference (3,652 MIBs). We can build our own `dd_traps_db`-equivalent end-to-end from public sources with no closed-artefact dependency.

### Approach

**We absorb the knowledge of the community by transforming, not copying.**

1. **(Out of scope for SOW-0035–0039 — follow-up candidates per §17.)** Build offline conversion tools that ingest each major cohort system's per-OID configuration format and emit our profile YAML:
   - OpenNMS `eventconf.xml` → profile YAML
   - Centreon DB catalogue dump → profile YAML
   - Zenoss ZenPack `objects.xml` → profile YAML
   - LibreNMS PHP handler classes → profile YAML (heuristic — handler names map to OID families)
   - Public MIB tree (pysnmp `mibs.pysnmp.com` mirror, IETF SMI archives, vendor public MIBs) → baseline profile YAML
   - SNMPTT `.conf` files → profile YAML

2. **Apply curation** to assign category slugs and severity defaults during conversion. Many cohort sources express severity directly; others require pattern-based inference (OID family → category slug).

3. **Ship the conversion tools with Netdata** as `netdata-snmp-profile-convert` (or similar). These are operationally useful for two reasons:
   - Operators migrating from OpenNMS / Centreon / Zenoss / etc. can convert their site-specific customizations to our profile format.
   - Future cohort knowledge updates (e.g., LibreNMS adds a new handler class) can be re-imported.

4. **Seek additional sources of community trap knowledge** — vendor support sites, RFCs, IETF MIB tree, MIBs Depot, public MIB collections, etc.

### Licensing

We are **transforming knowledge at development time**, not redistributing other systems' files. The output (our profile YAML) is original work informed by reading public documentation, MIB definitions, and open-source classifications. License obligations on others' source files (the project-declared GPL family for LibreNMS, AGPL-3.0 for OpenNMS) require legal review before any conversion tool runs against shipped MIBs; this design pass does not pre-judge the legal analysis. We attribute sources in commit messages and `CREDITS.md`, not by copying files.

### Coverage target

Aim for the LibreNMS-to-Datadog band: ~2,000-12,000 OID families across major vendors (Cisco, Juniper, Arista, Aruba, HPE, Dell, Fortinet, Palo Alto, F5, MikroTik, Ubiquiti, …). Ship YAML in the repo so operators can `grep` for their vendor before install (the Datadog `dd_traps_db.json.gz` opacity is a real customer pain — `datadog-agent.md` §17).

## 9. Hot Path vs Cold Path Throughput

### Per-thread targets

- **Single trap listener + decoder thread**: design target 10s of thousands of decode operations/sec, limited by BER decode + counter increment + dedup-cache check (in-memory). **The exact ceiling is to be measured during implementation** — this number is a design rule, not a benchmarked claim.
- **Single journal writer thread**: ~30k entries/sec. This figure comes from existing benchmarks of the Netdata journal-writer behavior used in the **NetFlow plugin**. Cited here as a reference point for partitioning thresholds; the Go trap implementation will validate the selected backend during implementation.

### Scaling beyond one thread

To exceed ~30k entries/sec total, scale **horizontally with per-job isolation** (§5 listener-as-job model):

- Each listener job is its own writer thread and its own journal directory at `/var/cache/netdata/traps/{job_name}/`.
- Operators scale by adding more jobs (each bound to a different port and/or with different community/USM/source-IP allowlists), partitioning the trap stream at the listener layer.
- Intra-listener multi-writer sharding (one listener feeding multiple writer threads by source-IP hash) is explicitly out of scope (§14 Non-Goals); if a single high-volume sender exceeds one writer's ceiling, the operator splits the sender's traffic across multiple jobs.

Phase A cohort numbers for context (`comparison/feature-matrix.md`):

| System | Throughput |
|---|---|
| Telegraf single-goroutine | ~10k/sec (flagged as ceiling) |
| Datadog c5.large ActiveGate | 30k-45k/min ≈ 0.5-0.75k/sec |
| Splunk SC4SNMP | 1,500/sec vendor-marketing |
| Dynatrace c5.large | 30k-45k/min ≈ 0.5-0.75k/sec |

Our target (10s of thousands/sec sustained on a single hub, scalable via partitioning) is roughly 10×-60× cohort numbers.

### Cold path throughput

At 1Hz emission, even 1,000 devices × 2 contexts × ~10 dimensions = ~20,000 SET lines/sec — trivial through a pipe.

## 10. Deduplication — OPT-IN, per-job, first-wins, no delay, periodic summaries

**Dedup is an opt-in feature, disabled by default.** In the default configuration the plugin writes one journal entry per received trap — the forensic-store guarantee (§2 point 1) holds without exception. Operators who run flap-heavy environments can enable dedup per-job; doing so accepts that individual suppressed PDUs are summarized rather than persisted in full.

Netdata already ships built-in alerts for UDP receive-buffer overflow on all listeners. That covers the kernel-level overflow case. We do NOT duplicate that as a plugin feature.

When enabled, dedup operates per-job: each listener has its own in-memory dedup cache. Source devices are always routed to the job that received their trap, so the dedup state is naturally partitioned by listener — no cross-job synchronization, no shared lock.

### Mechanism — hot path (only when `dedup.enabled: true` on the job)

1. Compute fingerprint per trap: `hash(source_device, trap_OID, key_varbinds)`. **Default key varbinds = `[]` meaning the fingerprint uses only `(source_device, trap_OID)`.** Profiles can override per-OID via `dedup_key_varbinds:` (e.g., port-security trap fingerprints by `[macAddress, vlan]` so different MAC/VLAN combinations are NOT collapsed). If a configured key varbind is absent from a received PDU, the canonical fingerprint uses a missing-value sentinel distinct from the empty string. Operators should list only varbinds that the trap normally emits. The "all non-timestamp varbinds" default was rejected by Phase B because volatile counter varbinds (`ifInErrors`, BGP counters) trivially differ per event, bypassing dedup entirely.
2. Check the per-job in-memory dedup cache (LRU-bounded, default 100k entries, configurable):
   - **Fingerprint NOT present** → write journal entry immediately, increment metric counters, insert fingerprint into cache with TTL = dedup window. **Real-time, no buffering, no delay.**
   - **Fingerprint present** → suppress: no journal write, no per-event metric increment. Increment the in-memory per-period suppression counter (broken down by trap-OID).
3. Cache entries expire after the dedup window (default 5 seconds; configurable per-job).

### Periodic summary entry — for operator transparency

A separate background timer (cadence configurable, default = dedup window length) runs in the cold path. If any suppression happened in the period, it emits **one summary journal entry**:

```
MESSAGE=DEDUPLICATED TRAPS: 247 events have been deduplicated:
- ifDown 120
- authenticationFailure 80
- ciscoPsmTrapSrvUnauthorized 47
PRIORITY=6
SYSLOG_IDENTIFIER=local
ND_LOG_SOURCE=snmp-trap
TRAP_REPORT_TYPE=deduplication_summary
TRAP_SUPPRESSED_COUNT=247
TRAP_SUPPRESSED_FINGERPRINTS=12
TRAP_REPORT_PERIOD_SEC=5
TRAP_JSON={"period_sec":5,"total_suppressed":247,"by_trap":{"1.3.6.1.6.3.1.1.5.3":120,"1.3.6.1.6.3.1.1.5.5":80,"1.3.6.1.4.1.9.9.315.0.1":47}}
```

Note: dedup summary entries omit `_HOSTNAME` (the summary is across multiple source devices, not about one device) and omit `ND_NIDL_NODE` for the same reason. `SYSLOG_IDENTIFIER` carries the job name so operators can attribute the summary to a specific listener.

The MESSAGE field is **multi-line by design** — operators reading the journal directly (e.g., `journalctl TRAP_REPORT_TYPE=deduplication_summary`) get the full breakdown without parsing JSON. The Logs UI renders the multi-line MESSAGE natively. Multi-line MESSAGE values are written using systemd-journal's binary field encoding (see §11) so newlines inside MESSAGE never inject other fields.

This summary entry lives in the same journal alongside the real trap entries. The `TRAP_REPORT_TYPE=deduplication_summary` field distinguishes it from real trap entries (which have `TRAP_REPORT_TYPE` absent or set to `trap`). Operators query summaries cleanly:

```
journalctl TRAP_REPORT_TYPE=deduplication_summary
journalctl TRAP_REPORT_TYPE=trap        # real trap entries only
```

If there was no suppression in the period, no summary entry is emitted.

### Why this matches our rules (when enabled)

- **Rule 6 (real-time alerting)**: hot path commits the first occurrence immediately. No buffer, no window-close delay. Alerts fire on the metric the moment the first trap arrives.
- **Rule 2 (operator simplicity)**: journal stays clean — one entry per real event, plus a periodic summary when duplicates were suppressed.
- Operator transparency: the metric `snmp.trap.dedup_suppressed` (per-job dimension) increments per suppressed trap (continuous signal); the periodic summary entry provides on-journal narrative.

### Default (dedup disabled) behavior

Every trap → one journal entry. No suppression. No summary entries (no suppression means no summary to emit). The `snmp.trap.dedup_suppressed` metric (§12 Context 3) is not emitted at all when no job has dedup enabled. Operators get the full forensic-store guarantee per §2 point 1.

### Per-source rate-limiting vs dedup

Dedup collapses **identical** repeated traps. Per-source rate-limiting (§7.5 `rate_limit:`, default off) shields the plugin from a misbehaving sender flooding with **unique** traps. The two solve different problems and can both be enabled independently per job.

### Out of scope (first release)

- Per-fingerprint suppression summary entries — `TRAP_JSON.by_trap` in the periodic summary gives enough breakdown. Per-fingerprint detail can be added later if operators need it.
- Periodic re-notification within a sustained dedup window (operator sees one entry every dedup-window during a multi-hour storm). Configurable knob deferred — default is hard suppression for the full window.
- Paired-clear semantics (linking `linkUp` to `linkDown` for auto-recovery) — covered as a §14 Non-Goal; trap-based alarm lifecycle belongs to the alert engine.

## 11. Journal Storage — per-job, universal capture, capital-letter fields, OID + name

### Per-job journal directories + retention

Each listener job (§5) owns its own configured journal root:

```
/var/cache/netdata/traps/{job_name}/   ← configured per-job root (one writer thread per job)
```

The SDK-backed writer stores files under the machine-id child directory:

```
/var/cache/netdata/traps/{job_name}/{machine_id}/   ← effective journalctl --directory path
```

Per-job retention policy mirrors the retention semantics used by the NetFlow plugin (`src/crates/netflow-plugin/src/plugin_config/types/journal.rs`), with intentional deviation on defaults — the trap plugin ships with size-only eviction by default (no time-based cap). Time-based retention is operator opt-in. The Go implementation maps these knobs to the SDK `RotationPolicy` and `RetentionPolicy`. Rationale: traps are operator-relevant forensic data with low per-event rates in typical deployments; aging entries out by time discards forensic value before the operator's investigation window closes. A follow-up will align NetFlow's default to match. The trap plugin's per-job knobs:

| Knob | Default | Semantics |
|---|---|---|
| `retention.max_size` | `10GB` | Total bytes of journal files for this job; oldest files are deleted when exceeded. Set to `null` to disable size-based eviction. |
| `retention.max_duration` | `null` (disabled) | Maximum age of the oldest journal file; older files are deleted. Set to a duration (e.g., `7d`, `30d`) to enable. |
| `retention.rotation_size` | auto (`max_size / 20`, clamped 5MB-200MB) | Per-file rotation size. |
| `retention.rotation_duration` | `1h` | Per-file rotation duration. `0`/`null` disables time-based rotation. |

**Retention rules apply independently and inclusively** — either threshold being exceeded triggers cleanup of the oldest file. Both `null` is allowed only as an explicit manual-cleanup configuration; the default is size-capped. When both are `null`, the periodic retention sweep is a no-op and no file is deleted by the plugin.

`retention.rotation_size: null` auto-calculates as `retention.max_size / 20` and clamps to 5MB-200MB. If `retention.max_size` is `null`, the auto rotation size uses the 200MB upper clamp. Rotation still happens when size-based retention is disabled; files accumulate until age-based retention or external cleanup removes them. Time-based retention requires a periodic retention sweep in addition to rotation-time cleanup, so `retention.max_duration` is honored even during low-volume periods.

The active journal file must be queryable by `journalctl --directory=...`; waiting until rotation/close to build indexes is not acceptable for the MVP. The SDK writer owns the journal file format details, active-file indexes, rotation, retention, and existing-chain validation/reopen behavior.

The journal-direct backend is Linux-only in SOW-0035. On non-Linux platforms, trap job creation must fail with a coded unsupported-backend error instead of starting and failing at runtime. Future SOWs may add a non-journal backend such as OTLP for platforms without `journalctl`.

When dedup is enabled (§10), summary entries land in the same per-job journal directory and obey the same retention.

### Universal capture (in default config)

When dedup is disabled (default), every trap received by a job lands in that job's journal with all its varbinds. This holds whether or not the OID is in the loaded profile. The journal entry is the source of truth.

When dedup is opt-in enabled for a job (§10), the forensic-store guarantee narrows to first-occurrence-per-window plus periodic summary entries; operators who enable dedup accept this trade-off.

### Standard journal expectations

The trap plugin acts as the journal **writer** for its trap entries (think of it as a journald-style infrastructure component, not as an application logging). The "application" emitting the log is the **source device** that sent the trap. Standard systemd-journal fields are set on the entry on behalf of that source device, so that when trap entries are multiplexed with other logs in the Logs UI, the common fields describe the device the trap is about — not the trap plugin.

Journal field names conform to `^[A-Z][A-Z0-9_]*$` per systemd-journal requirements. Standard fields always emitted or exposed by the journal file:

```
MESSAGE=<rendered description template; free-form, high-cardinality content welcome>
PRIORITY=<numeric 0-7 from the canonical 8-severity table below>
SYSLOG_IDENTIFIER=<the per-job listener name (e.g., "local") — the operator-meaningful identity of the collection daemon producing the entry>
_HOSTNAME=<the source device hostname from enrichment, or source IP when enrichment has no hostname — the "host" the trap is about>
```

`_HOSTNAME` is normally a systemd "trusted field" set by journald; the trap plugin's journal writer writes directly to journal files through the Go-compatible backend selected in SOW-0035 M1 (bypassing journald) and controls every field, including `_HOSTNAME`. The hostname resolution priority is:

1. **Vnode hostname** — when the SNMP polling job has an explicit `vnode.hostname` configured for the source device (available in the in-process `DeviceRegistry` as `VnodeHostname`).
2. **SNMP sysName** — the device's self-reported `sysName` from polling state, unless empty or equal to literal `"unknown"` (case-insensitive).
3. **SNMP/topology sysName** — the source device `sysName` from the topology cache when the trap source IP matches topology-managed IP state and the direct SNMP registry lookup missed, for example when the SNMP collector target was configured by DNS name but traps arrive from an IP.
4. **Reverse DNS** — non-blocking cached PTR lookup for the source IP, only when `reverse_dns.enabled: true` is set in the trap job configuration (disabled by default). Cached lookups never block the packet path; async resolution warms the cache in the background.
5. **Source IP** — the string form of the validated trap source IP (from RFC 3584 cascade). `_HOSTNAME` is always emitted; the source IP is the mandatory fallback when no enrichment identity exists.

The serializer sets `_HOSTNAME` to `DeviceHostname` when the enrichment layer provides a non-empty value; otherwise it falls back to `SourceIP`. No synchronous DNS lookup is performed in the writer or on the hot path. Operators querying `journalctl _HOSTNAME=core-sw-01` see every log line about that device — including traps the device emitted, polled-metric alerts on it, and topology updates — in one cohesive view.

**Operator UX caveats** (documented in the plugin README — SOW-0039 M2):

- `journalctl -m` matches journal entries by their `_MACHINE_ID` field, not by `_HOSTNAME`. Trap entries written by the plugin carry the agent's `_MACHINE_ID` (from the agent's `/etc/machine-id`) and the source device's `_HOSTNAME`. So `journalctl -m <agent-machine-id>` returns all trap entries (and the agent's own logs); it does NOT segment by source device. Use `journalctl _HOSTNAME=<source-device>` for per-source-device filtering.
- Tools that route by `_HOSTNAME` (e.g., journald's `ForwardToSyslog`, syslog-ng's `$HOST` macro, log shippers) will tag the forwarded trap entries with the source device's hostname. This is intentional — the trap entry IS about that device — but mixed deployments should be aware.

In addition to systemd standards, the plugin also populates two **existing Netdata journal fields** so traps correlate with the rest of the Netdata data model:

```
ND_LOG_SOURCE=snmp-trap                          # marks this log family
ND_NIDL_NODE=<source device's Netdata vnode>     # ties this entry to the same vnode used by SNMP polling metrics for the device
```

The closed 8-severity set (matches RFC 5424 / syslog and the values
enforced in `classify.py` + `profile-format.md`):

| Profile slug | PRIORITY | When to use |
|---|---|---|
| `emerg`   | 0 | System is unusable — exceptional vendor catastrophe |
| `alert`   | 1 | Action must be taken immediately |
| `crit`    | 2 | Critical conditions: hardware failure, security breach in progress |
| `err`     | 3 | Error conditions: failure, fault, denial |
| `warning` | 4 | Warning conditions: threshold breach, degradation, recoverable error |
| `notice`  | 5 | Normal-but-significant: routine state changes, completed operations |
| `info`    | 6 | Informational: status updates, periodic events |
| `debug`   | 7 | Debug-level: rare; reserved for traps the MIB explicitly marks debug |

`MESSAGE` is the rendered description template from the profile (§7) — fully resolved with varbind values, including high-cardinality content (MAC, source IP, username, packet details, etc.). This is the operator's primary view: `journalctl TRAP_CATEGORY=security` shows one-line readable rows without further field inspection.

### Field-name conventions (fixed prefix universe)

| Prefix | Used for | Source |
|---|---|---|
| Standard systemd fields (`MESSAGE`, `PRIORITY`, `SYSLOG_IDENTIFIER`, `_HOSTNAME`, `_MACHINE_ID`) | Per-entry standard fields plus file-header machine identity exposed by `journalctl`; `_HOSTNAME` carries the source device | Plugin (writing on behalf of the source device) |
| `ND_LOG_SOURCE`, `ND_NIDL_NODE` | Existing Netdata fields the plugin populates for correlation with metrics + logs | Plugin via Netdata state |
| `TRAP_*` (closed reserved set) | Plugin-controlled trap-content fields (OID, name, category, severity, …) present on every entry | Plugin |
| `TRAP_TAG_*` | Labels — from profile `labels:` (vendor-curated) AND from operator per-job `labels:` config | Profile or operator config (rendered by the plugin from templates) |

The plugin's reserved `TRAP_*` field set is the closed list documented later in this section. The `TRAP_TAG_*` namespace is dedicated to all labels regardless of source — separating labels from plugin-controlled fields removes any possibility of profile-label or operator-label collisions with plugin field names (now or in any future plugin release). The variable per-trap data (varbinds) lives in a single well-known JSON-valued field (`TRAP_JSON`), not in dynamically-named per-OID fields, keeping the journal's field universe stable; `journalctl --fields` does not grow unboundedly with each new vendor MIB.

**Closed reserved `TRAP_*` field set** (operators cannot use `TRAP_TAG_*` keys that uppercase to any of these — but since labels are namespaced under `TRAP_TAG_*`, collision is structurally impossible):

```
TRAP_REPORT_TYPE          trap / deduplication_summary / decode_error_summary
TRAP_OID                  Numeric OID
TRAP_NAME                 MIB-qualified <MIB-MODULE>::<symbol>
TRAP_CATEGORY             One of 8 canonical category slugs (§3)
TRAP_SEVERITY             One of 8 canonical severity slugs
TRAP_PDU_TYPE             trap / inform
TRAP_VERSION              v1 / v2c / v3
TRAP_SOURCE_IP            Identified source per RFC 3584 cascade (§5)
TRAP_SOURCE_UDP_PEER      UDP transport peer
TRAP_DEVICE_VENDOR        Source-device vendor slug
TRAP_INTERFACE            Source-device topology interface (when topology enrichment is co-located, §13 Q4)
TRAP_NEIGHBORS            Source-device topology neighbors (same as above)
TRAP_JSON                 Structured varbind payload, JSON object
TRAP_SUPPRESSED_COUNT     Dedup summary entries only
TRAP_SUPPRESSED_FINGERPRINTS  Dedup summary entries only
TRAP_REPORT_PERIOD_SEC    Dedup summary entries only
```

`decode_error_summary` is reserved in the enum for forward compatibility; SOW-0035 does not emit that report type. Real trap entries use `trap`; deduplication summary entries are owned by the later deduplication SOW.

### Real trap entry (full example)

```
MESSAGE=Port-security violation: MAC aa:bb:cc:dd:ee:ff on GigabitEthernet0/1 (VLAN 10, ifIndex=12) on core-sw-01
PRIORITY=4
SYSLOG_IDENTIFIER=local
_HOSTNAME=core-sw-01
ND_LOG_SOURCE=snmp-trap
ND_NIDL_NODE=core-sw-01
TRAP_REPORT_TYPE=trap
TRAP_OID=1.3.6.1.4.1.9.9.315.0.1
TRAP_NAME=CISCO-PORT-SECURITY-MIB::ciscoPsmTrapSrvUnauthorized
TRAP_CATEGORY=security
TRAP_SEVERITY=warning
TRAP_PDU_TYPE=trap
TRAP_VERSION=v2c
TRAP_SOURCE_IP=10.0.0.5
TRAP_SOURCE_UDP_PEER=10.0.0.5
TRAP_DEVICE_VENDOR=cisco
TRAP_INTERFACE=GigabitEthernet0/1
TRAP_NEIGHBORS=dist-sw-01,dist-sw-02
TRAP_TAG_INTERFACE=GigabitEthernet0/1
TRAP_TAG_VLAN=10
TRAP_TAG_COMPLIANCE=pci
TRAP_TAG_TENANT=acme
TRAP_JSON={"cpsIfViolationMacAddress":{"oid":"1.3.6.1.4.1.9.9.315.1.2.1.1.1","type":"OctetString","value":"aa:bb:cc:dd:ee:ff"},"cpsIfViolationVlan":{"oid":"1.3.6.1.4.1.9.9.315.1.2.1.1.2","type":"INTEGER","value":10},"ifIndex":{"oid":"1.3.6.1.2.1.2.2.1.1","type":"INTEGER","value":12},"ifDescr":{"oid":"1.3.6.1.2.1.31.1.1.1.1","type":"OctetString","value":"GigabitEthernet0/1"}}
```

Note `SYSLOG_IDENTIFIER=local` (the job name — operator-meaningful) and `_HOSTNAME=core-sw-01` (the source device — the host the trap is about). The plugin-controlled topology fields `TRAP_INTERFACE` / `TRAP_NEIGHBORS` (from co-located topology enrichment) and the operator-defined labels `TRAP_TAG_INTERFACE` / `TRAP_TAG_VLAN` / `TRAP_TAG_COMPLIANCE` / `TRAP_TAG_TENANT` live in distinct namespaces — labels are always under `TRAP_TAG_*` and cannot collide with plugin field names.

`TRAP_INTERFACE` is the topology interface that owns the trap source IP in the co-located topology cache. `TRAP_NEIGHBORS` is the sorted, de-duplicated set of known LLDP/CDP neighbor sysNames for that source device cache; it is device-level topology context, not a claim that every listed neighbor is involved in the specific trap event. When the source IP matches the topology cache's local device management IP but has no interface-IP mapping, `TRAP_NEIGHBORS` can still be emitted while `TRAP_INTERFACE` is omitted.

**Cardinality contract** is visible: the MAC (`aa:bb:cc:dd:ee:ff`) appears in **MESSAGE** (templated, free-form) and in **TRAP_JSON** (full structured form). It does **NOT** appear in any `TRAP_TAG_*` operator label. The MAC is fully searchable in the journal (Logs UI substring search, `journalctl -g aa:bb:cc` for MESSAGE substring match, or `jq` against `TRAP_JSON`) without polluting metric label cardinality.

### Deduplication summary entry (full example)

Same journal file, different entry type. Distinguished by `TRAP_REPORT_TYPE`. See §10.

```
MESSAGE=DEDUPLICATED TRAPS: 247 events have been deduplicated in the last 5s:
- ifDown 120
- authenticationFailure 80
- ciscoPsmTrapSrvUnauthorized 47
PRIORITY=6
SYSLOG_IDENTIFIER=local
ND_LOG_SOURCE=snmp-trap
TRAP_REPORT_TYPE=deduplication_summary
TRAP_SUPPRESSED_COUNT=247
TRAP_SUPPRESSED_FINGERPRINTS=12
TRAP_REPORT_PERIOD_SEC=5
TRAP_JSON={"period_sec":5,"total_suppressed":247,"by_trap":{"1.3.6.1.6.3.1.1.5.3":120,"1.3.6.1.6.3.1.1.5.5":80,"1.3.6.1.4.1.9.9.315.0.1":47}}
```

The MESSAGE is multi-line as designed (§10) — the same shape as the §10 example. Multi-line MESSAGE values are written using systemd-journal's binary field encoding (see CWE-117 below) so newlines inside MESSAGE cannot inject other fields.

Filterable: `journalctl TRAP_REPORT_TYPE=trap` (real traps only), `journalctl TRAP_REPORT_TYPE=deduplication_summary` (suppression history only). New report types may be added later for other pipeline events (e.g., `decode_error_summary`).

### `TRAP_JSON` content

The full structured varbind payload as a single-line JSON object. Keyed by varbind symbolic name when known (profile inline `varbinds:` resolved it, or MIB index has it), by OID otherwise. Each value carries `{oid, type, value}` and optionally `enum`/`display_hint` rendering applied. If malformed input creates duplicate JSON keys after symbolic-name and OID fallback, the first occurrence keeps the base key and later occurrences use deterministic suffixed keys (`<key>#2`, `<key>#3`, ...), preserving every received varbind without introducing duplicate JSON object keys.

This guarantees forensic completeness: even with zero profile coverage and no MIB loaded, the journal contains everything needed to reconstruct what arrived on the wire. The fixed field name avoids the per-OID field-name explosion of older designs.

### Varbind value sanitization — binary field encoding (CWE-117 protection)

systemd-journal supports two field encodings: text-line (`KEY=value\n`) and **binary (size-prefixed)**. Text-line fields cannot contain newlines, NULL bytes, or other control characters — those bytes would be interpreted as field-record delimiters. Binary fields can carry ANY byte sequence including embedded newlines.

The plugin chooses encoding per-field at write time:

| Field value characteristics | Encoding |
|---|---|
| ASCII-printable / valid UTF-8, no newlines, no NULL, no control chars (0x00-0x1F except whitespace 0x09/0x20, no 0x7F) | text-line |
| Contains any newline, NULL, or control char | **binary, size-prefixed** |
| Deliberately multi-line MESSAGE values (e.g., the deduplication summary entry's MESSAGE in §10) | binary, size-prefixed |

This eliminates **CWE-117 (log injection)** structurally. A malicious varbind value of `injected_value\nFAKE_FIELD=spoofed\n` lands as ONE field with the bytes `injected_value\nFAKE_FIELD=spoofed\n` as its value — never as two separate journal fields. The selected writer backend's binary-field encoding handles this transparently when the writer chooses the binary form for fields containing control bytes.

The plugin's journal-write path applies this check uniformly to `MESSAGE`, all `TRAP_*` and `TRAP_TAG_*` fields, `ND_LOG_SOURCE`, `ND_NIDL_NODE`, `_HOSTNAME`, and `TRAP_JSON` values. No field bypasses the check.

### Forensics

All operator-facing questions (Phase A `operator-features.md` E1-E6) become journal queries through the existing Logs UI. No new UI code.

The optional standards-compliant OTLP exporter (operator opt-in, defaults off) is documented separately in §11b. It uses dotted-lowercase attribute names per OTEL semantic conventions and is intentionally vendor-neutral — Netdata's own OTEL plugin is one possible receiver among many.

## 11b. OTLP Exporter Attribute Universe — optional second backend

When the operator enables the OTLP exporter (defaults off; configured per §7.5), every trap also lands as a vendor-neutral OTLP LogRecord that any OTEL-compatible receiver can ingest (Netdata's own OTEL plugin, Splunk, Datadog, Grafana Cloud, OpenSearch, vendor-X). The OTLP path is intentionally standards-compliant — it does NOT use the journal field names from §11 because OTEL attribute naming convention is dotted-lowercase. The journal-direct path (§11) remains the high-fidelity Netdata-native path.

The plugin emits both shapes from a single internal `TrapEntry` model — the writer interface applies the per-backend naming convention at serialization time.

### Standard OTLP LogRecord fields

| OTLP field | Source | Notes |
|---|---|---|
| `body` | Rendered description template | Primary log message (counterpart to journal `MESSAGE`) |
| `severity_number` | Mapped from syslog PRIORITY | See severity mapping below |
| `severity_text` | Mapped severity name | OTLP standard text label |
| `EventName` (top-level LogRecord field) | `"snmp.trap.<category>"` (e.g. `snmp.trap.security`) | The OTel Logs Data Model `EventName` field (OTLP proto field `event_name` in `LogRecord`) — top-level typed-event identifier per the OTel spec. Enables backend-side category routing without parsing attributes. Dotted-lowercase per OTEL semconv. Dedup summary entries use `"snmp.trap.deduplication_summary"`. Note: the OTel semconv attribute `event.name` is a separate concept (an attribute key, lowercased+dotted); Netdata uses the top-level `EventName` field here. |
| Resource attribute `service.name` | `"netdata-snmptrap"` (constant — identifies the Netdata producer) | OTEL standard |
| Resource attribute `service.instance.id` | `<job_name>` (the listener job, matches journal `SYSLOG_IDENTIFIER`) | OTEL standard |

### Severity mapping (syslog → OTLP, per OpenTelemetry Logs Data Model Appendix B "Mapping of SeverityNumber" for syslog)

| Profile slug | Syslog PRIORITY | OTLP `severity_number` | OTLP `severity_text` |
|---|---|---|---|
| `emerg`   | 0 | 21 | FATAL |
| `alert`   | 1 | 19 | ERROR3 |
| `crit`    | 2 | 18 | ERROR2 |
| `err`     | 3 | 17 | ERROR |
| `warning` | 4 | 13 | WARN |
| `notice`  | 5 | 10 | INFO2 |
| `info`    | 6 |  9 | INFO |
| `debug`   | 7 |  5 | DEBUG |

These are the exact values from the OpenTelemetry Logs Data Model Appendix B syslog table — confirmed against https://opentelemetry.io/docs/specs/otel/logs/data-model-appendix/. The mapping preserves syslog severity ordering: `emerg > alert > crit > err > warning > notice > info > debug` translates to `21 > 19 > 18 > 17 > 13 > 10 > 9 > 5`.

OTLP `severity_number` loses the 8-slug Netdata taxonomy (only one of `emerg/alert/crit` lands inside the FATAL range; the other two map into the ERROR range). The canonical Netdata slug is preserved as a separate attribute `snmp.trap.severity` so downstream OTEL backends can group by Netdata's native taxonomy when desired.

### Attribute namespaces

| Namespace | Used for |
|---|---|
| OTEL standard (`network.*`, `service.*`, `event.*`) | Use the official semantic-convention name |
| `snmp.*` | SNMP-protocol facts (no OTEL semconv exists) |
| `netdata.*` | Netdata-platform enrichment |
| `trap.*` | Operator-defined attributes (from profile `labels:` or plugin config `labels:`) |

### Attribute table (mirror of the §11 journal field universe)

| OTLP attribute | Journal-path equivalent (§11) | Notes |
|---|---|---|
| `network.peer.address` | `TRAP_SOURCE_UDP_PEER` | OTEL standard for transport peer. **Always emitted.** The UDP socket's `recvfrom()` source address. |
| `snmp.source.ip` | `TRAP_SOURCE_IP` | Identified source per §5 step 6 cascade (`snmpTrapAddress.0` → v1 `agent-addr` → UDP peer). **Always emitted.** Carries the same value as `network.peer.address` when no proxy / NAT / explicit `snmpTrapAddress.0` is involved; carries a different value when the trap was relayed (e.g., via NAT, proxy, or sender-set `snmpTrapAddress.0` differing from UDP peer). Both attributes always present preserves query uniformity across all entries. |
| `snmp.version` | `TRAP_VERSION` | v1 / v2c / v3 |
| `snmp.trap.oid` | `TRAP_OID` | Numeric OID |
| `snmp.trap.name` | `TRAP_NAME` | MIB-qualified `<MIB>::<symbol>` |
| `snmp.trap.category` | `TRAP_CATEGORY` | One of the 8 canonical category slugs |
| `snmp.trap.severity` | `TRAP_SEVERITY` | Profile slug — kept alongside OTLP `severity_number` to preserve Netdata 8-slug taxonomy |
| `snmp.trap.pdu_type` | `TRAP_PDU_TYPE` | trap / inform |
| `snmp.trap.report_type` | `TRAP_REPORT_TYPE` | trap / deduplication_summary / decode_error_summary |
| `snmp.device.hostname` | `_HOSTNAME` | Intentionally NOT `host.name` — the SNMP device is not the OTEL host of the producing process |
| `snmp.device.vendor` | `TRAP_DEVICE_VENDOR` | Vendor slug from PEN |
| `snmp.varbinds` | `TRAP_JSON` | Structured nested object (OTEL supports nested attributes) — receivers may flatten as `snmp.varbinds.<name>.<field>` |
| `netdata.nidl.node` | `ND_NIDL_NODE` | Netdata vnode identity of the source device — correlates trap entries with the same device's polled metrics |
| `netdata.topology.interface` | `TRAP_INTERFACE` | Topology overlay |
| `netdata.topology.neighbors` | `TRAP_NEIGHBORS` | Topology overlay |
| `trap.<key>` (lowercase original key) | `TRAP_TAG_<KEY_UPPERCASE>` | Labels from profile YAMLs and operator per-job config |

### Deduplication summary entry — OTLP shape

For dedup summary entries (§10), the same translation applies:

- `body` = rendered summary line (e.g., `"Suppressed 247 duplicate traps in the last 5s — ifDown×120, authenticationFailure×80, …"`)
- `EventName` (top-level LogRecord field) = `"snmp.trap.deduplication_summary"`
- `severity_number` = 9 (INFO)
- `snmp.trap.report_type` = `"deduplication_summary"`
- `snmp.trap.suppressed_count`, `snmp.trap.suppressed_fingerprints`, `snmp.trap.report_period_sec` carry the structured counts
- `snmp.varbinds` carries the per-trap suppression map

### Injection-safety in the OTLP path

OTLP is protobuf-encoded — attribute values are length-prefixed and cannot escape their field boundary on the wire. Newlines, NULs, and other control bytes in varbind values traverse OTLP safely. Downstream receivers' handling is out of scope for this spec; Netdata's own OTEL plugin (`src/crates/netdata-otel/otel-plugin/src/logs_service.rs`) writes flattened key/value pairs to systemd-journal using the journal-writer crate, so the same byte-safety properties of that crate apply when traps reach the journal via the OTEL path.

## 12. Plugin-Self Metrics (NIDL contexts) — always emitted

Three NIDL contexts. Contexts 1 and 2 (`snmp.trap.events`, `snmp.trap.errors`) are **always emitted**. Context 3 (`snmp.trap.dedup_suppressed`) is **conditionally emitted** — only when at least one job has opt-in dedup enabled (see §10). The SOW-0036 implementation emits contexts 1 and 2 with `job_name` only. The richer per-device labels below are the target contract for the SOW-0037/SOW-0039 enrichment/collector-consistency closeout, where device vnode identity is available.

### Context 1: `snmp.trap.events`

| Aspect | Value |
|---|---|
| Instance | Per device (one instance per source device the hub knows about) |
| Dimensions | Categories: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown` (the closed 8-slug set per §3) |
| Unit | events/s (incremental counter) |
| Labels | `job_name`, `severity` (one of the 8 syslog severities), `device`, `vendor`, `hub`, plus operator-defined labels from profile/plugin config |
| Node / vnode | Per-device vnode — **inherited from the SNMP polling subsystem**. The trap plugin does not create vnodes; it emits metrics against the vnodes that SNMP polling already established for each monitored device. If polling is not configured for a given device, traps for that device emit against the hub vnode with `device` as a label. |
| Title | "SNMP Traps Received" |

This is one chart. Operator slices/dices via NIDL controls:
- Filter to severity=crit → see crit events only across devices.
- Group by device → per-device breakdown.
- Group by category → per-category breakdown across the fleet.
- Filter to one vnode (device) → drill into that device's recent traps.
- Group by `job_name` → per-listener volume.

### Context 2: `snmp.trap.errors`

| Aspect | Value |
|---|---|
| Instance | Per job (one instance per listener) |
| Dimensions | `unknown_oid`, `decode_failed`, `template_unresolved`, `malformed_pdu`, `dropped_allowlist`, `rate_limited`, `auth_failures`, `usm_failures`, `unknown_engine_id`, `inform_response_failed`, `sanitized`, `profile_load_failed`, `journal_write_failed` |
| Unit | events/s (incremental counter) |
| Labels | `job_name`, `hub`, possibly `source_device` where source is identifiable |
| Node / vnode | Hub vnode |
| Title | "SNMP Trap Pipeline Errors" |

Operators alert on these for pipeline health:
- `unknown_oid > 0 sustained` → profile coverage gap; consider adding a profile YAML to `/etc/netdata/go.d/snmp.trap-profiles/` (operators convert MIBs offline — see §7).
- `decode_failed > 0` or `malformed_pdu > 0` → investigate sending device.
- `template_unresolved > 0` → profile template references a varbind not present in incoming traps; fix profile or sender.
- `dropped_allowlist > 0` sustained → policy drop before journaling: source CIDR miss, community miss, or SNMP version disabled for this listener.
- `rate_limited > 0` → trap storm signal; investigate source.
- `auth_failures` / `usm_failures` / `unknown_engine_id` > 0 → SNMPv3 misconfiguration on sender or whitelist on receiver.
- `inform_response_failed > 0` → INFORM Response send failures; investigate UDP socket health.
- `sanitized > 0` sustained → varbind values containing control characters being binary-encoded; investigate sender.
- `profile_load_failed > 0` → operator-provided profile YAML failed to parse during DynCfg hot-reload (SOW-0037 M3); plugin continues with the previous profile index; operator must fix the bad YAML and reload.
- `journal_write_failed > 0` → disk-full, permission, or filesystem error while writing to the per-job journal directory; trap is dropped, hot path continues (the writer never blocks).

### Context 3: `snmp.trap.dedup_suppressed` (only when opt-in dedup is enabled on at least one job)

| Aspect | Value |
|---|---|
| Instance | Per job (only jobs with `dedup.enabled: true`) |
| Dimensions | Single dimension: `suppressed` |
| Unit | events/s (incremental counter) |
| Labels | `job_name`, `hub` |
| Title | "SNMP Trap Deduplication Suppressed" |

Operators alert on `dedup_suppressed > X/sec sustained` as a flap-storm signal. When dedup is disabled (the default), this metric is not emitted.

### Operator-opted-in per-OID metrics

In addition to the two plugin-self contexts, operator-selected OIDs (via §7.5) produce their own context. Naming convention: `snmp.trap.<vendor>_<short_name>` (e.g., `snmp.trap.cisco_config_changes`). These are surgical, opt-in, and shaped per the operator's alerting needs.

## 13. Open Questions (post-Phase-B status)

Phase B resolved most of the original questions. What remains:

1. **Go process / writer backend** — **ACCEPTED (SOW-0035 M1, ADR-0001; amended 2026-05-26)**: Standard in-process go.d collector V2 module with a thin SDK-backed Go journal adapter over `github.com/netdata/systemd-journal-sdk/go/journal` `go/v0.1.0`. No separate process, no CGo, no subprocess bridge. See `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md`.

2. **Profile YAML hot-reload mechanism**: the plugin needs to detect operator-added/edited YAML files under `/etc/netdata/go.d/snmp.trap-profiles/` and refresh the in-memory profile index. Two options: inotify on the directory, or explicit `dyncfg`-triggered reload. Phase B recommended inotify is overkill for the new-profile cadence; explicit reload via DynCfg is simpler and matches the per-job lifecycle. SOW-0037 M3 resolves.

3. **MIB upload via API**: out of scope for first release. Operators convert MIBs offline using `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` and drop the resulting YAML; this is documented user workflow (see §7 and the profile-format documentation). UI-driven upload via DynCfg is a future enhancement.

4. **Topology integration when topology is NOT co-located**: omit `TRAP_INTERFACE` / `TRAP_NEIGHBORS` journal fields entirely when topology state is unavailable. Same rule for the corresponding OTLP attributes (`netdata.topology.interface`, `netdata.topology.neighbors`) — omit, do not emit empty strings.

5. **v3 USM Secrets binding UX**: per-job USM users reference Netdata Secrets by name. Exact schema follows the SNMP polling plugin's existing pattern. SOW-0036 M1 finalizes.

6. **Northbound trap re-emit**: SaaS-cohort lacks this. Marked Non-Goal for SOW-0035–SOW-0039 (§14). The journal-as-source design enables a future SOW to add this cleanly when operator demand surfaces.

7. **`TRAP_JSON` shape**: object keyed by symbolic name with OID + type + value inside each entry. Profile-extracted names ensure stability across re-extractions. Edge case for vendors with duplicate symbolic names within a single MIB module (rare) — fall back to OID-keyed for the duplicates, documented in `profile-format.md`.

### Resolved by user decisions (this design pass)

- **Implementation language** — Go.
- **Listener endpoint model** — per-listener (one job = one listener = one or more endpoints = one writer = one journal dir). Multiple listeners are scaling/isolation, not the only way to accept multiple ports/protocols.
- **Creation-time failure model** — all resource failures needed to start a listener are DynCfg apply failures, including endpoint bind, unsupported protocol, profile load, journal directory create/open, writer initialization, and retention validation.
- **Profile memory model** — profiles load on first runnable trap job creation, are shared across listeners, and are released when no runnable trap jobs remain.
- **Dedup default key** — `(source_device, trap_OID)` only; profiles override per-OID via `dedup_key_varbinds:`.
- **Dedup default state** — disabled; opt-in per-job (§10).
- **DynCfg lifecycle** — job-level restart on config change; plugin process does not restart.
- **Per-job retention** — 10GB default, configurable max-size and/or max-duration, mirrored from NetFlow plugin pattern.
- **MIB compilation** — none at runtime; operators convert offline via the shipped helper, drop YAML.
- **Profile override merge** — multipath + filename-dedup + extends-chain field-merge, mirrored from SNMP polling plugin (`src/go/plugin/go.d/collector/snmp/ddsnmp/load.go`).
- **Trap `name` field** — required, MIB-qualified.
- **Spec example varbind layout** — file-scoped table (matches the shipped `profile-format.md` schema).
- **Collector consistency bundle** — owned by SOW-0039 (the final SOW); SOW-0035–0038 are not independently mergeable (single PR sequence ending at SOW-0039 M6).
- **OTel severity mapping** — corrected to OTel Logs Data Model Appendix B values (§11b).
- **SNMPv3 receiver-local engine ID** — `local_engine_id` is optional; when omitted, Netdata generates and persists a stable per-job value at `/var/lib/netdata/snmp-trap/{job_name}/local-engine-id`. This state is used for v3 INFORM authentication/Responses and is separate from the v3 Trap sender `engine_id_whitelist`.
- **snmpEngineBoots persistence** — per-job receiver-local counter at `/var/lib/netdata/snmp-trap/{job_name}/engine-boots` (SOW-0036 M1).
- **CWE-117 scope** — applies to the journal writer path; OTLP path is wire-safe via protobuf encoding. Downstream OTLP receivers' encoding choices are out of scope.

## 14. Non-Goals (deliberately not building)

- Built-in alert engine for traps (existing Netdata alert engine on emitted metrics).
- Built-in notification routing (existing channels).
- Built-in dedicated trap UI (existing Logs UI on the journal).
- Built-in alarm-lifecycle state machine (open/ack/clear, paired-clear linking `linkUp` to `linkDown`) — alert-engine territory; out of scope for this design pass.
- Central correlation across hubs (hub-local by design choice).
- Automatic device profiling (Rule 4 — operator drops a profile YAML; everything else decodes automatically).
- Profile YAML controlling journal field names (the journal always captures all varbinds; no profile knob needed).
- Profile YAML controlling metric emission (metric emission is operator choice in plugin config).
- **Runtime MIB compilation** — no `pysmi`/`gosmi`/Rust-MIB-crate dependency at runtime. Operators convert MIBs to profile YAMLs offline using `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` (see §7). This mirrors the SNMP polling plugin's pattern.
- **DTLS / TLS-TM** — Phase A finding: zero cohort systems support this (universal gap). Defer until production demand surfaces and mature libraries exist.
- **Intra-listener multi-writer sharding** — operators scale by adding more jobs when one listener's writer ceiling is exceeded or when they need isolation. A single listener can already own multiple endpoints; the documented operational threshold for splitting is throughput or materially different policy, not the need for another port alone.
- **Northbound trap re-emit (trap forwarding)** — design enables it cleanly via the journal, but not in scope for SOW-0035–0039. Future SOW if operator demand surfaces.
- **Trap-driven topology refresh** — receiving `linkDown` does not trigger an immediate topology re-poll. Topology refresh remains on its 30-min schedule (consistent with the existing SNMP polling pattern). Future enhancement if cohort evidence shows operational pain.
- **Per-OID dedup window** — global per-job dedup window. Per-OID window is deferred; no cohort system found to require it.
- **MIB upload via DynCfg API** — file-drop only for first release.
- **Trap storm detection across sources** — per-source rate limiting + dedup are the in-plugin shields. Aggregate storm detection is an alert-engine concern over the metrics in §12.

## 15. Existing-Netdata Leverage Points

Pipeline reuses (not rebuilds) these existing pieces:

| Concern | Existing Netdata |
|---|---|
| Alert evaluation | Alert engine (real-time on metric updates) |
| Notifications | Existing channels: Slack, Discord, PagerDuty, ServiceNow, OpsGenie, email, webhook |
| Secret storage | Existing Secrets (community strings, v3 USM keys) |
| Forensics UI | Logs UI on top of systemd-journal source |
| Topology surface | Existing SNMP topology — annotation overlay, drilldown to trap detail |
| Multi-host distribution | Netdata Cloud aggregates hubs for presentation only — no correlation across hubs |
| Configuration | `dyncfg` for runtime job config and profile YAML hot-reload (no runtime MIB compilation) |
| Function surface | Existing Functions framework (`logs:`, `topology:`, `dyncfg:` patterns) |
| UDP-overflow alerting | Built-in alerts on all UDP listeners |
| Per-device vnodes | Existing SNMP polling pattern |

## 16. Cohort-Win Audit (what this design wins relative to the cohort)

| Phase A cohort gap | This design |
|---|---|
| 1. No cohort system does listener-layer per-source rate-limit | We ship per-job, default-off `rate_limit:` (token-bucket per source IP, §7.5) for operators who need to shield the plugin from misbehaving senders flooding with **unique** traps, plus first-wins opt-in plugin-level dedup with periodic summary entries (§10) for **identical** repeats. The two solve different problems and can be enabled independently per job. |
| 2. No cohort system supports DTLS/TLS-TM | Deferred — universal cohort gap. Out of scope for SOW-0035–0039; revisited when production demand surfaces AND mature Rust/Go libraries exist (§14 Non-Goals). |
| 3. No cohort system does topology-aware suppression | **Hub-local enrichment makes this cheap.** Journal carries upstream-device identity; alert engine can suppress downstream alerts when upstream is in alarm. |
| 4. MIB management divergence is extreme | **Ship comprehensive vendor packs derived from public MIBs (437 vendor PENs / 71,787 traps in the OOB pack) + offline conversion tooling for operator MIBs (no runtime compilation) + profile YAML hot-reload via DynCfg.** Targets the LibreNMS-to-Datadog coverage band with the SNMP polling plugin's stock+override pattern. |
| 5. NSTI shipped-defects cautionary tale | Acknowledged. First-release quality bar enforced via the same per-system review protocol that produced the cohort specs. |

## 17. Implementation Sequencing — 5-SOW Plan

The implementation is tracked through five sequential SOWs under `.agents/sow/pending/`. Each ships with reviewer rounds per the AGENTS.md rerun rule. SOW-0035 through SOW-0038 land on a feature branch and are NOT independently mergeable; the merge gate is SOW-0039 which ships the collector consistency bundle (per AGENTS.md "7-artifact rule"), the systemd-journal facet registration, user documentation, and the SOW-0032 closeout.

| SOW | Scope | Acceptance |
|---|---|---|
| **SOW-0035** | Go implementation architecture decision (process model, journal-writer backend, TrapWriter interface contract); multi-endpoint listener (per-job DynCfg orchestration; every configured endpoint binds or job creation fails with HTTP-422 surfaced in DynCfg — no automatic high-port fallback) + SNMPv1/v2c decode + source identification + replayable pcap test corpus; shared lazy profile YAML loader loaded on first runnable job creation (multipath, filename-dedup, extends-chain merge — mirroring the SNMP polling plugin) + OID index + 2-tier varbind resolution + template rendering; journal writer per-job (one journal directory per job at `/var/cache/netdata/traps/{job_name}/`, retention config with intentional deviation on the `max_duration` default) with creation-time directory/writer preflight and CWE-117 binary-field encoding | Operator sees decoded trap from a replayed pcap in a per-job journal directory |
| **SOW-0036** | SNMPv3 USM (static engineID whitelist, per-job) + INFORM acknowledgement + `snmpEngineBoots` persistence per job; per-job allowlist + rate limiting; plugin configuration schema + DynCfg per-job orchestration refinement; plugin-self NIDL metrics (per-job dimensions, full error universe per §12, including BER limit violations from §18) | Production-grade per-job auth + rate limiting + telemetry |
| **SOW-0037** | Cross-plugin enrichment (sysName/vendor/topology); **opt-in** deduplication (per-job, disabled by default — see §10) with periodic summary entries; profile YAML hot-reload via DynCfg (no runtime MIB compilation per §14); operator per-OID metric opt-in | Operational depth: enriched, optionally deduped, hot-reloadable |
| **SOW-0038** | Throughput benchmark harness; SNMPv3 dynamic engineID discovery (opt-in); standards-compliant OTLP exporter (§11b — optional, vendor-neutral; works with Netdata's OTEL plugin and any OTLP-compliant receiver) | Scale + interop |
| **SOW-0039** | **Collector consistency bundle**: `metadata.yaml` + `config_schema.json` + stock `.conf` + `health.d/snmp_trap.conf` + `README.md` + `taxonomy.yaml` (passes `check-markdown.yml` + `check_collector_taxonomy.py` CI gates). **systemd-journal default-facet registration** in `src/collectors/systemd-journal.plugin/systemd-journal.c` `SYSTEMD_KEYS_INCLUDED_IN_FACETS` for the plugin-controlled `TRAP_*` field names (operators pick `TRAP_TAG_*` labels via the UI's facet selector — macro is defaults only). **End-user AI skill `query-snmp-traps`** (`docs/netdata-ai/skills/query-snmp-traps/`) + how-tos catalog. **User documentation** for the offline MIB-to-YAML conversion workflow (per §7). **SOW-0032 comparative-analysis.md closeout**. **Final merge gate** — single PR sequence ending here. | Mergeable, CI-passing, documented |

The OTLP exporter (§11b) is intentionally deferred to SOW-0038 — it is optional, operator-opt-in, and not part of the MVP. The journal-direct backend (§11) is the load-bearing path and ships in SOW-0035.

DTLS/TLS-TM and conversion tools for additional NMS systems (OpenNMS, Centreon, LibreNMS, Zenoss) are out of scope for the 5-SOW lineup; they remain follow-up candidates after SOW-0039 closes.

## 18. BER decode resource limits (DoS protection)

The hot path (§5) decodes untrusted UDP-delivered ASN.1 BER. Malformed or hostile PDUs are a DoS surface. The plugin enforces hard limits per trap; exceeding any of them drops the trap, increments `snmp.trap.errors.malformed_pdu`, and continues processing.

| Limit | Default | Rationale |
|---|---|---|
| Max UDP datagram bytes | 8192 (8 KiB) | RFC 3417 §3 recommends 484-byte minimum receive capability; 8 KiB covers practical real-world traps (including verbose Cisco/Juniper varbinds) with margin. |
| Max varbinds per PDU | 256 | Cohort survey: largest legitimate trap observed in `splunk-sc4snmp` fixture corpus uses ~80 varbinds. 256 is a 3x safety margin. |
| Max constructed BER nesting depth | 8 | SNMPv3 message structure tops out at ~6 constructed levels; 8 covers all standard PDU shapes. |
| Max OID encoded length | 128 bytes | RFC 2578 OID encoding cap. |
| Max OctetString varbind value | 1024 bytes | Long enough for real-world MIB strings, short enough to bound memory per trap. |
| Per-PDU decode budget | 1 ms | Wall-clock budget per PDU on the hot path. Exceeding triggers drop. |

These limits apply per-job (each job has its own decoder thread per §5) with the same default values listed in the table. They are not operator-configurable. The wording "global" here means the same defaults apply across every job; it does NOT mean a single shared decoder state — each job's decoder enforces the limits independently against its own UDP buffer.

## 19. TrapWriter interface contract

The `TrapWriter` interface is the contract between the trap pipeline and the storage/transport backends. It must accommodate both the journal-direct path (§11) and the opt-in OTLP path (§11b) without retrofit.

**Status: Accepted (SOW-0035 M1, ADR-0001, reviewer round 5 findings folded in).** The Go interface definition and `TrapEntry` struct are recorded in `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md` §3-4. Key design decisions:

- The interface is `Write(entry *TrapEntry) error`, `Flush() error`, `Close() error` — fast accept into backend-owned bounded queues, backend-internal batching
- CWE-117 encoding is owned by the journal writer backend, not the interface
- The journal-direct backend implements this interface by calling the Go-native `JournalWriter.WriteEntry()` method
- The OTLP backend (SOW-0038) implements the same interface with protobuf serialization
- `TrapEntry` is the language-neutral semantic representation; field names are semantic (not journal/OTLP); the writer applies its backend's naming convention at serialization time

### Minimal contract (Go)

```
TrapWriter:
  Write(entry *TrapEntry) error    # fast accept into backend-owned queue or return drop-worthy error
  Flush() error                    # drains pending writes and fsyncs/syncs backend state
  Close() error                    # closes the writer; idempotent
```

Semantics:

- **Fast `Write`** (returns when the backend has accepted ownership of the immutable entry into a bounded queue, not when it is durable; `Flush()` is the durability boundary) — the journal-direct backend serializes queued entries into the per-job journal files via the Go-compatible backend selected in SOW-0035 M1 (bypassing journald so the writer controls `_HOSTNAME` and other "trusted" fields). The OTLP writer batches internally (default 200ms flush window) and `Write` returns as soon as the entry is enqueued into the batch buffer.
- **Hot-path non-blocking behavior** — `Write` must not perform blocking disk or network I/O on the decode hot path. If the backend queue is full or the writer is in a permanent failed state, `Write` returns an error; the caller increments `journal_write_failed` and drops the trap while the hot path continues.
- **Ownership** — on successful `Write`, the caller transfers ownership of the `TrapEntry` to the writer and must not mutate maps, slices, or strings reachable from it. Reusing a `TrapEntry`, `Labels` map, `SummaryCounts.ByTrap` map, or `Varbinds` backing array after successful `Write` is a correctness bug unless the implementation deep-copies the reused data first. On failed `Write`, ownership does not transfer, but the caller still discards the entry and uses a fresh entry for the next trap. The writer must not retain references on an error return.
- **Default queue / flush policy** — journal-direct writer queue defaults to 10,000 entries per job; queue-full and permanent writer failure are both drop-and-continue errors. The journal-direct writer flushes every 1s or 1,000 accepted entries, whichever comes first, and on `Flush()` / `Close()`.
- **Backend-internal batching** — the interface does not expose batching. Each writer decides its own batching strategy.
- **Error handling** — `Write` returns error only for drop-worthy conditions (for example, journal writer permanently failed, internal queue full, or OTLP receiver unreachable and retry buffer full). Transient backend failures are absorbed by the writer's internal retry/buffering policy until the bounded buffer is exhausted.
- **CWE-117 ownership** — the **journal writer** owns CWE-117 binary-field encoding (§11); the OTLP writer relies on protobuf field encoding for wire safety (§11b). `TrapEntry` carries raw values; the writer applies whatever encoding its backend needs.
- **`Flush` / `Close` concurrency** — `Flush` creates a barrier and waits for entries accepted before that barrier to be written and synced; entries accepted after the barrier may flush later. `Close` stops new acceptance, drains, finalizes, syncs, and closes. Calling `Close` multiple times is safe; later calls return nil after a successful first close or the stored terminal error after a failed first close.

### `TrapEntry` shape

`TrapEntry` is the language-neutral semantic representation. Field names are semantic (not journal nor OTLP); the writer applies its backend's naming convention.

| Field | Type | Notes |
|---|---|---|
| `JobName` | string | Which job produced this entry. |
| `ReportType` | enum {`trap`, `deduplication_summary`, `decode_error_summary`} | §11. `decode_error_summary` is reserved and not emitted in SOW-0035. |
| `ReceivedRealtimeUsec` | int64 | Wall-clock receive timestamp captured on the recv path; journal realtime and OTLP `time_unix_nano` derive from this |
| `ReceivedMonotonicUsec` | int64 | `CLOCK_MONOTONIC` receive timestamp captured with `ReceivedRealtimeUsec`; journal monotonic timestamp derives from this |
| `TrapOID` | string | Numeric OID |
| `TrapName` | string | MIB-qualified `<MIB>::<symbol>` |
| `Category` | enum (8 slugs) | §3 |
| `Severity` | enum (8 slugs) | §11/§11b syslog scale |
| `Message` | string | Rendered description template; CWE-117 raw bytes |
| `SourceIP` | string | Validated and normalized identified source (§5 step 6) |
| `SourceUDPPeer` | string | Transport peer |
| `DeviceHostname`, `DeviceVendor` | string, string | Enrichment. Empty `DeviceHostname` makes `_HOSTNAME` fall back to `SourceIP`; empty `DeviceVendor` is omitted. Hostname priority: SNMP registry `VnodeHostname` > SNMP registry `SysName` (excluding literal `"unknown"`) > topology-matched `SysName` > reverse DNS cached name (only when `reverse_dns.enabled: true`) > `SourceIP`. |
| `PduType` | enum {`trap`, `inform`} | |
| `SnmpVersion` | enum {`v1`, `v2c`, `v3`} | |
| `SourceVnodeID` | string | Source device's Netdata vnode identity. Journal serializer maps to `ND_NIDL_NODE`; OTLP serializer maps to `netdata.nidl.node`. Empty when SNMP polling has no state for the device. |
| `TopologyInterface`, `TopologyNeighbors` | string, string | Enrichment; omitted from journal and OTLP output when empty |
| `Labels` | map<string, string> | Profile + operator labels. Nil means no labels. Lowercase keys → `TRAP_TAG_<KEY>` in journal, `trap.<key>` in OTLP |
| `Varbinds` | ordered list of `{Name, OID, Type, Value, Enum?}` | Structured; writer serializes to `TRAP_JSON` or `snmp.varbinds`; `display_hint` is reserved and not emitted initially |
| `SummaryCounts` | optional `{TotalSuppressed, Fingerprints, PeriodSec, ByTrap}` | Only when `ReportType=deduplication_summary`; `ByTrap` is keyed by numeric OID and the MESSAGE renderer resolves names from the profile index when available |

The interface contract is defined formally in `.agents/sow/specs/snmp-traps/decisions/0001-go-process-and-trapwriter.md` §3-4 (SOW-0035 M1, ADR-0001). The SOW-0038 M3 OTLP exporter implements the same interface as the SOW-0035 M4 journal writer.

End of design proposal.
