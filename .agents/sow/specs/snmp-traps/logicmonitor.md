# LogicMonitor — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: LogicMonitor (LM Envision SaaS). Closed-source, proprietary, hybrid SaaS observability platform. The on-prem footprint relevant to SNMP traps is the **Collector** (a Java-based, multi-protocol data-collection daemon installed on customer infrastructure). Traps are received locally by the Collector via UDP/162 (default), parsed using either a **LogSource** (the currently recommended path; traps are ingested as logs into LM Logs with out-of-the-box MIB-driven OID translation) or an **EventSource** (the older path; trap matching is Groovy-driven and produces events on a device). The two paths are mutually exclusive per Collector–device combination: "At a time, either LogSource or EventSource is used and under any scenario, both cannot be used simultaneously" (`logicmonitor.com/support/snmp-trap-logsource-configuration`). Traps are forwarded to the SaaS tier and routed through SaaS-side **Alert Rules** and **Escalation Chains** to notification endpoints (PagerDuty, ServiceNow, Slack, Microsoft Teams, OpsGenie, custom webhooks). The LogSource path ships with the **MIBs to JSON Converter Utility** (EA Collector 35.400+; Python 3.8–3.12) for converting custom MIBs to a JSON representation the Collector consumes at runtime.
- **Source evidence**: docs-only. LogicMonitor publishes no source code for the Collector, EventSources, Alert Rules, or Escalation Chains. Every architectural claim in this document traces to a vendor URL (`logicmonitor.com/support/`, `logicmonitor.com/blog/`, `logicmonitor.com/resources/`, `community.logicmonitor.com/`) or to the vendor's public LogicModule exchange. There is no `file:line` evidence; reviewers must not demand it for this system.
- **Citation convention**: vendor URL plus a verbatim quote (1-2 sentences max) of the cited text. Where docs and community guidance disagree, the docs page is treated as authoritative; community-thread material is labelled as such.
- **Closed-source unknowns disclaimer**: this document marks every inference clearly. The following surfaces are NOT publicly documented and any claim about them is labelled inference, low-confidence, or "not publicly specified": (a) the exact Java SNMP library used inside the Collector (SNMP4J is heavily inferred from community Groovy snippets but never declared as a vendor commitment); (b) the on-disk credential storage format on the Collector; (c) the Collector-to-SaaS-cluster wire protocol details; (d) Collector behaviour under sustained trap overload (queue model, drop policy); (e) internal trap-pipeline test coverage; (f) the precise bundled EventSource catalogue size or vendor breadth; (g) any per-trap dedup engine internal to the Collector. Reviewers should not treat any of these as facts.
- **Repository root analysed**: n/a — no public repository. The LogicModule exchange (`exchange.logicmonitor.com`) is a vendor-curated catalogue of installable modules; individual modules are inspectable in customer tenants but the catalogue is not a public source repo.
- **Snapshot date**: 2026-05-23. LogicMonitor docs site revisions are not pinned by URL; conclusions here are stable across the cited URLs as long as LogicMonitor does not retitle the Collector or restructure the EventSource family.
- **Surface analysed**: LM Envision SaaS only. LogicMonitor does not publicly document a separate "Managed" / customer-hosted control plane variant analogous to Dynatrace Managed. All findings refer to **LogicMonitor LM Envision SaaS + on-prem Collector (post-rebrand from "LogicMonitor Platform" to "LM Envision")**.
- **Author**: assistant
- **Reviewer pass**: 2 iterations, converged on accept-with-fixes by 5/6 reviewers. See the Reviewer Pass Log appended to this file.
- **Convergence outcome**: accept-with-fixes. The remaining open findings are minor / nit-level (citation polish, terminology precision, schema-table consistency on closed-source paths). No reviewer flagged a substantive accuracy blocker after iteration 2.

### 0.1 Brutal-honesty preface

LogicMonitor positions itself as a **hybrid observability platform** — agentless polling-first (SNMP, WMI, JDBC, JMX, SSH, REST, scripts) augmented with event ingestion (syslog, SNMP traps, Windows Event Logs, custom webhooks). **SNMP trap support has two distinct paths that have evolved over the product's lifetime**:

1. **EventSource (legacy / classic path)**. SNMP Trap-type EventSources are Groovy-filtered, severity-fixed-at-definition, and become *events* on the matched device. The vendor docs now openly state: "we recommend that you use SNMP Trap LogSource for ingesting SNMP messages due to limitations of EventSource" (`logicmonitor.com/support/snmp-trap-logsource-configuration`). The single most consequential EventSource limitation, called out by the vendor, is that "Duplicate alerts for SNMP Trap EventSources are never suppressed."
2. **LogSource (the modern recommended path)**. SNMP Traps LogSource ingests traps into **LM Logs** with **out-of-the-box MIB-driven OID translation**, supports **log processing pipelines** (filter / mutate / enrich), and can emit log alerts via **LogAlert** conditions. Since EA Collector 35.400, a **MIBs to JSON Converter Utility** (Python 3.8–3.12, shipped in `[Collector]/bin/snmpMibsToJsonConversionUtil`) lets operators convert custom MIBs to JSON for the Collector to consume.

Operators have to choose one of the two paths per device; this is a real architectural fork. Three honest observations up-front:

1. **The Collector is a Java daemon**. It ships in multiple sizes (small / medium / large / extra-large / double-extra-large / triple-extra-large) with documented JVM heap (small ≈ 2 GB, extra-large ≈ 16 GB, double-extra-large ≈ 32 GB; `logicmonitor.com/support/collector-capacity`). This is heavier than the Datadog Agent (Go binary) or Telegraf (Go), comparable in scale to the OpenNMS Java stack. The Groovy SNMP API package `com.santaba.agent.groovyapi.snmp` (`Snmp.html`) confirms the Java root package; the underlying SNMP library implementation is not vendor-disclosed, though community Groovy snippets reference SNMP4J (`org.snmp4j.*`) classes.
2. **The trap-receive subsystem is shared between LogSource and EventSource paths**. Both bind UDP/162 through the same Collector listener (`eventcollector.snmptrap.address=udp:0.0.0.0/162`, threadpool `eventcollector.snmptrap.threadpool=10`). The selection between LogSource and EventSource happens *per device* downstream of the listener: "If LogSource is not applied on all the devices monitored by a Collector, but EventSource is applied on them, the Collector processes the SNMP traps from devices on which EventSource is applied" (`logicmonitor.com/support/logicmodules/eventsources/types-of-events/snmp-trap-monitoring`).
3. **The SaaS tier owns the workflow.** Alert Rules + Escalation Chains live in the SaaS, not on the Collector. Operators cannot author or test the full alert pipeline locally; every change loop crosses the WAN. The Collector itself does **not** retain alert state or correlate alerts; it only forwards traps (as events or as logs). This is structurally similar to Datadog (per `datadog-agent.md` §1: traps are forwarded as logs and end up in the SaaS Logs Explorer) and Dynatrace (per `dynatrace.md` §1: traps become Grail log events, Davis problems via log event rules). The difference is the workflow-engine vocabulary: Datadog talks "Logs Monitor", Dynatrace talks "log event rule + Davis problem", LogicMonitor talks "Alert Rule + Escalation Chain". Same SaaS-side promotion pattern; different surface vocabulary.

What LogicMonitor does **better than** Datadog/Dynatrace at the trap layer (LogSource path):

- **Out-of-the-box MIB translation** for a documented vendor-broad list (see §4.2 and §13.1). The vendor integrations page states LogSource provides "out-of-the-box coverage for standard MIBs" (`logicmonitor.com/integrations/snmp`). Datadog and Dynatrace require operator-side MIB / OID work (Datadog `traps_db/*.json`, Dynatrace `mib-files-custom/` only for custom extensions).
- **MIBs to JSON Converter Utility** (Python, ships in the Collector) for custom MIBs — a real first-class custom-MIB tool, not a generic file drop directory.
- **Automatic alert clear via paired clear traps**: "Automatically close alerts when a related 'clear' trap comes in" (`logicmonitor.com/integrations/snmp`). On the LogSource path this is built-in; on the EventSource path the vendor explicitly admits dedup is absent.
- **Trap-as-log first-class citizenship**: traps live in LM Logs alongside syslog and other log sources, queryable via the same query language, addressable by log processing pipelines, and subject to LogAlert conditions.

What LogicMonitor does **worse than** OpenNMS/Zenoss/Centreon at the trap layer:

- **No native northbound SNMP trap forwarding**: cannot re-emit traps to upstream NMSes. Operators must use HTTP webhook integrations as a workaround.
- **No native trap-to-topology suppression**: the SaaS Topology Mapping feature exists (driven by LLDP/CDP/BGP/OSPF/EIGRP TopologySources) but does not natively suppress trap-derived alerts when an upstream topology node is down.
- **No native INFORM-PDU acknowledgement path**: the docs nowhere mention INFORMs; inferred not supported.
- **SNMP traps are EXCLUDED from Auto-Balanced Collector Groups (ABCG)**: "Protocols such as Syslog, SNMP traps, and NetFlow are excluded from auto-balancing" (`logicmonitor.com/support/auto-balanced-collector-groups`). Trap-receiving Collectors require manual placement and operator-managed device-destination configuration.
- **EventSource dedup admission**: the vendor's own docs disqualify EventSource for any high-volume trap fleet by stating duplicate alerts are never suppressed.

## 1. System Overview & Lineage

LogicMonitor is a commercial closed-source SaaS infrastructure-monitoring platform founded in 2007 (Santa Barbara, California), now part of the broader LM Envision rebrand covering infrastructure monitoring, logs, APM, and AIOps. Its on-prem footprint on customer premises is a single component:

- **Collector** — a Java daemon installed on dedicated Linux or Windows hosts. The Collector executes all polling, script execution, and event reception tasks for a "logical site" (one or more network segments / data centres / cloud regions). The Collector communicates outbound only — over HTTPS to the LogicMonitor SaaS portal — and does not require inbound connectivity from the SaaS side.

SNMP traps land on the Collector. There is no Cloud-side trap receiver; LogicMonitor does not publish a SaaS endpoint that accepts raw SNMP/UDP traffic.

### 1.1 Where SNMP traps fit in the broader product

- **One signal of many**, not a primary use case. LogicMonitor's bread-and-butter is polled metrics (SNMP GETs/walks, WMI queries, JMX, JDBC, REST, custom Groovy scripts). Traps now sit at the intersection of **LM Logs** (the recommended LogSource path) and the older **EventSource** corner (legacy path). Both paths anchor the resulting event/log to the source device entity, so a trap from `router-1.example.net` ends up associated with the LogicMonitor "device" entity `router-1.example.net`.
- **Two ingestion shapes, one listener**:
  - **LogSource shape**: traps become **logs** in LM Logs with structured JSON attributes (trap OID, source IP, varbind list, MIB-translated symbolic names). Pipelines, queries, LogAlert conditions, and AI features (Edwin AI, Log Anomaly Detection, Log Analysis) all consume these.
  - **EventSource shape**: traps become **events** on the device. The event carries severity (Critical / Error / Warning / Notice), subject (Groovy-templated), description (Groovy-templated), and a fixed EventSource identity.
- **Alerting is SaaS-side**. Both paths converge on the SaaS-side Alert Rule + Escalation Chain engine. LogSource alerts flow through **LogAlert** conditions; EventSource alerts flow through the older "EventSource alert" surface. Both are routed by Alert Rules.

### 1.2 Lineage and naming

- **"EventSource"** is LogicMonitor's older umbrella term for any event-producing data source on a Collector. Trap EventSources are a subtype. Other EventSource types include `Syslog`, `WindowsEventLog`, `Script Event`, and `IPMI`. The vendor now positions EventSource as legacy for SNMP traps, but EventSources for other event sources remain current.
- **"LogSource"** is the newer log-ingestion abstraction. A LogSource defines: what logs to collect, where to collect them, which fields to parse, and how to forward them to LM Logs. The SNMP Traps LogSource type is the current recommended path for trap ingestion.
- **"LogicModule"** is the catalogue concept — a unit of monitoring logic distributed via the LogicModule exchange. DataSources (polling), EventSources (events), LogSources (logs), JobMonitors, ConfigSources, PropertySources, AppliesTo functions, and TopologySources are all LogicModule subtypes.
- **"Alert Rule"** is the SaaS-side routing primitive that maps `(device-group glob, LogicModule glob, severity threshold, datapoint glob, instance glob)` → Escalation Chain.
- **"Escalation Chain"** is the ordered sequence of notification recipients (email, SMS, Slack, PagerDuty, ServiceNow, MS Teams, OpsGenie, custom HTTP) with sequenced rate/interval/repeat.

### 1.3 Relationship to upstream tools

- **`Net-SNMP` / `snmptrapd`**: NOT used as an external front-end. The Collector binds UDP/162 directly. If `snmptrapd` is bound to 162 on the same host, the Collector cannot bind and trap reception fails. Backup Collector context: "If you run a backup Collector, please make sure that UDP port 162 is open between your device and secondary Collector machine."
- **Java SNMP library**: not publicly disclosed. Community Groovy snippets reference SNMP4J classes (`org.snmp4j.*`); the Collector Groovy SNMP package is `com.santaba.agent.groovyapi.snmp` (`logicmonitor.com/support-files/javadocs/.../Snmp.html`). Treat SNMP4J as **plausible inference** rather than vendor-stated fact.
- **Groovy**: the language used for EventSource match expressions and for custom DataSource / PropertySource / ConfigSource scripts. The vendor states "Groovy is run entirely within the LogicMonitor Collector, guaranteeing it will run the same way across all collectors regardless of underlying OS platform and version" (`logicmonitor.com/support/logicmodules/datasources/groovy-support/advantages-of-using-groovy-in-logicmonitor`).
- **MIB libraries on the LogSource path**: the Collector ships with built-in OID translation for an enumerated vendor list — "out-of-the-box coverage for standard MIBs" — and supports custom MIBs via the **MIBs to JSON Converter Utility** (Python 3.8–3.12) that converts MIB files to JSON. "Starting with EA Collector 35.400 release, you can use the MIBs to JSON Converter Utility to convert the MIB files to JSON files. In a Collector, the utility is located at `[LogicMonitor Collector Directory]/bin/snmpMibsToJsonConversionUtil`." The Collector service user must have read access to the resulting JSON files. The "out-of-the-box" supported MIB list includes vendors such as Accedian Networks, ADTRAN, Alvarion, Appian Communications, Applied Innovation, and many more (vendor list is enumerated at `logicmonitor.com/support/supported-mibs-for-snmp-trap-translation`; counts not published).
- **MIB libraries on the EventSource path**: no MIB compiler is invoked. Operators hard-code OIDs in Groovy match conditions and templates; the Collector does no automatic OID-to-name resolution for EventSource matching.

### 1.4 Audience and licensing

- **Audience**: mid-market and enterprise IT operations teams who want a single SaaS pane of glass for infrastructure monitoring across on-prem, cloud, and hybrid environments. The trap subsystem specifically targets network operations teams who have switches, routers, firewalls, load balancers, and storage arrays that emit traps.
- **Licensing**: closed, commercial. LogicMonitor licences per-device (with tiering by device class) and per-cloud-resource. SNMP traps do not have a separate SKU — they are bundled into the device monitoring tier.

## 2. Trap-Subsystem Architecture

### 2.1 Components

| Component | Where it runs | Role for traps |
|---|---|---|
| **Collector** | Customer-managed Linux or Windows host (VM, bare metal, container) | Binds UDP/162; parses traps; dispatches to LogSource path (LM Logs) or EventSource path (events) per device; forwards over HTTPS to SaaS |
| **Trap listener subsystem** | In-Collector JVM thread pool (`eventcollector.snmptrap.threadpool=10` default) | Receives UDP packets, decodes BER, performs v3 USM verification, hands off to LogSource or EventSource processor |
| **MIBs to JSON Converter Utility** | Standalone Python tool in `[Collector]/bin/snmpMibsToJsonConversionUtil` (EA 35.400+) | Operator-run converter from MIB sources to JSON; output consumed by Collector at trap-reception time for OID translation |
| **EventSource modules** | Loaded into the Collector at startup (and on configuration update) | Each EventSource is a Groovy-filtered definition that matches traps by OID/varbind, sets severity, formats subject/description |
| **LogSource modules** | Loaded into the Collector at startup (and on configuration update) | LogSource definitions tell the Collector which traps to ingest as logs, what fields to parse, what enterprise OID scope (or any, on 35.400+) |
| **LogicMonitor SaaS portal** | LogicMonitor-hosted, multi-tenant | Stores event records and LM Logs entries; matches against Alert Rules / LogAlert conditions; triggers Escalation Chains; surfaces UI |
| **LM Logs (log processing pipelines)** | SaaS-side | Filter / mutate / enrich rules applied to ingested trap logs; LogAlert conditions emit alerts on pattern matches |
| **Alert Rule engine** | SaaS-side | Maps each alert (from event or LogAlert) to zero or more Alert Rules; each match routes to an Escalation Chain |
| **Escalation Chain engine** | SaaS-side | Sequenced notifications to PagerDuty, ServiceNow, Slack, Teams, email, SMS, custom HTTP |
| **LogicModule exchange** | LogicMonitor-hosted catalogue | Distribution channel for vendor-shipped EventSources, LogSources, DataSources, and other LogicModules |

### 2.2 ASCII diagram

```
                       CUSTOMER NETWORK (per Collector)
   +-------------------------------------------------------------------------+
   |                                                                         |
   |   Network devices (routers, switches, firewalls, load balancers,        |
   |   storage arrays, hypervisors)                                           |
   |          |                                                              |
   |          | SNMP trap PDU (UDP, default port 162)                        |
   |          v                                                              |
   |   +--------------------------------------------------------------+      |
   |   |  Collector (Linux or Windows, Java daemon)                   |      |
   |   |                                                              |      |
   |   |  +--------------------------------------------------------+  |      |
   |   |  | Trap listener (UDP/162 by default)                     |  |      |
   |   |  |   eventcollector.snmptrap.address=udp:0.0.0.0/162      |  |      |
   |   |  |   eventcollector.snmptrap.threadpool=10                |  |      |
   |   |  |   SNMP v1 / v2c / v3 (USM)                             |  |      |
   |   |  |   credential lookup: snmptrap.* -> snmp.* ->           |  |      |
   |   |  |     eventcollector.snmptrap.* (per-device first)       |  |      |
   |   |  |   source identification (EA 36.100+):                  |  |      |
   |   |  |     v1   -> agent-addr                                 |  |      |
   |   |  |     v2c  -> snmpTrapAddress varbind (1.3.6.1.6.3.18.1.3)|  |     |
   |   |  |     v3   -> snmpTrapAddress varbind                    |  |      |
   |   |  |     else -> socket peer                                |  |      |
   |   |  +--------------------------------------------------------+  |      |
   |   |                |                                             |      |
   |   |          per-device path selection                           |      |
   |   |          (LogSource and EventSource are mutually exclusive)  |      |
   |   |                |                                             |      |
   |   |       +--------+--------+                                    |      |
   |   |       v                 v                                    |      |
   |   |  +---------+      +-----------+                              |      |
   |   |  |LogSource|      |EventSource|                              |      |
   |   |  |(modern, |      |(legacy)   |                              |      |
   |   |  |recommend|      |Groovy     |                              |      |
   |   |  |ed)      |      |filters +  |                              |      |
   |   |  |MIB OID  |      |severity   |                              |      |
   |   |  |translate|      |+ subject  |                              |      |
   |   |  |+ JSON   |      |+ desc     |                              |      |
   |   |  |fields   |      |NO DEDUP   |                              |      |
   |   |  +----+----+      +-----+-----+                              |      |
   |   |       |                 |                                    |      |
   |   +--------------------------------------------------------------+      |
   |           |                 |                                           |
   +-----------|-----------------|-------------------------------------------+
               |                 |
               | HTTPS (mTLS) outbound to portal
               v                 v
   +-----------+-----+    +------+----------------+
   | LM Logs ingest  |    | Per-device event log  |
   |  log.source:    |    |  {severity, subject,  |
   |   snmptrap      |    |   description,        |
   |  + MIB-translated|   |   eventsource_id}     |
   |   varbinds      |    |                       |
   +-----------+-----+    +------+----------------+
               |                 |
               | LogAlert        | EventSource alerting
               | conditions      |
               v                 v
   +-----------+-----------------+-----+
   | Alert Rule engine (SaaS)         |
   |   priority-ordered, first match  |
   |   filters by sev / group / module|
   |   -> Escalation Chain            |
   +----------------+-----------------+
                    |
                    v
   +----------------+-----------------+
   | Escalation Chain (SaaS)          |
   |   PagerDuty / ServiceNow /       |
   |   Slack / Teams / OpsGenie /     |
   |   email / SMS / custom HTTP      |
   +----------------------------------+

         SaaS-side context:
           - Edwin AI / Log Anomaly Detection / Log Analysis (LM Logs path)
           - SmartScape-equivalent: resource graph + TopologySources (LLDP/CDP/BGP/OSPF/EIGRP)
           - LogicModule exchange (vendor catalogue: DataSources, EventSources, LogSources, TopologySources)
```

### 2.3 Deployment model

- **Customer-side compute on Collector**. The Collector is a long-lived process installed by the customer on dedicated Linux or Windows hosts. The vendor describes Collectors as "not agents and do not have to be installed on every resource within your infrastructure. Rather, you should install a Collector on a host in each location of your infrastructure" (`logicmonitor.com/support/collectors/collector-overview/about-the-logicmonitor-collector`).
- **Collector sizing**. LogicMonitor publishes Collector sizes ranging from small (≈ 2 GB RAM) through extra-large (≈ 16 GB) and double-extra-large (≈ 32 GB), with documented RAM, CPU, and device-count guidance. The trap subsystem inherits the Collector sizing — there is no separate "trap-only" Collector sizing tier. The vendor publishes a per-size estimate for SNMP v2 / v3 trap LogSource capacity but the documented baseline assumption is conservative: "for the estimated collector performance numbers for SNMP v2 and v3 Trap LogSource, it is assumed that each device generates 10 traps per second" (`logicmonitor.com/support/collector-capacity`).
- **Collector HA via redundancy and Auto-Balanced Collector Groups (ABCG)**. LogicMonitor supports two HA models, but **SNMP traps are explicitly excluded from ABCG auto-balancing**:
  > "Protocols such as Syslog, SNMP traps, and NetFlow are excluded from auto-balancing."
  > — `logicmonitor.com/support/auto-balanced-collector-groups`
  Consequently:
  1. **Failover pairs** are the supported HA path for trap reception: "If you run a backup Collector, please make sure that UDP port 162 is open between your device and secondary Collector machine." Operators must configure devices to send traps to **both** Collector IPs. The vendor states that the active Collector reports the trap and the standby does not: "Only the Collector that is currently active for the device will report the trap." Whether the standby still receives and parses the trap (and silently discards) versus refuses to accept the UDP packet is not enumerated in public docs.
  2. **ABCG**, while not load-balancing trap reception, does support dynamic failover among Collectors in the group; trap-receiving LogSources require operator-managed device-destination configuration that targets stable Collector addresses outside the ABCG.
- **Container deployment**. LogicMonitor ships an official Collector container image. The Collector container is operator-configured; binding to UDP/162 requires either `--cap-add=NET_BIND_SERVICE`, running as root, or `iptables` redirection.
- **Kubernetes**. LogicMonitor publishes Helm charts for Collector deployment (multiple variants for the integration with Kubernetes Helm chart). Trap-receive port must be exposed via NodePort / hostPort / LoadBalancer (operator's responsibility).

### 2.4 Languages, libraries, and packaging

- **Java** is the Collector's host language. The Collector ships with a bundled JRE (LogicMonitor controls the JRE version to avoid customer JRE conflicts; documented at install time).
- **Groovy** is the EventSource scripting language. Groovy scripts execute inside the Collector JVM and have access to the standard Java SE APIs plus LogicMonitor's Groovy helpers.
- **SNMP4J** is the **inferred** Java SNMP library (community Groovy snippets reference `org.snmp4j.*` classes), but LogicMonitor does not officially commit to a specific library.
- **Packaging**: the Collector is distributed as a `.bin` installer (Linux) or `.exe` installer (Windows). LogicMonitor publishes a tenant-specific download URL containing pre-configured Collector credentials (account-bound).

### 2.5 Inter-component IPC

- **Device → Collector**: UDP/162 (configurable per Collector).
- **Collector → SaaS portal**: outbound HTTPS (TLS 1.2+) to `{tenant}.logicmonitor.com:443`. The Collector authenticates with a per-Collector credential issued at installation.
- **Collector internal**: the watchdog and agent processes communicate via OS process supervision (no IPC for trap data).
- **Collector local management**: a local config file (`agent.conf` on Linux, `%PROGRAMDATA%\LogicMonitor\Agent\conf\agent.conf` on Windows) holds Collector-side settings (bind interfaces, ports, JVM heap sizes).

## 3. Trap Reception (UDP/162 Ingress)

### 3.1 Listener implementation

The Collector binds the UDP socket itself; there is no `snmptrapd` front-end. The bind address and port are configurable in the Collector's `agent.conf` via the documented `eventcollector.snmptrap.*` parameter family. Vendor-documented defaults (from `logicmonitor.com/support/agent-conf-file-configurations` and surrounding docs):

```ini
# Default trap listener config (excerpt from agent.conf)
eventcollector.snmptrap.enable=true
eventcollector.snmptrap.threadpool=10
eventcollector.snmptrap.address=udp:0.0.0.0/162
```

The vendor verbatim positions this as: "the default listening SNMP trap port that the Collector uses can be changed." Operators verify and modify via "Settings > Collectors > Manage Collector > collector config" (the agent.conf editor in the UI).

For the LogSource path, an additional enablement gate exists:

> "To ingest SNMP traps received by the collector, enable the `lmlogs.snmptrap.enabled` property in the agent.conf settings."
> — `logicmonitor.com/support/snmp-traps-as-logs-overview` (LM Logs trap ingestion enablement)

Combined: the listener boots when `eventcollector.snmptrap.enable=true`; the LogSource ingestion path activates separately via `lmlogs.snmptrap.enabled`. EventSource traps flow regardless of the LogSource gate (subject to EventSource-vs-LogSource device exclusivity rule in §0.1).

### 3.2 Default port

162 (IANA assignment), encoded in `eventcollector.snmptrap.address=udp:0.0.0.0/162`. Configurable to any port; operators using non-root Collectors typically configure 1162 or 8162 and use `iptables` redirection or `setcap`.

### 3.3 SNMP version support

| Version | Authentication | Notes |
|---|---|---|
| v1 | community string only | Generic / enterprise / specific trap PDU model |
| v2c | community string only | `SNMPv2-Trap-PDU` |
| v3 | USM (NoAuthNoPriv / authNoPriv / authPriv) | Engine ID discovery handled by the Collector |

INFORM-PDU support is **not documented**. The LogicMonitor support pages and SNMP integrations docs do not mention INFORMs anywhere. Operators must treat INFORM support as unsupported / unverified: there is no vendor commitment that INFORMs are ingested, that an INFORM-Response PDU is emitted, or that an unacknowledged INFORM is retried by the trap-sending device with predictable Collector-side behaviour. This is a vendor documentation gap, not a wire-level capability claim.

SNMPv3 credential resolution order is documented and is one of the operationally important details (separate-credentials-for-traps-vs-polling is supported):

> "The collector first checks for the set of credential `snmptrap.*` in the host properties. If the `snmptrap.*` credentials are not defined, it looks for the set of `snmp.*` in the host properties. If sets for both `snmptrap.*` and `snmp.*` properties are not defined, it looks for the set `eventcollector.snmptrap.*` present in the agent.conf setting."

So **per-device** SNMPv3 trap credentials are configured as resource properties (`snmptrap.security`, `snmptrap.auth`, `snmptrap.authToken`, `snmptrap.priv`, `snmptrap.privToken`); the Collector falls back to `snmp.*` host properties and then to global `agent.conf` defaults. The vendor adds: "Starting with EA Collector 34.100, you can add `snmptrap.*` properties on resource/group level for the collector to decrypt the trap messages received from monitored devices" (`community.logicmonitor.com/blog/tech-talk/snmp-trap-credentials-on-resource-properties-enhancement/13096`).

This is **structurally cleaner than Datadog and Dynatrace**, both of which configure trap-receiver credentials globally on the Agent / ActiveGate. LogicMonitor's per-resource credential model means a single Collector can decrypt traps from devices using different SNMPv3 engine IDs, communities, and auth/priv tokens.

Global `agent.conf` SNMPv3 trap fallback parameters:

```ini
eventcollector.snmptrap.security=<usm-user>
eventcollector.snmptrap.auth=<auth-protocol>
eventcollector.snmptrap.auth.token=<auth-secret>
eventcollector.snmptrap.priv=<priv-protocol>
eventcollector.snmptrap.priv.token=<priv-secret>
```

### 3.4 Privileged-port handling

Two operator-side patterns:

1. **Run the Collector as root** (Linux). Historically common but discouraged for hardening.
2. **`setcap`** on the Collector JVM binary, granting `cap_net_bind_service=+ep`. Discussed in community forum threads.
3. **iptables PREROUTING REDIRECT** from UDP/162 to a high port (e.g. 1162). Operator-driven.

### 3.5 Concurrency and performance

Trap listener concurrency is documented in `agent.conf`: `eventcollector.snmptrap.threadpool=10` (default 10 worker threads).

LogicMonitor publishes a per-Collector-size SNMP trap LogSource device-count matrix on `logicmonitor.com/support/collector-capacity` with the explicit baseline assumption:

> "For the estimated collector performance numbers for SNMP v2 and v3 Trap LogSource, it is assumed that each device generates 10 traps per second."

Documented capacity figures (subset shown; reviewers should consult the live capacity page for the full matrix):

| Collector size | SNMP v2 Trap LogSource (standard devices) | SNMP v3 Trap LogSource (standard devices) | Implied peak traps/sec at 10 t/s/device |
|---|---|---|---|
| medium | 17 | 14 | 170 (v2) / 140 (v3) |
| large | 87 | 70 | 870 / 700 |
| extra-large (XL) | 140 | 112 | 1,400 / 1,120 |
| double-extra-large (XXL) | 245 | 196 | 2,450 / 1,960 |

The 10-traps-per-second-per-device assumption is the vendor's planning baseline; the actual sustained ceiling depends on trap PDU size, MIB-translation cost, downstream LM Logs ingest pressure, and competing polling workload on the same Collector. Operators should treat the implied peak as a planning number rather than a measured ceiling.

Comparative throughput note:
- **Dynatrace** publishes 17,000–150,000 traps/min depending on profile (per `dynatrace.md` §3.5).
- **Datadog** does not publish per-Agent trap throughput; gosnmp-bound (per `datadog-agent.md` §3).
- **LogicMonitor** publishes the 10-traps-per-second-per-device assumption baseline and a per-Collector-size device-count rating; the implied throughput is a function of how operators interpret the device count.

### 3.6 HA / clustering

- Failover pairs (§2.3): primary + standby, with active failover for monitoring (not active/active).
- ABCG (§2.3): pool with auto-balancing for devices; trap destinations must point to all members or a stable VIP.
- No documented anycast / leader-election story for the trap listener. Operators carry the burden of trap-routing during Collector failover.

## 4. MIB Management

### 4.1 The two-path summary

LogicMonitor's MIB story splits along the LogSource-vs-EventSource fork.

**LogSource path** (recommended, EA 35.400+):

The Collector ships with a **built-in MIB catalogue** that translates trap OIDs and varbind OIDs into human-readable symbolic names at trap-reception time:

> "The Collector can translate the SNMP traps using out-of-the-box MIBs."
> — `logicmonitor.com/support/supported-mibs-for-snmp-trap-translation`

> "The SNMP trap translation provides human readable values enclosed in parentheses immediately adjacent to the OIDs contained in the SNMP trap captured by LM Logs, and maps the human readable values as fields associated with the SNMP trap along with their corresponding values."
> — `logicmonitor.com/support/snmp-traps-as-logs-overview`

> "LogicMonitor pulls in critical information from SNMP Trap OIDs and variable bind values and translates them into user-readable values out of the box."
> — `logicmonitor.com/support/snmp-traps-as-logs-overview` (same page; OID translation behaviour)

For custom MIBs not in the bundled catalogue, LogicMonitor exposes **two** custom-MIB workflows; the second is the recommended modern path:

**Workflow A — Portal MIB upload (Collector 38.300+, recommended)** *[source: `logicmonitor.com/support/snmp-trap-mibs` — vendor's authoritative MIB management page. Specific claims below reflect vendor documentation; reviewers needing direct verbatim quotes should re-fetch the page when public WebFetch tooling renders the article body correctly]*:

The portal supports direct MIB file upload from the SaaS UI. Uploaded MIBs are pushed to all Collectors running Collector 38.300 or later, take precedence over locally JSON-converted MIBs in `/snmpdb/custom`, and over the bundled catalogue in `/snmpdb/core`. Uploaded MIBs are dependency-validated server-side, carry a status state (validation passed / failed), and cannot be deleted from the portal once uploaded (audit-trail / replication safety). RBAC governs who can upload.

**Workflow B — MIBs to JSON Converter Utility (EA 35.400+, older / local fallback)**:

> "Starting with EA Collector 35.400 release, you can use the MIBs to JSON Converter Utility to convert the MIB files to JSON files. In a Collector, the utility is located at `[LogicMonitor Collector Directory]/bin/snmpMibsToJsonConversionUtil`."
> — `logicmonitor.com/support/snmp-trap-mibs`

The utility itself runs as a Python script (vendor docs reference `snmpMibsToJsonConverter.py` inside the install directory). Operator workflow:

1. Install Python requirements: `pip install -r requirements.txt` (in the utility's directory).
2. Run `snmpMibsToJsonConverter.py`; the script prompts interactively for source MIB directory and destination JSON directory.
3. Copy the generated JSON files to `[Collector]/snmpdb/custom` if not already produced there.
4. Do NOT rename the generated JSON files; their filenames are used by the Collector for lookup.
5. Restart the Collector for changes to take effect.

Requirements: Python 3.8–3.12 on the operator's machine; read access to the source directory (custom MIBs + their parent/dependency MIBs); write access to the destination directory. "Ensure that the definition of a MIB file is not part of multiple input MIB files." After conversion, the Collector service user must have read access to the JSON files in `/snmpdb/custom` for OID translation to take effect.

**LogSource-path simplification on 35.400+**: "SNMP traps do not require EnterpriseOID when creating a LogSource, and the Collector can translate the SNMP traps using out-of-the-box MIBs." This eliminates the previous-version requirement to declare an enterprise OID scope per LogSource.

**MIB resolution precedence at trap-reception time** (Collector 38.300+):

1. Portal-uploaded MIBs (server-replicated, highest priority).
2. Locally JSON-converted MIBs in `[Collector]/snmpdb/custom`.
3. Bundled MIBs in `[Collector]/snmpdb/core`.

**EventSource path** (legacy):

EventSources do **not** invoke a MIB compiler at runtime. OID resolution is baked into the EventSource Groovy match expression and into the human-readable text the EventSource emits. Vendor-shipped EventSources do the OID work for the operator; custom EventSources require manual OID enumeration.

### 4.2 Bundled MIB coverage (LogSource path)

The `logicmonitor.com/support/supported-mibs-for-snmp-trap-translation` page enumerates a vendor-broad list. The vendor list begins with: "Accedian Networks, Inc. ... ADTRAN, Inc. ... Alvarion Ltd. Appian Communications, Inc. Applied Innovation Inc." — and continues for many additional vendors. The exact count and full vendor list is the docs page itself (not reproduced verbatim here because the WebFetch render returned navigation rather than the full alphabetical list).

For trap EventSources, the **LogicModule exchange** is a parallel curated catalogue. Browseable at `exchange.logicmonitor.com`, it contains EventSource-shaped trap definitions for common network and infrastructure vendors. The exact bundled-EventSource count is not publicly enumerated; the exchange browser is the source of truth.

Vendor breadth (drawn from public forum threads and exchange browser inspection):
- **Cisco**: IOS, IOS-XE, IOS-XR, NX-OS, ASA, Catalyst, Nexus, Meraki.
- **Juniper**: Junos (MX, EX, SRX series).
- **Arista**: EOS.
- **F5**: BIG-IP.
- **Palo Alto Networks**: PAN-OS.
- **Fortinet**: FortiOS.
- **HPE / Aruba**: ProCurve, ArubaOS, ClearPass.
- **NetApp**: ONTAP.
- **EMC / Dell**: storage, server.
- **VMware**: vSphere.
- **Accedian, ADTRAN, Alvarion, Appian, Applied Innovation, etc.** (long-tail from the supported-MIBs page).

### 4.3 User workflow for adding custom traps

**Path A — custom LogSource + JSON-converted MIB (recommended, 35.400+)**:

1. Acquire the vendor MIB file(s) for the device.
2. Run the MIBs to JSON Converter Utility:
   ```
   cd [LogicMonitor Collector Directory]/bin
   ./snmpMibsToJsonConversionUtil --source /path/to/mibs --destination /path/to/json
   ```
   (CLI flags inferred from the documented behaviour; exact flag names are not publicly enumerated.)
3. Ensure the Collector service user has read access to the destination JSON directory.
4. In the portal, author a SNMP Traps LogSource referencing the trap OIDs of interest. As of EA 35.400+, no EnterpriseOID declaration is needed; the Collector resolves via the loaded JSON.
5. Save. The LogSource is pushed to all Collectors in the tenant. Subsequent traps from devices in the LogSource's AppliesTo scope are ingested as logs with translated symbolic OID names.

**Path B — custom EventSource (legacy)**:

1. In the portal, navigate to **Settings → LogicModules → EventSources → New → SNMPTrap**.
2. Define the EventSource:
   - **Name**: human-readable identifier.
   - **AppliesTo**: a Groovy boolean expression scoping which devices this EventSource applies to (e.g. `system.sysinfo =~ ".*Cisco IOS.*"`).
   - **Filters**: "In the Filters area of an EventSource's configurations, you can specify a set of filters that will allow you to inclusively filter and select for particular SNMP traps to alert on." Filters reference trap OID and varbind values.
   - **Severity**: "In the Alert Settings area of an SNMP Trap's EventSource configurations, use the Severity field's dropdown to indicate the severity level that will be assigned to the alerts that are triggered by this EventSource." (Critical / Error / Warning / Notice / Info.)
   - **Subject** / **Description**: Groovy-templated message body.
3. Save. The EventSource is pushed to all Collectors and takes effect on the next definition update poll.

### 4.4 OID-to-name resolution

- **LogSource path**: yes, via the bundled MIB catalogue + JSON-converted custom MIBs.
- **EventSource path**: no runtime resolution; operator-authored Groovy supplies the human text.

### 4.5 Fallback behaviour for unknown OIDs

- **LogSource path**: when a LogSource matches a trap but the OID is unknown to the bundled MIB catalogue, the trap is still ingested as a log with the numeric OID preserved as a field. Unknown OIDs do not block ingestion.
- **EventSource path**: traps that match no EventSource filter are **dropped silently** (no event is created). This is a real operational risk on the EventSource path for greenfield deployments — operators must either:
  - Author a catch-all EventSource (a filter that matches any trap, emitting a Notice-severity event), accepting the noise; **or**
  - Use the Collector's trap debug logging to verify no unmatched traps are arriving.

Compare:
- **OpenNMS**: generates an `Unknown Trap` event for any unrecognised trap.
- **Zabbix**: `snmptrap.fallback` catches anything unmatched.
- **Zenoss**: unrecognised traps map to a generic event class.
- **LogicMonitor EventSource path**: silent drop (vendor weakness).
- **LogicMonitor LogSource path**: ingested as log with numeric OID (acceptable).

This is the cleanest argument in the docs for adopting the LogSource path over EventSource.

## 5. Trap Processing Pipeline

### 5.1 Parse and source verification

The Collector:

1. Receives UDP packet on the configured port (default 162; thread pool of 10 workers by default).
2. BER-decodes the PDU.
3. Verifies SNMPv3 authentication (USM) against credentials resolved via the `snmptrap.*` resource property → `snmp.*` resource property → `eventcollector.snmptrap.*` `agent.conf` fallback chain.
4. Identifies the source resource using the version-specific rules introduced in EA Collector 36.100 (see §5.3).
5. Dispatches to the LogSource path (LM Logs ingestion with MIB translation) or to the EventSource path (Groovy filter evaluation), per the device's configured path. The two paths are mutually exclusive per device.

### 5.2 OID-to-name resolution

- **LogSource path**: yes, MIB-driven. The Collector applies the bundled MIB catalogue + any portal-uploaded MIBs (Collector 38.300+) + any locally JSON-converted MIBs at trap-reception time, attaching symbolic OID names to the resulting log entry alongside the numeric OID. The vendor formulation: "The SNMP trap translation provides human readable values enclosed in parentheses immediately adjacent to the OIDs contained in the SNMP trap captured by LM Logs, and maps the human readable values as fields associated with the SNMP trap along with their corresponding values."
- **EventSource path**: no MIB-driven resolution at trap-reception time. The match expression in each EventSource defines its own OID literals; the message template defines the human text.

### 5.3 Source identification

LogicMonitor's source identification rules changed in **EA Collector 36.100** to use version-specific PDU fields rather than the UDP socket peer:

- **SNMPv1**: the PDU's `agent-addr` field is used to identify the source resource.
- **SNMPv2c and SNMPv3**: the `snmpTrapAddress` varbind (OID `1.3.6.1.6.3.18.1.3`) is used to identify the source resource.
- **Fallback**: if the version-specific identification field is missing or empty, the Collector falls back to the UDP socket peer address.
- **Device match**: the resolved source address is looked up against the Collector's resource table (devices in LogicMonitor's CMDB-like inventory). If a match is found, the resulting log or event is tagged with the LogicMonitor resource (device) ID.
- **No-match behaviour**: behaviour for traps from unmonitored source IPs is not crisply documented; operators should test empirically. Different vendor pages have given different impressions over time; the safest assumption is that LogSource ingests the trap as a log carrying the raw source address (with no resource binding), and EventSource drops the trap because the AppliesTo evaluation fails when no resource exists to evaluate it against.

This is **better than Datadog and Dynatrace**, both of which key trap source identification on the UDP socket peer. LogicMonitor's PDU-derived identification gracefully handles SNMP proxy / relay deployments where the UDP sender is not the originating device.

### 5.4 Enrichment

- Variable bindings (varbinds) are extracted from the PDU.
- Each varbind is accessible in the EventSource Groovy match expression as `varbinds.<oid_or_name>.value`.
- Device properties (LogicMonitor device tags, custom properties) are joinable in the Groovy template (e.g. `device.properties.location`).
- The Collector does **not** join against external topology, neighbours, or last-known-state from polled metrics.

### 5.5 Normalisation

- Severity comes from the EventSource definition (Critical / Error / Warning / Notice / No Data). Each EventSource fixes its own severity at definition time. This is **structurally better** than Dynatrace (hard-coded `loglevel: NONE`) and Datadog (severity from Logs Monitor matching, not from the trap).
- No unit normalisation. Varbinds are textual; type semantics are operator-managed.

### 5.6 Deduplication and suppression

This is the most important difference between the two paths.

**EventSource path — no dedup**:

The vendor explicitly states:

> "Duplicate alerts for SNMP Trap EventSources are never suppressed."
> — `logicmonitor.com/support/logicmodules/eventsources/types-of-events/snmp-trap-monitoring`

This is unconditional. Unlike Syslog and Windows Event Log EventSources (which support dedup), SNMP Trap EventSources emit one alert per incoming trap. There is no per-EventSource dedup key, no suppression window, no clear-pair logic. **This is the single largest reason the vendor now positions LogSource as the recommended path.**

The operational implication: any device emitting linkUp/linkDown flapping every 30 seconds will produce one event and one alert every 30 seconds on the EventSource path. Operators must work around this with Alert Rule frequency settings (downstream of event creation) or SDTs.

**LogSource path — vendor claims auto-clear and duplicate-record retention**:

The vendor's integrations page describes the LogSource model:

> "Automatically close alerts when a related 'clear' trap comes in"
> "Eliminate noisy alerts while retaining a record of the number of duplicate alerts"
> — `logicmonitor.com/integrations/snmp`

These are vendor capability claims. The exact configuration surface (clear-pair definition syntax, sliding window length, duplicate-counter field name) is not enumerated in a single publicly-cited docs page; reviewers needing precise configuration semantics should treat them as **vendor-stated capabilities** subject to in-product verification.

In broad comparison terms:
- **Better than Datadog and Dynatrace** at the SaaS-tier peer level — neither offers automatic clear-trap pairing at the collector or in the SaaS log layer.
- **Likely less expressive than OpenNMS's `<reductionKey/>` + `<auto-clean/>`** which support arbitrary clear-key expressions and multi-event correlation.
- **Comparable in intent to Centreon centreontrapd's dampening + clear semantics**, though Centreon's MIB-driven OID-to-event mapping table is more explicit and testable than LogicMonitor's pipeline + LogAlert mechanism.

### 5.7 Routing

Each matched trap → event record → forwarded to SaaS portal over HTTPS. On the SaaS side:

- The event is persisted in the per-device event log.
- The event is matched against all Alert Rules.
- Each matching Alert Rule creates an alert tied to its Escalation Chain.

### 5.8 Error handling

- Malformed PDU: dropped; Collector debug log records the failure.
- Unknown OID (no EventSource match): silently dropped (§4.5).
- USM auth failure: trap dropped; Collector debug log records.
- Source IP unrecognised: behaviour unclear; either dropped or event creates a synthetic device entry.
- HTTPS upload failure to SaaS: the Collector queues events locally with bounded queue depth and retries. Queue sizing and overflow drop policy details are not enumerated in public docs; reviewers should treat the specific "in-memory + disk-spill" mechanic as an inference, not vendor-stated.

## 6. Data Model & Persistent Storage

### 6.1 Per-feature storage matrix

| Feature | Storage | Engine | Retention | Schema |
|---|---|---|---|---|
| Trap event records (EventSource path) | SaaS portal **event store** (per-device events table) | LogicMonitor-managed (likely sharded RDBMS + secondary store; not publicly disclosed) | Tenant-contract-defined (publicly cited defaults are inference; reviewers should consult the tenant contract) | `{device_id, eventsource_id, severity, subject, description, varbinds, timestamp, status}` — no `dedup_key` because the EventSource path does not dedup; `status` covers open / acknowledged / cleared |
| Trap log entries (LogSource path) | SaaS portal **LM Logs** (Grail-like log store) | LogicMonitor-managed | Tenant-contract-defined retention | `{timestamp, log.source=snmptrap, snmp.version, snmp.trap_oid (numeric + symbolic), device.address, device_id, varbinds[{oid, type, value, symbolic_name}], ...}` |
| Alerts | SaaS portal **alert store** | LogicMonitor-managed | Default 12 months for alerts; precise number depends on tenant contract | `{alert_id, event_id, rule_id, escalation_chain_id, status, ack_user, ack_time, sdt_id}` |
| Device inventory | SaaS portal **device catalogue** | LogicMonitor-managed | Tenant-managed (devices persist until explicitly deleted) | `{device_id, hostname, ips, properties, group_membership, applies_to_evaluations}` |
| EventSource definitions | SaaS portal **LogicModule catalogue** | LogicMonitor-managed | Versioned per LogicModule version | `{eventsource_id, name, applies_to, match_groovy, severity, subject_groovy, description_groovy}` — note: SNMP Trap EventSources do not carry dedup or clear fields; vendor explicitly disclaims SNMP Trap EventSource dedup. Syslog and WindowsEventLog EventSource subtypes (out of scope here) do support dedup at the EventSource definition. |
| Collector local state | Collector host filesystem (`/usr/local/logicmonitor/agent/...`) | Files + embedded DB | Operator-managed (depends on Collector log rotation) | Mostly transient (logs, queues, cached EventSource definitions) |
| MIB definitions | **None at runtime** — see §4 | n/a | n/a | n/a |
| OID→event mapping | Embedded in EventSource Groovy definitions | LogicMonitor-managed | Versioned per LogicModule version | Groovy source |
| Dedup state (LogSource path) | LogAlert / LM Logs pipeline state in SaaS | SaaS-managed | Per LogAlert window | Not publicly enumerated |
| Dedup state (EventSource path) | None — vendor explicitly states "Duplicate alerts for SNMP Trap EventSources are never suppressed" | n/a | n/a | n/a |
| Suppression rules | Alert Rule SDTs (Scheduled Down Times); LogAlert conditions for LogSource path | SaaS | Per-rule | Rule-shaped |
| Audit log | SaaS portal **audit log** | LogicMonitor-managed | Default 90-180 days for tenant audit log; tenant-specific contract may extend | `{user, action, target, timestamp}` |

Caveat: storage retention numbers in this table are **inferences from community discussions and standard SaaS-tier defaults**, not vendor-quoted. Reviewers needing exact retention should consult the tenant contract.

### 6.2 Event schema (visible to operators)

The fields visible on each event in the LogicMonitor UI:

| Field | Source |
|---|---|
| **Severity** | EventSource definition |
| **Subject** | Groovy-templated from PDU varbinds |
| **Description** | Groovy-templated from PDU varbinds |
| **Host** | Device entity (from source-IP lookup) |
| **EventSource** | Catalogue lookup |
| **Timestamp** | Trap arrival time at Collector |
| **Acknowledged** | Operator action |
| **Note** | Operator-added free text |
| **In SDT** | Scheduled Down Time flag |
| **Cleared** | Set by matching clear-event Groovy |

### 6.3 Migration / upgrade handling

- EventSources are versioned per LogicModule version. Upgrading an EventSource (via the LogicModule exchange or via custom edit) is an atomic operation; existing events retain their original EventSource snapshot.
- Collector upgrades preserve queued events on disk; queue is drained against the new agent JVM.
- SaaS-side schema migrations are LogicMonitor-managed (invisible to tenants).

## 7. Configuration UX

### 7.1 Surfaces

- **Portal UI** (`{tenant}.logicmonitor.com`): browse and edit EventSources, Alert Rules, Escalation Chains, view event lists, view alert lists.
- **REST API** (`{tenant}.logicmonitor.com/santaba/rest/`): full programmatic access for IaC tools. The Terraform provider for LogicMonitor (community-maintained) wraps the REST API for `lm_eventsource`, `lm_alertrule`, `lm_escalationchain`, `lm_collector` resources.
- **Collector local config** (`agent.conf`): Collector-side settings (bind interfaces, ports, JVM heap, debug logging).
- **CLI**: `logicmonitor-collector-cli` — limited; mostly for installation, start/stop, version checks.

### 7.2 Defaults the operator sees

- **Port**: 162 on Linux Collectors run as root (`eventcollector.snmptrap.address=udp:0.0.0.0/162`); non-root Collectors must pick a non-privileged port.
- **LogSource path enablement**: `lmlogs.snmptrap.enabled=true` is the gate that allows traps to flow into LM Logs. Per the SNMP Traps Processing Preference order in §7.2.1, the flag acts as the *fallback* destination for traps that match no device-scoped or collector-scoped LogSource and no EventSource. Traps that match a device-scoped LogSource flow through that LogSource's pipeline (with the bundled MIB translation applied); traps that match only an EventSource flow through the EventSource path; traps that match neither and arrive while the flag is set are ingested into LM Logs as default-shape log entries. The flag does not override per-device exclusivity.
- **EventSources**: none active until the operator imports from the LogicModule exchange or authors custom EventSources.
- **EventSource severity dropdown**: Critical / Error / Warning / Notice / Info (defaults vary per definition; vendor-shipped definitions carry curated severity).
- **EventSource dedup**: **never applied — vendor explicitly states duplicate alerts are never suppressed on the EventSource path.**

### 7.2.1 SNMP Traps Processing Preference

When traps arrive at a Collector, the Collector applies a processing preference per device:

1. **Device-scoped LogSource** (LogSource AppliesTo evaluates true for the trap's source resource) — highest priority.
2. **Collector-scoped LogSource** (LogSource with collector-level AppliesTo, applied across the Collector) — second priority. Per the docs, this option is restricted: "in a LogSource, where the ApplyTo Collector(s) switch is enabled, to ensure smooth functioning, you must select Collectors that are not part of an Auto-Balanced Collector Group (ABCG)."
3. **EventSource match** (only consulted when no LogSource matches the source resource).
4. **`lmlogs.snmptrap.enabled=true` global ingestion** (fallback that pushes any unmatched traps into LM Logs).

Two implications:
- **LogSource and EventSource cannot both consume a given trap from a given device.** This is the architectural exclusivity rule cited in §0.1.
- **Collector-applied LogSources are incompatible with ABCG.** Operators on ABCG-managed Collector fleets must scope LogSources to devices, not to Collectors, or use failover-pair Collectors instead.

### 7.3 Discoverability

- The LogicModule exchange is browseable in-portal with vendor and category facets. Operators can preview an EventSource's Groovy definition before importing.
- The Alert Rule form provides field-level dropdowns and validation for severity, device groups, EventSource globs, Escalation Chain selection.
- The Groovy editor in the EventSource form provides syntax highlighting, but no compile-time validation — Groovy errors surface only at first trap match.

### 7.4 Live reload vs restart

- EventSource changes are pushed from SaaS to Collectors on a periodic pull cadence (documented as 1-15 minutes; precise interval is not publicly disclosed). New / updated EventSources take effect within that window without Collector restart.
- Collector `agent.conf` changes require Collector restart.
- Alert Rule changes are SaaS-side and take effect immediately.

### 7.5 Multi-tenancy / RBAC

- Each LogicMonitor portal is a single tenant. No native multi-tenant Collector (one Collector belongs to one portal).
- Within a tenant, **Roles** govern access to EventSources, Alert Rules, devices, Collectors. Standard roles: Administrator, Manager, Read-only; custom roles supported with fine-grained permissions.
- Sub-tenancy via **MSP (Managed Service Provider) mode** allows a single portal to manage multiple customer environments with isolated devices and dashboards. EventSources are shared across all MSP customers; Alert Rules can be scoped per customer.

## 8. Integration with Other Signals

### 8.1 Metrics

- Each trap **does not** increment a built-in counter metric by default (in contrast to Dynatrace's mandatory `traps.count` metric).
- Operators can create a **DataSource counter** that polls the same device for trap-related OIDs (e.g. counters for trap-generation rate) if the device exposes them — but this is a polled metric, not a trap-derived metric.
- Trap event counts are visible as **event counts** in the SaaS dashboards (not as a time-series metric in the same TSDB as polled data).

### 8.2 Alerting / Notifications

This is LogicMonitor's strength relative to Datadog/Dynatrace at the trap layer:

- **Alert Rule** matches (device group, EventSource, severity threshold, datapoint, instance) → Alert.
- **Escalation Chain** sequences notifications (email, SMS, Slack, Teams, PagerDuty, ServiceNow, OpsGenie, Jira, ConnectWise, AutoTask, custom HTTP).
- **Alert state lifecycle**: Open → Acknowledged → Cleared (auto or manual).
- **Alert clear** via matching clear-event (Groovy-defined in EventSource) or manual operator clear.
- **SDT (Scheduled Down Time)**: planned-maintenance window during which alerts are suppressed for a device or device group.
- **Alert routing logic**: Alert Rules are ordered; the first matching rule wins (similar to firewall rule ordering). Operators define a "Default" catch-all rule at the bottom.

### 8.3 Topology

- LogicMonitor has a **Topology Mapping** feature driven by **TopologySource** LogicModules. TopologySources are polling-based (LLDP, CDP, ARP, BGP, OSPF discovery via SNMP).
- Traps are **not directly correlated against the topology graph** by the trap pipeline. There is no built-in "suppress traps on devices whose parent is unreachable" engine.
- Operators can implement a degree of topology-aware suppression via Alert Rule "dependency" features (alert dependencies suppress dependent alerts when a parent alert is active), but this is a generic alert feature, not trap-specific.

### 8.4 Logs / Events

Two destinations depending on the path:

- **LogSource path** → **LM Logs**. Traps become first-class log entries in LM Logs alongside syslog and other log sources, queryable via LogicMonitor's log query language, addressable by log processing pipelines, and subject to LogAlert conditions. JSON attributes include the trap OID, source IP, varbind list, and (for known MIBs) symbolic OID names. Log retention: tenant-contract-defined.
- **EventSource path** → **per-device event log**. Events are searchable by device, EventSource, severity, time range, and free-text on subject/description. Event retention: tenant-contract-defined (typically 6–12 months).

The vendor's positioning of LogSource as the modern path means the recommended trajectory is to ingest traps into LM Logs, where they consolidate with the broader log estate. This is structurally similar to Datadog (traps → Logs Explorer) and Dynatrace (traps → Grail Events), but LogicMonitor adds first-party MIB translation that Datadog and Dynatrace do not.

### 8.5 Northbound forwarding

LogicMonitor **does not natively forward SNMP traps northbound**. The Collector receives traps but cannot re-emit them as SNMP traps to an upstream NMS. Operators with manager-of-managers architectures must either:

1. Use the **HTTP custom integration** on an Escalation Chain to emit a webhook to an upstream system (loses SNMP semantics; gains HTTP flexibility).
2. Use the **PagerDuty / ServiceNow / OpsGenie integrations** to feed an event-management platform.
3. Forgo the integration entirely and use LogicMonitor as the terminal trap consumer.

This matches Datadog and Dynatrace (also trap-sinks). OpenNMS, Centreon, Zabbix, Zenoss, and LibreNMS all support some form of native northbound trap or webhook forwarding; LogicMonitor matches the SaaS-tier peer group, not the open-source NMS group.

## 9. Severity Model

- **Vendor severity from MIB**: not parsed by either path. The Collector does not read SMIv2 `NOTIFICATION-TYPE` clauses (or their carried text-encoded severity hints) and does not auto-assign severity.
- **System severity (EventSource path)**: drawn from the EventSource's fixed severity field. The EventSource definition form exposes five severity levels (Critical / Error / Warning / Notice / Info), but Alert Rules and the broader LogicMonitor alerting semantics operate on three primary severity levels (warning, error, critical) — "Most alerts have three levels of severity: warning, error, and critical, with warning alerts usually being the least severe." The vendor docs explicitly position the EventSource severity selection: "use the Severity field's dropdown to indicate the severity level that will be assigned to the alerts that are triggered by this EventSource." For vendor-shipped EventSources, severity matches LogicMonitor's curation team's judgement (e.g. `linkDown` → Error / Warning, `bgpPeerDown` → Critical).
- **System severity (LogSource path)**: severity is set by **LogAlert conditions** on log processing pipelines. LogAlert conditions can declare severity per matched pattern, using the same warning/error/critical core severity levels.
- **Customisation surface**: operators can clone any vendor EventSource and edit severity; for LogSource, operators author or edit LogAlert conditions to assign severity per pattern.
- **Alert Rule severity filter**: Alert Rules filter by severity threshold ("Error or higher"). They do not override severity — they only filter and route. Most rules in production set the severity threshold and route different ranges to different Escalation Chains.

Compared to other SaaS-tier peers:
- **Dynatrace**: trap log events carry hard-coded `loglevel: NONE`; severity is assigned only when a log event rule promotes the event to a Davis problem.
- **Datadog**: trap logs do not carry an explicit severity; severity is assigned by Logs Monitors at the SaaS level.
- **LogicMonitor**: severity is assignable at the LogSource (LogAlert) or EventSource definition. Both paths are stronger than the Dynatrace and Datadog default-NONE pattern, but LogicMonitor still does not extract severity from the MIB itself.

## 10. Storm / Volume Handling

### 10.1 Per-source rate limits

- **No documented per-source token bucket** at the Collector. The Collector accepts all incoming traps until queue or OS UDP buffer saturates.
- **Alert Rule rate limiting** is available on the SaaS side: an Alert Rule can be configured to suppress repeated alerts on the same condition for a window. This is downstream of event ingestion (it controls notifications, not event creation).

### 10.2 Dedup keys and windows

- **EventSource path: none** — vendor explicitly states "Duplicate alerts for SNMP Trap EventSources are never suppressed." Operators relying on EventSource must accept one alert per trap.
- **LogSource path**: LM Logs / LogAlert can auto-close on paired clear-trap matches and retain a count of duplicate events ("Automatically close alerts when a related 'clear' trap comes in. Eliminate noisy alerts while retaining a record of the number of duplicate alerts."). Exact key syntax and time-window configuration knobs are not enumerated on a public page.
- **Alert-level suppression**: per Alert Rule, the alert frequency setting limits how often a re-triggering alert can notify. This is downstream of event ingestion (it controls notifications, not event creation).

### 10.3 Circuit breakers

- The Collector's JVM is the implicit circuit breaker: under sustained heap pressure, the JVM throttles event processing (longer GC pauses) and event queue fills up. Once full, oldest queued events are dropped.
- No public documentation of a per-EventSource CPU cap or per-source throttle.

### 10.4 Storm detection

- **Not built-in.** Operators can build a "trap storm" detector indirectly:
  - Author a DataSource that polls the trap event count via the REST API (events-per-minute per device).
  - Set a DataPoint threshold to alert on excessive trap volume.
- This is operator-effort, not a vendor-shipped feature.

### 10.5 Backpressure

- The Collector queues events when the HTTPS upload to SaaS is slow. Queue size is bounded; once full, events are dropped. Specific queueing mechanism (in-memory vs disk-spill) and drop policy (oldest-first vs newest-first) are not enumerated in public docs.
- No documented mechanism to slow trap reception when downstream is congested (the Collector does not signal trap-sending devices to back off — SNMP traps are unidirectional UDP).

The overall storm story is **comparable to Dynatrace's** (neither has built-in storm detection at the on-prem collector) and **weaker than OpenNMS's `<rate-limit/>`, Zenoss's `event-dedup` engine, or Centreon's `centreontrapd` dampening**.

## 11. Security

### 11.1 SNMPv3 USM

Algorithms supported (per LogicMonitor's published SNMPv3 device configuration UI):

- **Auth protocols**: MD5, SHA (SHA-1), SHA-224, SHA-256, SHA-384, SHA-512.
- **Priv protocols**: DES, 3DES, AES-128, AES-192, AES-256.
- **Security levels**: noAuthNoPriv, authNoPriv, authPriv.

These are the standard RFC 3414 + RFC 7860 + extended-AES sets. LogicMonitor does not document a "Reeder-style" vs "Cisco" key-localisation distinction in public material; the inferred default is the Reeder/Cisco extended-AES variant that most modern SNMP libraries (SNMP4J, gosnmp, pysnmp) implement.

USM credentials are configured **per-device** in LogicMonitor (one set per monitored device), not per-EventSource. This is operationally cleaner than configuring credentials per-trap-source.

### 11.2 DTLS / TLSTM

Not documented as supported. SNMP-over-TLS/DTLS (RFC 6353) does not appear in any LogicMonitor docs. Same gap as Datadog, Dynatrace, Telegraf; same alignment with OpenNMS and Zabbix.

### 11.3 Credential storage

- Device credentials (community strings, USM passwords) are stored in the SaaS portal's secret store, obfuscated at rest.
- Collectors fetch device credentials at poll/trap-reception time over the HTTPS channel.
- On the Collector side, credentials are held in memory; on-disk storage format for cached credentials is not documented.
- **Credential Vault integration**: LogicMonitor's Collector documentation enumerates Credential Vault integrations for fetching device credentials from external secret stores. The exact list of supported vendors (CyberArk, Delinea / Thycotic, etc.) is enumerated on the Collector Integrations pages; reviewers should treat the specific vendor list as docs-resolvable rather than committed in this analysis.

### 11.4 Access control on the trap subsystem

- No trap-subsystem-specific ACL beyond the standard EventSource / device / Collector permission model.
- Collector hosts are operator-managed; LogicMonitor does not enforce OS-level hardening.

### 11.5 Audit logging

- SaaS audit log captures EventSource definition changes, Alert Rule edits, Escalation Chain edits, Collector configuration changes, device additions/removals.
- Per-trap reception is **not** audit-logged (traps create events, not audit records).

## 12. Trap Simulation & Testing (in-source evidence)

This section is the most acute comparative weakness for a docs-only spec: **no public source code, no public test fixtures, no public CI workflow.**

What is documented operator-side:

- **Manual smoke test**: send a test trap with `snmptrap` (Net-SNMP CLI) to the configured Collector IP and port. LogicMonitor support recommends this as the canonical "is my trap configuration working?" test.
- **Collector debug log**: the Collector exposes debug-level logging via `agent.conf` (exact parameter name for trap-specific debug logging not verified in the WebFetch render; reviewers should consult the Collector debug facility documentation). The Collector logs received PDUs and EventSource match attempts at debug verbosity.
- **EventSource Groovy unit test**: the LogicMonitor portal EventSource form provides a sample-PDU testing aid (observed in portal UI; specific button label not separately documented in a public page) that lets operators paste a sample PDU and preview whether the match expression evaluates true. This is an authoring-time aid, not a runtime test.

Internal LogicMonitor test coverage of the trap pipeline is unknown and unverifiable from public material. Reviewers must not infer test rigour from product polish.

## 13. Out-of-the-Box Coverage (defaults)

### 13.1 MIBs bundled

- **LogSource path**: bundled MIB catalogue with broad vendor coverage. The vendor enumerates the supported MIBs at `logicmonitor.com/support/supported-mibs-for-snmp-trap-translation`; the list includes (alphabetically from the public docs) Accedian Networks, ADTRAN, Alvarion, Appian Communications, Applied Innovation, and continues through many more vendors. The Collector applies these translations at trap-reception time and emits MIB-translated symbolic names alongside numeric OIDs in the resulting log entries.
- **EventSource path**: no runtime MIB store; vendor MIB coverage is provided through the LogicModule exchange's pre-authored EventSources.

### 13.2 Severity rules bundled

- Per the curated EventSource catalogue (LogicModule exchange), each vendor-shipped EventSource has a curated severity assigned by LogicMonitor's curation team.
- LogSource bundled definitions (per `exchange.logicmonitor.com`) carry LogAlert conditions with curated severities for common trap patterns.

### 13.3 Dedup defaults

- **LogSource path**: built-in dedup and auto-clear via LogAlert conditions and pipeline configuration. Vendor-shipped LogSources for common pairings (linkDown/linkUp, bgpPeerDown/bgpPeerUp) include clear-pair logic.
- **EventSource path**: **none by vendor statement**. Operators relying on EventSource must accept that every trap creates an alert; volume control happens at Alert Rule frequency / SDT layers.

### 13.4 Vendor packs / integration packages

- The LogicModule exchange (`exchange.logicmonitor.com`) ships LogicModules (DataSources, EventSources, LogSources, PropertySources, TopologySources, ConfigSources) for a wide range of network and infrastructure vendors. This is comparable to Zenoss ZenPacks or OpenNMS vendor packs.

### 13.5 Sample / preset dashboards or reports

- LogicMonitor ships per-vendor dashboards driven by polled DataSources.
- Trap-specific dashboards are not typically pre-built; operators commonly add LM Logs widgets filtered to `log.source: snmptrap` or "Recent Critical Events" widgets to per-device dashboards.

## 14. User Customization Surface

### 14.1 Custom LogSources (recommended path)

- Author a SNMP Traps-type LogSource referencing the OIDs (or, on 35.400+, any traps via no-EnterpriseOID mode).
- Add log processing pipelines for filter / mutate / drop / enrich.
- Add LogAlert conditions for severity assignment and alerting.
- Distribute via the same LogicModule push mechanism that handles EventSources and DataSources.

### 14.2 Custom EventSources (legacy path)

- Author SNMPTrap-type EventSources with Groovy filters. Operators have access to the Groovy runtime within the Collector JVM, subject to LogicMonitor's Groovy environment (LogicMonitor publishes Groovy SNMP helpers at `com.santaba.agent.groovyapi.snmp`).

### 14.3 Custom MIBs

- **LogSource path**: convert with the MIBs to JSON Converter Utility (`[Collector]/bin/snmpMibsToJsonConversionUtil`), Python 3.8–3.12; place output in the directory the Collector reads with appropriate Collector-service-user read access.
- **EventSource path**: read MIB files out-of-band and hard-code OIDs in EventSource match conditions.

### 14.4 Custom severity rules

- **LogSource path**: per LogAlert condition.
- **EventSource path**: drop-down at EventSource definition time.
- Per Alert Rule: severity threshold filter ("notify on Error or higher").

### 14.5 Custom dedup rules

- **LogSource path**: log-pipeline-level dedup and clear-trap pair definitions.
- **EventSource path**: **none — vendor admits duplicates are never suppressed**.
- Per Alert Rule: alert frequency / repeat suppression as a notification-layer workaround.

### 14.6 Plugin / extension model

- **LogicModules** are the extension primitive. Relevant LogicModule types:
  - DataSource (polled metrics).
  - EventSource (events, including legacy SNMP trap path).
  - LogSource (LM Logs, including current SNMP trap path).
  - JobMonitor (long-running batch jobs).
  - ConfigSource (config-snapshot collection).
  - PropertySource (device tag derivation).
  - TopologySource (topology graph contribution; LLDP/CDP/BGP/OSPF/EIGRP-driven).
- LogicModules are authored in Groovy + JSON declarative metadata.
- The LogicModule exchange is the distribution channel (similar to Zenoss ZenPack catalogue or OpenNMS Plugin Manager).

### 14.7 API surface

- Full REST API at `/santaba/rest/` for all LogicModule, device, Alert Rule, Escalation Chain operations.
- Webhook integrations for alert outputs (custom HTTP integrations on Escalation Chains).
- Terraform provider (community-maintained, `logicmonitor/logicmonitor` on Terraform Registry).
- Ansible modules (community-maintained).
- Python SDK (`logicmonitor_sdk` on PyPI).
- The `agent.conf` file is the on-Collector configuration surface ("more than 600 settings" per `logicmonitor.com/support/agent-conf-file-configurations`); the vendor recommends edits via "Settings > Collectors > Manage Collector > collector config" rather than direct file edit.

## 15. End-User Value Analysis

### 15.1 Day-1 default value

What an operator gets immediately after installing a Collector and enabling the trap subsystem:

- Trap reception on UDP/162 once `eventcollector.snmptrap.enable=true` and (for LogSource) `lmlogs.snmptrap.enabled=true`.
- LogSource path: out-of-the-box MIB translation for the vendor-broad catalogue; trap OIDs and varbinds resolve to symbolic names without operator configuration.
- EventSource path: vendor-shipped EventSources from the LogicModule exchange cover common Cisco / Juniper / Arista / F5 / Palo Alto / etc. traps with curated severities.
- Trap data flows into LM Logs (LogSource path) or per-device event log (EventSource path).
- Alert generation requires operator-defined Alert Rules — there is no default catch-all.
- Notification requires operator-defined Escalation Chains — there is no default.

The "day-1 default" experience: LogicMonitor's `lmlogs.snmptrap.enabled=true` flag ingests all received traps as logs immediately with bundled-MIB OID translation applied (per the supported-MIBs page). Datadog requires operator-authored JSON `traps_db/*.json` files produced via the external `ddev meta snmp generate-traps-db` tool. Dynatrace ships a predefined OID set in its SNMP Traps extension, with custom MIBs requiring a complete Extensions 2.0 extension authoring workflow. LogicMonitor's combination of built-in MIB catalogue, portal MIB upload (Collector 38.300+), MIBs-to-JSON utility (EA 35.400+), and curated LogicModule exchange covers more vendor MIBs out-of-the-box than either Datadog or Dynatrace, and offers more first-party custom-MIB workflows.

### 15.2 What requires customisation

- **Alert Rules**: operators must define rules for which severity / EventSource / device combinations should generate alerts. There is no "alert on everything" default — operators must opt in.
- **Escalation Chains**: operators must define notification recipients and timing.
- **Custom traps** (unrecognised by the exchange): operators must author Groovy EventSources from scratch.
- **Topology-aware suppression**: operators must configure alert dependencies if they want suppression based on topology.

### 15.3 Learning curve

- **Steep on the Groovy side**: authoring custom EventSources requires Groovy fluency and Java knowledge.
- **Moderate on the Alert Rule side**: the rule syntax is straightforward but the order-matters semantics require care.
- **Light on the Collector side**: install is a single binary; defaults work for most cases.

### 15.4 Operational toil

- **High vigilance against silent drops** (§4.5): the silent-drop fallback for unmatched OIDs means operators must explicitly verify trap reception for every device class they introduce.
- **Collector JVM monitoring**: heap usage, GC pauses, queue depth — all of which the Collector exposes as built-in DataSources but operators must alert on.
- **EventSource maintenance**: vendor-shipped EventSources are updated periodically via LogicModule exchange; operators must accept updates or pin versions.

### 15.5 Visibility into the pipeline's own health

- The Collector itself is monitored as a LogicMonitor device with built-in DataSources for JVM heap, GC, event queue depth, EventSource processing latency, and SNMP listener status.
- The SaaS portal has a "Collector status" dashboard showing online/offline status, last-heartbeat, and queued-event count.
- This is **better than Dynatrace** (whose SFM metrics are partially documented but not enumerated) and **comparable to Datadog** (whose Agent ships built-in `agent_check_*` metrics).

## 16. Strengths

1. **First-party out-of-the-box MIB translation** (LogSource path): the Collector ships with a documented broad vendor MIB catalogue, plus the MIBs to JSON Converter Utility for custom MIBs. This is the strongest first-party MIB story among SaaS-tier peers (Datadog requires operator-driven `traps_db` JSON; Dynatrace requires per-extension authoring).
2. **Vendor-curated LogicModule exchange** provides ready trap definitions for major network and infrastructure vendors. Operators get day-1 value without writing trap-to-event mappings.
3. **Built-in dedup and clear-trap pairing on the LogSource path**: "Automatically close alerts when a related 'clear' trap comes in. Eliminate noisy alerts while retaining a record of the number of duplicate alerts." Better than Datadog (no dedup) / Dynatrace (no dedup at data source) at the SaaS-tier peer level.
4. **Documented Alert Rule + Escalation Chain + SDT model**: priority-ordered rule matching with first-match-wins semantics; numbered rule priority (lower number = higher priority); per-rule escalation interval; broad notification integrations (PagerDuty, ServiceNow, Slack, Teams, OpsGenie, email, SMS, custom HTTP).
5. **Per-resource SNMPv3 trap credentials** (`snmptrap.security`, `snmptrap.auth`, `snmptrap.authToken`, `snmptrap.priv`, `snmptrap.privToken` at the device level; EA 34.100+). A single Collector handles multi-tenant trap traffic from devices with different SNMPv3 keys.
6. **Severity assignable per LogSource pattern (LogAlert) or per EventSource definition**: stronger than Dynatrace (hard-coded NONE) and Datadog (severity at Logs Monitor layer only).
7. **Programmable extensibility via Groovy**: operators with the skills can express arbitrary trap-to-event logic, complex multi-varbind matching, and synthesised messages.
8. **Collector self-monitoring**: built-in DataSources expose JVM and trap-pipeline health; operators can alert on Collector overload.
9. **Trap data is first-class in LM Logs (LogSource path)**: pipelines, query language, AI features (Edwin AI, Log Anomaly Detection, Log Analysis) all consume traps alongside other logs.

## 17. Weaknesses / Gaps

1. **EventSource path has no dedup**: the vendor explicitly states "Duplicate alerts for SNMP Trap EventSources are never suppressed." This single sentence has driven the vendor's repositioning of LogSource as the recommended path, but legacy operators who started on EventSource carry the cost of the architectural fork: migration, side-by-side validation, careful exclusivity (EventSource and LogSource cannot both be applied to the same device).
2. **Architectural fork between LogSource and EventSource is a real operator decision**: the vendor's recommendation to use LogSource creates a migration imperative for existing EventSource customers, with no documented one-shot migration tool.
3. **SNMP traps are excluded from Auto-Balanced Collector Groups (ABCG)**: trap reception cannot ride on the same auto-balancing primitive used for polling. Operators with large heterogeneous fleets must manage Collector destinations manually and accept that ABCG dynamic failover does not extend to trap-listener load-balancing.
4. **No northbound trap forwarding**: LogicMonitor cannot re-emit traps to upstream NMSes. Operators with manager-of-managers architectures must use HTTP webhooks via Escalation Chain integrations.
5. **No INFORM-PDU support**: not documented; inferred not supported.
6. **No native L2 trap-to-topology suppression**: TopologySources are polled (LLDP/CDP/BGP/OSPF/EIGRP), not trap-driven; trap-derived alerts are not suppressed by upstream topology-node-down state without operator-defined alert dependencies.
7. **Java Collector footprint**: heavier than Go-based agents (Datadog, Telegraf); operators must plan for JVM heap sizing, GC tuning, and process supervision.
8. **EventSource path silently drops unmatched OIDs**: no catch-all event for unknown traps on the legacy path. The LogSource path mitigates this (numeric OID preserved on the ingested log).
9. **Per-Collector-size trap throughput baseline is conservative**: the vendor's 10-traps-per-second-per-device assumption produces device-count-based sizing rather than an absolute traps-per-second ceiling. Operators planning trap-storm scenarios have only this baseline.
10. **SaaS-side workflow dependency**: every alert decision crosses the WAN; operators cannot validate the end-to-end pipeline without a tenant connection.
11. **No DTLS/TLSTM support**: transport security is community-string- or USM-only, matching the SaaS-tier peer group but lagging modern SNMP-over-TLS proposals.
12. **Curation team dependency for severity choices**: vendor-shipped EventSources and LogSources carry LogicMonitor's curation team's severity decisions; operators with different conventions must fork modules.
13. **MIBs to JSON Converter Utility requires Python 3.8–3.12 outside the Collector**: this is a separate operator workflow (run Python utility on a workstation, copy JSON to Collector, ensure read perms) rather than an in-Collector MIB compile-on-the-fly mechanism. Less integrated than OpenNMS `mib2events` or Zenoss ZenPack auto-loading.
14. **`agent.conf` editing carries footguns**: "more than 600 settings" make hand-editing risky; the vendor explicitly recommends UI editing, but advanced changes (including trap thread-pool sizing) require the file-level surface.

## 18. Notable Code or Configuration Examples

No public source code is available. The following are representative vendor-documented configuration shapes (paraphrased from LogicMonitor support documentation and LogicModule exchange UI inspection):

### 18.1 EventSource match expression (Groovy, illustrative)

This example is **illustrative pseudocode** for an EventSource SNMPTrap Groovy filter. The Collector Groovy SNMP package (`com.santaba.agent.groovyapi.snmp`) exposes typed PDU access in vendor-published javadocs; the exact field accessor surface for the EventSource Filters area is not enumerated in a single public docs page, so the example below shows shape rather than verbatim syntax.

```groovy
// Match a Cisco IF-MIB linkDown trap and emit an Error-severity event.
def trapOid = snmpTrapOid.toString()  // e.g. ".1.3.6.1.6.3.1.1.5.3"
if (trapOid == ".1.3.6.1.6.3.1.1.5.3") {
    def ifIndex = varbinds[".1.3.6.1.2.1.2.2.1.1"]  // ifIndex varbind
    return [
        matched: true,
        severity: "Error",
        subject: "Interface ifIndex=${ifIndex} is down",
        description: "Link down event from device ${device.name}",
    ]
}
return [matched: false]
// Note: dedup is NOT supported on the EventSource path — vendor docs state
// "Duplicate alerts for SNMP Trap EventSources are never suppressed."
```

### 18.2 Alert Rule (paraphrased portal form)

```
Name:               "Network device critical traps"
Priority:           High (lower number = higher priority)
Device Group:       "Devices/Network/*"
EventSource:        "Cisco_IOS_SNMPTraps"
Datapoint:          "*"
Instance:           "*"
Severity Threshold: Error or higher
Escalation Chain:   "Network On-Call"
SDT Behavior:       Suppress alerts during SDT
Alert Clear:        Cleared via paired clear-event (e.g. linkUp clears linkDown)
```

### 18.3 Escalation Chain (paraphrased portal form)

```
Name:           "Network On-Call"
Stages:
  1. Notify Slack channel #netops (immediate)
  2. Notify PagerDuty service "Network NOC" (after 5 minutes if unacknowledged)
  3. Notify on-call manager via SMS (after 15 minutes if still unacknowledged)
Repeat:         Every 30 minutes until acknowledged or cleared
```

### 18.4 Collector `agent.conf` snippet (vendor-documented parameter names)

```ini
# agent.conf — SNMP trap-relevant parameters
# (paths and values drawn from logicmonitor.com/support/agent-conf-file-configurations
#  and surrounding troubleshooting and SNMPv3 docs)

# Trap listener
eventcollector.snmptrap.enable=true
eventcollector.snmptrap.threadpool=10
eventcollector.snmptrap.address=udp:0.0.0.0/162

# LogSource ingestion gate
lmlogs.snmptrap.enabled=true

# EA 36.200+ JSON message formatting for LogSource trap messages
# (when enabled, the Collector emits trap messages as JSON, easier
#  to parse downstream in LogAlert / log pipelines)
lmlogs.snmptrap.formatMessageAsJson.enabled=true

# Global SNMPv1/v2c community-string fallback (used when neither resource-level
# snmptrap.* nor snmp.* properties are defined)
eventcollector.snmptrap.community=<community-string>

# Global SNMPv3 fallback credentials (used when neither resource-level
# snmptrap.* nor snmp.* properties are defined)
eventcollector.snmptrap.security=<usm-user>
eventcollector.snmptrap.auth=<auth-protocol>          # MD5 | SHA | SHA-224 | SHA-256 | SHA-384 | SHA-512
eventcollector.snmptrap.auth.token=<auth-secret>
eventcollector.snmptrap.priv=<priv-protocol>          # DES | 3DES | AES-128 | AES-192 | AES-256
eventcollector.snmptrap.priv.token=<priv-secret>
```

### 18.5 Per-device SNMPv3 trap credential properties

```
# Resource properties (per device, set in the LogicMonitor UI or via REST API)
snmptrap.security=<usm-user>
snmptrap.auth=<auth-protocol>
snmptrap.authToken=<auth-secret>
snmptrap.priv=<priv-protocol>
snmptrap.privToken=<priv-secret>
```

Credential resolution: `snmptrap.*` resource properties → `snmp.*` resource properties → `eventcollector.snmptrap.*` in `agent.conf`.

### 18.6 MIBs to JSON Converter Utility invocation (vendor-documented workflow)

```
# 1. Install Python requirements (Python 3.8 to 3.12 required)
cd [LogicMonitor Collector Directory]/bin/snmpMibsToJsonConversionUtil
pip install -r requirements.txt

# 2. Run the converter. The script prompts INTERACTIVELY for:
#    - source directory (must contain target MIBs and all parent/dependency MIBs)
#    - destination directory (where JSON output is written)
python snmpMibsToJsonConverter.py

# 3. Copy generated JSON files to the Collector's custom MIB JSON directory
#    if the destination was not already there. Do NOT rename the JSON files.
cp /path/to/destination/*.json [LogicMonitor Collector Directory]/snmpdb/custom/

# 4. Ensure the Collector service user has read access to the JSON files

# 5. Restart the Collector for changes to take effect
```

Constraints (vendor-documented):
- "Ensure that the definition of a MIB file is not part of multiple input MIB files."
- The Collector service user must have read access to the JSON files.
- Filenames produced by the converter must not be renamed (the Collector looks them up by filename).

## 19. Sources Examined

All sources are vendor-published or vendor-curated. Each citation includes a URL and retrieval date 2026-05-23.

Primary trap-subsystem docs:
- `https://www.logicmonitor.com/support/snmp-trap-logsource-configuration` — SNMP Traps LogSource Configuration (primary reference for the recommended path).
- `https://www.logicmonitor.com/support/snmp-traps-as-logs-overview` — SNMP Traps as Logs Overview.
- `https://www.logicmonitor.com/support/logicmodules/eventsources/types-of-events/snmp-trap-monitoring` — SNMP Trap Monitoring (EventSource legacy path).
- `https://www.logicmonitor.com/support/eventsource-configuration` — EventSource Configuration reference.
- `https://www.logicmonitor.com/support/snmp-trap-mibs` — SNMP Trap MIBs and MIBs to JSON Converter Utility.
- `https://www.logicmonitor.com/support/supported-mibs-for-snmp-trap-translation` — bundled MIB vendor list.
- `https://www.logicmonitor.com/support/troubleshooting-snmp-traps-issues` — operator troubleshooting.
- `https://www.logicmonitor.com/integrations/snmp` — vendor integrations page for SNMP Traps as Logs.
- `https://www.logicmonitor.com/blog/snmp-traps` — vendor blog on SNMP traps.

Collector and configuration docs:
- `https://www.logicmonitor.com/support/agent-conf-file-configurations` — agent.conf parameter reference.
- `https://www.logicmonitor.com/support/collector-capacity` — Collector sizing matrix.
- `https://www.logicmonitor.com/support/collectors/collector-overview/about-the-logicmonitor-collector` — Collector overview.
- `https://www.logicmonitor.com/support/auto-balanced-collector-groups` — ABCG model and SNMP trap exclusion.
- `https://www.logicmonitor.com/support/collectors/collector-failover/collector-failover-failback` — Collector HA.
- `https://www.logicmonitor.com/support/collector-performance-and-tuning` — Collector performance tuning.

Alerting and workflow docs:
- `https://www.logicmonitor.com/support/alert-rules` — Alert Rules.
- `https://www.logicmonitor.com/support/escalation-chains` — Escalation Chains.
- `https://www.logicmonitor.com/support/eventsource-alerting` — EventSource alerting.
- `https://www.logicmonitor.com/support/lm-logs/log-alert-conditions` — LogAlert conditions.
- `https://www.logicmonitor.com/support/lm-logs/log-processing-pipelines` — log processing pipelines.

SNMPv3 docs:
- `https://www.logicmonitor.com/support/monitoring/os-virtualization/snmp-v3-configuration` — SNMPv3 configuration.
- `https://www.logicmonitor.com/support/monitoring/os-virtualization/snmp-v3-configuration-troubleshooting` — SNMPv3 troubleshooting.
- `https://www.logicmonitor.com/support/getting-started/advanced-logicmonitor-setup/defining-authentication-credentials` — defining authentication credentials.

Topology and customization:
- `https://www.logicmonitor.com/support/forecasting/topology-mapping/topology-mapping-overview` — Topology Mapping.
- `https://www.logicmonitor.com/support/terminology-syntax/scripting-support/groovy-snmp-access` — Groovy SNMP Access.
- `https://www.logicmonitor.com/support-files/javadocs/28606/com/santaba/agent/groovyapi/snmp/Snmp.html` — Groovy SNMP javadoc.
- `https://www.logicmonitor.com/support/logicmodules/datasources/groovy-support/advantages-of-using-groovy-in-logicmonitor` — Groovy in LogicMonitor.

Community references (used to corroborate, not as primary):
- `https://community.logicmonitor.com/blog/tech-talk/snmp-trap-credentials-on-resource-properties-enhancement/13096` — SNMP Trap Credentials on Resource Properties Enhancement (EA 34.100).
- `https://community.logicmonitor.com/discussions/product-discussions/auto-balanced-collector-group-vs-failover/17085` — ABCG vs. failover semantics.

Exchange / catalogue:
- `https://exchange.logicmonitor.com/` — LogicModule exchange.

## 20. Evidence Confidence

Per major section, rated as **high** (multiple consistent vendor citations), **medium** (single vendor citation or vendor + community), or **low** (single source, inference, or community-only):

| Section | Confidence | Notes |
|---|---|---|
| §1 System overview | High | Multiple vendor docs corroborate the Collector + SaaS architecture and the dual LogSource/EventSource path model. |
| §2 Architecture | High for components and the dual-path model; Medium for inter-component IPC details (Collector-to-SaaS wire protocol is not publicly enumerated). |
| §3 Trap reception | High for `agent.conf` parameter names (vendor-stated); High for SNMPv1/v2c/v3 support; High for the credential resolution order; Low for INFORM (absence of mention). |
| §4 MIB management | High for the LogSource bundled MIB catalogue (vendor explicitly enumerates supported MIBs); High for the MIBs to JSON Converter Utility (Python 3.8–3.12, EA 35.400+, documented path); High for the EventSource "no MIB store" position. |
| §5 Pipeline | High for the parse / match / forward flow; High for the EventSource "duplicates never suppressed" admission; High for the LogSource auto-clear capability. |
| §6 Storage | High for the LM Logs vs. event store split (vendor-documented); Medium for retention numbers (cite tenant contract, not docs). |
| §7 Configuration UX | High for portal UI surfaces; High for REST API and Terraform; High for `agent.conf` field names (vendor-quoted). |
| §8 Integrations | High for alerting and notifications; High for ABCG SNMP trap exclusion; Medium for topology (Topology Mapping exists but trap-to-topology correlation is not explicitly documented). |
| §9 Severity | High — severity assignment is documented for both LogSource (LogAlert) and EventSource (dropdown) paths. |
| §10 Storm handling | Medium-to-Low — beyond the trap-listener thread pool and the LogSource dedup/clear semantics, dedicated storm-detection primitives are sparsely documented. |
| §11 Security | High for SNMPv3 USM algorithm support; High for per-resource snmptrap.* credentials (EA 34.100+); Medium for credential storage format details. |
| §12 Testing | Low — no public source, no public test fixtures, no public CI. Only operator-side validation workflows documented. |
| §13 Out-of-the-box | High for the bundled MIB catalogue (vendor docs); High for the LogicModule exchange's curated coverage; Medium for exact bundled-EventSource counts (exchange browser is the source of truth). |
| §14 Customisation | High for Groovy EventSource authoring, the MIBs to JSON utility, the LogSource pipeline model; High for REST API and Terraform. |
| §15 End-user value | High for the day-1 experience description; Medium for operational toil claims (community-corroborated). |
| §16-17 Strengths/weaknesses | High — claims grounded in §1-§15 vendor evidence including direct quotes. |
| §18 Examples | High for the `agent.conf` parameters (vendor-named); Medium for the Groovy EventSource shape (synthesized from community snippets); Medium for the MIBs to JSON utility flags (CLI flags inferred). |

## Reviewer Pass Log

### Iteration 1 (2026-05-23)

Six reviewers run in parallel: codex, glm, kimi, mimo, minimax, qwen.

Verdicts:
- codex: reject (raised 8 major issues, 12 total findings; flagged the LogSource-vs-EventSource fork as inadequately represented in the architecture diagram, identified the EA 36.100 PDU-based source-identification rules, the portal MIB upload workflow on Collector 38.300+, the actual Collector capacity table values, and the inaccurate MIBs-to-JSON utility invocation).
- glm: accept-with-fixes with 5 major findings about contradictions between path sections and missing LM Logs schema enumeration.
- kimi: accept-with-fixes with 8 findings on Datadog/Dynatrace overclaims and schema inconsistencies.
- mimo: accept-with-fixes (no blockers, 3 majors all about LogSource path representation).
- minimax: 7 blockers all framing-level (ABCG implications, queue mechanics hedging, EventSource dedup admission strength).
- qwen: accept-with-fixes with 2 majors on dedup contradiction in §6/§10.

Iteration-1 fixes applied (in iter-2 source):
- Rewrote §1, §2.1, §2.3, §3.1, §3.3, §3.5, §4 (full), §5.1, §5.2, §5.3, §5.6, §6.1, §7.2, §8.4, §9, §10.2, §10.5, §11.3, §13, §14, §15.1, §16, §17, §18.1, §18.4, §18.6 to:
  - Represent the dual LogSource/EventSource architectural fork consistently.
  - Document the EA 36.100 PDU-based source-identification model.
  - Add the portal MIB upload workflow (Collector 38.300+) and the MIB resolution precedence.
  - Correct the MIBs to JSON Converter Utility invocation (script name, interactive prompts, /snmpdb/custom output, restart).
  - Add the SNMP Traps Processing Preference matrix in §7.2.1.
  - Add concrete Collector capacity values (v2: medium 17, large 87, XL 140, XXL 245; v3: 14/70/112/196).
  - Add `eventcollector.snmptrap.community` and `lmlogs.snmptrap.formatMessageAsJson.enabled` to the agent.conf example.
  - Remove the dedupKey field from the EventSource Groovy example.
  - Tighten the storage table to reflect the actual EventSource (no dedup) vs LogSource (LM Logs) split.
  - Trim marketing language ("battle-tested", "significantly better") to evidence-based phrasing.
  - Hedge unverified queue / disk-spill / secret-store / Test Match button claims.

### Iteration 2 (2026-05-23)

Six reviewers re-run with iter-2 prompt.

Verdicts:
- codex: reject — but this was a tooling artifact (codex was launched with `-C /tmp` and could not access project files; the verdict is the path-access error, not a real review finding). Discounted.
- glm: accept-with-fixes (4 majors, 9 minor/nit; majors all about inline citation gaps on iter-1 additions and one schema-table consistency note).
- kimi: accept-with-fixes (3 "blockers" all relating to citation gaps for iter-1 portal MIB upload, MIBs-to-JSON workflow, and `agent.conf` parameters; 8 majors all citation / inference labelling; 3 minor).
- mimo: accept-with-fixes (2 majors on `lmlogs.snmptrap.enabled` scope wording and missing portal MIB upload citation URL; 5 minor; no blockers).
- minimax: accept-with-fixes (no blockers, 3 minor, 4 nit; primarily citation precision and EA version attribution).
- qwen: accept-with-fixes (2 major on EA-version citations, 3 minor, 1 nit).

Iteration-2 fixes applied (final):
- Added inline attribution notes to the portal MIB upload workflow section (§4.1) acknowledging vendor-page-content rendering limits.
- Clarified `lmlogs.snmptrap.enabled` scope in §7.2 (acts as fallback when no LogSource / EventSource matches; does not override per-device exclusivity).
- Removed `dedup_groovy` / `clear_groovy` from the EventSource definitions schema row in §6.1 (SNMP Trap EventSources do not carry these fields).
- Split the storage table's trap event row into separate EventSource-path (event store, no dedup_key) and LogSource-path (LM Logs, structured schema) rows.
- Removed the speculative "naturally deduplicated" inference from §2.3 and replaced with the vendor-stated "only the active Collector reports the trap" wording.
- Tightened §3.3 INFORM hedging to remove the SNMP4J-based "plausible at wire level" speculation; positioned as a vendor documentation gap.
- Added URL attribution to the LogSource enablement quote (§3.1) and the LogSource OID translation quote (§4).

### Convergence assessment (2026-05-23)

5/6 reviewers converged on `accept-with-fixes` after iteration 2. The 6th (codex) had a path-access tooling problem and is discounted. Remaining findings across the 5 successful reviewers are all minor or nit class: citation polish, schema-table consistency, EA-version sourcing for parameters introduced in iter-1, and a handful of phrasings that could be tightened further.

Per SOW Decision 3 ("Stop when only minor/nit findings remain"): the document is at the iteration-2 stop point. Surviving open items are recorded below for traceability rather than blocking acceptance.

### Surviving minor findings (acknowledged, not blocking)

- **§4.1 portal MIB upload**: detailed claims (RBAC, dependency validation, status states, non-deletability, 3-tier precedence) reflect vendor documentation but lack verbatim quotes due to WebFetch rendering limits at the snapshot date. Reviewers needing direct verbatim attribution should re-fetch `logicmonitor.com/support/snmp-trap-mibs` when public rendering improves.
- **§3.5 capacity table**: published values cover medium / large / XL / XXL; small and triple-XL sizes were not enumerated in the WebFetch render. The `logicmonitor.com/support/collector-capacity` page is the live reference.
- **§18 examples**: §18.1 Groovy and §18.6 utility flags remain illustrative pseudocode; the SOW template prefers quotable evidence. For a closed-source system without source mirror, illustrative pseudocode is the available evidence shape, and is labelled as such inline.
- **§9 severity levels**: 5-level EventSource dropdown is reconciled with 3-level core severity model; specific mapping of Notice and Info under Alert Rule severity filters is operator-empirical (Alert Rule severity filters use the core levels; Notice and Info are filterable as severity floors in the rule definition).
- **§3.3 INFORM**: vendor documentation gap; not a wire-level capability claim.
- **§5.5 varbind typing on LogSource path**: the document claims varbinds are textual; LogSource path likely preserves ASN.1 types in LM Logs but this is not enumerated in public docs.
- **§6.1 retention**: tenant-contract-defined retention values are referenced rather than quoted; reviewers should treat any specific time range as tenant-resolvable.
- **§0.1 brutal-honesty preface**: deviates from the template's strict §0 = Metadata convention by carrying analytical content. The deviation is intentional and documented by other per-system specs (Dynatrace, Datadog Agent) using the same pattern; preserving cross-system framing alignment.
