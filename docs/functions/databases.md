# Function: Database Queries

## Quick Info

- **Plugin**: `go.d.plugin`
- **Type**: Simple Table (real-time and historical data)
- **Availability**: 13 databases supported
- **Required Access**: Database-specific permissions (see per-database sections)

## Purpose

Database query functions provide deep visibility into SQL and NoSQL database performance, helping you identify problematic queries, detect deadlocks, and understand error patterns—all without leaving Netdata.

Traditional database monitoring shows aggregate metrics (queries per second, average latency), but when performance degrades, you need to know *which specific queries* are responsible. These functions bridge that gap by exposing query-level details directly in the Netdata dashboard.

### Key Capabilities

- **Top Queries Analysis**: Identify the most expensive queries by execution time, I/O, rows processed, or other metrics
- **Running Queries**: See currently executing queries in real-time (supported databases)
- **Deadlock Detection**: View the latest detected deadlock with full transaction details
- **Error Attribution**: Correlate SQL errors with the queries that caused them

### The Value You Get

| Scenario | Without This | With Database Functions |
|----------|--------------|------------------------|
| Slow application response | "Database is slow" | "Query X on table Y takes 2.3s avg, examining 1.2M rows" |
| High CPU on database server | "Something is consuming CPU" | "Top 5 queries by CPU time, sorted by impact" |
| Intermittent errors | "Users report random failures" | "Error 1205 (lock wait timeout) on query Z, 47 times today" |
| Application deadlock | "App hangs occasionally" | "Deadlock between transactions A and B on table C" |

## Function Support Matrix

| Database | Top Queries | Running Queries | Deadlock Info | Error Info |
|----------|:-----------:|:---------------:|:-------------:|:----------:|
| [ClickHouse](#clickhouse) | ✅ | - | - | - |
| [CockroachDB](#cockroachdb) | ✅ | ✅ | - | - |
| [Couchbase](#couchbase) | ✅ | - | - | - |
| [Elasticsearch](#elasticsearch) | ✅ | - | - | - |
| [MongoDB](#mongodb) | ✅ | - | - | - |
| [Microsoft SQL Server](#microsoft-sql-server) | ✅ | - | ✅ | ✅* |
| [MySQL / MariaDB](#mysql--mariadb) | ✅ | - | ✅ | ✅* |
| [Oracle Database](#oracle-database) | ✅ | ✅ | - | - |
| [PostgreSQL](#postgresql) | ✅ | - | - | - |
| [ProxySQL](#proxysql) | ✅ | - | - | - |
| [Redis](#redis) | ✅ | - | - | - |
| [RethinkDB](#rethinkdb) | - | ✅ | - | - |
| [YugabyteDB](#yugabytedb) | ✅ | ✅ | - | - |

*\* Error Info is integrated directly into Top Queries results—each query row shows its associated errors.*

## Top Queries Function

**Data type**: Historical/Aggregated

The Top Queries function retrieves **accumulated query statistics** over a time window. These are not real-time snapshots but aggregated metrics (total calls, total time, average time, etc.) collected by the database's query statistics infrastructure.

| Database | Time Window |
|----------|-------------|
| ClickHouse | Query log retention (configurable) |
| CockroachDB | Since last stats reset |
| Couchbase | Completed requests buffer |
| Elasticsearch | Currently running (real-time) |
| MongoDB | Profiler collection (configurable retention) |
| MSSQL | Query Store window (default: 7 days, configurable) |
| MySQL | Since last performance_schema reset |
| Oracle | Since last stats reset or instance startup |
| PostgreSQL | Since last pg_stat_statements reset |
| ProxySQL | Since last stats reset |
| Redis | SLOWLOG entries (configurable count) |
| YugabyteDB | Since last pg_stat_statements reset |

This is a two-stage process:

1. **Filter by** (server-side): The database returns the top N queries ranked by your chosen metric (e.g., "Top queries by Execution Time" fetches the most time-consuming queries)
2. **Sort/Filter** (client-side): The UI can then sort, filter, and explore the returned data by any available column

The number of queries fetched is configurable (default: 500, max: 5000). This limit applies to the server-side filter—you're seeing the top N by your chosen metric, not a random sample.

### Filter Options by Database

The "Filter by" dropdown determines which queries are fetched from the database. Available options vary by database:

| Database | Available Filter Options |
|----------|-------------------------|
| **ClickHouse** | Calls, Total Time, Avg Time, Rows Read |
| **CockroachDB** | Calls, Total Time, Avg Time, Rows Read, Rows Written, Network Bytes, Max Memory, Contention Time |
| **Couchbase** | Request Time, Elapsed Time, Service Time, Result Count |
| **Elasticsearch** | Start Time, Running Time |
| **MongoDB** | Execution Time, Docs Examined, Keys Examined, Docs Returned, Docs Deleted/Inserted/Modified, Response Length, Num Yield, Planning Time, CPU Time |
| **MSSQL** | Calls, Total Time, Avg CPU, Avg Logical Reads/Writes, Avg Physical Reads, Avg Parallelism (DOP), Avg Memory Grant, Avg Row Count, Avg Log Bytes, Avg TempDB Usage |
| **MySQL** | Calls, Total Time, Avg Time, Lock Time, Errors, Rows Affected/Sent/Examined, Temp Disk Tables, Full Joins, Table Scans, Rows Sorted, No Index Used, P95/P99 Time, CPU Time, Memory |
| **Oracle** | Executions, Total Time, Avg Time, CPU Time, Buffer Gets, Disk Reads, Rows Processed, Parse Calls |
| **PostgreSQL** | Calls, Total Time, Avg Time, Rows Returned, Shared Blocks Hit (Cache), Shared Blocks Read (Disk), Temp Blocks Written |
| **ProxySQL** | Calls, Total Time, Avg Time, Rows Affected, Rows Sent, Errors, Warnings |
| **Redis** | Duration (from SLOWLOG) |
| **YugabyteDB** | Calls, Total Time, Mean Time, Max Time, Rows Returned |

### Common Use Cases

1. **Find slow queries**: Filter by "Total Execution Time" or "Avg Execution Time"
2. **Find I/O-heavy queries**: Filter by "Disk Reads", "Rows Examined", or "Shared Blocks Read"
3. **Find memory-hungry queries**: Filter by "Memory Grant" (MSSQL), "Max Memory" (CockroachDB)
4. **Find lock contention**: Filter by "Lock Time" (MySQL) or "Contention Time" (CockroachDB)
5. **Find inefficient queries**: Filter by "Full Joins", "Table Scans", or "No Index Used" (MySQL)

Once the queries are fetched, use the UI to further sort, filter by text, or group by schema/database/user.

## Running Queries Function

**Data type**: Real-time

Shows **currently executing queries** at the moment of request. Each refresh fetches a fresh snapshot of active queries. Essential for diagnosing stuck queries, long-running transactions, or unexpected load.

**Supported databases**: CockroachDB, Oracle, RethinkDB, YugabyteDB

| Database | Key Information Shown |
|----------|----------------------|
| **CockroachDB** | Query, Node, Phase, Distributed flag, Elapsed time |
| **Oracle** | SQL text, SID, Serial#, Username, Machine, Status, Elapsed time |
| **RethinkDB** | Query, Type, User, Client, Servers, Duration |
| **YugabyteDB** | Query, Database, User, Client, State, Elapsed time |

## Deadlock Info Function

**Data type**: Point-in-time (last occurrence)

Displays the **most recently detected deadlock** only—not a historical list. When a new deadlock occurs, it replaces the previous one. Critical for diagnosing intermittent application hangs.

If no deadlock has occurred since database startup (or since the deadlock info was cleared), this function returns empty results.

**Supported databases**: MySQL, Microsoft SQL Server

### Information Provided

| Field | Description |
|-------|-------------|
| Deadlock ID | Unique identifier based on detection timestamp |
| Timestamp | When the deadlock was detected (UTC) |
| Process/Transaction ID | Database-specific transaction identifier |
| Is Victim | Which transaction was rolled back to resolve the deadlock |
| Query Text | The SQL statement involved |
| Lock Mode | Type of lock (e.g., "REC_NOT_GAP", "X", "S") |
| Lock Status | "WAITING" or "HOLDS" |
| Wait Resource | The resource being contested |
| Database | Database name |

### MySQL-Specific Notes

- Data source: `SHOW ENGINE INNODB STATUS` (deadlock section)
- Requires PROCESS privilege
- Shows InnoDB deadlocks only

### MSSQL-Specific Notes

- Data source: `system_health` Extended Events session (XML deadlock graphs)
- Requires VIEW SERVER STATE permission
- Parses XML deadlock graph for detailed transaction info

## Error Info Function

**Data type**: Recent (rolling window)

Shows **recent SQL errors** from the database's error history buffer. The retention depends on database configuration:

- **MySQL**: `events_statements_history_long` table size (configurable, typically last 10,000 statements)
- **MSSQL**: Extended Events ring buffer size (configurable)

**Supported databases**: MySQL, Microsoft SQL Server

**Important**: Error attribution is **embedded directly in the Top Queries results**. When you view top queries, each row includes an `errorAttribution` column showing whether errors occurred for that query, along with error details (error number, message, and SQL state for MySQL / error state for MSSQL).

This means you don't need to correlate two separate views—slow queries and their errors appear together.

### MySQL Error Info

- **Data source**: `performance_schema.events_statements_history_long` (or `events_statements_history`)
- **Columns added to Top Queries**: `errorAttribution`, `errorNumber`, `sqlState`, `errorMessage`
- **Attribution status**: `enabled` (errors found), `no_data` (no errors), `not_enabled` (history tables disabled), `not_supported`

### MSSQL Error Info

- **Data source**: User-managed Extended Events session
- **Columns added to Top Queries**: `errorAttribution`, `errorNumber`, `errorState`, `errorMessage`
- **Linked by**: Query hash (matches top queries to their errors)
- **Setup required**: Create an Extended Events session to capture `error_reported` events

```sql
CREATE EVENT SESSION netdata_errors ON SERVER
ADD EVENT sqlserver.error_reported (
    ACTION (sqlserver.sql_text, sqlserver.query_hash)
)
ADD TARGET package0.ring_buffer
WITH (STARTUP_STATE=ON);
GO
ALTER EVENT SESSION netdata_errors ON SERVER STATE = START;
```

---

## Database-Specific Details

### ClickHouse

**Data Source**: `system.query_log`

ClickHouse's query log provides comprehensive execution statistics. The function dynamically detects available columns based on your ClickHouse version.

**Key Metrics**:
- Execution time (total, avg, min, max)
- Rows read/written, bytes read/written
- Result rows/bytes
- Memory usage

**Requirements**: Read access to `system.query_log` and `system.columns`

### CockroachDB

**Data Source**: `crdb_internal.node_statement_statistics` (top queries), `SHOW CLUSTER STATEMENTS` (running queries)

CockroachDB provides detailed distributed query statistics including network transfer and contention metrics unique to distributed databases.

**Key Metrics**:
- Execution count and time
- Rows read/written
- Network bytes sent
- Max memory usage
- Contention time (lock waiting)
- Service latency percentiles (P50, P90, P99)

**Requirements**: Read access to `crdb_internal` schema

### Couchbase

**Data Source**: `system:completed_requests` (N1QL query service)

Couchbase exposes completed N1QL query information through its query service API.

**Key Metrics**:
- Elapsed time vs Service time (helps identify queuing delays)
- Result count and size
- Error and warning counts
- User and client context

**Requirements**: Query service must be enabled; read access to system keyspace

### Elasticsearch

**Data Source**: Tasks API (`/_tasks?actions=*search&detailed=true`)

Elasticsearch shows currently running search tasks across the cluster. This is effectively a "running queries" view rather than historical aggregation.

**Key Metrics**:
- Running time
- Node name and ID
- Action type
- Task description (includes search parameters)
- Cancellable/cancelled status

**Requirements**: Access to Tasks API

### MongoDB

**Data Source**: `system.profile` collection

MongoDB's profiler captures slow operations. You must enable profiling to use this function.

**Key Metrics**:
- Execution time, planning time, CPU time
- Documents examined, keys examined, documents returned
- Plan summary (index usage)
- Yield count (lock releases)

**Setup Required**:
```javascript
// Enable profiling (level 1 = slow ops, level 2 = all ops)
db.setProfilingLevel(1, { slowms: 100 })
```

**Note**: Query text may contain unmasked literals (potential PII). Consider profiling level and security implications.

### Microsoft SQL Server

**Data Source**: Query Store (`sys.query_store_*` views)

MSSQL's Query Store provides rich historical query performance data with extensive metrics.

**Key Metrics**:
- Execution count and duration
- CPU time, logical/physical reads, logical writes
- Degree of parallelism (DOP)
- Memory grant usage
- Row count, log bytes, TempDB usage

**Configuration Options**:
- `query_store_time_window_days`: Look-back period (default: 7 days, 0 = all available)
- `top_queries_limit`: Maximum queries returned (default: 500)

**Requirements**: Query Store must be enabled; VIEW SERVER STATE permission

### MySQL / MariaDB

**Data Source**: `performance_schema.events_statements_summary_by_digest`

MySQL's Performance Schema provides the most comprehensive query metrics among all supported databases, with version-specific enhancements.

**Key Metrics**:
- Execution count, time (total/avg/min/max), lock time
- Rows affected/sent/examined
- Temp tables (memory and disk)
- Join and sort statistics
- Index usage indicators
- Percentiles (P95, P99, P999) - MySQL 8.0+
- CPU time - MySQL 8.0.28+
- Memory usage - MySQL 8.0.31+

**Requirements**: Performance Schema enabled; SELECT on performance_schema

### Oracle Database

**Data Source**: `V$SQLSTATS` (top queries), `V$SESSION` (running queries)

Oracle provides both aggregated statistics and real-time session information.

**Key Metrics**:
- Executions, elapsed time, CPU time
- Buffer gets (logical I/O), disk reads (physical I/O)
- Rows processed
- Parse calls

**Requirements**: SELECT on V$SQLSTATS, V$SESSION, and related views

### PostgreSQL

**Data Source**: `pg_stat_statements` extension

PostgreSQL requires the pg_stat_statements extension for query statistics.

**Key Metrics**:
- Calls, total/mean execution time
- Rows returned
- Shared blocks hit (cache) vs read (disk)
- Temp blocks written

**Setup Required**:
```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

**Note**: Column names changed in PostgreSQL 13+ (e.g., `total_time` → `total_exec_time`). The collector handles this automatically.

### ProxySQL

**Data Source**: `stats_mysql_query_digest`

ProxySQL aggregates query statistics across all backend MySQL servers.

**Key Metrics**:
- Execution count and time
- Rows affected/sent
- Errors and warnings
- First/last seen timestamps

**Requirements**: Admin interface access

### Redis

**Data Source**: `SLOWLOG GET`

Redis SLOWLOG captures commands that exceeded the configured slowlog threshold.

**Key Metrics**:
- Command duration
- Command name and arguments
- Client address and name
- Timestamp

**Note**: Command arguments may contain sensitive data. Configure `slowlog-log-slower-than` to control what gets logged.

### RethinkDB

**Data Source**: `rethinkdb.jobs` system table

RethinkDB exposes currently running queries through its jobs system table.

**Key Metrics**:
- Duration
- Query type
- User
- Client address
- Servers involved

**Requirements**: Read access to `rethinkdb.jobs`

### YugabyteDB

**Data Source**: `pg_stat_statements` (top queries), `pg_stat_activity` (running queries)

YugabyteDB, being PostgreSQL-compatible, uses the same statistics infrastructure.

**Key Metrics**:
- Calls, total/mean/max time
- Rows returned
- Database and user context

**Requirements**: pg_stat_statements extension enabled

---

## Security Considerations

### PII Exposure by Function

Not all functions expose raw query text. Some databases normalize queries (replacing literals with placeholders like `?` or `$1`), while others show actual values.

| Database | Top Queries | Running Queries | Deadlock Info |
|----------|:-----------:|:---------------:|:-------------:|
| ClickHouse | Normalized | - | - |
| CockroachDB | ⚠️ Raw | ⚠️ Raw | - |
| Couchbase | ⚠️ Raw | - | - |
| Elasticsearch | ⚠️ Raw | - | - |
| MongoDB | ⚠️ Raw | - | - |
| MSSQL | ⚠️ Raw | - | ⚠️ Raw |
| MySQL | Normalized | - | ⚠️ Raw |
| Oracle | ⚠️ Raw | ⚠️ Raw | - |
| PostgreSQL | Normalized | - | - |
| ProxySQL | Normalized | - | - |
| Redis | ⚠️ Raw | - | - |
| RethinkDB | - | ⚠️ Raw | - |
| YugabyteDB | ⚠️ Raw | ⚠️ Raw | - |

**Legend**:
- **Normalized**: Query text has literals replaced with placeholders (`SELECT * FROM users WHERE id = ?`)
- **⚠️ Raw**: Query text may contain actual values (`SELECT * FROM users WHERE id = 12345`)

**Note**: Error messages (in Error Info) may contain sensitive values regardless of query normalization

**What you can do**:

1. **Disable functions you don't need** - In each collector's configuration, set `*_function_enabled: false`:
   ```yaml
   # Example: mysql.conf
   jobs:
     - name: local
       dsn: user:pass@tcp(localhost:3306)/
       deadlock_info_function_enabled: false
       error_info_function_enabled: false
   ```

2. **Use Netdata Cloud access controls** - Assign users to appropriate Rooms and Roles to limit who can access these functions

3. **Use a dedicated database user** - Create a monitoring user with only the permissions required (see per-database requirements above)

## Performance Considerations

- Functions execute queries against database system tables/views
- Large query history (high `top_queries_limit`) increases response time
- Running queries functions have minimal overhead
- Consider adjusting collection intervals for busy systems

## Related Documentation

- [Top Consumers Overview](/docs/top-monitoring-netdata-functions.md)
- [Processes Function](/docs/functions/processes.md)
- Individual collector documentation in the [Integrations](/src/collectors/COLLECTORS.md) section
