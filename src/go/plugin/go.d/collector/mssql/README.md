# Microsoft SQL Server collector

This collector monitors Microsoft SQL Server instances.

## Requirements

- SQL Server 2016 or later
- A user with `VIEW SERVER STATE` permission

## Metrics

The collector provides the following metric categories:

### Instance metrics

- **User connections** - Number of active user connections
- **Blocked processes** - Number of blocked processes
- **Batch requests** - Number of batch requests per second
- **SQL compilations/recompilations** - Query compilation statistics

### Buffer and memory metrics

- **Buffer cache hit ratio** - Percentage of pages found in buffer pool
- **Page life expectancy** - How long pages stay in buffer pool
- **Page reads/writes** - Buffer I/O operations
- **Checkpoint pages** - Pages flushed during checkpoint
- **Total server memory** - Memory used by SQL Server
- **Connection memory** - Memory used for connections
- **Memory grants pending** - Queries waiting for memory

### Per-database metrics

- **Transactions** - Transaction throughput
- **Active transactions** - Currently active transactions
- **Log bytes flushed** - Transaction log write throughput
- **Data file size** - Size of database data files
- **Lock statistics** - Requests, waits, timeouts, deadlocks by lock type

### Wait statistics

- **Wait time** - Resource and signal wait times by wait type
- **Waiting tasks** - Number of tasks waiting

### Lock metrics

- **Lock count** - Current locks by resource type

### SQL Agent jobs (optional)

- **Job status** - Enabled/disabled status for each job

### Replication metrics (optional)

- **Replication status** - Publication status and warning flags
- **Replication latency** - Average, best, and worst latency in seconds
- **Subscription count** - Number of subscriptions and running agents

## Configuration

Edit the `go.d/mssql.conf` configuration file using `edit-config`:

```bash
cd /etc/netdata
sudo ./edit-config go.d/mssql.conf
```

### Basic configuration

```yaml
jobs:
  - name: local
    dsn: "sqlserver://netdata_user:password@localhost:1433"
```

### Connection string format

The DSN follows the [go-mssqldb connection string format](https://github.com/microsoft/go-mssqldb#connection-parameters-and-dsn):

```
sqlserver://username:password@host:port?param=value
```

Common parameters:
- `database` - Initial database to connect to
- `encrypt` - Enable encryption (disable/false/true/strict)
- `TrustServerCertificate` - Skip certificate validation
- `ApplicationIntent` - ReadOnly for read-only routing

### Windows Authentication

```yaml
jobs:
  - name: local
    dsn: "sqlserver://localhost:1433?trusted_connection=yes"
```

### Named instance

```yaml
jobs:
  - name: named
    dsn: "sqlserver://netdata_user:password@localhost/INSTANCENAME"
```

### Configuration options

| Option | Description | Default |
|--------|-------------|---------|
| `dsn` | SQL Server connection string | required |
| `timeout` | Query timeout in seconds | 5 |
| `vnode` | Virtual node name for multi-instance setups | "" |
| `collect_transactions` | Collect per-database transaction metrics | true |
| `collect_waits` | Collect wait statistics | true |
| `collect_locks` | Collect lock metrics | true |
| `collect_jobs` | Collect SQL Agent job status | true |
| `collect_buffer_stats` | Collect buffer manager statistics | true |
| `collect_database_size` | Collect data file sizes | true |
| `collect_user_connections` | Collect user connection counts | true |
| `collect_blocked_processes` | Collect blocked process count | true |
| `collect_sql_errors` | Collect SQL error statistics | true |
| `collect_database_status` | Collect database state and read-only status | true |
| `collect_replication` | Collect replication monitoring metrics | true |

## Permissions

Create a monitoring user with the required permissions:

```sql
-- Create login
CREATE LOGIN netdata_user WITH PASSWORD = 'YourStrongPassword!';

-- Grant VIEW SERVER STATE (required for DMVs)
GRANT VIEW SERVER STATE TO netdata_user;

-- Optional: Grant access to msdb for SQL Agent job monitoring
USE msdb;
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT SELECT ON dbo.sysjobs TO netdata_user;
```

## Troubleshooting

### Connection refused

- Verify SQL Server is running
- Check that TCP/IP is enabled in SQL Server Configuration Manager
- Ensure the firewall allows connections on port 1433

### Login failed

- Verify credentials are correct
- Ensure SQL Server is configured for mixed mode authentication
- Check that the user has permission to connect

### Permission denied on DMVs

Grant VIEW SERVER STATE permission:

```sql
GRANT VIEW SERVER STATE TO netdata_user;
```

### SQL Agent job metrics not collected

Grant access to the msdb database:

```sql
USE msdb;
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT SELECT ON dbo.sysjobs TO netdata_user;
```

### Replication metrics not collected

For replication monitoring, grant access to the distribution database:

```sql
USE distribution;
CREATE USER netdata_user FOR LOGIN netdata_user;
GRANT SELECT ON dbo.MSreplication_monitordata TO netdata_user;
GRANT SELECT ON dbo.MSpublications TO netdata_user;
GRANT SELECT ON dbo.MSsubscriptions TO netdata_user;
```

Note: The distribution database only exists if replication is configured on the server.
