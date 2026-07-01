# SOW-YYYYMMDD-<slug> - <Title>

## Status

Status: planning | ready | in-progress | paused | completed

`planning` means analysis or decisions are incomplete. `ready` means the
Pre-Implementation Gate is complete and, where the goal-approval round ("Plan
before non-trivial work") applies, the user has approved the goal and plan.
`completed` means work is validated and durable memory transferred. SOW files
are local-only working memory under `.agents/sow/q/` (gitignored) and are never
committed.

Sub-state: <short current truth>

## Requirements

### Purpose

<User-stated purpose. All recommendations must align with this.>

### User Request

<Concise quote or faithful summary. Do not lose constraints.>

Regresses (optional): PR #NNNNN

### Assistant Understanding

Facts:

- <Established from user/project/code/specs.>

Inferences:

- <Reasoned but not directly stated.>

Unknowns:

- <Only real unknowns that cannot be resolved by investigation.>

### Acceptance Criteria

- <Outcome with verification method.>
- <Outcome with verification method.>

## Analysis

Sources checked:

- <file/source>

Current state:

- <evidence>

Risks:

- <risk and implication>

## Pre-Implementation Gate

Status: blocked | ready | needs-user-decision

Problem / root-cause model:

- <What is happening, why it is happening, and evidence supporting that model.>

Evidence reviewed:

- <Specs, code, docs, tests, logs, traces, prior PRs/issues, external references.>
- <For mirrored open-source repositories: cite `owner/repo @ commit` and repository-relative paths; never paste machine-specific absolute mirror paths (the mirror lives at `${NETDATA_REPOS_DIR}`).>

Affected contracts and surfaces:

- <APIs, schemas, files, commands, UI, docs, specs, skills, tests, integrations, operators, users.>

Clean-end-state target:

- <The structure the codebase should have once the approved scope is fully delivered.>
- Removed as redundant (i): <code/config/docs/tests this change makes redundant.>
- Excluded coupled items (ii): <coupled items NOT part of this clean end state, each with reason + scope source.>
- Reference search (when a path/contract is replaced): <command(s) run + result; every surviving reference mapped to (i)/(ii), or the target is incomplete.>

Existing patterns to reuse:

- <Local modules, helpers, conventions, tests, and docs that shape the implementation.>

Risk and blast radius:

- <Regression, compatibility, performance, security, data loss, migration, rollout, and operational risks.>

Sensitive data handling plan:

- <Whether the work may expose secrets, credentials, bearer tokens, SNMP communities, community/customer data, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident details; how evidence will be redacted in SOWs, specs, docs, skills, instructions, and code comments.>

Implementation plan:

1. <Ordered chunk with scope, dependencies, and likely files/modules.>
2. <Ordered chunk with scope, dependencies, and likely files/modules.>

Validation plan:

- <Tests, fixtures, manual checks, real-use evidence, review passes, same-failure searches.>

Artifact impact plan:

- AGENTS.md: <expected update or reason likely unaffected>
- Runtime project skills: <expected update or reason likely unaffected>
- Specs: <expected update or reason likely unaffected>
- End-user/operator docs: <expected update or reason likely unaffected>
- End-user/operator skills: <expected update or reason likely unaffected>
- SOW lifecycle: <local-only working file under .agents/sow/q/ (never committed); durable-knowledge targets (skills/docs/code/tests); regression = new linked SOW; follow-up issues>

Open-source reference evidence:

- <If local mirrored repositories under `${NETDATA_REPOS_DIR}` were checked, list each as `owner/repo @ commit` plus repository-relative paths. If none were checked, record why external OSS references were not relevant.>

Open decisions:

- <Resolved decision, or numbered options that block implementation until the user decides.>

## Implications And Decisions

<Numbered user decisions, options, selection, and reasoning. User decisions must be recorded before implementation.>

## Plan

1. <chunk, scope, risk, dependencies>
2. <chunk, scope, risk, dependencies>

## Execution Log

### YYYY-MM-DD

- <files touched, decisions, deviations, reviewers>

## Validation

Acceptance criteria evidence:

- <evidence>

Tests or equivalent validation:

- <command/output summary>

Real-use evidence:

- <manual/API/CLI/UI path>

Reviewer findings:

- <reviewer and findings>

Same-failure scan:

- <search and result>

Sensitive data gate:

- <Confirm durable artifacts contain no raw secrets, credentials, bearer tokens, SNMP communities, community member names, customer names, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident details; note redactions used.>

## Artifact Maintenance Gate

- AGENTS.md: <updated path or evidence-backed reason no update was needed>
- Runtime project skills: <updated .agents/skills/project-*/ path or evidence-backed reason no update was needed>
- Specs: <updated .agents/sow/specs/ path or evidence-backed reason no update was needed>
- End-user/operator docs: <updated docs/runbooks/help paths or evidence-backed reason none were affected>
- End-user/operator skills: <updated output/reference skill paths or evidence-backed reason none were affected>
- SOW lifecycle: <durable knowledge transferred to skills/docs/code/tests; follow-ups moved to GitHub issues or rejected; `Status: completed` set; SOW working file is local-only under .agents/sow/q/ and never committed; regression-as-new-SOW handling recorded>

Specs update:

- <updated spec or specific reason no update was needed>

Project skills update:

- <updated runtime project skill or specific reason no update was needed>

End-user/operator docs update:

- <updated docs or evidence-backed reason none were affected>

End-user/operator skills update:

- <updated output/reference skills affected by docs/spec changes, or evidence-backed reason none were affected>

Lessons:

- <lesson or specific reason none>

Follow-up mapping:

- <implemented/rejected/GitHub issue link>

## Outcome

Pending.

## Lessons Extracted

Pending.

## Follow-up Issues

None yet.
