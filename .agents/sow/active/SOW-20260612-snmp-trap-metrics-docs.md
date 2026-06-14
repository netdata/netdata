# SOW-20260612-snmp-trap-metrics-docs - SNMP Trap Metrics And Documentation

## Status

Status: completed

Sub-state: Phase 1 use-case inventory and Phase 2 recommended design are written
in the committed-spec draft. Four external design review rounds have been
integrated. A follow-up UX review round has been run and the user approved the
recommended authoring model: operator docs use compact syntax; the loader expands
to canonical form; stock/generated profiles use canonical form for reviewability.
The user approved the revised source identity and metric continuity model:
accepted traps are committed independently from metric attribution, vnode
attribution is enrichment, and unresolved sources use bounded fallback metric
identity. The user approved the long-term-best compatibility decision: SNMP
traps have been merged but have not shipped as a public documented contract, so
the old job-level trap `metrics:` list can be removed or renamed instead of
retained as a deprecated compatibility shim.
The user initially confirmed that full receiver pipeline monitoring was a
separate follow-up product phase, then expanded this SOW on 2026-06-13: implement
profile-defined trap metrics first, run the six requested external reviewers,
then implement receiver/pipeline metrics and run the same reviewer gate again.

## Requirements

### Purpose

Make SNMP trap metrics fit for operator use:

- Operators MUST be able to distinguish which device generated trap activity.
- Operator-defined trap metrics MUST have clear, documented semantics and bounded cardinality.
- End-user documentation MUST explain SNMP traps functionality, configuration, metrics, logs, enrichment, and limitations accurately.
- Profile-defined trap metrics MUST be production-ready for users in this SOW:
  documented, example-driven, schema-visible, and tested across the supported
  rule types, identity modes, cardinality controls, and failure paths before
  pipeline metrics work starts.
- The urgent counter-algorithm fix is handled separately by PR #22693; this SOW covers the broader metrics extraction and documentation closeout.
- Full receiver pipeline monitoring is now the second implementation phase in
  this SOW. It must reuse the same source identity, trap-commitment, cardinality,
  and continuous-emission contracts as profile-defined trap metrics.

### User Request

The user requested a separate worktree with an SOW to properly analyze and fix metrics extraction from traps and add end-user documentation about SNMP traps functionality in general.

Related immediate fix:

- PR #22693: `Fix SNMP trap counter chart algorithms`

### Assistant Understanding

Facts:

- Current SNMP trap metric counters are keyed by trap listener job only.
- Current static and operator-created chart instances use `job_name` as the instance label.
- Current operator metrics count trap events for configured OIDs; they do not extract numeric metric values from trap varbinds.
- Current operator metrics support one optional dynamic dimension from a bounded varbind value.
- Current shipped docs describe metrics as per-job.
- The design spec describes richer per-device and vnode behavior as a target, not as the current implementation.

Inferences:

- The current per-job metric identity is insufficient for operators running one listener for many devices.
- Fixing metric identity may touch chart templates, in-memory counters, collector labels, metadata, health, docs, and tests.
- Adding value extraction or filter conditions for trap metrics would be a product contract expansion, not a bug fix.

Unknowns:

- Whether the clean target should be per listener plus source device, per source device only, or host/vnode-scoped per device.
- How unknown or unenriched devices should be identified in metric labels without causing high cardinality or misleading merges.
- Whether operator-defined metrics should remain event counters only, or support configured filters and/or numeric value extraction from trap varbinds.
- Whether this branch should wait until PR #22693 is merged, or rebase/merge that fix before implementation if it is still open.

### Acceptance Criteria

- SNMP trap metric identity is explicitly designed, implemented, tested, and documented.
- Operators can distinguish trap metric activity by source device or an approved equivalent identity.
- Static and operator-created trap metrics have explicit, documented chart instance logic.
- Operator metric extraction semantics are documented and match the implementation.
- Receiver/pipeline metrics expose listener health by attributable source where
  possible, without losing receiver-level totals for errors that have no
  trustworthy source.
- End-user documentation covers SNMP trap setup, trap profile behavior, logs, metrics, labels, enrichment, dedup, OTLP/direct journal output, and known limitations.
- Metadata/generated integration docs are updated consistently with source docs and code.
- Tests cover chart identity, operator metric extraction, algorithm declarations, cardinality boundaries, and regression cases for multi-device trap activity.
- Profile-metrics tests cover the public operator contract, including rule
  selection modes, compact and canonical profile syntax, counters, samples,
  state rules, predicates, scaling, vnode/fallback/listener identity, source and
  resource caps, missing values, overflow diagnostics, failed writes, and docs
  examples where practical.
- Profile-metrics documentation gives operators enough information to author and
  enable custom rules without reading Go code: job config, profile YAML syntax,
  examples, identity semantics, limits, troubleshooting, and known lossy-trap
  behavior.
- Durable knowledge is transferred to specs, project skills, docs, code, and tests before this SOW is completed and deleted before merge.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/snmp_traps/metrics.go`
- `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go`
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml`
- `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml`
- `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`
- `.agents/sow/specs/snmp-traps/netdata.md`
- `src/go/plugin/go.d/collector/prometheus/`
- `src/go/plugin/go.d/collector/prometheus/promprofiles/`
- `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/haproxy.yaml`
- `src/go/plugin/framework/charttpl/README.md`
- `src/go/plugin/framework/chartengine/README.md`
- `.agents/sow/specs/go-v2-host-scope.md`

Current state:

- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:57` defines `trapMetrics.jobs map[string]*perJobMetrics`.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:73` creates/reuses metrics by `jobName` only.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:241` emits event counters with only `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:254` emits severity counters with only `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:267` emits error counters with only `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:287` emits dedup counters with only `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:116` defines operator metrics without source-device state.
- `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:380` collects operator metrics with only `job_name`, plus optional `varbind_value`.
- `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:406` builds operator chart templates with `instances.by_labels: [job_name]`.
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:46` makes `snmp.trap.events` instances by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:70` makes `snmp.trap.severity` instances by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:94` makes `snmp.trap.errors` instances by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:132` makes `snmp.trap.dedup_suppressed` instances by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml:516` documents metrics as grouped per job and scoped by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:519` documents metrics as grouped per job and scoped by `job_name`.
- `.agents/sow/specs/snmp-traps/netdata.md:1052` says the current implementation emits `job_name` only, while richer per-device labels are the target contract.
- `.agents/sow/specs/snmp-traps/netdata.md:1058` says `snmp.trap.events` target instance is per source device.
- `.agents/sow/specs/snmp-traps/netdata.md:1062` says target node/vnode behavior should inherit per-device vnode identity from SNMP polling, with hub fallback when polling is unavailable.

Risks:

- Changing chart instance identity can create new chart instances and affect dashboards, alerts, saved views, and downstream consumers.
- Per-device metrics can create high cardinality on large trap hubs if not bounded and intentionally scoped.
- Using source IP alone can misidentify devices behind NAT or when devices change addresses.
- Using hostname/sysName can merge unrelated devices if names collide or enrichment changes.
- Host/vnode scoping is likely the best long-term operator model, but it may touch framework and topology contracts beyond the narrow metrics fix.
- Adding value extraction from varbinds can turn traps into arbitrary metrics and requires stricter validation, type handling, unit semantics, and cardinality rules.

## Pre-Implementation Gate

Status: in-progress

Problem / root-cause model:

- Current trap metrics are listener-level counters, not device-level counters.
- A listener job can receive traps from many devices, so one `job_name` instance merges all source devices into one chart instance.
- This prevents operators from identifying which device generated the observed trap rate.
- The implementation and generated docs agree on current behavior, but the design spec contains an unresolved target for richer per-device/vnode identity.

Evidence reviewed:

- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:57` stores static metrics in a map keyed by job.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:73` retrieves metrics with `getJobMetrics(jobName)`.
- `src/go/plugin/go.d/collector/snmp_traps/metrics.go:241` collects event metrics with `job_name` only.
- `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:380` collects operator-created metrics with `job_name` only.
- `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:419` creates operator chart instances by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/charts.yaml:46` creates static chart instances by `job_name`.
- `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml:516` documents per-job metrics.
- `.agents/sow/specs/snmp-traps/netdata.md:1058` specifies per-device `snmp.trap.events` target instances.
- `.agents/sow/specs/snmp-traps/netdata.md:1062` specifies per-device vnode inheritance with hub fallback.

Affected contracts and surfaces:

- Go collector runtime metrics and labels.
- V2 chart templates and dynamic chart generation.
- Metric context, instance, dimension, and label contracts.
- Operator `metrics:` configuration semantics.
- Cardinality and memory behavior for trap hubs.
- Health alerts under `src/health/health.d/snmp_traps.conf`.
- Collector metadata and generated integration documentation.
- End-user documentation and operator guidance.
- Project SNMP trap specs and runtime project skills.
- Tests and benchmarks for trap pipeline, metrics, operator metrics, and chart templates.

Clean-end-state target:

- SNMP trap metrics have explicit, intentional identity semantics that let operators distinguish source devices.
- Static trap metrics and operator-created trap metrics use the same approved identity model unless a specific metric is intentionally scoped differently.
- All trap metrics declare explicit chart algorithms after PR #22693 is in the branch history.
- Operator metric extraction semantics are deliberately expanded through
  profile-local `metrics:` and `charts:` rules with documented filters, state,
  sample extraction, and tests.
- Built-in receiver/pipeline metrics are implemented after profile metrics and
  use the same source identity, trap commitment, cardinality, and continuous
  emission rules.
- Documentation and metadata match the implementation exactly.
- Specs and runtime project skills contain the lessons needed to prevent recurrence.
- Removed as redundant (i): obsolete job-level trap `metrics:` authoring schema
  and docs; obsolete per-job-only operator metric tests after replacement;
  obsolete shim/collision tests tied only to retaining job-level `metrics:`;
  duplicate docs generated from stale metadata.
- Excluded coupled items (ii): synthetic vnode creation remains excluded and
  must be rejected until a future approved design. Full receiver pipeline
  monitoring is no longer excluded after the user's 2026-06-13 scope expansion.
- Reference search: required before implementation because the metric identity contract may change. Initial search used:
  - `rg -n "snmp\\.trap\\.events|per device|vnode|job_name|instances\\.by_labels|dimension_from_varbind|operator metrics|trapMetrics|perJobMetrics|buildChartTemplateYAMLOperatorMetric" .agents/sow/specs/snmp-traps src/go/plugin/go.d/collector/snmp_traps -g '*.md' -g '*.go' -g '*.yaml'`
  - Surviving references are mapped in this SOW as current evidence. A broader repository-wide search is required once the selected target is known.
- Selected-target reference search after final compatibility approval used:
  - `rg -n 'job-level metrics|job-level trap metrics|legacy shim|compatibility shim|deprecated per-job|retained as a deprecated|Implementation remains blocked|needs-user-decision|Pending user decisions|reserved legacy shim|profile_metrics|snmp_traps.*metrics|OperatorMetric|operator_metric|dimension_from_varbind|perJobMetrics|trapMetrics|getJobMetrics|buildChartTemplateYAMLOperatorMetric|snmp\\.trap\\.events|instances\\.by_labels|job_name' .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md .agents/sow/specs/snmp-traps/trap-metrics-profiles.md src/go/plugin/go.d/collector/snmp_traps src/go/plugin/go.d/config/go.d/snmp.trap-profiles src/health/health.d/snmp_traps.conf -g '*.md' -g '*.go' -g '*.yaml' -g '*.yml' -g '*.json' -g '*.conf'`
  - Implementation surfaces found: `config.go`, `operator_metric.go`,
    `metrics.go`, `charts.yaml`, `config_schema.json`, `metadata.yaml`,
    `health.d/snmp_traps.conf`, SNMP trap profile format docs, operator metric
    tests, pipeline/dedup/reload/listener/OTLP tests, and the active SOW/spec.
  - The current listener-owned static metrics are part of Phase 2 and must be
    either migrated or intentionally preserved with documented receiver-level
    scope where no trustworthy source attribution exists.

Existing patterns to reuse:

- Existing SNMP trap enrichment already resolves `SourceVnodeID` for logs and OTLP.
- Existing dedup logic already prefers vnode identity over source IP for fingerprints.
- Existing operator metric cardinality guard accepts only bounded `dimension_from_varbind` values.
- Existing chart template patterns use `instances.by_labels`.
- Existing generated integration docs are sourced from collector metadata.
- Existing SOW specs contain SNMP trap design and stress-test analysis that must be reconciled with implementation.

Risk and blast radius:

- Regression: dashboards or alerts expecting per-job metrics may change instance layout.
- Compatibility: shipped context names should not be renamed without explicit approval.
- Performance: per-device counters need lifecycle management to avoid unbounded maps on high-cardinality trap storms.
- Security/privacy: metric labels must not expose secrets, credentials, SNMP communities, or sensitive varbind values.
- Operational: unknown devices need a stable fallback identity that is useful without merging unrelated devices.
- Documentation: generated and source docs can drift if metadata is not the source of truth.

Sensitive data handling plan:

- Do not write raw SNMP communities, auth/privacy credentials, bearer tokens, customer names, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident details to durable artifacts.
- Use placeholders such as `[REDACTED_SECRET]`, `[PRIVATE_ENDPOINT]`, and documentation-only example ranges.
- Tests must use synthetic/private example addresses and names.
- SOW, specs, docs, skills, and code comments must contain only sanitized evidence.

Implementation plan:

1. Implement profile-defined trap metrics.
   - Scope: profile-local `metrics:` and `charts:`, source/vnode/fallback
     identity, bounded resources, filters, numeric samples, state pairs,
     diagnostics, chart templates, tests, metadata, docs, specs, and skills.
2. Run the profile-metrics reviewer gate.
   - Scope: `glm`, `kimi`, `mimo`, `minimax`, `deepseek`, and `qwen` must
     review the whole SOW and implementation read-only for correctness, smells,
     unwanted side effects, security, and long-term-best fit.
   - Any real findings must be fixed, and the same-scope reviewer gate rerun
     until reviewers cannot identify blocking issues.
3. Implement built-in receiver/pipeline metrics.
   - Scope: raw receive/accepted/committed rates, decode/profile/unknown-OID
     errors, SNMPv3/USM/auth classes where available, INFORM response outcomes,
     dedup/rate-limit/export/write failures, source attribution/enrichment
     outcomes, source cardinality, last-seen/silence where feasible, and
     receiver-level totals for unattributable errors.
4. Run the pipeline-metrics reviewer gate.
   - Scope: same reviewers, same read-only review protocol, same requirement to
     fix real findings and rerun with unchanged scope until no blocking issues
     remain.
5. Update end-user documentation and generated metadata/integration docs.
   - Scope: configuration, examples, logs, metrics, enrichment, dedup, OTLP/direct journal, troubleshooting.
6. Update specs and runtime project skills with durable lessons.
7. Validate with targeted unit tests, generation checks, taxonomy/integration checks, same-failure searches, SOW audit, and sensitive-data scans.

Validation plan:

- Run targeted Go tests for `src/go/plugin/go.d/collector/snmp_traps/...`.
- Add regression tests for multiple devices sending traps to one listener.
- Add tests for operator metric chart identity and bounded varbind dimensions.
- Add tests for unknown-device fallback behavior.
- Add tests proving all trap charts declare explicit algorithms after PR #22693 is present.
- Run integration metadata generation/check commands required by the collector workflow.
- Run taxonomy checks if contexts/metadata change.
- Run SOW audit and sensitive-data scan before completing the SOW.
- Run repository-wide same-failure searches for stale `job_name`-only assumptions after implementation.

Artifact impact plan:

- AGENTS.md: likely unaffected unless project-wide SOW or collector rules are discovered stale.
- Runtime project skills: expected update to SNMP trap or Go collector skill if the work exposes durable authoring rules.
- Specs: expected update under `.agents/sow/specs/snmp-traps/` to reconcile target vs implementation.
- End-user/operator docs: expected update to collector metadata and generated integration docs; additional docs may be required after locating the canonical SNMP traps docs surface.
- End-user/operator skills: expected update if SNMP trap querying/authoring skills need metric semantics.
- SOW lifecycle: branch-local working file; durable knowledge targets are specs/skills/docs/code/tests; delete before merge; any regression or deferred work must become a linked SOW or GitHub issue.

Open-source reference evidence:

- RFC 3416 says SNMPv2 Trap-PDU and InformRequest-PDU varbind lists start with
  `sysUpTime.0` and `snmpTrapOID.0`, then copy the notification OBJECTS clause
  variables, which makes per-trap varbind extraction a first-class protocol
  pattern.
- RFC 2863 defines `IF-MIB::linkDown` / `linkUp` with `ifIndex`,
  `ifAdminStatus`, and `ifOperStatus` varbinds.
- Arista's `ARISTA-HARDWARE-UTILIZATION-MIB` defines
  `aristaHardwareUtilizationAlert` with `aristaHardwareUtilizationInUseEntries`,
  `aristaHardwareUtilizationHighWatermark`, and
  `aristaHardwareUtilizationHighWatermarkTime`.
- `librenms/librenms @ 5ffbb16324ad7c25f9588801ca9d4d52da45ea21`
  `LibreNMS/Snmptrap/Handlers/LinkDown.php:48` extracts `IF-MIB::ifIndex`,
  `:50` resolves the port, and `:58-74` updates interface state and logs
  interface-scoped events.
- `librenms/librenms @ 5ffbb16324ad7c25f9588801ca9d4d52da45ea21`
  `LibreNMS/Snmptrap/Handlers/LinkUp.php:48-71` performs the inverse
  interface-scoped state update.
- `librenms/librenms @ 5ffbb16324ad7c25f9588801ca9d4d52da45ea21`
  `LibreNMS/Snmptrap/Handlers/CiscoCCMCLIRunningConfigChanged.php:46-49`
  extracts Cisco config-change varbinds including terminal type.
- `OpenNMS/opennms @ 40cc8535351f09c24978771a8832cfc286b85572`
  `docs/modules/operation/pages/deep-dive/alarms/configuring-alarms.adoc:58-63`
  documents reduction keys as the alarm signature mechanism.
- `OpenNMS/opennms @ 40cc8535351f09c24978771a8832cfc286b85572`
  `docs/modules/operation/pages/deep-dive/alarms/configuring-alarms.adoc:108-134`
  documents clear-key pairwise correlation for resolving events such as
  `interfaceUp` clearing `interfaceDown`.

Metric extraction use-case candidates:

1. Filtered event counters from bounded varbind predicates.
   - Current limitation: `MetricConfig` has only `oid`, `context`, and
     `dimension_from_varbind`; the collector increments when the trap OID
     matches and has no predicate language.
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/config.go:72` defines only the
       three current metric fields.
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:196`
       selects the operator metric by trap OID and `:201-218` increments the
       counter.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15536`
       defines `IF-MIB::linkDown` with `ifIndex`, `ifAdminStatus`, and
       `ifOperStatus`.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:2209`
       defines `ifAdminStatus` as an enum.
   - Concrete unsupported examples:
     - Count unexpected `IF-MIB::linkDown` only when `ifAdminStatus=up`; do not
       count planned administrative shutdowns the same way.
     - Count Cisco CLI config changes only when
       `ccmHistoryEventTerminalType in [console, terminal, virtual]`, while
       keeping `ccmHistoryEventTerminalUser` in logs only.
   - Required capability: a bounded `where` / `match` predicate over symbolic
     varbind names, with enum/boolean/small-range validation and explicit
     missing-varbind behavior.

2. Numeric value extraction from trap varbinds.
   - Current limitation: all operator-created metrics are counters with
     `events/s` units; trap-carried values are not observed as metric values.
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:388` emits
       `ObserveTotal(singleCount)` for metrics without a varbind dimension.
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:398` emits
       per-dimension counter totals.
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:416` hardcodes
       operator chart units to `events/s`.
   - Concrete unsupported examples:
     - Arista hardware utilization trap:
       `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:245`
       carries `aristaHardwareUtilizationInUseEntries` and
       `aristaHardwareUtilizationHighWatermark`.
     - Juniper IDP CPU/memory traps:
       `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4542`
       carries `jnxIdpSensorCpuUsage` and `jnxIdpSensorCpuThreshold`; `:4559`
       carries `jnxIdpSensorMemUsage` and `jnxIdpSensorMemThreshold`.
     - Juniper DFC packet-rate traps:
       `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4429`
       carries `jnxDfcInputPktRate` plus soft watermarks.
   - Required capability: `value_from_varbind` with declared units, algorithm
     (`absolute` vs `incremental`/counter), numeric type validation, scale, and
     missing/non-numeric behavior. Documentation must state these are last
     trap-reported values held by the collector, not continuously polled values.

3. Multi-value extraction from one notification.
   - Current limitation: `validateMetrics` rejects duplicate configured OIDs,
     so one trap OID cannot emit several independent metric values.
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:27` starts
       duplicate-OID tracking.
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:39` rejects
       duplicate metric OIDs.
   - Concrete unsupported examples:
     - Arista CLB flow threshold traps carry allocated, unallocated, learned,
       and threshold/limit fields in the same notification:
       `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:300`
       through `:347`.
     - Juniper DFC hard memory threshold traps carry usage, low/high watermarks,
       and criteria in one notification:
       `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:4497`
       through `:4523`.
   - Required capability: either allow multiple metric entries per trap OID
     when their value selectors/contexts differ, or support one metric config
     with multiple named extracted values.

4. Resource identity extraction with bounded operational shape.
   - Current limitation: `dimension_from_varbind` only accepts enum-backed,
     boolean, or small-range varbinds; interface names, MAC addresses,
     usernames, large indexes, and source IPs are intentionally rejected for
     metric cardinality. This protects the system but prevents per-resource
     trap metrics.
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:109`
       rejects unbounded `dimension_from_varbind` values.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:2224`
       defines `ifIndex` as `InterfaceIndex` with range `1..2147483647`.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/ciscosystems.yaml:23357`
       defines `cpsIfSecureLastMacAddress` as `MacAddress`, which must stay
       log-only.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:337`
       allows high-cardinality values in descriptions/varbind logs and `:341`
       restricts labels to bounded cardinality only.
   - Concrete unsupported examples:
     - Count `linkDown` / `linkUp` per device interface using `ifIndex` or
       enriched `TRAP_INTERFACE`, without exposing raw interface names as an
       unbounded global label.
     - Count Cisco port-security violations per interface/VLAN while keeping
       the violating MAC address in logs, not in metric labels.
     - Count Juniper DFC packet-rate threshold events per interface name with a
       selector/cap, not unbounded global dimensions.
   - Required capability: a resource identity model separate from arbitrary
     metric labels, probably tied to the approved trap metric identity decision:
     source device/vnode plus selected resource key, with selectors, caps,
     lifecycle/obsoletion rules, and clear cardinality documentation.

5. Stateful set/clear or firing/resolved metrics.
   - Current limitation: operator metrics are event counters only; there is no
     state table that maps one trap to `1` and another trap to `0` for the same
     resource key.
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:122`
       through `:124` store `singleCount` / `dimCounts`, not current state.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:15536`
       and `:15546` define `linkDown` / `linkUp` as separate trap OIDs.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:254`
       and `:262` define external alarm asserted/deasserted notifications.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:271`
       and `:285` define CloudVision alert firing/resolved notifications.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/juniper-networks-inc.yaml:3147`
       and `:3155` define exceeded/abated memory threshold pairs.
   - Concrete unsupported examples:
     - `linkDown` sets an interface trap-state metric to `1`; matching
       `linkUp` clears it to `0`.
     - Arista external alarm asserted sets active external alarm count/state;
       deasserted clears it for the same alarm key.
     - Juniper threshold exceeded/restored pairs expose current threshold state
       while event counters still preserve flap rate.
   - Required capability: explicit problem/clear OID pairing, a stable
     resource key, TTL/reconciliation policy for missed clear traps, and docs
     warning that trap-derived state is lossy unless reconciled with polling.

6. Dynamic severity / priority extraction from vendor severity varbinds.
   - Current limitation: profile severity is static per trap entry. Operator
     metrics can split by bounded severity-like varbinds, but cannot use them
     to override `TRAP_SEVERITY`, filter "critical only", or emit normalized
     severity-state metrics without extra logic.
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/config.go:61` through `:65`
       allows static per-OID severity override only.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:106`
       defines `aristaCvAlertSeverity` as `info`, `warning`, `error`, and
       `critical`.
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/arista-networks-inc-formerly-arastra-inc.yaml:271`
       defines CloudVision firing notifications with static Netdata severity
       `warning` while carrying the vendor severity varbind.
   - Concrete unsupported examples:
     - Count only CloudVision alerts whose carried severity is `critical`.
     - Normalize vendor severity values into Netdata's closed 8-severity
       taxonomy for metrics/alerts while retaining raw vendor severity in logs.
   - Required capability: either predicates over severity-like varbinds for
     metrics only, or a larger dynamic severity override contract. The latter is
     broader than metric extraction and requires a separate user decision.

Prometheus profile pattern analysis after rebase:

- Rebase baseline: branch rebased cleanly onto `upstream/master` at
  `b6256b7a4f` before inspecting Prometheus profile support.
- What Prometheus profiles are:
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/profile.go:14`
    through `:26` defines a profile as `match`, optional `app`, and a
    standard `charttpl.Group` template.
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/profile.go:45`
    through `:50` validates the profile template by wrapping it in a
    `charttpl.Spec`.
  - `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/haproxy.yaml:1`
    through `:10` states the profile is template-only and selects HAProxy by
    `match: 'haproxy_*'`.
- How Prometheus loads profiles:
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/catalog.go:22`
    names the profile directory `prometheus.profiles`.
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/catalog.go:61`
    through `:64` documents basename identity, user override behavior, and
    fatal stock-profile errors versus skipped invalid user profiles.
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/catalog.go:183`
    through `:199` strictly decodes YAML, sets profile identity from filename,
    and validates the loaded profile.
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/catalog.go:216`
    through `:242` builds search dirs from installed stock profiles plus user
    collector config dirs.
  - `src/go/plugin/go.d/collector/prometheus/promprofiles/default_catalog.go:19`
    through `:42` caches the loaded catalog process-wide outside tests.
- How operators select profiles:
  - `src/go/plugin/go.d/collector/prometheus/config.go:51` through `:61`
    defines `none`, `auto`, `exact`, and `combined`.
  - `src/go/plugin/go.d/collector/prometheus/config.go:103` through `:131`
    validates exact/combined entries as non-empty, unique profile basenames.
  - `src/go/plugin/go.d/collector/prometheus/config_schema.json:183`
    through `:199` exposes these modes in the job config schema.
  - `src/go/plugin/go.d/collector/prometheus/README.md:185` through `:192`
    documents that uncovered metrics keep generic autogen charts.
- Runtime behavior:
  - `src/go/plugin/go.d/collector/prometheus/runtime.go:23` through `:44`
    selects profiles against scraped metric families during `Check()` and
    caches the merged chart template.
  - `src/go/plugin/go.d/collector/prometheus/runtime.go:73` through `:107`
    implements mode-based selection.
  - `src/go/plugin/go.d/collector/prometheus/runtime.go:173` through `:185`
    matches profile `match` patterns against scraped family base names.
  - `src/go/plugin/go.d/collector/prometheus/chart_template.go:24`
    through `:40` builds a base autogen template.
  - `src/go/plugin/go.d/collector/prometheus/chart_template.go:48`
    through `:68` appends selected profile chart groups while keeping autogen
    as fallback.
  - `src/go/plugin/go.d/collector/prometheus/collector.go:124`
    through `:128` serves the cached runtime chart template through
    `ChartTemplateYAML()`.
  - `src/go/plugin/go.d/collector/prometheus/writer.go:117` through `:145`
    writes scraped metric families into `metrix` separately from profiles.
- Chart-template contract:
  - `src/go/plugin/framework/charttpl/README.md:5` through `:16` defines chart
    templates as the mechanism for organizing already-emitted metrics into
    charts, dimensions, labels, and per-instance charts.
  - `src/go/plugin/framework/charttpl/README.md:22` through `:30` says the
    engine matches incoming metrics each cycle and creates chart instances from
    instance identity labels.
  - `src/go/plugin/framework/chartengine/README.md:19` through `:28` states
    V2 collectors provide `MetricStore()` and `ChartTemplateYAML()`, while
    `Collect()` writes metrics.
  - `src/go/plugin/framework/chartengine/README.md:98` through `:112`
    documents the plan lifecycle, including expiry/removal actions.
- Tests worth copying:
  - `src/go/plugin/go.d/collector/prometheus/runtime_test.go:110` through
    `:218` covers profile auto/named/combined selection.
  - `src/go/plugin/go.d/collector/prometheus/collector_test.go:607` through
    `:720` verifies full HAProxy profile selection and chart coverage through
    the collector runtime.
  - `src/go/plugin/go.d/collector/prometheus/collector_test.go:722` through
    `:795` feeds a broad HAProxy scrape through the merged template and checks
    chart-ID collision safety.

Applicability to SNMP trap metrics:

- Reusable patterns:
  - Use a stock/user profile catalog with basename identity, strict YAML, user
    overrides, fatal stock errors, skipped invalid user profiles, and
    process-wide cache.
  - Reuse the `none` / `auto` / `exact` / `combined` operator selection model,
    but choose the default only after cardinality risk is evaluated for traps.
  - Reuse `charttpl.Group` / `charttpl.Spec` validation for chart layout.
  - Keep generated chart templates served through `ChartTemplateYAML()` and
    verify them with chartengine planning tests, not only YAML-schema tests.
- Not reusable directly:
  - Prometheus profiles shape an existing metric stream. They do not decide
    which events become metrics.
  - SNMP traps need an extraction layer before charting: event filters,
    numeric `value_from_varbind`, multiple extracted values per trap, state
    pair handling, resource identity, and per-device routing.
  - Current SNMP trap metric config is only `oid`, `context`, and optional
    `dimension_from_varbind` (`src/go/plugin/go.d/collector/snmp_traps/config.go:72`
    through `:76`).
  - Current operator metric collection emits counters with `job_name` and
    optional `varbind_value` labels (`src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:380`
    through `:403`), and current chart generation instances by `job_name`
    (`src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:406` through
    `:437`).
  - SNMP trap decode profiles MUST NOT grow a `metric:` block; the project
    skill records that per-OID metric emission belongs in plugin configuration,
    while profiles stay vendor-curated knowledge
    (`.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md:97`
    through `:100`).
- Superseded recommendation:
  - Status: superseded by later user direction to follow the SNMP profile
    methodology and allow trap profile files to carry metric rules after the rule
    change is explicitly designed.
  - Long-term-best: create a separate trap metric profile layer, not a metric
    block inside stock trap decode profiles.
  - That layer should borrow Prometheus catalog/selection/chart-template
    mechanics, but add trap-specific extraction semantics:
    - `match`: trap OID/name/profile/vendor/varbind-presence predicates.
    - `extract`: event counter, filtered counter, numeric varbind sample,
      multi-value extraction, or state pair.
    - `identity`: source device/vnode and optional bounded resource key.
    - `bounds`: selectors, maximum devices/resources/dimensions, missing-value
      behavior, and lifecycle/expiry.
    - `template`: standard `charttpl.Group` for chart layout.
  - Surgical adaptation: extend current `metrics:` config with explicit
    profile references and minimal filters first, while leaving numeric
    extraction/state pairs for a second approved stage.
  - Per-device requirement: profile-generated samples must be emitted per source
    device. The long-term-best implementation should use V2 host scopes/vnodes
    once product behavior is approved; the surgical alternative is
    `instances.by_labels: [job_name, source_device]` with a bounded, documented
    fallback for unknown devices.

Proposed operator workflow under the Prometheus-like philosophy:

- Status: superseded design sketch, kept only as evidence of the Prometheus
  profile comparison.
- Operator goal: the operator should select metric behavior by profile name and
  identity policy, not hand-author chart templates for common trap cases.
- Step 1: configure trap reception and authentication as today.
  - Listener endpoint, SNMP versions, communities or USM users, output backends,
    rate limits, dedup, and retention remain normal `snmp_traps` job config.
- Step 2: configure source attribution only when topology requires it.
  - Direct sender path: no extra relay config is required; UDP peer is the
    authoritative fallback.
  - Relay/proxy path: the operator must declare trusted relays before
    Netdata accepts `snmpTrapAddress.0` as the original source.
- Step 3: configure device identity for metrics.
  - Long-term-best: route trap-derived metrics to the source device's vnode
    when the device is known from SNMP polling/topology.
  - Fallback: use a bounded source-device label such as source IP/hostname only
    when no vnode mapping exists.
  - Unknown-device behavior must be explicit and documented; the collector must
    not silently merge unrelated unknown devices.
- Step 4: choose metric profile selection mode.
  - `none`: logs only, no trap-derived metric profiles.
  - `auto`: enable stock metric profiles whose trap match rules apply. This is
    Prometheus-like, but for traps it must be limited to curated, bounded-safe
    profiles because future trap arrivals are not known at startup.
  - `exact`: enable only named metric profiles. For event-driven traps, exact
    mode should validate that profiles exist and reference known trap
    definitions; it should not require a live matching trap during `Check()`.
  - `combined`: enable bounded-safe auto profiles plus named profiles.
- Step 5: set cardinality controls when resource-level metrics are enabled.
  - Examples: maximum source devices per listener, maximum resources per device,
    allowed resource selectors, and fallback bucket behavior for overflow.
- Step 6: add a custom metric profile only when stock profiles do not cover the
  use case.
  - Custom profiles should live in an operator directory analogous to
    Prometheus user profiles, not inside regenerated stock trap decode profiles.
  - The custom file should define match/extraction/identity/bounds/template in
    one reviewable YAML document.

Illustrative job-level config shape:

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
    metrics:
      identity:
        mode: vnode_with_source_fallback
        unknown_device: separate_source
        max_devices: 2000
      profiles:
        mode: combined
        mode_combined:
          entries:
            - name: cisco_config_changes
            - name: if_mib_link_state
      bounds:
        max_resources_per_device: 512
```

Illustrative custom metric profile shape:

```yaml
match:
  traps:
    - IF-MIB::linkDown
    - IF-MIB::linkUp

identity:
  device: vnode_with_source_fallback
  resource:
    key_from_varbind: ifIndex
    max_per_device: 512

extract:
  - id: unexpected_link_down_events
    kind: counter
    on_trap: IF-MIB::linkDown
    where:
      ifAdminStatus: up
    metric: snmp_trap_if_unexpected_link_down_events

  - id: interface_link_state
    kind: state_pair
    problem_trap: IF-MIB::linkDown
    clear_trap: IF-MIB::linkUp
    metric: snmp_trap_if_link_down_state
    ttl: 24h

template:
  family: Interfaces
  context_namespace: if_mib
  metrics:
    - snmp_trap_if_unexpected_link_down_events
    - snmp_trap_if_link_down_state
  charts:
    - title: Unexpected link-down events
      context: unexpected_link_down_events
      units: events/s
      algorithm: incremental
      dimensions:
        - selector: snmp_trap_if_unexpected_link_down_events
          name: events
    - title: Link-down trap state
      context: link_down_state
      units: state
      algorithm: absolute
      dimensions:
        - selector: snmp_trap_if_link_down_state
          name: down
```

Operator burden summary:

- Common case: configure listener/security/source attribution, then choose
  `metrics.profiles.mode`.
- Deterministic production case: use `exact` and list approved profile names.
- Advanced/custom case: write one metric profile YAML; do not edit stock trap
  decode profiles and do not hand-author collector code.
- Per-device requirement: configure or accept the documented device identity
  fallback; the system must make device scoping visible in generated charts.

User direction: move from Prometheus-like split profiles to SNMP-like integrated
trap profiles:

- Status: user direction recorded; exact schema still requires design.
- The user prefers the SNMP profile model: one declarative profile should carry
  both trap interpretation and metric/chart behavior, even if this requires
  changing the current project rule that trap profiles must not contain metric
  definitions.
- This means the earlier Prometheus-like two-stage design should no longer be
  the primary recommendation.
- New working target:
  - `snmp.trap-profiles` can grow a `metrics:` or equivalent section.
  - The trap profile remains the source of truth for trap OID, varbind metadata,
    category/severity, message rendering, and now optional trap-to-metric rules.
  - Operator configuration should mainly select/override profiles and identity
    policy, not reference a second trap-metric-profile catalog.
  - The schema should be closer to SNMP polling profiles than to Prometheus
    profiles: declarative extraction, chart metadata, labels/tags, resource
    identity, derived/state metrics, and bounds live near the decoded OID data.
- Required rule change:
  - `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md:97` through
    `:100` currently forbids metric blocks in trap profiles.
  - If this direction is approved after schema design, that rule must be
    rewritten, not worked around.
- Design risks to resolve:
  - Stock generated trap profiles may become much larger if every discovered
    trap gains metric sections.
  - Metric rules must not be blindly enabled for high-cardinality traps.
  - The generator must distinguish vendor decode knowledge from useful,
    bounded, operator-safe metric candidates.
  - Operator overrides must be able to disable or tune shipped metric rules
    without editing regenerated stock profiles.
  - Per-device/vnode identity and unknown-device fallback remain mandatory.

User direction: custom trap profiles with metrics should follow the SNMP profile
methodology:

- Status: user direction recorded; exact metric schema still requires design.
- Evidence from current SNMP polling profiles:
  - `src/go/plugin/go.d/collector/snmp/README.md:86` through `:87` documents
    stock profiles under `/usr/lib/netdata/conf.d/go.d/snmp.profiles/default/`
    and user profiles under `/etc/netdata/go.d/snmp.profiles/`.
  - `src/go/plugin/go.d/collector/snmp/profile-format.md:209` through `:233`
    documents `extends:` inheritance, ordered merge, and override semantics.
  - `src/go/plugin/go.d/collector/snmp/ddsnmp/load.go:270` through `:283`
    builds the same user-before-stock profile search path in code.
- Evidence from current SNMP trap profiles:
  - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:36`
    through `:46` already documents stock trap profiles under
    `/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/` and operator
    profiles under `/etc/netdata/go.d/snmp.trap-profiles/`.
  - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:132`
    through `:137` already says operators convert custom MIBs offline and place
    generated YAML under `/etc/netdata/go.d/snmp.trap-profiles/`.
  - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md:346`
    through `:365` already documents same-filename replacement,
    different-filename additions, and `extends:` chain field-merge.
  - `src/go/plugin/go.d/collector/snmp_traps/profile.go:156` through `:164`
    defines `ProfileDefinition.Extends`, `Varbinds`, and `Traps`, but no
    profile-local `Metrics` field yet.
  - `src/go/plugin/go.d/collector/snmp_traps/load.go:44` through `:65` builds
    the user-before-stock trap profile load path.
  - `src/go/plugin/go.d/collector/snmp_traps/load.go:230` through `:356`
    loads non-abstract YAML profiles, resolves `extends:`, merges base traps by
    OID, validates traps, and returns the final trap set.
- Working operator model:
  - Operators write full custom trap profiles under
    `/etc/netdata/go.d/snmp.trap-profiles/` when the device or MIB is not
    shipped.
  - Operators write small override profiles under the same directory when the
    stock profile already decodes the trap but needs site-specific metric rules.
  - Override profiles should be able to `extends:` a stock or local base profile
    and add, override, or disable profile-local metric definitions without
    copying the whole stock profile.
  - Job configuration should enable or select trap-profile metrics and define
    identity/cardinality policy; it should not carry the normal trap-to-metric
    extraction rules.
- Required design changes implied by this direction:
  - Add a profile-local metric definition surface to trap profiles.
  - Extend the existing trap profile merge logic to merge metric definitions
    with deterministic identity, probably by metric name plus trap OID/name and
    resource key.
  - Add explicit disable/override semantics for stock metric rules so operators
    can tune generated profiles after package upgrades.
  - Keep abstract `_*.yaml` files as reusable mixins for common metric blocks if
    the schema needs reusable vendor-independent trap metric patterns.

Capability coverage matrix for the SNMP-like trap metric profile system:

- Status: preliminary analysis recorded before external Phase 1 gap review;
  superseded by the approved profile-local `metrics:` / `charts:` design and
  the implemented `profile_metrics` job selector.
- Brutal assessment:
  - The SNMP-like model is powerful enough only if `metrics:` becomes a real,
    validated profile section with extraction, identity, bounds, and chart
    metadata.
  - Merely moving today's job-level `metrics:` entries into profiles would not be
    powerful enough; it would still support only trap event counters.
  - Current trap profile parsing must be tightened for this work. The loader uses
    `yaml.Unmarshal` in `src/go/plugin/go.d/collector/snmp_traps/load.go:1000`
    through `:1006`, and `ProfileDefinition` has no `Metrics` field
    (`src/go/plugin/go.d/collector/snmp_traps/profile.go:156` through `:164`).
    Without schema changes, a profile-local `metrics:` key can be accepted by the
    parser but ignored by the runtime.
- Minimum required primitives:
  - `metrics:` array in trap profile YAMLs.
  - `kind:` enum for at least:
    - `counter`: event counters, optionally filtered.
    - `value`: numeric sample extracted from a varbind.
    - `state_pair`: trap-derived current state from problem/clear notifications.
  - Trap selector by symbolic trap name and numeric OID.
  - `where:` predicates over symbolic varbind names, profile category/severity,
    and bounded special fields.
  - `value_from_varbind:` with numeric type validation, units, algorithm,
    optional scaling, and explicit missing/non-numeric behavior.
  - Multiple metric definitions per trap OID, keyed by metric name, not by trap
    OID. This replaces today's duplicate-OID rejection for profile-defined
    metrics.
  - `identity:` with source device/vnode scope and optional resource key.
  - `resource:` key extraction from a varbind or enriched field, with
    per-device caps, TTL/expiry, and obsoletion behavior.
  - `labels:` limited to fixed values or bounded varbind values; high-cardinality
    values stay in logs only unless they are explicitly configured as a capped
    resource key.
  - `chart_meta:` or `template:` with context, title, units, algorithm, chart
    type, dimensions, and instance labels.
  - `enabled:` / `disabled:` or equivalent override semantics so operator
    profiles can turn off shipped metric definitions by stable metric name.
  - Use-case coverage:
  - Filtered event counters:
    - Covered by `kind: counter`, `on_trap`, and `where`.
    - Required for unexpected `linkDown` where `ifAdminStatus=up`, and for
      config-change counters filtered by terminal type.
  - Numeric value extraction:
    - Covered by `kind: value` and `value_from_varbind`.
    - Required for Arista utilization, Juniper CPU/memory, and DFC packet-rate
      trap-carried values.
  - Multi-value extraction from one notification:
    - Covered by allowing several metric definitions to reference the same trap
      OID.
    - Required for Arista CLB flow fields and Juniper DFC watermark fields.
  - Resource identity:
    - Covered by `identity.device` plus `resource.key_from_varbind` or an
      enriched resource key.
    - Required for per-interface link state, port-security per interface/VLAN,
      and packet-rate threshold events per interface.
  - Stateful set/clear metrics:
    - Covered by `kind: state_pair`, `problem_trap`, `clear_trap`, stable
      resource key, and TTL.
    - Required for `linkDown`/`linkUp`, alarm asserted/deasserted, alert
      firing/resolved, and exceeded/abated threshold pairs.
  - Vendor severity handling:
    - Covered for metrics by `where` predicates and bounded enum labels or
      normalized severity mappings.
    - Not covered for log/event severity unless the product also approves a
      separate dynamic `TRAP_SEVERITY` override contract.
- Required merge semantics:
  - Trap definitions continue to merge by OID as today.
  - Metric definitions should merge by stable metric name within the final
    profile, with later profiles overriding earlier profiles.
  - Multiple metric names may reference the same trap OID.
  - Operator profiles must be able to add metrics without redefining traps.
  - Operator profiles must be able to disable or override stock metrics without
    replacing the whole stock profile file.
  - Required safety gates:
  - Profile validation rejects unknown metric fields, invalid trap references,
    invalid varbind references, duplicate metric names after merge, unsafe labels,
    unsafe resource keys without caps, missing chart metadata, and invalid units
    or algorithms.
  - Generated stock metrics must be curated or marked disabled unless they are
    bounded-safe by construction.
  - The default enablement policy must be explicit; automatic enablement of every
    generated trap metric would be unsafe for large trap hubs.

Phase 1 external gap analysis results:

- Status: completed and integrated into
  `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md`.
- Reviewers run: `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen`.
- Findings accepted only when backed by current Netdata profiles/code or
  established open-source NMS behavior.
- Major accepted gaps:
  - Standard alarm set/clear patterns, especially same-OID set/clear via
    `SNMP-ALARM-MIB::snmpAlarmStatusChange`.
  - Environmental, power, UPS, fan, supply, sensor, and CPU threshold traps.
  - Routing protocol, HA, and adjacency state by peer/neighbor/group.
  - Capacity, pool, address, NAT, CPU, and RMON threshold values.
  - LLDP/STP topology and neighbor churn counters, without trap-driven topology
    mutation.
  - Per-source receiver pipeline health for decode/auth/rate-limit/INFORM errors,
    accepted as a follow-up product phase rather than the initial
    trap-to-metrics implementation scope.
- Existing use-case revisions from review:
  - Vendor severity is a specialization of filtered counters/state metrics, not a
    separate metric kind.
  - Operator-defined custom semantics is a cross-cutting authoring requirement,
    not a standalone metric behavior.
  - Device lifecycle counters are separate from routing/adjacency churn because
    routing/HA needs peer/resource identity.
  - MAC addresses, usernames, free-form strings, and peer identifiers remain
    log-only unless explicitly configured as bounded resource identity.
- Explicit non-goals added:
  - No dynamic journal severity override in this phase.
  - No full alarm/correlation engine.
  - No polling reconciliation or interpolation.
  - No automatic metrics for every decoded trap.
  - No arbitrary high-cardinality labels.
  - No trap-driven topology mutation.
  - No cross-device aggregation inside the collector.

User decision: create a committed spec document through a two-step reviewer
workflow:

- Status: approved by user on 2026-06-12.
- Durable spec target:
  - `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md`
- Step 1:
  - Write only the practical trap metric use cases and concrete examples.
  - Do not include the recommended solution or schema design yet.
  - Run `glm`, `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen` for gap
    analysis against profiles and practical operator needs.
  - Treat model findings as hypotheses; accept only findings backed by profile,
    MIB, or established open-source NMS evidence.
- Step 2:
  - After the use-case goal is complete, write the design for supporting the use
    cases.
  - Run the same reviewers again to find simpler, more powerful designs and
    unwanted side effects.
  - Refine the design only with evidence-backed findings.
- This design-review blocking condition was cleared by later user approvals.
  Implementation is now in progress.

User decision: approve the trap metric profile UX direction from the follow-up
reviewer pass:

- Status: approved by user on 2026-06-12.
- Approved option: A.
- Decision:
  - Operator-facing documentation should present the compact authoring syntax for
    simple and intermediate cases.
  - The loader should expand compact syntax into the canonical metric/chart rule
    form before validation, merge, and runtime use.
  - Stock/generated profiles should use the canonical syntax so generated diffs
    stay explicit, stable, and reviewable.
- Rationale:
  - This keeps simple operator use cases natural and close to SNMP polling
    profile methodology.
  - This preserves one canonical validation/runtime contract for complex cases.
  - This avoids weakening the stock-profile curation and source-control review
    contract.

User correction: source identity, enrichment, and metric continuity:

- Status: approved by the user on 2026-06-12 and incorporated into the spec.
- Approved:
  - Profile-local metric rules are approved as option 1A.
  - Accepted traps must be committed even when vnode/source enrichment or
    profile metric attribution is incomplete.
  - Device-attributable metrics use V2 host scope when `SourceVnodeID` is known.
  - When `SourceVnodeID` is absent, profile metrics use a bounded fallback
    source identity under the receiver/default host scope.
  - Synthetic vnodes are not part of the initial schema.
- Rejected framing:
  - Unknown source-device enrichment must not mean silently dropping received
    traps. Accepted traps must still be committed to the configured trap log or
    output backend.
  - Vnode attribution is enrichment, not the primary truth source for whether a
    trap exists.
- Required design direction:
  - `TRAP_ENRICHMENT` or equivalent fields should provide evidence that lets
    operators diagnose and fix enrichment and source-identification problems.
  - Trap metrics should derive metric instances from received and enriched trap
    information even when a vnode does not exist, subject to explicit
    cardinality controls.
  - When vnode attribution works and the framework supports it, metrics that
    belong to a vnode should appear under that vnode.
  - Receiver-owned metrics must emit continuously; Netdata does not support
    sparse receiver metrics.
- Resolved technical evidence:
  - `.agents/sow/specs/go-v2-host-scope.md` states that one V2 job runtime owns
    one chartengine engine per host scope and that series identity includes host
    scope.
  - `src/go/plugin/framework/jobruntime/job_v2_scope.go` lazily creates
    per-scope runtime state, including chartengine engines.
  - `src/go/plugin/framework/jobruntime/job_v2.go` processes and emits metrics
    per scope.
  - `src/go/plugin/go.d/collector/cato_networks/write_metrics.go` shows an
    existing collector emitting metrics with `metrix.WithHostScope`.

User decision: full receiver pipeline monitoring was initially a separate
follow-up phase:

- Status: approved by the user on 2026-06-13 and incorporated into the spec.
- Originally approved:
  - The initial trap-to-metrics implementation scope remains profile-defined
    metric extraction, source identity, bounded attribution, and continuous
    extraction diagnostics.
  - Full receiver pipeline monitoring was a later product phase.
  - The trap-to-metrics design must still preserve source identity, trap
    commitment, and diagnostic evidence so the later phase could report
    receiver/pipeline health per source.
- Implications:
  - This decision was superseded on 2026-06-13 when the user asked to implement
    pipeline metrics after profile metrics in the same SOW.
  - Existing listener/job-scoped built-in static charts such as trap events,
    severities, processing errors, and dedup suppression became part of the
    Phase 2 pipeline-metrics implementation review.
  - Built-in extraction diagnostics required by profile metrics remain in scope:
    attribution failures, ambiguity, rule misses, extraction failures, cap
    overflows, and source route transitions.
  - Phase 2 should cover raw receive rate, accepted/committed rate,
    drop/error stages, unknown OID/MIB gaps, SNMPv3 USM breakdown, INFORM
    outcomes, dedup/throttle suppression, source cardinality, top talkers,
    per-source last-seen/silence, and OS receive-buffer evidence where it can
    be collected safely.

User decision: implement profile metrics first, then pipeline metrics:

- Status: approved by the user on 2026-06-13 and incorporated into the SOW.
- Approved direction: long-term-best.
- Decision:
  - Phase 1 implements profile-defined trap metrics.
  - After Phase 1, run `glm`, `kimi`, `mimo`, `minimax`, `deepseek`, and `qwen`
    as read-only reviewers over the whole SOW and implementation.
  - Phase 2 implements built-in receiver/pipeline metrics.
  - After Phase 2, run the same six reviewers again over the same broad scope.
  - Both phases must preserve trap commitment before metric attribution and
    per-source identity where the received or enriched trap provides a
    trustworthy source.
- Implications:
  - Pipeline metrics are no longer deferred to a separate SOW.
  - Review gates are milestone gates, not final-only checks.
  - Receiver-level totals remain valid only for receiver-owned signals that
    cannot be attributed to a source; source-attributable signals must be
    emitted per source with bounded cardinality.
  - Netdata's no-sparse-metrics constraint applies to receiver metrics,
    profile counters, state values, and fresh trap sample values.

User decision: commit the two SNMP trap research documents as durable spec
memory:

- Status: approved by the user on 2026-06-13 and incorporated into the specs
  index.
- Approved option: B.
- Documents:
  - `.agents/sow/specs/snmp-traps/research/playbooks/Skill-Distillation-SNMP-Traps-in-Network-Performance-Monitoring-NetOps-SecOps.md`
  - `.agents/sow/specs/snmp-traps/research/playbooks/PLAYBOOK-Monitoring-SNMP-Traps-in-Modern-Enterprise-NPM-NetOps-SecOps.md`
- Classification:
  - Specs/research evidence, not end-user documentation and not operator skills.
  - They preserve domain model, operational signals, failure modes, KPIs,
    maturity model, receiver pipeline monitoring evidence, and validation
    scenarios.
- Implications:
  - `.agents/sow/specs/README.md` must index both files.
  - Sensitive-data scanning must cover both files before commit.
  - The trap-to-metrics spec must treat them as evidence for current and future
    SNMP trap design, not as immediate implementation scope.
- Sanitization:
  - Two OID examples were formatted with `[.]` separators because the SOW
    sensitive-data scanner otherwise interpreted SNMP OID prefixes as public IP
    addresses on lines that also mention logs or requests.

User decision: remove/rename obsolete job-level trap `metrics:` instead of
retaining a compatibility shim:

- Status: approved by the user on 2026-06-13 and incorporated into the spec.
- Approved direction: long-term-best.
- Evidence provided by the user:
  - SNMP traps were merged into Netdata on 2026-06-13.
  - SNMP traps have not been released with end-user documentation.
  - No users depend on the old trap metric configuration contract.
- Decision:
  - The old job-level trap `metrics:` list is not a public compatibility
    contract.
  - The implementation should remove or rename it as needed for the clean
    profile-local trap metric model.
  - No deprecated per-job compatibility shim is required.
- Implications:
  - Profile-local trap `metrics:` and `charts:` are the only supported authoring
    surface for new trap-derived metrics.
  - Tests and docs for the obsolete job-level trap `metrics:` authoring path must
    be removed or rewritten.
  - Collision checks no longer need a legacy-shim namespace, but still need to
    reject collisions with built-in static collector metrics.
  - Release notes or migration notes for a deprecated shim are not required.

Open decisions:

1. Approve the profile-based design.
   - Approved long-term-best decision: trap profiles may define optional
     profile-local `metrics:` extraction rules and profile-local `charts:`
     chart layout, validated and merged with the existing `extends:` mechanism.
2. Approve the source-device identity target.
   - Approved long-term-best decision: device-attributable profile metrics use
     V2 host scope when `SourceVnodeID` is known; unknown sources default to a
     bounded `source_label` fallback; over-cap metric instances are skipped with
     continuous diagnostics, but accepted traps are still committed.
3. Decide implementation staging for V2 host-scope support.
   - Approved long-term-best decision: the framework already supports emitting
     multiple host scopes from one V2 collector job, so device-attributable
     profile metrics should use it in the first implementation that enables
     those rules.
4. Decide legacy job-level `metrics:` compatibility.
   - Approved long-term-best decision: remove or rename the job-level trap
     `metrics:` list as needed; do not retain a deprecated compatibility shim.
5. Decide whether `synthetic_vnode` is in scope.
   - Approved surgical decision: reject `synthetic_vnode` in the initial schema
     and defer it to a future approved design.
6. Decide receiver/pipeline metrics staging.
   - Approved long-term-best decision: implement pipeline metrics in this SOW as
     Phase 2 after profile-defined metrics and a profile-metrics reviewer gate.

## Implications And Decisions

Resolved user decisions:

1. Profile-local metric rules.
   - A. Approved.
     - Pros: aligns with SNMP profile methodology; keeps custom trap decode,
       metric extraction, and charting together; supports the identified use
       cases.
     - Cons: changes the current trap-profile rule that profiles do not define
       metrics; requires loader, generator, docs, tests, and skill updates.
     - Implications: profile YAML becomes a stronger public authoring contract.
     - Risks: silent operator failures if strict validation is not implemented.
     - Decision class: long-term-best.
   - B. Keep metrics only in job configuration.
     - Pros: smaller authoring surface.
     - Cons: does not follow SNMP profile methodology and makes vendor/site
       semantics harder to maintain.
     - Implications: the use cases would require a separate second profile
       system or a complex job-level schema.
     - Risks: duplicated profile knowledge and weaker custom-profile workflows.

2. V2 host-scope staging.
   - A. Approved: make V2 host-scope emission a prerequisite before enabling
     device-attributable profile metrics.
     - Pros: satisfies the per-device requirement without per-job fallback
       surprises.
     - Cons: larger first implementation.
     - Implications: implementation must touch host-scope metric writing, chart
       planning, docs, and health impact together.
     - Risks: wider blast radius if not tested thoroughly.
     - Decision class: long-term-best.
   - B. Stage with explicit disable/fail diagnostics.
     - Pros: allows schema and validation work to land before host-scope
       emission.
     - Cons: operators cannot use device-attributable profile metrics until the
       host-scope stage lands.
     - Implications: selected rules must fail `Check()` or be disabled with
       visible diagnostics, never silently fall back to per-job charts.
     - Risks: partial feature confusion if docs are not strict.

3. Legacy job-level `metrics:` compatibility.
   - B. Approved: remove or rename job-level trap `metrics:` before release.
     - Pros: cleaner final configuration and one trap metric authoring model.
     - Cons: not compatible with earlier unreleased branch-local examples.
     - Implications: implementation removes obsolete schema/docs/tests instead
       of carrying a shim.
     - Risks: low because the feature has not shipped as a documented public
       contract.
     - Decision class: long-term-best.

4. Receiver/pipeline metrics staging.
   - B. Approved: implement receiver/pipeline metrics as Phase 2 in this SOW.
     - Pros: closes the operational monitoring gap while the trap metrics
       identity model is still being implemented.
     - Cons: larger SOW and two review/validation milestones.
     - Implications: profile metrics must be completed and externally reviewed
       before pipeline metrics start; pipeline metrics must then receive the
       same six-reviewer treatment.
     - Risks: wider blast radius around existing built-in charts and health
       rules, requiring stronger tests and same-failure searches.
     - Decision class: long-term-best.
   - A. Defer pipeline metrics to a later SOW.
     - Pros: smaller initial implementation.
     - Cons: leaves receiver health mostly listener-level and risks designing
       profile metrics without proving the shared identity model for receiver
       diagnostics.
     - Implications: a second SOW would have to revisit chart identity,
       continuity, and docs.
     - Risks: product ships without the pipeline evidence operators need to
       debug trap reception and attribution.

## Plan

1. Finish evidence gathering.
   - Scope: source references, docs references, health rules, metadata generation, framework host/vnode patterns, and external OSS references.
   - Risk: low; read-only.
2. Present user decisions with concrete evidence.
   - Scope: metric identity, unknown fallback, operator extraction, pipeline metric scoping, docs scope.
   - Risk: low; implementation remains blocked until decisions are recorded.
3. Implement profile-defined trap metrics and profile metric identity.
   - Scope: collector runtime state, profile rule loading, extraction, chart
     labels, chart templates, diagnostics, and tests.
   - Risk: high because this introduces the public trap metric authoring model.
4. Run the profile-metrics external reviewer gate and fix real findings.
   - Scope: six-model read-only review over the whole SOW and implementation.
   - Risk: medium; review findings may expose design or implementation debt.
5. Implement receiver/pipeline metrics.
   - Scope: built-in receiver metrics, per-source attribution where possible,
     receiver totals where required, chart identity, health impact, docs, and
     tests.
   - Risk: high because this touches existing built-in charts and receiver
     observability.
6. Run the pipeline-metrics external reviewer gate and fix real findings.
   - Scope: same six-model read-only review over the whole SOW and implementation.
   - Risk: medium; repeated reviews may surface additional issues.
7. Update docs, metadata, specs, and project skills.
   - Scope: end-user docs, generated integration docs, durable AI-facing rules.
   - Risk: medium; docs must match implementation exactly.
8. Validate and complete SOW lifecycle.
   - Scope: tests, generation checks, same-failure scan, sensitive-data gate, SOW deletion before merge.
   - Risk: low if prior steps are complete.

## Execution Log

### 2026-06-12

- Created branch/worktree for broader SNMP trap metrics and docs work.
- Recorded planning SOW with current evidence and open design decisions.
- Added concrete metric-extraction use-case candidates from current Netdata code,
  shipped trap profiles, RFCs, and open-source NMS reference implementations.
- Rebasing onto `upstream/master` completed cleanly at `b6256b7a4f`.
- Analyzed Prometheus collector chart profiles and recorded which profile
  catalog, selection, chart-template, and test patterns can be reused for SNMP
  trap metric extraction.
- Recorded user direction to move away from a Prometheus-like two-stage
  trap-to-metric-profile catalog and toward SNMP-like trap profiles carrying
  optional metric rules.
- Created `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md` as the durable
  spec target.
- Completed Phase 1 use-case-only draft and ran external gap review with `glm`,
  `minimax`, `kimi`, `mimo`, `deepseek`, and `qwen`.
- Integrated evidence-backed Phase 1 findings into the spec, including standard
  alarm set/clear, environmental/power, routing/HA adjacency, capacity
  threshold, L2 topology-change counter, and the per-source receiver-health use
  case that was initially recorded as a separate follow-up product phase and
  later brought into this SOW as Phase 2.
- Drafted Phase 2 recommended design in
  `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md`.
- Ran Phase 2 design review with `glm`, `minimax`, `kimi`, `mimo`, `deepseek`,
  and `qwen`.
- Integrated the first Phase 2 review findings:
  - default-disable / `auto_safe` stock metric enablement policy;
  - strict profile loader and data-model requirements for `metrics:` / `charts:`;
  - separate `metrics:` extraction rules from profile-local `charts:` layout;
  - full-replacement metric/chart merge semantics in `extends:` chains;
  - bounded unresolved-source fallback and privacy mode;
  - numeric predicate operators and sample type validation;
  - lifecycle, TTL sweep, chart caps, overflow counters, and runtime ordering;
  - stock generator curation contract;
  - explicit compatibility decision point for existing job-level `metrics:`.
- Ran Phase 2 design review round 2 with the same six reviewers and integrated
  remaining high/medium findings:
  - eager metric rule catalog requirements so lazy stock profile loading cannot
    defer validation until trap arrival;
  - explicit V2 host-scope mapping and no silent per-job fallback;
  - unresolved-source eviction, hash privacy, NAT/multi-homed caveat, and
    overflow behavior;
  - reload behavior for added/removed/disabled/renamed metric rules;
  - in-memory chart-template compilation path;
  - stock metric curation source and promotion workflow;
  - concrete recommendation to keep legacy job-level `metrics:` as a deprecated
    shim unless maintainers prove it has not shipped. This recommendation was
    superseded on 2026-06-13 by the user-confirmed long-term-best decision that
    the feature has not shipped and no shim is required.
- Ran Phase 2 design review round 3 with the same six reviewers and integrated
  remaining substantive findings:
  - explicit V2 host-scope dependency behavior, including fail/disable
    diagnostics and no silent per-job degradation;
  - strict loader migration requirements based on the current lenient
    `yaml.Unmarshal` profile load path;
  - type-specific selector requirements for `counter`, `sample`, and `state`
    rules;
  - profile-local chart reference resolution after full `extends:` merge;
  - cardinality cap precedence for rules, resources, sources, charts, and jobs;
  - `not: true`, `range`, `TimeTicks`, and missing-varbind semantics;
  - rejection of `incremental` algorithms for initial trap-carried sample rules;
  - in-memory state restart semantics and periodic `Collect()` cycle wording;
  - legacy job-level `metrics:` shim preserving per-job behavior by default.
    This was superseded on 2026-06-13 by the user-confirmed long-term-best
    decision that the feature has not shipped and no shim is required;
  - preferred generator-owned `curated_metrics.yaml` source for stock metric
    curation;
  - health alert, reserved metric-name, concurrency, and performance validation
    requirements.
- Ran Phase 2 design review round 4 with the same six reviewers and integrated
  the final surgical findings:
  - safe defaults for absent `profile_metrics` and identity policies;
  - explicit job-level identity enum and `profile_metrics` versus legacy
    `metrics:` distinction;
  - source identity transition behavior from fallback source labels to vnode
    host scope;
  - collector-enforced cardinality caps before charttpl best-effort lifecycle
    caps;
  - reserved metric prefixes and built-in static chart ID/context collision
    checks;
  - `exists`/`absent` predicate semantics and `TimeTicks` validation limits;
  - legacy shim migration/double-count documentation and invalid-entry
    validation. This was superseded on 2026-06-13 by the user-confirmed
    long-term-best decision that the feature has not shipped and no shim is
    required;
  - operator `auto_safe` override rejection and additional validation/test
    requirements.
- Ran a follow-up UX review round with the same six reviewers and recorded the
  user-approved direction:
  - operator docs use compact authoring syntax for simple/intermediate cases;
  - compact syntax expands to canonical form before validation and runtime use;
  - stock/generated profiles use canonical form for reviewable generated diffs.
- Recorded the user-approved identity and continuity correction:
  - accepted traps are committed even when enrichment or profile metric
    attribution is incomplete;
  - device-attributable metrics use V2 host scope when `SourceVnodeID` is known;
  - unresolved sources use bounded fallback source labels under the
    receiver/default host scope;
  - cap overflow skips only new metric instances and increments continuous
    diagnostics;
  - receiver metrics, profile counters, state values, and fresh sample values
    are emitted continuously across periodic `Collect()` cycles.
- Recorded the user-approved receiver pipeline monitoring phase split, then
  superseded it after the user expanded this SOW:
  - profile-defined trap metrics remain Phase 1;
  - receiver/pipeline metrics are now Phase 2;
  - this spec keeps one identity, commitment, and continuous diagnostic contract
    for both phases.
- Recorded the user-approved decision to commit the two SNMP trap research
  documents as durable spec memory:
  - added them to `.agents/sow/specs/README.md`;
  - classified them as specs/research evidence, not end-user docs or operator
    skills;
  - sanitized two OID examples that triggered scanner false positives.
- Rebasing onto `upstream/master` completed cleanly at `cd5870122f` after the
  user noted that SNMP traps were merged into Netdata.
- Recorded the user-approved long-term-best compatibility decision:
  - SNMP traps have not shipped as a public documented contract;
  - obsolete job-level trap `metrics:` can be removed or renamed;
  - no deprecated per-job compatibility shim is required.
- Recorded the user-approved implementation sequencing update:
  - Phase 1 implements profile-defined trap metrics;
  - `glm`, `kimi`, `mimo`, `minimax`, `deepseek`, and `qwen` review Phase 1
    before Phase 2 starts;
  - Phase 2 implements receiver/pipeline metrics in this SOW;
  - the same six reviewers review Phase 2 before final completion.
- Loaded the collector, Go V2, SNMP trap profile authoring, chart template,
  chartengine, metrix, host-scope, and collector consistency references before
  implementation.
- Implementation is now in progress; no implementation files had changed before
  the SOW gate was updated.
- Recorded the user requirement that profile-defined trap metrics are
  user-facing and must be fully tested, documented, schema-visible, and usable
  before Phase 2 pipeline metrics begins.
- Implemented Phase A profile-defined trap metrics:
  - removed the unreleased job-level trap `metrics:` authoring path and its
    obsolete operator-metric runtime/tests;
  - added profile-local `metrics:` / `charts:` loading, strict validation,
    `extends:` merge behavior, and compact syntax expansion;
  - added `profile_metrics` job configuration for enablement, selection mode,
    identity policy, privacy, and cardinality caps;
  - added runtime support for counter, sample, and state rules with predicates,
    scaling, resource identity, vnode host scope, fallback source labels,
    listener identity, cap diagnostics, missing-value handling, and source route
    transition diagnostics;
  - updated collector ordering so profile metrics update only after a trap is
    successfully committed to the output backend;
  - added cardinality index cleanup when inactive chart instances expire so
    source/resource caps release capacity over long-running jobs;
  - updated config schema, metadata, generated integration copy, trap profile
    format docs, durable spec, and SNMP trap profile authoring skill.
- Integrated real Phase A reviewer-round findings before the rerun:
  - reserved the built-in `profile_metric_diagnostics` chart ID and
    `snmp.trap.profile_metric_diagnostics` chart context from profile-local
    chart definitions;
  - changed state `problem_value` to pointer-backed YAML state so
    `problem_value: 0` is a valid explicit value instead of being treated as the
    default;
  - bounded source-route transition diagnostic memory and pruned oldest raw
    route entries when the cap is exceeded;
  - made `missing: drop` increment rule-miss diagnostics while `missing: error`
    increments extraction-failure diagnostics;
  - converted `TimeTicks` sample values from hundredths of seconds to seconds
    before profile `scale`;
  - made hashed fallback source IDs include local Agent identity, job name, and
    selected source value;
  - added one-time loading of metric-bearing stock profile files when
    `profile_metrics` is enabled, while preserving lazy stock decode for jobs
    without profile metrics;
  - rejected invalid predicate combinations, bad ranges, negative multipliers,
    compact `state.varbind` without `set`/`clear`, duplicate output metrics,
    and reserved chart IDs/contexts;
  - made rule-local `identity.resource.max_per_source` optional again, so
    omitted values correctly fall back to the job-level
    `limits.max_resources_per_source` default while negative values are still
    rejected;
  - aligned the built-in profile-metric diagnostics chart context to
    `snmp.trap.profile_metric_diagnostics` across runtime, metadata, integration
    docs, profile-format docs, and the durable spec;
  - sorted runtime rule evaluation per trap OID by chart ID then rule name so
    over-cap instance creation follows the deterministic spec tie-breaker;
  - documented that sample `scale` is applied before metric emission and that
    profile authors must make scaled semantics explicit in metric names, titles,
    and units;
  - renamed the active dashboard taxonomy selector from `operator-metrics` to
    `profile-metrics` and grouped profile-metric diagnostics under pipeline
    health instead of the dynamic profile selector;
  - fixed concurrent `update()` and `collect()` access by snapshotting emitted
    series values, host scopes, and labels while holding the runtime mutex;
  - rejected mixed resource and non-resource rules on one chart, and rejected
    multiple `identity.resource.class` values on one chart, so chart instances
    have stable label shapes;
  - rejected non-integer resource key varbinds in Phase A to prevent accidental
    string/MAC/username/cardinality labels;
  - fixed the built-in diagnostics chart template to use template-local
    `profile_metric_diagnostics` context while compiling to the public
    `snmp.trap.profile_metric_diagnostics` context through the existing
    `snmp` + `trap` chart-template namespace;
  - changed `identity.unresolved_source: drop_metric_instance` diagnostics from
    `overflow_dropped` to `attribution_failed`, because this is an attribution
    refusal, not a cardinality overflow;
  - clarified listener-scoped diagnostics, hash truncation/salt fallback,
    source-transition semantics, and collection-cycle lifecycle timing in
    operator docs and durable spec;
  - removed stale documentation that said per-OID metric extraction lives only
    in plugin configuration.
- Integrated the next real Phase A reviewer findings before the final rerun:
  - narrowed profile metric chart algorithms to `incremental` and `absolute`,
    matching the V2 `charttpl` validator instead of accepting unsupported
    `percentage-of-*` algorithms that would fail later during chart-template
    compilation;
  - added runtime chart-instance cap accounting so
    `charts.lifecycle.max_instances` is enforced before metrics reach
    chartengine and skipped instances increment `overflow_dropped`;
  - rebuilt chart-instance cap indexes after lifecycle expiry so chart caps
    release capacity like source and resource caps;
  - added continuous sample-emission coverage through lifecycle expiry, combined
    mode disabled-include coverage, chart `max_instances` rejection/release
    coverage, unsupported chart-algorithm validation coverage, and an explicit
    profile-metrics hot-path benchmark near configured caps;
  - tightened profile-format and generated integration documentation for
    sample-only `missing: zero`, `incremental`/`absolute` chart algorithms, and
    profile metric `source_id`/`source_kind` plus resource labels.
- Implemented Phase B receiver/pipeline metrics:
  - added job-level pipeline counters for received, decoded, accepted,
    committed, dedup-suppressed, dropped, and write-failed traps;
  - added bounded source-attributed receiver metrics for accepted, committed,
    dedup-suppressed, write-failed, source-attributed errors, and last-seen age;
  - reused the profile-metrics source identity resolver so source metrics use
    vnode host scope when `SourceVnodeID` is available and hashed fallback
    `source_id` / `source_kind` labels otherwise;
  - kept listener-wide event, severity, error, and dedup totals job-scoped for
    unattributable packets and global listener health;
  - deliberately did not duplicate category/severity charts per source because
    profile metrics cover per-device semantic trap activity and a full
    per-source clone would add avoidable high-cardinality mostly-zero series;
  - bounded source receiver metrics to 2000 active sources per job and expired
    inactive sources after 60 collection cycles;
  - preserved accepted trap commitment: source attribution failures and cap
    overflow skip only source metric instances and increment diagnostics;
  - fixed the panic recovery path so `pipeline.dropped` is not incremented after
    a trap has already been committed;
  - split async OTLP export failure accounting by output role: secondary OTLP
    failures are export/source errors only when journal is authoritative, while
    OTLP-only export failures remain terminal output write failures;
  - removed fanout flush/close OTLP error increments so the secondary writer
    owns export-failure accounting when it has the actual export context;
  - updated chart templates, taxonomy, metadata, generated integration copy,
    profile-format docs, durable spec, and SNMP trap profile authoring skill for
    the Phase B metric surface.

## Validation

Acceptance criteria evidence:

- Spec use-case inventory and recommended design are complete in
  `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md`.
- Specs index updated in `.agents/sow/specs/README.md`.
- Implementation gate is ready after the user-approved long-term-best decision
  to remove or rename obsolete job-level trap `metrics:` instead of retaining a
  deprecated compatibility shim.
- Pipeline metrics are in scope as Phase 2 after the user-approved sequencing
  update on 2026-06-13.
- Phase A implementation validation:
  - `go test ./plugin/go.d/collector/snmp_traps/...` passed after adding
    profile-metrics runtime, loader, config, docs, and tests.
  - `go test -race ./plugin/go.d/collector/snmp_traps/...` passed after the latest
    profile-metrics fixes.
  - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
  - Profile-metrics tests cover selection modes, `max_rules` enforcement,
    auto/exact/combined behavior, disabled include rejection,
    enum/synthetic-field/in/existence/absence/numeric/not predicate behavior,
    sample extraction, sample `where` predicates, sample scaling, TimeTicks conversion,
    `missing: zero`, `missing: drop`, `missing: error`, same-OID state set/clear,
    same-OID state `where` predicates, custom same-OID state values,
    separate-OID state set/clear, explicit `problem_value: 0`, state TTL
    clear-and-expire, vnode host scope, hashed source privacy, source-label
    identity, strict unresolved-source drop, attribution-failure diagnostics,
    listener identity, fallback source labels, source route transition
    diagnostics and pruning, resource identity and resource key type validation,
    resource class stock/site-prefix validation,
    source caps, job instance caps, chart instance caps, deterministic over-cap
    rule ordering, source-cap and chart-cap lifecycle release, resource caps,
    resource-cap lifecycle release, job-default resource caps, continuous sample
    emission until lifecycle expiry,
    `missing: unknown_dimension`, chart-template resource labels, stable chart
    label-shape validation, concurrent updates, concurrent update-and-collect
    race coverage, failed-write ordering, metric-bearing stock profile eager
    loading for `profile_metrics`, profile `extends:` rule/chart merge,
    compact/canonical YAML syntax, inline `chart_meta`, diagnostics chart
    context, compiled chart-template contexts, dynamic `ChartTemplateYAML()`
    selection, collector `collect()` emitting built-in and profile metrics in
    the same cycle, duplicate output metric rejection, unsupported chart
    algorithm/type rejection, chart type defaults, and unsupported public config
    rejection, and fanout writer source-attributed OTLP export failure metrics.
  - `go test -run '^$' -bench '^BenchmarkProfileMetricRuntimeUpdateAndCollect$' -benchtime=100x ./plugin/go.d/collector/snmp_traps` passed and reports profile-metrics update+collect overhead near configured source/resource caps with hash source identity, state updates, resource cap checks, metric collection, and TTL sweep.
  - Latest profile-metrics benchmark result after Phase B docs/code alignment:
    `252833 ns/op`, `3955 cycles/s`, `89 series`, `30 sources`,
    `355008 B/op`, `2740 allocs/op`.
  - `python -m json.tool src/go/plugin/go.d/collector/snmp_traps/config_schema.json >/tmp/snmp_traps_config_schema.json.pretty` passed.
  - `git diff --check -- .agents/skills/project-snmp-trap-profiles-authoring/SKILL.md .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md .agents/sow/specs/snmp-traps/trap-metrics-profiles.md src/go/plugin/go.d/collector/snmp_traps src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` passed.
  - `.agents/sow/scan-sensitive.sh .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md .agents/sow/specs/snmp-traps/trap-metrics-profiles.md .agents/skills/project-snmp-trap-profiles-authoring/SKILL.md src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md src/go/plugin/go.d/collector/snmp_traps/metadata.yaml src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md` passed.
  - Stale-contract scan returned no matches for old vnode/source-cap wording:
    `Do not add source_id labels`, `Vnode-scoped known devices do not consume`,
    `Maximum unresolved fallback`, `source fallback transitions`, and old
    multi-mode overflow validation text.
  - Post-review-fix stale scan returned no contradictory active docs/spec hits;
    the only hit was the intentional future-only `bucket_and_count` note in the
    spec.
- Implementation sequencing update validation:
  - Targeted stale-scope scan found only historical/superseded references or
    search commands, not active requirements excluding pipeline metrics.
  - `git diff --check -- .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md .agents/sow/specs/snmp-traps/trap-metrics-profiles.md` passed.
  - `.agents/sow/scan-sensitive.sh .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md .agents/sow/specs/snmp-traps/trap-metrics-profiles.md` passed.
- Phase B implementation validation:
  - `go test ./plugin/go.d/collector/snmp_traps/...` passed after adding
    receiver/pipeline source metrics, docs, and tests.
  - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
  - `go test -race ./plugin/go.d/collector/snmp_traps/...` passed.
  - `git diff --check` passed.
  - YAML parse validation passed for `metadata.yaml`, `charts.yaml`, and
    `taxonomy.yaml`.
  - Removed-source-surface scan returned no matches for the rejected
    source-event/source-severity/source-dedup chart names.
  - Sensitive-data scan passed for the active SOW, durable spec, SNMP trap
    authoring skill, collector metadata, integration docs, and profile-format
    docs.
  - Phase B tests cover job-level pipeline counters, source fallback hashing
    without raw label leakage, vnode host-scope routing, source cap overflow,
    source lifecycle expiry, source attribution diagnostics for failed,
    ambiguous, fallback, vnode, and transition cases, source-attributed errors,
    source-attributed dedup, source write failures, and built-in plus profile
    metrics emitted in the same collect cycle.
  - Post-review profile-metrics validation fixes:
    - `profile_metrics.include` is now rejected unless `mode` is `exact` or
      `combined`, with regression tests for `auto` and `none`.
    - Custom operator metric rules can reference stock traps by MIB-qualified
      trap name; stock profiles are loaded before custom metric rule validation
      when loaded profile metrics require them. A regression test covers a
      user-only metric rule over stock `SNMPv2-MIB::coldStart`.
    - User docs, schema description, generated integration copy, and durable
      spec now document include/mode validation, stock trap-name references, and
      accepted-vs-committed metric timing.
    - Profile chart `type` now defaults to `line` during profile validation,
      unsupported chart types are rejected before runtime, and regression tests
      cover inline `chart_meta` and canonical chart defaults.
    - `identity.resource.class` validation now matches the durable spec: stock
      classes are accepted, operator-defined classes must use a `site_` prefix,
      and regression tests cover both rejection and acceptance paths.
    - Additional profile-metric runtime tests cover `sample` rules with
      `where`, same-OID `state` rules with `where`, and custom state
      `problem_value` / `clear_value`.
    - Final reviewer-driven cleanup aligned the durable spec with the
      implementation and operator docs: `missing: zero` is sample-only, while
      state rules must use explicit set/clear predicates or trap pairs.
    - Profile-format docs now explicitly state that compact aliases are for
      operator-authored profiles; stock/generated profile output should use
      canonical fields.
    - The trap-profile authoring skill now records the committed-only timing
      contract: dedup-suppressed and write-failed traps do not update profile
      metrics.
    - Collector metadata and generated integration copy now describe the full
      self-metrics surface: pipeline counters, per-source receiver health, and
      profile-metric diagnostics.
    - Additional tests cover resource cap release after lifecycle expiry and
      fanout writer source-attributed OTLP export failure metrics.
    - Final semantic cleanup after the Phase B reviewer pass fixed
      synchronous `pipeline.write_failed` conservation: OTLP secondary write
      failures now increment `otlp_export_failed` and source-attributed OTLP
      errors without also marking the trap as a terminal pipeline write failure
      when the authoritative journal commit succeeds. Regression tests cover the
      secondary-write-failure path and the both-backends-fail path.
    - A final same-scope reviewer rerun found the async OTLP variant of the same
      conservation issue: the OTLP writer background export path called
      `recordWriteFailure()` even when OTLP was secondary to journal. Fixed by
      adding an OTLP writer terminal-error role: journal+OTLP treats async OTLP
      export failures as export/source errors only, while OTLP-only treats them
      as terminal write failures. Regression tests cover both roles.
    - Follow-up reviewer polish fixed two low-risk profile-metric UX gaps:
      reserved the built-in `snmp_trap_profile_metrics_` diagnostics prefix, and
      rejected predicates that name a varbind or synthetic field but provide no
      condition operator. Regression tests and docs/spec/skill wording were
      updated.
    - Compact map-form `where` syntax now has direct YAML loader coverage in
      `TestLoadProfileAcceptsCompactAndCanonicalMetricSyntax`.
    - Fanout `Flush()` / `Close()` no longer add a second job-level OTLP export
      error for failures already counted by the OTLP writer. The real secondary
      async export regression test now flushes through the fanout to prove this
      no-double-count behavior.
    - Durable spec polish now records the one-terminal-`pipeline.write_failed`
      invariant, explicit ambiguous-enrichment evidence keys, and the
      unknown-`source_kind` to `other` mapping.
    - Final reviewer-gate feedback requested full collector-path coverage for
      source-attributed dedup pipeline metrics. The existing duplicate-packet
      collector test now asserts job and source `accepted`, `committed`, and
      `dedup_suppressed` pipeline metrics from the real dedup path.
    - Ambiguous/conflicting vnode enrichment now falls back to bounded
      source-label metrics instead of emitting vnode-scoped profile or source
      metrics. Regression tests cover profile metrics and built-in
      source-pipeline metrics for `vnode_mismatch` enrichment evidence.
    - `source_kind` is now a closed label set; unknown future enrichment methods
      map to `other` instead of creating new label values. A regression test
      covers this bounded-label behavior.
    - Profile metric validation now rejects unknown `where` varbind names,
      unsupported synthetic predicate fields, unknown `state.set_when` /
      `state.clear_when` varbind names, and unsupported `state.ttl_behavior`
      values. Tests cover each rejection path.
    - Operator docs, schema text, metadata, generated integration copy, and the
      durable spec now document unambiguous-vnode routing, fallback on ambiguous
      enrichment, closed `source_kind` values, `profile_metrics.enabled`
      defaulting to false, compact map-form `where`, predicate reference
      validation, and absolute chart algorithms for sample/state rules.
    - The stock `go.d/snmp_traps.conf` example no longer shows the removed
      job-level `metrics:` syntax. It now points operators to profile-local
      metric rules plus job-level `profile_metrics` enablement.
    - User-facing metrics docs now explicitly distinguish journal+OTLP export
      errors from OTLP-only terminal write failures, including which
      `write_failed` dimensions increment in OTLP-only mode.
  - Latest full validation after these fixes:
    - `go test -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
    - `go test -race -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `git diff --check` passed.
    - JSON parse validation passed for `config_schema.json`.
    - YAML parse validation passed for `metadata.yaml`, `charts.yaml`, and
      `taxonomy.yaml`.
    - Sensitive-data scan passed for the active SOW, durable spec, SNMP trap
      authoring skill, collector metadata, integration docs, profile-format
      docs, and config schema.
  - `go test -run '^$' -bench '^BenchmarkPipelineSourceMetricsUpdateAndCollect$' -benchtime=10x ./plugin/go.d/collector/snmp_traps` passed and reports worst-case source receiver metric update+collect overhead near the 2000-source cap:
    `22394798 ns/op`, `44.65 cycles/s`, `1999 sources`,
    `26903920 B/op`, `130264 allocs/op`.
  - Latest focused benchmark after the post-review fixes:
    - Profile metrics update+collect:
      `218656 ns/op`, `4573 cycles/s`, `93 series`, `34 sources`,
      `281847 B/op`, `2086 allocs/op`.
    - Pipeline source metrics update+collect:
      `24127448 ns/op`, `41.45 cycles/s`, `1999 sources`,
      `26670560 B/op`, `128603 allocs/op`.
  - Latest validation after the async OTLP terminal-error-role fix:
    - `go test -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
    - `go test -race -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - Focused regression tests for profile predicate validation, compact
      map-form `where`, fanout writer semantics, and async OTLP terminal-error
      roles passed.
    - `git diff --check` passed.
    - JSON/YAML parse validation passed for config schema, metadata, charts,
      and taxonomy.
    - Latest profile-metrics benchmark:
      `288395 ns/op`, `3467 cycles/s`, `89 series`, `30 sources`,
      `354934 B/op`, `2740 allocs/op`.
    - Latest pipeline source metrics benchmark:
      `25378325 ns/op`, `39.40 cycles/s`, `1999 sources`,
      `26903330 B/op`, `130263 allocs/op`.
  - Latest validation after the collector dedup-path source metric test and
    stock `snmp_traps.conf` profile-metrics example fix:
    - `go test -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
    - `go test -race -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `git diff --check` passed.
    - JSON parse validation passed for `config_schema.json`.
    - YAML parse validation passed for `metadata.yaml`, `charts.yaml`, and
      `taxonomy.yaml`.
    - Sensitive-data scan passed for the active SOW, durable spec, SNMP trap
      authoring skill, collector metadata, config schema, stock
      `snmp_traps.conf`, integration docs, and profile-format docs.
    - Targeted stale stock-config scan found no remaining old job-level
      `metrics:` example in `src/go/plugin/go.d/config/go.d/snmp_traps.conf`.
  - Latest validation after the OTLP/journal metrics documentation clarity fix:
    - `git diff --check` passed.
    - JSON parse validation passed for `config_schema.json`.
    - YAML parse validation passed for `metadata.yaml`, `charts.yaml`, and
      `taxonomy.yaml`.
    - Sensitive-data scan passed for the active SOW, durable spec, SNMP trap
      authoring skill, collector metadata, config schema, stock
      `snmp_traps.conf`, integration docs, and profile-format docs.
  - Latest validation after the final reviewer-finding fix batch:
    - Focused regression tests for OTLP close/drain failure accounting, OTLP
      terminal-error roles, profile metric TTL validation, strict chart YAML
      keys, and fanout close behavior passed.
    - `go test -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
    - `go test -race -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `git diff --check` passed.
    - JSON parse validation passed for `config_schema.json`.
    - YAML parse validation passed for `metadata.yaml`, `charts.yaml`, and
      `taxonomy.yaml`.
    - Sensitive-data scan passed for the active SOW, durable spec, SNMP trap
      authoring skill, collector metadata, config schema, stock
      `snmp_traps.conf`, integration docs, and profile-format docs.
    - Latest profile-metrics benchmark:
      `360400 ns/op`, `2775 cycles/s`, `157 series`, `64 sources`,
      `428790 B/op`, `3292 allocs/op`.
    - Latest pipeline source metrics benchmark:
      `21782182 ns/op`, `45.91 cycles/s`, `1999 sources`,
      `27189104 B/op`, `132281 allocs/op`.
  - Latest validation after the profile-metrics user-facing validation fix
    batch:
    - Fixed a latent `newProfileMetricRuntime(nil)` panic by returning a clean
      `profile index not available` error.
    - Profile validation now rejects non-finite or non-numeric
      `greater_than`, `less_than`, and `range` predicate bounds at load time.
    - Duplicate `output.dimension` names are now rejected for rules selected by
      a listener job, with an error naming both conflicting rules.
    - `profile-format.md` now clarifies that hashed `source_id` values are local
      listener identifiers, not portable join keys across Agents, listeners, or
      reinstalls.
    - Focused regression tests for the new validation paths passed.
    - `go test -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `go vet ./plugin/go.d/collector/snmp_traps/...` passed.
    - `go test -race -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`
      passed.
    - `git diff --check` passed.
  - Latest validation after the profile-metrics user documentation and
    non-finite predicate hardening pass:
    - Runtime numeric predicates now reject non-finite trap values (`NaN`,
      `+Inf`, `-Inf`) so malformed values cannot accidentally satisfy a numeric
      `range`.
    - Profile validation rejects reversed numeric ranges at load time.
    - Profile validation rejects non-positive `state.ttl` durations at load time.
    - `profile-format.md` documents finite numeric bounds, `lower <= upper`
      ranges, runtime non-finite rule misses, and positive `state.ttl`
      durations.
    - The durable SNMP traps design spec now describes profile-defined metrics,
      job-level receiver totals, bounded source receiver metrics, and dynamic
      profile metric contexts.
    - Final pipeline accounting hardening fixed OTLP worker panic recovery:
      accepted batched or queued entries are drained through the same
      `incNewOTLPExportFailures()` path as normal export failures, and recovery
      replies to an active `Flush()` or `Close()` waiter.
    - Focused profile-metric validation tests passed.
    - Focused OTLP panic/export regression tests passed.
    - Full collector validation passed:
      `go test -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`,
      `go vet ./plugin/go.d/collector/snmp_traps/...`, and
      `go test -race -count=1 -timeout 180s ./plugin/go.d/collector/snmp_traps/...`.
    - `git diff --check` passed.
    - JSON parse validation passed for `config_schema.json` and
      `catalogue.json`.
    - YAML parse validation passed for `charts.yaml`, `metadata.yaml`,
      `taxonomy.yaml`, and `snmp_traps.conf`.
    - Stale active-doc/spec scan found no remaining public/operator references
      to the old job-level `metrics:` / `dimension_from_varbind` contract
      outside historical evidence in `trap-metrics-profiles.md`.
  - Phase B external reviewer gate ran after the first post-review fixes and
    returned production-grade verdicts from the captured reviewers except for
    real findings around write-failure semantics and ambiguous-vnode routing.
    Those findings were fixed above.
  - Final same-scope reviewer gate from the stable post-panic-fix baseline
    completed: `glm`, `minimax`, `kimi`, `mimo`, `qwen`, and `deepseek` returned
    `PRODUCTION GRADE` with no blocking findings.
  - Post-review focused validation passed:
    `go test -count=1 -timeout 120s -run 'TestProfileMetricRuntimeRejectsNonFinitePredicateActual|TestProfileMetricValidationRejectsNonNumericPredicateBounds|TestProfileMetricRuntimePredicateEdgeCases|TestProfileMetricValidationRejectsUnsupportedPublicConfig' ./plugin/go.d/collector/snmp_traps/`.
  - Final doc/SOW hygiene checks passed for the last range-wording and reviewer
    status edits: `git diff --check` on the changed files and targeted
    sensitive-data scan.

Tests or equivalent validation:

- Targeted sensitive-data scan passed for the active SOW, the new spec, and the
  specs index.
- Follow-up UX spec update validation:
  - `git diff --check` passed for the changed SOW/spec files.
  - Targeted sensitive-data scan passed for the changed SOW/spec files.
- Identity and continuity correction validation:
  - Targeted obsolete identity-term scan returned no matches for the changed
    SOW/spec files.
  - `git diff --check -- .agents/sow/specs/snmp-traps/trap-metrics-profiles.md .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md` passed.
  - `.agents/sow/scan-sensitive.sh .agents/sow/specs/snmp-traps/trap-metrics-profiles.md .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md` passed.
- Receiver pipeline monitoring phase-split validation:
  - Targeted scope-wording scan returned no stale mandatory-current-scope
    wording for receiver pipeline health.
  - `git diff --check -- .agents/sow/specs/snmp-traps/trap-metrics-profiles.md .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md` passed.
  - `.agents/sow/scan-sensitive.sh .agents/sow/specs/snmp-traps/trap-metrics-profiles.md .agents/sow/active/SOW-20260612-snmp-trap-metrics-docs.md` passed.
- Research document commit validation:
  - `git diff --check -- .agents/sow/specs/snmp-traps/research/playbooks/PLAYBOOK-Monitoring-SNMP-Traps-in-Modern-Enterprise-NPM-NetOps-SecOps.md .agents/sow/specs/snmp-traps/research/playbooks/Skill-Distillation-SNMP-Traps-in-Network-Performance-Monitoring-NetOps-SecOps.md` passed.
  - `.agents/sow/scan-sensitive.sh .agents/sow/specs/snmp-traps/research/playbooks/PLAYBOOK-Monitoring-SNMP-Traps-in-Modern-Enterprise-NPM-NetOps-SecOps.md .agents/sow/specs/snmp-traps/research/playbooks/Skill-Distillation-SNMP-Traps-in-Network-Performance-Monitoring-NetOps-SecOps.md` passed after scanner-safe OID formatting.
  - Specs index updated in `.agents/sow/specs/README.md`.
  - Docs gate: end-user docs unchanged because no public configuration,
    command, schema, or operator workflow changed in this commit.
  - Specs gate: updated with the two durable research/spec evidence documents
    and the index entry.
  - Operator skill gate: unchanged because no portable operator workflow changed.
- `.agents/sow/audit.sh` passes all checks for the new active SOW and spec index
  entry, but still fails on pre-existing legacy SOW references in older durable
  specs under `.agents/sow/specs/snmp-traps/`. Those failures are unrelated to
  this spec work and were not introduced by this change.

Real-use evidence:

- External review was run in the requested two-step process:
  - Phase 1 use-case gap analysis with `glm`, `minimax`, `kimi`, `mimo`,
    `deepseek`, and `qwen`;
  - Phase 2 design review with the same six reviewers, iterated through four
    rounds until only surgical spec clarifications remained and were integrated.

Reviewer findings:

- Phase A implementation reviewer round 1:
  - `qwen` identified stale docs saying metric opt-in lived in plugin config,
    missing separate-OID state coverage, unbounded `sourceRoutes`, and
    `not`+`absent` predicate ambiguity; all were fixed.
  - `kimi` identified missing diagnostics chart reservation,
    `problem_value: 0` defaulting, unbounded `sourceRoutes`, and missing
    practical tests; all correctness items and practical tests were fixed.
  - `minimax` identified stock metric catalog/lazy-load risk, TimeTicks
    semantics, duplicate output/validation gaps, and missing docs/tests; fixed
    with metric-bearing stock-file loading, TimeTicks seconds conversion,
    stricter validation, and docs/tests.
  - `glm` reported no blockers; its notes on missing-error semantics and
    cardinality cleanup were addressed by making `missing: drop` and
    `missing: error` diagnostically distinct and bounding source-route memory.
  - Earlier MIMO/DeepSeek sessions did not return stable final output before
    process exit; a later full six-reviewer gate superseded that incomplete
    capture.
  - A reviewer finding that profile chart templates must be rebuilt after series
    expiry was rejected as a false positive: chart templates describe rule
    dimensions and label sets, not per-source runtime instances; chart instances
    expire through chart lifecycle.
- Phase A implementation reviewer rerun findings:
  - `mimo` and `deepseek` reported production-grade status and requested cheap
    integration coverage for dynamic chart template generation and collector
    collection of built-in plus profile metrics; both tests were added.
  - `qwen` identified the lock-free collect/update race; fixed by snapshotting
    emitted series under the runtime mutex and validating with `go test -race`.
  - `minimax` identified stale taxonomy naming, stale historical-baseline
    wording, and unbounded resource key risk; fixed with taxonomy/profile-metrics
    naming, explicit Pre-Phase-A historical wording, and integer-like bounded
    resource key validation.
  - A later reviewer pass identified a real diagnostics chart-template context
    issue and a misleading `drop_metric_instance` diagnostic counter; fixed by
    compiling the public diagnostics context through the existing chart-template
    namespace and counting unresolved-source drops as `attribution_failed`.
  - A later reviewer pass identified a real chart-algorithm validator mismatch:
    profile metrics accepted `percentage-of-*` algorithms while `charttpl`
    accepts only `incremental` and `absolute`. Fixed in runtime validation and
    operator docs with a regression test.
  - A later reviewer pass identified that `charts.lifecycle.max_instances` was
    documented as a runtime cap but enforced only by chartengine. Fixed by
    adding profile-runtime chart-instance accounting, overflow diagnostics, and
    cap-release tests after lifecycle expiry.
  - A later reviewer pass identified the missing explicit profile-metrics
    overhead benchmark and continuous sample-emission test. Both were added.
  - Final same-scope six-reviewer rerun completed after the latest fixes:
    `glm`, `minimax`, `mimo`, `kimi`, `qwen`, and `deepseek` all returned
    `PRODUCTION GRADE`.
  - Final reviewer non-blocking notes were accepted as safe:
    `collect()` holds the runtime mutex only while taking a snapshot; source
    route pruning and template rebuild cost are bounded by configured caps;
    raw source route keys may exist in bounded in-memory maps when source label
    privacy is `hash` but are not emitted as labels; the fixed hash-salt
    fallback is used only when machine identity and hostname are unavailable;
    profile metric file length and predicate formatting are maintainability or
    future-optimization notes, not correctness blockers.
- Phase B implementation reviewer round 1 findings:
  - `mimo` returned production-grade status with non-blocking notes.
  - `kimi` found real ship-readiness gaps: `profile_metrics.include` with
    `auto`/`none` was silently ignored, custom metric rules referencing stock
    trap names could fail validation, and the durable spec/status was stale.
    All three were fixed with validation, loader, docs/spec, and tests.
  - `glm` did not provide a final verdict in the captured run; `qwen`,
    `minimax`, and `deepseek` did not have reliable final captured output after
    the interruption. A later full six-reviewer Phase B gate superseded that
    incomplete capture with the same full scope after the fixes.
- Phase B final same-scope rerun, pre-async-OTLP fix:
  - `deepseek` found a real blocker: async OTLP export failures still used
    terminal `recordWriteFailure()` even when OTLP was only the secondary output
    behind an already-successful journal commit. Fixed with an explicit OTLP
    terminal-error role and regression tests for secondary and OTLP-only async
    failures.
  - A subsequent reviewer stream found low-risk polish items that were fixed
    before the final gate: reserve `snmp_trap_profile_metrics_`, reject
    no-condition predicates, add direct compact map-form `where` YAML coverage,
    and avoid double-counting fanout flush/close OTLP export failures.
  - Other captured reviewers reported production-grade or non-blocking notes,
    but the code changed after the blocker fix, so their verdicts are treated as
    superseded. A later full six-reviewer gate reran from the fixed baseline.
  - Final same-scope reviewer round from the dedup/config/doc-clarity baseline:
    `glm`, `mimo`, and `minimax` returned production-grade verdicts with only
    bounded performance or polish notes. `kimi` returned not-production-grade
    with real findings around profile-format field documentation, profile metric
    chart YAML key strictness, invalid `state.ttl` load-time validation, and
    OTLP close/drain accounting; those were fixed. `deepseek` returned
    not-production-grade primarily because the new implementation files were
    still untracked; those files were marked intent-to-add so future diffs and
    commits include them. `qwen` did not return a reliable final captured
    verdict before the harness lost the session, so the next full six-reviewer
    gate treated its verdict as missing and reran from this fixed baseline.
  - Final same-scope reviewer round from the fixed baseline:
    `glm`, `minimax`, `mimo`, and `qwen` returned production-grade verdicts
    with only non-blocking performance or polish notes. `kimi` returned an
    incomplete/non-final captured report that was superseded by the next full
    reviewer gate. `deepseek` returned production-grade overall but identified
    concrete medium user-readiness gaps. `qwen` also identified the
    accepted-but-ignored profile chart `dimensions:` surface. A slower
    `minimax` pass additionally raised OTLP drain accounting and fanout close
    test clarity. All real findings were fixed in the final profile-metrics
    user-readiness hardening batch.
  - Intermediate same-scope reviewer round after the user-facing validation fix:
    captured `glm`, `mimo`, `qwen`, `minimax`, and `kimi` verdicts were
    production-grade with non-blocking notes after the OTLP panic recovery
    accounting fix landed. `minimax` identified the real OTLP worker panic
    accounting gap before it was fixed. `deepseek` output from that round was
    not captured because the tool output hid its session id.
  - Final same-scope reviewer gate from the stable post-panic-fix baseline:
    `glm`, `minimax`, `kimi`, `mimo`, `qwen`, and `deepseek` all returned
    `PRODUCTION GRADE`. No production blockers remain. The final low-risk
    documentation note from `minimax` was fixed by changing range wording to
    `lower <= upper`, and a focused `mimo` rerun independently verified the
    final range wording, profile-metric tests, OTLP panic accounting, old
    contract removal, YAML/JSON parsing, and `go vet`.
- Phase 1 gap-analysis findings from `glm`, `minimax`, `kimi`, `mimo`,
  `deepseek`, and `qwen` were reviewed and integrated when backed by concrete
  profile/code/OSS evidence.
- Phase 2 design review round 1 findings from `glm`, `minimax`, `kimi`, `mimo`,
  `deepseek`, and `qwen` were reviewed and integrated.
- Phase 2 design review round 2 findings from `glm`, `minimax`, `kimi`, `mimo`,
  `deepseek`, and `qwen` were reviewed and integrated.
- Phase 2 design review round 3 findings from `glm`, `minimax`, `kimi`, `mimo`,
  `deepseek`, and `qwen` were reviewed and integrated.
- Phase 2 design review round 4 findings from `glm`, `minimax`, `kimi`, `mimo`,
  `deepseek`, and `qwen` were reviewed and integrated. Reviewers reported the
  remaining changes were surgical spec clarifications; no further external
  review round is required before committing the spec.
- Follow-up UX review findings from `glm`, `minimax`, `kimi`, `mimo`,
  `deepseek`, and `qwen` converged on keeping the canonical design and adding
  compact operator-facing authoring sugar. The user approved option A: compact
  syntax for operator docs, canonical form for stock/generated profiles and
  canonical validation/runtime behavior.

Same-failure scan:

- Initial targeted scan recorded in the Pre-Implementation Gate.
- Post-implementation replacement-contract scan completed:
  - Active public/operator docs, runtime code, schema, and trap-profile authoring
    skill reference the implemented profile-local `metrics:` / `charts:` schema
    and job-level `profile_metrics` selector.
  - The removed unreleased `operator_metric.go` / `operator_metric_test.go`
    implementation has no remaining direct runtime references.
  - Remaining `dimension_from_varbind` references are historical evidence in the
    SOW/spec comparison documents, not active public configuration or runtime
    implementation.

Sensitive data gate:

- This SOW contains only file paths, line numbers, PR number, and synthetic labels.
- No raw secrets, SNMP communities, bearer tokens, customer names, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident details are recorded.

## Artifact Maintenance Gate

- AGENTS.md: unchanged.
- Runtime project skills: SNMP trap profile authoring skill updated for
  profile-local metric/chart schema, validation, reserved built-in prefixes, and
  the built-in/profile metric responsibility split.
- Specs: durable trap-metrics spec updated for Phase A profile metrics and Phase
  B receiver/pipeline metrics.
- End-user/operator docs: `profile-format.md`, collector metadata, generated
  integration copy, taxonomy, and config schema updated for profile metrics and
  receiver/source pipeline metrics.
- End-user/operator skills: no separate public operator skill exists in this
  repo for trap profile metric authoring; the operator-facing contract is in
  `profile-format.md` and generated integration docs.
- SOW lifecycle: active branch-local SOW; must be completed and deleted before merge.

Specs update:

- `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md` added.
- `.agents/sow/specs/README.md` updated with the new spec.

Project skills update:

- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` updated for
  profile metric rule types, predicates, numeric sample constraints, integer
  resource keys, chart label-shape rules, missing-value behavior, reserved
  prefixes, and `auto_safe` review requirements.

End-user/operator docs update:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`
  documents `metrics:` / `charts:`, job `profile_metrics`, identity,
  cardinality controls, rule syntax, examples, diagnostics, and validation.
- `metadata.yaml`, `config_schema.json`, and generated integration copy expose
  `profile_metrics`, diagnostics, dynamic profile metric contexts, and profile
  metric labels.
- `metadata.yaml`, generated integration copy, charts, and taxonomy expose the
  built-in receiver pipeline and source-attributed health metrics, including the
  source cap/lifecycle behavior and a custom profile metric rule example.

End-user/operator skills update:

- No separate public operator skill was added; the operator-facing workflow is
  documented in profile-format and integration docs.

Lessons:

- Pending.

Follow-up mapping:

- Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Follow-up Issues

None yet.
