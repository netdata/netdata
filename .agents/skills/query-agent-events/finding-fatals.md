# Finding fatals (deliberate exits)

A "fatal" = the agent **chose** to exit because of a
condition it could not recover from (OOM, disk full,
assertion failure, listen-socket conflict, fatal() call).
DIFFERENT from a signal crash (the kernel didn't kill us; we
called `exit()` or `_exit()` ourselves).

## Quick recipe

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --exit-cause fatal \
    --since '24h ago' \
    --versions auto
```

`--exit-cause fatal` is shorthand for the deliberate-fatal
class of `AE_EXIT_CAUSE` values (see below).

## Distinguishing predicates

- `AE_FATAL_SIGNAL_CODE` is **EMPTY** (this is what separates
  fatals from signal crashes).
- `AE_EXIT_CAUSE` is one of:
  - `no last status`
  - `out of memory` (NOT `cannot allocate` -- the .local draft
    was wrong)
  - `disk full` / `disk almost full` / `disk read-only`
  - `already running`
  - `fatal on start` / `fatal on exit` / `fatal and exit`
  - `exit timeout`
  - `abnormal power off`
- `AE_AGENT_EXIT_REASON_*` may contain `out-of-memory`,
  `already-running`, `fatal`, `shutdown-timeout`.
- `AE_FATAL_MESSAGE`, `AE_FATAL_FUNCTION`, `AE_FATAL_FILENAME`,
  `AE_FATAL_LINE` -- populated from the `fatal()` call site.

## Index-friendly query

```json
{
  "after":  -86400,
  "before": 0,
  "last":   500,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_EXIT_CAUSE": [
      "no last status",
      "out of memory",
      "disk full",
      "disk almost full",
      "disk read-only",
      "already running",
      "fatal on start",
      "fatal on exit",
      "fatal and exit"
    ],
    "AE_AGENT_VERSION": ["v2.10.0", "v2.10.0-135-nightly", "v2.10.0-130-nightly"]
  },
  "facets": ["AE_EXIT_CAUSE", "AE_FATAL_FUNCTION", "AE_HOST_ARCHITECTURE", "AE_OS_FAMILY"]
}
```

## Per-cause triage

### `out of memory`

Agent panicked when an allocation failed. Look at
`AE_FATAL_FUNCTION` to localize -- often `mallocz`,
`reallocz`, dbengine page allocators, or memory-pool
constructors. Cross-correlate with
`AE_HOST_MEMORY_FREE_PERCENT` (low at POST time = host was
under pressure) and `AE_HOST_MEMORY_NETDATA` (how much
netdata was using).

Most useful slicers:
- `AE_AGENT_DB_MODE` -- dbengine memory mode.
- `AE_AGENT_DB_TIERS` -- number of tiers.
- `AE_HOST_MEMORY_TOTAL` -- absolute host RAM. OOM on a 1GB
  host is different from OOM on a 64GB host.

### `disk full` / `disk almost full` / `disk read-only`

Storage failure. Look at:
- `AE_HOST_DISK_DB_FREE` / `AE_HOST_DISK_DB_INODES_FREE` --
  what state the disk was in.
- `AE_HOST_DISK_DB_READ_ONLY` -- bool, set on read-only.
- `AE_AGENT_DB_MODE` -- whether dbengine is in `dbengine`,
  `ram`, `ram-cache`, `none`, `alloc`, `save`, ...

### `already running`

Listen-socket conflict at init. Look at `AE_AGENT_VERSION`
and `AE_AGENT_RESTARTS` -- if restarts > 0 and the same
agent_id keeps hitting this, there's a stale lock or a
concurrent agent on the same host.

### `fatal on start` / `fatal on exit` / `fatal and exit`

Generic `fatal()` calls. Look at `AE_FATAL_MESSAGE` to find
the panic message and `AE_FATAL_FUNCTION` to localize. Group
by message to find the most common patterns.

### `no last status`

First-ever start, or the prior status file was corrupted /
unreadable. Usually transient. If recurring on the same
agent (`AE_AGENT_RESTARTS` going up), the status file is
being deleted or the disk path is broken.

### `abnormal power off`

Power loss between "agent was running" and "agent restarted".
Useful to see whether power events correlate with crashes
elsewhere -- usually filter these OUT of crash analysis.

### `exit timeout`

Shutdown didn't complete in time. Look at
`AE_AGENT_TIMINGS_EXIT` for how long shutdown ran before the
timeout fired. Long exit timings + worker_job_id may localize
the stuck thread.

## Triage flow

1. **Get the dump** -- `get-events.sh --exit-cause fatal`.
2. **Group by exit_cause** -- `analyze-events.sh --by exit_cause`
   to see the distribution.
3. **For the dominant cause**, group by function or message --
   `analyze-events.sh --by fatal_function --filter "exit_cause=out of memory"`.
4. **Cross-check environment** -- is this OOM specific to
   small-RAM hosts? Specific dbengine mode?
5. **Read source at the panic site**, fix the bug or improve
   the error path.

## Pitfalls

- **`abnormal power off` is not a bug** -- it's environmental.
  Filter out of crash analysis unless you specifically want
  power events.

- **`fatal_errno`** vs `fatal.errno` -- the producer emits
  `fatal.errno` (nested), not `fatal_errno` (top-level). The
  journal field is `AE_FATAL_ERRNO` in both cases (the
  underscore comes from the dot transliteration).

- **`exit timeout` records have NO `AE_FATAL_*` context** --
  the shutdown timer fired without a panic site. Use
  `AE_AGENT_TIMINGS_EXIT` and `AE_FATAL_WORKER_JOB_ID` instead.

- **Disk-related causes propagate** -- a `disk full` event
  may show up as `cannot allocate` in subsequent attempts.
  The first record is the meaningful one.

## Related recipes

- `finding-crashes.md` -- the OTHER class (signal-delivered).
- `recipes/find-by-function.md` -- localize by function name.
- `recipes/find-by-version.md` -- regression spotter.
