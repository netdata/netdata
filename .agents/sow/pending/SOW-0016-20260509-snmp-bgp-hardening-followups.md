# SOW-0016 - SNMP BGP typed projection hardening

## Status

Status: open

Sub-state: follow-up created from SOW-0015 close-out review.

## Requirements

### Purpose

Resolve the remaining correctness, validation, memory, and fixture-hardening
items found during the SNMP BGP typed projection migration without blocking the
vendor-migration commit sequence that removed the legacy `bgp_public*` runtime.

### User Request

The user accepted the typed BGP projection direction and repeatedly prioritized
a clean end state over churn minimization. During SOW-0015 review, several
items were classified as not blocking the migration commits but still requiring
explicit implementation, rejection, or tracking before SOW close-out.

### Assistant Understanding

Facts:

- SOW-0015 migrated all BGP-bearing stock profiles to typed `bgp:` rows and
  deleted the old `bgp_public*` runtime.
- SOW-0015 still leaves several hardening gaps that reviewers found after the
  core migration was already split into commits.
- SOW-0014 already tracks typed licensing + typed BGP unsupported table-root
  caching and should not absorb unrelated BGP validation or cache-retention
  work.

Inferences:

- These items are coherent as a post-migration hardening SOW because they are
  about validation guarantees, pathological runtime behavior, and fixture
  confidence, not about the migration architecture itself.
- Keeping them in one pending SOW prevents vague "later" debt while preserving
  one-SOW-at-a-time execution discipline.

Unknowns:

- Nokia/TiMOS high-numbered oper OIDs `.177-.188` require a current public
  SR OS MIB, upstream profile comparison, or real device walk before they can
  be confirmed or corrected.

### Acceptance Criteria

- Typed BGP cross-table `table:` references are validated against available
  table evidence or explicitly documented when the source table is inferred
  from the value OID rather than a declared row table.
- BGP state `partial: true` behavior is made internally consistent between
  spec, docs, and validator tests.
- BGP value configs reject or clearly document invalid combinations of
  `index`, `index_from_end`, and `index_transform`.
- `index_from_end` plus `format` propagation has focused test coverage.
- Permanent stale BGP cache entries are bounded or reaped after expiry.
- Cross-profile chart-key collision risk is either fixed or rejected with
  concrete evidence that the typed structural ID and current chart-key model
  are sufficient.
- Nokia/TiMOS high-numbered oper OIDs are verified against current evidence or
  corrected.
- Additional fixture/test gaps from SOW-0015 are disposed, including Dell IPv6
  typed rows and typed BGP descriptor-label handling.

## Analysis

Sources checked:

- `.agents/sow/current/SOW-0015-20260508-snmp-bgp-typed-projection.md`
- `collector/snmp/bgp_typed_metrics.go`
- `collector/snmp/collect_snmp.go`
- `collector/snmp/charts.go`
- `collector/snmp/ddsnmp/ddprofiledefinition/validation.go`
- `collector/snmp/ddsnmp/ddprofiledefinition/bgp_test.go`
- `collector/snmp/ddsnmp/ddsnmpcollector/collector_bgp.go`
- `collector/snmp/profile-format.md`
- `.agents/sow/specs/snmp-profile-projection.md`

Current state:

- BGP values can use `table:` with source OIDs; runtime can infer referenced
  table roots from cross-table value OIDs when no declared BGP row has that
  table name.
- Validation requires cross-table BGP values to have a source OID, but does not
  yet validate every referenced `table:` name against a declared table map.
- `partial: true` state mapping behavior has reviewer-noted edge cases around
  empty mappings and `partial_states`.
- Runtime precedence for multiple row-index selectors is
  `index_from_end`, then `index`, then `index_transform`; profiles should use
  one selector, but validation does not yet reject combinations.
- BGP stale cache entries can remain allocated after they are filtered from
  function output as expired.
- Public chart keys still derive from metric name plus non-underscore tag
  values; typed structural ID does not currently feed chart IDs.
- Typed BGP chart labels still use the generic SNMP convention where
  underscore-prefixed tags are excluded from chart identity and later promoted
  to visible labels. This is not the deleted legacy `bgp_public*` routing
  protocol, but it should be verified or replaced intentionally.

Risks:

- Over-tight validation could reject existing valid vendor profiles that rely
  on inferred cross-table roots.
- Changing chart-key identity can orphan chart history if done without a
  compatibility plan.
- Reaping stale cache entries too aggressively can remove useful stale function
  output before the intended stale window.
- Correcting Nokia OIDs without real evidence can regress SR OS coverage if the
  local MIB excerpt is stale rather than authoritative.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The typed BGP migration removed the major architecture smell, but some edge
  contracts were intentionally left with current-runtime behavior instead of
  stronger validation or cleanup. These need focused investigation against
  shipped profiles, tests, and MIB evidence before changing behavior.

Evidence reviewed:

- SOW-0015 close-out scans found no remaining production `bgp_public*`,
  `BGPSignalKind`, `bgpScopeAuto`, or legacy public router identifiers.
- SOW-0015 close-out scans found typed BGP still using underscore-prefixed
  descriptor tags for the generic "visible label but not chart-key" convention.
- SOW-0015 reviewer findings identified RT-4, RT3-1, SCH-2, SCH-3, SCH2-4,
  SCH2-6, NOK-1, and Dell IPv6 coverage as post-migration hardening items.

Affected contracts and surfaces:

- SNMP BGP profile validation.
- ddsnmpcollector BGP cross-table dependency resolution.
- SNMP chart identity and label behavior.
- `snmp:bgp-peers` cache lifetime and stale behavior.
- Vendor profile fixture coverage, especially Dell OS10 IPv6 and Nokia/TiMOS.
- `.agents/sow/specs/snmp-profile-projection.md` and
  `collector/snmp/profile-format.md`.

Existing patterns to reuse:

- Table-driven validation tests in `ddprofiledefinition/bgp_test.go`.
- Cross-table tag resolver logic already used by regular metric tags.
- Existing stale-cache tests under `collector/snmp/*bgp*test.go`.
- SOW-0014 unsupported table-root cache for table-root no-such handling.

Risk and blast radius:

- Runtime and validation changes can affect all BGP-capable SNMP profiles.
- Chart-key changes can affect public chart IDs and history.
- Tests should lead implementation; avoid broad refactors unless a test proves
  a real behavioral problem.

Sensitive data handling plan:

- Use synthetic PDUs, RFC 5737 / RFC 6996 test values, and public MIB/object
  names only.
- Do not commit raw MIB files or real device walks.
- If real device evidence is needed, summarize sanitized behavior only.

Implementation plan:

1. Add focused validation tests for partial-state, row-index-selector, and
   cross-table source cases.
2. Decide whether to enforce or document each validation behavior based on
   existing stock profiles.
3. Add stale-cache expiry/reaping tests and implement bounded retention if the
   tests prove unbounded memory growth.
4. Add chart-key collision coverage and decide whether to fix chart identity
   or reject with evidence.
5. Add Dell IPv6 typed BGP fixture coverage.
6. Verify Nokia/TiMOS high-numbered OIDs against current public evidence or
   correct the profile.
7. Update spec/profile-format docs and project skill guidance for any behavior
   changed.

Validation plan:

- `go test -count=1 ./collector/snmp/ddsnmp/ddprofiledefinition`
- `go test -count=1 ./collector/snmp/ddsnmp/ddsnmpcollector`
- `go test -count=1 ./collector/snmp`
- `go test -count=1 ./collector/snmp/...`
- `git diff --check`

Artifact impact plan:

- AGENTS.md: update only if the general Go test/profile authoring rule changes.
- Runtime project skills: update `project-snmp-profiles-authoring` if BGP
  authoring rules change.
- Specs: update `snmp-profile-projection.md` for validation or runtime contract
  changes.
- End-user/operator docs: update `profile-format.md` if profile syntax or
  authoring guidance changes.
- End-user/operator skills: no expected update unless public SNMP BGP operator
  workflows change.
- SOW lifecycle: keep this SOW pending until SOW-0015 is closed.

Open-source reference evidence:

- None yet. Nokia/TiMOS OID verification may require public MIB/profile
  evidence before implementation.

Open decisions:

- None for planning. Any chart identity or Nokia OID correction decision must
  be made with concrete evidence before code changes.

## Implications And Decisions

- SOW-0014 remains the owner for typed licensing + typed BGP unsupported
  table-root caching.
- This SOW owns unrelated BGP typed-projection hardening.

## Plan

1. Investigate and test validation hardening.
2. Investigate and test runtime cache/chart-key hardening.
3. Investigate and test remaining vendor fixture gaps.
4. Update specs/docs/skills according to shipped behavior.

## Execution Log

### 2026-05-09

- Created as follow-up from SOW-0015 close-out review.

## Validation

Acceptance criteria evidence:

- Pending implementation.

Tests or equivalent validation:

- Pending implementation.

Real-use evidence:

- Pending implementation.

Reviewer findings:

- Pending implementation.

Same-failure scan:

- Pending implementation.

Sensitive data gate:

- Pending implementation.

Artifact maintenance gate:

- AGENTS.md: pending implementation outcome.
- Runtime project skills: pending implementation outcome.
- Specs: pending implementation outcome.
- End-user/operator docs: pending implementation outcome.
- End-user/operator skills: pending implementation outcome.
- SOW lifecycle: pending implementation outcome.

Specs update:

- Pending implementation.

Project skills update:

- Pending implementation.

End-user/operator docs update:

- Pending implementation.

End-user/operator skills update:

- Pending implementation.

Lessons:

- Pending implementation.

Follow-up mapping:

- Pending implementation.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
