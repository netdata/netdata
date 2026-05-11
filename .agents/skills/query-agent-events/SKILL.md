---
name: query-agent-events
description: Bug-investigation tool for the Netdata agent-events ingestion namespace -- triage crashes, panics, fatals across the fleet by downloading events of interest and clustering locally. Covers the three transports (Cloud API and direct agent API are primary; ssh is operator-only), the verified AE_* field map and enum meanings, the dedup model (23h client-side per agent and event signature), the after-the-fact event timing (POST only on agent restart), and the Netdata systemd-journal plugin multi-value filter syntax (FIELD in A, B, C) AND ... Use when investigating crashes / panics / fatals; when grepping for events touching a specific function or file or version; when looking for regressions across versions; when an agent is reported crashing in a way you want to triage. Ships scripts get-events.sh and analyze-events.sh that fetch events with index-friendly filters and compute group-by stats. Defaults to last 24 hours and to the latest stable plus latest 2-3 nightlies.
---

# query-agent-events

Private developer skill for triaging crashes, panics, and
fatals across the Netdata fleet. Reads the agent-events
systemd-journal namespace via the Netdata `systemd-journal`
Function (Cloud-proxied or direct-agent transport) and ships
scripts that bake in index-friendly query patterns.

## Why this skill exists

40k-200k status events arrive on the ingestion server every
day on stable releases. The total fleet is 1.5M agents, so
the dataset is large and noisy (many unupdated agents report
crashes that have been fixed). Naive "grep all" queries are
slow and wasteful. This skill teaches the maintainer (and any
AI assistant helping them) how to slice the dataset
efficiently and how to interpret what comes back.

## Workflow

```
+-------------------------+        +---------------------+
|  get-events.sh          |  -->   |  <timestamp>.json   |
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
                              |  + fix the bug         |
                              +------------------------+
```

The skill is a bug-investigation tool, not a generic logs
query tool. The two existing `query-netdata-cloud` and
`query-netdata-agents` skills already cover transport
mechanics; this skill EXTENDS them with the agent-events
specifics (what fields are present, what predicates are
index-friendly, what each enum value means for triage).

## Key concepts (read first)

1. **The dataset**: 40k-200k status events / day on stable
   releases, spread across 1.5M agents (not all restart
   daily). Naive full-namespace queries with bare FTS are
   slow.

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
   latest stable + latest 2-3 nightlies for version. This
   focuses triage on bugs that still matter. Wide windows
   (`--since '7d'` or longer) are reserved for rare crashes
   (1-per-few-days class) and for "when did this start /
   get fixed" investigations.

7. **AE_* field naming**: every JSON path in the producer's
   status document becomes an `AE_`-prefixed journal field
   (per `log2journal --prefix 'AE_'` on the ingestion server).
   See `AE_FIELDS.md` for the verified map and enum meanings.

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
| `how-tos/INDEX.md` | Live catalog: every analysis question becomes a how-to entry. |

## Live how-to rule (mandatory)

If asked a concrete question about agent-events that requires
non-trivial analysis (multiple file reads, running queries,
cross-referencing with producer source) AND the answer is not
already documented in the per-domain guides above or in
`recipes/`, the assistant MUST author a new how-to under
`how-tos/<slug>.md` and add a one-line entry to
`how-tos/INDEX.md` BEFORE completing the task.

## Scripts (in scripts/)

| Script | Purpose |
|---|---|
| `_lib.sh` | Helpers (`agentevents_*` prefix). Sources `query-netdata-agents/scripts/_lib.sh`. Token-safe; ships a no-leak self-test. |
| `get-events.sh` | Fetch events of interest. Index-friendly defaults. JSON output to `.local/audits/query-agent-events/`. |
| `analyze-events.sh` | Group-by stats over a downloaded dump (signal, version, fatal_function, architecture, etc.). |
| `redact-events.sh` | Opt-in redaction (machine_guid / claim_id / host_id / ephemeral_id -> placeholders). For sharing only. |

## Path discipline

This skill follows
`<repo>/.agents/sow/specs/sensitive-data-discipline.md`:

- Repo files: repo-relative (`<repo>/src/...`).
- Sibling Netdata-org repos: `${NETDATA_REPOS_DIR}/<repo>/...`.
- agent-events host / namespace / machine GUID / node ID:
  ALWAYS via env keys. Never literal values in any committed
  file.
- Producer ingest URL: NEVER quoted literally. Reference only
  as `src/daemon/status-file.c:988`.
- Fetched event payloads land under
  `<repo>/.local/audits/query-agent-events/<timestamp>.json`
  (gitignored). Do NOT paste raw event JSON into committed
  artifacts.

## Required env keys

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
