# SNMP Trap Feature Matrix — 16-system comparative analysis

This matrix records observed support per system, with file:line evidence per cell. Cells use:
- `✓ <file:line>` — supported, evidence cited
- `partial <file:line>` — partial support, evidence cited
- `✗ <file:line>` — explicitly NOT supported, evidence cited
- `– no evidence` — spec is silent

Systems in order: OpenNMS, Zenoss, CheckMK, Centreon, Zabbix, LibreNMS, Nagios+SNMPTT, Sensu, Telegraf, Logstash, Datadog Agent, Splunk SC4SNMP, Cribl, SolarWinds, Dynatrace, LogicMonitor.

Lens reference: `snmp-traps-in-observability.md`.

---

## RECEPTION

### v1 / v2c / v3-USM support

| System | v1 | v2c | v3 USM |
|---|---|---|---|
| OpenNMS | ✓ opennms.md:121-122 | ✓ opennms.md:122 | ✓ opennms.md:123 |
| Zenoss | ✓ zenoss.md:189 | ✓ zenoss.md:190 | ✓ zenoss.md:191 |
| CheckMK | ✓ checkmk.md:144 | ✓ checkmk.md:145 | ✓ checkmk.md:146-150 |
| Centreon | ✓ centreon.md:187 (via snmptrapd) | ✓ centreon.md:187 | ✓ centreon.md:188 (no UI) |
| Zabbix | ✓ zabbix.md:156 (via snmptrapd) | ✓ zabbix.md:156 | ✓ zabbix.md:156 |
| LibreNMS | ✓ librenms.md:149-151 (via snmptrapd) | ✓ librenms.md:149-151 | ✓ librenms.md:157 (docker only) |
| Nagios+SNMPTT | ✓ nagios-snmptt.md:202-204 (via snmptrapd) | ✓ nagios-snmptt.md:204 | ✓ nagios-snmptt.md:204 (manual conf) |
| Sensu | partial sensu.md:208-210 (v2c only in Classic ext); ✓ sensu.md:218-219 (via snmptrapd2sensu) | ✓ sensu.md:218-219 | ✓ sensu.md:218-219 (delegated to snmptrapd) |
| Telegraf | ✓ telegraf.md:122-124 | ✓ telegraf.md:124-126 (default) | ✓ telegraf.md:127 |
| Logstash | ✓ logstash.md:145 | ✓ logstash.md:146 | ✓ logstash.md:147 |
| Datadog Agent | ✓ datadog-agent.md:207-217 | ✓ datadog-agent.md:218 | ✓ datadog-agent.md:219-222 |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md:184 | ✓ splunk-sc4snmp.md:185 | ✓ splunk-sc4snmp.md:186 |
| Cribl | ✓ cribl.md:156-157 | ✓ cribl.md:156 | ✓ cribl.md:159 |
| SolarWinds | ✓ solarwinds.md:183 (inferred) | ✓ solarwinds.md:184 | ✓ solarwinds.md:185 |
| Dynatrace | ✓ dynatrace.md:201 | ✓ dynatrace.md:202 | ✓ dynatrace.md:203 |
| LogicMonitor | ✓ logicmonitor.md:236 | ✓ logicmonitor.md:237 | ✓ logicmonitor.md:238 |

### Multiple v3 USM users per listener

| System | Multi-user |
|---|---|
| OpenNMS | ✓ opennms.md:525 (multi-USM-context via chained dispatcher) |
| Zenoss | ✓ zenoss.md:582 (per-device users) |
| CheckMK | ✓ checkmk.md:151 (multi-user; engine_ids required) |
| Centreon | partial centreon.md:188 (snmptrapd-based; no Centreon UI) |
| Zabbix | partial zabbix.md:159 (via snmptrapd_custom.conf only) |
| LibreNMS | partial librenms.md:157 (delegated to snmptrapd) |
| Nagios+SNMPTT | partial nagios-snmptt.md:204 (snmptrapd-managed) |
| Sensu | – no evidence (sensu.md:190 — Classic ext has no USM table) |
| Telegraf | ✗ telegraf.md:498 ("one stanza accepts exactly one v3 credential set") |
| Logstash | ✗ logstash.md:166-167 ("A single user can be configured") |
| Datadog Agent | ✓ datadog-agent.md:219 (users table with multiple USM users) |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md:187 (multi-user × multi-engineID) |
| Cribl | ✓ cribl.md:159 (V3Users list, minItems:1) |
| SolarWinds | ✓ solarwinds.md:186 (multi-user table per release notes) |
| Dynatrace | – no evidence (dynatrace.md:198-204) |
| LogicMonitor | ✓ logicmonitor.md:245-249 (per-resource credentials) |

### Multiple listeners (different ports/contexts)

| System | Multi-listener |
|---|---|
| OpenNMS | partial opennms.md:138-143 (per-deployment one core+Minions) |
| Zenoss | partial zenoss.md:120 (multi-Collector deployment) |
| CheckMK | ✗ checkmk.md:106 (single ec; one trap socket) |
| Centreon | partial centreon.md:112-114 (multi-poller, one per host) |
| Zabbix | partial zabbix.md:121-122 (proxy-side reception; one trapper per server) |
| LibreNMS | ✗ librenms.md (single snmptrapd per host) |
| Nagios+SNMPTT | partial nagios-snmptt.md:133-134 (per-collector) |
| Sensu | – no evidence |
| Telegraf | ✓ telegraf.md:498 (multiple `[[inputs.snmp_trap]]` stanzas on different ports) |
| Logstash | ✓ logstash.md:139 (multiple input blocks, one port each) |
| Datadog Agent | ✗ datadog-agent.md:250 (single-producer single-consumer; no multi-listener) |
| Splunk SC4SNMP | partial splunk-sc4snmp.md:174-178 (single internal port 2162; HPA scaling) |
| Cribl | ✓ cribl.md:139 (each Source binds independently) |
| SolarWinds | partial solarwinds.md:140-141 (APE per location) |
| Dynatrace | partial dynatrace.md:155-160 (ActiveGate group; all run same config) |
| LogicMonitor | ✓ logicmonitor.md:184 (failover pairs, per-Collector) |

### Privileged-port handling

| System | Approach |
|---|---|
| OpenNMS | partial opennms.md:136-141 (default 10162 package / 1162 Docker; needs port-forward for 162) |
| Zenoss | ✓ zenoss.md:204-208 (`zensocket` suid helper) |
| CheckMK | ✓ checkmk.md:115 (`mkeventd_open514` with `cap_net_bind_service+ep` default, setuid fallback) |
| Centreon | ✓ centreon.md:209 (snmptrapd runs as root or with CAP_NET_BIND_SERVICE) |
| Zabbix | partial zabbix.md:176-178 (1162 internal, host NAT 162→1162 in container) |
| LibreNMS | partial librenms.md:186 (snmptrapd runs as root assumed; SELinux module documented) |
| Nagios+SNMPTT | partial nagios-snmptt.md:232 (snmptrapd as root assumed) |
| Sensu | partial sensu.md:212 (Classic ext default 1062; operator iptables) |
| Telegraf | partial telegraf.md:147-152 (`setcap` or unprivileged port) |
| Logstash | partial logstash.md:171-179 (default 1062; operator setcap/iptables) |
| Datadog Agent | ✓ datadog-agent.md:254-256 (default 9162; setcap documented) |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md:216-220 (binds 2162 internally; k8s service 162→2162) |
| Cribl | partial cribl.md:145 (default 162 from spec; stock 9162) |
| SolarWinds | ✓ solarwinds.md:215-218 (Windows; no Unix restriction; OS SNMP Trap conflict) |
| Dynatrace | partial dynatrace.md:188-194 (setcap or iptables; 162 not vendor-default) |
| LogicMonitor | partial logicmonitor.md:262-267 (run-as-root, setcap, or iptables redirect) |

### Per-source community/IP allowlist

| System | Allowlist |
|---|---|
| OpenNMS | ✗ opennms.md:538 ("No source-IP allow-list at the trap listener") |
| Zenoss | ✗ zenoss.md:596-597 |
| CheckMK | ✗ checkmk.md:645 ("UDP 162 is open to *the world*") |
| Centreon | ✗ centreon.md (snmptrapd has authCommunity but disableAuthorization yes is shipped default) |
| Zabbix | partial zabbix.md:162-167 (`authCommunity` in snmptrapd.conf; default public + disableAuthorization yes) |
| LibreNMS | ✗ librenms.md:147 (default `disableAuthorization yes`) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:541-547 (default `authCommunity public` + disableAuthorization yes) |
| Sensu | – no evidence |
| Telegraf | ✗ telegraf.md (no per-source allowlist) |
| Logstash | – no evidence (logstash docs don't show IP allowlist) |
| Datadog Agent | partial datadog-agent.md:247-248 (constant-time community compare; no IP allowlist) |
| Splunk SC4SNMP | – no evidence (no IP allowlist documented) |
| Cribl | ✓ cribl.md:151-152, 196 (`ipWhitelistRegex`, default `.*`) |
| SolarWinds | – no evidence |
| Dynatrace | ✓ dynatrace.md:347-349 (CIDR `ip` field on monitoring config) |
| LogicMonitor | – no evidence |

### INFORM acknowledgement

| System | INFORM |
|---|---|
| OpenNMS | ✓ opennms.md:124 (Response PDU via SNMP4J; tested for v3) |
| Zenoss | ✓ zenoss.md:192 (`snmpInform()` clones inbound PDU; v3 untested) |
| CheckMK | ✗ checkmk.md:152 (explicit `# Disable receiving of SNMPv3 INFORM messages. We do not support them (yet)`) |
| Centreon | ✓ centreon.md:190 (snmptrapd handles transparently) |
| Zabbix | ✓ zabbix.md:134 (gosnmp generates Response inside TrapListener) |
| LibreNMS | partial librenms.md:151 (snmptrapd handles transparently) |
| Nagios+SNMPTT | partial nagios-snmptt.md:206 (snmptrapd handles transparently) |
| Sensu | partial sensu.md:218-219 (snmptrapd-based path) |
| Telegraf | partial telegraf.md:134 (gosnmp ACKs Inform inside TrapListener; untested) |
| Logstash | – no evidence (logstash.md:148-149 — undocumented) |
| Datadog Agent | – no evidence |
| Splunk SC4SNMP | – no evidence (splunk-sc4snmp.md:188 — undocumented) |
| Cribl | – no evidence (cribl.md:158 — schema silent) |
| SolarWinds | – no evidence (solarwinds.md:188 — undocumented) |
| Dynatrace | ✗ dynatrace.md:209-213 ("SNMP inform requests aren't supported") |
| LogicMonitor | – no evidence (logicmonitor.md:241 — undocumented) |

### DTLS / TLS-TM

| System | DTLS/TLS-TM |
|---|---|
| OpenNMS | ✗ opennms.md:125, 528-529 ("hard-wired to `DefaultUdpTransportMapping`") |
| Zenoss | ✗ zenoss.md:193 |
| CheckMK | ✗ checkmk.md:153 |
| Centreon | ✗ centreon.md:189 (would need snmptrapd `tlstm` build) |
| Zabbix | ✗ zabbix.md:157 (gosnmp does not implement TLS/DTLS) |
| LibreNMS | ✗ librenms.md:159 |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:205 |
| Sensu | ✗ sensu.md:210 |
| Telegraf | ✗ telegraf.md:132 |
| Logstash | ✗ logstash.md:150 |
| Datadog Agent | ✗ datadog-agent.md:220 |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:189 |
| Cribl | ✗ cribl.md:152 |
| SolarWinds | ✗ solarwinds.md:187 |
| Dynatrace | – no evidence |
| LogicMonitor | – no evidence |

### IPv4 + IPv6

| System | IPv4 | IPv6 |
|---|---|---|
| OpenNMS | ✓ opennms.md:114 | ✓ opennms.md:114 |
| Zenoss | ✓ zenoss.md:162 | ✗ zenoss.md:174-184 (IPv6 disabled by `return False` hack) |
| CheckMK | ✓ checkmk.md:154 | ✓ checkmk.md:154 (dual-stack; v4-mapped normalisation) |
| Centreon | ✓ centreon.md (via snmptrapd) | – no evidence |
| Zabbix | ✓ zabbix.md | – no evidence |
| LibreNMS | ✓ librenms.md:154 (IPv4+IPv6 src parsing) | ✓ librenms.md:154 |
| Nagios+SNMPTT | ✓ nagios-snmptt.md (via snmptrapd) | partial nagios-snmptt.md:1119 (SNMPTT v1.4 lacks IPv6; v1.5 adds it) |
| Sensu | ✓ sensu.md:194 | partial sensu.md:369 (parsing IPv6 broken in snmptrapd2sensu port-parse) |
| Telegraf | ✓ telegraf.md:154 | ✓ telegraf.md:154 (dual-stack with v4-mapped normalisation) |
| Logstash | ✓ logstash.md:137 | ✓ logstash.md:137 (default `0.0.0.0`; supports `::`) |
| Datadog Agent | ✓ datadog-agent.md | – no evidence |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md | partial splunk-sc4snmp.md:166-168 (IPv6 via env var) |
| Cribl | ✓ cribl.md:144 | ✓ cribl.md:144 |
| SolarWinds | ✓ solarwinds.md:178 ("UDP Port 162 must be open for both IPv4 and IPv6") | ✓ solarwinds.md:178 (but HA wrapper is IPv4-only at solarwinds.md:152) |
| Dynatrace | ✓ dynatrace.md | – no evidence |
| LogicMonitor | ✓ logicmonitor.md (default 0.0.0.0) | – no evidence |

---

## DECODE

### BER decode in-process vs delegate to snmptrapd

| System | Decoder |
|---|---|
| OpenNMS | in-process (SNMP4J) opennms.md:29 |
| Zenoss | in-process (pynetsnmp wraps net-snmp C lib) zenoss.md:30 |
| CheckMK | in-process (PySNMP) checkmk.md:26 |
| Centreon | delegate to snmptrapd centreon.md:157-159 |
| Zabbix | delegate to snmptrapd zabbix.md:35-37 |
| LibreNMS | delegate to snmptrapd librenms.md:23-28 |
| Nagios+SNMPTT | delegate to snmptrapd nagios-snmptt.md:182-198 |
| Sensu | in-process (Classic Ruby `snmp` gem) sensu.md:196 ; delegate to snmptrapd (snmptrapd2sensu) sensu.md:218 |
| Telegraf | in-process (gosnmp) telegraf.md:114 |
| Logstash | in-process (SNMP4J, v4+) logstash.md:45 |
| Datadog Agent | in-process (gosnmp) datadog-agent.md:28 |
| Splunk SC4SNMP | in-process (pysnmp) splunk-sc4snmp.md:29 |
| Cribl | in-process (closed-source) cribl.md:152 |
| SolarWinds | in-process (proprietary; not snmptrapd) solarwinds.md:56-58 |
| Dynatrace | in-process (EEC data source binds UDP) dynatrace.md:182-183 |
| LogicMonitor | in-process (Collector Java; likely SNMP4J) logicmonitor.md:25-27, 70-72 |

### v1 `agent-addr` extraction

| System | v1 agent-addr |
|---|---|
| OpenNMS | ✓ opennms.md:121 (mapped to event `<snmp>`) |
| Zenoss | ✓ zenoss.md:189, 287 (`result["device"] = agent-addr`) |
| CheckMK | ✗ checkmk.md:305 (NOT used; UDP sender wins) |
| Centreon | partial centreon.md:183 (captured into `$var[6]` enterprise; not used as device id) |
| Zabbix | ✗ zabbix.md:966 (not used; trap matched by UDP source) |
| LibreNMS | ✗ librenms.md:155-156 (not consulted for v1) |
| Nagios+SNMPTT | partial nagios-snmptt.md:377 (`$A`/`$aA` exposed in FORMAT) |
| Sensu | – no evidence (sensu.md doesn't preserve `agent-addr` as source) |
| Telegraf | ✓ telegraf.md:287 (`agent_address` tag; v1 only) |
| Logstash | ✓ logstash.md:281 (`[@metadata][input][snmptrap][pdu][agent_addr]` for v1) |
| Datadog Agent | ✗ datadog-agent.md:491 (NOT extracted; UDP source wins) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:444 (NOT used; UDP source wins) |
| Cribl | partial cribl.md:157 (likely RFC 3584 v1-to-v2c bridge; not directly documented) |
| SolarWinds | – no evidence (solarwinds.md:308 — undocumented) |
| Dynatrace | – no evidence (dynatrace.md:361 — `device.address` is UDP source; v1 agent-addr not split) |
| LogicMonitor | ✓ logicmonitor.md:438 (EA 36.100+: v1 uses agent-addr for source identification) |

### `snmpTrapAddress.0` varbind extraction (RFC 3584)

| System | snmpTrapAddress |
|---|---|
| OpenNMS | ✓ opennms.md:242, 247 (`use-address-from-varbind="true"`) |
| Zenoss | ✓ zenoss.md:190, 288 (automatic for v2/v3) |
| CheckMK | ✗ checkmk.md:305 (only UDP source used) |
| Centreon | ✓ centreon.md:174 (extracted into `$var[4]`) |
| Zabbix | ✓ zabbix.md:324 (Docker bash bridge override; Perl bridge does NOT) |
| LibreNMS | – no evidence (librenms.md doesn't document RFC 3584) |
| Nagios+SNMPTT | – no evidence (depends on snmptrapd format options) |
| Sensu | ✗ sensu.md:317 (only UDP source IP used; no opt-in) |
| Telegraf | ✗ telegraf.md:292 ("no `snmpTrapAddress` RFC 3584 handling") |
| Logstash | – no evidence |
| Datadog Agent | ✗ datadog-agent.md:491 ("no agent-addr handling") |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:444 |
| Cribl | partial cribl.md:293 (operator-added via Eval idiom for outbound only) |
| SolarWinds | – no evidence |
| Dynatrace | partial dynatrace.md:361 (not split from UDP source) |
| LogicMonitor | ✓ logicmonitor.md:439 (EA 36.100+: v2c/v3 use snmpTrapAddress varbind) |

### v3 dynamic engine-ID discovery

| System | EngineID discovery |
|---|---|
| OpenNMS | partial opennms.md:525 (relies on SNMP4J engine-id discovery; multi-USM-context dispatcher) |
| Zenoss | – no evidence |
| CheckMK | ✗ checkmk.md:151 (engine_ids must be pre-configured per credential) |
| Centreon | – no evidence (operator config in snmptrapd.conf) |
| Zabbix | – no evidence |
| LibreNMS | – no evidence |
| Nagios+SNMPTT | – no evidence |
| Sensu | – no evidence |
| Telegraf | – no evidence (gosnmp handles internally) |
| Logstash | – no evidence (logstash.md:168 — undocumented) |
| Datadog Agent | partial datadog-agent.md:228-243 (FNV-128 hashed engineID from hostname) |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md:194-205 (DISCOVER_ENGINE_ID opt-in; BER-decode header to capture) |
| Cribl | ✗ cribl.md:160 (no `localEngineId`, no `contextEngineId`) |
| SolarWinds | – no evidence |
| Dynatrace | – no evidence |
| LogicMonitor | partial logicmonitor.md:71-72 (Collector handles engine-id) |

### OID-to-symbolic-name resolution (at decode time)

| System | OID resolution |
|---|---|
| OpenNMS | partial opennms.md:233-235 (numeric kept; names in match-time event def) |
| Zenoss | ✓ zenoss.md:278-283 (OidMap cached from ZenHub at trap time) |
| CheckMK | partial checkmk.md:243-245 (off by default; `translate_snmptraps` opt-in) |
| Centreon | partial centreon.md:232 (Net-SNMP translation; may keep symbolic) |
| Zabbix | ✓ zabbix.md:248-254 (`-O STte` flags in snmptrapd; symbolic by default) |
| LibreNMS | ✓ librenms.md:23-28 (resolved BY snmptrapd before PHP sees it) |
| Nagios+SNMPTT | ✓ nagios-snmptt.md:325-326 (snmptrapd `-On` numeric; SNMPTT resolves via MIBs offline) |
| Sensu | ✓ sensu.md:295-305 (`@mibs.name()` at trap time; falls back to dotted) |
| Telegraf | ✓ telegraf.md:165-167 (gosmi or netsnmp lookup; trap DROPPED on failure) |
| Logstash | ✓ logstash.md:248-252 (`oid_mapping_format`: default/ruby_snmp/dotted_string) |
| Datadog Agent | ✓ datadog-agent.md:271-285 (oidresolver at format time) |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md:240-261 (mibserver via HTTP; lazy compile) |
| Cribl | ✗ cribl.md:207-214 (no embedded MIB compiler) |
| SolarWinds | ✓ solarwinds.md:300 (bundled MIB DB; numeric fallback for unknown) |
| Dynatrace | ✓ dynatrace.md:283-285 (bundled OID set for default ext; numeric fallback) |
| LogicMonitor | ✓ logicmonitor.md:312 (LogSource: out-of-the-box MIB translation) |

---

## MIB / PROFILE MANAGEMENT

### Bundled vendor MIB pack (count if known)

| System | Bundled MIBs |
|---|---|
| OpenNMS | partial opennms.md:161 (9 standard MIBs compiled; 17,442 event defs across 230 XML files not auto-loaded) |
| Zenoss | partial zenoss.md:241 (standard IETF MIBs only; 3 trap-specific event mappings) |
| CheckMK | partial checkmk.md:217 (Net-SNMP standard MIBs only; no third-party vendor MIBs) |
| Centreon | partial centreon.md:233-238 (zero raw .mib files; 214 seeded `traps` rows, 8 vendors) |
| Zabbix | partial zabbix.md:227-230 (none from Zabbix; `snmp-mibs-downloader` in Ubuntu image) |
| LibreNMS | ✓ librenms.md:208-211 (4,770 MIB files; 371 vendor dirs; 2,245 with notifications) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:277-281 (zero bundled MIBs across mirrored repos) |
| Sensu | ✗ sensu.md:245 (none in Classic ext production gem) |
| Telegraf | ✗ telegraf.md:227-228 (zero MIB files) |
| Logstash | partial logstash.md:205-208 (libsmi 0.5.0 IETF set; no vendor MIBs) |
| Datadog Agent | partial datadog-agent.md:387, 394 (`dd_traps_db.json.gz` covers "more than 11,000 MIBs"; closed bundle) |
| Splunk SC4SNMP | partial splunk-sc4snmp.md:265 (~500 standard MIBs; full set from `pysnmp/mibs`) |
| Cribl | ✗ cribl.md:211 (nothing ships in-product; community pack ~15,000 MIBs in 168 CSVs) |
| SolarWinds | ✓ solarwinds.md:251 ("over 250,000 precompiled unique OIDs from hundreds of standard and vendor MIBs") |
| Dynatrace | partial dynatrace.md:283-284 (fixed predefined OID set; size not enumerated) |
| LogicMonitor | partial logicmonitor.md:309-310 (out-of-the-box MIB translation; vendor list at supported-mibs page) |

### Operator MIB upload UX

| System | MIB upload UX |
|---|---|
| OpenNMS | ✓ opennms.md:200-203 (UI MIB compiler; jsmiparser) |
| Zenoss | ✓ zenoss.md:247-249 (UI upload runs `zenmib` via SubprocessJob) |
| CheckMK | ✓ checkmk.md:224 (WATO upload; PySMI compile; `.zip` collections supported) |
| Centreon | ✓ centreon.md:281-286 (web UI runs `centFillTrapDB`) |
| Zabbix | partial zabbix.md:209-211 (mount `MIBDIRS` files; restart snmptrapd) |
| LibreNMS | partial librenms.md:233-237 (drop to `/opt/librenms/mibs/<vendor>/`; restart snmptrapd) |
| Nagios+SNMPTT | partial nagios-snmptt.md:259-272 (offline `snmpttconvertmib`; flat conf file) |
| Sensu | partial sensu.md:235-237 (drop into `/etc/sensu/mibs`; smidump shell-out) |
| Telegraf | partial telegraf.md:218-224 (filesystem path; `gosmi` walks dirs) |
| Logstash | partial logstash.md:213-218 (offline `smidump`-to-.dic; `mib_paths` config) |
| Datadog Agent | partial datadog-agent.md:400-408 (separate `ddev meta snmp generate-traps-db` CLI tool) |
| Splunk SC4SNMP | partial splunk-sc4snmp.md:270-275 (file an upstream issue OR mount local-mibs PVC) |
| Cribl | ✗ cribl.md:214-222 (operator-prepared CSV; no in-product MIB compiler) |
| SolarWinds | ✗ solarwinds.md:254-273 ("If you have a specific device MIB, you can have it added to the SolarWinds MIB database" — email it to the vendor) |
| Dynatrace | partial dynatrace.md:296-309 (per-extension `snmp/` dir or ActiveGate `mib-files-custom/`; only for custom extensions) |
| LogicMonitor | ✓ logicmonitor.md:320-339 (portal upload on Collector 38.300+; or local JSON converter) |

### Hot-reload of MIBs (no restart)

| System | Hot MIB reload |
|---|---|
| OpenNMS | partial opennms.md:386 (event defs reload via REST; MIB compiler at upload) |
| Zenoss | ✓ zenoss.md:436 (OidMap refresh every 120 s) |
| CheckMK | ✓ checkmk.md:481-483 (`COMMAND RELOAD` rebuilds resolver) |
| Centreon | partial centreon.md:628-630 (SIGHUP daemon; manual Generate-then-Reload) |
| Zabbix | ✗ zabbix.md:241-245 (snmptrapd reload requires SIGHUP) |
| LibreNMS | ✗ librenms.md:237 (snmptrapd restart needed) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:614-616 (SIGHUP snmptt; polling delay) |
| Sensu | ✗ sensu.md:510 (sensu-client restart required for Classic ext) |
| Telegraf | ✗ telegraf.md:251-252 (SIGHUP triggers Init; brief window of socket-unbound) |
| Logstash | ✗ logstash.md:217 (restart or reload Logstash) |
| Datadog Agent | ✗ datadog-agent.md:410 ("No live reload"; Agent restart required) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:276 (rollout-restart pods) |
| Cribl | – no evidence (CSV lookup reload semantics not stated) |
| SolarWinds | partial solarwinds.md:464 (installer restarts services automatically) |
| Dynatrace | partial dynatrace.md:302-307 (EEC restart for added custom MIBs in running ext) |
| LogicMonitor | partial logicmonitor.md:585-587 (LogicModule push 1-15 min; agent.conf restart) |

### MIB-to-event mapping format

| System | Format |
|---|---|
| OpenNMS | XML (`<event>` definitions in DB-backed `eventconf_events`) opennms.md:308 |
| Zenoss | ZODB (`EventClass`/`EventClassInst`) + Python transforms zenoss.md:376-378 |
| CheckMK | Python rules (`config/snmptraps.php`-like in `rule_packs`) checkmk.md:375 |
| Centreon | MariaDB `traps` table (DB rows) centreon.md:444-470 |
| Zabbix | per-host items (`snmptrap[regex]`) — no central catalogue zabbix.md:439 |
| LibreNMS | PHP handler classes in code (181 OID → 177 classes) librenms.md:333 |
| Nagios+SNMPTT | flat text EVENT blocks in `snmptt.conf` nagios-snmptt.md:347-368 |
| Sensu | Ruby `RESULT_MAP`/`RESULT_STATUS_MAP` constants sensu.md:334-345 |
| Telegraf | (no mapping; passes through as fields) telegraf.md:295-298 |
| Logstash | (no mapping; passes through as fields; operator filters) logstash.md:291-300 |
| Datadog Agent | JSON / YAML (`traps_db/*.json`) datadog-agent.md:275-298 |
| Splunk SC4SNMP | (no mapping; YAML for custom traps) splunk-sc4snmp.md:519 |
| Cribl | CSV lookups + Eval/Code functions cribl.md:215-222 |
| SolarWinds | (rules engine in Trap Viewer / Log Viewer; opaque format) solarwinds.md:343 |
| Dynatrace | declarative YAML in extension package dynatrace.md:166-170 |
| LogicMonitor | Groovy EventSource scripts; LogSource pipelines logicmonitor.md:347-349 |

### Per-OID severity defaults shipped

| System | Default severity |
|---|---|
| OpenNMS | ✓ opennms.md:215, 692 (bundled definitions; default `Indeterminate` for unmatched) |
| Zenoss | partial zenoss.md:265 (default `SEVERITY_WARNING`; per EventClass zEventSeverity) |
| CheckMK | ✗ checkmk.md:475 (no rules ship; severity is per-rule operator-defined) |
| Centreon | partial centreon.md:736-741 (`getStatus()` heuristic from MIB severity comment) |
| Zabbix | ✗ zabbix.md:625 (no trap-level severity; trigger-author defines) |
| LibreNMS | partial librenms.md:626-630 (per-handler hard-coded; e.g. `Error` for BgpBackward) |
| Nagios+SNMPTT | partial nagios-snmptt.md:700-715 (5-level SNMPTT severity per EVENT block) |
| Sensu | partial sensu.md:333-345 (RESULT_STATUS_MAP regex defaults) |
| Telegraf | ✗ telegraf.md:558-561 (no severity model) |
| Logstash | ✗ logstash.md:303-306 (no normalization done by plugin) |
| Datadog Agent | ✗ datadog-agent.md:530-533 (no severity field) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:454-456 (no severity normalization) |
| Cribl | ✗ cribl.md:468-471 (no built-in severity model) |
| SolarWinds | partial solarwinds.md:313-314 (`ObservationSeverity` from matched rule; defaults not published) |
| Dynatrace | ✗ dynatrace.md:377-381 (`loglevel: NONE` hard-coded) |
| LogicMonitor | partial logicmonitor.md:455-456 (per-EventSource fixed at definition) |

---

## ENRICHMENT

### Device identity lookup (sysName/IP/sysObjectID family)

| System | Identity lookup |
|---|---|
| OpenNMS | ✓ opennms.md:245 (`InterfaceToNodeCache.getFirstNodeId`) |
| Zenoss | ✓ zenoss.md (device by IP/hostname) |
| CheckMK | ✓ checkmk.md:311-318 (`HostConfig` by name/address/alias) |
| Centreon | ✓ centreon.md:375 (host_address join) |
| Zabbix | ✓ zabbix.md:307-322 (string-exact IP/DNS match against config cache) |
| LibreNMS | ✓ librenms.md:339-345 (hostname/IP/ipv4_addresses/ipv6_addresses) |
| Nagios+SNMPTT | partial nagios-snmptt.md:376-378 (snmptrapd's `$A`/`$aA`/`$R`; mapping by EXEC operator) |
| Sensu | partial sensu.md:313-318 (reverse DNS via Resolv.getname) |
| Telegraf | ✗ telegraf.md:286-292 (`source` tag = raw IP; no enrichment) |
| Logstash | ✗ logstash.md:283-287 (no inventory lookup; filter-based) |
| Datadog Agent | partial datadog-agent.md:476-483 (`snmp_device` tag; no inventory join on Agent) |
| Splunk SC4SNMP | partial splunk-sc4snmp.md:443-444 (raw IP; no inventory join) |
| Cribl | ✗ cribl.md:283-287 (no device-inventory in Cribl Stream) |
| SolarWinds | ✓ solarwinds.md:304 (NodeID FK to `Orion.Nodes`) |
| Dynatrace | ✓ dynatrace.md:362-365 (`dt.source_entity` from SmartScape topology) |
| LogicMonitor | ✓ logicmonitor.md:436-441 (device match against Collector's resource table) |

### Topology annotation (interface/neighbours)

| System | Topology annotation |
|---|---|
| OpenNMS | partial opennms.md:417-420 (separate topology DB; NOT used at trap time) |
| Zenoss | partial zenoss.md:465-467 (separate Network Map; NOT consulted in trap path) |
| CheckMK | ✗ checkmk.md:517 (topology not mapped onto trap path) |
| Centreon | ✗ centreon.md:679-680 (no integrated topology in OSS) |
| Zabbix | ✗ zabbix.md:599-600 (no native L2/L3 topology) |
| LibreNMS | partial librenms.md:535-538 (LLDP/CDP discovery; not used at trap time) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:655-662 |
| Sensu | ✗ sensu.md (no topology) |
| Telegraf | ✗ telegraf.md:540-541 |
| Logstash | – no evidence |
| Datadog Agent | partial datadog-agent.md:21 (SaaS-side NDM device link only) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md (no topology in Connector) |
| Cribl | ✗ cribl.md:448-450 |
| SolarWinds | partial solarwinds.md:497-505 (Orion Maps; not topology-aware suppression at trap) |
| Dynatrace | partial dynatrace.md:546-558 (SmartScape entities; not topology-aware suppression) |
| LogicMonitor | partial logicmonitor.md:39-40 (TopologySources LLDP/CDP/BGP/OSPF; no trap suppression) |

### Polling-state cross-reference

| System | Polling cross-ref |
|---|---|
| OpenNMS | partial opennms.md:402-405 (trap-payload-to-metric IS built-in but opt-in) |
| Zenoss | ✓ zenoss.md:511-512 (`UpsTrapOnBattery` writes to sensors table) |
| CheckMK | ✗ checkmk.md:503 (no trap-to-metric pipeline) |
| Centreon | partial centreon.md:649-650 (no trap-to-metric; via active checks) |
| Zabbix | partial zabbix.md:558-565 (preprocessing can extract numeric; not built-in) |
| LibreNMS | partial librenms.md:509-513 (`UpsTrapOnBattery` updates `sensors`; narrow) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:632 |
| Sensu | ✗ sensu.md (no polling cross-ref) |
| Telegraf | ✗ telegraf.md (no enrichment) |
| Logstash | – no evidence |
| Datadog Agent | partial datadog-agent.md:21 (NDM device cross-link; SaaS-side) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:447-451 ("Trap events do NOT carry inventory metadata") |
| Cribl | ✗ cribl.md (no built-in) |
| SolarWinds | partial solarwinds.md:480 (PerfStack cross-stack correlation) |
| Dynatrace | partial dynatrace.md:512-520 (`traps.count` metric + log-metric extraction) |
| LogicMonitor | partial logicmonitor.md:601-602 (manual DataSource counter pattern) |

### NetFlow / flow cross-reference

| System | NetFlow cross-ref |
|---|---|
| (all systems) | – no evidence in any system spec for native trap↔NetFlow correlation in the trap pipeline. |

### Cardinality discipline

| System | Cardinality posture |
|---|---|
| OpenNMS | high-cardinality varbinds in `event_parameters` (separate table), not labels — opennms.md:312, 424 |
| Zenoss | varbinds in `details_json` blob; not labels — zenoss.md:372 |
| CheckMK | varbinds flattened into single `text` field — checkmk.md:303 |
| Centreon | varbinds in `traps_args` template (output line; not labels) — centreon.md:453 |
| Zabbix | each varbind = field on item; LOG/TEXT history table — zabbix.md:343-358 |
| LibreNMS | varbinds in eventlog `message` text; not labels — librenms.md:426 |
| Nagios+SNMPTT | varbinds flattened into `formatline` varchar(255) — nagios-snmptt.md:495 |
| Sensu | varbinds as `Check.Output` JSON + `snmp_<oid>` annotations — sensu.md:113-115 |
| Telegraf | per-varbind FIELD (not TAG); explicit cardinality discipline — telegraf.md:355 |
| Logstash | varbinds as event fields; `target` namespace recommended — logstash.md:275 |
| Datadog Agent | dual-encoding: `variables[]` array + top-level named keys — datadog-agent.md:521-527 |
| Splunk SC4SNMP | varbinds in JSON event body inside `event` field — splunk-sc4snmp.md:379-385 |
| Cribl | varbinds map (flat) or typed array (`varbindsWithTypes`) — cribl.md:164 |
| SolarWinds | one row per varbind in `Orion.TrapVarbinds` — solarwinds.md:413 |
| Dynatrace | one log attribute per varbind (key from OID) — dynatrace.md:441-443 |
| LogicMonitor | LogSource: varbind list as structured JSON fields — logicmonitor.md:57 |

---

## CLASSIFICATION

### Severity assignment model

| System | Severity model |
|---|---|
| OpenNMS | per-event-definition; 7-level OnmsSeverity (Indeterminate/Cleared/Normal/Warning/Minor/Major/Critical) opennms.md:492 |
| Zenoss | per-EventClass `zEventSeverity` + per-mapping transforms; 7-level (Clear/Debug/Info/Notice/Warning/Error/Critical/Original) zenoss.md:528-536 |
| CheckMK | rule outcome → 4-level (OK/WARN/CRIT/UNKNOWN); 3 derivation modes checkmk.md:541-545 |
| Centreon | per-trap `traps_status` (0/1/2/3 Nagios) + per-match override centreon.md:444-748 |
| Zabbix | trigger `priority` 0-5; not trap-level zabbix.md:625-635 |
| LibreNMS | per-handler `Severity` enum (0-5: Unknown/Ok/Info/Notice/Warning/Error); inline librenms.md:364 |
| Nagios+SNMPTT | 5-level SNMPTT severity (Normal/Warning/Minor/Major/Critical) collapsing to Nagios 3-level nagios-snmptt.md:700-711 |
| Sensu | RESULT_STATUS_MAP regex match; Sensu 4-level (0/1/2/3) sensu.md:340 |
| Telegraf | none — telegraf.md:558 |
| Logstash | none — logstash.md:303 |
| Datadog Agent | none on Agent; SaaS-side Logs Monitor — datadog-agent.md:531 |
| Splunk SC4SNMP | none — splunk-sc4snmp.md:454 |
| Cribl | none built-in — cribl.md:470 |
| SolarWinds | per-rule (matched rule sets `ObservationSeverityName`); defaults not published solarwinds.md:313-314 |
| Dynatrace | hard-coded `loglevel: NONE` — dynatrace.md:377-381 |
| LogicMonitor | EventSource: 5-level (Critical/Error/Warning/Notice/Info) per definition logicmonitor.md:392-455 |

### Clear-pair semantics (linkDown↔linkUp)

| System | Clear pair |
|---|---|
| OpenNMS | ✓ opennms.md:278 (`clear-key` on resolution event) |
| Zenoss | ✓ zenoss.md:461 (`zEventClearClasses` + `clear_fingerprint_hash`) |
| CheckMK | partial checkmk.md:865 (cancelling rules via `match_ok`) |
| Centreon | ✗ centreon.md:673 ("No automatic 'clear' pair mechanic") |
| Zabbix | partial zabbix.md:587-590 (RECOVERY_EXPRESSION operator-authored) |
| LibreNMS | partial librenms.md:528-529 (alert rule auto-clears; not paired traps directly) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:646-651 ("Nothing brings it back to OK"; freshness or paired EVENTs) |
| Sensu | partial sensu.md:323-326 (`link_status_<n>` shared check name) |
| Telegraf | ✗ telegraf.md:313-315 (no dedup; no clear semantics) |
| Logstash | ✗ logstash.md:309-316 |
| Datadog Agent | ✗ datadog-agent.md:537 (SaaS-side rules) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:458-459 |
| Cribl | ✗ cribl.md:313-316 (no alarm lifecycle) |
| SolarWinds | partial solarwinds.md:494 (`Acknowledged` flag manual; no auto-clear) |
| Dynatrace | ✗ dynatrace.md:531-532 ("no native trap-as-alert-clear semantics") |
| LogicMonitor | ✓ logicmonitor.md:477 ("Automatically close alerts when a related 'clear' trap comes in" — LogSource only) |

### Fingerprint dedup (same trap within window)

| System | Trap dedup |
|---|---|
| OpenNMS | partial opennms.md:265 (alarm-level via `reductionKey`; not trap-level) |
| Zenoss | ✓ zenoss.md:329-330 (FingerprintPipe SHA-1 fingerprint_hash on event_summary) |
| CheckMK | partial checkmk.md:567-573 (counting/expect; not raw trap dedup) |
| Centreon | ✓ centreon.md:357 (MD5 digest; `duplicate_trap_window` 1s) |
| Zabbix | ✗ zabbix.md:669 (no dedup) |
| LibreNMS | ✗ librenms.md:380-386 (no dedup at trap layer) |
| Nagios+SNMPTT | partial nagios-snmptt.md:406 (`duplicate_trap_window`; details limited) |
| Sensu | partial sensu.md:349-351 (Sensu Go `event_summary` per-check collapsing) |
| Telegraf | ✗ telegraf.md:313 |
| Logstash | ✗ logstash.md:309 |
| Datadog Agent | ✗ datadog-agent.md:537 |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:458 |
| Cribl | partial cribl.md:249 (Suppress function; in-memory state) |
| SolarWinds | ✗ solarwinds.md:333 (every PDU is a row) |
| Dynatrace | ✗ dynatrace.md:388-395 (no in-product dedup; counter-metric workaround only) |
| LogicMonitor | ✗ logicmonitor.md:466 (EventSource: "Duplicate alerts ... never suppressed"); ✓ logicmonitor.md:477 (LogSource clear-trap path) |

### Storm detection / rate-limit per source

| System | Storm protection |
|---|---|
| OpenNMS | partial opennms.md:512 (block-when-full back-pressure; no per-source rate limit) |
| Zenoss | ✗ zenoss.md:557 ("No first-class storm-handling") |
| CheckMK | partial checkmk.md:584-591 (`event_limit` post-processing only) |
| Centreon | ✗ centreon.md:771 ("Per-source rate limits — NOT IMPLEMENTED") |
| Zabbix | ✗ zabbix.md:957 ("no per-source rate limit, no storm detection") |
| LibreNMS | ✗ librenms.md:643 ("None in LibreNMS") |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:730-735 |
| Sensu | ✗ sensu.md (no rate limit) |
| Telegraf | ✗ telegraf.md:585 ("no native trap-storm handling") |
| Logstash | ✗ logstash.md (no documented per-source) |
| Datadog Agent | ✗ datadog-agent.md:541 (SaaS-side cost only) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md (no storm protection at trap) |
| Cribl | partial cribl.md:497 (kernel SO_RCVBUF; PQ; Drop function) |
| SolarWinds | partial solarwinds.md:489 (alert thresholds; not storm detection) |
| Dynatrace | ✗ dynatrace.md:388-395 (silent kernel drop only) |
| LogicMonitor | ✗ logicmonitor.md (no per-source rate limit; ABCG excludes traps) |

### Per-OID routing rules

| System | Per-OID rules |
|---|---|
| OpenNMS | ✓ opennms.md:792-815 (eventconf XML mask + varbind matchers) |
| Zenoss | ✓ zenoss.md:317-328 (TrapFilter rules; v1/v2 globbed OIDs) |
| CheckMK | ✓ checkmk.md:436 (Rule packs with match_application etc.) |
| Centreon | ✓ centreon.md:476-486 (`traps_matching_properties`) |
| Zabbix | ✓ zabbix.md:498-512 (per-host `snmptrap[regex]` items) |
| LibreNMS | ✓ librenms.md:332-333 (OID → handler registry in `config/snmptraps.php`) |
| Nagios+SNMPTT | ✓ nagios-snmptt.md:347-368 (per-EVENT NODES/MATCH/REGEX) |
| Sensu | partial sensu.md:334 (RESULT_MAP regex matching) |
| Telegraf | ✗ telegraf.md (no routing) |
| Logstash | ✓ logstash.md:319-324 (operator-authored Routes/filters) |
| Datadog Agent | ✗ datadog-agent.md (no agent-side routing; SaaS-side) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:462-463 (no routing) |
| Cribl | ✓ cribl.md:244 (Routes table; first-match-wins) |
| SolarWinds | ✓ solarwinds.md:486 (Trap Viewer/Log Viewer rules) |
| Dynatrace | ✓ dynatrace.md:316 (operator-built log event rules) |
| LogicMonitor | ✓ logicmonitor.md:332-334 (EventSource filters; LogSource pipelines) |

---

## ALERTING

### Real-time evaluation (sub-second)

| System | Real-time |
|---|---|
| OpenNMS | ✓ opennms.md:264 (alarm engine on event bus; cosmicClear in Drools) |
| Zenoss | ✓ zenoss.md:34 (zeneventd pipeline → ZEP) |
| CheckMK | ✓ checkmk.md:158 (in-process synchronous) |
| Centreon | partial centreon.md:200 (median ~1s latency via 2s polling) |
| Zabbix | ✓ zabbix.md:572-583 (trigger evaluates on item value arrival) |
| LibreNMS | partial librenms.md:521-525 (alert state synchronous; transport via cron) |
| Nagios+SNMPTT | partial nagios-snmptt.md:640 (state-stable dedup; no real-time pulse) |
| Sensu | ✓ sensu.md (event arrives at backend immediately) |
| Telegraf | ✗ telegraf.md (no alerting) |
| Logstash | ✗ logstash.md:38-39 (no built-in alerting) |
| Datadog Agent | ✗ datadog-agent.md:21 (alerting on SaaS side) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md (no alerting) |
| Cribl | ✗ cribl.md:441-442 |
| SolarWinds | ✓ solarwinds.md:487 (pub/sub Event alert condition) |
| Dynatrace | partial dynatrace.md:526-528 (Davis problems via log event rules) |
| LogicMonitor | partial logicmonitor.md:498 (SaaS Alert Rule engine; round-trip) |

### Batched / polled / cron evaluation

| System | Batched/Cron |
|---|---|
| OpenNMS | No (real-time) |
| Zenoss | No (real-time) |
| CheckMK | No (real-time) |
| Centreon | partial (2s poll cycle) |
| Zabbix | No (real-time) |
| LibreNMS | partial (alert transport via 1-min cron `alerts.php` librenms.md:525) |
| Nagios+SNMPTT | partial (notification interval) |
| Sensu | No (real-time) |
| Telegraf | n/a |
| Logstash | n/a |
| Datadog Agent | n/a |
| Splunk SC4SNMP | n/a |
| Cribl | n/a |
| SolarWinds | – no evidence |
| Dynatrace | partial (log event rules at ingest) |
| LogicMonitor | n/a |

### Alert model paradigm

| System | Paradigm |
|---|---|
| OpenNMS | trap → event-store → alarm engine (Drools) |
| Zenoss | trap → event pipeline → ZEP alarm |
| CheckMK | trap → ec event → rule-driven state |
| Centreon | trap → passive check submission → Nagios state |
| Zabbix | trap → item value → trigger evaluation |
| LibreNMS | trap → handler → eventlog row + alert-rule re-eval |
| Nagios+SNMPTT | trap → passive check (`PROCESS_SERVICE_CHECK_RESULT`) |
| Sensu | trap → Sensu event (check-result) |
| Telegraf | trap → metric (external pipeline alerts) |
| Logstash | trap → log document (external Watcher/Kibana alerts) |
| Datadog Agent | trap → log event (SaaS Log Monitor) |
| Splunk SC4SNMP | trap → HEC event (Splunk-side alerting) |
| Cribl | trap → event → operator-chosen Destination |
| SolarWinds | trap → log entry (Log Viewer) → pub/sub Event alert |
| Dynatrace | trap → log event in Grail → log event rule → Davis problem |
| LogicMonitor | trap → log/event → SaaS Alert Rule + Escalation Chain |

### Default OOB alert templates (count)

| System | OOB alerts |
|---|---|
| OpenNMS | partial opennms.md:215 (bundled XML defs include severity; not alert templates per se) |
| Zenoss | partial zenoss.md:241 (3 trap-specific EventClassInst rows) |
| CheckMK | ✗ checkmk.md:760 ("No example rules; no vendor packs") |
| Centreon | partial centreon.md:269-274 (214 seeded `traps` rows; no service relations seeded) |
| Zabbix | partial zabbix.md:536, 844 (`zabbix[process,snmp trapper,*]` health template; 0 per-OID `snmptrap[regex]` in built-in) |
| LibreNMS | partial librenms.md:489 (2 of 238 bundled alert rules filter `eventlog.type='trap'` — Zebra) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:888-892 (zero) |
| Sensu | ✗ sensu.md:245 (none) |
| Telegraf | ✗ telegraf.md (no alerting) |
| Logstash | ✗ logstash.md (no alerts) |
| Datadog Agent | partial (NDM preset monitors via integrations-core; counts not published) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md (Splunk-side; none shipped) |
| Cribl | ✗ cribl.md:441 |
| SolarWinds | – no evidence |
| Dynatrace | ✗ dynatrace.md:524-528 (operator-built log event rules) |
| LogicMonitor | partial logicmonitor.md:359-368 (LogicModule exchange of EventSources; count not published) |

### Operator alert authoring UX

| System | Alert UX |
|---|---|
| OpenNMS | UI/REST/XML eventconf; Drools rules |
| Zenoss | EventClass+transform Python in ZMI/UI |
| CheckMK | WATO rule packs |
| Centreon | Web UI Trap Definitions + Service Templates |
| Zabbix | Trigger expressions per host/template |
| LibreNMS | Alert rule query builder (jQuery QueryBuilder) |
| Nagios+SNMPTT | hand-edit `snmptt.conf` + `services.cfg` |
| Sensu | Sensu checks/handlers config |
| Telegraf | external backend |
| Logstash | filter pipelines + downstream alerting |
| Datadog Agent | SaaS Logs Monitor |
| Splunk SC4SNMP | Splunk SPL searches/alerts |
| Cribl | Cribl Routes/Pipelines + downstream destination |
| SolarWinds | Web Console Alerts UI |
| Dynatrace | log event rules in Grail UI |
| LogicMonitor | Portal Alert Rule + Escalation Chain UI |

### Alert deduplication

| System | Alert dedup |
|---|---|
| OpenNMS | ✓ opennms.md:265 (`reductionKey` per alarm) |
| Zenoss | ✓ zenoss.md:329 (event_summary fingerprint_hash) |
| CheckMK | partial checkmk.md:568 (counting only) |
| Centreon | ✓ centreon.md:357 (MD5 window 1s) |
| Zabbix | partial zabbix.md:643 (per-trigger state-stable) |
| LibreNMS | ✓ librenms.md:528-529 (alert rule `(device_id, rule_id)` unique) |
| Nagios+SNMPTT | partial nagios-snmptt.md:644 (state-stable; not trap-level) |
| Sensu | partial sensu.md:351 (per-check upsert) |
| Telegraf | ✗ telegraf.md |
| Logstash | ✗ logstash.md:309 |
| Datadog Agent | ✗ datadog-agent.md (Agent side) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md |
| Cribl | partial cribl.md:249 (Suppress) |
| SolarWinds | partial solarwinds.md:329 (rule-level threshold) |
| Dynatrace | partial dynatrace.md (operator-built log event rules) |
| LogicMonitor | partial logicmonitor.md:477 (LogSource clear-pair) |

### Alert recovery / clear flow

| System | Recovery |
|---|---|
| OpenNMS | ✓ opennms.md:921-944 (cosmicClear Drools rule) |
| Zenoss | ✓ zenoss.md:461 (clear_fingerprint_hash) |
| CheckMK | partial checkmk.md:511 (operator ack or livetime) |
| Centreon | ✗ centreon.md:673 (no auto pair) |
| Zabbix | partial zabbix.md (operator authored) |
| LibreNMS | partial librenms.md:528 (alert state clears via SQL query) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:646 |
| Sensu | partial sensu.md:323-326 |
| Telegraf | ✗ |
| Logstash | ✗ |
| Datadog Agent | partial (SaaS-side) |
| Splunk SC4SNMP | ✗ |
| Cribl | ✗ |
| SolarWinds | partial solarwinds.md:494 (`Acknowledged` manual) |
| Dynatrace | ✗ dynatrace.md:531 (no clear semantics) |
| LogicMonitor | ✓ logicmonitor.md:477 (LogSource auto-clear) |

---

## STORAGE

### Trap-as-X primitive

| System | Primitive |
|---|---|
| OpenNMS | event row (in `events` table) + optional alarm |
| Zenoss | event row (event_summary / event_archive) |
| CheckMK | event row (`Event` TypedDict, total=False) |
| Centreon | passive service check submission (no native trap row by default) |
| Zabbix | item value (history_log / history_text) |
| LibreNMS | eventlog row |
| Nagios+SNMPTT | MySQL `snmptt` row + passive check |
| Sensu | Sensu event (check_result) |
| Telegraf | metric (`snmp_trap` measurement) |
| Logstash | log document |
| Datadog Agent | log event (event-platform; `ndmtraps` intake) |
| Splunk SC4SNMP | HEC event (sourcetype `sc4snmp:traps`) |
| Cribl | Cribl event (JSON; forward-only via PQ) |
| SolarWinds | row in `Orion.Traps` / `Orion.TrapVarbinds` (or Log Analyzer DB) |
| Dynatrace | log event in Grail + counter metric |
| LogicMonitor | log entry (LogSource) or device event (EventSource) |

### Storage backend

| System | Backend |
|---|---|
| OpenNMS | PostgreSQL (`events`, `alarms`); optional RRD/Newts for opt-in metrics |
| Zenoss | MariaDB/MySQL (`event_summary`); Lucene (default) or Solr index; Redis queues |
| CheckMK | file / SQLite WAL / MongoDB (history backends) |
| Centreon | MariaDB (`log_traps` opt-in) |
| Zabbix | history_log / history_text in MySQL/MariaDB/PostgreSQL/Oracle/SQLite (proxy) |
| LibreNMS | MariaDB/MySQL `eventlog` |
| Nagios+SNMPTT | MySQL `snmptt` (MyISAM, latin1) + Nagios status.dat |
| Sensu | etcd default (Sensu Go) or PostgreSQL events table |
| Telegraf | (none — pass-through to chosen output) |
| Logstash | (none in Logstash; downstream Elasticsearch / Kafka / etc.) |
| Datadog Agent | (none — forwarded to SaaS intake) |
| Splunk SC4SNMP | (none — Splunk HEC) |
| Cribl | (none — Cribl is forwarder; PQ disk only) |
| SolarWinds | SQL Server (Orion DB) or Log Analyzer DB |
| Dynatrace | (none on-prem — Grail in SaaS) |
| LogicMonitor | (none on-prem — SaaS LM Logs / event store) |

### Retention configurability

| System | Retention |
|---|---|
| OpenNMS | ✓ opennms.md:332 (vacuumd; default 6 weeks events) |
| Zenoss | ✓ zenoss.md:390 (event_archive 3 days→archive; 90 days purge) |
| CheckMK | ✓ checkmk.md:743 (`history_lifetime` 365 days default) |
| Centreon | ✗ centreon.md:550 (operator-managed; no built-in) |
| Zabbix | ✓ zabbix.md:402 (per-item `history` default 31d) |
| LibreNMS | ✓ librenms.md:451-454 (`eventlog_purge` default 30 days) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:1097 (operator scripts) |
| Sensu | partial (tenant/storage-specific) |
| Telegraf | n/a (downstream) |
| Logstash | n/a (downstream) |
| Datadog Agent | n/a (SaaS-side retention) |
| Splunk SC4SNMP | n/a (Splunk-side) |
| Cribl | ✗ cribl.md:333-335 (no event index; PQ only) |
| SolarWinds | partial solarwinds.md:418 (operator-managed; 30d common, no published defaults) |
| Dynatrace | ✓ dynatrace.md (Grail Events SKU; metered retention) |
| LogicMonitor | partial logicmonitor.md:523 (tenant contract; defaults inferred) |

### Per-row cost example

| System | Cost note |
|---|---|
| SolarWinds | DELETE on `TrapVarbinds` "on the order of 90 minutes" on large deployments — solarwinds.md:419 |
| OpenNMS | trapd has block-when-full back-pressure; per-row insert into `events` plus N varbinds in `event_parameters` |
| Centreon | `log_traps` truncation/silent rollback at 2048-byte boundary — centreon.md:536 |
| Nagios+SNMPTT | MyISAM no-index full-table scan on filters — nagios-snmptt.md:1098 |

---

## OPERATIONAL FOOTPRINT

### Process model

| System | Process |
|---|---|
| OpenNMS | single JVM (Trapd within core); optional Minions |
| Zenoss | multi-daemon (zentrap, zeneventd, zeneventserver, zenhub) + MariaDB + RabbitMQ + Redis |
| CheckMK | single `mkeventd` Python process + `mkeventd_open514` C++ helper |
| Centreon | snmptrapd + centreontrapdforward (per-trap) + centreontrapd Perl daemon |
| Zabbix | single SNMP trapper thread per server/proxy + external snmptrapd |
| LibreNMS | snmptrapd + per-trap PHP CLI invocation (no daemon) |
| Nagios+SNMPTT | snmptrapd + SNMPTT daemon + Nagios Core + NSCA/NRDP + NSTI |
| Sensu | sensu-client (Classic) OR per-trap Go binary (snmptrapd2sensu) |
| Telegraf | single Telegraf Go agent (one plugin among many) |
| Logstash | single Logstash JVM (pipelines) |
| Datadog Agent | single Go binary (`comp/snmptraps/` subsystem) |
| Splunk SC4SNMP | Kubernetes pods: trap + worker-trap + worker-sender + mongo + redis + mibserver |
| Cribl | Cribl Worker Node (Worker Processes per CPU core) + Leader Node |
| SolarWinds | Windows `SolarWindsTrapService.exe` + SQL Server + IIS Web Console |
| Dynatrace | ActiveGate + EEC + SNMP traps data source binary |
| LogicMonitor | Collector Java daemon (Linux/Windows) |

### Required dependencies

| System | Deps |
|---|---|
| OpenNMS | Java 21, PostgreSQL, SNMP4J, Drools, Karaf |
| Zenoss | Python 2.7, ZODB, MariaDB, Lucene 4.7.2, Redis, RabbitMQ, Java 21 (ZEP) |
| CheckMK | Python 3.13, PySNMP, PySMI, OMD |
| Centreon | Net-SNMP snmptrapd, Perl 5, MariaDB, Gorgone |
| Zabbix | external Net-SNMP snmptrapd, Perl bridge or shell handler |
| LibreNMS | external snmptrapd, PHP 8.2-8.4, Laravel 12, MariaDB |
| Nagios+SNMPTT | snmptrapd, Perl SNMPTT, Nagios Core, NSCA/NRDP, MySQL |
| Sensu | (Classic) Ruby + RabbitMQ + Redis; (Go) etcd or PostgreSQL |
| Telegraf | Go binary; gosnmp; gosmi or net-snmp snmptranslate |
| Logstash | JVM, SNMP4J (v4+), libsmi for MIB conversion |
| Datadog Agent | Go binary; gosnmp; PySMI in integrations-core CLI |
| Splunk SC4SNMP | Kubernetes (MicroK8s) or Docker Compose; pysnmp; Celery; Redis; MongoDB; mibserver |
| Cribl | Cribl Worker (closed-source binary); Leader Node |
| SolarWinds | Windows OS, SQL Server, IIS |
| Dynatrace | ActiveGate (host-only; not containerized) |
| LogicMonitor | Java JRE bundled in Collector; Python 3.8-3.12 for MIB converter |

### Distributed vs central

| System | Distribution |
|---|---|
| OpenNMS | central core + Minions (Kafka/gRPC/JMS sinks) — opennms.md:85 |
| Zenoss | central + multiple Collectors per site |
| CheckMK | per-site EC; central not supported for trap aggregation |
| Centreon | central + multiple pollers + SQLite cache |
| Zabbix | per-server or per-proxy reception (no central aggregator) |
| LibreNMS | single host (no distributed reception) |
| Nagios+SNMPTT | distributed via NSCA/NRDP forwarder |
| Sensu | per-Collector/per-agent (no distributed reception) |
| Telegraf | per-host (no clustering) |
| Logstash | per-pipeline (no native clustering); CPM/Git mode for config |
| Datadog Agent | per-host (Agent DaemonSet) |
| Splunk SC4SNMP | Kubernetes scale-out (trap pods, worker pods) |
| Cribl | distributed Worker Nodes; UDP HA operator-supplied |
| SolarWinds | central + Additional Polling Engines (APE) |
| Dynatrace | distributed across ActiveGate groups |
| LogicMonitor | distributed Collectors per site |

### Stateful or stateless

| System | State |
|---|---|
| OpenNMS | stateful (DB + cache + Drools state) |
| Zenoss | stateful (event_summary + ZODB + Redis) |
| CheckMK | stateful (in-RAM event store; flushed to var/mkeventd/status) |
| Centreon | stateful (DB-driven catalogue + in-memory throttle hash) |
| Zabbix | stateful (item history; trapper file offset in globalvars) |
| LibreNMS | stateless per-trap PHP processes |
| Nagios+SNMPTT | stateful (MySQL `snmptt` table + Nagios status) |
| Sensu | stateless (snmptrapd2sensu); stateful Classic ext (in-thread queue) |
| Telegraf | stateless plugin |
| Logstash | optional PQ; otherwise stateless |
| Datadog Agent | stateless forwarder (bounded channel) |
| Splunk SC4SNMP | stateful (engine_id_records in Mongo) |
| Cribl | stateless forwarder (PQ for backpressure) |
| SolarWinds | stateful (SQL Server) |
| Dynatrace | stateless data source on ActiveGate (Grail in SaaS) |
| LogicMonitor | stateless Collector forward (SaaS owns state) |

---

## STORM / BACKPRESSURE

### Bounded queue

| System | Bounded queue |
|---|---|
| OpenNMS | ✓ opennms.md:130 (`queue-size=10000`) |
| Zenoss | partial zenoss.md (PBDaemon `maxqueuelen=5000`) |
| CheckMK | ✗ checkmk.md:163 (no in-process queue beyond kernel UDP buffer) |
| Centreon | partial (spool dir; not bounded) |
| Zabbix | ✓ zabbix.md:660 (64KB read buffer) |
| LibreNMS | ✗ librenms.md (no in-PHP queue) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:750 (spool dir bounded by filesystem) |
| Sensu | ✓ sensu.md:211 (Classic ext Queue unbounded) |
| Telegraf | ✓ telegraf.md (accumulator buffer `metric_buffer_limit` default 10000) |
| Logstash | ✓ logstash.md:191 (`maxBufferSize` per-Source; PQ optional) |
| Datadog Agent | ✓ datadog-agent.md:248 (packets channel buffer 100) |
| Splunk SC4SNMP | partial splunk-sc4snmp.md (Redis broker) |
| Cribl | ✓ cribl.md:147 (`maxBufferSize` default 1000; PQ optional) |
| SolarWinds | – no evidence |
| Dynatrace | – no evidence (kernel SO_RCVBUF only) |
| LogicMonitor | ✓ logicmonitor.md:271 (threadpool=10; queue model not documented) |

### Per-source rate-limit

| System | Per-source rate-limit |
|---|---|
| (All systems) | ✗ no system in the cohort ships native per-source rate-limit at the trap listener. All rely on kernel UDP buffer + external firewall rules. |

### Explicit drop telemetry

| System | Drop telemetry |
|---|---|
| OpenNMS | ✓ opennms.md:271 (`TrapsDiscarded` JMX; via `discardtraps`) |
| Zenoss | partial zenoss.md (eventFilterDroppedCount via /App/Zenoss event hourly) |
| CheckMK | partial checkmk.md:602 (`_perfcounters.count("drops")` rule-decision drops only) |
| Centreon | ✗ centreon.md (no drop counter) |
| Zabbix | partial zabbix.md (buffer-full WARN log only) |
| LibreNMS | ✗ librenms.md (no drop counter) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md (no drop counter) |
| Sensu | ✗ sensu.md (no drop counter) |
| Telegraf | partial telegraf.md (no per-plugin drop counter) |
| Logstash | – no evidence |
| Datadog Agent | partial datadog-agent.md (`EventPlatformEventsErrors[network-devices-snmp-traps]` proxy) |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md (Celery task counters) |
| Cribl | partial cribl.md (Source metrics) |
| SolarWinds | ✗ solarwinds.md:170 (no trap pipeline self-telemetry) |
| Dynatrace | ✗ dynatrace.md:239 (silent kernel drop; no data-source counter) |
| LogicMonitor | – no evidence |

### Silent drop?

| System | Silent drop possible |
|---|---|
| OpenNMS | partial (kernel UDP overflow only) |
| Zenoss | ✓ zenoss.md (v3 unknown-creds silent drop at INFO level) |
| CheckMK | ✓ checkmk.md (unauthenticated PDU silent drop at VERBOSE) |
| Centreon | ✓ centreon.md (unknown traps silently dropped unless `unknown_trap_enable=1`) |
| Zabbix | ✓ zabbix.md (kernel drops silent) |
| LibreNMS | ✓ librenms.md (no-device-match silent drop, `Log::warning` to file only) |
| Nagios+SNMPTT | ✓ nagios-snmptt.md (unknown OIDs silent unless `unknown_trap_log_enable=1`) |
| Sensu | ✓ sensu.md (v1 ignored by Classic ext) |
| Telegraf | ✓ telegraf.md (trap DROPPED on OID lookup failure) |
| Logstash | – no evidence |
| Datadog Agent | partial (silent kernel drop on channel-full) |
| Splunk SC4SNMP | partial (silent drop only at decode failure) |
| Cribl | ✓ cribl.md (silent UDP loss in Cloud tier) |
| SolarWinds | – no evidence |
| Dynatrace | ✓ dynatrace.md:239 |
| LogicMonitor | ✓ logicmonitor.md:405 (EventSource: unmatched traps silently dropped) |

---

## UI SURFACE

### Dedicated trap UI

| System | Dedicated trap UI |
|---|---|
| OpenNMS | partial (Events + Alarms tables; UI MIB compiler; Vue3 Trapd Config page) |
| Zenoss | partial (ExtJS Events Console; not trap-specific) |
| CheckMK | partial (Events Console — unified events) |
| Centreon | ✓ (Configuration → SNMP traps + sub-pages) |
| Zabbix | ✗ zabbix.md:537 (no dashboard widget for traps) |
| LibreNMS | ✗ librenms.md:894 (eventlog table only; no first-class trap dashboard) |
| Nagios+SNMPTT | ✓ NSTI Flask UI (Traplist/Inspector) |
| Sensu | partial (Sensu UI generic) |
| Telegraf | ✗ telegraf.md (no UI at all) |
| Logstash | ✗ logstash.md (Kibana via output) |
| Datadog Agent | partial (SaaS Logs Explorer source:snmp-traps) |
| Splunk SC4SNMP | partial (Splunk index `netops`) |
| Cribl | partial (Cribl visual pipeline builder; no trap-specific UI) |
| SolarWinds | ✓ Log Viewer / legacy Trap Viewer |
| Dynatrace | partial (Unified Analysis screen for SNMP Traps extension) |
| LogicMonitor | partial (portal Alert/Event UI) |

### Generic logs/events UI

| System | Generic UI |
|---|---|
| (most systems) | yes — all of {OpenNMS, Zenoss, CheckMK, Centreon, Zabbix, LibreNMS, Datadog Agent, Splunk SC4SNMP, Cribl, SolarWinds, Dynatrace, LogicMonitor} surface traps in a generic events/logs view. |

### Device-pivot view

| System | Device-pivot |
|---|---|
| OpenNMS | ✓ (node → events) |
| Zenoss | ✓ (device → events) |
| CheckMK | ✓ (host → events) |
| Centreon | ✓ (host/service) |
| Zabbix | ✓ (host → triggers/items) |
| LibreNMS | ✓ (device → eventlog tab) |
| Sensu | ✓ (entity → events) |
| SolarWinds | ✓ (node → traps) |
| Dynatrace | ✓ (entity → Unified Analysis) |
| LogicMonitor | ✓ (device → events) |

### OID-pivot view

| System | OID-pivot |
|---|---|
| OpenNMS | partial (filter by UEI / mask) |
| Zenoss | partial (filter by eventClassKey) |
| (most other) | – no evidence of dedicated OID-pivot views |

### Alert → trap drill

| System | Alert-to-trap drill |
|---|---|
| OpenNMS | ✓ (alarm → events sequence) |
| Zenoss | ✓ |
| CheckMK | ✓ (Alerts UI → ec event) |
| Datadog Agent | ✓ (SaaS Logs Monitor → log) |
| (most other) | partial — link present via event references |

### Unknown-OID review

| System | Unknown-OID review |
|---|---|
| Centreon | ✓ centreon.md:312 (`unknown_trap_file` opt-in) |
| Nagios+SNMPTT | ✓ nagios-snmptt.md:506 (`snmptt_unknown` MySQL table opt-in) |
| OpenNMS | ✓ opennms.md:215 (default-trap UEI alarm) |
| (most other) | – no dedicated unknown-OID UI |

---

## CONFIGURATION UX

### Hot-reload (no restart)

| System | Hot reload |
|---|---|
| OpenNMS | ✓ opennms.md:384 (reload via `reloadDaemonConfig`) |
| Zenoss | ✓ zenoss.md:434 (trap filters, OidMap, SNMPv3 users live) |
| CheckMK | ✓ checkmk.md:479-483 (`COMMAND RELOAD`) |
| Centreon | ✓ centreon.md:628 (SIGHUP daemon) |
| Zabbix | ✗ zabbix.md:494 (restart required for StartSNMPTrapper) |
| LibreNMS | partial librenms.md:495 (Laravel config cache) |
| Nagios+SNMPTT | partial nagios-snmptt.md:614 (SIGHUP snmptt) |
| Sensu | ✗ sensu.md:510 |
| Telegraf | partial telegraf.md:251 (SIGHUP, brief socket unbind) |
| Logstash | ✓ logstash.md:128 (`--config.reload.automatic`) |
| Datadog Agent | ✗ datadog-agent.md:410 |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md:276 |
| Cribl | partial cribl.md:427 (Leader push every 10s) |
| SolarWinds | partial solarwinds.md:464 |
| Dynatrace | partial dynatrace.md:483 (~10 min propagation) |
| LogicMonitor | partial logicmonitor.md:585 (1-15 min push) |

### Dyncfg-equivalent runtime config

| System | Runtime config API |
|---|---|
| OpenNMS | ✓ REST v2 `/api/v2/trapd/config` |
| Zenoss | ✓ JSON-RPC routers (`zep.py`); REST not first-class |
| CheckMK | ✓ OpenAPI v3 (events); WATO change-activation |
| Centreon | partial (CLAPI; no REST CRUD for trap) |
| Zabbix | partial (JSON-RPC for items) |
| LibreNMS | partial (REST API for eventlog read; no trap-handler API) |
| Nagios+SNMPTT | ✗ (config files only; NSTI mixed API) |
| Sensu | ✓ Sensu Go REST API |
| Telegraf | ✗ telegraf.md (no REST API) |
| Logstash | partial (CPM via Elasticsearch) |
| Datadog Agent | partial (datadog.yaml + per-extension config) |
| Splunk SC4SNMP | partial (Helm values) |
| Cribl | ✓ REST + OpenAPI + SDKs |
| SolarWinds | ✓ SWIS API |
| Dynatrace | ✓ Settings 2.0 + Extensions API |
| LogicMonitor | ✓ REST API + Terraform |

### Per-listener vs global config

| System | Per-listener |
|---|---|
| OpenNMS | partial (Minion location scope) |
| Zenoss | partial (per-Collector via trap_filters) |
| CheckMK | global single ec |
| Centreon | per-poller |
| Zabbix | partial (per server/proxy) |
| LibreNMS | per-snmptrapd host |
| Nagios+SNMPTT | per-collector |
| Sensu | per-Collector / per-agent |
| Telegraf | ✓ per-stanza |
| Logstash | per-pipeline |
| Datadog Agent | per-Agent |
| Splunk SC4SNMP | per-Helm-deployment |
| Cribl | per-Source |
| SolarWinds | per-APE |
| Dynatrace | per-monitoring-configuration |
| LogicMonitor | per-Collector |

---

## INTEGRATION

### Northbound trap forwarding (re-emit)

| System | Northbound trap |
|---|---|
| OpenNMS | ✓ opennms.md:430 (snmptrap-northbounder; v1/v2c/v3 traps + v2/v3 informs) |
| Zenoss | ✓ zenoss.md:476-496 (SNMPTrapAction v1/v2c + SNMPv3Action v3) |
| CheckMK | ✗ checkmk.md:524 ("No native outbound trap forwarding") |
| Centreon | partial centreon.md:696 (`@TRAPFORWARD()@` v2c only, hard-coded community) |
| Zabbix | ✗ zabbix.md:611-619 (no `MEDIA_TYPE_SNMP_TRAP`) |
| LibreNMS | ✓ librenms.md:558-612 (SNMPv2c alert transport via `snmptrap` CLI; no v1/v3) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:1103 (no native) |
| Sensu | ✓ sensu-snmp-trap-handler v1/v2c (Sensu Enterprise outbound v1/v2c documented but EOL) |
| Telegraf | ✗ telegraf.md:40 (no outbound trap output plugin) |
| Logstash | partial logstash.md:323 (no in-mirror evidence of logstash-output-snmptrap currency) |
| Datadog Agent | ✗ datadog-agent.md:24 (no trap emit) |
| Splunk SC4SNMP | ✗ splunk-sc4snmp.md (no outbound) |
| Cribl | ✓ cribl.md:34-37 (SNMP Trap Destination + `snmp_trap_serialize` function) |
| SolarWinds | ✓ solarwinds.md:50-52 (Trap Templates outbound) |
| Dynatrace | ✗ dynatrace.md:40 (no northbound forwarding) |
| LogicMonitor | ✗ logicmonitor.md:38 (no native; webhook workarounds) |

### Notification channels

| System | Notification channels |
|---|---|
| OpenNMS | ✓ (notifd: email/SMS/PagerDuty/XMPP/snmptrap) |
| Zenoss | ✓ (Email, SMS, Command, Snmptrap, Slack, PagerDuty via ZenPacks) |
| CheckMK | ✓ (`@NOTIFY` → cmk --notify; email/Slack/PagerDuty) |
| Centreon | ✓ (notification commands; Stream Connectors) |
| Zabbix | ✓ (EMAIL/EXEC/SMS/WEBHOOK) |
| LibreNMS | ✓ (57 alert transports) |
| Nagios+SNMPTT | ✓ (Nagios commands.cfg notification commands) |
| Sensu | ✓ (Sensu handlers) |
| Telegraf | ✗ (no notification) |
| Logstash | ✗ (downstream) |
| Datadog Agent | partial (SaaS-side) |
| Splunk SC4SNMP | partial (Splunk-side) |
| Cribl | ✗ (downstream) |
| SolarWinds | ✓ (Email, SMS, EXEC, snmptrap, ServiceNow integrations) |
| Dynatrace | ✓ (PagerDuty/Slack/ServiceNow/Jira) |
| LogicMonitor | ✓ (PagerDuty/ServiceNow/Slack/Teams/OpsGenie/email/SMS/HTTP) |

### API access to trap data

| System | API access |
|---|---|
| OpenNMS | ✓ REST v2 events/alarms |
| Zenoss | ✓ JSON-RPC + Jetty events |
| CheckMK | ✓ OpenAPI events |
| Centreon | ✓ JSON-RPC + direct SQL |
| Zabbix | ✓ JSON-RPC + history.* API |
| LibreNMS | ✓ REST eventlog |
| Nagios+SNMPTT | partial NSTI JSON API |
| Sensu | ✓ Sensu Go API |
| Telegraf | n/a (downstream) |
| Logstash | n/a (downstream Elasticsearch) |
| Datadog Agent | ✓ SaaS Logs API |
| Splunk SC4SNMP | ✓ Splunk Search API |
| Cribl | ✓ Cribl Search / Cribl Lake |
| SolarWinds | ✓ SWIS / SWQL |
| Dynatrace | ✓ DQL / Grail API |
| LogicMonitor | ✓ REST API |

### Manager-of-managers integration

| System | MoM |
|---|---|
| OpenNMS | ✓ (snmptrap-northbounder) |
| Zenoss | ✓ (SNMPTrapAction + ZENOSS-MIB) |
| LibreNMS | ✓ (LIBRENMS-NOTIFICATIONS-MIB outbound) |
| SolarWinds | ✓ (Trap Templates outbound) |
| Cribl | ✓ (SNMP Trap Destination) |
| (others) | partial via syslog forwarder or webhook |

---

## TEST FIXTURES

### Test traps / PCAPs / fixture files

| System | Fixtures |
|---|---|
| OpenNMS | ✓ opennms.md:580-588 (TRAP-TEST-MIB.mib; udpgen synthetic load tester) |
| Zenoss | ✓ zenoss.md:631-632 (trapdump.pcap 2 packets; sendSnmpPcap.py) |
| CheckMK | partial checkmk.md:707 (synthetic via `snmptrap` CLI; no PCAPs) |
| Centreon | partial centreon.md:908 (Cypress JSON form fixtures only) |
| Zabbix | ✓ zabbix.md:762-783 (ha{1..4}.trap fixtures for HA resume) |
| LibreNMS | ✓ librenms.md:710-749 (80 PHPUnit test classes; embedded textual traps) |
| Nagios+SNMPTT | partial nagios-snmptt.md:861 (upstream SNMPTT sample-trap files; not in mirror) |
| Sensu | – no evidence (Sensu has no unit tests for trap subsystem) |
| Telegraf | partial telegraf.md (unit tests via `gosnmp.SendTrap()`) |
| Logstash | – no evidence |
| Datadog Agent | partial datadog-agent.md:394 (integrations-core IF-MIB test fixture) |
| Splunk SC4SNMP | partial splunk-sc4snmp.md (integration tests via Celery) |
| Cribl | – no evidence (closed-source) |
| SolarWinds | – no evidence (closed-source) |
| Dynatrace | – no evidence (closed-source) |
| LogicMonitor | – no evidence (closed-source) |

### Test framework

| System | Test framework |
|---|---|
| OpenNMS | JUnit (13 trap-specific classes); smoke-test |
| Zenoss | Python unittest (5 test files, ~2,492 lines) |
| CheckMK | pytest; 20 cmk-ec tests + integration testEc.py |
| Centreon | Cypress E2E (UI only); Behat REST/Postman + IF-MIB.txt |
| Zabbix | testSnmpTrapsInHa.php (160 lines, integration) |
| LibreNMS | 80 PHPUnit feature tests with textual fixtures |
| Nagios+SNMPTT | NONE for the trap subsystem in any of NSTI/Nagios/NSCA |
| Sensu | NONE for the trap subsystem |
| Telegraf | `snmp_trap_test.go` 1646 LOC (5 functions) |
| Logstash | upstream plugin source not mirrored |
| Datadog Agent | 2,494 lines of Go tests |
| Splunk SC4SNMP | Celery integration tests |
| Cribl | n/a (closed-source) |
| SolarWinds | n/a (closed-source) |
| Dynatrace | n/a (closed-source) |
| LogicMonitor | n/a (closed-source) |

### Sample-trap dataset size

| System | Sample size |
|---|---|
| OpenNMS | 17,442 event definitions in 230 example XML files |
| LibreNMS | 4,770 MIB files in tree |
| Cribl pack | ~92,000 trap OIDs in 168 CSV lookups (community pack) |
| (others) | smaller (~5-15 fixtures or upstream) |

---

## SECURITY

### Secret storage (community/USM keys)

| System | Secret storage |
|---|---|
| OpenNMS | ✓ opennms.md:533 (Secure Credentials Vault `${scv:...}`); known issue: GET returns plaintext |
| Zenoss | ✗ zenoss.md (ZODB cleartext for v3 passphrases) |
| CheckMK | ✗ checkmk.md:639 (plaintext Python in global.mk) |
| Centreon | ✗ centreon.md:840 (cleartext in conf.pm) |
| Zabbix | partial (config.Secret for v3 since 1.20) |
| LibreNMS | ✗ librenms.md:688 (plaintext authpass/cryptopass in `devices` table; alert transport `text` type) |
| Nagios+SNMPTT | ✗ nagios-snmptt.md:797 (plaintext in snmptrapd.conf + snmptt.ini) |
| Sensu | ✗ sensu.md:592 (ZODB / Mongo plaintext); Sensu Go uses k8s Secrets |
| Telegraf | ✓ telegraf.md:402 (`config.Secret` for v3 since 1.x) |
| Logstash | partial logstash.md (Logstash keystore) |
| Datadog Agent | – no evidence |
| Splunk SC4SNMP | ✓ k8s Secrets for v3 USM (`charts/.../traps/deployment.yaml:158-178`) |
| Cribl | partial cribl.md (closed-source; UI-protected) |
| SolarWinds | ✗ solarwinds.md:421-425 (Community column in `Orion.Traps` plaintext; SWIS-exposed) |
| Dynatrace | – no evidence (closed-source) |
| LogicMonitor | partial (per-resource property; storage format undocumented) |

### SHA-2 / AES support

| System | SHA-2 / AES |
|---|---|
| OpenNMS | ✓ opennms.md:124 (SHA-224/256/512; AES192/256) |
| Zenoss | ✓ zenoss.md:191 (Net-SNMP SHA-2 + AES-192/256) |
| CheckMK | ✓ checkmk.md:147-148 |
| Centreon | ✓ (inherited from Net-SNMP) |
| Zabbix | ✓ (inherited from Net-SNMP) |
| LibreNMS | ✓ (inherited from Net-SNMP) |
| Nagios+SNMPTT | ✓ (inherited from Net-SNMP) |
| Sensu | ✓ sensu.md:191 (Net-SNMP MD5/SHA/SHA-2 family; AES-192/256 non-standard OIDs) |
| Telegraf | ✓ telegraf.md:458-459 (MD5/SHA/SHA224/SHA256/SHA384/SHA512; AES/AES192/AES256/AES192C/AES256C) |
| Logstash | ✓ logstash.md:157-159 (md5/sha/sha2/hmac*; DES/3des/aes/aes128/aes192/aes256/aes256with3desKey) |
| Datadog Agent | ✓ datadog-agent.md:222 (MD5/SHA/SHA-2; DES/AES/AES192/AES256/AES192C/AES256C — Reeder/Blumenthal variants) |
| Splunk SC4SNMP | ✓ splunk-sc4snmp.md:191 (Net-SNMP) |
| Cribl | ✓ cribl.md:159 |
| SolarWinds | ✓ solarwinds.md:194 (SHA-256/SHA-512 added in Platform 2024.2; lagged polling by 2 years) |
| Dynatrace | partial dynatrace.md:202 (USM only) |
| LogicMonitor | partial (assumed SNMP4J set) |

### Audit logging of trap-rule changes

| System | Audit |
|---|---|
| OpenNMS | ✓ opennms.md:544 (eventconf_events.last_modified/modified_by) |
| Zenoss | ✓ zenoss.md:601 (`Products.ZenMessaging.audit`) |
| CheckMK | ✓ checkmk.md:651 (WATO audit trail) |
| Centreon | ✓ centreon.md:854 (`CentreonLogAction->insertLog`) |
| Zabbix | ✓ zabbix.md:716 (auditlog table) |
| LibreNMS | partial librenms.md:700 (eventlog as audit) |
| Nagios+SNMPTT | partial nagios-snmptt.md (Apache log only for NSTI) |
| Sensu | partial (Sensu Go API audit) |
| Telegraf | – no evidence |
| Logstash | – no evidence |
| Datadog Agent | – no evidence |
| Splunk SC4SNMP | – no evidence |
| Cribl | – no evidence |
| SolarWinds | partial (Web Console audit) |
| Dynatrace | ✓ dynatrace.md (Settings 2.0 audit log) |
| LogicMonitor | ✓ logicmonitor.md (portal audit log) |

### Multi-tenant RBAC

| System | Multi-tenant |
|---|---|
| OpenNMS | ✗ opennms.md:390-391 (RBAC but not multi-tenant) |
| Zenoss | partial zenoss.md:441 (per-Collector ACL via filter regex) |
| CheckMK | ✗ checkmk.md:494 (single-tenant; CME `customer` GUI-only) |
| Centreon | partial centreon.md:637 (ACL screens only) |
| Zabbix | partial (host group permissions) |
| LibreNMS | partial librenms.md:499 (devices_perms) |
| Nagios+SNMPTT | partial nagios-snmptt.md:622 (cgi.cfg basic auth) |
| Sensu | partial (namespace) |
| Telegraf | ✗ |
| Logstash | ✗ |
| Datadog Agent | n/a (Agent side) |
| Splunk SC4SNMP | n/a (Splunk-side RBAC) |
| Cribl | partial (Cribl RBAC) |
| SolarWinds | partial solarwinds.md:470 (Account Limitations; one Orion per customer typical) |
| Dynatrace | partial dynatrace.md:488 (tenant + IAM) |
| LogicMonitor | partial logicmonitor.md:593 (MSP mode) |

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
