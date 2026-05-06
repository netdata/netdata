# SOW-0012 - SNMP profile projection and topology row schema

## Status

Status: completed

Sub-state: Implementation, focused validation, final review, and commit prep completed.

## Requirements

### Purpose

Replace the hacky SNMP metrics-vs-topology profile split with a clean, schema-driven profile catalog, resolver, and projection model. Regular SNMP compatibility matters. SNMP topology is WIP/nightly-only, so topology behavior can change to reach a clean end state.

### User Request

User request summary:

- Recently added SNMP topology works but was added in a hacky way.
- Focus on how profiles are loaded for regular metrics versus topology.
- Review `src/go/plugin/go.d/collector/snmp/ddsnmp` and `src/go/plugin/go.d/collector/snmp_topology`.
- Prefer clean end state over low churn.
- Topology backward compatibility is not a constraint because it is WIP/nightly-only.
- Use independent AI reviews to avoid missing issues.

Detailed design source:

- `src/go/plugin/go.d/TODO-snmp-profile-loading-topology.md`

### Assistant Understanding

Facts:

- Regular SNMP and SNMP topology currently use the same physical profile loader in `collector/snmp/ddsnmp/load.go`.
- `collector/snmp/ddsnmp/profile.go:25-71` selects profiles by `sysObjectID`/`sysDescr`, or by `manual_profiles` only when `sysObjectID` is empty.
- Regular SNMP calls `ddsnmp.FindProfiles()` and then strips topology data via `collector/snmp/topology_profile_filter.go`.
- SNMP topology calls `ddsnmp.FindProfiles()` and then keeps topology data via `collector/snmp_topology/profile_filter.go`.
- Topology classification is hardcoded in `collector/snmp/ddsnmp/topology_classify.go`.
- Topology ingestion dispatch is hardcoded by metric name in `collector/snmp_topology/topology_cache_metric_dispatch.go`.
- VLAN-context topology bypasses the resolver and hardcodes topology profile filenames in `collector/snmp_topology/topology_vlan_context_collect.go`.
- Current `_topology_*` rows are hidden through the generic underscore-prefix `HiddenMetrics` path in `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:122-123`.
- `HiddenMetrics` is not topology-owned. It is a general delivery container and must not be deleted or redefined without auditing non-topology users.

Inferences:

- The root issue is not physical profile loading. The root issue is the lack of an explicit consumer/topology contract in the profile schema and resolver output.
- A single catalog plus explicit projection preserves the strong shared-profile model while removing name/prefix heuristics.
- A top-level `topology:` list is cleaner than embedding topology rows in `metrics[]` because topology rows are not regular chart metrics and should not share metric merge/dedup ambiguity.

Unknowns:

- No product/design unknowns remain for this SOW. User decisions 1.D, 2.B, 3.C, 4.A, 5.B, 6.A, 7.A, and 8.A are recorded in the TODO and summarized below.
- Some implementation details will be discovered while refactoring tests and moving per-profile mutations to load-time plus matched-set deduplication to resolve-time, but they are bounded by the gate and validation plan.

### Acceptance Criteria

- Regular SNMP metrics collection uses the new catalog/resolver/projection path and remains behaviorally equivalent to the current `selectCollectionProfiles(FindProfiles(...))` path.
- SNMP topology uses top-level `topology:` rows, `TopologyKind`, and `ProfileMetrics.TopologyMetrics`; it no longer depends on `_topology_*` metric names or underscore-prefix `HiddenMetrics`.
- VLAN-context topology uses `Project(ConsumerTopology).FilterByKind(vlanScopableKinds)` instead of hardcoded `LoadProfileByName()` calls.
- `HiddenMetrics` remains available as a generic non-topology underscore-prefixed metric delivery container, and the existing preservation test continues to pass.
- Old topology classifier/filter code and dead hardcoded profile-name constants are removed.
- Profile-format documentation, project SNMP profile authoring skill, and a new SOW spec describe the shipped contract.

## Analysis

Sources checked:

- `src/go/plugin/go.d/TODO-snmp-profile-loading-topology.md`
- `collector/snmp/ddsnmp/load.go`
- `collector/snmp/ddsnmp/profile.go`
- `collector/snmp/ddsnmp/topology_classify.go`
- `collector/snmp/ddsnmp/ddprofiledefinition/profile_definition.go`
- `collector/snmp/ddsnmp/ddprofiledefinition/metrics.go`
- `collector/snmp/ddsnmp/ddprofiledefinition/validation.go`
- `collector/snmp/ddsnmp/ddsnmpcollector/collector.go`
- `collector/snmp/ddsnmp/ddsnmpcollector/collector_scalar.go`
- `collector/snmp/ddsnmp/ddsnmpcollector/collector_table.go`
- `collector/snmp/ddsnmp/ddsnmpcollector/metric_builder.go`
- `collector/snmp/profile_sets.go`
- `collector/snmp/topology_profile_filter.go`
- `collector/snmp_topology/collector.go`
- `collector/snmp_topology/profile_filter.go`
- `collector/snmp_topology/topology_cache_metric_dispatch.go`
- `collector/snmp_topology/topology_vlan_context_collect.go`
- `collector/snmp_topology/topology_cache_ingest.go`
- `config/go.d/snmp.profiles/default/_system-base.yaml`
- `config/go.d/snmp.profiles/default/_std-topology-lldp-mib.yaml`
- `config/go.d/snmp.profiles/default/_std-cdp-mib.yaml`
- `collector/snmp/profile-format.md`
- `.agents/skills/project-snmp-profiles-authoring/SKILL.md`
- `.agents/sow/pending/SOW-0002-20260501-unified-multi-layered-topology-schema.md`
- `.agents/sow/specs/go-v2-host-scope.md`
- `.agents/sow/specs/sensitive-data-discipline.md`

Current state:

- Profile loading is shared, but consumer projection is implemented by mirrored package-private filters.
- Topology row identity is encoded in metric names instead of profile schema.
- Topology dispatch is tied to metric-name constants.
- VLAN-context topology has a separate hardcoded profile load path.
- Post-load profile mutations exist in resolver/collector paths and must move before shared immutable projections are safe.

Risks:

- Regular SNMP regression if metrics projection changes selected metrics, metadata, global tags, virtual metrics, or manual profile behavior.
- Topology row loss if `TopologyKind` is not threaded from profile rows through scalar/table collectors into topology ingest.
- `HiddenMetrics` regression if implementation treats it as topology-only and breaks other underscore-prefixed delivery consumers.
- Profile merge regressions if inherited topology rows do not use a precise identity key.
- Validation churn from adding `topology:` and consumer fields to the profile schema.
- Documentation drift if profile-format docs and project SNMP authoring skill are not updated with the new contract.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The profile loader is not the primary problem. The loader already centralizes profile directories, YAML parsing, embedded defaults, `extends`, and caching.
- The root cause is that resolved profiles contain a mixed bag of regular metric rows, topology rows, metadata, and tags without an explicit schema-level consumer contract.
- Regular SNMP and SNMP topology compensate with mirrored filters and a hardcoded classifier based on `_topology_*` names and tag/metadata prefixes.
- VLAN-context topology worsens the split by directly loading hardcoded mixins and bypassing resolver semantics.

Evidence reviewed:

- `collector/snmp/ddsnmp/topology_classify.go:16-30` exact topology metric name allowlist.
- `collector/snmp/ddsnmp/topology_classify.go:43-65` prefix-based topology identifier heuristic.
- `collector/snmp/topology_profile_filter.go:10-50` regular SNMP strips topology data.
- `collector/snmp_topology/profile_filter.go:10-47` topology keeps topology data.
- `collector/snmp_topology/topology_vlan_context_collect.go:16-28` hardcoded VLAN-context profile loads.
- `collector/snmp/ddsnmp/ddsnmpcollector/collector.go:122-123` generic underscore-prefixed hidden metrics bucketing.
- `collector/snmp_topology/collector.go:220-224` topology currently ingests hidden and regular metric slices.
- `collector/snmp_topology/topology_cache_ingest.go:11-30` existing profile tag hook that reads `pm.DeviceMetadata`.
- `collector/snmp/ddsnmp/profile.go:194-252` current regular metric merge identity.
- `collector/snmp/ddsnmp/profile.go:375-440` enrichment/dedup currently only covers `Definition.Metrics` and `Definition.VirtualMetrics`.

Affected contracts and surfaces:

- Profile schema: `ProfileDefinition`, `MetricsConfig`, metadata fields, top-level/global metric tags, virtual metrics validation.
- ddsnmp public API: catalog, resolve request, manual profile policy, resolved profile set, projected views.
- ddsnmpcollector output: `Metric.TopologyKind`, `ProfileMetrics.TopologyMetrics`, preservation of `HiddenMetrics`.
- SNMP collector metrics path: profile selection/projection and chart labels.
- SNMP topology path: profile selection, VLAN-context, topology ingest, handler registry.
- Default SNMP profile YAMLs under `config/go.d/snmp.profiles/default/`.
- Tests under `collector/snmp`, `collector/snmp/ddsnmp`, `collector/snmp/ddsnmp/ddprofiledefinition`, `collector/snmp/ddsnmp/ddsnmpcollector`, and `collector/snmp_topology`.
- Documentation and durable artifacts: `collector/snmp/profile-format.md`, `.agents/skills/project-snmp-profiles-authoring/SKILL.md`, `.agents/sow/specs/snmp-profile-projection.md`.

Existing patterns to reuse:

- Keep the shared loader/resolver model from `collector/snmp/ddsnmp/load.go` and `profile.go`.
- Reuse SNMP row shape from `MetricsConfig` for topology rows via `TopologyConfig`.
- Reuse existing table/scalar collection logic but thread parent topology row metadata into emitted metrics.
- Reuse `updateTopologyProfileTags` as the hook for Decision 8.A.
- Reuse existing profile merge identity concepts: scalar name/OID and table identity plus symbol name.
- Reuse existing validation style in `ddprofiledefinition/validation.go`.
- Reuse focused Go package tests and profile fixture tests instead of broad full-repo validation.

Risk and blast radius:

- Regular SNMP blast radius is broad because default profiles affect production metric collection. Metrics projection must be parity-protected.
- Topology blast radius is acceptable for topology behavior because the feature is WIP/nightly-only, but topology must still produce coherent data.
- `HiddenMetrics` has cross-PR/non-topology risk. It must remain a general-purpose delivery path until separately audited and refactored.
- Moving per-profile mutations to load-time and matched-set deduplication to resolve-time changes pointer and clone assumptions. Tests must prove projections cannot mutate shared catalog state.
- YAML migration touches topology profile mixins and `_std-cdp-mib.yaml`; profile load validation must catch mistakes early.
- Validation rejecting topology-row chart/export fields may expose existing accidental fields during migration.

Sensitive data handling plan:

- This SOW and implementation should reference profile filenames, metric names, OIDs, struct fields, and tests only.
- Do not write raw SNMP communities, SNMPv3 credentials, bearer tokens, passwords, customer hostnames, customer sysName/sysDescr values, private endpoints, non-private customer-identifying IPs, customer names, or personal data into SOWs, specs, docs, skills, tests, fixtures, or code comments.
- Any real SNMP fixture used later must be sanitized before committing. Use neutral device names and placeholder values.
- Existing profile YAMLs contain public OIDs and generic vendor/device metadata, not credentials.

Implementation plan:

1. Schema and API surface, no behavior change.
   - Add `Topology []TopologyConfig` to `ProfileDefinition`.
   - Add a closed 18-value `TopologyKind` enum for current topology row shapes. `systemUptime` is not a topology kind.
   - Add `MetadataField.Consumers`.
   - Add validation for unknown topology kinds and metrics-only fields under topology row anchor symbols.
   - Add `Catalog`, `Resolve`, `Project`, and `FilterByKind` API surface without cutting over call sites.

2. Split mutation handling between load-time/catalog compilation and resolve-time matched-set processing.
   - Move per-profile/idempotent mutation to load-time: `enrichProfiles` and `handleCrossTableTagsWithoutMetrics`.
   - Extend cross-table-tag synthesis to scan both `Definition.Metrics` and `Definition.Topology`, placing synthesized entries on the owning slice.
   - Keep `deduplicateMetricsAcrossProfiles` at resolve-time inside `Catalog.Resolve()` because it needs the matched, sorted profile set.
   - Extend resolve-time deduplication to `Definition.Topology`.
   - Preserve metrics-path parity with current `FindProfiles`.

3. Profile YAML migration.
   - Move `_topology_*` rows from `metrics:` to top-level `topology:` in topology mixins.
   - Split `_std-cdp-mib.yaml` into real metrics and `_std-topology-cdp-mib.yaml`.
   - Add `kind:` to every topology row.
   - Keep metadata/global tag annotations minimal per Decision 4.A.
   - Do not modify `_system-base.yaml`; `systemUptime` remains a regular metric.
   - Phases 3-6 are one logical topology cutover and must not ship as a broken intermediate topology state.

4. Catalog/Resolve/Project introduction with parity tests.
   - Keep old `FindProfiles` temporarily.
   - Prove `Catalog.Resolve(...).Project(ConsumerMetrics)` matches current `selectCollectionProfiles(FindProfiles(...))`.
   - Prove `Project(ConsumerTopology)` matches current topology selection after YAML migration.

5. Plumb topology collection through `ddsnmpcollector`.
   - Add `Metric.TopologyKind`.
   - Add `ProfileMetrics.TopologyMetrics`.
   - Prefer a topology collection wrapper that stamps `TopologyKind` after existing scalar/table emission; do not widen regular builders unless the SOW records why.
   - Add explicit topology collection from `Definition.Topology`.
   - Do not map regular `systemUptime` metrics into topology projection. Topology queries uptime through `pkg/snmputils.GetSysUptime`.
   - Remove topology's dependency on underscore-prefix hidden metrics.
   - Preserve generic `HiddenMetrics` behavior for non-topology underscore-prefixed metrics.
   - Enforce that topology rows cannot be delivered through both `pm.HiddenMetrics` and `pm.TopologyMetrics` in one poll.

6. Cut over call sites and dispatch.
   - Switch `collector/snmp/profile_sets.go`, `collector/snmp_topology/collector.go`, and `collector/snmp_topology/topology_vlan_context_collect.go`.
   - Replace metric-name dispatch switch with handler registry keyed by `TopologyKind`.
   - Extend `updateTopologyProfileTags` to apply `pm.Tags` as local device/profile labels.
   - Add side-by-side fixture-level runtime parity before deleting the old path.

7. Delete dead code and update artifacts.
   - Delete topology classifier/filter code, dead topology profile constants, and obsolete metric-name dispatch constants; remove topology runtime reliance on `FinalizeProfiles`.
   - Update `collector/snmp/profile-format.md`.
   - Update `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
   - Create `.agents/sow/specs/snmp-profile-projection.md`.

Validation plan:

- Resolver parity: for every default profile, `Catalog.Resolve(...).Project(ConsumerMetrics)` matches current `selectCollectionProfiles(FindProfiles(...))`.
- Topology parity: `Project(ConsumerTopology)` matches current `selectTopologyRefreshProfiles(FindProfiles(...))` after YAML migration.
- VLAN-context equivalence: `Project(ConsumerTopology).FilterByKind(vlanScopableKinds)` matches today's hardcoded VLAN-context loader.
- Manual policy: regular metrics with `manual_profiles` plus matching `sysObjectID` do not augment; topology does augment.
- Topology extends merge: a vendor/root profile extending a topology mixin inherits `Definition.Topology` rows through `Profile.merge().mergeTopology(base)`.
- Topology-to-topology merge: topology rows require explicit `kind`; matching rows with conflicting explicit kinds are rejected.
- Clone coverage: `ProfileDefinition.Clone()`, `TopologyConfig.Clone()`, `MetadataField.Clone()`, and consumer-set clone paths do not share mutable state.
- Validation rejects unknown `TopologyKind`, metrics-only fields on `TopologyConfig`, underscore-prefixed topology row anchor names, and mixed-consumer virtual metrics.
- Top-level/global `metric_tags`: values in `pm.Tags` reach topology local device/profile labels through `updateTopologyProfileTags`, not per-row dispatch tags.
- HiddenMetrics non-topology preservation: `TestCollector_Collect_PreservesHiddenMetrics` in `collector/snmp/ddsnmp/ddsnmpcollector/collector_test.go` continues to pass.
- Topology double-bucketing guard: a topology row must not appear in both `pm.HiddenMetrics` and `pm.TopologyMetrics` in the same poll.
- `systemUptime`: `_system-base.yaml` remains metrics-only; topology receives uptime through `pkg/snmputils.GetSysUptime` without a topology kind or YAML topology row.
- VLAN-context kind flow: `vlanScopableKinds` contains exactly `KindIfName`, `KindBridgePortIfIndex`, `KindFdbEntry`, and `KindStpPort`, and VLAN-context synthetic metrics carry `TopologyKind`.
- Side-by-side runtime parity: representative default-profile fixture emits equivalent regular SNMP `ProfileMetrics` before old path deletion.
- Mutation isolation: mutating one projection cannot affect another after resolve.
- `Metric.TopologyKind`: every topology kind is emitted correctly from fixture collection.
- Run narrow suites:
  - `go test ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition`
  - `go test ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector`
  - `go test ./plugin/go.d/collector/snmp/...`
  - `go test ./plugin/go.d/collector/snmp_topology/...`

Artifact impact plan:

- AGENTS.md: no expected change; existing SOW/process and SNMP skill triggers are sufficient.
- Runtime project skills: update `.agents/skills/project-snmp-profiles-authoring/SKILL.md` with `topology:` row and `TopologyKind` guidance.
- Specs: create `.agents/sow/specs/snmp-profile-projection.md`.
- End-user/operator docs: update `collector/snmp/profile-format.md`; check whether SNMP topology docs/examples need updates.
- End-user/operator skills: no expected public skill update unless docs/spec changes are mirrored into public skill artifacts.
- SOW lifecycle: SOW remains `open` in `pending/` until implementation starts; move to `current/` and mark `in-progress` only when implementation begins.

Open-source reference evidence:

- No external mirrored open-source repositories were checked. The work concerns Netdata's internal SNMP profile schema and collector implementation; local repo code, profile YAMLs, tests, and project skills are the relevant ground truth for this SOW.

Open decisions:

- None. User decisions and implementation sub-decisions are recorded below.

## Implications And Decisions

User decisions:

1. Manual profile policy: use internal policy. Regular metrics use fallback-only manual profiles; topology uses augment.
2. Topology merge behavior: topology rows require explicit `kind`; explicit derived topology wins for matching row identities; conflicting explicit topology kinds are rejected. Earlier "derived omits kind" inheritance wording is unreachable because validation rejects persisted topology rows without `kind`.
3. Virtual metric projection: reject mixed-consumer virtual metrics at validation.
4. Metadata/global tag defaults: metadata fields and top-level/global `metric_tags` default to both `metrics` and `topology`, with explicit narrowing when needed.
5. Topology row schema: use a separate top-level `topology:` list for topology rows instead of embedding topology rows in `metrics[]`.
6. TopologyKind granularity: define one closed `TopologyKind` per current topology row shape, including LLDP management-address rows.
7. `sysUptime` path: superseded during implementation. Remove `KindSysUptime`; topology uptime acquisition uses `pkg/snmputils.GetSysUptime`.
8. Global metric tags in topology: apply top-level/global `metric_tags` as local device/profile labels, not as dispatch keys on every topology row.
9. `HiddenMetrics`: treat as a general delivery container, not topology-owned. Do not delete or redefine it without auditing non-topology consumers.
10. Before implementation starts, write a step-by-step implementation plan detailed enough for external readiness review. Ask Claude to judge the plan as `READY TO WRITE CODE` or `NEEDS ADJUSTMENTS`; do not start code until that review is accepted or required adjustments are recorded.
11. Fifth readiness review adjustments are accepted: B1-B6 and S1-S6. Phases 3-6 are one logical topology cutover; the original `KindSysUptime` projection/collector mapping from existing regular `systemUptime` rows was later superseded by Decision 14.
12. Sixth readiness review adjustments are accepted: N1 and N2-N5. `Profile.merge()` must merge `Definition.Topology` during `extends:` loading, clone targets are explicit, VLAN-context synthetic metrics carry `TopologyKind`, `vlanScopableKinds` is pinned to existing VLAN ingest kinds, and new fields follow existing YAML/JSON tag conventions.
13. Seventh readiness review verdict is `READY TO WRITE CODE`. Non-blocking hygiene tightenings NB1-NB4 are accepted: expanded stale-helper scans, `Catalog`/`Resolve`/`Project` live in `collector/snmp/ddsnmp`, top-level/global metric tags use a wrapper for `Consumers`, and post-cutover topology ingest loops over `pm.TopologyMetrics`. The original regular `systemUptime` metrics mapping was later superseded by Decision 16.
14. User correction during implementation: remove `KindSysUptime`. `systemUptime` is not a topology row kind. A direct `pkg/snmputils` uptime helper was proposed and later accepted in Decision 16.
15. Regular SNMP metrics must keep uptime collection through existing metrics profile rows. Do not move or remove uptime from `_system-base.yaml`.
16. Topology uptime acquisition after removing `KindSysUptime`: use option A. Add `pkg/snmputils.GetSysUptime(gosnmp.Handler)` with the same three OIDs and scale rules as `_system-base.yaml`, and call it from `snmp_topology` during refresh. Regular SNMP uptime metrics remain profile-driven.
17. Public profile-format documentation should describe the current topology schema without mentioning the old underscore-prefixed topology metric-name implementation.

Resolved implementation decision:

1. Topology uptime acquisition after removing `KindSysUptime`. Resolved: A.
   - Evidence:
     - `config/go.d/snmp.profiles/default/_system-base.yaml:11-44` defines the three regular uptime fallback rows: `snmpEngineTime` in seconds, `hrSystemUptime` scaled by `0.01`, and `sysUpTime` scaled by `0.01`.
     - `collector/snmp_topology/topology_cache_ingest.go:52-67` stores uptime into the local topology device and the `sys_uptime` label.
     - `collector/snmp_topology/topology_local_actor_attrs.go:32-34` exports `sys_uptime` as local actor attributes when present.
     - `collector/snmp/ddsnmp/profile_catalog.go:271-320` currently keeps a special topology projection path for `systemUptime`/`sysUpTime`; this should disappear with `KindSysUptime`.
   - A. Add `pkg/snmputils.GetSysUptime(gosnmp.Handler)` with the same three OIDs and scale rules, and call it from `snmp_topology` during refresh.
     - Pros: removes `KindSysUptime`; keeps profile topology projection pure; keeps regular SNMP metrics unchanged; localizes the hardcoded fallback to one SNMP utility helper.
     - Cons: duplicates uptime OID/scale knowledge from `_system-base.yaml`; future uptime fallback changes need the helper updated too; adds one SNMP GET per topology refresh.
   - B. Keep using ddsnmp regular metric collection for uptime, but identify `systemUptime`/`sysUpTime` by name in `snmp_topology`.
     - Pros: no extra SNMP GET; reuses profile scaling and fallback behavior.
     - Cons: keeps metric-name special casing and forces topology projection to retain a regular metric row.
   - C. Stop collecting uptime for topology.
     - Pros: simplest implementation; no duplicate OIDs or extra SNMP request.
     - Cons: topology output loses `sys_uptime` on local actors; existing tests and output paths already treat it as useful local device enrichment.
   - Recommendation: A.

Implementation sub-decisions:

1. Add `ProfileMetrics.TopologyMetrics []Metric` and remove topology's dependency on underscore-prefix `HiddenMetrics`.
2. Use `(kind, table_identity, symbol_name)` as topology row identity for both load-time `extends:` merge through `Profile.merge().mergeTopology(base)` and resolve-time matched-set deduplication.
3. Extend `updateTopologyProfileTags` to read `pm.Tags` for Decision 8.A.
4. Reject metrics-only fields on topology row anchor `symbol` / `symbols`, while keeping `metric_tags` extraction fields valid.
5. Split mutation timing: move per-profile enrichment and cross-table-tag synthesis to load-time/catalog compilation; keep `deduplicateMetricsAcrossProfiles()` at resolve-time inside `Catalog.Resolve()` on catalog-cloned matched profiles and extend it to topology rows.

## Plan

1. Keep this SOW in `pending/` until user explicitly approves implementation.
2. Before implementation starts, run a read-only Claude readiness review against the step-by-step implementation plan below.
3. If Claude says `NEEDS ADJUSTMENTS`, record each accepted/rejected point in the SOW and TODO before code.
4. On implementation start, move this SOW to `.agents/sow/current/`, set `Status: in-progress`, and execute the 7-phase implementation plan from the Pre-Implementation Gate.
5. Maintain the SOW execution log after each implementation phase.
6. Close only after implementation, docs/spec/skill updates, validation, artifact maintenance gate, and follow-up mapping are complete.

### Step-by-Step Implementation Plan

This is the pre-code checklist that must be reviewed before implementation. It is intentionally more detailed than the phase list so ordering bugs, hidden coupling, and missing validation can be found before edits begin.

#### 0. Activate SOW And Capture Baseline

1. Move this SOW from `.agents/sow/pending/` to `.agents/sow/current/`.
2. Change status from `open` to `in-progress`.
3. Record the branch name and current dirty files in the execution log.
4. Run baseline focused tests before code changes where practical:
   - From `src/go`: `go test ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition`
   - From `src/go`: `go test ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector`
   - From `src/go`: `go test ./plugin/go.d/collector/snmp/...`
   - From `src/go`: `go test ./plugin/go.d/collector/snmp_topology/...`
5. If any baseline test fails before edits, record the exact package/test and failure class. Do not count pre-existing failures as validation of the new implementation.

#### 1. Add Schema And API Surface Without Behavior Change

1. Add canonical `ProfileConsumer` values for `metrics` and `topology`.
2. Add a single canonical closed `TopologyKind` definition with the 18 accepted topology row kinds. Do not include `sys_uptime`.
3. Avoid duplicated string constants. If multiple packages need the type, use one definition plus aliases/imports rather than parallel enums.
4. Add `Topology []TopologyConfig` to `ProfileDefinition`.
5. Add `TopologyConfig` as a wrapper around the existing metric row shape plus required `kind`.
6. Add `Consumers` to metadata fields.
7. Add a top-level/global metric-tag wrapper that carries `Consumers`. Do not add consumer semantics to per-row `MetricTagConfig`; per-row metric tags inherit their row's consumer.
8. Follow the existing schema tag convention for every new persisted field: matching `yaml` and `json` names, `omitempty` for optional fields, and `yaml:"-" json:"-"` for runtime-only fields.
9. Update clone/deep-copy paths explicitly: `ProfileDefinition.Clone()` must clone `Topology`; `TopologyConfig.Clone()` must clone the embedded row config; `MetadataField.Clone()` must clone/copy `Consumers`; top-level/global metric-tag wrapper clone paths must clone/copy `Consumers`.
10. Add validation for unknown topology kinds.
11. Add validation for metrics-only fields inside topology row anchor `symbol`/`symbols`: `Options`, `ChartMeta`, `MetricType`, `Mapping`, `Transform`, `ScaleFactor`, `Format`, and `ConstantValueOne`.
12. Extend the existing symbol traversal context, either with `TopologyScalarSymbol`/`TopologyColumnSymbol` or a topology-row flag, so value-symbol validation can reject topology-only-invalid fields without breaking `MetricTagSymbol`.
13. Extend `validateEnrichVirtualMetrics` to accept topology rows and reject any virtual metric source that resolves to a `topology:` row.
14. Reject underscore-prefixed `name:` values on topology row anchor symbols so topology rows cannot also flow into generic `HiddenMetrics`.
15. Keep metric-tag extraction fields valid under topology rows where they extract tags from indexes/values.
16. Do not reject existing `_topology_*` rows in `metrics:` until the YAML migration and cutover are complete; add final rejection only in cleanup.
17. Add parse/clone/validation tests in `ddprofiledefinition`.

#### 2. Split Profile Mutations By Required Context

1. Inventory every mutation that currently happens after profile load: `handleCrossTableTagsWithoutMetrics`, `enrichProfiles`, `deduplicateMetricsAcrossProfiles`, and any resolver-time mutation found while editing.
2. Move `handleCrossTableTagsWithoutMetrics` out of `ddsnmpcollector.New()` and into catalog/profile compilation before projections share profile pointers.
3. Extend `Profile.merge()` with `mergeTopology(base)` and call it during `extends:` loading.
4. `mergeTopology(base)` must use topology row identity `(kind, table_identity, symbol_name)` and Decision 2.B merge semantics.
5. Add `profile_test.go` coverage proving a profile extending a topology mixin inherits `Definition.Topology` rows.
6. Extend cross-table-tag synthesis to scan both `Definition.Metrics` and `Definition.Topology`; place synthesized entries on the slice that owns the consuming row so the correct collection path walks them.
7. Move `enrichProfiles()` to load/catalog compilation and extend it to process `Definition.Topology`.
8. Keep `deduplicateMetricsAcrossProfiles()` at resolve-time inside `Catalog.Resolve()` because it needs the already-matched, specificity-sorted profile set. It must operate on catalog-cloned profiles before projections are returned.
9. Extend resolve-time deduplication to process `Definition.Topology` with topology row identity `(kind, table_identity, symbol_name)`.
10. Preserve `Definition.Metrics` behavior exactly for regular SNMP.
11. Treat the suspected `removeConstantMetrics()` value-copy issue as separate unless touched by this refactor. Do not extend `removeConstantMetrics()` to topology because topology row validation rejects `ConstantValueOne`; if this changes or the existing value-copy bug becomes relevant, either fix it with a narrow test or record a separate follow-up SOW.
12. Add mutation-isolation tests showing one projected view cannot mutate another or the catalog. The minimum assertion is: resolve a device twice, mutate a nested map/slice in view 1, including nested state under `Definition.Topology`, then assert view 2 and a fresh resolve from the same catalog do not contain that mutation.

#### 3. Migrate Profile YAML

1. Move topology rows from `metrics:` to top-level `topology:` in the topology mixins.
2. Split `_std-cdp-mib.yaml` into regular CDP metrics and `_std-topology-cdp-mib.yaml` topology rows.
3. Add `kind:` to every topology row using the accepted `TopologyKind` enum.
4. Rename topology row anchor symbol names away from `_topology_*`; dispatch must use `kind`, not the old hidden-metric name.
5. Preserve OIDs, table identities, symbols, and metric_tags unless the move exposes a concrete bug.
6. Do not modify `_system-base.yaml`; `systemUptime` remains a regular metrics row. Topology obtains uptime through `pkg/snmputils.GetSysUptime`.
7. If any symbol/tag OID is added or changed, run the SNMP profile authoring MAX-ACCESS checks and record evidence. Pure row moves with unchanged OIDs should record that no readable-symbol semantics changed.
8. Update profile extender references so vendors that previously extended mixed CDP/topology content still get the intended regular and topology rows.
9. Inventory every profile extending `_std-cdp-mib.yaml` and update each profile that should retain CDP topology to also extend `_std-topology-cdp-mib.yaml`. Current default extenders are `_cisco-base.yaml` and `cisco-sb.yaml`.
10. Add or update profile load tests for the migrated files.
11. Treat phases 3-6 as one logical topology cutover. Do not ship a state where topology YAML has moved to `topology:` but topology runtime still reads only `_topology_*` metrics from `metrics:`.
12. Record that topology mixins may become topology-only/abstract after migration; validation/tests must not assume every topology mixin produces regular `metrics:` rows when loaded as a root.

#### 4. Introduce Catalog, Resolve, Project, And Filter

1. Add `Catalog`, `ResolveRequest`, `ManualProfilePolicy`, `ResolvedProfileSet`, and projected view types.
2. Implement the catalog/resolver/projection API in `collector/snmp/ddsnmp`, next to the existing profile loader, `FindProfiles()`, and profile model.
3. Implement manual profile policies: metrics call sites use fallback-only; topology call sites use augment.
4. Implement `Project(ConsumerMetrics)` and `Project(ConsumerTopology)`.
5. Implement `FilterByKind(map[TopologyKind]bool)` for topology projections.
6. Make projections non-mutating. Prefer immutable catalog-owned profiles plus read-only projected slices or precomputed buckets over per-call deep clone.
7. Keep temporary wrappers such as `FindProfiles()` only as needed to prove parity during the same SOW; remove or simplify them during cleanup.
8. Add resolver parity tests for regular SNMP.
9. Add topology projection parity tests against the current topology filter behavior after YAML migration.
10. Add manual-policy tests covering matching `sysObjectID` plus `manual_profiles` for both metrics and topology.

#### 5. Plumb Topology Collection Through ddsnmpcollector

1. Add `TopologyKind` to emitted `ddsnmp.Metric`.
2. Add `TopologyMetrics []Metric` to `ProfileMetrics`.
3. Preserve `HiddenMetrics []Metric` as the generic underscore-prefixed non-topology delivery container.
4. Add explicit topology collection from `Definition.Topology`, parallel to the existing scalar/table collection path.
5. Prefer a topology collection wrapper that calls the existing scalar/table collection helpers and stamps `TopologyKind` after emit. Do not widen regular scalar/table builder signatures unless the wrapper proves insufficient and the SOW records why.
6. Add `pkg/snmputils.GetSysUptime(gosnmp.Handler)` with the `_system-base.yaml` uptime OIDs and scale rules, and call it from `snmp_topology` refresh. Do not add duplicate `systemUptime` YAML topology rows or topology kinds.
7. Stop relying on underscore-prefix metric names for topology delivery.
8. Keep `collectHiddenMetrics()` behavior for non-topology underscore-prefixed regular metrics.
9. Enforce the invariant that a topology row lives in exactly one delivery slice for a single poll: not both `pm.HiddenMetrics` and `pm.TopologyMetrics`.
10. Ensure `TestCollector_Collect_PreservesHiddenMetrics` continues to pass.
11. Add tests proving topology rows populate `pm.TopologyMetrics` with the correct `Metric.TopologyKind`.
12. Add a double-bucketing assertion test that fails if a `_topology_*` or topology-kind row appears in both `pm.HiddenMetrics` and `pm.TopologyMetrics`.

#### 6. Cut Over SNMP And Topology Call Sites

1. Switch regular SNMP profile selection to `DefaultCatalog().Resolve(...).Project(ConsumerMetrics)`.
2. Switch SNMP topology profile selection to `DefaultCatalog().Resolve(...).Project(ConsumerTopology)`.
3. Replace VLAN-context hardcoded `LoadProfileByName()` calls with `Project(ConsumerTopology).FilterByKind(vlanScopableKinds)`.
4. Define `vlanScopableKinds` in topology Go code, not profile YAML. Pin it to the current VLAN-context ingest set: `KindIfName`, `KindBridgePortIfIndex`, `KindFdbEntry`, and `KindStpPort`.
5. Replace metric-name topology dispatch with a handler registry keyed by `TopologyKind`.
6. Make topology row handlers receive the full `ddsnmp.Metric`; uptime is handled by the explicit `snmputils` helper path, not by the topology handler registry.
7. Register each topology cache handler from its domain file.
8. Extend `updateTopologyProfileTags` to read `pm.Tags` and apply them as local device/profile labels, not per-row dispatch keys.
9. Update `ingestTopologyVLANContextMetrics()` so any synthetic metric passed to kind-keyed dispatch carries/preserves `TopologyKind`.
10. Update `ingestTopologyProfileMetrics` so the post-cutover loop dispatches only `pm.TopologyMetrics` by `TopologyKind`; regular `pm.Metrics` are not part of topology projection.
11. Add dispatch parity tests and VLAN-context equivalence tests, including `TopologyKind` population for VLAN-context synthetic metrics.
12. Add a side-by-side fixture-level runtime parity test before deleting the old path. Use a representative default-profile fixture such as the existing Cisco Nexus profile path in `collector/snmp/ddsnmp/profile_test.go:178`, and compare emitted `ProfileMetrics` fields that must remain stable for regular SNMP: `Tags`, `DeviceMetadata`, `Metrics`, ordering, and metric tags.

#### 7. Delete Dead Code And Update Artifacts

1. Search for stale references before deleting: `_topology_`, `IsTopologyMetric`, `LooksLikeTopologyIdentifier`, `MetricConfigContainsTopologyData`, `MetricTagConfigContainsTopologyData`, `MetadataFieldContainsTopologyData`, `MetadataContainsTopologyData`, `SysobjectIDMetadataContainsTopologyData`, `ProfileContainsTopologyData`, `ProfileHasCollectionData`, `TopologySysUptime`, `LoadProfileByName`, and `HiddenMetrics`.
2. Delete dead topology classifier/filter code only after call sites and tests no longer depend on it.
3. Delete `FinalizeProfiles()` only after VLAN-context and other callers no longer require it.
4. Delete only topology-specific use of hidden metrics; do not delete generic `HiddenMetrics`.
5. Update `collector/snmp/profile-format.md`.
6. Update `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
7. Create `.agents/sow/specs/snmp-profile-projection.md`.
8. Run the focused validation suites.
9. Run same-failure/stale-reference scans and record results in this SOW.
10. Complete the artifact maintenance gate and follow-up mapping.
11. Explicit test impact:
    - Delete/rewrite `collector/snmp/ddsnmp/topology_classify_test.go`.
    - Rewrite `collector/snmp/profile_sets_test.go` around projection.
    - Rewrite `collector/snmp_topology/profile_filter_test.go` around projection.
    - Rewrite `collector/snmp_topology/topology_profiles_test.go` if hardcoded topology profile constants disappear.
    - Rewrite `collector/snmp/ddsnmp/ddsnmpcollector/topology_profile_index_test.go` if it no longer needs `LoadProfileByName()`.
    - Rewrite topology fixtures in `collector/snmp_topology/topology_cache_test.go:1327-1371` and `collector/snmp_topology/collector_refresh_test.go:123-152` to use `TopologyMetrics`, not topology `HiddenMetrics`.
    - Preserve `collector/snmp/ddsnmp/ddsnmpcollector/collector_test.go:153` `TestCollector_Collect_PreservesHiddenMetrics`.

## Execution Log

### 2026-05-06

- Created pending SOW from the reviewed design in `src/go/plugin/go.d/TODO-snmp-profile-loading-topology.md`.
- Filled Pre-Implementation Gate with accepted decisions, implementation sub-decisions, risks, validation, and artifact impact plan.
- Added step-by-step implementation plan for external readiness review before code.
- Reconciled fifth readiness review: accepted B1-B6 and S1-S6, split load-time vs resolve-time mutation handling, made phases 3-6 one logical topology cutover, kept `_system-base.yaml` unchanged, and added validation/test gates.
- Reconciled sixth readiness review: accepted N1 and N2-N5 plus actionable minor notes, added `Profile.merge().mergeTopology(base)` to the plan, enumerated clone/tag conventions, pinned VLAN-context kinds, required VLAN-context synthetic metrics to carry `TopologyKind`, kept `removeConstantMetrics()` metrics-only, and added CDP extender inventory.
- Reconciled seventh readiness review: verdict `READY TO WRITE CODE`; folded in non-blocking NB1-NB4 hygiene around stale-helper scans, API package location, global metric-tag consumer wrapper, and post-cutover topology ingest loop shape.
- User approved proceeding with implementation.
- Activated SOW on branch `snmp-profile-projection`.
- Dirty files at activation: `src/go/plugin/go.d/TODO-snmp-profile-loading-topology.md` and `.agents/sow/current/SOW-0012-20260506-snmp-profile-projection.md`.
- Phase 0 baseline checks started before implementation code edits.
- Phase 0 baseline checks completed:
  - `go test ./plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition`: passed from cache.
  - `go test ./plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector`: passed.
  - `go test ./plugin/go.d/collector/snmp/...`: passed.
  - `go test ./plugin/go.d/collector/snmp_topology/...`: initial sandbox run failed because the Go build cache was not writable; rerun outside sandbox passed.
- Phase 1 schema/API surface completed:
  - Added canonical profile consumers, 18-value topology kind enum, `ProfileDefinition.Topology`, `TopologyConfig`, metadata `Consumers`, and top-level/global metric-tag consumer wrapper.
  - Added clone support for `Topology`, metadata consumers, and global metric-tag consumers.
  - Added validation for unknown topology kinds, topology-row metrics-only fields, underscore-prefixed topology symbols, invalid/duplicate consumers, and virtual metrics that source topology rows.
  - Kept per-row metric tags on the existing `MetricTagConfig` type; only top-level/global `metric_tags` use the consumer wrapper.
  - Updated transitional callers/tests that consume top-level/global metric tags.
- Phase 1 validation:
  - `go test ./collector/snmp/ddsnmp/ddprofiledefinition`: passed.
  - `go test ./collector/snmp/ddsnmp/ddsnmpcollector`: passed.
  - `go test ./collector/snmp/...`: passed.
  - `go test ./collector/snmp_topology/...`: passed.
- Phase 2 mutation split completed:
  - Moved cross-table synthetic row preparation from `ddsnmpcollector.New()` into `ddsnmp` load/profile preparation.
  - Kept resolve-time cross-profile dedup in `deduplicateMetricsAcrossProfiles()` and extended it to `Definition.Topology`.
  - Added `Profile.merge().mergeTopology(base)` with topology identity `(kind, table_identity, symbol_name)` and conflict rejection for same row/different kind.
  - Extended load-time `enrichProfile()` mapping-ref handling to `Definition.Topology`.
  - Extended cross-table synthesis to scan both `Definition.Metrics` and `Definition.Topology`, placing synthetic rows on the owning slice.
  - Preserved `ddsnmp.FinalizeProfiles()` as the temporary programmatic preparation path until its planned cleanup.
  - Projection mutation-isolation coverage remains tied to Phase 4 because the `Catalog.Resolve().Project()` view does not exist yet.
- Phase 2 validation:
  - `go test ./collector/snmp/ddsnmp/...`: passed.
  - `go test ./collector/snmp/...`: passed.
  - `go test ./collector/snmp_topology/...`: passed.
- Phase 3 profile YAML migration completed:
  - Moved topology rows from `metrics:` to top-level `topology:` in `_std-topology-lldp-mib.yaml`, `_std-topology-fdb-arp-mib.yaml`, `_std-topology-q-bridge-mib.yaml`, `_std-topology-stp-mib.yaml`, and `_std-topology-cisco-vtp-mib.yaml`.
  - Split CDP topology into `_std-topology-cdp-mib.yaml` and updated `_cisco-base.yaml` plus `cisco-sb.yaml` to extend it.
  - Renamed topology row anchor symbols away from `_topology_*` and assigned explicit `kind:` values.
  - `_system-base.yaml` was not modified; `systemUptime` remains a regular metric.
- Phase 4 catalog/projection completed:
  - Added `Catalog.Resolve()`, manual profile policies, `ResolvedProfileSet.Project()`, and `ProjectedView.FilterByKind()`.
  - Regular SNMP projection keeps regular metrics and virtual metrics; topology projection keeps topology rows and drops regular metrics.
  - Added projection separation and mutation-isolation tests.
- Phase 5 topology collection completed:
  - Added `Metric.TopologyKind` and `ProfileMetrics.TopologyMetrics`.
  - Added topology collection wrapper over existing scalar/table collectors without widening regular builder signatures.
  - Preserved generic `HiddenMetrics` for non-topology underscore-prefixed metrics.
  - Added a double-bucketing guard test proving topology rows are delivered through `TopologyMetrics`, not `HiddenMetrics`.
- Phase 6 call-site cutover completed:
  - Regular SNMP uses `DefaultCatalog().Resolve(...).Project(ConsumerMetrics)`.
  - SNMP topology uses `DefaultCatalog().Resolve(...).Project(ConsumerTopology)`.
  - VLAN-context topology uses projection plus `FilterByKind(vlanScopableKinds)` instead of hardcoded mixin filename loads.
  - Topology dispatch now uses `TopologyKind`.
  - `updateTopologyProfileTags` applies `pm.Tags` as local device/profile labels.
- User selected option A for topology uptime after removing `KindSysUptime`.
- Removed `KindSysUptime`; topology now queries uptime through `pkg/snmputils.GetSysUptime`, while regular SNMP uptime remains profile-driven in `_system-base.yaml`.
- Reconciled final Claude close-out review:
  - Accepted B1 and filled `Outcome` plus `Lessons Extracted`.
  - Accepted NF1 as a prose-only correction because `kind` is required by validation, so kind inheritance from an omitted derived kind is unreachable.
  - Accepted NF2 by recording that existing resolver/profile fixture tests are the parity proxy rather than a now-impossible side-by-side old-path test.
  - Accepted NF7 by mapping retained `LoadProfileByName` and `FinalizeProfiles` as explicit retained APIs rather than hidden deferred cleanup.
  - Rejected NF5 as stale because the `KindSysUptime`/`IsTopologySysUptimeMetric` predicates no longer exist after the uptime-helper change.
- Removed old-implementation wording about underscore-prefixed topology metric names from `collector/snmp/profile-format.md`; public docs now describe only the current `topology:` schema.
- Phase 7 cleanup/artifacts completed:
  - Deleted the topology classifier, regular/topology filter files, legacy topology profile constants, and obsolete metric-name dispatch constants.
  - Rewrote/deleted affected tests around projection and topology-kind delivery.
  - Updated `collector/snmp/profile-format.md`.
  - Updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
  - Created `.agents/sow/specs/snmp-profile-projection.md`.
  - Runtime topology no longer calls `LoadProfileByName()`. The helper remains for intentional abstract-profile tests/programmatic checks because the normal catalog skips `_std-*` abstract roots.
  - `ddsnmp.FinalizeProfiles()` remains because synthetic `ddsnmpcollector` table tests still require programmatic profile preparation outside the catalog path; topology runtime no longer depends on it.

## Validation

Acceptance criteria evidence:

- Regular SNMP now selects profiles through `DefaultCatalog().Resolve(...ManualProfileFallback).Project(ConsumerMetrics)` in `collector/snmp/profile_sets.go`.
- SNMP topology now selects profiles through `DefaultCatalog().Resolve(...ManualProfileAugment).Project(ConsumerTopology)` in `collector/snmp_topology/collector.go`.
- VLAN-context topology now calls projection plus `FilterByKind(vlanScopableKinds)` in `collector/snmp_topology/topology_vlan_context_collect.go`.
- Topology rows are emitted through `ProfileMetrics.TopologyMetrics` and carry `Metric.TopologyKind`.
- Topology uptime is queried through `pkg/snmputils.GetSysUptime`; it is not a `TopologyKind`.
- Generic non-topology hidden metrics are preserved by `TestCollector_Collect_PreservesHiddenMetrics`.
- Topology double-bucketing is guarded by `TestCollector_Collect_SeparatesTopologyMetricsFromHiddenMetrics`.
- Old classifier/filter source files and hardcoded topology profile constants were deleted.

Tests or equivalent validation:

- `go test -count=1 ./pkg/snmputils`: passed.
- `go test -count=1 ./collector/snmp/ddsnmp/...`: passed.
- `go test -count=1 ./collector/snmp/...`: passed.
- `go test -count=1 ./collector/snmp_topology/...`: passed.
- `go test -race -count=1 ./pkg/snmputils`: passed.
- `go test -race -count=1 ./collector/snmp/ddsnmp/...`: passed.
- `go test -race -count=1 ./collector/snmp/...`: passed.
- `go test -race -count=1 ./collector/snmp_topology/...`: passed.
- `git diff --check`: passed.
- Same-failure search over code/artifacts confirmed no code references to `KindSysUptime`, `IsTopologySysUptimeMetric`, `updateTopologyScalarMetric`, or `ingestTopologySysUptimeMetricSet` remain.
- The planned pre-deletion side-by-side old/new runtime parity test was not added because the old classifier/filter path is now removed. Existing `FindProfiles`/catalog resolver tests, profile merge tests, Cisco Nexus fixture coverage, and focused collector suites are the retained parity proxy.

Real-use evidence:

- No live SNMP device was used. This SOW changes profile schema/loading/dispatch internals; validation used default profile loading, synthetic SNMP handler tests, and topology cache/collector unit tests.

Reviewer findings:

- Multiple read-only AI reviews were summarized and reconciled in `src/go/plugin/go.d/TODO-snmp-profile-loading-topology.md`.
- Fourth readiness review recorded `READY TO PROCEED` after the `HiddenMetrics` constraint and implementation sub-decisions were added.
- Fifth readiness review recorded `NEEDS ADJUSTMENTS`; accepted blocking adjustments B1-B6 and suggestions S1-S6 are reflected in this SOW before implementation starts.
- Sixth readiness review recorded `NEEDS ADJUSTMENTS`; accepted blocker N1, tightenings N2-N5, and actionable minor notes are reflected in this SOW before implementation starts.
- Seventh readiness review recorded `READY TO WRITE CODE`; non-blocking hygiene tightenings NB1-NB4 are reflected in this SOW.

Same-failure scan:

- `rg` found no stale classifier/filter/helper references for `IsTopologyMetric`, `LooksLikeTopologyIdentifier`, `MetricConfigContainsTopologyData`, `MetricTagConfigContainsTopologyData`, `MetadataFieldContainsTopologyData`, `MetadataContainsTopologyData`, `SysobjectIDMetadataContainsTopologyData`, `ProfileContainsTopologyData`, `ProfileHasCollectionData`, `selectTopologyRefreshProfiles`, `selectCollectionProfiles`, or legacy topology metric-name constants.
- `_topology_` remains only in topology chart/tag names, build tags, and validation tests that intentionally reject underscore-prefixed topology row names.
- `HiddenMetrics` remains only in the generic collector path and non-topology hidden-metric tests.
- `LoadProfileByName` remains as an abstract-profile helper; runtime topology call sites no longer use it.

Sensitive data gate:

- This SOW contains only file paths, public OIDs/metric names, struct/function names, and design decisions. It contains no raw credentials, SNMP communities, bearer tokens, customer names, personal data, customer-identifying IPs, private endpoints, or proprietary incident details.

Artifact maintenance gate:

- AGENTS.md: no update needed; existing SOW/process and SNMP skill triggers were sufficient.
- Runtime project skills: updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md` with top-level `topology:` and `TopologyKind` authoring rules.
- Specs: created `.agents/sow/specs/snmp-profile-projection.md`.
- End-user/operator docs: updated `collector/snmp/profile-format.md` with topology rows, consumers, topology kinds, and hidden-metric guidance.
- End-user/operator skills: no public/operator skill update needed; this changes profile authoring guidance, which is covered by the runtime project skill and profile-format doc.
- SOW lifecycle: completed and moved to `done/` during commit prep; status/directory are consistent.

Specs update:

- Created `.agents/sow/specs/snmp-profile-projection.md`.

Project skills update:

- Updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.

End-user/operator docs update:

- Updated `collector/snmp/profile-format.md`.

End-user/operator skills update:

- No update required; no public skill artifact describes SNMP profile schema authoring.

Lessons:

- Normal catalog resolution intentionally excludes abstract `_std-*` profiles. Tests or programmatic checks that need a specific abstract mixin still need an explicit by-name loader; topology runtime should not use that path.
- `FinalizeProfiles()` is still useful for synthetic collector tests that construct profiles outside the catalog. Treat it as programmatic preparation, not a topology runtime dependency.

Follow-up mapping:

- The probable `removeConstantMetrics()` value-copy bug in `collector/snmp/ddsnmp/profile.go` was not touched by this SOW because topology validation rejects `constant_value_one` rows and regular metric behavior did not require changing that path.
- `LoadProfileByName` is intentionally retained as a programmatic abstract-profile loader for tests/checks such as `topology_profile_index_test.go`; topology runtime no longer calls it.
- `FinalizeProfiles` is intentionally retained as a programmatic profile preparation helper for synthetic `ddsnmpcollector` tests that build profiles outside the catalog path; topology runtime no longer calls it.

## Outcome

Implementation is ready to commit.

- Regular SNMP profile selection now uses the catalog resolver with metrics projection and fallback-only manual-profile semantics.
- SNMP topology now uses topology projection, explicit `TopologyKind` row dispatch, `ProfileMetrics.TopologyMetrics`, and VLAN-context kind filtering instead of hardcoded topology profile filenames.
- Topology profile YAML rows moved to top-level `topology:` with explicit `kind`; CDP regular metrics and topology rows are split.
- `HiddenMetrics` remains a generic non-topology underscore-prefixed delivery path.
- `KindSysUptime` was removed; topology queries uptime through `pkg/snmputils.GetSysUptime`, and regular SNMP uptime remains profile-driven in `_system-base.yaml`.
- Docs, runtime SNMP profile authoring skill, and the SNMP projection spec were updated.

## Lessons Extracted

- Required topology `kind` makes "inherit base kind when derived omits kind" unreachable; durable design text must say explicit kind plus conflict rejection.
- `LoadProfileByName` and `FinalizeProfiles` are no longer topology runtime dependencies, but they remain useful programmatic test/preparation helpers until those synthetic paths are redesigned.
- A direct `snmputils` helper is cleaner for topology-only uptime enrichment than keeping a fake topology kind or retaining regular metrics in topology projection.
- Readiness reviews can go stale quickly during active edits; final close-out review points must be checked against the current diff before accepting them.

## Followup

- Track the probable `removeConstantMetrics()` value-copy bug separately if regular metric behavior requires touching that path.
- Consider a validation rule against duplicate `(table, symbol name)` topology collection rows with different kinds if real profiles ever create an ambiguous `TopologyKind` stamping case.
- Consider strict YAML/schema validation for unknown keys in per-row `metric_tags` if author typo detection becomes important.
- Consider a dedicated VLAN-context projection unit test beyond current transitive topology suite coverage.
- Keep `LoadProfileByName` and `FinalizeProfiles` only as programmatic helpers; revisit cleanup if synthetic tests no longer need them.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
