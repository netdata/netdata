# IBM DB2 collector

## Overview

This collector monitors IBM DB2 database performance metrics.

It supports all DB2 editions:
- DB2 LUW (Linux, Unix, Windows) 
- DB2 for z/OS
- DB2 for i (AS/400)
- Db2 on Cloud

## Requirements

This collector is part of the `ibm.d.plugin` which requires:
- CGO enabled build
- IBM DB2 client libraries

See the [AS/400 collector documentation](../as400/README.md) for build instructions.

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

Edit the `ibm.d/db2.conf` configuration file.

```yaml
jobs:
  - name: db2_local
    dsn: 'DATABASE=sample;HOSTNAME=localhost;PORT=50000;PROTOCOL=TCPIP;UID=db2inst1;PWD=password'
    
    # Cardinality limits (to prevent memory issues)
    max_databases: 10
    max_bufferpools: 20  
    max_tablespaces: 100
    max_connections: 200
```

For Db2 on Cloud with SSL:
```yaml
jobs:
  - name: db2_cloud
    dsn: 'DATABASE=bludb;HOSTNAME=xxx.databases.appdomain.cloud;PORT=32733;PROTOCOL=TCPIP;UID=user;PWD=pass;SECURITY=SSL;SSLServerCertificate=/path/to/cert.crt'
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