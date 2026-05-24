# SNMP Trap Profile Inventory — per-vendor per-OID classification datasets surfaced in the cohort

Inventory of reusable trap-classification datasets observable across the 16 system specs.

---

## 1. OpenNMS event-XML corpus

- **Source system**: OpenNMS (opennms.md:191-194, 220).
- **Format**: XML — `<event>` definitions with `<mask>` (id/generic/specific/varbind matchers), `<uei>`, `<descr>`, `<logmsg>`, `<severity>`, `<alarm-data reduction-key="..." clear-key="..."/>`, `<varbindsdecode>` (enum-to-string maps).
- **Approximate count**: **230 `.events.xml` files** in `opennms-base-assembly/src/main/filtered/etc/examples/events/` containing **17,442 `<event>` tags** (verified via reproducible grep). Not all are trap-derived; a fraction are syslog/wmi events bundled in the same files.
- **License**: AGPLv3 (opennms.md:19) — OSI-approved.
- **Update cadence**: per OpenNMS release; vendor MIB-derived; community PRs.
- **Fitness for derivation**: **HIGH** — fully open, XML schema documented, parseable. Path to derivation is clear (parse `<mask>`, `<uei>`, `<severity>`, `<alarm-data>`).

## 2. Centreon `traps_*` MariaDB seed

- **Source system**: Centreon (centreon.md:269-275).
- **Format**: SQL INSERT statements in `centreon/www/install/insertBaseConf.sql` populating MariaDB `traps_vendor` + `traps` tables.
- **Approximate count**: **8 `traps_vendor` rows** (Cisco, HP, 3com, Linksys, Dell, Generic, Zebra, HP-Compaq) + **214 `traps` rows** (verified by `grep -cE '^INSERT INTO \`traps\`'`).
- **License**: Apache-2.0 for the trap-engine package (centreon.md:24); SQL data file under same license.
- **Update cadence**: per Centreon release.
- **Fitness for derivation**: **HIGH** — flat SQL, openly inspectable.

## 3. LibreNMS bundled MIB tree

- **Source system**: LibreNMS (librenms.md:208-214).
- **Format**: ASN.1 MIB text files.
- **Approximate count**: **4,770 MIB files** in `mibs/` ; **371 vendor directories** ; **2,245 contain at least one `NOTIFICATION-TYPE` or `TRAP-TYPE` macro**.
- **License**: GPL-3.0-or-later (LibreNMS repo).
- **Update cadence**: maintainers add MIBs as new collector OS modules are merged.
- **Fitness for derivation**: **HIGH** — raw MIB sources, full ASN.1, vendor-broad coverage.

## 4. LibreNMS PHP handler classes

- **Source system**: LibreNMS (librenms.md:333, 488).
- **Format**: PHP classes implementing `LibreNMS\Interfaces\SnmptrapHandler`; registered in `config/snmptraps.php` OID→class array.
- **Approximate count**: **181 OID→class mappings** (177 distinct handler classes; some handlers serve 2-3 OIDs).
- **License**: GPL-3.0-or-later.
- **Update cadence**: community PRs.
- **Fitness for derivation**: **MEDIUM-HIGH** — handler logic is PHP code, not declarative data, but the registry plus handler bodies are inspectable and code-pattern-derivable.

## 5. Zenoss seeded events.xml + ZenPack ecosystem

- **Source system**: Zenoss (zenoss.md:241-243, 246-249).
- **Format**: XML `EventClass` / `EventClassInst` seeded via `src/Products/ZenModel/data/events.xml`; ZenPacks ship MIBs at install time.
- **Approximate count**: **6,920-line `events.xml`** with **136 EventClass organizers + 339 EventClassInst mappings**, of which **3 are `snmp_*`-named** (snmp_authenticationFailure, snmp_linkDown, snmp_linkUp) plus additional vendor-specific trap-derived mappings; broader vendor coverage from ZenPacks (count unverifiable from mirror).
- **License**: GPL-2 with linking exception (zenoss.md:36); ZenPack licenses vary.
- **Update cadence**: per Zenoss release + community ZenPacks.
- **Fitness for derivation**: **MEDIUM** — XML is open; transforms are Python (operator-authored, harder to derive from generic data).

## 6. Datadog Agent `dd_traps_db.*` bundle

- **Source system**: Datadog Agent (datadog-agent.md:387, 394).
- **Format**: JSON or YAML (optionally gzip-compressed) with `traps:` (per trap OID) and `vars:` (per variable OID) sections. Each entry: `name`, `mib`, `descr`; variables additionally carry `enum` and `bits` maps.
- **Approximate count**: vendor claim "more than 11,000 MIBs". The actual file ships inside the closed Omnibus installer.
- **License**: Apache-2.0 for `comp/snmptraps/` source; the generated `dd_traps_db.*` file is build output, not committed to the open repo. The integrations-core tooling (`ddev meta snmp generate-traps-db`) is Apache-2.0.
- **Update cadence**: tied to integrations-core releases.
- **Fitness for derivation**: **MEDIUM** — the JSON schema (`oidresolver/def/traps_db.go:13-47`) is open and parseable; the bundle itself requires extraction from an installed Agent.

## 7. ZENOSS-MIB outbound trap schema

- **Source system**: Zenoss (zenoss.md:482).
- **Format**: ZENOSS-MIB (`baseOID = '1.3.6.1.4.1.14296.1.100'`); fixed 35-varbind layout under `1.3.6.1.4.1.14296.1.100.0.0.1`.
- **Approximate count**: 1 outbound trap OID, ~35 fixed varbinds.
- **License**: ships with Zenoss; same license as the daemon.
- **Update cadence**: stable; rarely changed.
- **Fitness for derivation**: **HIGH** as a *reference for outbound* trap shape, not for inbound classification.

## 8. LIBRENMS-NOTIFICATIONS-MIB

- **Source system**: LibreNMS (librenms.md:597).
- **Format**: ASN.1 MIB under IANA PEN 60652 with stateClear/stateActive/stateAcknowledged/stateWorse/stateBetter enum and ~21 attributes (defaultAlertTitle, defaultAlertID, etc.).
- **License**: GPL-3.0-or-later.
- **Update cadence**: per LibreNMS release.
- **Fitness for derivation**: **HIGH** for reference outbound semantics.

## 9. SENSU-ENTERPRISE MIBs

- **Source system**: Sensu (sensu.md:257-265).
- **Format**: 5 ASN.1 MIB files shipped under PEN 1.3.6.1.4.1.45717 (`SENSU-ENTERPRISE-{ROOT,V1,NOTIFY}-MIB` + RFC-1212-MIB + RFC-1215-MIB).
- **License**: ships with `sensu-snmp-trap-handler` (MIT/Apache or per the repo).
- **Update cadence**: legacy; Sensu Enterprise is EOL.
- **Fitness for derivation**: **MEDIUM** (reference only; ecosystem deprecated).

## 10. SNMPTT EVENT-block conventions

- **Source system**: Nagios+SNMPTT (nagios-snmptt.md:346-368, 384-388).
- **Format**: flat text `snmptt.conf` with `EVENT <name> <OID> <category> <severity>` + `FORMAT` (with `$1..$n`, `$A`, `$aA`, `$H`, `$x`, `$X` substitutions) + optional `NODES`, `MATCH`, `REGEX`, `PREEXEC`, `EXEC`, `SDESC`/`EDESC`.
- **Approximate count**: zero shipped — every operator builds their own catalogue via `snmpttconvertmib`.
- **License**: SNMPTT is GPL v2 or later (nagios-snmptt.md:39).
- **Update cadence**: upstream SNMPTT v1.5 is the latest (2022). The NSTI deploy script pulls v1.4 (older).
- **Fitness for derivation**: **HIGH** for syntax and conventions; **LOW** as a vendor catalogue since none is bundled.

## 11. Centreon CLAPI EXPORT (per-installation catalogue dump)

- **Source system**: Centreon (centreon.md:1092).
- **Format**: line-oriented CLAPI export: `TRAP;ADD;<name>;<oid>;…`, `TRAPMATCHING;ADD;…`, `VENDOR;ADD;…`.
- **Approximate count**: depends on installation; an operator can re-export an existing catalogue.
- **License**: Apache-2.0 (trap-engine package); GPL-2.0 (centreon-collect).
- **Fitness for derivation**: **MEDIUM** — useful as a per-vendor extraction artifact.

## 12. Cribl SNMP Trap Varbind Translation Pack

- **Source system**: Cribl Stream (cribl.md:217-218).
- **Format**: CSV files with `oid, oidName, mibName, objects` columns.
- **Approximate count**: **168 CSV lookups parsed from over 15,000 upstream MIBs containing ~92,000 trap OIDs**.
- **License**: community-maintained; license per the pack page — Cribl Packs marketplace.
- **Update cadence**: community; operator may need to update manually as MIBs evolve.
- **Fitness for derivation**: **HIGH** — flat CSV, mechanically parseable, broad OID coverage. The largest single classifier-grade dataset surfaced in the cohort.

## 13. SolarWinds MIB DB (`MIBs.msi`)

- **Source system**: SolarWinds (solarwinds.md:251).
- **Format**: Windows MSI installer; on-disk store format proprietary and not publicly documented.
- **Approximate count**: vendor claim "over 250,000 precompiled unique OIDs from hundreds of standard and vendor MIBs".
- **License**: proprietary (SolarWinds commercial).
- **Update cadence**: vendor-controlled; operators submit MIBs to vendor for inclusion (solarwinds.md:258).
- **Fitness for derivation**: **NONE** — closed-source, vendor-curated; not usable as a reference dataset.

## 14. LogicMonitor LogicModule exchange (EventSources, LogSources)

- **Source system**: LogicMonitor (logicmonitor.md:64).
- **Format**: Groovy-scripted EventSources + YAML-shaped LogSources; downloadable from `exchange.logicmonitor.com`.
- **Approximate count**: per-vendor; not publicly enumerated.
- **License**: closed commercial.
- **Update cadence**: vendor-curated.
- **Fitness for derivation**: **LOW** (vendor-locked).

## 15. Splunk SC4SNMP custom-trap YAML (per-tenant operator-authored)

- **Source system**: Splunk SC4SNMP (splunk-sc4snmp.md:519).
- **Format**: per-operator YAML under `scheduler.customTranslations` and `traps_db/`.
- **License**: Apache-2.0 (SC4SNMP repo).
- **Update cadence**: per-tenant.
- **Fitness for derivation**: **MEDIUM** as a schema example; zero bundled vendor content.

## 16. Dynatrace SNMP Traps extension bundled OIDs

- **Source system**: Dynatrace (dynatrace.md:283).
- **Format**: declarative YAML inside the signed extension package.
- **Approximate count**: vendor docs do not enumerate; inferred to include at least IF-MIB::linkDown/linkUp.
- **License**: proprietary commercial.
- **Update cadence**: per extension version on Dynatrace Hub.
- **Fitness for derivation**: **LOW** — vendor bundle.

---

## Summary — datasets best suited for derivation

For an open-source implementation that needs a starter catalogue:

1. **OpenNMS event-XML corpus** — 17,442 `<event>` definitions, AGPLv3, MIB-derived, severity + reduction-key already encoded.
2. **LibreNMS MIB tree + handler PHP** — 4,770 MIB files, 371 vendor dirs, GPL-3.0-or-later.
3. **Centreon `traps_*` SQL seed** — 214 rows, Apache-2.0.
4. **Cribl community pack CSVs** — 168 CSVs covering ~92,000 OIDs (community license, see Cribl Packs marketplace for terms).
5. **Datadog Agent `dd_traps_db.*`** — schema is open (`oidresolver/def/traps_db.go:13-47`); the bundle file is build output. Apache-2.0 for the tooling.

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
