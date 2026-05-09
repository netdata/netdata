# SOW-0015 - SNMP BGP typed profile projection

## Status

Status: in-progress

Sub-state: reopened for final review-feedback fixes before merge; user decisions 1-23 resolved; all BGP-bearing stock profiles migrated to typed `bgp:` rows; legacy `bgp_public*` runtime deleted; metadata and generated SNMP integration docs regenerated; local `mibs/` reference files removed.

## Requirements

### Purpose

Replace the WIP SNMP BGP monitoring branch's metric-name/tag routing layer with a clean typed BGP profile section and projection. Preserve normal ddsnmp chart-per-row behavior, keep BGP per-peer/per-family health templates enabled by default, and remove unsupported public coverage claims until real fixture evidence exists.

Scope expansion on 2026-05-09: normalize typed SNMP profile organization before SOW close by moving remaining inline `bgp:` and `licensing:` sections into dedicated underscore-prefixed typed-domain fragments where practical.

### User Request

User request summary:

- Analyze the `pr-ktsaou-snmp-bgp` branch after merge/rebase work.
- Find hacks, smells, correctness risks, missing tests, and clean-end-state refactor directions.
- Prefer a clean end state over low churn or backward compatibility with this WIP branch.
- Use multiple external review agents, reconcile their findings, and record decisions before implementation.
- Use a typed `bgp:` projection rather than the current stringly typed BGP public-metric router.

Detailed design source:

- `src/go/plugin/go.d/TODO-snmp-bgp-monitoring-review.md` (local development TODO for this work).

### Assistant Understanding

Facts:

- The branch implements BGP as ordinary SNMP metrics plus a rewrite layer in `collector/snmp/bgp_public*.go`.
- The rewrite layer routes by metric name, infers peer vs peer-family scope from tags, and uses underscore-prefixed labels as an in-band protocol.
- `ddsnmp` already has typed projection precedents: `topology:` and `licensing:` sections, `ProfileMetrics.TopologyMetrics`, and `ProfileMetrics.LicenseRows`.
- The existing generic ddsnmp behavior creates one chart per table metric row. The user explicitly decided BGP must keep that behavior and must not add BGP-specific caps.
- The existing BGP health templates should remain default-on; no alert test framework will be created in this SOW.
- Cumulus and Alcatel BGP capability claims must be removed until real fixtures exist.
- Downloaded BGP MIB files are local reference evidence under untracked `mibs/`; they must not be committed and must be removed before close-out.

Inferences:

- The root problem is architectural: BGP has typed rows, identities, state enums, AFI/SAFI enums, and fixed signal classes, but the branch models these as string-routed chart metrics.
- Fixing individual switch cases would leave the same hidden-protocol class of technical debt that topology and licensing migrations removed.
- The clean migration should be schema-first, then runtime projection, then vendor-by-vendor profile migration, then deletion of the old router.

- Unknowns:

- No open user decisions remain.
- Real Cumulus and Alcatel fixtures are not available now; coverage claims will be removed until they exist.
- Cisco profile attachment requires a profile audit before Cisco migration, but it does not block schema/API scaffolding.

### Acceptance Criteria

- SNMP BGP profile data is represented by a first-class typed `bgp:` profile section/projection, not the `bgp_public*` metric-name router.
- `ProfileMetrics` exposes typed `BGPRows` or an equivalent typed BGP output shape.
- BGP function/cache/charts read typed BGP rows, not underscore-prefixed labels or `bgp_public*`-normalized ordinary metrics.
- `bgp_public.go`, `bgp_public_routes.go`, `bgp_public_specs.go`, and related old-routing paths are deleted by the final migration.
- The underscore-prefix BGP tag protocol is deleted by the final migration.
- BGP structural identity uses a canonical length-prefixed key derived from typed identity fields, not display labels or underscore-concatenated strings.
- AFI/SAFI values use closed canonical enums with an explicit vendor-private allow-list.
- BGP state mappings cover all six RFC 4271 peer states unless a row explicitly declares partial coverage.
- Profile validation rejects readable-symbol use of MIB `MAX-ACCESS not-accessible` objects; such objects must be derived from row indexes.
- BGP profile projection is profile-gated: `snmp:bgp-peers` is exposed only for BGP-capable jobs/devices.
- Normal ddsnmp chart-per-row behavior is preserved; no BGP-specific peer/peer-family chart caps are introduced.
- Per-peer/per-family BGP health templates remain default-on.
- Stale cache semantics are explicit: function output distinguishes fresh, stale-within-TTL, stale-expired, no-data-yet, and valid filtered-empty states; chart-side collection freshness is exposed.
- Cisco BGP is removed from `_cisco-base.yaml` and attached only to audited router/L3-switch Cisco profiles.
- Cumulus and Alcatel BGP capability claims are removed until real fixtures exist.
- Each migrated vendor has production-shaped full-profile tests through normal profile loading/resolution/finalization.
- `.agents/sow/specs/snmp-profile-projection.md` documents the BGP projection contract.
- Metadata/docs/generated integration artifacts are coherent after final migration.
- Typed-domain SNMP profile sections are organized as dedicated fragments where practical; concrete device profiles extend those fragments instead of mixing typed-domain sections with regular metric sections.
- Local raw MIB files under `mibs/` are not committed and are removed before SOW close-out.

## Analysis

Sources checked:

- `src/go/plugin/go.d/TODO-snmp-bgp-monitoring-review.md`
- `collector/snmp/bgp_public.go`
- `collector/snmp/bgp_public_routes.go`
- `collector/snmp/bgp_public_specs.go`
- `collector/snmp/bgp_integration.go`
- `collector/snmp/collect_snmp.go`
- `collector/snmp/charts.go`
- `collector/snmp/metric_ids.go`
- `collector/snmp/func_bgp_peers.go`
- `collector/snmp/func_bgp_peers_cache.go`
- `collector/snmp/ddsnmp/ddprofiledefinition/profile_definition.go`
- `collector/snmp/ddsnmp/metric.go`
- `collector/snmp/metadata.yaml`
- `collector/snmp/integrations/snmp_devices.md`
- `src/health/health.d/snmp_bgp.conf`
- `config/go.d/snmp.profiles/default/*bgp*.yaml`
- `config/go.d/snmp.profiles/default/cisco*.yaml`
- `config/go.d/snmp.profiles/default/_cisco*.yaml`
- `.agents/sow/specs/snmp-profile-projection.md`
- `.agents/skills/project-snmp-profiles-authoring/SKILL.md`
- `.agents/skills/project-writing-collectors/SKILL.md`
- `.agents/skills/integrations-lifecycle/SKILL.md`
- Local untracked BGP MIB files under `mibs/`

Current state:

- `profile_definition.go` supports `metrics`, `topology`, and `licensing`, but no `bgp`.
- `ProfileMetrics` supports regular `Metrics`, `TopologyMetrics`, and `LicenseRows`, but no typed BGP rows.
- `bgp_public_routes.go` routes raw vendor metric names through a large switch and decides peer vs peer-family scope from row tags.
- `bgp_public_specs.go` separately defines public BGP chart specs.
- `func_bgp_peers_cache.go` keeps another switch-like mapping from public BGP leaf names into function cache fields.
- `metric_ids.go` derives BGP chart IDs and contexts through `strings.HasPrefix(name, "bgp.")`.
- `collect_snmp.go` and `charts.go` show the generic ddsnmp chart-per-row behavior that BGP must preserve.
- `src/health/health.d/snmp_bgp.conf` defines 7 default-on BGP health templates.
- `collector/snmp/metadata.yaml` lists 7 BGP alerts and includes Cumulus/Alcatel capability claims that lack real fixture backing.

Risks:

- Regular SNMP metrics can regress if BGP migration changes generic profile projection or charting paths.
- BGP function discovery can remain noisy unless `funcRouter` registration becomes profile-aware.
- BGP charts and function output can freeze on stale SNMP data unless freshness is modeled for both chart and function surfaces.
- Vendor profile migration can break real device coverage if MIB index accessibility or Cisco profile attachment scope is guessed instead of audited.
- Integration artifacts can drift unless metadata, generated docs, health alerts, config, and docs are closed together.
- Raw local MIBs and fixture provenance must not leak into committed artifacts.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- BGP monitoring is currently implemented as a post-collection public-metric rewrite. The code takes ordinary SNMP metrics, routes by name, infers row kind from tag presence, carries hidden label fields through underscore-prefixed tags, and then builds chart/function surfaces from those derived strings. This repeats the hidden-protocol architecture already removed from SNMP topology and licensing. BGP needs the same typed projection treatment because its row kinds, identities, states, AFI/SAFI values, and signals are domain data, not chart naming accidents.

Evidence reviewed:

- `collector/snmp/bgp_public_routes.go` contains the metric-name routing layer and peer/peer-family inference.
- `collector/snmp/bgp_public_specs.go` contains separate public chart specs that must stay in sync with routes.
- `collector/snmp/func_bgp_peers_cache.go` contains a separate BGP leaf-to-function-field mapping.
- `collector/snmp/bgp_public_routes.go` and `func_bgp_peers_cache.go` carry underscore-prefixed hidden label handling.
- `collector/snmp/metric_ids.go` uses `strings.HasPrefix(name, "bgp.")` as a chart context contract.
- `collector/snmp/ddsnmp/ddprofiledefinition/profile_definition.go` and `collector/snmp/ddsnmp/metric.go` show typed topology/licensing projections but no BGP projection.
- `collect_snmp.go` and `charts.go` show existing ddsnmp chart-per-row behavior.
- `src/health/health.d/snmp_bgp.conf` shows 7 default BGP alert templates.
- Local untracked MIBs show `MAX-ACCESS not-accessible` peer index objects that must not be configured as readable symbols.

Affected contracts and surfaces:

- SNMP profile schema and validation.
- ddsnmp catalog projection and profile consumers.
- ddsnmpcollector typed output and stats.
- SNMP collector BGP charts, functions, cache, stale semantics, and health alert inputs.
- Default SNMP profile YAMLs for standard BGP4-MIB, Cisco, Juniper, Nokia, Huawei, Arista, Dell, Cumulus, Alcatel, and related mixins.
- SNMP metadata and generated integration artifacts.
- `src/health/health.d/snmp_bgp.conf`.
- SNMP profile projection spec.
- Runtime SNMP profile authoring skill if authoring rules change.

Existing patterns to reuse:

- Top-level typed `topology:` and `licensing:` profile sections.
- `ResolvedProfileSet.Project(metrics, licensing)` variadic projection pattern.
- Licensing row structural identity and inheritance override validation patterns.
- Table/index `MAX-ACCESS not-accessible` discipline from `project-snmp-profiles-authoring`.
- Existing ddsnmp scalar/table collection, table cache, stats, chart construction, and row reaping paths.
- Table-driven Go tests using `map[string]struct{}` for shared setup/assertion shapes.

Risk and blast radius:

- The code touches a broad collector and many vendor SNMP profiles. Keep the first slice schema/API-only where possible.
- Backward compatibility with this WIP BGP implementation is not required, but ordinary SNMP metrics must not regress.
- Vendor profile migrations must be small enough to validate independently.
- Function registration lifecycle changes may affect other SNMP functions if done at the shared router layer.
- Health alerts remain default-on; chart context and dimension names must be statically reviewed until/unless a project alert test framework exists.
- Removing Cumulus/Alcatel claims can reduce advertised coverage, but it removes unsupported public debt.

Sensitive data handling plan:

- Do not commit local raw MIB files under `mibs/`.
- Do not commit SNMP communities, SNMPv3 credentials, bearer tokens, customer hostnames, customer sysName/sysDescr, customer IPs, customer names, personal data, or private endpoints.
- Any fixture provenance must use public repository/commit/relative-path evidence or sanitized text, never workstation-local paths.
- SOW/spec/docs/code comments must cite MIB facts by object name, table, and relative local evidence only while MIB files remain untracked; raw MIB text is not copied into durable artifacts.
- Remove local `mibs/` files before SOW close-out.

Implementation plan:

1. Schema/API scaffold:
   - Add typed BGP profile config structs, closed enums, validation helpers, identity helpers, and `ProfileMetrics.BGPRows`.
   - Add `ProfileConsumerBGP` projection and projection tests.
   - Update `.agents/sow/specs/snmp-profile-projection.md` with the BGP projection contract.
2. Typed producer scaffold:
   - Add BGP row collection/projection in ddsnmpcollector without changing vendor profiles yet.
   - Add full-profile/synthetic tests for the standard BGP shape.
3. Runtime consumer/function/charts:
   - Feed typed rows into BGP charts/function cache.
   - Preserve chart-per-row behavior.
   - Implement stale cache and chart freshness semantics.
   - Make BGP function registration profile-aware.
4. Vendor migrations:
   - Migrate one vendor/profile family at a time, with full-profile tests.
   - Delete that vendor's old `bgp_public*` routes/spec/cache handling in the same slice.
   - Audit Cisco profile attachment before Cisco migration.
   - Remove Cumulus/Alcatel claims unless real fixtures are available.
5. Final cleanup:
   - Delete remaining `bgp_public*` router, underscore-prefix BGP protocol, and `bgp.` string-prefix context heuristics.
   - Regenerate metadata/integration artifacts.
   - Remove local untracked MIB files before close-out.

Validation plan:

- Schema validation tests for row kind, identity, state mappings, AFI/SAFI enums, MIB accessibility, and invalid signal/source combinations.
- Projection tests for `Project(metrics)`, `Project(bgp)`, and mixed `Project(metrics, bgp)`.
- Full-profile smoke tests per migrated vendor through normal profile loading/resolution/finalization.
- Fixture-backed typed BGP row tests for each migrated vendor.
- Function tests for no data, fresh rows, filtered-empty rows, stale-within-TTL rows, and stale-expired rows.
- Chart tests for chart-per-row identity stability and stale/freshness signals.
- Static/manual review of `src/health/health.d/snmp_bgp.conf` contexts and dimensions against typed chart output.
- Same-failure searches for leftover `bgp_public`, underscore-prefixed BGP protocol, `strings.HasPrefix(name, "bgp.")`, Cumulus/Alcatel unsupported claims, workstation paths, and raw MIB files in git status.
- Narrow Go test suites:
  - `go test -count=1 ./collector/snmp/ddsnmp/...`
  - `go test -count=1 ./collector/snmp/...`

Artifact impact plan:

- AGENTS.md: no expected update; existing SOW, collector, git, and SNMP profile authoring rules already apply.
- Runtime project skills: update `.agents/skills/project-snmp-profiles-authoring/SKILL.md` if typed BGP authoring adds durable MIB/index guidance beyond the existing rule.
- Specs: update `.agents/sow/specs/snmp-profile-projection.md` with BGP consumer/projection/delivery/validation rules.
- End-user/operator docs: update `collector/snmp/metadata.yaml`, generated integration artifacts, and any BGP function documentation affected by output/config changes.
- End-user/operator skills: no expected update unless public Netdata AI skills reference SNMP BGP schema or function behavior.
- SOW lifecycle: move to `current/in-progress` before implementation; close only after validation, artifact gates, local MIB cleanup, and follow-up mapping are complete.

Open-source reference evidence:

- No local mirrored open-source repositories were checked for this gate. The actionable evidence is in current branch code, project specs/skills, local untracked vendor MIB files, and review outputs.

Open decisions:

- None. User decisions 1-23 are resolved in `## Implications And Decisions`.

## Implications And Decisions

1. **BGP architecture: A.**
   - Use typed `bgp:` projection and typed `ProfileMetrics.BGPRows`.
2. **BGP function and integration exposure: A.**
   - Expose BGP only for profiles/jobs that declare typed BGP rows; restructure `funcRouter` lifecycle as needed.
3. **Chart cardinality: A.**
   - Keep existing ddsnmp chart-per-row behavior; no BGP-specific chart caps.
4. **Stale cache semantics: A.**
   - Implement stale-aware function behavior and chart-side freshness.
5. **Per-peer alerts: A.**
   - Keep per-peer/per-family BGP health templates default-on.
6. **Cisco BGP attachment: A.**
   - Move BGP out of `_cisco-base.yaml`; attach only to audited router/L3-switch profiles.
7. **Unproven Cumulus/Alcatel coverage: C.**
   - Remove claims until real fixtures exist.
8. **Structural identity: A.**
   - Use typed structural identity with canonical length-prefixed key.
9. **AFI/SAFI policy: A.**
   - Use closed canonical enums with explicit vendor-private allow-list.
10. **MIB accessibility: A.**
    - Enforce `MAX-ACCESS` accessibility; `not-accessible` fields come from indexes.
11. **Function lifecycle: A.**
    - Make function registration profile-aware.
12. **Cap overflow: not applicable.**
    - Decision 3A removes BGP-specific caps.
13. **Production-shaped tests: A.**
    - Require full-profile tests per migrated vendor.
14. **Migration/artifact gate: A.**
    - Use schema-first typed migration and delete old router rows/specs per vendor.
    - Implementation constraint: standard BGP4 metric-name routes are shared by still-unmigrated vendor profiles. The standard profile migration removes its old `metrics:`/`virtual_metrics:` definitions to prevent duplicate output, but shared `bgp_public*` cases remain until the last dependent vendor is migrated.
15. **State mapping completeness: A.**
    - Require all six RFC 4271 states unless explicitly partial.
16. **Underscore protocol: A.**
    - Delete underscore-prefix BGP protocol with `bgp_public*`.
17. **Function cache representation: A.**
    - Make `BGPRow` the canonical snapshot/cache representation.
18. **Health alert testing: B.**
    - Do not build an alert test framework; validate by static/manual review.
19. **Projection spec: A.**
    - Update `.agents/sow/specs/snmp-profile-projection.md` with BGP.
20. **BGP signal payload / row field model: B.**
    - Runtime planning exposed that `Signals map[BGPSignalKind]BGPValue` is not a clean end-state row model.
    - Claude review accepted Option B and flagged a current scaffold bug: categorical signals such as `last_down_reason`, `graceful_restart_state`, and `unavailability_reason` are routed through numeric conversion.
    - Replace generic `Signals` with typed row/config fields/groups before runtime consumer/function/chart work.
21. **Typed BGP cross-table value sources: A.**
    - Cisco peer-family rows in `cbgpPeer2AddrFamilyPrefixTable` need `remote_as`, local address, local AS, identifiers, and BGP version from `cbgpPeer2Table`.
    - Current raw Cisco YAML gets these with cross-table `metric_tags`; typed `BGPValueConfig` has no equivalent first-class source.
    - Add first-class cross-table support to `BGPValueConfig`, mirroring `MetricTagConfig.table` + `index_transform`, so typed identity/descriptors can stay schema-owned.
22. **Variable-length BGP index tail extraction: A.**
    - Cisco peer-family AFI/SAFI fields follow variable-length `InetAddress` in the row index, so fixed absolute positions work only for IPv4 rows.
    - Add reusable tail-based index extraction for typed BGP values and use it for Cisco AFI/SAFI.
23. **Catalyst 9k BGP attachment scope: A.**
    - `cisco-catalyst.yaml` is currently BGP-free but covers Catalyst families that can run BGP.
    - Attach typed Cisco BGP broadly to `cisco-catalyst.yaml`.
    - Accepted trade-off: unsupported BGP table walk cost on non-BGP Catalyst devices is temporary and expected to be mitigated by a generalized unsupported table-root cache follow-up.
    - Follow-up scope: `.agents/sow/pending/SOW-0014-20260507-snmp-licensing-unsupported-table-cache.md` has been expanded from licensing-only to typed licensing + typed BGP unsupported table-root caching.

## Plan

1. Move this SOW to `current/in-progress` before code changes.
2. Land schema/API scaffold and tests:
   - BGP config structs and enums.
   - Validation helpers.
   - Projection plumbing.
   - `ProfileMetrics.BGPRows`.
   - Spec update.
3. Land typed producer scaffold with a minimal standard BGP fixture path.
4. Land typed runtime consumer/function/chart path:
   - First replace generic `BGPRow.Signals` with strongly typed BGP row fields/groups if Decision 20 is resolved as 20B.
   - Preserve chart-per-row behavior.
   - Add stale/freshness semantics.
   - Make BGP function profile-gated.
5. Audit Cisco profile attachment and MIB accessibility before vendor migrations.
6. Migrate standard BGP4-MIB.
7. Migrate Cisco and remove BGP from `_cisco-base.yaml`.
8. Migrate Juniper.
9. Migrate Nokia.
10. Migrate Huawei.
11. Migrate Arista and Dell.
12. Remove Cumulus/Alcatel public claims unless real fixtures are added.
13. Delete remaining old BGP router/protocol code and string-prefix context gates.
14. Regenerate metadata/integration artifacts and validate generated outputs.
15. Run same-failure and sensitive-data scans.
16. Remove local untracked `mibs/` files before close-out.
17. Complete validation gate, move SOW to `done/`, and commit lifecycle change with implementation.
18. Normalize remaining inline typed-domain profile sections into dedicated fragments before close-out.

## Execution Log

### 2026-05-08

- Created pending SOW from review TODO after user resolved decisions 1-19.
- Moved SOW to current/in-progress before code changes.
- Landed schema/API/projection scaffold:
  - `ddprofiledefinition.BGPConfig`, BGP row kinds, peer-state enums, AFI/SAFI enums, and typed BGP row field groups.
  - `ProfileDefinition.BGP`, `ProfileMetrics.BGPRows`, and typed BGP row output structs.
  - `ProfileConsumerBGP` projection support, mixed `Project(metrics, bgp)` behavior, inheritance merge, and cross-profile dedup scaffolding.
  - BGP schema validation tests and projection tests.
  - Initial BGP projection contract in `.agents/sow/specs/snmp-profile-projection.md`.
- Landed typed producer scaffold:
  - `ddsnmpcollector.collectBGPRows` supports scalar and table typed BGP rows.
  - BGP rows are collected best-effort so BGP processing errors do not drop regular SNMP metrics.
  - BGP scalar row collection honors the shared missing-OID cache.
  - BGP table row collection uses the existing table cache and table dependency walk path.
  - Main SNMP profile projection includes `ConsumerBGP` with metrics and licensing.
  - Internal profile stats now include BGP timing, row count, and processing error dimensions.
  - Added scalar, table, best-effort, and missing-OID cache tests for typed BGP rows.
- Reworked the BGP scaffold per Decision 20B:
  - Removed the generic `Signals map[BGPSignalKind]BGPValue` row model.
  - Added typed BGP row/config groups for admin, state, previous state, connection, traffic, transitions, timers, last-error, last-notifications, reasons, graceful restart, routes, route limits, and device counts.
  - Added categorical field coverage so text/mapped fields such as last-down reason are not routed through numeric conversion.
  - Updated BGP validation and projection spec wording to describe typed fields rather than signal kinds.
- Started typed runtime consumer wiring:
  - Added typed BGP row to public chart-metric projection for existing BGP chart contexts.
  - Updated the BGP function cache to accept typed `BGPRow` snapshots directly, preserving descriptor fields without underscore-prefixed tag routing.
  - Added tests for typed BGP chart metrics and typed-row function-cache output.
- Recorded the Decision 14 implementation constraint:
  - `_std-bgp4-mib.yaml` is extended by multiple still-unmigrated vendor profiles, so shared `bgp_public*` standard BGP4 route/spec/cache cases cannot be deleted until the last dependent vendor profile is migrated.
  - The old standard profile `metrics:`/`virtual_metrics:` definitions still need to be removed during the standard migration to avoid double-emission.
- Migrated the standard BGP4-MIB profile to typed BGP rows:
  - `_std-bgp4-mib.yaml` now declares `bgpPeerTable` as typed `bgp:` row `bgp4-peer`.
  - Removed old standard `metrics:` and `virtual_metrics:` from `_std-bgp4-mib.yaml`.
  - Removed Nokia fallback alternatives that pointed at the old standard BGP4 virtual metrics.
  - Kept shared `bgp_public*` standard BGP4 route/spec/cache cases temporarily because unmigrated vendor profiles still depend on them.
  - Set BGP `OriginProfileID` during profile load, matching licensing origin behavior.
  - Updated standard BGP4 and Cumulus/FRR fixture coverage to assert typed `BGPRows`, while keeping the legacy alert-surface test scoped to still-unmigrated vendor raw paths.
- Landed typed BGP runtime lifecycle and stale-cache handling:
  - `Collector` no longer allocates/registers BGP integration by default.
  - BGP integration is enabled only for resolved profiles that contain typed `bgp:` rows or temporary legacy BGP public-router inputs.
  - BGP collection failures mark the function cache stale instead of leaving stale rows indistinguishable from fresh rows.
  - `snmp:bgp-peers` returns stale rows during the bounded stale window, returns 503 after the stale window expires, and returns a 200 empty table for valid filtered-empty views.
  - Function row building now snapshots the cache and releases the cache lock before sorting/building response rows.
  - Framework boundary recorded: the static module method list still advertises `snmp:bgp-peers` globally. Fully hiding the method from global discovery for non-BGP jobs requires a broader `funcctl`/collectorapi capability change; the SNMP handler itself is profile-gated.
- Landed typed cross-table BGP value sources and the Cisco typed migration slice:
  - `BGPValueConfig` now supports first-class `table:` value sources with `index_transform`, mirroring the existing cross-table tag pattern without making typed identity depend on labels.
  - `_cisco-bgp4-mib.yaml` now declares typed peer and peer-family BGP rows instead of raw `metrics:`/`virtual_metrics:`.
  - Cisco BGP was removed from `_cisco-base.yaml`.
  - Cisco BGP is attached only to audited router/L3/data-center profiles: `cisco-asr.yaml`, `cisco-csr1000v.yaml`, `cisco-isr.yaml`, `cisco-isr-4431.yaml`, `cisco-3850.yaml`, `cisco-nexus.yaml`, and the new narrow `cisco-ncs.yaml` profile for the NCS 540 fixture.
  - Generic `cisco.yaml` remains BGP-free by design.
  - Added `BGPValueConfig.index_from_end` and corrected Cisco peer-family AFI/SAFI extraction to read the final two `cbgpPeer2AddrFamilyPrefixTable` row-index components, which works for both IPv4 and IPv6 `InetAddress` indexes.
- Resolved the Claude slice-review commit blockers after Decision 22A:
  - Protected `funcRouter` runtime handler registration with a router `sync.RWMutex`.
  - Surfaced per-profile typed BGP collection failures through `ProfileMetrics.BGPCollectError`.
  - BGP integration now marks the function cache stale and skips peer-cache reset/finalize when a profile-level BGP failure occurs inside an otherwise successful SNMP collection cycle.
  - Added focused regression tests for concurrent router registration, per-profile BGP failure stale-cache preservation, Cisco IPv6 peer-family AFI/SAFI extraction, and generic Cisco profile BGP non-inheritance.
- Reconciled the Claude post-Decision 22A review:
  - Accepted RT2-1: the first stale-cache fix was all-or-nothing across profiles. Updated BGP integration/cache handling so failed sources preserve stale rows while successful sources refresh normally in the same cycle.
  - Added mixed-profile failure regression coverage, including the case where an expired failed source must not be kept alive by another source refreshing successfully.
  - Accepted MIB2-2 and normalized empty typed BGP routing instances to `default` for chart/function tags.
  - Recorded Catalyst BGP attachment as Decision 23 instead of silently attaching or silently dropping it.
- Applied Decision 23A:
  - `cisco-catalyst.yaml` now extends `_cisco-bgp4-mib.yaml`.
  - Added a Catalyst 9300 profile-merge regression test proving typed Cisco BGP rows are inherited.
  - Expanded SOW-0014 from licensing-only to typed licensing + typed BGP unsupported table-root caching, because broad Catalyst BGP has the same unsupported-table probe trade-off as broad Cisco licensing.
- Reconciled the Claude final pre-commit review:
  - Included `config/go.d/snmp.profiles/default/cisco-ncs.yaml` in the slice because this SOW claims the new NCS profile.
  - Added repo-root `/mibs/` to `.gitignore` so local raw MIB audit files remain uncommitted.
  - Accepted remaining schema/spec/runtime hygiene findings as SOW-close items, not commit blockers for this slice.
- Migrated Juniper BGP to typed rows:
  - `_juniper-bgp4-v2.yaml` now declares typed peer and peer-family BGP rows instead of raw `metrics:`/`virtual_metrics:`.
  - Juniper base/router profiles no longer extend `_juniper-bgp-virtual.yaml`; the old virtual-metric path is no longer referenced.
  - `BGPValueConfig.lookup_symbol` now supports typed BGP cross-table value lookups, needed because Juniper prefix counters are keyed by peer ID while peer identity lives in `jnxBgpM2PeerTable`.
  - Empty successful BGP dependency walks are cached as empty walked tables, so optional augmented tables do not drop otherwise valid peer rows.
  - Runtime BGP rows are skipped when required identity fields are missing at collection time.
  - Added BGP projection coverage to `collector/snmp/ddsnmp/profile_test.go`.
  - Reworked Juniper fixture tests to assert typed `BGPRows` from BGP-projected profiles and refactored similar test cases to `map[string]struct{}` tables.
- Migrated Nokia/TiMOS BGP to typed rows:
  - `_nokia-timetra-bgp.yaml` now declares typed peer and six typed peer-family BGP rows instead of raw `metrics:`/`virtual_metrics:`.
  - Nokia no longer inherits `_std-bgp4-mib.yaml`; SR OS BGP rows come from TIMETRA-BGP-MIB only.
  - Peer address is derived from the `tBgpPeerNgTable`/`tBgpPeerNgOperTable` row index because the MIB index objects are `MAX-ACCESS not-accessible`.
  - Deleted unused `_juniper-bgp-virtual.yaml` after the Juniper migration removed all references.
  - Updated BGP structural identity to include typed config ID so multiple logical typed rows over the same table/index do not collide.
  - Updated Nokia profile, LibreNMS identity, and TiMOS fixture coverage to assert typed `BGPRows`.
  - Added Nokia BGP projection coverage to `collector/snmp/ddsnmp/profile_test.go`.
  - Reviewed new BGP tests for the branch's table-driven style rule and refactored same-shape Cisco prefix, Arista/Dell fixture, and TiMOS fixture tests to `map[string]struct{}` or equivalent map-keyed tables.
- Reconciled the Claude Nokia/TiMOS commit review:
  - Accepted the review verdict as ready with follow-ups.
  - Added Nokia profile assertions for admin and six-state peer-state mappings.
  - Added synthetic Nokia typed-collection coverage for admin-disabled, non-established states, and non-default VRF routing-instance extraction.
  - Added broader Nokia profile assertions for descriptor, timer, traffic, notification, transition, and last-error fields.
  - Updated `.agents/sow/specs/snmp-profile-projection.md` so BGP table structural identity includes typed config ID.
  - Recorded the inherited TIMETRA high-numbered oper OID verification gap as a SOW follow-up.
- Migrated Huawei BGP to typed rows:
  - `huawei-routers.yaml` now declares typed BGP device, peer, and peer-family rows instead of raw BGP `metrics:`/`virtual_metrics:`.
  - Removed `_huawei-bgp-statistics.yaml`; Huawei BGP session counts now flow through typed `device_counts`.
  - Extended typed BGP device counts with `ibgp_peers` and `ebgp_peers` so Huawei preserves the public `configured`/`ibgp`/`ebgp` peer-count dimensions without the legacy public router.
  - Removed Huawei-specific cases from the temporary `bgp_public*` metric-name router.
  - Added Huawei BGP projection coverage to `collector/snmp/ddsnmp/profile_test.go`.
  - Reworked Huawei LibreNMS fixture coverage to assert typed `BGPRows` directly, including IPv4/IPv6 peer-family rows, route totals, peer-level message/update counters, and device peer counts.
  - Added synthetic Huawei typed-collection coverage for state/admin mapping and peer-statistics remote-AS lookup by peer address.
  - Local raw MIB audit gap: `mibs/` contains `HUAWEI-MPLS-BGP-VPN-MIB.mib`, not the exact `HUAWEI-BGP-VPN-MIB`; profile OIDs are carried forward from the existing Huawei profile and the linked public Huawei BGP MIB references.

### 2026-05-09

- Migrated the remaining BGP-bearing vendor profiles and deleted the legacy BGP public router:
  - Arista, Dell OS10, and Alcatel now declare typed `bgp:` rows.
  - All stock BGP coverage is typed: standard `BGP4-MIB`, Cisco, Juniper, Nokia SR OS, Huawei, Arista, Dell OS10, and Alcatel.
  - `bgp_public.go`, `bgp_public_routes.go`, and `bgp_public_test.go` were deleted.
  - The retained chart-filter and chart-spec behavior now lives in `bgp_chart_filter.go` and `bgp_metric_specs.go`.
  - The BGP function cache consumes typed `BGPRow` updates only.
- Reconciled final migration review blockers:
  - Corrected inherited Alcatel IPv6 BGP table OIDs from `.12` to `.14` according to `ALCATEL-IND1-BGP-MIB`.
  - Restored end-to-end collection-level assertions for BGP metric IDs and chart context.
  - Removed dead BGP alert-surface test helpers and moved the remaining shared profile helper.
  - Reviewed and refactored applicable BGP tests to map-keyed table-driven cases.
- Resolved AC#69:
  - Updated `collector/snmp/metadata.yaml` to remove Cumulus and Alcatel from public BGP support claims until real fixture evidence exists.
  - Regenerated integration artifacts with `integrations/gen_integrations.py`, `integrations/gen_docs_integrations.py -c go.d.plugin/snmp`, `integrations/gen_doc_collector_page.py`, and `integrations/gen_doc_secrets_page.py`.
  - Committed generated output changed in `collector/snmp/integrations/snmp_devices.md`.
  - Gitignored generated `integrations/integrations.js` and `integrations/integrations.json` were regenerated locally.
  - `src/collectors/COLLECTORS.md` and `src/collectors/SECRETS.md` had no tracked diff after generation.
- Updated durable authoring and behavior references:
  - `collector/snmp/profile-format.md` now documents typed BGP `partial_states`, routing-instance default labeling, and `index_from_end` selector behavior.
  - `.agents/sow/specs/snmp-profile-projection.md` now records typed BGP structural identity, default routing-instance normalization, per-source stale-cache semantics, and row-index selector validation behavior.
  - `.agents/skills/project-snmp-profiles-authoring/SKILL.md` now records the BGP authoring checks for typed `bgp:` rows, RFC 4271 state mappings, and index-derived BGP fields.
- Created `.agents/sow/pending/SOW-0016-20260509-snmp-bgp-hardening-followups.md` for non-blocking validation/runtime/fixture hardening that should not be hidden as vague follow-up debt.
- Applied the user's typed-domain profile organization decision:
  - Extracted inline Arista, Dell OS10, and Huawei `bgp:` sections into `_arista-bgp4-v2.yaml`, `_dell-os10-bgp4-v2.yaml`, and `_huawei-bgp-vpn.yaml`.
  - Extracted inline Blue Coat, Check Point, Fortinet FortiGate, MikroTik RouterOS, and Sophos XGS `licensing:` sections into `_bluecoat-proxysg-licensing.yaml`, `_checkpoint-licensing.yaml`, `_fortinet-fortigate-licensing.yaml`, `_mikrotik-routeros-licensing.yaml`, and `_sophos-xgs-firewall-licensing.yaml`.
  - Concrete device profiles now extend the typed fragments and no longer mix inline typed-domain sections with regular metric sections.
  - Updated Arista/Dell typed BGP fixture assertions to expect the new fragment origin profile IDs.
- Folded the pending SOW-0016 hardening scope back into SOW-0015 by user decision:
  - Deleted the separate pending SOW-0016 file after migrating its remaining work into this SOW's follow-up mapping.
  - Reclassified RT-4 as an intentional chart-identity invariant: public BGP chart keys follow the same logical SNMP table identity model as regular table metrics (`name` plus visible tag values), while the BGP function cache keeps structural row identity separately.
  - Added a focused runtime test proving same logical BGP peers from different typed rows resolve to the same public chart identity and do not include source or structural IDs in the chart key.
  - Kept remaining hardening work in this SOW instead of tracking it through a separate pending SOW.
- Completed the folded SOW-0016 hardening items selected by the user:
  - Added typed BGP validation for mutually-exclusive row-index selectors (`index`, `index_from_end`, `index_transform`).
  - Added conservative typed BGP cross-table validation for declared referenced BGP tables, including source and lookup-symbol OID prefix checks.
  - Added stale BGP function-cache entry reaping after the stale window expires.
  - Added Dell OS10 IPv6 typed BGP synthetic-PDU coverage.
  - Added `index_from_end` plus `format` propagation coverage.
  - Extended the logical chart-identity test to verify descriptor labels remain visible labels while public chart keys omit source and structural identity.
- Reconciled final external review findings:
  - Final review verdict was `READY TO MERGE`; no P0/P1 blockers were reported.
  - Added remaining row-index selector combination negative tests.
  - Added BGP row kind vs field-group compatibility negative tests.
  - Extended Dell OS10 IPv6 coverage with a dependency-table traffic counter assertion.
  - Replaced local-only Nokia/TiMOS MIB closure evidence with reproducible source references.

## Validation

Acceptance criteria evidence:

- Met for the implemented migration:
  - Typed `bgp:` profile section exists in `ddprofiledefinition.ProfileDefinition`.
  - Typed BGP output exists in `ddsnmp.ProfileMetrics.BGPRows`.
  - `Project(bgp)` and `Project(metrics, bgp)` projection paths exist and are tested.
  - Closed row kind, peer state, AFI/SAFI, and typed-field validation exists.
  - `ddsnmpcollector` emits typed BGP rows for scalar and table `bgp:` configs.
  - BGP collection is wired into the regular SNMP profile collection path and keeps regular metrics on BGP row errors.
  - BGP profile stats are exposed alongside scalar/table/licensing/internal stats.
  - Typed BGP rows project into BGP chart metrics and the BGP peers function cache.
  - All BGP-bearing stock profile coverage is typed: standard `BGP4-MIB`, Cisco, Juniper, Nokia SR OS, Huawei, Arista, Dell OS10, and Alcatel.
  - `bgp_public.go`, `bgp_public_routes.go`, and `bgp_public_test.go` are deleted.
  - Production runtime scans found no remaining `bgp_public`, `BGPSignalKind`, `bgpScopeAuto`, `mergeBGPPeerEntryTags`, `bgpPeerMetricLeaf`, `normalizeCollectorMetrics`, or `routeBGPPublicMetric` references outside tests/docs.
  - Inline typed-domain profile section scan now finds `bgp:` and `licensing:` only in underscore-prefixed typed fragments.
  - Legacy BGP identity/router underscore protocol (`_routing_instance`, `_neighbor`, `_remote_as`, `_address_family`, `_subsequent_address_family`) has no production code or profile YAML use. Remaining hits are the generic virtual-metric example in `collector/snmp/profile-format.md`.
  - Typed BGP descriptor labels still use the generic SNMP convention of underscore-prefixed chart labels (`_local_address`, `_local_as`, `_peer_identifier`, and related descriptors) in `bgp_typed_metrics.go`; this is not the deleted legacy BGP public-router protocol. `TestTypedBGPMetricsUseLogicalChartIdentity` verifies descriptor labels are visible chart labels and are not part of public chart-key identity.
  - Public typed BGP chart keys intentionally use logical SNMP table metric identity, not typed structural row identity. `TestTypedBGPMetricsUseLogicalChartIdentity` covers the invariant that same logical peer tags from different typed rows resolve to the same public chart key.
  - Typed BGP profile validation rejects values that combine multiple row-index selectors.
  - Typed BGP profile validation now verifies `table:` references to declared BGP row tables when the table root is known at profile-load time. It validates source OIDs and `lookup_symbol` OIDs against the referenced table OID prefix. Runtime-inferred cross-table roots remain supported for existing fragments whose `table:` source is not a declared BGP row table.
  - Cisco BGP was removed from `_cisco-base.yaml` and attached to audited router/L3/data-center Cisco profiles, including Catalyst by Decision 23A.
  - Cumulus and Alcatel were removed from public BGP capability claims in `metadata.yaml` and regenerated `snmp_devices.md`.
  - `.agents/sow/specs/snmp-profile-projection.md` documents the BGP projection contract, including the typed config ID in structural identity.
  - Metadata and generated integration docs are coherent for the BGP support list and BGP alert/metric/function surface.
- No open items remain before moving this SOW to `done/`.

Tests or equivalent validation:

- `go test -count=1 ./collector/snmp/ddsnmp/...` passed on 2026-05-08.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08.
- `go test -count=1 -run 'TestCollector_Collect_BGPRows|TestCollector_Collect_StatsSnapshot' ./collector/snmp/ddsnmp/ddsnmpcollector` passed on 2026-05-08.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed on 2026-05-08 after the standard BGP4-MIB typed-profile migration.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after the standard BGP4-MIB typed-profile migration.
- `go test -count=1 -run 'TestFuncBGPPeers|TestBGPPeerCache|TestProfilesHaveBGP|TestCollectSNMP_HidesBGPDiagnosticsButKeepsFunctionCache|TestCollector_BGPFunctionHandlerIsRegisteredOnlyWhenEnabled|TestTypedBGPMetricsFromProfileMetrics' ./collector/snmp` passed on 2026-05-08 after typed BGP runtime lifecycle changes.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after typed BGP runtime lifecycle changes.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed on 2026-05-08 after Decision 21A and the Cisco typed-profile migration.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after Decision 21A and the Cisco typed-profile migration.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition` passed on 2026-05-08 after Decision 22A and the Claude blocker fixes.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector` passed on 2026-05-08 after Decision 22A and the Claude blocker fixes.
- `go test -count=1 ./collector/snmp` passed on 2026-05-08 after Decision 22A and the Claude blocker fixes.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'Test_Cisco'` passed on 2026-05-08 after Decision 22A and the Claude blocker fixes.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed on 2026-05-08 after Decision 22A and the Claude blocker fixes.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after Decision 22A and the Claude blocker fixes.
- `go test -race -run TestFuncRouter_ConcurrentRegisterAndHandle ./collector/snmp` passed on 2026-05-08.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed on 2026-05-08 after the RT2-1 mixed-profile failure fix.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after the RT2-1 mixed-profile failure fix.
- `go test -race -run 'TestFuncRouter_ConcurrentRegisterAndHandle|TestBGPIntegration_MixedProfileBGPErrorRefreshesSuccessfulProfiles|TestBGPIntegration_ExpiredFailedProfileDoesNotKeepStaleRowsWithFreshProfiles' ./collector/snmp` passed on 2026-05-08 after the RT2-1 mixed-profile failure fix.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'Test_CiscoBGPProfileMergedIntoCiscoCatalyst|Test_CiscoGenericProfilesDoNotInheritBGP|Test_CiscoBGPProfileMergedIntoCiscoASR|Test_CiscoBgpPrefixProfileMergedIntoCiscoASR'` passed on 2026-05-08 after Decision 23A.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed on 2026-05-08 after Decision 23A.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after Decision 23A.
- `go test -count=1 ./collector/snmp/ddsnmp ./collector/snmp/ddsnmp/ddprofiledefinition ./collector/snmp/ddsnmp/ddsnmpcollector` passed on 2026-05-08 after the Juniper typed-profile migration and BGP projection test refactor.
- `go test -count=1 ./collector/snmp` passed on 2026-05-08 after the Juniper typed-profile migration and BGP projection test refactor.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after the Juniper typed-profile migration and BGP projection test refactor.
- `go test -race -count=1 ./collector/snmp` passed on 2026-05-08 after the Juniper typed-profile migration and BGP projection test refactor.
- `go test -count=1 ./collector/snmp/ddsnmp ./collector/snmp/ddsnmp/ddprofiledefinition ./collector/snmp/ddsnmp/ddsnmpcollector` passed on 2026-05-08 after the Nokia/TiMOS typed-profile migration and test refactor.
- `go test -count=1 ./collector/snmp` passed on 2026-05-08 after the Nokia/TiMOS typed-profile migration and test refactor.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after the Nokia/TiMOS typed-profile migration and test refactor.
- `go test -race -count=1 ./collector/snmp` passed on 2026-05-08 after the Nokia/TiMOS typed-profile migration and test refactor.
- `go test -count=1 ./collector/snmp/ddsnmp ./collector/snmp/ddsnmp/ddprofiledefinition ./collector/snmp/ddsnmp/ddsnmpcollector` passed on 2026-05-08 after the Claude Nokia/TiMOS review follow-ups.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after the Claude Nokia/TiMOS review follow-ups.
- `go test -race -count=1 ./collector/snmp` passed on 2026-05-08 after the Claude Nokia/TiMOS review follow-ups.
- `go test -count=1 ./collector/snmp/ddsnmp ./collector/snmp/ddsnmp/ddprofiledefinition ./collector/snmp/ddsnmp/ddsnmpcollector` passed on 2026-05-08 after the Huawei typed-profile migration.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-08 after the Huawei typed-profile migration.
- `go test -race -count=1 ./collector/snmp` passed on 2026-05-08 after the Huawei typed-profile migration.
- `go test -count=1 ./collector/snmp -run 'Test(BGPLastErrorText|BGPPeerEntryKey|BGPPeerCache|FuncBGPPeers|CollectSNMP_HidesBGP)'` passed on 2026-05-09 after the BGP table-driven test refactor.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'Test_(LibreNMSBGPIdentityFixtures|CiscoBGPProfiles|CiscoGeneric|AlcatelBGPProfile|HuaweiBGP|JuniperBGP|NokiaBGP)'` passed on 2026-05-09 after the BGP table-driven test refactor.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_(GenericBGP4Rows|HuaweiBGP|AlcatelBGP|AristaAndDellBGP|CiscoBgpPeer2|CiscoBgpPeer3|JuniperBGP|TiMOSBGP)'` passed on 2026-05-09 after the BGP table-driven test refactor.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-09 after the BGP table-driven test refactor.
- `git diff --check` passed on 2026-05-09 after the BGP table-driven test refactor.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-09 after metadata/docs/spec/skill close-out updates and local integration docs regeneration.
- `git diff --check` passed on 2026-05-09 after metadata/docs/spec/skill close-out updates and local integration docs regeneration.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'Test_(AristaAndDellBGPProfilesUseTypedRows|HuaweiBGPProfileUsesTypedRows|ProfilesProjectConsumers|NokiaBGPProfile|CiscoBGPProfiles|JuniperBGPProfiles)|TestProfile_Merge'` passed on 2026-05-09 after typed-domain profile fragment extraction.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_(LicensingProfiles|LicensingProfileFixtures|SophosLicensingProfile|FortiGate|BlueCoat|HuaweiBGP|AlcatelBGP|AristaAndDellBGP|TiMOSBGP|GenericBGP4Rows)'` passed on 2026-05-09 after typed-domain profile fragment extraction.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-09 after typed-domain profile fragment extraction.
- `git diff --check` passed on 2026-05-09 after typed-domain profile fragment extraction.
- `go test -count=1 ./collector/snmp -run 'Test(BGPPeerCache_UpdateRow|BGPIntegration_(PreservesFunctionCacheOnProfileBGPError|RecoveryClearsStaleFunctionRows|MixedProfileBGPErrorRefreshesSuccessfulProfiles|ExpiredFailedProfileDoesNotKeepStaleRowsWithFreshProfiles))'` passed on 2026-05-09 after pre-merge BGP cache hardening tests.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition -run 'TestValidateEnrichProfile_BGP'` passed on 2026-05-09 after adding the `partial: true` empty-mapping rejection test.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'Test_(AristaAndDellBGPProfilesUseTypedRows|JuniperBGPProfilesUseOnlyJuniperPeerTables|AlcatelBGPProfileUsesTypedRows|HuaweiBGPProfileUsesTypedRows|NokiaBGPProfileMergedIntoNokiaSROS)'` passed on 2026-05-09 after adding vendor six-state mapping assertions.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-09 after pre-merge hardening tests.
- `go test -race -count=1 ./collector/snmp` passed on 2026-05-09 after pre-merge hardening tests.
- `go test -count=1 ./collector/snmp -run 'TestTypedBGPMetricsUseLogicalChartIdentity|TestTypedBGPMetricsFromProfileMetrics|TestBGPPeerCache_UpdateRow'` passed on 2026-05-09 after folding SOW-0016 back into SOW-0015 and adding the logical chart-identity invariant test.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition -run 'TestValidateEnrichProfile_BGP'` passed on 2026-05-09 after typed BGP row-index selector and declared cross-table validation hardening.
- `go test -count=1 ./collector/snmp -run 'Test(BGPPeerCache_UpdateRow|BGPIntegration_ExpiredFailedProfileDoesNotKeepStaleRowsWithFreshProfiles|TypedBGPMetricsUseLogicalChartIdentity)'` passed on 2026-05-09 after stale-entry reaping and descriptor-label chart-key coverage.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_(AristaAndDellBGP_FromLibreNMSFixtures|DellOS10BGP_IPv6Rows|BGPRowsWithCrossTableBGPValues)'` passed on 2026-05-09 after Dell IPv6 and `index_from_end` plus `format` coverage.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-09 after completing the folded SOW-0016 hardening items selected by the user.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition -run 'TestValidateEnrichProfile_BGP'` passed on 2026-05-09 after final-review P2 validation-test polish.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_DellOS10BGP_IPv6Rows'` passed on 2026-05-09 after final-review P2 Dell IPv6 dependency-table coverage.
- `go test -count=1 ./collector/snmp/...` passed on 2026-05-09 after final-review P2 polish.
- `git diff --check` passed on 2026-05-09 after final-review P2 polish.

Real-use evidence:

- No live production device walk was run in this SOW.
- Production-shaped profile fixture and synthetic-PDU evidence covers the migrated typed paths:
  - Standard `BGP4-MIB` and Cumulus/FRR-style standard fixture paths now assert typed `BGPRows`.
  - Cisco typed peer and peer-family rows cover IPv4 and IPv6 `InetAddress` row-index AFI/SAFI extraction.
  - Juniper, Nokia/TiMOS, Huawei, Arista, Dell OS10, and Alcatel tests assert typed rows through normal profile loading/resolution/finalization or collector production paths.
  - `snmp:bgp-peers` cache/function tests cover typed row updates, stale rows, mixed per-source failures, expiry, and recovery.
- Integration docs were regenerated through the repository generator path, not hand-edited in the generated file.

Reviewer findings:

- External Claude review passes were run by the user during the SOW and reconciled before commits.
- Final external review on 2026-05-09 returned `READY TO MERGE` with no P0/P1 blockers.
- Commit blockers fixed during the SOW include:
  - `funcRouter` handler map runtime race.
  - Per-profile BGP failure wiping stale peer cache.
  - Cisco IPv6 peer-family AFI/SAFI positional extraction.
  - Mixed-profile BGP failure freezing successful profiles.
  - Catalyst BGP attachment decision and test.
  - Untracked `cisco-ncs.yaml` slice drift.
  - Alcatel IPv6 BGP table OID `.12` vs authoritative `.14`.
  - Lost end-to-end BGP metric ID/chart-context coverage.
  - Dead BGP alert-surface test helper file.
- Non-blocking findings were either implemented during close-out or explicitly tracked:
  - SOW-0014 owns typed licensing + typed BGP unsupported table-root caching.
  - This SOW completed typed BGP validation/runtime/fixture hardening after the user's decision to fold SOW-0016 back into SOW-0015.
  - Final-review P2 test gaps SCH5-1, SCH5-3, and PROF5-2 were implemented in SOW-0015.

Same-failure scan:

- Legacy router scan:
  - Command: `rg -n "bgp_public|BGPSignalKind|bgpScopeAuto|mergeBGPPeerEntryTags|bgpPeerMetricLeaf|normalizeCollectorMetrics|routeBGPPublicMetric" src/go/plugin/go.d/collector/snmp src/go/plugin/go.d/config/go.d/snmp.profiles/default --glob '!**/*_test.go'`
  - Result: no hits.
- Legacy BGP underscore identity protocol scan:
  - Command: `rg -n '"_(routing_instance|neighbor|remote_as|address_family|subsequent_address_family)"|\b_(routing_instance|neighbor|remote_as|address_family|subsequent_address_family)\b' src/go/plugin/go.d/collector/snmp src/go/plugin/go.d/config/go.d/snmp.profiles/default --glob '!**/*_test.go'`
  - Result: only the generic virtual-metric example in `collector/snmp/profile-format.md`; no production code or profile YAML uses the deleted BGP identity-router protocol.
- Cumulus/Alcatel BGP claim scan:
  - Command: `rg -n "Cumulus|Alcatel" src/go/plugin/go.d/collector/snmp/metadata.yaml src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md integrations/integrations.js src/collectors/COLLECTORS.md`
  - Result: remaining `Alcatel-Lucent` hits are in the generic Wireless vendor table, not the BGP capability/support list; regenerated `integrations.js` contains the updated BGP support sentence without Cumulus or Alcatel.
- Raw MIB tracking scan:
  - Command: `git ls-files mibs`
  - Result: no tracked MIB files.
  - Command: `git status --short --ignored=matching mibs`
  - Result: no output after cleanup.
  - Command: `test -d mibs && echo exists || echo missing`
  - Result: `missing`.
- Generated artifact status:
  - `integrations/integrations.js` and `integrations/integrations.json` were regenerated and remain gitignored.
  - `collector/snmp/integrations/snmp_devices.md` is the committed generated SNMP integration page diff for this SOW.
- Typed-domain profile organization scan:
  - Command: `rg -n "^bgp:|^licensing:" config/go.d/snmp.profiles/default -g "*.yaml"`
  - Result: all hits are in underscore-prefixed typed fragments; no concrete device profile has an inline `bgp:` or `licensing:` section.

Sensitive data gate:

- Sensitive string scan:
  - Command: `rg -n "BEGIN (RSA|OPENSSH|PRIVATE)|Authorization:|Bearer |password|passphrase|community:|SNMP_V3|SNMP_V2C|customer|token|secret|private key" .agents/sow/current/SOW-0015-20260508-snmp-bgp-typed-projection.md .agents/sow/specs/snmp-profile-projection.md .agents/skills/project-snmp-profiles-authoring/SKILL.md src/go/plugin/go.d/collector/snmp/metadata.yaml src/go/plugin/go.d/collector/snmp/integrations/snmp_devices.md src/go/plugin/go.d/collector/snmp/profile-format.md`
  - Result: hits are documented placeholder SNMP examples (`community: public`, `auth_protocol_passphrase`, `priv_protocol_passphrase`) and the SOW's own sensitive-data policy text. No real credentials or customer data found.
- Workstation path scan:
  - Command: ran `rg` for home-directory paths, private scratch paths, mirrored-repo workstation paths, and user/workstation markers across the SOW, spec, project skill, metadata, generated SNMP integration docs, and profile-format docs.
  - Result: no hits.
- Local raw MIB files under `mibs/` were removed before SOW close-out.

Artifact maintenance gate:

- AGENTS.md:
  - No project-wide workflow change needed. The existing Go test style rule already captures the user's table-driven test preference.
- Runtime project skills:
  - Updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md` with typed BGP profile authoring checks.
  - No update needed to `project-writing-collectors`; this SOW refined SNMP profile-specific BGP rules rather than general collector authoring policy.
  - `integrations-lifecycle` was used for generator workflow; no missing pipeline behavior was discovered that required a skill update.
- Specs:
  - Updated `.agents/sow/specs/snmp-profile-projection.md` with BGP typed projection, structural identity, routing-instance normalization, stale-cache semantics, `partial_states`, row-index selector mutual exclusivity, and declared cross-table reference validation.
- End-user/operator docs:
  - Updated `collector/snmp/metadata.yaml`.
  - Regenerated `collector/snmp/integrations/snmp_devices.md`.
  - Updated `collector/snmp/profile-format.md` for typed BGP authoring details.
  - `src/collectors/COLLECTORS.md` and `src/collectors/SECRETS.md` had no tracked diff after generation.
- End-user/operator skills:
  - No public Netdata AI skill or operator workflow skill was affected by the BGP SNMP profile schema/docs changes.
  - SOW audit flagged an email-address-pattern false positive in `.agents/skills/mirror-netdata-repos/SKILL.md` for an SSH clone URL example. The line was reworded to avoid durable false-positive sensitive-data output.
- SOW lifecycle:
  - SOW-0015 status is `completed` and the file is moved to `.agents/sow/done/`.
  - Initially created pending SOW-0016 for post-migration BGP hardening, then folded it back into SOW-0015 by user decision because the work is directly related and the branch is not under merge time pressure.
  - SOW-0014 remains the owner for typed licensing + typed BGP unsupported table-root caching.

Specs update:

- Updated `.agents/sow/specs/snmp-profile-projection.md` with BGP consumer, row shape, projection, inheritance, delivery, validation guarantees, structural identity, routing-instance normalization, stale-cache semantics, `partial_states`, and current row-index selector behavior.
- Updated `.agents/sow/specs/snmp-profile-projection.md` again during folded hardening to record row-index selector mutual exclusivity and declared cross-table OID validation.

Project skills update:

- Updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.

End-user/operator docs update:

- Updated `collector/snmp/metadata.yaml`.
- Regenerated `collector/snmp/integrations/snmp_devices.md`.
- Updated `collector/snmp/profile-format.md`.

End-user/operator skills update:

- No public/end-user AI skills were affected.

Lessons:

- Typed projection work should migrate source profiles and runtime consumers before deleting string-router code; doing the deletion only after the final vendor migration made the old-protocol leak scan decisive.
- Variable-length SNMP indexes require tail-based extraction for fields such as AFI/SAFI; fixed positional index assumptions work for IPv4 and fail for IPv6.
- BGP function cache freshness must be source-aware. A single profile failure must not freeze successful profiles in the same collection cycle.
- Integration docs should be regenerated locally after metadata changes. Waiting for CI would have hidden AC#69 drift until after merge.

Follow-up mapping:

- Implemented in SOW-0015:
  - Remove Cumulus and Alcatel BGP public capability claims until real fixtures exist.
  - Regenerate SNMP integration artifacts after metadata changes.
  - Document `partial_states`, empty routing-instance normalization, per-source stale-cache semantics, and `index_from_end` current behavior.
  - Delete the legacy `bgp_public*` runtime and BGP public-router underscore protocol.
  - Normalize remaining inline typed-domain profile sections into dedicated `bgp:` and `licensing:` fragments.
- Tracked in SOW-0014:
  - Typed licensing + typed BGP unsupported table-root caching for broad Cisco/Catalyst unsupported table walks.
- Resolved during pre-merge review:
  - RT-4 typed BGP chart-key identity. Public chart keys intentionally use the existing logical SNMP table metric model (`name` plus visible tag values), not BGP structural row identity. `TestTypedBGPMetricsUseLogicalChartIdentity` covers that same logical peer tags from different typed rows resolve to the same public chart key. Function-cache identity remains structural and is tested separately.
  - NOK-1 Nokia/TiMOS `tBgpPeerNgOperTable` high-numbered OID verification.
    Nokia documentation states release MIB files are packaged with releases and
    available from the Nokia support portal or on-device MIB bundles
    (`https://documentation.nokia.com/srlinux/25-3/books/system-mgmt/snmp.html`).
    The local untracked `mibs/TIMETRA-BGP-MIB.mib` was refreshed from the Nokia
    `TIMETRA-BGP-MIB` revision `LAST-UPDATED "202302150000Z"`. The public
    Observium mirror of Nokia's `TIMETRA-BGP-MIB`
    (`https://mibs.observium.org/mib/TIMETRA-BGP-MIB/`) shows the same
    TiMetra branch and high-numbered `tBgpPeerNgOperEntry` objects, including
    entries beyond `.160`. The refreshed MIB confirms `.177`, `.178`, and
    `.181-.188` as `MAX-ACCESS read-only`.
  - SCH-3 `partial: true` empty mapping/spec-validator consistency. The
    validator now returns a specific error for `partial: true` state configs
    without mapping items, and `TestValidateEnrichProfile_BGP` covers that
    contract.
  - RT3-1 stale BGP cache entries from permanent per-source failure. Expired
    stale entries are reaped after the configured stale window, and
    `TestBGPIntegration_ExpiredFailedProfileDoesNotKeepStaleRowsWithFreshProfiles`
    asserts the expired source key is removed from the cache.
  - SCH-2 typed BGP cross-table `table:` reference validation. Profile
    validation now resolves declared BGP row table names and rejects source or
    lookup-symbol OIDs outside the referenced table root. Existing runtime
    inference for non-declared cross-table roots remains supported by design.
  - SCH2-4 row-index selector mutual exclusivity. Profile validation rejects
    typed BGP values that set more than one of `index`, `index_from_end`, and
    `index_transform`.
  - SCH2-6 `index_from_end` plus `format` propagation coverage. The
    cross-table BGP row test now covers `IndexFromEnd` with `Format: "hex"`.
  - Dell IPv6 typed BGP coverage. `TestCollector_Collect_DellOS10BGP_IPv6Rows`
    covers IPv6 peer and peer-family rows with typed identity, descriptors,
    connection fields, and route counts.
  - Typed BGP descriptor underscore label behavior. The logical chart-identity
    test verifies descriptor labels such as `_local_address` and
    `_peer_description` remain visible labels and do not participate in public
    chart-key identity.
  - Final-review P2 validation coverage. `TestValidateEnrichProfile_BGP` now
    covers the remaining row-index selector combinations and BGP row kind vs
    field-group compatibility branches.
  - Final-review P2 Dell IPv6 dependency-table coverage.
    `TestCollector_Collect_DellOS10BGP_IPv6Rows` now asserts an IPv6
    dependency-table traffic counter in addition to identity, descriptors,
    state, and routes.
  - TST5-2/TST5-4/TST5-9 pre-merge test gaps. Added function-cache coverage
    for same-peer structural IDs from different profile rows, stale failure
    recovery, and vendor six-state mapping assertions for Arista, Dell,
    Juniper, and Alcatel.
- Rejected as a SOW-0015 follow-up:
  - Generated function-name capitalization (`Snmp:*`, `Docker:*`, `Mysql:*`, etc.) is a pre-existing global integration template behavior in `integrations/templates/functions.md`, not specific to BGP typed projection. It should be handled only as a separate integrations-docs decision if the project wants to normalize all generated Function names.
- Nothing remains pending before SOW close.

## Outcome

Completed. SNMP BGP monitoring now uses typed `bgp:` projection end to end, all BGP-bearing stock profiles are migrated, the legacy `bgp_public*` runtime is deleted, metadata/docs/spec artifacts are reconciled, external review blockers and selected P2 polish are addressed, SOW-0016 is folded into this SOW, and local raw `mibs/` reference files are removed.

## Lessons Extracted

Recorded in the Validation gate.

## Followup

All valid follow-up items from this SOW are implemented, rejected with evidence, or represented by SOW-0014. SOW-0016 was intentionally deleted after migration into this SOW by user decision.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
