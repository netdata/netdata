# IBM DB2 collector

## Overview

This collector monitors IBM DB2 database performance metrics across all editions.

It supports all DB2 editions with automatic adaptation:
- DB2 LUW (Linux, Unix, Windows) - Full feature set
- DB2 for z/OS - Adapted for mainframe limitations 
- DB2 for i (AS/400) - Limited SYSIBMADM view support
- Db2 on Cloud - Cloud-specific restrictions

The collector automatically detects the DB2 edition and version, then enables/disables features based on availability. Version information is added as labels to all charts for filtering and grouping.

### Version Labels

All charts include these labels:
- `db2_edition`: The DB2 edition (LUW, z/OS, i, Cloud)
- `db2_version`: The full version string (e.g., "DB2 v11.5.7.0")

This allows you to:
- Filter dashboards by DB2 version: `db2_edition="LUW"`
- Group metrics across multiple DB2 instances of the same version
- Track version-specific performance differences
- Monitor mixed-version environments during migrations

## Requirements

- IBM DB2 client libraries installed
- Database user with SELECT permissions on monitoring views
- Network connectivity to DB2 database

## Metrics

The collector provides:

**Global metrics:**
- Database connections (total, active, executing, idle)
- Lock waits, timeouts, deadlocks
- Sort operations and overflows
- Row read/write activity
- Buffer pool hit ratios
- Log space usage

**Per-instance metrics** (with configurable limits):
- **Databases**: status, application count
- **Buffer pools**: hit ratio, I/O operations, page usage
- **Tablespaces**: usage percentage, size breakdown
- **Connections**: state, row activity, CPU usage

## Configuration

### DSN Configuration

The `dsn` (Data Source Name) string is used to connect to the DB2 database. Here are some common examples:

**Standard TCP/IP Connection:**
```yaml
dsn: 'DATABASE=sample;HOSTNAME=localhost;PORT=50000;PROTOCOL=TCPIP;UID=db2inst1;PWD=password'
```

**Db2 on Cloud with SSL:**
```yaml
dsn: 'DATABASE=bludb;HOSTNAME=xxx.databases.appdomain.cloud;PORT=32733;PROTOCOL=TCPIP;UID=user;PWD=pass;SECURITY=SSL;SSLServerCertificate=/path/to/cert.crt'
```

**DSN Parameters:**

*   `DATABASE`: The name of the database to connect to.
*   `HOSTNAME`: The hostname or IP address of the DB2 server.
*   `PORT`: The port number of the DB2 server.
*   `PROTOCOL`: The connection protocol (usually `TCPIP`).
*   `UID`: The username to connect with.
*   `PWD`: The password for the specified user.
*   `SECURITY`: Set to `SSL` to enable SSL/TLS encryption.
*   `SSLServerCertificate`: The path to the SSL server certificate (if required).

### Example Job Configuration

```yaml
jobs:
  - name: db2_local
    dsn: 'DATABASE=sample;HOSTNAME=localhost;PORT=50000;PROTOCOL=TCPIP;UID=db2inst1;PWD=password'
    
    # Cardinality limits (to prevent memory issues)
    max_databases: 10
    max_bufferpools: 20  
    max_tablespaces: 100
    max_connections: 200
    max_tables: 50
    max_indexes: 100

    # Enable table and index metrics
    collect_table_metrics: true
    collect_index_metrics: true

    # Selectors for filtering
    collect_tables_matching: 'USER.*'
    collect_indexes_matching: 'USER.*'
```

## Troubleshooting

### Edition-Specific Behavior

The collector automatically adapts to different DB2 editions:

- **DB2 LUW**: Full feature set available
- **DB2 for z/OS**: Limited buffer pool metrics, mainframe-specific features
- **DB2 for i (AS/400)**: SYSIBMADM views not available, basic monitoring only
- **Db2 on Cloud**: System-level metrics restricted

### Common Issues

- **SQL0204N errors**: Expected on older versions or limited editions. The collector logs which features are disabled.
- **Missing metrics**: Check logs for feature availability messages. Some metrics are edition/version specific.
- **Version detection**: The collector logs detected edition and version on startup.

### Logs to Monitor

Look for these informational messages:
- "detected DB2 edition: X version: Y"
- "Feature disabled: X - reason"
- "Feature availability: X"

## Permissions

Grant SELECT on monitoring views:
```sql
GRANT SELECT ON SYSIBMADM.APPLICATIONS TO monitor_user;
GRANT SELECT ON SYSIBMADM.SNAPDB TO monitor_user;
GRANT SELECT ON SYSIBMADM.SNAPBP TO monitor_user;
GRANT SELECT ON SYSIBMADM.TBSP_UTILIZATION TO monitor_user;
GRANT SELECT ON SYSIBMADM.LOG_UTILIZATION TO monitor_user;
GRANT SELECT ON SYSIBMADM.ENV_INST_INFO TO monitor_user;
```