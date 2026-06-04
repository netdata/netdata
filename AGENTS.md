# AGENTS.md

## Goals

This repository is the Netdata Agent codebase. It is a large, multi-language, multi-platform monolith that serves production monitoring, troubleshooting, data collection, alerting, storage, streaming, cloud integration, packaging, and documentation workflows.

Work in this repository must prioritize root-cause understanding, correctness, performance, maintainability, portability, security, and consistency with existing project conventions.

## Requirement Language

This repository uses RFC-style requirement language:

- **MUST** / **REQUIRED**: mandatory. Work that violates it is not acceptable
  unless the user explicitly changes the requirement.
- **MUST NOT**: prohibited.
- **SHOULD** / **RECOMMENDED**: expected default. Deviate only with evidence
  and explain the trade-off.
- **MAY** / **OPTIONAL**: allowed, not required.

CRITICAL RULES:

1. You MUST ALWAYS find the root cause of a problem, before offering/giving a solution.
   Patching without understanding the problem IS NOT ALLOWED.

2. Before patching code, you MUST understand the codebase and the potential implications of the changes.
   What else is affected? What else is using this part of the code?

3. Do not duplicate code.
   First check if similar code already exists and reuse it.

## Mandatory Development Principles

These principles are mandatory for every task in this repository:

1. **Clean end state over less churn.**
   - Target: you MUST always aim for the clean end state, not the smallest diff.
     While designing and implementing, actively search for the structure that
     should exist after the work is complete.
   - Re-evaluation: you MUST periodically re-evaluate already-written changes
     against that target. Do not keep a compromise only because it already
     exists in the branch.
   - Recommendation default: when options include a clean end state and a
     lower-churn partial state, you MUST recommend the clean end state unless it
     is technically impossible, unsafe, or explicitly outside the approved
     scope.
   - Exception handling: technical impossibility, safety risk, or scope conflict
     are pause conditions, not permission to choose the partial route
     automatically. Present the evidence, interrupt the work, and get explicit
     human approval before proceeding with a temporary non-clean state.
   - Staged delivery: if recommending staged delivery, explain why the staged
     plan still reaches the clean end state for the approved work, or explicitly
     state that you are asking the user to accept a temporary non-clean state.
   - Invalid justification: risk reduction, review convenience, or issue staging
     is not enough justification for recommending a partial end state.
   - Deferral check: before recommending deferral, check the issue, SOW,
     acceptance criteria, and affected migration scope for evidence that the
     deferred work is genuinely outside the approved clean end state.

2. **Scope discipline at every step.**
   At each milestone, you MUST check whether the work has drifted outside the
   approved scope. If the new work is valid but independent, you MUST defer it
   to a later step or pause and submit the independent work first, then rebase
   the current branch after it merges. Complex features MUST be delivered in
   coherent steps where each step builds on the previous one.

USER COMMUNICATION:

1. ALWAYS DO YOUR HOMEWORK BEFORE ASKING QUESTIONS OR REQUESTING USER DECISIONS.
   PROACTIVELY CHECK ALL RELATED ASPECTS AND ALL POSSIBILITIES SO THAT YOUR QUESTIONS AND REQUESTS ARE WELL INFORMED AND TO THE POINT.

2. NEVER WRITE WALLS OF TEXT TO THE USER, UNLESS THEY ASKED FOR IT.
   YOUR COMMUNICATION MUST BE SIMPLE, DIRECT, LEAN, ORDERED BY IMPORTANCE.
   PROVIDE THE FULL PICTURE AT THE BEGINNING, START FROM THE HIGH LEVEL, AND LET THE USER ASK FOR DETAILS.

3. NEVER AGREE TO THE USER WHEN THE FACTS CONTRADICT THEIR UNDERSTANDING.
   YOU MUST ALWAYS PROVIDE CLEAR DESCRIPTIONS OF THE RISKS AND IMPLICATIONS OF THEIR DECISIONS.
   YOU ARE HELPFUL WHEN YOU ACCURATELY REVEAL THE TRUTH, NOT WHEN YOU AGREE.

## SOW System

Project SOW status: initialized

This project uses a local Statement of Work system.

SOWs are branch-local working memory, not product artifacts. During active work,
including draft PR and ready-for-review takeover work, a SOW may live on the
feature branch so it preserves the root-cause model, decisions, evidence, and
validation for PR takeover. Commit the active SOW on the feature branch when
takeover or handoff is expected. When no takeover is expected, keeping the
active SOW local and uncommitted is acceptable, but the SOW still MUST be used
as working memory. Before merge, complete the SOW, transfer durable knowledge,
and delete the active SOW file. `master` and the final merge head MUST contain
no SOW working files; durable memory belongs in `.agents/sow/specs/`, project
skills, docs, code, and tests.

The SOW system is self-contained in this repository. Normal SOW work must not depend on `~/.agents`, `~/.AGENTS.md`, global skills, global templates, or global scripts. Use this `AGENTS.md`, the branch-local SOW, project-local specs, and project-local skills.

### Roles

- **User responsibilities:** purpose, scope decisions, design forks, risk acceptance, destructive approvals, and final product judgment.
- **Assistant responsibilities:** investigation, evidence, implementation, tests or equivalent validation, reviews, documentation, memory updates, and concise reporting.

### Required First Checks

Before non-trivial work:

1. Read the current branch's SOW under `.agents/sow/active/` if one exists. Since SOWs are branch-local, discover other in-flight work through open PRs and issues, not through `master`.
2. Read relevant specs under `.agents/sow/specs/`.
3. Inspect `.agents/skills/*/SKILL.md` if any exist, and load every runtime project skill whose trigger matches the work.
4. Inspect legacy runtime skills listed below when the user request matches their frontmatter trigger.
5. Inspect code, docs, tests, and existing project instructions as ground truth.
6. Ask the user only for irreducible product/design/risk decisions.

### Git Worktrees

Assistants must not create git worktrees on their own. Create a git worktree only when the user explicitly asks for it or approves it.

### Sensitive Data In Durable Artifacts

SOWs, specs, documentation, project skills, agent instructions, and code comments are commit-ready artifacts. Treat them as public unless a repository-specific policy explicitly says otherwise.

CRITICAL: Never write raw sensitive data to durable artifacts. This includes passwords, API keys, bearer tokens, SNMP communities, private keys, connection strings with embedded credentials, session cookies, community member names, customer names, customer identifiers, personal data, non-private IP addresses that can identify customers, private endpoints, account IDs, and proprietary incident details.

Write only sanitized evidence:

- use placeholders such as `[REDACTED_SECRET]`, `[CUSTOMER]`, `[ACCOUNT]`, `[PRIVATE_ENDPOINT]`;
- use stable aliases such as `customer-a` only when the real mapping is not stored in the repository;
- cite file paths, line numbers, command names, schema fields, or error classes instead of copying sensitive values;
- summarize logs and traces; include only minimal redacted snippets.

If sensitive data is required to continue, stop and ask the user for a secure handling path. If sensitive data is found in a durable artifact, sanitize it before any commit. If sensitive data was already committed, tell the user and do not rewrite history without explicit approval.

### Durable AI-Facing Artifact Formatting

AI-facing durable artifacts include `AGENTS.md`, SOW specs, runtime project
skills, public/operator skills, SOW templates, instruction bridge files, and
other docs primarily written so future AI agents can execute repository rules
correctly.

When writing or updating these artifacts:

- Structure for retrieval and scanning. Use headings, short sections, labeled
  bullets, and numbered procedures so both humans and AI agents can find the
  exact rule quickly.
- Avoid dense multi-rule paragraphs. If a paragraph contains multiple
  requirements, exceptions, or decision branches, split it into bullets or a
  table.
- Use tables only for matrices or comparisons where the cells stay short. Use
  bullets for rules, workflows, checklists, and exception handling.
- Put RFC-style requirement words (`MUST`, `MUST NOT`, `SHOULD`, `MAY`) close
  to the action they govern. Do not hide mandatory behavior in explanatory
  prose.
- Prefer labeled bullets for operational guardrails, such as `Target`,
  `Exception handling`, `Validation`, or `Failure mode`.
- Keep one durable idea per bullet. If a bullet needs multiple sentences, the
  first sentence states the rule and later sentences provide evidence,
  rationale, or examples.
- Preserve precision over brevity. Formatting is for readability, not for
  weakening contracts or removing necessary evidence.

### Open-Source Reference Evidence

When SOW evidence comes from other open-source repositories, cite the upstream repository and checked commit instead of the workstation absolute path.

Use:

```text
owner/repo @ commit
relative/path/inside/repo:line
```

Resolve `owner/repo` from the repository remote, record the checked commit, and keep paths relative to the upstream repository root. Never write absolute paths into SOW evidence.

### Pre-Implementation Gate

Implementation must not begin until the branch-local SOW contains a concrete `## Pre-Implementation Gate` section with `Status: ready` or `Status: in-progress`. Before changing implementation files, or before continuing implementation in an existing SOW that lacks this section, fill the gate.

The gate must record the problem/root-cause model, evidence reviewed, affected contracts and surfaces, existing patterns to reuse, risk and blast radius, sensitive data handling plan, implementation plan, validation plan, artifact impact plan, and open decisions. The sensitive data plan must cover SOWs, specs, documentation, project skills, agent instructions, and code comments. Generic placeholders such as `TBD`, `N/A`, or "to be checked later" are invalid unless the SOW explains why the item truly does not apply. If the gate exposes an unknown that cannot be resolved by investigation, stop and ask the user before implementation.

### When A SOW Is Required

Create or reuse a SOW for non-trivial work:

- feature work;
- bug fixes with behavioral impact;
- refactors;
- migrations;
- documentation or content changes with product/business impact;
- process changes;
- regressions;
- spec hygiene;
- project skill changes;
- collector changes;
- packaging, install, or deployment changes;
- PR review iteration;
- static analysis triage that changes source, docs, or project policy;
- any work with unclear risk.

Trivial work does not need a SOW:

- typo fixes;
- formatting-only changes;
- mechanical rename with no behavior change;
- simple search/replace with low risk.

When unsure, treat the work as non-trivial.

### SOW Locations And Naming

- Active branch-local SOWs: `.agents/sow/active/`
- Specs: `.agents/sow/specs/`
- Template for new SOWs: `.agents/sow/SOW.template.md`
- Local audit: `.agents/sow/audit.sh`

There is no `done/` directory and no committed pending queue. On `master`,
`.agents/sow/active/` is empty except for `.gitkeep`; real SOW files exist only
on feature branches. Feature branches and PRs may commit active SOW files when
takeover or handoff is expected, but active SOW files are deleted before merge.

Create new SOW files from `.agents/sow/SOW.template.md`. The template is project-local and may be customized for this repository.

Empty SOW directories must contain `.gitkeep` or `.keep` so the committed repository preserves the full SOW layout after clone/checkout.

### Local SOW Parking

Users may keep private paused, abandoned, or not-yet-public SOW drafts under
`<repo-root>/.local/sow/`. This directory is gitignored and outside the project
SOW lifecycle.

Use `<repo-root>/.local/sow/` when the user wants to preserve work locally
without creating a public or team-visible GitHub issue yet.

Local parked SOWs are private memory only:

- they are not durable project memory;
- they are not visible to other contributors;
- they are not acceptable as the only tracking for work that must coordinate a
  team, block a merge, or survive across machines.

Deferred work has two valid tracking paths:

- public or team-visible follow-up: GitHub issue;
- private or local follow-up: `<repo-root>/.local/sow/`.

Active implementation work still MUST use `.agents/sow/active/`. Active SOW
files MAY be committed for takeover or handoff and still MUST be deleted before
merge.

Filename:

```text
SOW-YYYYMMDD-{slug}.md
```

Use the creation date plus a descriptive slug. There is no sequential `NNNN`
counter because it cannot be allocated safely across parallel branches.

SOW state lives in the file's `Status:` field:

- `planning` - analysis or decisions are incomplete; implementation is blocked.
- `ready` - the Pre-Implementation Gate is complete and implementation can start.
- `in-progress` - implementation is underway.
- `paused` - work is intentionally stopped but may resume on the branch.
- `completed` - work is validated and durable memory has been transferred; this is a transient state before deleting the SOW file.

### SOW Completion And Merge

The successful terminal SOW status is `completed`.

When a SOW's work is ready to merge:

1. Finish implementation, docs, specs, skills, validation, and follow-up mapping.
2. Transfer all durable knowledge into `.agents/sow/specs/`, project skills, docs, code, and tests. After this step, the SOW body MUST hold nothing durable that is not captured elsewhere.
3. Update the SOW to `Status: completed`.
4. Delete the SOW working file before merge.

Draft and ready-for-review PRs MAY temporarily contain
`.agents/sow/active/SOW-*.md` files when takeover or handoff is expected. The
SOW CI job still rejects committed active SOW files; that red check is an
intentional merge guard, not a sign that handoff or takeover is forbidden. The
branch HEAD that merges MUST contain no `.agents/sow/active/SOW-*.md` file.

### Enforcement

The SOW system is enforced by local audit tooling and CI:

- `.agents/sow/audit.sh` is the local consistency audit for SOW rules, specs,
  references, and sensitive-data scanning.
- `.agents/sow/scan-sensitive.sh` is the shared sensitive-data scanner used by
  local audit and CI.
- `.github/workflows/sow.yml` rejects pull requests that contain branch-local
  SOW working files under `.agents/sow/active/SOW-*.md` or legacy SOW working
  files under `.agents/sow/{pending,current,done}/SOW-*.md`. This failure is
  expected when an active SOW is intentionally committed for takeover or
  handoff; it MUST be cleared before merge.
- The same workflow scans changed SOW, spec, instruction, and cross-tool
  bridge files for raw sensitive data.

These checks are guards, not substitutes for the SOW Validation Gate. The
assistant still owns transferring durable knowledge out of the SOW before
merge.

### One SOW At A Time

Never execute multiple SOWs as one batch.

If work overlaps:

- coordinate through the relevant open PRs and issues;
- merge or consolidate branches before implementation; or
- split into separate SOWs and complete one before starting the next.

Progress reports are not stop points. Once a SOW is in progress, continue until it is delivered, failed with evidence, blocked on a real user decision/approval, or superseded by newer user instructions.

### User Decisions

When user decisions are needed:

1. Present concrete evidence with files/lines or source references.
2. Provide numbered options.
3. Explain pros, cons, implications, and risks.
4. Recommend one option with reasoning.
5. Record the user's decision in the SOW before implementation.

### Followup Discipline

"Deferred" is not a terminal outcome.

Before a SOW can close, every valid deferred item must be:

- implemented in the current SOW; or
- explicitly rejected as not worth doing, with evidence; or
- represented by a GitHub issue linked from the current SOW or PR.

Pre-close, search the SOW for:

```text
defer|later|follow-up|future|TODO|pending
```

Map every remaining item to implemented, rejected, or tracked.

### Regressions

A regression is broken behavior discovered after a SOW's work merged, where the
original claimed outcome is no longer true.

Because completed SOWs are not retained on `master`, a regression is handled as
new work:

1. Open a new branch-local SOW under `.agents/sow/active/`.
2. In `## Requirements`, link the prior work: `Regresses: PR #NNNNN` and cite
   any known commit, spec, issue, or test evidence.
3. Run the normal Pre-Implementation Gate and Validation for the new SOW.
4. Update the relevant spec, skill, doc, code, or test so durable memory reflects
   current reality.

Do not attempt to resurrect or mutate a prior SOW.

### Validation Gate

A SOW cannot be completed until Validation records:

- acceptance criteria evidence;
- tests or equivalent validation;
- real-use evidence when a runnable path exists;
- reviewer findings and how they were handled;
- same-failure search results;
- artifact maintenance gate for `AGENTS.md`, runtime project skills, specs, end-user/operator docs, end-user/operator skills, and SOW lifecycle;
- SOW working file removed before merge;
- spec update or specific reason no spec update was needed;
- project skill update or specific reason no skill update was needed;
- end-user/operator docs update or evidence-backed reason none were affected;
- end-user/operator skill update or evidence-backed reason none were affected by docs/spec changes;
- lessons extracted or specific reason there were none;
- follow-up mapping.

Generic "N/A" is invalid.

### Artifact Maintenance Gate

Every SOW close must explicitly record whether each durable artifact class was updated or why no update was needed:

- `AGENTS.md` - workflow, responsibility, local framework, project-wide guardrails.
- Runtime project skills - `.agents/skills/project-*/SKILL.md` for HOW to work here.
- Specs - `.agents/sow/specs/` for WHAT the project does.
- End-user/operator docs - README, docs site, runbooks, published guides, help text, or other human-facing documentation.
- End-user/operator skills - output/reference skills copied or consumed outside normal repo work.
- SOW lifecycle - branch-local active SOW, durable memory transfer, SOW deletion before merge, deferred work tracked as GitHub issues, and regressions handled as new linked SOWs.

This is an assistant responsibility. If a SOW changes behavior, docs, specs, commands, schemas, defaults, workflows, examples, or operating procedure, the assistant must update every affected artifact in the same SOW, or record the evidence-backed reason an artifact is unaffected.

### Specs

Specs are memory of WHAT this project does.

This repository is bootstrapped incrementally. The existing source tree and public documentation remain the primary ground truth. SOW specs under `.agents/sow/specs/` should capture durable project decisions, cross-cutting behavioral rules, and area-specific contracts as they are worked.

`.agents/sow/specs/` stays flat until scale proves hierarchy is needed. Use
`<domain>-<topic>.md` names, one durable contract or cross-cutting rule per file,
and update `.agents/sow/specs/README.md` in the same change. Do not split specs
by repository path; specs are organized by contract ownership, not source-file
location.

Update specs when shipped work changes:

- product behavior;
- public contracts;
- collector behavior;
- APIs and schemas;
- data formats;
- alerting semantics;
- packaging or deployment behavior;
- operational guarantees;
- known edge cases.

Specs describe current reality, not aspiration. If specs and code disagree, record the discrepancy in the active SOW and resolve or track it.

### Project Skills

Project skills are memory of HOW to work here.

Runtime input project skills should live under `.agents/skills/*/SKILL.md`. Before non-trivial work, inspect those skill descriptions and load every matching runtime skill.

Output/reference skills may also exist under product documentation or generated skill directories. Do not rename, shorten, or change their descriptions only to satisfy runtime discovery. Update them when their related public/operator workflow changes.

### Public skill convention (`docs/netdata-ai/skills/`)

End-user-facing AI skills under `docs/netdata-ai/skills/` follow the directory shape `docs/netdata-ai/skills/<skill-name>/SKILL.md`, with optional supporting docs (`<topic>.md`) and an optional `scripts/` subdirectory for helper code. SKILL.md frontmatter has `name` and `description`; the description is the trigger-matching text and must enumerate the phrases users will actually type.

Public skills are for operators and end-users. They may teach users how to
query Netdata Cloud, query Agents, inspect metrics/logs/topology/alerts, or run
safe operational commands. They must not contain developer-contract validation,
schema migration plans, producer authoring workflows, UI adapter work,
aggregator implementation notes, SOW handoff instructions, fixture maintenance,
PR-review tasks, or codebase-internal implementation recipes.

Developer-facing skills must live under `.agents/skills/`, preferably with a
`project-` prefix when they are runtime input for repository work. If a workflow
requires reading source files, updating schemas, validating fixtures, changing
collectors/producers, or coordinating frontend/backend/aggregator code, it is a
project developer skill, not a public skill.

Skill verification harness inputs are not public skill content. Keep seed
questions, grader rubrics, runner scripts, and transcript-generation prompts
under `.agents/skill-verification/<skill>/`, not under
`docs/netdata-ai/skills/<skill>/`.

Each public skill is reachable from `.agents/skills/<skill-name>` via a relative symlink (`.agents/skills/<name>` → `../../docs/netdata-ai/skills/<name>`) so local AI assistants reading from `.agents/skills/` see the same skill as end-users. Create the symlink with `ln -srfn`. Verify with `readlink -f .agents/skills/<name>`.

Public-skill scripts must follow the same `_lib.sh` shape as existing skills (`set -euo pipefail`, ANSI colors with real ESC bytes via `$'\033[...]'`, `<prefix>_repo_root` via `git rev-parse --show-toplevel`, `<prefix>_load_env` that sources `<repo>/.env` with `: "${VAR:?}"` validation, `<prefix>_audit_dir` that creates `<repo>/.local/audits/<topic>/`, masked-token `<prefix>_run`/`<prefix>_run_read` wrappers).

Public-skill scripts that touch credentials (cloud tokens, per-agent bearers, claim ids, session cookies) MUST be **token-safe** -- helpers that handle credential bytes are named with a leading underscore (`_skill_*`, internal-only) and return them via bash namerefs into the caller's local variables, NEVER to stdout. Public wrappers (no leading underscore) read credentials from `.env` internally and emit ONLY the response body. Each token-handling lib must ship a `<prefix>_selftest_no_token_leak` function that drives every public wrapper with a sentinel token and asserts the sentinel never appears on captured stdout.

### How-tos catalog rule

Each public skill ships a `how-tos/` subdirectory with `INDEX.md`. The catalog is **live**: every time an AI assistant is asked a concrete operator/end-user question that requires analysis (multiple wrapper calls, jq pipelines, or cross-referencing more than one per-domain guide) and the answer isn't already documented under `how-tos/`, the assistant MUST author a new how-to and add it to `INDEX.md` BEFORE completing the task. This rule is repeated in each skill's `SKILL.md` so future assistants honor it. Skipping it means the next assistant repeats the same analysis from scratch -- an explicit framework violation.

The how-to rule does not override audience boundaries. If the analysis produced
a developer validation recipe, put it in the matching `.agents/skills/` project
skill and update that skill's index instead of adding it under
`docs/netdata-ai/skills/`.

The existing private skills (`coverity-audit`, `sonarqube-audit`, `graphql-audit`, `pr-reviews`) keep their `.agents/skills/<name>/` location -- they are intentionally private and have no `docs/netdata-ai/skills/` counterpart.

### Project Skills Index

Runtime input skills:

- `.agents/skills/project-snmp-profiles-authoring/`
  Trigger: editing SNMP profile YAMLs, topology SNMP profiles, ddsnmp profile parsing, or SNMP profile-format documentation.
  Purpose: require MIB `MAX-ACCESS` checks and index-derived extraction for `not-accessible` INDEX objects.

- `.agents/skills/project-writing-collectors/`
  Trigger: authoring or modifying any Netdata data-collection plugin or module (Go go.d / ibm.d, Rust crates, internal C plugins, external plugins via PLUGINSD). Read before adding a new collector, modifying an existing one, working on NetFlow/sFlow/IPFIX, OTEL ingestion, topology, SNMP profiles, or interactive Functions.
  Status: live. Updates that close gaps or fix outdated pointers must ship in the same PR that exposed the issue.

- `.agents/skills/project-create-topology/`
  Trigger: creating or updating Netdata topology producers, topology Function payloads, topology schema fixtures, graph presentation, correlation rules, direction semantics, topology drilldowns, telemetry overlays, or Cloud topology aggregation fixtures.
  Status: live. Developer-facing topology authoring workflow. End-user/operator-facing AI skills belong under `docs/netdata-ai/skills/`; this project skill is the runtime guidance for repository work.

- `.agents/skills/project-writing-go-modules-framework-v2/`
  Trigger: creating or migrating a Go go.d collector to framework V2; touching `CollectorV2`, `metrix.CollectorStore`, `ChartTemplateYAML` / `charts.yaml`, `charttpl`, `chartengine`, V2 host scopes, or V2 collector tests.
  Purpose: mirror maintainer-preferred framework V2 patterns from accepted collectors so new or migrated modules blend with repository style.

- `.agents/skills/integrations-lifecycle/`
  Trigger: editing any `metadata.yaml` or collector `taxonomy.yaml`; modifying `integrations/` generators, schemas, taxonomy registries, or templates; debugging generated gitignored integration outputs (`integrations.js`, `integrations.json`, `integrations/taxonomy.json`); working with committed per-integration `.md` files / `COLLECTORS.md` / `SECRETS.md` / `SERVICE-DISCOVERY.md`; ibm.d module generation (`contexts.yaml` -> `metadata.yaml`); CI workflows `generate-integrations.yml` and `check-markdown.yml`; the collector-consistency rule.
  Status: live. SKILL.md plus per-domain guides (`pipeline.md`, `schema-reference.md`, `per-type-matrix.md`, `artifacts-and-banners.md`, `ibm-d.md`, `consistency.md`, `in-app-contract.md`, `gotchas.md`) and `recipes/`, `how-tos/` directories.

- `.agents/skills/learn-site-structure/`
  Trigger: adding/moving/renaming/deleting any docs page that should appear on `learn.netdata.cloud`; editing `<repo>/docs/.map/map.yaml`; investigating why a Learn page looks the way it does; reading the live `ingest/ingest.py` orchestrator or the legacy `ingest.js` / `ingest.md` (which are stale); MDX escape rules; redirects; the Netlify deploy contract.
  Status: live. SKILL.md plus per-domain guides (`mapping.md`, `pipeline.md`, `sidebars.md`, `mdx-rules.md`, `redirects.md`, `pitfalls-and-gotchas.md`, `authoring-boundary.md`) and `recipes/`, `how-tos/` directories.
- `.agents/skills/learn-pr-preview/`
  Trigger: only when the user explicitly asks to build, run, preview, inspect, or validate `learn.netdata.cloud` locally using the contents of a PR or documentation branch before merge.
  Status: live. SKILL.md with an isolated preview workflow that copies PR source content, runs Learn ingest with `--local-repo`, builds Docusaurus with the Netlify-pinned runtime, and inspects representative pages without dirtying the real Learn checkout.
- `.agents/skills/query-agent-events/`
  Trigger: investigating crashes, panics, or fatals across the Netdata fleet; downloading events from the agent-events ingestion namespace; analyzing AE_* fields and their enums; understanding the 23h client-side dedup or the after-the-fact event timing; using the systemd-journal Function multi-value `selections` filter for index-friendly queries.
  Status: live. SKILL.md plus per-domain guides (`AE_FIELDS.md`, `transports.md`, `update-cadence.md`, `query-discipline.md`, `finding-crashes.md`, `finding-fatals.md`), scripts (`scripts/_lib.sh`, `get-events.sh`, `analyze-events.sh`, `redact-events.sh`) and `recipes/`, `how-tos/` directories. Bug-investigation tool, NOT a generic logs query skill -- consumes `query-netdata-{cloud,agents}` for transport.

- `.agents/skills/mirror-netdata-repos/`
  Trigger: setting up or updating a local mirror of Netdata-org source repositories at `${NETDATA_REPOS_DIR}` for cross-repo grep / code review without GitHub API calls; running the vendored sync script; questions about the reset-to-default-branch safety mechanism or the `--repo NAME` scoping flag.
  Status: live. SKILL.md (single-file overview) plus the vendored `scripts/sync-netdata-repos.sh` (env-driven, sanitized, `--repo` scoping, `gh` optional for Phase 2) and `how-tos/` catalog. Independent from any other repo mirrors this workstation may have.

- `.agents/skills/coverity-audit/`
  Trigger: Coverity Scan defect triage for this repository.
  Status: live.

- `.agents/skills/sonarqube-audit/`
  Trigger: SonarCloud findings triage for this repository.
  Status: live.

- `.agents/skills/graphql-audit/`
  Trigger: GitHub Code Scanning/CodeQL triage for this repository.
  Status: live.

- `.agents/skills/pr-reviews/`
  Trigger: PR comment and review iteration work for this repository.
  Status: live.

- `.agents/skills/codacy-audit/`
  Trigger: Codacy Cloud workflow for this repository -- pre-push local analysis (`codacy-analysis-cli` via docker or local binary) and read-only PR-issue fetching via the v3 API.
  Status: live. SKILL.md plus `scripts/_lib.sh` (token-safe wrappers + sentinel no-leak self-test), `scripts/analyze-local.sh`, `scripts/pr-issues.sh`, and a live `how-tos/INDEX.md` catalog. Read-only by design; write actions require a GitHub issue or branch-local SOW.

Public skills (canonical under `docs/netdata-ai/skills/<name>/`; relative symlinks at `.agents/skills/<name>`):

- `docs/netdata-ai/skills/query-netdata-cloud/`
  Trigger: querying Netdata Cloud REST API -- metrics, logs (systemd-journal), alerts, generic Function calls on a node.
  Symlink: `.agents/skills/query-netdata-cloud` -> `../../docs/netdata-ai/skills/query-netdata-cloud`.
  Status: live. SKILL.md plus per-domain guides (`query-metrics.md`, `query-logs.md`, `query-alerts.md`, `query-functions.md`).

- `docs/netdata-ai/skills/query-netdata-agents/`
  Trigger: querying Netdata Agents directly on port 19999, including auto-mint of per-agent bearer tokens from a Cloud token.
  Symlink: `.agents/skills/query-netdata-agents` -> `../../docs/netdata-ai/skills/query-netdata-agents`.
  Status: live. SKILL.md plus `scripts/_lib.sh` helpers (`agents_resolve_bearer`, `agents_call_function`, `agents_netdata_prefix`).

Output/reference skills:

- `docs/netdata-ai/skills/`
  Consumer: downstream assistants and users of Netdata AI skill artifacts.
  Update when: public/operator AI skill docs, examples, commands, schemas, or workflows change.

- `src/ai-skills/`
  Consumer: downstream assistants and users of generated or source AI skill artifacts when this tree is present in the working copy.
  Update when: generated/source AI skill behavior, tests, examples, commands, schemas, or workflows change.

### Project-specific commands

- This bootstrap pass does not define a full-project command matrix for the monolith.
- Use the narrowest existing command that validates the changed subsystem.
- Do not claim full-project validation from a narrow subsystem command.
- Existing local helper scripts such as `install.sh` may exist in this working copy; inspect before use and do not assume they are tracked project interfaces.

### Go test style

- Prefer table-driven tests using `map[string]struct{}` keyed by test-case name
  when cases share setup and assertion shape.
- Use separate test functions only when setup or assertions are materially
  different.
- Prefer map keys over a `name` field in `[]struct{}` so case names are
  prominent and order-independent.

### Project-specific overrides

All existing project-specific instructions in this file remain active. The SOW framework adds durable work tracking; it does not weaken the root-cause, collector consistency, C code, naming, local-output, or secret-handling rules below.

## Collector Consistency Requirements

When working on collectors, runtime behavior, metrics, charts, configuration,
alerts, taxonomy, and generated documentation MUST stay consistent in one PR.
The detailed collector consistency checklist and CI enforcement notes live in
`.agents/skills/integrations-lifecycle/consistency.md`.

## C code
- gcc, clang, glibc and muslc
- libnetdata.h includes everything in libnetdata (just a couple of exceptions) so there is no need to include individual libnetdata headers
- Functions with 'z' suffix (mallocz, reallocz, callocz, strdupz, etc.) handle allocation failures automatically by calling fatal() to exit Netdata
- The freez() function accepts NULL pointers without crashing
- Resuable, generic, module agnostic code, goes to libnetdata
- Double linked lists are managed with DOUBLE_LINKED_LIST_* macros
- json-c for json parsing
- buffer_json_* for manual json generation

## Naming Conventions
- "Netdata Agent" (capitalized) when referring to the product
- "`netdata`" (lowercase, code-formatted) when referring to the process
- See DICTIONARY.md for precise terminology

## Local-only working directory

`/.local/` at the repo root is gitignored and reserved for per-user runtime
artifacts: audit reports, fetched API data, scratch notes, queue files,
intermediate triage decisions. Agents writing skill output should default to
`<repo-root>/.local/audits/<topic>/...` -- where `<topic>` is the skill
name with any trailing `-audit` suffix removed (so `coverity-audit/`
writes under `coverity/`, `pr-reviews/` writes under `pr-reviews/`).

Convention:
- `/.local/audits/coverity/`  - Coverity raw fetches, per-defect details, triage decisions
- `/.local/audits/sonarqube/` - Sonar finding queues, FP comment templates
- `/.local/audits/graphql/`   - GitHub Code Scanning fetches and dismissals
- `/.local/audits/pr-reviews/`- Per-PR comment / review caches

Naming: each skill `<topic>-audit/` writes to `.local/audits/<topic>/`
(the `-audit` suffix is dropped from the directory name so the URL-style
path stays short). Skills without the `-audit` suffix keep their full
name (e.g. `pr-reviews/` writes to `.local/audits/pr-reviews/`). When
adding a new skill, follow this convention.

Nothing under `/.local/` is committed. Treat the directory as ephemeral
between users and machines, not as a shared source of truth.

## Per-user secrets via `.env`

`/.env` at the repo root is gitignored and holds per-user secrets and
endpoint configuration consumed by skill scripts: API tokens, session
cookies, project keys. Never commit secrets; never hard-code tokens in scripts.

**Setup**: copy `<repo>/.env.template` to `<repo>/.env` and fill in
the keys you need.

**Reference**: `<repo>/.agents/ENV.md` is the single canonical guide
covering every key -- what it is, where to find the value, sample
format, common mistakes, and which skills require it. When a script
errors with `<KEY> is empty`, check `.agents/ENV.md` for that key.
