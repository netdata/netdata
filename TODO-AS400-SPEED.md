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
| `message_queue_aggregates`, `count_message_queues` | `QSYS2.MESSAGE_QUEUE_INFO` (detail table, full scan) | Full table scan, aggregation, `FETCH FIRST` post-scan | ✅ done (table function per queue, documented fallback + defaults) |
| `collectJobQueues` (`queryJobQueues`) | `QSYS2.JOB_QUEUE_INFO` | View per queue with WHERE filter; still executes every fast loop | ✅ done (slow-path cache with per-queue fan-out and concurrency) |
| `collectOutputQueues` (`queryOutputQueueInfo`) | `QSYS2.OUTPUT_QUEUE_INFO` | View-based per queue | ✅ done (table function per queue with view fallback; counts derive from entries) |
| `collectActiveJobs` (`buildActiveJobQuery`) | `TABLE(QSYS2.ACTIVE_JOB_INFO(...))` per job | ✅ done (requires explicit `active_jobs` list; one table-function call per job with user/job filters, skips when not found) |
| `collectSubsystems` (`querySubsystems` + `queryCountSubsystems`) | `QSYS2.SUBSYSTEM_INFO` | View | Aggregates active subsystems every 15 s | ✅ done (slow-path cache with interval override) |
| `collectPlanCache` (`CALL QSYS2.ANALYZE_PLAN_CACHE('03', …)`) | Stored procedure | Walks the entire plan cache every cycle, blocking fast loop | ✅ done (runs on slow-path worker; latency separated) |

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
| `queryCountJobQueues`, `queryJobQueues` | `QSYS2.JOB_QUEUE_INFO` | View | Per-queue WHERE clause | ⚠️ | Live data only; keep filtered view but shift work to slow producer and optionally enrich with `SYSTOOLS.JOB_QUEUE_ENTRIES` (7.4 TR9+) |
| `queryCountMessageQueues`, `queryMessageQueueAggregates` | `QSYS2.MESSAGE_QUEUE_INFO` | View (detail rows) | No SQL filter | ❗️ | Must switch to table function w/ per-queue sampling |
| `queryCountOutputQueues`, `queryOutputQueueInfo` | `QSYS2.OUTPUT_QUEUE_INFO` | View | No SQL filter | ❗️ | Rewrite like message/job queues |
| `queryNetworkConnections` | `QSYS2.NETSTAT_INFO` | View | Excludes loopback | ⚠️ | Consider host filter support if selectors introduced |
| `queryNetworkInterfaces`, `queryCountNetworkInterfaces` | `QSYS2.NETSTAT_INTERFACE_INFO` | View | `WHERE LINE_DESCRIPTION != '*LOOPBACK'` | ⚠️ | Add optional selector list |
| `queryTempStorageTotal`, `queryTempStorageNamed` | `QSYS2.SYSTMPSTG` | View | Filters global bucket null/not null | ⚠️ | Acceptable cost today; keep under review |
| `querySubsystems`, `queryCountSubsystems` | `QSYS2.SUBSYSTEM_INFO` | View | `WHERE STATUS = 'ACTIVE'` | ✅ | Already filtered |
| `buildActiveJobQuery` | `TABLE(QSYS2.ACTIVE_JOB_INFO(...))` | Table function | Filters by job name/user; per-job query | ⚠️ | Executes once per configured job; still tied to number of tracked jobs but avoids full system scan |
| `queryHTTPServerInfo`, `queryCountHTTPServers` | `QSYS2.HTTP_SERVER_INFO` | View | No filter | ⚠️ | Consider selector per server name |
| `queryIBMiVersion`, `queryIBMiVersionDataArea`, `queryTechnologyRefresh` | Metadata views | — | — | ✅ | One-off |
| `callAnalyzePlanCache`, `queryPlanCacheSummary` | Stored proc + temp table | — | None | ❗️ | Run asynchronously with throttling |

## 4. Strategy (phased)

### Phase 1 – Query Rewrite & Filtering
1. **Explicit queue targets only:** retire generic selectors and `max_*` limits for message/job/output queues; accept a list of fully-qualified queue names (library/queue) and run one table-function call per entry. Provide sensible defaults (e.g., `QSYS/QSYSOPR`, `QSYS/QSYSMSG`, `QSYS/QHST`) and keep a simple `collect_*` enable/disable switch for each feature.
2. **Prefer table functions (7.4+):** when available, use IBM’s table functions (`TABLE(QSYS2.MESSAGE_QUEUE_INFO(...))`, `TABLE(QSYS2.ACTIVE_JOB_INFO(...))`, etc.) so the filtering happens via parameters instead of scanning the entire detail view. On older releases fall back to views but construct strict `WHERE library/name IN (...)` clauses from the explicit lists.
3. **Helper for list expansion:** implement shared helpers that turn config lists into SQL fragments and per-collector worklists. If glob support is introduced, expand to explicit names on a periodic cadence and reuse the list so the optimized SQL always receives concrete values (never `LIKE`).
4. **Lightweight counts:** reuse the explicit target list for cardinality checks (length of the list) instead of running `COUNT(*)` scans where possible; otherwise cache the last-known count and refresh less frequently.
5. **Output queues:** ✅ implemented via `OUTPUT_QUEUE_ENTRIES` with view fallback.

### Phase 2 – Background Producers & Staging
1. Introduce a dedicated slow-path worker (“producer”) that runs on aligned beats (default 30 s) to execute heavy collectors (`message_queue_*`, `job_queue_*`, `output_queue_*`, `count_subsystems`/`collectSubsystems`, plan cache). Producer keeps its own `context.Context` so it stops when the job stops or configuration reloads.
2. Expose new configuration knobs:
   - `slow_path` (bool, default `true`) to enable/disable the background worker.
   - `slow_path_update_every` (duration, default `30s`, must be ≥ main `update_every`) to control beat cadence.
   - `slow_path_max_connections` (int, default `1`) to set `db.SetMaxOpenConns` for the slow worker so queries can run concurrently when increased.
3. Convert the existing sequential collectors into “consumers” for the heavy datasets: on each fast loop they read from the slow-path cache (values + timestamps + error state) and export data only when fresh. Fast loop stays synchronous for lightweight collectors (system status, memory pools, disks, temp storage, job info, active jobs, network stats, HTTP servers, etc.).
4. Implement the cache layer with per-feature entries (latest value, timestamp, last error) and proper locking. Bring query-latency logging inside the slow producer so timing reflects the asynchronous work.
5. Ensure lifecycle management: producer goroutines respect `slow_path` changes and job shutdown (cancel + wait); structure the code so we can register additional slow tracks in the future without refactoring.

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
2. **Job queues:** keep the `QSYS2.JOB_QUEUE_INFO` per-queue WHERE filtering, but execute it from the slow producer cache and, when present, enrich results with `SYSTOOLS.JOB_QUEUE_ENTRIES` (7.4 TR9+/7.5 TR3+) without increasing call volume.
3. **Output queues:** mirror job queue treatment, including spool file considerations. (Research complete: use `QSYS2.OUTPUT_QUEUE_ENTRIES(library, queue, detail)` table function on 7.2+; fall back to view if unavailable.)
4. **Active jobs:** ✅ complete — configuration now requires explicit `active_jobs` list; per-job table-function queries replace global scan.
5. **Subsystem inventory:** migrate to slow producer/cache, publishing active subsystem metrics only when refreshed.
6. **Plan cache:** migrate to the slow producer (dedicated cadence & timeout); only publish when fresh data arrives.
7. **Producer/consumer cache:** implement the background fetchers, shared caches (values + timestamps + errors), and consumer-side merge logic so fast loops simply read whichever data is ready. Slow worker should support concurrent query fan-out per `slow_path_max_connections`.
8. **SetUpdateEvery plumbing:** after the cache layer exists, wire per-context update intervals so slow charts report their natural cadence.
9. **Selector infrastructure:** build reusable helper that expands globs (if any) into explicit lists and share it across queues/disks/interfaces.
10. **Instrumentation:** name all queries in latency map, add logging for large result sets, and monitor improvements via `netdata.plugin_ibm.as400_query_latency`, splitting fast/slow instances so both timings are visible.

## 7. Outstanding Questions
- Confirm IBM i versions we must support (table functions require 7.4+). Decide on fallback mechanism for older releases.
- Determine acceptable concurrency level per partition (number of simultaneous ODBC connections).
- Validate whether additional IBM services (e.g., HTTP server info) provide table functions or existing views can be filtered by name list.
- Decide cadence for selector refresh (e.g., once per minute vs. per iteration) balancing staleness vs. overhead.
