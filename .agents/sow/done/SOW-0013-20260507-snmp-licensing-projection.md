# SOW-0013 - SNMP licensing profile projection

## Status

Status: completed

Sub-state: implementation completed and final review findings closed on 2026-05-07; ready to move to `.agents/sow/done/`.

## Requirements

### Purpose

Replace the WIP SNMP licensing monitoring branch's hidden-metric delivery protocol with a clean typed licensing profile section and projection. Fix concrete profile correctness bugs while migrating to the typed schema, rather than patching the old hidden `_license_row*` protocol.

### User Request

User request summary:

- Analyze the squashed `pr-ktsaou-licensing-monitoring` branch after rebase on `master`.
- Find code smells, hacks, side effects, and profile correctness bugs.
- Prefer a clean end state over low churn or backward compatibility because licensing monitoring is WIP/nightly.
- Treat the current HiddenMetrics licensing path as another instance of the topology anti-pattern that was just removed.
- Use typed projection/schema-driven delivery analogous to the SNMP topology profile projection.
- Fix concrete correctness bugs as part of the projection migration, not as temporary patches to the hidden-metric protocol.

Detailed design source:

- `src/go/plugin/go.d/TODO-snmp-licensing-monitoring-review.md` (local-only development file; must not be committed).

### Assistant Understanding

Facts:

- The original WIP branch implemented SNMP licensing by encoding semantic license rows as underscore-prefixed hidden metrics named `_license_row*`.
- `ddsnmpcollector` generically moves underscore-prefixed metrics into `ProfileMetrics.HiddenMetrics`; this remains available for unrelated private metrics.
- The original SNMP collector licensing code consumed `pm.HiddenMetrics`, recognized `_license_row*`, and dispatched by string tag `_license_value_kind`.
- Recent SNMP topology work replaced the same class of metric-name/HiddenMetrics hack with top-level `topology:` profile rows and typed `ProfileMetrics.TopologyMetrics`.
- `ProfileMetrics.HiddenMetrics` must remain a generic delivery container for unrelated underscore-prefixed/private metrics; licensing must stop using it as its semantic transport.
- Downloaded local MIBs are review evidence only and must remain untracked:
  - `CISCO-SMART-LIC-MIB.my`
  - `CISCO-LICENSE-MGMT-MIB.mib`
  - `BLUECOAT-LICENSE-MIB.mib`
  - `CHECKPOINT-MIB.mib`
- User decision: keep these raw MIB files at the repository root during implementation, do not commit them, and delete them after implementation is verified.

Inferences:

- The root problem is not just a few bad OIDs. The root problem is the absence of a typed licensing contract between SNMP profiles, `ddsnmpcollector`, and the SNMP collector's licensing aggregation.
- Profile correctness bugs should not be used as parity targets for the new design. The typed profile migration should be authored from MIB truth.
- The clean shape likely mirrors topology: top-level `licensing:` rows, closed enums, catalog projection, typed `ProfileMetrics` output, validation, and profile-format documentation.

Unknowns:

- No open design decisions remain. The only accepted follow-up is unsupported licensing table-root caching for broad Cisco coverage, tracked by `.agents/sow/pending/SOW-0014-20260507-snmp-licensing-unsupported-table-cache.md`.

### Acceptance Criteria

- SNMP licensing profile data is represented by a first-class typed profile section/projection, not `_license_row*` hidden metrics.
- `ProfileMetrics.HiddenMetrics` remains available and tested as a generic non-licensing underscore-prefixed metric delivery container.
- SNMP licensing consumes typed licensing output from `ddsnmpcollector`, not magic metric names or `_license_value_kind` tags.
- Licensing profile schema has closed validation for signal kinds and rejects malformed profile rows at load time.
- Concrete profile correctness bugs are fixed in the migrated schema:
  - Cisco Smart scalar rows at the current `_cisco-base.yaml` lines 148, 172, 211, 229, and 247 use scalar instance suffix `.0`; Cisco Smart table rows are not treated as scalars.
  - Cisco traditional row identity does not merge distinct rows with the same feature name.
  - Check Point licensing OID mapping follows the `svnLicensing` table from the refreshed `CHECKPOINT-MIB.mib`.
  - Blue Coat derives `appLicenseStatusIndex` from the row index instead of reading the `not-accessible` object as `symbol.OID`.
  - Sophos and MikroTik ignored-state/sentinel behavior is no longer filename-gated.
  - Cisco licensing is represented by dedicated typed licensing mixins, not hidden `_license_row*` blocks in `_cisco-base.yaml`; broad Cisco coverage comes from `cisco.yaml` extending those mixins.
- Runtime merge identity uses `OriginProfileID` plus table OID plus an INDEX-derived row key for table rows. `OriginProfileID` is the logical profile file that declared the licensing row, including mixin-origin rows after `extends:` merge. It must not use display table names, stripped filenames, root matched profile names, or absolute source paths as structural identity.
- Eval/trial states do not generate warning/critical alerts by default; they may be exposed as informational function/chart state.
- Licensing health alerts are scoped by their SNMP-specific `snmp.license.*` contexts; extra `chart labels: component=licensing` filters are intentionally not used.
- Workstation-local provenance comments are sanitized before commit, including `bluecoat-proxysg.yaml:47` and path-bearing licensing fixtures under `ddsnmpcollector/testdata/licensing/`.
- Provenance sanitation explicitly covers:
  - `config/go.d/snmp.profiles/default/bluecoat-proxysg.yaml:47`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/checkpoint.snmprec:2`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/cisco-smart-iosxe-c9800.snmprec:2`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/cisco-traditional.snmpwalk:2`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/mikrotik-router.snmprec:2-3`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/sophos-xgs-firewall.snmprec:2`
- The four raw MIB files are deleted from repo root before SOW close-out and are never staged or committed.
- The local `TODO-snmp-licensing-monitoring-review.md` file remains out of the PR.
- Tests cover full loaded profiles, not only sliced licensing blocks.
- Profile-format documentation and project SNMP authoring skill describe the new licensing authoring contract.

## Analysis

Sources checked:

- `src/go/plugin/go.d/TODO-snmp-licensing-monitoring-review.md`
- `src/go/plugin/go.d/collector/snmp/licensing.go`
- `src/go/plugin/go.d/collector/snmp/licensing_state.go`
- `src/go/plugin/go.d/collector/snmp/licensing_vendor_sanity.go`
- `src/go/plugin/go.d/collector/snmp/licensing_charts.go`
- `src/go/plugin/go.d/collector/snmp/licensing_integration.go`
- `src/go/plugin/go.d/collector/snmp/func_licenses.go`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/collector.go`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/collector_scalar.go`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/collector_license_fixtures_test.go`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/licensing_test_helpers_test.go`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/*`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/_cisco-base.yaml`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/checkpoint.yaml`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/bluecoat-proxysg.yaml`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/fortinet-fortigate.yaml`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/mikrotik-router.yaml`
- `src/go/plugin/go.d/config/go.d/snmp.profiles/default/sophos-xgs-firewall.yaml`
- `src/go/plugin/go.d/collector/snmp/profile-format.md`
- `src/go/plugin/go.d/collector/snmp/metadata.yaml`
- `src/health/health.d/snmp.conf`
- `.agents/sow/specs/snmp-profile-projection.md`
- `.agents/sow/specs/sensitive-data-discipline.md`
- `.agents/skills/project-snmp-profiles-authoring/SKILL.md`
- `.agents/skills/project-writing-collectors/SKILL.md`

Current state:

- Licensing rows are first-class `licensing:` profile data delivered through typed `ProfileMetrics.LicenseRows`.
- License signal kinds, sentinel policies, state policies, symbol forbid-lists, row identity, and duplicate signals are validated at profile load.
- The `_license_row*` / `_license_value_kind` hidden-metric protocol is removed from production profile YAML and runtime licensing consumption.
- Cisco, Check Point, Blue Coat, Fortinet, MikroTik, and Sophos licensing profiles have been migrated to typed rows with MIB-derived corrections recorded in this SOW.
- Full-profile and fixture-backed tests cover the migrated licensing profile families, while focused unit tests cover schema, projection, aggregation, function, and delivery edge cases.

Risks:

- Broad SNMP profile blast radius if migrated profiles break ordinary metric collection.
- Function/UI regression if licensing aggregation output changes without matching docs/metadata/health updates.
- Alert noise if eval/grace states remain mapped to degraded warning semantics.
- Silent data loss if typed schema does not model scalar-only licensing rows and table licensing rows cleanly.
- Sensitive data/provenance risk from committing local MIB files, workstation paths, or unsanitized SNMP fixtures.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The licensing feature repeats the old topology design flaw: it encodes semantic non-chart observations as hidden chart metrics plus magic names/tags. The transport is fragile, hard to validate, and easy to silently break. Concrete profile bugs show why the hidden protocol is the wrong parity target; the clean implementation needs a typed licensing schema and output projection built from MIB evidence.

Evidence reviewed:

- `collector/snmp/licensing.go` hidden `_license_row*` extraction and `_license_value_kind` dispatch.
- `collector/snmp/ddsnmp/ddsnmpcollector/collector.go` generic underscore-prefix `HiddenMetrics` bucketing.
- `collector/snmp/ddsnmp/metric.go` and `ddsnmpcollector/collector_topology.go` typed topology projection precedent.
- `.agents/sow/specs/snmp-profile-projection.md` current metrics/topology profile projection contract.
- `config/go.d/snmp.profiles/default/_cisco-base.yaml` Cisco Smart scalar object OIDs without `.0`.
- Local `CISCO-SMART-LIC-MIB.my` proving Cisco Smart subtree is valid but scalar instances still need `.0`.
- Local `CISCO-LICENSE-MGMT-MIB.mib` proving Cisco traditional table has three index components.
- Local `CHECKPOINT-MIB.mib` proving `svnLicensing` object order and `licensingIndex MAX-ACCESS read-only`.
- Local `BLUECOAT-LICENSE-MIB.mib` proving `appLicenseStatusIndex MAX-ACCESS not-accessible`.

Affected contracts and surfaces:

- SNMP profile schema and profile validation.
- ddsnmp catalog projection consumers.
- ddsnmpcollector `ProfileMetrics` output.
- SNMP collector licensing aggregation, charts, function output, and alert inputs.
- Default SNMP profile YAMLs for Cisco, Check Point, Blue Coat, Fortinet, MikroTik, Sophos.
- SNMP profile-format documentation.
- Runtime project SNMP profile authoring skill.
- Integration metadata and health alerts if chart/function/alert semantics change.

Existing patterns to reuse:

- Top-level `topology:` typed profile section and `ProfileMetrics.TopologyMetrics`.
- Closed enum validation from topology kind validation.
- Catalog projection consumer model from `.agents/sow/specs/snmp-profile-projection.md`.
- Existing scalar/table collection machinery from `ddsnmpcollector`.
- Table/index accessibility rules in `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.
- Table-driven Go tests using `map[string]struct{}` where setup/assertion shapes match.

Risk and blast radius:

- Production regular SNMP metrics must not regress even though licensing is WIP/nightly.
- Licensing itself can break backward compatibility because it is WIP/nightly, but the end state must be coherent and maintainable.
- HiddenMetrics cannot be removed or redefined globally inside this SOW because it remains a generic private-metric mechanism with canary coverage; after licensing migration, grep currently shows no non-licensing production consumers.
- Profile YAML migration can change SNMP walk/GET load on broad vendor base profiles.
- Health templates and function output are user/operator-facing and require consistency with metadata/docs.

Sensitive data handling plan:

- Do not commit downloaded raw MIB files.
- Do not commit this local TODO file.
- Do not commit SNMP communities, SNMPv3 credentials, bearer tokens, customer hostnames, customer sysName/sysDescr, customer IPs, customer names, or personal data.
- Replace workstation-local provenance paths with upstream repository/commit/relative-path evidence where public, or sanitized descriptions where not.
- Any new fixtures must use sanitized public/vendor-derived values only.

Implementation plan:

- See `## Plan` below. That top-level plan is the canonical execution order and starts with validation/test scaffolding before schema/API work.

Validation plan:

- Strict fixture GET helper that fails on unexpected missing OIDs.
- Duplicate-detecting license row/signal assertions.
- Full-profile smoke tests for all licensing-bearing profile families.
- Negative schema validation tests for invalid licensing signal kinds/fields.
- Cisco Smart scalar `.0` collection test.
- Check Point corrected object-mapping test.
- Blue Coat index-derived identity test.
- Sentinel/ignored-state tests independent of profile filename.
- Sentinel parity test proving policies apply to both timestamp and remaining-value signals.
- Sophos state test proving ignored raw-state hints beat severity `0` according to the selected state policy.
- Sophos static `_license_id` uniqueness test covering the copy-pasted Sophos licensing blocks.
- Chart registration matrix tests for partial signal availability.
- Health alert tests if alerts remain in scope.
- Cold-start function test proving the current 503-before-first-collect behavior is either preserved or deliberately changed by a recorded decision.
- Cross-extends Cisco mixin dedup test.
- `Project(licensing)` metric-tag propagation positive and negative tests.
- Narrow Go test suites for `collector/snmp/ddsnmp/...` and `collector/snmp/...`.

Artifact impact plan:

- AGENTS.md: no expected update; existing process and collector rules already apply.
- Runtime project skills: update `.agents/skills/project-snmp-profiles-authoring/SKILL.md` if licensing authoring rules are added.
- Specs: update `.agents/sow/specs/snmp-profile-projection.md` or add a licensing-specific spec after implementation defines the durable contract. The spec update must capture the licensing consumer, projection rules, inheritance/merge rules, signal/policy enums, and local-only MIB evidence policy.
- End-user/operator docs: update `collector/snmp/profile-format.md`, metadata/docs/health references if chart/function/alert semantics change.
- End-user/operator skills: no expected update unless public Netdata AI skills reference SNMP licensing profile schema.
- SOW lifecycle: decisions are resolved; move to current/in-progress only when implementation begins.

Open-source reference evidence:

- Existing branch artifacts cite workstation-local mirrored-OSS paths and must be sanitized before commit:
  - `config/go.d/snmp.profiles/default/bluecoat-proxysg.yaml:47`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/checkpoint.snmprec:2`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/cisco-smart-iosxe-c9800.snmprec:2`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/cisco-traditional.snmpwalk:2`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/mikrotik-router.snmprec:2-3`
  - `collector/snmp/ddsnmp/ddsnmpcollector/testdata/licensing/sophos-xgs-firewall.snmprec:2`
- The four downloaded MIB files and the local TODO are currently untracked at the repository root / working directory and are not ignored. User decision: keep the MIB files at repo root during implementation, do not commit them, and delete them after implementation is verified. Keep this local TODO out of commits.

Open decisions:

- None. User selected: `2A 3A 4A 5C 6B 7B 8B 9B 10A 11A 12B 13A 14A 15A 17A 18A 19A 20A 21A 22A`, plus profile-origin identity option `1B`.
- Post-slice review decision: regular SNMP uses an explicit combined metrics+licensing projection, not ad hoc reattachment and not a second collector pass. This keeps single-consumer projections pure while allowing the regular SNMP collector to collect both chart metrics and typed licensing rows in one `ddsnmpcollector` pass.
- Follow-up projection API decision: use variadic `Project(consumer, consumers...)` for combined projections instead of a one-off `ProjectMetricsAndLicensing()` helper. Existing single-consumer callers stay unchanged, and regular SNMP calls `Project(metrics, licensing)`.

## Cross-Cutting Resolution Rules

These rules combine the resolved decisions above and prevent implementation drift.

1. Licensing structural identity uses a real origin profile id, not a stripped filename and not the root matched profile. Add an `OriginProfileID` field to the resolved profile / `ProfileMetrics` / typed licensing row path and use it for licensing identity. The value identifies the logical profile file that declared the licensing row, including mixin-origin rows after `extends:` merge; it must not expose absolute workstation paths as user-facing license source. Typed `LicenseRow` carries the table OID directly from the licensing producer; do not add table OID to generic `ddsnmp.Metric` solely for licensing.
2. Scalar identity defaults to `(origin-profile-id, scalar-symbol-OID)`. Profiles using explicit scalar grouping to aggregate multiple scalar OIDs into one row use `(origin-profile-id, licensing group id)` for that grouped row; grouped member signals must not also produce standalone scalar rows.
3. Sentinel policies are evaluated at typed licensing projection emit time. A sentinel-rejected value sets the target signal's `Has` flag to false, so SNMP licensing aggregation never sees sentinel values.
4. Bucket evaluation order is: ignored raw-state policy suppresses all other buckets; non-ignored hard failure conditions such as expired timers and exhausted usage produce broken; eval/trial-like states produce informational; grace/degraded states produce degraded; otherwise normal valid signals produce healthy or ignored according to the selected state policy.
5. Top-level metric tags propagate to `Project(licensing)` when `consumers:` is unset, matching topology semantics. The licensing-row forbid-list applies only to licensing row value symbols, not to top-level tags.
6. Repeated-signal load errors apply to duplicate `(structural identity, signal kind)` inside one resolved profile after inheritance merge. Table-row and scalar-row identities are different shapes and are never duplicates by accident, so a Cisco profile extending both Cisco licensing mixins is valid.
7. `from: <oid>` sibling references are expanded by the typed licensing producer at collection time. For table rows, `from` must refer to a peer in the same SNMP table row; for scalar rows, `from` must refer to another scalar in the same profile or explicit scalar group. Cross-profile `from` is a validation error.
8. Sophos-style sibling date migration uses this canonical shape:

   ```yaml
   licensing:
     - id: sophos-base-firewall
       identity:
         id: { value: base_firewall }
         name: { value: Base Firewall }
       state:
         from: 1.3.6.1.4.1...
         mapping: { 0: ignored, 1: healthy, 2: degraded, 3: broken }
       signals:
         expiry:
           from: 1.3.6.1.4.1...
           format: text_date
           sentinel: [timer_zero_or_negative]
   ```

9. Regular SNMP profile setup uses `Project(metrics, licensing)`. `Project(metrics)` remains metrics-only, `Project(licensing)` remains licensing-only, and the variadic projection keeps the typed licensing producer reachable without a duplicate SNMP pass.
10. Mixed projections use the public variadic `Project(consumer, consumers...)` API. This keeps the single-consumer API stable while avoiding one-off helpers for every valid consumer combination.

## Implications And Decisions

### Decision 1 - Licensing delivery contract

Status: resolved by user.

Selected option: A. First-class typed licensing profile section/projection.

Rejected alternatives:

- Keep `_license_row*` hidden metrics and add validation. Rejected because it preserves the same technical debt that topology just removed.
- Model licenses as normal exported profile metrics. Rejected because license rows are semantic aggregation inputs, not chart metrics.

### Decision 2 - YAML section key name

Status: resolved by user: 2A.

Options:

- A. `licensing:`
  - Pros: matches singular top-level `topology:` precedent; describes the feature domain rather than individual row count.
  - Cons: less literal than `licenses:` for a list.
- B. `licenses:`
  - Pros: literal list name.
  - Cons: diverges from `topology:` naming style and reads more like a user-facing entity list than a profile capability section.

Recommendation: A. Use top-level `licensing:`.

Selected option: A. Use top-level `licensing:`.

### Decision 3 - YAML schema shape

Status: resolved by user: 3A.

Options:

- A. Row-centric top-level `licensing:` blocks.
  - Each block declares table/scalar source, identity/descriptors/state, and typed signals.
  - Pros: mirrors how users think about one license row; keeps identity and signals together; suitable for table-based vendors.
  - Cons: scalar-only vendors need a row wrapper.
- B. Signal-centric top-level `licensing:` blocks.
  - Each block declares one signal and separate grouping metadata.
  - Pros: close to the current hidden-metric implementation.
  - Cons: repeats row reconstruction complexity; easier to mis-group.
- C. Separate top-level `licenses:` identity blocks plus `license_signals:` signal blocks.
  - Pros: highly explicit.
  - Cons: verbose, more merge/linkage validation, more authoring friction.

Recommendation: A. Row-centric `licensing:` blocks are the cleanest end state and best match the topology top-level-section precedent. The schema must explicitly support scalar-only rows without forcing authors to invent a synthetic table wrapper.

Selected option: A. Row-centric top-level `licensing:` blocks.

### Decision 4 - Runtime typed output shape

Status: resolved by user: 4A.

Options:

- A. `ProfileMetrics.LicenseRows []LicenseRow`, where each row has identity/descriptors plus typed grouped signal structs:
  - `State LicenseState`
  - `Expiry LicenseTimer`
  - `Authorization LicenseTimer`
  - `Certificate LicenseTimer`
  - `Grace LicenseTimer`
  - `Usage LicenseUsage`
  - each grouped struct carries an explicit `Has` boolean rather than using pointers.
- B. `ProfileMetrics.LicenseRows []LicenseRow`, where each row has `Signals map[LicenseSignalKind]LicenseSignal`.
- C. Separate typed slices such as `LicenseTimers`, `LicenseUsage`, `LicenseStates`.

Recommendation: A. Licensing signals are heterogeneous. Timer, usage, and state values have different shapes, so a uniform `map[LicenseSignalKind]LicenseSignal` either becomes a tagged union or a wide optional-field struct. Keep `LicenseSignalKind` as the closed schema/validation enum, but make runtime rows compile-time clear with grouped structs. Use by-value grouped structs plus `Has` flags so `LicenseRows` can be shallow-cloned like the current cache without pointer aliasing surprises.

Selected option: A. `ProfileMetrics.LicenseRows []LicenseRow` with by-value grouped signal structs plus `Has` flags.

### Decision 5 - Runtime row identity and display grouping

Status: resolved by user: 5C.

Options:

- A. Structural identity only: origin profile id + table OID + INDEX-derived row key for table rows; origin profile id + scalar OID/block id for scalar rows.
- B. Semantic identity: vendor-provided license ID/name/feature only.
- C. Hybrid: structural identity for collection/dedup, semantic identity for display/grouping.

Recommendation: C. Structural identity prevents Cisco-style feature-name collisions; semantic identity remains useful for UI/function display. Structural identity must use table OID, not display table name, and must not use stripped filenames or the root matched profile as the source identity. Add a real `OriginProfileID` field to the resolved profile / `ProfileMetrics` / typed licensing row path for licensing structural identity. `OriginProfileID` is the logical profile file that declared the licensing row, including mixin-origin rows after `extends:` merge. Typed `LicenseRow` carries the table OID directly from the licensing producer; do not add table OID to generic `ddsnmp.Metric` solely for licensing.

Selected option: C. Structural identity for collection/dedup; semantic identity for display/grouping.

### Decision 6 - Numeric sentinel policy

Status: resolved by user: 6B.

Options:

- A. Fully declarative YAML sentinel rules.
  - Pros: maximum flexibility.
  - Cons: turns profile YAML into a rule language and is hard to validate cleanly.
- B. Closed built-in sentinel policy names referenced from YAML.
  - Initial set: `timer_zero_or_negative`, `timer_u32_max`, `timer_pre_1971`.
  - Pros: validates cleanly; removes filename gates; keeps YAML readable.
  - Cons: new sentinel patterns require code/schema updates.
- C. Runtime vendor-specific code keyed by profile filename/source.
  - Pros: fastest to patch.
  - Cons: repeats current MikroTik filename-gated behavior.

Recommendation: B. Use closed built-in sentinel policies referenced from typed signal config. Sentinel policies attach per signal field and apply to both absolute timestamp signals and remaining-value signals; do not repeat today's timestamp-only sentinel asymmetry.

Selected option: B. Closed built-in sentinel policy names referenced from YAML.

### Decision 7 - Raw-state classification policy

Status: resolved by user: 7B.

Options:

- A. Fully declarative YAML raw-state match rules.
  - Pros: vendor-specific states can be modeled without code changes.
  - Cons: another profile rule language; easy to make matching inconsistent across vendors.
- B. Closed built-in state policy names referenced from YAML, with profile-provided severity mappings allowed.
  - Pros: validates cleanly and centralizes bucket semantics.
  - Cons: new policy classes require code/schema updates.
- C. Runtime vendor-specific code keyed by profile filename/source.
  - Pros: quick for one-off vendors.
  - Cons: repeats current filename/source gates.

Recommendation: B. State classification is string/bucket policy, distinct from numeric sentinel handling. The initial rule must state that ignored raw-state hints win over severity `0` so Sophos `none` / `not_subscribed` rows do not become fake healthy licenses. Prefer modeling this in the typed YAML by not mapping ignored vendor states to severity `0`; if a profile still supplies both raw ignored state and severity `0`, runtime suppression must choose ignored.

Selected option: B. Closed built-in state policy names referenced from YAML, with ignored raw-state hints winning over severity `0`.

### Decision 8 - Chart lifecycle

Status: resolved by user: 8B.

Options:

- A. Register fixed licensing chart set and rely on gaps.
- B. Register charts lazily based on observed signal classes.
- C. Fixed charts but split eval/grace/informational states away from degraded alerts.

Recommendation: B. Lazy charts match the docs' conditional-language intent and avoid empty chart clutter. Lazy registration must guard per chart id, not by checking only the first licensing chart id.

Selected option: B. Register charts lazily based on observed signal classes.

### Decision 9 - Alert policy and health scoping

Status: resolved by user: 9B.

Options:

- A. Keep current broad alert behavior.
  - Pros: no alert rewrite.
  - Cons: eval/trial/grace can alert too aggressively.
- B. Explicit alert table scoped by SNMP-specific licensing contexts.
  - Policy: eval/trial -> informational only, no alert; grace -> WARN with delay; degraded -> WARN; broken/expired -> CRIT; usage pressure remains WARN/CRIT by percentage thresholds.
  - Health templates target `snmp.license.*` contexts directly; additional chart-label filters are redundant for these SNMP-specific contexts.
  - Pros: operator intent is explicit and avoids noisy eval/trial alerts.
  - Cons: requires health-template review and tests.
- C. No default alerts for WIP licensing.
  - Pros: avoids false positives while the feature matures.
  - Cons: less useful out of the box.

Recommendation: B. Licensing contexts are already SNMP-specific, so direct `on: snmp.license.*` health templates are sufficient. This decision depends on Decision 19 for the eval/trial/grace bucket model.

Selected option: B. Explicit alert table scoped by SNMP-specific licensing contexts. User later confirmed that alert `chart labels: component=licensing` filters are redundant and should not be used.

### Decision 10 - Catalog projection consumer

Status: resolved by user: 10A.

Options:

- A. Add `ConsumerLicensing` and `Project(licensing)`.
  - Projection keeps `licensing:` rows, drops regular `metrics:`, drops `topology:`, and drops `virtual_metrics`.
  - Metadata and top-level tag behavior must be explicit for licensing.
  - Pros: matches topology projection design; prevents accidental metrics/topology leakage.
  - Cons: requires extending consumer validation and projection tests.
- B. Reuse metrics projection and filter licensing later.
  - Pros: less projection work.
  - Cons: repeats hidden coupling and tag leakage risk.

Recommendation: A. Add a first-class `licensing` consumer. `Project(licensing)` drops `metrics:`, `topology:`, and `virtual_metrics`. Metadata and top-level metric tags with no explicit `consumers:` should propagate to licensing only when they are device/profile identity; licensing-specific projection tests must cover positive and negative tag propagation.

Selected option: A. Add `ConsumerLicensing` and `Project(licensing)`.

### Decision 11 - Profile inheritance/extends merge

Status: resolved by user: 11A.

Options:

- A. Merge licensing rows by structural identity and derived profile rows override inherited rows.
- B. Append all licensing rows and deduplicate only at runtime.
- C. Reject duplicate licensing structural identities at load time.

Recommendation: A with load errors for conflicting incompatible definitions. Use table OID plus INDEX-derived row key for table rows; use scalar OID/block id for scalar rows. Cross-profile dedup runs after profile matching, mirroring topology projection semantics.

Selected option: A. Merge by structural identity with derived override and load errors for incompatible conflicts.

### Decision 12 - Cisco licensing inheritance scoping

Status: resolved by user: 12B.

Options:

- A. Keep Cisco licensing in `_cisco-base.yaml`.
  - Pros: every Cisco profile gets licensing automatically.
  - Cons: broad walk/GET blast radius across many Cisco profiles, including devices that may not support the licensing MIBs.
- B. Move Cisco licensing to explicit Cisco licensing mixin(s), and extend only profiles with evidence/fixtures.
  - Pros: clean scoping and lower SNMP cost; makes licensing support explicit.
  - Cons: requires deciding which Cisco profiles opt in.
- C. Keep in base but add runtime/sysObjectID capability gates.
  - Pros: centralized.
  - Cons: profile system does not currently express per-row sysObjectID gates cleanly.

Recommendation: B. Licensing is WIP and clean scoping matters more than low churn. Name the mixins `_cisco-licensing-traditional.yaml` and `_cisco-licensing-smart.yaml`; per-profile opt-in requires MIB evidence or fixture coverage.

Selected option: B. Move Cisco licensing to explicit Cisco licensing mixin(s).

### Decision 13 - `licenseDateFromTag` disposition

Status: resolved by user: 13A.

Options:

- A. Delete `licenseDateFromTag` during migration; parse text dates through typed licensing signal config or existing `format: text_date` on fresh symbol values.
- B. Generalize it as a non-licensing `text_date` transform helper.
- C. Keep it for compatibility with old hidden licensing rows.

Recommendation: A. It is licensing-domain logic inside generic transform machinery and should not survive the hidden-protocol removal. This requires Decision 18 so Sophos-style sibling-OID date values can be declared directly in the typed schema.

Selected option: A. Delete `licenseDateFromTag` during migration.

### Decision 14 - Function/chart/health unit contract

Status: resolved by user: 14A.

Options:

- A. Keep separate units per surface and document them: chart metrics in seconds, Function duration fields in milliseconds, health alerts convert chart seconds to days for display.
- B. Normalize every surface to seconds.
- C. Normalize every surface to milliseconds.

Recommendation: A. It matches current Netdata function duration conventions while keeping chart metrics simple for health calculations. Conversion ownership: charts store raw seconds, health divides chart seconds by 86400 for day display, and the Function multiplies durations by 1000 for millisecond `FieldTransformDuration` cells.

Selected option: A. Keep per-surface units and document conversion ownership.

### Decision 15 - `HiddenMetrics` post-migration status

Status: resolved by user: 15A.

Options:

- A. Preserve `HiddenMetrics` as a generic underscore/private-metric mechanism and keep its non-licensing canary tests.
- B. Remove `HiddenMetrics` entirely after licensing migration if no production non-licensing consumers remain.
- C. Keep producer only, but document it as deprecated and unused.

Recommendation: A for this SOW. Existing tests intentionally cover `_privateMetric` preservation. After licensing migrates, current grep evidence shows no non-licensing production consumers; the canary test is the reason to preserve the generic producer until a separate audit decides removal.

Selected option: A. Preserve `HiddenMetrics` as a generic underscore/private-metric mechanism for this SOW.

### Decision 16 - MIB/source evidence policy

Status: resolved by user: 16A.

Selected option: A. Keep raw MIBs local-only, cite only sanitized object names/OIDs in SOW/docs.

Follow-through requirement: sanitize committed workstation-local provenance comments, including `bluecoat-proxysg.yaml:47`.

### Decision 17 - Scalar row structural identity

Status: resolved by user: 17A.

Options:

- A. Scalar identity is `(origin-profile-id, scalar-symbol-OID)`.
  - Pros: prevents unrelated scalar license blocks from collapsing when they share a semantic id; simple to validate.
  - Cons: scalar rows that intentionally compose multiple OIDs into one license need an explicit row/group id.
- B. Scalar identity is `(origin-profile-id, semantic license id)`.
  - Pros: easy for Cisco Smart-style scalar groups.
  - Cons: repeats current collision risk across unrelated scalar MIB objects.
- C. Scalar rows require an explicit structural `id:` in YAML.
  - Pros: author-controlled grouping.
  - Cons: easy to make unstable or semantic by accident.

Recommendation: A, with an explicit grouping field only for scalar blocks that intentionally aggregate multiple scalar OIDs into one semantic license row. This depends on the `OriginProfileID` field selected by Decision 5; do not use stripped filenames or root matched profile names as scalar identity.

Selected option: A. Scalar identity is `(origin-profile-id, scalar-symbol-OID)`, with explicit grouping for intentional multi-scalar rows.

### Decision 18 - Sibling-OID signal decoding

Status: resolved by user: 18A.

Options:

- A. Allow typed signals to declare `from: <oid>` plus optional `format:` so a signal can decode a sibling scalar/table OID directly.
  - Pros: removes `licenseDateFromTag`; supports Sophos-style sibling expiry values cleanly; keeps date parsing in typed licensing collection.
  - Cons: schema and collector need explicit source/reference validation.
- B. Require every signal value to be the row anchor symbol value.
  - Pros: simpler collector.
  - Cons: blocks deleting `licenseDateFromTag` for current Sophos profiles or forces awkward profile duplication.
- C. Keep a generic transform/helper path for sibling values.
  - Pros: lower schema work.
  - Cons: preserves the transform side channel this migration is meant to delete.

Recommendation: A. Typed `from:` references are the clean schema replacement for Sophos-style sibling-OID decoding. The typed licensing producer expands `from` at collection time. For table rows, `from` must refer to a peer value in the same SNMP table row; for scalar rows, `from` must refer to another scalar in the same profile or explicit scalar group. Cross-profile `from` is a validation error.

Selected option: A. Allow typed signals to declare `from: <oid>` plus optional `format:`.

### Decision 19 - Eval/trial/grace bucket model

Status: resolved by user: 19A.

Options:

- A. Add an informational bucket/dimension. Eval/trial/waiting/initialized states map to informational; grace remains degraded and alerts with delay.
  - Pros: preserves visibility without warning on normal eval/trial states; matches the selected alert policy.
  - Cons: changes chart dimensions and metadata/health docs.
- B. Map eval/trial states to ignored.
  - Pros: no new bucket.
  - Cons: hides useful licensing state and overloads ignored semantics.
- C. Keep eval/trial as degraded but suppress alerts by expression.
  - Pros: less aggregation change.
  - Cons: charts still look degraded and health logic becomes more complex.

Recommendation: A. Add an informational bucket/dimension and keep actionable grace/degraded/broken separate. Move `evaluation`, `eval`, `trial`, `evaluation_subscription`, and `evaluation_period` out of degraded hints into a new informational hint set.

Selected option: A. Add an informational bucket/dimension.

### Decision 20 - Licensing-row validation forbid-list

Status: resolved by user: 20A.

Options:

- A. Licensing row value symbols allow `format` and `mapping`, but reject chart/export fields and transforms: `chart_meta`, `metric_type`, `transform`, `scale_factor`, `constant_value_one`, and underscore-prefixed generated names.
  - Pros: keeps useful SNMP decoding (`format`, simple mappings) while blocking chart-only and side-channel behavior.
  - Cons: requires licensing-specific validation paths rather than reusing topology's forbid-list exactly.
- B. Reuse topology's stricter forbid-list.
  - Pros: simpler validation.
  - Cons: incorrectly forbids `format` and `mapping`, which licensing needs for dates and state decoding.
- C. Allow all `SymbolConfig` fields.
  - Pros: flexible.
  - Cons: repeats hidden protocol mistakes via transforms/chart-only fields.

Recommendation: A.

Selected option: A. Allow `format` and `mapping`; reject chart/export fields, transforms, scale/constant hacks, and underscore-generated names.

### Decision 21 - Repeated-signal conflict semantics

Status: resolved by user: 21A.

Options:

- A. Same structural identity + same signal kind is a load-time error unless an extending profile overrides the inherited definition through the merge rules.
  - Pros: prevents silent first-wins/last-wins data loss.
  - Cons: stricter authoring.
- B. Derived wins and same-file duplicates are allowed last-wins.
  - Pros: flexible.
  - Cons: hides copy-paste mistakes.
- C. First wins, matching current runtime behavior.
  - Pros: easiest migration from current code.
  - Cons: preserves today's silent duplicate loss.

Recommendation: A.

Selected option: A. Same structural identity plus same signal kind is a load-time error unless handled by valid extends override semantics.

### Decision 22 - Function RequiredParams policy

Status: resolved by user: 22A.

Options:

- A. Add explicit `RequiredParams` for `snmp:licenses`, matching the stricter Function interface style used by related functions.
  - Pros: clearer function contract and consistency with `interfaces`.
  - Cons: requires checking any caller assumptions.
- B. Keep no required params.
  - Pros: preserves current Function shape.
  - Cons: leaves the contract looser than adjacent functions.

Recommendation: A, unless caller review shows `snmp:licenses` is intentionally parameterless. Caller review found `snmp:licenses` is intentionally parameterless: it returns the complete per-device license table, and unlike `interfaces`, there is no natural required filter. Implementation should keep `RequiredParams` empty and add a short code comment plus a test asserting the method config is intentionally parameterless.

Selected option: A with caller-review exception. `snmp:licenses` remains intentionally parameterless; document and test that contract.

## Plan

1. Move SOW to `.agents/sow/current/` and mark `Status: in-progress`.
2. Add validation/test scaffolding first: strict GET helper, duplicate-detecting helpers, cold-start Function test, sentinel parity tests, projection tag-propagation tests, and full-profile smoke harness.
3. Add schema/API types and validation for typed licensing.
4. Add typed collector output and tests while preserving generic HiddenMetrics.
5. Rewrite SNMP licensing aggregation to consume typed `ProfileMetrics.LicenseRows` while legacy hidden rows remain present.
6. Re-author profiles from MIB truth and add full-profile tests.
7. Delete hidden licensing protocol artifacts.
8. Before final commit/close-out, delete the four raw MIB files from repo root and keep the local TODO out of the PR.
9. Validate, run reviewer pass, update artifacts/specs/skills, and close SOW.

## Execution Log

### 2026-05-07

- Created pending SOW from the completed licensing branch review.
- Recorded accepted typed-projection direction and open implementation decisions.
- No source code implementation started.
- Folded in Claude's pending-decision review:
  - accepted row-centric YAML direction;
  - replaced runtime signal-map recommendation with typed grouped sub-struct recommendation;
  - split sentinel and raw-state policies;
  - added missing decisions for projection consumer, inheritance, Cisco scoping, transform helper disposition, units, health scoping, and HiddenMetrics status;
  - recorded MIB evidence policy as resolved 16A.
- Folded in second Claude readiness review:
  - accepted all 16 decisions with tightenings;
  - added scalar row identity, sibling-OID decoding, eval/trial/grace bucket model, licensing validation forbid-list, repeated-signal conflict semantics, and Function RequiredParams decisions;
  - fixed decision numbering to use SOW decision ids consistently;
  - recorded workstation-path sanitation and local MIB/TODO hygiene requirements;
  - tightened Cisco Smart scalar `.0`, sentinel parity, per-chart lazy guards, SNMP-specific health context scoping, metric-tag projection, Cisco mixin names, unit conversion ownership, and HiddenMetrics preservation facts.
- Recorded user decision bundle: `2A 3A 4A 5C 6B 7B 8B 9B 10A 11A 12B 13A 14A 15A 17A 18A 19A 20A 21A 22A`.
- Recorded local MIB handling decision: keep raw MIBs at repo root during implementation, do not commit them, delete them after implementation is verified.
- Recorded user decision `1B`: licensing structural identity uses `OriginProfileID`, the logical profile file that declared the licensing row, including mixin-origin rows after `extends:` merge.
- Recorded post-slice review decision `combined metrics+licensing projection`: regular SNMP profile setup uses `Project(ConsumerMetrics, ConsumerLicensing)` so `licensing:` rows are delivered to `ddsnmpcollector` in the same pass as ordinary metrics; single-consumer projections remain pure.
- Recorded user decision `projection API option A`: replace the one-off combined projection helper with variadic `Project(consumer, consumers...)`; regular SNMP will call `Project(ConsumerMetrics, ConsumerLicensing)`.
- Activated this SOW by moving it from `.agents/sow/pending/` to `.agents/sow/current/` and setting `Status: in-progress`.
- Phase 1 scaffolding started:
  - replaced the lenient fixture GET helper with `mustExpectSNMPGetFromFixture`, which fails immediately when a test asks for an OID absent from the fixture;
  - added duplicate-detecting license grouping helpers for `(license id, signal kind)` and Sophos state/expiry fixture grouping, so tests fail on silent overwrite instead of hiding duplicates;
  - kept the Cisco Smart entitlement-only fixture scoped to the entitlement table because the public fixture explicitly lacks Smart Licensing scalar registration/auth/certificate timers.
- Validation note: strict fixture coverage exposed Cisco Smart scalar fixture coverage gaps. A later reviewer also found the broad ddsnmpcollector `Version()` failures were caused by licensing test helper leakage of inherited topology rows, not pre-existing branch behavior; the helper must clear `Topology` and `Licensing` for legacy hidden-protocol tests.
- Phase 2 schema/API skeleton started:
  - added top-level `licensing:` profile definition storage with clone support;
  - added `ConsumerLicensing` to the closed consumer enum and projection path;
  - added closed licensing signal kind, sentinel policy, and state policy enums;
  - added typed runtime `ProfileMetrics.LicenseRows []LicenseRow` with grouped state/timer/usage structs;
  - added licensing validation for invalid policy names, invalid signal kinds, and forbidden licensing value-symbol fields while allowing `format` and `mapping`.
- Added projection coverage proving `Project(licensing)` keeps `licensing:` rows and licensing-selected metadata/metric tags while dropping metrics, topology rows, and virtual metrics.
- Added `OriginProfileID` propagation:
  - profile loading assigns a relative logical origin path to each directly declared `licensing:` row;
  - `extends:` merge appends base licensing rows while preserving their original declaring profile id;
  - added a merge test proving a row declared in `device.yaml` keeps `device.yaml` and an inherited row declared in `_licensing.yaml` keeps `_licensing.yaml`.
- Phase 4 typed collector output started:
  - added a typed licensing producer that emits `ProfileMetrics.LicenseRows` from top-level `licensing:` rows;
  - scalar licensing rows use SNMP GETs and table licensing rows use SNMP walks while preserving `HiddenMetrics` as a separate generic underscore-prefixed metric container;
  - table licensing rows reuse the existing table structure cache path after the first walk, so the typed projection does not introduce a permanent per-cycle table walk;
  - table licensing rows carry `OriginProfileID`, table OID, raw row index, structural id, identity/descriptors, state, timer, usage, static tags, and row metric tags;
  - scalar licensing rows support explicit group ids, literal values, sibling `from:` OIDs, text-date formatting, mappings, and sentinel filtering at emit time.
- Folded in post-slice readiness review fixes before starting the consumer rewrite:
  - wired regular SNMP setup to `Project(ConsumerMetrics, ConsumerLicensing)` so typed `LicenseRows` are reachable in production without a second SNMP pass;
  - fixed the legacy licensing test helper to clear inherited `Topology` and `Licensing` rows, which removed the extra topology `Version()` calls in Cisco/MikroTik hidden-protocol tests;
  - extended licensing validation for repeated `(structural identity, signal kind)` rows, table `from:` scope, scalar multi-signal grouping, descriptor-only rows, scalar index misuse, underscore-prefixed legacy names, and closed value formats;
  - applied sentinel filtering to usage signals too, not only timer signals;
  - kept licensing table walk failures from poisoning the generic metric `missingOIDs` cache;
  - added `Stats.Metrics.Licensing` so typed licensing rows are counted separately from ordinary metric rows.
- Replaced the temporary combined-projection helper shape with the resolved variadic `Project(consumer, consumers...)` API; regular SNMP now calls `Project(ConsumerMetrics, ConsumerLicensing)`, while topology and licensing-only callers continue using single-consumer projections.
- Folded in the follow-up Claude implementation-slice review before starting the consumer rewrite:
  - accepted and fixed `mergeLicensing` override semantics by making derived `licensing:` rows replace inherited rows with the same pre-collection merge identity;
  - accepted and fixed the shared `missingOIDs` poisoning risk by removing the empty-walk table-missing cache write; explicit no-such GET responses still mark OIDs missing;
  - accepted and fixed licensing cross-table tag wiring by building a shared licensing table-name/OID map, walking cross-table dependencies before row processing, and caching dependency metadata;
  - accepted and fixed the D20 forbid-list gap by rejecting `extract_value`, `match_pattern`, and `match_value` on licensing row value symbols;
  - accepted and fixed state-policy-without-source validation, timer timestamp/remaining ambiguity, usage sentinel parity, cache-config namespace collisions, and duplicated structural-identity helper logic;
  - accepted and fixed licensing observability separation by adding `Stats.Timing.Licensing` and `Stats.Errors.Processing.Licensing`;
  - accepted the stats accounting direction: `Stats.Metrics.Licensing` counts typed license rows, while `Stats.Metrics.Tables` and `Stats.Metrics.Rows` remain ordinary chart-metric table counters;
  - rejected treating scalar `from:` acceptance as a blocker: scalar rows are the scalar group scope, table `from:` has the strict same-table gate, and cross-profile `from:` has no schema path.
- Started the consumer rewrite slice:
  - replace `collector/snmp/licensing.go` hidden `_license_row*` / `_license_value_kind` extraction with typed `ProfileMetrics.LicenseRows`;
  - keep hidden-profile fixture tests only as migration reference, not as the target consumer contract;
  - update aggregation/state/function tests around typed rows, including ignored/informational/degraded/broken bucket behavior;
  - migrate vendor profile families only after the typed consumer is working.
- Recorded Cisco licensing scope refinement:
  - Cisco licensing remains in dedicated licensing mixin/profile files, not `_cisco-base.yaml`;
  - current WIP Cisco licensing has two table OIDs (`clmgmtLicenseInfoTable` and `ciscoSlaEntitlementInfoTable`);
  - unsupported scalar GET OIDs are cached as exact missing OIDs after clean no-data responses;
  - unsupported table-root walks that return explicit `NoSuchObject` / `NoSuchInstance` should be distinguished from empty tables with zero rows;
  - user selected broad Cisco coverage: `cisco.yaml` should extend the dedicated Cisco traditional and smart licensing mixins;
  - broad Cisco licensing coverage is acceptable with a tracked follow-up for carefully scoped explicit table-root no-such caching so unsupported licensing table probes do not remain permanent per-poll cost.
- Migrated Cisco licensing profiles:
  - added `_cisco-licensing-traditional.yaml` and `_cisco-licensing-smart.yaml`;
  - removed hidden Cisco `_license_row*` and `_license_value_kind` profile blocks from `_cisco-base.yaml`;
  - made `cisco.yaml` extend both dedicated licensing mixins;
  - Cisco Smart scalar OIDs now use scalar instance suffix `.0`; the Smart entitlement OID remains a table;
  - Cisco traditional typed rows use the three-component MIB index as the semantic row id, while runtime structural identity remains origin profile + table OID + row key;
  - treated zero-length SNMP `DateAndTime` values as no-value for licensing timers, matching the Cisco MIB's empty-octet-string not-applicable behavior.
- Cisco migration validation passed:
  - `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_Cisco|TestCollector_Collect_.*LicensingProfile_Fixture'`;
  - `go test -count=1 ./collector/snmp/ddsnmp/...`;
  - `go test -count=1 ./collector/snmp/...`;
  - `rg '_license_row|_license_value_kind|licenseDateFromTag' config/go.d/snmp.profiles/default` returned no matches.
- Folded in the final pre-PR review fix batch:
  - replaced all-at-once licensing chart registration with per-signal-class lazy registration and per-chart idempotent guards;
  - kept SNMP licensing health alerts scoped by their `snmp.license.*` contexts and added the agreed grace-period warning delay;
  - verified Fortinet licensing table OIDs against Fortinet's FortiGate system MIB documentation for `fgLicContractTable`, `fgLicVersionTable`, and `fgLicAlContractTable`, and recorded the official documentation URL in the Fortinet profile;
  - corrected Cisco Smart entitlement `invalidTag(11)` from healthy to broken based on the local `CISCO-SMART-LIC-MIB.my` definition;
  - removed the dead MikroTik filename/source-gated consumer sanity path because `timer_pre_1971` now drops the sentinel at typed-producer time;
  - documented licensing stats separation, documented and tested the intentionally parameterless `snmp:licenses` function contract, hardened scalar literal-only licensing row validation, expanded licensing forbid-list tests, and consolidated licensing fixture tests into a map-driven harness;
  - aligned `.agents/sow/specs/snmp-profile-projection.md` with implemented scalar `from:` validation and metadata `id_tags` projection behavior.
- Updated durable project artifacts for the typed projection slice:
  - extended `.agents/sow/specs/snmp-profile-projection.md` with the licensing consumer, projection rules, typed delivery, identity rules, and validation guarantees;
  - extended `.agents/skills/project-snmp-profiles-authoring/SKILL.md` with licensing authoring guardrails and the `ProfileMetrics.LicenseRows` delivery rule.

## Validation

Acceptance criteria evidence:

- Typed schema/projection:
  - `collector/snmp/ddsnmp/ddprofiledefinition/licensing.go` defines typed licensing config, closed signal kinds, sentinel policies, state policies, clone methods, and source fields.
  - `collector/snmp/ddsnmp/metric.go` defines `ProfileMetrics.LicenseRows` and typed `LicenseRow`, `LicenseState`, `LicenseTimer`, and `LicenseUsage`.
  - `collector/snmp/profile_sets.go` uses `Project(ConsumerMetrics, ConsumerLicensing)` so regular SNMP collection gets metrics and licensing in one ddsnmpcollector pass.
- Hidden protocol removal:
  - `rg '_license_row|_license_value_kind|licenseDateFromTag' config/go.d/snmp.profiles/default` returned no matches during implementation.
  - Runtime licensing consumes `pm.LicenseRows`; `HiddenMetrics` remains only as generic underscore/private metric delivery.
- Profile correctness:
  - Cisco licensing lives in `_cisco-licensing-traditional.yaml` and `_cisco-licensing-smart.yaml`, with `cisco.yaml` broadly extending both by user decision.
  - Cisco Smart scalar OIDs use scalar instance `.0`; the entitlement table remains modeled as a table.
  - Cisco traditional identity derives the three-component MIB index instead of feature-name-only identity.
  - Blue Coat derives `appLicenseStatusIndex` from the row index.
  - Check Point licensing follows the refreshed `svnLicensing` table.
  - MikroTik pre-1971 sentinel filtering happens at typed producer time, not by filename.
- Final review fixes:
  - Licensing errors are best-effort relative to regular scalar/table metrics.
  - Scalar licensing OIDs honor the shared exact missing-OID cache before future GETs.
  - Workstation-local fixture provenance paths were removed.
  - Public metadata wording no longer uses PR-specific branch coverage phrasing.
  - Global integration template edits were reverted to avoid a repo-wide generated-doc inconsistency.

Tests or equivalent validation:

- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestParseSNMPWalkLine_IntegerEnumValue|TestCollector_Collect_CheckPointLicensingProfile_CommunitySample|TestCollector_Collect_SophosLicensingProfile_Fixture$'` passed.
- `go test -count=1 ./collector/snmp -run 'TestFuncLicensesHandleUnavailable|TestFuncLicensesHandleUnavailableWhenNoRowsWereCollected|TestFuncLicensesHandle$'` passed, covering the existing cold-start `503` behavior.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition` passed.
- `go test -count=1 ./collector/snmp/ddsnmp` passed.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'TestProfile_MergeLicensingPreservesOriginProfileID|TestProfileDefinition|TestResolvedProfileSet_Project'` passed.
- `go test -count=1 ./collector/snmp/ddsnmp -run 'TestResolvedProfileSetProject_SeparatesMetricsAndTopology|TestResolvedProfileSetProject_DoesNotShareMutableProjectionState|TestProjectedViewFilterByKind'` passed.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_LicenseRows'` passed, covering scalar and table typed licensing row emission plus table-cache reuse.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_PreservesHiddenMetrics|TestCollector_Collect_SeparatesTopologyMetricsFromHiddenMetrics|TestCollector_Collect_LicenseRows'` passed, covering hidden/topology/licensing delivery separation.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_LicenseRows|TestCollector_Collect_StatsSnapshot'` passed, covering typed licensing rows and the normal non-licensing stats path.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run TestParseSNMPWalkLine_IntegerEnumValue` passed, compiling `ddsnmpcollector` after shared `ProfileMetrics` changes.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_(Cisco|MikroTik)LicensingProfile'` passed after clearing inherited topology rows from the hidden-protocol licensing profile helper.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_LicenseRows|TestCollector_Collect_PreservesHiddenMetrics|TestCollector_Collect_SeparatesTopologyMetricsFromHiddenMetrics|TestCollector_Collect_StatsSnapshot'` passed after the combined projection, usage-sentinel, and licensing-stats fixes.
- `go test -count=1 ./collector/snmp/ddsnmp` passed after replacing the one-off combined projection helper with variadic `Project(consumer, consumers...)`.
- `go test -count=1 ./collector/snmp/ddsnmp/... ./collector/snmp/... ./collector/snmp_topology/...` passed after replacing the one-off combined projection helper with variadic `Project(consumer, consumers...)`.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition` passed after adding the licensing forbid-list, state-source, and timer-source validation gates.
- `go test -count=1 ./collector/snmp/ddsnmp` passed after the variadic projection and licensing merge-override fixes.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector` passed after the cross-table licensing dependency walk, missing-OID, cache-namespace, sentinel-matrix, and licensing stat fixes.
- `go test -count=1 ./collector/snmp/ddsnmp/... ./collector/snmp/... ./collector/snmp_topology/...` passed after the full post-review fix batch.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector` passed.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed.
- `go test -count=1 ./collector/snmp/...` passed.
- `go test -count=1 ./collector/snmp_topology/...` passed, covering shared profile projection changes against the topology consumer.
- `go test -count=1 ./collector/snmp` passed after replacing the licensing consumer's hidden-metric extraction with typed `ProfileMetrics.LicenseRows`, adding the `informational` bucket, and rewriting SNMP collector aggregation/function tests around typed rows.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector` passed after migrating MikroTik, Check Point, Blue Coat, and Fortinet licensing profiles from hidden `_license_row` metrics to typed `licensing:` rows.
- `go test -count=1 ./collector/snmp/ddsnmp/... ./collector/snmp` passed after the typed consumer rewrite, typed profile migrations, and licensing documentation updates in this slice.
- `go test -count=1 ./collector/snmp/... ./collector/snmp_topology/...` passed after the typed consumer rewrite, migrated profile subset, and documentation/health updates.
- `go test -count=1 ./collector/snmp -run 'TestCollector_AddLicenseCharts|TestLicensesMethodConfig|TestFuncLicenses|TestAggregateLicenseRows|TestNormalizeLicenseStateBucket|TestExtractLicenseRows'` passed after lazy chart registration, parameterless function contract, and typed consumer cleanup.
- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestValidateEnrichProfile_Licensing|TestCollector_Collect_LicensingProfileFixtures|TestCollector_Collect_LicensingProfiles|TestCollector_Collect_Sophos|TestCollector_Collect_MikroTik|TestCollector_Collect_Cisco'` passed after licensing validation hardening and fixture test consolidation.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed after the final pre-PR review fix batch.
- `go test -count=1 ./collector/snmp/...` passed after the final pre-PR review fix batch.
- `go test -count=1 ./collector/snmp_topology/...` passed after the shared projection/spec cleanup.
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector -run 'TestCollector_Collect_LicenseRowsBestEffortForRegularMetrics|TestCollector_Collect_LicenseRowsSkipsKnownMissingScalarOIDs'` passed after the final review runtime fixes.
- `go test -count=1 ./collector/snmp/ddsnmp/...` passed after the final review runtime and artifact fixes.
- `go test -count=1 ./collector/snmp/... ./collector/snmp_topology/...` passed after the final review runtime and artifact fixes.
- `git diff --check master...HEAD` passed during final review.

Real-use evidence:

- No live SNMP device validation was run in this SOW. Licensing profile behavior is validated through typed collector mocks, public fixture-derived SNMP data, full-profile loading tests, and MIB-derived OID checks. This is acceptable for the WIP/nightly feature because the target devices are not locally available and all changed runtime surfaces are covered by narrow Go suites.

Reviewer findings:

- Latest Claude implementation-slice review disposition:
  - accepted/fixed P0 merge override, missing-OID poisoning, and licensing cross-table dependency wiring before consumer rewrite;
  - accepted/fixed P1 validation/runtime gaps for forbidden licensing symbol transforms, state policy source, timer source ambiguity, cache namespace, licensing timing/error counters, and structural helper reuse;
  - accepted/documented separate stats semantics for typed license rows versus ordinary chart-metric table rows;
  - rejected scalar `from:` as an additional blocker because scalar `from:` is scoped to the scalar licensing row/group by construction, while table `from:` remains explicitly same-table validated.
- Final GPT-5.5 review disposition:
  - accepted/fixed licensing errors dropping regular metrics by making typed licensing best-effort in `collectProfile`;
  - accepted/fixed scalar licensing missing-OID cache bypass by filtering known missing scalar licensing OIDs before future GETs;
  - accepted/fixed fixture workstation-path leakage by sanitizing licensing fixture headers;
  - accepted/fixed stale public docs wording in `metadata.yaml`;
  - accepted/fixed projection spec example severity mappings to use runtime-valid `"0"`, `"1"`, `"2"` values;
  - accepted/fixed generated-doc/template inconsistency by reverting unrelated global integration template edits;
  - accepted/tracked Cisco unsupported table-root no-such caching as `.agents/sow/pending/SOW-0014-20260507-snmp-licensing-unsupported-table-cache.md`.

Same-failure scan:

- Same-failure search for workstation-local provenance paths and PR-specific branch coverage phrasing returned no matches in metadata, licensing fixtures, projection specs, and integration templates.
- `rg -n "_license_row|_license_value_kind|licenseDateFromTag|tagLicense|licenseValueKind|licenseSourceMetricName|mergeLicenseSignal|mergeLicenseTags|licenseRowMergeKey" collector/snmp config/go.d/snmp.profiles/default` shows only intentional validation-test literals and generic `HiddenMetrics` canary references, not production licensing consumption.
- `git status --short` was checked; raw MIB files were removed from repo root and the local TODO remained untracked.

Sensitive data gate:

- Durable artifacts contain no raw SNMP communities, SNMPv3 credentials, bearer tokens, customer names, customer hostnames, customer IPs, or raw MIB content.
- Workstation-local fixture provenance comments were sanitized.
- The four downloaded raw MIB files were deleted from repo root before close-out.
- `src/go/plugin/go.d/TODO-snmp-licensing-monitoring-review.md` remains local/untracked and must not be staged.

Artifact maintenance gate:

- AGENTS.md: no update needed; existing SOW, collector, sensitive-data, and follow-up discipline rules already covered this work.
- Runtime project skills: updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md` with typed licensing authoring rules and table-driven test preference.
- Specs: updated `.agents/sow/specs/snmp-profile-projection.md` with licensing consumer, projection, typed delivery, identity, validation, and metadata tag behavior.
- End-user/operator docs: updated `collector/snmp/profile-format.md`, `collector/snmp/metadata.yaml`, `collector/snmp/integrations/snmp_devices.md`, and `src/health/health.d/snmp.conf` as the SNMP licensing feature became user-visible.
- End-user/operator skills: no update needed; no public Netdata AI skill currently documents SNMP profile licensing authoring or SNMP licensing runtime use.
- SOW lifecycle: SOW-0013 status set to `completed`; file will move to `.agents/sow/done/` with the implementation and follow-up SOW in the same commit.

Specs update:

- Updated `.agents/sow/specs/snmp-profile-projection.md`.

Project skills update:

- Updated `.agents/skills/project-snmp-profiles-authoring/SKILL.md`.

End-user/operator docs update:

- Updated:
  - `collector/snmp/profile-format.md`
  - `collector/snmp/metadata.yaml`
  - `collector/snmp/integrations/snmp_devices.md`
  - `src/health/health.d/snmp.conf`

End-user/operator skills update:

- No update needed; no end-user/operator skill exposes this SNMP licensing schema or function workflow.

Lessons:

- Hidden-metric side channels become hard to validate as soon as one logical row is reconstructed from several scalar/table fragments. Topology and licensing now share the cleaner pattern: schema-owned typed sections plus typed `ProfileMetrics` outputs.
- Optional feature telemetry must be best-effort relative to ordinary device metrics; otherwise a WIP optional section can regress established collection.

Follow-up mapping:

- Implemented in this SOW:
  - typed licensing projection and consumer rewrite;
  - profile migrations and MIB-derived corrections;
  - hidden-protocol removal from production licensing;
  - validation/test/docs/spec/skill updates;
  - final review runtime and artifact fixes.
- Tracked as follow-up:
  - `.agents/sow/pending/SOW-0014-20260507-snmp-licensing-unsupported-table-cache.md` tracks explicit unsupported licensing table-root no-such caching for broad Cisco coverage.

## Outcome

Completed. SNMP licensing is now represented by top-level typed `licensing:` profile rows and delivered through `ProfileMetrics.LicenseRows`; the SNMP collector consumes typed rows directly for charts, health inputs, and the `snmp:licenses` function. The old hidden `_license_row*` / `_license_value_kind` protocol is removed from production licensing code and profile YAML.

## Lessons Extracted

Typed profile projections are the right boundary for non-chart SNMP observations. The collector can still reuse scalar/table collection internals, but row identity, validation, and consumer contracts need to live in schema-owned typed fields, not hidden metric names or string tag protocols.

## Followup

- `.agents/sow/pending/SOW-0014-20260507-snmp-licensing-unsupported-table-cache.md`

## Regression Log

None yet.
