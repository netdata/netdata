# Netdata — SNMP Trap Support: Design Proposal (Pipeline + Storage Scope)

## 0. Document Metadata

- **System**: Netdata Agent — proposed SNMP trap reception, decode, enrichment, metric generation, and journal storage subsystem.
- **Scope of this document**: the **ingestion-through-storage pipeline only**. Alerting (existing Netdata alert engine), notifications (existing channels), forensics UI (existing Logs UI), secret storage (existing Secrets), topology integration (existing topology), and user/RBAC (existing Cloud spaces/rooms) are explicitly out of scope as they are handled by existing Netdata subsystems. This document deliberately does NOT redesign any of those.
- **Architectural premise**: distributed by design — one Netdata Agent per site is appointed as that site's SNMP hub. The hub consolidates polling, discovery, topology, traps, NetFlow, and syslog. There is no central correlation tier; correlation happens locally on each hub. This is a deliberate design choice for unbounded horizontal scalability. All decisions below assume it.
- **Author**: assistant.
- **Status**: design proposal, Phase A (cohort-naïve matrix) complete, Phase B stress-test pending.
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

1. **Every trap is captured in the journal.** First occurrences within the dedup window land as full structured journal entries (one per real event). Subsequent identical occurrences within the window are deliberately collapsed (deduplication is a feature, not a side effect — see §10) and surfaced periodically through a dedup-summary entry that names the suppressed OIDs and counts. No silent loss; collapsed traps are visible via the summary entry's MESSAGE and SNMP_TRAP_JSON fields.
2. **Plugin-self metrics** (two NIDL contexts — see §12) are emitted always for plugin health monitoring.
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

1. **Journal field** (`SNMP_TRAP_CATEGORY`) — operators query journals by category.
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

PDU type (TRAP / INFORM / v1) is **not** a category — it is recorded separately in the journal field `SNMP_TRAP_PDU_TYPE`. The same OID can arrive as either TRAP or INFORM with identical meaning; the content category is independent of how it was delivered.

**Note on traps with polled metric equivalents** — several `state_change` traps (e.g., `linkDown`, BGP transitions, fan/PSU/temp) have Netdata polled-metric equivalents (`ifOperStatus`, `bgpPeerState`, env sensors). Operators should alert on the polled metric, which carries the **current** state and supports hysteresis/ML, not on the trap counter. The trap journal still has forensic detail (reason codes, peer identity, change actor) that the polled metric does not carry. A future release may add a `metric_filter` profile field that explicitly links a trap to its polled equivalent so the Logs UI can show the metric time-series next to the trap detail.

## 4. The Cardinality Discipline Rule

Cardinality discipline applies **only** to metric labels and dimensions. The journal — including templated MESSAGE content — has no cardinality constraint and is the proper home for high-cardinality detail (MAC, IP, username, packet content, RAID slot, etc.).

| Surface | Cardinality rule | Why |
|---|---|---|
| Description template → `MESSAGE` field | **No restriction** — use any varbind | MESSAGE is per-row free-form text. 10k distinct MAC values = 10k journal entries with different MESSAGEs. That's normal journal behavior; it is **the** place high-cardinality detail belongs. |
| Varbind capture → `SNMP_TRAP_JSON` field | **No restriction** — all varbinds always | Single structured field; per-row JSON. No per-OID journal-field-name pollution. |
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

The forensic question "which MAC violated port-security on switch X today" is a journal query (Logs UI), not an alert. The metric "port-security violations on switch X" fires the alert; the operator clicks through to the journal — where the MAC is in the templated MESSAGE field and in the `SNMP_TRAP_JSON` structured varbind payload.

Plugin enforces this structurally: label templates with unbounded-cardinality varbind references are **rejected at config-load** with a clear error. Description templates have no such check.

## 5. Plugin Architecture

### Language (open question — both options viable)

| Option | Pros | Cons |
|---|---|---|
| **Rust** | Reuses the existing Rust systemd-journal writer directly (in-process). Strongest performance envelope. Memory safety. | Needs `netipc` to read state from the Go SNMP polling and topology plugins for enrichment. |
| **Go** | Direct in-process interop with the existing Go SNMP polling and topology plugins (shared structs, no IPC for enrichment). | Needs `netipc` to write to the Rust journal writer, OR a Go port of the journal writer. |

Either way one cross-language boundary exists. The decision is whether to pay it on the **enrichment** path (Rust) or on the **journal-write** path (Go). Defer the call until prototype benchmarks; both are workable. Phase B should expose this trade-off.

### Concurrency model

- **Hot path** (per-packet decode + enrich + counter increment): single thread per listener, zero-allocation.
- **Journal write**: single writer per journal file. The journal writer caps at ~30k rows/sec per writer thread. To exceed that, partition: either run multiple writer threads each owning its own journal file, or shard listeners → writers by source-IP hash. Both options preserve "single writer per file" while permitting horizontal scaling on a single hub.

### Process model

External collector loaded by Netdata via the PLUGINSD protocol over stdio pipes. Pipes + plugins.d are battle-tested for this. **The plugin lifecycle follows the Netdata agent's lifecycle** — restarting Netdata restarts the plugin (the standard Netdata pattern; do not invent a separate lifecycle).

### Hot path (executes per trap)

1. UDP recv into a reusable buffer (no allocation).
2. BER decode in place (no copy).
3. Identify source: PDU-based first (v1 `agent-addr`; v2c/v3 `snmpTrapAddress.0` varbind per RFC 3584); fall back to UDP peer. Pattern from LogicMonitor EA 36.100+ (`logicmonitor.md` §3.4) — the cohort's cleanest source-identification logic.
4. OID lookup against the prebuilt MIB index (perfect-hash or radix-trie at scale).
5. Apply profile entry: category tag, severity default, symbolic name.
6. Enrich: device identity (sysName, vendor); topology position if co-located; recent polling state if available. Cross-language data access via in-process state (Go) or `netipc` (Rust).
7. Dedup check (§10): if the same `(source, trap-OID, key-varbind-hash)` has fired within the dedup window, increment in-memory dedup counter and skip metric/journal emission; otherwise proceed.
8. Atomic increment of in-memory counters for `snmp.trap.events` (per device, per category, per severity).
9. Build journal entry; one `sd_journal_send` syscall.
10. Return.

### Cold path (per Netdata collection tick, default 1Hz)

Walk counter map → emit PLUGINSD `BEGIN`/`SET`/`END` lines on stdout → flush. Standard Netdata pattern.

This decoupling means the hot path is not blocked by stdout back-pressure; if the pipe stalls, traps still ingest, journal still writes, counters still increment, metrics catch up on next tick.

### v3 USM engine-ID discovery (opt-in)

Dynamic discovery pattern from Splunk SC4SNMP (`splunk-sc4snmp.md` §3.5; `traps.py:229-258`). Subclass the SNMP transport to peek at the raw bytes pre-parse, ASN.1-decode the SNMPv3 header to extract `engineID`+`username`, hot-register the pair, retry parse.

**This feature is opt-in (disabled by default)** because hot-registering arbitrary `(engineID, username)` pairs at runtime has two security/correctness concerns:

1. **Spoofing surface**: a malicious source can present arbitrary `engineID` values to the listener; without operator-curated whitelist, the listener will hot-register and trust the pair on subsequent traps.
2. **`snmpEngineBoots` persistence**: SNMPv3 requires `snmpEngineBoots` to be monotonically increasing across restarts for replay-protection and INFORM acknowledgement integrity. Hot-registered engines without persistent boot-counter state break INFORM handshakes and weaken replay protection.

Operators who genuinely need dynamic discovery enable it explicitly in plugin config; operators with a known set of devices enumerate engineIDs in plugin config (the safer default). The configuration knob is in §7.5.

## 6. Reception Surface

- UDP/162 binding via `setcap CAP_NET_BIND_SERVICE` on the binary (Rust dedicated plugin) OR bind to a non-privileged port (Go implementation, since Go plugins typically aren't given capabilities).
- Multiple listeners supported (multi-tenant on same hub; different community/USM contexts).
- v1, v2c, v3-USM with the full HMAC-SHA-2 family and AES-256 priv (per `logstash.md` §3.6 source verification of SNMP4j). Rust crate / Go pkg parity required.
- Multiple v3 USM users per listener (gosnmp/equivalent's `TrapSecurityParametersTable` semantics).
- Per-source community/IP allowlist as the first filter — drops unwanted traffic before decode.
- INFORM acknowledgement support (Cisco/Juniper devices often configure INFORM by default).
- IPv4 and IPv6.
- DTLS / TLS-TM: in scope for first release **if** mature libraries exist in the chosen language. Phase A finding: zero cohort systems support this (universal gap).

## 7. Profile YAML — vendor knowledge only

Profile YAML defines vendor-curated knowledge ONLY. It does NOT define journal field names (the journal always captures all varbinds — see §11), and it does NOT define metric emission (that lives in plugin configuration — see §7.5).

A profile entry per OID:

```yaml
- oid: 1.3.6.1.4.1.9.9.315.0.1                  # ciscoPsmTrapSrvUnauthorized
  name: CISCO-PORT-SECURITY-MIB::ciscoPsmTrapSrvUnauthorized   # MIB-qualified, globally unique
  category: security
  severity: warning

  # Description — template, free use of varbinds (cardinality unrestricted, see §4).
  # Becomes the journal MESSAGE field.
  description: "Port-security violation: MAC {cpsIfViolationMacAddress} on {ifDescr} (VLAN {cpsIfViolationVlan}, ifIndex={ifIndex}) on {SNMP_DEVICE_HOSTNAME}"

  # Labels — bounded-cardinality varbinds only (§4). Templates allowed.
  # Each label emits as journal field TRAP_<KEY_UPPERCASE> (e.g., TRAP_INTERFACE).
  labels:
    interface: "{ifDescr}"                       # OK: bounded per device → TRAP_INTERFACE
    vlan: "{cpsIfViolationVlan}"                 # OK: bounded set per device → TRAP_VLAN
    # mac: "{cpsIfViolationMacAddress}"          # REJECTED at config-load (unbounded)

  # Inline varbind definitions — pre-extracted MIB knowledge.
  # If present, the plugin uses these directly and does NOT need a raw MIB file loaded
  # to decode this trap. Optional: omit and rely on the MIB index if loaded.
  varbinds:
    - oid: 1.3.6.1.4.1.9.9.315.1.2.1.1.1
      name: cpsIfViolationMacAddress
      type: OctetString
      # display_hint: "1x:" — future field (not currently emitted; see profile-format.md)
    - oid: 1.3.6.1.4.1.9.9.315.1.2.1.1.2
      name: cpsIfViolationVlan
      type: INTEGER
    - oid: 1.3.6.1.2.1.2.2.1.1
      name: ifIndex
      type: INTEGER
    - oid: 1.3.6.1.2.1.31.1.1.1.1
      name: ifDescr
      type: OctetString

  # Dedup fingerprint key varbinds (default = all non-timestamp varbinds)
  dedup_key_varbinds: [cpsIfViolationMacAddress, cpsIfViolationVlan]
```

**Required**: `oid`, `category`, `severity`. Everything else is optional. `name` overrides the MIB symbolic name (use when MIB isn't loaded for this OID). `description` is a template (see template syntax below) that becomes the journal MESSAGE — use freely, no cardinality restriction. `labels` are key-value pairs emitted with every trap of this OID; values can be templated, but template references must be bounded-cardinality. `varbinds:` carries pre-extracted MIB knowledge inline — when present, the plugin can decode this trap fully without a raw MIB file.

### Varbind resolution order

The plugin resolves each varbind in the order: **profile inline `varbinds:` → loaded MIB index → raw fallback**.

1. **Profile inline `varbinds:`** — if the profile defines the varbind, use its name, type, enum, display_hint directly. **No MIB file needed.** This is the Datadog `dd_traps_db.json.gz` pattern adapted: pre-extracted MIB knowledge ships with the profile, making the profile self-contained.
2. **Loaded MIB index** — if profile doesn't define this varbind but the MIB is in `/etc/netdata/snmp-mibs/` or stock, fall back to MIB-derived name/type.
3. **Raw fallback** — neither available; render as OID-keyed entry with the ASN.1-decoded type only. The varbind still lands in `SNMP_TRAP_JSON` (§11) with its OID and value; just without a symbolic name.

Stock profiles ship with inline `varbinds:` populated by the conversion tools (§8) — operators get full decoding for top vendors **without ever installing a MIB file**.

### Description template syntax

`{varname}` substitutions. Recognized references:

| Reference | Resolved to |
|---|---|
| `{ifDescr}`, `{ifIndex}`, `{cpsIfViolationMacAddress}`, … | Varbind by MIB symbolic name |
| `{1.3.6.1.2.1.31.1.1.1.1}` | Varbind by numeric OID (fallback when no MIB name) |
| `{SNMP_DEVICE_HOSTNAME}`, `{SNMP_SOURCE_IP}`, `{SNMP_TRAP_NAME}`, `{SNMP_DEVICE_VENDOR}` | Standard journal fields |
| `{ND_TOPOLOGY_INTERFACE}`, `{ND_TOPOLOGY_NEIGHBORS}` | Topology fields when co-located |
| `{ifOperStatus}` | MIB enum value, symbolic (e.g., `down`) by default |
| `{ifOperStatus.raw}` | MIB enum value, raw numeric (e.g., `2`) |

If a reference cannot be resolved (varbind absent, MIB not loaded, etc.):
- Missing varbind → `<missing>`
- Unrecognized variable name → `<unresolved:varname>` plus `snmp.trap.errors.template_unresolved` increment so the operator notices

If `description` is absent, default template is:
```
{SNMP_TRAP_NAME} from {SNMP_DEVICE_HOSTNAME} ({SNMP_SOURCE_IP})
```

Templates are compiled at profile-load (tokenize → segment list). Hot-path substitution is bounded-size buffer fill. MESSAGE capped at 512 chars; truncated with marker if exceeded.

### No `journal_fields:` list, no `metric:` block

The journal captures every varbind always (§11). The plugin emits its own self-metrics always (§12). Operator-defined per-OID metrics are configured separately (§7.5).

Stock vendor profiles ship in `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/` (matching the existing SNMP polling profile convention at `/usr/lib/netdata/conf.d/go.d/snmp.profiles/default/`). Operator-provided overrides live in `/etc/netdata/go.d/snmp.trap-profiles/` and take precedence. The plugin loads these profiles only when the trap subsystem is enabled — agents that do not receive traps never pay the memory footprint. The profile schema is documented in `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`; the on-disk YAML uses a file-scoped `varbinds:` table referenced by name from each trap entry, which keeps both disk and loaded-memory footprint linear in the number of distinct varbinds per vendor rather than in the number of traps that use them.

Operator-provided **MIB files** (raw `.mib` / `.txt` SMIv1/v2 files for vendors not covered by stock profiles) live in `/etc/netdata/snmp-mibs/`. The plugin watches the directory via inotify; compiles new MIBs on file change; updates the in-memory MIB index without restart. Newly recognized OIDs are journal-tagged with `category: unknown` until the operator chooses to set the category in plugin config.

## 7.5 Plugin Configuration — per-OID metric opt-in and overrides

The plugin's own configuration (in `/etc/netdata/...`, dyncfg-editable) controls:

1. Listener configuration (ports, community strings, v3 USM users — community/keys via existing Netdata Secrets).
2. Dedup window (default e.g., 5 seconds; see §10).
3. **Per-OID metric opt-in** — explicit list of OIDs the operator wants emitted as their own metric chart for finer-grained alerting:

```yaml
metrics:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1     # Cisco config change
    context: snmp.trap.cisco_config_changes
    dimension_from_varbind: ccmHistoryEventTerminalType   # MUST be bounded-cardinality
  - oid: 1.3.6.1.4.1.9.9.46.2.0.1     # Cisco port-security violation
    context: snmp.trap.cisco_port_security
    # no dimension_from_varbind → single-dimension counter
```

4. **Per-OID severity/category overrides** (rare; profile defaults are normally enough).

5. **Per-OID label additions/overrides** — operators add their own taxonomy on top of profile baseline labels:

```yaml
labels:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1     # Cisco config change
    add:
      compliance: pci
      tenant: acme
      change_window: business_hours
  - oid_prefix: 1.3.6.1.4.1.9.9.46    # whole Cisco port-security subtree
    add:
      compliance: pci
      team: secops
```

Labels are free-form key-value pairs. Keys must match `[a-z][a-z0-9_]*` (lowercase plugin convention; uppercased when written to journal as `TRAP_<KEY>=<VALUE>`). Values are arbitrary strings. Labels are slicing metadata on the journal and on the metric-instance, not metric dimensions — they don't expand the chart dimension count.

**Reserved-name check**: operator label keys are rejected at config-load if uppercasing them would produce a journal field name colliding with anything plugin-controlled. Specifically rejected: any key that would produce a field starting with `SNMP_`, `ND_`, or `_` (systemd-reserved trusted-field prefix), or that would equal a standard systemd field name (`MESSAGE`, `PRIORITY`, `SYSLOG_IDENTIFIER`, etc.). The error message names the offending key + the colliding standard field.

Operators do NOT copy entire profiles to enable metrics or add labels. The profile remains the vendor's curated knowledge; per-installation choices are surgical edits in plugin config.

If `dimension_from_varbind` references a varbind that can take unbounded values (MAC, IP, username), the plugin REJECTS the config at load with a clear error. Cardinality discipline is structurally enforced.

## 8. OOB Catalog Strategy

Phase A surfaced (`comparison/profile-inventory.md`):

- Datadog Agent: claims "11,000+ MIBs" in public marketing; verified count via a copy of `dd_traps_db.json.gz` is **3,652 MIBs** (67,680 trap definitions, 40,617 varbind definitions). The compiled artifact is closed (Omnibus build) but the **compiler is Apache-2.0** (`datadog/integrations-core :: datadog_checks_dev/datadog_checks/dev/tooling/commands/meta/snmp/generate_traps_db.py`) and the **input MIBs are public** (pysnmp mirror + integrations-core's own MIB tree).
- LibreNMS: 4,770 MIB files / 2,245 with notifications (GPL-3.0-or-later per the project's `composer.json`).
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

1. **Build offline conversion tools** that ingest each major cohort system's per-OID configuration format and emit our profile YAML:
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

We are **transforming knowledge at development time**, not redistributing other systems' files. The output (our profile YAML) is original work informed by reading public documentation, MIB definitions, and open-source classifications. License obligations on others' source files (GPL-2.0 for LibreNMS, AGPL-3.0 for OpenNMS) do not propagate to our derived YAML. We attribute sources in commit messages and `CREDITS.md`, not by copying files.

### Coverage target

Aim for the LibreNMS-to-Datadog band: ~2,000-12,000 OID families across major vendors (Cisco, Juniper, Arista, Aruba, HPE, Dell, Fortinet, Palo Alto, F5, MikroTik, Ubiquiti, …). Ship YAML in the repo so operators can `grep` for their vendor before install (the Datadog `dd_traps_db.json.gz` opacity is a real customer pain — `datadog-agent.md` §17).

## 9. Hot Path vs Cold Path Throughput

### Per-thread targets

- **Single trap listener + decoder thread**: design target 10s of thousands of decode operations/sec, limited by BER decode + counter increment + dedup-cache check (in-memory). **The exact ceiling is to be measured during implementation** — this number is a design rule, not a benchmarked claim.
- **Single journal writer thread**: ~30k entries/sec. This figure comes from existing benchmarks of the Netdata Rust journal-writer crate as used in the **NetFlow plugin** (same backend, same write pattern). Cited here as basis for partitioning thresholds; the trap plugin will validate the figure during implementation.

### Scaling beyond one thread

To exceed 30k entries/sec total, scale **horizontally with isolation**:

- Multiple journal files, each with its own writer thread. Listeners route entries to writers by source-IP hash (or by configured shard count).
- This preserves "single writer per file" while letting two writer threads achieve ~60k entries/sec.

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

## 10. Deduplication — first-wins, no delay, periodic summaries

Netdata already ships built-in alerts for UDP receive-buffer overflow on all listeners. That covers the kernel-level overflow case. We do NOT duplicate that as a plugin feature.

What we DO add: **plugin-level deduplication** of repeated traps within a short window — **first-wins, zero latency.**

### Mechanism — hot path

1. Compute fingerprint per trap: `hash(source_device, trap_OID, key_varbinds)`. Key varbinds are profile-specified (default: all non-timestamp varbinds).
2. Check in-memory dedup cache (LRU-bounded, e.g., 100k entries):
   - **Fingerprint NOT present** → write journal entry immediately, increment metric counters, insert fingerprint into cache with TTL = dedup window. **Real-time, no buffering, no delay.**
   - **Fingerprint present** → suppress: no journal write, no per-event metric increment. Increment an in-memory per-period suppression counter (broken down by trap-OID).
3. Cache entries expire after the dedup window (default e.g., 5 seconds, configurable globally and per-OID).

### Periodic summary entry — for operator transparency

A separate background timer (cadence configurable, default = dedup window length) runs in the cold path. If any suppression happened in the period, it emits **one summary journal entry**:

```
MESSAGE=DEDUPLICATED TRAPS: 247 events have been deduplicated:
- ifDown 120
- authenticationFailure 80
- ciscoPsmTrapSrvUnauthorized 47
PRIORITY=6
SYSLOG_IDENTIFIER=netdata-snmptrap
SNMP_TRAP_REPORT_TYPE=deduplication_summary
SNMP_TRAP_SUPPRESSED_COUNT=247
SNMP_TRAP_SUPPRESSED_FINGERPRINTS=12
SNMP_TRAP_REPORT_PERIOD_SEC=5
ND_HUB=hub-amsterdam
SNMP_TRAP_JSON={"period_sec":5,"total_suppressed":247,"by_trap":{"1.3.6.1.6.3.1.1.5.3":120,"1.3.6.1.6.3.1.1.5.5":80,"1.3.6.1.4.1.9.9.315.0.1":47}}
```

The MESSAGE field is **multi-line by design** — operators reading the journal directly (e.g., `journalctl SNMP_TRAP_REPORT_TYPE=deduplication_summary`) get the full breakdown without parsing JSON. The Logs UI renders the multi-line MESSAGE natively. Multi-line MESSAGE values are written using systemd-journal's binary field encoding (see §11) so newlines inside MESSAGE never inject other fields.

This summary entry lives in the same journal alongside the real trap entries. The `SNMP_TRAP_REPORT_TYPE=deduplication_summary` field distinguishes it from real trap entries (which have `SNMP_TRAP_REPORT_TYPE` absent or set to `trap`). Operators query summaries cleanly:

```
journalctl SNMP_TRAP_REPORT_TYPE=deduplication_summary
journalctl SNMP_TRAP_REPORT_TYPE=trap        # real trap entries only
```

If there was no suppression in the period, no summary entry is emitted.

### Why this matches our rules

- **Rule 6 (real-time alerting)**: hot path commits the first occurrence immediately. No buffer, no window-close delay. Alerts fire on the metric the moment the first trap arrives.
- **Rule 2 (operator simplicity)**: journal stays clean — one entry per real event, plus a periodic summary when duplicates were suppressed.
- Operator transparency: the metric `snmp.trap.errors.deduplicated` increments per suppressed trap (continuous signal); the periodic summary entry provides on-journal narrative.

### Out of scope (first release)

- Per-source rate-limiting at the listener layer — Netdata's existing UDP-overflow alert is the right signal; we don't add a competing mechanism.
- Per-fingerprint suppression summary entries — `SNMP_TRAP_JSON.by_trap` in the periodic summary gives enough breakdown. Per-fingerprint detail can be added later if operators need it.
- Periodic re-notification within a sustained dedup window (operator sees one entry every dedup-window during a multi-hour storm). Configurable knob deferred — default is hard suppression for the full window.

## 11. Journal Storage — universal capture, capital-letter fields, OID + name

### Universal capture

Every trap, always, no exceptions, lands in the journal with all its varbinds. This holds whether or not the OID is in the MIB index. The journal entry is the source of truth.

### Standard journal expectations

Journal field names conform to `^[A-Z][A-Z0-9_]*$` per systemd-journal requirements. Standard fields are always emitted so `journalctl` works without further config:

```
MESSAGE=<rendered description template; free-form, high-cardinality content welcome>
PRIORITY=<numeric 0-7 from the canonical 8-severity table below>
SYSLOG_IDENTIFIER=netdata-snmptrap
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

`MESSAGE` is the rendered description template from the profile (§7) — fully resolved with varbind values, including high-cardinality content (MAC, source IP, username, packet details, etc.). This is the operator's primary view: `journalctl SNMP_TRAP_CATEGORY=security` shows one-line readable rows without further field inspection.

### Field-name conventions (fixed prefix universe)

| Prefix | Used for | Source |
|---|---|---|
| `SNMP_TRAP_*` | Standard trap-content fields, plugin-controlled, present on every entry | Plugin |
| `SNMP_*` | Other SNMP-protocol fields (version, source, device identity) | Plugin |
| `TRAP_*` | Operator-defined attributes from `labels:` in profile or plugin config | Operator (via profile/config templating) |
| `ND_*` | Netdata-platform enrichment (topology, hub, node identity) | Plugin via Netdata state |
| Standard journal fields | systemd-journal conventional fields | Plugin |

Every trap entry — regardless of vendor, version, or MIB coverage — uses the same fixed set of standard fields. The variable per-trap data (varbinds) lives in a single well-known JSON-valued field (`SNMP_TRAP_JSON`), not in dynamically-named per-OID fields. This keeps the journal's field universe stable; `journalctl --fields` does not grow unboundedly with each new vendor MIB.

### Real trap entry (full example)

```
MESSAGE=Port-security violation: MAC aa:bb:cc:dd:ee:ff on GigabitEthernet0/1 (VLAN 10, ifIndex=12) on core-sw-01
PRIORITY=4
SYSLOG_IDENTIFIER=netdata-snmptrap
SNMP_TRAP_REPORT_TYPE=trap
SNMP_TRAP_OID=1.3.6.1.4.1.9.9.315.0.1
SNMP_TRAP_NAME=CISCO-PORT-SECURITY-MIB::ciscoPsmTrapSrvUnauthorized
SNMP_TRAP_CATEGORY=security
SNMP_TRAP_SEVERITY=warning
SNMP_TRAP_PDU_TYPE=trap
SNMP_VERSION=v2c
SNMP_SOURCE_IP=10.0.0.5
SNMP_SOURCE_UDP_PEER=10.0.0.5
SNMP_DEVICE_HOSTNAME=core-sw-01
SNMP_DEVICE_VENDOR=cisco
ND_HUB=hub-amsterdam
ND_NODE=core-sw-01
ND_TOPOLOGY_INTERFACE=GigabitEthernet0/1
ND_TOPOLOGY_NEIGHBORS=dist-sw-01,dist-sw-02
TRAP_INTERFACE=GigabitEthernet0/1
TRAP_VLAN=10
TRAP_COMPLIANCE=pci
TRAP_TENANT=acme
SNMP_TRAP_JSON={"cpsIfViolationMacAddress":{"oid":"1.3.6.1.4.1.9.9.315.1.2.1.1.1","type":"OctetString","value":"aa:bb:cc:dd:ee:ff"},"cpsIfViolationVlan":{"oid":"1.3.6.1.4.1.9.9.315.1.2.1.1.2","type":"INTEGER","value":10},"ifIndex":{"oid":"1.3.6.1.2.1.2.2.1.1","type":"INTEGER","value":12},"ifDescr":{"oid":"1.3.6.1.2.1.31.1.1.1.1","type":"OctetString","value":"GigabitEthernet0/1"}}
```

**Cardinality contract** is visible: the MAC (`aa:bb:cc:dd:ee:ff`) appears in **MESSAGE** (templated, free-form) and in **SNMP_TRAP_JSON** (full structured form). It does **NOT** appear in any `TRAP_*` operator label. The MAC is fully searchable in the journal (Logs UI substring search, `journalctl _MESSAGE_MATCH=aa:bb:cc`, or `jq` against `SNMP_TRAP_JSON`) without polluting metric label cardinality.

### Deduplication summary entry (full example)

Same journal file, different entry type. Distinguished by `SNMP_TRAP_REPORT_TYPE`. See §10.

```
MESSAGE=Suppressed 247 duplicate traps in the last 5s — ifDown×120, authenticationFailure×80, ciscoPsmTrapSrvUnauthorized×47
PRIORITY=6
SYSLOG_IDENTIFIER=netdata-snmptrap
SNMP_TRAP_REPORT_TYPE=deduplication_summary
SNMP_TRAP_SUPPRESSED_COUNT=247
SNMP_TRAP_SUPPRESSED_FINGERPRINTS=12
SNMP_TRAP_REPORT_PERIOD_SEC=5
ND_HUB=hub-amsterdam
SNMP_TRAP_JSON={"period_sec":5,"total_suppressed":247,"by_trap":{"1.3.6.1.6.3.1.1.5.3":120,"1.3.6.1.6.3.1.1.5.5":80,"1.3.6.1.4.1.9.9.315.0.1":47}}
```

Filterable: `journalctl SNMP_TRAP_REPORT_TYPE=trap` (real traps only), `journalctl SNMP_TRAP_REPORT_TYPE=deduplication_summary` (suppression history only). New report types may be added later for other pipeline events (e.g., `decode_error_summary`).

### `SNMP_TRAP_JSON` content

The full structured varbind payload as a single-line JSON object. Keyed by varbind symbolic name when known (profile inline `varbinds:` resolved it, or MIB index has it), by OID otherwise. Each value carries `{oid, type, value}` and optionally `enum`/`display_hint` rendering applied.

This guarantees forensic completeness: even with zero profile coverage and no MIB loaded, the journal contains everything needed to reconstruct what arrived on the wire. The fixed field name avoids the per-OID field-name explosion of older designs.

### Varbind value sanitization — binary field encoding (CWE-117 protection)

systemd-journal supports two field encodings: text-line (`KEY=value\n`) and **binary (size-prefixed)**. Text-line fields cannot contain newlines, NULL bytes, or other control characters — those bytes would be interpreted as field-record delimiters. Binary fields can carry ANY byte sequence including embedded newlines.

The plugin chooses encoding per-field at write time:

| Field value characteristics | Encoding |
|---|---|
| ASCII-printable / valid UTF-8, no newlines, no NULL, no control chars (0x00-0x1F except whitespace 0x09/0x20, no 0x7F) | text-line |
| Contains any newline, NULL, or control char | **binary, size-prefixed** |
| Deliberately multi-line MESSAGE values (e.g., the deduplication summary entry's MESSAGE in §10) | binary, size-prefixed |

This eliminates **CWE-117 (log injection)** structurally. A malicious varbind value of `injected_value\nFAKE_FIELD=spoofed\n` lands as ONE field with the bytes `injected_value\nFAKE_FIELD=spoofed\n` as its value — never as two separate journal fields. The `sd_journal_sendv()` and equivalent Rust crate APIs handle this transparently when the appropriate API is used.

The plugin's journal-write path applies this check uniformly to MESSAGE, TRAP_*, SNMP_*, ND_*, and SNMP_TRAP_JSON values. No field bypasses the check.

### Forensics

All operator-facing questions (Phase A `operator-features.md` E1-E6) become journal queries through the existing Logs UI. No new UI code.

## 12. Plugin-Self Metrics (NIDL contexts) — always emitted

Two NIDL contexts, always emitted, used for plugin-health monitoring and broad-trend alerting.

### Context 1: `snmp.trap.events`

| Aspect | Value |
|---|---|
| Instance | Per device (one instance per source device the hub knows about) |
| Dimensions | Categories: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown` (the closed 8-slug set per §3) |
| Unit | events/s (incremental counter) |
| Labels | `severity` (one of the 8 syslog severities per §11), `device`, `vendor`, `hub`, plus operator-defined labels from profile/plugin config |
| Node / vnode | Per-device vnode — **inherited from the SNMP polling subsystem**. The trap plugin does not create vnodes; it emits metrics against the vnodes that SNMP polling already established for each monitored device. If polling is not configured for a given device, traps for that device emit against the hub vnode with `device` as a label. |
| Title | "SNMP Traps Received" |

This is one chart. Operator slices/dices via NIDL controls:
- Filter to severity=crit → see crit events only across devices.
- Group by device → per-device breakdown.
- Group by category → per-category breakdown across the fleet.
- Filter to one vnode (device) → drill into that device's recent traps.

### Context 2: `snmp.trap.errors`

| Aspect | Value |
|---|---|
| Instance | Per hub (one instance per Netdata hub agent) |
| Dimensions | `unknown_oid`, `decode_errors`, `deduplicated` |
| Unit | events/s (incremental counter) |
| Labels | `hub`, possibly `source_device` where source is identifiable |
| Node / vnode | Hub vnode |
| Title | "SNMP Trap Pipeline Errors" |

Operators alert on these for pipeline health:
- `unknown_oid > 0 sustained` → MIB coverage gap; consider adding MIB to `/etc/netdata/snmp-mibs/`.
- `decode_errors > 0` → malformed PDUs; investigate sending device.
- `deduplicated > X/sec sustained` → trap storm signal; investigate source.

### Operator-opted-in per-OID metrics

In addition to the two plugin-self contexts, operator-selected OIDs (via §7.5) produce their own context. Naming convention: `snmp.trap.<vendor>_<short_name>` (e.g., `snmp.trap.cisco_config_changes`). These are surgical, opt-in, and shaped per the operator's alerting needs.

## 13. Open Questions (deliberately surfaced — for Phase B)

The design has gaps. Phase B should attack each.

1. **Rust vs Go**: defer to prototype benchmark; the cross-language boundary exists either way, just on different paths. No early commit.
2. **MIB-to-category default for unprofiled OIDs**: default = `unknown`. Heuristic upgrade rules (e.g., OID under a vendor's security subtree → `security`)? Or strict — only profile-curated entries get non-`unknown` categories? Recommend: strict.
3. **Label cardinality bounds**: labels don't expand chart dimensions but do consume label-index storage. Should the plugin cap operator-defined label cardinality (e.g., warn at >100 distinct `tenant=` values across the fleet)? Or trust operators?
4. **Profile `metric_filter` field for polled-equivalent linking** (future): the §3 note describes the future enhancement that explicitly links a trap to its polled-metric equivalent so the Logs UI can render the metric time-series. Exact schema TBD; out of scope for first release.
5. **Dedup window default**: 5 seconds is a guess. Phase B should propose evidence-based defaults using cohort dedup-window values where they exist.
6. **Topology integration when topology is NOT co-located**: omit `ND_TOPOLOGY_*` fields entirely or emit `unknown`? Recommend: omit when unavailable.
7. **v3 USM Secrets binding UX**: profile-to-Secrets binding (which Secret holds which engineID's key) — exact syntax TBD. Likely a follow-up SOW. Note: v3 USM dynamic engine-ID hot-registration is now opt-in (§5) with the security trade-off documented; default operation expects operators to enumerate engineIDs explicitly in plugin config.
8. **Northbound trap re-emit**: SaaS-cohort lacks this. Defer to a separate SOW (recommended) or in-scope here? The design enables it cleanly via the journal as source.
9. **Hot-reload semantics** when a profile changes: atomic swap of MIB index, brief drop window, or in-flight-tolerant copy-on-write? Recommend: copy-on-write swap.
10. **MIB upload via API**: file-drop in `/etc/netdata/snmp-mibs/` is the baseline. UI-driven upload via `dyncfg` is nice-to-have — defer.
11. **Multi-tenant listener + Cloud RBAC**: out of scope per §0, but Phase B should confirm by checking the operator workflows in `comparison/operator-features.md`.
12. **Cohort-feature deferrals** — each cohort feature we marked "existing Netdata handles it" is a Phase B audit item: confirm the existing Netdata feature genuinely covers the cohort-observed need, or surface the gap.
13. **Partitioning thresholds**: at what trap rate do we recommend operators enable multi-writer partitioning? Auto-shard by source-IP hash from day 1, or operator-explicit configuration?
14. **`SNMP_TRAP_JSON` shape stability**: object keyed by symbolic name (current proposal) vs object keyed by OID (more stable but less readable) vs array of varbinds (canonical order preserved). Recommend object keyed by name with OID inside; works with `jq` filters operators write. Phase B may surface counter-evidence.
15. **Vendor coverage curation priority**: with ~28k raw MIB files mirrored / ~8-12k unique modules available (§8), which vendors to curate first for OOB profile shipping? Likely top 8-10 by enterprise-network deployment share: Cisco, Juniper, Arista, Aruba, Palo Alto, Fortinet, F5, MikroTik, Ubiquiti — but Phase B should consult Phase A's `operator-features.md` for cohort evidence of vendor importance.

## 14. Non-Goals (deliberately not building)

- Built-in alert engine for traps (existing Netdata alert engine on emitted metrics).
- Built-in notification routing (existing channels).
- Built-in dedicated trap UI (existing Logs UI on the journal).
- Built-in alarm-lifecycle state machine (open/ack/clear) — alert-engine territory.
- Central correlation across hubs (hub-local by design choice).
- Per-source listener-layer rate-limiting (Netdata UDP-overflow alert already covers kernel-level; plugin dedup covers application-level).
- Automatic device profiling (Rule 4 — operator drops MIB if needed; everything else decodes automatically).
- Profile YAML controlling journal field names (the journal always captures all varbinds; no profile knob needed).
- Profile YAML controlling metric emission (metric emission is operator choice in plugin config).

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
| Configuration | `dyncfg` for runtime config and MIB hot-reload |
| Function surface | Existing Functions framework (`logs:`, `topology:`, `dyncfg:` patterns) |
| UDP-overflow alerting | Built-in alerts on all UDP listeners |
| Per-device vnodes | Existing SNMP polling pattern |

## 16. Cohort-Win Audit (what this design wins relative to the cohort)

| Phase A cohort gap | This design |
|---|---|
| 1. No cohort system does listener-layer per-source rate-limit | We don't either — but we do **first-wins plugin-level dedup with periodic summary entries**, which solves the same operational problem more elegantly. First occurrence commits real-time; suppression activity emits as `SNMP_TRAP_REPORT_TYPE=deduplication_summary` rows in the same journal. |
| 2. No cohort system supports DTLS/TLS-TM | Conditional: in scope for first release if libraries support it; otherwise deferred. |
| 3. No cohort system does topology-aware suppression | **Hub-local enrichment makes this cheap.** Journal carries upstream-device identity; alert engine can suppress downstream alerts when upstream is in alarm. |
| 4. MIB management divergence is extreme | **Ship comprehensive vendor packs derived from public MIBs + open-source cohort knowledge transformed via our conversion tools + file-drop for custom MIBs + hot-reload.** Targets the LibreNMS-to-Datadog coverage band. |
| 5. NSTI shipped-defects cautionary tale | Acknowledged. First-release quality bar enforced via the same per-system review protocol that produced the cohort specs. |

## 17. Implementation Sequencing (suggested order, separate SOW)

Out-of-scope for this design doc; included only because Phase B will likely ask.

1. Reception + decode + journal write (minimal vertical slice — operator can see all traps in Logs UI on day 1, with or without MIB coverage).
2. Profile YAML loader + stock vendor pack for top-5 vendors (Cisco, Juniper, Arista, Aruba, Palo Alto).
3. Plugin-self metrics (`snmp.trap.events`, `snmp.trap.errors`).
4. Custom MIB hot-reload from `/etc/netdata/snmp-mibs/`.
5. Dedup mechanism.
6. Per-OID metric opt-in via plugin config.
7. Conversion tools (OpenNMS, Centreon, LibreNMS, Zenoss, public MIBs).
8. Topology annotation when co-located.
9. Polling-context enrichment when co-located.
10. Multi-writer partitioning for high-throughput hubs.
11. DTLS/TLS-TM if libraries available.
12. Coverage expansion across remaining vendors.

End of design proposal. Awaiting Phase B stress-test.
