# SOW-YYYYMMDD-<slug> - <Title>

This template is the schema of a SOW. `.agents/sow/audit.sh` derives the required sections from the `##` headings and
their tag, written exactly `<!-- sow:VALUE -->` (tags on `###` headings are ignored): no tag = required in every SOW,
umbrellas included; `sow:implementation` = required except in umbrella SOWs; `sow:umbrella-only` = required in umbrella
SOWs; `sow:optional` = never required. The audit also expects two field labels verbatim: `Sensitive data handling plan:`
in every SOW and `Sensitive data gate:` in non-umbrella SOWs (renaming either needs an audit change). The check is
advisory and covers presence only; filling every field is your responsibility. Field semantics live in the placeholders.

## Status

Status: planning

One value only: `planning`, `ready`, `in-progress`, `paused`, or `completed`.

`planning` means analysis or decisions are incomplete. `ready` means the
Pre-Implementation Gate is complete and, where the goal-approval round ("Plan
Before Non-Trivial Work") applies, the user has approved the goal and plan.
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

Umbrella (optional, step SOWs only): SOW-YYYYMMDD-<family>-umbrella

Branch (optional): <branch name, once created>

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

## Analysis <!-- sow:optional -->

Sources checked:

- <file/source>

Current state:

- <evidence>

Risks:

- <risk and implication>

## Pre-Implementation Gate

Gate status: blocked

One value only: `blocked`, `ready`, or `needs-user-decision`. This is the gate's key, distinct from the SOW `Status:`.

Problem / root-cause model:

- <What is happening, why it is happening, and evidence supporting that model.>

Evidence reviewed:

- <Specs, code, docs, tests, logs, traces, prior PRs/issues, external references.>
- <For mirrored open-source repositories: cite `owner/repo @ commit` and repository-relative paths; never paste
  machine-specific absolute mirror paths (the mirror lives at `${NETDATA_REPOS_DIR}`).>

Affected contracts and surfaces:

- <APIs, schemas, files, commands, UI, docs, specs, skills, tests, integrations, operators, users.>

Clean-end-state target:

- <The structure the codebase should have once the approved scope is fully delivered.>
- Removed as redundant (i): <code/config/docs/tests this change makes redundant.>
- Excluded coupled items (ii): <coupled items NOT part of this clean end state, each with reason + scope source.>
- Reference search (when a path/contract is replaced): <command(s) run + result; every surviving reference mapped to
  (i)/(ii), or the target is incomplete.>

Existing patterns to reuse:

- <Local modules, helpers, conventions, tests, and docs that shape the implementation.>

Risk and blast radius:

- <Regression, compatibility, performance, security, data loss, migration, rollout, and operational risks.>

Sensitive data handling plan:

- <Whether the work may expose secrets, credentials, bearer tokens, SNMP communities, community/customer data, personal
  data, non-private customer-identifying IPs, private endpoints, or proprietary incident details; how evidence will be
  redacted in SOWs, specs, docs, skills, instructions, and code comments.>

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
- SOW lifecycle: <local-only working file under .agents/sow/q/ (never committed); durable-knowledge targets
  (skills/docs/code/tests); regression = new linked SOW; follow-up issues>

Open-source reference evidence:

- <If local mirrored repositories under `${NETDATA_REPOS_DIR}` were checked, list each as `owner/repo @ commit` plus
  repository-relative paths. If none were checked, record why external OSS references were not relevant.>

Open decisions:

- <Resolved decision, or numbered options that block implementation until the user decides.>

## Implications And Decisions

<Numbered user decisions, options, selection, and reasoning. User decisions must be recorded before implementation.
A step SOW cites the umbrella's decisions by number instead of restating them.>

## Steps <!-- sow:umbrella-only -->

See "Umbrella And Step SOWs" in `AGENTS.md`; delete this section in non-umbrella SOWs.

| # | Step SOW | Status | PR |
|---|---|---|---|
| 01 | SOW-YYYYMMDD-<family>-01-<step>.md | planning | |

Cross-step follow-up mapping:

- <every item deferred across steps: implemented in step NN, rejected with evidence, or a linked GitHub issue>

## Execution Log

### YYYY-MM-DD

- <Material state transitions, files touched, decisions, deviations, validation,
  and reviewers. Keep this a current-state handoff: consolidate superseded rounds
  and do not reproduce the conversation or every review nit.>

## Workflow Friction & Rule Gaps <!-- sow:implementation -->

Running capture of anything that suggests a rule or workflow change: a rule that
was missing, ambiguous, or slowed the work; a practice worth codifying; a review
pattern that helped. Jot entries as they happen — do not reconstruct them from
memory at close. Every entry is triaged before completion (see the Artifact
Maintenance Gate).

- <observation + which artifact it may touch (AGENTS.md / project skill / spec / SOW template)>

## Validation <!-- sow:implementation -->

Acceptance criteria evidence:

- <evidence>

Clean-end-state evidence:

- <Delivered state vs the recorded target: (i) removed as redundant, (ii) excluded coupled items, and the recorded
  reference search where a path or contract was replaced; or a link to the user approval for a non-clean state.>

Deferred clean-end-state remainder:

- <Each deferred target item with why deferral was acceptable and when (or under what condition) it lands; or
  "none".>

Tests or equivalent validation:

- <command/output summary>

Real-use evidence:

- <manual/API/CLI/UI path; or why no runnable path exists>

Reviewer findings:

- <reviewer; each finding and how it was handled: verified and fixed, rejected with evidence, or tracked>

Same-failure scan:

- <search and result>

SOW audit:

- <`bash .agents/sow/audit.sh` summary line; each remaining failure fixed or explained>

Sensitive data gate:

- <Confirm durable artifacts contain no raw secrets, credentials, bearer tokens, SNMP communities, community member
  names, customer names, personal data, non-private customer-identifying IPs, private endpoints, or proprietary incident
  details; note redactions used.>

## Artifact Maintenance Gate <!-- sow:implementation -->

- AGENTS.md: <updated path or evidence-backed reason no update was needed>
- Runtime project skills: <updated .agents/skills/project-*/ path or evidence-backed reason no update was needed>
- Specs: <updated .agents/sow/specs/ path or evidence-backed reason no update was needed>
- End-user/operator docs: <updated docs/runbooks/help paths or evidence-backed reason none were affected>
- End-user/operator skills: <updated output/reference skill paths or evidence-backed reason none were affected>
- SOW lifecycle: <durable knowledge transferred to skills/docs/code/tests; follow-ups moved to GitHub issues or
  rejected; `Status: completed` set; SOW working file is local-only under .agents/sow/q/ and never committed;
  regression-as-new-SOW handling recorded>
- Workflow friction triaged: <each `Workflow Friction & Rule Gaps` entry resolved to a rule update (file + change), an
  evidence-backed rejection, or a tracked follow-up; "no workflow friction arose" if the section is empty>

Lessons:

- <lesson or specific reason none>

Follow-up mapping:

- <implemented/rejected/GitHub issue link>

## Outcome <!-- sow:optional -->

Pending.
