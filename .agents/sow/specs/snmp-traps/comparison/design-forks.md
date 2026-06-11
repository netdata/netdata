# SNMP Trap Architecture — Design Forks

For each fork: the question, the cohort options with systems picking each, and trade-off citations. No recommendations are made.

---

## Fork 1: Trap-as-X primitive

**Question**: After decode, what data-model primitive does a trap become?

Options observed in the cohort:

- **(a) Event-row in a dedicated event/alarm store + optional alarm**
  - Systems: OpenNMS (events + alarms tables, opennms.md:312-321); Zenoss (event_summary/event_archive, zenoss.md:358-372); CheckMK (mkeventd `Event` TypedDict + history backends, checkmk.md:375).
  - Trade-offs (+): operator gets a stateful event lifecycle (open / ack / clear) right at the trap stratum; query-by-OID, query-by-device, query-by-severity all first-class.
  - Trade-offs (-): heavy storage tier required (PostgreSQL, MariaDB, etc.); the event store becomes the throughput bottleneck and storm-failure surface.

- **(b) Passive check / service status submission to a polling engine**
  - Systems: Centreon (`PROCESS_SERVICE_CHECK_RESULT`, centreon.md:140); Nagios+SNMPTT (same external command file, nagios-snmptt.md:431-440).
  - Trade-offs (+): inherits the entire Nagios state machine, escalation, and notification routing; no new infrastructure if Nagios already exists.
  - Trade-offs (-): no first-class trap row by default (Centreon: `log_traps` is opt-in twice; Nagios: relies on operator-curated SNMPTT MySQL); 5-level SNMP severity collapses to 3 Nagios states (`nagios-snmptt.md:700-711`).

- **(c) Metric / item value in the time-series pipeline**
  - Systems: Zabbix (history_log/history_text item value, zabbix.md:39-48); Telegraf (`snmp_trap` measurement, telegraf.md:38).
  - Trade-offs (+): trap is queryable next to polled metrics with the same backend; cross-signal correlation by `source` tag/label is trivial.
  - Trade-offs (-): no native event semantics — open/ack/clear becomes "is this metric above threshold in the last window?"; trap-text-as-LOG value-type leaks high-cardinality varbinds into the TSDB unless operator carefully preprocesses.

- **(d) Log document in a log indexer / log-event store**
  - Systems: Logstash → Elasticsearch (logstash.md:34-37); Datadog Agent → SaaS Logs (`network-devices-snmp-traps`, datadog-agent.md:20); Splunk SC4SNMP → HEC `sourcetype=sc4snmp:traps` (splunk-sc4snmp.md:33); Dynatrace → Grail log event (dynatrace.md:36-39); LogicMonitor → LM Logs (LogSource path, logicmonitor.md:33); SolarWinds → Log Analyzer DB (current-version, solarwinds.md:41-42).
  - Trade-offs (+): unifies traps with syslog and other event sources; SaaS-tier search and dashboarding apply; no on-prem state tier.
  - Trade-offs (-): no alarm lifecycle, dedup, or correlation in the on-prem agent; all alerting work moves to the SaaS / downstream tier.

- **(e) Event for a Nagios-family check_result + a separate eventlog row**
  - Systems: LibreNMS (eventlog row + per-handler relational updates + AlertRule re-eval, librenms.md:386).
  - Trade-offs (+): handler code can update first-class relational state (ports.ifOperStatus etc.) directly, shortening latency.
  - Trade-offs (-): no dedup at the eventlog row (every trap = 1+ rows); alerting via SQL query against eventlog is operator-built.

- **(f) Cribl event (pure pass-through to operator-chosen Destination)**
  - Systems: Cribl Stream (cribl.md:32-37).
  - Trade-offs (+): maximum flexibility; one event can fan out to N destinations (SNMP / Splunk / S3 / Kafka / OTLP).
  - Trade-offs (-): no storage, no alerting, no severity in Cribl itself — every downstream concern lives in another product.

---

## Fork 2: MIB / profile authoring model

**Question**: How does the operator translate vendor MIB OIDs into actionable trap definitions?

- **(a) Centralized XML/JSON/YAML event definitions, DB-loaded**
  - OpenNMS: 230 `.events.xml` files with explicit `<mask>` + `<alarm-data reduction-key>` + `<severity>` (opennms.md:191-194).
  - Centreon: 214 seeded rows in MariaDB `traps` table + matching/preexec/groups tables (centreon.md:269-273, 444-470).
  - Datadog Agent: 11,000+ MIBs compiled into JSON/YAML `dd_traps_db.*` files at build time (datadog-agent.md:387, 394).
  - (+): centralized, queryable, versioned, diffable.
  - (-): every change requires authoring the XML/SQL/YAML form correctly; vendor MIB authoring tooling is separate (`mib2opennms`, `centFillTrapDB`, `ddev meta snmp generate-traps-db`).

- **(b) ZODB / object-database Python with transforms**
  - Zenoss: EventClass tree with inherited Python transforms (zenoss.md:301-313).
  - (+): max flexibility — full Python access to `dmd`, `device`, `evt`; transforms are inherited and reusable.
  - (-): transforms are unsandboxed code; `customcode` security risk; transform exceptions kill the worker fork.

- **(c) Flat text EVENT blocks in `snmptt.conf`**
  - Nagios+SNMPTT: vendor MIBs converted via `snmpttconvertmib` to per-MIB conf snippets (nagios-snmptt.md:259-272).
  - (+): human-readable, easy to grep, version-control friendly.
  - (-): no DB schema, no automated dependency resolution; ordering matters per-file; lossy 5→3 severity mapping; v1.4 (NSTI default) has documented spool-race fix gap (nagios-snmptt.md:755-758).

- **(d) Python rules / regex via WATO**
  - CheckMK: rule packs with `match_application`/`match` (text) over flattened `application=trapOID + text=joined-varbinds` event (checkmk.md:300-303).
  - (+): unified syntax across traps, syslog, named-pipe events.
  - (-): varbinds flattened lose structure (single text field), limits cross-varbind reasoning; no MIB compiler — MIBs uploaded via WATO + PySMI.

- **(e) Handler-as-code (PHP classes registered by OID)**
  - LibreNMS: 181 OID → 177 PHP handler classes; `Fallback::class` for unmatched (librenms.md:474-489).
  - (+): IDE auto-complete, refactoring, typed enums; per-handler tests.
  - (-): fork the repo to add custom handler; no plugin discovery; OID → class registry is build-time.

- **(f) Native trap items per host (regex over textual snmptrapd output)**
  - Zabbix: `snmptrap[regex]` and `snmptrap.fallback` items per host (zabbix.md:497-499).
  - (+): unified with item/trigger model; severity is a trigger property.
  - (-): no central catalogue; vendor MIB normalization left to operator templates; built-in templates ship zero per-OID items (zabbix.md:843-845).

- **(g) Email-vendor / round-trip-through-vendor**
  - SolarWinds: "If you have a specific device MIB, you can have it added to the SolarWinds MIB database" (solarwinds.md:258).
  - (+): vendor-curated quality; no local compiler complexity.
  - (-): slow update cycle; operator cannot solve their own problem locally; no first-party diff/version-control.

- **(h) Declarative YAML extension package + operator-shipped MIBs**
  - Dynatrace: extension YAML with `snmptraps` node; MIBs in extension `snmp/` dir or ActiveGate `mib-files-custom/` (dynatrace.md:296-304).
  - (+): self-contained extension packaging; versionable via the Hub.
  - (-): bundled SNMP Traps extension uses fixed predefined OID set and ignores `mib-files-custom/` (dynatrace.md:277-278); custom MIBs require authoring a custom extension.

- **(i) Operator-supplied CSV lookups via pipeline functions**
  - Cribl: external `snmptranslate`/`pysnmp` → CSV → Lookup function (cribl.md:214-222).
  - (+): pipeline-tier flexibility; one CSV can be reused across syslog and trap pipelines.
  - (-): no embedded MIB compiler; the community pack ships 168 CSVs covering ~92,000 OIDs but is operator-maintained.

- **(j) Portal MIB upload + local JSON converter**
  - LogicMonitor: portal upload (Collector 38.300+) OR local Python `snmpMibsToJsonConversionUtil` for custom MIBs (logicmonitor.md:320-339).
  - (+): server-validated, replicated to all Collectors, RBAC-controlled.
  - (-): two paths exist (portal + local); operator must understand precedence (portal > custom JSON > bundled core).

- **(k) Per-CSV-supplied catch-all (no per-trap definition at all)**
  - Telegraf: trap → metric, no event-definition layer at all; OID translation via gosmi/netsnmp at decode (telegraf.md:165-167).
  - (+): minimal config surface.
  - (-): trap DROPPED on OID lookup failure (telegraf.md:236-247); no native severity.

---

## Fork 3: OOB coverage strategy

**Question**: How much vendor knowledge does the system ship by default?

- **(a) Ship nothing; operator builds all coverage**
  - Nagios+SNMPTT (nagios-snmptt.md:884-892): zero bundled MIBs, zero EVENTs.
  - CheckMK (checkmk.md:758-760): zero vendor MIBs, zero example rule packs.
  - Sensu (sensu.md:245): zero bundled MIB store in Classic ext production gem.
  - Telegraf (telegraf.md:227-228): zero MIB files; operators install Net-SNMP.
  - Logstash: libsmi 0.5.0 IETF set only (logstash.md:205-208); no vendor MIBs.
  - (+): minimal install footprint; no maintenance debt for vendor MIB updates the operator doesn't use.
  - (-): day-1 trap visibility approaches zero; every customer rebuilds the same vendor catalogue.

- **(b) Ship curated subset, operator activates by binding**
  - Centreon: 214 trap-definition rows + 8 vendors seeded, NO service relations (centreon.md:269-275). Definitions catalogued but not actionable until operator binds them to passive services.
  - OpenNMS: 17,442 event-XML definitions across 230 files but NOT auto-loaded (opennms.md:191-196); operator uploads per-vendor file via REST.
  - (+): curated catalogue head-start; opt-in growth.
  - (-): non-actionable until binding step; operators frequently miss the binding requirement on first install.

- **(c) Ship vendor packs (curated per-vendor coverage)**
  - LibreNMS: 4,770 MIB files in tree, 371 vendor dirs; per-handler PHP classes for ~177 OIDs (librenms.md:208-211, librenms.md:333).
  - SolarWinds: "over 250,000 precompiled unique OIDs" via `MIBs.msi` (solarwinds.md:251).
  - LogicMonitor: enumerated vendor list at supported-mibs page; long-tail vendor breadth (logicmonitor.md:362-368).
  - (+): broad day-1 coverage.
  - (-): vendor coverage breadth depends on contributor/vendor cadence; legacy vendors may lag.

- **(d) Ship comprehensive bundle covering most modern vendors**
  - Datadog Agent: closed bundle covering "more than 11,000 MIBs" (datadog-agent.md:394).
  - (+): largest bundled coverage in cohort.
  - (-): bundle is closed (Omnibus-only); operators cannot diff or audit the shipped MIB set; integration must use `ddev meta snmp generate-traps-db` for additions.

- **(e) Ship community-pack add-on, ship nothing in product**
  - Cribl Stream: nothing in-product (cribl.md:211); community pack ~92,000 OIDs in 168 CSVs (cribl.md:217-218).
  - (+): pack is upgradeable independently of Stream.
  - (-): coverage burden on operator if pack lags upstream MIB updates.

---

## Fork 4: Storm / backpressure model

**Question**: What happens when a trap storm arrives?

- **(a) Block-when-full back-pressure with bounded queue**
  - OpenNMS: `isBlockWhenFull(): return true`; UDP reader blocks; kernel UDP becomes the burst absorber (opennms.md:131, opennms.md:507-512).
  - (+): no in-JVM drops; visible at kernel netstat counter.
  - (-): no per-source limit; no storm-aware suppression.

- **(b) Bounded async queue with silent overflow**
  - Datadog Agent: channel size 100 (datadog-agent.md:248-249); listener blocks; kernel UDP absorbs; no high-watermark drop signal.
  - (+): simple model.
  - (-): under storm, decoder + USM + formatter run for every packet even if downstream would discard.

- **(c) Post-processing event-count gate**
  - CheckMK: `event_limit` caps open events per-host/per-rule/overall (checkmk.md:585-591); listener does no rate-limit.
  - (+): protects in-RAM event store.
  - (-): does not protect listener CPU; all packets still decoded and rule-matched.

- **(d) No protection at any layer**
  - Centreon (centreon.md:771): explicit "Per-source rate limits — not implemented at the trap layer".
  - Zabbix (zabbix.md:957): "no per-source rate limit, no storm detection".
  - LibreNMS (librenms.md:643): "None in LibreNMS".
  - Splunk SC4SNMP: implicit (Celery queue can grow; Redis AOF persistence is the only buffer).
  - SolarWinds: no trap-pipeline self-telemetry documented (solarwinds.md:170).
  - Dynatrace (dynatrace.md:239): vendor admits "many of them may be dropped by the operating system".
  - LogicMonitor (logicmonitor.md:43): SNMP traps EXCLUDED from Auto-Balanced Collector Groups; manual placement required.

- **(e) Pipeline-tier dedup primitive (operator-applied)**
  - Cribl: `Suppress` function dedups on a JS expression; operator-defined window (cribl.md:249).
  - Telegraf: `processors.dedup` downstream (telegraf.md:317-318); no built-in.
  - Logstash: `aggregate` + `throttle` filters (logstash.md:312-314); operator-built.

- **(f) Per-trap timestamp-bookmark with cross-node hash resume**
  - Zabbix: SHA-512 content hash of trap text; resume after HA failover at last hash boundary OR ±60s timestamp window (zabbix.md:188-195).
  - (+): cross-node resume without shared storage.
  - (-): bookkeeping is per-record, not per-storm.

---

## Fork 5: Correlation surface

**Question**: After a trap is decoded, what other data is available for correlation?

- **(a) Trap-only, no correlation**
  - Nagios+SNMPTT, Sensu, Telegraf, Logstash (forwarders).
  - (+): minimal scope.
  - (-): no topology/polling cross-reference at the trap layer.

- **(b) Topology-annotated alarms (separate engine)**
  - OpenNMS: alarmd + Drools `situations.drl` (opennms.md:419-420); operator-built or via OCE plug-in.
  - (+): real topology correlation possible.
  - (-): not out-of-the-box; requires Drools authoring.

- **(c) Polling-state cross-reference (per-handler)**
  - Zenoss: `UpsTrapOnBattery` writes back to `sensors` (zenoss.md:511).
  - LibreNMS: `LinkUp`/`LinkDown` directly updates `ports.ifOperStatus` (librenms.md:354).
  - (+): trap-driven state update reduces latency vs next poll cycle.
  - (-): tightly couples handler code with database schema; narrow per-vendor coverage.

- **(d) Topology relation surface but NOT used at trap reception**
  - OpenNMS, Zenoss, LibreNMS, SolarWinds (Network Atlas / Orion Maps), Dynatrace (SmartScape), LogicMonitor (TopologySources).
  - (+): topology exists for visualisation/drill.
  - (-): no topology-aware trap suppression in any system.

- **(e) Pipeline-tier enrichment via Lookup**
  - Cribl: Lookup function joins against a CSV (cribl.md:285-287).
  - Logstash: `translate`/`elasticsearch` filters.
  - (+): cross-signal enrichment reusable across syslog and trap pipelines.
  - (-): topology must be supplied externally; static at-most.

- **(f) NetFlow / flow cross-reference**
  - No system in the cohort ships native trap↔NetFlow correlation at the trap pipeline.

---

## Fork 6: UI surface

**Question**: How does the operator see trap content and act on it?

- **(a) Dedicated Trap UI / Event Console**
  - OpenNMS (Events + Alarms tables; Vue3 Trapd Config; opennms.md:361-368).
  - Centreon (Configuration → SNMP traps + 4 sub-pages; centreon.md:594-600).
  - NSTI for Nagios+SNMPTT (Flask Traplist/Inspector; nagios-snmptt.md:583-589).
  - SolarWinds (legacy Trap Viewer; current Log Viewer; solarwinds.md:447-449).

- **(b) Generic events/logs UI consuming traps as one source**
  - Zenoss (ExtJS Events Console).
  - CheckMK (Event Console — unified with syslog).
  - Datadog Agent → Logs Explorer (`source:snmp-traps`).
  - Splunk SC4SNMP → Splunk index `netops`.
  - Cribl (Cribl visual pipeline builder; downstream destination has the UI).
  - Dynatrace (Logs Viewer + Unified Analysis screen).
  - LogicMonitor (portal alert/event UI).

- **(c) Function-pattern (operator runs query/function)**
  - LibreNMS (eventlog SQL queries; librenms.md:894).
  - Zabbix (Latest Data → snmptrap items per host).

- **(d) No UI**
  - Telegraf (telegraf.md:476): "no GUI" — downstream output backend owns the UI.
  - Logstash (logstash.md): Kibana via Elasticsearch output.
  - Sensu (Sensu UI generic; no trap-specific surface).

---

## Fork 7: Operational footprint

**Question**: What infrastructure must the operator stand up to receive traps?

- **(a) Single process on a single host (lightest)**
  - Telegraf (one Go binary).
  - LibreNMS (snmptrapd + per-trap PHP CLI; no daemon).
  - Datadog Agent (one Go binary on each host).
  - Dynatrace ActiveGate (one host-based daemon).

- **(b) Multi-process + central database**
  - OpenNMS (JVM + PostgreSQL).
  - Centreon (snmptrapd + centreontrapdforward + centreontrapd Perl + MariaDB + Gorgone).
  - LibreNMS (with MariaDB).
  - SolarWinds (Windows service + SQL Server + IIS + Web Console).

- **(c) Multiple daemons + queue + state backends (heavy)**
  - Zenoss (zentrap + zeneventd + zeneventserver + RabbitMQ + Redis + MariaDB + Lucene/Solr).
  - Nagios+SNMPTT (snmptrapd + SNMPTT + Nagios + NSCA/NRDP + NSTI + MySQL).
  - Splunk SC4SNMP (Kubernetes pods: trap + worker-trap + worker-sender + mibserver + MongoDB + Redis).

- **(d) Distributed pipeline with control-plane**
  - Cribl (Leader Node + N Worker Nodes; OpenAPI / SDK / Terraform).
  - LogicMonitor (per-site Collector + SaaS control plane).

- **(e) Single-tenant SaaS architecture (closed-source on-prem agent + SaaS backend)**
  - Datadog (Agent + Datadog SaaS).
  - Splunk SC4SNMP (Connector + Splunk Cloud/Enterprise).
  - Dynatrace (ActiveGate + Dynatrace SaaS/Managed).
  - LogicMonitor (Collector + LM Envision SaaS).
  - Cribl Stream (Worker + Cribl.Cloud or self-hosted).
  - SolarWinds (proprietary; no SaaS equivalent, Hybrid Cloud Observability is a self-hosted bundle).

- **(f) Cluster-deployment-only (no single-binary path)**
  - Splunk SC4SNMP: Helm-on-Kubernetes or Docker-Compose ONLY — no `./snmp-trap-receiver --config foo.yaml` binary (splunk-sc4snmp.md:35).

---

## Fork 8: Real-time vs batched alerting

**Question**: Latency from trap arrival to operator notification.

- **(a) Real-time, sub-second alarm engine**
  - OpenNMS: alarmd on event bus + Drools rules.
  - Zenoss: zeneventd → ZEP.
  - CheckMK: in-process synchronous rule evaluation.
  - Sensu Go: backend in-process.

- **(b) Real-time at decode, but cron-driven delivery**
  - LibreNMS (librenms.md:521-525): alert state synchronous in `Dispatcher::handle`; notification transport via `alerts.php` 1-minute cron — trap-to-page latency = handler + ≤60s cron + transport delivery.

- **(c) Polled (filesystem spool)**
  - Centreon: 2-second polling default (centreon.md:200) — median ~1s latency.
  - Nagios+SNMPTT: SNMPTT daemon polls `/var/spool/snmptt/` (default `sleep=5s`, nagios-snmptt.md:217).

- **(d) Stream + SaaS-side rule evaluation**
  - Datadog Agent (datadog-agent.md:21): SaaS Logs Monitor.
  - Splunk SC4SNMP (splunk-sc4snmp.md:511): Splunk SPL searches/alerts.
  - Dynatrace (dynatrace.md:526-528): log event rules in Grail → Davis problem; rate caps documented at SaaS-side.
  - LogicMonitor: portal Alert Rule + Escalation Chain (logicmonitor.md:498).

- **(e) Pipeline-tier only; alerting downstream**
  - Telegraf, Logstash, Cribl Stream — operator's downstream backend (Kapacitor, Watcher, Splunk monitor, etc.) decides latency.

---

## Fork 9: Northbound forwarding (re-emit traps)

**Question**: Can the system EMIT SNMP traps to an upstream NMS?

- **(a) Native multi-version trap + inform emitter, configurable per alarm**
  - OpenNMS: `snmptrap-northbounder` v1/v2c/v3 + v2-inform/v3-inform (opennms.md:438). SpEL-mapped varbinds.
  - Zenoss: SNMPTrapAction (v1/v2c) + SNMPv3Action (v3) — hardcoded ZENOSS-MIB varbinds (zenoss.md:480-496).

- **(b) Shell-out to external snmptrap CLI per alert**
  - LibreNMS: SNMPv2c only via `snmptrap -v 2c` shell-out, `LIBRENMS-NOTIFICATIONS-MIB` (librenms.md:558-612).
  - Centreon: `@TRAPFORWARD()@` macro, v2c only, hard-coded community `centreon` (centreon.md:696-707).

- **(c) Trap forwarding via verbatim re-emission + serialize-from-event**
  - Cribl: SNMP Trap Destination (verbatim from `__snmpRaw`) + `snmp_trap_serialize` Function (re-build PDU after pipeline mod) (cribl.md:259-296).

- **(d) Alert action with Trap Templates**
  - SolarWinds: Trap Templates emit outbound v1/v2c traps; multiple destinations supported (solarwinds.md:50-52).

- **(e) Outbound by external script only**
  - Nagios+SNMPTT (nagios-snmptt.md:1103): no native; operator wires `snmptrap` into `commands.cfg`.

- **(f) No northbound trap path**
  - CheckMK (checkmk.md:524): explicit "No native outbound trap forwarding".
  - Zabbix (zabbix.md:611-619): no `MEDIA_TYPE_SNMP_TRAP`.
  - Telegraf (telegraf.md:40): no `outputs.snmp_trap`.
  - Datadog Agent (datadog-agent.md:24): no trap emit.
  - Splunk SC4SNMP, Dynatrace (dynatrace.md:40), LogicMonitor (logicmonitor.md:38): no native northbound.

---

## Fork 10: Security defaults

**Question**: How safe is the trap subsystem out of the box?

- **(a) Open community + open allowlist + plaintext credential**
  - LibreNMS shipped default: `disableAuthorization yes` plus `authCommunity log,execute,net <community>` (librenms.md:147); v3 USM not configured.
  - Nagios+SNMPTT installer (`install.sh` / `snmptt_deploy.sh`): malformed `traphandle default` line (nagios-snmptt.md:768-785); community `public`; MySQL root hardcoded to `nagiosxi` (nagios-snmptt.md:801-802); NSTI Flask UI is unauthenticated.
  - Zabbix container default: `authCommunity log,execute,net public` + `disableAuthorization yes` (zabbix.md:162-167).
  - Centreon postinstall: same pattern with `disableAuthorization yes` (centreon.md:1322).

- **(b) Plaintext-on-disk credentials**
  - Zenoss: ZODB plaintext for v3 passphrases (zenoss.md:583).
  - CheckMK: plaintext Python literals in `global.mk` (checkmk.md:639).
  - SolarWinds: `Orion.Traps.Community` exposed plaintext via SWIS API (solarwinds.md:421-425).

- **(c) Secret-store integration**
  - OpenNMS: Secure Credentials Vault `${scv:...}` (opennms.md:533); known issue: REST GET returns plaintext.
  - Telegraf: `config.Secret` for v3 (telegraf.md:402).
  - Splunk SC4SNMP: k8s Secrets for v3 USM (`charts/.../traps/deployment.yaml:158-178`).

- **(d) RBAC, but single-tenant trap pipeline**
  - OpenNMS, CheckMK, Centreon, Zabbix, LibreNMS, SolarWinds — all have user/role RBAC for the UI but the trap listener itself is single-tenant.

- **(e) Multi-tenant trap pipeline**
  - Dynatrace (tenant-scoped; ActiveGate group per environment).
  - LogicMonitor (MSP mode at portal level; logicmonitor.md:593).

- **(f) SaaS-managed credential plane**
  - Datadog Agent, Dynatrace, LogicMonitor: credentials managed via SaaS UI; on-prem agent reads from local config/secret.

---

## Read-verification block

Read `snmp-traps-in-observability.md` in whole (1038 lines, last line: "---")
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
