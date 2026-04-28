# AGENTS.md

This file provides guidance to AI coding agents (Claude Code, Codex CLI, Gemini CLI, Opencode, Qwen-code, Crush, and others) working with code in this repository. The repo-root `CLAUDE.md` and `GEMINI.md` are relative symlinks to this file so every tool reads the same instructions.

THE MOST IMPORTANT RULES ARE:

1. You MUST ALWAYS find the root cause of a problem, before giving a solution.
2. Patching without understanding the problem IS NOT ALLOWED.
3. Before patching code, we MUST understand the code base and the potential implications of our changes.
4. We do not duplicate code. We first check if similar code already exists and to reuse it.

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

When an agent learns something new while running a skill (a new gotcha, a
working API call, a corrected workflow) it MUST update the skill's
`SKILL.md` and commit it before proceeding. Knowledge that isn't committed
is lost.

Currently available skills:
- `.agents/skills/coverity-audit/` - Coverity Scan defect triage
- `.agents/skills/sonarqube-audit/` - SonarCloud findings triage
- `.agents/skills/graphql-audit/` - GitHub Code Scanning (CodeQL) triage
- `.agents/skills/pr-reviews/` - PR comment / review iteration loop

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
cookies, project keys. Each skill's `SKILL.md` documents the variables it
needs. Never commit secrets; never hard-code tokens in scripts.
