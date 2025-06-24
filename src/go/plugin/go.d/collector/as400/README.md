# IBM AS/400 (IBM i) collector

## Overview

This collector monitors IBM AS/400 (IBM i) system performance metrics via IBM DB2 connection.

It collects:

**System-wide metrics:**
- CPU utilization
- Active jobs count
- System ASP (Auxiliary Storage Pool) usage
- Memory pool usage (Machine, Base, Interactive, Spool)
- Aggregate disk busy percentage
- Aggregate job queue length

**Per-instance metrics (when enabled):**
- **Per-disk**: Busy percentage, I/O requests/s, throughput, response time
- **Per-subsystem**: Active/held jobs, storage usage
- **Per-job-queue**: Waiting/held/scheduled jobs

## Collected metrics

### System-wide metrics

| Metric | Description | Unit |
|--------|-------------|------|
| as400.cpu_utilization | Average CPU utilization | percentage |
| as400.active_jobs | Number of active jobs in the system | jobs |
| as400.system_asp_usage | System ASP usage percentage | percentage |
| as400.memory_pool_usage | Memory pool sizes | megabytes |
| as400.disk_busy | Average disk busy percentage (aggregate) | percentage |
| as400.job_queue_length | Number of jobs in queue (aggregate) | jobs |

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

Edit the `go.d/as400.conf` configuration file using `edit-config` from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md#the-netdata-config-directory).

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config go.d/as400.conf
```

Example configurations:

### Basic connection
```yaml
jobs:
  - name: as400_production
    dsn: 'DATABASE=;HOSTNAME=as400.example.com;PORT=446;PROTOCOL=TCPIP;UID=monitor;PWD=secret'
    timeout: 5
```

### SSL connection (IBM Cloud)
```yaml
jobs:
  - name: as400_cloud
    dsn: 'DATABASE=bludb;HOSTNAME=your-host.databases.appdomain.cloud;PORT=32211;PROTOCOL=TCPIP;UID=your-uid;PWD=your-pwd;SECURITY=SSL;SSLServerCertificate=/path/to/cert.crt'
    timeout: 5
```

### Cardinality control
```yaml
jobs:
  - name: as400_large_system
    dsn: 'DATABASE=;HOSTNAME=large.as400.com;PORT=446;PROTOCOL=TCPIP;UID=monitor;PWD=secret'
    # Limit per-instance metrics to prevent high cardinality
    max_disks: 20              # Monitor only first 20 disks
    max_subsystems: 10         # Monitor only first 10 subsystems
    collect_job_queue_metrics: false  # Disable job queue metrics
```

### Filtered collection
```yaml
jobs:
  - name: as400_selective
    dsn: 'DATABASE=;HOSTNAME=as400.example.com;PORT=446;PROTOCOL=TCPIP;UID=monitor;PWD=secret'
    # Use SQL LIKE patterns to filter instances
    collect_disks_matching: '%SSD%'        # Only SSD disks
    collect_subsystems_matching: 'QINTER'  # Only interactive subsystem
    collect_job_queues_matching: 'QBATCH%' # Only batch queues
```

For all available options, see the [module configuration file](https://github.com/netdata/netdata/blob/master/src/go/plugin/go.d/collector/as400/config_schema.json).

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

To troubleshoot issues with the `as400` collector, run the `go.d.plugin` with the debug option enabled.

First, navigate to your plugins directory, usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case on
your system, open `netdata.conf` and look for the setting `plugins directory`.

```bash
cd /usr/libexec/netdata/plugins.d/
```

Switch to the `netdata` user with the proper environment:

```bash
sudo -u netdata -s
export IBM_DB_HOME=/path/to/clidriver
export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:$LD_LIBRARY_PATH
```

Run the `go.d.plugin` to debug the collector:

```bash
./go.d.plugin -d -m as400
```

### Verify Library Installation

Check if the DB2 libraries are properly installed:

```bash
# Check if libraries are found
ldd ./go.d.plugin | grep db2

# List DB2 client libraries
ls -la $IBM_DB_HOME/lib/

# Test with db2cli
$IBM_DB_HOME/bin/db2cli validate -database bludb -host your-host -port your-port -user your-user -passwd your-password
```