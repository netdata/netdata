# IBM DB2 collector

## Overview

This collector monitors IBM DB2 database performance metrics.

It supports all DB2 editions:
- DB2 LUW (Linux, Unix, Windows) 
- DB2 for z/OS
- DB2 for i (AS/400)
- Db2 on Cloud

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