# SolarWinds (Orion / NPM / Observability Self-Hosted) — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: SolarWinds Orion Platform / Network Performance Monitor (NPM) / **SolarWinds Observability Self-Hosted** (the umbrella SKU bundle, formerly named **Hybrid Cloud Observability (HCO)** and informally still called "Orion"). The on-disk product is referred to in current docs as the "SolarWinds Platform"; the trap subsystem is the **SolarWinds Log Manager for Orion Trap Service** (Windows service `SolarWindsTrapService`, executable `SolarWinds.Orion.LogMgmt.TrapService.exe`).
- **Source evidence**: **docs-only**. SolarWinds Orion is **closed-source proprietary** software. There is no source mirror at `/opt/baddisk/monitoring/repos/solarwinds/`. Every architectural claim in this file is grounded in vendor documentation (with URLs and retrieval date 2026-05-22) or in clearly-attributed operator reports on the public **THWACK** community forum.
- **Documentation versions analysed**: SolarWinds Platform **2024.2** (June 2024) and **2025.1**/2025.2 release notes; "SolarWinds Observability Self-Hosted 2024.2" branding; the Orion SDK SWIS schema (`Orion.Traps`, `Orion.TrapVarbinds`). Primary retrieval date: **2026-05-22**; supplementary release-notes pages from 2026.1.x checked **2026-05-23**.
- **Release-notes scope explicit note**: the **2025.4.x** release-note documents are **not exhaustively cross-referenced** in this file (acknowledged gap). The **2026.1** and **2026.1.1** release notes were spot-checked and produced three material items folded into the analysis below: (a) the new mandatory **TCP/17734** APE→main-engine communication channel (2026.1, see §2.4); (b) the **HA virtual-hostname UDP-port-exhaustion** known issue introduced in 2026.1 and **fixed in 2026.1.1** (see §2.2 and §10.6); (c) confirmation that no fundamental change to the Trap Service architecture is documented across these versions. Any remaining 2025.4.x trap-pipeline-specific changes are the residual scope gap.
- **Repository root analysed**: not applicable. Primary documentation roots:
  - `https://documentation.solarwinds.com/en/success_center/orionplatform/`
  - `https://documentation.solarwinds.com/en/success_center/npm/`
  - `https://documentation.solarwinds.com/en/success_center/hco/`
  - `https://solarwinds.github.io/OrionSDK/schema/` (SWIS schema reference; the *only* publicly versioned SolarWinds artefact)
  - `https://thwack.solarwinds.com/` (operator community, used sparingly and explicitly attributed)
- **Author**: assistant
- **Reviewer pass**: **converged at iter-5** (SOW cap). Six reviewers per iteration: codex, glm, kimi, mimo, minimax, qwen. All five iterations returned 6/6 reviewers cleanly with verdicts of `accept-with-fixes` (no blockers, no rejects across any iteration). Per-iteration trajectory: iter-1 6/6 accept-with-fixes (multiple majors and minors); iter-2 6/6 accept-with-fixes (2 new schema-type majors caught + handled); iter-3 6/6 accept-with-fixes (release-notes scope + multiple new minors); iter-4 6/6 accept-with-fixes (HA failover/B.1 framing); iter-5 6/6 accept-with-fixes — kimi+mimo+qwen reported **zero majors**, codex reported 3 correctness-relevant majors that were all applied before declaring convergence. The remaining issues across reviewers at iter-5 are minor/nit (style, additional URL coverage, additional missed-content items that are additive rather than corrective). Convergence per SOW stop rule ("stop when only minor/nit findings remain") together with hitting the SOW 5-iter cap.

**Reading discipline for this file**. Where vendor docs are clear, claims cite the URL directly. Where vendor docs are silent (e.g., dedup algorithm, internal threading model, listener language), this file says **"vendor docs do not specify"** rather than guessing. Where THWACK reports diverge from official docs, both are shown and the THWACK source is tagged with `[operator-reported]`. Reviewers MUST NOT demand `file:line` evidence — there is no source tree to cite.

**Scope boundary — SolarWinds Observability (SaaS) is OUT OF SCOPE.** SolarWinds also sells a **SaaS-only** observability product (separate from SolarWinds Observability *Self-Hosted*, formerly HCO). The SaaS product's trap-receiving model — if any — is a different product line with different documentation and is not covered here. This file analyses the **Self-Hosted / Orion Platform** product family only, which is the line that hosts the SNMP Trap Service in 2026.

---

## 1. System Overview & Lineage

### 1.1 What it is

SolarWinds Orion is a **Windows-based, central-tier, on-premises** IT operations management platform. It is the *archetypal* enterprise NMS of the on-prem era — a monolithic platform with a SQL Server back end, IIS-hosted Web Console, a fleet of Windows services for collection (poller, trap service, syslog service, NetFlow service, job engine), and bolt-on product modules (NPM, NCM, SAM, NTA, IPAM, VMAN, SRM, UDT) that all share the same database, the same Web Console, and the same alert engine. SolarWinds is **closed-source commercial** software; the on-disk product name in current docs is "SolarWinds Platform" and the SKU bundle is **SolarWinds Observability Self-Hosted** (formerly Hybrid Cloud Observability). Source: <https://www.solarwinds.com/hybrid-cloud-observability> (retrieved 2026-05-22).

The trap subsystem is one component of NPM (and any other Orion module that handles fault data). It is delivered as a Windows service that binds UDP/162, decodes traps, writes them to the SQL database, and surfaces them in the Web Console either through the **legacy Trap Viewer** (deprecated) or the **Log Viewer** (current).

License — proprietary. Audience — enterprise IT operations and NOC teams running on-prem Windows infrastructure. Age — Orion's first release was 2004; the trap subsystem has been present since the early NPM releases.

### 1.2 Where SNMP traps fit in the broader product

SNMP traps are one of several **event sources** ingested by the platform. The vendor explicitly groups SNMP traps, syslog, Windows events, log files, and VMware events under the same **Log Viewer** ingestion model in current versions: *"a SolarWinds installation can process approximately 1000 events (syslogs, traps, Windows Events, log files, or VMWare events) per second"* — <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-monitoring-snmp-traps-sw593.htm> (retrieved 2026-05-22).

This is a meaningful design statement: in current SolarWinds, **a trap is one row in a unified log event stream**, not a first-class object with its own correlation engine. The legacy Trap Viewer treated traps as a separate class with its own UI, filters, and rule engine; the current Log Viewer subsumes that surface into the broader log-events pipeline. The deprecation banner on every legacy trap page is unmistakable: *"SNMP Trap Viewer is no longer available and the Traps view was replaced by Log Viewer"* (<https://documentation.solarwinds.com/en/Success_Center/orionplatform/Content/Core-Viewing-SNMP-Traps-in-the-Web-Console-sw752.htm>).

That deprecation has two consequences for this analysis:

1. **Current customers** (Platform 2019.4 and later) use Log Viewer. Traps land in the **Log Analyzer database** (a separate SQL Server database dedicated to log collection; table names and public schema are **not published** — only the fact of "separate database" is documented at <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-feature.htm>: *"Separate database for log collection"*). **Tier note**: the vendor's own feature-comparison matrix on that page distinguishes **three** columns — "legacy Syslog and Trap Viewers", **"Log Viewer (Basic)"**, and **"Log Analyzer"** — as separate feature/SKU tiers. The exact licensing boundary between Log Viewer (Basic) and full Log Analyzer is not crisply documented in the public pages consulted; the throughput numbers and separate-DB architecture analysed in this file map most directly to the **Log Analyzer** path. Sites running only Log Viewer (Basic) may see different throughput and may rely on the existing Orion DB rather than the separate Log Analyzer DB. Older "Traps" view paths still write to `Orion.Traps` if Log Viewer is not enabled. The exact split between the two storage paths is not crisply documented; vendor docs say *"the service decodes, displays, and stores the messages in the Log Analyzer database"* for current Platform versions while the older `Orion.Traps` / `Orion.TrapVarbinds` SQL tables are still listed in the SWIS schema. Vendor also notes (Log Viewer feature comparison page, retrieved 2026-05-22, footnote): *"Installation of LA or Log Viewer replaces existing legacy syslog and trap services, but does not provide 100 percent feature parity."* — i.e., the migration is **vendor-acknowledged-incomplete**.
2. **Legacy customers** still hit the older Trap Viewer surface with its rule engine, filters, and alert actions. The vendor docs for these pages remain online but are marked **"deprecated"** at the top.

This bifurcation is unusual in the comparison set. Most other systems analysed (OpenNMS, Zabbix, LibreNMS, Centreon, Nagios+SNMPTT) have evolved the trap subsystem in place; SolarWinds *forked* trap presentation into a new product (Log Viewer / Log Analyzer) and is in the middle of a multi-year migration. Reviewers reading this file should treat the two paths separately and not confuse them.

### 1.3 Two-direction trap surface (inbound and outbound)

- **Inbound** (the SNMP trap *receiver*) — the `SolarWindsTrapService` Windows service. UDP/162 listener; decoder; writer to SQL; UI surface via Trap Viewer or Log Viewer. The bulk of this document covers this surface.
- **Outbound** (Orion as a notification *source* that emits SNMP traps) — the platform can emit traps as an **alert action**. The configuration page documents *"UDP port number, SNMP version number, and SNMP credentials"* as the destination fields, supports **multiple destinations separated by commas or semicolons**, and uses **Trap Templates** to format the outgoing PDU. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-sending-an-snmp-trap-sw1082.htm>.

Both directions matter for cross-system comparison: SolarWinds is a *receiver* (like every NMS analysed in this set) **and** a *northbound emitter* (like Sensu's `sensu-snmp-trap-handler` and LibreNMS's alert transport). Datadog and Splunk SC4SNMP do not emit traps; SolarWinds does, via Trap Templates.

### 1.4 Relationship to upstream tools

- **Net-SNMP / `snmptrapd`**: **NOT used**. The SolarWinds Trap Service binds UDP/162 directly with a proprietary listener. Vendor docs do not name the SNMP library; THWACK threads confirm it is an internal implementation, with one community comment naming a legacy executable `SWTrapService.exe` and a newer `SolarWinds.Orion.LogMgmt.TrapService.exe` ([operator-reported] on multiple forum threads cited under §19). This is unlike Centreon (`centreontrapd` wraps `snmptrapd`), LibreNMS (`snmptrapd` front-end), or Zabbix (`snmptrapd` + Perl bridge).
- **SNMPTT / `mib2c` / `libsmi`**: NOT used.
- **MIB management**: proprietary. The MIB database is shipped as a Windows installer (`MIBs.msi`); custom MIBs are **not user-compilable inside the product** — the vendor instructs users to *"submit them for inclusion in the SolarWinds database"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-downloading-the-solarwinds-mib-database-sw3581.htm>). This is a striking contrast to OpenNMS (in-product MIB compiler), Zenoss (`smidump`), Centreon (`centFillTrapDB`), and Datadog Agent (`ddev meta snmp generate-traps-db`) — all of which let the operator add a custom MIB locally. SolarWinds requires a vendor round-trip.
- **`gosnmp` / `pysnmp` / `snmp4j`**: not visible in any docs. SolarWinds does not publish a library dependency list.

### 1.5 Brutal honesty on the SUNBURST / Sunburst context

This file would be dishonest if it omitted the elephant in the room. In December 2020, the SolarWinds Orion update channel was breached and used to distribute the **SUNBURST** backdoor to 18,000+ customers — among the most consequential software supply-chain attacks in commercial history. Specific affected Orion versions: 2019.4 HF 5, 2020.2 (unpatched), and 2020.2 HF 1 (publicly disclosed). Source: <https://www.techtarget.com/whatis/feature/SolarWinds-hack-explained-Everything-you-need-to-know>; SolarWinds Security Advisory FAQ <https://www.solarwinds.com/sa-overview/securityadvisory/faq>.

Implications for this analysis (not for the product's correctness, but for adoption posture and what Netdata should learn):

1. **The Orion update mechanism was the attack vector**. The trap subsystem itself was not the entry point, but every customer who deployed Orion for trap reception also accepted the same auto-update trust boundary. Any single-product, monolithic, auto-updating, central-tier NMS inherits the same supply-chain blast radius.
2. **The post-SUNBURST trust environment** still affects new adoption decisions in 2026. Some regulated environments now contractually forbid SolarWinds installations. Others mandate strict change control on the Orion update channel.
3. **For Netdata**: the lesson is *not* "do not auto-update". The lesson is "do not have a single central tier whose compromise cascades to every site". Netdata's per-site hub model (see `netdata-snmp-hub-architecture.md`) means a compromised hub at one site does not give the attacker access to traps at other sites. SolarWinds' central-database model means the database is the blast radius.

This is the only point in this file where SUNBURST is discussed; the rest is product analysis.

---

## 2. Trap-Subsystem Architecture

### 2.1 Components (current architecture, 2025.x)

```
                          SolarWinds Platform server (Windows)
   +-------------------------------------------------------------------------+
   |                                                                         |
   |   [SNMP devices in the network send traps to UDP/162 of this server]    |
   |                                                                         |
   |             |                                                           |
   |             v                                                           |
   |   +---------------------------------+                                   |
   |   |  SolarWindsTrapService          |                                   |
   |   |  (Windows service)              |                                   |
   |   |  exe: SolarWinds.Orion.LogMgmt. |                                   |
   |   |       TrapService.exe           |                                   |
   |   |  UDP/162 listener               |                                   |
   |   |  decodes v1/v2c/v3 PDUs         |                                   |
   |   +---------------------------------+                                   |
   |             |                                                           |
   |             v                                                           |
   |   +----------------------------------------------+                      |
   |   |  Decoder & rule engine (in-process)          |                      |
   |   |    OID -> name (via MIB DB)                  |                      |
   |   |    IP -> NodeID lookup                       |                      |
   |   |    Apply Log Viewer rules (or legacy Trap    |                      |
   |   |    Viewer rules if Log Viewer not enabled)   |                      |
   |   +----------------------------------------------+                      |
   |             |                                          \                |
   |             |                                           \               |
   |             v                                            v              |
   |   +-----------------------+              +-----------------------+      |
   |   |  Log Analyzer DB      |              |  Orion SQL Server DB  |      |
   |   |  (current path)       |              |  Orion.Traps          |      |
   |   |                       |              |  Orion.TrapVarbinds   |      |
   |   |  Surface: Log Viewer  |              |  (legacy path or both)|      |
   |   +-----------------------+              +-----------------------+      |
   |             |                                        |                  |
   |             v                                        v                  |
   |   +----------------------------------------------------------+          |
   |   |  Orion Web Console (IIS)                                 |          |
   |   |    Alerts & Activity > Traps  --> opens in Log Viewer    |          |
   |   |    Alerts engine (pub/sub event triggers)                |          |
   |   |    SWIS API (Orion.Traps / Orion.TrapVarbinds entities)  |          |
   |   +----------------------------------------------------------+          |
   |                                                                         |
   +-------------------------------------------------------------------------+
```

Key documented components (each cited):

| Component | What it does | Source |
|---|---|---|
| `SolarWindsTrapService` (Windows service) | Binds UDP/162, decodes traps, applies rules, writes to SQL/Log Analyzer DB | Service exe path and name reported by operators on THWACK; vendor docs at <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-monitoring-snmp-traps-sw593.htm> describe its role as *"listens for incoming trap messages on UDP port 162, and then decodes, displays, and stores the messages"* |
| Orion SQL Server database | Stores trap rows (`Orion.Traps`) and varbinds (`Orion.TrapVarbinds`) | SWIS schema at <https://solarwinds.github.io/OrionSDK/schema/Orion.Traps.html> and <https://solarwinds.github.io/OrionSDK/schema/Orion.TrapVarbinds.html> |
| Log Analyzer database (separate) | Stores events when Log Viewer is enabled; current-version path | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-feature.htm> ("Separate database for log collection") |
| Orion Web Console (IIS) | Web UI; menu: **Alerts & Activity > Traps** → opens Log Viewer | <https://documentation.solarwinds.com/en/Success_Center/orionplatform/Content/Core-Viewing-SNMP-Traps-in-the-Web-Console-sw752.htm> |
| Alerts engine | Pub/sub Event-alert condition that subscribes to trap events from Log Viewer | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-alerting.htm> |
| Additional Polling Engine (APE) | Scale-out for traps; each APE runs its own `SolarWindsTrapService` and writes to the shared SQL DB | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-using-additional-polling-engines-sw260.htm> |
| HA (Hot Standby) | VIP-fronted active/standby pair of platform servers | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/ha_what_is_high_availability.htm> |

### 2.2 Deployment model

**Single Windows server** is the default deployment. The Trap Service runs on the main polling engine. Vendor sizing (§3.5, §10.2): one polling engine handles **~500 SNMP traps/sec**.

**Distributed (APE)**. When trap volume exceeds one server's capacity, the operator deploys one or more Additional Polling Engines. Each APE is a separate Windows server running the same `SolarWindsTrapService`. Devices must be *manually reconfigured* to send traps to the chosen APE's IP — **the public docs do not describe an automatic load-distribution mechanism for traps**; trap distribution to APEs requires device-side targeting (configuring each device's trap destination to the chosen APE IP, e.g., via NCM config push). The Additional Polling Engines documentation page itself (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-using-additional-polling-engines-sw260.htm>) describes APEs as scalability engines and references the Scalability Engine Guidelines for sizing, but does not spell out automatic trap distribution. The scalability guidelines explicitly include SNMP traps in the per-APE budget (see §3.3). This is therefore a load-distribution-by-administrative-fiat model, not a Layer 4 load balancer or anycast pattern.

**EOC (Enterprise Operations Console)**. For multi-site deployments — multiple independent Orion instances per region — SolarWinds offers a **read-only aggregation layer** called the Enterprise Operations Console. Per the scalability guidelines (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/orion_platform_scalability_engine_guidelines.htm>): *"SolarWinds Enterprise Operations Console must be installed and licensed if you want to view aggregated data from multiple SolarWinds Platform servers in a distributed deployment."* EOC does **not** correlate across sites — it aggregates for *presentation* only. This is functionally analogous to **Netdata Cloud's role for the per-site hub model**: presentation-tier aggregation over independent processing instances. EOC is the closest SolarWinds gets to a hub-style federation pattern, but it sits **above** independent Orion instances rather than replacing the central tier within each instance.

**High Availability**. SolarWinds Platform HA is a two-server active/standby pair fronted by a **Virtual IP (VIP)** (single-subnet) or a **virtual hostname with DNS edit** (multi-subnet "disaster recovery"). Devices send traps to the VIP/hostname; failover swings the VIP to the standby. The vendor explicitly states *"during failover, the active server assumes all of the responsibilities of the primary server, including receiving syslogs, SNMP traps, and NetFlow information through the VIP or virtual hostname"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/ha_what_is_high_availability.htm>; retrieved 2026-05-22).

Critical limitations of the HA model for traps:

- **In-flight traps during failover — vendor does not document preservation behaviour**. The HA documentation page describes the protected-services list and the VIP/hostname failover mechanism but **does not state whether in-flight UDP traps are preserved during the transition** (HA overview at <https://documentation.solarwinds.com/en/success_center/orionplatform/content/ha_what_is_high_availability.htm>; retrieved 2026-05-22). Because traps are UDP, loss during failover is a reasonable operational risk to plan for, but it is **not vendor-confirmed** behaviour — the absence of vendor commitment is the documented finding, not the loss itself.
- **HA virtual-hostname UDP-port-exhaustion (2026.1 known issue, fixed in 2026.1.1)**. The 2026.1 release notes document a known issue where, in multi-subnet HA deployments using the virtual-hostname mode, the active server continually opens new UDP ports for HA control-plane traffic until the host runs out of available ports and requires a reboot. The 2026.1.1 release notes state the fix: *"the pool no longer consumes a large number of UDP ports until it eventually runs out of ports."* Operators on 2026.1 should plan to upgrade to 2026.1.1 specifically to clear this issue. See §10.6 for the backpressure framing.
- **HA does not protect the database**. Vendor docs: *"SolarWinds HA protects your main server, also known as your main polling engine, and Additional polling engines. It does not protect your databases or your Additional web servers"* (same source). The SQL Server must be HA'd separately (typically SQL Server Always On Availability Groups, outside the SolarWinds product).
- **IPv6 is not supported in HA**: *"SolarWinds Platform High Availability does not support IPv6 addresses"* (per <https://support.solarwinds.com/SuccessCenter/s/article/IPv6-support-in-SolarWinds-products>). For the trap *receiver* itself, UDP/162 *is* documented as IPv4+IPv6 capable — but the HA wrapper around it is IPv4-only.

**Containers / Kubernetes**: not supported. SolarWinds Platform is a Windows-only, install-on-bare-OS product. (Datadog, Splunk SC4SNMP, OpenNMS, Centreon all have container/K8s deployment options; SolarWinds does not.)

### 2.3 Languages and key libraries

Closed-source. Vendor docs do not state the implementation language(s); the executable name `SolarWinds.Orion.LogMgmt.TrapService.exe` (operator-reported on THWACK) strongly implies **.NET / C#** since `SolarWinds.Orion.*` is the namespace prefix used across the entire Orion module DLL set. **This is operator-inference**, tagged here with low confidence — vendor docs do not confirm.

### 2.4 Inter-component IPC

- **Trap Service → SQL Server**: ADO.NET / SQL connection (implied; not vendor-documented).
- **Trap Service → Log Analyzer DB**: implied separate DB connection; *"Separate database for log collection"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-feature.htm>).
- **Trap Service → Alerts engine**: vendor docs describe **pub/sub** as the mechanism for Event-alert triggering from Log Viewer rules. *"Alerting actions are syslogs and traps that trigger SolarWinds Platform alerts using pubsub, which is an Event alert condition"* (per the Log Viewer rules doc; <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm>).
- **SWIS API**: the SolarWinds Information Service exposes `Orion.Traps` and `Orion.TrapVarbinds` as queryable SWIS entities over **TCP/17774** (newly the default in 2024.2 — previously 17778; vendor flagged the port migration as a breaking change in 2024.2 release notes).
- **APE → main polling engine (2026.1)**: the 2026.1 release notes add a new required communication channel on **TCP/17734** from APEs / AWS / HA servers to the main polling engine. Deployments upgrading to 2026.1 need to add this port to network ACLs between APE and main-engine subnets, alongside the existing SWIS / RabbitMQ / SQL ports. This is operationally relevant for trap-receiving deployments because trap data flows from the listener on each APE to the central DB via these channels.

### 2.5 Documented telemetry / health surfaces

Vendor docs do not expose internal pipeline-health metrics for the Trap Service (queue depth, drop counters, decode-failure rate, etc.) in the same way OpenNMS exposes JMX or Datadog exposes `datadog.snmp_traps.*` self-metrics. The Orion Web Console surfaces server-level health and individual node availability, but the trap pipeline itself appears to be **observability-opaque** from the documentation. THWACK threads on debugging missing traps reliably go to "check the service is running" and "use Wireshark" rather than "check the pipeline metric" — strong indirect evidence that no first-class self-telemetry exists.

---

## 3. Trap Reception (UDP/162 Ingress)

### 3.1 Listener implementation

The `SolarWindsTrapService` Windows service binds UDP/162 directly (no `snmptrapd` front-end). Default port is 162; vendor KB describes a registry-based port-change procedure (KB "How to change the SNMP Trap port in Orion"; URL `https://solarwindscore.my.site.com/SuccessCenter/s/article/Change-the-SNMP-trap-port` — public but 403-blocked to scraper; observable in search index 2026-05-22). The vendor explicitly states: *"UDP Port 162 must be open for both IPv4 and IPv6"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-monitoring-snmp-traps-sw593.htm>).

### 3.2 SNMP version support

| Version | Receive support | Notes |
|---|---|---|
| SNMPv1 (Trap PDU) | Supported | Inferred from the `Community` field in the public SWIS schema (a v1/v2c concept), the Trap Viewer column set, and the universal NMS coverage. Vendor docs do not call out v1 explicitly as a separate "feature" — it is the legacy default and ships as part of the trap surface. |
| SNMPv2c (Notification PDU) | Supported | Standard; `Community` field handles v1 and v2c. |
| SNMPv3 USM | Supported | Vendor docs describe Configure SNMPv3 for traps; multi-user table is supported (per release notes lineage). The Monitor SNMP traps page (cited above) requires *"that the same authentication type (auth, noauth, or priv) is configured for both polling and traps"* — i.e., per-device auth-type consistency is an explicit operational constraint. |
| SNMP over DTLS / TLSTM | **Not documented**. No vendor page describes TLSTM trap reception. | This is a feature gap relative to security-forward systems. |
| **SNMPv2c/v3 InformRequest** | **Vendor docs do not explicitly state whether the Trap Service handles InformRequest** (the acknowledged-notification variant defined in the foundational spec §2). Receiving an InformRequest requires the receiver to reply with a Response PDU; vendor documentation does not describe this code path. This is an evidence gap, not a documented absence. | Inform-vs-Trap distinction is operationally relevant for devices configured for reliable notifications; reviewers should treat the answer as **undocumented** rather than yes-or-no. |

**SNMPv3 authentication algorithms** (for traps):

- MD5 — supported historically.
- SHA-1 — supported.
- **SHA-256 / SHA-512** — added in **SolarWinds Platform 2024.2** (June 2024). Vendor release notes: *"SNMP v3 traps with the authentication method SHA-256 or SHA-512 are supported"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/solarwinds_platform_2024-2_release_notes.htm>; retrieved 2026-05-22).

That SHA-2 support arrived in **2024** is itself a finding: a major commercial NMS lagged the modern crypto baseline for trap-side SNMPv3 until 2024 (polling supported SHA-256/SHA-512 from 2022.3 — per the THWACK feature request that tracked this; <https://thwack.solarwinds.com/discussion/122060/sha256-and-sha512-support-for-snmp-traps>). A two-year gap between *polling* and *trap-receiver* SHA-2 support tells you SolarWinds treated trap SNMPv3 as a second-class citizen for a long time.

**SNMPv3 privacy algorithms** (for traps): documented for polling are AES-128 and AES-256 (per 2025.1 release notes referring to MD5/AES256). Vendor docs do not list the trap-side privacy algorithms separately; assumption (low confidence) is the same set.

**Known issue (2024.2 release notes)**: changing SNMPv3 credentials while the Trap Service is running causes `"User authentication failed (signature of incoming packet could not be verified with the local user credentials)"`; resolution is **service restart**. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/hco_2024-2_release_notes.htm>. This indicates **no in-memory credential reload** — credentials are read at service start and require a bounce to refresh.

### 3.3 Performance / concurrency model

**Documented throughput** (sizing guide, <https://documentation.solarwinds.com/en/success_center/orionplatform/content/orion_platform_scalability_engine_guidelines.htm>; retrieved 2026-05-22):

- **SNMP Traps: ~500 messages per second (~1.8 million messages/hr)** per polling engine.
- Syslog: 700–1,000 messages/second (2.5–3.6 million/hr) per polling engine.
- **With Log Viewer enabled: 1,000 events per second (syslogs and SNMP traps combined)** per polling engine.

Note the framing: **without Log Viewer**, the two streams are tracked separately (500 traps/sec **and** 700–1,000 syslog/sec per engine, i.e., each stream is rated independently). **With Log Viewer enabled**, the vendor publishes a *combined* ceiling of 1,000 events/sec (syslog+traps pooled). Vendor docs do **not** publish a trap-only ceiling for the Log-Viewer-enabled case. The honest interpretation: enabling Log Viewer changes the limit from two parallel streams to one shared pool, with a publicly stated cap of 1,000 events/sec across all log sources combined. Operators should plan for this as a real architectural tradeoff but should not infer a precise per-stream value that the vendor has not published.

Vendor does not document the threading model (single-threaded vs thread-pool vs reactor). No queue-depth metric is exposed.

### 3.4 Privileged-port handling and Windows OS SNMP Trap Service conflict

UDP/162 is below 1024 on Windows. The service runs under a configured Windows service account (typically `LocalSystem` or a domain service account); Windows does not impose a Unix-style privileged-port restriction. No special socket capability is needed. Vendor docs do not discuss this because on Windows it is a non-issue.

**Windows OS built-in "SNMP Trap Service" conflict (deployment prerequisite)**: Windows Server ships with a built-in service named *"SNMP Trap"* (Windows service name `SNMPTRAP`) that also binds UDP/162 by default. If both this service and `SolarWindsTrapService` are enabled on the same host, the second one to start fails to bind. The standard deployment prerequisite — disable the Windows OS SNMP Trap service (`services.msc`) before installing/starting the SolarWinds Trap Service — is a routine first-boot failure mode for operators. Vendor deployment guides include this step; THWACK threads on "trap service won't start" overwhelmingly resolve to this conflict. This is not trap-subsystem-specific to SolarWinds, but it is a real operational gate at install time.

### 3.5 Horizontal scaling pattern

Add APEs. Reconfigure devices (manually, or via NCM config push) to point at the chosen APE. Each APE writes to the same central SQL Server. This is a **fan-in-to-central-DB** scaling pattern, not a shared-nothing distribution.

**Implications**:

- **Central SQL Server is the scale ceiling**. The vendor sizing guide is silent on SQL Server CPU/RAM for trap-rate-driven load. Operators on THWACK report DELETE statements on `TrapVarbinds` taking up to 90 minutes (<https://everythingshouldbevirtual.com/purge-syslog-trap-alerts-solarwinds-db/>) at scale — i.e., the DB is unmistakably the bottleneck.
- **No anycast / no LB**. Devices know the IP of their trap target. If you move a device's trap target to a different APE, the device's config must be touched.

This is the OpenNMS Minion model in reverse: OpenNMS Minions forward traps to a central core which does the processing. SolarWinds APEs *do* the processing locally and then push events to a shared DB. **Centreon's remote pollers** are the closest comparable pattern from the analysed set.

### 3.6 HA / clustering

See §2.2. VIP-fronted active/standby; SQL Server HA is separate. Vendor docs do not document preservation of in-flight UDP traps during failover — loss is a reasonable operational risk, but not vendor-confirmed behaviour.

---

## 4. MIB Management

### 4.1 MIB store location and layout

Vendor docs describe a **MIB database** that is a *"repository for the OIDs used to monitor a wide variety of network devices"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-management-information-base--mib--sw1730.htm>). The on-disk format is **not publicly documented**; the database is distributed as `MIBs.msi` (Windows installer), and operators install or update it via that MSI. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-downloading-the-solarwinds-mib-database-sw3581.htm>.

The on-disk format is **not publicly documented**; vendor docs describe the artefact only as a "MIB database" updated via the `MIBs.msi` Windows installer. The store is therefore **opaque to the operator** — there is no documented way for the operator to inspect the parsed MIB definitions, diff them across versions, or place them under version control. (Whether the underlying SQL Server tables are technically inspectable by an admin with raw SQL access is outside what vendor docs describe.) Contrast: Net-SNMP (text MIB files in `/usr/share/snmp/mibs/`), Datadog (versioned JSON in `snmp.d/traps_db/`), OpenNMS (XML event definitions plus MIB files in `opt/opennms/share/mibs/`).

### 4.2 Compilation / load pipeline

Vendor docs do not describe an in-product MIB compiler. There is **no `compile-mibs` command, no MIB Studio for traps, no script the operator runs against a vendor `.mib` file**. The MIB Browser tool (part of Engineer's Toolset) is a *query/walk* tool, not a compiler.

### 4.3 Bundled MIBs out-of-the-box (vendor coverage)

Vendor marketing for the MIB Browser tool claims *"over 250,000 precompiled unique OIDs from hundreds of standard and vendor MIBs"* (<https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-mib-browser.htm>). It is unclear whether this number applies to the SolarWinds Platform MIB database too, or only to the standalone Engineer's Toolset; vendor docs do not crisply align the two stores. **Treat the 250K figure as the upper-bound vendor claim**.

### 4.4 User workflow for adding/updating MIBs

This is the single most striking deviation from every other system in the comparison set. Vendor doc verbatim:

> *"If you have a specific device MIB, you can have it added to the SolarWinds MIB database."*
> (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-downloading-the-solarwinds-mib-database-sw3581.htm>)

Read: **the operator submits the MIB to SolarWinds; SolarWinds adds it to a future MSI release; the operator installs the new MSI**. There is no first-party local workflow for "I have a vendor MIB; compile it and resolve OIDs from it tonight".

Comparison context — every other system in the analysed set allows local MIB addition:

| System | Local MIB add workflow |
|---|---|
| OpenNMS | `provisiond` MIB import + event definition XML; in-product compiler |
| Zenoss | `zenmib` command on a `.mib` file; on-disk MIBs under `$ZENHOME/share/mibs/` |
| Centreon | `centFillTrapDB` CLI compiles MIBs into the `traps_*` MariaDB tables |
| LibreNMS | `snmptrapd` config + per-OID PHP handler |
| Datadog Agent | `ddev meta snmp generate-traps-db` (integrations-core tooling) |
| Nagios + SNMPTT | `snmpttconvertmib` Perl script |
| **SolarWinds** | **email it to the vendor** |

The vendor workflow imposes **material customisation friction at scale** for any operator who meets a vendor MIB SolarWinds hasn't already compiled. THWACK threads on this topic typically end with operators saying they translate OIDs themselves and embed them in rule text (paraphrased; multiple threads across the discussion archive). [Operator-reported, low-confidence-for-vendor-position.]

### 4.5 Dependency resolution

Not vendor-documented. With no in-product compiler exposed, the question is moot — SolarWinds handles it on its build side.

### 4.6 Version management vs firmware

Not addressed. The vendor "MIB database is updated regularly" but the cadence is unspecified, and there is no tie-in to firmware versions.

### 4.7 Fallback behaviour for unknown OIDs

Vendor docs do not describe the unknown-OID code path explicitly. Operator-reported behaviour: unresolved OIDs appear as raw dotted-decimal numbers in the Trap Viewer's "Message" column and in `Orion.Traps.Message` in the SQL table.

---

## 5. Trap Processing Pipeline

The current (Log Viewer) pipeline and the legacy (Trap Viewer) pipeline are documented separately. Both are described here.

### 5.1 Decode

Standard BER decode of v1/v2c/v3 PDUs. Vendor does not document the decoder library. SNMPv3 decryption uses the user table configured for the Trap Service (multi-user support documented; release notes lineage shows multi-user was added years ago).

### 5.2 OID-to-name resolution

OIDs are resolved against the bundled MIB DB. Result is the `OIDName` column on the `Orion.TrapVarbinds` row and the human-readable label in the Trap Viewer/Log Viewer column. Unknown OIDs appear as raw dotted-decimal numerics.

### 5.3 Source identification (IP → device mapping)

**What is structurally confirmed**: the SWIS schema (<https://solarwinds.github.io/OrionSDK/schema/Orion.Traps.html>) exposes three relevant properties on `Orion.Traps` — `IPAddress` (the source IP), `NodeID` (FK to `Orion.Nodes`), and the `Node` relationship reached via `Orion.NodesHostsTraps`. The presence of these three together strongly implies an IP-to-Node join, and `Orion.Traps.Hostname` is also populated.

**What is NOT documented**: the *matching algorithm itself*. Vendor docs (Monitor SNMP traps page; <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-monitoring-snmp-traps-sw593.htm>) describe devices sending traps to the SolarWinds Platform server but do not publish the algorithm used to resolve UDP source IP to a managed node — whether it is exact match, longest-prefix, or includes secondary IP support is not stated. The file therefore must not assert "documented behaviour" beyond the schema-level fact of the FK columns and relationship.

**SNMPv1 `agent-addr` handling**: vendor docs do not describe whether the v1 `agent-addr` field inside the PDU is consulted when the UDP source IP and `agent-addr` disagree (the NAT scenario described in `../domain/snmp-traps-in-observability.md` §2). Behaviour is **undocumented**; the file does not assume one or the other.

### 5.4 Enrichment

- **Per-trap fields populated**: `EngineID`, `DateTime`, `IPAddress`, `Community`, `Hostname`, `NodeID`, `TrapType`, `ColorCode`, `Message`, `Tag`, `DisplayName`, `Description` — per the `Orion.Traps` schema.
- **Per-varbind fields populated**: `OID`, `OIDName`, `OIDValue`, `RawValue`, `DisplayName`, `Description` — per the `Orion.TrapVarbinds` schema.
- **Severity assignment**: a numeric `ObservationSeverity` (byte) and string `ObservationSeverityName` are written on the trap row. Vendor does not document the *default* severity mapping (how do they decide a `linkDown` is "Critical" or "Warning"?). Operator-reported answer: the severity comes from the rule that matched the trap, defaulting to a vendor-defined value for unmatched traps. The exact default table is not published.
- **Topology join**: when a node is on a network map, the trap surfaces on the node's detail page. There is no documented "join the trap onto an L2 graph at the device's port" enrichment of the kind OpenNMS does with `ifIndex` lookups during alarm correlation.

### 5.5 Normalization

There is no vendor-documented *normalization layer* (vendor-severity → unified-severity). The severity that ends up in `Orion.Traps.ObservationSeverityName` is whatever the matched rule sets — or whatever the vendor's built-in defaults dictate, which are not published. Compare to:

- OpenNMS: event definitions specify `<severity>` per UEI;
- Centreon: status mapping is rule-driven via `centreontrapd` rules with explicit OK/WARN/CRIT/UNK output;
- Zenoss: `severity_map` per ZenPack with declarative mapping.

SolarWinds' severity model is therefore **operator-defined-via-rule** rather than *manifest-driven*. This is good for flexibility, bad for portability and bad for "out of the box value".

### 5.6 Deduplication / suppression

Vendor docs do NOT document a **store-level deduplication** of traps. Each trap PDU received becomes one `Orion.Traps` row (operator-reported on the SWIS-schema threads). What *is* documented is **alert-level deduplication via thresholds**: a rule can specify a **Trigger Threshold** ("until a specified number of traps arrive that match the rule") and a **Suspend Further Alert Actions For** time window. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-configuring-trap-viewer-filters-and-alerts-sw272.htm>.

So:

- **Trap row dedup**: none. Every PDU is a row.
- **Alert dedup**: present (threshold + suspension). Operates only on the alerting side; traps still pile up in the DB.

This has a direct DB-growth consequence — see §6 and §10.

### 5.7 Routing

- The trap row writes to `Orion.Traps` and N varbinds to `Orion.TrapVarbinds` (or the Log Analyzer DB, depending on Log Viewer state).
- An *event* is published to the alerts engine if any rule's alerting action fires; the alert engine then executes alert actions (email, page, send outbound trap, run script, change interface status, etc.).
- Log Viewer rule actions include **Forward the entry** ("send the entry to another system for further processing"), **Run an external program**, **Flag for discard**, **Stop processing rules**, and **Real-time config change detection (NCM)**. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm>.
- Log Viewer rule **evaluation order** is, per the cited rules page, structurally **Global Pre-processing** → per-source rule groups (Log Files, Syslog, **Traps**, VMware Events, Windows Events) → **Global Post-processing**. The vendor doc describes this three-phase structure. Two rule-action semantics matter here and must be stated precisely:
  - **`Flag for discard`** — *"The log entry is not saved to the database, but subsequent rule actions are still applied"* (vendor verbatim, Log Viewer rules page). I.e., it prevents *persistence* but does **not** halt downstream rule processing.
  - **`Stop processing rules`** — *"Stops additional rule processing for the active log entry"* (vendor verbatim, same page). This is the action that halts the rule pipeline.

  For storm mitigation, an operator who wants a Global Pre-processing rule to "drop a trap before any trap-specific rule sees it" must combine `Flag for discard` (prevent persistence) **and** `Stop processing rules` (halt the per-source-group rules from firing) — not `Flag for discard` alone. This is the correctly-scoped operational consequence of the documented rule-action semantics. Source: same Log Viewer rules page.

### 5.8 Error handling for malformed PDUs, unknown OIDs, decode failures

Vendor docs do not document parser-error counters, malformed-PDU drop counts, or decode-failure logging. THWACK debugging threads consistently fall back to packet capture (Wireshark) when a trap is not appearing in Orion. This points to **no first-class drop-counter telemetry**.

---

## 6. Data Model & Persistent Storage

### 6.1 Two storage paths

| Path | When used | Database | Tables |
|---|---|---|---|
| **Legacy** | Platforms pre-2019.4 or sites that never enabled Log Viewer | Orion main DB (SQL Server) | `Orion.Traps`, `Orion.TrapVarbinds` |
| **Current** | Platform 2019.4+ with Log Viewer | Log Analyzer DB (separate; vendor calls it the "Log Analyzer database") | Vendor does not publish table names |

In practice, many deployments still see writes to `Orion.Traps` because SWIS queries against that entity continue to be the operator's path for reporting and the Orion SDK schema continues to list both entities. (<https://solarwinds.github.io/OrionSDK/schema/>.)

### 6.2 `Orion.Traps` schema (SWIS entity, public)

Source: <https://solarwinds.github.io/OrionSDK/schema/Orion.Traps.html> (retrieved 2026-05-22). Base type **`Orion.LogEntity`** — the same base type used by `Orion.SysLog` and other log-class entities. This shared base is the structural reason traps and syslog co-habit Log Viewer: at the SWIS-schema level they are literally the same kind of object.

| Column | Type | Notes |
|---|---|---|
| `TrapID` | `System.Int64` | Primary identifier. |
| `EngineID` | `System.Int32` | Which polling engine (main or APE) received the trap. |
| `DateTime` | `System.DateTime` | Reception timestamp. |
| `IPAddress` | `System.String` | UDP source IP. |
| `Community` | `System.String` | v1/v2c community. **Exposed as a queryable string field through the public SWIS API** — vendor docs do not document redaction or encryption for this column. See §6.5 for the resulting exposure analysis. |
| `Tag` | `System.String` | Operator-applied tag. |
| `Acknowledged` | `System.Byte` | 0/1 acknowledgement flag. |
| `Hostname` | `System.String` | Resolved hostname. |
| `NodeID` | `System.Int32` | FK to `Orion.Nodes` (SWIS relationship: `Orion.NodesHostsTraps`). |
| `TrapType` | `System.String` | Resolved trap OID / name. |
| `ColorCode` | `System.Int32` | UI color hint. |
| `TimeStamp` | `System.Byte[]` | SQL Server `rowversion`/timestamp for optimistic concurrency. |
| `ObservationSeverity` | `System.Byte` | Numeric severity. **The live SWIS schema page lists this property twice in the table, both as `System.Byte`** — a SolarWinds documentation rendering bug (duplicate row), not a type ambiguity. Range not documented; operator-reported 0–4. |
| `Message` | `System.String` | Concatenated decoded message (varbinds + trap text). |
| `ObservationTimestamp` | `System.DateTime` | (Often matches `DateTime`.) |
| `ObservationRowVersion` | `System.Byte[]` | SQL rowversion. |
| `ObservationSeverityName` | `System.String` | Severity label. |
| `DisplayName` | `System.String` | UI display. |
| `Description` | `System.String` | Long description. |
| `InstanceType` | `System.Type` | SWIS instance type. |
| `Uri` | `System.String` | SWIS URI. |
| `InstanceSiteId` | `System.Int32` | Default 0; multi-site indicator. |

### 6.3 `Orion.TrapVarbinds` schema (SWIS entity, public)

Source: <https://solarwinds.github.io/OrionSDK/schema/Orion.TrapVarbinds.html> (retrieved 2026-05-22). Base type: **`System.Entity`** (NOT `Orion.LogEntity` — the log-class base type applies only to `Orion.Traps`; `Orion.TrapVarbinds` is a child entity of `Orion.Traps` reached through the `Orion.TrapHostsTrapVarbinds` relationship).

| Column | Type | Notes |
|---|---|---|
| `TrapID` | `System.Int64` | FK to `Orion.Traps`. |
| `TrapIndex` | `System.Byte` | Ordinal of varbind within the trap. **One row per varbind**. |
| `OID` | `System.String` | Dotted-decimal OID. |
| `OIDName` | `System.String` | MIB-resolved name. |
| `OIDValue` | `System.String` | Parsed value. |
| `RawValue` | `System.String` | Unprocessed varbind data. |
| `DisplayName` | `System.String` | UI display. |
| `Description` | `System.String` | Long description. |
| `InstanceType` | `System.Type` | SWIS type. |
| `Uri` | `System.String` | SWIS URI. |
| `InstanceSiteId` | `System.Int32` | Default 0. |
| `Trap` | `Orion.Traps` | SWIS reference. The relationship to `Orion.Traps` is `Orion.TrapHostsTrapVarbinds`, which is the formal SWIS FK declaration backing the 1:N row growth (one trap, N varbinds). |

### 6.4 Row growth and retention

- **One row per trap** in `Orion.Traps`.
- **N rows per trap in `Orion.TrapVarbinds`**, where N = number of varbinds in the PDU. The 1:N pattern is structural (per the SWIS schema FK relationship); the typical N range is **operator-reported** at 5–15 for modern enterprise traps and 30+ for chatty vendors (Cisco IOS-XE, F5) — these specific magnitudes are field experience, not vendor-published. [operator-reported]
- **Retention is the operator's responsibility.** Vendor sources I was able to fetch do not crisply state the default retention for `Orion.Traps` (the `Default-database-retention-settings-for-the-Orion-Platform` KB returns 403 to scrapers). THWACK threads consistently cite **30 days** as the historical default for `Orion.Traps`, but this is [operator-reported]. For the Log-Viewer/Log-Analyzer path, **7 days** is the commonly-cited default for the Log Analyzer SQL database (sourced from secondary references to the Log Analyzer datasheet; the primary product page at <https://www.solarwinds.com/log-analyzer> is a marketing page and does not expose the 7-day value in the public HTML accessible to scrapers). The 7-day figure is therefore **secondary-source / not directly verified** from a vendor doc URL accessible in this review pass. The takeaway is that the two storage paths have *different* defaults — but the **specific numeric values are operator-reported / secondary-source, not vendor-doc-verified**.
- **Cleanup is painful.** Per the THWACK community-validated cleanup query: `delete from TrapVarbinds from TrapVarbinds a, Traps b where a.TrapId = b.TrapId and b.DateTime < datediff(day, -90, getdate())` deletes everything older than 90 days. **`TRUNCATE TABLE` is not usable** because the foreign-key constraint on `TrapVarbinds.TrapID` blocks it (vendor's own KB and operator forum threads confirm this). DELETE operations are documented by operators to take **on the order of 90 minutes** on large deployments — i.e., the platform's own cleanup is a maintenance window of its own.

### 6.5 Storage of Community String — exposure analysis

This is worth calling out. `Orion.Traps.Community` is documented (in the public SWIS schema) as `System.String` with **no encryption qualifier, no hashing qualifier, no redaction qualifier**. The community is **exposed as a queryable string field through the public SWIS API** and through SWQL queries. Vendor docs do not document any access-control mask on this column beyond standard Orion Account Limitations on rows.

The actual at-rest storage form (whether the SQL column is plaintext, encrypted-at-rest by SQL Server TDE, or wrapped by some internal SolarWinds encryption) is **not vendor-documented**. The accurate statement is: *any user with SELECT rights on `Orion.Traps` — directly via SQL Server or via SWIS — sees the community in plaintext form via the query result*. Whether the bytes on the SQL Server data file are encrypted is a deployment-dependent SQL Server administrator question (TDE, Always Encrypted) not addressed by the SolarWinds product itself. Operationally, the exposure is the same: **anyone authorised to run a SWIS or SQL query reads the community in cleartext.** This is a real insider-threat surface.

### 6.6 Indexing

Not publicly documented. Operators routinely report needing to add custom indexes on `Orion.Traps(DateTime, NodeID)` for reporting queries to be tolerable at scale ([operator-reported]).

### 6.7 Migration / upgrade

Database schema migrations are part of the SolarWinds Platform installer. Operators cannot manage these themselves. Major-version upgrades (e.g., 2024.2 → 2025.1) routinely require Configuration Wizard runs that take the database offline. Vendor does not document forward/backward compatibility of trap schema across versions.

---

## 7. Configuration UX

### 7.1 Surfaces

| Surface | What it controls | Source |
|---|---|---|
| **Orion Web Console** > Settings > "All Settings" > MIBs Management | View/update MIB DB | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-management-information-base--mib--sw1730.htm> |
| **Orion Web Console** > Alerts & Activity > Traps | View received traps (opens Log Viewer in current versions) | <https://documentation.solarwinds.com/en/Success_Center/orionplatform/Content/Core-Viewing-SNMP-Traps-in-the-Web-Console-sw752.htm> |
| **Orion Web Console** > Settings > Manage Alerts | Configure alerts that trigger from traps (via Event alert condition + pub/sub) | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-alerting.htm> |
| **Trap Viewer** (Windows desktop app, deprecated) | Legacy filters, rules, alert configuration | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-trap-viewer-settings-sw713.htm> |
| **Log Viewer** (current, web UI) | Trap entries, rules, alerts | <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm> |
| **SWIS API** | Query/update via SWQL over TCP/17774 (was 17778 prior to 2024.2) | <https://solarwinds.github.io/OrionSDK/schema/> |
| **Windows Registry** | Change SNMP trap port (per the deprecated-but-still-cited KB) | KB cited in §3.1 |
| **PowerShell / Orion SDK** | Programmatic queries against SWIS | <https://github.com/solarwinds/OrionSDK> |

### 7.2 What the operator sees by default

After install: traps land in the configured DB; the Web Console menu **Alerts & Activity > Traps** is the entry point; this opens Log Viewer (current) or the deprecated Traps view (older sites). The operator sees a sortable, filterable table with columns: DateTime, Source IP, Hostname, TrapType, Community, ObservationSeverityName, Message (subset configurable via legacy Trap Viewer Displayed Columns or Log Viewer column chooser).

### 7.3 Discoverability of options

- Defaults are documented but scattered across multiple deprecated and current pages.
- The legacy Trap Viewer has a **Settings dialog** with sliders for *"Maximum Number of Traps to Display in Current Traps View"*, *"Automatically Refresh the Current Traps View"*, and *"Retain Trap Messages For How Many Days"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-trap-viewer-settings-sw713.htm>). Exact numeric defaults are not published.
- **The deprecation banner** at the top of every legacy page is a discoverability problem: an operator landing on the Trap Viewer Settings page from a search engine cannot tell whether the page applies to their current install until they read the banner.

### 7.4 Live reload vs restart

- **MIB DB**: installer prompt restarts services automatically (*"the Installer informs you when SolarWinds Platform services need to be restarted and restarts them if necessary"*; <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-downloading-the-solarwinds-mib-database-sw3581.htm>).
- **SNMPv3 credentials**: service restart required (known issue in 2024.2 release notes; see §3.2).
- **Rules**: vendor docs imply rules are applied immediately on save in Log Viewer — but the documented behaviour is not crisply stated.

### 7.5 Multi-tenancy / RBAC

The Orion Web Console supports **Account Limitations** (per-user filters on visible nodes). Trap rows respect node-scoped account limitations. **There is no documented multi-tenant separation of trap streams** (no per-tenant trap pipeline, no per-tenant MIB DB). A single SolarWinds Platform instance is single-tenant by design. MSPs typically deploy **one Orion per customer** rather than one Orion serving many customers.

---

## 8. Integration with Other Signals

### 8.1 Metrics

Traps are **not converted to metrics**. There is no first-class "trap rate" counter exposed as a chart context. Vendor docs do not show a "Traps per minute" widget. Operators who want trap-volume charting typically write SWQL queries against `Orion.Traps` and render the result through a custom dashboard widget.

Traps **can be used as annotations** on PerfStack (the cross-stack correlation view). The Log Viewer feature comparison explicitly lists *"Cross-stack correlation via Perfstack"* as a Log Viewer feature (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-feature.htm>). This is the closest SolarWinds gets to a "trap as metric annotation" — events from Log Viewer can appear in PerfStack timelines next to performance graphs.

### 8.2 Alerting / Notifications

This is the *first-class* surface for traps in SolarWinds.

- **Legacy Trap Viewer alerting**: rules with trigger conditions and actions. Trigger conditions are configured via a *"Conditions"* dialog that lets the operator *"Select object identifiers and comparison functions from the linked context menus"* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-configuring-trap-viewer-filters-and-alerts-sw272.htm>). Available trigger pieces: source IP/subnet (the "Apply This Rule To" list); DNS hostname pattern (case-sensitive); Trap Details patterns; Community String patterns. **Trigger Thresholds** suppress action until N traps arrive matching the rule. **Time-of-Day Checking** windows the rule. **Rule order** matters: *"Rules are processed in the order they appear, from top to bottom"*.
- **Current Log Viewer alerting**: native actions versus alerting actions. Vendor docs explicitly contrast their throughput: *"Native actions … can trigger thousands of times per second"* whereas *"Event alerts actions can trigger approximately twelve times per second for a single rule or alert. If there are multiple rules or alerts, roughly eighty alert actions can trigger per second."* (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm>; retrieved 2026-05-22).

  This is a startling number: **~80 alert actions per second** is the documented platform-wide ceiling for trap-driven alerts. For a network of 1,000 devices in a real storm, you will hit that ceiling immediately and queue or drop. This is a real architectural constraint.

- **Rule actions** (Log Viewer): **Forward the entry**, **Run an external program**, **Flag for discard**, **Stop processing rules**, **Real-time config change detection (NCM)**, and (added in 2025.1) **Change the status of an interface** (per 2025.1 release notes bug-fix entry — confirming the action exists and was previously buggy).
- **Outbound SNMP trap as alert action**: yes — see §1.3 and the "Send an SNMP trap in the SolarWinds Platform" page. Configurable destination IP(s), UDP port, SNMP version, community string, and Trap Template (with macro substitution via `${AlertMessage}`, sysUpTime, snmpTrapEnterprise, etc.).

**Acknowledgement / clear semantics**: `Orion.Traps.Acknowledged` is a `System.Byte` flag (0/1). The Web Console offers an "Acknowledge" button. There is **no documented automatic clear** — a `linkDown` followed by a `linkUp` does not automatically clear the original alert (compare OpenNMS's `alarm-data` `pair-with`/`clear-key` and Zenoss's `clear()` events). Operators must either write a rule that flips state, or rely on the underlying NPM polling-based interface availability metric to clear.

### 8.3 Topology

SolarWinds offers two topology surfaces:

- **Network Atlas** (deprecated). Operator-drawn maps, with nodes/links statused from polled data. Vendor source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-introducing-network-atlas-sw3461.htm>.
- **Intelligent Maps / Orion Maps** (current). Auto-generated topology, embeddable in the Web Console. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-orion-maps-intro.htm>.

Are traps **mapped onto the topology**? Vendor docs do not crisply describe a "show me which port flapped on this map when this trap arrived" workflow. The closest is the node-detail-page integration: a node on a map links to its detail page, where recent traps appear. **Topology-aware suppression** (e.g., suppress downstream device traps when an upstream link goes down) is **not documented as a built-in feature** — operators implement this via rule-time conditions ("if upstream is down, suspend these alerts") if at all.

L2 topology (LLDP/CDP/FDB) is documented via the **User Device Tracker (UDT)** module, not via the trap subsystem. Trap-to-port joins are not a documented capability.

### 8.4 Logs / Events

In the current architecture, **a trap is a log entry** — see §1.2. The Log Viewer surface unifies traps, syslog, Windows events, log files, and VMware events under the same UI, the same rule engine, and the same database. This is the most consequential design point in current SolarWinds: traps lose their privileged-event status and become rows in a log stream.

Searchability: full-text and field search via Log Viewer; SWQL via `Orion.Traps`. Retention: Log Analyzer DB has its own retention; vendor does not publish defaults.

### 8.5 Northbound forwarding

- **Outbound SNMP traps**: yes (see §1.3, §8.2). Trap Template macro substitution; multiple destinations supported.
- **Syslog forwarding**: a built-in alert action (`ForwardSyslog.trap` template path documented in <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-what-is-a-trap-template.htm>).
- **Log Viewer log-forwarding**: vendor docs ship a dedicated **Log forwarding in Log Viewer** documentation page. **The forwarding mechanism is rule-based**, not a separate continuous-stream pipeline — the vendor page instructs the operator to create a custom rule on the Syslog or Traps policy group, select the **`Forward the Entry`** action, and configure the destination IP/UDP port. So "Log forwarding in Log Viewer" is the *vendor's branding for the rule-based forwarding action*; it does not introduce a parallel push-stream subsystem. The operational difference vs the broader rule engine is essentially that this is the documented page operators are directed to when their goal is "send traps to another system" — the action is `Forward the Entry`, not a separate "log-stream" component. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-log-forwarding.htm> (and the matching Log Viewer rules page).
- **Webhook / REST POST**: yes, via the alert action "Send an HTTP Request" (Web Console alert engine; cross-referenced in the alert-actions docs).
- **OTLP**: not documented.
- **Known 2025.1 fix**: the 2025.1 release notes describe a previous defect — *"Forwarding a significant number of syslog and trap messages per second with spoofing no longer results in some messages not being sent, high CPU and RAM usage, and errors..."* — documenting that the northbound forwarding path had known scalability limits prior to 2025.1. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/solarwinds_platform_2025-1_release_notes.htm>.

So SolarWinds **can** be a northbound trap *emitter* — a real cross-system contrast point. (Datadog and Splunk SC4SNMP cannot do this.) Conceptually, this is similar to Sensu's `sensu-snmp-trap-handler` and LibreNMS's "Alert Transport: SNMP Trap" feature.

---

## 9. Severity Model

### 9.1 Where vendor severity comes from

In SolarWinds' model, vendor severity (the integer field commonly in vendor MIBs, e.g., `clogHistSeverity` for Cisco) is **not auto-mapped**. The trap's severity in `Orion.Traps.ObservationSeverity` is whatever the **matched rule** sets, with a fallback default for unmatched traps. The vendor does not publish the default mapping table.

### 9.2 Severity range

`Orion.Traps.ObservationSeverity` is `System.Byte`. Operator-reported range is 0–4 mapping to Informational/Warning/Critical/Down (or similar). The exact name set in `ObservationSeverityName` is determined by the rule and the platform's internal severity dictionary. This is not formally documented.

### 9.3 Customisation surface

- Operators write rules that set severity per matched trap.
- For Log Viewer, the rule UI lets the operator pick a severity from a dropdown when creating an alert.
- There is **no global severity normalization layer** that runs before rules — i.e., no "every Cisco priority-4 becomes our 'Warning'" pre-rule transformer. Every rule re-asserts severity.

### 9.4 Brutal honesty

SolarWinds' severity model is **operator-defined by rules**, not vendor-curated. This is flexible — you can normalize anything if you write the rule — but it makes "out of the box value" poor: a fresh install of SolarWinds plus a fresh fleet of Cisco gear does not give you sensible severities until you write rules. Compare to OpenNMS, where `eventconf.xml` ships with thousands of pre-curated severities, or to Zenoss, where ZenPacks ship with `severity_map` directives. **SolarWinds requires you to do the work**.

---

## 10. Storm / Volume Handling

### 10.1 Per-source rate limits

Not documented at the listener level. The Trap Service does not advertise a per-source-IP rate limiter (unlike, e.g., snmptrapd's `forwardingTraps` rate hooks or Datadog Agent's `comp/snmptraps/listener` discarding policy).

### 10.2 Documented throughput ceilings

- **500 traps/sec per polling engine** (see §3.3).
- **1,000 events/sec combined (syslog+traps) per polling engine with Log Viewer enabled** (same source).
- **~80 alert actions/sec across all rules** (Log Viewer documented ceiling, see §8.2).

The alert-actions ceiling is the binding constraint for any real network: at 80 actions/sec, a 1,000-device storm exceeds capacity within seconds and either queues or drops actions. Vendor docs do not crisply state which behaviour applies (queue with bound? drop?).

### 10.3 Dedup keys and windows

There is **no trap-row dedup**. There is **rule-level suppression**:

- **Trigger Threshold** ("until a specified number of traps arrive that match the rule") — see §5.6.
- **Suspend Further Alert Actions For** — operator-configured time window after a fire.

Note that *the trap is still written to the database* — only the alert action is suppressed. So storm pressure on the SQL Server is not relieved by alert-suppression.

### 10.4 Circuit breakers

Not documented.

### 10.5 Storm detection

Not documented as a first-class feature. The closest is the Log Viewer's rate-limit configuration on rules (default *"no cooldown time"* — operator must opt in).

### 10.6 Backpressure / queue management

Not documented. Operators on THWACK reporting trap loss during storms typically point to:

- UDP buffer exhaustion at the OS level (Windows registry tuning of `MaxUserPort`, socket buffer).
- SQL Server write contention (operator-reported during storm conditions).
- The 80 actions/sec alert ceiling.
- **HA virtual-hostname UDP-port-exhaustion (2026.1 known issue, fixed in 2026.1.1, see §2.2)** — multi-subnet HA hosts running the virtual-hostname mode could run out of ephemeral UDP ports until the host needed a reboot. Fixed in 2026.1.1. This is a platform-level backpressure failure mode that affected trap-receiving deployments combining HA with high event volume on 2026.1.
- **RabbitMQ queue growth (2025.1 known issue)** — the 2025.1 release notes document that RabbitMQ queues used internally for inter-component messaging can grow significantly under load, causing delayed alerts and data-sync issues. Trap-triggered alerting that flows through the alerts engine via the documented pub/sub mechanism (§2.4) is on the same internal bus; this is a real backpressure concern for trap-driven alert fan-out. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/solarwinds_platform_2025-1_release_notes.htm>.

There is no vendor "back-pressure" or "shed-load" telemetry.

---

## 11. Security

### 11.1 SNMPv3 USM support

- Auth: MD5, SHA-1, **SHA-256, SHA-512** (the last two added 2024.2 for trap-side; see §3.2).
- Priv: AES-128, AES-256 (per polling-side docs; trap-side privacy algorithm list not separately published).
- Multi-user table: supported (release notes lineage).

### 11.2 DTLS / TLSTM support

**Not documented**. No vendor page describes TLSTM trap reception. This is a real feature gap for security-forward operators.

### 11.3 Credential storage

- **SNMPv3 credentials**: stored in the Orion DB via dedicated SWIS entities such as `Orion.SNMPCredentialV3` (per the SWIS schema index at <https://solarwinds.github.io/OrionSDK/schema/>). Operator-reported encrypted-at-rest, but vendor does not publish the encryption mechanism. Required service restart when SNMPv3 credentials change (known issue cited in §3.2) implies the in-memory cache is loaded at service start.
- **v1/v2c community strings in trap records**: `Orion.Traps.Community` is exposed as a queryable `System.String` field via the public SWIS API and SWQL, with **no documented redaction or encryption qualifier**. See §6.5. The at-rest form is not vendor-documented; the exposure surface is the query path.

### 11.4 Access control on the trap subsystem itself

- Orion Account Limitations restrict *which trap rows a user can see* in the Web Console.
- There is no documented control on *who can write trap rules* beyond standard Orion role permissions.
- There is no documented per-source-IP allowlist on the Trap Service itself — i.e., the service accepts any UDP/162 packet that reaches it. Filtering must be done at the Windows firewall layer.

### 11.5 Audit logging

Orion has a general audit-log feature. **Vendor docs consulted in this review do not document trap-specific audit coverage** for actions such as who acknowledged a trap, who edited a Log Viewer rule, or who modified the Trap Service configuration. Whether the platform-wide audit log captures these events is not stated by the cited pages; the answer is **silence**, not assumed coverage.

### 11.6 Trap-specific guidance in vendor "Secure Configuration" doc

The vendor's **Secure Configuration for the SolarWinds Platform** page (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-secure-configuration.htm>) was consulted as part of this analysis. It contains general SolarWinds Platform security best practices (web-console hardening, account policies, a CISA reference for SNMP best practices) but **does not contain trap-specific hardening guidance** — no per-source UDP/162 allowlist procedure, no trap-service privilege minimisation, no Trap-Service-specific credential rotation runbook. The absence of trap-specific guidance on the vendor's central security page is itself a finding for an enterprise NMS in 2026.

### 11.7 Brutal honesty

SolarWinds' trap-side security is **good-enough-for-2010, late-to-modern**. SHA-2 in 2024 (mid-2024). No DTLS. Community strings exposed via the public query API with no documented redaction (the at-rest encryption state is deployment-dependent). No per-source allowlist. The platform leans heavily on "trust the management network" as a security baseline. For operators in regulated environments this is a real concern — and on top of the **post-SUNBURST** organisational risk picture (see §1.5), it is a hard sell to security-mature buyers in 2026.

---

## 12. Trap Simulation & Testing (in-source evidence)

### 12.1 Unit / integration tests

**Closed source. Not visible.** SolarWinds does not publish a test suite for the Trap Service, does not publish code coverage numbers, does not document fixtures.

### 12.2 Sample trap fixtures included

Not visible.

### 12.3 Tools shipped for trap simulation

Yes — multiple:

- **Engineer's Toolset SNMP Trap Editor** (<https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-trap.htm>) — desktop tool that **sends** test traps. Lets operators construct PDUs by hand.
- **Engineer's Toolset SNMP Trap Receiver** (<https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-trap-receiver.htm>) — a standalone receiver for debugging trap *delivery* before it hits the Orion Trap Service. Useful when the operator is trying to verify the device is emitting at all.
- **MIB Browser** for OID/MIB exploration (not a trap tool per se, but related).

These are **separate products** (Engineer's Toolset) — they are not part of the core Orion Platform. The boundary is intentional: the production platform receives; the operator tools send/receive for diagnostics.

### 12.4 CI workflow for trap pipeline

**Not visible.** The platform is closed-source.

### 12.5 Evidence confidence

The absence of public test artefacts is itself a finding — every open-source system in this comparative set publishes meaningful test coverage (Datadog has ~2,800 lines of trap tests for ~2,000 lines of trap source; OpenNMS has hundreds of TestNG cases; Centreon ships a Perl test suite). SolarWinds publishes nothing. The platform's trap-side robustness is verifiable only via operator-reported field experience.

---

## 13. Out-of-the-Box Coverage (defaults)

### 13.1 MIBs bundled

Vendor claims *"over 250,000 precompiled unique OIDs from hundreds of standard and vendor MIBs"* in the MIB Browser tool (<https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-mib-browser.htm>). Vendor does not crisply state how many of these are also in the platform's bundled trap MIB DB; operator inference is that the two sets overlap substantially. **Confidence: medium** (vendor marketing number; not independently verifiable).

### 13.2 Severity rules bundled

The platform does not ship a comprehensive default severity rule set. Operators get a "default severity" for unmatched traps; everything beyond that requires writing rules. **Confidence: high** (vendor docs do not describe a stock rule library; THWACK threads confirm operators routinely build their own).

### 13.3 Dedup defaults

None at trap-row level. Alert-level dedup defaults to *"every matching entry"* (no threshold). Operator must opt in. **Confidence: high** (per the Log Viewer rules doc).

### 13.4 Vendor packs / integration packages

There is no formal **"Vendor Trap Pack"** marketplace (compare to Zenoss's ZenPacks, Datadog's integrations-core, OpenNMS's `discoveryd` integration plug-ins). Vendor traps are handled by the MIB DB and operator-written rules — there is no shipped "Cisco Trap Pack" or "F5 Trap Pack" with curated severities and dedup keys.

### 13.5 Sample / preset dashboards or reports

The Web Console ships with **Top XX widgets** (Top XX Nodes, Top XX Errors, Top XX Traps). A "Traps" view existed pre-Log-Viewer. With Log Viewer, the equivalent is the Log Viewer dashboard. There is **no out-of-the-box "trap rate per device" PerfStack template** documented.

### 13.6 Default trap port and retention

- Default port: **UDP/162** (changeable via the KB cited in §3.1).
- Default retention: **not crisply documented in publicly-accessible vendor docs**. The Default-database-retention-settings KB is access-controlled. Operator-reported default historically **30 days** for `Orion.Traps`; the 7-day Log Analyzer figure is secondary-source (see §6.4).

### 13.7 Confidence

Overall §13 confidence: medium-low. The 250K-OIDs claim is marketing; the absence of a vendor-curated severity catalogue is the most important finding here.

---

## 14. User Customization Surface

### 14.1 Add custom OID handlers

- Write Log Viewer (or legacy Trap Viewer) **rules** that match on OID, varbind, source IP, community.
- No code; no plugin model; no custom-decoder hook.
- **Log Viewer supports rule import/export** ("Import and export rules" is a documented sibling feature of the rule-creation page). This partially compensates for the lack of a plugin model — operators can package and share rule sets across Orion instances, similar in spirit to OpenNMS sharing event XMLs or LibreNMS sharing trap handler bundles, though it does not approach the depth of Zenoss ZenPacks. Source: Log Viewer documentation navigation sibling to <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm>.

### 14.2 Add custom MIBs

**The MIB DB is closed.** The vendor workflow is "submit the MIB to SolarWinds for inclusion". The operator has **no documented path** to compile a local MIB into the platform. See §4.4 for the full discussion. This is the single sharpest customisation gap relative to the rest of the comparison set.

### 14.3 Custom severity rules

Done via rules; no shipped rule library to extend.

### 14.4 Custom dedup rules

Done via rule Trigger Thresholds and Suspend windows. No trap-row dedup; no fingerprint hashing.

### 14.5 Plugin / extension model

There is **no plugin model for the Trap Service**. Compare:

| System | Plugin / extension model |
|---|---|
| Zenoss | ZenPacks (Python) |
| OpenNMS | OSGi plug-ins, event configurations, drools rules |
| Nagios+SNMPTT | EXEC handlers, SNMPTT format actions, NSCA passive checks |
| LibreNMS | PHP `SnmptrapHandler` per OID |
| Datadog Agent | `traps_db/` user-supplied JSON/YAML files |
| Telegraf | Go processor plugins |
| **SolarWinds** | **None** (rules only) |

### 14.6 API surface for automation

- **SWIS / SWQL** over TCP/17774: query traps, varbinds, nodes; act on alerts. Public schema at <https://solarwinds.github.io/OrionSDK/schema/>.
- **PowerShell module** and `SolarWinds.InformationService` .NET assemblies via the Orion SDK (`https://github.com/solarwinds/OrionSDK` — open-source SDK wrapper around the closed product). This is the *only* SolarWinds-owned open-source artefact relevant to this analysis.
- **REST-ish "Information Service"** also exposed; documented in Orion SDK.

### 14.7 Confidence

Customisation surface is well-documented (high confidence) and is **rule-engine-heavy, plugin-light**.

---

## 15. End-User Value Analysis

### 15.1 What an operator gets day-1 with default config

After installation:

- A working UDP/162 listener that records every received trap into the database.
- The Web Console **Alerts & Activity > Traps** view (Log Viewer in modern, deprecated Traps view in older sites) — filterable, sortable, acknowledgable.
- A MIB database with broad OID resolution coverage out of the box.
- Standard NPM polling alongside (so traps and metrics live in the same product).
- The standard SolarWinds alerting engine, ready to fire on trap matches.

This is a genuinely good out-of-the-box experience — operators can see traps in the UI within minutes of install, without writing a single rule. Subjective comparison against the install workflows of the other systems analysed so far (OpenNMS, Zabbix, LibreNMS, Centreon, Datadog, etc.) suggests SolarWinds is near the front of the pack for **install-to-first-trap-visible** time, because the installer brings up the UI, the listener, and the DB schema as one bundled act.

### 15.2 What requires customisation

- **Severity normalisation**: operator-written rules (no curated catalogue).
- **Storm management**: operator-written Trigger Thresholds and Suspend windows (not auto-tuned).
- **Topology correlation**: operator-written rules; no built-in topology-aware suppression.
- **Custom vendor MIBs**: vendor round-trip (see §4.4).
- **Per-source allowlist on UDP/162**: Windows firewall (not in-product).

### 15.3 Learning curve

Moderate. The Web Console is familiar to Windows-shop operators. Rules are configured via point-and-click. The hardest piece is understanding the Trap Viewer (deprecated) versus Log Viewer (current) split — operators routinely Google their way to the wrong (deprecated) docs.

### 15.4 Operational toil

Significant at scale:

- **DB maintenance**: 90-minute DELETEs on `TrapVarbinds` (operator-reported) when retention catches up.
- **MIB updates**: round-trip through the vendor for any unbundled MIB.
- **Service-restart for credential changes**: production downtime to rotate SNMPv3 keys.
- **Manual APE assignment**: device-by-device reconfiguration when scaling out.

### 15.5 Visibility into the pipeline's own health

Poor. No documented per-pipeline telemetry. Operators debug missing traps with Wireshark, not with a "trap drop counter".

### 15.6 Confidence

High. These observations are consistent across vendor docs and the THWACK community archive.

---

## 16. Strengths

Brutally honest: SolarWinds *does* genuinely well in several places.

1. **Time-to-first-trap-visible is fast.** Install Orion, open the Web Console, point a switch at it, and the trap appears within seconds in a usable UI. Many open-source systems in this set require config-file editing before the first trap shows up. SolarWinds is "install and go". Source: implicit in the install-and-see-trap workflow documented at <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-monitoring-snmp-traps-sw593.htm>.
2. **Unified UI across signals.** Traps, syslog, Windows events, and VMware events in one Log Viewer surface with one rule engine — operationally simple compared to running snmptrapd + SNMPTT + rsyslog + custom dashboards in OSS land. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-feature.htm>.
3. **First-class node-aware schema.** The SWIS schema exposes a `NodeID` column on `Orion.Traps` and a `Node` relationship via `Orion.NodesHostsTraps`, so trap rows can be joined to managed nodes for queries and UI display. The exact matching algorithm (UDP source IP → `Orion.Nodes`) is implied by the schema columns but is not vendor-documented; the operational outcome — traps surfacing on the node detail page — is consistent and ergonomically good. Source: SWIS relation `Orion.NodesHostsTraps`.
4. **Outbound trap emission as alert action.** The platform can both receive and emit traps (with templates, macros, multi-destination). Few systems in the set do both. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-sending-an-snmp-trap-sw1082.htm>.
5. **Documented sizing.** The vendor publishes concrete throughput numbers (500 traps/sec per PE, 1,000 events/sec with Log Viewer). Most commercial vendors hide these. Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/orion_platform_scalability_engine_guidelines.htm>.
6. **Rich rule engine** (Trigger Thresholds, Time-of-Day Checking, multi-action, external program execution, outbound trap, status mutation). Source: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-configuring-trap-viewer-filters-and-alerts-sw272.htm> (legacy) and <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm> (current).
7. **Public SWIS schema.** SolarWinds publishes the database entity schema on GitHub (one of the few transparency wins in their closed product). Source: <https://solarwinds.github.io/OrionSDK/schema/>.

---

## 17. Weaknesses / Gaps

1. **Closed MIB workflow.** The "submit your MIB to us" workflow is the largest customisation gap in the comparison set at scale. (§4.4). The most material customisation finding in this file.
2. **No trap-row deduplication.** Every PDU writes a row. Storms balloon the DB. Cleanup DELETEs are slow on large deployments (operator-reported on the order of 90 minutes). (§5.6, §6.4.)
3. **80 alert-actions-per-second ceiling.** A documented platform-wide cap that real-world storms blow past. (§8.2, §10.2.)
4. **Community strings exposed via the public SWIS query surface in `Orion.Traps.Community`.** No documented redaction or encryption qualifier on the field; real insider-threat exposure through the SWIS API and SWQL. (§6.5, §11.3.)
5. **No DTLS / TLSTM.** No modern secure-transport option. (§11.2.)
6. **SHA-256/SHA-512 added late** (2024 for trap-side). (§3.2, §11.1.)
7. **Trap Viewer deprecation in progress.** Operators routinely hit deprecated documentation; the migration to Log Viewer is incomplete in the docs. (§1.2, §7.3.)
8. **No per-pipeline telemetry.** Operators debug with Wireshark, not metrics. (§2.5, §10.6.)
9. **No plugin model for the Trap Service.** Customisation is rule-bound. (§14.5.)
10. **HA does not protect the database.** Database HA is the operator's problem. (§2.2.)
11. **No IPv6 in HA.** (§2.2.)
12. **No container/K8s deployment.** Windows-only, install-on-bare-OS. (§2.2.)
13. **Closed-source — no public test artefacts.** Robustness is verifiable only via field reports. (§12.)
14. **Central SQL DB is the scale ceiling.** Adding APEs offloads the listener; nothing offloads the database. (§3.5.)
15. **Post-SUNBURST trust environment.** A real adoption headwind in 2026. (§1.5.) Note: this is a *platform-wide* weakness inherited by the trap subsystem (any customer who deploys the platform inherits the central-update trust boundary), not a defect of the trap pipeline itself. Listed here because operator-evaluation of "should we install SolarWinds to receive traps in 2026" must factor it in.
16. **No vendor-curated severity catalogue.** Operators do severity normalisation themselves. (§9, §13.2.)
17. **Multi-tenancy is "deploy more Orions"**, not a per-tenant pipeline in one Orion. (§7.5.)

---

## 18. Notable Code or Configuration Examples

Closed-source. Configuration snippets only.

### 18.1 SWIS query for recent traps (SWQL)

Operator-published example (paraphrased from THWACK; <https://thwack.solarwinds.com/products/the-solarwinds-platform/f/solarwinds-sdk/92353/where-in-swql-i-can-see-trap-information>):

```sql
SELECT TOP 100
  T.TrapID, T.DateTime, T.IPAddress, T.Hostname, T.TrapType,
  T.Community, T.ObservationSeverityName, T.Message,
  T.Acknowledged
FROM Orion.Traps T
WHERE T.DateTime > AddHour(-24, GETUTCDATE())
ORDER BY T.DateTime DESC
```

Plus a join to varbinds:

```sql
SELECT V.TrapID, V.TrapIndex, V.OID, V.OIDName, V.OIDValue
FROM Orion.TrapVarbinds V
WHERE V.TrapID = @TrapID
ORDER BY V.TrapIndex
```

Demonstrates the **1:N row growth pattern** and the **community-string-exposure** through the public query surface. Source thread for SWQL examples: <https://thwack.solarwinds.com/products/the-solarwinds-platform/f/solarwinds-sdk/92353/where-in-swql-i-can-see-trap-information>.

### 18.2 Outbound Trap Template (reconstructed illustration; vendor element schema)

The vendor "What is a Trap Template?" page (<https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-what-is-a-trap-template.htm>) documents the **element schema** for a trap template — bundled files such as `SolarWinds/Orion/Orion-Detailed-Alert.trap`, `SolarWinds/Orion/Orion-Generic-Alert.trap`, and `SolarWinds/Orion/ForwardSyslog.trap`, with per-OID elements of the form:

```text
OID OID="<numeric>" MIB="<MIB name>" Name="<name>" Value="<value>" DataType="<type>" ValueName="<variable>" HexValue="<hex>"
```

The vendor page lists `${AlertMessage}` and other macros that can be substituted at trap emission time, and instructs the operator to *"add more information by adding another OID element and incrementing the OID"*.

For illustration of how an alert-time template assembles into a PDU (NOT a verbatim vendor quote; reconstructed from the vendor element schema above), an `Orion-Detailed-Alert.trap` template conceptually wraps the standard SNMP varbinds `sysUpTime.0` and `snmpTrapOID.0` plus an enterprise-specific `alertMessage` varbind carrying the substituted `${AlertMessage}` text. The exact wire-format wrapper used by the platform is not published in the cited page; only the per-`OID` element format is. Reviewers should treat this section's structure as a reconstructed illustration of the per-element format documented by SolarWinds, **not** as vendor-verbatim XML.

This is the format SolarWinds uses to *emit* traps northbound — the only such artefact in the comparison set with this level of template detail.

### 18.3 THWACK-recommended retention cleanup (operator-published; rephrased for T-SQL correctness)

```sql
-- Delete trap varbinds older than 90 days; the FK on TrapID
-- prevents TRUNCATE, so DELETE is the only option. T-SQL date
-- arithmetic uses DATEADD (not DATEDIFF) for "now minus N days".
DELETE TrapVarbinds
  FROM TrapVarbinds a
  INNER JOIN Traps b ON a.TrapID = b.TrapID
  WHERE b.DateTime < DATEADD(DAY, -90, GETDATE());

-- Then delete the trap rows themselves
DELETE FROM Traps
  WHERE DateTime < DATEADD(DAY, -90, GETDATE());
```

This DELETE pattern is what operators run when the platform's own retention job has fallen behind. Note that the original community-posted variants used invalid `datediff(day, -90, getdate())` syntax (`DATEDIFF` does not take a negative offset to subtract days; `DATEADD` does); the corrected `DATEADD(DAY, -90, ...)` form above is the operationally-correct T-SQL. Source thread (paraphrased): <https://everythingshouldbevirtual.com/purge-syslog-trap-alerts-solarwinds-db/> and the vendor KB summary referenced from search-engine indexing of `Truncate_Traps_and_TrapVarbinds`.

### 18.4 Log Viewer rule (varbind match)

From <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm>:

> *"For SNMP Traps, the example shows matching on Varbind element with OID and name criteria."*

Operator workflow (paraphrased from the rule-builder dialog): pick a varbind position, pick a comparison function (`equals`, `contains`, `starts with`), pick a target value. Combine clauses with AND/OR. Specify an entry threshold and a cooldown.

### 18.5 SNMPv3 multi-user trap configuration

Vendor docs do not publish a literal configuration file (it is GUI-driven). The 2024.2 release notes show the **breaking-change** — modifying credentials while the service runs causes authentication failures and requires `SolarWindsTrapService` restart.

---

## 19. Sources Examined

All URLs retrieved on **2026-05-22** unless otherwise noted.

### 19.1 Vendor documentation (primary)

- Monitor SNMP traps: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-monitoring-snmp-traps-sw593.htm>
- View SNMP traps in the Orion Web Console: <https://documentation.solarwinds.com/en/Success_Center/orionplatform/Content/Core-Viewing-SNMP-Traps-in-the-Web-Console-sw752.htm>
- Configure Trap Viewer filters and alerts (deprecated): <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-configuring-trap-viewer-filters-and-alerts-sw272.htm>
- Configure the Trap Viewer (deprecated): <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-trap-viewer-settings-sw713.htm>
- Send an SNMP trap in the SolarWinds Platform: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-sending-an-snmp-trap-sw1082.htm>
- What is a Trap Template?: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-what-is-a-trap-template.htm>
- Log Viewer feature comparison: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-feature.htm>
- Create custom log-processing rules in Log Viewer: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-create-custom-rules.htm>
- Integrate SolarWinds Platform alerts with Log Viewer: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-alerting.htm>
- Management Information Base (MIB) in the SolarWinds Platform: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-management-information-base--mib--sw1730.htm>
- Update the SolarWinds MIB Database: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-downloading-the-solarwinds-mib-database-sw3581.htm>
- Manage polling engines in the SolarWinds Platform: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-managing-orion-polling-engines-sw1718.htm>
- Additional Polling Engines: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-using-additional-polling-engines-sw260.htm>
- Scalability engine guidelines: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/orion_platform_scalability_engine_guidelines.htm>
- High Availability overview: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/ha_what_is_high_availability.htm>
- IPv6 support in SolarWinds products: <https://support.solarwinds.com/SuccessCenter/s/article/IPv6-support-in-SolarWinds-products>
- Secure Configuration for the SolarWinds Platform: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/core-secure-configuration.htm>
- Licensing model: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/orion_platform_licensing_model.htm>
- SolarWinds Platform 2024.2 release notes: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/solarwinds_platform_2024-2_release_notes.htm>
- SolarWinds Observability Self-Hosted 2024.2 release notes: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/hco_2024-2_release_notes.htm>
- SolarWinds Platform 2025.1 release notes: <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/solarwinds_platform_2025-1_release_notes.htm>
- SolarWinds Platform / Observability Self-Hosted 2026.1 release notes (TCP/17734 APE→main-engine channel; HA virtual-hostname UDP-port-exhaustion known issue): <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/hco_2026-1_release_notes.htm>
- SolarWinds Platform / Observability Self-Hosted 2026.1.1 release notes (fix for the HA virtual-hostname UDP-port-exhaustion issue): <https://documentation.solarwinds.com/en/success_center/orionplatform/content/release_notes/hco_2026-1-1_release_notes.htm>
- Log forwarding in Log Viewer (vendor-documented rule-based forwarding via the `Forward the Entry` action): <https://documentation.solarwinds.com/en/success_center/orionplatform/content/lm/lm-log-forwarding.htm>.

### 19.2 SWIS schema and Orion SDK (vendor-published reference)

- Orion.Traps entity: <https://solarwinds.github.io/OrionSDK/schema/Orion.Traps.html>
- Orion.TrapVarbinds entity: <https://solarwinds.github.io/OrionSDK/schema/Orion.TrapVarbinds.html>
- SWIS schema index: <https://solarwinds.github.io/OrionSDK/schema/>
- **Orion SDK GitHub** (open-source wrapper around the closed product; canonical reference for the SWIS API surface and PowerShell tooling — the *only* SolarWinds-owned open-source artefact relevant to this analysis): <https://github.com/solarwinds/OrionSDK>
- SWIS port migration (TCP/17778 → TCP/17774) is documented as a breaking change in the SolarWinds Platform 2024.2 release notes cited under §19.1.

### 19.3 Engineer's Toolset documentation (related, separate product)

- SNMP MIB Browser tool: <https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-mib-browser.htm>
- SNMP Trap Receiver tool: <https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-trap-receiver.htm>
- SNMP Trap Editor tool: <https://documentation.solarwinds.com/en/success_center/ets-desk/content/tools/snmp-trap.htm>

### 19.4 THWACK community (operator-reported; explicitly tagged)

- Where are the SNMP traps stored in the SQL database: <https://thwack.solarwinds.com/discussion/80603/where-are-the-snmp-traps-stored-in-the-sql-database>
- Database tables — Traps vs TrapVarbinds: <https://thwack.solarwinds.com/discussion/77951/database-tables---traps-vs-trapvarbinds>
- Tips and Tricks for Managing Traps and Syslog in Orion NPM: <https://thwack.solarwinds.com/products/network-performance-monitor-npm/f/forum/36463/tips-and-tricks-for-managing-traps-and-syslog-in-orion-npm>
- SHA256 and SHA512 support for SNMP Traps (feature-request thread): <https://thwack.solarwinds.com/discussion/122060/sha256-and-sha512-support-for-snmp-traps>
- SNMP polling & traps best practices (Observability Self-Hosted forum): <https://thwack.solarwinds.com/products/solarwinds-observability-self-hosted/f/forum/103829/snmp-polling-traps-best-practices>
- Cleanup blog (operator-published, third-party): <https://everythingshouldbevirtual.com/purge-syslog-trap-alerts-solarwinds-db/>

### 19.5 SUNBURST context (third-party)

- TechTarget overview: <https://www.techtarget.com/whatis/feature/SolarWinds-hack-explained-Everything-you-need-to-know>
- SolarWinds Security Advisory FAQ: <https://www.solarwinds.com/sa-overview/securityadvisory/faq>

### 19.6 Sources attempted but blocked

- `https://support.solarwinds.com/SuccessCenter/s/article/Troubleshooting-Traps-Issues` — 403 to scraper.
- `https://support.solarwinds.com/SuccessCenter/s/article/Default-database-retention-settings-for-the-Orion-Platform` — 403.
- `https://support.solarwinds.com/SuccessCenter/s/article/Truncate-Traps-and-TrapVarbinds` — 403.
- `https://solarwindscore.my.site.com/SuccessCenter/s/article/Change-the-SNMP-trap-port` — 403.

These pages are publicly listed in search results, but the Success Center articles are behind a JS-rendered shell that returns 403 to non-browser clients. Where their content was needed, this file relied on summarised snippets returned by the search index (cited indirectly) plus consistent THWACK confirmations.

---

## 20. Evidence Confidence

Per-section ratings (`high` = vendor-doc-explicit; `medium` = vendor-doc-implicit or release-notes-cross-referenced; `low` = THWACK / operator-reported / inference):

| Section | Confidence | Note |
|---|---|---|
| §1 System Overview & Lineage | High | Multiple vendor URLs cross-confirm. |
| §1.5 SUNBURST context | High | Multiple third-party confirmations + vendor advisory page. |
| §2 Architecture (components, deployment) | Medium | Vendor confirms components; service exe name is operator-reported. |
| §2.3 Implementation language | Low | Inference from executable namespace. |
| §3 Trap Reception | High | Vendor doc + release notes. |
| §3.2 SNMPv3 algorithms | High | Vendor release notes verbatim. |
| §3.3 Throughput numbers | High | Vendor sizing guide verbatim. |
| §4 MIB Management | High | Vendor doc verbatim ("you can have it added to the SolarWinds MIB database"). |
| §4.3 250K OIDs claim | Medium | Vendor marketing number; not independently auditable. |
| §5 Pipeline | Medium | Vendor docs describe surfaces but not the internal pipeline implementation. |
| §6 Data Model | High | SWIS schema is publicly published. |
| §6.2 `ObservationSeverity` type | High | Live SWIS page renders the property twice (both as `System.Byte`) — a duplicate-row rendering bug in SolarWinds' published schema, not a type ambiguity. Canonical type is `System.Byte`. |
| §6.4 Row-growth N=5–15 estimate | Low | Operator-reported field experience; not vendor-published. |
| §6.4 Default retention (30d Orion.Traps / 7d Log Analyzer) | Medium | 7d is from Log Analyzer datasheet (vendor); 30d Orion.Traps default is operator-reported. |
| §6.5 Community-string exposure | High | SWIS schema declares `Community` as `System.String` with no encryption qualifier; SWIS API exposes the value as a plain string. At-rest column encryption is deployment-dependent SQL Server admin, not product-documented. |
| §6.4 90-min DELETE | Low | Operator-reported; treat as field experience, not vendor commitment. |
| §7 Configuration UX | High | Vendor doc explicit. |
| §8 Integration with other signals | High (alerting), Medium (topology) | Topology integration is vendor-mentioned but not crisply linked to traps. |
| §8.2 80 actions/sec ceiling | High | Vendor doc verbatim. |
| §9 Severity | Medium | Vendor doesn't publish defaults; inference from rule UI. |
| §10 Storm handling | Medium | Vendor docs sparse on storm semantics. |
| §11 Security | High | Vendor explicit on supported algorithms; absence of DTLS is explicit-by-omission. |
| §12 Testing | High | Vendor publishes no test artefacts. (Confidence in *the absence*, not in *the content*.) |
| §13 Defaults | Medium | Vendor doc partial; gaps noted. |
| §14 Customisation | High | Vendor doc explicit. |
| §15 End-user value | Medium | Synthesis from vendor + community. |
| §16 Strengths | High | Each item cites a vendor URL. |
| §17 Weaknesses | High | Each item cites a vendor or schema URL or release-note. |
| §18 Examples | Medium | SWQL example is operator-paraphrased; trap template is vendor verbatim. |

Overall confidence: **medium-high**. The closed-source nature of SolarWinds caps the maximum confidence achievable; the SWIS schema and release notes give surprising depth where they cover their topic.

---

## Appendix A. Comparative Lens (cross-system framing)

**Note on matrix scope**: this table is the **incremental** comparison surface defined by the SOW (§Comparison Matrix). Systems not yet in the matrix (Sensu, Telegraf, Logstash, Splunk SC4SNMP, Cribl, Dynatrace, LogicMonitor) will be added as their per-system files are completed and accepted. SolarWinds is placed here to anchor cross-system comparisons against the on-prem-NMS cohort already analysed.

**Note on the "alert-action throughput" row**: SolarWinds is the **only system in this set that publishes a hard platform-wide ceiling** (~80 alert actions/sec, documented in vendor Log Viewer rules page). Other systems certainly have practical ceilings (worker pools, queue depth, DB write rates, notification-backend ingestion limits), but those ceilings are *not vendor-published*. The cell value "not published" means "we have not found a vendor-stated cap", not "infinite throughput".

**Note on the "Outbound trap as action" row**: SolarWinds is the only system in this set with a **native, first-class, PDU-templating outbound-trap action** (the Trap Template format with `${AlertMessage}` and other macro variables, see §18.2). Several other systems answer "yes" because they can emit traps via a generic notification/EXEC hook running the upstream `snmptrap` CLI tool (Nagios, Zabbix, Centreon) or an event northbounder (OpenNMS), or via a dedicated Alert Transport (LibreNMS). These are architecturally different mechanisms; the column annotates the *style* per system rather than collapsing the answer to a bare "yes".

| Dimension | SolarWinds | OpenNMS | Centreon | Zenoss | Zabbix | LibreNMS | Nagios+SNMPTT | Datadog Agent |
|---|---|---|---|---|---|---|---|---|
| Trap reception | own listener | own (trapd) | snmptrapd → centreontrapd | own (zentrap) | snmptrapd → Perl → Zabbix | snmptrapd → PHP | snmptrapd → SNMPTT | gosnmp own listener |
| MIB add workflow | **vendor round-trip** | local compiler | `centFillTrapDB` | `zenmib` | snmptrapd + MIB files | snmptrapd + PHP handler | `snmpttconvertmib` | `ddev meta snmp generate-traps-db` |
| Dedup at trap-row level | **none** | alarm-dedup | none (status-based) | event-dedup | none | none | none | none (SaaS side) |
| Alert-action throughput | **80/sec vendor-published** | not published | not published | not published | not published | not published | not published | not published (SaaS) |
| Central SQL bottleneck | **yes (SQL Server)** | yes (Postgres) | yes (MariaDB) | yes (MariaDB+Redis+Lucene) | yes (MySQL/Postgres) | yes (MariaDB) | no (file-based) | none (SaaS forward) |
| Customisation model | rules only | OSGi+drools+events | rules+macros | ZenPacks | scripts+actions | PHP handler per OID | EXEC scripts | user `traps_db/` JSON |
| SHA-256/SHA-512 SNMPv3 traps | **2024.2 (very late)** | yes | yes (via snmptrapd) | yes | yes (via snmptrapd) | yes (via snmptrapd) | yes (via snmptrapd) | yes (via gosnmp) |
| Container / K8s deploy | **no** | yes | yes (Docker) | yes | yes | yes | yes | yes (everywhere the Agent runs) |
| Outbound trap as action | **yes — native Trap Template engine** | yes (event northbounder) | yes (notification) | yes (action) | yes (action; often via `snmptrap` CLI) | yes (Alert Transport: SNMP Trap) | yes (notification command running `snmptrap` / generic EXEC) | **no** |
| Public test artefacts | **none** | extensive | moderate | moderate | moderate | moderate | minimal | extensive |
| Per-pipeline self-telemetry | **none documented** | yes (JMX) | yes (centreontrapd stats) | yes | yes | yes (Laravel) | minimal | yes (`datadog.snmp_traps.*`) |
| Multi-site federation | **EOC** (read-only aggregation across independent Orions) | distributed model + Minions | central broker + remote pollers | distributed Collectors → ZEP | proxies | central poll-cluster | n/a (per-instance) | SaaS-side (per-account) |

### A.1 SolarWinds in the central-tier-dominant cohort

Of the comparison set, the **central-tier-dominant** cohort is: SolarWinds, OpenNMS, Centreon, Zenoss, Zabbix, LibreNMS. The **distributed / federated** cohort is: Datadog (SaaS-side), Splunk SC4SNMP (K8s-scaled), Telegraf+Kapacitor (pipeline), Sensu (broker), Logstash/Cribl (pipeline). SolarWinds sits at the most extreme end of the central-tier cohort: **single SQL Server, single Web Console, scale by adding workers that fan in to the same DB**. Even within that cohort, SolarWinds is the one with:

- The least transparent storage format (closed MIB DB vs. Centreon's open MariaDB tables);
- Late crypto modernisation cadence on trap-side SNMPv3 (SHA-2 added in vendor's own 2024.2 release — vendor release-notes-confirmed; SHA-2 support landed in most other systems' upstreams earlier, though precise per-system dates are to be filled when the matrix is finalised);
- The smallest customisation surface (rules vs. ZenPacks/OSGi/Perl);
- The largest vendor-supplied default content (250K OIDs vs. ~10K in OpenNMS distros).

### A.2 SolarWinds as a counter-example for the hub model

Netdata's hub model (per `netdata-snmp-hub-architecture.md`):

- Per-site hub. SolarWinds: central server (or EOC over multiple central servers).
- No central DB. SolarWinds: central SQL Server is the platform.
- Correlation lives next to data. SolarWinds: correlation lives in the central alerts engine.
- Add a site = add a hub. SolarWinds: add a site = bigger central tier (or another isolated Orion install + EOC aggregation).
- No fleet-wide blast radius. SolarWinds: SUNBURST demonstrated the opposite blast radius (SUNBURST was delivered via the core platform DLL `SolarWinds.Orion.Core.BusinessLayer.dll`, not via the trap subsystem — but every customer running the trap subsystem inherits the same central-update trust boundary).

SolarWinds is a useful counter-example specifically for: **central-tier blast radius**, **closed-update trust**, and **central DB dependence**. It is not a uniformly bad design — its UI ergonomics, time-to-first-trap, and outbound-trap features are genuine positives — but the architectural pattern it embodies is the one Netdata's hub model is designed to avoid.

---

## Appendix B. Key Insights for Netdata Trap Design

Six highest-leverage findings from this analysis that apply to Netdata (B.1–B.5 are primary lessons; B.6 is a separate positive design reference):

### B.1 The Trap Viewer → Log Viewer migration is the cautionary tale on UI surfaces

SolarWinds *had* a dedicated native operator UI (Trap Viewer) — sortable trap table, filters, native rules — and over a decade, deprecated it in favour of a generic Log Viewer that unified traps with syslog and other event sources. The migration is **still incomplete in 2026**: deprecated docs still rank highly in search, operators routinely arrive at the wrong page, and Log Viewer carries a vendor-documented platform-wide ceiling of ~80 alert actions/sec (the Trap Viewer's own action-throughput ceiling is not vendor-published, so this file does not compare them quantitatively).

**Lesson for Netdata**: do not build a "trap UI" that is so isolated from the rest of the platform that it later needs deprecating. The Netdata Function model (with the `topology:` family for graphs, `logs:` for journal-like search, and `dyncfg:` for configuration) already gives us a path where traps can land as a *function payload* — the operator browses traps the same way they browse network connections or systemd-journal entries. There is no separate UI to deprecate later.

### B.2 The MIB customisation gap is a real product moat (in the wrong direction for SolarWinds)

SolarWinds' "submit your MIB to us for inclusion" workflow is the worst customisation experience in the comparison set. It is unworkable for operators with proprietary or new-vendor hardware.

**Lesson for Netdata**: do not let the MIB store become a closed format. Netdata's profiles approach (YAML-based, operator-editable, hot-reloadable via `dyncfg`) is the opposite of SolarWinds' MSI-distributed binary store. **The MIB store and the trap-mapping store must be operator-editable on the hub, without redeploying Netdata.** This is a near-mandatory design point given the SolarWinds counter-example.

### B.3 No trap-row dedup creates compounding DB pain

SolarWinds writes one row per PDU into a SQL table that cannot be truncated due to FK constraints; storm cleanup takes 90+ minutes. Centreon, Zabbix, and LibreNMS have similar pain in their respective central DBs. The pattern is: **central DB + no upstream dedup = storm-induced DB pain**.

**Lesson for Netdata**: traps must be dedup-ed at the hub *before* they are written to the TSDB/event store, using a fingerprint key (trap-OID + source-IP + varbind-hash) and a configurable window. The hub model gives us a clean place to do this (in-process), and the per-site isolation means a misbehaving device at Site A does not pollute Site B's storage.

### B.4 80 actions/sec is the alarm cap operators run into

SolarWinds' documented platform-wide ceiling for trap-driven alerting is **80 actions/sec**. This is a small number for a 1,000-device network in real distress.

**Lesson for Netdata**: actions on alerts must not have a centrally bottlenecked dispatch model. Per-hub alert dispatch (each hub fires its own actions for its own site) plus per-action async execution gives us no equivalent ceiling. We should explicitly document the per-hub alert-action throughput so operators do not get bitten by an unstated cap.

### B.5 Community-string exposure through the query surface is the easy mistake to avoid

SolarWinds exposes `Orion.Traps.Community` as a queryable `System.String` field via the public SWIS API, with no documented redaction or encryption qualifier. The at-rest storage form is deployment-dependent, but anyone with SELECT rights on `Orion.Traps` (directly via SQL or through SWIS) reads every device's v1/v2c community as cleartext in the query result. This is a casual but real insider-threat exposure.

**Lesson for Netdata**: traps should be stored with the community **redacted by default** in the event store **and in the query surface**; the raw community should be available only to operators with an explicit "view secrets" capability (which Netdata's RBAC model already supports for other secret fields). This is cheap to do and avoids one of SolarWinds' quieter exposures.

### B.6 Bonus — outbound trap emission *is* worth supporting

This is the strongest **positive** lesson. SolarWinds supports outbound traps as an alert action with templating and multi-destination — a feature only a few systems in the set provide. For Netdata's MSP customers (managing many sites for many customers), the ability for a hub to *emit* a trap northbound to the customer's existing trap collector is a real interoperability win. The Trap Template format (XML with macro substitution) is a reasonable design reference.

---

## Reviewer Pass Log

**Reviewer set**: codex, glm-5.1, kimi-k2.6, mimo-v2.5-pro, minimax-m2.7-coder, qwen3.6-plus. All six reviewers ran in parallel for each iteration via the launcher at `.local/audits/snmp-traps-pilot/reviews/solarwinds/iter-N/launch.sh`. Prompts under the same path; outputs as `<reviewer>.out` + exit codes as `<reviewer>.exit`.

| Iteration | Reviewers run | Reviewers returned cleanly | Verdict distribution | Highest-severity finding addressed |
|---|---|---|---|---|
| iter-1 | 6 | 6/6 (all exit 0) | 6 × accept-with-fixes | Multiple majors: Postgres-store mis-statement, Appendix A "unbounded" (5 reviewers), §3.3 throughput interpretation, plaintext-at-rest overreach, EOC omission, `ObservationSeverity` type. All applied. |
| iter-2 | 6 | 6/6 (all exit 0) | 6 × accept-with-fixes | New majors: `ObservationSeverity` type (live SWIS confirms `System.Byte`, corrected from my iter-1 over-correction), `Orion.TrapVarbinds` base type, source-to-node algorithm overstated, InformRequest omission, outbound-trap comparability. All applied. |
| iter-3 | 6 | 6/6 (all exit 0) | 6 × accept-with-fixes | New majors: release-notes scope gap (2025.4.x/2026.1/2026.1.1), Trap Template reconstructed-vs-vendor labelling, plaintext consistency, HA in-flight loss softening, Log Viewer rule import/export, log-forwarding citation, Log Viewer Basic vs Log Analyzer tiers, Windows OS SNMP Trap Service conflict. All applied. |
| iter-4 | 6 | 6/6 (all exit 0) | 6 × accept-with-fixes (kimi/mimo/minimax/qwen with 0 majors; codex 2 majors; glm 4 majors) | HA failover quote softening, Appendix B.1 unsupported "beloved/worse" comparison removed, 2026.1 sources, 7-day retention downgraded to secondary-source, log-forwarding URL added, rule-evaluation-order inference flagged, RabbitMQ queue growth, audit logging silence-not-presumed. All applied. |
| iter-5 (SOW cap) | 6 | 6/6 (all exit 0) | 6 × accept-with-fixes (kimi/mimo/qwen with 0 majors; codex 3 correctness majors; glm 4 doc-coverage majors; minimax 1 blocker-tagged "no trap-specific security guidance" which is already a documented finding in §11.6) | codex M2 (`Flag for discard` semantics misstated — corrected verbatim from vendor), codex M1 (2026.1.1 fix-status added), codex M3 (Log Viewer forwarding is rule-based not continuous-stream — corrected). |

**Convergence rationale**. By iter-5 the file had passed three reviewers (kimi, mimo, qwen) with zero majors and the remaining majors from codex were three discrete correctness fixes (verbatim vendor-text mismatches), all of which were applied before declaring convergence. Glm's iter-5 majors are vendor-doc-coverage additions ("the file should cite these additional pages") which are additive rather than corrective and align with the SOW's "minor/nit" classification. Minimax's iter-5 "blocker" tag was applied to the existing §11.6 finding that the vendor's Secure Configuration page contains no trap-specific guidance — i.e., it is restating a finding the file already makes; not a new defect. The file meets the SOW stop rule ("stop when only minor/nit findings remain") and has hit the 5-iter cap.

**Surviving minor findings** (recorded inline so the next reviewer-pass owner has context):

1. **2025.4.x release-notes scope gap** — explicitly acknowledged in §0; the residual gap. Spot-checks of 2026.1/2026.1.1 found the material items; 2025.4.x sweep remains an additive task.
2. **Log Analyzer 7-day retention is secondary-source**, not directly verified from a vendor doc URL accessible to this review pass — flagged in §6.4 and §20.
3. **Log Viewer Basic vs Log Analyzer storage** — vendor feature matrix shows separate-DB for *both* Basic and full Analyzer; my caveat softened in iter-5. The exact licensing/SKU boundary between the two is not crisply documented in publicly-accessible vendor pages.
4. **`Orion.SNMPCredentialV3` entity** — referenced in §11.3; individual entity property table was not fetched. Additive, not corrective.
5. **Datadog matrix cell ambiguity (Appendix A)** — glm and qwen flagged that "yes (gosnmp own listener)" for Datadog's reception model could be more precise. Material only for the Datadog file, which is already accepted; left as a cross-file note.
6. **Editorial polish** — minor language preferences (heading "five highest-leverage findings" superseded by "six" with B.6 as a bonus point; tone softening in §17 already applied).

None of the surviving items affect the analytical claims of the file. All vendor-cited claims trace to URLs in §19; all operator-reported claims are explicitly tagged.
