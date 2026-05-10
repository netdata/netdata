# SOW-0023 - Topology cross-payload matching

## Status

Status: open

Sub-state: opened as the structural follow-up split from SOW-0021. Do not implement until SOW-0021 defines the graph presentation contract and this SOW receives its own analysis/reviewer pass. This SOW is ordered before SOW-0022 so modal/table composition can build on the final actor correlation model.

## Requirements

### Purpose

Define how Cloud reconciles actors and links that arrive from different topology producers and may describe the same real-world entity or relationship with different identity keys.

### User Request

The user identified that the Cloud aggregator must eventually know how to match across topology payloads:

- network-connections by socket endpoint identity such as IP, port, protocol, direction, and address-space context;
- SNMP/L2 by device, MAC, interface, port, chassis, and management identity;
- streaming by machine or node identity;
- vSphere by stable inventory object identity.

The user accepted splitting this structural problem out of SOW-0021 so SOW-0021 can focus on presentation.

### Assistant Understanding

Facts:

- `netdata.topology.v1` currently has per-actor-type `identity` and `merge_identity`.
- Evidence types have `match_columns`, but those preserve relationship detail inside one payload and do not by themselves define cross-producer actor replacement.
- Different topology producers use different identity vocabularies and may legitimately fail to correlate.
- If two observations do not correlate, both remain valid; this is not a factual contradiction.

Inferences:

- Cross-payload matching needs a shared identity vocabulary or strategy registry, normalization rules, ambiguity policy, confidence, and tests.
- This is structural graph reconciliation, not presentation.
- The schema may need additional producer declarations, but those should be designed in this SOW, not hidden in presentation profiles.

Unknowns:

- Exact shared identity vocabulary and normalization rules.
- Whether matching is pairwise between producer kinds or generic through typed identity facts.
- How Cloud should handle ambiguous matches, partial matches, and conflicting confidence.
- How much matching should be done in the first MVP versus deferred.

### Acceptance Criteria

- Inventory identity and match evidence emitted by network-connections, SNMP/L2, streaming, and vSphere.
- Define a compact schema contract for cross-payload identity declarations if needed.
- Define Cloud aggregator matching strategies, normalization rules, ambiguity policy, and diagnostics.
- Define how endpoint actors are replaced, merged, or left separate.
- Add service fixtures for successful match, no match, ambiguous match, and conflicting presentation-independent identities.
- Update topology schema/spec/docs/skill if producer declarations are added.

## Analysis

Sources checked:

- `.agents/sow/current/SOW-0021-20260509-topology-presentation-contract.md`
- `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`
- `src/collectors/network-viewer.plugin/network-viewer.c`
- `src/go/plugin/go.d/collector/snmp_topology/func_topology_v1.go`
- `src/web/api/functions/function-topology-streaming.c`
- Cloud topology service evidence recorded in SOW-0021.

Current state:

- SOW-0021 records that `merge_identity` is per actor type and does not define shared identity classes across producers.
- SOW-0021 records that evidence `match_columns` preserve exact relationship details but do not declare endpoint replacement across topology payloads.
- SOW-0021 records the user decision to split this into SOW-0023.

Risks:

- False positive matches could collapse unrelated actors.
- False negative matches could duplicate actors that represent the same entity.
- NAT, load balancers, address reuse, namespaces, and reused MAC/IP identities can make simple exact matching unsafe.
- Matching strategies can leak sensitive infrastructure identities if durable artifacts include raw examples.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The compact topology schema has enough identity to aggregate within producer-defined types, but it does not yet define how Cloud should reconcile actors across different producer domains.

Evidence reviewed:

- SOW-0021 reviewer findings and user decision notes.
- Current topology schema identity and evidence match-column fields.

Affected contracts and surfaces:

- `netdata.topology.v1` schema.
- Cloud topology service aggregation algorithm.
- Topology producers for network-connections, SNMP/L2, streaming, vSphere, and future topology domains.
- Cloud frontend behavior when merged actors replace endpoint actors.

Existing patterns to reuse:

- Actor `identity`, `merge_identity`, and `parent_identity`.
- Evidence `match_columns`.
- Link direction and aggregation semantics.

Risk and blast radius:

- High: incorrect matching can materially change topology meaning.
- High: Cloud aggregation behavior changes across all topology kinds.
- Medium: producer schemas may need new typed identity declarations.

Sensitive data handling plan:

- Do not copy raw IP addresses, MAC addresses, hostnames, machine GUIDs, node IDs, account IDs, customer identifiers, or private topology examples into this SOW, specs, docs, skills, code comments, commits, or PR text.
- Use sanitized synthetic fixtures and placeholder identifiers.
- Keep any raw captured payloads under `.local/` only.

Implementation plan:

1. Inventory producer identity/evidence fields and current Cloud aggregation behavior.
2. Define match strategy schema and Cloud algorithm options with evidence and user decisions.
3. Implement Cloud aggregator matching and tests after approval.
4. Update producers only if new declarations are required.

Validation plan:

- Service-level fixtures for match, no match, ambiguous match, and unsafe match.
- Schema validation for any new identity declarations.
- Payload-size checks for added declarations.
- Same-failure search across topology producers.

Artifact impact plan:

- AGENTS.md: likely unaffected.
- Runtime project skills: likely unaffected unless topology workflow changes.
- Specs: update topology function schema spec.
- End-user/operator docs: update topology developer guide if producer contract changes.
- End-user/operator skills: update create-topology skill if producer contract changes.
- SOW lifecycle: this SOW tracks the SOW-0021 split decision.

Open-source reference evidence:

- Not checked yet. This SOW is blocked pending SOW-0021 and will perform its own reference review before implementation.

Open decisions:

- Pending full analysis.

## Implications And Decisions

1. User decision from SOW-0021: cross-payload actor reconciliation should be solved, but it can be split into SOW-0023.
2. User decision from SOW-0021: execution order is SOW-0021, then SOW-0023, then SOW-0022.

## Plan

1. Wait until SOW-0021 defines the presentation contract.
2. Inventory producer identity fields and Cloud matching needs.
3. Present match strategy options with concrete evidence.
4. Implement after decisions.

## Execution Log

### 2026-05-09

- Created as follow-up from SOW-0021 decision discussion.

## Validation

Acceptance criteria evidence:

- Pending.

Tests or equivalent validation:

- Pending.

Real-use evidence:

- Pending.

Reviewer findings:

- Pending.

Same-failure scan:

- Pending.

Sensitive data gate:

- This SOW currently contains only sanitized generic examples.

Artifact maintenance gate:

- AGENTS.md: pending close review.
- Runtime project skills: pending close review.
- Specs: pending analysis.
- End-user/operator docs: pending analysis.
- End-user/operator skills: pending analysis.
- SOW lifecycle: pending SOW created from SOW-0021 split.

Specs update:

- Pending.

Project skills update:

- Pending.

End-user/operator docs update:

- Pending.

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

## Followup

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
