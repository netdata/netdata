# SNMP Trap Operator Feature Catalogue — ranked by importance for network-team operators

Audience: enterprise network engineers running multi-vendor Cisco/Juniper/Arista gear, managing thousands of devices, working in a NOC, on-call at 3 AM. Importance tiers reflect what such operators *need*, not what is technically interesting.

---

## Tier-1: table-stakes (every network team expects this)

### T1-01: v1, v2c, v3 USM reception

- **Description**: receive SNMPv1 trap PDUs, SNMPv2c notification PDUs, and SNMPv3 USM-encrypted notifications.
- **Rationale**: every device in production emits one of these three. v1 is still common in legacy gear (older Cisco IOS, environmental sensors, PDUs); v2c is the most-deployed; v3 is mandated by government, healthcare, finance compliance.
- **Cohort coverage**: 16 of 16. Every system covers all three. (Sensu Classic ext is v2c only at the listener level, but Sensu-Go via snmptrapd2sensu inherits snmptrapd's full coverage.)

### T1-02: OID-to-symbolic-name resolution

- **Description**: translate numeric OIDs (`1.3.6.1.6.3.1.1.5.3`) to symbolic names (`IF-MIB::linkDown`) so operators can read the trap.
- **Rationale**: the foundational spec calls "Naked OID Storage" an anti-pattern (`../domain/snmp-traps-in-observability.md:439-443`). Operators triage at 3 AM by name, not by dotted decimal.
- **Cohort coverage**: 14 of 16 fully (CheckMK is off-by-default; Cribl Stream has no embedded compiler). 
  - Always-on: OpenNMS, Zenoss, Centreon (Net-SNMP), Zabbix (via snmptrapd `-O STte`), LibreNMS (snmptrapd-side), Nagios+SNMPTT, Sensu (Classic), Telegraf (configurable backend), Logstash, Datadog Agent, Splunk SC4SNMP, SolarWinds, Dynatrace, LogicMonitor (LogSource).
  - Off-by-default: CheckMK (`translate_snmptraps=False`).
  - Not supported in-product: Cribl Stream (operator brings CSV lookups).

### T1-03: Device identification (source IP → monitored device)

- **Description**: map the trap's source to a device row in the local CMDB/inventory.
- **Rationale**: operators triage by device name, not by source IP literal. The lens calls "trap from `10.0.0.1`" instead of "trap from `core-router-01, NYC`" a Stage-1 maturity indicator (`../domain/snmp-traps-in-observability.md:743`).
- **Cohort coverage**: 12 of 16 ✓; 4 partial.
  - ✓: OpenNMS, Zenoss, CheckMK, Centreon, Zabbix, LibreNMS, SolarWinds, Dynatrace, LogicMonitor (LogSource and EventSource paths, EA Collector 36.100+), Sensu (Classic ext reverse-DNS lookup), Datadog Agent (`snmp_device` tag), Splunk SC4SNMP (host field).
  - Telegraf, Logstash: `source`/`host` field is raw IP; no inventory join (operator-built).
  - Cribl: no device-inventory in Cribl Stream.

### T1-04: Severity assignment

- **Description**: each trap should arrive with an operator-readable severity (critical / major / warning / info), either intrinsic to the trap definition or assigned by a rule.
- **Rationale**: every NOC ticketing system is severity-routed (P1/P2/P3). The lens calls "Severity Trust Without Normalization" an anti-pattern (`../domain/snmp-traps-in-observability.md:474-478`).
- **Cohort coverage**:
  - Built-in: OpenNMS (7-level OnmsSeverity per event definition), Zenoss (per-EventClass + transforms), LibreNMS (per-handler), Nagios+SNMPTT (5-level SNMPTT severity per EVENT), Sensu Classic (RESULT_MAP), LogicMonitor (5-level per EventSource), SolarWinds (rule-driven).
  - Operator-defined: CheckMK (per-rule), Centreon (per-trap row), Zabbix (per-trigger), Datadog Agent (SaaS log monitor), Splunk SC4SNMP (Splunk SPL), Dynatrace (log event rule).
  - None: Telegraf, Logstash, Cribl.

### T1-05: Storage / retention / search

- **Description**: traps must be queryable for incident review weeks or months later. Forensic analysis cited in the lens (`../domain/snmp-traps-in-observability.md:493-497`) requires this.
- **Rationale**: post-mortems require "what traps were firing 3 weeks ago at 02:13?"
- **Cohort coverage**: 14 of 16 have some store.
  - PostgreSQL: OpenNMS (default 6 weeks).
  - MariaDB: Zenoss (3 days → archive, 90-day purge), Centreon (operator-managed; `log_traps` opt-in), LibreNMS (30 days default), Nagios+SNMPTT (MyISAM, no retention).
  - SQL Server: SolarWinds.
  - file/sqlite/mongo: CheckMK (365 days default).
  - LOG/TEXT history (per-item retention): Zabbix.
  - SaaS-side store: Datadog (Grail-like), Dynatrace (Grail), Splunk SC4SNMP (Splunk), LogicMonitor (LM Logs/event store).
  - No store: Telegraf, Logstash, Cribl Stream (downstream owns it).

### T1-06: Web UI to view traps

- **Description**: a browsable list of recent traps with filters by device, OID, severity, time range.
- **Rationale**: a NOC operator needs to look up "did this device send anything in the last 10 minutes?" without writing SQL.
- **Cohort coverage**:
  - Dedicated trap UI: OpenNMS, Zenoss, Centreon, SolarWinds (Log Viewer/legacy Trap Viewer), NSTI (for Nagios+SNMPTT).
  - Generic events/logs UI: CheckMK (Event Console), LibreNMS (eventlog tab), Zabbix (Latest Data), Datadog (Logs Explorer), Splunk (sourcetype `sc4snmp:traps`), Dynatrace (Logs Viewer), LogicMonitor (portal), Cribl (downstream destination's UI).
  - No UI: Telegraf, Logstash, Sensu (no trap-specific surface).

### T1-07: Alerting integration with paging tools

- **Description**: trap-driven alerts flow into PagerDuty / ServiceNow / Slack / email.
- **Rationale**: NOC operators don't watch the trap UI; they get paged.
- **Cohort coverage**: 16 of 16 ✓ (every system can route alerts via notification channels, though some require operator configuration on the SaaS side).

### T1-08: Authentication / restricted access

- **Description**: SNMPv3 USM credentials + ideally restricted IP allowlist + audit log of trap-related config changes.
- **Rationale**: management-plane attacks against SNMP are real; cleartext communities are a long-known liability (the lens flags `SNMPv1 Everywhere` as an anti-pattern, `../domain/snmp-traps-in-observability.md:432-436`).
- **Cohort coverage**:
  - SNMPv3 USM: 16 of 16.
  - Secret vault integration: OpenNMS (SCV), Telegraf (config.Secret), Splunk SC4SNMP (k8s Secrets).
  - Per-source allowlist: only Cribl (`ipWhitelistRegex`) and Dynatrace (CIDR `ip` field). Others rely on kernel firewall.
  - Plaintext credential storage: Zenoss, CheckMK, Centreon, LibreNMS, Nagios+SNMPTT, SolarWinds (queryable via SWIS).

---

## Tier-2: commonly used (network operators expect this for real deployments)

### T2-01: Vendor MIB pack out of the box

- **Description**: the system ships with vendor MIB coverage for the operator's hardware fleet.
- **Rationale**: nobody wants to compile 200 vendor MIBs on day 1.
- **Cohort coverage**:
  - Comprehensive: Datadog Agent (~11K MIBs), SolarWinds (~250K OIDs).
  - Broad: LibreNMS (4,770 MIB files), LogicMonitor (enumerated vendor list).
  - Curated: OpenNMS (17K event defs opt-in), Centreon (214 seeded rows).
  - Minimal: Zenoss (3 seeded + ZenPacks), Splunk SC4SNMP (~500 IETF base), Dynatrace (fixed predefined set).
  - None: CheckMK, Zabbix built-in templates, Cribl (community pack), Nagios+SNMPTT, Sensu, Telegraf, Logstash.

### T2-02: Custom MIB upload UX

- **Description**: a way to add a vendor MIB the system doesn't bundle.
- **Rationale**: every vendor releases new MIBs; operators must add them locally.
- **Cohort coverage**:
  - Web UI: OpenNMS, Zenoss, CheckMK (WATO), Centreon (UI runs `centFillTrapDB`), LogicMonitor (portal upload 38.300+).
  - CLI tool: Datadog Agent (`ddev meta snmp generate-traps-db`), LogicMonitor (local JSON converter), Nagios+SNMPTT (`snmpttconvertmib`).
  - Drop into directory + restart: Zabbix, LibreNMS, Sensu (`/etc/sensu/mibs`), Telegraf, Logstash (`.dic` from `smidump`), Dynatrace (`mib-files-custom/`; only for custom extensions), Splunk SC4SNMP (mount PVC).
  - No in-product workflow: Cribl (operator builds CSV externally), SolarWinds (email to vendor).

### T2-03: Trap-to-alert routing rules

- **Description**: per-OID rules to assign severity, route to specific recipient, escalate.
- **Rationale**: not every trap should page; tuning is critical for alert fatigue (`../domain/snmp-traps-in-observability.md:425-431`).
- **Cohort coverage**: 13 of 16 ✓ via various surfaces (OpenNMS event defs, Zenoss transforms, CheckMK rule packs, Centreon traps_matching_properties, Zabbix triggers, LibreNMS alert rules, Nagios+SNMPTT EVENT EXEC, Sensu handlers, Datadog/Splunk/Dynatrace/LogicMonitor SaaS-side, SolarWinds Log Viewer rules). Telegraf, Logstash, Cribl: downstream.

### T2-04: Alert acknowledgement / clear flow

- **Description**: an alert can be ACK'd; ideally a paired clear trap auto-resolves it.
- **Rationale**: lens §6.5 anti-pattern is "Absence of Clear Events".
- **Cohort coverage**:
  - Full lifecycle with auto-clear: OpenNMS (cosmicClear Drools rule + `clear-key`), Zenoss (`zEventClearClasses` + `clear_fingerprint_hash`), LogicMonitor LogSource (vendor capability claim).
  - Cancelling rules (operator-defined): CheckMK (`match_ok`, `cancel_application`).
  - Manual ACK only / no auto-clear: Centreon, Zabbix (recovery_expression operator-built), LibreNMS, Nagios+SNMPTT, Datadog, Splunk SC4SNMP, Dynatrace, SolarWinds, Sensu.
  - n/a (pipeline-tier): Telegraf, Logstash, Cribl.

### T2-05: Dedup / suppression at the trap layer

- **Description**: identical traps within a window are collapsed.
- **Rationale**: storms (lens §11.1) destroy on-call usefulness without dedup.
- **Cohort coverage**:
  - Built-in: OpenNMS (alarm-level `reductionKey`), Zenoss (`fingerprint_hash`), Centreon (1s MD5 window), CheckMK (counting / expect), Cribl (Suppress function).
  - Partial / alert-level: Zabbix, LibreNMS, Nagios+SNMPTT, Sensu.
  - None: Telegraf, Logstash, Datadog Agent, Splunk SC4SNMP, Dynatrace, LogicMonitor EventSource (vendor explicit), SolarWinds (every PDU = row).

### T2-06: Topology-aware view (drill from alert → device → neighbours)

- **Description**: from an alert page, navigate to the device, then to its physical/logical neighbours.
- **Rationale**: blast-radius analysis (lens §6.4 Topology-Aware Correlation).
- **Cohort coverage**:
  - Built-in: OpenNMS (enlinkd topology), Zenoss (Network Map), LibreNMS (LLDP/CDP), SolarWinds (Network Atlas / Orion Maps), Dynatrace (SmartScape), LogicMonitor (TopologySources).
  - Limited (drill via filter): CheckMK, Centreon, Zabbix.
  - None: Datadog Agent, Splunk SC4SNMP, Cribl, Telegraf, Logstash, Sensu, Nagios+SNMPTT.
- **Topology-aware suppression**: NONE of the 16 systems suppress downstream traps when an upstream device is down — universally an operator-built pattern.

### T2-07: Pipeline self-monitoring

- **Description**: the trap subsystem exposes its own health metrics (traps/sec received, drops, matched vs unmatched, queue depth).
- **Rationale**: lens §11.1 storms; operators need to know if traps are being dropped.
- **Cohort coverage**:
  - First-class: OpenNMS (11 JMX counters + per-device opt-in), Telegraf (gosnmp internal), Datadog Agent (`datadog.snmp_traps.*` self-metrics).
  - Partial: Zabbix (shipped server-health template includes `zabbix[process,snmp trapper,avg,busy]`), Zenoss (rrdStats counter + hourly /App/Zenoss event for filter-drops).
  - Log-scrape only: CheckMK (no first-class drop counter), Centreon (no metrics endpoint), LibreNMS (no counters), Nagios+SNMPTT (no metrics), Sensu (none), Splunk SC4SNMP (Celery task counters), Dynatrace (kernel SO_RCVBUF only — explicitly "many of them may be dropped by the operating system"), SolarWinds (no trap-pipeline self-telemetry), LogicMonitor (Collector metrics not trap-specific), Cribl (Source metrics but per-Worker).

### T2-08: Distributed deployment (multi-site collectors)

- **Description**: multiple receiving collectors per geographic location, central correlation.
- **Rationale**: WAN cost; firewall ACLs; per-site failure isolation.
- **Cohort coverage**:
  - Multi-tier (collector + central): OpenNMS (Minions), Zenoss (Collectors), Centreon (Pollers), Splunk SC4SNMP (Kubernetes), SolarWinds (APE), Cribl (Worker Nodes), LogicMonitor (Collectors), Datadog Agent (per-host Agent), Dynatrace (ActiveGate groups).
  - Per-pipeline (no clustering): Logstash, Telegraf (per-host), Sensu (per-Collector/per-Agent), Nagios+SNMPTT (via NSCA/NRDP), Zabbix (per-server/per-proxy).
  - Single-host: CheckMK, LibreNMS.

---

## Tier-3: advanced / specialty (network-team-specific or niche)

### T3-01: Northbound trap forwarding (re-emit to upstream NMS)

- **Description**: the system can EMIT SNMP traps to a parent NMS (Tivoli, Netcool, NNMi, etc.).
- **Rationale**: large enterprises have layered NMS hierarchies; operators with a local NMS want to feed events upstream.
- **Cohort coverage**:
  - Native v1+v2c+v3+inform: OpenNMS (alarm-driven SpEL-mapped).
  - Native v1+v2c+v3 (hardcoded MIB): Zenoss (ZENOSS-MIB).
  - Native v2c (limited): LibreNMS (`LIBRENMS-NOTIFICATIONS-MIB`), Centreon (`@TRAPFORWARD()@`, hardcoded community).
  - Native via Trap Templates: SolarWinds.
  - Native via Cribl SNMP Destination: Cribl Stream (verbatim or re-serialised).
  - None: CheckMK, Zabbix, Nagios+SNMPTT, Sensu (post-EOL Sensu Enterprise had it; Sensu Go does NOT), Telegraf (no `outputs.snmp_trap`), Logstash (community plugin not in mirror), Datadog Agent, Splunk SC4SNMP, Dynatrace, LogicMonitor (no native).

### T3-02: INFORM acknowledgement (reliable trap delivery)

- **Description**: device emits an InformRequest expecting an InformResponse; the receiver acknowledges.
- **Rationale**: lens §2 "InformRequest provides reliability"; some financial / healthcare environments require it.
- **Cohort coverage**:
  - ✓ Documented: OpenNMS, Zenoss, Centreon (snmptrapd transparently), Zabbix (gosnmp handles inside TrapListener).
  - partial (snmptrapd-delegated): Centreon, LibreNMS, Nagios+SNMPTT, Sensu (snmptrapd2sensu path).
  - partial (gosnmp/pysnmp library default): Telegraf, Splunk SC4SNMP, Datadog Agent — all undocumented for behaviour.
  - ✗ Explicit: CheckMK (`# Disable receiving of SNMPv3 INFORM messages. We do not support them (yet)`); Dynatrace (`"SNMP inform requests aren't supported"`).
  - Undocumented: Logstash, SolarWinds, LogicMonitor, Cribl.

### T3-03: Multiple v3 USM users on one listener

- **Description**: one socket accepts v3 traps from multiple devices using different USM credentials.
- **Rationale**: enterprise fleets with multiple compliance scopes; legacy + new device generations.
- **Cohort coverage**:
  - Multi-user table: OpenNMS, CheckMK, Datadog Agent, Splunk SC4SNMP, Cribl, SolarWinds, LogicMonitor (per-resource credentials).
  - Single user per listener: Telegraf, Logstash (workaround: multiple input blocks on different ports).
  - via snmptrapd: Centreon, Zabbix, LibreNMS, Nagios+SNMPTT (depends on snmptrapd `createUser` directives).

### T3-04: SNMP-over-DTLS / TLS-TM (RFC 5953/6353/9456)

- **Description**: encrypt the transport itself.
- **Rationale**: emerging compliance ask; very few vendor agents support it as of 2026.
- **Cohort coverage**: NONE of 16 systems support DTLS/TLS-TM in-product.

### T3-05: Storm protection / rate limiting at the listener

- **Description**: per-source rate limit, circuit breaker, storm detection.
- **Rationale**: lens §11.1 worst-case scenario.
- **Cohort coverage**: NONE of 16 systems implement per-source rate-limit at the trap listener. Universal pattern: rely on kernel UDP buffer + external firewall.

### T3-06: Trap-driven topology suppression

- **Description**: trap from a downstream device is suppressed when upstream is known down.
- **Rationale**: lens §6.4 Topology-Aware Correlation.
- **Cohort coverage**: NONE of 16 ship this as a built-in feature. OpenNMS Drools allows it; OCE plug-in offers a real surface. All other implementations are operator-built.

### T3-07: Real-time trap-as-annotation on metric charts

- **Description**: trap appears as a vertical line on a CPU-usage or interface-bandwidth chart.
- **Rationale**: lens §6.5 Trap-to-Event Bridge.
- **Cohort coverage**:
  - Built-in (via PerfStack): SolarWinds (cross-stack correlation Vendor-documented).
  - Built-in (via metric counter): Dynatrace (`com.dynatrace.extension.snmp-traps-generic.traps.count`).
  - Manual (via Grafana / Prometheus annotations API): Telegraf, OpenNMS Prometheus exporter rule.
  - None: every other system.

### T3-08: Trap-driven automated remediation

- **Description**: lens §9 Stage-6: trap triggers a script that shuts an interface, shifts BGP preference, etc.
- **Rationale**: predictive / closed-loop posture.
- **Cohort coverage**:
  - Zenoss: pre-/post-exec hooks per EventClass transform.
  - LibreNMS: handler-as-code can update relational state directly (port status); not arbitrary script.
  - Nagios+SNMPTT: EVENT EXEC can run arbitrary scripts.
  - Centreon: per-trap `traps_execution_command`.
  - SolarWinds: rule "Run an external program" action.
  - LogicMonitor: EventSource Groovy + Alert Rule script.
  - Datadog, Splunk SC4SNMP, Dynatrace: SaaS-side webhook automation.
  - Most others: shell-out from rule actions.

### T3-09: Multi-tenant trap pipeline

- **Description**: one deployment, multiple tenant-isolated trap streams.
- **Rationale**: MSP context; SaaS deployment.
- **Cohort coverage**: Dynatrace, LogicMonitor (MSP mode), Datadog (tenant per Datadog org), Splunk Cloud are the only multi-tenant offerings. SolarWinds — typically one Orion per customer. Others: single-tenant.

### T3-10: Audit logging of trap-rule changes

- **Description**: who changed what trap rule when.
- **Rationale**: SOX / ITGC compliance.
- **Cohort coverage**: OpenNMS, Zenoss, CheckMK, Centreon, Zabbix, Dynatrace, LogicMonitor, SolarWinds. Others: limited or none.

### T3-11: Replay PCAP / sample traps

- **Description**: replay a captured trap stream for testing.
- **Rationale**: pre-prod validation of new MIB / rule changes.
- **Cohort coverage**:
  - Native: Zenoss (Capture / PacketReplay + `trapdump.pcap` + `sendSnmpPcap.py`); CheckMK uses `snmptrap` CLI for integration tests; OpenNMS ships `OpenNMS/udpgen`.
  - External tool only: every other system relies on `snmptrap` CLI from Net-SNMP.

---

## Cross-cohort summary

| Tier | Feature | Top performers | Notable gaps |
|---|---|---|---|
| 1 | v1/v2c/v3 reception | all | none |
| 1 | OID resolution | most | Cribl (no compiler), CheckMK (off by default) |
| 1 | Device identification | most | Telegraf, Logstash, Cribl (raw IP only) |
| 1 | Severity | OpenNMS, Zenoss, LibreNMS, Nagios+SNMPTT | Telegraf, Logstash, Cribl (none); Dynatrace (`loglevel: NONE`) |
| 1 | Storage / retention | OpenNMS, Zenoss, CheckMK, SolarWinds | Cribl (none); Telegraf, Logstash (downstream) |
| 1 | Web UI | dedicated: OpenNMS/Centreon/SolarWinds | Telegraf, Logstash (no UI) |
| 1 | Alerting | all (varying mechanisms) | none |
| 1 | Auth/RBAC | OpenNMS (SCV) | most other: plaintext credential storage |
| 2 | Vendor MIB pack | Datadog (~11K), SolarWinds (~250K), LibreNMS (4,770) | CheckMK, Zabbix, Sensu, Telegraf, Logstash, Nagios+SNMPTT (zero) |
| 2 | Custom MIB upload | OpenNMS, Zenoss, CheckMK, LogicMonitor | SolarWinds (email to vendor); Cribl (no in-product) |
| 2 | Routing rules | most | Telegraf, Logstash, Cribl (downstream) |
| 2 | ACK/clear flow | OpenNMS, Zenoss, LogicMonitor LogSource (auto-clear) | Centreon, Nagios+SNMPTT, Sensu, Datadog, Dynatrace, SolarWinds (no auto-clear) |
| 2 | Dedup | OpenNMS, Zenoss, Centreon, CheckMK, Cribl | Telegraf, Logstash, Datadog, Splunk SC4SNMP, Dynatrace, LogicMonitor EventSource, SolarWinds |
| 2 | Topology drill | OpenNMS, Zenoss, LibreNMS, SolarWinds, Dynatrace, LogicMonitor | Datadog, Splunk SC4SNMP, Cribl, Telegraf, Logstash, Sensu, Nagios+SNMPTT |
| 2 | Self-monitoring | OpenNMS (11 JMX), Telegraf, Datadog, Zabbix | CheckMK, Centreon, LibreNMS, Nagios+SNMPTT, Sensu, SolarWinds, Dynatrace, LogicMonitor (limited) |
| 2 | Distributed deployment | OpenNMS, Zenoss, Centreon, Splunk SC4SNMP, SolarWinds, Cribl, LogicMonitor | CheckMK, LibreNMS, Sensu |
| 3 | Northbound trap | OpenNMS, Zenoss, Cribl, SolarWinds | CheckMK, Zabbix, Telegraf, Datadog, Splunk SC4SNMP, Dynatrace, LogicMonitor (no native) |
| 3 | INFORM ack | OpenNMS, Zenoss, Zabbix | CheckMK ✗ explicit; Dynatrace ✗ explicit |
| 3 | Multiple v3 users | OpenNMS, CheckMK, Datadog, Splunk SC4SNMP, Cribl, SolarWinds, LogicMonitor | Telegraf, Logstash (single user per listener) |
| 3 | DTLS/TLS-TM | NONE | all |
| 3 | Storm protection at listener | NONE | all |
| 3 | Topology suppression | NONE built-in | all |

---

## Read-verification block

Read `../domain/snmp-traps-in-observability.md` in whole (1038 lines, last line: "---")
Read `opennms.md` in whole (1405 lines, last line: "regression per the SOW process.")
Read `zenoss.md` in whole (1322 lines, last line: "documented in §17.")
Read `checkmk.md` in whole (1369 lines, last line: "do not affect the analytical conclusions.")
Read `centreon.md` in whole (1755 lines, last line: "comparative-analysis document.")
Read `zabbix.md` in whole (1596 lines, last line: "satisfies the SOW convergence threshold.")
Read `librenms.md` in whole (1394 lines, last line: "is to forward to Splunk/Elastic/Loki/Lake and query there.")
Read `nagios-snmptt.md` in whole (1751 lines, last line: "The single remaining ambiguity (LOGONLY field position) is upstream's, not this analysis's; both interpretations are acknowledged explicitly in §5 Phase 7, §9, and §20.")
Read `sensu.md` in whole (1299 lines, last line: "End of document.")
Read `telegraf.md` in whole (1371 lines, last line: "documented model-side reviewer instability (qwen across all three iterations; glm/kimi/mimo at iter-3) as the only reason the formal \"≥3 outright accept\" target was not met.")
Read `logstash.md` in whole (1244 lines, last line: "The SNMP4J backend attribution was upgraded from inferred to source-verified mid-iter-3 via the missed-content discovery at `elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:98-101`.")
Read `datadog-agent.md` in whole (1560 lines, last line covered via §5)
Read `splunk-sc4snmp.md` in whole (1377 lines, last line: "additive, not corrective.")
Read `cribl.md` in whole (1258 lines, last line: "Not run — convergence declared at iter-2 per the SOW stop rule.")
Read `solarwinds.md` in whole (1114 lines, last line: "None of the surviving items affect the analytical claims of the file. All vendor-cited claims trace to URLs in §19; all operator-reported claims are explicitly tagged.")
Read `dynatrace.md` in whole (1135 lines, last line: "**Iter-5 not required.** This document is at convergence per the SOW stop rule.")
Read `logicmonitor.md` in whole (1138 lines, last line: "preserving cross-system framing alignment.")
