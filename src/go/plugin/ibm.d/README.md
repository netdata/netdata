# IBM.d.plugin

The `ibm.d.plugin` is a specialized Netdata plugin for monitoring IBM systems including DB2 databases, AS/400 (IBM i) systems, and WebSphere Application Server.

## Overview

This plugin provides monitoring for:
- **IBM DB2 databases**: All editions (LUW, z/OS, IBM i, Db2 on Cloud)
- **IBM AS/400 (IBM i) systems**: System performance, storage, subsystems, job queues
- **IBM WebSphere Application Server**: Traditional WAS (PMI), Liberty (MicroProfile), JMX monitoring

Unlike other Netdata collectors, this plugin requires CGO compilation and IBM DB2 client libraries because it uses the native IBM database drivers.

## Installation

### Package Installation (Recommended)

The IBM plugin is available as a separate package:

**DEB-based systems (Ubuntu, Debian):**
```bash
sudo apt-get install netdata-plugin-ibm
```

**RPM-based systems (RHEL, CentOS, Fedora):**
```bash
sudo yum install netdata-plugin-ibm
# or
sudo dnf install netdata-plugin-ibm
```

The package installation will automatically download and install the required IBM DB2 client libraries.

### Static Installation

For static Netdata installations:

```bash
# Download and install IBM libraries
sudo /opt/netdata/usr/libexec/netdata/install-ibm-libs.sh

# The plugin should already be installed with Netdata
```

### Building from Source

To build Netdata with IBM plugin support:

```bash
# Configure with IBM plugin enabled
cmake -DENABLE_PLUGIN_IBM=On ..

# Build and install
make
sudo make install
```

The build process will automatically download the required IBM headers.

### Configuration

Configure monitoring by editing the configuration files:

```bash
# Main plugin configuration
sudo vim /etc/netdata/ibm.d.conf

# DB2 collector configuration
sudo vim /etc/netdata/ibm.d/db2.conf

# AS/400 collector configuration
sudo vim /etc/netdata/ibm.d/as400.conf

# WebSphere PMI collector configuration
sudo vim /etc/netdata/ibm.d/websphere_pmi.conf

# WebSphere MicroProfile collector configuration  
sudo vim /etc/netdata/ibm.d/websphere_mp.conf
```

## Manual Build Process

If the automatic build script doesn't work for your environment, you can build manually:

### 1. Install IBM DB2 Client Libraries

Download the IBM Data Server Driver Package from [IBM Support](https://www.ibm.com/support/pages/db2-data-server-drivers).

For Linux x64:
```bash
# Download the package (example URL, check IBM site for latest)
wget https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/linuxx64_odbc_cli.tar.gz

# Extract
tar -xzf linuxx64_odbc_cli.tar.gz

# Set IBM_DB_HOME
export IBM_DB_HOME=/path/to/clidriver
```

### 2. Install Build Dependencies

Ubuntu/Debian:
```bash
sudo apt-get update
sudo apt-get install gcc build-essential golang-go
```

CentOS/RHEL:
```bash
sudo yum install gcc gcc-c++ golang
```

### 3. Build the Plugin

```bash
# Set environment variables
export IBM_DB_HOME=/path/to/clidriver
export CGO_ENABLED=1
export CGO_CFLAGS="-I$IBM_DB_HOME/include"
export CGO_LDFLAGS="-L$IBM_DB_HOME/lib"
export LD_LIBRARY_PATH="$IBM_DB_HOME/lib:$LD_LIBRARY_PATH"

# Build
cd /usr/src/netdata-ktsaou.git/src/go
go build -o ibm.d.plugin ./cmd/ibmdplugin
```

## Runtime Configuration

### Library Paths

The plugin is built with RPATH to automatically find IBM libraries in these locations:
- `/usr/lib/netdata/ibm-clidriver/lib` (system packages)
- `/opt/netdata/lib/netdata/ibm-clidriver/lib` (static installations)
- Relative to the plugin location (portable installations)

### Manual Library Installation

If automatic installation fails, you can manually install the libraries:

```bash
# Determine the correct directory based on your installation
INSTALL_DIR="/usr/lib/netdata/ibm-clidriver"  # For system packages
# or
INSTALL_DIR="/opt/netdata/lib/netdata/ibm-clidriver"  # For static installations

# Download and extract
sudo mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"
sudo wget https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/v11.5.9/linuxx64_odbc_cli.tar.gz
sudo tar -xzf linuxx64_odbc_cli.tar.gz
sudo mv clidriver/* .
sudo rmdir clidriver
sudo rm linuxx64_odbc_cli.tar.gz
```

### Environment Variables (Optional)

If you need to override the built-in library paths:

```bash
export LD_LIBRARY_PATH=/custom/path/to/ibm-clidriver/lib:$LD_LIBRARY_PATH
```

### Docker Configuration

For containerized Netdata:

```dockerfile
# Install IBM DB2 client libraries
RUN mkdir -p /opt/ibm && \
    cd /opt/ibm && \
    wget https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli/linuxx64_odbc_cli.tar.gz && \
    tar -xzf linuxx64_odbc_cli.tar.gz && \
    rm linuxx64_odbc_cli.tar.gz

# Set environment
ENV IBM_DB_HOME=/opt/ibm/clidriver
ENV LD_LIBRARY_PATH=$IBM_DB_HOME/lib:$LD_LIBRARY_PATH
```

## Collector Configuration

### DB2 Collector

Example `/etc/netdata/ibm.d/db2.conf`:

```yaml
jobs:
  - name: db2_production
    dsn: 'DATABASE=sample;HOSTNAME=db2.example.com;PORT=50000;PROTOCOL=TCPIP;UID=monitor;PWD=password'
    update_every: 5
    
    # Cardinality control
    max_databases: 10
    max_tablespaces: 50
    max_connections: 100
    
    # Filtering (using wildcards)
    collect_databases_matching: 'PROD*'
    collect_tablespaces_matching: 'USER*|SYS*'
    
    # Virtual node assignment
    vnode: 'db2-prod-cluster'
```

### AS/400 Collector

Example `/etc/netdata/ibm.d/as400.conf`:

```yaml
jobs:
  - name: as400_main
    dsn: 'DATABASE=;HOSTNAME=as400.example.com;PORT=446;PROTOCOL=TCPIP;UID=quser;PWD=password'
    update_every: 5
    
    # Cardinality control
    max_disks: 20
    max_subsystems: 10
    max_job_queues: 20
    
    # Filtering (using wildcards)
    collect_disks_matching: '*SSD*'
    collect_subsystems_matching: 'Q*'
    collect_job_queues_matching: 'QBATCH*|QINTER*'
    
    # Virtual node assignment
    vnode: 'as400-production'
```

### WebSphere Collectors

#### PMI Collector (Traditional WAS)

Example `/etc/netdata/ibm.d/websphere_pmi.conf`:

```yaml
jobs:
  - name: was_traditional
    url: http://websphere.example.com:9080/wasPerfTool/servlet/perfservlet
    username: wasadmin
    password: adminpwd
    
    # PMI statistics level
    pmi_stats_type: extended
    
    # Metrics selection
    collect_jvm_metrics: true
    collect_threadpool_metrics: true
    collect_jdbc_metrics: true
    
    # Cardinality control
    max_applications: 30
    max_threadpools: 20
    
    # Virtual node assignment
    vnode: 'websphere-cluster1'
```

#### MicroProfile Collector (Liberty)

Example `/etc/netdata/ibm.d/websphere_mp.conf`:

```yaml
jobs:
  - name: liberty_mp
    url: https://liberty.example.com:9443/metrics
    username: admin
    password: adminpwd
    
    # Metrics selection
    collect_jvm_metrics: true
    collect_rest_metrics: true
    
    # Cardinality control
    max_rest_endpoints: 50
    
    # Virtual node assignment
    vnode: 'liberty-cluster1'
```

## Metrics Collected

### DB2 Metrics

**Global metrics:**
- Database connections (total, active, executing, idle)
- Lock waits, timeouts, deadlocks
- Sort operations and overflows
- Row activity (reads, inserts, updates, deletes)
- Buffer pool hit ratios (aggregate)
- Log space usage

**Per-database metrics:**
- Database status
- Application connections count
- Lock waits and deadlocks
- Row activity
- Log space used

**Per-tablespace metrics:**
- Usage percentage
- Total/used/free size
- Page size
- State (normal, backup pending, etc.)

### AS/400 Metrics

**System-wide metrics:**
- CPU utilization
- Active jobs count
- System ASP (Auxiliary Storage Pool) usage
- Memory pool usage (Machine, Base, Interactive, Spool)
- Aggregate disk busy percentage
- Aggregate job queue length

**Per-disk metrics:**
- Disk busy percentage
- I/O requests per second (read/write)
- I/O throughput (read/write bytes/s)
- Average response time

**Per-subsystem metrics:**
- Active jobs count
- Jobs on job queues
- Jobs held on job queues
- Storage used (MB)

### WebSphere Metrics

**JVM metrics:**
- Heap usage (used, committed, max)
- Garbage collection (count, time)
- Thread count (total, daemon, peak)
- Loaded/unloaded classes

**Thread pool metrics:**
- Pool size and max size
- Active threads
- Hung threads

**Connection pool metrics:**
- Pool size and free connections
- Wait time
- Timeouts

**Application metrics:**
- Request count
- Response time
- Error count

## Pattern Matching

The collectors support flexible pattern matching for filtering instances:

- `*` - matches any sequence of characters
- `?` - matches any single character
- `|` - separates multiple patterns (OR logic)

Examples:
- `PROD*` - matches anything starting with PROD
- `*TEST*` - matches anything containing TEST
- `DB?` - matches DB1, DB2, etc.
- `PROD*|TEST*` - matches anything starting with PROD or TEST

## Troubleshooting

### Build Issues

**"error loading shared libraries: libdb2.so"**
- Ensure LD_LIBRARY_PATH includes DB2 client library path
- For systemd services, add to the service file:
  ```
  Environment="LD_LIBRARY_PATH=/path/to/db2/lib"
  ```

**"cgo: C compiler not found"**
- Install gcc or build-essential package
- The ibm.d.plugin requires CGO

**"SQL1042C An unexpected system error occurred"**
- Check DB2 client version compatibility
- Verify SSL certificate path and permissions

### Runtime Issues

**Plugin not starting**
```bash
# Test manually
cd /usr/libexec/netdata/plugins.d/
sudo -u netdata -s
export IBM_DB_HOME=/path/to/clidriver
export LD_LIBRARY_PATH=$IBM_DB_HOME/lib:$LD_LIBRARY_PATH
./ibm.d.plugin -d -m db2
```

**Connection failures**
- Verify network connectivity to IBM systems
- Check firewall rules
- Validate credentials
- For SSL connections, ensure certificates are accessible

**High memory usage**
- Reduce cardinality limits (max_databases, max_tablespaces, etc.)
- Use filtering patterns to monitor only critical instances
- Increase update_every interval

### Debug Mode

Run collectors in debug mode for troubleshooting:

```bash
# Debug all collectors
./ibm.d.plugin -d

# Debug specific collector
./ibm.d.plugin -d -m db2
./ibm.d.plugin -d -m as400
./ibm.d.plugin -d -m websphere_pmi
./ibm.d.plugin -d -m websphere_mp
```

## Security Considerations

### Database Permissions

**DB2 minimum permissions:**
```sql
GRANT SELECT ON SYSIBMADM.APPLICATIONS TO monitor_user;
GRANT SELECT ON SYSIBMADM.SNAPDB TO monitor_user;
GRANT SELECT ON SYSIBMADM.SNAPBP TO monitor_user;
GRANT SELECT ON SYSIBMADM.TBSP_UTILIZATION TO monitor_user;
GRANT SELECT ON SYSIBMADM.LOG_UTILIZATION TO monitor_user;
```

**AS/400 minimum permissions:**
```sql
GRANT SELECT ON QSYS2.SYSTEM_STATUS_INFO TO monitor_user;
GRANT SELECT ON QSYS2.ASP_INFO TO monitor_user;
GRANT SELECT ON QSYS2.MEMORY_POOL_INFO TO monitor_user;
GRANT SELECT ON QSYS2.DISK_STATUS TO monitor_user;
GRANT SELECT ON QSYS2.SUBSYSTEM_INFO TO monitor_user;
GRANT SELECT ON QSYS2.JOB_QUEUE_INFO TO monitor_user;
```

### SSL/TLS Configuration

For encrypted connections:

**DB2 SSL:**
```yaml
dsn: 'DATABASE=sample;HOSTNAME=db2.example.com;PORT=50001;PROTOCOL=TCPIP;UID=user;PWD=pass;SECURITY=SSL;SSLServerCertificate=/path/to/cert.arm'
```

**WebSphere HTTPS/TLS:**
```yaml
# For both PMI and MicroProfile collectors
tls_ca: /path/to/ca.crt
tls_cert: /path/to/client.crt
tls_key: /path/to/client.key
tls_skip_verify: false
```

### Credential Management

- Use strong passwords
- Consider using environment variables for sensitive data
- Restrict configuration file permissions:
  ```bash
  sudo chmod 600 /etc/netdata/ibm.d/*.conf
  sudo chown netdata:netdata /etc/netdata/ibm.d/*.conf
  ```

## Performance Tuning

### Connection Pooling

```yaml
# DB2/AS400
max_db_conns: 2          # Increase for parallel queries
max_db_life_time: 10m    # Reduce for connection freshness

# WebSphere
timeout: 10              # Increase for slow networks
```

### Update Intervals

Balance between data freshness and system load:

```yaml
update_every: 5   # Production critical
update_every: 30  # Development/test
update_every: 60  # Low-priority monitoring
```

### Cardinality Management

Prevent metric explosion in large environments:

```yaml
# Strict limits
max_databases: 5
max_tablespaces: 20
max_connections: 50

# With filtering
collect_databases_matching: 'PROD*|CRITICAL*'
max_databases: 0  # No limit since we're filtering
```

## Virtual Nodes

Virtual nodes allow organizing metrics from multiple IBM systems:

```yaml
# Group by environment
vnode: 'ibm-production'
vnode: 'ibm-staging'
vnode: 'ibm-development'

# Group by location
vnode: 'ibm-datacenter-east'
vnode: 'ibm-datacenter-west'

# Group by application
vnode: 'erp-database-cluster'
vnode: 'crm-application-servers'
```

## Contributing

When contributing to the IBM.d.plugin:

1. Follow the patterns in [WRITING_COLLECTORS.md](../../WRITING_COLLECTORS.md)
2. Use context.Context properly in all functions
3. Implement proper error handling and logging
4. Add unit tests for new functionality
5. Update documentation for new features

## Support

For issues and questions:

1. Check the [troubleshooting section](#troubleshooting)
2. Review Netdata logs: `journalctl -u netdata -f`
3. Run in debug mode: `./ibm.d.plugin -d`
4. Open an issue on [GitHub](https://github.com/netdata/netdata/issues)

## License

The IBM.d.plugin is part of Netdata and is released under the [GPL v3+ License](https://github.com/netdata/netdata/blob/master/LICENSE).

Note: IBM DB2 client libraries are subject to IBM's licensing terms.