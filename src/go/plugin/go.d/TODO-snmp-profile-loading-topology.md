# TODO - SNMP Profile Loading and Topology Cleanup

## TL;DR

Understand how SNMP profiles are loaded for regular metric collection versus SNMP topology, identify the root cause of the current hardcoded/hacky behavior, and implement the reviewed clean end-state design through SOW-0012.

## Requirements

User request, verbatim:

> We recently added snmp topology functionality. It works, but has a lot of problems, it was added in a hacky way.
>
>
> See src/go/plugin/go.d/collector/snmp/ddsnmp, src/go/plugin/go.d/collector/snmp_topology
>
> There are a lot of issues, but lets focus on one for now - how we load snmp profiles and there is difference between regular (metrics) and for topology. I don't rememeber details but my recollection is logic is hardcoded.
>
> So I need you to understand how we load profiles and suggest a clean solution. Important point: we don't care aboyt less churn, our goal is clean end state. Also backward compatibility is not an issue, this topology feature is wip and we have it only in nightly.
>
> In order to not miss anything you can spawn 3 GPT 5.5 subagents so they can explore the code and give their review. Tell them to do the work directlky - w/o delegating to other subagents.

## Facts

- Current working directory is `src/go/plugin/go.d`.
- Relevant existing TODO files checked:
  - `TODO-snmp-topology-index-refresh-review.md` is about a prior branch review, not this profile-loading design task.
  - `TODO-azure-monitor-custom-tags.md` is unrelated Azure Monitor work and must not be touched.
- Relevant project skills loaded:
  - `.agents/skills/project-writing-collectors/SKILL.md`
  - `.agents/skills/project-snmp-profiles-authoring/SKILL.md`
- Existing SOW/spec overlap checked:
  - `.agents/sow/pending/SOW-0002-20260501-unified-multi-layered-topology-schema.md` is topology-related and must be reviewed for relevant decisions.
  - `.agents/sow/current/SOW-0002-20260503-azure-monitor-workload-vnodes.md` is unrelated current work and must not be changed.
  - `.agents/sow/specs/go-v2-host-scope.md` and `.agents/sow/specs/sensitive-data-discipline.md` exist; neither is clearly the source of truth for SNMP profile loading, but relevant artifact impact must be checked before any future implementation.
- Regular SNMP metrics and SNMP topology use the same physical profile loader:
  - `collector/snmp/ddsnmp/load.go:27-33` defines global cached `ddProfiles`.
  - `collector/snmp/ddsnmp/load.go:35-74` loads profiles once.
  - `collector/snmp/ddsnmp/load.go:220-235` resolves profile directories.
  - `collector/snmp/ddsnmp/load.go:116-155` walks profile YAML files and skips abstract `_*.yaml` files as roots.
  - `collector/snmp/ddsnmp/load.go:157-217` loads one profile and resolves `extends`.
  - `collector/snmp/ddsnmp/profile.go:25-71` selects profiles by `sysObjectID`/`sysDescr` or by `manual_profiles` only when `sysObjectID` is empty.
- Regular SNMP metrics flow:
  - `collector/snmp/profile_sets.go:16-20` calls `ddsnmp.FindProfiles()` and then `selectCollectionProfiles()`.
  - `collector/snmp/topology_profile_filter.go:10-50` strips topology metrics/tags/metadata out of the matched profiles before metric collection.
  - `collector/snmp/collect.go:107-118` creates the ddsnmp collector from the filtered profile set.
- SNMP topology flow:
  - `collector/snmp_topology/config.go:7-11` has only global timing config; it has no device/profile config.
  - `collector/snmp/collect.go:143` registers SNMP devices for topology after regular SNMP initialization.
  - `collector/snmp/topology_device_registry.go:42-73` copies SNMP connection info, `SysObjectID`, `SysDescr`, and `ManualProfiles` into the global device registry.
  - `collector/snmp_topology/collector.go:91-117` polls devices from `ddsnmp.DeviceRegistry`.
  - `collector/snmp_topology/collector.go:216-218` calls `ddsnmp.FindProfiles()` and then `selectTopologyRefreshProfiles()`.
  - `collector/snmp_topology/profile_filter.go:10-47` keeps only topology metrics/tags/metadata.
  - `collector/snmp_topology/collector.go:220-235` ingests hidden and exported metrics, but topology rows are expected to be hidden `_topology_*` metrics.
- The current metrics/topology split is hardcoded:
  - `collector/snmp/ddsnmp/topology_classify.go:16-30` has an exact allowlist of topology metric names.
  - `collector/snmp/ddsnmp/topology_classify.go:43-65` classifies topology tags/metadata with string prefix heuristics such as `lldp`, `cdp`, `dot1d`, `dot1q`, `stp`, `vtp`, `fdb`, `bridge`, and `arp`.
  - `collector/snmp_topology/topology_cache_tags.go:5-24` duplicates the topology metric-name list as ingestion constants.
  - `collector/snmp_topology/topology_cache_metric_dispatch.go:7-35` dispatches topology rows by those metric names.
- Topology protocol composition is partly declarative and partly hardcoded:
  - Vendor/root profiles extend topology mixins, e.g. `config/go.d/snmp.profiles/default/_cisco-base.yaml:3-10`.
  - `_std-lldp-mib.yaml:4-5` extends `_std-topology-lldp-mib.yaml`.
  - `collector/snmp_topology/topology_profiles.go:5-14` still hardcodes topology mixin filenames.
  - `collector/snmp_topology/topology_vlan_context_collect.go:16-28` hardcodes VLAN-context topology loading to `_std-topology-fdb-arp-mib.yaml` and `_std-topology-stp-mib.yaml` through `ddsnmp.LoadProfileByName()`.
- Documentation currently says SNMP profiles avoid hardcoded Go logic:
  - `collector/snmp/profile-format.md:9-16` describes profiles as declarative support for new devices without collector source-code changes.
  - The topology split violates that principle because new topology row kinds currently require Go classifier/dispatch changes.
- Subagent consensus:
  - All 3 GPT-5.5 subagents agreed the physical loader should stay shared.
  - All 3 identified the missing contract as explicit profile purpose/consumer semantics.
  - All 3 recommended replacing name/prefix topology inference with schema-driven profile projection.
  - All 3 called out hardcoded VLAN-context profile loading as a separate cleanup target.

## User Made Decisions

1. Focus on how SNMP profiles are loaded for regular metrics versus topology.
2. Prioritize a clean end state over minimizing churn.
3. Do not treat topology backward compatibility as a constraint because the feature is WIP and nightly-only.
4. Use `src/go/plugin/go.d/collector/snmp/ddsnmp` and `src/go/plugin/go.d/collector/snmp_topology` as primary investigation targets.
5. Spawn 3 GPT-5.5 subagents for independent code exploration/review.
6. Subagents must do the work directly and must not delegate to other subagents.
7. Profile kind/consumer semantics should not be inferred from filenames. Filenames can remain organizational, but runtime behavior should come from explicit YAML metadata such as profile/item consumers and topology row kind.
8. Update the design to adopt the Claude-review corrections that were checked against local code and agreed: topology rows are marked by an explicit closed-enum kind, VLAN-context is not a separate consumer, and projection/dispatch should be based on explicit topology kinds instead of metric-name heuristics.
9. Decision 1: use internal manual profile policy. Regular metrics use fallback-only manual profiles; topology uses augment.
10. Decision 2: topology rows require explicit `kind`; explicit derived topology
    wins for matching row identities; reject conflicting explicit topology
    kinds. Earlier "derived omits kind" inheritance wording was superseded
    because validation rejects persisted topology rows without `kind`.
11. Decision 3: reject mixed-consumer virtual metrics at validation.
12. Decision 4: metadata fields and top-level/global `metric_tags` default to both `metrics` and `topology`, with explicit narrowing when needed.
13. Decision 5: use a separate top-level `topology:` list for topology rows instead of embedding topology rows in `metrics[]`.
14. Decision 6: define one closed `TopologyKind` per current topology row shape, including LLDP management-address rows.
15. Decision 7 was superseded during implementation: remove `KindSysUptime`.
    `systemUptime` is not a topology row kind.
16. Decision 8: topology applies top-level/global `metric_tags` as local device/profile labels, not as dispatch keys on every topology row.
17. `HiddenMetrics` is a general delivery container, not topology-owned. Other work/PRs may use it to deliver ddsnmp-collected values to another collector, e.g. license-related values processed by a license collector. The topology cleanup may remove topology's dependency on underscore-prefix hidden metrics, but must not blindly delete or redefine `HiddenMetrics` without auditing non-topology consumers.
18. Fourth readiness review verdict: `READY TO PROCEED` to a SOW after recording the `HiddenMetrics` preservation constraint, the five implementation sub-decisions, and the explicit non-topology `HiddenMetrics` preservation test.
19. Before implementation starts, write a step-by-step implementation plan detailed enough for an external readiness review. Ask Claude to judge the plan as `READY TO WRITE CODE` or `NEEDS ADJUSTMENTS`; do not start code until that review is accepted or the plan is adjusted.
20. Fifth readiness review verdict: `NEEDS ADJUSTMENTS`. Accept blocking points B1-B6 and non-blocking suggestions S1-S6 as implementation-plan fixes. For B2, use option A: phases 3-6 are one logical topology cutover, not independently shippable. B6's original option C (`KindSysUptime` collector/projection-side mapping) was later superseded by Decision 7's removal of `KindSysUptime`.
21. Sixth readiness review verdict: `NEEDS ADJUSTMENTS`. Accept blocker N1 and tightenings N2-N5. `Profile.merge()` must merge the new `Definition.Topology` slice during `extends:` loading, clone targets must be explicit, VLAN-context synthetic metrics must carry `TopologyKind`, `vlanScopableKinds` is pinned to the existing VLAN ingest kinds, and new schema fields must follow existing YAML/JSON tag conventions.
22. Seventh readiness review verdict: `READY TO WRITE CODE`. Accept non-blocking hygiene tightenings NB1-NB4: expand stale-helper scans, pin catalog/projection API package location to `collector/snmp/ddsnmp`, use a top-level/global metric-tag wrapper for `Consumers` instead of adding consumer semantics to per-row `MetricTagConfig`, and spell out the post-cutover topology ingest loop shape.
23. User approved proceeding with implementation. Activate SOW-0012, move it to current/in-progress, run Phase 0 baseline checks, then start implementation according to the reviewed plan.
24. User correction during implementation: remove `KindSysUptime`. `systemUptime`
    is not a topology row kind. A direct `pkg/snmputils` uptime helper is a
    proposed replacement path, not yet a user decision.
25. Regular SNMP metrics must keep uptime collection through existing metrics
    profile rows. Do not move or remove uptime from `_system-base.yaml`.
26. Topology uptime acquisition after removing `KindSysUptime`: use option A.
    Add `pkg/snmputils.GetSysUptime(gosnmp.Handler)` with the same three OIDs
    and scale rules as `_system-base.yaml`, and call it from `snmp_topology`
    during refresh. Regular SNMP uptime metrics remain profile-driven.
27. Public profile-format documentation should describe the current topology
    schema without mentioning the old underscore-prefixed topology metric-name
    implementation.

## Implied Decisions

- This pass is read-only design investigation unless a follow-up request asks for code changes.
- The recommendation must be based on current code paths and profile-format contracts, not recollection.
- The recommendation should identify affected tests/docs/specs for a future implementation.
- The final answer must separate facts from working theories and call out uncertainty.

## Resolved Implementation Decisions

1. Topology uptime acquisition after removing `KindSysUptime`. Resolved: A.
   - Context:
     - Regular SNMP metrics keep `_system-base.yaml` uptime rows; this decision
       is only about how `snmp_topology` gets `localDevice.SysUptime`.
   - Evidence:
     - `config/go.d/snmp.profiles/default/_system-base.yaml:11-44` defines the
       three regular uptime fallback rows: `snmpEngineTime` in seconds,
       `hrSystemUptime` scaled by `0.01`, and `sysUpTime` scaled by `0.01`.
     - `collector/snmp_topology/topology_cache_ingest.go:52-67` stores uptime
       into the local topology device and the `sys_uptime` label.
     - `collector/snmp_topology/topology_local_actor_attrs.go:32-34` exports
       `sys_uptime` as local actor attributes when present.
     - `collector/snmp/ddsnmp/profile_catalog.go:271-320` currently keeps a
       special topology projection path for `systemUptime`/`sysUpTime`; this is
       the part that should disappear with `KindSysUptime`.
   - A. Add `pkg/snmputils.GetSysUptime(gosnmp.Handler)` with the same three
     OIDs and scale rules, and call it from `snmp_topology` during refresh.
     - Pros: removes `KindSysUptime`; keeps profile topology projection pure;
       keeps regular SNMP metrics unchanged; localizes the hardcoded fallback to
       one SNMP utility helper; one extra GET every topology refresh is cheap.
     - Cons: duplicates uptime OID/scale knowledge from `_system-base.yaml`;
       future changes to uptime metric fallback need the helper updated too.
   - B. Keep using ddsnmp regular metric collection for uptime, but identify
     `systemUptime`/`sysUpTime` by name in `snmp_topology`.
     - Pros: no extra SNMP GET; reuses profile scaling and fallback behavior.
     - Cons: keeps metric-name special casing and forces topology projection to
       retain a regular metric row; this preserves the hardcoded logic we are
       trying to remove.
   - C. Stop collecting uptime for topology.
     - Pros: simplest implementation; no duplicate OIDs or extra SNMP request.
     - Cons: topology output loses `sys_uptime` on local actors; existing tests
       and output paths already treat it as useful local device enrichment.
   - Recommendation: A. It cleanly removes uptime from `TopologyKind` and from
     profile projection while preserving the existing topology `sys_uptime`
     output. The duplication is real but small, explicit, and isolated.

Historical options and evidence are kept below for implementation context.

1. Manual profile semantics under `Catalog.Resolve`.
   - Evidence:
     - `collector/snmp/ddsnmp/profile.go:39-54` uses `manual_profiles` only
       when `sysObjectID == ""`.
     - `collector/snmp_topology/collector.go:216-218` inherits the same behavior
       through `FindProfiles()`, so topology cannot force topology mixins when a
       device already returns `sysObjectID`.
   - A. Always augment auto-detected profiles with manual profiles.
     - Pros: intuitive "force these in addition" behavior; fixes topology.
     - Cons: behavior change for regular SNMP.
     - Risk: existing metric users may see unexpected extra metrics.
   - B. Manual profiles override auto-detected profiles.
     - Pros: simple precedence.
     - Cons: users lose vendor-detected enrichment.
     - Risk: broad regular SNMP behavior change.
   - C. Preserve fallback-only behavior.
     - Pros: no regular SNMP behavior change.
     - Cons: keeps the topology bug.
     - Risk: cleaner resolver still carries the old surprise.
   - D. Add an internal resolver policy: regular metrics use fallback-only;
     topology uses augment.
     - Pros: fixes topology without changing regular metrics.
     - Cons: slightly larger API surface.
     - Risk: low if the policy is internal first.
   - Recommendation: D.

2. Mixin merge behavior for topology rows.
   - Evidence:
     - `collector/snmp/ddsnmp/profile.go:186-252` merges metric blocks across
       base and derived profiles.
     - Vendor profiles can override a topology row inherited from a topology
       mixin; without a rule, the override can silently drop topology intent.
   - A. Derived topology row wins completely.
     - Pros: simple.
     - Cons: accidental topology drops.
     - Risk: high.
   - B. Historical wording: preserve base topology kind when derived override
     omits it; explicit derived kind wins. Final implementation requires
     explicit `kind`, so the omitted-kind branch is unreachable from valid YAML.
     - Pros: protects inherited topology intent.
     - Cons: demoting/removing an inherited topology row needs a different
       explicit mechanism if that ever matters.
     - Risk: low for topology WIP.
   - C. Reject conflicting merges and force authors to be explicit.
     - Pros: no silent behavior.
     - Cons: stricter migration and authoring burden.
   - Recommendation: B, plus validation error when both sides explicitly set
     different topology kinds for the same topology row identity.

3. Virtual metric projection.
   - Evidence:
     - Virtual metrics are source-derived; projection can remove some source
       metrics before virtual metric evaluation.
     - A mixed-source virtual metric would otherwise be ambiguous across metrics
       and topology views.
   - A. Inherit the union of source consumers.
     - Pros: no new YAML.
     - Cons: risks leaking topology-derived values into metrics view.
   - B. Add explicit virtual metric consumer/topology fields.
     - Pros: explicit.
     - Cons: extra schema for a currently unused case.
   - C. Reject mixed-consumer virtual metrics at validation.
     - Pros: simplest and safest current contract.
     - Cons: closes a future mixed-use case until we redesign it.
   - Recommendation: C.

4. Default consumer for metadata fields and top-level/global `metric_tags`.
   - Evidence:
     - Topology profiles use metadata/global tags for device identity and local
       topology identity, while metric blocks can be cleanly classified by
       presence/absence of `topology:`.
     - A `[metrics]` default for metadata would silently drop identity fields from
       topology unless every topology mixin repeats `consumers: [topology]`.
   - A. Default metadata fields and top-level/global `metric_tags` to both
     `metrics` and `topology`; narrow with `consumers` only when needed.
     - Pros: safe for topology mixins; keeps most metrics-only files untouched.
     - Cons: truly metrics-only metadata must be explicitly narrowed.
   - B. Default to `metrics` only.
     - Pros: common metric-only case is short.
     - Cons: silent topology data loss.
   - C. Infer default from filename/directory.
     - Pros: fewer annotations after migration.
     - Cons: filesystem layout becomes semantic API and is fragile for user
       profiles.
   - Recommendation: A.

5. Topology row schema shape.
   - Evidence:
     - The earlier recommended design put `topology.kind` inside entries under
       `metrics:`.
     - `collector/snmp/ddsnmp/profile.go:194-252` deduplicates/merges metric
       blocks by scalar/table identity and has no current way to distinguish
       "same SNMP row used as regular metric" from "same SNMP row used as
       topology evidence".
     - `_system-base.yaml:11-44` currently exposes `systemUptime` as a regular metric,
       while topology also needs it as topology freshness evidence.
   - A. Keep per-item `metrics[].topology`.
     - Pros: smaller schema change; all SNMP collection rows remain in one list.
     - Cons: shared or duplicate rows such as `systemUptime` need careful merge
       rules; topology intent remains embedded inside a list named `metrics`.
     - Risk: cross-consumer dedup/merge bugs are easy to reintroduce.
   - B. Add a separate top-level `topology:` list that reuses the same SNMP row
     shape plus required `kind`.
     - Pros: metrics and topology rows are pre-bucketed by schema; cleaner
       authoring model; avoids regular-metric/topology-row dedup ambiguity;
       handles topology-only scalar rows such as `systemUptime` cleanly.
     - Cons: larger schema departure from the upstream Datadog profile shape;
       ddsnmpcollector needs either a topology view that maps `topology:` rows
       into collectable rows or a collection API that accepts a row slice.
     - Risk: still needs topology-to-topology merge semantics, but no longer
       needs metrics-vs-topology merge semantics.
   - Recommendation: B for the clean end state. It better matches the user goal
     that topology compatibility is not a constraint and avoids encoding
     topology rows inside a `metrics:` list.

6. TopologyKind granularity for LLDP management-address rows.
   - Evidence:
     - `collector/snmp/ddsnmp/topology_classify.go:18-25` recognizes 18 topology
       metric names.
     - The current TODO enum listed 15 kinds and missed
       `_topology_lldp_loc_man_addr_entry`,
       `_topology_lldp_rem_man_addr_entry`, and
       `_topology_lldp_rem_man_addr_compat_entry`.
     - `collector/snmp_topology/topology_cache_metric_dispatch.go:11,15`
       dispatches local LLDP management addresses separately and maps both
       remote management-address variants to `updateLldpRemManAddr`.
   - A. Define one closed enum kind per current topology row shape, including
     the three LLDP management-address rows.
     - Pros: parity is obvious; compatibility/fallback row shapes are explicit;
       tests can map one current metric row to one new kind.
     - Cons: more enum values.
   - B. Collapse related LLDP rows into broader kinds such as `lldp_local` and
     `lldp_remote`.
     - Pros: shorter enum.
     - Cons: handlers must infer row shape from tags again.
     - Risk: recreates part of today's implicit-dispatch problem.
   - Recommendation: A.

7. `systemUptime` / `sysUptime` topology pathway.
   - Evidence:
     - `collector/snmp/ddsnmp/topology_classify.go:32-41` treats `sysUptime` and
       `systemUptime` as topology scalar metrics.
     - `collector/snmp_topology/collector.go:232-233` routes those metrics to
    the local topology device uptime updater.
     - `collector/snmp_topology/topology_cache_ingest.go:43-59` stores the value
       on the local topology device and labels it as `sys_uptime`.
     - `_system-base.yaml:11-44` defines `systemUptime` as a regular metric.
   - A. Superseded. The later accepted design removes `KindSysUptime` and uses
     `pkg/snmputils.GetSysUptime` from `snmp_topology` refresh.
   - B. Keep a dedicated scalar pathway outside the kind enum.
     - Pros: smaller dispatch change.
     - Cons: leaves a second special-case classifier.
   - C. Model uptime as metadata.
     - Pros: reads like device identity/enrichment.
     - Cons: metadata validation does not currently allow `sys_uptime`, and the
       topology freshness value is scalar evidence, not static identity.
   - Recommendation: A.

8. Topology use of top-level/global `metric_tags`.
   - Evidence:
     - `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:182` stores
       top-level `metric_tags` as `ProfileMetrics.Tags`.
     - Regular SNMP chart code copies those tags into chart labels in
       `collector/snmp/charts.go:213-214` and `collector/snmp/charts.go:270-271`.
     - Current topology ingestion uses per-row `m.Tags` and `pm.DeviceMetadata`;
       it does not consume `pm.Tags`
       (`collector/snmp_topology/topology_cache_ingest.go:11-30`).
   - A. In topology, collect global tags and apply them as local device/profile
     labels, not as dispatch keys on every topology row.
     - Pros: makes vendor/model/serial-like labels available to topology without
       polluting row-dispatch tags.
     - Cons: requires explicit topology ingest code for `pm.Tags`.
   - B. Merge global tags into every topology row's tag map.
     - Pros: every handler can see them.
     - Cons: unnecessary tag fan-out and possible collisions; dispatch does not
       need them.
   - C. Do not collect top-level/global `metric_tags` for topology by default.
     - Pros: lowest topology overhead.
     - Cons: contradicts Decision 4.A and drops useful identity labels.
   - Recommendation: A.

## Implementation Sub-Decisions

These are not new product/design forks. They are implementation constraints
surfaced by final external review and direct code inspection. They must be
honored by the implementation SOW.

1. Replace underscore-prefix hidden topology bucketing with explicit topology
   metrics.
   - Evidence:
     - `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:122-123` puts
       underscore-prefixed metrics into `pm.HiddenMetrics` and removes them from
       `pm.Metrics`.
     - `collector/snmp_topology/collector.go:220-224` currently ingests both
       `pm.HiddenMetrics` and `pm.Metrics`.
     - Decision 5.B removes the `_topology_*` / underscore-prefix contract from
       topology row selection.
   - Implementation direction:
     - Add an explicit `ProfileMetrics.TopologyMetrics []Metric` field.
     - Add a topology collection path in `ddsnmpcollector` parallel to scalar and
       table collection, fed from `Definition.Topology`.
     - Remove the topology dependency on underscore-prefix hidden metrics.
     - Treat `HiddenMetrics` as a general transport container, not a topology
       implementation detail. Other PRs may use it for non-topology collectors
       such as license processing.
     - Before deleting or redefining `HiddenMetrics`, audit all current branch
       and target-branch consumers. It can be refactored later as a separate
       cleanup, but topology implementation must not break non-topology users.

2. Define topology-to-topology merge identity as
   `(kind, table_identity, symbol_name)`.
   - Evidence:
     - `collector/snmp/ddsnmp/profile.go:194-252` currently deduplicates regular
       metrics by scalar identity or by table identity plus symbol name.
     - Decision 5.B adds `Definition.Topology`, so topology rows need equivalent
       inheritance/override semantics.
     - `collector/snmp/ddsnmp/profile.go:186-192` `Profile.merge()` currently
       merges metadata, metrics, top-level metric tags, and static tags, but not
       topology rows.
     - `collector/snmp/ddsnmp/load.go:197-214` calls `Profile.merge()` while
       resolving `extends:`, before resolver-time matched-set deduplication.
   - Implementation direction:
     - `kind` alone is too coarse.
     - `kind + table` is still too coarse for multi-symbol table rows.
     - Use `kind + table_identity + symbol_name`, mirroring the regular metric
       merge key shape.
     - Add `mergeTopology(base)` and call it from `Profile.merge()`.
     - `mergeTopology(base)` must implement the final Decision 2 behavior for
       inherited/overridden topology rows: explicit kind required, explicit
       derived topology wins, and conflicting explicit kinds reject.
     - Add `profile_test.go` coverage proving a profile extending a topology
       mixin inherits `Definition.Topology` rows.
     - Do not implement omitted-kind inheritance because validation rejects
       persisted topology rows without kind.

3. Implement Decision 8.A in `updateTopologyProfileTags`.
   - Evidence:
     - `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:182` stores
       top-level/global `metric_tags` as `pm.Tags`.
     - `collector/snmp_topology/topology_cache_ingest.go:11-30` already applies
       device metadata to topology local-device/profile state.
   - Implementation direction:
     - Extend `updateTopologyProfileTags` to also read `pm.Tags`.
     - Apply those values as local device/profile labels.
     - Do not merge `pm.Tags` into each topology row's dispatch tags.

4. Validate and reject metrics-only fields under top-level `topology:` rows.
   - Evidence:
     - `TopologyConfig` embeds/reuses `MetricsConfig`, which currently carries
       chart/export fields from `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go`.
   - Fields to reject on topology row anchor `symbol` / `symbols` entries:
     - `Options`;
     - `Symbol.ChartMeta`;
     - `Symbol.MetricType`;
     - `Symbol.Mapping`;
     - `Symbol.Transform`;
     - `Symbol.ScaleFactor`;
     - `Symbol.Format`;
     - `Symbol.ConstantValueOne`.
   - Implementation direction:
     - Add clear validation errors in `ddprofiledefinition/validation.go`.
     - Keep SNMP access/index/tag fields valid for topology rows.
     - Keep `metric_tags` extraction fields valid, including `MetricTags.Symbol.Format`,
       `MetricTags.Mapping`, and index/index_transform fields, because topology
       rows use those to build labels/evidence.

5. Split mutation handling by when the required information exists.
   - Evidence:
     - `collector/snmp/ddsnmp/profile.go:375-400` `enrichProfiles()` currently
       updates only `Definition.Metrics`.
     - `collector/snmp/ddsnmp/profile.go:402-440`
       `deduplicateMetricsAcrossProfiles()` currently deduplicates only
       `Definition.Metrics` and `Definition.VirtualMetrics`.
     - `deduplicateMetricsAcrossProfiles()` relies on the already-matched,
       sorted profile set; load time has individual catalog profiles, not a
       matched set.
   - Implementation direction:
     - Move per-profile/idempotent mutation into load-time/catalog
       compilation: `enrichProfiles()` and
       `handleCrossTableTagsWithoutMetrics()`.
     - Keep matched-set mutation at resolve-time:
       `deduplicateMetricsAcrossProfiles()` runs inside `Catalog.Resolve()` on
       catalog-cloned profiles before projection returns read-only views.
     - Extend enrichment, cross-table-tag synthesis, and resolve-time
       deduplication to process `Definition.Topology` where applicable.
     - Preserve metrics-path parity before deleting the old classifier/filter
       code.

## Recommended Clean Design - One Catalog With Explicit Projection

### TL;DR

"One catalog" means `ddsnmp` still owns a single SNMP profile catalog, one
inheritance model, and one resolver. "Explicit projection" means consumers ask
for a read-only view of resolved profile data, and topology rows are selected by
declared `TopologyKind`, not by filename, metric name, or tag prefix.

### Current shape

Regular SNMP metrics currently do:

```go
profiles := ddsnmp.FindProfiles(sysObjectID, sysDescr, manualProfiles)
profiles = selectCollectionProfiles(profiles) // strips topology by name/prefix
```

SNMP topology currently does:

```go
profiles := ddsnmp.FindProfiles(sysObjectID, sysDescr, manualProfiles)
profiles = selectTopologyRefreshProfiles(profiles) // keeps topology by name/prefix
```

Problems:

- `FindProfiles()` returns mixed profile data.
- `collector/snmp` and `collector/snmp_topology` each mutate those profiles with
  mirrored filters.
- The filters rely on hardcoded `_topology_*` metric names and tag/metadata
  prefix heuristics.
- VLAN-context topology has a separate direct-load path that hardcodes topology
  mixin filenames.

### End-state runtime shape

```go
catalog := ddsnmp.DefaultCatalog()

resolved := catalog.Resolve(ddsnmp.ResolveRequest{
    SysObjectID:    sysObjectID,
    SysDescr:       sysDescr,
    ManualProfiles: manualProfiles,
    Policy:         ddsnmp.ManualFallback, // regular metrics per Decision 1.D
})

metricView := resolved.Project(ddsnmp.ConsumerMetrics)
```

Topology uses the same resolved set shape, with augment semantics for manual
profiles per Decision 1.D:

```go
resolved := catalog.Resolve(ddsnmp.ResolveRequest{
    SysObjectID:    device.SysObjectID,
    SysDescr:       device.SysDescr,
    ManualProfiles: device.ManualProfiles,
    Policy:         ddsnmp.ManualAugment, // topology per Decision 1.D
})

topologyView := resolved.Project(ddsnmp.ConsumerTopology)
```

VLAN-context is not a consumer. It is topology projection plus a transport
rewrite and a kind filter:

```go
var vlanScopableKinds = map[ddsnmp.TopologyKind]bool{
    ddsnmp.KindFdbEntry:           true,
    ddsnmp.KindQbridgeFdbEntry:    true,
    ddsnmp.KindStpPort:            true,
    ddsnmp.KindBridgePortIfIndex:  true,
    ddsnmp.KindIfName:             true,
}

vlanView := resolved.
    Project(ddsnmp.ConsumerTopology).
    FilterByKind(vlanScopableKinds)
```

Responsibilities:

- `Catalog`: load all profile YAMLs, resolve `extends`, validate, compile
  immutable profile data, and keep path precedence rules in one place.
- `Resolve`: match profiles for a device using `sysObjectID`, `sysDescr`, manual
  profile list, and selected manual-profile policy.
- `Project`: produce a consumer-specific read-only view from the resolved set.
- `FilterByKind`: narrow topology views by closed `TopologyKind` when a runtime
  path needs only a subset, such as VLAN-context polling.

### YAML contract

Decision 5 selected a separate top-level `topology:` list. Regular metrics stay
under `metrics:`. Topology rows reuse the same SNMP row shape but must declare a
closed-enum `kind`.

```yaml
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets

topology:
  - kind: lldp_rem
    MIB: LLDP-MIB
    table:
      OID: 1.0.8802.1.1.2.1.4.1
      name: lldpRemTable
    symbols:
      - OID: 1.0.8802.1.1.2.1.4.1.1.6
        name: lldpRemPortIdSubtype
    metric_tags:
      - tag: lldp_loc_port_num
        index: 2
      - tag: lldp_rem_index
        index: 3
```

Metadata fields and top-level/global `metric_tags` may use `consumers`, but only
for narrowing. The recommended default is both `metrics` and `topology`:

```yaml
metadata:
  device:
    fields:
      vendor:
        value: Cisco

      lldp_loc_chassis_id:
        consumers: [topology]
        symbol:
          OID: 1.0.8802.1.1.2.1.3.2.0
          name: lldpLocChassisId
```

### Go model

```go
type ProfileConsumer string

const (
    ConsumerMetrics  ProfileConsumer = "metrics"
    ConsumerTopology ProfileConsumer = "topology"
)

type TopologyKind string

const (
    KindLldpLocPort            TopologyKind = "lldp_loc_port"
    KindLldpLocManAddr         TopologyKind = "lldp_loc_man_addr"
    KindLldpRem                TopologyKind = "lldp_rem"
    KindLldpRemManAddr         TopologyKind = "lldp_rem_man_addr"
    KindLldpRemManAddrCompat   TopologyKind = "lldp_rem_man_addr_compat"
    KindCdpCache               TopologyKind = "cdp_cache"
    KindIfName                 TopologyKind = "if_name"
    KindIfStatus               TopologyKind = "if_status"
    KindIfDuplex               TopologyKind = "if_duplex"
    KindIpIfIndex              TopologyKind = "ip_if_index"
    KindBridgePortIfIndex      TopologyKind = "bridge_port_if_index"
    KindFdbEntry               TopologyKind = "fdb_entry"
    KindQbridgeFdbEntry        TopologyKind = "qbridge_fdb_entry"
    KindQbridgeVlanEntry       TopologyKind = "qbridge_vlan_entry"
    KindStpPort                TopologyKind = "stp_port"
    KindVtpVlan                TopologyKind = "vtp_vlan"
    KindArpEntry               TopologyKind = "arp_entry"
    KindArpLegacyEntry         TopologyKind = "arp_legacy_entry"
)

type ResolveRequest struct {
    SysObjectID    string
    SysDescr       string
    ManualProfiles []string
    Policy         ManualProfilePolicy
}

func (c *Catalog) Resolve(req ResolveRequest) *ResolvedProfileSet
func (r *ResolvedProfileSet) Project(consumer ProfileConsumer) ProjectedView
func (v ProjectedView) FilterByKind(kinds map[TopologyKind]bool) ProjectedView
```

Profile schema additions:

```go
type ProfileDefinition struct {
    Metrics  []MetricsConfig `yaml:"metrics,omitempty" json:"metrics,omitempty"`
    Topology []TopologyConfig `yaml:"topology,omitempty" json:"topology,omitempty"`
    // existing fields...
}

type TopologyConfig struct {
    Kind TopologyKind `yaml:"kind" json:"kind"`
    MetricsConfig    `yaml:",inline"`
}

type MetadataField struct {
    Consumers ConsumerSet `yaml:"consumers,omitempty" json:"consumers,omitempty"`
    // existing fields...
}
```

Projection must avoid per-call deep cloning. Preferred implementation shape:

- catalog-owned profiles are immutable after load;
- projection returns read-only profile views or precomputed buckets;
- existing mutation in `ddsnmpcollector.New()` via
  `handleCrossTableTagsWithoutMetrics(prof)` moves into catalog/profile
  compilation before profiles are shared;
- `enrichProfiles()` and `deduplicateMetricsAcrossProfiles()` currently run in
  the per-call `FindProfiles()`/`FinalizeProfiles()` path and also mutate cloned
  profiles. `enrichProfiles()` moves to load-time/catalog compilation.
  `deduplicateMetricsAcrossProfiles()` stays resolve-time because it needs the
  matched, sorted profile set; it must run on catalog-cloned profiles inside
  `Catalog.Resolve()` before read-only projection views are returned.

### Topology dispatch

Topology ingestion should dispatch by `TopologyKind`, not `Metric.Name`.

```go
type topologyHandler interface {
    Kind() ddsnmp.TopologyKind
    Ingest(c *topologyCache, metric ddsnmp.Metric)
}

var topologyHandlers = map[ddsnmp.TopologyKind]topologyHandler{}

func registerTopologyHandler(h topologyHandler) {
    topologyHandlers[h.Kind()] = h
}
```

Each topology domain file registers itself:

```go
func init() {
    registerTopologyHandler(fdbHandler{})
}
```

The collector metric produced from a topology profile row must carry the
declared kind:

```go
type Metric struct {
    Name         string
    Value        float64
    Tags         map[string]string
    TopologyKind ddsnmp.TopologyKind
}

type ProfileMetrics struct {
    Source          string
    DeviceMetadata  map[string]MetaTag
    Tags            map[string]string
    Metrics         []Metric
    TopologyMetrics []Metric
    HiddenMetrics   []Metric // general-purpose delivery for non-topology underscore-prefixed metrics; preserved as-is
    Stats           CollectionStats
}
```

### Expected cleanup

- Remove `collector/snmp/topology_profile_filter.go`.
- Remove `collector/snmp_topology/profile_filter.go`.
- Remove `collector/snmp/ddsnmp/topology_classify.go`.
- Remove metric-name dispatch constants where `TopologyKind` replaces them.
- Replace `collector/snmp_topology/topology_vlan_context_collect.go:16-28`
  hardcoded `LoadProfileByName()` calls with topology projection plus
  `FilterByKind`.
- Remove `ddsnmp.FinalizeProfiles()` if VLAN-context no longer calls
  `LoadProfileByName()`.
- Remove dead constants in `collector/snmp_topology/topology_profiles.go`;
  currently only `fdbArpProfileName` and `stpProfileName` are live.
- Split `_std-cdp-mib.yaml` into real metrics and
  `_std-topology-cdp-mib.yaml`.
- Update `collector/snmp/profile-format.md` and
  `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.

### Key risks

- `TopologyKind` plumbing through `ddsnmpcollector` is the deepest code change.
- Merge semantics for inherited/overridden topology rows follow Decision 2.B and
  must be tested carefully.
- Metadata/global-tag default semantics follow Decision 4.A and runtime topology
  handling follows Decision 8.A.
- Regular SNMP manual-profile behavior cannot change accidentally.
- Read-only projection requires moving current profile mutations out of the
  per-resolve/per-collector path.
- The reason to avoid per-call cloning is primarily correctness and API clarity,
  not raw throughput; topology refresh is low frequency compared to SNMP walk
  cost.

## Claude Review - Adopted Notes

Claude reviewed the design with independent subagent input. The review was
checked against local code and is largely correct. The recommended design above
has been updated to reflect these notes.

### Accepted Corrections

1. Topology rows should not need `consumers: [topology]`.
   - Decision 5.B supersedes the earlier `metrics[].topology` shape.
   - Regular metric rows stay under `metrics:`.
   - Topology rows live under top-level `topology:` and carry required `kind`.
   - This removes duplicate YAML such as `consumers: [topology]` plus a topology
     kind.

   Revised topology row shape:

   ```yaml
   topology:
     - kind: lldp_rem
       MIB: LLDP-MIB
       table:
         OID: 1.0.8802.1.1.2.1.4.1
         name: lldpRemTable
       symbols:
         - OID: 1.0.8802.1.1.2.1.4.1.1.6
           name: lldpRemPortIdSubtype
       metric_tags:
         - tag: lldp_loc_port_num
           index: 2
         - tag: lldp_rem_index
           index: 3
   ```

   Revised regular metric shape stays unchanged:

   ```yaml
   metrics:
     - MIB: IF-MIB
       table:
         OID: 1.3.6.1.2.1.2.2
         name: ifTable
       symbols:
         - OID: 1.3.6.1.2.1.2.2.1.10
           name: ifInOctets
   ```

2. VLAN-context should not be modeled as a separate consumer.
   - VLAN-context is a transport modifier: SNMPv1/v2 community is rewritten to
     `community@vlan`, SNMPv3 context is rewritten to `vlan-<id>`
     (`collector/snmp_topology/topology_vlan_context_collect.go:57-72`).
   - Use topology projection plus a kind filter instead:

   ```go
   view := resolved.Project(ddsnmp.ConsumerTopology).FilterByKind(vlanScopableKinds)
   ```

   Evidence:
   - `collector/snmp_topology/topology_vlan_context_collect.go:16-28`
     hardcodes profile loading.
   - `collector/snmp_topology/topology_vlan_context_ingest.go:30-37`
     hardcodes the VLAN-context metric allowlist.

3. Topology row `kind` must be a closed enum validated at profile load.
   - An open string repeats the current typo-prone `_topology_*` contract.
   - Adding a topology kind should require:
     - YAML topology row `kind`;
     - Go enum value;
     - registered ingestion handler;
     - tests.

4. Topology dispatch should move from metric-name switch to a handler registry
   keyed by `TopologyKind`.
   - Current central switch:
     `collector/snmp_topology/topology_cache_metric_dispatch.go:7-35`.
   - Cleaner shape:

   ```go
   type topologyHandler interface {
       Kind() ddsnmp.TopologyKind
       Ingest(c *topologyCache, metric ddsnmp.Metric)
   }

   var topologyHandlers = map[ddsnmp.TopologyKind]topologyHandler{}

   func registerTopologyHandler(h topologyHandler) {
       topologyHandlers[h.Kind()] = h
   }
   ```

   Then each topology domain file registers itself:

   ```go
   func init() {
       registerTopologyHandler(fdbHandler{})
   }
   ```

5. `ddsnmp.Metric` needs to carry `TopologyKind`.
   - Today the topology kind is encoded only in `Metric.Name`.
   - The collector path must thread kind from parsed profile metric config through
     table/scalar collectors to topology ingestion.
   - This is one of the highest-risk implementation areas.

6. Split `_std-cdp-mib.yaml`.
   - It currently contains both real CDP metrics and `_topology_cdp_cache_entry`.
   - Split into:
     - `_std-cdp-mib.yaml` for real metrics;
     - `_std-topology-cdp-mib.yaml` for topology.
   - This mirrors the LLDP pattern where `_std-lldp-mib.yaml` extends
     `_std-topology-lldp-mib.yaml`.

7. Projection should avoid deep-cloning per call.
   - `FindProfiles()` already clones matched profiles
     (`collector/snmp/ddsnmp/profile.go:62-65`).
   - Re-cloning per consumer/projection per device poll is less important than
     correctness: shared projected views must not be mutable/torn across
     consumers.
   - Preferred shape: immutable catalog/resolved profiles plus read-only projected
     views or load-time/precomputed buckets.
   - Required cleanup: `ddsnmpcollector.New()` currently mutates profiles by
     calling `handleCrossTableTagsWithoutMetrics(prof)`
     (`collector/snmp/ddsnmp/ddsnmpcollector/collector.go:37-39`). That rewrite
     must move into catalog/profile compilation, or projected views cannot safely
     share profile pointers.

8. Metadata/global tag defaults should differ from metric defaults.
   - Regular metric rows under `metrics:` are metrics-only.
   - Topology rows under `topology:` are topology-only unless the future schema
     explicitly supports sharing.
   - Metadata fields and top-level/global `metric_tags` default to both metrics
     and topology because they often identify the device for both consumers.
   - Narrow explicitly when needed:

   ```yaml
   metadata:
     device:
       fields:
         vendor:
           value: Cisco

         lldp_loc_chassis_id:
           consumers: [topology]
           symbol:
             OID: 1.0.8802.1.1.2.1.3.2.0
             name: lldpLocChassisId
   ```

The concrete Go model, YAML contract, and resolved decisions have been folded
into the sections above so there is only one canonical design in this TODO.

## Second Claude Review - Evaluation

Each point from the second Claude review was checked against local code and is
accepted/rejected as follows.

1. Verdict "agree with changes".
   - Accept.
   - The design is directionally correct. The review exposed additional
     decisions, which the user has since resolved.

2. "No stale leftovers, no contradictions".
   - Partially accept.
   - Accept that stale `ConsumerTopologyVLANContext` and metric-block
     `consumers: [topology]` examples were removed.
   - Reject the absolute wording: the Plan section was stale and has now been
     updated.

3. `_std-cdp-mib.yaml` is the only mixed regular/topology profile file.
   - Accept.
   - Current evidence: `_std-cdp-mib.yaml` has regular CDP metrics followed by
     `_topology_cdp_cache_entry`.

4. Decision 4.A migration count is effectively zero for metadata narrowing.
   - Accept with narrower wording.
   - The shipped metadata fields reviewed are device identity/enrichment fields
     that topology can reasonably consume. Top-level/global `metric_tags` still
     need explicit topology runtime handling; see Decision 8.

5. TopologyKind enum is short by three LLDP management-address rows.
   - Accept.
   - The enum example was expanded and Decision 6 now records the granularity
     decision.

6. `systemUptime` / `sysUptime` has no path in the new design.
   - Accept.
   - Superseded by user decision 26. Topology uptime uses
     `pkg/snmputils.GetSysUptime`, not a topology kind.

7. Top-level/global `metric_tags` default-both consequence is unspecified.
   - Accept the gap; correct one detail.
   - Current regular SNMP charting copies `m.Profile.Tags` into chart labels.
     Current topology ingest does not consume `pm.Tags`, so these tags become
     available to topology only if the cutover deliberately applies them.
   - Decision 8 records the recommended behavior.

8. Plan section is stale.
   - Accept.
   - Plan now says the design recommendation and decisions are completed, with
     one final external review pending.

9. Cleanup list is incomplete.
   - Accept.
   - Added `FinalizeProfiles()` and dead `topology_profiles.go` constants to the
     cleanup list.

10. Mutation inventory is bigger than `handleCrossTableTagsWithoutMetrics`.
    - Accept.
    - Added `enrichProfiles()` and `deduplicateMetricsAcrossProfiles()` to the
      load-time/catalog-compilation requirement.
    - Accept the probable `removeConstantMetrics()` value-copy bug as a separate
      follow-up candidate, not part of the design decision itself.

11. `TopologyKind` plumbing is deeper than the TODO suggested.
    - Accept.
    - The important implementation detail is that table/scalar build sites have
      parent `MetricsConfig` access, while the current builders receive only
      `SymbolConfig`.

12. Per-call cloning performance cost is overstated.
    - Accept.
    - The TODO now frames immutable projection primarily as correctness/API
      clarity, not throughput.

13. Alternative 1: top-level `topology:` list.
    - Accept as a real architectural fork and currently recommend it.
    - Reject the claim that it eliminates all topology merge semantics. It
      eliminates metrics-vs-topology dedup ambiguity, but topology-to-topology
      override/merge behavior still needs a rule.
    - Decision 5 now captures the choice.

14. Previously recommended choices 1.D, 2.B, 3.C, 4.A.
    - Accept.
    - Since Decision 5.B is now selected, Decision 2 narrows from
      metrics-vs-topology merge behavior to topology-to-topology override
      behavior.

15. Recommended implementation/test plan.
    - Accept with adjustments.
    - The SOW should phase schema choice first, then parity-protected load-time
      mutation movement, then cutover and deletion. Tests listed in the review
      are the right baseline.

## Third Claude Review - Evaluation

Third external review verdict was `NEEDS ADJUSTMENTS` before implementation,
with decisions `1.D`, `2.B`, `3.C`, `4.A`, `5.B`, `6.A`, `7.A`, and `8.A`
still accepted as coherent. Each finding was checked against code and handled as
follows.

1. `pm.HiddenMetrics` bucketing must be redesigned.
   - Accept.
   - Added Implementation Sub-Decision 1.
   - The implementation should introduce explicit `pm.TopologyMetrics` instead
     of relying on underscore-prefix hidden metrics for topology.

2. Topology-to-topology merge identity must be specified.
   - Accept.
   - Added Implementation Sub-Decision 2.
   - Use `(kind, table_identity, symbol_name)`.

3. Runtime hook point for Decision 8.A must be named.
   - Accept.
   - Added Implementation Sub-Decision 3.
   - Extend `updateTopologyProfileTags` to apply `pm.Tags`.

4. Validation must reject metrics-only fields under `topology:` rows.
   - Accept.
   - Added Implementation Sub-Decision 4.

5. `enrichProfiles` and `deduplicateMetricsAcrossProfiles` must extend to
   `Definition.Topology`.
   - Accept.
   - Added Implementation Sub-Decision 5.

6. Citation tweaks.
   - Accept.
   - Updated `_system-base.yaml`, `topology_cache_ingest.go`, and
     `updateTopologyProfileTags` references.

7. Stale data check.
   - Accept.
   - No current-design stale `metrics[].topology`,
     `ConsumerTopologyVLANContext`, or default `[metrics]` semantics remain.
     Remaining mentions are historical review/option context.

## Fourth Claude Review - Evaluation

Fourth external review verdict was `READY TO PROCEED`. The review confirmed:

- Decisions `1.D`, `2.B`, `3.C`, `4.A`, `5.B`, `6.A`, `7.A`, and `8.A` remain
  coherent against the code.
- `HiddenMetrics` is correctly recorded as a general-purpose delivery container,
  not topology-owned.
- Topology should introduce `ProfileMetrics.TopologyMetrics` and stop reading
  topology evidence from `pm.HiddenMetrics`.
- The generic `HiddenMetrics` path must remain intact unless a later, separate
  cleanup audits and migrates all non-topology consumers.
- The five implementation sub-decisions are concrete enough to seed the SOW
  Pre-Implementation Gate.

Non-blocking adjustments from this review were applied:

- Tightened the `ProfileMetrics.HiddenMetrics` comment to avoid implying it can
  be deleted as part of the topology cleanup.
- Added the existing non-topology hidden-metric preservation test as an explicit
  required test.

## Fifth Claude Review - Evaluation

Readiness review verdict was `NEEDS ADJUSTMENTS`. Each point was checked
against local code and accepted/rejected as follows.

Blocking adjustments:

1. B1: Phase 2 conflates load-time and resolve-time.
   - Accept.
   - Evidence: `collector/snmp/ddsnmp/profile.go:402-440`
     `deduplicateMetricsAcrossProfiles()` explicitly relies on the already
     sorted matched profile set.
   - Plan update: split mutation handling. `enrichProfiles()` and
     `handleCrossTableTagsWithoutMetrics()` move to load-time/catalog
     compilation. `deduplicateMetricsAcrossProfiles()` remains resolve-time
     inside `Catalog.Resolve()` on catalog-cloned profiles.

2. B2: Phase 3 alone breaks topology.
   - Accept.
   - Evidence: current topology still reads `metrics:` rows through
     `snmp_topology/profile_filter.go`, `snmp_topology/collector.go`, and
     `_topology_*` classifier/hidden-metric paths.
   - Plan update: choose option A. Phases 3-6 are one logical topology cutover;
     Phase 3 must not ship independently.

3. B3: Phase 5 has no double-counting gate.
   - Accept.
   - Evidence: `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:122-123`
     collects underscore-prefixed regular metrics into `pm.HiddenMetrics`;
     adding `pm.TopologyMetrics` without an invariant could deliver one topology
     row twice during transition.
   - Plan update: add invariant and test that topology rows cannot appear in
     both `pm.HiddenMetrics` and `pm.TopologyMetrics` in one poll.

4. B4: `handleCrossTableTagsWithoutMetrics()` is not extended to topology.
   - Accept.
   - Evidence: `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:241-283`
     scans and appends only `Definition.Metrics`.
   - Plan update: extend cross-table-tag synthesis to scan both metrics and
     topology, and place synthesized rows on the owning slice.

5. B5: `validateEnrichVirtualMetrics()` cannot enforce Decision 3.C.
   - Accept.
   - Evidence: `collector/snmp/ddsnmp/ddprofiledefinition/validation.go:77`
     calls `validateEnrichVirtualMetrics(p.Metrics, p.VirtualMetrics)`; topology
     rows are not visible to the validator.
   - Plan update: extend the validation signature to include topology rows and
     reject virtual metric sources that resolve to topology rows.

6. B6: `systemUptime` move-vs-duplicate was unresolved.
   - Original option C was accepted during review, then superseded by user
     decision 26 during implementation.
   - Evidence: `config/go.d/snmp.profiles/default/_system-base.yaml:11-44`
     defines regular exported `systemUptime` metrics.
   - Plan update: do not modify `_system-base.yaml`; topology queries uptime
     through `pkg/snmputils.GetSysUptime`, not through a topology kind or YAML
     `topology:` row.

Non-blocking suggestions:

1. S1: Pin builder/threading shape.
   - Accept.
   - Plan update: prefer a topology collection wrapper that stamps
     `TopologyKind` after existing scalar/table emission; do not widen regular
     builder signatures unless the SOW records why.

2. S2: Name the validator traversal mechanism.
   - Accept.
   - Plan update: extend existing `SymbolContext` with topology value-symbol
     context or an equivalent topology-row flag; keep `MetricTagSymbol`
     allowances.

3. S3: Phase 7 deletion list misses test files.
   - Accept.
   - Plan update: add explicit delete/rewrite/preserve test-impact list.

4. S4: Validation should reject underscore-prefix names on topology rows.
   - Accept.
   - Plan update: reject underscore-prefixed topology row anchor symbol names so
     topology rows cannot also enter generic `HiddenMetrics`.

5. S5: Phase 6 should require side-by-side runtime parity.
   - Accept.
   - Plan update: add fixture-level parity gate before deleting the old path.

6. S6: Mutation-isolation test is underspecified.
   - Accept.
   - Plan update: specify a test that resolves twice, mutates one projected view,
     and proves the second view plus a fresh resolve are unaffected.

## Sixth Claude Review - Evaluation

Readiness review verdict was `NEEDS ADJUSTMENTS`. B1-B6 and S1-S6 remained
clean after the prior edits, but a fresh blocker and four tightenings were
reported. Each point was checked against local code and accepted/rejected as
follows.

1. N1: `Profile.merge()` is not extended to the new topology slice.
   - Accept.
   - Evidence:
     - `collector/snmp/ddsnmp/profile.go:186-192` currently calls
       `mergeMetadata()` and `mergeMetrics()`, then appends top-level
       metric/static tags. It does not merge topology rows.
     - `collector/snmp/ddsnmp/load.go:197-214` calls `Profile.merge()` while
       resolving `extends:`.
     - `config/go.d/snmp.profiles/default/_cisco-base.yaml:3-10` extends
       topology mixins, so missing `mergeTopology()` would drop inherited
       topology rows before projection.
   - Plan update: add `mergeTopology(base)` to `Profile.merge()` using
     `(kind, table_identity, symbol_name)` plus Decision 2.B semantics. Add
     `profile_test.go` coverage proving vendor/root topology-mixin inheritance.

2. N2: enumerate clone targets explicitly.
   - Accept.
   - Evidence: `ProfileDefinition.Clone()`, `MetadataField.Clone()`,
     `MetricsConfig.Clone()`, and related clone helpers are explicit code paths
     under `collector/snmp/ddsnmp/ddprofiledefinition/`.
   - Plan update: Phase 1 now names `ProfileDefinition.Clone()`,
     `TopologyConfig.Clone()`, `MetadataField.Clone()`, and consumer-set clone
     paths explicitly.

3. N3: state JSON/YAML tag convention for new fields.
   - Accept.
   - Evidence: existing profile schema structs consistently define `yaml` and
     `json` tags for persisted fields, with runtime-only fields tagged `-`.
   - Plan update: new persisted fields must use matching YAML/JSON names and
     `omitempty` where optional; runtime-only fields must be `yaml:"-" json:"-"`.

4. N4: VLAN-context ingest creates synthetic metrics without `TopologyKind`.
   - Accept.
   - Evidence: `collector/snmp_topology/topology_vlan_context_ingest.go:22-25`
     constructs `ddsnmp.Metric{Name, Tags}` before calling
     `updateTopologyCacheEntry()`.
   - Plan update: after kind-keyed dispatch, VLAN-context synthetic metrics must
     preserve/populate `TopologyKind`.

5. N5: pin `vlanScopableKinds`.
   - Accept.
   - Evidence:
     - `collector/snmp_topology/topology_vlan_context_ingest.go:30-37` currently
       accepts only interface name, bridge-port map, FDB, and STP rows for VLAN
       context ingest.
   - Plan update: Phase 6 pins `vlanScopableKinds` to `KindIfName`,
     `KindBridgePortIfIndex`, `KindFdbEntry`, and `KindStpPort`.

6. P1: abstract topology mixins as roots may surprise validation.
   - Accept as a note, not a blocker.
   - Plan update: topology mixins may become topology-only/abstract after YAML
     migration; tests should validate inherited/root use intentionally rather
     than assuming those mixins produce regular metrics by themselves.

7. P2: `TopologyConfig` embeds `MetricsConfig`, so `IsScalar()` / `IsColumn()`
   predicates remain usable for topology merge/dedup keys.
   - Accept as an implementation note.
   - Plan update: no separate implementation step needed beyond using the
     existing row-shape predicates when implementing topology row identity.

8. P3: `removeConstantMetrics()` only walks `Definition.Metrics`.
   - Accept as an implementation note.
   - Evidence: topology validation will reject `ConstantValueOne` on topology
     row anchor symbols.
   - Plan update: do not extend `removeConstantMetrics()` to topology unless
     topology validation changes; keep the existing value-copy bug as a separate
     follow-up unless this SOW touches it.

9. P4: mutation isolation must include public `Profile.Definition.Topology`.
   - Accept.
   - Plan update: mutation-isolation tests must mutate topology nested state as
     well as metrics/metadata state.

10. NB: `_std-cdp-mib.yaml` split needs extender inventory.
    - Accept.
    - Evidence: current default extenders are
      `config/go.d/snmp.profiles/default/_cisco-base.yaml:6` and
      `config/go.d/snmp.profiles/default/cisco-sb.yaml:5`.
    - Plan update: Phase 3 must inventory every extender of `_std-cdp-mib.yaml`
      and update each profile that should keep CDP topology to extend
      `_std-topology-cdp-mib.yaml` too.

## Seventh Claude Review - Evaluation

Readiness review verdict was `READY TO WRITE CODE`. B1-B6, S1-S6, N1-N5, and
the recorded minor notes were verified as clean. Four non-blocking hygiene
tightenings were accepted and folded into the plan.

1. NB1: extend stale-reference scans with every topology classifier helper.
   - Accept.
   - Plan update: Phase 7 stale scans now include
     `MetricTagConfigContainsTopologyData`,
     `MetadataFieldContainsTopologyData`, `MetadataContainsTopologyData`,
     `SysobjectIDMetadataContainsTopologyData`, `ProfileContainsTopologyData`,
     and `ProfileHasCollectionData`.

2. NB2: pin `Catalog` / `Resolve` / `Project` package location.
   - Accept.
   - Plan update: API lives in `collector/snmp/ddsnmp`, next to the existing
     profile loader, `FindProfiles()`, and profile model.

3. NB3: clarify where `Consumers` lives for global metric tags.
   - Accept with clean implementation shape.
   - Plan update: use a top-level/global metric-tag wrapper that carries
     `Consumers`, rather than adding consumer semantics to per-row
     `MetricTagConfig`. Per-row metric tags inherit their row's consumer.

4. NB4: state the post-cutover topology ingest loop.
   - Accept.
   - Plan update: after cutover, `ingestTopologyProfileMetrics` iterates only
     `pm.TopologyMetrics` for kind-keyed dispatch. Uptime is queried separately
     through `pkg/snmputils.GetSysUptime`.

## Plan

1. Record the task scope in this TODO. Completed.
2. Review SOW/spec/project skill context that may constrain topology/profile work. Completed.
3. Spawn 3 GPT-5.5 subagents with independent read-only review scopes. Completed.
4. Inspect profile loading code, embedded profile assets, runtime config paths, and tests. Completed.
5. Compare local findings with subagent findings. Completed.
6. Present a clean design recommendation with evidence, risks, and likely implementation/test/doc impact. Completed.
7. Resolve design decisions 1-8 with the user before implementation. Completed.
8. Run one final read-only external review and confirm ready-to-proceed versus needs-adjustments. Completed.
9. Start implementation only after creating/updating the required SOW with a
   concrete Pre-Implementation Gate. Completed.
10. Write a step-by-step implementation checklist before code starts. Completed.
11. Run a read-only Claude readiness review of the step-by-step checklist.
    Completed.
12. Start implementation only after the readiness review says `READY TO WRITE
    CODE`, or after any required adjustments are recorded here and in the SOW.
    Completed. Proceeding to SOW activation and Phase 0 baseline checks.

### Implementation Progress

- 2026-05-06: Phase 0 completed. Baseline focused tests passed; the topology
  package required an escalated rerun because the sandbox could not write the
  Go build cache.
- 2026-05-06: Phase 1 completed. Added profile consumers, closed topology kind
  schema, top-level `topology:` row config, metadata/global-tag consumers,
  clone support, validation rules, and transitional wrapper compatibility for
  existing callers. Focused tests passed:
  - `go test ./collector/snmp/ddsnmp/ddprofiledefinition`
  - `go test ./collector/snmp/ddsnmp/ddsnmpcollector`
  - `go test ./collector/snmp/...`
  - `go test ./collector/snmp_topology/...`
- 2026-05-06: Phase 2 completed. Moved cross-table synthetic row preparation to
  `ddsnmp` profile preparation, extended topology merge/dedup/enrichment, and
  kept resolve-time cross-profile dedup separate from load-time mutation. The
  projection mutation-isolation test remains Phase 4 work because projection
  views do not exist yet. Focused tests passed:
  - `go test ./collector/snmp/ddsnmp/...`
  - `go test ./collector/snmp/...`
  - `go test ./collector/snmp_topology/...`
- 2026-05-06: Phases 3-6 completed as one topology cutover. Migrated topology
  mixins to top-level `topology:`, split CDP topology into
  `_std-topology-cdp-mib.yaml`, added catalog resolve/project/filter APIs,
  threaded `TopologyKind` into `ProfileMetrics.TopologyMetrics`, cut over
  regular SNMP/topology/VLAN-context call sites, and replaced topology
  metric-name dispatch with kind dispatch. Focused tests passed:
  - `go test ./collector/snmp/ddsnmp/...`
  - `go test ./collector/snmp/...`
  - `go test ./collector/snmp_topology/...`
- 2026-05-06: User selected option A for topology uptime after removing
  `KindSysUptime`. Removed `KindSysUptime`; topology now queries uptime through
  `pkg/snmputils.GetSysUptime`, while regular SNMP uptime remains profile-driven
  in `_system-base.yaml`. Focused tests passed:
  - `go test -count=1 ./pkg/snmputils`
  - `go test -count=1 ./collector/snmp/ddsnmp/...`
  - `go test -count=1 ./collector/snmp_topology/...`
- 2026-05-06: Phase 7 cleanup/artifacts completed. Deleted the topology
  classifier/filter files and hardcoded topology profile constants, rewrote
  affected tests, updated `collector/snmp/profile-format.md`, updated
  `.agents/skills/project-snmp-profiles-authoring/SKILL.md`, and created
  `.agents/sow/specs/snmp-profile-projection.md`. Runtime topology no longer
  uses `LoadProfileByName()` or `HiddenMetrics`; `LoadProfileByName()` remains
  only as an intentional abstract-profile helper for tests/programmatic checks,
  and `FinalizeProfiles()` remains for synthetic collector tests that build
  profiles outside the catalog.

### Implementation Phases For SOW

1. Schema and API surface, no behavior change.
   - Add `Topology []TopologyConfig` to `ProfileDefinition`.
   - Add the 18-value `TopologyKind` enum. Do not include `sys_uptime`.
   - Add `MetadataField.Consumers`.
   - Add validation for unknown topology kinds and metrics-only fields under
     `topology:`.
   - Add parse, clone, and validation coverage for the new schema.
   - Catalog, resolve, and projection APIs are introduced in Phase 4.

2. Split mutation handling between load-time/catalog compilation and
   resolve-time matched-set processing.
   - Move per-profile/idempotent mutation to load-time:
     `enrichProfiles` and `handleCrossTableTagsWithoutMetrics`.
   - Extend `Profile.merge()` with `mergeTopology(base)` so `extends:` loads
     inherit topology rows from mixins.
   - Extend cross-table-tag synthesis to scan both `Definition.Metrics` and
     `Definition.Topology`, placing synthesized entries on the owning slice.
   - Keep `deduplicateMetricsAcrossProfiles` at resolve-time inside
     `Catalog.Resolve()` because it needs the matched, sorted profile set.
   - Extend resolve-time deduplication to `Definition.Topology`.
   - Preserve metrics-path parity with current `FindProfiles`.

3. Profile YAML migration.
   - Move `_topology_*` rows from `metrics:` to top-level `topology:` in the
     topology mixins.
   - Split `_std-cdp-mib.yaml` into regular CDP metrics and
     `_std-topology-cdp-mib.yaml`.
   - Add `kind:` to every topology row.
   - Keep metadata/global tag annotations minimal per Decision 4.A.
   - Do not modify `_system-base.yaml`; `systemUptime` remains a regular metric.
   - Phases 3-6 are one logical topology cutover. Do not ship Phase 3 by itself.

4. Catalog/Resolve/Project introduction with parity tests.
   - Keep old `FindProfiles` temporarily.
   - Prove `Catalog.Resolve(...).Project(ConsumerMetrics)` matches current
     `selectCollectionProfiles(FindProfiles(...))`.
   - Prove `Project(ConsumerTopology)` matches current topology profile
     selection after YAML migration.

5. Plumb topology collection through `ddsnmpcollector`.
   - Add `Metric.TopologyKind`.
   - Add `ProfileMetrics.TopologyMetrics`.
   - Thread topology kind through scalar/table builders from parent row config.
   - Add explicit topology collection from `Definition.Topology`.
   - Remove topology's dependency on underscore-prefix hidden metrics.
   - Preserve generic `HiddenMetrics` behavior for non-topology underscore
     metrics.
   - Query uptime through `pkg/snmputils.GetSysUptime`, not through a topology
     kind or topology projection of regular `systemUptime` rows.
   - Prefer a topology collection wrapper that stamps `TopologyKind` without
     widening regular scalar/table builder signatures.

6. Cut over call sites and dispatch.
   - Switch `collector/snmp/profile_sets.go`, `collector/snmp_topology/collector.go`,
     and `collector/snmp_topology/topology_vlan_context_collect.go`.
   - Replace metric-name dispatch switch with handler registry keyed by
     `TopologyKind`.
   - Extend `updateTopologyProfileTags` to apply `pm.Tags` as local
     device/profile labels.
   - Add a side-by-side fixture-level parity test before deleting the old path.

7. Delete dead code and update artifacts.
   - Delete `topology_classify.go`, `topology_profile_filter.go`,
     `snmp_topology/profile_filter.go`, `FinalizeProfiles`, dead
     `topology_profiles.go` constants, and obsolete metric-name dispatch
     constants.
   - Rewrite/delete topology-specific tests that depend on the old classifier,
     filters, hardcoded profile constants, `LoadProfileByName()`, or topology
     `HiddenMetrics` fixtures.
   - Preserve `TestCollector_Collect_PreservesHiddenMetrics` as the generic
     hidden-metric canary.

### Step-by-Step Implementation Plan For Readiness Review

This checklist is the pre-code implementation plan. It is intentionally more
granular than the phase list so an external reviewer can find ordering bugs,
missing tests, or unwanted side effects before any code is written.

#### 0. Activate SOW And Capture Baseline

1. Move `.agents/sow/pending/SOW-0012-20260506-snmp-profile-projection.md` to
   `.agents/sow/current/`.
2. Change SOW status from `open` to `in-progress`.
3. Record the branch name and current dirty files in the SOW execution log.
4. Run baseline focused tests before code changes where practical:
   - From `src/go`: `go test ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition`
   - From `src/go`: `go test ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector`
   - From `src/go`: `go test ./plugin/go.d/collector/snmp/...`
   - From `src/go`: `go test ./plugin/go.d/collector/snmp_topology/...`
5. If any baseline test fails before edits, record the exact package/test and
   failure class in the SOW. Do not count pre-existing failures as validation of
   the new implementation.

#### 1. Add Schema And API Surface Without Behavior Change

1. Add the canonical `ProfileConsumer` values for `metrics` and `topology`.
2. Add a single canonical closed `TopologyKind` definition with the 18 accepted
   topology row kinds. Do not include `sys_uptime`.
3. Avoid duplicated string constants. If multiple packages need the type, use a
   single definition plus aliases/imports rather than parallel enums.
4. Add `Topology []TopologyConfig` to `ProfileDefinition`.
5. Add `TopologyConfig` as a wrapper around the existing metric row shape plus
   required `kind`.
6. Follow the existing schema tag convention for every new persisted field:
   matching `yaml` and `json` names, `omitempty` for optional fields, and
   `yaml:"-" json:"-"` for runtime-only fields.
7. Add `Consumers` to metadata fields.
8. Add a top-level/global metric-tag wrapper that carries `Consumers`. Do not
   add consumer semantics to per-row `MetricTagConfig`; per-row metric tags
   inherit their row's consumer.
9. Update clone/deep-copy paths explicitly:
   - `ProfileDefinition.Clone()` must clone `Topology`;
   - `TopologyConfig.Clone()` must clone the embedded row config;
   - `MetadataField.Clone()` must clone/copy `Consumers`;
   - top-level/global metric tag wrapper clone paths must clone/copy
     `Consumers`.
10. Add validation for unknown topology kinds.
11. Add validation for metrics-only fields inside topology row anchor
   `symbol`/`symbols`: `Options`, `ChartMeta`, `MetricType`, `Mapping`,
   `Transform`, `ScaleFactor`, `Format`, and `ConstantValueOne`.
12. Extend the existing symbol traversal context, either with
    `TopologyScalarSymbol`/`TopologyColumnSymbol` or a topology-row flag, so
    value-symbol validation can reject topology-only-invalid fields without
    breaking `MetricTagSymbol`.
13. Extend `validateEnrichVirtualMetrics` to accept topology rows and reject any
    virtual metric source that resolves to a `topology:` row.
14. Reject underscore-prefixed `name:` values on topology row anchor symbols so
    topology rows cannot also flow into generic `HiddenMetrics`.
15. Keep metric-tag extraction fields valid under topology rows where they are
    used to extract tags from indexes/values.
16. Do not reject existing `_topology_*` rows in `metrics:` until the YAML
    migration and cutover are complete; add the final rejection only in cleanup.
17. Add parse/clone/validation tests in `ddprofiledefinition`.

#### 2. Split Profile Mutations By Required Context

1. Inventory every mutation that currently happens after profile load:
   `handleCrossTableTagsWithoutMetrics`, `enrichProfiles`,
   `deduplicateMetricsAcrossProfiles`, and any resolver-time mutation found
   while editing.
2. Move `handleCrossTableTagsWithoutMetrics` out of `ddsnmpcollector.New()` and
   into catalog/profile compilation before projections share profile pointers.
3. Extend `Profile.merge()` with `mergeTopology(base)` and call it from
   `Profile.merge()` during `extends:` loading.
4. `mergeTopology(base)` must use topology row identity
   `(kind, table_identity, symbol_name)` and Decision 2.B merge semantics.
5. Add `profile_test.go` coverage proving a profile extending a topology mixin
   inherits `Definition.Topology` rows.
6. Extend cross-table-tag synthesis to scan both `Definition.Metrics` and
   `Definition.Topology`; place synthesized entries on the slice that owns the
   consuming row so the correct collection path walks them.
7. Move `enrichProfiles()` to load/catalog compilation and extend it to process
   `Definition.Topology`.
8. Keep `deduplicateMetricsAcrossProfiles()` at resolve-time inside
   `Catalog.Resolve()` because it needs the already-matched, specificity-sorted
   profile set. It must operate on catalog-cloned profiles before projections
   are returned.
9. Extend resolve-time deduplication to process `Definition.Topology` with
   topology row identity `(kind, table_identity, symbol_name)`.
10. Preserve `Definition.Metrics` behavior exactly for regular SNMP.
11. Treat the suspected `removeConstantMetrics()` value-copy issue as separate
   unless touched by this refactor. Do not extend `removeConstantMetrics()` to
   topology because topology row validation rejects `ConstantValueOne`; if this
   changes or the existing value-copy bug becomes relevant, either fix it with a
   narrow test or record a separate follow-up SOW.
12. Add mutation-isolation tests showing one projected view cannot mutate another
   or the catalog. The minimum assertion is: resolve a device twice, mutate a
   nested map/slice in view 1, including nested state under
   `Definition.Topology`, then assert view 2 and a fresh resolve from the same
   catalog do not contain that mutation.

#### 3. Migrate Profile YAML

1. Move topology rows from `metrics:` to top-level `topology:` in the topology
   mixins.
2. Split `_std-cdp-mib.yaml` into:
   - regular CDP metrics in `_std-cdp-mib.yaml`;
   - topology CDP rows in `_std-topology-cdp-mib.yaml`.
3. Add `kind:` to every topology row using the accepted `TopologyKind` enum.
4. Rename topology row anchor symbol names away from `_topology_*`; dispatch
   must use `kind`, not the old hidden-metric name.
5. Preserve OIDs, table identities, symbols, and metric_tags unless the move
   exposes a concrete bug.
6. Do not modify `_system-base.yaml`; `systemUptime` remains a regular metrics
   row. Topology obtains uptime through `pkg/snmputils.GetSysUptime`.
7. If any symbol/tag OID is added or changed, run the SNMP profile authoring
   MAX-ACCESS checks and record evidence. Pure row moves with unchanged OIDs
   should record that no readable-symbol semantics changed.
8. Update profile extender references so vendors that previously extended mixed
   CDP/topology content still get the intended regular and topology rows.
9. Inventory every profile extending `_std-cdp-mib.yaml` and update each profile
   that should retain CDP topology to also extend `_std-topology-cdp-mib.yaml`.
   Current default extenders are `_cisco-base.yaml` and `cisco-sb.yaml`.
10. Add or update profile load tests for the migrated files.
11. Treat phases 3-6 as one logical topology cutover. Do not ship a state where
    topology YAML has moved to `topology:` but topology runtime still reads only
    `_topology_*` metrics from `metrics:`.
12. Record that topology mixins may become topology-only/abstract after
    migration; validation/tests must not assume every topology mixin produces
    regular `metrics:` rows when loaded as a root.

#### 4. Introduce Catalog, Resolve, Project, And Filter

1. Add `Catalog`, `ResolveRequest`, `ManualProfilePolicy`,
   `ResolvedProfileSet`, and projected view types.
2. Implement the catalog/resolver/projection API in `collector/snmp/ddsnmp`,
   next to the existing profile loader, `FindProfiles()`, and profile model.
3. Implement manual profile policies:
   - metrics call sites use fallback-only;
   - topology call sites use augment.
4. Implement `Project(ConsumerMetrics)` and `Project(ConsumerTopology)`.
5. Implement `FilterByKind(map[TopologyKind]bool)` for topology projections.
6. Make projections non-mutating. Prefer immutable catalog-owned profiles plus
   read-only projected slices or precomputed buckets over per-call deep clone.
7. Keep temporary wrappers such as `FindProfiles()` only as needed to prove
   parity during the same SOW; remove or simplify them during cleanup.
8. Add resolver parity tests for regular SNMP.
9. Add topology projection parity tests against the current topology filter
   behavior after YAML migration.
10. Add manual-policy tests covering matching `sysObjectID` plus
   `manual_profiles` for both metrics and topology.

#### 5. Plumb Topology Collection Through ddsnmpcollector

1. Add `TopologyKind` to emitted `ddsnmp.Metric`.
2. Add `TopologyMetrics []Metric` to `ProfileMetrics`.
3. Preserve `HiddenMetrics []Metric` as the generic underscore-prefixed
   non-topology delivery container.
4. Add explicit topology collection from `Definition.Topology`, parallel to the
   existing scalar/table collection path.
5. Prefer a topology collection wrapper that calls the existing scalar/table
   collection helpers and stamps `TopologyKind` after emit. Do not widen regular
   scalar/table builder signatures unless the wrapper proves insufficient and
   the SOW records why.
6. Add `pkg/snmputils.GetSysUptime(gosnmp.Handler)` with the `_system-base.yaml`
   uptime OIDs and scale rules, and call it from `snmp_topology` refresh. Do not
   add duplicate `systemUptime` YAML topology rows or topology kinds.
7. Stop relying on underscore-prefix metric names for topology delivery.
8. Keep `collectHiddenMetrics()` behavior for non-topology underscore-prefixed
   regular metrics.
9. Enforce the invariant that a topology row lives in exactly one delivery
   slice for a single poll: not both `pm.HiddenMetrics` and
   `pm.TopologyMetrics`.
10. Ensure `TestCollector_Collect_PreservesHiddenMetrics` continues to pass.
11. Add tests proving topology rows populate `pm.TopologyMetrics` with the
   correct `Metric.TopologyKind`.
12. Add a double-bucketing assertion test that fails if a `_topology_*` or
    topology-kind row appears in both `pm.HiddenMetrics` and
    `pm.TopologyMetrics`.

#### 6. Cut Over SNMP And Topology Call Sites

1. Switch regular SNMP profile selection to
   `DefaultCatalog().Resolve(...).Project(ConsumerMetrics)`.
2. Switch SNMP topology profile selection to
   `DefaultCatalog().Resolve(...).Project(ConsumerTopology)`.
3. Replace VLAN-context hardcoded `LoadProfileByName()` calls with
   `Project(ConsumerTopology).FilterByKind(vlanScopableKinds)`.
4. Define `vlanScopableKinds` in topology Go code, not profile YAML. Pin it to
   the current VLAN-context ingest set:
   - `KindIfName`;
   - `KindBridgePortIfIndex`;
   - `KindFdbEntry`;
   - `KindStpPort`.
5. Replace metric-name topology dispatch with a handler registry keyed by
   `TopologyKind`.
6. Make topology row handlers receive the full `ddsnmp.Metric`; uptime is
   handled by the explicit `snmputils` helper path, not by the topology handler
   registry.
7. Register each topology cache handler from its domain file.
8. Extend `updateTopologyProfileTags` to read `pm.Tags` and apply them as local
   device/profile labels, not per-row dispatch keys.
9. Update `ingestTopologyVLANContextMetrics()` so any synthetic metric passed to
   kind-keyed dispatch carries/preserves `TopologyKind`.
10. Update `ingestTopologyProfileMetrics` so the post-cutover loop dispatches
    only `pm.TopologyMetrics` by `TopologyKind`; regular `pm.Metrics` are not
    part of topology projection.
11. Add dispatch parity tests and VLAN-context equivalence tests, including
    `TopologyKind` population for VLAN-context synthetic metrics.
12. Add a side-by-side fixture-level runtime parity test before deleting the old
    path. Use a representative default-profile fixture such as the existing
    Cisco Nexus profile path in `collector/snmp/ddsnmp/profile_test.go:178`,
    and compare emitted `ProfileMetrics` fields that must remain stable for
    regular SNMP: `Tags`, `DeviceMetadata`, `Metrics`, ordering, and metric
    tags.

#### 7. Delete Dead Code And Update Artifacts

1. Search for stale references before deleting:
   - `_topology_`
   - `IsTopologyMetric`
   - `LooksLikeTopologyIdentifier`
   - `MetricConfigContainsTopologyData`
   - `MetricTagConfigContainsTopologyData`
   - `MetadataFieldContainsTopologyData`
   - `MetadataContainsTopologyData`
   - `SysobjectIDMetadataContainsTopologyData`
   - `ProfileContainsTopologyData`
   - `ProfileHasCollectionData`
   - `TopologySysUptime`
   - `LoadProfileByName`
   - `HiddenMetrics`
2. Delete dead topology classifier/filter code only after call sites and tests
   no longer depend on it.
3. Delete `FinalizeProfiles()` only after VLAN-context and other callers no
   longer require it.
4. Delete only topology-specific use of hidden metrics; do not delete generic
   `HiddenMetrics`.
5. Update `collector/snmp/profile-format.md`.
6. Update `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
7. Create `.agents/sow/specs/snmp-profile-projection.md`.
8. Run the focused validation suites.
9. Run same-failure/stale-reference scans and record results in the SOW.
10. Complete the SOW artifact maintenance gate and follow-up mapping.
11. Explicit test impact:
    - Delete/rewrite `collector/snmp/ddsnmp/topology_classify_test.go`.
    - Rewrite `collector/snmp/profile_sets_test.go` around projection.
    - Rewrite `collector/snmp_topology/profile_filter_test.go` around
      projection.
    - Rewrite `collector/snmp_topology/topology_profiles_test.go` if hardcoded
      topology profile constants disappear.
    - Rewrite `collector/snmp/ddsnmp/ddsnmpcollector/topology_profile_index_test.go`
      if it no longer needs `LoadProfileByName()`.
    - Rewrite topology fixtures in
      `collector/snmp_topology/topology_cache_test.go:1327-1371` and
      `collector/snmp_topology/collector_refresh_test.go:123-152` to use
      `TopologyMetrics`, not topology `HiddenMetrics`.
    - Preserve
      `collector/snmp/ddsnmp/ddsnmpcollector/collector_test.go:153`
      `TestCollector_Collect_PreservesHiddenMetrics`.

## Testing Requirements

- Implementation validation run:
  - `go test -count=1 ./collector/snmp/ddsnmp/...`: passed.
  - `go test -count=1 ./collector/snmp/...`: passed.
  - `go test -count=1 ./collector/snmp_topology/...`: passed.
- Implementation test coverage added/updated:
  - Resolver parity: for every default profile, `Catalog.Resolve(...).Project(ConsumerMetrics)` matches current `selectCollectionProfiles(FindProfiles(...))`.
  - Topology parity: `Project(ConsumerTopology)` matches current `selectTopologyRefreshProfiles(FindProfiles(...))` after YAML migration.
  - VLAN-context equivalence: `Project(ConsumerTopology).FilterByKind(vlanScopableKinds)` matches today's hardcoded VLAN-context loader.
  - Manual policy: regular metrics with `manual_profiles` plus matching `sysObjectID` do not augment; topology does augment.
  - Topology extends merge: a vendor/root profile extending a topology mixin inherits `Definition.Topology` rows through `Profile.merge().mergeTopology(base)`.
  - Topology-to-topology merge: derived override preserves base kind when omitted and rejects conflicting explicit kinds.
  - Clone coverage: `ProfileDefinition.Clone()`, `TopologyConfig.Clone()`, `MetadataField.Clone()`, and consumer-set clone paths do not share mutable state.
  - Validation rejects unknown `TopologyKind`, metrics-only fields on `TopologyConfig`, underscore-prefixed topology row anchor names, and mixed-consumer virtual metrics.
  - Top-level/global `metric_tags`: values in `pm.Tags` reach topology local device/profile labels through `updateTopologyProfileTags`, not per-row dispatch tags.
  - HiddenMetrics non-topology preservation: `TestCollector_Collect_PreservesHiddenMetrics` in `collector/snmp/ddsnmp/ddsnmpcollector/collector_test.go` must continue to pass after topology stops using `pm.HiddenMetrics`.
  - Topology double-bucketing guard: a topology row must not appear in both `pm.HiddenMetrics` and `pm.TopologyMetrics` in the same poll.
  - `systemUptime`: `_system-base.yaml` remains metrics-only; topology receives uptime through `pkg/snmputils.GetSysUptime` without a topology kind or YAML topology row.
  - VLAN-context kind flow: `vlanScopableKinds` contains exactly `KindIfName`, `KindBridgePortIfIndex`, `KindFdbEntry`, and `KindStpPort`, and VLAN-context synthetic metrics carry `TopologyKind`.
  - Side-by-side runtime parity: representative default-profile fixture emits equivalent regular SNMP `ProfileMetrics` before old path deletion.
  - Mutation isolation: mutating one projection cannot affect another after resolve.
  - `Metric.TopologyKind`: every topology kind is emitted correctly from fixture collection.
  - Existing tests deleted/rewritten: `collector/snmp/ddsnmp/topology_classify_test.go`, `collector/snmp/profile_sets_test.go`, `collector/snmp_topology/profile_filter_test.go`, `collector/snmp_topology/topology_profiles_test.go`, `collector/snmp/ddsnmp/ddsnmpcollector/topology_profile_index_test.go`, and topology HiddenMetrics fixtures in `collector/snmp_topology/topology_cache_test.go` / `collector/snmp_topology/collector_refresh_test.go`.

## Documentation Updates Required

- Updated:
  - `collector/snmp/profile-format.md`: topology rows, consumers, topology
    kinds, `systemUptime`, and hidden-metric guidance.
  - `.agents/skills/project-snmp-profiles-authoring/SKILL.md`: top-level
    `topology:` row and `TopologyKind` authoring rules.
  - `.agents/sow/specs/snmp-profile-projection.md`: durable contract for
    catalog, projection, consumers, topology rows, defaults, merge rules, and
    delivery slices.
- Checked SNMP topology function docs/examples; no user-facing topology function
  schema changed.
