---
name: triage-agent-events
description: Investigate Netdata crashes, panics and fatals from agent-events captures or authorized fleet queries. Use for AE_* fields, restart/dedup timing, structured filters, version comparisons and reviews of these investigation helpers. Ordinary logs use the Agent/Cloud query skills.
---

# triage-agent-events

Private developer skill for triaging crashes, panics, and
fatals across the Netdata fleet. Reads the agent-events
systemd-journal namespace via the Netdata `systemd-journal`
Function (Cloud-proxied or direct-agent transport) and ships
scripts that bake in index-friendly query patterns.

## Choose the task

| Task | Read or use |
|---|---|
| Analyze a supplied capture | `AE_FIELDS.md`, relevant crash/fatal/recipe guidance, `analyze-events.sh --input PATH`; no credential setup |
| Explain fields, timing or query construction | `AE_FIELDS.md`, `update-cadence.md`, `query-discipline.md`; transport reference only as needed |
| Fetch live evidence | Selected transport and query discipline, then configured environment and `get-events.sh` within existing authorization |
| Review helper or investigation changes | Affected contracts/source and existing tests; examples do not authorize live queries or bug fixes |

The no-leak self-test uses synthetic transport in a subshell, without environment loading or network access. It checks
Cloud/direct dispatch success and visible request masking; arbitrary event response contents remain private evidence.

## Why this skill exists

Historical observations found 40k-200k daily status events
and a fleet around 1.5M agents. These are not current size
guarantees; the dataset can be large and noisy (many unupdated agents report
crashes that have been fixed). Naive "grep all" queries are
slow and wasteful. This skill teaches the maintainer (and any
AI assistant helping them) how to slice the dataset
efficiently and how to interpret what comes back.

## Workflow

```
+-------------------------+        +---------------------+
|  get-events.sh          |  -->   |  <run>.json         |
|  (cloud or agent API)   |        |  in .local/audits   |
+-------------------------+        +---------------------+
                                          |
                                          v
                              +------------------------+
                              |  analyze-events.sh     |
                              |  --by signal|version|  |
                              |       function|...     |
                              +------------------------+
                                          |
                                          v
                              +------------------------+
                              |  cluster + read source |
                              |  + report the finding  |
                              +------------------------+
```

The skill is a bug-investigation tool, not a generic logs
query tool. The two existing `query-netdata-cloud` and
`query-netdata-agents` skills already cover transport
mechanics; this skill EXTENDS them with the agent-events
specifics (what fields are present, what predicates are
index-friendly, what each enum value means for triage).

## Key concepts (read first)

1. **The dataset**: fleet-scale status events, not a complete
   census of crashes at occurrence time. Naive full-namespace
   queries with bare FTS are slow.

2. **Index-friendly queries** (HARD RULE): use multi-value
   field filters FIRST. The Netdata `systemd-journal` plugin
   supports the syntax:
   ```
   (FIELD1 in A, B, C) AND (FIELD2 in D, E, F) AND ...
   ```
   Between fields = AND. Between values = OR. This is a
   facet-engine feature, NOT raw journalctl. Use FTS via
   `query=` only as a residual narrower over the structured
   slice. See `query-discipline.md`.

3. **Three transports** (priority order):
   - **Cloud API** -- proxied through Netdata Cloud at the
     agent-events space. Primary for the team.
   - **Direct agent API** -- against the agent-events node's
     `/api/v3/function?function=systemd-journal`. Primary for
     scripts.
   - **ssh to the host** -- operator-only path; mentioned in
     `transports.md` but no scripted ssh transport.

4. **After-the-fact event model**: agents POST events ONLY on
   start (the previous session's exit reason). They commit
   status to disk on start, stop, and at most every 10
   minutes. So the meaningful query unit is "events posted in
   the last 24 hours"; "the last hour" misses real crashes
   that haven't restarted yet.

5. **23h client-side dedup** (`src/daemon/status-file-dedup.c:11`):
   same agent + same event-content hash within 23h ->
   suppressed at the producer. So 1 record per agent per
   event-signature per day is the natural unit. Different
   agents posting the same crash signature -> both arrive
   (server does not dedup).

6. **Default time + version filters**: 24h time window;
   highest numeric stable + up to three nightlies observed
   in the discovery response, not the published release catalog. This
   focuses triage on bugs that still matter. Wide windows
   (`--since '7d ago'` or longer) are reserved for rare crashes
   (1-per-few-days class) and for "when did this start /
   get fixed" investigations.

7. **AE_* field naming**: every JSON path in the producer's
   status document becomes an `AE_`-prefixed journal field
   (per `log2journal --prefix 'AE_'` on the ingestion server).
   See `AE_FIELDS.md` for the verified map and enum meanings.

`get-events.sh` fetches one page (default 500 rows), not a paginated census. Before count or absence claims, inspect
status, partial/sampling flags and matched/returned limits as in
`./how-tos/trace-stack-symbol-regression-to-mutator.md#3-prove-that-the-response-is-complete`. Narrow or paginate through
the transport API when needed. Client-side version regexes change rows only; facets/totals still describe the server
response. Prefer an explicit `--input` when analyzing a particular run; the default latest-file choice is a convenience.

## Table of contents

| Doc | Purpose |
|---|---|
| `AE_FIELDS.md` | Verified field map (~80 rows) + enum meanings for triage. Indispensable. |
| `transports.md` | Cloud API + direct agent API call patterns; ssh footnote. |
| `update-cadence.md` | After-the-fact model, dedup, push timing, disk commits, query implications. |
| `query-discipline.md` | The multi-value filter syntax, structured-filters-first rule, anti-patterns. |
| `finding-crashes.md` | Recipe: signal crashes (SIGSEGV / SIGBUS / SIGFPE / SIGABRT) on stable. |
| `finding-fatals.md` | Recipe: deliberate fatals (OOM, disk full, asserts). |
| `recipes/INDEX.md` | Live catalog of recipes (find-by-function, find-by-version, find-related-to-work). |
| `how-tos/INDEX.md` | Catalog of reusable investigation how-tos. |

## Knowledge Capture

Capture timing and authorization follow `AGENTS.md#knowledge-capture`. Investigation recipes live in `how-tos/` and
are listed in `./how-tos/INDEX.md`; check the per-domain guides and `recipes/` before adding another recipe.

## Scripts (in scripts/)

| Script | Purpose |
|---|---|
| `_lib.sh` | Helpers (`agentevents_*` prefix). Sources `query-netdata-agents/scripts/_lib.sh`. Token-safe; ships a no-leak self-test. |
| `get-events.sh` | Fetch events of interest. Index-friendly defaults. Fresh private JSON output to `.local/audits/query-agent-events/`; explicit output paths must be new. |
| `analyze-events.sh` | Group-by stats over a downloaded dump (signal, version, fatal_function, architecture, etc.). |
| `redact-events.sh` | Opt-in redaction (machine_guid / claim_id / host_id / ephemeral_id -> placeholders). For sharing only. |

## Path discipline

This skill follows
`<repo>/.agents/sensitive-data-discipline.md`:

- Repo files: repo-relative (`<repo>/src/...`).
- Sibling Netdata-org repos: `${NETDATA_REPOS_DIR}/<repo>/...`.
- agent-events host / namespace / machine GUID / node ID:
  ALWAYS via env keys. Never literal values in any committed
  file.
- Producer ingest URL: NEVER quoted literally. Reference only
  as `src/daemon/status-file.c:988`.
- Fetched event payloads land under
  `<repo>/.local/audits/query-agent-events/<run>.json`
  (gitignored). Do NOT paste raw event JSON into committed
  artifacts.

## Required env keys

`get-events.sh` requires Bash 4 or later for associative arrays; select a compatible Bash on systems with an older
default. Only live `get-events.sh` calls load these settings. Its current loader requires all listed values for
either transport;
offline analysis/redaction, explanation, source review and the synthetic self-test do not load `.env`.

| Key | Role |
|---|---|
| `NETDATA_CLOUD_TOKEN` | Cloud REST token (long-lived). |
| `NETDATA_CLOUD_HOSTNAME` | Cloud REST API host. |
| `AGENT_EVENTS_HOSTNAME` | Dual-duty: ssh host AND direct-HTTP host of the ingestion node. Can be IP or DNS name. NOT the journalctl namespace (hardcoded `agent-events`); NOT the Cloud room name (also hardcoded `agent-events`). |
| `AGENT_EVENTS_MACHINE_GUID` | Agent machine GUID for direct-agent transport. |
| `AGENT_EVENTS_NODE_ID` | Cloud node UUID for cloud-proxy transport. |

All values live in `<repo>/.env` (gitignored). See
`<repo>/.agents/ENV.md` for setup (where each value comes
from, sample formats, common mistakes).

## Related skills

- `query-netdata-cloud` -- transport: Cloud REST API.
- `query-netdata-agents` -- transport: direct agent REST + bearer auto-mint.
- This skill consumes both via their `_lib.sh` helpers.
