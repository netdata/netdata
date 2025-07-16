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

**Database Connectivity:**

This collector uses **unixODBC exclusively** for database connectivity, which provides several advantages:

✅ **No IBM DB2 client library licensing required**  
✅ **Works with both DB2 and AS/400 systems**  
✅ **Simplified deployment and configuration**  
✅ **Consistent behavior across all IBM database types**

**Prerequisites:**
- unixODBC driver manager installed
- IBM DB2 ODBC driver installed and configured  
- Database user with SELECT permissions on monitoring views
- Network connectivity to DB2 database

**ODBC Driver Sources:**
- IBM Data Server Driver Package (includes ODBC drivers)
- IBM i Access Client Solutions (for AS/400 connectivity)
- Distribution packages (e.g., `unixodbc`, `ibm-db2-odbc`)

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

### ODBC Configuration

The collector uses unixODBC for all database connections. You can configure connections using either component-based parameters (recommended) or ODBC DSN strings.

#### Option 1: Component-based Configuration (Recommended)

```yaml
# Standard DB2 LUW connection
hostname: localhost
port: 50000
database: sample
username: db2inst1
password: password
odbc_driver: "IBM DB2 ODBC DRIVER"  # Optional: auto-detected if not specified
```

```yaml
# DB2 on Cloud with SSL
hostname: xxx.databases.appdomain.cloud
port: 32733
database: bludb
username: your_username
password: your_password
use_ssl: true
ssl_server_cert_path: "/path/to/DigiCertGlobalRootCA.crt"
odbc_driver: "IBM DB2 ODBC DRIVER"
```

#### Option 2: ODBC DSN String

```yaml
# ODBC connection string format
dsn: 'Driver={IBM DB2 ODBC DRIVER};Database=sample;Hostname=localhost;Port=50000;Protocol=TCPIP;Uid=db2inst1;Pwd=password'
```

```yaml
# DB2 on Cloud with SSL
dsn: 'Driver={IBM DB2 ODBC DRIVER};Database=bludb;Hostname=xxx.databases.appdomain.cloud;Port=32733;Protocol=TCPIP;Uid=username;Pwd=password;Security=SSL;SSLServerCertificate=/path/to/cert.crt'
```

### Configuration Parameters

**Connection Parameters:**
*   `hostname`: The hostname or IP address of the DB2 server
*   `port`: The port number (50000 for DB2 LUW, 446 for AS/400, 5023 for z/OS)
*   `database`: The name of the database to connect to
*   `username`: The username to connect with
*   `password`: The password for the specified user
*   `use_ssl`: Enable SSL/TLS encryption (boolean)
*   `ssl_server_cert_path`: Path to SSL server certificate file
*   `odbc_driver`: ODBC driver name (auto-detected: "IBM DB2 ODBC DRIVER" or "IBM i Access ODBC Driver")

**DSN String Parameters (when using `dsn`):**
*   `Driver`: The ODBC driver name (e.g., `{IBM DB2 ODBC DRIVER}`)
*   `Database`: The name of the database to connect to
*   `Hostname`: The hostname or IP address of the DB2 server
*   `Port`: The port number of the DB2 server
*   `Protocol`: The connection protocol (usually `TCPIP`)
*   `Uid`: The username to connect with
*   `Pwd`: The password for the specified user
*   `Security`: Set to `SSL` to enable SSL/TLS encryption
*   `SSLServerCertificate`: The path to the SSL server certificate

### Example Job Configuration

```yaml
jobs:
  - name: db2_production
    # Component-based configuration (recommended)
    hostname: db2.example.com
    port: 50000
    database: sample
    username: db2inst1
    password: password
    
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

  - name: db2_cloud
    # Alternative: ODBC DSN string format
    dsn: 'Driver={IBM DB2 ODBC DRIVER};Database=bludb;Hostname=xxx.databases.appdomain.cloud;Port=32733;Protocol=TCPIP;Uid=username;Pwd=password;Security=SSL'
    ssl_server_cert_path: "/path/to/DigiCertGlobalRootCA.crt"
    timeout: 10
```

## Troubleshooting

### ODBC Setup

**Driver Installation:**
- Ensure unixODBC is installed: `sudo apt-get install unixodbc` or `sudo yum install unixODBC`
- Install IBM ODBC drivers from IBM Data Server Driver Package or IBM i Access Client Solutions
- Verify driver registration: `odbcinst -q -d`
- Test connection: `isql -v "DSN_STRING"`

### Edition-Specific Behavior

The collector automatically adapts to different DB2 editions via ODBC:

- **DB2 LUW**: Full feature set available through ODBC
- **DB2 for z/OS**: Limited buffer pool metrics, mainframe-specific features
- **DB2 for i (AS/400)**: SYSIBMADM views may not be available, basic monitoring
- **Db2 on Cloud**: System-level metrics restricted

### Common Issues

- **Connection refused**: Verify ODBC driver installation and DSN configuration
- **SQL0204N errors**: Expected on older versions or limited editions. The collector logs which features are disabled.
- **Missing metrics**: Check logs for feature availability messages. Some metrics are edition/version specific.
- **Version detection**: The collector logs detected edition and version on startup.
- **ODBC driver not found**: Check `/etc/odbcinst.ini` for proper driver registration.

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