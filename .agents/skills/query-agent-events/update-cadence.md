# Update cadence

agent-events is **not real-time**. Understanding when events
arrive is essential for query design.

## The "after-the-fact" model

When a Netdata agent crashes, the crash itself does NOT post
to the ingestion server. The agent has to start again, read
its on-disk status file (the previous session's state), and
POST that to the ingestion server.

Concretely (`src/daemon/status-file.c`):

1. **During a session** -- the agent maintains its current
   status in memory and writes a snapshot to
   `/var/lib/netdata/status-netdata.dat` periodically (see
   "Disk save cadence" below).
2. **On exit / crash** -- a final snapshot is written if
   possible (signal-async-safe writer at
   `<repo>/src/daemon/status-file-io.c`). For SIGKILL / OOM-kill,
   no final snapshot is written -- the prior periodic snapshot
   is what gets reported.
3. **On NEXT start** -- the new agent reads the on-disk
   status, computes `agent_health`, classifies `exit_cause`,
   then POSTs to the ingestion server (`status-file.c:988`).

So a crash at 14:00 produces a journal record on the ingestion
server only after the agent restarts -- which might happen
seconds, minutes, hours, or days later (depending on
operator policy and whether the agent loops).

### Implication for queries

- **"Last hour"** is misleading -- it misses crashes from
  agents that crashed in the last hour but haven't restarted
  yet.
- **"Last 24 hours"** is the natural unit -- accommodates
  typical restart latency AND aligns with the dedup window.
- For rare crashes (1-per-few-days class), expand to 7+ days.

## Disk save cadence

`<repo>/src/daemon/status-file.c:835`:

> "Update disk footprint at most once every 10 minutes (600
> seconds)"

The in-memory snapshot is refreshed at most every 10 minutes.
Each refresh triggers a save to
`/var/lib/netdata/status-netdata.dat` (atomic temp+rename via
`<repo>/src/daemon/status-file-io.c`). Saves also happen on:

- start-up (before claiming the new session);
- exit (graceful shutdown writes the final state);
- inside a deadly-signal handler (best-effort write before
  the process dies).

So the on-disk snapshot a restarting agent reads is at most
~10 minutes old, plus any signal-handler-time updates.

## 23h client-side dedup

`<repo>/src/daemon/status-file-dedup.c:11`:

```c
#define REPORT_EVENTS_EVERY (86400 - 3600)  // -1 hour to tolerate cron randomness
```

= 82800 seconds = 23 hours.

Each agent maintains a small dedup table at
`/var/lib/netdata/dedup-netdata.dat` (binary; see `:13-22`):
50 slots, each storing `(hash, sentry-flag, timestamp)`.

The hash includes (`status-file-dedup.c:42-95`):

- `version` (schema), `status`, `signal_code`, `profile`,
  `exit_reason`, `db_mode`, `db_tiers`, `kubernetes`,
  `sentry_available`, `sentry_fatal`,
- `host_id` (Netdata machine GUID), `machine_id` (OS),
  `worker_job_id`, `line` (in source),
- `version` string, `filename`, `function`, `stack_trace`
  (with hex addresses zeroed for hashing only),
  `thread`,
- `msg`, `cause`.

So **each unique combination of these fields** has its own
dedup slot. If the agent saw the same combination within 23h,
the new POST is suppressed (`dedup_already_posted` returns
`true` -> producer skips the POST).

### Implication: 1 record per agent per event-class per day

A given agent will report the same crash signature at most
once per 23h window. So at the journal level:

- 1 distinct crash class per restart, per agent, per day.
- Different agents posting the same crash signature both
  arrive (the dedup is **client-side**, not server-side).
- Different crash signatures from the same agent within 23h
  all arrive (different hashes).

This is why "last 24 hours" is the natural query window: the
dedup makes daily counts meaningful (each agent contributes
at most one record per signature).

### Stack-trace anonymization is dedup-only

`status-file-dedup.c:26-36`:

```c
static void stack_trace_anonymize(char *s) {
    char *p = s;
    while (*p && (p = strstr(p, "0x"))) {
        p[1] = '0';
        p += 2;
        while(isxdigit((uint8_t)*p)) *p++ = '0';
    }
}
```

Hex addresses in the stack trace are zeroed ONLY when
computing the dedup hash. The journal-emitted
`AE_FATAL_STACK_TRACE` retains real addresses (useful for
debugging; sensitive when sharing externally -- see
`redact-events.sh`).

## Dataset volume

40k-200k events / day on stable releases. Total fleet ~1.5M
agents (not all restart daily). The dataset is large and
spread across days.

### Implication: index-friendly queries are mandatory

A 7-day FTS-only query over the agent-events namespace will
scan ~1M records on a high-volume week. With structured
`selections` filters (e.g. `AE_AGENT_HEALTH`, `AE_AGENT_VERSION`),
the same query slices to thousands or tens of thousands of
records before any FTS -- orders of magnitude cheaper.

See `query-discipline.md` for the rule and worked examples.

## Default time windows for the skill

| Use case | Default window | Why |
|---|---|---|
| Routine crash triage | 24h | Aligns with 23h dedup; one record per agent per signature. |
| Regression spotting | 24h | Same as above; group-by version. |
| Rare-crash hunting | 7 days | Catches 1-per-few-days classes. |
| "When did this start / get fixed?" | 14-30 days | Wide window with strong structured filters. Use sparingly. |

## Default version filter

Latest stable + latest 2-3 nightlies. The dataset is noisy
because many unupdated agents report crashes that have been
fixed. Filtering to recent versions focuses triage on bugs
that still matter.

`get-events.sh --versions auto` (the default) computes:
1. Quick discovery query: 24h window, no version filter,
   `AE_AGENT_VERSION` as a facet.
2. From the facet result, pick the latest stable
   (`v\d+\.\d+\.\d+` matching `^v2\.([89]|\d\d)\.\d+$` or the
   newest by version sort) plus the top 3 nightlies
   (`v\d+\.\d+\.\d+-\d+-nightly`).
3. Re-run the main query with `selections.AE_AGENT_VERSION`
   set to that list.

`--all-versions` skips the auto filter for "when did X
start?" investigations.
`--versions <regex>` overrides with a custom pattern.

## What this means in practice

- A crash you just got reported in chat will appear on
  agent-events anywhere from minutes to days later.
- "Why don't I see this crash?" probably means "the agent
  hasn't restarted yet, OR the dedup suppressed a duplicate
  within 23h, OR the version filter excluded it".
- High-cardinality crashes from old versions are mostly
  noise; default-version-filtering keeps triage focused.
- For "is this fixed in v2.X?", run `--versions auto` and
  compare crash counts across the auto-selected versions.
