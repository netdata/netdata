# Netdata SNMP Hub Architecture — Distributed-by-Design Scalability

## Purpose

Document the architectural principle that shapes Netdata's SNMP support — including the trap subsystem under design. This is **not a limitation** of Netdata; it is the deliberate architectural choice that gives Netdata unbounded horizontal scalability for SNMP infrastructure observability. The comparative analysis of other monitoring systems must be read with this lens — most of them are central-correlation architectures with vertical-scale ceilings; Netdata is not.

Scope note: this document includes target-state hub architecture and correlation
principles. Public end-user documentation MUST use implementation evidence for
shipped trap behavior and MUST NOT treat target-state topology, flow, or
cross-signal correlation language in this document as shipped trap behavior.

## The principle

> **Every site appoints one Netdata Agent as its "SNMP hub." That hub consolidates all SNMP-domain functions for the site. Hubs are independent of each other. Scale is achieved by adding more sites, not by scaling a central tier.**

## What each SNMP hub contains

A site's SNMP hub is **one Netdata Agent process** running on a host within the site. It serves the entire SNMP observability surface for that site:

| Function | Description |
|---|---|
| **SNMP polling** | Scheduled SNMP GET/WALK/GETBULK against site devices; produces metrics, charts, profile-derived dashboards. |
| **SNMP service discovery** | Auto-detection of SNMP-enabled devices on the site's network ranges. |
| **SNMP topology** | LLDP/CDP/FDB/ARP/STP/VLAN discovery and the L2/L3 graph for the site. |
| **NetFlow / sFlow / IPFIX** | Optional. If the site collects flow telemetry from its routers/switches, the hub receives it on the relevant UDP ports. |
| **SNMP traps** | UDP/162 listener that accepts traps from the site's devices, decodes them, and integrates them locally with the metrics/topology already on the same hub. |
| **Syslog / device logs** | The hub can also ingest syslog from the same devices, giving the site one place where every device-originated signal lives. |

All of these functions are **co-located on the same Netdata Agent**. They share its in-process memory, its TSDB, its log indexer, its topology graph, and — crucially — its event timeline. This co-location is what enables correlation **inside the hub**: an SNMP trap arriving from a device can be annotated against that device's metric history, its topology position, its log stream, and its NetFlow records — all without leaving the hub process.

## Why this matters: correlation lives where the data lives

Across hubs, there is **no central database**. There is no shared correlation engine. There is no global event bus that all hubs feed into and that runs cross-site rules.

This is intentional. Correlation in observability is **latency-sensitive** and **cardinality-explosive**: the natural place for it is right next to the data. A trap arriving at Site A's hub is correlated against Site A's metrics in the same process, in microseconds. Trying to ship every site's traps to a central correlation engine — the OpenNMS model, the Centreon central-broker model, the SolarWinds model — creates four scaling problems:

1. **Bandwidth tax**: every trap, every metric, every flow shipped over WAN.
2. **Single-tier failure**: the central correlation engine becomes a fleet-wide blast radius.
3. **Cardinality ceiling**: the central database's index, queue, and rule engine become the system bottleneck.
4. **Operational coupling**: an upgrade to the central tier blocks rollouts at every site.

Netdata's distributed-hub model avoids all four. Each hub does its own correlation against its own local data. Adding a thousand more sites adds a thousand more hubs and a thousand more **independent** correlation engines. There is no central tier to overload.

## Netdata Cloud's role: presentation, not correlation

Netdata Cloud aggregates **for presentation**:

- Metrics from all hubs are queryable in one UI; queries fan out to the relevant agents and the agent-side TSDBs answer.
- Topology graphs from multiple hubs can be displayed together (e.g. a global view of all sites' L2 graphs).
- Logs from multiple hubs appear in one log explorer.
- Traps (once supported) will surface in one event view.

What Cloud does NOT do:

- It does NOT execute correlation rules across hubs.
- It does NOT maintain a fleet-wide event store that must be sized to hold every trap ever received.
- It does NOT serialise trap dedup through a central pipeline.

This separation — agent-side correlation, cloud-side presentation — is the same pattern Netdata uses for metrics today. Trap support inherits the same model: traps correlate locally on the hub, and Cloud surfaces them globally.

## Implication for the trap subsystem design

A Netdata trap listener must:

1. **Live on the hub agent**, not as a separate centralised receiver.
2. **Share the agent's data path** — the trap pipeline, the polling-derived device map, the topology cache, the alert engine, and the TSDB all run in the same process.
3. **Be self-sufficient per site** — bundled MIBs/profiles, dedup state, severity normalization rules, and storage all live on the hub.
4. **Refuse to depend on cross-hub state** — no design that "needs all hubs to agree on a trap" is acceptable.

This is **directly opposite** to systems like OpenNMS Minion (which ships traps from the remote site back to a central core for processing). The Netdata equivalent is the reverse: the hub IS the trap processor for its site.

## Implication for the comparative analysis

Read each system spec under `.agents/sow/specs/snmp-traps/` with the question: **what does this system assume about centralization, and how does that assumption hold up at network-of-networks scale?**

Tentative pattern from the systems analysed so far:

| System | Centralization model | Per-site cost of a new device | Hub-style equivalent? |
|---|---|---|---|
| OpenNMS | Core JVM + optional Minions (which still ship traps to core) | scales until the core PostgreSQL/JVM saturates | Minion is **not** a hub — it forwards traps back to the central core where matching, dedup, alarm correlation all happen. |
| Centreon | Central broker + remote pollers; pollers run snmptrapd locally but the catalogue and broker live centrally | scales until the central MariaDB and broker saturate | Remote pollers run `centreontrapd` locally and DO process traps on-site (good — closer to the hub model) but the catalogue and Gorgone-bus are still central. |
| Zenoss | Central event server (ZEP) + remote Collectors that run `zentrap` per site | scales until ZEP's MariaDB / Lucene / Redis saturate | Collectors are **partial** hubs (they receive traps locally) but events are pushed to the central ZEP for storage, indexing, and de-dup. |
| Zabbix | Central server + optional proxies | central server is the bottleneck | proxies relieve poller load but trap data still flows centrally for storage + history. |
| LibreNMS | Central web/poller (can scale to a polling cluster) + central MariaDB | central DB is the bottleneck | no native hub equivalent. |
| Nagios + SNMPTT/NSTI | Central Nagios + local SNMPTT translators | Nagios passive-check submission centralised | SNMPTT runs locally but Nagios's passive-check pipeline is central. |
| Netdata (target architecture) | N independent hubs, no central correlation | adding a site adds an independent hub | **YES** — this is the model. |

Every system except Netdata has at least one centralised tier (database, broker, event engine) that becomes the scale ceiling. Netdata has none.

## The user value of this architecture

For a customer operating a network-of-networks (e.g., an MSP with 200 customer sites, or an enterprise with 50 datacentres):

- **Site isolation**: a misbehaving device at Site A cannot drown the trap pipeline at Site B. They are physically separate Netdata processes on separate hosts.
- **Onboarding is local**: adding Site 201 means deploying one new Netdata Agent and configuring its profiles. No central capacity planning. No central queue tuning.
- **Failure radius is per-site**: if Site A's hub goes down, Site B's hub keeps working. The operator gets a "Site A hub down" alert (via the same agent-health mechanism that already exists in Netdata) and addresses it locally.
- **Upgrade is per-site**: hubs can be upgraded one site at a time.
- **Operator agency**: each site's operator owns their hub. They can tune severity normalization, dedup windows, MIB coverage, alert routing per their site's character.
- **Cloud-side global view is additive**: Cloud aggregates hubs for visibility but never gates them. If Cloud is offline, every hub continues to receive, correlate, and alert.

## Implication for the eventual Netdata trap design discussion

When we synthesize `research/comparison/netdata-design-implications.md` after all 16 systems are analysed, the hub model is **the load-bearing design constraint** — every design decision must be evaluated against it:

- Where does the trap listener bind UDP/162? **On the hub.**
- Where does the MIB store live? **On the hub.**
- Where does dedup happen? **On the hub.**
- Where does severity normalization live? **On the hub.**
- Where does trap-driven topology update execute? **On the hub.**
- Where do operators write custom rules? **On the hub.**
- What does Cloud do with traps? **Aggregate for presentation; not for correlation.**

A design that requires any of these to live anywhere other than "on the hub" is incompatible with Netdata's scalability story and should be rejected.

## Operator-facing documentation implication

The end-user/operator docs (under `docs/`) for SNMP support will need to clearly tell operators: **"For each site you operate, appoint one Netdata Agent to be the SNMP hub. Run the SNMP polling, discovery, topology, traps, NetFlow, and syslog ingestion on that single agent."** Operators familiar with central-NMS designs (OpenNMS, Zabbix, etc.) will instinctively look for "where do I configure my central trap collector" — the answer is "you don't; each site has its own."

End of document.
