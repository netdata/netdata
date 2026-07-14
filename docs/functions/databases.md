# Database Query Functions

Database collectors can expose on-demand Functions for investigating expensive queries, currently running work, deadlocks, and recent errors. The available Functions and returned fields depend on the database and collector version.

Use the integration documentation linked below as the source of truth for database privileges, collector configuration, query semantics, filters, and result columns.

## Supported Capabilities

| Database                 | Top queries | Running queries | Deadlock info | Error info | Integration documentation                                                                        |
|:-------------------------|:-----------:|:---------------:|:-------------:|:----------:|:-------------------------------------------------------------------------------------------------|
| ClickHouse               |      ✓      |                 |               |            | [ClickHouse](/src/go/plugin/go.d/collector/clickhouse/integrations/clickhouse.md)                |
| CockroachDB              |      ✓      |        ✓        |               |            | [CockroachDB](/src/go/plugin/go.d/collector/cockroachdb/integrations/cockroachdb.md)             |
| Couchbase                |      ✓      |                 |               |            | [Couchbase](/src/go/plugin/go.d/collector/couchbase/integrations/couchbase.md)                   |
| Elasticsearch            |      ✓      |                 |               |            | [Elasticsearch](/src/go/plugin/go.d/collector/elasticsearch/integrations/elasticsearch.md)       |
| OpenSearch               |      ✓      |                 |               |            | [OpenSearch](/src/go/plugin/go.d/collector/elasticsearch/integrations/opensearch.md)             |
| MongoDB                  |      ✓      |                 |               |            | [MongoDB](/src/go/plugin/go.d/collector/mongodb/integrations/mongodb.md)                         |
| Microsoft SQL Server     |      ✓      |                 |       ✓       |     ✓      | [Microsoft SQL Server](/src/go/plugin/go.d/collector/mssql/integrations/microsoft_sql_server.md) |
| MySQL                    |      ✓      |                 |       ✓       |     ✓      | [MySQL](/src/go/plugin/go.d/collector/mysql/integrations/mysql.md)                               |
| MariaDB                  |      ✓      |                 |       ✓       |     ✓      | [MariaDB](/src/go/plugin/go.d/collector/mysql/integrations/mariadb.md)                           |
| Percona Server for MySQL |      ✓      |                 |       ✓       |     ✓      | [Percona Server for MySQL](/src/go/plugin/go.d/collector/mysql/integrations/percona_mysql.md)    |
| Oracle Database          |      ✓      |        ✓        |               |            | [Oracle Database](/src/go/plugin/go.d/collector/oracledb/integrations/oracle_db.md)              |
| PostgreSQL               |      ✓      |        ✓        |               |            | [PostgreSQL](/src/go/plugin/go.d/collector/postgres/integrations/postgresql.md)                  |
| ProxySQL                 |      ✓      |                 |               |            | [ProxySQL](/src/go/plugin/go.d/collector/proxysql/integrations/proxysql.md)                      |
| Redis                    |      ✓      |                 |               |            | [Redis](/src/go/plugin/go.d/collector/redis/integrations/redis.md)                               |
| RethinkDB                |             |        ✓        |               |            | [RethinkDB](/src/go/plugin/go.d/collector/rethinkdb/integrations/rethinkdb.md)                   |
| YugabyteDB               |      ✓      |        ✓        |               |            | [YugabyteDB](/src/go/plugin/go.d/collector/yugabytedb/integrations/yugabytedb.md)                |

## Function Families

### Top Queries

Ranks accumulated query or command statistics using information exposed by the database. Ranking dimensions, time windows, normalization, and reset behavior vary by database. Treat the selected integration's description as authoritative.

Use this Function to identify workload that contributes disproportionately to latency, executions, I/O, rows, CPU, or another database-specific measure.

### Running Queries

Returns work executing when the Function is called. Use it to investigate long-running statements, blocked work, unexpected load, or active transactions. This is a live snapshot, not query history.

### Deadlock Info

Returns the deadlock detail retained by the database or collector. Retention and replacement behavior are database-specific; do not treat the result as a complete deadlock archive.

### Error Info

Returns recent error information exposed by supported MySQL-family and Microsoft SQL Server collectors. Some collectors can associate those errors with entries in top-query results. The integration documentation describes the required database features and privileges.

## Access and Data Handling

Database Function results can contain statement text, object names, user names, client addresses, error messages, or literal values. These may expose secrets, personal data, or business data.

- Grant the collector only the database permissions documented by its integration.
- Restrict Function access to users who need query-level visibility.
- Review and redact results before sharing them.
- Configure database-side query normalization or obfuscation where supported.
- Do not assume Netdata can remove sensitive literals that the database returns.

## Use a Database Function

1. Configure the database collector and any required database permissions.
2. Confirm that its regular metrics are being collected.
3. Open the [Live tab](/docs/dashboards-and-charts/live-tab.md) or the node's `f(x)` control.
4. Select an available database Function and the target node.
5. Apply the filters and ranking options documented for that integration.
6. Correlate the result with database charts, logs, and application traces.

## Related Documentation

- [Netdata Functions](/docs/top-monitoring-netdata-functions.md)
- [Live tab](/docs/dashboards-and-charts/live-tab.md)
