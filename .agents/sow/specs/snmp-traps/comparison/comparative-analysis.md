# Netdata SNMP Trap Support - Final Comparative Analysis

## 0. Scope and Evidence

This document closes the Phase A comparative-analysis loop after the Netdata
implementation work in SOW-0035 through SOW-0039. It compares the shipped
Netdata SNMP trap behavior against the 16-system cohort:

- OpenNMS, Zenoss, CheckMK, Centreon, Zabbix, LibreNMS, Nagios+SNMPTT, Sensu,
  Telegraf, Logstash, Datadog Agent, Splunk SC4SNMP, Cribl, SolarWinds,
  Dynatrace, LogicMonitor.

Evidence sources:

- Per-system specs: `../opennms.md`, `../zenoss.md`, `../checkmk.md`,
  `../centreon.md`, `../zabbix.md`, `../librenms.md`,
  `../nagios-snmptt.md`, `../sensu.md`, `../telegraf.md`,
  `../logstash.md`, `../datadog-agent.md`, `../splunk-sc4snmp.md`,
  `../cribl.md`, `../solarwinds.md`, `../dynatrace.md`,
  `../logicmonitor.md`.
- Cross-system matrix: `feature-matrix.md`.
- Operator feature catalogue: `operator-features.md`.
- Netdata design stress-test and implications:
  `netdata-stress-test.md`, `netdata-design-implications.md`.
- Netdata target design: `../netdata.md`.
- Current generated OOB catalogue:
  `../../../../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json`.

Current implementation evidence used for this synthesis:

- `../netdata.md:111` defines listener-as-job lifecycle, creation-time
  preflight, per-job journal layout, and profile lazy-loading.
- `../netdata.md:199` defines UDP/162 binding, capabilities, and fatal
  endpoint-bind semantics.
- `../netdata.md:223` defines INFORM response semantics and persisted local
  engine state.
- `../netdata.md:573` defines opt-in dedup with periodic summaries.
- `../netdata.md:643` defines per-job journal storage fields.
- `../netdata.md:933` defines plugin self-metrics.
- `operator-features.md:9` through `operator-features.md:76` define the
  Tier-1 operator table-stakes.
- `operator-features.md:84` through `operator-features.md:149` define the
  Tier-2 deployment expectations.
- `operator-features.md:162` through `operator-features.md:248` define the
  Tier-3 specialty features.
- `feature-matrix.md:17`, `feature-matrix.md:38`, `feature-matrix.md:59`,
  `feature-matrix.md:80`, `feature-matrix.md:101`, `feature-matrix.md:122`,
  and `feature-matrix.md:143` cover reception/security comparisons.
- `feature-matrix.md:273`, `feature-matrix.md:407`,
  `feature-matrix.md:501`, `feature-matrix.md:610`,
  `feature-matrix.md:782`, `feature-matrix.md:879`,
  `feature-matrix.md:946`, and `feature-matrix.md:1138` cover decode,
  identity, severity, realtime, storage, distribution, rate-limit, and
  northbound forwarding.

The analysis below is about the feature-branch behavior before final real-use
Logs UI validation. Where a feature is implemented but not yet real-device/UI
verified, this file says so explicitly.

## 1. Executive Verdict

Netdata is merge-candidate competitive for a first SNMP trap release, provided
the final gate keeps three truths visible:

1. Netdata now covers all Tier-1 table-stakes except a trap-specific lifecycle
   state machine, which is intentionally left to existing Netdata alerting and
   log workflows.
2. Netdata is above the cohort median on creation-time failure detection,
   per-source allowlist, per-source rate limiting, distributed hub deployment,
   OOB profile breadth, journal field richness, default Logs facets, OTLP
   export, and pipeline self-metrics.
3. Netdata does not yet ship native trap ACK/clear lifecycle, northbound SNMP
   forwarding, topology-aware suppression, DTLS/TLS-TM, or trap annotations on
   metric charts.

The release posture should be:

- **Ship as an SNMP trap listener, profile resolver, structured journal
  writer, and alertable self-metric producer.**
- **Do not market it as a replacement for OpenNMS/Zenoss-style trap alarm
  lifecycle management.**
- **Be explicit that custom MIB conversion is CLI-based, not a UI upload flow.**
- **Keep reverse DNS disabled by default; prefer SNMP/topology identity.**
- **Keep allowlist open by default for easy adoption, but document production
  hardening strongly.**

## 2. Shipped Netdata Behavior by Operator Need

### Tier-1 Table Stakes

| Need | Cohort baseline | Netdata behavior | Verdict |
|---|---|---|---|
| v1/v2c/v3 USM reception | All 16 support v1/v2c/v3 in some form (`operator-features.md:9`, `feature-matrix.md:17`). | Supports v1, v2c, v3 USM. Multiple USM users can be accepted by one job. | Meets table-stakes. |
| OID to symbolic name | 14/16 full, CheckMK off by default, Cribl external lookup (`operator-features.md:15`). | OOB trap profiles resolve OID, name, category, severity, description, varbind labels. Unknown OIDs still journal as raw OID with severity `notice`. | Meets and is stronger than pipeline-only systems. |
| Device identification | 12/16 full, 4 partial (`operator-features.md:24`). | Uses PDU source identity first, then SNMP collector/topology identity where available, reverse DNS only when explicitly enabled, source IP last. | Meets, with better NAT/VRF correctness than many systems. |
| Severity assignment | Built-in or operator-defined in most cohort systems; none in Telegraf/Logstash/Cribl (`operator-features.md:33`, `feature-matrix.md:501`). | Profile `severity` is required and authoritative; operator override can replace it; unknown OID fallback is `notice`. | Meets. This was in the spec from the start. |
| Storage, retention, search | 14/16 have store; Telegraf/Logstash/Cribl delegate downstream (`operator-features.md:42`, `feature-matrix.md:782`). | Per-job SDK-backed journal files under the trap cache directory, with configurable retention/rotation and systemd-journal query path. | Meets, but real-use UI validation is still pending. |
| Web/UI trap browsing | Most full NMS/SaaS systems provide dedicated or generic logs/events UI (`operator-features.md:55`). | Uses existing Logs UI through systemd-journal fields and SOW-0039 default facets. | Meets via generic Logs UI, not a dedicated trap screen. |
| Alerting integration | 16/16 can route alerts somehow (`operator-features.md:64`). | Emits self-metrics by category/severity/errors/dedup. Operators can opt selected OIDs into dedicated metric contexts for alerts. | Meets through Netdata alerting; no trap-specific ACK/clear engine. |
| Authentication / restricted access | SNMPv3 everywhere; only Cribl and Dynatrace have first-class IP allowlist (`operator-features.md:70`, `feature-matrix.md:101`). | SNMPv3 USM, static engine whitelist by default, dynamic engine discovery opt-in, source CIDR allowlist pre-decode. | Above cohort median. |

### Tier-2 Real Deployment Features

| Need | Cohort baseline | Netdata behavior | Verdict |
|---|---|---|---|
| Vendor MIB/profile pack | Datadog and SolarWinds broadest; LibreNMS broad; many systems minimal (`operator-features.md:84`). | Current generated catalogue has 437 profile files, 3,131 MIB modules, 71,787 trap definitions, and 44,462 varbind definitions. Profiles load lazily on first trap job and are shared across jobs. | Above most OSS systems for first release breadth. |
| Custom MIB workflow | UI in OpenNMS/Zenoss/CheckMK/Centreon/LogicMonitor; CLI/drop-in elsewhere (`operator-features.md:95`). | Installed Go helper `snmp-trap-profile-gen` converts MIBs offline; operator copies YAML to `/etc/netdata/go.d/snmp.trap-profiles/` and runs `snmp_traps:reload-profiles` or re-applies the job. | Meets CLI/drop-in tier; no UI upload yet. |
| Trap-to-alert routing | 13/16 support via rules or downstream systems (`operator-features.md:105`). | Alertable self-metrics plus opt-in per-OID metrics. Profile severity is in journal and can be used in Logs queries. | Meets basic routing; less lifecycle-rich than OpenNMS/Zenoss/LogicMonitor. |
| Alert acknowledgement / clear | Full auto-clear only OpenNMS, Zenoss, LogicMonitor; CheckMK partial (`operator-features.md:111`). | No native trap lifecycle state machine. Operators use logs plus existing alerts, or alert on polled state. | Intentional gap. Do not over-market. |
| Dedup/suppression | Built-in in OpenNMS/Zenoss/Centreon/CheckMK/Cribl; partial in others (`operator-features.md:121`). | Optional per-job dedup, default off, first event journaled, later duplicates counted and summarized. | Competitive when enabled; forensic trade-off is explicit. |
| Topology drilldown | Built-in in OpenNMS/Zenoss/LibreNMS/SolarWinds/Dynatrace/LogicMonitor; limited or none elsewhere (`operator-features.md:130`). | Enriches device identity from SNMP/topology state; no dedicated topology suppression. | Partial for first release. |
| Pipeline self-monitoring | First-class in OpenNMS, Telegraf, Datadog; partial elsewhere (`operator-features.md:140`). | First-class self-metrics for events by category/severity, error dimensions, dedup suppression, OTLP export failure. | Above cohort median. |
| Distributed deployment | Multi-tier in several NMS/SaaS systems; pipeline systems deploy per site (`operator-features.md:149`, `feature-matrix.md:879`). | Netdata hub model: one Agent per site can receive traps, with multiple listeners only for scaling/isolation. | Strong fit for Netdata architecture; hub-down trade-off remains operator design. |

### Tier-3 Specialty Features

| Need | Cohort baseline | Netdata behavior | Verdict |
|---|---|---|---|
| Northbound SNMP forwarding | Native in OpenNMS/Zenoss/LibreNMS/Centreon/SolarWinds/Cribl; absent in many (`operator-features.md:162`, `feature-matrix.md:1138`). | Not implemented. OTLP export exists as vendor-neutral log export, not SNMP re-emit. | Gap. |
| INFORM acknowledgement | Mixed; CheckMK and Dynatrace explicitly no (`operator-features.md:174`, `feature-matrix.md:122`). | v2/v3 INFORM response using receive socket; persisted local engine ID and boots for v3. | Above several systems; still needs real-device verification. |
| Multiple v3 users | Multi-user in several systems; Telegraf/Logstash single-user (`operator-features.md:185`, `feature-matrix.md:38`). | Multiple USM users per listener job. | Meets enterprise need. |
| DTLS/TLS-TM | None of the 16 support it (`operator-features.md:194`, `feature-matrix.md:143`). | Not implemented. | Acceptable non-goal. |
| Listener rate limiting | None of the 16 has first-class listener per-source rate limit (`operator-features.md:200`, `feature-matrix.md:946`). | Per-source token bucket with drop/sample behavior, default off. | Cohort-leading, but operators must enable it. |
| Topology-aware suppression | None ship it as built-in (`operator-features.md:206`). | Not implemented. | Acceptable first-release gap. |
| Trap annotations on metric charts | SolarWinds/Dynatrace partial; most none (`operator-features.md:212`). | Not implemented. | Gap for later UX work. |
| Replay/sample trap tooling | Zenoss/OpenNMS/CheckMK strongest; others use Net-SNMP CLI (`operator-features.md:248`). | Test fixtures and pcap replay exist in Go tests; no operator-facing replay CLI yet. | Developer coverage good; operator tooling later. |

## 3. Where Netdata Leads

### 3.1 Creation-Time Failure Detection

Netdata's job creation preflights endpoint bind, journal directory creation/open,
writer initialization, profile loading, retention validation, local engine ID
state, engine boots state, and OTLP preflight where enabled (`../netdata.md:111`,
`../netdata.md:199`, `../netdata.md:223`). This is materially better than
systems that allow runtime-only log failure for bind, MIB, or output errors.

This matters because DynCfg users see an apply-time error instead of a false
"started" signal followed by logs-only failure.

### 3.2 Profile Memory Discipline

Profiles load on first trap job, not at plugin process start, and the loaded
cache is shared across trap jobs. This matches the product constraint that
go.d.plugin is installed everywhere, while only a small subset of agents will
receive traps.

The current OOB catalogue is broad enough to be meaningful on day one:

- 437 generated profile files.
- 3,131 MIB modules represented.
- 71,787 trap definitions.
- 44,462 varbind definitions.

These figures come from the committed `catalogue.json` aggregate at the merge
gate.

### 3.3 Security and Abuse Controls

Compared with the cohort:

- Source CIDR allowlist is first-class, while only Cribl and Dynatrace expose
  comparable first-class listener allowlists in the matrix
  (`operator-features.md:70`, `feature-matrix.md:101`).
- Dynamic SNMPv3 sender engine discovery is opt-in, not default, addressing the
  security concern raised in `netdata-stress-test.md:193` and converted into
  implementation guidance in `netdata-design-implications.md:206`.
- Per-source token-bucket rate limiting is implemented, while the 16-system
  cohort had no first-class listener rate-limit feature
  (`operator-features.md:200`).

### 3.4 Structured Journal Contract

Netdata writes stable plugin-controlled fields for OID, name, category,
severity, PDU type, version, source IP, UDP peer, vendor, interface, neighbor
context, dedup summary, and varbind JSON (`../netdata.md:643`). SOW-0039 adds
these static `TRAP_*` fields to systemd-journal default facets, while dynamic
`TRAP_TAG_*` labels remain user-selectable facets rather than hard-coded
defaults.

This positions Netdata closer to SaaS log-document systems (Datadog,
Dynatrace, Splunk, LogicMonitor) than to classic trap daemons, but without
requiring a central SaaS ingestion tier.

### 3.5 Self-Monitoring

Netdata exposes the trap pipeline as metrics:

- Events by category and severity.
- Error counters by failure class.
- Dedup-suppressed counters.
- OTLP export failure counters.

The cohort has first-class trap-pipeline telemetry only in a few systems
(`operator-features.md:140`). Netdata should keep this as a differentiator.

## 4. Where Netdata Is Competitive but Not Dominant

### 4.1 Custom MIB Workflow

The CLI-based `snmp-trap-profile-gen` flow is comparable to Datadog,
Nagios+SNMPTT, Zabbix, LibreNMS, Telegraf, Logstash, Dynatrace, and Splunk
drop-in workflows (`operator-features.md:95`). It is less convenient than
OpenNMS/Zenoss/CheckMK/Centreon/LogicMonitor UI workflows.

The important release requirement is documentation clarity:

- The installed helper is the operator path.
- The legacy Python tooling is source-tree reference tooling.
- Reload uses the Function name `snmp_traps:reload-profiles`, not a JSON
  method wrapper.

### 4.2 Alerting Model

Netdata can alert on broad trap rates and opt-in OID counters. That is enough
for first release and matches the "trap as event/log + metrics for alerts"
architecture in `../netdata.md:23` and `../netdata.md:52`.

It does not match OpenNMS/Zenoss/LogicMonitor for paired-clear lifecycle. This
is acceptable only if docs avoid implying that trap ACK/clear state is native.

### 4.3 Topology Integration

Netdata has a strong advantage because SNMP polling and topology can run on the
same hub. The shipped fallback order should remain:

1. SNMP collector vnode hostname.
2. SNMP sysName from registry.
3. Topology sysName matched by topology IP state.
4. Reverse DNS only when explicitly enabled.
5. Source IP.

This is better than raw-IP-only pipeline systems. It is not yet equivalent to a
full topology-aware suppression engine.

### 4.4 Performance

Current evidence:

- In-memory packet path is much faster than the journal output path.
- The committed full packet-to-journal benchmark with SDK `go/v0.4.0` and
  SOW-0045 local writer hot-path optimization measured about 62.5K to 72.6K
  persisted traps/sec for synthetic v2c profile-hit traffic on the workstation
  for repeated 30,000-packet runs. The longer 100,000-packet run measured
  about 63.3K to 66.0K persisted traps/sec. Historical `go/v0.3.0` evidence
  measured about 54K to 62K persisted traps/sec for the committed benchmark;
  the early `go/v0.4.0` pre-optimization repeat measured about 30.5K to
  38.0K persisted traps/sec.
- Narrow benchmark names in the committed tree include decode, packet path,
  multi-job, BER rejection, queued writer drain, and direct journal write.

Release interpretation:

- The old unsupported claim "30K rows/sec per writer thread" is no longer the
  decision basis.
- The final gate should cite the committed v0.4.0 dependency and committed
  SOW-0045 benchmark, not the earlier temporary overlay benchmark or the
  pre-optimization v0.4.0 repeat.
- The benchmark is synthetic; real device mixes with many varbinds, v3 privacy,
  OTLP export, dedup, and slower storage can be lower.

## 5. Known Gaps and Non-Goals

These are not regressions. They are conscious first-release boundaries:

1. **No native trap ACK/clear lifecycle state machine.**
   Evidence: only OpenNMS, Zenoss, and LogicMonitor have full auto-clear
   (`operator-features.md:111`).

2. **No northbound SNMP trap forwarding.**
   Evidence: cohort support is mixed and mainly classic NMS oriented
   (`operator-features.md:162`, `feature-matrix.md:1138`). Netdata's OTLP
   exporter is not equivalent.

3. **No DTLS/TLS-TM.**
   Evidence: none of the 16 systems supports this in product
   (`operator-features.md:194`, `feature-matrix.md:143`).

4. **No topology-aware suppression.**
   Evidence: no cohort system ships it as built-in
   (`operator-features.md:206`). Netdata can revisit this later because it has
   local topology state.

5. **No trap annotation overlay on metric charts.**
   Evidence: only SolarWinds and Dynatrace have partial built-in stories
   (`operator-features.md:212`).

6. **No UI MIB upload flow.**
   The current workflow is CLI conversion plus profile reload. This is
   acceptable for first release but should not be described as a UI experience.

7. **No guarantee that reverse DNS is used for identity.**
   Reverse DNS is optional and disabled by default. Primary identity is SNMP
   collector/topology-derived, then source IP fallback.

## 6. Release Gate

Before closing SOW-0039 and the implementation SOWs, the branch must satisfy:

1. **Dependency truth:** the committed Go dependency must be
   `github.com/netdata/systemd-journal-sdk/go v0.4.0` or newer if the final
   throughput evidence depends on newer SDK behavior.
2. **Benchmark truth:** run a fresh trap benchmark batch against the committed
   dependency and record the result in SOW-0039. Do not cite temporary-module
   numbers as final branch evidence.
3. **Runnable trap evidence:** receive at least one real or synthetic trap
   through the installed plugin, verify journal rows exist, and verify the
   systemd-journal Function can filter by at least `TRAP_OID`,
   `TRAP_CATEGORY`, and `TRAP_SEVERITY`.
4. **Logs UI evidence:** if Cloud/UI is reachable in the test environment,
   verify the default trap facets appear. If not reachable, record it as a
   tracked validation gap, not as passed.
5. **Docs honesty:** docs must state that UDP/162 requires packaged
   capabilities or equivalent manual capability setup; custom MIB conversion is
   CLI-based; allowlist and rate-limit defaults are adoption-friendly rather
   than hardened.
6. **No sensitive artifacts:** benchmark and validation notes must not include
   real SNMP communities, USM keys, live device hostnames, customer identifiers,
   or public IPs.

## 7. Final Comparative Position

Netdata's first trap release should be positioned as:

- A distributed, hub-local SNMP trap listener.
- A broad-profile OID resolver and structured journal writer.
- A Logs UI and Function query source.
- A producer of trap-pipeline self-metrics and optional per-OID alert metrics.
- A safer operational receiver than many cohort systems because creation-time
  failures are surfaced at DynCfg apply, not left to runtime logs.

It should not be positioned as:

- A classic NMS alarm-lifecycle engine.
- A northbound SNMP trap router.
- A full trap rule GUI.
- A topology-suppression engine.
- A replacement for device state polling.

The cohort evidence supports shipping the current scope after the final
validation gate, with the above caveats visible in docs and SOW outcome.
