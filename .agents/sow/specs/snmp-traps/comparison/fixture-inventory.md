# SNMP Trap Test-Fixture Inventory — artefacts usable to validate a new trap implementation

This inventory catalogues every shipped test artefact (fixtures, fixtures-generators, integration tests, simulators) the open-source cohort exposes that could be used to validate a trap implementation. Only test fixtures from systems whose source is in the mirror are included; closed-source systems (SolarWinds, Dynatrace, LogicMonitor, Cribl) ship no public test fixtures and are omitted.

---

## 1. OpenNMS — udpgen + multiple test layers

### 1.1 OpenNMS/udpgen (synthetic trap load tester)

- **Source**: opennms.md:587-591 — `OpenNMS/udpgen` companion repo @ `500967216ddad627480b7d204411a3ec6b1ec4b0`.
- **Format**: C++ + libnet-snmp; multi-threaded SNMPv2c trap generator.
- **Coverage**: hard-coded `coldStart`-shaped trap with mis-typed OID (`.1.3.6.1.1.6.3.1.1.5.1`, extra `.1` at position 4 — opennms.md:591). Default target port 1162.
- **Location**: `OpenNMS/udpgen @ 500967216 :: trap_generator.cpp`.
- **License**: AGPLv3 (OpenNMS family).
- **Fitness**: useful for load testing; the OID typo means it follows the unmatched-trap path rather than the documented `SNMP_Cold_Start` UEI.

### 1.2 OpenNMS in-product trap tests

- **Source**: opennms.md:551-572 — 13 test classes + 1 helper fixture under `features/events/traps/src/test/java/org/opennms/netmgt/trapd/`.
- **Notable tests**: `TrapdInformIT.java` (273 lines, v3 inform PDU); `Snmp4JTrapHandlerIT.java`; `TrapdIT.java` (459 lines, end-to-end with real socket); `TrapdConfigReloadIT.java`; `NMS19070IT.java` (JIRA regression).
- **Fixtures**: `features/events/traps/src/test/resources/org/opennms/netmgt/trapd/eventconf.xml` (minimal eventconf); `trapd-configuration.xml` (port 1163).
- **License**: AGPLv3.
- **Fitness**: high. These are integration tests using `SnmpUtils.getV1TrapBuilder()` / `getV2TrapBuilder()` — they exercise the entire decode + match + dispatch flow with real sockets.

### 1.3 OpenNMS smoke tests

- **Source**: opennms.md:572-577 — `smoke-test/src/test/java/org/opennms/smoketest/minion/{TrapIT,TrapWithGrpcIT,TrapdWithKafkaIT}.java` (176, 197, 188 lines).
- **Format**: Docker-Compose-style: spin up OpenNMS core + Minion + PostgreSQL; send real traps via gosnmp.
- **Coverage**: v1, v2c, v3 USM round-trip; tests `SNMP_Warm_Start` UEI matching plus `TrapWithGrpcIT` (gRPC sink), `TrapdWithKafkaIT` (Kafka sink).
- **License**: AGPLv3.

---

## 2. Zenoss

### 2.1 zentrap unit tests

- **Source**: zenoss.md:609-619.
- **Files**: `test_decode.py` (123 lines), `test_filterspec.py` (739 lines), `test_handlers.py` (898 lines), `test_oidmap.py` (58 lines), `test_trapfilter.py` (674 lines). Total ~2,492 lines.
- **Format**: Python unittest with synthetic `FakePacket` constructor pattern.
- **Coverage**: decoders (OID tuple, UTF-8, IPv4, IPv6, DateAndTime, base64); v1 + v2/v3 handlers; trap filter language; OidMap; varbind processor modes.
- **License**: GPL-2 with linking exception (Zenoss).
- **Fitness**: high — comprehensive decoder + filter coverage, but synthetic packets only.

### 2.2 Zenoss real PCAP fixture

- **Source**: zenoss.md:632 — `src/Products/ZenEvents/tests/trapdump.pcap` (436 bytes, 2 packets).
- **Format**: real captured PCAP plus `sendSnmpPcap.py` extractor (uses `tcpdump -x -r` to strip the 28-byte UDP header and send via `socket.socket(AF_INET, SOCK_DGRAM)`).
- **Coverage**: 2 v1/v2c sample traps.
- **License**: GPL-2.
- **Fitness**: high — real PDU bytes, replayable into any trap receiver.

### 2.3 pynetsnmp example `trapd.c`

- **Source**: zenoss.md:626 — `pynetsnmp/example/trapd.c` (113 lines).
- **Format**: C SNMPv3 trap receiver demonstrating libnet-snmp USM call pattern.
- **Coverage**: hard-codes a v3 user `"-e 0x8000000001020304 traptest SHA mypassword AES"`.
- **License**: same as pynetsnmp.

---

## 3. CheckMK

### 3.1 cmk-ec test suite

- **Source**: checkmk.md:660-682 — 20 files, 4,116 LOC under `packages/cmk-ec/tests/`.
- **Notable**: `test_rule_matching.py` (464 lines), `test_event_creator.py` (835 lines), `test_ec_history_sqlite.py` (413 lines), `test_ec_history_mongo.py` (142 lines).
- **Format**: pytest with `make_event` builder + `helpers.py` fixtures.
- **Coverage**: rule matching, history backends, syslog parsing, perfcounters, forwarder, host_config. **No `test_ec_snmp.py`** — SNMP-specific tests are integration only.
- **License**: GPLv2 (Apache-2.0 outside `x-pack`).

### 3.2 CheckMK integration test

- **Source**: checkmk.md:685-695 — `tests/integration/cmk/ec/test_ec.py` (505 lines).
- **Trap-specific tests**: `test_ec_rule_match_snmp_trap` (lines 354-377), `test_ec_rule_no_match_snmp_trap` (lines 380-400), `test_ec_global_settings` (lines 401-435, currently `@pytest.mark.skip(reason="CMK-33230")`).
- **Format**: shells out to Net-SNMP `snmptrap` CLI: `snmptrap -v 1 -c public 127.0.0.1 .1.3.6.1 192.168.178.30 6 17 '""' .1.3.6.1 s "<test message>"`. **v1 only** (test helper `_get_snmp_trap_cmd` at `test_ec.py:194-209`).
- **License**: GPLv2.
- **Fitness**: medium — exercises the real PDU path but v2c and v3 are not exercised by any test.

---

## 4. Centreon

### 4.1 Cypress E2E

- **Source**: centreon.md:881-887 — `tests/e2e/features/Snmp-Traps/` (4 files; 14 scenarios across 3 feature files).
- **Format**: Cypress + TypeScript form-fill tests.
- **Fixture**: `tests/e2e/fixtures/snmp-traps/snmp-trap.json` (58 lines per `wc -l`).
- **Coverage**: UI form save → DB round-trip; vendor add/duplicate/delete; **does NOT exercise the centreontrapd Perl daemon, snmptrapd, or actual UDP trap receipt**.
- **License**: Apache-2.0 / GPL-2.0 split.

### 4.2 IF-MIB fixture + Postman/Behat REST collection

- **Source**: centreon.md:900-906.
- **Files**: `centreon/tests/rest_api/behat-collections/IF-MIB.txt` (1,108+ lines, full IF-MIB) + `rest_api.postman_collection.json` (REST coverage at :28181, :28207, :37140-37167 generatetraps, :37740-37784, :37812-38278).
- **Coverage**: legacy REST CLAPI wrapper at `centreon_clapi.class.php:90-123`; `linkDown` / `linkUp` MIB definitions through the import pipeline.
- **License**: Apache-2.0 / GPL-2.0.
- **Fitness**: medium — the only real MIB-fixture used to test the configuration pipeline; does not exercise the daemon.

---

## 5. Zabbix — HA-resume fixtures

- **Source**: zabbix.md:762-783 — `ui/tests/integration/data/snmptrap/ha{1..4}.trap`.
- **Files**: 4 files (14, 70, 42, 28 lines respectively).
- **Format**: fully-formed Zabbix-format trap records (post-`zabbix_trap_receiver.pl`), not raw PDU bytes.
- **Coverage**: SHA-512 hash-resume across HA failover; 4 records `linkUp.0` across 2024-01-10 to 2024-01-11.
- **License**: AGPLv3.
- **Fitness**: medium — the records exercise the file-tailing parser and HA-resume content-hash comparison; do NOT exercise the Perl/Bash receiver bridges, `snmptrapd` PDU decoding, or regex matching.

---

## 6. LibreNMS — embedded textual fixtures

- **Source**: librenms.md:710-751 — 81 files under `tests/Feature/SnmpTraps/`.
- **Format**: per-test fixtures embedded as PHP heredocs via `SimpleTemplate::parse` for substitution.
- **Coverage**: 80 test classes (one per handler family); pipeline-level `CommonTrapTest.php` covers garbage, findByIp, fallback, authorization, BridgeNewRoot/TopologyChanged, coldStart/warmStart, EntityDatabaseChanged.
- **Sample fixture (linkDown, PortsTrapTest.php:48-57)**:
```
<UNKNOWN>
UDP: [$device->ip]:57123->[192.168.4.4]:162
DISMAN-EVENT-MIB::sysUpTimeInstance 2:15:07:12.87
SNMPv2-MIB::snmpTrapOID.0 IF-MIB::linkDown
IF-MIB::ifIndex.$port->ifIndex $port->ifIndex
IF-MIB::ifAdminStatus.$port->ifIndex down
IF-MIB::ifOperStatus.$port->ifIndex down
IF-MIB::ifDescr.$port->ifIndex GigabitEthernet0/5
IF-MIB::ifType.$port->ifIndex ethernetCsmacd
OLD-CISCO-INTERFACES-MIB::locIfReason.$port->ifIndex "down"
```
- **License**: GPL-3.0-or-later.
- **Fitness**: HIGH — 80 ready-made trap-text fixtures across BGP, OSPF, UPS, link state, vendor-specific (Cisco, Juniper, APC, Veeam, etc.). Templated to allow device-IP/ifIndex substitution. The contract between Net-SNMP `snmptrapd`'s textual emission and the parser is exercised only by these hand-written fixtures (not by an end-to-end test of `snmptrapd` itself).

---

## 7. Nagios+SNMPTT

- **Source**: nagios-snmptt.md:836-858.
- **NSTI**: **zero test files** (`nsti @ 58ca81d`).
- **NSCA**: shell-script tests in `nsca_tests/` (NSCA-transport tests, not trap-specific).
- **Nagios Core**: `nagioscore @ 8d1d276 :: t/` and `t-tap/` (TAP-style; none trap-related).
- **NSTI fixture**: synthetic data generator `nsti/trapdumperdaemon.py` (49 lines) — inserts random rows into MySQL `snmptt` table, NOT a real trap simulator.
- **SNMPTT upstream**: per docs, ships "sample trap files for testing" — not source-mirrored.
- **License**: GPL-2 across the family.
- **Fitness**: LOW — the Nagios family has the most absent CI/test coverage observed in the cohort.

---

## 8. Sensu

- **Source**: sensu.md:609-619.
- **Sensu Classic ext** (`sensu-extensions-snmp-trap`): rspec test in `spec/snmp-trap_spec.rb`; fixture `spec/mibs/` contains `SENSU-ENTERPRISE-NOTIFY-MIB.txt`, `SENSU-ENTERPRISE-ROOT-MIB.txt`, `SNMPv2-SMI.txt`, `RFC-1212-MIB.txt`, `RFC-1215-MIB.txt`, `rfc1158.mib`, `more_mibs/APACHE-MIB.txt`. Tests use FakePacket pattern.
- **snmptrapd2sensu**: no integration tests; relies on Go unit tests in `parsers/snmptrapd_test.go` (line counts not in spec).
- **License**: MIT (Sensu); per-component varies.
- **Fitness**: LOW-MEDIUM — Sensu Go side has effectively no trap-pipeline test coverage.

---

## 9. Telegraf

- **Source**: telegraf.md:12 — `plugins/inputs/snmp_trap/snmp_trap_test.go` (1646 LOC; 5 test functions).
- **Format**: Go unit tests using `gosnmp.SendTrap()` against a real socket.
- **Coverage**: v2c trap reception; OID translation success and failure (`TestOidLookupFail`); USM cases for v3 — partial.
- **Fixtures**: none on disk; all test traps constructed via gosnmp.
- **License**: MIT (Telegraf).
- **Fitness**: medium — exercises gosnmp through real UDP; no PCAP fixtures, no fuzz, no negative-path BER fixtures.

---

## 10. Logstash

- **Source**: logstash.md (logstash-input-snmptrap source NOT mirrored; built-docs at `logstash-versioned-plugins/`).
- **Files**: zero in-mirror fixtures.
- **Format**: n/a (source unavailable).
- **Coverage**: docs only; no test artefacts.
- **License**: Apache-2.0 / Elastic License (per `logstash :: LICENSE.txt`).
- **Fitness**: LOW.

---

## 11. Datadog Agent

- **Source**: datadog-agent.md:12, datadog-agent.md:394.
- **Files**: 2,494 lines of `*_test.go` + 294 lines of test-only helpers across `comp/snmptraps/` (test-to-prod ratio ~1.24:1).
- **Integration-core fixture**: `datadog_checks_dev/tests/tooling/commands/meta/snmp/data/A3COM-HUAWEI-LswTRAP-MIB` (222 lines) plus two `expected_expanded.json` / `expected_compact.json` siblings — pins the `ddev meta snmp generate-traps-db` compiler's output shape against a real Huawei MIB.
- **Format**: Go tests + JSON-fixture validation.
- **License**: Apache-2.0 / BSD-3-Clause per-file.
- **Fitness**: HIGH for compiler-output validation; medium for trap-pipeline integration.

---

## 12. Splunk SC4SNMP

- **Source**: splunk-sc4snmp.md (integration tests under `integration_tests/tests/test_trap_integration.py`).
- **Tests**: `test_trap_v1` (lines 80-107), `test_trap_v2` (lines 109-136), `test_trap_v3` (lines 253-283).
- **Format**: pysnmp-based test client; sends `CommunityData(community, mpModel=0|1)` for v1/v2c and USM users for v3.
- **License**: Apache-2.0.
- **Fitness**: HIGH — covers v1, v2c, and v3 with USM round-trip.

---

## 13. Cribl Stream

- **Source**: cribl.md (closed-source; no source mirror).
- **Files**: zero in mirror — only OpenAPI / SDK docs.
- **License**: proprietary.
- **Fitness**: NONE (no fixtures).

---

## 14. SolarWinds / Dynatrace / LogicMonitor

- **Source**: docs-only across all three; no public test artefacts.
- **Files**: none.
- **Fitness**: NONE.

---

## Summary — fixtures most useful for a new trap-receiver implementation

For validating a new receiver, this is the priority order in terms of what is available:

1. **Zenoss `trapdump.pcap`** — real wire bytes. 2 packets.
2. **LibreNMS 80 templated text fixtures** — covers the broadest vendor variety (BGP, OSPF, UPS, link state, vendor-specific).
3. **OpenNMS integration tests** — exercise the entire decode + match + dispatch via real sockets.
4. **OpenNMS udpgen** — synthetic load generator (note OID typo).
5. **Zabbix `ha{1..4}.trap`** — fully-formed records suitable for testing file-tailing parsers.
6. **Splunk SC4SNMP `test_trap_integration.py`** — pysnmp-based client covering v1+v2c+v3 with USM.
7. **Centreon IF-MIB.txt** — real MIB for testing import pipelines.

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
