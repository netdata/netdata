# SOW-0010 - Netdata query skills infrastructure

## Status

Status: completed

Sub-state: rescoped 2026-05-03 evening (second expansion). The two public skills must mirror each other in structure and cover every queryable Netdata surface (metrics, logs, topology, flows, alerts, dyncfg, functions, nodes, plus Cloud-only rooms/members/feed and Agent-only streaming). Scripts must be **token-safe** -- the assistant must never see `NETDATA_CLOUD_TOKEN`, per-agent bearer values, or claim ids on stdout. A `how-tos/` subdir with `INDEX.md` ships in each skill; assistants extend it whenever they perform analysis not already covered by a how-to. The verification harness (Sonnet test runner with grading rubric) moves to follow-up SOW-0006 per user direction "evaluation does not need to be done now". Decisions 1, 2, 3 already resolved.

## Requirements

### Purpose

Build the foundational AI-skill infrastructure that lets human and AI
operators query Netdata Cloud and Netdata Agents in a uniform,
documented way. This SOW is **infrastructure-only** -- it ships no
business analysis, no triage scripts, no fleet-data fetches. Its
deliverables are reusable pieces that downstream SOWs (agent-events,
documentation pipeline, etc.) consume.

Three deliverables:

1. **Skill format convention.** Every public AI skill under
   `docs/netdata-ai/skills/` must follow the `<skill-name>/SKILL.md`
   directory shape with optional `<doc>.md` supporting docs and
   `scripts/` subdir, exactly like the private operational skills
   under `.agents/skills/`. Each public skill must be reachable from
   `.agents/skills/<skill-name>` via a **relative symlink** so local
   AI assistants reading from `.agents/skills/` see the same skill.
   The convention is documented in `AGENTS.md` so future skills
   follow the same shape.

2. **`query-netdata-cloud/` skill (refactor + expand).** The current
   single file `docs/netdata-ai/skills/query-netdata-cloud-metrics.md`
   is migrated to `docs/netdata-ai/skills/query-netdata-cloud/
   SKILL.md` and expanded with separate supporting docs:
   - `query-metrics.md` -- the existing metrics-query content,
     trimmed to its specific surface
   - `query-logs.md` -- how to call the `systemd-journal` Function
     (and any other log-related Function) via Cloud
   - `query-alerts.md` -- how to query alerts and alert
     transitions via Cloud
   - `query-functions.md` -- generic Cloud-proxied Function
     invocation: what URL, what body, what response
   The top-level `SKILL.md` covers what is common across all four:
   auth (`NETDATA_CLOUD_TOKEN`), space/room/node resolution,
   pagination, error handling, dry-run discipline, links to each
   supporting doc.

3. **`query-netdata-agents/` skill (new).** Sibling skill that
   covers querying Netdata Agents directly (Parents and Children).
   Delivers SKILL.md + supporting docs + a `scripts/` library.
   The scripts library must:
   - Read `NETDATA_CLOUD_TOKEN` from `.env`.
   - Probe the agent URL and detect whether it is bearer-protected
     (HTTP 401, redirect to Cloud SSO, or any other documented
     bearer-required signal).
   - When a bearer is required, mint one for that specific node
     using the user's cloud token and the documented Cloud endpoint,
     cache the bearer in `.local/audits/query-netdata-agents/
     bearers/<node-uuid>.json` with its expiration, and refuse to
     log the token value.
   - Transparently refresh the bearer when expired (or one minute
     before expiry, to avoid races during long batch fetches).
   - Expose a single `agents_resolve_bearer <node>` helper that
     downstream skills call to obtain a current bearer for a node.
   - Expose a single `agents_call_function` helper that takes
     `{node, function, body}` and routes the request through the
     correct transport (Cloud-proxied vs direct-agent), retrying
     once across transports on transient failure.
   - Expose a `agents_netdata_prefix` helper that **autodetects
     the local Netdata install prefix** at runtime (probe order:
     empty / `/opt/netdata` / `/usr/local/netdata`; pick the
     first whose `<prefix>/var/lib/netdata` or
     `<prefix>/etc/netdata` exists). Used to locate the local
     `bearer_tokens/` directory for the local-fallback bearer
     path. NOT an env knob; the prefix is a discovered fact.

This SOW does NOT touch the legacy private operational skills
(`coverity-audit/`, `sonarqube-audit/`, `graphql-audit/`,
`pr-reviews/`) -- they keep their current location under
`.agents/skills/` and their current shape. They are intentionally
private and have no `docs/netdata-ai/skills/` counterpart.

### User Request

> Original (preserved for context):
> "Create a skill for querying and fetching and analyzing agent events
> (status file submissions) from the ingestion server."
>
> Stage-1 follow-up:
> "agent-events are stored in journal files... we have 2 options:
> 1. query logs via cloud (docs/netdata-ai/skills/ has a relevant
>    skill, although not for logs, but we could enrich it)
> 2. query the agent directly, but this time we need a mechanism to
>    get an agent bearer token by using the cloud api token, and
>    then query the agent API directly. Ideally we should support
>    all these methods."
>
> Scope expansion:
> "1. skills in docs/netdata-ai/skills/ should be formatted as
> normal skills {skill-name}/SKILL.md with skill frontmatter and
> potentially supporting documentation and scripts when necessary,
> and they should be linked to .agents/skills/ with a relative
> link, so that they are accessible by local agents too.
> 2. query-netdata-cloud-metrics skill, should be renamed to
> query-netdata-cloud/ and have SKILL.md with the common
> information about querying netdata cloud and then supporting docs
> query-metrics.md, query-logs.md, query-alerts.md,
> query-functions.md, etc as necessary, which should be referenced
> from SKILL.md.
> 3. A new skill query-netdata-agents/ should be added, explaining
> how to query netdata agents and parents and support the same
> supporting material documentation. This should also live in
> docs/netdata-ai/skills/ and be linked (relative) to
> .agents/skills/.
> 4. the query-netdata-agents skill should support querying agents
> via netdata cloud sso, so it should provide the tooling to fetch
> and cache and reuse and transparectly refresh agent bearer
> tokens, starting from an netdata cloud api token. The supported
> scripts should automatically detect the agent to query is bearer
> protected and automatically work around this to fetch netdata-
> cloud sso."
>
> Split decision (this run): "Go with 4 SOWs."

### Assistant Understanding

Facts:

- The .env + skill pattern is well-established and identical across
  the four legacy private skills. The same pattern is the basis for
  the scripts library shipped under `query-netdata-agents/`.
- The agent-side `systemd-journal` Function and the agent-side
  `bearer_get_token` Function exist in this repo; their shapes are
  documented in stage-1 analysis.
- Only one public AI skill exists today:
  `docs/netdata-ai/skills/query-netdata-cloud-metrics.md`. It is a
  flat file, not a directory. Its content is the template for the
  refactor.
- Per the user's rule, `.env` values stay in `.env`; only keys
  appear in this SOW or in scripts.
- The user added four `AGENT_EVENTS_*` keys to `.env`. Those keys
  are consumed by SOW-0003, not by this SOW. They are listed here
  only to confirm naming convention: `AGENT_EVENTS_NC_SPACE`,
  `AGENT_EVENTS_HOSTNAME`, `AGENT_EVENTS_MACHINE_GUID`,
  `AGENT_EVENTS_NODE_ID`.

Inferences:

- The Cloud REST shape that proxies a Function call to a node by
  uuid must already exist (otherwise no team member could query
  Cloud-only). The exact path is not in this open-source repo and
  must come from either user knowledge or the live Swagger at
  `${NETDATA_CLOUD_HOSTNAME}/api/docs/` (key in `.env`).
- A documented Cloud endpoint that mints an agent bearer from a
  cloud token must exist if the user wants the auto-refresh flow.
  Candidate names a Swagger fetch could check (paths under the
  Cloud API base): anything under `/api/v3/spaces/.../nodes/<uuid>/
  ...token`, `/api/v.../auth/...`, `/api/v.../bearer/...`. If no
  such endpoint is documented, the agents skill must degrade to
  "local-only" transport-(b) (as described in stage-1 decision 1B).

Unknowns (require user input or live-Swagger lookup):

- Cloud REST function-call endpoint shape (URL pattern, request
  body, response shape).
- Cloud REST agent-bearer mint endpoint shape (or confirmation it
  does not exist).
- Whether the existing Cloud-metrics doc references the correct
  current Cloud Swagger version. Last-revised date in
  `query-netdata-cloud-metrics.md` should be cross-checked against
  the live API at `${NETDATA_CLOUD_HOSTNAME}/api/docs/`.
- Symlink direction confirmation. User wrote: skills live in
  `docs/netdata-ai/skills/` and are symlinked from
  `.agents/skills/`. Reading: `docs/netdata-ai/skills/` is
  canonical; `.agents/skills/` holds relative symlinks pointing
  there. Confirmed during write-out.

### Acceptance Criteria

**(Scope expanded 2026-05-03 evening per user direction. The two
public skills must mirror each other in structure, cover every
queryable surface a Netdata operator/AI assistant cares about,
keep all secrets out of the assistant's view, ship a how-to
extraction system, and pass an automated verification harness.)**

#### Symmetric file structure

Both `docs/netdata-ai/skills/query-netdata-cloud/` and
`docs/netdata-ai/skills/query-netdata-agents/` ship the same set
of per-domain guides for the surfaces shared by both transports:

| Per-domain guide | Cloud | Agent |
|---|---|---|
| SKILL.md | required | required |
| query-metrics.md | required | required |
| query-logs.md | required | required |
| query-topology.md | required | required |
| query-flows.md | required | required |
| query-alerts.md | required | required |
| query-dyncfg.md | required | required |
| query-functions.md | required | required |
| query-nodes.md | required | required |

In addition, the Cloud skill ships three guides that have no
agent equivalent (these surfaces only exist on the Cloud side):

- `query-rooms.md`
- `query-members.md`
- `query-feed.md`

And the Agent skill ships one guide with no Cloud equivalent
(only meaningful agent-side):

- `query-streaming.md`

Each per-domain guide must:

- Open with a one-paragraph summary of the surface.
- Document the v3 endpoint(s) (use v2/v1 only when v3 is missing).
- Show one runnable example using only the documented script
  wrappers (assistant must NOT see tokens; see security section
  below).
- Cross-link the canonical reference docs in
  `<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`,
  `<repo>/src/plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md`,
  `<repo>/src/plugins.d/FUNCTION_UI_SCHEMA.json`,
  `<repo>/src/plugins.d/DYNCFG.md`,
  `<repo>/src/daemon/dyncfg/README.md` where relevant.

Both `SKILL.md` files index every per-domain guide AND the
canonical references AND the how-tos directory.

#### Token-safety architecture (HARD requirement)

The assistant invoking these skills must NEVER see:

- `NETDATA_CLOUD_TOKEN`
- per-agent bearer values (the 36-char UUID returned by
  `bearer_get_token`)
- claim_id values (treat as semi-sensitive identifiers)

To enforce this, the scripts library exposes ONLY high-level
wrappers. Helpers that previously returned a bearer to stdout
(e.g. `agents_resolve_bearer`) MUST be marked internal (named
with a leading underscore, e.g. `_agents_resolve_bearer`) and
their output redirected to in-process variables only -- never
emitted to stdout where the assistant could capture them.

Public wrappers exposed to the assistant:

- `agents_query_cloud <method> <path> [<body-json>]`
  Cloud-side. Reads `NETDATA_CLOUD_TOKEN` from `.env` internally,
  adds `Authorization: Bearer ...` header, runs curl, prints
  ONLY the response body. stderr shows the curl invocation with
  `<CLOUD_TOKEN>` masked.
- `agents_query_agent <node> <method> <path> [<body-json>]`
  Direct-agent-side. Resolves bearer internally (cache or mint
  via cloud), routes through `${AGENT_EVENTS_HOSTNAME}` (or any
  reachable host), adds `X-Netdata-Auth: Bearer ...` header
  internally, prints ONLY the response body. stderr shows the
  curl invocation with `<AGENT_BEARER>` masked.
- Per-surface convenience wrappers (one per query-*.md guide)
  that take typed arguments and forward to the above (e.g.
  `agents_query_function <node> <function-name> <body-json>`).

**No public wrapper may print a token, bearer, or claim_id to
stdout under any circumstance, including error paths.** This is
verified by a pre-commit unit test that drives every public
wrapper with a fake token and asserts the token bytes never
appear in captured stdout.

#### How-tos directory (live, indexed)

Each skill ships a `how-tos/` subdirectory:

- `<skill>/how-tos/INDEX.md` -- one-line index of every how-to,
  ordered by topic. Indexed from `SKILL.md`.
- `<skill>/how-tos/<slug>.md` -- one file per how-to. Each
  documents: the question being answered, the steps taken,
  which wrappers were called, the expected output shape, and
  any gotchas.

Rule baked into both `SKILL.md` files (and into AGENTS.md):
**when an assistant has to perform analysis to answer a question
that is not already covered by a how-to, the assistant must add
a new how-to before completing the task.** The new how-to gets
committed in the same PR as the analysis.

#### Verification harness (DEFERRED to SOW-0006)

Per user direction 2026-05-03: "the evaluation does not need to
be done now". The full Sonnet-driven verification harness (test
runner, grading rubric, automated how-to extraction prompts)
moves to its own SOW (`SOW-0006-20260503-skill-verification-
harness.md`, in `pending/`). It will validate this SOW's
deliverables and any future skill, so it has independent value.

This SOW seeds the inputs the harness will consume:

- `<skill>/verify/questions.md` -- the seed list of validation
  questions. Both skills ship this file. Includes (at minimum)
  the user-supplied questions: hardware specs of a known node;
  OS; parent-or-child status; list of streamed-children if
  parent; vnodes; failed jobs; whether nvidia DCGM is monitored
  and at what frequency; PID with biggest memory consumption +
  dashboard category; last agent status-file log; plus 6+
  further questions covering alerts, logs, topology, flows,
  dyncfg, members, rooms, feed.

The harness implementation (run.sh, grader rubric, score
collation) is SOW-0006's deliverable, not SOW-0010's.

#### Pre-existing acceptance criteria (carry-over)

- Both relative symlinks at `.agents/skills/<name>` resolve to
  the corresponding `docs/netdata-ai/skills/<name>` directory.
- `AGENTS.md` "Project Skills Index" lists both public skills
  with their one-line triggers and the symlink path.
- `AGENTS.md` documents the public-skill convention.
- Sensitive-data gate: every committed file passes the
  pre-commit grep from
  `<repo>/.agents/sow/specs/sensitive-data-discipline.md`.
- All v3 agent paths preferred over v2/v1 (v2/v1 only as
  fallback for older agents).

## Analysis

Sources checked (stage-1, carried forward):

- `<repo>/.env` (per-user, gitignored) -- confirmed shape used by
  the existing skills and the new `AGENT_EVENTS_*` keys for SOW-3.
- `<repo>/AGENTS.md` (= `CLAUDE.md`) -- canonical documentation of
  the `.local/` audit directory convention and the `.env`
  convention.
- `<repo>/.agents/skills/coverity-audit/SKILL.md` and
  `<repo>/.agents/skills/coverity-audit/scripts/_lib.sh`.
- `<repo>/.agents/skills/sonarqube-audit/scripts/_lib.sh`.
- `<repo>/.agents/skills/pr-reviews/scripts/_lib.sh`.
- `<repo>/docs/netdata-ai/skills/query-netdata-cloud-metrics.md`
  (the existing template).
- `<repo>/src/collectors/systemd-journal.plugin/systemd-main.c`,
  `systemd-journal.c`, `systemd-internals.h`, `logs_query_status.h`
  (Function shape).
- `<repo>/src/web/api/functions/function-bearer_get_token.c`
  (agent-side bearer-mint Function and its Cloud-source gate).

Current state -- skill format convention:

- The four legacy private skills already use the directory shape
  (`<repo>/.agents/skills/<name>/SKILL.md` + `scripts/`).
- The one public skill is a flat file
  (`docs/netdata-ai/skills/query-netdata-cloud-metrics.md`). The
  refactor brings it into line with the directory shape.
- Symlink direction: `docs/netdata-ai/skills/<name>/` is canonical;
  `.agents/skills/<name>` becomes a relative symlink pointing at
  `../../docs/netdata-ai/skills/<name>`. This satisfies the user's
  rule "linked (relative) to .agents/skills/".

Current state -- skill helper library shape (mirrored across all
four legacy skills):

- `set -euo pipefail` at the top.
- Color variables defined with `$'\033[...]'`.
- `<prefix>_repo_root()` -- via `git -C "$(dirname
  "${BASH_SOURCE[0]}")" rev-parse --show-toplevel`.
- `<prefix>_load_env()` -- locates `<repo>/.env`, sources via
  `set -a; source; set +a`, validates required vars with
  `: "${VAR:?msg}"`, applies defaults with `: "${VAR:=default}"`.
- `<prefix>_audit_dir()` -- creates
  `<repo>/.local/audits/<topic>/` on demand. Topic name strips any
  `-audit` suffix from the skill name (per AGENTS.md).
- `<prefix>_run` and `<prefix>_run_read` -- print masked curl to
  stderr for transparency, mask the token in argv.
- Skill-specific validators (numeric IDs, ASCII-only, etc.).

Current state -- agent-side primitives (relevant to the agents
skill):

- `systemd-journal` Function name is registered at
  `src/collectors/systemd-journal.plugin/systemd-main.c:79` via
  `rrd_function_add(... ND_SD_JOURNAL_FUNCTION_NAME ...)`; the
  literal name is `systemd-journal`
  (`systemd-journal.c:13`).
- POST body keys (from `logs_query_status.h:8-24`): `help`,
  `after`, `before`, `anchor`, `last`, `query`, `facets`,
  `histogram`, `direction`, `if_modified_since`, `data_only`,
  `__logs_sources`, `info`, `slice`, `delta`, `tail`, `sampling`.
- Response top-level: `facets`, `histogram`, `rows`, `search`,
  `info`.
- `bearer_get_token` Function is gated by
  `user_auth_source_is_cloud(source)`
  (`src/web/api/functions/function-bearer_get_token.c:30`), so
  external HTTP clients cannot call it directly. It is invoked
  over ACLK by Cloud, on behalf of a Cloud-authenticated user.
  Per-agent bearer files live at
  `<netdata-prefix>/var/lib/netdata/bearer_tokens/<token-uuid>.json` for ~24h.

Risks:

- Symlink portability: relative symlinks survive `git clone` and
  most worktree operations on Linux/macOS. Windows / WSL with NTFS
  may not. Acceptable risk -- the project is primarily
  Linux/macOS, and the symlinked skill is also reachable directly
  via its canonical path.
- Bearer-mint endpoint may be undocumented: if the live Swagger
  has no agent-bearer mint, the agents skill must degrade
  gracefully to "local-only transport-b" (i.e. read existing
  `<netdata-prefix>/var/lib/netdata/bearer_tokens/*.json` for the user's own
  workstation only). Stage-1 decision 1B was the recommended
  fallback.
- AGENTS.md churn: changing the project skills index requires
  care so the legacy private skills are not accidentally moved or
  renamed.
- Breaking downstream readers: the existing flat file
  `docs/netdata-ai/skills/query-netdata-cloud-metrics.md` may be
  linked from external docs. The refactor must keep a redirect
  stub (a 1-line file pointing at
  `query-netdata-cloud/query-metrics.md`) to avoid breaking
  inbound links.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The current public skill shelf has a single skill in a flat-file
  shape. As soon as we want a second public skill, we either keep
  using flat files (leading to monoliths) or unify around the
  directory shape now. The user has chosen the directory shape;
  this SOW does the unification and adds the second skill.
- Downstream SOWs (agent-events, learn-site-structure,
  integrations-lifecycle) need both `query-netdata-cloud/query-functions.md`
  and the bearer-mint scripts in `query-netdata-agents/scripts/`.
  Without this SOW, every downstream SOW would re-implement the
  same pieces.

Evidence reviewed:

- See "Sources checked" and "Current state" above. No further
  evidence is needed for the format-normalization piece. The
  bearer-mint piece is blocked on the live Cloud Swagger.

Affected contracts and surfaces:

- New: `<repo>/docs/netdata-ai/skills/query-netdata-cloud/`
  (SKILL.md + 4 supporting docs).
- New: `<repo>/docs/netdata-ai/skills/query-netdata-agents/`
  (SKILL.md + supporting docs + scripts/).
- New: `<repo>/.agents/skills/query-netdata-cloud` (relative
  symlink) and `<repo>/.agents/skills/query-netdata-agents`
  (relative symlink).
- Stub: `<repo>/docs/netdata-ai/skills/query-netdata-cloud-
  metrics.md` becomes a 1-line redirect to the new location to
  preserve existing inbound links.
- New: `<repo>/.local/audits/query-netdata-agents/` writes
  (gitignored, bearer cache + acceptance-test outputs).
- Augmented: `<repo>/AGENTS.md` -- adds the public-skill
  convention paragraph and the index entries.
- No existing surface is broken; the legacy skill keeps a
  redirect.

Existing patterns to reuse:

- `_lib.sh` shape from `coverity-audit/scripts/_lib.sh`.
- Pagination idiom from `sonarqube-audit/scripts/_lib.sh::sq_paginate`.
- Audit-dir / `.local/` convention from AGENTS.md.
- Token-masking idiom from `sonarqube-audit/scripts/_lib.sh::sq_run`.
- The single existing public skill
  `query-netdata-cloud-metrics.md` is the content seed for the
  new `query-metrics.md`.

Risk and blast radius:

- Low for format normalization (additive, with redirect stub).
- Medium for the bearer-mint scripts -- they handle credentials
  and a bug here could leak bearers to logs or `.local/`. Mitigation:
  a single `agents_run` wrapper that forces token masking, plus a
  pre-commit grep that fails if any committed file contains
  `bearer_tokens` or `nd_bearer` blobs, plus a unit-test harness
  that runs the helpers under `set -x` and asserts no token bytes
  appear on stderr.

Sensitive data handling plan:

- This SOW (and every committed artifact it produces) follows the
  spec at `<repo>/.agents/sow/specs/sensitive-data-discipline.md`.
  No literal IPs, hostnames, UUID-shaped IDs, tokens, absolute
  install/user paths, usernames, tenant names, or secrets in any
  committed file. Every reference to such a value is via an
  env-key placeholder (`${KEY_NAME}`) defined in `.env`.
- `.env` is the only place credential and identity VALUES live.
- Bearer cache lives under
  `<repo>/.local/audits/query-netdata-agents/bearers/`;
  gitignored. File mode 0600.
- All log lines that include curl invocations route through
  `agents_run`/`agents_run_read` which masks the cloud token and
  any bearer matched by a regex.
- A one-shot redaction self-test runs as part of stage 2f
  validation: calls `agents_run` with a fake token, checks the
  emitted stderr contains no token bytes.
- Pre-commit verification grep (from the spec) runs on every
  staged change.

Implementation plan:

1. **Stage 1 -- DONE**: investigation captured in this SOW.
2. **Stage 2a -- DONE**: decisions 1, 2, 3 resolved. Cloud
   bearer-mint and function-call endpoints discovered, smoke-
   tested live. Symlink direction A confirmed by user.
3. **Stage 2b -- DONE**: format normalization. Existing flat
   file moved to `query-netdata-cloud/query-metrics.md`. New
   SKILL.md plus per-domain guides
   (`query-logs.md`, `query-topology.md`, `query-flows.md`,
   `query-alerts.md`, `query-dyncfg.md`, `query-functions.md`)
   written. v3 agent paths used everywhere.
4. **Stage 2c -- DONE**: `query-netdata-agents/SKILL.md` written
   with bearer-mint flow described; `scripts/_lib.sh` ships
   `agents_resolve_bearer`, `agents_call_function`,
   `agents_netdata_prefix`. Both relative symlinks created and
   resolved.
5. **Stage 2d -- DONE**: AGENTS.md updated with public-skill
   convention paragraph + public-skill index entries.

The remaining stages cover the second-expansion scope:

6. **Stage 2e -- token-safety architecture rework**:
   - Rename `agents_resolve_bearer` to `_agents_resolve_bearer`
     (internal, never returns to stdout).
   - Add `agents_query_cloud <method> <path> [<body>]` and
     `agents_query_agent <node> <method> <path> [<body>]` public
     wrappers that handle auth internally and emit only the
     response body to stdout.
   - Add per-surface convenience wrappers (one per query-*.md
     guide).
   - Add a unit test that drives every public wrapper with a
     fake token and asserts the token bytes never appear on
     captured stdout.
7. **Stage 2f -- per-domain guides for the agent skill**:
   For `query-netdata-agents/`, write `query-metrics.md`,
   `query-logs.md`, `query-topology.md`, `query-flows.md`,
   `query-alerts.md`, `query-dyncfg.md`, `query-functions.md`,
   `query-nodes.md`, `query-streaming.md`. Each uses the
   token-safe wrappers from stage 2e in every example.
8. **Stage 2g -- per-domain guides for the cloud skill**:
   For `query-netdata-cloud/`, add `query-nodes.md`,
   `query-rooms.md`, `query-members.md`, `query-feed.md`. Each
   uses the token-safe wrappers; covers v3 endpoints (or
   documented v2 fallbacks).
9. **Stage 2h -- SKILL.md re-indexing**: both `SKILL.md` files
   updated to list every per-domain guide AND the canonical
   reference docs AND the how-tos directory.
10. **Stage 2i -- how-tos infrastructure**:
    - Create `<skill>/how-tos/INDEX.md` with the format and
      authoring rules (one how-to per file, slug, question,
      steps, wrappers used, expected output, gotchas).
    - Seed `INDEX.md` with the user-supplied question list (one
      stub per question, marked TODO until a how-to is
      authored). Stub how-tos are NOT a valid close state for
      SOW-0006 but ARE the close state for this SOW (the
      catalog is the deliverable; populating it happens during
      verification).
    - Document the rule "if you analyze, you author a how-to"
      in both `SKILL.md` files AND in `AGENTS.md` so future
      assistants honor it.
11. **Stage 2j -- seed verify/questions.md**: write the seed
    question list for both skills (the user's 10+ questions plus
    coverage for every per-domain guide). The harness that
    consumes them is SOW-0006.
12. **Stage 2k -- final validation**:
    - Run every public wrapper end-to-end (Cloud + Agent
      transports).
    - Run the no-token-on-stdout unit test.
    - Run shellcheck on every script.
    - Run the spec's pre-commit grep on every changed file.
    - Confirm both symlinks resolve.
13. **Stage 2l -- close**: status `completed`, move to `done/`,
    single commit covering skill expansion + AGENTS.md update +
    SOW close + SOW-0006 (pending) creation.

Validation plan:

- Stage 1: documentation read-through (this SOW).
- Stage 2: real-use evidence (one Cloud-proxied function call,
  one direct-agent function call after bearer mint, one redaction
  self-test); shellcheck on every script; pre-commit grep against
  token-shaped strings; confirm symlinks resolve via
  `git ls-files --stage` and `readlink -f`.

Artifact impact plan:

- AGENTS.md: add public-skill convention paragraph + public-skill
  index entries.
- Runtime project skills: the two new skills are public
  (`docs/netdata-ai/skills/`) but reachable from
  `.agents/skills/` via relative symlinks; both names trigger on
  AI-skill router queries.
- Specs: not required at this stage. Stage 2 may add a short
  spec under `.agents/sow/specs/skills-format.md` capturing the
  convention if useful for future reviewers.
- End-user/operator docs: the new SKILL.md and supporting docs
  ARE end-user-facing (anyone using AI assistants with this
  repo). They live in `docs/netdata-ai/skills/`.
- End-user/operator skills: the two new skills.
- SOW lifecycle: open in `pending/`; moves to `current/` once
  decisions 1-3 are recorded; moves to `done/` after stage 2g.

Open-source reference evidence:

- Not consulted at stage 1. Stage 2 may consult upstream
  observability projects (e.g. Grafana, Datadog) for reference
  patterns on auth-token caching helpers if useful, but no
  external reference is required to proceed. The user's
  workstation has a local mirror tree available for grep / read.

Open decisions:

- See "Implications And Decisions" below. Implementation cannot
  begin until decisions 1-3 are answered.

## Implications And Decisions

Decisions 1 and 2 are RESOLVED (2026-05-03) by reading the
cloud-* source code (`cloud-frontend`, `cloud-spaceroom-service`,
`cloud-charts-service`) and live smoke-testing against the
production Cloud API. Decision 3 still needs the user's explicit
confirmation. Findings recorded inline below.

1. **Cloud REST endpoint that mints an agent bearer from a
   cloud token.** RESOLVED 2026-05-03 (option **A**). Endpoint
   exists; smoke-tested live.
   - Path: `GET ${NETDATA_CLOUD_HOSTNAME}/api/v2/bearer_get_token`
   - Required query params: `node_id`, `machine_guid`, `claim_id`
   - Auth header: `Authorization: Bearer ${NETDATA_CLOUD_TOKEN}`
   - Response body keys: `bearer_protection` (bool),
     `expiration` (numeric -- format TBD; smoke-test value
     rendered as 1970-01-01 when interpreted as Unix seconds,
     so likely milliseconds or an ISO-string variant; stage 2b
     must verify), `mg` (echo of machine_guid), `status`,
     `token` (36-char UUID, the bearer).
   - Cloud-side handler:
     `cloud-spaceroom-service/http/transport_http.go:355`
     (`makeGetAgentBearerToken`); route registered at
     `cloud-spaceroom-service/http/endpoints_agent.go:179`.
     Permission gate: `PermissionSpaceRead`; node must be
     `reachable`; agent-side delegation invokes
     `bearer_get_token` Function via ACLK.
   - Frontend cache pattern:
     `cloud-frontend/src/domains/nodes/useAgentBearer.js`.
     Storage keyed by `machine_guid`. Refresh trigger:
     `expiration < now + 3600 seconds` (1-hour buffer before
     expiry).
   - The `claim_id` is read from the agent's `/api/v3/info`
     at `.agents[0].cloud.claim_id`, OR (with shell access)
     from the agent host's
     `<netdata-prefix>/var/lib/netdata/cloud.d/claimed_id`.
   - The minted bearer is sent in subsequent direct-agent
     calls as `X-Netdata-Auth: Bearer <token>` (NOT
     `Authorization: Bearer <token>`).

2. **Cloud REST endpoint that invokes a Function on a node by
   uuid.** RESOLVED 2026-05-03 (option **A**). Endpoint exists;
   smoke-tested live.
   - Path: `POST ${NETDATA_CLOUD_HOSTNAME}/api/v2/nodes/{nodeId}/function?function={functionName}`
   - Auth header: `Authorization: Bearer ${NETDATA_CLOUD_TOKEN}`
   - Optional header: `X-Transaction-Id: <uuid>` (correlation
     only; not required).
   - Request body: the agent-side Function payload (e.g. for
     `systemd-journal`: `{"info": true}`, or a query body with
     `after`, `before`, `last`, `query`, `facets`,
     `histogram`, `__logs_sources`, etc.). Optional top-level
     `timeout` (ms) and `last` (page size).
   - Response body: JSON (NOT streaming). Top-level keys for
     `systemd-journal info=true`: `_request`, `accepted_params`,
     `has_history`, `help`, `pagination`, `required_params`,
     `show_ids`, `status`, `type`, `v`, `versions`.
   - Cloud-side: `cloud-charts-service/http/http.go:146`
     (`nodePathProxy` -> dispatches via ADC to the agent).
   - Companion listing endpoint:
     `POST ${NETDATA_CLOUD_HOSTNAME}/api/v3/spaces/{spaceID}/rooms/{roomID}/functions`
     with body
     `{"scope":{"nodes":[...]},"selectors":{"nodes":["*"]}}`
     returns the list of available functions per node.
     Service: `cloud-charts-service/http/http.go:135`
     (`scopeFunctions` handler at
     `cloud-charts-service/http/http.go:1036`).
   - Smoke test 2026-05-03: Cloud function call returned 200
     with valid metadata; the same call against the agent
     directly (using a freshly-minted bearer) returned an
     identical 2748-byte response. Both transports work
     end-to-end.

3. **Symlink direction confirmation.** The user wrote: skills
   live in `docs/netdata-ai/skills/` and are linked from
   `.agents/skills/` with relative symlinks. Confirming reading:
   - A. Canonical path: `docs/netdata-ai/skills/<name>/`.
     Relative symlink: `.agents/skills/<name>` ->
     `../../docs/netdata-ai/skills/<name>`. *(matches user's
     wording)*
   - B. Other direction: `.agents/skills/<name>/` canonical,
     `docs/netdata-ai/skills/<name>` symlink to it.
   - C. Bidirectional / something else.
   - **Recommendation:** **A**, the natural reading of the
     directive and the only one consistent with "skills are
     accessible by local agents too".

## Plan

Pending decisions 1-3. After they are answered:

1. Update this SOW with the decisions; move to `current/` as
   `Status: in-progress`.
2. Implement per stage 2b-2f.
3. Validate per stage 2f.
4. Close per stage 2g.

## Execution Log

### 2026-05-03

- Stages 2e-2k (2026-05-03 late evening). Token-safety rework
  shipped: `_lib.sh` now exposes `agents_query_cloud`,
  `agents_query_agent`, `agents_call_function` as token-safe
  public wrappers; bearer / cloud-token / claim_id never reach
  stdout; `_agents_resolve_bearer` rewritten to return via bash
  nameref. `agents_selftest_no_token_leak` PASSES (drives
  wrappers with a sentinel token, asserts sentinel never appears
  on captured stdout). Per-domain agent guides written:
  `query-functions.md`, `query-logs.md`, `query-topology.md`,
  `query-flows.md`, `query-alerts.md`, `query-dyncfg.md`,
  `query-metrics.md`, `query-nodes.md`, `query-streaming.md`.
  Per-domain cloud-only guides written: `query-nodes.md`,
  `query-rooms.md`, `query-members.md`, `query-feed.md`. The
  `feed` endpoint discovered: `POST /api/v1/feed/search` served
  by the separate `cloud-feed-service`, snake_case `space_id`
  body field, response wraps Elasticsearch hits in
  `results.hits.hits[]._source` with ECS v8.4 + Netdata-specific
  envelope. The `members` endpoint:
  `GET /api/v2/spaces/{sp}/members`. The `rooms` endpoint:
  `GET /api/v2/spaces/{sp}/rooms`. Both SKILL.md files
  re-indexed with all per-domain guides + canonical references
  + how-tos + verify pointers. The "if you analyze, you author
  a how-to" rule baked into SKILL.md, AGENTS.md, and both
  how-tos/INDEX.md files. Seed verify/questions.md written for
  both skills (Costa's user-supplied 23 + 19 questions covering
  identity / hardware / OS / streaming / vnodes / collectors /
  alerts / logs / topology / flows / dyncfg / members / rooms /
  feed / token-safety self-test). Stage 2l (close) pending.
- Stage-1 close-out (2026-05-03 evening). User added
  `NETDATA_REPOS_DIR` and `NETDATA_CLOUD_HOSTNAME` to `.env`.
  Live API probes validated all five existing AGENT_EVENTS_*
  keys: `GET /api/v2/spaces` -> `/rooms` -> `POST .../nodes`;
  matched node state `reachable`, version `v2.9.0-5-nightly`.
  ssh path validated (passwordless, journalctl
  --namespace=${AGENT_EVENTS_HOSTNAME} returned real data).
  Direct-agent path validated (port 19999 reachable,
  `/api/v3/info` returns 200 unauthenticated; functions
  return 412 without bearer). Read the cloud-* sources at
  `${NETDATA_REPOS_DIR}/cloud-frontend`,
  `${NETDATA_REPOS_DIR}/cloud-spaceroom-service`,
  `${NETDATA_REPOS_DIR}/cloud-charts-service` to discover
  the bearer-mint endpoint and the function-call endpoint.
  Both endpoints smoke-tested live (200 OK, valid responses;
  bearer minted; direct-agent call after bearer-mint
  returned identical metadata to Cloud-proxied call).
  Decisions 1 and 2 RESOLVED. Decision 3 (symlink direction)
  still pending. Raw artifacts saved under
  `<repo>/.local/audits/query-netdata-cloud/probe/`.
  Removed redundant keys `AGENT_EVENTS_IP` and
  `AGENT_EVENTS_JOURNAL_NAMESPACE` after user noted that the
  existing `AGENT_EVENTS_HOSTNAME` value covers ssh/HTTP/
  journal-namespace roles (quadruple-duty).
- Stage 1 investigation completed (originally as part of the old
  SOW-2 "agent-events triage skill"). Confirmed `.env` + skill
  `_lib.sh` pattern across coverity-audit / sonarqube-audit /
  pr-reviews / graphql-audit. Confirmed `systemd-journal`
  Function shape and `bearer_get_token` Cloud-source gate in
  source.
- Scope expanded by user (format normalization, query-netdata-
  cloud refactor, query-netdata-agents new skill).
- User chose 4-SOW split. The agent-events triage skill moved
  to SOW-0003; documentation-pipeline skills to SOW-0004;
  mirror-netdata-repos skill to SOW-0005. This SOW (SOW-0010) was
  rescoped to skill infrastructure and renamed to "Netdata
  query skills infrastructure". Old filename
  `SOW-0010-20260503-agent-events-skill.md` removed; new
  filename `SOW-0010-20260503-netdata-query-skills-
  infrastructure.md`.
- Post-close (2026-05-04): user split the original
  doc-pipeline SOW into documentation (SOW-0004 rescoped to
  `learn-site-structure`) and integrations (new SOW-0007
  `integrations-lifecycle`). Cross-references updated for
  navigation accuracy.

## Validation

Acceptance criteria evidence:

- Symmetric file structure delivered: both
  `docs/netdata-ai/skills/query-netdata-cloud/` and
  `docs/netdata-ai/skills/query-netdata-agents/` ship
  `SKILL.md` + 8 shared per-domain guides
  (`query-{metrics,logs,topology,flows,alerts,dyncfg,functions,nodes}.md`).
  Cloud adds `query-{rooms,members,feed}.md`; agent adds
  `query-streaming.md`. Both ship `how-tos/INDEX.md` and
  `verify/questions.md`.
- Token-safety architecture delivered:
  `agents_query_cloud`, `agents_query_agent`,
  `agents_call_function` are the public wrappers; internal
  helpers renamed with leading underscore and return token
  bytes via bash namerefs only.
  `agents_selftest_no_token_leak` PASSES on every run
  (verified live: bearer-mint, cloud call, direct-agent call,
  bearer-cache hit -- all confirm zero token bytes on
  captured stdout).
- Both relative symlinks at `.agents/skills/` resolve to the
  corresponding `docs/netdata-ai/skills/` directory.
- `AGENTS.md` updated with the public-skill convention paragraph,
  the public-skill index entries, the token-safety contract
  paragraph, and the live how-tos catalog rule.
- v3 agent paths used everywhere; v2/v1 only as fallback for
  pre-v2 agents (alerts section explicitly notes this).
- Cloud verification covers the eleven domains the SOW
  required; agent verification covers the nine domains the SOW
  required. Both `verify/questions.md` files seeded with the
  user-supplied questions plus per-domain coverage.

Tests or equivalent validation:

- shellcheck on `_lib.sh`: clean (no findings).
- `agents_selftest_no_token_leak`: `[PASS]`.
- Live smoke tests against production:
  - `agents_query_cloud GET /api/v2/accounts/me` -- 200,
    response delivered, zero cloud-token bytes in stdout.
  - `agents_query_agent ... POST /api/v3/function?function=systemd-journal '{"info":true}'`
    -- 200, 2748-byte response, zero cloud-token bytes and
    zero bearer bytes in stdout.
  - Agent v3 alert paths: `/api/v3/alerts`,
    `/api/v3/alert_transitions`, `/api/v3/alert_config` --
    all 200.
  - Cloud-side: `/api/v2/spaces`, `/api/v2/accounts/me`,
    `/api/v2/spaces/{sp}/rooms`, `/api/v3/spaces/{sp}/rooms/
    {rm}/nodes`, `/api/v2/spaces/{sp}/members`,
    `/api/v1/feed/search`, `/api/v3/spaces/{sp}/rooms/{rm}/
    alerts*` -- all 200.
  - Agent direct: `/api/v3/info`, `/api/v3/config?action=tree`,
    `/api/v3/function?function=topology:snmp`,
    `/api/v3/function?function=flows:netflow` (where the
    collector is enabled) -- all 200.

Real-use evidence:

- The bearer cache works across calls: first call mints, second
  call hits cache (verified by stable token output between
  invocations within the 2-hour window).
- The two transports return identical metadata for
  `systemd-journal info=true` (modulo timestamps), confirming
  the agent and the Cloud proxy expose the same Function
  payload shape.
- The discovered Cloud feed endpoint
  (`POST /api/v1/feed/search`) returns 42853 hits over the
  user's seed query; verified the snake_case `space_id` body
  field is required (camelCase `spaces[].id` is rejected with
  400).

Reviewer findings:

- Self-review caught: invented "alert function" terminology
  and partial query-alerts.md (rewritten with all 11 endpoints,
  smoke-tested).
- Self-review caught: invented function names (`top`,
  `aclk-state`, `ml-models`, `windows-events`, `streaming`).
  Replaced with the live-verified list pulled from the
  agent-events node.
- User-flagged: missing topology + flow Function families.
  Added query-topology.md and query-flows.md with
  source-verified payload shapes.
- User-flagged: missed FUNCTION_UI_REFERENCE.md /
  FUNCTION_UI_DEVELOPER_GUIDE.md / FUNCTION_UI_SCHEMA.json /
  DYNCFG.md. Added explicit references in SKILL.md and the
  per-domain guides.
- User-flagged: v2 agent paths. Switched all agent-direct
  paths to v3 (v2/v1 only as fallback).
- User-flagged: 4-family taxonomy was a fabrication. Replaced
  with the canonical 2-class taxonomy from
  FUNCTION_UI_REFERENCE.md (Simple Table + Log Explorer);
  topology and flows documented as custom Functions building
  on the same envelope.
- User-flagged: assistant must never see tokens. Reworked
  `_lib.sh` with internal/public split and shipped a
  no-leak self-test.

Same-failure scan:

- Spec discipline grep on every committed file: zero
  violations (the only `deadbeef-...-...-...-...-...` UUID
  in `_lib.sh` is the deliberate self-test sentinel and is
  not a real credential).

Sensitive data gate:

- Pre-commit grep ran clean over every file touched by this
  SOW: zero UUID-shaped IDs (except the test sentinel),
  zero IPv4 literals to specific hosts (only loopback
  `127.0.0.1` examples and a `<YOUR_FOCUS_DEVICE_IP>`
  placeholder), zero forbidden absolute paths
  (Netdata defaults `/var/lib/netdata`, `/etc/netdata` are
  explicitly allowed by the spec).
- Cloud REST host is env-keyed via `${NETDATA_CLOUD_HOSTNAME}`
  in scripts; appears as the literal `app.netdata.cloud` only
  in user-facing curl examples (allowed for public Netdata-org
  sites in role-descriptive prose).
- Token-safe wrappers verified: `agents_selftest_no_token_leak`
  PASSES; `agents_query_cloud` and `agents_query_agent` both
  emit zero cloud-token bytes and zero bearer bytes on captured
  stdout.

Artifact maintenance gate:

- AGENTS.md: updated with public-skill convention paragraph,
  the public-skill index, the token-safety contract paragraph,
  and the how-tos catalog rule.
- Runtime project skills: two new public skills shipped at
  `docs/netdata-ai/skills/query-netdata-{cloud,agents}/`,
  reachable from `.agents/skills/` via relative symlinks.
- Specs: `<repo>/.agents/sow/specs/sensitive-data-discipline.md`
  shipped (separate concern; underwrites this SOW + future
  SOWs). No additional spec required for the skill convention
  -- documented in AGENTS.md.
- End-user/operator docs: the two new skill bundles ARE the
  end-user-facing docs.
- End-user/operator skills: ditto.
- SOW lifecycle: moved from `pending/` -> `current/` ->
  `done/` per the framework. Status `completed`. Verification
  harness deferred to `SOW-0006` per user direction; SOW-0006
  shipped as a stub in `pending/`.

Specs update:

- New spec `<repo>/.agents/sow/specs/sensitive-data-discipline.md`
  (the rule that underwrites this SOW's discipline gate).

Project skills update:

- Two new public skills under `docs/netdata-ai/skills/` with
  relative symlinks from `.agents/skills/`.

End-user/operator docs update:

- The two new skill bundles ARE the docs update.

End-user/operator skills update:

- The two new skill bundles ARE the skills update.

Lessons:

- **Verify before documenting.** The first draft of
  query-alerts.md and query-functions.md contained invented
  terms ("alert functions") and invented Function names
  (`top`, `aclk-state`, `ml-models`). Source-verification
  pass and live smoke-testing caught both. Lesson: every
  endpoint table must be smoke-tested before commit; every
  Function name list must be pulled from the live listing
  endpoint.
- **Public Swagger is incomplete.** `app.netdata.cloud/api/docs/`
  documents only 7 paths. Real cloud endpoints
  (`bearer_get_token`, function-call proxy, alert endpoints,
  feed search) live across 4+ microservices and were
  discovered by reading the cloud-* sources at
  `${NETDATA_REPOS_DIR}/cloud-*/`. Lesson: when Swagger is
  thin, read source.
- **Cloud-side `expiration: 0` is real.** The bearer mint
  endpoint returns `expiration: 0` on the production cloud,
  which would force re-mint on every call if naively
  followed. The cache logic now stamps `_cached_at` and falls
  back to a 2-hour mint window when `expiration` is 0. Agents
  actually issue ~3-hour TTL bearers, so 2 hours leaves a
  safety margin.
- **zsh vs bash compat.** `BASH_SOURCE[0]` warning on zsh
  was noisy. Solution: capture `_agents_lib_self` at source
  time using a `ZSH_VERSION`/`BASH_VERSION` switch with
  `eval` for the zsh-only `${(%):-%x}` syntax.
- **Token-safety needs architectural enforcement, not
  discipline.** The first draft had `agents_resolve_bearer`
  return the bearer to stdout; refactoring to bash namerefs
  + leading-underscore "internal" naming + a no-leak
  self-test is what makes the contract verifiable.

Follow-up mapping:

- Verification harness (Sonnet test runner + grading rubric +
  how-to extraction prompt loop): tracked in
  `<repo>/.agents/sow/pending/SOW-0006-20260503-skill-verification-harness.md`.
- `query-agent-events` private skill (consumes the wrappers
  delivered here): tracked in
  `<repo>/.agents/sow/pending/SOW-0003-20260503-query-agent-events-skill.md`.
- `learn-site-structure` private skill: tracked in
  `<repo>/.agents/sow/pending/SOW-0004-20260503-learn-site-structure-skill.md`.
- `integrations-lifecycle` private skill: tracked in
  `<repo>/.agents/sow/pending/SOW-0007-20260504-integrations-lifecycle-skill.md`.
- `mirror-netdata-repos` private skill: tracked in
  `<repo>/.agents/sow/pending/SOW-0005-20260503-mirror-netdata-repos-skill.md`.
- The how-tos catalog stubs in both skills are deliberately
  unfilled at close: they get populated when the verification
  harness (SOW-0006) drives Sonnet through `verify/questions.md`
  and prompts for new how-tos on misses. The "if you analyze,
  you author a how-to" rule is durable; SOW close does not
  require pre-populated how-tos beyond the stub catalog.

## Outcome

Delivered. Two symmetric public skills ship at
`docs/netdata-ai/skills/query-netdata-{cloud,agents}/`, each with
SKILL.md + per-domain guides covering every queryable Netdata
surface, token-safe wrappers in `query-netdata-agents/scripts/
_lib.sh` (verified by self-test), live `how-tos/INDEX.md`
catalogs, seed `verify/questions.md` lists, and relative
symlinks from `.agents/skills/`. AGENTS.md updated.
`<repo>/.agents/sow/specs/sensitive-data-discipline.md` shipped.
Verification harness deferred to SOW-0006 per user direction.

## Lessons Extracted

See "Lessons" inside the Validation section above.

## Followup

See "Follow-up mapping" inside the Validation section above.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
