# IBM i AS400 Collector – Query Performance Roadmap

## 1. Current Situation
- The latency chart (`netdata.plugin_ibm.as400_query_latency`) for the last 10 minutes shows sustained slowdowns from three collectors:
  - `job_queues` / `count_job_queues`: average ~2.9 s, peaks >4.0 s.
  - `output_queue_info`: average ~2.8 s, peaks >4.3 s.
  - `other`: residual bucket averaging ~1.2 s (needs attribution).
- Message queue collectors are currently disabled in production (0 ms latency), but the SQL still performs full scans when enabled.
- The collector remains single-threaded; any slow query blocks all 5 s metrics.

## 2. Immediate Offenders (must-fix)
The following collectors must be reworked first; they combine high latency with unbounded scans:

| Query | Source | Problem | Required Fix |
| --- | --- | --- | --- |
| `message_queue_aggregates`, `count_message_queues` | `QSYS2.MESSAGE_QUEUE_INFO` (detail table, full scan) | Full table scan, aggregation, `FETCH FIRST` post-scan | Rewrite around the table function (`TABLE(QSYS2.MESSAGE_QUEUE_INFO(...))` on 7.4+), expand selectors into explicit lists, round-robin per queue, run on slow track |
| `collectJobQueues` (`queryJobQueues`, `queryCountJobQueues`) | `QSYS2.JOB_QUEUE_INFO` | Fetches all queues, filtering happens in Go after the fact, `FETCH FIRST` post-scan | Convert selector/limits into `WHERE` clauses or per-queue table function calls, use explicit list expansion, move to slow track |
| `collectOutputQueues` (`queryOutputQueueInfo`, `queryCountOutputQueues`) | `QSYS2.OUTPUT_QUEUE_INFO` | Same pattern as job queues, plus spool file stats | Same treatment as job queues; prefer table function or filtered views if IBM introduces them |
| `collectActiveJobs` (`queryTopActiveJobs`, `queryCountActiveJobs`) | `TABLE(QSYS2.ACTIVE_JOB_INFO(...))` with `*ALL` filters | Materializes entire active-job list before `FETCH FIRST` | Pass explicit subsystem/job filters, allow inclusion list expansion, add per-group scheduling |
| `collectPlanCache` (`CALL QSYS2.ANALYZE_PLAN_CACHE('03', …)`) | Stored procedure | Walks the entire plan cache every cycle, blocking fast loop | Move to slow track, investigate narrower arguments or sampling schedule |

## 3. Full Query Inventory & Table Scan Risk

Legend: ✅ (fine), ⚠️ (needs filtering/guardrails), ❗️ (full scan / heavy).

| Query Constant | Source | Type | Filters today | Risk | Notes |
| --- | --- | --- | --- | --- | --- |
| `querySystemStatus*`, `querySystemActivity*`, `queryConfiguredCPUs`, `queryAverageCPU`, `querySystemASP`, `queryActiveJobs`, `querySerialNumber`, `querySystemModel` | `TABLE(QSYS2.SYSTEM_STATUS(...))` | Table function | No additional filters | ✅ | Table function already scoped by IBM; negligible overhead |
| `queryMemoryPools*` | `TABLE(QSYS2.MEMORY_POOL(...))` | Table function | Hard-coded pool names | ✅ | Already filtered |
| `queryDiskStatus` | `QSYS2.SYSDISKSTAT` | View | No filter | ⚠️ | Should add unit selector `WHERE UNIT_NUMBER IN (...)` when available |
| `queryDiskInstances`, `queryDiskInstancesEnhanced` | `QSYS2.SYSDISKSTAT` | View | No filter other than Go-side matcher | ⚠️ | Translate selector to `WHERE UNIT_NUMBER IN (...)` when matcher configured |
| `queryCountDisks` | `QSYS2.SYSDISKSTAT` | View | DISTINCT, no filter | ⚠️ | Same as above; cache counts if possible |
| `queryJobInfo` | `TABLE(QSYS2.JOB_INFO(JOB_STATUS_FILTER => '*JOBQ'))` | Table function | Filter built-in | ✅ | Already narrow |
| `queryCountJobQueues`, `queryJobQueues` | `QSYS2.JOB_QUEUE_INFO` | View | No SQL filter; `FETCH FIRST` limits | ❗️ | Needs selector-to-SQL rewrite |
| `queryCountMessageQueues`, `queryMessageQueueAggregates` | `QSYS2.MESSAGE_QUEUE_INFO` | View (detail rows) | No SQL filter | ❗️ | Must switch to table function w/ per-queue sampling |
| `queryCountOutputQueues`, `queryOutputQueueInfo` | `QSYS2.OUTPUT_QUEUE_INFO` | View | No SQL filter | ❗️ | Rewrite like message/job queues |
| `queryNetworkConnections` | `QSYS2.NETSTAT_INFO` | View | Excludes loopback | ⚠️ | Consider host filter support if selectors introduced |
| `queryNetworkInterfaces`, `queryCountNetworkInterfaces` | `QSYS2.NETSTAT_INTERFACE_INFO` | View | `WHERE LINE_DESCRIPTION != '*LOOPBACK'` | ⚠️ | Add optional selector list |
| `queryTempStorageTotal`, `queryTempStorageNamed` | `QSYS2.SYSTMPSTG` | View | Filters global bucket null/not null | ⚠️ | Acceptable cost today; keep under review |
| `querySubsystems`, `queryCountSubsystems` | `QSYS2.SUBSYSTEM_INFO` | View | `WHERE STATUS = 'ACTIVE'` | ✅ | Already filtered |
| `queryTopActiveJobs`, `queryCountActiveJobs` | `TABLE(QSYS2.ACTIVE_JOB_INFO(...))` | Table function | All filters `*ALL` | ❗️ | Needs explicit lists |
| `queryHTTPServerInfo`, `queryCountHTTPServers` | `QSYS2.HTTP_SERVER_INFO` | View | No filter | ⚠️ | Consider selector per server name |
| `queryIBMiVersion`, `queryIBMiVersionDataArea`, `queryTechnologyRefresh` | Metadata views | — | — | ✅ | One-off |
| `callAnalyzePlanCache`, `queryPlanCacheSummary` | Stored proc + temp table | — | None | ❗️ | Run asynchronously with throttling |

## 4. Strategy (phased)

### Phase 1 – Query Rewrite & Filtering
1. **Explicit queue targets only:** retire generic selectors and `max_*` limits for message/job/output queues; accept a list of fully-qualified queue names (library/queue) and run one table-function call per entry. Provide sensible defaults (e.g., `QSYS/QSYSOPR`, `QSYS/QSYSMSG`, `QSYS/QHST`) and keep a simple `collect_*` enable/disable switch for each feature.
2. **Prefer table functions (7.4+):** when available, use IBM’s table functions (`TABLE(QSYS2.MESSAGE_QUEUE_INFO(...))`, `TABLE(QSYS2.ACTIVE_JOB_INFO(...))`, etc.) so the filtering happens via parameters instead of scanning the entire detail view. On older releases fall back to views but construct strict `WHERE library/name IN (...)` clauses from the explicit lists.
3. **Helper for list expansion:** implement shared helpers that turn config lists into SQL fragments and per-collector worklists. If glob support is introduced, expand to explicit names on a periodic cadence and reuse the list so the optimized SQL always receives concrete values (never `LIKE`).
4. **Lightweight counts:** reuse the explicit target list for cardinality checks (length of the list) instead of running `COUNT(*)` scans where possible; otherwise cache the last-known count and refresh less frequently.

### Phase 2 – Scheduler & Parallelization
1. Implement multi-track scheduler in the ibm.d framework so slow groups (message/output queues, plan cache, job queues) run in parallel to the 5 s fast loop.
2. Assign ODBC connections per track (limited pool) and warehouse results via channels to avoid data races.
3. Ensure dump mode / shutdown handle background goroutines.

### Phase 3 – Instrumentation & Validation
1. Extend query latency tracking to name every query (remove “other” bucket).
2. Add debug telemetry to confirm selector expansion and SQL rewriting.
3. Establish regression test fixtures (mock DB responses) for selector translation and per-queue sampling.

## 5. Selector Handling & Glob Patterns
- Current usage does not rely on glob patterns, but long-term usability may. Proposed approach:
  1. Accept plain lists (preferred). For globs, expand to explicit queue/library names on a cadence (e.g., refresh every minute using targeted list queries).
  2. Build per-feature caches that hold resolved names and feed them into the optimized SQL (`WHERE library/name IN (...)`) or per-queue execution list.
  3. When expansion yields no matches, skip the heavy query entirely to avoid wasted scans.

## 6. Action Plan

1. **Message queues:** (✅ done) explicit list + table function (7.4+) / documented behaviour on older releases.
2. **Job queues:** translate selectors to SQL, add per-group execution, cache lists; move to slow track. Research 7.4+ scoped SQL service (see docs task).
3. **Output queues:** mirror job queue treatment, including spool file considerations. Research scoped SQL service where available.
4. **Active jobs:** introduce configuration for subsystem/job inclusion lists, adjust table function parameters, and run on slow track.
5. **Plan cache:** throttle invocation (e.g., every N iterations) and dispatch on dedicated slow track.
6. **Selector infrastructure:** build reusable helper that expands globs (if any) into explicit lists and share it across queues/disks/interfaces.
7. **Instrumentation:** name all queries in latency map, add logging for large result sets, and monitor improvements via `netdata.plugin_ibm.as400_query_latency`.

## 7. Outstanding Questions
- Confirm IBM i versions we must support (table functions require 7.4+). Decide on fallback mechanism for older releases.
- Determine acceptable concurrency level per partition (number of simultaneous ODBC connections).
- Validate whether additional IBM services (e.g., HTTP server info) provide table functions or existing views can be filtered by name list.
- Decide cadence for selector refresh (e.g., once per minute vs. per iteration) balancing staleness vs. overhead.
