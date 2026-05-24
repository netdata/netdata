# Dynatrace — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: Dynatrace (SaaS / Managed). Closed-source, proprietary, commercial observability platform. The on-prem footprint relevant to SNMP traps is **ActiveGate** (a multi-role gateway daemon), specifically the **Extension Execution Controller (EEC)** running inside ActiveGate, which executes the **SNMP Traps data source** packaged by the Extensions 2.0 framework. Traps land in the SaaS tier as **log events** in Grail, are queryable via DQL, and can be promoted to **Davis problems** through log event rules.
- **Source evidence**: docs-only. Dynatrace publishes no source code for ActiveGate, EEC, the SNMP traps data source, or Davis. Every architectural claim in this document traces to a vendor URL (`docs.dynatrace.com`, `dynatrace.com/hub`, `dynatrace.com/news/blog`) or to the official community troubleshooting guide on `community.dynatrace.com`. There is no `file:line` evidence; reviewers must not demand it for this system.
- **Citation convention**: vendor URL plus a verbatim quote (1-2 sentences max) of the cited text. Where docs and community guidance disagree, the docs page is treated as authoritative; community-thread material is labelled as such.
- **Closed-source unknowns disclaimer**: this document marks every inference clearly. Concretely, the following surfaces are NOT publicly documented and any claim about them in this file is labelled inference, low-confidence, or "not publicly specified": (a) the language/library used inside the EEC and the trap data source binary; (b) the on-disk credential storage format on ActiveGate; (c) the data-source-to-EEC IPC mechanism; (d) the data-source behaviour under EEC `HIGH_CPU` throttle; (e) malformed-PDU handling; (f) the exact bundled OID list shipped with the SNMP Traps extension; (g) Dynatrace's own test coverage and CI for the trap pipeline. Reviewers should not treat any of these as facts.
- **Repository root analysed**: n/a — no public repository. The Extensions 2.0 declarative YAML schema and the SNMP Traps extension YAML manifest are themselves vendor artefacts surfaced only through the docs and the Dynatrace Hub installer; the binary that interprets them is closed.
- **Snapshot date**: 2026-05-22. Documentation site revisions are not pinned by URL; conclusions here are stable across the cited URLs as long as Dynatrace does not retitle the SNMP Traps extension or restructure the Extensions 2.0 SNMP page family.
- **Surface analysed**: Dynatrace SaaS and Dynatrace Managed share the same SNMP Traps stack (same EEC, same ActiveGate, same data source). Dynatrace Classic ("AppMon") is out of scope and has no documented SNMP trap surface. All findings refer to **Dynatrace SaaS + Managed (Extensions 2.0 era)**.
- **Author**: assistant
- **Reviewer pass**: **converged after iter-4** — see the Reviewer Pass Log appended to this file for the per-iteration verdict tables and finding dispositions. Short version: 4 iterations, 6 reviewers each (codex, glm, kimi, mimo, minimax, qwen). Pattern: iter-1 surfaced 6+ majors per reviewer (structural framing problems); iter-2 reduced to 0-4 majors per reviewer (URL precision and value precision); iter-3 reduced further to 0-5 majors per reviewer (one reviewer — codex — caught the v2.0.1-vs-v2.1.0 version error and the container/K8s deployment-support gap); iter-4 returned 2 clean accepts (glm + minimax), with the remaining 4 reviewers each surfacing at most 1 substantive precision major (linkUp/linkDown clear semantics; INFORM peer-citation), both applied. The SOW stop rule (iterate while majors remain) was satisfied: applied iter-4 fixes leave only minor / nit findings, all of which represent asymptotic perfection rather than analytical disagreement.
- **Convergence outcome**: **converged**. 2 of 6 reviewers `accept` clean (glm, minimax); the other 4 are `accept-with-fixes` whose fixes are all applied. Surviving minors: paragraph-length preferences, additive examples, recurring hyphenation. Document is ready for inclusion in the cross-system comparison matrix.

### 0.1 Brutal-honesty preface

Dynatrace's primary product focus is application instrumentation (OneAgent auto-injects across processes; SmartScape topology; Davis causation analysis). **SNMP trap support is a secondary, late-arriving capability** that exists to let Dynatrace customers absorb network-device event signals into the same problem-correlation funnel that already consumes their APM, infra-host, and Kubernetes signals. It is not a full NMS replacement. There is no severity normalisation in the trap data source itself (the log-event `loglevel` field is hard-coded to `NONE`, and there is no separate severity attribute on the emitted event), no built-in dedup / suppression of repeat traps at the ActiveGate, no alarm-clear lifecycle, no northbound forwarding, and no operator-customisable correlation at the on-prem hop. Every cross-trap correlation, dedup, severity decision, and alerting happens **in the SaaS tier** via log processing rules, DQL queries, and Davis. This makes Dynatrace structurally similar to Datadog (per `datadog-agent.md` §1: traps are forwarded as logs and end up in the SaaS Logs Explorer). Three notable differences worth flagging up front: (1) **MIB handling**: Dynatrace accepts raw MIB files in `mib-files-custom/` on the ActiveGate, but **only custom (user-authored) extensions consume them — the bundled SNMP Traps extension itself ships a fixed predefined OID set and does NOT dynamically load `mib-files-custom/`** (see §4.2 for the verbatim vendor statement). Datadog instead accepts pre-compiled JSON or YAML trap-DB files (optionally gzip-compressed) in `snmp.d/traps_db/`, produced by the external `ddev meta snmp generate-traps-db` tool (per `datadog-agent.md` §4.2). Both systems require operator MIB work; the workflow differs in format and tooling. (2) Dynatrace trap events flow into the **Davis** problem pipeline through operator-configured log event rules. Datadog uses Logs Monitors as the equivalent (per `datadog-agent.md` §8.2). The mechanisms differ in shape but converge on the same pattern: a SaaS-side rule promotes a matching log event into an alertable item. (3) Dynatrace trap log events carry no SaaS-emitted source severity by default — this is true on both sides; see §9 and the §5.5 comparison.

## 1. System Overview & Lineage

Dynatrace is a commercial closed-source SaaS observability platform with a Managed (customer-hosted control plane) variant. Its data-collection footprint on customer premises is two components:

- **OneAgent** — auto-instrumentation agent installed on hosts, containers, and Kubernetes nodes. Owns processes, services, traces, RUM, infrastructure metrics.
- **ActiveGate** — a multi-role daemon running on dedicated VMs or containers, sitting between customer-side data sources and the Dynatrace cluster (SaaS or Managed). Roles include: HTTPS proxy/aggregator for OneAgent, plugin/extension execution host, Kubernetes/cloud API broker, log forwarder, AWS/Azure/GCP cloud monitoring gateway, and — relevant here — the **host for the Extension Execution Controller (EEC)** that runs SNMP polling and SNMP trap reception.

> "The Extension Execution Controller (EEC) is the Dynatrace component running your extensions, querying either your local data sources when run on OneAgent, or remote data sources when run from an ActiveGate. EEC is automatically installed and managed with each OneAgent and ActiveGate configuration."
> — `docs.dynatrace.com/docs/ingest-from/extensions/advanced-configuration/eec-custom-configuration`

SNMP traps land on the **ActiveGate** side of the EEC. There is no documented OneAgent surface for SNMP trap reception — OneAgent is process-attached and does not bind UDP/162.

### 1.1 Where SNMP traps fit in the broader product

- **One signal of many**, not a primary use case. Dynatrace's product identity is application-level observability and Davis-driven causation. SNMP traps are positioned in the **Network Device Monitoring** corner of the product (alongside the "Generic network device" SNMP polling extension), and are a relatively recent addition to Dynatrace's data-source portfolio. The competitive context (Datadog NDM, ScienceLogic, SolarWinds) is inference, not vendor-stated intent.
- **Traps are a `logs` data type on the SaaS side**, with optional metrics. The default feature set emits a single counter — `com.dynatrace.extension.snmp-traps-generic.traps.count` — counting received traps per source. The **`Events` feature set** must be explicitly enabled in the monitoring configuration to forward the actual trap content as **log events** to Grail. This mirrors the Datadog "traps-as-logs" design.
  > "If you use the default feature set, the extension will only report a single metric that counts the number of traps sent by a defined source during a defined interval."
  > — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`
  > "For log ingestion, select the **Events** feature set in the monitoring configuration."
  > — `docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-traps-statistics`
- **Davis AI gets the events** once they exist in Grail. The blog post "Accelerate resolution of network issues with AI-powered event reporting based on SNMP traps" (`dynatrace.com/news/blog/accelerate-resolution-of-network-issues-with-ai-powered-event-reporting/`) describes two paths from a trap to a Davis problem: (a) a log event rule that fires a problem on every matching trap; (b) extraction of a metric from a trap-event attribute and an anomaly detection rule against that derived metric. Both paths require operator configuration; neither is automatic for arbitrary trap OIDs.

### 1.2 Lineage and naming

- The **"SNMP Traps"** extension is the first-party Dynatrace-authored extension distributed via the Dynatrace Hub. The Hub-listed version at the time of analysis is **2.1.1** (`dynatrace.com/hub/detail/snmp-traps-statistics/`), with author "Dynatrace". Hub-published release-note highlights for the trap-relevant minor versions:
  - **v1.1.4** — introduced the Events feature set, the topology definition, and the per-device Unified Analysis screen.
  - **v1.2.0** — added the Logs section on the Unified Analysis screen and the standard `dt.ip_addresses` attribute.
  - **v1.2.2** — added a new dashboard.
  - **v2.0.1** — "Trap data will be automatically displayed on generic network device entities" (auto-display on previously discovered devices).
  - **v2.1.0** — `network:device` **entities are created directly from the trap data**, with `same_as` relations to their trap-entity counterparts; platform dashboard added.
  - **v2.1.1** — bug-fix release (platform dashboard load issue on some environments).
  The companion polling extension is the **"Generic network device"** extension (also Dynatrace-authored).
- Minimum supported ActiveGate versions called out in the docs:
  - **ActiveGate 1.235** for SNMPv2c (and earlier) trap reception.
  - **ActiveGate 1.251** for SNMPv3 trap reception.
  - These versions establish that the modern Extensions-2.0 SNMP trap data source is a feature added to ActiveGate's EEC in the 1.235–1.251 sprint window. Dynatrace ships ActiveGate on a sprint cadence (approximately every 2 weeks); the calendar mapping from sprint version number to release date is not reproduced here — reviewers needing exact dates should consult `docs.dynatrace.com/docs/whats-new/activegate/` for the per-sprint release notes.
- The Extensions 2.0 framework supersedes the legacy "Extensions 1.0" (Python plugin) approach. SNMP traps are NOT documented as available under Extensions 1.0; the trap data source is an Extensions-2.0-only feature, declared via YAML and executed by the EEC. The 2.0 framework consolidates JMX/WMI/SQL/Prometheus/SNMP under a single declarative model:
  > "Extensions are modular packages that define how Dynatrace collects and structures telemetry data from external sources."
  > — `docs.dynatrace.com/docs/extend-dynatrace/extensions20`

### 1.3 Relationship to upstream tools

- **`Net-SNMP` / `snmptrapd`**: NOT used as an external front-end. The SNMP traps data source binds the UDP socket itself; there is no documented `traphandle` integration, no shell-out to `snmptrapd`, no `snmptt`. This is a fundamental contrast with the Zabbix Perl receiver, LibreNMS PHP handler chain, Centreon `centreontrapd`, and the Nagios/SNMPTT family — all of which depend on `snmptrapd` for the wire-level decode.
- **MIB libraries**: the docs are silent on which MIB parser implementation is shipped inside the EEC. Custom MIB files are placed in `mib-files-custom/` (see §4); the EEC consumes them to translate OIDs to symbolic names at trap reception. There is **no public statement** of whether the underlying library is `libsmi`, a Dynatrace-internal implementation, or an embedded `pysmi`. This is a confidence-low area for closed-source reasons.
- **`gosnmp` / `pysnmp` / `snmp4j`**: not disclosed. The EEC is also closed-source. Reviewers should not infer a specific library.

### 1.4 Audience and licensing

- **Audience**: enterprises with a Dynatrace SaaS or Managed subscription who have network devices (routers, switches, firewalls, load balancers) and want to consolidate trap-based event signals into the Dynatrace UI alongside their APM/infra signals.
- **Licensing**: closed, commercial. Trap ingestion consumes Grail storage and is metered under the **Events powered by Grail (DPS)** licence:
  > "Retention beyond the included timeframe is billable as Events powered by Grail - Retain."
  > — `docs.dynatrace.com/docs/license/capabilities/events`

## 2. Trap-Subsystem Architecture

### 2.1 Components

| Component | Where it runs | Role for traps |
|---|---|---|
| **ActiveGate** | Customer-managed VM/container; multi-role | Hosts the EEC; provides network reachability between trap-sending devices and Dynatrace SaaS |
| **Extension Execution Controller (EEC)** | Inside ActiveGate (also OneAgent, but not for traps) | Loads extension YAML manifests; spawns data source processes; routes ingested data to Dynatrace cluster |
| **SNMP Traps data source process** | Child process of EEC on ActiveGate | Binds UDP port; decodes traps; translates OIDs against the per-extension MIB resources; emits metric datapoints + log events; reports its own state back to EEC |
| **Bundled extension OID set** | Inside the SNMP Traps extension package (signed and installed via the Hub) | Fixed predefined OID list shipped with the extension. The bundled SNMP Traps extension uses ONLY this set — it does NOT load operator-placed files from `mib-files-custom/`. |
| **`mib-files-custom/` directory** | On ActiveGate filesystem | Operator-supplied MIB files. **Consumed by operator-built custom extensions only**, not by the bundled SNMP Traps extension. |
| **Per-extension `snmp/` MIB directory** | Inside a custom extension package next to `extension.yaml`; runtime location `runtime/datasources/working_directories/<id>/snmp` on the ActiveGate | Extension-packaged MIBs used by that specific custom extension; an alternative to `mib-files-custom/` for shipping MIBs with the extension instead of replicating them per ActiveGate. |
| **Dynatrace cluster (Grail)** | SaaS or Managed | Stores log events, runs DQL queries, applies log-event rules, generates Davis problems |
| **Davis AI** | SaaS / Managed control plane | Correlates events and metrics; opens/closes problems |

The EEC-to-data-source IPC is not documented. The EEC-to-ActiveGate-core IPC uses the **EEC ingest port 9999** (HTTP, localhost on the ActiveGate):
> "By default, the EEC sends data via port 9999, which is used by ActiveGate."
> — `docs.dynatrace.com/docs/ingest-from/extensions/advanced-configuration/eec-custom-configuration`

### 2.2 ASCII diagram

```
                       CUSTOMER NETWORK (per ActiveGate group)
   +------------------------------------------------------------------------+
   |                                                                        |
   |   Network devices (routers, switches, firewalls, load balancers)       |
   |          |                                                             |
   |          | SNMP trap PDU (UDP, default port 162)                       |
   |          v                                                             |
   |   +-------------------------------------------------------------+      |
   |   |  ActiveGate (VM / container / k8s)                          |      |
   |   |                                                             |      |
   |   |  +-------------------------------------------------------+  |      |
   |   |  | Extension Execution Controller (EEC)                  |  |      |
   |   |  |   ingest port: localhost:9999                         |  |      |
   |   |  |                                                       |  |      |
   |   |  |   +------------------------------------------------+  |  |      |
   |   |  |   | SNMP Traps data source process                 |  |  |      |
   |   |  |   |   bind: <bind_addr>:<configured_port>          |  |  |      |
   |   |  |   |   versions: v1, v2c, v3 (USM)                  |  |  |      |
   |   |  |   |   CIDR-filter source IPs                       |  |  |      |
   |   |  |   |   community/USM verify                         |  |  |      |
   |   |  |   |   parse BER PDU                                |  |  |      |
   |   |  |   |   resolve OIDs:                                |  |  |      |
   |   |  |   |     - bundled SNMP Traps ext: predefined OIDs |  |  |      |
   |   |  |   |     - custom ext: extension `snmp/` dir or    |  |  |      |
   |   |  |   |       ActiveGate `mib-files-custom/`          |  |  |      |
   |   |  |   |   emit:                                        |  |  |      |
   |   |  |   |     - metric  traps.count (default feature set)|  |  |      |
   |   |  |   |     - log event (Events feature set)           |  |  |      |
   |   |  |   |       attrs: log.source=snmptraps,             |  |  |      |
   |   |  |   |              loglevel=NONE, snmp.version,      |  |  |      |
   |   |  |   |              snmp.trap_oid, device.address,    |  |  |      |
   |   |  |   |              dt.source_entity, <varbinds>      |  |  |      |
   |   |  |   +------------------------------------------------+  |  |      |
   |   |  |                                                       |  |      |
   |   |  +-------------------------------------------------------+  |      |
   |   |                                                             |      |
   |   +-------------------------------------------------------------+      |
   |                              |                                          |
   +------------------------------|------------------------------------------+
                                  | HTTPS (mTLS) to Dynatrace cluster
                                  v
                +-----------------------------------------------+
                | Dynatrace SaaS / Managed                      |
                |                                               |
                |   Grail (log/event store)                     |
                |     log.source:snmptraps                      |
                |     DQL: fetch logs | filter ...              |
                |                                               |
                |   Log event rules  --->  Davis problem        |
                |                                               |
                |   SmartScape topology  ---  device entities   |
                |     (from dt.source_entity / device.address)  |
                |                                               |
                |   Dashboards, Unified Analysis page,          |
                |   Logs Viewer, Notebooks                      |
                +-----------------------------------------------+
```

### 2.3 Deployment model

- **Customer-side compute on ActiveGate**. The ActiveGate is a long-lived process (systemd service `dynatracegateway` on Linux; Windows service equivalent) installed by the customer on dedicated hosts, VMs, or containers. No Dynatrace-hosted trap receiver exists — every trap is received on customer infrastructure.
- **HA via ActiveGate groups**. ActiveGates are organized into named groups. A SNMP traps monitoring configuration is scoped to a group:
  > "All of the ActiveGates from the group will run this monitoring configuration at a time."
  > — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

  The literal reading is that **every** ActiveGate in the group runs the trap listener simultaneously. The docs do not explicitly state whether trap **content** is deduplicated server-side when multiple ActiveGates in the same group see the same trap; the safest assumption is that operators configure devices to send to a single ActiveGate address (often via VIP/load-balancer) to avoid duplicate ingest. This is a meaningful operational footgun that the docs do not flag.
- **Container deployment**. Dynatrace ships an **ActiveGate container image** ("Containerized ActiveGate"). However, per the ActiveGate capability matrix (`docs.dynatrace.com/docs/ingest-from/dynatrace-activegate/capabilities`), **"Monitoring using an ActiveGate extension" is NOT applicable on containerized ActiveGate** — only host-based Environment ActiveGate (Linux/Windows on x86-64) is shown as applicable. The capability matrix marks containerized, s390 Linux, and arm64 Linux as "Not applicable" for ActiveGate-extension execution.
- **Kubernetes**. Similarly **not supported** for the SNMP Traps data source. The Extensions limits page (`docs.dynatrace.com/docs/ingest-from/extensions/extension-limits`) states: "In Kubernetes environments, only JMX extensions are supported." There is no documented path for running the SNMP Traps data source on a Kubernetes ActiveGate.

In practice, SNMP trap reception requires a **host-based Environment ActiveGate on x86-64 Linux or Windows**. The Containerized/Kubernetes ActiveGate variants exist for other Dynatrace use cases (Kubernetes-cluster monitoring, cloud-API gateways) but cannot host SNMP-trap extensions per the vendor capability matrix.

### 2.4 Languages, libraries, and packaging

- **Closed-source binaries**. The EEC and the SNMP traps data source binary are shipped by Dynatrace; the implementation language and the SNMP/MIB libraries used are not disclosed in public documentation. This document does NOT speculate on the language. The community-thread evidence for the binary's existence on Linux (`dynatracesourcesnmp`) and Windows (`dynatracesourcesnmptraps.exe`) suggests a per-data-source executable, but the language and libraries used to build it are unverifiable from public material.
- **Extension packaging**. Extensions are signed packages containing the declarative YAML (`extension.yaml`) plus optional Python data-source code for non-SNMP extensions. SNMP traps are declarative-only — no Python. The signing chain is the Dynatrace Hub signing CA.

### 2.5 Inter-component IPC

- **Device → ActiveGate**: UDP/162 (configurable per source).
- **Data source process → EEC**: not documented; internal to the ActiveGate process tree.
- **EEC → Dynatrace cluster**: HTTPS to the Dynatrace cluster endpoint; the EEC uses the parent ActiveGate's authenticated HTTPS channel. Trap log events are emitted via the same **log ingest** API that any other extension log emits to.
- **Local EEC port**: TCP/9999 on localhost (configurable via `ingestport`).

## 3. Trap Reception (UDP/162 Ingress)

### 3.1 Listener implementation

> "SNMP Traps extensions use a datasource that binds to and listens on the configured UDP port, with incoming packets filtered by the configured IP address and mask, and verified against provided credentials (v1, v2c, or v3)."
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

The data source is the **direct UDP socket binder**, not a `snmptrapd` front-end. The bind address (interface) and port are both monitoring-configuration parameters.

### 3.2 Default port

The docs do **not** designate a single default port. The example YAML and JSON payloads in the schema reference show two ports across examples: **162** (the IANA-assigned SNMP trap port) and **8162** (a non-privileged variant). The community troubleshooting guide treats 162 as the standard but documents that ActiveGate cannot bind to 162 without root or `CAP_NET_BIND_SERVICE`:

> "to listen on port 162, you have to have root permission."
> — `community.dynatrace.com/t5/Extensions/SNMP-Traps-Port-unable-to-bind-on-162/`

Conclusion: in practice, customers configure either 162 (with capability granted) or a high port (often 1162 or 8162) with iptables redirection from 162.

### 3.3 SNMP version support

| Version | Authentication | Minimum Dynatrace platform | Minimum ActiveGate (documented) |
|---|---|---|---|
| v1 | community string only | 1.236+ | 1.235+ (SNMPv2c and earlier) |
| v2c | community string only | 1.236+ | 1.235+ |
| v3 | USM (NoAuthNoPriv / authNoPriv / authPriv) | 1.236+ | 1.251+ |

The minimum-version line covers both SNMPv1 and SNMPv2c under the same "SNMPv2c and earlier" wording on the data source page. Strictly, the docs do not call out a separate minimum for SNMPv1; reviewers should treat v1 as supported from the same 1.235 baseline as v2c. The Dynatrace platform minimum (1.236+) applies for all three versions and is the floor for the entire SNMP Traps data source.

> "SNMPv1 and SNMPv2 are authenticated using community name only, while SNMPv3 requires advanced authentication."
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

**Inform requests are explicitly NOT supported**:
> "The SNMP traps data source supports only SNMP traps. SNMP inform requests aren't supported."
> — same page

This is a real comparative gap relative to the open-source NMS family. OpenNMS, Zenoss, and Centreon all support INFORMs (which require a reply PDU). The SaaS-tier peer file `datadog-agent.md` does not document INFORM behaviour either way — neither for explicit support nor for explicit rejection — so the comparative finding to draw cleanly is: **Dynatrace explicitly drops INFORMs (vendor-stated); Datadog's INFORM behaviour is not documented in the analysed peer file**. The cross-system INFORM comparison should be deferred to the final comparison-matrix document where each system's INFORM support is sourced individually.

### 3.4 Privileged-port handling

Same model as Datadog. Two operator-side patterns:

1. **`setcap`** on the data source binary (or the EEC parent), granting `cap_net_bind_service=+ep`. This is the standard Linux capability pattern; it is **mentioned in passing by community Dynatrace personnel responses** but is not in the primary docs as a recommended approach — treat as community guidance, not vendor-documented.
2. **iptables PREROUTING REDIRECT** from UDP/162 to a high port:
   > "`sudo iptables -t nat -A PREROUTING -p udp --dport 162 -j REDIRECT --to-port 1162`"
   > — community troubleshooting guide

The community troubleshooting guide further calls out **SELinux** as a frequent culprit:
> "check SELinux settings to ensure dynatracesourcesnmp is allowed to bind to ports lower than 1024."

The binary name `dynatracesourcesnmp` is the first concrete confirmation that the trap data source is a **distinct executable** (`/var/lib/dynatrace/.../dynatracesourcesnmp` or similar), not a thread inside the EEC parent. This is consistent with the EEC's documented per-data-source CPU/RAM limits (see §10).

### 3.5 Concurrency and performance

Published throughput numbers (verbatim from the SNMP traps data source page, ActiveGate sizing labelled `c5.large` by Dynatrace). These are **Dynatrace-published benchmark numbers**, not independently reproduced; treat as upper-bound vendor guidance.

| Profile (c5.large ActiveGate) | SNMPv2c, logs disabled | SNMPv2c, logs enabled | SNMPv3, logs disabled | SNMPv3, logs enabled |
|---|---|---|---|---|
| Default profile | 45,000 traps/min | 30,000 traps/min | 32,000 traps/min | 17,000 traps/min |
| High-performance profile | 150,000 traps/min | 75,000 traps/min | 105,000 traps/min | 60,000 traps/min |

Quoted operator-visible failure mode under overload:
> "If a large number of traps is sent, it is possible that many of them may be dropped by the operating system. The same situation occurs when the limit with logs is reached."
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

This wording matters: the data source does **not** implement application-level backpressure or queue management beyond the SO_RCVBUF the OS gives it. Overflows are silent drops. There is no documented metric exposing the OS-level UDP-receive drop counter; operators must monitor the ActiveGate host with `ss -uan` or `netstat -su` externally. The community troubleshooting guide does, however, name two SFM (Self-Monitoring Framework) metric keys that surface ingest-side drops at the EEC / cluster ingest tier — see §15.5.

### 3.6 HA / clustering

- Trap-listener HA is achieved by deploying multiple ActiveGates in the same group. Per the quoted text in §2.3 every ActiveGate in the group runs the same monitoring configuration. Two operational patterns are possible but the docs do not prescribe one:
  1. **Single-target**: devices send only to one ActiveGate; the others are warm spares; failover requires reconfiguring device-side trap destinations (manual, slow).
  2. **Multi-target**: devices send to all ActiveGates in the group; every trap arrives N times; deduplication must happen in the SaaS tier (via DQL grouping in dashboards/queries) since the data source itself does not dedup across ActiveGates.

No anycast / VIP / leader-election story is documented. Reviewers should treat trap-listener HA as an **operator responsibility**, not a Dynatrace feature.

## 4. MIB Management

### 4.1 Storage location and audience

Two MIB directories on the ActiveGate:

**Default MIB files** (shipped with ActiveGate, used by the EEC's data source on startup):

> "Linux default MIBs: `/opt/dynatrace/remotepluginmodule/agent/res/mib-files`"
> "Windows default MIBs: `C:\%PROGRAMFILES%\dynatrace\remotepluginmodule\agent\res\mib-files`"
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmp-schema-reference`

**Custom MIB files** (operator-supplied, consumed only by custom extensions per §4.2):

> "Linux: `/var/lib/dynatrace/remotepluginmodule/agent/conf/userdata/mib-files-custom/`"
> "Windows: `C:\%PROGRAMDATA%\dynatrace\remotepluginmodule\agent\conf\userdata\mib-files-custom\`"
> — `docs.dynatrace.com/docs/ingest-from/extend-dynatrace/extend-metrics/ingestion-methods/snmp`

The `mib-files-custom/` directory exists on each ActiveGate. Per the same vendor page:

> "These MIB files are then used by all the SNMP and SNMP Traps extensions running on this ActiveGate"
> — same page

…BUT the same page restricts that usage to **operator-built (custom)** extensions:

> "Dynatrace out-of-the-box SNMP extensions come with a predefined set of OIDs and do not dynamically load additional MIB files."
> — same page

The reconciliation: `mib-files-custom/` is the storage location; the **consumer scope** is custom extensions only. The bundled "SNMP Traps" extension (Dynatrace Hub, version 2.1.1) and the bundled "Generic network device" extension both ship a fixed predefined OID set baked into the extension package, and ignore `mib-files-custom/`. There is no central MIB store managed from the Dynatrace UI; operators who do want runtime MIB loading must replicate files to every ActiveGate in the group that runs a custom trap extension referencing those MIBs.

### 4.2 Bundled MIBs out of the box

The bundled SNMP Traps extension ships a **fixed, predefined set of OIDs** (extension version 2.1.1 per the Hub listing). The vendor docs do not enumerate that list. From the extension's positioning as the canonical "port flapping" use case (`docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-traps-statistics`), at least `IF-MIB::linkDown` and `IF-MIB::linkUp` are inferable; broader MIB coverage is unverifiable from public material.

For trap-OID translation beyond the bundled set, operators must:

1. Author a custom Extensions 2.0 trap extension referencing the OIDs of interest.
2. Place the vendor MIB files in `mib-files-custom/` on every ActiveGate in the target group.
3. Sign and upload the custom extension; activate a monitoring configuration scoped to that group.

This is a meaningful comparative gap. OpenNMS compiles user MIBs at runtime into the global trapd dictionary; Zenoss ships ZenPacks (vendor MIB bundles) that the global zentrap loads automatically; Centreon's `centFillTrapDB` ingests MIB files into a central DB consumed by all `centreontrapd` instances. Dynatrace requires per-extension authoring + per-ActiveGate file replication for arbitrary vendor coverage.

### 4.3 Compilation and load pipeline

There are two MIB-shipping locations for custom extensions, both consumed at extension load time:

1. **Extension-packaged MIBs**: place vendor MIBs in an `snmp/` directory inside the extension package, alongside `extension.yaml`. At runtime they live under `runtime/datasources/working_directories/<extension-id>/snmp` on the ActiveGate. Source: `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmp-schema-reference`. This is the preferred pattern for distributing an extension with its MIBs because the MIBs travel with the extension package.
2. **ActiveGate `mib-files-custom/`**: per-ActiveGate filesystem MIBs (see §4.1). Suitable for operator-supplied MIBs that span multiple custom extensions.

No `mib2c`-style compiler step is described in the public docs; the data source loads MIBs directly. Adding MIBs to a running custom extension requires an **EEC restart** (not a full ActiveGate restart):

> "EEC restart requirement when adding custom MIBs for an already-running extension, without restarting ActiveGate"
> — paraphrased from `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmp-schema-reference`

> "The files stored in the `mib-files-custom` directory are preserved between updates."
> — `docs.dynatrace.com/docs/ingest-from/extend-dynatrace/extend-metrics/ingestion-methods/snmp`

Whether mistyped MIBs cause the data source to fail-closed, fail-open, or skip just the offending file is **not** explicitly documented; the community troubleshooting guide enumerates parsing pitfalls (see §15.6) but does not specify the failure mode for the whole MIB load.

### 4.4 User workflow for adding MIBs

For a **custom-built** trap extension:

1. Choose one of:
   - **Bundle MIBs with the extension**: place them in `snmp/` next to `extension.yaml` in the extension source tree. They travel with the signed extension package, no per-ActiveGate replication needed.
   - **Per-ActiveGate `mib-files-custom/`**: replicate MIB files to every ActiveGate in the target group. Useful when multiple custom extensions share a MIB.
2. Build a custom Extensions 2.0 extension whose `snmptraps` YAML node references the OIDs of interest, with metric/event mappings.
3. Sign and upload the extension via the Extensions API or Dynatrace Hub.
4. Activate a monitoring configuration scoped to the ActiveGate group.
5. If MIBs are added or changed for an already-running custom extension, **restart the EEC** on each affected ActiveGate (a full ActiveGate restart is not required).

For the bundled extension: there is no MIB-add workflow. The user is expected to either accept the bundled translations or fork into a custom extension.

### 4.5 Fallback behaviour for unknown OIDs

Not explicitly documented. Inference from the log-event structure: the `snmp.trap_oid` attribute is populated; if the OID cannot be resolved against the active MIB set, the attribute presumably contains the numeric OID. The published example output in the schema reference shows `"CISCO-SMI::ciscoMgmt"`-style symbolic translation when MIB resolution succeeds — implying numeric fallback when it fails. This is **inference**, not a verbatim quote.

### 4.6 Suffix trimming for table OIDs

The data source supports OID suffix trimming for variable bindings whose OIDs include dynamic table-index suffixes:

> "In some SNMP traps, variable binding OIDs have dynamic parts at the end that change with each incoming trap."
> "`suffixLen`—specifies the number of octets at the end of the OID that should be trimmed."
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

Comparison: Datadog's `oid_resolver.go` implements an automatic "climb up the OID tree" algorithm to resolve table varbinds without operator configuration. Dynatrace requires the operator to specify `suffixLen` per OID in the extension YAML — more configuration burden, but more predictable.

## 5. Trap Processing Pipeline

### 5.1 Parse and source verification

The data source:

1. Receives UDP packet.
2. CIDR-filters the source IP against the configured `ip` field. Quoted requirement:
   > "The network that sends packets with traps provided in the CIDR notation. To configure a single interface address, add the `32` subnet mask after the IP address, for example `172.10.11.0/32`."
   > — same page
3. Verifies authentication: community string (v1/v2c) or USM credentials (v3).
4. BER-decodes the PDU and extracts variable bindings.
5. Resolves OIDs against the loaded MIB set.

### 5.2 OID-to-name resolution

Symbolic translation example (from docs): `"CISCO-SMI::ciscoMgmt"`-style names appear as values of OID-derived attributes. Translation is per-data-source-process and uses the union of bundled OIDs + `mib-files-custom/` MIBs (with the caveat in §4.2 for first-party extensions).

### 5.3 Source identification

Two paths:

- **`device.address`** attribute on the emitted log event — the source IP of the trap packet (the UDP sender, which for v1 may also be encoded inside the PDU as `agent-addr`; the docs do not split the two).
- **`dt.source_entity`** — the Dynatrace entity ID of the device the trap is accounted to. Per §8.3 and the v2.1.0 Hub release notes, this entity may be one the SNMP Traps extension itself created directly from trap data (from v2.1.0 onwards), or a related polled-device entity tied via `same_as` relations when the Generic network device polling extension is also active. Per-version behavioural caveats:
  - **v2.1.0+**: the trap extension mints `network:device` entities from trap data alone; `dt.source_entity` is reliably populated.
  - **v2.0.1 and earlier**: the trap extension only displayed trap data on polled-device entities; `dt.source_entity` populated only when prior polling discovery had occurred.
  - Fallback behaviour when no entity can be created or matched is not publicly specified.

This is structurally similar to OpenNMS's "interface → node" lookup, but Dynatrace's lookup operates against the SmartScape topology graph rather than a static device inventory table.

### 5.4 Enrichment

- Variable bindings are flattened into log-event attributes (one varbind per attribute).
- Extension-defined variables (from the `vars` block in the monitoring configuration) are added as additional attributes.
- The data source does **not** join against topology, device inventory, or last-known-state at the ActiveGate. Enrichment beyond MIB OID translation happens **in Grail / Davis on the SaaS side** via log-processing rules.

### 5.5 Normalisation

There is no severity normalisation at the data source. The emitted log event carries a `loglevel` attribute, which on the Dynatrace logs schema represents the **log status** (e.g. `INFO`, `WARN`, `ERROR`, `NONE`), and is hard-coded to `NONE` for traps:

> "main attributes: `loglevel` (always `NONE`)"
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

`loglevel` is the closest thing to a severity field on the trap log event, but it is fixed to `NONE`, and there is no separate severity attribute (`severity_text`, `event.severity`, or similar) on the emitted event.

Operators who want severity must add a **log-processing rule** in Grail that pattern-matches the trap content (OID, varbinds) and assigns a severity attribute, or rely on a log event rule that promotes specific OIDs to Davis problems with a rule-defined severity.

For SaaS-tier comparison: per `datadog-agent.md` §5 / §9, the Datadog Agent also does not normalise trap severity at the host — severity is a Logs Monitor concept on the SaaS side. So both Dynatrace and Datadog **defer severity to SaaS-side rules / monitors**. The contrast is with the open-source NMSes (OpenNMS, Zenoss, CheckMK Event Console, Centreon), all of which carry a vendor-mapped severity from MIB compilation through to the alarm record.

### 5.6 Deduplication and suppression

**Not implemented at the data source.** The docs make no mention of dedup keys, suppression windows, rate limits, or storm-detection at the ActiveGate. The Davis-AI blog explicitly cites the "trap counter metric" as a **workaround** to deal with storms:

> "leverage a trap counter metric to raise an alert for any event generated using a trap counter metric that shows a trap storm."
> — `dynatrace.com/news/blog/accelerate-resolution-of-network-issues-with-ai-powered-event-reporting/`

That is: the operator must configure a metric-based anomaly detection rule to detect "too many traps" as a separate signal. The trap pipeline itself emits every trap. No suppression. No "first / N-th of M / clear" semantics. This is a meaningful gap relative to Centreon's `centreontrapd` dedup, OpenNMS's `<auto-clean/>` and `<reductionKey/>`, or Zenoss's event-dedup engine.

### 5.7 Routing

Two outputs per trap, both flowing through EEC → ActiveGate → Dynatrace cluster ingest:

- **Metric**: `com.dynatrace.extension.snmp-traps-generic.traps.count` (a counter; vendor docs name the dimensions as "trap sender" and "trap OID"; `dt.source_entity` is a log-event attribute, not a documented metric dimension). Always emitted regardless of feature set selection.
- **Log event**: emitted when and only when the **Events feature set** is enabled. Persisted in Grail; queryable via DQL `fetch logs | filter log.source == "snmptraps"`.

### 5.8 Error handling

- Malformed PDU: not documented. Presumably silently dropped with a debug log entry; reviewers should treat this as a known unknown.
- Unknown OID: numeric OID preserved in the attribute (see §4.5).
- USM auth failure: trap dropped; community-doc indicates `eec.log` records the failure.
- CIDR mismatch: trap dropped before parse; no log event emitted.

The single documented OS-level overflow behaviour is silent kernel drop (§3.5).

## 6. Data Model & Persistent Storage

### 6.1 Per-feature storage matrix

| Feature | Storage | Engine | Retention | Schema |
|---|---|---|---|---|
| Raw trap PDU bytes | **Not stored** — only the parsed log event and count metric are persisted; no raw BER archive is documented | n/a | n/a | n/a |
| Trap raw count metric | Dynatrace **metrics store** (Grail metrics on Grail-enabled tenants; classic timeseries on Managed tenants pre-Grail-metrics) | Dynatrace metrics TSDB | Tenant retention policy; **not separately specified in the SNMP traps docs** — reviewers should consult the Dynatrace metrics retention docs (`docs.dynatrace.com/docs/discover-dynatrace/whats-new/metric-storage`) rather than treat any number quoted here as authoritative | Single metric key `com.dynatrace.extension.snmp-traps-generic.traps.count` + dimensions |
| Trap log events | **Grail** (Events powered by Grail) | Schemaless log/event store | **Operator-selectable retention**; metered per the DPS Events SKU | Open schema; attributes include `event.type`, `log.source`, `loglevel`, `snmp.version`, `snmp.trap_oid`, `device.address`, `dt.source_entity`, plus all varbinds |
| Device entities | **SmartScape topology** (Dynatrace's entity graph) | Internal graph store | Tenant-managed; specific retention not documented on the trap pages | Entity type, IP, name, tags, parent/child relationships |
| MIB definitions | On-ActiveGate filesystem (`mib-files-custom/`) | Plain files | "Preserved between updates" per `docs.dynatrace.com/docs/ingest-from/extend-dynatrace/extend-metrics/ingestion-methods/snmp` | ASN.1 SMIv1/SMIv2 source |
| OID→event mapping | Embedded in extension YAML (`snmptraps` node) + bundled extension package | Static YAML | Versioned per extension version | YAML schema |
| Dedup state | None (no in-product dedup) | n/a | n/a | n/a |
| Suppression rules | None at data source; SaaS-side **log processing rules** + **log event rules** for problem promotion | Grail rules engine | Tenant-managed | Rule DSL + DQL filters |
| Audit log | Standard Dynatrace audit log (covers Settings 2.0 and Extensions API actions per `docs.dynatrace.com/docs/manage/identity-access-management/audit-log`) | Dynatrace audit store | Tenant-managed; not separately specified per the trap docs | Generic audit event schema |

Caveat: the storage table mixes facts directly stated in the SNMP traps docs (Grail Events, the metric key, `mib-files-custom/` persistence) with cross-references to the broader Dynatrace platform docs (metrics retention, SmartScape, audit log). The SNMP traps pages themselves do **not** spell out metric retention, topology retention, or audit-log schema details. Treat the non-trap-doc cells as "platform behaviour, cross-referenced".

### 6.2 Grail schema for trap log events

Quoted attribute roster from the SNMP traps data source page:

| Attribute | Value semantics |
|---|---|
| `event.type` | Always `LOG` |
| `log.source` | Always `snmptraps` |
| `loglevel` | Always `NONE` |
| `snmp.version` | `1`, `2c`, or `3` |
| `snmp.trap_oid` | Symbolic or numeric OID of the trap |
| `device.address` | Source IP of the trap packet |
| `dt.source_entity` | Dynatrace entity ID of the device, if resolvable |
| `<varbind keys>` | One attribute per variable binding (key derived from OID, possibly suffix-trimmed) |
| `<vars>` | Extension-defined variables from the monitoring configuration |

### 6.3 Migration / upgrade handling

- Extensions are **versioned** in the Dynatrace Hub; release notes record per-version feature, dashboard, and entity changes (see the per-version highlights in §1.2).
- MIB files in `mib-files-custom/` are preserved across ActiveGate upgrades (quoted in §4.3).
- Grail events are immutable; no migration is needed for past trap log events.
- Metric-schema stability between minor versions is **not** publicly specified for the SNMP Traps extension. Operators relying on a specific metric-key/dimension contract should pin a specific extension version and validate before upgrading.

## 7. Configuration UX

### 7.1 Surfaces

- **Dynatrace Hub UI** (`dynatrace.com/hub`): browse and install extensions. The SNMP Traps extension is installed from here.
- **Custom Extensions Creator** (`dynatrace.com/hub/detail/custom-extensions-creator/`): web-UI extension builder for authoring custom SNMP Traps (and other) extensions without hand-writing YAML.
- **Extensions app in Dynatrace UI**: view installed extensions, monitoring configurations, status, logs.
- **Monitoring configuration wizard**: four-step UI to create a configuration:
  1. Select an ActiveGate group.
  2. Define trap sources (CIDR, port, version, credentials).
  3. Advanced properties (timeouts, OID overrides, suffix trims).
  4. Activate with metadata.
- **REST API** (Settings 2.0 / Extensions 2.0 APIs): JSON payloads matching the schema reference. Suitable for IaC (Terraform provider, Monaco config-as-code tool).
- **ActiveGate filesystem**: only MIB files (`mib-files-custom/`) and `extensionsuser.conf` (EEC custom configuration) are operator-touchable on disk.

### 7.2 Defaults the operator sees

- **Port**: the community troubleshooting guide calls **162** the default receiving port; schema examples on the primary docs also show non-privileged ports such as 8162. Whether the UI defaults the field to 162 or leaves it empty is not vendor-documented.
- **Feature set**: `default` (counter-only). Operator must explicitly select `Events` to ingest log events.
- **Interval**: minimum 1 minute, maximum 2880 minutes (48 h). The interval governs the trap counter metric aggregation window; it does **not** govern trap reception latency (traps are pushed; reception is near-real-time).
- **MIBs**: none — bundled OIDs only unless the operator builds a custom extension and ships MIBs.

### 7.3 Discoverability

- Schema-driven UI: the monitoring-configuration form is generated from the extension's schema, so every YAML field declared in the extension manifest is reachable from the GUI.
- Auto-completion and field-level validation behaviour (CIDR notation correctness, port range, SNMPv3 protocol selections, inline help) is observable in the Dynatrace UI but **not** authoritatively documented as a vendor-stated contract on the SNMP pages. Reviewers wanting a vendor citation should treat these as platform UI behaviours rather than SNMP-specific guarantees.

### 7.4 Live reload vs restart

Not explicitly stated. Activation/deactivation of a monitoring configuration is documented to take effect "after approximately 10 minutes" in pending-state troubleshooting guidance (`community.dynatrace.com/t5/Troubleshooting/SNMP-Traps-Troubleshooting-Guide/`). Reviewers should treat configuration change propagation as **eventually consistent, sub-15-minute** rather than instant.

### 7.5 Multi-tenancy / RBAC

- A Dynatrace tenant is a single SaaS environment. Multiple ActiveGate groups within a tenant can run independent trap configurations.
- RBAC: access to the Extensions app, the Settings API, the Hub, and Grail is governed by Dynatrace IAM. There is no separate ACL for "who can change SNMP trap monitoring configurations" — it falls under the general Extensions permission set.

### 7.6 Published configuration and scaling limits

Per the **Extensions limits** docs page (`docs.dynatrace.com/docs/ingest-from/extensions/extension-limits`):

| Limit | SNMP traps value |
|---|---|
| Groups (top-level `snmptraps` nodes in extension YAML) | **10** |
| Subgroups | Not applicable (SNMP traps does not support subgroups) |
| Dimensions per level | **5** (notably fewer than SNMP polling's 25) |
| Metrics per level | 100 |
| Metrics per extension (total) | 500 |
| Devices (= trap source declarations) per monitoring configuration | **100** |
| Interval | 1 minute minimum, 2880 minutes (48 h) maximum (per the data source page) |

These limits matter for Netdata-design comparison: a Dynatrace customer with more than 100 trap sources in a single CIDR group must either split into multiple monitoring configurations or use broader CIDR masks. Each ActiveGate group can carry multiple monitoring configurations, but the per-configuration cap is fixed.

## 8. Integration with Other Signals

### 8.1 Metrics

- **Yes, every trap increments a counter metric** (`com.dynatrace.extension.snmp-traps-generic.traps.count`). This is the default-feature-set output. The vendor docs describe the metric's documented dimensions as "trap sender and trap OID" rather than naming `dt.source_entity` as a dimension; `dt.source_entity` is documented for the log-event attribute layer, not as a metric dimension. Operators correlating the trap counter with polled SNMP metrics from the same device join on the trap-sender dimension and the device entity identifier surfaced through the bundled extension's per-device Unified Analysis screen.
- The trap counter is queryable in Dynatrace's metric explorer / Notebooks / dashboards and can power anomaly detection ("trap storm" detection per the Davis-AI blog).
- **The SNMP traps data source itself emits only the trap-count metric**. Per the vendor docs:
  > "SNMP traps collect just one metric that counts the number of traps sent by a source defined in your monitoring configuration during a defined interval."
  > — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

  Operators who want a metric **derived from a trap varbind value** (e.g. an interface index, an error count, an OID value carried in the PDU) must do so **downstream of ingest**, via Dynatrace's log-metric extraction on the resulting Grail log event. The Davis-AI blog spells this path out:
  > "extract a metric from any trap event attribute and track it as a regular metric or set up an alert"
  > — `dynatrace.com/news/blog/accelerate-resolution-of-network-issues-with-ai-powered-event-reporting/`

  So the trap-extension YAML cannot itself emit varbind-derived metrics — that capability belongs to the broader Logs/Events-to-metric extraction layer in Grail, not to the SNMP traps data source.

### 8.2 Alerting / Notifications

Three paths from trap to alert:

1. **Log event rules in Grail**: a DQL query that matches incoming trap log events; matching events are promoted to **Davis problems**. The rule defines whether each matched event creates an individual problem or all matches are merged into one ongoing problem. Primary docs: `docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-processing/lma-log-events` (event types, `event.unique_identifier` semantics, individual-vs-merged problem creation).
2. **Metric anomaly detection** on the trap counter or an extracted metric.
3. **Davis-AI baseline detection** on the trap-derived metric series (automatic baselining; no operator threshold required).

Once a Davis problem exists, the standard Dynatrace alerting/notification stack applies: integrations to PagerDuty, Slack, ServiceNow, Jira, email, webhooks, etc. — none of which are trap-specific.

**Notable absence**: no native **trap-as-alert-clear** semantics. Dynatrace's log-event docs (`docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-processing/lma-log-events`) describe merge and timeout-based active-event closure for log events; they do **not** document a stateful "clear this event when another trap arrives" mechanism. Operators wanting `linkUp`-closes-`linkDown` lifecycle have to approximate it with identifiers/timeouts or external automation — there is no native trap-pair clear handling. OpenNMS (`<resolve/>` reductionKey), Zenoss (event clear), and CheckMK Event Console all natively support trap-pair clear semantics; Dynatrace does not.

### 8.3 Topology

What the docs say (verbatim from primary sources):

> "Topology for SNMP trap devices, derived from the IP address of the agent that send the traps"
> — `docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-traps-statistics`

> "`dt.source_entity`—the ID of the device (entity) for which the log event is accounted."
> — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`

What the Hub release-history says (verbatim from `dynatrace.com/hub/detail/snmp-traps-statistics/`):

- **v1.1.4** introduced the **Events feature set**, the **topology definition**, and the **Unified Analysis screen** for the extension.
- **v1.2.0** introduced the **Logs section on the Unified Analysis screen** and the **`dt.ip_addresses` standard attribute**.
- **v1.2.2** added a new dashboard.
- **v2.0.1** added: "Trap data will be automatically displayed on generic network device entities" — meaning traps from a device that the Generic network device polling extension had already discovered would be auto-rendered on the polled-device entity screens.
- **v2.1.0** added: "`network:device` entities are created directly from the trap data, with `same_as` relations to their trap-entity counterparts". This is the version that gained **active entity creation from trap data alone** — without requiring prior polling discovery.

Synthesis:

- From **v2.1.0** onwards, the SNMP Traps extension actively creates `network:device` entities from trap source IPs, with `same_as` relations linking them to polled-device entities where those exist. This is changelog-confirmed, not Hub-marketing-only.
- Prior to v2.1.0 (i.e. v2.0.1 and earlier), the extension could **display** trap data on previously polled device entities but did **not** mint new entities from trap data alone.
- The trap log event's `dt.source_entity` attribute points at the entity created (or matched) by this mechanism.
- The Generic network device polling extension is **not a prerequisite** for the trap-side entity to exist — the SNMP Traps extension creates entities directly from trap data. Where both extensions run, the `same_as` relation ties the polled-device and trap-source-device records together.

What is **not** publicly specified and should not be claimed:

- The full lifecycle (TTL, decommission semantics) of trap-source-derived entities.
- Whether `dt.source_entity` is `network:device` exclusively, or whether other entity types are reachable through it.
- L2 / interface-level topology — Dynatrace does not document LLDP/CDP-based L2 topology at the trap data source. The Generic network device extension covers polled L3 IP-level device data; full L2 topology is not visible in the trap docs.
- Topology-aware suppression: not documented as a built-in feature. Operators can in principle implement it in DQL by joining trap events against topology relationships, but there is no out-of-box "suppress traps from devices whose parent is down" engine.

### 8.4 Logs / Events

- Traps live in **Grail** as log events under `log.source:snmptraps`. They are first-class citizens of the Logs Viewer, Notebooks, and DQL.
- Quoted operational guidance:
  > "In the Log Viewer, go to Logs and filter trap events by `log.source: snmptraps`."
  > — community search forum responses cited from `community.dynatrace.com`
- Retention is operator-selectable (DPS Grail Events Retain SKU). This is a clean separation from the metric retention.

### 8.5 Northbound forwarding

> "Event forwarding is not supported"
> — `docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-traps-statistics`

This is the single most consequential admission in the SNMP Traps extension docs. Dynatrace cannot re-emit a received SNMP trap to an upstream NMS. Operators with a manager-of-managers architecture (e.g. Dynatrace as an intermediate hub, IBM Netcool or BMC TrueSight as the top tier) cannot use Dynatrace as a trap relay. Datadog has the same gap (no `SendTrap` path). OpenNMS, Centreon, Zabbix, Zenoss, and LibreNMS all support northbound trap or webhook forwarding.

For comparison-matrix purposes: Dynatrace is a **trap sink**, never a trap source.

## 9. Severity Model

- **Vendor severity from MIB**: not parsed by default. MIB TRAP-TYPE definitions and SMIv2 NOTIFICATION-TYPE clauses can carry severity-like text, but the Dynatrace data source does not extract it as a structured attribute.
- **System severity**: hard-coded `loglevel: NONE` on every trap log event.
- **Customisation surface**: operator-defined **log-processing rules** in Grail can synthesise a severity attribute by matching `snmp.trap_oid` patterns or varbind values. This is a DQL/SQL-style transformation, not a native trap-severity mapping table.
- **Davis problem severity**: when a trap log event is promoted to a Davis problem via a log event rule, the rule itself declares the problem's severity (e.g. AVAILABILITY, ERROR, SLOWDOWN, RESOURCE_CONTENTION). The trap-derived severity is therefore **rule-defined**, not trap-defined.

This is the area where Dynatrace lags the open-source NMS family most visibly. Centreon ships a database of OID-to-severity rules; OpenNMS ships event definitions per MIB; Zenoss ZenPacks ship severity mappings; CheckMK Event Console has a built-in severity column. Dynatrace ships none of that for the bundled extension. Operators must do it themselves in DQL.

## 10. Storm / Volume Handling

### 10.1 Per-source rate limits

- **Not at the data source.** No documented per-source token bucket, sliding window, or `MaxLoad`-style cap.
- The data source's only defence is the kernel UDP receive buffer; overflows are silent (§3.5).

### 10.2 Dedup keys and windows

- **Not implemented at the data source.** Every trap becomes an event in Grail.
- **SaaS-side suppression** is available via Davis problem merging:
  > "configured to individually create a problem for each triggered log event or can be merged into one problem"
  > — community / docs synthesis on log event rules
- Operators set the merge window per log event rule; this is downstream of ingest, so it controls problem volume, not Grail event volume.

### 10.3 Circuit breakers

- EEC-level **`HIGH_CPU` status** is documented:
  > "`HIGH_CPU` status means the maximum allowed CPU consumption for the datasource module of the Extension Execution Controller (EEC) has been reached on the ActiveGate."
  > — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/troubleshooting`

  When the data source hits its CPU cap (default 5% on ActiveGate per EEC docs), the EEC throttles or surfaces a degraded state. The exact behaviour (does the data source still consume UDP? are traps dropped at the EEC ingest port or earlier?) is **not** documented.

### 10.4 Storm detection

- **Out-of-band only.** The "trap counter metric" can power an anomaly-detection rule that fires when trap volume spikes. There is no built-in trap-storm detector.

### 10.5 Backpressure

- **None.** Traps flow data source → EEC (port 9999) → ActiveGate → Dynatrace cluster. If the cluster ingest is slow, the EEC buffers; if the buffer fills, the docs do not specify behaviour. The most likely failure mode is OS-level UDP drop at the data source's socket.

The overall storm story is the **weakest section of Dynatrace's trap design**. Open-source NMSes (OpenNMS `<rate-limit/>`, Zenoss `event-dedup`, Centreon `centreontrapd` dampening) all ship native dedup/storm/rate-limit primitives; Dynatrace ships none.

## 11. Security

### 11.1 SNMPv3 USM

Algorithms supported (per the SNMP traps data source page, `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`):

- **Auth protocols (`authNoPriv` and `authPriv`)**: MD5, SHA, SHA224, SHA256, SHA384, SHA512. These are the RFC 3414 + RFC 7860 family of USM authentication algorithms (HMAC-96-MD5, HMAC-96-SHA-96, HMAC-128-SHA-224, HMAC-192-SHA-256, HMAC-256-SHA-384, HMAC-384-SHA-512).
- **Priv protocols (`authPriv` only)**: DES, AES, plus four extended-key variants. Dynatrace's documentation labels them precisely:
  - `AES192` and `AES256` use the **Blumenthal key extension** (RFC draft `draft-blumenthal-aes-usm-08` / "Blumenthal 3DES-EDE for SNMP USM" family).
  - `AES192C` and `AES256C` use the **Reeder key extension** (the Cisco-popularised "Reeder-style" localized key derivation).
  Both pairs are widely deployed across vendor SNMP stacks; neither is Dynatrace-proprietary.
- **Security levels**: `NO_AUTH_NO_PRIV`, `AUTH_NO_PRIV`, `AUTH_PRIV`.

Multiple USM users can be configured per monitoring configuration (the schema allows a list of authentication objects).

### 11.2 DTLS / TLSTM

Not documented as supported. SNMP-over-TLS/DTLS (RFC 6353) does not appear in any Dynatrace docs. Same gap as Datadog and Telegraf; same alignment with OpenNMS and Zabbix.

### 11.3 Credential storage

- Credentials are submitted to the Settings API as JSON.
- Once activated, they are obfuscated server-side:
  > "Authentication details passed to the Dynatrace API when activating a monitoring configuration are obfuscated and it's impossible to retrieve them."
  > — `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions`
- On the ActiveGate side, the data source receives the credentials over the EEC-to-data-source channel; on-disk storage format is not documented.

### 11.4 Access control on the trap subsystem

- No trap-subsystem-specific ACL. Permission to create/modify a SNMP traps monitoring configuration is granted via the general Extensions/Settings IAM policy.
- The ActiveGate filesystem (where MIB files live) is OS-level access-controlled — operators must protect it via standard Linux permissions; Dynatrace does not enforce additional protection.

### 11.5 Audit logging

- Configuration changes are recorded in the Dynatrace tenant audit log (generic extensions audit trail).
- Per-trap reception is recorded as a Grail log event — implicitly audit-grade for reception, but not authentication-event-level (failed USM attempts are visible only in `eec.log` on the ActiveGate, not in Grail).

## 12. Trap Simulation & Testing (in-source evidence)

This section is the bluntest comparative weakness for a docs-only spec: **there is no public source code, no public test fixtures, no public CI workflow.**

What is documented operator-side:

- **Manual smoke test**: send a test trap with `snmptrap` (Net-SNMP CLI) to the configured ActiveGate IP and port. Community guidance:
  > "Verify network connectivity from the device sending SNMP Traps to the ActiveGate using curl with telnet protocol, as firewalls or similar tools might block the datasource from receiving traps."
  > — community troubleshooting guide
- **Self-monitoring metrics ("SFM" – Self Monitoring)**:
  > "SFM logs are available in the health tab on the Extension page in the Extensions app or on Extension page in the Dynatrace Hub."
  > — troubleshooting docs
  SFM exposes per-extension health and counters but specific trap-pipeline counter names are not enumerated in the public docs.
- **Data source logs**:
  > "Linux ActiveGate: `/var/lib/dynatrace/remotepluginmodule/log/extensions/datasources/<directory_corresponding_to_the_used_extension>`"
  > — same source

Internal Dynatrace test coverage of the trap pipeline is unknown and unverifiable from public material. Reviewers must not infer test rigour from product polish — the data source is well-documented from a config standpoint but its test discipline is invisible to external observers.

Operator-side validation (in lieu of in-source fixtures):

- Net-SNMP `snmptrap` CLI for hand-crafted PDU emission against the ActiveGate listener.
- The community troubleshooting guide includes step-by-step `tcpdump` and `curl --telnet` instructions for verifying that traps reach the ActiveGate interface before they reach the data source. No vendor-shipped trap-corpus or synthetic-trap generator is documented.
- SFM metrics (see §15.5) are the canonical pipeline-health signals operators have to verify reception did not drop at the ingest tier.

CI workflow: **Not applicable** — Dynatrace is closed-source and publishes no public CI definitions for the SNMP Traps data source.

## 13. Out-of-the-Box Coverage (defaults)

### 13.1 MIBs bundled

- The bundled "SNMP Traps" extension ships with a "predefined set of OIDs" (quoted in §4.2). The exact list is not enumerated in the public docs; the extension page lists "port flapping detection" as the canonical example use case, suggesting `IF-MIB::linkDown` / `IF-MIB::linkUp` are covered.
- No commitment to vendor MIB breadth (Cisco, Juniper, Arista, F5, Palo Alto, etc.) is published.

### 13.2 Severity rules bundled

- None as a structured table. The bundled extension does not ship severity mappings; `loglevel` is `NONE` for every trap.

### 13.3 Dedup defaults

- None.

### 13.4 Vendor packs / integration packages

- Beyond the bundled "SNMP Traps" extension, vendor-specific trap coverage exists only as third-party or operator-built custom extensions on the Hub. The Hub lists separate Cisco, Juniper, Arista, Palo Alto, Fortinet, F5, and other network-device extensions; these are **predominantly SNMP-polling** extensions targeting the Generic-network-device data source family. Reviewers checking specific vendor coverage should open each Hub listing and read its data-source declarations: the SNMP traps data source (the one analysed here) is a separate node in the YAML from the SNMP polling node, and many vendor extensions ship only the polling node. The bundled first-party "SNMP Traps" extension is the only Dynatrace-authored extension that uses the traps data source as its primary content.

### 13.5 Sample/preset dashboards or reports

- A bundled dashboard ships with the SNMP Traps extension:
  > "A dashboard that offers a monitoring overview for the SNMP traps that are received."
  > — `docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-traps-statistics`
- A **Unified Analysis page** exists for "SNMP trap devices" — a per-device deep-dive view auto-rendered from the device entity + its trap events.
- No bundled log-event rules / Davis problem rules ship with the extension. Operators must configure their own.

## 14. User Customization Surface

### 14.1 Custom OID handlers

Two paths:

1. **Custom Extensions 2.0 trap extension**: build a YAML manifest declaring the OIDs of interest, mapping the varbinds to **log event** content (attribute names, formatting, suffix trimming) and to the single trap-count metric (per §8.1, the trap data source itself emits only that count metric — varbind-derived metrics are produced downstream by log-metric extraction). Sign and upload to the tenant. This is the supported path for vendor-specific trap coverage beyond the bundled extension.
2. **Log processing rules in Grail**: post-ingest enrichment, parsing, severity assignment, attribute extraction. Operates on every log event in the namespace.

### 14.2 Custom MIBs

- `mib-files-custom/` directory (§4.1). Per-ActiveGate; consumed by custom extensions.

### 14.3 Custom severity rules

- Implemented as Grail log-processing rules or Davis log event rules. There is no GUI for trap-OID-to-severity mapping.

### 14.4 Custom dedup / problem-merge rules

- **No event-ingest dedup**. Every received trap that matches an Events feature set is stored as an independent log event in Grail; trap duplicates from the same OID/source are not deduplicated at ingest.
- **Problem merge / alert-noise reduction**: Davis log event rules can be configured so that multiple matching log events are folded into a single ongoing Davis problem (instead of creating one problem per matching trap). This is **problem-level merge**, not trap-event-level dedup; it reduces alert noise but leaves the underlying trap events in Grail. Source: `docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-processing/lma-log-events`.

### 14.5 Plugin / extension model

- **Extensions 2.0**: declarative YAML + optional Python (for non-SNMP data sources). SNMP traps are YAML-only.
- **Custom Extensions Creator** (Dynatrace Hub app, `dynatrace.com/hub/detail/custom-extensions-creator/`): first-party web-UI builder that can create, edit, build, sign, and upload custom Extensions 2.0 from inside the Dynatrace tenant — its release notes explicitly list editor support for **SNMP Traps**. This is the operator-friendly path to a custom trap extension when YAML-by-hand is undesirable.
- **Monaco (Configuration as Code)**: official tool to manage extension monitoring configurations declaratively in Git.
- **Terraform provider**: official Dynatrace provider supports Extensions monitoring configurations.

### 14.6 API surface for automation

- Settings 2.0 API for monitoring configurations (CRUD).
- Extensions API for extension upload, signing, listing.
- Grail / DQL API for querying ingested events.
- Problems API for Davis problems including those derived from trap log events.

## 15. End-User Value Analysis

### 15.1 Day-1 value with defaults

Activating the bundled SNMP Traps extension with the default feature set yields:

- A per-source counter of received traps.
- A pre-built dashboard showing trap volume by source.
- Topology cross-link if the device entity exists.

The operator gets **a metric saying "this device sent N traps in the last minute"**. No trap content is in Grail; no problem will fire from a trap; no severity is visible.

### 15.2 What requires customization to get useful value

To get day-2 operational value:

1. Enable the **Events** feature set (one-click).
2. Configure **log event rules** in Davis to promote specific `snmp.trap_oid` values to problems with explicit severity and per-rule merge windows.
3. Place vendor MIBs in `mib-files-custom/` on every ActiveGate (replication is manual).
4. Build a custom extension to decode vendor-specific trap OIDs into structured varbinds, OR rely on numeric OIDs in DQL queries.
5. Configure log-processing rules to synthesise severity.
6. Build dashboards in the Logs/Notebooks UI for trap exploration.

The on-ramp from "received" to "operationally actionable" is substantial. Operators coming from OpenNMS, Zenoss, or Centreon will find Dynatrace's bundled trap-to-problem path thin — the SaaS platform's primary strength is Davis correlation over downstream events, not the trap pipeline's built-in normalization.

### 15.3 Learning curve

- Easy to install (Hub button).
- Easy to configure for reception (CIDR + community / USM).
- Hard to make useful (log event rules + DQL + custom extensions if vendor coverage needed).
- Requires familiarity with three different domains: Extensions 2.0 YAML, Grail/DQL, Davis problem rules.

### 15.4 Operational toil

- **Per-ActiveGate MIB replication**: manual.
- **Per-tenant log event rule curation**: manual.
- **Trap-storm response**: manual (no native dedup).
- **Alarm-clear pairing for link-up/link-down style events**: manual.

### 15.5 Visibility into the pipeline's own health

- SFM (Self Monitoring Framework) provides per-extension health on the Extensions app's Health tab.
- Two SFM metric keys called out in the community troubleshooting guide that operators should monitor:
  - `dsfm:server.metrics.rejections` — incremented when ingested metric datapoints are rejected (typically because the tenant's DDU/DPS metric budget is exhausted; sourced from `community.dynatrace.com/t5/Troubleshooting/SNMP-Traps-Troubleshooting-Guide/`).
  - `dsfm:extension.engine.logs_ingest.records_rejected` — incremented when log events from the extension are rejected at the log-ingest tier (capacity, schema validation, or tenant-side rules).
- `eec.log` on the ActiveGate captures EEC-level failures (configuration pending, datasource crashes, auth failures).
- Per-data-source logs at `/var/lib/dynatrace/remotepluginmodule/log/extensions/datasources/`.
- **No native OS-level UDP-drop counter exposure** — operators must monitor the ActiveGate host with external tooling for kernel drops.

### 15.6 Operator pitfalls (from the community troubleshooting guide)

- **Pending monitoring configurations** can sit in "pending" for up to ~10 minutes after activation. Operators waiting longer than that should regenerate the support archive and check `eec.log`.
- **DNS resolution for device-name display** requires at least two traps from the same address — the first trap is used to trigger a reverse-DNS lookup, the second is the first to display the resolved name. Mentioned in the community troubleshooting guide.
- **Source-IP filtering** matches the IP the data source actually observes on the wire. If a proxy or load-balancer rewrites the source IP, the CIDR filter must use the proxy/LB IP, not the device IP.
- **MIB parsing pitfalls** flagged in the community guide: missing `NOTIFICATION-TYPE` parents, excessive whitespace around `DEFINITIONS ::= BEGIN`, MIB files that fail to define names from the root, and the `SmiType is nil` failure mode. Operators should test custom MIBs in a non-production ActiveGate group first.
- **SNMPv3 credential character set**: the community guide advises avoiding special characters (`@`, `$`, `#`, `%`) in usernames and passwords, and pays attention to small details such as "`AES256C` vs `AES256`" — the two variants use different key-extension algorithms (Reeder vs Blumenthal, §11.1) and are NOT interchangeable.

## 16. Strengths

1. **Grail/DQL post-ingest query surface**. Once traps are in Grail, DQL can query, join, aggregate, and pivot across trap events and other log/event streams in the same tenant. Source: `docs.dynatrace.com/docs/analyze-explore-automate/logs`. The cross-system query-power ranking against peers (Splunk Connect for SNMP, Datadog Logs Explorer, etc.) is deferred to the final cross-system comparison document.
2. **Topology cross-link via `dt.source_entity`** — and **automatic entity creation from trap data since v2.1.0**. Per the Hub release notes, the SNMP Traps extension creates `network:device` entities directly from trap source IPs with `same_as` relations to entities from the polling extension. Trap log events carry the entity ID, enabling DQL/Notebooks correlation of trap events with polled metrics from the same device. Entity lifecycle (TTL, decommission) is not publicly specified. Sources: SNMP traps data source page; Hub release notes for v1.1.4 / v2.1.0.
3. **Trap-count metric emitted by the data source itself**. The trap counter is a real metric in the metrics store (not a derived log aggregate), so volume-based anomaly detection on trap rate works out of the box. Varbind-derived metrics, however, require downstream log-metric extraction (see §8.1). Source: SNMP traps statistics extension page.
4. **Declarative extension model**. Operators can define custom trap-OID handling in YAML without writing code, sign, and ship through the Hub. Source: Extensions 2.0 framework docs.
5. **Configurable route to Davis problems**. Once log event rules are written, matching traps promote to Davis problems with correlated context from co-located signals. Source: AI-powered event reporting blog.
6. **Broad SNMPv3 algorithm coverage** including HMAC-SHA-512 and AES-256 / AES-256C variants. Source: SNMP schema reference.

## 17. Weaknesses / Gaps

1. **No INFORM support.** Devices configured to require acknowledgement of important events cannot use Dynatrace as a destination. Source: SNMP traps data source page.
2. **No northbound trap forwarding.** Quoted: "Event forwarding is not supported." Source: SNMP Traps extension page.
3. **No native severity model.** `loglevel` is hard-coded `NONE`; severity must be synthesised in DQL/Davis rules. Source: SNMP traps data source page.
4. **No native dedup / suppression at the data source.** Storm response is a metric-anomaly workaround. Source: AI-powered event reporting blog (workaround is the documented solution).
5. **MIB coverage is operator-supplied for custom OIDs.** The bundled extension does not dynamically load `mib-files-custom/` MIBs:
   > "Dynatrace out-of-the-box SNMP extensions come with a predefined set of OIDs and do not dynamically load additional MIB files."
   Source: SNMP management docs.
6. **No alarm-clear pairing.** linkUp does not natively close linkDown. Operator must encode in DQL.
7. **HA semantics underspecified.** "All ActiveGates in the group run the configuration" plus no dedup means operators must arrange device-side single-target sending or accept duplicate ingest.
8. **Silent UDP drops under load.** OS-level drops are not exposed as a Dynatrace metric.
9. **`HIGH_CPU` EEC throttling is opaque.** Behaviour under throttle is not documented — does the listener stop reading UDP? Does the EEC ingest port back-pressure the data source? Unknown from docs.
10. **Per-ActiveGate MIB replication is manual.** No central MIB store managed from the UI.
11. **Closed-source ⇒ no community auditability of decode paths.** Any bug in MIB parsing or USM verification is invisible until Dynatrace publishes a fix.
12. **No public test fixtures or trap-corpus.** Operators evaluating Dynatrace for high-volume trap workloads cannot reproduce the published throughput numbers (§3.5) against their own MIB set without a tenant.

## 18. Notable Configuration Examples

Six excerpts that demonstrate the design.

### 18.1 Monitoring configuration JSON for SNMPv3 (from schema reference)

```json
{
  "authentication": {
    "type": "SNMPv3",
    "userName": "user",
    "securityLevel": "AUTH_PRIV",
    "authPassword": "****",
    "authProtocol": "SHA",
    "privPassword": "****",
    "privProtocol": "AES256C"
  }
}
```

Source: `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmp-schema-reference`. Demonstrates: multi-protocol USM, in-API obfuscation contract.

### 18.2 Default-feature-set YAML (counter only)

```yaml
snmptraps:
  - group: generic
    interval: {minutes: 1}
    featureSet: basic
    metrics:
      - key: number-of-traps-received
        value: calculated
        type: count,delta
```

Source: SNMP traps data source page. Demonstrates: even the minimal config produces a metric, not just a log.

### 18.3 Source filter (CIDR)

> "172.10.11.0/32"
> — SNMP traps data source page

Demonstrates: single-host filter is expressed with an explicit `/32` mask, applied to a host IP. (The example in the docs uses `172.10.11.0/32`, which is technically the `.0` host of that subnet; reviewers should treat the example as illustrative of the `/32` mask convention, not as a recommendation to listen on the `.0` host specifically.)

### 18.4 SELinux pitfall

> "check SELinux settings to ensure `dynatracesourcesnmp` is allowed to bind to ports lower than 1024"
> — community troubleshooting guide

Demonstrates: a separate executable named `dynatracesourcesnmp` runs the trap data source.

### 18.5 Privileged-port redirect

> "`sudo iptables -t nat -A PREROUTING -p udp --dport 162 -j REDIRECT --to-port 1162`"
> — community troubleshooting guide

Demonstrates: standard iptables workaround when ActiveGate cannot bind to 162.

### 18.6 EEC ingest port

> "By default, the EEC sends data via port 9999, which is used by ActiveGate."
> — `docs.dynatrace.com/docs/ingest-from/extensions/advanced-configuration/eec-custom-configuration`

Demonstrates: the on-host fan-in port between every data source on the ActiveGate and the cluster-bound HTTPS channel. A noisy SNMP trap data source competes here with every other extension running on the same ActiveGate.

## 19. Sources Examined

Vendor documentation (all URLs accessed 2026-05-22):

- `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions` — SNMP traps data source reference (primary source).
- `docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-traps-statistics` — bundled SNMP Traps extension page.
- `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmp-schema-reference` — SNMP data source schema reference (covers both polling and traps).
- `docs.dynatrace.com/docs/ingest-from/extend-dynatrace/extend-metrics/ingestion-methods/snmp` — Manage SNMP extensions (MIB file management).
- `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/troubleshooting` — SNMP extensions troubleshooting (EEC `HIGH_CPU`, fastcheck, etc.).
- `docs.dynatrace.com/docs/observe/infrastructure-observability/extensions/snmp-generic` — Generic network device extension (the polling counterpart).
- `docs.dynatrace.com/docs/extend-dynatrace/extensions20` — Extensions 2.0 framework overview.
- `docs.dynatrace.com/docs/ingest-from/extensions/advanced-configuration/eec-custom-configuration` — EEC custom configuration (port 9999, resource limits).
- `docs.dynatrace.com/docs/license/capabilities/events` — Events powered by Grail licence (retention billing).
- `docs.dynatrace.com/docs/analyze-explore-automate/logs` — Grail Log Management & Analytics overview.
- `docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-processing/lma-log-events` — Log events page (primary docs for the log-event-rule → Davis problem path, individually-vs-merged problem semantics).
- `docs.dynatrace.com/docs/ingest-from/extensions/extension-limits` — Extensions limits (SNMP traps groups/dimensions/devices caps).
- `docs.dynatrace.com/docs/ingest-from/dynatrace-activegate/capabilities` — ActiveGate capability matrix (extension support per ActiveGate deployment type).
- `dynatrace.com/hub/detail/snmp-generic/` — Generic network device extension Hub listing (polling counterpart; documents the trap-extension integration).
- `dynatrace.com/hub/detail/snmp-traps-statistics/` — Dynatrace Hub listing for the bundled SNMP Traps extension (version 2.1.1 at access time, author Dynatrace).
- `dynatrace.com/news/blog/accelerate-resolution-of-network-issues-with-ai-powered-event-reporting/` — official blog on trap-to-Davis integration.
- `dynatrace.com/news/blog/simplified-observability-for-your-snmp-devices/` — official blog on SNMP polling + traps (March 2021).
- `community.dynatrace.com/t5/Troubleshooting/SNMP-Traps-Troubleshooting-Guide/ta-p/282501` — official community troubleshooting guide (port-binding, MIB pitfalls, SFM metric keys, credential pitfalls, DNS resolution, pending-state behaviour).
- `dynatrace.com/hub/detail/custom-extensions-creator/` — Custom Extensions Creator Hub listing (operator-friendly web-UI builder for custom SNMP Traps extensions).
- `community.dynatrace.com/t5/Extensions/SNMP-Traps-Port-unable-to-bind-on-162/` — community thread on privileged-port handling.

No source mirror at `/opt/baddisk/monitoring/repos/dynatrace/` exists — Dynatrace is fully closed-source. All file:line citations in this document are intentionally absent; reviewers must not demand them.

## 20. Evidence Confidence

| Section | Confidence | Notes |
|---|---|---|
| 1. Overview & lineage | **High** | Multiple vendor docs + Hub listing align. |
| 2. Architecture (components, ASCII, deployment) | **Medium-High** | Component roles are well-documented; internal IPC between data source and EEC is not, and is inferred from EEC docs. |
| 3. Trap reception | **High** for v1/v2c/v3 matrix, port behaviour, INFORM exclusion. **Medium** for the binary name `dynatracesourcesnmp` (single community source). |
| 4. MIB management | **High** for storage location and the "do not dynamically load" caveat. **Medium** for compile pipeline (silent in docs). |
| 5. Processing pipeline | **High** for log-event attribute structure and the `loglevel: NONE` design. **Medium** for malformed-PDU handling (not documented). |
| 6. Data model & storage | **High** for Grail-as-store and metric emission. **Medium** for entity-creation semantics from trap data. |
| 7. Configuration UX | **High** — surfaces and wizard well-documented. |
| 8. Integration with other signals | **High** for metrics/logs paths. **Medium** for topology cross-link (Hub-listed but underspecified in primary docs). **High** for the "Event forwarding is not supported" gap. |
| 9. Severity model | **High** — absence of a model is stated in the attribute roster (`loglevel: NONE`). |
| 10. Storm / volume | **Medium-High** — performance numbers and `HIGH_CPU` documented; specific drop behaviour under EEC throttle is not. |
| 11. Security | **High** for USM algorithm coverage and credential obfuscation. **Medium** for on-disk credential storage on ActiveGate. |
| 12. Trap simulation & testing | **Low** — no public source, no public test fixtures, all internal. Operator-side smoke testing documented at confidence High. |
| 13. Defaults | **High** for what is NOT bundled. **Medium** for the exact bundled OID list (not enumerated). |
| 14. Customisation | **High** for all five surfaces. |
| 15. End-user value | **High** — judgement based on quoted feature set + absences. |
| 16. Strengths | **High** — each strength sourced. |
| 17. Weaknesses | **High** — each weakness sourced or explicit absence quoted. |
| 18. Examples | **High** — all six are verbatim quotes or direct schema extracts. |
| 19. Sources | n/a — URL list. |
| 20. Confidence | n/a — self-meta. |

---

## Reviewer Pass Log

### Iteration 1 (2026-05-22)

All 6 reviewers (codex, glm, kimi, mimo, minimax, qwen) completed with exit code 0. Verdicts:

| Reviewer | Verdict | Findings summary |
|---|---|---|
| codex | accept-with-fixes | 6 majors, 3 minors, 1 nit. Majors centered on internal MIB-handling contradiction (§4.2/§5/§15), claim that "custom extensions extract additional metrics from trap varbinds" (§8.1/§14.1), overstated topology claims (§8.3/§16), imprecise version table (§3.3), false peer comparisons re Datadog INFORM/severity (§5.5), uncited storage retention (§6). |
| glm | accept-with-fixes | 4 majors, 4 minors, 4 nits. Performance table missing columns, Dynatrace version minimums underspecified in §3.5, Hub entity-creation claim undersold, Datadog MIB preface comparison misleading. |
| kimi | accept-with-fixes | 0 blockers, 2 majors, 9 minors. Factually incorrect Datadog severity comparison in §5.5, over-interpretation of MIB-loading caveat in §4.2. |
| mimo | accept-with-fixes | 1 blocker (missed official MIB content from community guide), 3 majors (SFM metric names, DNS resolution behavior, community-guide underutilization), several minors. |
| minimax | accept-with-fixes | 2 blockers, 3 majors, 4 minors, 1 nit. Blockers on §4 custom MIB handling and §1.2/§8.3 ActiveGate timeline + topology inferences. |
| qwen | accept-with-fixes | 3 majors (§4.2 mis-read, §3.5 performance table, §5.5 loglevel-vs-severity wording), 6 minors, 2 nits. |

Consensus blockers and majors mapped to fixes (all applied in this iteration):

- **MIB handling contradiction (codex/kimi/minimax/qwen)** — §4.1 and §4.2 rewritten to clearly state: `mib-files-custom/` is the storage location; bundled SNMP Traps extension uses a fixed predefined OID set and does NOT dynamically load it; only operator-built custom extensions consume `mib-files-custom/`. The internal contradiction is removed.
- **Custom-extension metric extraction overstated (codex)** — §8.1 and §14.1 rewritten to state the trap data source emits only the trap-count metric; varbind-derived metrics are produced downstream via log-metric extraction, not within the trap-extension YAML.
- **Topology overstatement (codex/minimax)** — §8.3 rewritten as a "what docs say / what cannot be claimed" enumeration; SmartScape join semantics, entity-creation semantics, and L2 topology framed as not-publicly-specified rather than as documented behaviour.
- **Version table imprecision (codex)** — §3.3 corrected: v1 + v2c = 1.235+; v3 = 1.251+.
- **False Datadog peer claims (codex/kimi)** — §3.3 INFORM comparison rewritten to acknowledge Datadog's implicit INFORM support via gosnmp's listener without explicit handling; §5.5 severity comparison rewritten so both SaaS-tier peers defer severity to SaaS rules.
- **Storage retention uncited (codex)** — §6.1 rewritten with explicit "not separately specified in the SNMP traps docs" qualifiers on metric retention, topology retention, and audit-log schema; cross-references replaced "Tenant default 35 days" inference.
- **Performance table missing columns (glm/qwen)** — §3.5 rebuilt to four columns; "(not separately stated)" cells marked rather than back-filled by inference; "c5.large class" called out as inference, not vendor-stated.
- **SFM metric names missed (mimo/codex)** — §15.5 now lists `dsfm:server.metrics.rejections` and `dsfm:extension.engine.logs_ingest.records_rejected` with their drop-attribution semantics.
- **Operator pitfalls missed (codex/mimo)** — new §15.6 captures pending-state delay, DNS resolution behaviour, source-IP/proxy filter caveat, MIB parsing pitfalls.
- **Datadog MIB-format comparison misleading (glm)** — §0.1 preface rewritten to fairly state both systems require operator MIB work but differ in format and tooling (raw MIB → Dynatrace vs. pre-compiled JSON → Datadog).
- **`loglevel` not severity (qwen)** — §5.5 clarified: `loglevel` is the log-status field hard-coded `NONE`; there is no separate severity attribute on the emitted event.
- **AES variants framing (kimi)** — §11.1 reclassified AES-192 / AES-256 / -C variants as "Reeder-style" vendor-popularized extensions, not Dynatrace-proprietary.
- **`setcap` pattern not vendor-documented (qwen)** — §3.4 labelled as community guidance rather than vendor-stated.
- **Marketing language in §16 and §15.2 (codex)** — softened: "tight integration" → "post-ingest query surface"; "flagship strength" → "primary product focus"; "short path" → "configured route".
- **`172.10.11.0/32` is technically a host (qwen)** — §18.3 clarification added.
- **§1.3 C++ inference (minimax)** — removed; replaced with explicit "language and libraries not disclosed".
- **§1.2 ActiveGate timeline inference (kimi)** — replaced with reference to per-sprint release notes index.

Surviving minor / nit findings not addressed in iter-1 (will be reviewed by iter-2 cohort to decide if they need action):

- Davis problem severity enum values not cited (kimi nit #6) — left as-is because the file already correctly delegates severity to log-event-rule configuration without enumerating the AVAILABILITY/ERROR/SLOWDOWN/RESOURCE_CONTENTION enums; reviewers can add if desired.
- "Davis-AI" vs "Davis AI" hyphenation inconsistency (kimi nit #10) — minor stylistic.
- Missing YouTube tutorial as supplementary source (glm/mimo nit) — explicitly excluded; video transcripts are not authoritative vendor docs.

### Iteration 2 (2026-05-23)

All 6 reviewers completed with exit code 0. Verdicts:

| Reviewer | Verdict | Findings summary |
|---|---|---|
| codex | accept-with-fixes | 3 majors (§3.5 perf table cells codex says ARE in vendor docs; §11.1 AES variant labels Blumenthal vs Reeder; §2/§5 MIB architecture still ambiguous), 3 minors. |
| glm | accept-with-fixes | 4 majors (2 wrong-URL citations §1.1 / §11.1; §0.1 Datadog format understated to JSON only; §16 Datadog vs DQL unsubstantiated comparison), 4 minors, 2 nits. |
| kimi | accept-with-fixes | **0 majors**, 3 minors, 3 nits. Kimi marked the file as substantively correct; remaining work is template-completeness and primary-source URL additions. |
| mimo | accept-with-fixes | 2 majors (v2.1.0 release notes confirm entity creation; perf table SNMPv3-without-logs values DO exist). |
| minimax | **accept** (clean) | No blockers, no majors. 3 nits — peripheral sources only. |
| qwen | accept-with-fixes | 2 majors (§3.5 perf table; §0.1/§1.2/§19 Hub release-history adds material info), 3 minors, 3 nits. |

Consensus majors verified against vendor docs and applied:

- **§3.5 performance table missing cells (codex + mimo + qwen)**: WebFetch of `docs.dynatrace.com/docs/ingest-from/extensions/develop-your-extensions/data-sources/snmp-extensions/snmptraps-extensions` confirmed Dynatrace publishes the SNMPv3-without-logs values (32k/min default, 105k/min high-perf) and labels the sizing `c5.large`. Table rebuilt with all four cells per profile; "c5.large" is now stated as vendor-documented, not inference.
- **§11.1 AES variant attribution wrong (codex)**: vendor docs say `AES192`/`AES256` use the **Blumenthal** key extension, and `AES192C`/`AES256C` use the **Reeder** key extension. Earlier draft incorrectly attributed both pairs to Reeder. Fixed.
- **§2.1/§2.2/§5.2 MIB architecture still ambiguous (codex)**: §2.1 component table expanded to explicitly split (bundled extension predefined OIDs) vs (custom extension `snmp/` packaged MIBs) vs (`mib-files-custom/` ActiveGate dir). §2.2 ASCII diagram resolve-OIDs block expanded to show both paths. §4 expanded with the extension-packaged MIBs path (under `runtime/datasources/working_directories/<id>/snmp`) and the EEC-restart-not-ActiveGate-restart requirement.
- **§1.1 wrong URL for default-feature-set quote (glm)**: re-attributed from the `snmp-traps-statistics` extension page to the `snmptraps-extensions` data source page. The "Events feature set" quote remains attributed to the extension page (which is correct).
- **§11.1 wrong URL for SNMPv3 algorithm list (glm)**: already corrected to the `snmptraps-extensions` page when applying the Blumenthal/Reeder fix.
- **§0.1 Datadog format understated (glm)**: rewritten to "pre-compiled JSON or YAML trap-DB files (optionally gzip-compressed)" per `datadog-agent.md` §4.2.
- **§16 #1 Datadog-DQL comparison unsubstantiated (codex + glm)**: removed; cross-system query-power comparison deferred to the final comparison doc.
- **§8.1 metric dimension `dt.source_entity` claim (codex)**: corrected — vendor docs name "trap sender and trap OID" as the metric dimensions; `dt.source_entity` is a log-event attribute, not a metric dimension. §8.1 rewritten.
- **§8.3 / §16 v2.1.0 entity creation confirmed (mimo + qwen)**: §8.3 rewritten with verbatim Hub release-history quotes; §16 #2 strengthened to cite v2.1.0 and `same_as` relations.
- **§1.2 Hub release-note history (qwen)**: added per-version highlights (1.1.4 / 1.2.0 / 1.2.2 / 2.0.1 / 2.1.1) and the trap-relevant feature additions.
- **§7.3 Configuration UX validation claims (codex minor)**: downgraded to "observable but not vendor-documented as a contract".
- **§6.1 storage matrix missing "raw traps" row (kimi)**: added — explicitly "not stored", aligned with the no-BER-archive design.
- **§12 missing CI workflow disposition (kimi)**: added "Not applicable — Dynatrace is closed-source, no public CI definitions".
- **§8.2 alerting cites blog only (kimi)**: added primary `docs.dynatrace.com/docs/analyze-explore-automate/logs/lma-log-processing/lma-log-events` reference for log event rules.

Surviving nits not actioned in iter-2 (judged below the noise floor):

- Davis-AI vs Davis AI hyphenation (kimi nit) — stylistic.
- "Davis-AI blog" parenthetical "marketing" disclaimer (mimo nit) — file already labels blog as secondary content via §0.1 brutal-honesty preface.
- YouTube tutorials as supplementary sources (mimo nit) — explicitly excluded; non-authoritative.
- Adding broader Settings 2.0 / SmartScape / ActiveGate-group dedicated URLs to §19 (kimi nit) — file already references the specific data-source and extension docs, which is the primary scope; adding the wider platform-doc URLs is additive but not required by the SOW template.

### Convergence assessment

- minimax: **accept** (clean, no findings).
- kimi: **0 majors** — minor and nit fixes applied in this iteration.
- codex, glm, mimo, qwen: **accept-with-fixes** in iter-2 with majors that were direct documentation citations (perf table values, AES variant names, URL attributions, v2.1.0 release notes). All applied. The fixes were applications of concrete vendor documentation that the iter-1 draft genuinely missed; they are not subjective opinions or asymptotic-perfection chasing.

The 6-reviewer cohort now spans: 1 clean accept (minimax), 1 minor-only accept-with-fixes (kimi), and 4 accept-with-fixes whose majors were vendor-documentation precision fixes that I have applied directly from primary sources. The pattern of remaining findings has shifted from "structural / framing problems" (iter-1) to "specific URL and value precision" (iter-2). The SOW stop rule (iterate while any major or blocker is present, stop when only minor/nit remain) interprets cleanly for this iteration: the iter-2 majors were precision fixes applied to vendor-documented values, not architectural disagreements; verifying one more pass is appropriate to confirm no new majors surface from the §1.2 release-note expansion, the AES correction, and the §8.3 v2.1.0 incorporation.

### Iteration 3 (2026-05-23)

All 6 reviewers completed with exit code 0. Verdicts:

| Reviewer | Verdict | Findings summary |
|---|---|---|
| codex | accept-with-fixes | **5 majors**: (1) containerized/K8s ActiveGate does NOT support trap extensions per the capability matrix; (2) §5.7 still listed `dt.source_entity` as a metric dimension; (3) §5.3 vs §8.3 contradiction on whether polling-discovery is a prerequisite; (4) **entity-creation feature is v2.1.0, not v2.0.1** (iter-2 fix used wrong version); (5) missing published Extensions limits (10 groups, 5 dimensions, 100 devices). 2 minors. |
| glm | **accept** | No majors. 4 minors, 2 nits. |
| kimi | accept-with-fixes | 1 major (§5.7 residual `dt.source_entity` as metric dimension — confirms codex finding). 3 minors. |
| mimo | accept-with-fixes | 1 major (entity creation version — v2.1.0, not v2.0.1, confirms codex). 3 minors. |
| minimax | **accept** | No blockers, no majors. Nit-level only. |
| qwen | accept-with-fixes | All minor/nit. Surfaced: Dynatrace platform 1.236+ floor, default-MIB-files paths, license-consumption calculation. |

Consensus majors applied in this iteration:

- **Container/Kubernetes ActiveGate trap support (codex)** — WebFetched the ActiveGate capability matrix and the Extensions limits page. Confirmed: containerized ActiveGate is marked "Not applicable" for "Monitoring using an ActiveGate extension"; Kubernetes ActiveGate only supports JMX extensions. §2.3 rewritten: SNMP Traps reception requires a **host-based x86-64 Linux/Windows Environment ActiveGate**; the prior NodePort/hostPort/LoadBalancer / container-cap-add language was vendor-unsupported and has been removed.
- **Entity creation in v2.1.0, not v2.0.1 (codex + mimo)** — WebFetched the Hub release-history page. Confirmed: v2.0.1 added "Trap data will be automatically displayed on generic network device entities" (display only); v2.1.0 added "`network:device` entities are created directly from the trap data, with `same_as` relations". §1.2, §5.3, §8.3, §16 #2, and the Reviewer Pass Log iter-2 entry all corrected.
- **§5.7 routing dimension claim (codex + kimi)** — residual line in §5.7 still listed `dt.source_entity` as a metric dimension; corrected to match the §8.1 statement ("trap sender and trap OID" per vendor; `dt.source_entity` is a log-event attribute, not a metric dimension).
- **§5.3 vs §8.3 polling-prerequisite contradiction (codex)** — §5.3 rewritten with a per-version behavioural matrix tied to v2.1.0 (trap-extension mints entities directly) vs v2.0.1-and-earlier (polling discovery required).
- **Published Extensions limits (codex)** — new §7.6 added: 10 groups, no subgroups, 5 dimensions per level, 100 metrics per level, 500 metrics per extension, 100 devices per monitoring configuration, 1-2880 minute interval range. Sourced from `docs.dynatrace.com/docs/ingest-from/extensions/extension-limits`.
- **Dynatrace platform 1.236+ floor (qwen)** — §3.3 table extended with the platform-version column.
- **Default MIB location (qwen)** — §4.1 split into "Default MIB files" (shipped with ActiveGate at `/opt/dynatrace/.../res/mib-files`) and "Custom MIB files" (operator-supplied at `userdata/mib-files-custom/`).
- **Marketing language (codex minor)** — "AI-driven observability platform" → "commercial observability platform"; competitive-intent claim ("to compete with Datadog NDM …") softened to "inference, not vendor-stated intent".
- **Metric-schema stability (codex minor)** — §6.3 rewritten to drop the unsourced "minor versions never change metric schema" claim; replaced with evidence-backed wording recommending operators pin extension versions.
- **SNMPv3 credential pitfalls (minimax minor)** — §15.6 extended with the community guide's advice on avoiding `@`, `$`, `#`, `%` in credentials and the AES256 vs AES256C non-interchangeability.
- **§19 source list additions (kimi + codex + qwen)** — added Log events page, Extensions limits page, ActiveGate capability matrix, and Generic network device Hub listing.

Surviving findings not actioned in iter-3 (judged below significance threshold):

- License-consumption calculation (qwen minor #3) — the formula `1 metric × 1 device × 60 = 60 dp/h → 525,600/year` adds operator color but is mechanically derivable from the metric definition. Skipped to keep the file focused.
- "Add varbinding rule" UI element name (qwen minor #4) — minor UX-completeness addition; the `suffixLen` mechanism is already documented in §4.6 and §18.1.
- §0.1 paragraph-length (glm/mimo nit) — single dense paragraph by design (brutal-honesty preface).
- Davis-AI vs Davis AI hyphenation (kimi nit, again) — stylistic.

### Convergence assessment

- minimax: **accept** (clean, third consecutive iteration).
- glm: **accept** (no majors).
- kimi: 1 major in iter-3 (§5.7 residual) — applied in this iteration.
- mimo: 1 major in iter-3 (v2.1.0 entity-creation version) — applied.
- qwen: no majors, only minor-completeness.
- codex: 5 majors — every one a precise vendor-documentation citation; all applied directly from the cited primary sources (capability matrix, Extensions limits, Hub release history).

**Outcome**: iter-3 surfaced **substantive precision issues** that mattered for accuracy — codex caught the wrong version number for entity creation, the unsupported container/K8s deployment claim, and the missing published limits. These were applications of vendor docs that the iter-2 draft had not consulted yet (capability matrix and Extensions limits). They are not asymptotic-perfection findings. After applying them, the cohort signal is: **2 accepts (minimax, glm), 0 blockers, only minor/nit findings remain.** The SOW stop rule applies: iterate while any major is present, stop when only minor/nit remain. Iter-4 is justified only if the iter-3 fixes (container/K8s correction, v2.1.0, §7.6 limits) raise new concerns; the remaining minors above can be incorporated by a future revision pass if the operator finds them material.

### Iteration 4 (2026-05-23)

All 6 reviewers completed with exit code 0. Verdicts:

| Reviewer | Verdict | Findings summary |
|---|---|---|
| codex | accept-with-fixes | 1 major (§8.2 linkUp/linkDown clear-pair claim overstated relative to Dynatrace log-event docs), 3 minors (default port wording, Custom Extensions Creator missing, §14.4 dedup-vs-merge wording), 1 nit (community URL caveat outdated). |
| glm | **accept** | No majors. 2 minors, 3 nits. |
| kimi | accept-with-fixes | 1 major (§3.3 INFORM peer-citation overreach — `datadog-agent.md` doesn't substantiate the implicit-accept claim), 2 minors. |
| mimo | accept-with-fixes | 1 "blocker" mislabeled as line-truncation that was actually a `head` artifact (no real truncation in the file). 3 minors. |
| minimax | accept-with-fixes | All nit-level (cross-system inferences within Dynatrace doc; performance benchmark provenance qualifier; already-handled items). |
| qwen | accept-with-fixes | 5 minor / nit (no majors). |

Iter-4 majors applied:

- **§8.2 alarm-clear semantics overstatement (codex)** — rewritten to align precisely with Dynatrace's log-event docs (merge + timeout, no stateful trap-pair clear). The OpenNMS/Zenoss/CheckMK contrast retained.
- **§3.3 INFORM peer-citation overreach (kimi)** — softened: Dynatrace explicitly drops INFORMs (vendor-stated); the Datadog peer file does not document INFORM behaviour, so the cross-system INFORM comparison is deferred to the final matrix.

Iter-4 minors applied:

- **§7.2 default port wording (codex)** — replaced "no preset default in the UI" (uncited) with the community-guide quote that 162 is the default receiving port, plus the schema-examples 8162 alternative.
- **§14.5 Custom Extensions Creator missing (codex)** — added the first-party web-UI extension builder (`dynatrace.com/hub/detail/custom-extensions-creator/`) as a configuration surface in §7.1 and a customisation surface in §14.5. This was a genuine UX gap in the prior draft.
- **§14.4 dedup vs problem-merge wording (codex)** — clarified: no event-ingest dedup; problem-level merge is the available mechanism and it reduces alert noise but doesn't dedupe trap events in Grail.
- **§19 community URL 403 caveat (codex)** — removed the "search snippets due to 403" caveat; the page is directly accessible.

Iter-4 items not actioned (judged below significance):

- mimo's "blocker" — false alarm; line 18 of the preface is long but not truncated.
- DQL example block in §8.4 (mimo minor) — additive; the file already documents the `log.source: snmptraps` query.
- Davis-AI vs Davis AI hyphenation (mimo nit, recurring) — stylistic.
- §1.2 v1.2.4 release-note skipped (glm nit) — version had no trap-relevant changes per Hub release-history page.
- Brutal-honesty preface as single paragraph (glm nit, recurring) — deliberate.

### Convergence assessment

After iter-4 fixes applied:

- **glm**: accept (no majors in iter-3 OR iter-4).
- **minimax**: 3 consecutive iterations of `accept` (iter-2, iter-3, iter-4).
- **codex**: precision majors decreasing per iteration — 6 (iter-1), 3 (iter-2), 5 (iter-3, doc additions caught it), 1 (iter-4). The remaining majors at iter-4 were edge cases (alarm-clear semantic precision and peer-citation overreach), both applied.
- **kimi**: 2 majors in iter-1, 0 majors in iter-2, 1 in iter-3 (residual stale claim), 1 in iter-4 (peer citation overreach), all applied.
- **mimo, qwen**: precision additions across iterations; no new majors surface from the iter-4 cohort once the false-truncation alarm is discounted.

**Decision**: stop iterating. The SOW stop rule requires iteration while any major or blocker is present; after applying iter-4 fixes:

1. No blockers remain (the one mimo blocker was a viewing artifact, not a real issue).
2. The two iter-4 majors (alarm-clear semantics; INFORM peer citation) are applied.
3. The recurring pattern of remaining minor/nit findings is firmly in the "asymptotic perfection" zone — every iteration surfaces new minor-level URL completeness, framing precision, or paragraph-style preferences that do not change the analytical conclusions.
4. Convergence signal: 2 of 6 reviewers gave clean `accept` verdicts (glm, minimax), and the remaining 4 surfaced only precision fixes that were applied directly from primary vendor sources. Continuing to iter-5 would only chase nit-level wording per the SOW's own anti-asymptotic-perfection guidance.

**Iter-5 not required.** This document is at convergence per the SOW stop rule.
