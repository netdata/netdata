# SOW-0015 - SNMP BGP typed profile projection

## Status

Status: in-progress

Sub-state: user decisions 1-23 resolved; schema/API/projection and typed producer scaffold landed; typed BGP grouped-row refactor is landed; Cisco migration now includes typed cross-table BGP value sources.

## Requirements

### Purpose

Replace the WIP SNMP BGP monitoring branch's metric-name/tag routing layer with a clean typed BGP profile section and projection. Preserve normal ddsnmp chart-per-row behavior, keep BGP per-peer/per-family health templates enabled by default, and remove unsupported public coverage claims until real fixture evidence exists.

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

## Validation

Acceptance criteria evidence:

- Partial:
  - Typed `bgp:` profile section exists in `ddprofiledefinition.ProfileDefinition`.
  - Typed BGP output exists in `ddsnmp.ProfileMetrics.BGPRows`.
  - `Project(bgp)` and `Project(metrics, bgp)` projection paths exist and are tested.
  - Closed row kind, peer state, AFI/SAFI, and typed-field validation exists.
  - Spec now documents the BGP profile projection contract.
  - ddsnmpcollector now emits typed BGP rows for scalar and table `bgp:` configs.
  - BGP collection is wired into the regular SNMP profile collection path and keeps regular metrics on BGP row errors.
  - BGP profile stats are exposed alongside scalar/table/licensing/internal stats.
  - Typed BGP rows now project into BGP chart metrics and the BGP peers function cache.

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

Real-use evidence:

- Pending implementation.

Reviewer findings:

- Pending implementation.

Same-failure scan:

- Pending implementation.

Sensitive data gate:

- Pending implementation. Local raw MIB files under `mibs/` are untracked and must remain uncommitted.

Artifact maintenance gate:

- AGENTS.md: pending final check.
- Runtime project skills: pending final check.
- Specs: pending `.agents/sow/specs/snmp-profile-projection.md` update.
- End-user/operator docs: pending metadata/generated integration/doc checks.
- End-user/operator skills: pending final check.
- SOW lifecycle: pending move to current before code and done after completion.

Specs update:

- Updated `.agents/sow/specs/snmp-profile-projection.md` with BGP consumer, row shape, projection, inheritance, delivery, and validation guarantees.

Project skills update:

- Pending final determination after implementation.

End-user/operator docs update:

- Pending implementation.

End-user/operator skills update:

- Pending final determination after implementation.

Lessons:

- Pending implementation.

Follow-up mapping:

- Pending implementation.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

Open items that must be implemented, rejected with evidence, or represented by a separate pending/current SOW before this SOW can close:

- Remove Cumulus and Alcatel BGP public capability claims until real fixtures exist, then regenerate integration artifacts.
- Resolve or track RT-4: typed BGP chart-key collision risk when multiple profiles emit the same peer identity.
- Resolve or track RT3-1: stale BGP cache entries from permanent per-source failure are filtered from output after TTL but not reaped from memory.
- Resolve or track SCH-2: typed BGP cross-table `table:` references are not load-time resolved against actual table definitions.
- Resolve or track SCH-3: `partial: true` with an empty state mapping disagrees between spec intent and validator behavior.
- Resolve or track SCH2-4: `index`, `index_from_end`, and `index_transform` mutual-exclusivity is not validated or documented.
- Document or test SCH2-6: `index_from_end` plus `format` propagation is correct through `bgpValueSymbol` but lacks focused coverage.
- Document `partial_states`, empty routing-instance normalization, per-source stale-cache semantics, and `index_from_end` precedence in the projection spec and/or profile format docs.
- Verify or correct NOK-1: inherited Nokia/TiMOS `tBgpPeerNgOperTable` high-numbered OIDs `.177-.188` are used by legacy and typed profiles but are absent from the downloaded TIMETRA MIB excerpt.
- Complete SOW validation gate: reviewer findings, same-failure scans, sensitive-data gate, artifact gate, lessons, and final follow-up mapping.
- Remove local raw `mibs/` files before closing the SOW; `/mibs/` is ignored as defense-in-depth.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
