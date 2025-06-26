# IBM AS/400 (IBM i) collector

## Requirements

- IBM DB2 client libraries installed (required for AS/400 connectivity)
- Database user with SELECT permissions on QSYS2 schema tables
- Network connectivity to AS/400 system on DB2 port (typically 446)

## Overview

This collector monitors IBM AS/400 (IBM i) system performance metrics via IBM DB2 connection.

The collector features automatic version detection and proactive adaptation:
- **Detects IBM i version** at startup (V6R1 through V7R5+)
- **Proactively disables** incompatible features before attempting queries
- **Adds version labels** to all charts for filtering and grouping
- **Comprehensive logging** of available and disabled features

It collects:

**System-wide metrics:**
- CPU utilization (overall, partition, interactive, database, shared pool)
- CPU configuration (configured CPUs, current processing capacity)
- Active jobs count and job type breakdown (batch, interactive, system, spooled, other)
- System ASP (Auxiliary Storage Pool) usage
- Memory pool usage (Machine, Base, Interactive, Spool)
- Memory pool sizes (current, defined, reserved for Machine and Base pools)
- IFS (Integrated File System) usage and file count
- IFS top directories by size
- System message queue depths (QSYSMSG, QSYSOPR)
- Critical message counts in system queues
- Aggregate disk busy percentage
- Aggregate job queue length

**Per-instance metrics (when enabled):**
- **Per-disk**: Busy percentage, I/O requests/s, throughput, response time
- **Per-subsystem**: Active/held jobs, storage usage
- **Per-job-queue**: Waiting/held/scheduled jobs
- **Per-job** (top CPU consumers): CPU time, temporary storage, active time, CPU percentage

## Collected metrics

### System-wide metrics

| Metric | Description | Unit |
|--------|-------------|------|
| as400.cpu_utilization | Average CPU utilization | percentage |
| as400.cpu_details | CPU configuration (configured CPUs, capacity) | cpus |
| as400.cpu_by_type | CPU utilization by type (partition, interactive, database, shared pool) | percentage |
| as400.active_jobs | Number of active jobs in the system | jobs |
| as400.job_type_breakdown | Jobs by type (batch, interactive, system, spooled, other) | jobs |
| as400.system_asp_usage | System ASP usage percentage | percentage |
| as400.memory_pool_usage | Memory pool current sizes | bytes |
| as400.memory_pool_defined | Memory pool defined sizes | bytes |
| as400.memory_pool_reserved | Memory pool reserved sizes | bytes |
| as400.disk_busy | Average disk busy percentage (aggregate) | percentage |
| as400.job_queue_length | Number of jobs in queue (aggregate) | jobs |
| as400.ifs_usage | IFS usage (used and total) | bytes |
| as400.ifs_files | IFS file count | files |
| as400.ifs_directory_usage | IFS top directories by size | bytes |
| as400.message_queue_depth | System message queue depths | messages |
| as400.message_queue_critical | Critical messages in system queues | messages |

### Per-instance metrics

#### Disk metrics (per disk unit)
| Metric | Description | Unit | Labels |
|--------|-------------|------|--------|
| as400.disk_busy | Individual disk busy percentage | percentage | disk_unit, disk_type, disk_model |
| as400.disk_io_requests | Disk I/O requests (read/write) | requests/s | disk_unit, disk_type, disk_model |
| as400.disk_io_bytes | Disk I/O throughput (read/write) | bytes/s | disk_unit, disk_type, disk_model |
| as400.disk_avg_time | Disk average response time | milliseconds | disk_unit, disk_type, disk_model |

#### Subsystem metrics (per subsystem)
| Metric | Description | Unit | Labels |
|--------|-------------|------|--------|
| as400.subsystem_jobs | Jobs in subsystem (active/held) | jobs | subsystem, library, status |
| as400.subsystem_storage | Subsystem storage usage | megabytes | subsystem, library, status |

#### Job queue metrics (per queue)
| Metric | Description | Unit | Labels |
|--------|-------------|------|--------|
| as400.jobqueue_length | Jobs in queue (waiting/held/scheduled) | jobs | job_queue, library, status |

## Configuration

### DSN Configuration

The `dsn` (Data Source Name) string is used to connect to the AS/400 system. Here are some common examples:

**Standard TCP/IP Connection:**
```yaml
dsn: 'DATABASE=;HOSTNAME=as400.example.com;PORT=446;PROTOCOL=TCPIP;UID=monitor;PWD=secret'
```

**SSL Connection (IBM Cloud):**
```yaml
dsn: 'DATABASE=bludb;HOSTNAME=your-host.databases.appdomain.cloud;PORT=32211;PROTOCOL=TCPIP;UID=your-uid;PWD=your-pwd;SECURITY=SSL;SSLServerCertificate=/path/to/cert.crt'
```

**DSN Parameters:**

*   `DATABASE`: The name of the database to connect to (usually blank for AS/400).
*   `HOSTNAME`: The hostname or IP address of the AS/400 system.
*   `PORT`: The port number of the DB2 server (usually 446).
*   `PROTOCOL`: The connection protocol (usually `TCPIP`).
*   `UID`: The username to connect with.
*   `PWD`: The password for the specified user.
*   `SECURITY`: Set to `SSL` to enable SSL/TLS encryption.
*   `SSLServerCertificate`: The path to the SSL server certificate (if required).

### Example Job Configuration

```yaml
jobs:
  - name: as400_production
    dsn: 'DATABASE=;HOSTNAME=as400.example.com;PORT=446;PROTOCOL=TCPIP;UID=monitor;PWD=secret'
    timeout: 5

    # Monitor top 20 IFS directories by size
    ifs_top_n_directories: 20

    # Cardinality control
    max_disks: 20
    max_subsystems: 10
    collect_job_queue_metrics: false

    # Filtered collection
    collect_disks_matching: '*SSD*'
    collect_subsystems_matching: 'QINTER'
    collect_job_queues_matching: 'QBATCH*'
```


## Prerequisites

### 1. Install IBM DB2 Client Libraries

The collector requires IBM DB2 client libraries. You can install one of:
- IBM Data Server Driver Package (recommended, smallest footprint)
- IBM Data Server Runtime Client
- IBM Data Server Client
- IBM Db2 Community Edition

#### Linux Installation Example:
```bash
# Download IBM Data Server Driver Package from IBM
# (requires free IBM account registration)
# https://www.ibm.com/support/pages/db2-data-server-drivers

# Extract the package
tar -xzf linuxx64_odbc_cli.tar.gz

# The files will be extracted to a 'clidriver' directory
```

### 2. Set Environment Variables

The following environment variables must be set for the Netdata service:

```bash
export IBM_DB_HOME=/path/to/clidriver
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib -Wl,-rpath,$IBM_DB_HOME/lib"
export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:$LD_LIBRARY_PATH
```

For systemd services, add these to the Netdata service file:
```ini
[Service]
Environment="IBM_DB_HOME=/path/to/clidriver"
Environment="LD_LIBRARY_PATH=/path/to/clidriver/lib"
```

### 3. Verify Connectivity

Test your connection before configuring Netdata:
```bash
# Using the db2cli tool from IBM Data Server Driver
$IBM_DB_HOME/bin/db2cli validate -dsn "DATABASE=bludb;HOSTNAME=your-host;PORT=your-port;PROTOCOL=TCPIP;UID=your-user;PWD=your-password"
```

### 4. Database Permissions

Create a monitoring user with SELECT permissions on QSYS2 schema tables:
```sql
-- For IBM i / AS/400
-- System-wide metrics
GRANT SELECT ON QSYS2.SYSTEM_STATUS_INFO TO monitor_user;
GRANT SELECT ON QSYS2.ASP_INFO TO monitor_user;
GRANT SELECT ON QSYS2.JOB_INFO TO monitor_user;
GRANT SELECT ON QSYS2.MEMORY_POOL_INFO TO monitor_user;
GRANT SELECT ON QSYS2.DISK_STATUS TO monitor_user;

-- Per-instance metrics (if enabled)
GRANT SELECT ON QSYS2.SUBSYSTEM_INFO TO monitor_user;
GRANT SELECT ON QSYS2.JOB_QUEUE_INFO TO monitor_user;
```

## Troubleshooting

### Common Issues

1. **"error loading shared libraries: libdb2.so"**
   - Ensure `LD_LIBRARY_PATH` includes the DB2 client library path
   - For systemd services, update the service file with the environment variable

2. **"cgo: C compiler not found"**
   - Install gcc or build-essential package
   - The go_ibm_db driver requires CGO

3. **"SQL1042C An unexpected system error occurred"**
   - Check if the DB2 client version is compatible with your server
   - Verify the SSL certificate path is correct and readable

4. **Certificate errors with SSL connections**
   - Ensure the certificate file has proper permissions (readable by netdata user)
   - Use absolute paths for SSLServerCertificate
   - Verify the certificate is in PEM format

### Debug Mode

To troubleshoot connection and data collection issues:

1. Test the DB2 connection using the db2cli tool:
   ```bash
   $IBM_DB_HOME/bin/db2cli validate -dsn "DATABASE=;HOSTNAME=your-as400;PORT=446;PROTOCOL=TCPIP;UID=your-user;PWD=your-password"
   ```

2. Check if the monitoring user has proper permissions

3. Verify network connectivity to the AS/400 system on port 446

### Verify DB2 Client Installation

Check if the IBM DB2 client libraries are properly installed:

```bash
# List DB2 client libraries
ls -la $IBM_DB_HOME/lib/

# Test connectivity with db2cli
$IBM_DB_HOME/bin/db2cli validate -database "" -host your-as400-host -port 446 -user your-user -passwd your-password
```

## IBM i Version Compatibility

The AS400 collector is designed to work with different IBM i versions and gracefully handle missing features:

### Version Detection and Proactive Adaptation
The collector automatically detects the IBM i version and proactively adapts:
- **Parses version** into major/release/modification components (e.g., V7R3M5)
- **Applies feature gates** based on known version compatibility
- **Logs feature availability** at startup for transparency
- **Core metrics** (CPU, ASP, Jobs) work on all IBM i versions (V6R1+)
- **Advanced metrics** are enabled only on compatible versions

### Resilient Collection
The collector uses individual queries for each metric to ensure maximum compatibility:
- If a metric is not available, it is automatically disabled after the first attempt
- Other metrics continue to be collected normally
- Warning messages are logged once (not repeatedly) for unavailable features

### Version-Specific Features

| Feature | Required Version | Proactive Behavior |
|---------|------------------|--------------------|
| Basic CPU/ASP metrics | All versions | Always enabled |
| SQL services support | V7R1+ | Disabled on V6R1 |
| MESSAGE_QUEUE_INFO | V7R2+ | Disabled on older versions |
| JOB_QUEUE_ENTRIES | V7R2+ | Disabled on older versions |
| ACTIVE_JOB_INFO function | V7R3+ | Disabled before V7R3 |
| IFS_OBJECT_STATISTICS | V7R3+ | Disabled before V7R3 |
| Enhanced performance metrics | V7R4+ | Latest features on V7R4-V7R5 |

### Feature Availability Logging

At startup, the collector logs:
- **Detected version**: "detected IBM i version: V7 R3"
- **Feature gates applied**: "Feature disabled: active_job_info - ACTIVE_JOB_INFO requires IBM i 7.3 or later"
- **Available features**: "Feature availability: IBM i 7.3 detected - ACTIVE_JOB_INFO and IFS_OBJECT_STATISTICS available"

### Known Compatibility Issues

1. **Table Functions**: Some metrics use table functions (UDTFs) that may not be available on older IBM i versions:
   - `ACTIVE_JOB_INFO()` - Used for job type breakdown and active job details
   - `IFS_OBJECT_STATISTICS()` - Used for IFS file system metrics

2. **Column Availability**: Some columns in QSYS2 tables were added in newer versions:
   - `CONFIGURED_CPUS`, `CURRENT_PROCESSING_CAPACITY` (V7R3+)
   - `SHARED_PROCESSOR_POOL_UTILIZATION` (V7R3+)

The collector handles these proactively by:
1. **Version detection** during initialization
2. **Proactive feature gating** before any queries are attempted
3. **Graceful error handling** for unexpected missing features
4. **Single-use logging** to prevent log spam
5. **Version labels** on all charts for easy filtering

This ensures reliable collection across all IBM i environments without manual configuration.