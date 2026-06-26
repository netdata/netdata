# SNMP Trap Metric Profiles

## Status

- Status: Trap-to-metrics design with Implementation Phase A profile-defined
  metrics and Implementation Phase B receiver/pipeline metrics implemented in
  this branch. The Phase B external reviewer gate is in progress.
- Design status: recommended design written; four external design review rounds
  and one follow-up UX review round incorporated. The user approved compact
  operator authoring syntax that expands to canonical form, while stock/generated
  profiles use canonical form for reviewability. The user approved long-term-best
  compatibility handling: trap metrics have not shipped as a public contract, so
  the old job-level trap `metrics:` list can be removed or renamed instead of
  retained as a deprecated compatibility shim.
- Review plan:
  - Phase 1: external gap analysis of the use-case inventory, completed and
    incorporated.
  - Phase 2: design proposal and external design review.
  - Implementation Phase A: profile-defined trap metrics.
  - Implementation Phase B: receiver/pipeline metrics after Phase A passes the
    requested external reviewer gate.

## Purpose

Netdata needs a trap-to-metrics system that lets operators convert selected SNMP
trap signals into useful metrics without losing the forensic value of trap logs.

The system must support practical, profile-backed use cases across stock and
operator-provided trap profiles. The first requirement is to identify the whole
goal before designing the schema.

## Scope Of This Spec

This document records:

- operator use cases;
- concrete trap/profile examples;
- why the current implementation is insufficient;
- practical constraints the future design must satisfy;
- the recommended profile-based design for supporting these use cases.

This spec intentionally does not define:

- runtime data structures;
- exact Go types;
- file-by-file implementation plan;
- final default enablement policy for stock-generated metric rules;
- UI or dashboard design.
- exact Go types for receiver/pipeline metrics. This spec defines the
  product-level requirements those metrics must satisfy when implemented as
  Phase B.

## Ground Rules

- Per-device visibility is mandatory. A trap listener often receives traps from
  many devices, so totals across all devices are not enough.
- Trap logs remain the source of full event detail. Metrics are derived signals
  for dashboards, alerting, trend detection, and quick triage.
- High-cardinality values such as usernames, MAC addresses, source IPs,
  interface names, peer addresses, alert keys, and free-form descriptions must
  not become unbounded metric labels by default.
- A trap-derived state is event-driven and lossy unless reconciled by polling.
  Missed clear traps, packet loss, and device reboot can make state stale.
- The stock profile pack can provide common, safe behavior, but operators must
  be able to add or adjust site-specific trap metrics through custom profiles.
- Every accepted use case must be backed by real profile/MIB evidence or by an
  established open-source monitoring implementation.

## Industry Context

Existing open-source NMS implementations mostly treat SNMP traps as events,
alarms, or state mutations, not as a profile-driven trap-to-time-series
extraction system. Netdata should reuse their operational lessons for resource
identity, problem/clear pairing, and cardinality, but should not assume there is
a mature open-source trap-metric profile schema to copy.

Evidence:

- `librenms/librenms @ 5ffbb16324ad7c25f9588801ca9d4d52da45ea21`
  `LibreNMS/Snmptrap/Handlers/LinkDown.php:48` through `:74` resolves
  `ifIndex`, updates the matched port, and logs interface-scoped events.
- `librenms/librenms @ 5ffbb16324ad7c25f9588801ca9d4d52da45ea21`
  `LibreNMS/Snmptrap/Handlers/BgpEstablished.php:48` through `:65` resolves a
  BGP peer from the trap OID suffix and updates peer state.
- `opennms/opennms @ 40cc8535351f09c24978771a8832cfc286b85572`
  `docs/modules/operation/pages/deep-dive/alarms/configuring-alarms.adoc:56`
  through `:142` documents reduction keys, problem/resolution alarm types, and
  clear keys.

## Pre-Phase-A Implementation Baseline

Historical facts from the unreleased SNMP trap collector before Phase A
replaced job-level operator metrics with profile-defined metrics:

- Job-level operator metrics were configured as `oid`, `context`, and optional
  `dimension_from_varbind`.
- The old runtime incremented counters when the trap OID matched.
- The old runtime did not extract numeric values from trap varbinds.
- The old runtime did not filter metrics by varbind predicates.
- The old runtime rejected duplicate configured OIDs, so one trap OID could not
  emit several independent metrics through the job-level contract.
- Existing static metric instances were scoped by listener job, not by source
  device.
- Trap profile YAML had `extends`, `varbinds`, and `traps`, but no profile-local
  metric section.

Evidence:

- Pre-Phase-A `src/go/plugin/go.d/collector/snmp_traps/config.go:72` through
  `:76`.
- Pre-Phase-A `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:23`
  through `:55` (file removed by Phase A).
- Pre-Phase-A
  `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:192` through
  `:219` (file removed by Phase A).
- Pre-Phase-A
  `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:380` through
  `:420` (file removed by Phase A).
- Pre-Phase-A `src/go/plugin/go.d/collector/snmp_traps/profile.go:156`
  through `:164`.

## Use Case 1: Per-Device Trap Activity

Operator question:

- Which device is producing trap activity?
- Which devices suddenly started sending more state-change, security, auth,
  config-change, or diagnostic traps?
- Which device is responsible for a spike seen on a shared trap listener?

Concrete examples:

- A single trap listener receives `IF-MIB::linkDown` and `IF-MIB::linkUp` from
  many switches.
- A single trap listener receives `SNMPv2-MIB::authenticationFailure` from many
  devices.
- A trap hub receives BGP transition traps from many routers.

Why it matters:

- Listener-level totals are operationally ambiguous. They answer "did the hub
  receive traps?", not "which device is affected?".
- Device-level views are the minimum useful identity for Netdata dashboards,
  alerts, and triage.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15536`
  through `:15555` defines `IF-MIB::linkDown` and `IF-MIB::linkUp`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15557`
  defines `SNMPv2-MIB::authenticationFailure`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:10068`
  through `:10086` includes current BGP established/backward-transition
  notifications.

## Use Case 2: Filtered Event Counters

Operator question:

- How many trap events of a specific kind happened after excluding expected or
  low-value events?
- Can planned or harmless state changes be separated from operational problems?

Concrete examples:

- Count `IF-MIB::linkDown` only when `ifAdminStatus=up`, so planned
  administrative shutdowns are not counted as unexpected link failures.
- Count Cisco CLI running-config changes only for terminal types such as
  `console`, `terminal`, `virtual`, or another explicitly selected enum value.
- Count only CloudVision firing notifications whose carried severity is
  `critical`.

Why it matters:

- Trap OID alone is often too coarse.
- Profiles already know the varbind names and many enum values needed to make
  useful distinctions.
- Without filtering, operators must either count noisy events or avoid metrics
  for those traps entirely.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:2209`
  through `:2215` defines `ifAdminStatus` enum values.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15536`
  through `:15545` shows `linkDown` carries `ifAdminStatus`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:4211`
  through `:4220` defines Cisco terminal type enum values.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:4221`
  through `:4224` defines the terminal user as `DisplayString`; this is useful
  log detail, not a safe default metric label.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:33139`
  through `:33148` shows `ccmCLIRunningConfigChanged` carries terminal type and
  terminal user.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:106`
  through `:113` defines `aristaCvAlertSeverity`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:271`
  through `:284` shows CloudVision firing notifications carry the severity
  varbind.

## Use Case 3: Numeric Samples Carried By Traps

Operator question:

- What value did the device report when it emitted the trap?
- How close was the resource to the threshold or limit?
- Did the reported value trend upward across repeated threshold traps?

Concrete examples:

- Arista hardware utilization alerts carry in-use entries and high-watermark
  values.
- Juniper DFC packet-rate threshold traps carry input packet rate and watermark
  values.
- Juniper IDP CPU and memory notifications carry usage and threshold values.

Why it matters:

- Some traps are not just event notifications. They carry the measurement that
  triggered the notification.
- Counting those traps loses the most useful numeric information.
- Operators need docs to understand that these are last trap-reported values
  held by the collector, not continuously polled gauges.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:244`
  through `:253` shows Arista hardware utilization values.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4428`
  through `:4460` shows Juniper DFC packet-rate and watermark values.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4542`
  through `:4565` shows Juniper IDP CPU/memory usage and threshold values.

## Use Case 4: Multiple Metrics From One Notification Or Notification Family

Operator question:

- Can one received trap update all useful metric values carried by that trap?
- Can related reported values be charted together without requiring duplicate
  trap-profile copies?

Concrete examples:

- Arista CLB flow threshold notifications carry allocated, unallocated, learned,
  threshold, and limit values across the same notification family.
- Juniper DFC memory threshold traps carry flow usage, flow watermarks, criteria
  usage, and criteria watermarks in one notification.

Why it matters:

- Real traps frequently carry several related numeric fields.
- Restricting one trap OID to one metric forces operators to choose one value
  and discard the rest.
- Related values often belong on the same chart or in a small group of charts.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:300`
  through `:347` shows Arista CLB flow fields.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4497`
  through `:4523` shows Juniper DFC hard memory fields.
- Pre-Phase-A `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:27`
  through `:41` shows the removed job-level metrics rejected duplicate OIDs.

## Use Case 5: Per-Resource Trap Metrics

Operator question:

- Which interface, VLAN, service set, port group, sensor, peer, or local device
  resource is affected?
- Can the dashboard show the noisy or failing resource without turning every
  arbitrary varbind into a metric label?

Concrete examples:

- Count `linkDown` and `linkUp` per device interface using `ifIndex` or an
  enriched interface identity.
- Count Cisco port-security violations per interface or VLAN while keeping the
  violating MAC address in logs only.
- Track Juniper DFC packet-rate threshold events per interface.
- Track Arista CLB flow threshold events per port group.

Why it matters:

- Device-level identity is necessary but not always sufficient.
- Network operators often troubleshoot at interface, peer, sensor, or service
  object level.
- Raw resource names can be high-cardinality, unstable, or sensitive, so the
  use case requires bounded resource identity rather than arbitrary labels.
- Resource strings such as port-group names can be used only when the profile or
  operator explicitly treats them as bounded resource identities.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:2224`
  through `:2227` defines `ifIndex` as a large interface index range.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:37706`
  through `:37735` shows Cisco port-security traps carry interface/VLAN/MAC
  detail.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4428`
  through `:4460` shows Juniper DFC packet-rate traps carry interface names.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:300`
  through `:347` shows Arista CLB port-group fields.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:337`
  through `:342` distinguishes high-cardinality log content from bounded metric
  labels.

## Use Case 6: Trap-Derived Current State

Operator question:

- Is this resource currently in a problem state according to the last received
  trap pair?
- Which resources are currently firing, asserted, exceeded, down, or degraded?

Concrete examples:

- `IF-MIB::linkDown` marks an interface down; matching `IF-MIB::linkUp` clears
  it.
- Arista external alarm asserted/deasserted notifications track active external
  alarms.
- Arista CloudVision firing/resolved notifications track active CloudVision
  alerts.
- Juniper exceeded/under-threshold or notify/restored pairs track active
  threshold state.

Why it matters:

- Event counters show rate and flapping, but they do not answer "is it still
  active?".
- Many vendor MIBs model problem and clear notifications as separate traps.
- The resulting state is useful, but it must be documented as trap-derived and
  potentially stale when clear traps are missed.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15536`
  through `:15555` defines `linkDown` and `linkUp`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:254`
  through `:269` defines external alarm asserted/deasserted notifications.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:271`
  through `:299` defines CloudVision firing/resolved notifications.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4428`
  through `:4523` includes exceeded/under-threshold notification pairs.

## Use Case 7: Severity-Aware Metric Filtering And Grouping

Operator question:

- Can metrics separate critical vendor alerts from lower-priority vendor alerts?
- Can vendor-specific severity values be normalized for metric grouping or
  filtering while the raw value remains available in logs?

Concrete examples:

- Arista CloudVision firing/resolved notifications carry
  `aristaCvAlertSeverity` with values `info`, `warning`, `error`, and
  `critical`.
- A user may want only `critical` CloudVision firing notifications to feed a
  critical-alert metric, while retaining all CloudVision trap logs.

Why it matters:

- Static profile severity describes the default Netdata classification for the
  trap OID.
- Some vendors put the actual alert severity in a varbind, so the same trap OID
  can carry different operational priority.
- Metrics may need severity-aware filtering or grouping without changing the
  journal severity contract in this phase.
- This is a specialization of filtered counters and trap-derived state, not a
  separate metric kind.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:106`
  through `:113` defines `aristaCvAlertSeverity`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:271`
  through `:299` shows CloudVision firing/resolved traps carry severity.

## Use Case 8: Audit And Security Counters With Sensitive Detail In Logs

Operator question:

- How often are configuration changes, auth failures, and security violations
  happening per device or resource?
- Can sensitive or high-cardinality event detail remain searchable in logs while
  metrics stay bounded?

Concrete examples:

- Cisco configuration-change traps include terminal user and terminal type.
- Cisco port-security traps include interface, VLAN, and violating MAC address.
- SNMP authentication-failure traps indicate authentication problems, but the
  system must not expose credentials or sensitive source details as metric
  labels.

Why it matters:

- Security and audit workflows need counters and trends, but the full event
  details are often sensitive or high-cardinality.
- Metrics should answer rate and scope questions; logs should preserve the full
  event context.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:33139`
  through `:33148` shows Cisco config-change user and terminal varbinds.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:37706`
  through `:37735` shows Cisco port-security MAC/interface/VLAN detail.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:23357`
  through `:23360` defines the last secure MAC address as `MacAddress`; this is
  useful log detail, not a safe default metric label.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15557`
  defines `SNMPv2-MIB::authenticationFailure`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:337`
  through `:342` records the existing bounded-label rule.

## Use Case 9: Lifecycle And Availability Event Counters

Operator question:

- Which devices rebooted or restarted their SNMP entity?
- Can lifecycle changes be tracked per device without turning each event into a
  long-lived state metric?

Concrete examples:

- `SNMPv2-MIB::coldStart` and `SNMPv2-MIB::warmStart` count device SNMP entity
  initialization events.
- Vendor restart or management-agent restart traps can be counted the same way
  when profiles identify them.

Why it matters:

- Some traps are best treated as event history and rates, not numeric samples or
  current state.
- These counters are useful for flapping detection, incident timelines, and
  post-reboot verification.
- Routing and adjacency churn is related, but it is a separate use case because
  it needs per-peer identity.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15525`
  through `:15534` defines `coldStart` and `warmStart`.

## Use Case 10: Operator-Defined Custom Trap Semantics

Operator question:

- How can an operator add metrics for vendor, site, or product traps that
  Netdata does not ship yet?
- How can an operator define metrics for generic or user-defined trap slots whose
  semantics are site-specific?

Concrete examples:

- A vendor MIB has local user-defined event traps.
- A site uses custom enterprise traps for UPS, environmental, access-control, or
  application events.
- A stock profile decodes a trap, but the operator wants a site-specific metric
  view or threshold filter.

Why it matters:

- Stock profiles cannot predict every site-specific trap meaning.
- Operators need a supported path that survives package upgrades.
- Custom metric behavior should live with custom trap-profile knowledge, not in
  collector source code.
- This is a cross-cutting authoring requirement. It does not add a new metric
  behavior by itself; it applies to every use case in this document.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:36`
  through `:46` documents stock and operator trap profile directories.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:132`
  through `:137` documents converting custom MIBs offline and dropping YAML
  files under `/etc/netdata/go.d/snmp.trap-profiles/`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:346`
  through `:365` documents operator override files and `extends:`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/milestone-systems-a-s.yaml:17`
  shows a shipped profile containing a user-defined event trap example.

## Use Case 11: Standard Alarm Set/Clear Metrics

Operator question:

- Which standard SNMP alarms are currently set per device or resource?
- Which alarm classes, probable causes, and perceived severities are recurring?
- Can a single status-change trap OID both assert and clear a state metric based
  on a carried condition varbind?

Concrete examples:

- `SNMP-ALARM-MIB::snmpAlarmStatusChange` carries `snmpAlarmLogCond` with `set`
  or `clear` values plus an alarm identifier.
- `SNMP-ALARM-MIB::snmpItuAlarmStatusChange` carries ITU alarm class, probable
  cause, perceived severity, and additional text.
- `RMON-MIB::risingAlarm` and `RMON-MIB::fallingAlarm` carry the sampled value,
  threshold, sample type, and alarmed variable.

Why it matters:

- Some common alarm MIBs do not use separate problem and clear trap OIDs.
- Trap-to-state logic must support set/clear semantics carried by a varbind, not
  only OID pairs.
- Alarm identifiers and affected variables can be resource keys, but additional
  text must remain log detail unless explicitly bounded.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:5516`
  through `:5525` defines `snmpAlarmLogCond` set/clear values and
  `snmpAlarmLogId`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:5559`
  through `:5588` defines ITU alarm class, severity, and probable-cause fields.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:10067`
  through `:10128` defines BGP and RMON rising/falling notifications.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15563`
  through `:15590` defines SNMP-ALARM-MIB status-change notifications and their
  varbinds.
- `opennms/opennms @ 40cc8535351f09c24978771a8832cfc286b85572`
  `docs/modules/operation/pages/deep-dive/alarms/configuring-alarms.adoc:94`
  through `:134` documents problem/resolution alarms and clear keys.

## Use Case 12: Environmental, Power, And Component State

Operator question:

- Which device sensor, fan, power supply, UPS, battery, or environmental contact
  is in a problem state?
- What value did the device report for voltage, temperature, CPU, or another
  component measurement when the trap fired?
- Which problem/clear pairs are flapping?

Concrete examples:

- APC UPS traps report overload, on-battery, low-battery,
  battery-needs-replacement, fault, and corresponding clear events.
- Cisco ENVMON traps report voltage, temperature, fan, and power-supply state
  changes, with value varbinds for voltage and temperature.
- Cisco CPU threshold traps report threshold and current interval values.

Why it matters:

- Trap profiles contain many hardware and environmental notifications that are
  operationally closer to component health than generic event counts.
- Operators need per-device and, where bounded, per-component visibility.
- Numeric values carried by these traps are useful samples, while problem/clear
  pairs are useful trap-derived state.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/american-power-conversion-corp.yaml:409`
  through `:449` defines UPS overload, on-battery, and low-battery traps.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/american-power-conversion-corp.yaml:481`
  through `:611` defines low-battery clear, battery replacement, contact fault,
  fan failure, and battery-pack communication events.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/american-power-conversion-corp.yaml:660`
  through `:684` defines overload-cleared and battery-replaced traps.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:32724`
  through `:32781` defines Cisco voltage and temperature notifications with
  value and state varbinds.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:32745`
  through `:32761` defines Cisco fan and redundant power-supply notifications.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:33716`
  through `:33736` defines Cisco CPU rising/falling threshold notifications.

## Use Case 13: Routing Protocol, HA, And Adjacency State

Operator question:

- Which peer, neighbor, or redundancy group changed state?
- Which device has routing adjacency churn or HA state flapping?
- Is a routing or HA session currently up, down, degraded, or in a vendor state
  according to the last trap?

Concrete examples:

- Standard BGP established and backward-transition notifications carry the BGP
  peer address and peer state.
- Vendor OSPF neighbor state-change notifications carry neighbor identity and
  neighbor state.
- Cisco HSRP notifications carry standby group state.

Why it matters:

- Device lifecycle counters are not enough for routing and HA operations.
- The useful identity is usually device plus peer, neighbor, or group, not only
  the trap OID.
- Peer addresses and neighbor identifiers can explode cardinality in large
  networks, so profiles need explicit resource-label discipline.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:10068`
  through `:10086` defines current BGP established/backward-transition
  notifications with `bgpPeerRemoteAddr` and `bgpPeerState`.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/nbase-switch-communication.yaml:2757`
  through `:2769` defines OSPF neighbor state-change varbinds.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:33708`
  through `:33714` defines Cisco HSRP state-change varbinds.
- `librenms/librenms @ 5ffbb16324ad7c25f9588801ca9d4d52da45ea21`
  `LibreNMS/Snmptrap/Handlers/BgpBackwardTransition.php:48` through `:66`
  resolves a BGP peer and updates peer state.

## Use Case 14: Capacity, Pool, And Utilization Thresholds

Operator question:

- Which device or bounded resource crossed a capacity threshold?
- What was the reported utilization, pool count, or allocation count at the
  time?
- Did the threshold condition clear?

Concrete examples:

- Cisco DHCP free-address low/high threshold traps carry scope, current free
  address value, threshold type, and threshold value.
- Cisco CGN NAT port-usage watermark traps carry current port allocation and low
  or high threshold values.
- Cisco CPU and RMON rising/falling traps carry reported values and thresholds.

Why it matters:

- Capacity traps are often only useful when the reported numeric value and the
  configured threshold are retained as metrics.
- Many capacity conditions have a clear notification and can also drive
  trap-derived state.
- Scope names and variables need bounded-resource handling; raw names are not
  safe labels by default.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:34109`
  through `:34133` defines Cisco DHCP free-address low/high threshold traps and
  carried scope/value/threshold varbinds.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:45656`
  through `:45691` defines Cisco CGN port-usage low/high watermark and clear
  traps.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:33716`
  through `:33736` defines Cisco CPU threshold value and current interval value
  varbinds.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:10105`
  through `:10128` defines RMON rising/falling alarm value and threshold
  varbinds.

## Use Case 15: L2 Topology And Neighbor Change Counters

Operator question:

- Which devices are reporting topology or neighbor churn?
- How many neighbor rows were inserted, deleted, dropped, or aged out?
- Which switches reported spanning-tree root changes?

Concrete examples:

- LLDP remote-table change traps carry insert, delete, drop, and age-out counts.
- STP root-election traps report topology changes that should be visible per
  device.

Why it matters:

- Topology-related traps are operationally useful as counters and rates even when
  Netdata does not mutate topology from traps.
- LLDP table-change traps already carry multiple numeric counters in one
  notification.
- STP and LLDP churn can indicate loops, cabling changes, switch instability, or
  monitoring blind spots.

Evidence:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/shanghai-meridian-technologies-co-ltd.yaml:1016`
  through `:1025` defines an LLDP remote-tables-change trap with insert, delete,
  drop, and age-out counters.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/dell-inc.yaml:21623`
  through `:21632` defines a Dell STP new-root-election trap.

## Use Case 16: Receiver Pipeline Health By Source

Scope disposition:

- This use case is accepted as an important operator requirement.
- Full support is Phase B of the implementation scope.
- This trap-to-metrics design preserves source identity, trap commitment, and
  continuous diagnostics so receiver/pipeline metrics can report health per
  source without changing the trap metric identity model.

Operator question:

- Which source device or relay path is producing unknown OIDs, decode failures,
  auth failures, rate-limit drops, or INFORM response failures?
- Is a trap listener healthy for all devices, or only healthy in aggregate?
- Did relay source attribution cause device identity ambiguity?

Concrete examples:

- A single listener receives valid traps from most devices but decode failures
  from one vendor profile.
- A relay forwards traps and supplies `snmpTrapAddress.0`; misconfigured trusted
  relay settings can affect source-device identity.
- INFORM responses can fail for one source even while normal traps continue.

Why it matters:

- Pipeline health is currently job-level. That hides per-source operational
  problems behind listener totals.
- Source identity is the foundation for all per-device trap metrics.
- These metrics are system metrics, not profile-derived vendor metrics, but they
  share the same per-device requirement.

Evidence:

- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:12` through `:28` defines
  trap error counters including unknown OID, decode failure, rate-limit drops,
  auth failures, unknown engine ID, INFORM response failure, and export/write
  failures.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:241` through `:284`
  collects events, severities, and errors with only a `job_name` label.
- `src/go/plugin/go.d/collector/snmp_traps/config.go:26` through `:28` defines
  trusted relay configuration.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go:740` through `:744`
  warns that catch-all trusted relays let every peer override source identity
  via `snmpTrapAddress.0`.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go:592` through `:600`
  handles INFORM responses and increments `inform_response_failed`.

## Explicit Non-Goals

This use-case inventory does not require the future design to support:

- Dynamic override of journal `TRAP_SEVERITY` or `PRIORITY` from varbind values.
  Severity-aware metrics must not silently change the trap log severity
  contract.
- A full alarm, incident, or correlation engine. Lightweight trap-derived state
  is required; OpenNMS-style alarm lifecycle management is not.
- Polling reconciliation, interpolation, or gap filling. Trap-carried numeric
  values are arrival-time samples, and trap-derived state can be stale.
- Automatic metric generation for every decoded stock trap.
- Arbitrary string, MAC address, IP address, username, free-form text, alarm
  text, or peer identifier as a metric label by default.
- Trap-driven topology mutation. Topology traps can produce counters or samples,
  but updating Netdata topology graphs is a separate subsystem.
- Cross-device aggregation inside the collector. Aggregation belongs in query,
  dashboard, or alerting layers; the collector must preserve per-device identity.
- A general-purpose expression language as a requirement of this use-case set.
  Filtering is required, but this phase does not prescribe syntax.
- Runtime MIB compilation, a GUI wizard, or profile editing inside the Agent.
  Operators must be able to provide custom profile files, matching existing trap
  profile authoring methodology.

## Cross-Cutting Practical Constraints

- Metric identity must include source device or an approved equivalent.
- Resource-level metrics must be bounded, stable enough for charts, and
  operator-understandable.
- Missing varbinds must have explicit behavior.
- Numeric trap samples must distinguish sample value, threshold value, and
  event count.
- Trap-derived current state must explain stale-state risk.
- Set/clear state must support both separate problem/clear OIDs and same-OID
  condition varbinds.
- Metric labels must not leak sensitive values or unbounded identifiers.
- Full per-device receiver pipeline monitoring is Phase B of the implementation
  scope. It MUST reuse the identity and continuous diagnostic hooks needed by
  profile metric extraction.
- Stock-generated metrics must not create surprising high cardinality by
  default.
- Custom profiles must not require editing shipped stock files.

## Phase 1 Review Questions

External reviewers should answer only use-case coverage questions:

1. Which practical trap-to-metric use cases are missing?
2. Which missing cases are backed by common vendor MIB patterns or established
   open-source monitoring behavior?
3. Which listed cases are too broad, unsafe, or not useful enough to support?
4. Which examples should be replaced with stronger real-world examples?
5. Which use cases need explicit non-goals to avoid overbuilding?

Reviewers should not propose schema syntax in Phase 1 except when needed to
explain a missing use case.

## Phase 2 Recommended Design

### Summary

Use the existing SNMP trap profile system as the source of truth for trap metric
rules. A trap profile should be able to define:

- how a trap is decoded, classified, and rendered;
- which optional metrics can be derived from selected traps;
- how those metrics are filtered, typed, scoped, bounded, and charted.

The job configuration should define runtime policy only:

- listener, authentication, source trust, dedup, rate limit, journal, and OTLP
  behavior as today;
- whether profile-defined trap metrics are enabled;
- which profile metric rules are allowed;
- per-source and per-resource cardinality limits;
- how unresolved source-device identity is handled.

This is intentionally closer to SNMP polling profiles than Prometheus profiles.
Prometheus profiles organize an existing metric stream. Trap profiles must also
define extraction, because a trap is an event plus varbinds, not an already
emitted metric family.

### Operator Workflow

Common operator path:

1. Configure the trap listener as today.
2. Configure `source.trusted_relays` only when traps pass through relays that
   must be allowed to set `snmpTrapAddress.0`.
3. Enable profile-defined trap metrics in the job.
4. Pick `auto`, `exact`, or `combined` selection for metric rules.
5. Set cardinality limits appropriate for the trap hub.
6. Add a custom or override trap profile under
   `/etc/netdata/go.d/snmp.trap-profiles/` only when stock profiles do not
   provide the desired metric behavior.

Illustrative job configuration:

```yaml
jobs:
  - name: campus-traps
    listen:
      endpoints:
        - protocol: udp
          address: 0.0.0.0
          port: 162
    versions: [v2c, v3]
    source:
      trusted_relays:
        - 192.0.2.0/24
    profile_metrics:
      enabled: true
      mode: combined          # none | auto | exact | combined
      include:
        - IF-MIB::unexpected-link-down-events
        - CISCO-CONFIG-MAN-MIB::cli-config-change-console-events
      identity:
        device: source
        unresolved_source: source_label       # vnode if known, bounded fallback otherwise
        source_id_privacy: raw                # raw | hash
      limits:
        max_rules: 500
        max_sources: 2000
        max_resources_per_source: 512
        max_instances_per_job: 50000
        overflow: drop_and_count
```

The exact field names can change during implementation, but the responsibilities
must not:

- profile files define metric semantics;
- jobs define enablement, source identity policy, and limits;
- source-device identity is mandatory for every device-attributable trap metric.
- `identity.device: source` inside a profile rule means "use the job's
  configured source-device identity policy". It does not mean "use the raw
  sender address as a label".
- `profile_metrics:` enables profile-defined metric rules.
- The existing job-level trap `metrics:` key is not retained as a compatibility
  shim in the clean end state because SNMP traps have not shipped as a public
  contract.

Safe defaults:

- When `profile_metrics` is absent, no profile-local metric rules are evaluated.
- `profile_metrics.enabled` defaults to `false`.
- If `profile_metrics` is present but `mode` is absent, the mode is `none`.
- `identity.device` defaults to `source`.
- `identity.unresolved_source` defaults to `source_label`.
- `identity.source_id_privacy` defaults to `hash` when `source_label` is
  used. Exposing raw source addresses requires explicit operator opt-in.

Job-level `identity.device` values:

- `source`: use V2 host scope for known `SourceVnodeID`; unknown sources follow
  `identity.unresolved_source`.
- `source_label`: always use bounded `source_id` labels instead of vnode host
  scope. This is an explicit operational/debug mode and remains capped.
- `listener`: listener-owned metrics only. This is invalid for selected
  device-attributable profile rules unless those rules explicitly declare
  listener scope.

Job-level `identity.unresolved_source` values:

- `source_label`: emit device-attributable profile metrics under the receiver's
  default host scope with bounded `source_id` and `source_kind` labels.
- `drop_metric_instance`: when `identity.device: source` cannot use vnode host
  scope and would fall back to a source label, skip only the profile metric
  instance. Accepted traps are still committed to the trap log/output backend,
  and the collector increments `attribution_failed` diagnostics. Use this only
  for strict deployments that prefer missing metrics over fallback-labelled
  metrics.

### Enablement Policy

Profile-defined metrics must be safe by default.

Job modes:

- `none`: no profile-defined trap metrics are emitted.
- `auto`: enable stock rules explicitly marked `auto_safe: true`, plus
  operator-owned rules explicitly marked `auto_safe: true`.
- `exact`: enable only rule names listed in `profile_metrics.include`.
- `combined`: enable `auto` rules plus rule names listed in
  `profile_metrics.include`.
- `profile_metrics.include` is valid only with `mode: exact` or
  `mode: combined`; it is rejected with `mode: none` or `mode: auto`.

Rule defaults:

- `auto_safe` defaults to `false`.
- Stock generated rules are not auto-enabled unless a human review marks them
  `auto_safe: true`.
- Operator profiles may set `enabled: false` to disable a merged rule from an
  `extends:` chain.
- Operator profiles may set `auto_safe: true` for operator-owned rules. A rule
  that replaces an inherited rule by the same `name` is an operator-owned full
  replacement; the operator owns its `auto_safe` decision.
- `combined` is intended for sites that want the curated stock baseline plus a
  small number of exact site rules. Operators who want only explicit site rules
  should use `exact`.

Stock `auto_safe: true` criteria:

- The rule is device-scoped or explicitly listener-owned.
- The rule has no unbounded resource identity.
- The rule has bounded labels only.
- The rule has explicit lifecycle limits.
- The rule has explicit units and algorithm.
- The rule has tests or curated evidence showing it is safe on trap hubs.

`profile_metrics.include` entries select metric rule names, not trap names and
not profile filenames.

Enablement truth table:

| Rule source | `auto_safe` | `enabled` | `none` | `auto` | `exact` when included | `combined` when included |
|---|---:|---:|---:|---:|---:|---:|
| stock | `false` | unset | off | off | on | on |
| stock | `true` | unset | off | on | on | on |
| stock/operator | any | `false` | off | off | off | off |
| operator | any | unset | off | off | on | on |
| operator | `true` | unset | off | on | on | on |

`enabled: false` always wins after profile merge.

### Profile Format Extension

Trap profiles should gain a first-class optional `metrics:` section. The section
is file-local, mergeable through `extends:`, and validated together with
`varbinds:`, `traps:`, and the optional profile-local `charts:` section.

The profile format has two YAML surfaces:

- Canonical form:
  - MUST be the validation, merge, runtime, generated-stock, and source-control
    review contract.
  - MUST be emitted by the stock trap profile generator for files under the stock
    profile directory.
  - SHOULD be used in implementation tests and generated curated metric diffs.
- Compact operator authoring form:
  - MAY be used by operator profiles under
    `/etc/netdata/go.d/snmp.trap-profiles/`.
  - SHOULD be the form shown first in operator-facing documentation for simple
    and intermediate use cases.
  - MUST normalize to canonical form before `extends:` merge, validation, job
    `Check()`, chart-template compilation, or runtime evaluation.
  - MUST preserve the source file and compact field names in validation errors
    so operators see errors for fields they actually wrote.
  - MUST NOT be emitted by the stock generator. A stock-profile lint should
    reject compact-only fields in generated stock output.

Canonical shape for stock/generated profiles and the compiled form:

```yaml
metrics:
  - name: IF-MIB::unexpected-link-down-events
    type: counter
    auto_safe: true
    on_trap: IF-MIB::linkDown
    where:
      - varbind: ifAdminStatus
        equals: up
    identity:
      device: source
      resource:
        class: interface
        key_from_varbind: ifIndex
        max_per_source: 512
    output:
      metric: snmp_trap_if_unexpected_link_down_events
      dimension: events
      chart: unexpected_link_down_events

charts:
  - id: unexpected_link_down_events
    title: Unexpected Link Down Events
    family: Network/Interface/State
    context: snmp.trap.if.unexpected_link_down_events
    units: events/s
    algorithm: incremental
    lifecycle:
      max_instances: 2000
      expire_after_cycles: 60
```

Compact operator authoring shape for the same rule:

```yaml
metrics:
  - name: IF-MIB::unexpected-link-down-events
    type: counter
    auto_safe: true
    on_trap: IF-MIB::linkDown
    where:
      ifAdminStatus: up
    resource:
      class: interface
      key: ifIndex
      max: 512
    chart_meta:
      title: Unexpected Link Down Events
      family: Network/Interface/State
```

The compact rule normalizes to the canonical rule above with these defaults:

- `identity.device: source`
- `identity.resource.key_from_varbind: ifIndex`
- `identity.resource.max_per_source: 512`
- `output.metric`: deterministic `snmp_trap_*` name derived from `name`
- `output.dimension: events`
- `output.chart`: deterministic chart ID derived from `name`
- `charts.context`: deterministic `snmp.trap.*` context derived from `name`
- `charts.units: events/s`
- `charts.algorithm: incremental`
- `charts.lifecycle`: job/profile defaults unless explicitly overridden

Operators can override any derived output, chart, identity, or lifecycle field
by writing the canonical field explicitly. The implementation must expose a
validation/debug command or equivalent diagnostics that show the fully expanded
canonical form for a compact operator profile.

Canonical metric rule fields:

- `name`: stable metric-rule identity, unique after profile merge. Stock rules
  should use a MIB-qualified stable name; operator rules should use a
  site-specific prefix to avoid collisions.
- `type`: one of the supported extraction types.
- `identity`: source-device scope and optional bounded resource identity.
- `output`: emitted metric name, dimension name, and referenced chart ID.

Compact operator rules MAY omit `identity` and `output` when the normalized
values can be derived deterministically. Validation must reject compact rules
whose derived values collide with built-in metrics or other enabled
profile-local rules.

Type-specific selector fields:

- `counter` and `sample`: `on_trap` is required.
- `state` with separate problem and clear traps: `problem_trap` and
  `clear_trap` are required, and `on_trap` is invalid.
- `state` with same-OID set/clear semantics: `on_trap` is required together
  with `state.set_when` and `state.clear_when`.
- Compact operator same-OID state rules MAY use `state.varbind` together with
  `state.set` and `state.clear`. This normalizes to canonical
  `state.set_when` and `state.clear_when` predicates using `equals`, with
  `problem_value: 1` and `clear_value: 0` defaults unless explicitly overridden.
- All trap selectors resolve after the full `extends:` chain is merged, against
  the resolved profile's `traps:` section. Symbolic names and numeric OIDs must
  resolve before `Check()` returns.

Canonical chart fields:

- `id`: stable chart ID within the merged profile.
- `context`: chart context, using the `snmp.trap.*` namespace.
- `title`, `family`, `units`, and `algorithm`.
- `lifecycle` for every chart that can create per-source or per-resource
  instances.

Compact operator rules MAY define `chart_meta` inline when one metric rule maps
to one chart. `chart_meta` normalizes to a canonical `charts:` entry. Shared
charts SHOULD use an explicit `chart_id` or canonical `output.chart` reference so
the grouping is reviewable and not inferred from title text.

Optional chart fields:

- `type`: chart type; defaults to the chart-template engine default. State charts
  should use the clearest supported state/line representation available at
  implementation time.
- `description`: operator-facing chart description for generated documentation.

Rule and chart names:

- `output.metric` names should follow the existing collector metric naming style
  and must not collide with built-in `snmp_trap_*` metric names.
- `output.metric` names must not collide with built-in metric names emitted by
  the current collector, including event, severity, error, and dedup metric
  names.
- `output.dimension` must match the chart-template dimension `name` that
  selects the rule's `output.metric`.
- `charts.context` values use the chart context namespace and must start with
  `snmp.trap.`.
- Profile-local chart IDs and contexts must not collide with built-in static
  chart IDs/contexts for events, severity, errors, or dedup-suppressed metrics.
- `output.chart` must reference a chart `id` present in the fully merged profile.
- Forward references to charts defined later in the same profile or inherited
  from an `extends:` base are allowed.
- `output.chart` validation happens after full profile resolution and before
  job `Check()` returns. Errors must name the profile file and metric rule.
- `output.chart` references are limited to the same resolved `extends:` chain.
  Cross-chain chart references are validation errors.

Optional rule fields:

- `where`: bounded predicates over profile-known varbinds or static trap fields.
- `missing`: explicit behavior for missing varbinds.
- `scale`: numeric multiplier/divisor for sampled values, for example
  `scale: { multiplier: 1, divisor: 100 }`.
- `enabled`: disable or re-enable a merged stock rule from an operator profile.
- `description`: author-facing note explaining why the rule exists.

Compact operator-only aliases:

- `metric`: alias for `output.metric`.
- `dimension`: alias for `output.dimension`.
- `chart_id`: alias for `output.chart`.
- `chart_meta`: inline chart metadata that creates or completes a canonical
  `charts:` entry.
- `resource.key`: alias for `identity.resource.key_from_varbind`.
- `resource.max`: alias for `identity.resource.max_per_source`.
- `value`: alias for `value_from_varbind` on `sample` rules.
- `state.varbind` with `state.set` and `state.clear`: alias for canonical
  same-OID state `set_when` / `clear_when` predicates using `equals`.
- map-form `where`, for example `where: { ifAdminStatus: up }`, is shorthand
  for a one-element canonical predicate list with `equals`.
- map-form `where` values may use the canonical predicate operators, for example
  `where: { ccmHistoryEventTerminalType: { in: [console, terminal] } }`.

Compact aliases are not separate runtime semantics. They are accepted only at
the loader boundary and normalize to canonical fields before merge and
validation. Ambiguous compact/canonical mixtures in the same rule must fail
validation with a filename and rule name.

The profile `charts:` section is a profile-local chart-template description. The
loader compiles it into an in-memory `charttpl.Spec`; unsupported chart-template
fields are rejected during profile validation.

### Supported Metric Rule Types

The initial design must support these rule types:

- `counter`: increments a cumulative event counter when the trap and optional
  predicates match.
- `sample`: stores the last numeric value extracted from a matching trap for the
  source/resource identity. Samples are not interpolated and do not imply
  continuous polling, but active sample series must still be emitted on every
  periodic `Collect()` cycle until they expire or are cleared by lifecycle
  rules.
- `state`: stores current trap-derived state for a source or source/resource.

The design should not add a separate "multi-value" type. Multiple metric rules
may reference the same trap, and chart metadata can group their outputs into one
chart. This supports multi-value notifications without making extraction rules
harder to validate.

The following examples use canonical form. Operator-facing documentation SHOULD
show compact examples first and provide canonical expansion examples as the
advanced/reference form.

#### Counter Rule Example

```yaml
metrics:
  - name: CISCO-CONFIG-MAN-MIB::cli-config-change-console-events
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    where:
      - varbind: ccmHistoryEventTerminalType
        in: [console, terminal, virtual]
    identity:
      device: source
    output:
      metric: snmp_trap_cisco_cli_config_change_events
      dimension: changes
      chart: cisco_cli_config_changes

charts:
  - id: cisco_cli_config_changes
    title: Cisco CLI Configuration Changes
    family: Configuration/Changes
    context: snmp.trap.cisco.cli_config_changes
    units: events/s
    algorithm: incremental
    lifecycle:
      max_instances: 2000
      expire_after_cycles: 60
```

#### Numeric Sample Rule Example

```yaml
metrics:
  - name: CISCO-PROCESS-MIB::cpu-threshold-current
    type: sample
    on_trap: CISCO-PROCESS-MIB::cpmCPURisingThreshold
    value_from_varbind: cpmCPUTotalMonIntervalValue
    scale: { multiplier: 1, divisor: 1 }
    identity:
      device: source
    output:
      metric: snmp_trap_cisco_cpu_threshold_sample_percent
      dimension: current
      chart: cisco_cpu_threshold

  - name: CISCO-PROCESS-MIB::cpu-threshold-limit
    type: sample
    on_trap: CISCO-PROCESS-MIB::cpmCPURisingThreshold
    value_from_varbind: cpmCPURisingThresholdValue
    scale: { multiplier: 1, divisor: 1 }
    identity:
      device: source
    output:
      metric: snmp_trap_cisco_cpu_threshold_limit_percent
      dimension: threshold
      chart: cisco_cpu_threshold

charts:
  - id: cisco_cpu_threshold
    title: Cisco CPU Threshold Trap Values
    family: System/CPU
    context: snmp.trap.cisco.cpu_threshold
    units: percentage
    algorithm: absolute
    lifecycle:
      max_instances: 2000
      expire_after_cycles: 60
```

#### State Rule With Separate Problem/Clear OIDs

```yaml
metrics:
  - name: IF-MIB::link-down-state
    type: state
    problem_trap: IF-MIB::linkDown
    clear_trap: IF-MIB::linkUp
    identity:
      device: source
      resource:
        class: interface
        key_from_varbind: ifIndex
        max_per_source: 512
    state:
      problem_value: 1
      clear_value: 0
      ttl: 24h
      ttl_behavior: clear_and_expire
    output:
      metric: snmp_trap_if_link_down_state
      dimension: down
      chart: if_link_down_state

charts:
  - id: if_link_down_state
    title: Trap-Derived Interface Link State
    family: Network/Interface/State
    context: snmp.trap.if.link_down_state
    units: state
    algorithm: absolute
    lifecycle:
      max_instances: 2000
      expire_after_cycles: 60
```

#### State Rule With Same-OID Set/Clear Varbind

```yaml
metrics:
  - name: SNMP-ALARM-MIB::alarm-set-state
    type: state
    on_trap: SNMP-ALARM-MIB::snmpAlarmStatusChange
    identity:
      device: source
      resource:
        class: alarm
        key_from_varbind: snmpAlarmLogId
        max_per_source: 1024
    state:
      set_when:
        varbind: snmpAlarmLogCond
        equals: set
      clear_when:
        varbind: snmpAlarmLogCond
        equals: clear
      problem_value: 1
      clear_value: 0
      ttl: 24h
      ttl_behavior: clear_and_expire
    output:
      metric: snmp_trap_alarm_set_state
      dimension: set
      chart: snmp_alarm_set_state

charts:
  - id: snmp_alarm_set_state
    title: SNMP Alarm State
    family: Alarms/State
    context: snmp.trap.alarm.state
    units: state
    algorithm: absolute
    lifecycle:
      max_instances: 2000
      expire_after_cycles: 60
```

#### Compact Operator Examples

Compact examples are not a second runtime model. They show what an operator may
write in a local profile; the loader expands them to the canonical examples
above before merge and validation.

Filtered counter with bounded varbind predicate:

```yaml
metrics:
  - name: CISCO-CONFIG-MAN-MIB::cli-config-change-console-events
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    where:
      ccmHistoryEventTerminalType:
        in: [console, terminal, virtual]
    chart_meta:
      title: Cisco CLI Configuration Changes
      family: Configuration/Changes
```

Numeric samples from one trap sharing one chart:

```yaml
metrics:
  - name: CISCO-PROCESS-MIB::cpu-threshold-current
    type: sample
    on_trap: CISCO-PROCESS-MIB::cpmCPURisingThreshold
    value: cpmCPUTotalMonIntervalValue
    dimension: current
    chart_id: cisco_cpu_threshold
    chart_meta:
      title: Cisco CPU Threshold Trap Values
      family: System/CPU
      units: percentage

  - name: CISCO-PROCESS-MIB::cpu-threshold-limit
    type: sample
    on_trap: CISCO-PROCESS-MIB::cpmCPURisingThreshold
    value: cpmCPURisingThresholdValue
    dimension: threshold
    chart_id: cisco_cpu_threshold
```

Separate-OID trap-derived state:

```yaml
metrics:
  - name: IF-MIB::link-down-state
    type: state
    problem_trap: IF-MIB::linkDown
    clear_trap: IF-MIB::linkUp
    resource:
      class: interface
      key: ifIndex
      max: 512
    chart_meta:
      title: Trap-Derived Interface Link State
      family: Network/Interface/State
      units: state
```

Same-OID trap-derived state:

```yaml
metrics:
  - name: SNMP-ALARM-MIB::alarm-set-state
    type: state
    on_trap: SNMP-ALARM-MIB::snmpAlarmStatusChange
    resource:
      class: alarm
      key: snmpAlarmLogId
      max: 1024
    state:
      varbind: snmpAlarmLogCond
      set: set
      clear: clear
      ttl: 24h
    chart_meta:
      title: SNMP Alarm State
      family: Alarms/State
      units: state
```

### Identity And Scoping

Trap metrics must never aggregate all source devices into one listener total
unless the metric is explicitly listener-owned and not device-attributable.

Recommended identity model:

- Trap commitment is separate from metric attribution:
  - Accepted traps must be committed to the configured journal and/or OTLP output
    backend even when source enrichment, vnode attribution, or profile metric
    attribution is incomplete.
  - Missing `SourceVnodeID` is not a reason to lose the trap.
  - Metric attribution failures must be visible through continuous receiver-owned
    diagnostics and `TRAP_ENRICHMENT` or equivalent log evidence.
- Authoritative output semantics:
  - When both journal and OTLP outputs are enabled, journal commitment is
    authoritative. OTLP export failures are export errors and source-attributed
    OTLP errors, not terminal pipeline write failures.
  - When OTLP is the only output backend, OTLP export failures are terminal
    output write failures.
  - For one accepted trap, `pipeline.write_failed` increments at most once and
    only when the authoritative output commit fails. Backend-specific
    `source_errors` may still record more than one failure for the same trap,
    for example `journal_write_failed` and `otlp_export_failed` when both
    outputs fail.
- Known source device:
  - Use existing trap enrichment to resolve `SourceVnodeID`.
  - Emit device-attributable trap metrics through V2 host scope for that vnode.
  - Add listener/job labels only as secondary labels where needed for debugging.
- Unknown source device:
  - Use the configured `unresolved_source` policy.
  - `source_label` is the default. It emits profile metrics under the receiver's
    default host scope with bounded `source_id` and `source_kind` labels.
  - `source_id` is derived from the selected trap source identity:
    trusted `snmpTrapAddress.0` when accepted from a trusted relay, otherwise the
    UDP peer address.
  - `synthetic_vnode` is a future opt-in mode and must be rejected by the
    initial schema until it is implemented deliberately.
  - Do not merge unrelated unknown devices into one chart.
  - Do not create unlimited synthetic vnodes by default.
- Ambiguous source device:
  - Commit the trap and preserve source/enrichment evidence in the trap log.
  - Prefer the transport-selected source identity for bounded fallback metrics
    when it is available and not over cap.
  - Increment an ambiguity counter.
  - Do not create or migrate vnode-scoped profile metrics from ambiguous vnode
    enrichment.
  - Ambiguity includes conflicting registry/topology identities for the same
    trap source, explicit `conflict` or `ambiguous` enrichment status,
    `vnode_mismatch` or `ambiguous_source` reasons, rejected candidates, or an
    original-source address supplied by an untrusted relay.
- Listener-owned diagnostics required by this spec:
  - Keep listener/job scope when the error has no trustworthy source.
  - Add source identity only for errors that can be attributed to a source.
  - Emit continuously every `Collect()` cycle. Netdata receiver metrics must not
    become sparse just because no trap arrived in a cycle.

Existing built-in static charts such as trap events, severities, processing
errors, and dedup suppression are receiver/pipeline metrics. Phase B keeps
listener-wide event, severity, error, and dedup totals job-scoped so receiver
health remains visible for unattributable packets and global listener failures.
It adds separate source-attributed receiver metrics for the signals that are
safe and useful per source:

- `snmp.trap.source_pipeline`: accepted, committed, dedup-suppressed, and
  write-failed accepted traps by source.
- `snmp.trap.source_errors`: unknown OID, unresolved template, profile load,
  journal write, and OTLP export errors by source when the source is known.
- `snmp.trap.source_last_seen`: source freshness.

Phase B deliberately does not duplicate category and severity charts per source.
Per-source category/severity would multiply the default 2000-source cap by many
mostly-zero time series and duplicate the role of profile-defined semantic
metrics. Operators that need per-device vendor semantics should define profile
metric rules for those traps.

Per-device trap activity from vendor semantics is delivered by profile-defined
device-attributable rules. Receiver health and processing behavior are delivered
by built-in receiver/pipeline metrics.

Built-in receiver/pipeline metrics added in Phase B:

- `snmp.trap.pipeline`: job-level packet and write pipeline progress.
- `snmp.trap.sources`: active source identities retained for continuous
  source-attributed metrics.
- `snmp.trap.source_attribution`: job-level vnode/fallback/ambiguous/failed/
  overflow/source-transition diagnostics.
- `snmp.trap.source_pipeline`, `snmp.trap.source_errors`, and
  `snmp.trap.source_last_seen`: bounded source-attributed receiver health.

The source-attributed built-in metrics use the same source identity resolver as
profile-defined metrics: vnode host scope when `SourceVnodeID` is available and
bounded `source_id` / `source_kind` labels otherwise. Their source cap is 2000
active sources per job, with inactive source identities expiring after 60
successful collection cycles.

Fallback source identity priority:

1. Trusted `snmpTrapAddress.0` only when the UDP peer is a configured trusted
   relay.
2. UDP peer address.
3. Reverse-DNS or sysName only as display metadata, not as the stable key unless
   the operator explicitly chooses it.

Required labels for fallback source metrics:

- `job_name`
- `source_id`
- `source_kind`, for example `trusted_trap_address` or `udp_peer`
- `source_kind` is a closed label set: `vnode`, `listener`,
  `trusted_trap_address`, `udp_peer`, `entry_source`, `hostname_or_ip`,
  `trap_varbind`, `topology_ifindex`, `source`, or `other`. Unknown future
  enrichment methods map to `other`.

`identity.source_id_privacy` controls fallback source label privacy. It accepts
`raw` or `hash`; `hash` is the default when `source_label` is used.
`source_id` must be stable within the job and must respect this configured
privacy mode:

- `raw`: use the canonical source address string.
- `hash`: use a deterministic one-way hash of the canonical source address and
  job name with the agent's stable local identity as salt; expose only a
  truncated fixed-length hex value.
- Initial `hash` mode should use SHA-256, truncate to 16 hexadecimal
  characters, and canonicalize addresses without transport ports.
- Hash mode is not a security boundary. Small source-address spaces can be
  enumerated, and an agent reinstall or stable-local-identity reset changes the
  salt and therefore changes every hashed `source_id`.
- The salt source must be a persisted Agent-local identity with restart-stable
  lifetime. The implementation must document which Agent identity is used; the
  salt value itself must not be exposed as a metric label.
- Phase A reads `/etc/machine-id`, then `/var/lib/dbus/machine-id`, then the
  hostname, and finally uses a fixed last-resort string only when no stable
  local identity is available.

The design may later add an explicit opt-in mode for synthetic trap-source
vnodes, but that must be an operator decision with caps. It is not the default.

Source identity transitions:

- A source can start unknown and later become known when SNMP polling, registry,
  or topology enrichment resolves `SourceVnodeID`.
- New emissions after the transition must use the vnode host scope.
- Existing fallback source-label chart instances are not migrated; they expire
  through chart lifecycle.
- State entries created under unresolved fallback identity must not be migrated
  silently to the vnode identity. They are cleared/expired according to the
  rule's configured TTL and reload semantics.
- The transition increments `snmp_trap_profile_metrics_source_transitions` so
  chart discontinuity is explainable.
- The source-transition history is diagnostic memory and must be bounded. When
  it exceeds the configured source cap, the oldest raw route entries are
  pruned; losing old transition memory is acceptable, but unbounded growth is
  not.

Source cap behavior:

- `max_sources` caps tracked non-listener sources per job, including vnode and
  fallback sources.
- Accepted traps are still committed when source caps are full.
- New profile metric instances for over-cap sources are skipped and counted.
- Inactive sources release cap capacity when their profile metric chart
  instances expire through chart lifecycle.
- NAT, multi-homed devices, and relays that do not provide trusted original
  source identity can merge devices under one source address. Operators must use
  trusted relays or source-device enrichment to avoid that; the collector must
  not pretend it can split devices it cannot identify.

Identity to host-scope mapping:

- When `SourceVnodeID` is known, use V2 host scope with
  `ScopeKey=SourceVnodeID` and `GUID=SourceVnodeID`.
- Use the enriched device hostname as host-scope hostname when present.
- All profile metric series include `source_id` and `source_kind`, including
  vnode-scoped series. This keeps one stable chart-template identity across
  vnode and fallback source modes while V2 host scope remains the primary node
  attribution.
- When `SourceVnodeID` is absent, use the bounded fallback source identity under
  the receiver/default host scope.
- When V2 host scopes are available and a source later resolves to
  `SourceVnodeID`, new emissions for that source use vnode host scope.
- Existing fallback source-label chart instances are not migrated; they expire
  through chart lifecycle. The transition must be visible through a diagnostic
  counter or log message so chart discontinuity is explainable.

Framework evidence:

- Go V2 host scopes support multiple non-default host scopes from one collector
  job; series identity includes host scope.
- One chartengine engine is used per host scope, and explicit non-default scopes
  emit under their `metrix.HostScope` GUID and metadata.
- Therefore a trap listener job can keep receiver-owned metrics in the default
  receiver scope while routing device-attributable metrics to per-source vnodes
  when enrichment supplies them.

### Metric Continuity

Netdata does not support sparse receiver metrics. Trap-derived metric state is
updated by trap arrival and emitted by the periodic collector cycle.

Required behavior:

- Receiver-owned metrics must be emitted every `Collect()` cycle regardless of
  trap arrival in that cycle.
- Counter rules keep cumulative totals per source/resource identity and emit the
  current total every `Collect()` cycle while the identity is active.
- State rules keep the last trap-derived state and emit it every `Collect()`
  cycle until an explicit clear, TTL expiry, reload cleanup, or chart lifecycle
  expiry removes it.
- Sample rules keep the last trap-reported numeric value and emit it every
  `Collect()` cycle while the sample is fresh.
- Sample rules must have freshness semantics. The first implementation may use
  chart lifecycle expiry as the freshness boundary, but it must not emit a
  stale sample forever without an explicit profile rule or job default.
- A missing varbind with `missing: drop` does not clear an existing active
  sample or state value. It only skips the update for the received trap and
  increments the rule-miss counter.
- A missing varbind with `missing: error` skips the update and increments the
  extraction-failure counter.
- New source/resource instances are created only after a matching accepted trap,
  unless a future approved inventory-seeding design adds pre-created instances.
- Expired source/resource instances are removed. If the same identity appears
  again after expiry, the next committed trap creates a fresh series; counter
  charts may show a reset after idle expiry.
- Source/resource cap overflow must skip only the new metric instance or bucket
  according to the configured overflow behavior. It must not drop the accepted
  trap from the trap log/output backend.
- Overflow, eviction, rule-miss, extraction-failure, and attribution-failure
  diagnostics must themselves be continuous receiver-owned metrics.

### Resource Identity

Resource identity is separate from arbitrary labels.

Allowed resource keys:

- Implementation Phase A accepts only integer-like bounded varbind types for
  `identity.resource.key_from_varbind`: `INTEGER`, `Integer32`, `Unsigned32`,
  and `Gauge32`.
- Table indexes such as `ifIndex` are valid when capped per source.
- Future support for non-integer peer, neighbor, pool, scope, sensor, MAC, or
  alarm identifiers requires an approved boundedness design, explicit caps, and
  documentation before profile authors can use it.

Rejected as default metric labels:

- usernames;
- MAC addresses;
- free-form strings;
- descriptive text;
- peer IPs and source IPs as arbitrary labels;
- unbounded interface names or pool names;
- counter, time-value, string, address, payload-like, and other non-integer
  resource-key varbinds.

Each resource rule must define:

- `class`: a stock resource class (`interface`, `peer`, `neighbor`, `sensor`,
  `alarm`, `pool`, `l2_topology`, `component`) or a site-specific lowercase
  class beginning with `site_`;
- `key_from_varbind` or `key_from_enrichment`;
- `max_per_source`;
- a job-level total cap also applies across all sources;
- `missing` behavior;
- `overflow` behavior.

Cardinality contract:

| Cap | Scope | Required behavior |
|---|---|---|
| `profile_metrics.limits.max_rules` | job | Maximum enabled metric rules evaluated for the job. Validation fails when the selected `auto`/`exact`/`combined` set exceeds it. |
| `profile_metrics.limits.max_sources` | job | Maximum non-listener source identities tracked by the job, including vnode and fallback sources. |
| `profile_metrics.limits.max_resources_per_source` | job | Upper bound for resources tracked per source across all enabled rules. |
| `identity.resource.max_per_source` | rule | Rule-local upper bound for resources of that class per source. |
| `charts.lifecycle.max_instances` | chart | Upper bound for chart instances created by that chart. |
| `profile_metrics.limits.max_instances_per_job` | job | Final upper bound across all profile-derived chart instances in the job. |

Effective runtime caps use the most restrictive applicable limit. A rule cannot
create a new source, resource, or chart instance when any applicable job, rule,
or chart cap is exhausted.

These caps are collector-enforced before writing to `metrix` and before the
chart-template engine sees the sample. `charts.lifecycle.max_instances` remains
a chartengine defense-in-depth limit because chart template lifecycle caps are
best-effort for already-active instances.

`max_rules` counts the post-merge, post-mode-filter set of enabled rules that
can be evaluated by the job. Disabled rules, stock rules excluded by the selected
mode, and rules that fail validation do not count.

When the job-level instance cap has one remaining slot and multiple rules would
create new instances in the same cycle, the implementation must use a
deterministic tie-breaker, for example lexical `chart.id` then metric rule
`name`. The tie-breaker must be documented and tested.

Validation must reject impossible cap combinations before runtime. For example,
if an enabled rule declares a required safe minimum that exceeds the job or chart
cap, the job must fail validation instead of starting with a rule that can never
emit correctly.

Initial stock resource classes should be limited to:

- `interface`
- `peer`
- `neighbor`
- `sensor`
- `alarm`
- `pool`
- `l2_topology`
- `component`

Operator-defined classes are allowed only with a site-specific prefix.
The loader must validate stock classes against the stock list above and operator
classes against a documented site-prefix pattern.
Stock class validation failures are hard errors. Operator class names must match
a documented lowercase identifier pattern and include a site-specific prefix, for
example `site_foo_sensor`.

Overflow behavior in Implementation Phase A:

- `drop_and_count`: drop new source/resource instances beyond the cap and
  increment a built-in overflow counter.
- `bucket_and_count` is not exposed in Phase A. It is a possible future
  extension only after the overflow bucket identity and chart UX are designed
  and tested.
- `error` is not exposed in Phase A. Runtime cap exhaustion must not stop the
  receiver job or drop accepted traps.

Dropped raw resource keys may be logged at debug level for troubleshooting, but
they must not be promoted to labels or durable public artifacts.

### Filtering Semantics

The design should support bounded predicates, not a general expression language.

Required operators:

- `equals`
- `in`
- `exists`
- `absent`
- `greater_than`
- `less_than`
- `range`

Allowed predicate inputs:

- trap name/OID;
- static trap category/severity;
- varbind enum labels;
- booleans;
- small numeric ranges;
- explicit string values only when the referenced varbind is already approved as
  bounded.

Predicate semantics:

- Multiple predicates are ANDed.
- Each predicate must include at least one condition operator: `equals`, `in`,
  `exists`, `absent`, `greater_than`, `less_than`, or `range`.
- A missing varbind in `where` makes the predicate false and the rule does not
  match.
- `exists: true` matches when the referenced varbind is present in the received
  trap.
- `exists: false` is equivalent to `absent: true`.
- `absent: true` is the only predicate that matches a missing varbind.
- Numeric comparison operators are valid only for numeric varbind types.
- `equals` and `in` over enum-backed varbinds match enum labels. `equals` and
  `in` over numeric varbinds match numeric values.
- Numeric predicates evaluate raw decoded numeric values. `TimeTicks` predicates
  compare raw hundredths of a second; sample output conversion to seconds is
  separate.
- `range` is inclusive on both ends: `low <= value <= high`.
- Negation is expressed with `not: true` on a single predicate, not with a
  general expression language.
- `not: true` negates the predicate result only after the referenced varbind is
  known to be present. A missing varbind remains false even when `not: true` is
  set.
- `{varbind: ifAdminStatus, equals: up, not: true}` means "the varbind is
  present and its value is not `up`".
- For `range`, `not: true` means outside the inclusive range. For `in`, it means
  not in the listed set. For `greater_than` or `less_than`, it negates that
  comparison.
- To require absence, use the `absent` operator instead of `not: true` with
  `exists`.
- Pattern matching is not required for the initial design. Authors should use
  `in` over bounded values instead of regular expressions over free-form text.

Missing-varbind behavior must be explicit per rule:

- `drop`: do not emit the metric and increment a rule-miss counter;
- `zero`: emit zero only for sample rules that explicitly declare absence means
  numeric zero;
- `unknown_dimension`: allowed only for bounded dimensions and capped;
- `error`: profile validation or runtime error depending on whether the varbind
  is statically impossible or just absent from a received trap.
- `zero` is invalid for `counter` and `state` rules. State rules must use
  explicit set/clear predicates or trap pairs.
- The default missing behavior for state rules is `drop`, which leaves state
  unchanged.

Sample validation:

- `value_from_varbind` must reference a profile-known numeric varbind.
- Accepted numeric source types include `INTEGER`, `Integer32`, `Unsigned32`,
  `Gauge32`, `Counter32`, `Counter64`, and `TimeTicks`.
- `DisplayString`, `OctetString`, `MacAddress`, `IpAddress`, `OBJECT
  IDENTIFIER`, and free-form textual conventions are rejected for `sample`
  rules.
- In the initial design, `sample` rules must use the `absolute` chart algorithm.
  `Counter32` and `Counter64` trap-carried values may be sampled only as
  absolute snapshots. Derived counter rates from sporadic trap arrivals are a
  deferred feature.
- `TimeTicks` samples are converted to seconds by dividing the raw value by
  100 before profile `scale` is applied.
- Runtime ASN.1 values that do not match the profile numeric type are dropped
  and counted.
- `scale.divisor` must be greater than zero.
- The default `missing` behavior for `sample` rules is `drop`.
- Implementation Phase A applies `scale` before metric emission. Profile
  authors must set `output.metric` names, chart titles, and units so the scaled
  semantics are explicit to operators.

### Chart Generation

Profile metric rules should compile into normal go.d V2 chart templates.

Required behavior:

- Every emitted metric must have explicit units and algorithm.
- Counters use cumulative storage and `incremental` chart algorithm.
- Samples and state metrics use `absolute`.
- Multiple rules can reference one `chart.id` when they describe dimensions of
  the same operational chart.
- Chart instances must include source-device scope or bounded fallback source
  identity.
- Chart lifecycle must expire stale source/resource instances according to
  configured caps and TTLs.
- Every generated chart template must pass `charttpl.Spec` validation.
- `context` values must use the `snmp.trap.*` namespace.

Compilation path:

- Profile metric rules and charts are compiled into an in-memory
  `charttpl.Spec`.
- `ChartTemplateYAML()` serves the compiled template; the implementation should
  not write runtime-generated chart files to satisfy public requests.
- Developer-only debug dumps of the compiled spec are allowed only outside
  public request paths and must never become the source served by
  `ChartTemplateYAML()`.
- Compilation happens when the trap profile metric catalog is built or refreshed,
  not on every collection cycle.
- `profile_metrics.include` validation uses the compiled metric rule catalog.

Chart conflict rules:

- `charts:` merge by `id` within the resolved profile.
- A child chart with the same `id` in an `extends:` chain replaces the base
  chart in full.
- Across the final loaded profile set, two charts may share a `context` only if
  `title`, `family`, `units`, `algorithm`, chart type, and dimension names are
  compatible.
- Two rules that reference one chart must produce unique dimension names.
- Two rules that reference one chart must have the same label shape. A chart
  must not mix resource and non-resource rules, and a resource chart must not
  mix multiple `identity.resource.class` values.
- Built-in static chart contexts and IDs are reserved; profile-local charts must
  not reuse `events`, `severity`, `errors`, `dedup_suppressed`, or
  `profile_metric_diagnostics`, nor their effective contexts such as
  `snmp.trap.profile_metric_diagnostics`.
- Conflicts must fail profile validation before chartengine planning.

Lifecycle:

- Charts that can create per-source or per-resource instances must declare
  `lifecycle.max_instances` and `lifecycle.expire_after_cycles`.
- `expire_after_cycles` is measured in the periodic go.d `Collect()` cycle for
  the trap listener job. It is not measured by trap receive goroutines. Changing
  the listener `update_every` changes the wall-clock lifetime represented by the
  same cycle count.
- Job-level `profile_metrics.limits.max_instances_per_job` is an additional cap
  across all profile metric charts for the job.
- State TTL sweep runs at the start of each periodic `Collect()` cycle, not in a
  background timer and not during per-trap event processing.
- On state TTL expiry, `clear_and_expire` emits the clear value once, removes
  the state entry, and lets chartengine remove stale chart instances through
  normal lifecycle planning.
- State rules evaluate `where` predicates before set/clear logic. If no
  set/clear predicate matches, the rule increments a rule-miss counter and does
  not change state.
- State tables must be synchronized; race-free state updates are a required
  implementation property.
- The current collector processes trap metric updates serially per job. If a
  future implementation parallelizes trap processing within a job, state table
  access must use per-rule, per-resource, or equivalent synchronization.
- State tables are in-memory only. After Agent restart, trap-derived state is
  unknown until new traps arrive; the implementation must not claim a persisted
  clear state unless a clear trap was observed after restart.
- For state rules with `ttl_behavior: clear_and_expire`, chart
  `expire_after_cycles` must not expire the chart before the state TTL can emit
  its configured clear value.

This reuses the chart-template engine, but extraction remains a trap-profile
responsibility.

### Metric Rule Catalog And Lazy Stock Profiles

Profile-defined metric rules must be discoverable before the first matching trap
arrives.

Required behavior:

- Job `Check()` must validate every `profile_metrics.include` entry against a
  metric rule catalog.
- The metric rule catalog must include operator profiles and stock rules
  relevant to the selected mode.
- The selected rule set must respect `profile_metrics.limits.max_rules` before
  the job starts.
- Lazy stock trap decode loading must not delay metric-rule validation until
  trap arrival.
- The implementation may satisfy this by eagerly loading metric sections, by
  shipping a generated stock metric-rule catalog, or by loading selected stock
  profiles during `Check()`.
- The Phase A implementation keeps operator profiles eager. Stock profiles stay
  lazy only when the loaded profile set has no metric rules and no job enables
  `profile_metrics`. If custom profile metric rules exist, or if a job enables
  `profile_metrics`, stock profiles are loaded before rule selection so stock
  metric rules and custom operator metric rules that reference stock trap names
  validate before trap arrival.
- The chosen implementation must preserve the existing lazy decode behavior for
  jobs that do not enable profile metrics.
- The implementation must measure the metric catalog memory footprint when stock
  metric rules are introduced. If the catalog is materially larger than the
  decode catalog used today, the first implementation must load only metric
  sections needed for validation rather than forcing full stock decode loading
  for every non-metric job.

### Reload Behavior

Metric rules participate in the existing operator-profile reload lifecycle.

Required behavior:

- Operator profile reload rebuilds the merged metric and chart rule set from
  scratch.
- After reload, the job revalidates `profile_metrics.include` names.
- Removed or disabled metric rules stop emitting new samples.
- State entries for removed or disabled rules emit a clear value once when the
  rule declares `ttl_behavior: clear_and_expire`; otherwise they expire through
  chart lifecycle.
- Renamed rules are treated as remove plus add and must be visible through
  reload diagnostics.
- Stock profile metric-rule changes require a process restart or explicit cache
  release, matching the existing stock profile lifecycle.
- Reload diagnostics must name rules added, removed, disabled, renamed, and
  skipped due to validation or host-scope prerequisites.
- Job configuration changes to `profile_metrics` mode, include list, limits, or
  identity policy require a collector job restart unless a later implementation
  explicitly supports dynamic job reconfiguration.

### Runtime Ordering

Trap metric extraction must preserve current collector ordering unless a later
design explicitly changes it:

- Decode and render the trap entry.
- Apply dedup admission.
- Do not emit profile metrics for dedup-suppressed traps.
- Write the trap entry to the authoritative output backend.
- Do not emit profile metrics when the write fails.
- Then update profile-defined metrics and built-in static metrics.

Evidence:

- `src/go/plugin/go.d/collector/snmp_traps/collector.go:637` through `:642`
  returns early for dedup-suppressed traps.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go:645` through `:650`
  returns early for write failures.
- `src/go/plugin/go.d/collector/snmp_traps/collector.go:653` through `:662`
  updates profile, event, and severity metrics after successful write.
- Phase A profile metric tests verify no profile metric is emitted for write
  failures and dedup-suppressed traps. Pre-Phase-A
  `src/go/plugin/go.d/collector/snmp_traps/operator_metric_test.go:818`
  through `:831` and `:962` through `:985` covered the removed job-level
  operator metric runtime.

### Merge And Override Semantics

Trap profile merge behavior should extend the existing `extends:` model:

- `varbinds:` continue to merge by symbolic name.
- `traps:` continue to merge by trap OID.
- `metrics:` merge by stable `name` within the resolved profile.
- Metric rule names must be globally unique after profile resolution unless a
  later rule is replacing an earlier rule through the same `extends:` chain.
- Cross-profile metric name collisions outside an `extends:` replacement chain
  are validation errors. Filesystem load order must never decide which metric
  rule wins.
- Later profiles in the `extends:` chain replace earlier metric rules with the
  same `name` in full. There is no field-level merge for metric rules.
- `enabled: false` in an extending profile disables a merged metric rule without
  disabling trap decode.
- `charts:` merge by `id` within the resolved profile, with full replacement in
  an `extends:` chain.
- Multiple metric rules may reference the same trap OID.
- Operator profiles can add metrics that reference stock traps without copying
  the full stock profile.
- Abstract `_*.yaml` profiles can hold reusable metric rule blocks.

Same-filename replacement remains available for full vendor-profile replacement:
an operator file with the same filename as a stock profile replaces the stock
file in full. Normal customization should use either a small metric-only site
profile that references stock traps by MIB-qualified trap name, or `extends:`
when the operator needs to merge/override trap decode metadata too. Same-filename
replacement discards all stock decode and metric content unless the operator file
redefines it.

Operator profiles that need to add metrics for traps from several stock files
can use one metric-only site profile that references those stock traps by
MIB-qualified trap name. If the metric needs varbind validation, resource keys,
or sample extraction, the referenced stock trap definition must already define
the needed varbind metadata.

### Compatibility With Current Job-Level Metrics

The current job-level `metrics:` list is too limited for the target design.

Approved clean end state:

- SNMP traps have been merged but not released with end-user documentation, so
  the old job-level trap `metrics:` list is not treated as a public compatibility
  contract.
- The implementation should remove or rename the job-level trap `metrics:` list
  as needed for the long-term-best configuration model.
- Profile-local `metrics:` and `charts:` are the only supported trap metric
  authoring surface.
- New capabilities belong only in profile-local `metrics:`.
- Documentation must not teach the obsolete job-level `metrics:` list.
- Tests that only assert the obsolete per-job trap metric authoring path should
  be removed or rewritten to validate the profile-local model.

### Stock Profile Generation Contract

The stock trap profile generator must not create enabled metrics for every
decoded trap.

Required generator behavior:

- Generated decode knowledge remains broad.
- Generated metric rules are absent by default unless curated inputs explicitly
  request them.
- Generated stock profile YAML must emit canonical metric and chart syntax, not
  compact operator aliases.
- A stock-profile lint should reject compact-only fields in generated stock
  output, including `chart_meta`, `chart_id`, top-level `metric`, top-level
  `dimension`, `value`, compact `resource.key`, compact `resource.max`, map-form
  `where`, and compact same-OID `state.varbind` / `state.set` /
  `state.clear`.
- Candidate generated metric rules, if produced, must default to
  `auto_safe: false`.
- The generator must validate metric rules against the generated `varbinds:` and
  `traps:` sections before writing YAML.
- Human-curated stock metric rules must be reviewable in source control.
- A regenerated stock profile must preserve curated metric rules or regenerate
  them from a durable curated source, not lose them silently.
- Stock profile package size and lazy-load memory impact must be checked when
  adding curated metric rules.

Curated metric rules must have a durable source:

- For generator-owned stock profiles, the preferred durable source is a
  committed generator input such as `curated_metrics.yaml`.
- Profile-local YAML blocks maintained directly in source control are acceptable
  only for profiles that are not regenerated, or when the generator explicitly
  preserves those blocks in a tested read-modify-write path.

The durable source must record:

- metric rule name;
- referenced trap name/OID;
- referenced varbinds;
- rule type;
- chart ID;
- auto-safe status;
- cardinality evidence.

Promotion to `auto_safe: true` requires review evidence that the rule is
bounded-safe and useful by default. The generator must never promote a rule to
`auto_safe: true` automatically.

Metric rules for generated profiles are therefore a curation layer on top of MIB
decode generation, not a mechanical "one metric per trap" output.

### Validation Requirements

Profile validation must reject:

- unknown top-level profile keys after the profile schema is upgraded; before
  full profile-wide strictness ships, unknown top-level `metrics:` and `charts:`
  keys must at minimum be rejected instead of silently ignored;
- unknown fields under `metrics:`;
- unknown fields under `charts:`;
- compact operator-only aliases in stock/generated profile output;
- ambiguous compact/canonical mixtures in one rule, such as both `metric` and
  `output.metric`, both `chart_id` and `output.chart`, both `value` and
  `value_from_varbind`, or compact and canonical same-OID state predicates;
- duplicate metric names after merge;
- duplicate `output.metric` values after merge;
- duplicate or colliding derived `output.metric`, `output.dimension`,
  `output.chart`, or `charts.context` values after compact normalization;
- duplicate chart IDs after merge unless handled by the documented replacement
  rule;
- chart IDs or effective chart contexts that collide with built-in static trap
  charts;
- metric rules referencing unknown traps;
- metric rules referencing unknown varbinds;
- unsafe labels;
- resource keys without caps;
- sample rules without numeric varbind types;
- counters without explicit units/algorithm;
- state rules without set and clear semantics;
- charts with conflicting metadata for the same `chart.id`;
- stock rules that would be enabled by default with unbounded cardinality;
- `where` predicates over sensitive varbinds unless the predicate uses an
  approved bounded enum or boolean representation;
- `value_from_varbind` targeting known sensitive varbinds;
- `output.metric` values that collide with built-in metrics emitted by
  `metrics.go`;
- `output.dimension` values that do not match the chart dimension name selecting
  the rule's `output.metric`.

Reserved metric name prefixes:

- `snmp_trap_events_`
- `snmp_trap_severity_`
- `snmp_trap_errors_`
- `snmp_trap_dedup_`
- `snmp_trap_pipeline_`
- `snmp_trap_source_`
- `snmp_trap_sources_`
- `snmp_trap_metric_`
- `snmp_trap_profile_metrics_`

Profile-local rules must not recreate built-in receiver pipeline/source health.
Use profile metrics for vendor or site semantics; built-in receiver metrics
cover pipeline progress, source attribution, source errors, and source
freshness.

The first implementation step that accepts profile-local metrics must:

- add a `Metrics` field and a `Charts` field to the profile data model;
- reject or strictly validate unknown metric/chart YAML fields;
- extend `extends:` merge logic for metric rules and chart definitions;
- resolve symbolic trap names to canonical OIDs at load time;
- validate every metric rule against the resolved trap's varbind set;
- expose metric validation errors at profile load or job creation time.

The loader must not silently ignore a top-level `metrics:` or `charts:` section.
Adding profile-local metrics requires updating the profile data model and
validation in the same implementation step.

Loader migration requirements:

- Current profile loading uses a lenient YAML unmarshal path, so adding
  `metrics:` and `charts:` is not sufficient by itself.
- The implementation must add strict known-key validation for profile-local
  `metrics:` and `charts:` in the same change that adds those fields.
- Compact operator syntax must normalize to canonical form at the loader boundary
  before merge, validation, catalog selection, or runtime evaluation.
- Full top-level strictness should use an audited allowlist of documented
  profile keys so existing stock profiles are not broken accidentally.
- Validation errors must include the profile filename, the offending key or
  rule name, and whether the error came from parsing, merge, or job `Check()`.
- Validation errors for compact operator profiles must also name the compact
  field path, not only the expanded canonical path.

### Documentation And Skill Updates

The implementation must update every durable surface that currently says trap
profiles do not define metrics.

Required updates:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`
- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`
- `src/go/plugin/go.d/collector/snmp_traps/config_schema.json`
- `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml`
- generated integration documentation for the SNMP trap listener
- health alert templates under `src/health/health.d/snmp_traps.conf` when chart
  contexts, label identity, or vnode scoping changes affect alert matching or
  alert text

The SNMP trap profile authoring skill must be updated from "profiles do not
define metrics" to "trap profiles may define metric rules only through the
validated profile-local `metrics:` and `charts:` schema described here".

Operator-facing docs and skills must present compact authoring syntax first for
simple and intermediate cases, then show the canonical expansion/reference form.
Stock profile generation docs and contributor docs must state that stock and
generated profile YAML uses canonical syntax only.

### Coverage Matrix

| Use case | Required design support |
|---|---|
| Per-device trap activity | Source vnode host scope or bounded fallback `source_id`; never listener-only totals for device-attributable metrics. |
| Filtered event counters | `counter` rules with bounded `where` predicates. |
| Numeric samples | `sample` rules with `value_from_varbind`, numeric validation, units, scale, and explicit algorithm. |
| Multiple metrics from one trap | Several rules can reference the same trap and share one chart. |
| Per-resource metrics | `identity.resource` with class, key, cap, missing behavior, and overflow behavior. |
| Trap-derived current state | `state` rules with problem/clear OIDs or same-OID set/clear predicates and TTL. |
| Severity-aware metrics | Bounded predicates or labels over vendor severity varbinds; no dynamic journal severity override. |
| Audit/security counters | Counters with sensitive detail kept in logs; unsafe values rejected as labels. |
| Lifecycle counters | Device-scoped counters for restart/init traps. |
| Operator-defined custom semantics | Custom or override trap profiles under the existing operator profile directory. |
| Standard alarm set/clear | Same-OID state rules using set/clear condition varbinds and bounded alarm resource keys. |
| Environmental/power/component state | State and sample rules with source/resource scope. |
| Routing/HA adjacency state | Resource-scoped counters/state with explicit caps for peers/neighbors/groups. |
| Capacity/pool/utilization thresholds | Sample plus threshold metrics and optional clear-state rules. |
| L2 topology/neighbor counters | Counter/sample rules only; no topology mutation. |
| Receiver pipeline health | Phase B built-in receiver metrics using the same source identity policy, trap commitment rule, and continuous extraction diagnostics as profile metrics. |

### Receiver Pipeline Metrics Phase

Full receiver pipeline health coverage is Phase B of the implementation, after
profile-defined trap metrics pass the external reviewer gate.

Phase B SHOULD cover receiver-owned signals such as raw receive rate,
accepted/committed rate, drop/error stages, unknown OID/MIB gaps, SNMPv3 USM
breakdown, INFORM outcomes, dedup/throttle suppression, source cardinality, top
talkers, per-source last-seen/silence, and OS receive-buffer evidence where it
can be collected safely.

The trap-to-metrics implementation MUST preserve the common contract required by
both phases:

- source identity and vnode/fallback attribution;
- accepted trap commitment before metric attribution;
- continuous extraction diagnostics for attribution failures, ambiguity, rule
  misses, extraction failures, cap overflows, and source route transitions.

Receiver/pipeline metrics MUST NOT silently drop accepted traps when enrichment,
profile matching, source attribution, or metric extraction fails. Those failures
are metrics and log evidence, not reasons to discard the trap.

Receiver/pipeline metrics MUST emit continuously. Source-attributable instances
MUST remain bounded by explicit caps and lifecycle rules; receiver-level totals
MUST remain available for unattributable errors and global listener state.
Job-level pipeline totals MAY be greater than the sum of per-source metrics when
a packet has no trustworthy source, attribution fails, or the source cap is full.

Implementation note:

- Phase B source receiver metrics are intentionally smaller than a full
  per-source clone of receiver charts. A benchmark at the 2000-source cap still
  emits about 20k source series per collect cycle for pipeline/errors/last-seen,
  so adding per-source category/severity would be an avoidable cardinality and
  allocation cost.

### Required Tests

The implementation must add tests for:

- profile-local `metrics:` parsing and strict validation;
- profile-local `charts:` parsing and strict validation;
- compact operator syntax normalization to canonical form;
- stock/generated profile rejection or lint failure for compact-only aliases;
- compact map-form `where` normalization and canonical list-form passthrough;
- compact `chart_meta` auto-creation of canonical `charts:` entries;
- compact `chart_id` grouping for multiple rules sharing one canonical chart;
- compact `resource.key` / `resource.max` normalization to canonical resource
  identity fields;
- compact `value` normalization to `value_from_varbind`;
- compact same-OID `state.varbind` / `state.set` / `state.clear` normalization
  to canonical `set_when` / `clear_when`;
- compact/canonical ambiguous mixtures rejected with compact field paths in
  validation errors;
- derived metric name, dimension, chart ID, and context collision rejection;
- unknown top-level key rejection or targeted rejection for `metrics:` and
  `charts:` before runtime support is complete;
- `extends:` metric merge, override, disable, and duplicate-name detection;
- chart merge, chart context conflict, duplicate dimension, and `charttpl.Spec`
  validation;
- built-in static chart ID/context collision rejection;
- metric rule `output.chart` references to inherited and local chart IDs;
- rejection of cross-chain chart references;
- metric rule catalog construction for lazy stock profiles and `include`
  validation at `Check()`;
- `max_rules` counting after profile merge and mode filtering;
- multiple metrics referencing the same trap OID;
- symbolic trap name and numeric OID resolution order;
- missing-varbind behavior;
- bounded filter predicates;
- `exists` and `absent` predicate semantics;
- numeric comparison predicates and invalid numeric predicates on non-numeric
  varbinds;
- `range` predicate inclusive bounds;
- `not: true` semantics, including missing-varbind behavior and absence via
  `absent`;
- numeric sample extraction, scaling, and non-numeric rejection;
- `TimeTicks` conversion to seconds;
- rejection of `incremental` algorithm for initial `sample` rules;
- separate-OID state pairs;
- same-OID set/clear state;
- state TTL sweep and race-free state updates;
- state reset semantics after restart;
- concurrent state updates from multiple trap-processing goroutines hitting the
  same rule and resource key;
- per-source identity with known `SourceVnodeID`;
- V2 host-scope emission for known `SourceVnodeID`;
- fallback source-label emission when `SourceVnodeID` is absent;
- transition from unresolved fallback identity to known `SourceVnodeID`;
- unresolved source fallback policy, including `source_label`, hash privacy
  mode, and overflow behavior;
- accepted trap commitment when profile metric attribution fails or cap overflow
  skips new metric instances;
- receiver pipeline counters for received, decoded, accepted, committed,
  dedup-suppressed, dropped, and write-failed traps;
- source-attributed receiver metrics for accepted, committed,
  dedup-suppressed, write-failed, source-attributed errors, and last-seen age;
- vnode host-scope source receiver metrics and hashed fallback source labels;
- source receiver metric cap overflow diagnostics and source lifecycle expiry;
- continuous emission of receiver counters, profile counters, state values, and
  fresh sample values across `Collect()` cycles with no new traps;
- hash privacy stability across restarts and absence of raw source label leakage
  in hash mode;
- ambiguous source handling;
- resource caps and overflow counters;
- rule-miss counters for missing predicates and extraction failures;
- sensitive/high-cardinality label rejection;
- sensitive/high-cardinality predicate and `value_from_varbind` rejection;
- built-in metric-name collision rejection;
- generated chart templates and chartengine planning;
- collector-enforced caps before charttpl best-effort lifecycle behavior;
- V2 host-scope routing for known source devices;
- config schema updates for `profile_metrics`;
- generated metadata/integration documentation updates;
- health alert compatibility when chart identity or labels change;
- profile generator preservation of curated metric rules;
- profile reload behavior when metric rules are added, removed, disabled, or
  renamed;
- job restart behavior for `profile_metrics` configuration changes;
- dedup/write-failure runtime ordering;
- rejection or removal of the obsolete job-level trap `metrics:` authoring path;
- per-cycle overhead benchmark near configured caps, covering rule evaluation,
  hash mode, state updates, resource cap checks, and TTL sweep. The benchmark
  must report time and allocations and define an implementation-specific
  regression budget before release.

### Phase 2 Review Questions

External reviewers should answer design questions:

1. Does the design cover every use case without adding a full alarm engine or
   topology mutation system?
2. Is profile-local `metrics:` the simplest model consistent with SNMP profile
   methodology?
3. Are the identity rules strong enough to satisfy per-device metrics without
   dangerous cardinality?
4. Are any schema concepts unnecessary, ambiguous, or weaker than existing
   Netdata/SNMP profile patterns?
5. What unwanted side effects could this design create in loader behavior,
   generated stock profiles, chart templates, docs, or backward compatibility?
