# How to find memory-corruption signatures on Windows agents (and compare per-OS rates)

Question answered: "Are Windows agents suffering memory corruption, where does it
manifest, and how does the rate compare to Linux?"

## Key idea

Heap corruption rarely reports itself. It surfaces as **integrity-check fatals in
unrelated subsystems** (the victims), not in the corrupting code. The best proxy
signatures in agent-events are the `AE_FATAL_FUNCTION` values of the agent's
internal consistency checks:

- `uuidmap_acquire_by_uuid` — message `UUIDMAP: corrupted JudyL array`
- `pgc_page_add` — message `DBENGINE CACHE: JudyLIns(...) failed ... result = 0xffffffffffffffff` (PJERR)
- `refcount_acquire_advanced_with_trace` / `refcount_release_and_acquire_for_deletion_advanced_with_trace`
  — message `REFCOUNT <garbage> is invalid ... called from string_freez()/string_dup()/...`
- `spinlock_deadlock_detect` — trashed lock words look like deadlocks

Plus signal crashes (`AE_AGENT_HEALTH` crash classes) whose stacks land in Judy /
STRING / dbengine code.

## Recipe

1. Windows crash overview (index-friendly; facets give full-slice counts even when
   the page caps):

```json
{
  "after": -604800, "before": 0, "last": 1000,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_OS_TYPE": ["windows"],
    "AE_AGENT_HEALTH": ["crash-first","crash-loop","crash-repeated","crash-entered"]
  },
  "facets": ["AE_FATAL_SIGNAL_CODE","AE_FATAL_FUNCTION","AE_AGENT_VERSION","AE_EXIT_CAUSE"]
}
```

2. Pull the corruption-signature fatals with full rows (stack traces, messages):
   same query but select `AE_FATAL_FUNCTION` on the list above instead of
   `AE_AGENT_HEALTH`.

3. Per-OS rate comparison — one cheap query, `last: 1`, selections only on
   `AE_FATAL_FUNCTION`, facet on `AE_OS_TYPE`. The facet option counts are the
   full-slice totals per OS.

4. Cluster fatal messages. Normalize digits before uniq so addresses/counters
   collapse:

```bash
dump=.local/audits/query-agent-events/events.json
fn_idx=$(jq -r '.columns | to_entries[] | select(.value.name=="AE_FATAL_FUNCTION") | .value.index' "$dump")
msg_idx=$(jq -r '.columns | to_entries[] | select(.value.name=="AE_FATAL_MESSAGE") | .value.index' "$dump")
jq --argjson fn_idx "$fn_idx" --argjson msg_idx "$msg_idx" -r \
  '.data[] | "\(.[$fn_idx]) | \(.[$msg_idx])"' "$dump" |
  sed 's/[0-9]\{3,\}/N/g' | sort | uniq -c | sort -rn
```

5. Frame histogram across stacks:

```bash
dump=.local/audits/query-agent-events/events.json
stack_idx=$(jq -r '.columns | to_entries[] | select(.value.name=="AE_FATAL_STACK_TRACE") | .value.index' "$dump")
jq --argjson stack_idx "$stack_idx" -r '.data[] | .[$stack_idx] // empty' "$dump" |
  sed -n 's/^#[0-9][0-9]* \([A-Za-z_][A-Za-z0-9_]*\) \[.*/\1/p' |
  sort | uniq -c | sort -rn
```

## Interpreting

- Facet counts cover the WHOLE filtered slice; `.data` is one page (`last`).
- Compare absolute per-OS counts against fleet sizes: the Linux fleet is orders of
  magnitude larger, so even equal absolute counts mean a much higher Windows rate.
- Crash stack line numbers refer to the DEPLOYED version. Map them with
  `git show <tag>:<path> | sed -n 'N,Mp'` in the netdata repo, not against HEAD.
- Detector thread names (`AE_FATAL_THREAD`) tell you who FOUND the corruption, not
  who caused it. Expect UV_WORKER / P[windows] / TRAIN to dominate — they are the
  heaviest users of Judy/STRING structures.

## Result of the first run (2026-07-08, 7-day window)

- Corruption-signature fatals: **108 windows vs 10 linux** — Windows per-agent rate
  3-4 orders of magnitude higher. Windows daemon heap corruption confirmed.
- Direct SIGSEGV observed inside perflib traversal (`isValidStructure` /
  `getCounterDefinition`, e.g. via `perflib-adcs.c`), consistent with perflib
  trusting `PERF_DATA_BLOCK->TotalByteLength` (provider-controlled) as its bounds
  anchor while discarding the actual size returned by `RegQueryValueEx`
  (see `src/libnetdata/os/windows-perflib/perflib.c`, `getPerformanceData()` /
  `getDataBlock()`).

## Method used

Files read: `finding-crashes.md`, `AE_FIELDS.md`, `scripts/get-events.sh`,
`scripts/_lib.sh`. Queries run via `agentevents_query_function cloud` with the
payloads above. Dumps land under `.local/audits/query-agent-events/`.
