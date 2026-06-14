# SNMP Trap Alerting Models — distinct paradigms in the cohort

Each system maps "trap arrived" → "operator notified" through a specific paradigm. The same paradigm can appear in multiple systems with different vocabulary; this file groups them.

---

## Paradigm 1: Trap → event-store → alarm engine (alarm-engine model)

**Definition**: trap becomes an event row in a dedicated store; a separate alarm engine processes events into open/closed alarms with a stateful lifecycle (acknowledge, clear, escalate).

**Systems**:
- **OpenNMS** (opennms.md:264, 411-413, 921-944). Trap → `events` row → `eventd` → `alarmd` consumes events with `<alarm-data>` → upsert `alarms` row keyed on `reductionKey` → Drools rules `cosmicClear`, `unclear`, `cleanUp` manage the lifecycle.
- **Zenoss** (zenoss.md:33-34, 329-330, 458-461). Trap → `$RawZenEvents` AMQP → `zeneventd` pipeline → `$ZepZenEvents` → `zeneventserver` (Java) → `event_summary` row keyed on `fingerprint_hash` (SHA-1 of `device|component|eventClass|eventKey|severity`). Auto-clear via `zEventClearClasses` zProperty (declarative) + ZEP's `clear_fingerprint_hash` matching (computed from `device|component|eventClass|eventKey`).
- **CheckMK Event Console** (checkmk.md:262-321, 411-471). Trap → `mkeventd` EventServer thread → rule matching against `Event` TypedDict → in-RAM `EventStatus` open events + history backend (file/sqlite/mongo). Cancelling rules via `match_ok`; livetime / TTL-based archival.
- **Sensu Classic + Enterprise** (sensu.md:317-326). Trap → Sensu check-result → Sensu server → Redis/MariaDB events. Sensu Go: PostgreSQL `event_summary` with unique constraint on `(sensu_namespace, sensu_check, sensu_entity)`.

**Worked example — OpenNMS Cisco linkDown**:
- Vendor MIB → `event` XML at `Cisco.events.xml`:
  - `<mask>` matches enterprise OID `.1.3.6.1.4.1.9.9.106.2`, generic 6, specific 1.
  - `<uei>uei.opennms.org/vendor/Cisco/traps/cHsrpStateChange</uei>`.
  - `<severity>Minor</severity>`.
  - `<alarm-data reduction-key="%uei%:%dpname%:%nodeid%:%interface%:%parm[#2]%:%parm[#3]%:%parm[#4]%" alarm-type="1"/>`.
- Trap arrives → Trapd matches `eventconf_events` row → `Event` created → `EventForwarder.sendNowSync` → `eventd` writes `events` row → `alarmd` reads `<alarm-data>` → upsert `alarms` row by reduction-key.
- Operator views `Status → Alarms`; can ACK; auto-clears via `cosmicClear` Drools rule when paired `ccmGatewayFailedClear` trap (alarm-type=2) arrives with matching reduction-key.

**Trade-offs**:
- (+) operator gets a full event lifecycle (open / ack / clear / suppress) at the trap stratum.
- (+) cross-event correlation possible via reduction-key.
- (-) requires a stateful store (PostgreSQL, MariaDB, etcd).
- (-) authoring an event definition + alarm-data + clear-key is a per-trap design effort.

**Latency**: real-time at trap arrival.
**OOB coverage**: OpenNMS 17,442 event defs (opt-in upload); Zenoss 3 seeded + ZenPacks; CheckMK zero; Sensu zero.
**Operator burden**: HIGH — definitions per OID, reduction keys per category, optional Drools rules.

---

## Paradigm 2: Trap → trigger evaluation (trigger model)

**Definition**: trap becomes an item value in a time-series database; triggers (boolean expressions over item values) fire on value changes; trigger state drives alert.

**Systems**:
- **Zabbix** (zabbix.md:24-42, 567-583). Trap → `snmptrap[regex]` or `snmptrap.fallback` item value (LOG/TEXT/STR/UINT64/FLOAT/BIN/JSON, by configured `value_type`) in `history_log` etc. Trigger expressions reference the item: `{Host:snmptrap[upsTrapBatteryLow].str("upsTrapBatteryLow")}=1`.

**Worked example — Socomec UPS (Zabbix community template)**:
- Item: `snmptrap["upsTrapBatteryLow"]` on host `Monitoring UPS`.
- Trigger expression (problem): `{Monitoring UPS:snmptrap["upsTrapBatteryLow"].str("upsTrapBatteryLow")}=1` with `priority=AVERAGE`.
- Trigger recovery_expression: `{Monitoring UPS:snmptrap["upsTrapAlarmEntryRemoved"].str("upsTrapBatteryLow")}=1`.
- Two separate trap items + two trigger expressions = problem/recovery pair.

**Trade-offs**:
- (+) trap is unified with metrics in the same TSDB — cross-signal join is free.
- (+) trigger severity is fine-grained (6 levels) and operator-controlled.
- (-) every actionable trap needs a trigger; recovery_expression requires a paired clear item.
- (-) zero per-OID coverage out-of-the-box (built-in templates ship `snmptrap.fallback` only); operator templates from `community-templates`.

**Latency**: real-time (item value triggers re-evaluation).
**OOB coverage**: zero per-OID alert templates in Zabbix built-in templates (zabbix.md:843-845).
**Operator burden**: HIGH — per-OID items, per-OID triggers, paired clear triggers.

---

## Paradigm 3: Trap → log event + log monitor (SaaS log-monitor model)

**Definition**: trap becomes a JSON log document forwarded to a SaaS backend; SaaS-side rules promote matching log lines to alerts/problems.

**Systems**:
- **Datadog Agent** (datadog-agent.md:20-21, 530-534). Trap → JSON log → `network-devices-snmp-traps` event type → HTTPS `snmp-traps-intake.<site>` → SaaS Logs Explorer → operator-built Logs Monitor.
- **Dynatrace** (dynatrace.md:36-39, 526-528). Trap → Grail log event (`log.source=snmptraps`, `loglevel=NONE`) → operator log event rule → Davis problem (with rule-defined severity).
- **Splunk SC4SNMP** (splunk-sc4snmp.md:33, 511). Trap → Celery pipeline → HEC event (`sourcetype=sc4snmp:traps`, `index=netops`) → Splunk SPL alerts.

**Worked example — Datadog Cisco `linkDown`**:
- Trap arrives at Agent → gosnmp decodes → formatter resolves OID against `dd_traps_db.json.gz` (11K+ MIBs) → JSON payload with `snmpTrapName`, `snmpTrapMIB`, `snmpTrapOID`, top-level `ifAdminStatus="down"`, `variables[]` array, `ddtags` (`snmp_version:v2c,device_namespace:default,snmp_device:192.0.2.5`) → HTTPS POST to `snmp-traps-intake.datadoghq.com` → Logs Explorer.
- Operator authors a Logs Monitor: query `source:snmp-traps @snmpTrapName:linkDown @snmp_device:<ip>` → severity ERROR → routes to PagerDuty integration.

**Trade-offs**:
- (+) zero on-prem state — the on-prem agent is forward-only.
- (+) cross-signal correlation (logs + metrics + traces) at the SaaS layer.
- (-) every alert decision requires SaaS-side authoring; round-trip latency for tuning.
- (-) per-trap dedup must be operator-built via Logs Monitor formulas; the agent emits every trap (Datadog) or applies no dedup (all three).
- (-) trap → page latency depends on SaaS-side scan cadence (typically 1 min for log monitors).
- (-) cost scales with trap volume (per-event SaaS ingest pricing).

**Latency**: SaaS-side rule evaluation; typically ≤1 min poll.
**OOB coverage**: Datadog NDM preset monitors via integrations-core (counts not public); Dynatrace zero (operator-built); Splunk SC4SNMP zero.
**Operator burden**: MEDIUM-HIGH — SaaS-side monitor authoring.

---

## Paradigm 4: Trap → passive check submission (Nagios model)

**Definition**: trap becomes a "passive check result" written to a Nagios-style external command file; the active polling engine ingests it as a service state change.

**Systems**:
- **Centreon** (centreon.md:140, 663-666). Trap → `centreontrapd` Perl daemon → `submitResult` writes `PROCESS_SERVICE_CHECK_RESULT;<host>;<service>;<status>;<output>` to `centengine.cmd` (FIFO) → centreon-engine evaluates notification rules.
- **Nagios + SNMPTT** (nagios-snmptt.md:431-440). Trap → snmptrapd → SNMPTT daemon → EVENT block's `EXEC` calls `submit_check_result` → writes to `/usr/local/nagios/var/rw/nagios.cmd` → Nagios Core's `CMD_PROCESS_SERVICE_CHECK_RESULT = 30` (`base/commands.c:724-725`).

**Worked example — Cisco environmental fan trap via Nagios+SNMPTT**:
- snmptrapd receives trap → invokes `snmptthandler` → writes spool file.
- SNMPTT daemon polls spool every 5s → matches `EVENT envmonFanNotification .1.3.6.1.4.1.9.9.13.3.2 "Environmental Status" Major` from operator-supplied `cisco.conf`.
- FORMAT substitutes varbinds; EXEC calls `submit_check_result <host> "SNMP Trap" 2 "$O: $1 $2"`.
- `submit_check_result` echoes `[<datetime>] PROCESS_SERVICE_CHECK_RESULT;<host>;SNMP Trap;2;<plugin_output>` into `nagios.cmd`.
- Nagios parses the line → service state → CRITICAL transition → notifies on-call via `notify-service-by-email` command.

**Trade-offs**:
- (+) inherits the entire Nagios state machine (acknowledge, escalation, time-of-day, contact groups).
- (+) operator can plug other tools into the same external-command interface.
- (-) per-host passive service must exist BEFORE Nagios accepts the result; otherwise the line is logged at NSLOG_RUNTIME_WARNING and discarded.
- (-) 5-tier SNMPTT severity collapses to 3-tier Nagios state (Minor/Major/Critical all → CRITICAL).
- (-) no automatic trap-clear (linkUp does not auto-resolve linkDown unless operator writes a paired EVENT or relies on `volatile_services`/`check_freshness`).

**Latency**: SNMPTT polling cadence (default 5s) + Nagios scheduler.
**OOB coverage**: zero — operator builds `snmptt.conf` + Nagios `services.cfg` from scratch.
**Operator burden**: VERY HIGH — every alert-worthy OID needs an EVENT block + a Nagios service + an EXEC line + (often) a paired clear EVENT.

---

## Paradigm 5: Trap → handler → typed dispatch (handler-as-code model)

**Definition**: trap routes to a code handler (PHP class / Python file) keyed by trap OID; the handler does relational state updates + writes an eventlog row + triggers downstream alert evaluation.

**Systems**:
- **LibreNMS** (librenms.md:330-345, 386, 521-525). Trap → `snmptrap.php` CLI (per-trap PHP process) → Laravel kernel bootstrap → `LibreNMS\Snmptrap\Trap` constructor parses text format → `Dispatcher::handle()` → `app(SnmptrapHandler::class, [$trap->getTrapOid()])` resolves a handler class from `config/snmptraps.php` (181 OID-to-class) → handler updates `ports.ifOperStatus` / `bgppeers.bgpPeerState` etc. → writes eventlog row → `AlertRules->runRules()` re-evaluates alert rules for the device.

**Worked example — LinkDown handler**:
- snmptrapd receives trap, invokes `/opt/librenms/snmptrap.php`.
- `Trap` parses STDIN → `$oid_data` map.
- `Dispatcher::handle()` finds `IF-MIB::linkDown` → resolves `LinkDown::class`.
- `LinkDown::handle($device, $trap)`:
  - Extract `IF-MIB::ifIndex` from varbinds.
  - Query `ports` table for matching port row.
  - Update `ports.ifOperStatus = 'down'`.
  - Write `Eventlog::log(...)` row with `type='interface'`, `severity=Severity::Error`, `reference=$port_id`.
- `AlertRules::runRules($device_id)` re-evaluates all rules whose AppliesTo includes the device.
- If matching alert rule (e.g., `eventlog.type='trap' AND severity=5 AND datetime > now()-5min`), `alerts` table state moves CLEAR→ACTIVE.
- Notification delivery happens asynchronously via `alerts.php` cron (1-min cadence).

**Trade-offs**:
- (+) handler can update first-class relational state directly (port status, BGP peer status) — much faster than waiting for next poll cycle.
- (+) IDE auto-complete, refactoring, typed enums; per-handler tests.
- (-) per-trap PHP process spawn cost (no batching, no daemon).
- (-) custom handler requires forking the repo (no plugin discovery).
- (-) eventlog has NO dedup — every trap = 1+ rows; only alert state dedups.
- (-) trap-to-page latency = handler (10s of ms) + ≤60s alerts.php cron + transport delivery.

**Latency**: real-time at handler; 0-60s + transport for actual page.
**OOB coverage**: 177 handler classes covering Cisco, Juniper, APC, Veeam, Adva, Ruckus, Fortinet, HPE, Huawei, OSPF, BGP, IF-MIB; 2 of 238 bundled alert rules filter `eventlog.type='trap'` (Zebra-specific).
**Operator burden**: MEDIUM-HIGH — alerting rules are operator-built; handlers are vendor-supplied.

---

## Paradigm 6: Trap → external forward (pipeline-tier model)

**Definition**: trap is decoded, normalised, optionally enriched, then forwarded to an external destination. The system has no internal alerting engine; alerting is the downstream destination's problem.

**Systems**:
- **Telegraf** (telegraf.md:38-42). Trap → `snmp_trap` metric (measurement) → InfluxDB/Prometheus/Kafka/OpenTelemetry/Elastic. Alerting via Kapacitor, Prometheus Alertmanager, Grafana Alerting.
- **Logstash** (logstash.md:29-34). Trap → Logstash event → `output { elasticsearch | kafka | http | ... }`. Alerting via Kibana Alerts, Watcher, ElastAlert.
- **Cribl Stream** (cribl.md:32-37). Trap → Cribl event → Routes/Functions pipeline → N Destinations (Splunk, S3, Kafka, another SNMP NMS via `snmp_trap_serialize`, Datadog, OTel). Cribl has no alerting on trap content (notifications are operational only — license usage, node offline).
- **Sensu snmptrapd2sensu** (sensu.md:106-125). Trap → snmptrapd2sensu process → HTTP POST to sensu-agent `/events`. Sensu acts on events through Sensu handlers/checks downstream.

**Worked example — Cribl Stream "trap → Splunk + S3 + upstream NMS" fan-out**:
- Trap arrives at Cribl Worker UDP listener → `__snmpRaw` preserved.
- Routes table: first match `__inputId == 'in_snmp_trap'` → Pipeline `snmp_normalise`.
- Pipeline applies Lookup function (community pack CSV) → adds `trap_*` fields.
- Operator uses 3 Routes (Final=off) to fan out:
  - Destination 1: Splunk HEC (JSON).
  - Destination 2: S3 (cold storage).
  - Destination 3: `snmp_trap_serialize` Function then SNMP Trap Destination → upstream NMS receives verbatim or re-serialised PDU.

**Trade-offs**:
- (+) maximum flexibility on outbound destinations.
- (+) no on-prem state tier.
- (-) no alerting on trap content within the pipeline.
- (-) dedup, severity, lifecycle — all downstream-responsibility.
- (-) for "be the alarm system" use-cases, operator must layer additional products.

**Latency**: real-time forward; SaaS rule cadence for alerting.
**OOB coverage**: depends entirely on operator's downstream config.
**Operator burden**: MEDIUM at the pipeline tier; downstream system has its own burden.

---

## Paradigm 7: Trap → vendor-curated alert lifecycle (commercial-NMS model)

**Definition**: vendor-shipped rules + UI define severity, auto-clear, escalation; operator's job is to enable/customise rather than author from scratch.

**Systems**:
- **SolarWinds** (solarwinds.md:478-494). Trap → `SolarWindsTrapService` → SQL DB → Log Viewer rules → pub/sub Event alert condition → Web Console alert.
  - Outbound: Trap Templates emit v1/v2c traps to upstream NMSes.
  - Vendor caveat: Log Viewer alert actions cap at ~80/sec platform-wide (solarwinds.md:487-489).
- **LogicMonitor** (logicmonitor.md:30-43, 477). LogSource path: trap → LM Logs → LogAlert conditions → Alert Rule + Escalation Chain.
  - Auto-close alerts on related "clear" trap (vendor capability claim).
  - EventSource path: dedup explicitly NOT supported.

**Worked example — LogicMonitor LogSource auto-clear**:
- Operator deploys SNMP Traps LogSource scoped to device group "Cisco Switches".
- Trap arrives → Collector → MIB-translated (bundled MIBs) → LM Logs.
- LogAlert condition: matches `IF-MIB::linkDown` → creates alert tied to Alert Rule → routes to Escalation Chain → PagerDuty incident.
- Paired `IF-MIB::linkUp` arrives → LogSource's "automatic clear" logic closes the corresponding alert.

**Trade-offs**:
- (+) low operator burden if vendor coverage matches deployed devices.
- (+) cleaner workflow for end-user operators.
- (-) closed-source bundles; operator cannot fork or diff.
- (-) vendor cadence drives MIB updates.
- (-) for SolarWinds in particular: ~80 alert actions/sec platform ceiling is a real architectural constraint at storm scale.

**Latency**: real-time.
**OOB coverage**: SolarWinds claims "over 250,000 OIDs" (solarwinds.md:251); LogicMonitor has a documented vendor list at supported-mibs page.
**Operator burden**: LOW-MEDIUM if vendor coverage matches.

---

## Cross-paradigm comparison

| Paradigm | Latency | Lifecycle | Dedup | Operator burden | OOB coverage |
|---|---|---|---|---|---|
| 1: Alarm engine | real-time | full | ✓ | HIGH | varies (17K OpenNMS / 0 CheckMK) |
| 2: Trigger model | real-time | partial (recovery_expression) | partial | HIGH | 0 per-OID alerts in Zabbix built-ins |
| 3: SaaS log monitor | ≤1 min | SaaS-side | operator | MEDIUM-HIGH | varies (Datadog NDM presets) |
| 4: Passive check (Nagios) | 5s poll + scheduler | partial (Nagios state machine) | partial | VERY HIGH | 0 (operator builds catalogue) |
| 5: Handler-as-code | real-time decode + cron transport | partial (alert-rule SQL) | partial (alert state only) | MEDIUM-HIGH | 177 LibreNMS handlers |
| 6: External forward | real-time forward | downstream | downstream | MEDIUM (varies) | n/a |
| 7: Vendor-curated | real-time | full (vendor-defined) | vendor-defined | LOW-MEDIUM | vendor-supplied bundles |

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
