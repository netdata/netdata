# Database Query Functions

## Overview

Database query functions provide deep visibility into SQL and NoSQL database performance through Netdata's Top tab. They help you identify problematic queries, detect deadlocks, and understand error patterns—all from the Netdata dashboard.

| Capability | Description |
|------------|-------------|
| **Top Queries** | Identify the most expensive queries by execution time, I/O, rows processed, or other metrics |
| **Running Queries** | See currently executing queries in real-time |
| **Deadlock Detection** | View the latest detected deadlock with full transaction details |
| **Error Attribution** | Correlate SQL errors with the queries that caused them |

## Supported Databases

| Database | Top Queries | Running Queries | Deadlock Info | Error Info | Integration Docs |
|----------|:-----------:|:---------------:|:-------------:|:----------:|------------------|
| ClickHouse | ✅ | - | - | - | [ClickHouse](/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md) |
| CockroachDB | ✅ | ✅ | - | - | [CockroachDB](/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md) |
| Couchbase | ✅ | - | - | - | [Couchbase](/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md) |
| Elasticsearch | ✅ | - | - | - | [Elasticsearch](/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md) |
| MongoDB | ✅ | - | - | - | [MongoDB](/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md) |
| Microsoft SQL Server | ✅ | - | ✅ | ✅* | [MSSQL](/src/go/plugin/go.d/collector/mssql/integrations/microsoft_sql_server.md) |
| MySQL | ✅ | - | ✅ | ✅* | [MySQL](/src/go/plugin/go.d/collector/mysql/integrations/mysql.md) |
| MariaDB | ✅ | - | ✅ | ✅* | [MariaDB](/src/go/plugin/go.d/collector/mysql/integrations/mariadb.md) |
| Percona Server | ✅ | - | ✅ | ✅* | [Percona](/src/go/plugin/go.d/collector/mysql/integrations/percona_mysql.md) |
| Oracle Database | ✅ | ✅ | - | - | [Oracle](/src/go/plugin/go.d/collector/oracledb/integrations/oracle_db.md) |
| PostgreSQL | ✅ | ✅ | - | - | [PostgreSQL](/src/go/plugin/go.d/collector/postgres/integrations/postgresql.md) |
| ProxySQL | ✅ | - | - | - | [ProxySQL](/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md) |
| Redis | ✅ | - | - | - | [Redis](/src/go/plugin/go.d/collector/redis/integrations/redis.md) |
| RethinkDB | - | ✅ | - | - | [RethinkDB](/src/go/plugin/go.d/collector/rethinkdb/integrations/rethinkdb.md) |
| YugabyteDB | ✅ | ✅ | - | - | [YugabyteDB](/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md) |

*\* Error Info is integrated directly into Top Queries results—each query row shows its associated errors.*

## Function Types

### Top Queries

Retrieves **accumulated query statistics** over a time window. These are aggregated metrics (total calls, total time, average time, etc.) from the database's query statistics infrastructure—not real-time snapshots.

**Filter options** vary by database but typically include:
- Execution time (total, average)
- Call count
- Rows processed (read, written, returned)
- I/O metrics (logical reads, physical reads)
- Resource usage (CPU, memory, locks)

The number of queries returned is configurable (default: 500). This is a two-stage process:
1. **Server-side**: Database returns the top N queries ranked by your chosen metric
2. **Client-side**: UI can further sort, filter, and explore the returned data

### Running Queries

Shows **currently executing queries** at the moment of request. Essential for diagnosing stuck queries, long-running transactions, or unexpected load.

**Supported**: CockroachDB, Oracle, PostgreSQL, RethinkDB, YugabyteDB

### Deadlock Info

Displays the **most recently detected deadlock**—not a historical list. When a new deadlock occurs, it replaces the previous one.

**Supported**: MySQL/MariaDB/Percona, Microsoft SQL Server

Information provided:
- Deadlock timestamp and ID
- Participating transactions
- Victim transaction (rolled back)
- Query text and lock details
- Wait resource

### Error Info

Shows **recent SQL errors** from the database's error history. Error attribution is embedded directly in Top Queries results—each query row includes error details when available.

**Supported**: MySQL/MariaDB/Percona, Microsoft SQL Server

Attribution status values:
- `enabled` — Error details available for this query
- `no_data` — No recent errors for this query
- `not_enabled` — Error tracking not configured
- `not_supported` — Database version lacks required features

## Security Considerations

### Query Text Exposure

Some databases normalize queries (replacing literals with placeholders), while others show actual values that may contain sensitive data:

| Database | Query Text |
|----------|:----------:|
| ClickHouse | Normalized |
| CockroachDB | ⚠️ Raw |
| Couchbase | ⚠️ Raw |
| Elasticsearch | ⚠️ Raw |
| MongoDB | ⚠️ Raw |
| Microsoft SQL Server | ⚠️ Raw |
| MySQL/MariaDB/Percona | Normalized (Top Queries), ⚠️ Raw (Deadlock) |
| Oracle | ⚠️ Raw |
| PostgreSQL | Normalized |
| ProxySQL | Normalized |
| Redis | ⚠️ Raw |
| RethinkDB | ⚠️ Raw |
| YugabyteDB | ⚠️ Raw |

**Legend**:
- **Normalized**: Literals replaced with placeholders (`SELECT * FROM users WHERE id = ?`)
- **⚠️ Raw**: May contain actual values (`SELECT * FROM users WHERE id = 12345`)

:::caution

Error messages may contain sensitive values regardless of query normalization. Ensure appropriate access controls are in place.

:::

### Recommendations

1. **Disable unneeded functions** — Each integration supports configuration options to disable specific functions
2. **Use Netdata Cloud access controls** — Assign users to appropriate Rooms and Roles
3. **Use dedicated database users** — Grant only the permissions required for monitoring

## Getting Started

Each database requires specific setup. Click the integration link in the table above for:

- Prerequisites and permissions required
- Configuration options
- Available metrics and columns
- Database-specific notes

## Related Documentation

- [Top Consumers Overview](/docs/top-monitoring-netdata-functions.md)
- [Processes Function](/docs/functions/processes.md)
