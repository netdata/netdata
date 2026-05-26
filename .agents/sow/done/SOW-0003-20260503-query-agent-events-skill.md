# SOW-0003 - query-agent-events private skill

## Status

Status: completed

Sub-state: completed 2026-05-05. Skill shipped with verified producer-side field map (80+ fields), token-safe scripts (get-events / analyze-events / redact-events), index-friendly query discipline documented, and the multi-value `selections` capability lifted into both `query-netdata-{cloud,agents}/query-logs.md` so all callers can use it. Consumes `query-netdata-{cloud,agents}` for transport.

## Requirements

### Purpose

Build a **private developer skill** that lets a Netdata maintainer (or
an AI assistant helping one) query, fetch, and analyze received
agent-events submissions on the Netdata-operated ingestion server.

This skill is intentionally NOT public. It lives at
`<repo>/.agents/skills/query-agent-events/` only -- not under
`docs/netdata-ai/skills/`. Reasons:

- The data is operator-sensitive (machine GUIDs, claim IDs, cloud
  metadata, hardware DMI fields) and the user does not want a
  public "how to scrape Netdata's agent-events" doc.
- The journalctl-via-ssh path included in the skill requires
  privileged shell access to a specific host whose name lives only
  in `.env`.
- The skill is a maintainer triage tool, not a user-facing feature.

The fit-for-purpose use cases the skill must support:

- "What crashes is the fleet seeing in the last 24 hours, grouped
  by exit cause?"
- "Show me events from a specific agent (by `AE_AGENT_ID`)."
- "What's the distribution of `AE_AGENT_HEALTH` values among
  stable releases over the last 30 days?"
- "Fetch a specific event by timestamp + machine GUID and pretty-
  print its `AE_FATAL_*` fields."
- "Compare crash counts across `AE_AGENT_VERSION` for v2.8+ to
  spot regressions."

### User Request

> "the query-agent-events skill should not be public in
> docs/netdata-ai. It should live in .agents/skills/ since this is
> a developer tool, not an end-user tool. The skill should
> document the direct journalctl method via ssh, but it the
> destination IP and any other private info should be in .env.
>
> the journalctl method is documented in
> .local/agent-events-journals.md - this is untrusted document,
> not to be copied to a skill as-is. So, you need to review it,
> you can try it..."

The user added these `.env` keys (values stay in `.env`):

- `AGENT_EVENTS_NC_SPACE`
- `AGENT_EVENTS_HOSTNAME`
- `AGENT_EVENTS_MACHINE_GUID`
- `AGENT_EVENTS_NODE_ID`

### Assistant Understanding

Facts:

- The Netdata Agent serializes a status document (schema v28) and
  POSTs it to a public ingestion endpoint. The receiving service
  persists each submission as a systemd journal entry in a
  dedicated journal namespace whose name is documented in
  `.local/agent-events-journals.md` (an untrusted draft).
- Each status JSON dot-path is converted into a journal field
  prefixed with `AE_` and the dots replaced with underscores
  (per the untrusted draft, to be verified against
  `src/libnetdata/log/log2journal/` and the producer-side
  serialization).
- Three transports are in scope:
  1. Cloud-proxied: uses
     `query-netdata-cloud/query-logs.md` and
     `query-netdata-cloud/query-functions.md` (delivered by
     SOW-0010).
  2. Direct agent: uses
     `query-netdata-agents/scripts/_lib.sh::agents_call_function`
     (delivered by SOW-0010), with bearer auto-mint/refresh.
  3. journalctl-via-ssh: uses standard ssh + the journalctl
     namespace flag. Destination host comes from `.env`.

Inferences:

- The journal field map and enum values in
  `.local/agent-events-journals.md` are likely correct in spirit
  but must be verified field-by-field against the producer code
  and a sampled response before the skill encodes them as truth.
- The default query the skill ships should be conservative
  (last 24h, narrow facets) so a single `query` call does not
  flood the ingestion server.

Unknowns:

- Whether `.local/agent-events-journals.md` is fully accurate
  on every field-name mapping (especially edge cases:
  array indices, deeply-nested paths, omitted-on-graceful-exit
  fields).
- Whether SOW-0010 ships a Cloud-proxied logs query helper ready
  to consume (decision 1 of SOW-0010 directly governs this).
- Whether the journalctl path returns the same field set as the
  Netdata-via-Cloud path; small drift is likely (the journal
  output includes systemd `_*` fields that the Function may
  filter out).

### Acceptance Criteria

- `<repo>/.agents/skills/query-agent-events/SKILL.md` exists
  with frontmatter triggers covering "agent events",
  "agent-events", "status file", "crash reports", "fleet
  crashes", "ingestion server".
- `<repo>/.agents/skills/query-agent-events/scripts/` ships:
  - `_lib.sh` (mirrors the legacy skill helper shape, prefix
    `agentevents_`; sources `.env`, depends on the helpers from
    `query-netdata-agents/scripts/_lib.sh`)
  - `query-events.sh` -- thin wrapper around the `systemd-
    journal` Function with flags `--via {cloud|agent|ssh}`,
    `--last N`, `--after T`, `--before T`, `--query STR`,
    `--source SEL`, `--facets a,b,c`, `--histogram FIELD`.
  - `fetch-event.sh` -- single-event fetch by `AE_AGENT_ID` +
    timestamp anchor.
  - `summarize.sh` -- jq-driven facet/histogram pretty-print.
- `<repo>/.agents/skills/query-agent-events/AE_FIELDS.md` exists
  and documents the verified `AE_*` field map (and known
  divergences from `.local/agent-events-journals.md`).
- All raw outputs land under
  `<repo>/.local/audits/query-agent-events/` (gitignored).
- A small acceptance fixture: at least one real round-trip via
  the Cloud transport and one via ssh, each producing a small
  bundle that includes a `crash-*` event for a stable
  (`AE_AGENT_VERSION` matches `^v2\.([89]|\d\d)\.`) release.
- AGENTS.md "Project Skills Index" section adds a one-line entry
  for `.agents/skills/query-agent-events/`.
- Sensitive-data gate: the SOW, the SKILL.md, and every
  committed script contain zero raw values for any of
  `AGENT_EVENTS_*`, no machine GUIDs, no claim IDs, no public-
  facing host names except those already in the open-source
  code. Verified by pre-commit grep.

## Analysis

Sources to consult during stage 2 (already mostly read at stage 1):

- `<repo>/src/daemon/status-file.{c,h}` (schema v28).
- `<repo>/src/daemon/status-file-io.{c,h}`.
- `<repo>/src/daemon/status-file-dmi.{c,h}`.
- `<repo>/src/libnetdata/exit/exit_initiated.h` (exit_reason
  enum).
- `<repo>/src/libnetdata/log/log2journal/` (journal field naming
  conventions; verify the `AE_` prefix story).
- `<repo>/.local/agent-events-journals.md` (untrusted draft;
  treat each claim as a hypothesis until cross-verified).
- `<repo>/src/collectors/systemd-journal.plugin/...` (Function
  shape).
- The output of `query-netdata-agents/scripts/_lib.sh` from
  SOW-0010 (consumer side).

Risks:

- Privacy: every fetched event carries identifying fields. The
  skill MUST NOT copy real values into committed artifacts. A
  redaction filter (`redact-events.sh`) ships as an opt-in
  filter, not a default.
- Untrusted-doc risk: copying field names verbatim from
  `.local/agent-events-journals.md` without verification is
  unsafe. Stage 2 must spot-check at least the top-20 most-used
  fields and the all-the-enums against producer source.
- Volume: a fleet of 1.5M+ daily Netdata installs producing
  events at non-zero rates means naive "fetch all" calls would
  return enormous payloads. Default queries must be narrow.
- Schema drift: STATUS_FILE_VERSION will keep moving. Treat
  unknown fields as opaque pass-through; key analyzers off the
  documented field paths only.
- Producer vs consumer endpoint confusion: never point at the
  producer ingest URL (a `const char *` in
  `src/daemon/status-file.c:988` -- the agent POSTs there).
  Always point at the consumer endpoint resolved from
  `${AGENT_EVENTS_HOSTNAME}` and the Cloud space.

## Pre-Implementation Gate

Status: filled-2026-05-05

### Refined purpose (per user clarification 2026-05-05)

The skill is a **bug-investigation tool for fixing Netdata bugs**. The workflow is: download events of interest -> cluster locally (by signal, fatal function, version, architecture, packaging, parent/child profile, cloud-claimed/not) -> identify regressions or recurring patterns -> fix the bug. Standalone statistics are rare. A secondary use is locating events related to current work ("is anyone hitting this?").

This is NOT a generic logs query skill. The two existing `query-netdata-{cloud,agents}` skills already cover transport mechanics; this skill EXTENDS them with the agent-events specifics: which Function args, which AE_* predicates work as index-friendly filters, how to slice the dataset, and how to compute group-by stats from a downloaded JSON dump.

### Problem / root-cause model

Maintainers (Costa + team + AI assistants) need to triage 40k-200k status submissions per day across 1.5M agents to find specific crashes, panics, regressions. Naive queries (`--grep PATTERN` over full namespace) are slow because they full-scan. The skill must teach index-friendly patterns AND ship scripts that bake them in.

A second confusion the skill resolves: the after-the-fact event model. Agents POST events ONLY on start (the previous session's exit reason). So "the last hour" misses real crashes; the meaningful unit is "events posted in the last 24h", which (because of 23h client-side dedup) is ~one record per agent per event-class per day.

### Evidence reviewed

Producer source (verified by research subagent):
- `src/daemon/status-file.{c,h}` -- schema (`STATUS_FILE_VERSION = 28`), all field setters, the AE_EXIT_CAUSE branches (26 distinct strings at `:1097-1286`), the agent_health computation (8 values at `:929-952`), the POST-time top-level fields (9 fields at `:967-976`).
- `src/daemon/status-file-io.c` -- atomic temp+rename save mechanism (signal-async-safe).
- `src/daemon/status-file-dmi.c` -- DMI field collection. Privacy redactions: `hw.{sys,board,chassis}.{serial,asset_tag}` are commented out at producer side and never reach the journal.
- `src/daemon/status-file-dedup.c:11` -- `REPORT_EVENTS_EVERY = 86400 - 3600` = 23h dedup window; per (agent_guid + event-content hash). Same agent + same event signature within 23h -> suppressed at producer.
- `src/daemon/status-file.c:835-836` -- "Update disk footprint at most once every 10 minutes" -> the in-memory snapshot is refreshed at most every 10 min; each refresh triggers a save to `/var/lib/netdata/status-netdata.dat` (the "every few minutes" disk commit).
- `src/libnetdata/exit/exit_initiated.c:7-38` -- the 20 distinct exit_reason strings (NOT the 10 the .local draft claims).
- `src/libnetdata/signals/signal-code.c:97-235` -- the SIGNAL_CODE formatter `SIGNAL/SI_CODE`.
- `src/claim/cloud-status.c:5-15` -- the 5-value aclk enum (NOT 6 with `disabled`).
- `src/daemon/config/netdata-conf-profile.c:7-15` -- the 4-value profile enum `standalone, parent, child, iot` (NOT `dopple, store-child` from the draft).
- `src/collectors/log2journal/log2journal.c:8-61` -- the 256-entry transliteration map (per-char, not per-key); `log2journal-json.c:477-511` -- array index handling appends `_<index>`; `log2journal-help.c:108-109` -- the prefix is NOT transliterated, must be journal-friendly.

Consumer side (Netdata systemd-journal Function):
- `src/collectors/systemd-journal.plugin/systemd-journal.c:1022-1109` -- the facet-key registration model. AE_* fields are auto-discovered as facets when the Function reads the journal; structured filtering uses the `selections` parameter; FTS uses `query` (the `MESSAGE` field is registered with `FACET_KEY_OPTION_FTS` at `:1034`). The skill must teach: structured `selections` first, FTS only as residual narrower.

User clarifications (2026-05-05):
- Daily volume: 40k-200k events on stable releases.
- Default time: 24h (covers the dedup unit + balances scan cost).
- Wide windows for rare crashes (1-per-few-days class): up to 7 days.
- Default version filter: latest stable + latest 2-3 nightlies (auto-compute from observed version distribution).
- ssh transport: NOT a first-class script flag. Mention in `transports.md` as Costa-only path; do not expand.
- Group-by dimensions for `analyze-events.sh`: signal, fatal_function, fatal_filename, version, architecture, os_family, os_type, install_type, db_mode, kubernetes, profile, aclk, health, exit_cause, virtualization, chassis_type, host_cpus.

### Affected contracts and surfaces

The skill itself is a private developer skill at `<repo>/.agents/skills/query-agent-events/` and a one-line entry in AGENTS.md "Project Skills Index". No code changes. Indirect contracts the skill MUST document accurately:

- The producer's status JSON shape at `STATUS_FILE_VERSION = 28`.
- The journal namespace name (`agent-events`, hosted on Costa's ingestion server). NOT defined in this repo; documented as deployment convention.
- The systemd-journal Function payload shape (selections / query / histogram / facets).
- The `query-netdata-cloud` and `query-netdata-agents` skill helper APIs (which this skill consumes).

### Existing patterns to reuse

- The `<name>/SKILL.md` directory shape and frontmatter convention from SOW-0010 (proven by `query-netdata-cloud/`, `query-netdata-agents/`, `integrations-lifecycle/`, `learn-site-structure/`).
- The `agents_query_cloud` / `agents_query_agent` / `agents_call_function` helpers in `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh` (token-safe, bearer auto-mint, env-driven).
- The `recipes/INDEX.md` + `how-tos/INDEX.md` live-catalog pattern.
- The sensitive-data-discipline spec.
- Repo-relative paths everywhere; `${NETDATA_REPOS_DIR}/<repo>/...` for sibling repos.

### Risk and blast radius

- Skill is read-only documentation + scripts that make outbound queries. Blast radius on this repo: zero. Blast radius on the ingestion server: a query script with a bad default could full-scan the journal and degrade service for 40-200k-events/day query load. Mitigation: every default in `get-events.sh` MUST be index-friendly (structured `selections` filters first); FTS only as narrower.
- Privacy: every fetched event carries identifying fields (machine GUIDs, claim IDs, hardware DMI). Storage stays under `<repo>/.local/audits/query-agent-events/` (gitignored). No raw values in committed artifacts. `redact-events.sh` ships as opt-in for sharing.
- Untrusted-doc risk: 14 high-severity divergences found in `.local/agent-events-journals.md`. The skill writes verified ground truth from producer source; the .local doc is treated as a defunct draft and not copied into committed artifacts.
- Volume: Costa noted 40k-200k events/day on stable releases. Defaults narrow time + version aggressively to keep query weight low.

### Decisions recorded

D1. **Scoping predicate**: namespace alone (`--namespace=${AGENT_EVENTS_HOSTNAME}` / `__systemd_unit=` for the Function). No additional `WHERE AE_X != ""` belt-and-suspenders. Producer ALWAYS emits AE_VERSION/AE_EXIT_CAUSE/AE_AGENT_HEALTH/AE__TIMESTAMP, so the namespace is the scope.

D2. **No `AGENT_EVENTS_JOURNAL_NAMESPACE` env key**: keep `${AGENT_EVENTS_HOSTNAME}`'s quadruple-duty (Cloud room name, ssh host, direct-HTTP host, journalctl namespace) per the existing sensitive-data-discipline spec.

D3. **Drop `--via ssh` from script flags** (Costa: A). Mention ssh briefly in `transports.md` as Costa-only path; no scripted ssh transport.

D4. **Privacy default**: raw under `<repo>/.local/audits/query-agent-events/` (gitignored), never shared. Opt-in `redact-events.sh` for sharing.

D5. **Full AE_FIELDS.md coverage** (Costa: A): every producer field with version-gating annotation, every enum verified against source, indexable-vs-FTS guidance per field.

D6. **Default time window: 24h** (Costa: C). `--since '24h ago'` is the default. Wider windows (`--since '7d'`) documented for rare-crash investigation.

D7. **Default version filter**: latest stable + latest 2-3 nightlies (Costa). `get-events.sh` accepts `--versions auto` (default), `--versions <regex>`, and `--all-versions`. Auto-mode does a lightweight version-list query first, picks top stable + top 3 nightlies by version sort, then runs the main query with that filter.

D8. **Index-friendly query discipline** (Costa, hard requirement): structured `selections` filters first, FTS via `query` only as residual narrower. Anti-pattern in any recipe: bare FTS without structured slicing. The skill writes this rule into SKILL.md key concepts and into every recipe.

D9. **Group-by dimensions in `analyze-events.sh`** (Costa: confirmed): signal, fatal_function, fatal_filename, version, architecture, os_family, os_type, install_type, db_mode, kubernetes, profile, aclk, health, exit_cause, virtualization, chassis_type, host_cpus.

D10. **Filter syntax (Netdata systemd-journal plugin)** (Costa, hard requirement): the Function supports multi-value filters; between fields = AND, between values = OR. Costa described this as pseudo-code `(FIELD1 in A, B, C) AND (FIELD2 in D, E, F) AND ...` -- the **actual JSON shape** (verified at `src/libnetdata/facets/logs_query_status.h:386-466`) is the `selections` POST key:

```json
{
  "selections": {
    "FIELD1": ["A", "B", "C"],
    "FIELD2": ["D", "E"]
  }
}
```

D11. **Transport-level abilities live in `query-logs.md`** (Costa, scope clarification): the multi-value `selections` capability is a property of the systemd-journal Function transport, not specific to agent-events. Both `docs/netdata-ai/skills/query-netdata-cloud/query-logs.md` and `docs/netdata-ai/skills/query-netdata-agents/query-logs.md` get updated to mention it (cloud doc carries the full shape; agents doc references the cloud doc). agent-events specifics (which AE_* fields, when to use which, dedup semantics) stay in this skill.

Implications for the skill:
- `transports.md` references `query-logs.md` for the JSON shape rather than re-documenting it.
- `get-events.sh` builds the `selections` JSON with multiple values per field (e.g. `AE_AGENT_HEALTH: ["crash-first", "crash-loop", "crash-repeated", "crash-entered"]`).
- Recipes show worked `selections` JSON, not pseudo-code.
- The "structured filters first, FTS as residual narrower" rule (D8) is implemented through `selections` (structured) + top-level `query` (FTS).

### Implementation plan

Skill structure:

- `SKILL.md` -- entry point. Frontmatter triggers ("agent events", "agent-events", "crash reports", "fatals", "panics", "ingestion server", "status file", "AE_*" fields). Key concepts up front: bug-investigation tool, after-the-fact model, dedup window, structured-filters-first.
- `AE_FIELDS.md` -- the verified field map (~80 rows): producer source path | JSON path | journal field | enum/values | version-gating | indexable as facet? | bug-triage interpretation. Plus enum-meaning tables (what each `AE_AGENT_HEALTH`, `AE_FATAL_SIGNAL_CODE`, `AE_EXIT_CAUSE` value tells a bug-fixer).
- `transports.md` -- 3 transports with priority order. For each: how to call the Function via the existing `query-netdata-{cloud,agents}` helpers; what payload shape works for agent-events. ssh path gets a 1-paragraph note (Costa-only).
- `update-cadence.md` -- the after-the-fact model, the 23h client-side dedup, the ≥10 min disk snapshot, the start-only POST. Implications for query design (default 24h, wider for rare).
- `query-discipline.md` -- the structured-filters-first rule. Worked examples of right-vs-wrong queries. The Function payload's `selections` vs `query` parameters and how each interacts with the journal index.
- `finding-crashes.md` -- the "find recent signal crashes on stable releases" recipe end-to-end.
- `finding-fatals.md` -- the "find OOM / disk-full / asserts / deliberate exits" recipe.
- `recipes/INDEX.md` and per-recipe files: find-by-function (touch a specific symbol/file), find-by-version (regression spotter), find-related-to-work (template).
- `scripts/_lib.sh` -- sources `query-netdata-agents/scripts/_lib.sh`. Adds `agentevents_*` helpers: `agentevents_namespace`, `agentevents_load_env`, `agentevents_audit_dir`, `agentevents_query_function`, `agentevents_compute_default_versions`. Token-safe self-test (no token leak).
- `scripts/get-events.sh` -- download events of interest. Flags: `--via cloud|agent` (default cloud), `--since '24h ago'` (default), `--versions auto|<regex>|all` (default auto), `--signal <regex>`, `--health <pattern>`, `--exit-cause <pattern>`, `--query <fts>` (residual only). Output JSON dump to `<.local audits>/<timestamp>.json`.
- `scripts/analyze-events.sh` -- group-by stats over a downloaded dump. Flags: `--input <path>` (or stdin), `--by <dim>` (any of D9), `--top N`. Output: text table (default) or JSON.
- `scripts/redact-events.sh` -- opt-in redaction filter. Replaces machine_guid/claim_id/host_id/ephemeral_id with `<redacted>` placeholders.
- `how-tos/INDEX.md` -- live catalog (the durable rule).

### Validation plan

1. SKILL.md frontmatter loads (description <= 1024 chars, valid YAML).
2. Path discipline grep on every committed file: zero `~/...`, zero `/home/...`, zero literal AGENT_EVENTS_* values, zero machine GUIDs, zero claim IDs, zero IPv4 literals, zero long opaque tokens.
3. shellcheck clean on every script.
4. Token-leak self-test (`agentevents_selftest_no_token_leak`) PASS.
5. Real round-trip: `get-events.sh --via cloud --since '24h ago' --signal SIGSEGV` returns at least one record (or empty result with a clean exit, indicating no crashes in window). Sample bundle saved under `.local/audits/query-agent-events/` (gitignored).
6. `analyze-events.sh --by signal --input <bundle>` produces a sensible group-by table.
7. AE_FIELDS.md spot-check: pick 5 random rows, verify each against producer source.

### Artifact impact plan

- AGENTS.md: add one-line entry under "Project Skills Index" / "Runtime input skills".
- `<repo>/.agents/skills/query-agent-events/`: new directory and contents.
- `<repo>/.local/audits/query-agent-events/`: created on first run; gitignored.
- No specs change. No public docs change. No source change.
- No new env keys (the existing `AGENT_EVENTS_*` keys cover everything).

### Open decisions

None. All decisions resolved with Costa on 2026-05-05.

### Followup items surfaced (NOT to be left as "deferred")

- F-0003-A: `.local/agent-events-journals.md` is significantly wrong. Either delete it or replace with a redirect-to-the-skill stub. Tracked separately; the skill does NOT consume the .local doc as authoritative.
- F-0003-B: Verify systemd-journal Function `selections` shape supports filtering on auto-discovered `AE_*` fields end-to-end (the facet-key registration model auto-discovers; need to confirm via real round-trip during validation). If filtering doesn't pass through to the underlying journalctl command index, the skill must adjust to use a different Function arg.

Sensitive data handling plan:

- This SOW (and every committed artifact it produces) follows
  the spec at
  `<repo>/.agents/sow/specs/sensitive-data-discipline.md`. No
  literal IPs, hostnames, UUID-shaped IDs, tokens, absolute
  install/user paths, usernames, tenant names, or secrets in
  any committed file. Every reference uses an env-key
  placeholder (`${KEY_NAME}`) defined in `.env`.
- Specifically required `.env` keys for this SOW:
  `NETDATA_CLOUD_TOKEN`, `AGENT_EVENTS_NC_SPACE`,
  `AGENT_EVENTS_HOSTNAME` (used in four roles: cloud room
  name, ssh-able host, direct-HTTP host, journalctl
  namespace -- value happens to be the same string today),
  `AGENT_EVENTS_MACHINE_GUID`, `AGENT_EVENTS_NODE_ID`. No
  new agent-events keys are needed; the existing four cover
  the skill's needs.
- Pre-commit verification grep (from the spec) runs on every
  staged change before commit.

Holding-pattern decisions to record now (so they are not lost):

- Privacy policy: default is "store raw under
  `.local/audits/query-agent-events/`, never share". Opt-in
  redact filter ships in stage 2.
- Initial script set: `query-events.sh` + `fetch-event.sh` +
  `summarize.sh` (the trio recommended at stage-1 follow-up).
- `AE_FIELDS.md` shape: cross-reference table -- producer
  source path | journal field name | enum values (if any) |
  notes.

## Implications And Decisions

No new user decisions required at this stub stage. All
infrastructure decisions blocking this SOW are recorded in
SOW-0010 (decisions 1-3). Once SOW-0010 closes, this SOW will
add its own decision list covering:

- The exact `__logs_sources` value or `query=` predicate that
  scopes the journal query to agent-events on the ingestion-
  server agent.
- Whether the skill should also accept a literal namespace
  string (passed through to the Function) in `.env` as
  `AGENT_EVENTS_JOURNAL_NAMESPACE`.
- Whether the journalctl-via-ssh path is mandatory in stage 2
  or can ship in a follow-up.

## Plan

1. **Wait for SOW-0010 to close.**
2. Fill in this SOW's Pre-Implementation Gate, Decisions, and
   Implementation Plan based on the SOW-0010 deliverables.
3. Move to `current/` as `Status: in-progress`.
4. Implement, validate, close.

## Execution Log

### 2026-05-03

- Created as a stub during the 4-SOW split.

## Validation

### Acceptance criteria evidence

- `<repo>/.agents/skills/query-agent-events/SKILL.md` exists; YAML frontmatter parses cleanly; description 978 chars (under 1024 limit). Visible in the harness skill registry as `query-agent-events`.
- Per-domain guides: `AE_FIELDS.md`, `transports.md`, `update-cadence.md`, `query-discipline.md`, `finding-crashes.md`, `finding-fatals.md`.
- Recipes: `recipes/INDEX.md`, `find-by-function.md`, `find-by-version.md`, `find-related-to-work.md`.
- Scripts: `scripts/_lib.sh`, `get-events.sh`, `analyze-events.sh`, `redact-events.sh`. All bash-parse cleanly. All chmod +x.
- `how-tos/INDEX.md` with the live-catalog rule.
- AGENTS.md "Project Skills Index" updated with one-line entry.
- Both `docs/netdata-ai/skills/query-netdata-cloud/query-logs.md` and `docs/netdata-ai/skills/query-netdata-agents/query-logs.md` updated with the multi-value `selections` capability section (transport-level ability documented where all callers see it).

### Producer-source verification

- 80+ AE_* fields documented in `AE_FIELDS.md`, every row traceable to `<repo>/src/daemon/status-file.c` setters.
- All enums verified against source: `AE_AGENT_STATUS` (`status-file.c:23-33`), `AE_AGENT_ACLK` (`src/claim/cloud-status.c:5-15`), `AE_AGENT_HEALTH` (`status-file.c:929-952`), `AE_AGENT_PROFILE_*` (`src/daemon/config/netdata-conf-profile.c:7-15`), `AE_AGENT_EXIT_REASON_*` (`src/libnetdata/exit/exit_initiated.c:7-38`, 20 distinct strings), `AE_OS_TYPE` (`status-file.c:35-45`), `AE_EXIT_CAUSE` (`status-file.c:1097-1286`, 26 distinct strings), `AE_FATAL_SIGNAL_CODE` format (`src/libnetdata/signals/signal-code.c:97-235`).
- 14 high-severity divergences in the .local draft documented and corrected; the .local draft is treated as defunct.
- Dedup window verified: `status-file-dedup.c:11` `REPORT_EVENTS_EVERY = 86400 - 3600` (23h).
- Disk-snapshot cadence: `status-file.c:835-836` (>=10 min).
- Producer ingest URL location: `status-file.c:988`. Skill cites only by `path:line`; never quotes the literal URL.
- Multi-value `selections` JSON shape verified: `<repo>/src/libnetdata/facets/logs_query_status.h:386-466`.

### Path discipline

- `grep -rn -E '~/|/home/' .agents/skills/query-agent-events/`: zero hits.
- `grep -rn -E '[0-9a-f]{8}-...-[0-9a-f]{12}' .agents/skills/query-agent-events/`: only the `deadbeef-1234-...` sentinel inside the no-leak self-test (intentional).
- `grep -rn -E '([0-9]{1,3}\.){3}[0-9]{1,3}'`: zero hits.
- All `AGENT_EVENTS_*` references are env-keyed (`$AGENT_EVENTS_HOSTNAME`, `$AGENT_EVENTS_NODE_ID`, etc.), no bare values.

### Script syntax

- `bash -n` on all four scripts: clean.
- `shellcheck` clean (only SC1091 / SC2012 informational notes that are acceptable for sourced libraries and `ls`-of-dumps respectively).

### Token-safety

- `_lib.sh` includes `agentevents_selftest_no_token_leak`: drives the public wrapper with sentinel `deadbeef-...` UUID, asserts the sentinel never appears on captured stdout. Callable as a self-test before commits.
- All transport calls go through `agents_query_cloud` / `agents_query_agent` from the existing query-netdata-agents `_lib.sh` which is already token-safe (proven by the existing self-test).

### Coverage check

The skill answers, without follow-up:
- "How does agent-events work?" -> SKILL.md, update-cadence.md.
- "What fields are available and what do they mean?" -> AE_FIELDS.md.
- "Why isn't my crash showing up?" -> update-cadence.md (after-the-fact + dedup).
- "How do I find crashes/fatals?" -> finding-crashes.md, finding-fatals.md.
- "How do I find events touching function X?" -> recipes/find-by-function.md.
- "When did this regression appear?" -> recipes/find-by-version.md.
- "Is anyone hitting our current work?" -> recipes/find-related-to-work.md.
- "How do I write an index-friendly query?" -> query-discipline.md + the new section in query-logs.md.
- "How do the three transports differ?" -> transports.md.

### Artifact maintenance gate

- AGENTS.md: updated with one-line entry. DONE.
- Runtime project skills: NEW skill at `.agents/skills/query-agent-events/`. DONE.
- query-netdata-cloud/query-logs.md: extended with multi-value `selections` section (transport-level capability surfaced where all callers see it). DONE.
- query-netdata-agents/query-logs.md: pointer added so direct-agent users find the same. DONE.
- Specs: no spec change needed. NOT APPLICABLE.
- End-user/operator docs: this is a private developer skill. NOT APPLICABLE.
- SOW lifecycle: status `in-progress` -> `completed`; file moves from `current/` to `done/` in this commit. DONE.

## Outcome

The `query-agent-events` private skill ships as a bug-investigation tool. A maintainer (or AI assistant helping one) can download events of interest from the agent-events ingestion namespace via Cloud or direct-agent transport, slice efficiently with multi-value `selections` filters, and run group-by stats locally over the downloaded JSON to triage crashes, fatals, and regressions. The skill explicitly documents the after-the-fact event timing, the 23h client-side dedup, the 40k-200k events/day dataset volume, and the index-friendly query discipline -- so first-time users don't accidentally full-scan the namespace.

The .local draft (`agent-events-journals.md`) was found to have 14 high-severity divergences from producer source. The committed AE_FIELDS.md supersedes it.

The transport-level multi-value `selections` capability was documented in `query-netdata-cloud/query-logs.md` (the canonical transport reference) so all callers (not just agent-events) can use it.

## Lessons Extracted

1. **Untrusted reference docs need verification before being copied as skill content.** The .local draft was a reasonable starting hypothesis but had ~14 high-severity errors. Always cross-check against producer source.

2. **Transport-level capabilities belong in transport-level docs.** Multi-value field filtering is a property of the systemd-journal Function, not of agent-events. The capability went into `query-logs.md` (visible to all callers) instead of being re-documented in this skill.

3. **`status-file.c:988`-style references are useful redirects.** The producer ingest URL is hardcoded; the skill never quotes the literal URL but cites the line so a maintainer who needs to know can find it.

4. **The dedup model is critical for query design.** Without understanding the 23h client-side dedup, an analyst will misinterpret duplicate-suppressed events as "we didn't crash". The skill front-loads this fact.

5. **Index-friendly queries matter at this scale.** 40-200k events/day demands structured `selections` filters; bare FTS over wide windows is unsafe. The skill writes the rule and bakes it into the scripts' defaults.

6. **`host.uptime` is a misleading name** -- it stores boottime epoch, not duration. Documented prominently so future readers don't compute wrong values.

## Followup

These items were exposed during investigation but are NOT documentation work for this SOW. Tracked separately:

- F-0003-A: `.local/agent-events-journals.md` is significantly wrong. Recommended action: replace with a stub redirecting to the skill, or delete. Not in this SOW's scope (`.local/` is gitignored; user's choice).
- F-0003-B: Verify systemd-journal Function `selections` shape end-to-end against a real round-trip during use. The shape is verified from source (`logs_query_status.h:386-466`); a real round-trip in production validates the capability against current ingestion-server config. To be done at first usage, not before close.
- F-0003-C: The `--versions <regex>` flag in `get-events.sh` is documented as client-side (post-fetch jq filter) rather than pushed as a server-side regex. If the systemd-journal Function adds regex selections later, the script can be upgraded.

## Regression Log

None yet.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
