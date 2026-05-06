# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex CLI, Gemini CLI, Opencode, Qwen-code, Crush, and others) working with code in this repository. The repo-root `CLAUDE.md` and `GEMINI.md` are relative symlinks to this file so every tool reads the same instructions.

THE MOST IMPORTANT RULES ARE:

1. You MUST ALWAYS find the root cause of a problem, before giving a solution.
2. Patching without understanding the problem IS NOT ALLOWED.
3. Before patching code, we MUST understand the code base and the potential implications of our changes.
4. We do not duplicate code. We first check if similar code already exists and to reuse it.

## Goals

This repository is the Netdata Agent codebase. It is a large, multi-language, multi-platform monolith that serves production monitoring, troubleshooting, data collection, alerting, storage, streaming, cloud integration, packaging, and documentation workflows.

Work in this repository must prioritize root-cause understanding, correctness, performance, maintainability, portability, security, and consistency with existing project conventions. Because the project is too broad for one bootstrap pass, SOW coverage grows incrementally by the area being worked.

## SOW System

This project uses a local Statement of Work system.

The SOW system is self-contained in this repository. Normal SOW work must not depend on `~/.agents`, `~/.AGENTS.md`, global skills, global templates, or global scripts. Use this `AGENTS.md`, project-local SOW files, project-local specs, project-local skills, and the active SOW.

### Roles

- **User responsibilities:** purpose, scope decisions, design forks, risk acceptance, destructive approvals, and final product judgment.
- **Assistant responsibilities:** investigation, evidence, implementation, tests or equivalent validation, reviews, documentation, memory updates, and concise reporting.

### Required First Checks

Before non-trivial work:

1. Read pending/current SOWs for overlap, contradictions, and existing decisions.
2. Read relevant specs under `.agents/sow/specs/`.
3. Inspect `.agents/skills/project-*/SKILL.md` if any exist, and load every runtime project skill whose trigger matches the work.
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

### Open-Source Reference Evidence

When SOW evidence comes from local mirrored open-source repositories under `/opt/baddisk/monitoring/repos/`, cite the upstream repository and checked commit instead of the workstation absolute path.

Use:

```text
owner/repo @ commit
relative/path/inside/repo:line
```

Resolve `owner/repo` from the repository remote, record the checked commit, and keep paths relative to the upstream repository root. Never write `/opt/baddisk/monitoring/repos/...` paths into SOW evidence.

### Pre-Implementation Gate

Implementation must not begin until the active SOW contains a concrete `## Pre-Implementation Gate` section. Before moving a SOW from `pending/open` to `current/in-progress`, or before continuing implementation in an existing current SOW that lacks this section, fill the gate.

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

### SOW Locations

- Pending: `.agents/sow/pending/`
- Current: `.agents/sow/current/`
- Done: `.agents/sow/done/`
- Specs: `.agents/sow/specs/`
- Template for new SOWs: `.agents/sow/SOW.template.md`
- Local audit: `.agents/sow/audit.sh`

Create new SOW files from `.agents/sow/SOW.template.md`. The template is project-local and may be customized for this repository.

Empty SOW directories must contain `.gitkeep` or `.keep` so the committed repository preserves the full SOW layout after clone/checkout.

Filename:

```text
SOW-NNNN-YYYYMMDD-{slug}.md
```

Status and directory must agree:

- `open` lives in `pending/`
- `in-progress` lives in `current/`
- `paused` lives in `current/`
- `completed` lives in `done/`
- `closed` lives in `done/`

### SOW Completion And Commit

The successful terminal SOW status is `completed`. `done` is a directory name, not a status value. Never write `Status: done` or `Status: complete`.

When a SOW's work is ready to close:

1. Finish implementation, docs, specs, skills, validation, and follow-up mapping.
2. Update the SOW to `Status: completed`.
3. Move the SOW file to `.agents/sow/done/`.
4. Commit the work, artifact updates, SOW status change, and SOW move together as one commit, unless the user explicitly requested a different commit split.

Do not create a separate commit just to mark or move the SOW. Do not claim a SOW is completed while the implementation and the SOW lifecycle change live in separate uncommitted or separately committed states.

### One SOW At A Time

Never execute multiple SOWs as one batch.

If work overlaps:

- merge or consolidate before implementation; or
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
- represented by a real pending/current SOW file.

Pre-close, search the SOW for:

```text
defer|later|follow-up|future|TODO|pending
```

Map every remaining item to implemented, rejected, or tracked.

### Regressions

A regression is discovered after a SOW was considered completed or closed, later testing or use finds broken behavior, and the original SOW's claimed outcome is no longer true.

When behavior that a completed SOW claimed working stops working:

1. Find the original SOW in `done/`.
2. Move it back to `current/`.
3. Mark it `in-progress` with a regression note in `## Status`.
4. Append a new dated `## Regression - YYYY-MM-DD` section at the end of the file, after the original outcome, lessons, and follow-up content.
5. In that appended section, record what broke, evidence, why previous validation missed it, the repair plan, validation, and updates needed to specs, skills, docs, audits, or follow-up SOWs.
6. Fix and validate there.

Never prepend regression content above the original SOW narrative. The original requirements, analysis, plan, validation, outcome, lessons, and follow-up must remain readable first.
Do not create a new SOW for a true regression.

### Validation Gate

A SOW cannot be completed until Validation records:

- acceptance criteria evidence;
- tests or equivalent validation;
- real-use evidence when a runnable path exists;
- reviewer findings and how they were handled;
- same-failure search results;
- artifact maintenance gate for `AGENTS.md`, runtime project skills, specs, end-user/operator docs, end-user/operator skills, and SOW lifecycle;
- SOW status/directory consistency;
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
- SOW lifecycle - split, merge, status, directory, deferred work, regression reopening, and follow-up mapping.

This is an assistant responsibility. If a SOW changes behavior, docs, specs, commands, schemas, defaults, workflows, examples, or operating procedure, the assistant must update every affected artifact in the same SOW, or record the evidence-backed reason an artifact is unaffected.

### Specs

Specs are memory of WHAT this project does.

This repository is bootstrapped incrementally. The existing source tree and public documentation remain the primary ground truth. SOW specs under `.agents/sow/specs/` should capture durable project decisions, cross-cutting behavioral rules, and area-specific contracts as they are worked.

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

Runtime input project skills should live under `.agents/skills/project-*/SKILL.md`. The `project-` prefix is the generic hook meaning "agents working in this repo must consider this skill." Before non-trivial work, inspect those skill descriptions and load every matching runtime skill.

Do not create generic `project-*` skills only to make the framework look complete. The user requested that project skills for this repository grow incrementally.

Existing non-`project-*` skills under `.agents/skills/` are preserved as legacy runtime skills. Use them when the request matches their frontmatter trigger. Do not rename them or add wrappers during this bootstrap pass.

Output/reference skills may also exist under product documentation or generated skill directories. Do not rename, shorten, or change their descriptions only to satisfy runtime discovery. Update them when their related public/operator workflow changes.

### Public skill convention (`docs/netdata-ai/skills/`)

End-user-facing AI skills under `docs/netdata-ai/skills/` follow the directory shape `docs/netdata-ai/skills/<skill-name>/SKILL.md`, with optional supporting docs (`<topic>.md`) and an optional `scripts/` subdirectory for helper code. SKILL.md frontmatter has `name` and `description`; the description is the trigger-matching text and must enumerate the phrases users will actually type.

Each public skill is reachable from `.agents/skills/<skill-name>` via a relative symlink (`.agents/skills/<name>` → `../../docs/netdata-ai/skills/<name>`) so local AI assistants reading from `.agents/skills/` see the same skill as end-users. Create the symlink with `ln -srfn`. Verify with `readlink -f .agents/skills/<name>`.

Public-skill scripts must follow the same `_lib.sh` shape as the legacy private skills (`set -euo pipefail`, ANSI colors with real ESC bytes via `$'\033[...]'`, `<prefix>_repo_root` via `git rev-parse --show-toplevel`, `<prefix>_load_env` that sources `<repo>/.env` with `: "${VAR:?}"` validation, `<prefix>_audit_dir` that creates `<repo>/.local/audits/<topic>/`, masked-token `<prefix>_run`/`<prefix>_run_read` wrappers).

Public-skill scripts that touch credentials (cloud tokens, per-agent bearers, claim ids, session cookies) MUST be **token-safe** -- helpers that handle credential bytes are named with a leading underscore (`_skill_*`, internal-only) and return them via bash namerefs into the caller's local variables, NEVER to stdout. Public wrappers (no leading underscore) read credentials from `.env` internally and emit ONLY the response body. Each token-handling lib must ship a `<prefix>_selftest_no_token_leak` function that drives every public wrapper with a sentinel token and asserts the sentinel never appears on captured stdout.

### How-tos catalog rule

Each public skill ships a `how-tos/` subdirectory with `INDEX.md`. The catalog is **live**: every time an AI assistant is asked a concrete question that requires analysis (multiple wrapper calls, jq pipelines, or cross-referencing more than one per-domain guide) and the answer isn't already documented under `how-tos/`, the assistant MUST author a new how-to and add it to `INDEX.md` BEFORE completing the task. This rule is repeated in each skill's `SKILL.md` so future assistants honor it. Skipping it means the next assistant repeats the same analysis from scratch -- an explicit framework violation.

The legacy private skills (`coverity-audit`, `sonarqube-audit`, `graphql-audit`, `pr-reviews`) keep their `.agents/skills/<name>/` location -- they are intentionally private and have no `docs/netdata-ai/skills/` counterpart.

### Project Skills Index

Runtime input skills:

- `.agents/skills/project-snmp-profiles-authoring/`
  Trigger: editing SNMP profile YAMLs, topology SNMP profiles, ddsnmp profile parsing, or SNMP profile-format documentation.
  Purpose: require MIB `MAX-ACCESS` checks and index-derived extraction for `not-accessible` INDEX objects.
- `.agents/skills/project-writing-collectors/`
  Trigger: authoring or modifying any Netdata data-collection plugin or module (Go go.d / ibm.d, Rust crates, internal C plugins, external plugins via PLUGINSD). Read before adding a new collector, modifying an existing one, working on NetFlow/sFlow/IPFIX, OTEL ingestion, topology, SNMP profiles, or interactive Functions.
  Status: live. Updates that close gaps or fix outdated pointers must ship in the same PR that exposed the issue.
- `.agents/skills/integrations-lifecycle/`
  Trigger: editing any `metadata.yaml`; modifying `integrations/` generators, schemas, or templates; working with `integrations.js` / `integrations.json` / per-integration `.md` files / `COLLECTORS.md` / `SECRETS.md` / `SERVICE-DISCOVERY.md`; ibm.d module generation (`contexts.yaml` -> `metadata.yaml`); CI workflows `generate-integrations.yml` and `check-markdown.yml`; the five-file collector-consistency rule.
  Status: live. SKILL.md plus per-domain guides (`pipeline.md`, `schema-reference.md`, `per-type-matrix.md`, `artifacts-and-banners.md`, `ibm-d.md`, `consistency.md`, `in-app-contract.md`, `gotchas.md`) and `recipes/`, `how-tos/` directories.
- `.agents/skills/learn-site-structure/`
  Trigger: adding/moving/renaming/deleting any docs page that should appear on `learn.netdata.cloud`; editing `<repo>/docs/.map/map.yaml`; investigating why a Learn page looks the way it does; reading the live `ingest/ingest.py` orchestrator or the legacy `ingest.js` / `ingest.md` (which are stale); MDX escape rules; redirects; the Netlify deploy contract.
  Status: live. SKILL.md plus per-domain guides (`mapping.md`, `pipeline.md`, `sidebars.md`, `mdx-rules.md`, `redirects.md`, `pitfalls-and-gotchas.md`, `authoring-boundary.md`) and `recipes/`, `how-tos/` directories.
- `.agents/skills/query-agent-events/`
  Trigger: investigating crashes, panics, or fatals across the Netdata fleet; downloading events from the agent-events ingestion namespace; analyzing AE_* fields and their enums; understanding the 23h client-side dedup or the after-the-fact event timing; using the systemd-journal Function multi-value `selections` filter for index-friendly queries.
  Status: live. SKILL.md plus per-domain guides (`AE_FIELDS.md`, `transports.md`, `update-cadence.md`, `query-discipline.md`, `finding-crashes.md`, `finding-fatals.md`), scripts (`scripts/_lib.sh`, `get-events.sh`, `analyze-events.sh`, `redact-events.sh`) and `recipes/`, `how-tos/` directories. Bug-investigation tool, NOT a generic logs query skill -- consumes `query-netdata-{cloud,agents}` for transport.
- `.agents/skills/mirror-netdata-repos/`
  Trigger: setting up or updating a local mirror of Netdata-org source repositories at `${NETDATA_REPOS_DIR}` for cross-repo grep / code review without GitHub API calls; running the vendored sync script; questions about the reset-to-default-branch safety mechanism or the `--repo NAME` scoping flag.
  Status: live. SKILL.md (single-file overview) plus the vendored `scripts/sync-netdata-repos.sh` (env-driven, sanitized, `--repo` scoping, `gh` optional for Phase 2) and `how-tos/` catalog. Independent from any other repo mirrors this workstation may have.

Legacy runtime skills:

- `.agents/skills/coverity-audit/`
  Trigger: Coverity Scan defect triage for this repository.
  Status: preserved under legacy name; project-skill alignment is deferred and tracked by `.agents/sow/pending/SOW-0003-20260501-legacy-runtime-skill-alignment.md`.
- `.agents/skills/sonarqube-audit/`
  Trigger: SonarCloud findings triage for this repository.
  Status: preserved under legacy name; project-skill alignment is deferred and tracked by `.agents/sow/pending/SOW-0003-20260501-legacy-runtime-skill-alignment.md`.
- `.agents/skills/graphql-audit/`
  Trigger: GitHub Code Scanning/CodeQL triage for this repository.
  Status: preserved under legacy name; project-skill alignment is deferred and tracked by `.agents/sow/pending/SOW-0003-20260501-legacy-runtime-skill-alignment.md`.
- `.agents/skills/pr-reviews/`
  Trigger: PR comment and review iteration work for this repository.
  Status: preserved under legacy name; project-skill alignment is deferred and tracked by `.agents/sow/pending/SOW-0003-20260501-legacy-runtime-skill-alignment.md`.
- `.agents/skills/codacy-audit/`
  Trigger: Codacy Cloud workflow for this repository -- pre-push local analysis (`codacy-analysis-cli` via docker or local binary) and read-only PR-issue fetching via the v3 API.
  Status: live. SKILL.md plus `scripts/_lib.sh` (token-safe wrappers + sentinel no-leak self-test), `scripts/analyze-local.sh`, `scripts/pr-issues.sh`, and a live `how-tos/INDEX.md` catalog. Read-only by design; write actions deferred to a future SOW.

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

When working on collectors (especially Go collectors), ALL of the following files MUST be kept in sync before creating a PR:

1. **The code** - All .go files implementing the collector
2. **metadata.yaml** - Proper information for the Netdata integrations page, including:
   - Metric descriptions with correct units
   - Alert definitions
   - Setup instructions
   - Configuration examples
3. **config_schema.json** - Schema for dynamic configuration in the dashboard
4. **Stock config file** (.conf file) - Example configuration users edit manually
5. **Health alerts** (health.d/*.conf) - Alert definitions for the collector metrics
6. **README.md** - Comprehensive documentation describing:
   - What the collector monitors
   - How it works
   - Configuration options
   - Troubleshooting

These files MUST be consistent with each other. For example:
- If units change in code, they MUST be updated in metadata.yaml
- If new metrics are added, they MUST be documented in metadata.yaml and README.md
- If configuration options change, they MUST be updated in config_schema.json, stock config, and documentation

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

## AI agent skills

Repo-scoped skills for AI agents live under `.agents/skills/<skill-name>/`.
Each skill is self-contained: a `SKILL.md` with frontmatter (`name`,
`description`) plus its own `scripts/` directory. Skills carry the operational
knowledge for tasks that recur across sessions (Coverity triage, SonarCloud
triage, GitHub Code Scanning triage, etc.).

These existing skills are legacy runtime skills, not SOW-generated `project-*`
skills. They are preserved under their original names during the incremental
SOW bootstrap.

When an agent learns something new while running a skill (a new gotcha, a
working API call, a corrected workflow) it MUST update the skill's
`SKILL.md` and commit it before proceeding. Knowledge that isn't committed
is lost.

Currently available skills:
- `.agents/skills/coverity-audit/` - Coverity Scan defect triage
- `.agents/skills/sonarqube-audit/` - SonarCloud findings triage
- `.agents/skills/graphql-audit/` - GitHub Code Scanning (CodeQL) triage
- `.agents/skills/pr-reviews/` - PR comment / review iteration loop
- `.agents/skills/codacy-audit/` - Codacy Cloud pre-push analysis + PR-issue triage

### Preservation Notes

- The pre-SOW `AGENTS.md` was copied to `AGENTS.md.pre-sow.bak` before this merge.
- Existing top-level rules, collector consistency requirements, C code notes, naming conventions, local-only directory rules, and `.env` secret rules were preserved.
- Existing non-`project-*` operational skills were preserved under their current paths.
- No `project-*` skills were created during bootstrap by user request.
- Existing root `TODO*.md` files were preserved in place and are tracked by a pending SOW for future classification.

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

Project SOW status: initialized
