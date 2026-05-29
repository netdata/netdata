# SOW-0041 - Cloud topology generic aggregation verification

## Status

Status: open

Sub-state: corrected by SOW-0044. Not started.

## Requirements

### Purpose

Verify and, only if needed, fix the generic Cloud topology aggregation path so
producer-declared `group_by`, actor rows, actor labels, and column aggregation
rules are preserved without topology-specific Cloud code. When the Cloud PR
changes the shared topology schema or adds merge features, keep the Agent copy
of the schema, developer documentation, and topology-authoring skill in sync.

### User Request

Track the Cloud-side generic validation separately from the Agent producer work.
The Cloud topology service and frontend must remain topology-agnostic; this SOW
must not implement container-specific behavior.

### Assistant Understanding

Facts:

- SOW-0044 repairs the Agent producer contract so grouped views merge variable
  enrichment through actor labels and declared `set` aggregation metadata.
- Cloud request forwarding is generic: reserved metadata names do not include
  `group_by`, and advertised non-reserved parameters are forwarded.
- Cloud actor aggregation is generic and producer-declared through actor
  identity/merge identity and column aggregation rules.
- Frontend Function selections and topology v1 actor attributes are generic.
- The topology schema source-of-truth is mirrored across Cloud and Agent work:
  a Cloud PR that changes merge/schema semantics also requires updates in this
  repository to `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`,
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`, and
  `.agents/skills/project-create-topology/SKILL.md`.

Inferences:

- This SOW should become a Cloud test/fixture PR only if generic verification
  exposes a gap.

Unknowns:

- Exact Cloud repo branch and PR sequencing must be confirmed before
  implementation.

### Acceptance Criteria

- Cloud forwarding accepts producer-advertised `group_by` selections without
  hardcoded topology knowledge.
- Cloud aggregation preserves actor rows, actor labels, and producer-declared
  `set` aggregation for merged attributes.
- Frontend topology v1 rendering consumes producer-declared actor rows, labels,
  modal recipes, and selections without container-specific code.
- Any Cloud changes are generic tests or generic schema/aggregation fixes.
- If the Cloud PR adds or changes topology merge/schema features, the same PR
  sequence updates this repository's topology schema
  (`src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`), developer guide
  (`src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`), and topology producer
  skill (`.agents/skills/project-create-topology/SKILL.md`) before the work is
  considered complete.

## Analysis

Sources checked:

- `.agents/sow/done/SOW-0036-20260526-network-viewer-topology-groupings.md`

Current state:

- Agent producer work is implemented in this repository; Cloud generic
  verification is tracked separately.

Risks:

- Cross-repository sequencing and API contract drift can produce UI controls or
  aggregation behavior that do not match Agent payload semantics.

## Pre-Implementation Gate

Status: blocked

Problem / root-cause model:

- The Agent emits generic topology metadata. Cloud consumers should forward
  selections and aggregate rows using the schema; if tests fail, the bug is in a
  generic Cloud topology path, not in missing container-specific Cloud logic.

Evidence reviewed:

- SOW-0036 Cloud-side consumer follow-up.

Affected contracts and surfaces:

- Cloud topology request forwarding, aggregation service, frontend Function
  selection flow, and topology v1 rendering.
- Agent topology schema copy:
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Agent topology developer guide:
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md`.
- Agent topology authoring skill:
  `.agents/skills/project-create-topology/SKILL.md`.

Existing patterns to reuse:

- Existing topology v1 request, normalization, aggregation, and frontend
  producer-declared modal/actor rendering paths in the Cloud repos.

Risk and blast radius:

- Medium-high. This is cross-repository user-visible Cloud behavior, but the
  intended changes are generic tests or generic aggregation fixes.

Sensitive data handling plan:

- Use synthetic topology payloads and redacted Cloud test data only.

Implementation plan:

1. Confirm Cloud repositories, owners, and target branch.
2. Audit current Cloud request forwarding, aggregation, and frontend selection
   behavior.
3. If Cloud merge/schema behavior changes, update the Agent schema copy,
   developer guide, and topology-authoring skill in the same implementation
   sequence.
4. Add generic tests or generic aggregation/schema fixes only if evidence shows
   a gap.

Validation plan:

- Synthetic SOW-0044 payloads through frontend and aggregation tests.
- Agent-side schema validation with the updated
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Developer-guide/skill consistency check for every new merge/schema feature:
  schema keyword exists, guide describes producer usage, and
  `project-create-topology` tells future topology producers how to use it.
- Manual Cloud UI verification in a non-production environment.

Artifact impact plan:

- AGENTS.md: update only if cross-repo workflow guardrails change.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md`
  when Cloud schema/merge features change producer authoring rules.
- Specs: update only if generic topology consumer semantics change.
- Topology schema: update `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json` when
  the Cloud PR adds or changes merge-feature schema.
- Developer documentation: update
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md` for every schema merge
  feature producers can use.
- End-user/operator docs: update Cloud topology operator docs.
- End-user/operator skills: update query topology skills if workflow changes.
- SOW lifecycle: move to current only after repository and ownership are confirmed.

Open-source reference evidence:

- Not checked yet; this SOW is only a tracker until activated.

Open decisions:

- None for container-specific behavior. Open only: confirm whether generic
  verification needs a Cloud PR after SOW-0044 payload testing.

## Implications And Decisions

Pending activation.

## Plan

1. Confirm Cloud-side implementation location.
2. Audit and design grouping flow.
3. Implement, test, and document.

## Execution Log

No work yet.

## Validation

Pending activation.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

None yet.

## Regression Log

None yet.
