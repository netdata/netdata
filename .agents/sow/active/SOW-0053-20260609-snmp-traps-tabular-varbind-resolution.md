# SOW-0053-20260609-snmp-traps-tabular-varbind-resolution - linkUp/linkDown `<missing>` from instance-suffixed varbinds

## Status

Status: in-progress

Sub-state: shared column-instance varbind resolution implemented and unit validated.

## Requirements

### Purpose

Fix the SNMP-traps collector so the standard IF-MIB linkUp/linkDown message,
structured varbind metadata, dedup keys, and any template/metric that references
a tabular varbind resolve the real interface and operational state instead of
`<missing>`. Discovered during real-device lab validation (the lab side of
SOW-0046).

Expanded purpose on 2026-06-10:

- Fix the remaining template-format class that can still render `<missing>` when
  real devices omit optional varbinds.
- Make the full-corpus LLM reclassification safe to run by switching generated
  descriptions to a restricted Go `text/template` syntax with explicit fallback
  behavior.
- Preserve job-creation failure semantics: invalid profile templates must fail
  while profiles are loaded, not after the listener appears to be running.

### User Request

"Open an SOW to it, in order to fix the bug" — the linkUp/linkDown trap message
renders `Link <missing> operational state changed to <missing>` for real devices.

### Assistant Understanding

Facts:

- Live lab capture of a Zyxel XS1930 linkDown trap:
  varbinds were textbook-correct — `1.3.6.1.2.1.2.2.1.1.1=1` (ifIndex.1),
  `...2.2.1.7.1=1` (ifAdminStatus.1), `...2.2.1.8.1=2`
  (ifOperStatus.1=down). The collector even enriched `TRAP_INTERFACE=swp00` and
  LLDP `TRAP_NEIGHBORS`, yet the rendered MESSAGE was still
  `Link <missing> operational state changed to <missing>`.
- Template (`standard.yaml:12294-12313`): `Link {ifIndex} operational state changed
  to {ifOperStatus} on {_HOSTNAME}.`, varbinds `[ifIndex, ifAdminStatus, ifOperStatus]`.
- Profile varbind OIDs are **column** OIDs without an instance: `ifIndex`
  `1.3.6.1.2.1.2.2.1.1` (`standard.yaml:1519-1520`), `ifAdminStatus`
  `…2.2.1.7` (`:1508-1509`), `ifOperStatus` `…2.2.1.8` (`:1531-1532`).
- Resolution uses **exact** OID equality: `resolver.go:176` (`resolveVarbindValue`)
  and `resolver.go:194` (`resolveRawVarbindByOID`) both do `if v.OID == oid`. A real
  PDU varbind `…1.1.1` never equals the column OID `…1.1`, so it returns `<missing>`.
- The same exact-match flaw exists on the metrics path:
  `operator_metric.go:228` (`resolveMetricDimValue`, `if v.OID == vb.OID`).
- The same exact-match flaw exists earlier in TrapEntry construction:
  `resolver.go:257` (`resolve2TierVarbind`, `td.varbindByOID(oid)`). If this is
  not fixed, entry varbinds keep raw OID keys instead of profile symbolic names
  and enums, so `TRAP_JSON` remains less useful and symbolic dedup keys can miss.
- A related exact-match flaw exists in dedup key lookup: `dedup.go:376` matches
  `vb.Name == name` or an exact numeric OID. Symbolic dedup keys depend on
  `resolve2TierVarbind` setting `Name`; numeric column keys also need the same
  column-instance matching.
- The profile-format documentation currently says "Varbind OIDs are matched
  exactly"; that becomes false once column-instance matching is implemented and
  must be updated with the new exact-first/column-instance rule.

Inferences:

- This affects EVERY standard linkUp/linkDown trap (and any template, metric,
  dedup key, or `TRAP_JSON` symbolic varbind metadata bound to a tabular/columnar
  varbind), since SMI table cells are always sent with an instance suffix.
  linkUp/linkDown is the most common trap type, so the impact is broad.
- Existing unit tests passed because their fixtures bind varbinds at the exact profile
  OID (no instance suffix), masking the real-PDU shape. Real-device validation
  (SOW-0046) is what surfaced it.

Unknowns:

- Whether any current profile intentionally encodes a full instance OID (handled
  safely by keeping exact-match-first).

### Acceptance Criteria

- A linkDown/linkUp trap whose varbinds carry instance suffixes renders
  `Link <ifIndex> operational state changed to <ifOperStatus-label>` (no `<missing>`).
  Verification: unit test built from the captured XS1930 varbinds + re-trigger in lab
  and read the journal MESSAGE.
- `{var.raw}` references and metric `dimension_from_varbind` resolve the same tabular
  varbinds. Verification: focused unit tests on `resolveRawVarbindByOID` and
  `resolveMetricDimValue`.
- `TrapEntry.Varbinds` are enriched with profile symbolic names and enum labels for
  instance-suffixed table cells, so `TRAP_JSON` keys remain profile-readable.
  Verification: focused unit test on `resolve2TierVarbind` and serialization.
- Dedup key varbinds resolve instance-suffixed table cells for symbolic keys and
  numeric column OIDs. Verification: focused dedup fingerprint/admission tests.
- Exact-OID profiles (instance already in the OID) keep working. Verification:
  existing resolver/metric tests stay green.
- No regression to scalar varbinds (e.g. `.0`). Verification: existing tests + a
  scalar case.
- New generated descriptions use restricted Go-template actions:
  `{{hostname}}`, `{{source_ip}}`, `{{trap_name}}`, `{{vendor}}`,
  `{{trap_interface}}`, `{{trap_neighbors}}`, `{{value "varbindName"}}`,
  `{{raw "varbindName"}}`, `{{first ...}}`, and `with`/`else`/`end`.
- Known but absent varbinds in the new template syntax render as empty strings or
  activate the template fallback, never as `<missing>`.
- Unknown functions, unknown varbind names, malformed templates, and forbidden
  template actions fail during profile load / job creation.
- Existing legacy `{var}` descriptions continue to render during the transition,
  but the classifier prompt and prompt version force newly classified profiles to
  use `{{...}}`.
- The classifier prompt tells the model to treat trap varbinds as optional unless
  a standard proves otherwise, and to derive messages from trap identity when the
  trap itself gives the state, such as linkUp/linkDown.
- The full-corpus LLM classification is started only after the runtime and
  generator tests for the template syntax pass.
- Before any full-corpus LLM batch starts, the exact classifier system prompt and
  3-5 representative user prompts are shown to the user for review.
- A 3-5 trap pilot classification run is executed with a separate scratch
  cache/output before the full overnight run. The full run starts only after the
  pilot output is inspected.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/snmp_traps/resolver.go` (170-199, 252-265),
  `operator_metric.go` (218-233), `dedup.go` (319-380), `serialize.go`
  (168-190, 497-510), `standard.yaml` (1508-1532, 12294-12313), and
  `profile-format.md` (191-199).
- Live journal capture under the local Netdata trap cache for lab devices.

Current state:

- The original SOW listed three exact-match call sites; the complete affected
  set is larger:
  - template display resolution: `resolver.go:176`;
  - raw numeric-OID resolution: `resolver.go:194`;
  - TrapEntry/profile metadata resolution: `resolver.go:257`;
  - operator metric dimension resolution: `operator_metric.go:228`;
  - dedup key varbind lookup for numeric column OIDs and symbolic keys that were
    not profile-resolved into `TrapEntry.Varbinds`: `dedup.go:376`.
- Tabular varbinds currently miss all these paths unless test fixtures use exact
  column OIDs without instance suffixes.

Risks:

- Columnar fallback could match the wrong instance when a PDU carries multiple
  instances of the same column (observed in lab: one D-Link trap dumped
  `ifIndex.1..8` with no `ifOperStatus`). Mitigation under Decision 3.
- Profile metadata lookup from observed PDU OID to profile varbind definition
  must not use Go map iteration "first match"; it must choose the longest
  matching profile column OID after exact match to avoid shorter-prefix matches.
- Updating runtime behavior without updating `profile-format.md` would leave a
  false authoring contract in committed docs.

## Pre-Implementation Gate

Status: in-progress

Problem / root-cause model:

- Template/metric varbind resolution matches the profile **column** OID by exact
  equality against PDU varbind OIDs that always carry an **instance** suffix → no
  match → `<missing>`. Evidence: `resolver.go:176,194`, `operator_metric.go:228`;
  template/varbind OIDs in `standard.yaml`; live lab capture.
- TrapEntry varbind metadata resolution has the same exact-match problem before
  rendering/serialization. Evidence: `resolver.go:257`; `serialize.go:172-190`
  and `serialize.go:497-510` use `VarbindValue.Name` for `TRAP_JSON` keys.
- Dedup key lookup depends on `VarbindValue.Name` for symbolic keys and exact OID
  for numeric keys. Evidence: `dedup.go:326-337` and `dedup.go:371-380`.

Evidence reviewed:

- Code call sites and profile definitions above; captured real-device varbinds
  for `lab-device-a` (Zyxel XS1930) and `lab-device-b` (D-Link).

Affected contracts and surfaces:

- `resolver.go` (`resolveVarbindValue`, `resolveRawVarbindByOID`,
  `resolve2TierVarbind`), `operator_metric.go` (`resolveMetricDimValue`),
  `dedup.go` (`dedupVarbind`), and profile-format docs.
- Rendered MESSAGE, `{var.raw}` output, metric dimension labels, `TRAP_JSON`
  symbolic varbind keys/enums, and dedup fingerprints.
- No schema change. Journal field names do not change, but `TRAP_JSON` keys for
  resolved tabular varbinds improve from raw OIDs to profile symbolic names.

Clean-end-state target:

- Shared varbind OID matching helpers used by every affected path:
  - profile-column OID → observed PDU varbind: exact match first, then first PDU
    varbind whose OID is under `profileOID + "."`;
  - observed PDU OID → profile varbind definition: exact match first, then the
    longest profile column OID that prefixes the observed OID as `column + "."`.
- No duplicated matching logic left behind.
- Removed as redundant (i): the open-coded `v.OID == oid`, `v.OID == vb.OID`,
  exact `td.varbindByOID(observedOID)`, and exact numeric dedup OID checks,
  replaced by the shared helpers.
- Excluded coupled items (ii): SMI index parsing / per-instance varbind correlation
  (multi-row traps) — larger feature, not required to fix linkUp/linkDown; tracked as
  a follow-up if needed.
- Reference search:
  - `rg -n 'v\.OID ==|td\.varbindByOID|dedupVarbind|resolve2TierVarbind|Varbinds:'
    src/go/plugin/go.d/collector/snmp_traps -g '*.go'`
  - Complete in-scope matches are the five runtime paths listed under Current
    state. Remaining matches are construction/tests/serialization consumers that
    do not perform profile-column matching.

Existing patterns to reuse:

- `td.varbindByName`/`varbindByOID` lookups already in `resolver.go`; `isNumericOID`;
  existing `*_test.go` table-test style for resolver/metric.
- Go standard library `text/template` for the new description syntax, with a
  small approved function map and explicit parse-tree validation.

Risk and blast radius:

- Moderate-low. Behavior mostly changes for OIDs that currently return
  `<missing>` or raw OID-keyed JSON, but `TRAP_JSON` keys can become symbolic for
  tabular varbinds. Exact-match-first preserves existing exact matches.
- Multi-instance traps remain limited to first-observed matching PDU varbind for
  display/raw/metric resolution; full row correlation is excluded and tracked.
- Template syntax change risk is moderate because it touches profile load,
  rendered MESSAGE, label rendering, generator validation, and prompt cache
  invalidation. Mitigation: keep legacy `{...}` as a migration fallback, but make
  new LLM output validate only under the restricted `{{...}}` grammar.
- Runtime concurrency risk: parsed templates are shared by loaded profiles, while
  function closures need per-trap data. Mitigation: parse/validate at load time,
  clone the parsed template at render time, and attach per-render functions to
  the clone.

Sensitive data handling plan:

- Evidence uses lab device aliases and model names only. No communities, keys,
  hostnames, or customer data in this SOW or tests. Test fixtures use
  synthetic/lab-derived varbind OIDs and values only.

Implementation plan:

1. Add shared OID helpers:
   - `oidMatchesColumn(profileOID, observedOID) bool`;
   - `findVarbindForProfileOID(entry, profileOID) (VarbindValue, bool)`;
   - `findVarbindDefForObservedOID(td, observedOID) *VarbindDef`.
2. Route `resolveVarbindValue`, `resolveRawVarbindByOID`,
   `resolveMetricDimValue`, `resolve2TierVarbind`, and dedup numeric/symbolic
   key lookup through the helpers.
3. Tests: new table tests built from the captured XS1930 linkDown varbinds
   (expect `Link 1 operational state changed to down`), raw-reference case,
   `TRAP_JSON` symbolic/enumerated varbind case, dedup key case, metric-dimension
   case, exact-OID and scalar `.0` regression cases, and longest-prefix metadata
   selection.
4. Update `profile-format.md` to replace the "Varbind OIDs are matched exactly"
   statement with the exact-first/column-instance matching rule. Update
   `metadata.yaml`/integrations doc only if rendered-example text changes.
5. Add restricted Go-template support:
   - compile and validate `{{...}}` descriptions at profile load;
   - render known-but-absent varbinds as empty strings;
   - support `first` and `with` fallbacks;
   - keep legacy `{...}` rendering for current stock/test profiles until the
     regenerated pack replaces them.
6. Update the generator prompt, validator, mechanical fallback, and prompt
   version so all new LLM classifications use the restricted `{{...}}` syntax.
7. Show the final classifier system prompt and 3-5 representative user prompts
   to the user. Do not make LLM calls before this review step.
8. Run a 3-5 trap pilot classification into an isolated scratch cache/output and
   inspect the resulting category, severity, description, and template syntax.
9. Start the full-corpus classification as a tracked background process using
   the normalized scratch `traps.jsonl`, the original source-priority roots, a
   scratch output directory, and a saved PID/log path.

Validation plan:

- From `src/go`: `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 180s` green.
- From `src/go`: `GOTOOLCHAIN=go1.26.0 go test ./cmd/snmptrapprofilegen -count=1 -timeout 180s` green.
- Focused tests must prove:
  - `{{first ...}}` and `with` fallbacks avoid `<missing>` for absent varbinds;
  - invalid `{{...}}` functions and varbind names fail at profile load;
  - generator accepts valid `{{...}}` classifier output and rejects legacy
    generated `{var}` output;
  - prompt version changed so stale cached descriptions are rejected.
- Pilot validation must include 3-5 representative traps, including at least:
  - a linkUp/linkDown-style state trap where the event identity provides the
    state;
  - a trap with useful optional varbind context and fallback;
  - a trap with no useful varbinds.
- Real-use: re-trigger a link event on XS1930 `.3` in the lab and confirm the journal
  MESSAGE resolves (captured via `journalctl --file=...`).
- Same-failure scan: re-grep for remaining exact profile/PDU varbind OID matches
  (`v.OID ==`, `td.varbindByOID(observedOID)`, dedup exact numeric OID checks).

Artifact impact plan:

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` if it documents varbind
  matching/rendering contract; otherwise note no change.
- End-user/operator docs: update `profile-format.md` for varbind OID matching.
  Update `integrations/snmp_trap_listener.md` example only if the rendered text
  shown there changes.
- End-user/operator skills: `query-snmp-traps` unaffected (query path unchanged).
- SOW lifecycle: branch-local working file under `active/`; delete before merge;
  durable knowledge lands in code + tests.

Open-source reference evidence:

- `ktsaou/netdata @ 6a7f0666188a` (branch `snmptraps`):
  `src/go/plugin/go.d/collector/snmp_traps/resolver.go:174-199,252-265`,
  `src/go/plugin/go.d/collector/snmp_traps/operator_metric.go:218-233`,
  `src/go/plugin/go.d/collector/snmp_traps/dedup.go:319-380`,
  `src/go/plugin/go.d/collector/snmp_traps/serialize.go:168-190,497-510`,
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/standard.yaml:1508-1532,12294-12313`.

Open decisions:

- D1 matching strategy, D2 scope, D3 multi-instance behavior — approved by user
  after red-test proof. Implement Decision 1A, full affected runtime scope, and
  first matching instance for multi-instance column fallback.
- D4 template syntax — approved by user direction on 2026-06-10. Implement the
  restricted Go `text/template` target recorded in the full-corpus SOW, keep
  legacy `{...}` as a transition fallback, and force new LLM output through the
  new `{{...}}` grammar.
- D5 prompt shape — approved by user direction on 2026-06-10. Keep the system
  prompt concise because the local model already understands Go
  `text/template`. The prompt should focus on Netdata's restricted function
  surface, validation constraints, and the SNMP-specific rule that optional
  varbinds must be used carefully and must not substitute the trap's own
  meaning.
- D6 pilot/full-run local model settings — approved by user direction on
  2026-06-10. Use the OpenAI-compatible endpoint
  `http://localhost:8356/v1`, model `qwen3.6-35b-a3b-nothinker`, and include
  `extra_body.chat_template_kwargs.enable_thinking=false` in LLM requests.
  The generator may also keep the direct `chat_template_kwargs` field for raw
  HTTP server compatibility, but the requested `extra_body` shape must be
  present.
- D7 full-run concurrency — approved by user direction on 2026-06-10. The local
  model can handle 16 concurrent requests, so the full overnight batch should be
  started with `--concurrency 24` to keep the model queue fed. This is a run
  setting, not a change to the shipped helper's default concurrency.

Test-first decision:

- User requested failing tests before any production-code fix.
- This SOW will first add focused failing tests that prove each confirmed bug
  class. Production code must remain unchanged until the failing evidence is
  captured and reported.
- After implementation, the same tests must pass without weakening assertions.

Test-first evidence captured:

- Added focused red tests:
  - `src/go/plugin/go.d/collector/snmp_traps/profile_test.go:891`
    proves linkDown MESSAGE currently renders `<missing>` for tabular varbind
    instances.
  - `src/go/plugin/go.d/collector/snmp_traps/profile_test.go:983`
    proves `{ifOperStatus.raw}` currently resolves to `<missing>`.
  - `src/go/plugin/go.d/collector/snmp_traps/profile_test.go:1373`
    proves `resolve2TierVarbind` does not attach profile name/enum metadata to
    instance-suffixed varbinds.
  - `src/go/plugin/go.d/collector/snmp_traps/serialize_test.go:268`
    proves `TRAP_JSON` currently keeps raw instance OID keys instead of profile
    varbind names.
  - `src/go/plugin/go.d/collector/snmp_traps/operator_metric_test.go:500`
    proves operator metrics count tabular varbind dimensions as `<missing>`.
  - `src/go/plugin/go.d/collector/snmp_traps/dedup_test.go:157`
    proves numeric column-OID dedup keys suppress distinct interface instances.
- Focused command run from `src/go`:
  `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run 'Test(RenderMessageResolvesTabularVarbindInstances|ResolveVarbindRawResolvesTabularVarbindInstance|Resolve2TierResolvesTabularVarbindInstance|SerializeToJournalFieldsTRAPJSONUsesProfileNamesForTabularVarbindInstances|OperatorMetricsIncDimensionFromTabularVarbindInstance|TrapDeduperNumericColumnOIDKeyVarbindNarrowFingerprint)$' -count=1 -timeout 180s`
- Result: expected failure. Failure classes matched the root-cause model:
  MESSAGE `<missing>`, raw varbind `<missing>`, missing profile name/enum,
  raw `TRAP_JSON` instance-OID key, metric `<missing>` bucket, and dedup
  suppression of a distinct column instance.

## Implications And Decisions

Approved decisions:

1. **Matching strategy** —
   - **(A, recommended)** Exact match first, then columnar fallback
     `HasPrefix(v.OID, profileOID+".")` returning the first observed PDU
     instance for profileOID-to-PDU lookup, and longest matching profile column
     for observed-PDU-to-profile lookup. Surgical; fixes tabular varbinds;
     preserves exact-OID profiles and scalars (`.0`).
   - (B) Require profiles to encode instance OIDs — wrong; instances are dynamic.
   - (C) Full SMI table/index parsing with per-instance correlation — correct long-term
     but over-engineered for this fix.
2. **Scope** —
   - **(recommended)** Fix all affected runtime paths via shared helpers
     (`resolver.go:176`, `resolver.go:194`, `resolver.go:257`,
     `operator_metric.go:228`, `dedup.go:376`) — honors the same-failure rule
     and leaves no duplicate logic.
   - (alt) Message path only — leaves metric, raw, `TRAP_JSON`, and dedup paths
     silently broken.
3. **Multiple instances of one column in a PDU** (e.g. D-Link `.69` dumping
   `ifIndex.1..8`) —
   - **(recommended)** Use the first matching instance and document the limitation;
     proper multi-row correlation is a separate follow-up.
   - (alt) Join all instance values — noisier, rarely desired.

## Plan

1. Shared helpers + route all affected runtime paths (Decision 1A, 2).
2. Tests from captured lab varbinds + regressions.
3. `profile-format.md` update, plus doc/example touch-ups if rendered text changes.
4. Lab re-trigger validation; same-failure scan; close out.

## Execution Log

### 2026-06-09

- SOW created. Root cause and call sites confirmed; live XS1930/D-Link evidence
  captured. Awaiting user approval of D1/D2/D3 before editing code.
- SOW audited before implementation. Found the original affected-surface list was
  incomplete: `resolve2TierVarbind`, `TRAP_JSON` symbolic/enumerated varbind
  metadata, dedup key lookup, profile-format docs, and the Go validation command
  were missing or incomplete. Sanitized local hostname/IP evidence into lab
  aliases. Still awaiting user approval of D1/D2/D3 before editing code.
- User requested a test-first reproduction phase before any fix. Proceeding with
  tests only; production-code changes remain blocked until failing test output is
  captured and reviewed.
- User approved fixing the bug after the focused regression tests failed as
  expected. Implementation started with Decision 1A, full affected runtime
  scope, and first-instance column fallback.
- Implemented shared exact-first/profile-column OID matching helpers and routed
  MESSAGE/raw resolution, TrapEntry metadata resolution, operator metrics, and
  numeric dedup key lookup through them. Updated `profile-format.md` and the
  SNMP traps design spec to document exact-first plus `profile_oid + "."`
  column-instance matching.
- Live lab validation found a second `<missing>` class after the column-instance
  fix: a D-Link switch emitted standard `IF-MIB::linkDown` with `ifIndex.1`
  only, omitting `ifAdminStatus.1` and `ifOperStatus.1`. This proves the
  remaining `<missing>` is not resolver matching; the stock IF-MIB profile text
  depends on a varbind that real devices may omit. RFC 2863 also says the
  `linkDown` `ifOperStatus` object is the previous/other state, not necessarily
  the new down state, so `changed to {ifOperStatus}` is misleading even for
  compliant PDUs that include it.
- Updated plan for the remaining bug:
  - add a stock-profile regression test with a real-device-shaped `linkDown`
    PDU carrying only `ifIndex`;
  - change the stock IF-MIB `linkDown`/`linkUp` descriptions to event wording
    that does not depend on `ifOperStatus`;
  - update the profile generator prompt so future regenerations do not teach
    the classifier to render link-down messages as `changed to {ifOperStatus}`;
  - rerun focused stock-profile and generator tests, then the full SNMP traps
    package tests.

## Validation

Acceptance criteria evidence:

- MESSAGE, raw reference, `TRAP_JSON`, metric dimension, and numeric dedup
  regression tests now pass with the same assertions that failed before the fix.
- Exact-match precedence, scalar `.0` fallback, and longest-prefix profile
  metadata selection have dedicated regression tests.
- Reviewer-suggested hardening tests cover OID arc-boundary false-prefix
  prevention and the approved first-matching-instance behavior.
- Reference scan confirmed old exact-only runtime paths were removed. Remaining
  exact checks are inside the shared exact-first helpers.

Tests or equivalent validation:

- Added and first ran a red stock-profile regression test:
  `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run TestStockIFMIBLinkMessagesDoNotDependOnIfOperStatus -count=1 -timeout 180s`
  Result before the fix: failed with `Link 1 operational state changed to <missing>
  on lab-switch.` for both `linkDown` and `linkUp` when the PDU carried only
  `ifIndex.1`.
- After updating the stock IF-MIB profile descriptions and generator prompt, the
  same focused command passed. The rendered messages are:
  `Link 1 went down on lab-switch.` and `Link 1 came up on lab-switch.`
- From `src/go`, focused regression command after implementation:
  `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run 'Test(RenderMessageResolvesTabularVarbindInstances|ResolveVarbindRawResolvesTabularVarbindInstance|Resolve2TierResolvesTabularVarbindInstance|SerializeToJournalFieldsTRAPJSONUsesProfileNamesForTabularVarbindInstances|OperatorMetricsIncDimensionFromTabularVarbindInstance|TrapDeduperNumericColumnOIDKeyVarbindNarrowFingerprint|FindVarbindForProfileOIDExactMatchWins|OIDMatchesColumnRequiresArcBoundary|FindVarbindForProfileOIDFirstMatchingInstanceWins|FindVarbindForProfileOIDMatchesScalarZeroInstance|FindVarbindDefForObservedOIDUsesLongestColumnPrefix)$' -count=1 -timeout 180s`
  Result: passed.
- From `src/go`: `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -count=1 -timeout 180s`
  Result: passed.
- From `src/go`: `GOTOOLCHAIN=go1.26.0 go test ./cmd/snmptrapprofilegen -count=1 -timeout 180s`
  Result: passed.
- From `src/go`: `GOTOOLCHAIN=go1.26.0 go test ./cmd/godplugin ./cmd/snmptrapprofilegen ./plugin/agent/jobmgr ./plugin/agent/jobmgr/funcctl ./plugin/go.d/collector/snmp_traps ./plugin/ibm.d/modules/as400 -count=1 -timeout 180s`
  Result: passed.
- From `src/go`: `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -race -count=1 -timeout 180s`
  Result: passed.
- Same-failure scan:
  `rg -n 'v\.OID ==|td\.varbindByOID|dedupVarbind|resolve2TierVarbind|findVarbindForProfileOID|findVarbindDefForObservedOID' src/go/plugin/go.d/collector/snmp_traps -g '*.go'`
  Result: remaining exact matches are only inside `findVarbindForProfileOID` and
  `findVarbindDefForObservedOID`; runtime callers use the helpers.

Real-use evidence:

- Pending re-trigger on real lab hardware; original failing capture recorded
  above.

Reviewer findings:

- `glm-5.1`: production-grade; no blocking findings. Suggested adding an
  explicit OID arc-boundary false-prefix test. Implemented.
- `minimax-m3-coder`: approve for production; no blocking findings. Ran package
  tests including `-race` during review. Suggested optional first-instance
  contract test and noted equal-length prefix collision as theoretical. The
  first-instance test was implemented. Equal-length distinct prefixes cannot
  both prefix the same observed OID at the same length unless identical, so no
  production-code change was made.
- `kimi-k2.6`: unavailable due quota.
- `qwen3.7-plus`: rerun started after test hardening, but the reviewer looped on
  repeated path inspection without a final verdict; stopped the exact qwen
  process after no actionable finding was produced.

Same-failure scan:

- Completed. Runtime callers use the shared helpers; exact matches remain only
  inside the helpers for exact-first precedence.

Sensitive data gate:

- Passed. Tests use documentation-range IPs and standard MIB OIDs only. SOW
  evidence uses lab aliases and model names only; no communities, hostnames,
  credentials, customer data, or personal data were added.

## Artifact Maintenance Gate

- AGENTS.md: no change.
- Runtime project skills: no change.
- Specs: updated `.agents/sow/specs/snmp-traps/netdata.md` to document
  exact-first plus `profile_oid + "."` varbind matching.
- End-user/operator docs: updated `profile-format.md` with the same runtime
  varbind matching contract.
- End-user/operator skills: no change; query surface is unchanged.
- SOW lifecycle: active branch-local SOW remains in-progress for review/handoff
  evidence; delete before merge.

Specs update:

- Completed: `.agents/sow/specs/snmp-traps/netdata.md`.

Project skills update:

- Not needed.

End-user/operator docs update:

- Completed:
  `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.

End-user/operator skills update:

- Not needed.

Lessons:

- Unit-test fixtures that bind varbinds at the exact profile OID hid a real-PDU shape
  mismatch; tabular-varbind tests must include instance suffixes.

Follow-up mapping:

- Full SMI table/index correlation for traps carrying multiple instances of the
  same column remains outside this bug fix. This SOW keeps the approved
  first-matching-instance behavior.

LLM pilot evidence on 2026-06-10:

- Prompt preview generated before pilot:
  `/tmp/snmp-trap-llm-prompt-preview-20260610-005515.txt`.
- Pilot input/output directory:
  `/tmp/snmp-trap-llm-pilot-20260610-005929`.
- First pilot attempt failed before writing classifications due a local
  generator panic in template validation. Root cause: Go `text/template` uses a
  typed-nil `*parse.ListNode` for a `with` block without an `else`, and both the
  generator and runtime profile validators recursed into it without a nil
  guard.
- Fix added:
  - nil guards for typed-nil `*parse.ListNode`, `*parse.ActionNode`,
    `*parse.WithNode`, and nested `*parse.PipeNode`;
  - generator regression test for `{{with value "ifIndex"}}...{{end}}`;
  - runtime profile load/render regression test for optional `with` text with
    no `else`.
- Focused validation after the fix:
  - `GOTOOLCHAIN=go1.26.0 go test ./cmd/snmptrapprofilegen -run 'Test(LLMResponseValidation|ClassifyOneFeedsValidationErrorBackToLLM)' -count=1 -timeout 180s`
    passed;
  - `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps -run 'TestRenderMessageGoTemplateWith(BlockWithoutElse|Fallback)|TestLoadProfileRejectsInvalidGoTemplates' -count=1 -timeout 180s`
    passed.
- Second pilot attempt succeeded with five cache misses, all accepted on
  `attempt-1`, using endpoint `http://localhost:8356/v1`, model
  `qwen3.6-35b-a3b-nothinker`, `--concurrency 5`, and `--require-llm`.
- Pilot outputs:
  - `UPS-MIB::upsTrapOnBattery`: `state_change` / `warning` /
    `UPS switched to battery power{{with value "upsEstimatedMinutesRemaining"}} with {{.}} minutes remaining{{end}} on {{hostname}}.`
  - `CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged`: `config_change` /
    `notice` /
    `Running configuration changed{{with value "ccmHistoryEventTerminalUser"}} by {{.}}{{end}} on {{hostname}}.`
  - `IF-MIB::linkDown`: `state_change` / `warning` /
    `Interface {{first (value "ifIndex") "link"}} went down on {{hostname}}.`
  - `IF-MIB::linkUp`: `state_change` / `notice` /
    `Interface {{first (value "ifIndex") "link"}} went up on {{hostname}}.`
  - `SNMPv2-MIB::authenticationFailure`: `auth` / `warning` /
    `SNMP authentication failure from {{source_ip}} on {{hostname}}.`
- User approved starting the full run as-is.
- Full run started in the background:
  `/tmp/snmp-trap-full-corpus-llm-20260610-010631`.
  - PID file: `classify.pid`
  - Log: `classify.log`
  - Resumable cache: `classification-cache.jsonl`
  - Final enriched JSONL: `enriched.jsonl`
  - Command settings: model `qwen3.6-35b-a3b-nothinker`, endpoint
    `http://localhost:8356/v1`, `--concurrency 24`, `--require-llm`,
    stdin closed.
- Resume behavior: the classifier reads `classification-cache.jsonl` on start,
  validates prompt/schema/description against the current code, and only sends
  cache misses to the LLM. Successful classifications are appended and `fsync`ed
  every 50 rows, plus once at normal exit. A hard stop can lose only the last
  unflushed batch and in-flight requests; rerunning the same command without
  `--force-llm` continues from the cache.

## Outcome

Implemented and unit validated; pending external review and optional real-lab
re-trigger evidence.

## Lessons Extracted

- Resolver tests must model received PDU OIDs, not only profile column OIDs.

## Follow-up Issues

- Consider a future SOW for full per-instance row correlation if lab or user
  reports show traps with multiple rows need richer rendering than first match.

None yet.
