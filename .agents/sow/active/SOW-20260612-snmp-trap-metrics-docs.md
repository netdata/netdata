# SOW-20260612-snmp-trap-metrics-docs - SNMP Trap Metrics And Documentation

## Status

Status: planning

Sub-state: Phase 1 use-case inventory and Phase 2 recommended design are written
in the committed-spec draft. Four external design review rounds have been
integrated. Implementation is blocked until the user approves the final design
and the remaining compatibility/staging decisions.

## Requirements

### Purpose

Make SNMP trap metrics fit for operator use:

- Operators MUST be able to distinguish which device generated trap activity.
- Operator-defined trap metrics MUST have clear, documented semantics and bounded cardinality.
- End-user documentation MUST explain SNMP traps functionality, configuration, metrics, logs, enrichment, and limitations accurately.
- The urgent counter-algorithm fix is handled separately by PR #22693; this SOW covers the broader metrics extraction and documentation closeout.

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
- End-user documentation covers SNMP trap setup, trap profile behavior, logs, metrics, labels, enrichment, dedup, OTLP/direct journal output, and known limitations.
- Metadata/generated integration docs are updated consistently with source docs and code.
- Tests cover chart identity, operator metric extraction, algorithm declarations, cardinality boundaries, and regression cases for multi-device trap activity.
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

Status: needs-user-decision

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
- Operator metric extraction semantics are either confirmed as event-count-only or deliberately expanded with documented filters/value extraction and tests.
- Documentation and metadata match the implementation exactly.
- Specs and runtime project skills contain the lessons needed to prevent recurrence.
- Removed as redundant (i): obsolete per-job-only documentation if the approved target changes it; obsolete tests that assert per-job-only identity after replacement; any duplicate docs generated from stale metadata.
- Excluded coupled items (ii): none approved yet. Any exclusion from this target requires a user decision because metric identity, docs, and tests are coupled.
- Reference search: required before implementation because the metric identity contract may change. Initial search used:
  - `rg -n "snmp\\.trap\\.events|per device|vnode|job_name|instances\\.by_labels|dimension_from_varbind|operator metrics|trapMetrics|perJobMetrics|buildChartTemplateYAMLOperatorMetric" .agents/sow/specs/snmp-traps src/go/plugin/go.d/collector/snmp_traps -g '*.md' -g '*.go' -g '*.yaml'`
  - Surviving references are mapped in this SOW as current evidence. A broader repository-wide search is required once the selected target is known.

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

1. Complete design analysis for metric identity and extraction semantics.
   - Scope: chart instances, labels, vnode/host scope, unknown-device fallback, operator metric count/filter/value behavior, cardinality limits.
   - Likely files: `metrics.go`, `operator_metric.go`, `enrich.go`, `charts.yaml`, tests, metadata, specs.
2. Update the SOW with explicit user decisions and set `Status: ready` only after the approved plan meets the Pre-Implementation Gate.
3. Implement the approved metric identity model.
   - Scope: static metrics, operator metrics, chart templates, labels, lifecycle cleanup, tests.
4. Implement approved operator extraction changes, if any.
   - Scope: config schema, validation, runtime extraction, docs, tests.
5. Update end-user documentation and generated metadata/integration docs.
   - Scope: configuration, examples, logs, metrics, enrichment, dedup, OTLP/direct journal, troubleshooting.
6. Update specs and runtime project skills with durable lessons.
7. Validate with targeted unit tests, generation checks, taxonomy/integration checks, and same-failure searches.

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
     missing/non-numeric behavior. Documentation must state these are
     trap-arrival samples, not continuously polled values.

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

- Status: preliminary analysis recorded before external Phase 1 gap review; exact
  schema still pending user approval.
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
  - Per-source receiver pipeline health for decode/auth/rate-limit/INFORM errors.
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
- Implementation remains blocked until the spec identifies the whole goal and
  the user approves the final design.

Open decisions:

1. Approve the profile-based design.
   - Recommended long-term-best decision: trap profiles may define optional
     profile-local `metrics:` extraction rules and profile-local `charts:`
     chart layout, validated and merged with the existing `extends:` mechanism.
2. Approve the source-device identity target.
   - Recommended long-term-best decision: device-attributable profile metrics
     use V2 host scope when `SourceVnodeID` is known; unknown sources default to
     `drop_profile_metrics`; `per_source_label` is explicit opt-in with caps.
3. Decide implementation staging for V2 host-scope support.
   - Recommended long-term-best decision: ship V2 host-scope emission before
     enabling device-attributable profile metrics. If staged, selected rules
     must fail `Check()` or be explicitly disabled with operator-visible
     diagnostics; they must not silently degrade to per-job charts.
4. Decide legacy job-level `metrics:` compatibility.
   - Recommended surgical decision: treat existing job-level `metrics:` as
     public unless proven otherwise, keep it as a deprecated per-job event
     counter shim, and require migration to profile-local rules for per-device
     behavior.
5. Decide whether `synthetic_vnode` is in scope.
   - Recommended surgical decision: reject `synthetic_vnode` in the initial
     schema and defer it to a future approved design.

## Implications And Decisions

Pending user decisions:

1. Profile-local metric rules.
   - A. Approve the recommended design.
     - Pros: aligns with SNMP profile methodology; keeps custom trap decode,
       metric extraction, and charting together; supports the identified use
       cases.
     - Cons: changes the current trap-profile rule that profiles do not define
       metrics; requires loader, generator, docs, tests, and skill updates.
     - Implications: profile YAML becomes a stronger public authoring contract.
     - Risks: silent operator failures if strict validation is not implemented.
     - Recommendation: long-term-best.
   - B. Keep metrics only in job configuration.
     - Pros: smaller authoring surface.
     - Cons: does not follow SNMP profile methodology and makes vendor/site
       semantics harder to maintain.
     - Implications: the use cases would require a separate second profile
       system or a complex job-level schema.
     - Risks: duplicated profile knowledge and weaker custom-profile workflows.

2. V2 host-scope staging.
   - A. Make V2 host-scope emission a prerequisite before enabling
     device-attributable profile metrics.
     - Pros: satisfies the per-device requirement without per-job fallback
       surprises.
     - Cons: larger first implementation.
     - Implications: implementation must touch host-scope metric writing, chart
       planning, docs, and health impact together.
     - Risks: wider blast radius if not tested thoroughly.
     - Recommendation: long-term-best.
   - B. Stage with explicit disable/fail diagnostics.
     - Pros: allows schema and validation work to land before host-scope
       emission.
     - Cons: operators cannot use device-attributable profile metrics until the
       host-scope stage lands.
     - Implications: selected rules must fail `Check()` or be disabled with
       visible diagnostics, never silently fall back to per-job charts.
     - Risks: partial feature confusion if docs are not strict.

3. Legacy job-level `metrics:` compatibility.
   - A. Keep a deprecated per-job compatibility shim.
     - Pros: preserves current dashboards and alerts for existing configs.
     - Cons: two metric authoring paths exist during migration.
     - Implications: docs and validation must clearly separate legacy per-job
       counters from profile-local per-device rules.
     - Risks: duplicate OID counting if operators enable both paths without
       understanding the migration.
     - Recommendation: surgical.
   - B. Remove or rename job-level `metrics:` before release if maintainers prove
     it has not shipped publicly.
     - Pros: cleaner final configuration.
     - Cons: only valid if there is concrete evidence that no public
       compatibility contract exists.
     - Implications: requires maintainer confirmation before implementation.
     - Risks: breaking existing users if the evidence is wrong.

## Plan

1. Finish evidence gathering.
   - Scope: source references, docs references, health rules, metadata generation, framework host/vnode patterns, and external OSS references.
   - Risk: low; read-only.
2. Present user decisions with concrete evidence.
   - Scope: metric identity, unknown fallback, operator extraction, pipeline metric scoping, docs scope.
   - Risk: low; implementation remains blocked until decisions are recorded.
3. Implement approved metric identity.
   - Scope: collector runtime state, chart labels, chart templates, tests.
   - Risk: medium to high depending on selected option.
4. Implement approved operator extraction changes, if selected.
   - Scope: config schema, validation, extraction, tests, docs.
   - Risk: medium to high if numeric value extraction is selected.
5. Update docs, metadata, specs, and project skills.
   - Scope: end-user docs, generated integration docs, durable AI-facing rules.
   - Risk: medium; docs must match implementation exactly.
6. Validate and complete SOW lifecycle.
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
  threshold, L2 topology-change counter, and per-source receiver-health use
  cases.
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
    shim unless maintainers prove it has not shipped.
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
  - legacy job-level `metrics:` shim preserving per-job behavior by default;
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
    validation;
  - operator `auto_safe` override rejection and additional validation/test
    requirements.
- No implementation files changed.

## Validation

Acceptance criteria evidence:

- Spec use-case inventory and recommended design are complete in
  `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md`.
- Specs index updated in `.agents/sow/specs/README.md`.
- Implementation remains blocked on user approval of the final design and
  compatibility/staging decisions.

Tests or equivalent validation:

- Targeted sensitive-data scan passed for the active SOW, the new spec, and the
  specs index.
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

Same-failure scan:

- Initial targeted scan recorded in the Pre-Implementation Gate.
- Broader repository-wide scan pending after design decisions.

Sensitive data gate:

- This SOW contains only file paths, line numbers, PR number, and synthetic labels.
- No raw secrets, SNMP communities, bearer tokens, customer names, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident details are recorded.

## Artifact Maintenance Gate

- AGENTS.md: pending.
- Runtime project skills: pending.
- Specs: pending.
- End-user/operator docs: pending.
- End-user/operator skills: pending.
- SOW lifecycle: active branch-local SOW; must be completed and deleted before merge.

Specs update:

- `.agents/sow/specs/snmp-traps/trap-metrics-profiles.md` added.
- `.agents/sow/specs/README.md` updated with the new spec.

Project skills update:

- Pending implementation; the spec records the required future update to
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`.

End-user/operator docs update:

- Pending implementation; the spec records the required documentation and
  generated metadata surfaces.

End-user/operator skills update:

- Pending.

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
