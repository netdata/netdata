# Netdata IBM Monitoring Plugin

Monitor your IBM infrastructure with Netdata's comprehensive IBM ecosystem plugin, providing real-time metrics for IBM i (AS/400), DB2, IBM MQ, and WebSphere Application Server.

## What Can Be Monitored

### IBM i (AS/400)
- CPU utilization and configuration
- Memory pools and storage
- Disk I/O and health status
- Active jobs and subsystems
- Job queues and message queues
- Network interfaces and connections

For a detailed comparison with other IBM i monitoring solutions, see [IBM i Monitoring Solutions Comparison](modules/as400/IBMi-MONITORING.md).

### IBM DB2
- Database connections and locks
- Buffer pool efficiency
- Tablespace utilization
- Transaction rates
- SQL statement performance
- Memory usage by component

### IBM MQ
- Queue managers and queue depth
- Channel status and performance
- Message flow rates
- Topic and subscription metrics
- Dead letter queue monitoring

For a comprehensive comparison with other MQ monitoring solutions, see [IBM MQ Monitoring Solutions Comparison](modules/mq/MQ-MONITORING.md).

### WebSphere Application Server
- JVM heap and thread pools
- Connection pool statistics
- Servlet response times
- Session management
- PMI (Performance Monitoring Infrastructure) metrics
- Transaction rates and response times

## Prerequisites

The IBM monitoring plugin requires specific libraries depending on which systems you want to monitor:

| System | Required Libraries | Minimum Version |
|--------|-------------------|-----------------|
| IBM i (AS/400) | unixODBC + IBM i Access ODBC Driver | IBM i 7.2+ |
| IBM DB2 | unixODBC + IBM DB2 ODBC Driver | DB2 10.5+ |

## Installation

### Using native DEB/RPM packages

After installing Netdata normally, use your system package manager to
install the `netdata-plugin-ibm` package.

### Using a local build

1. Ensure that unixODBC development files are installed (depending on
   your platform the required package will usually be either `unixodbc-dev`
   or `unixODBC-devel`).
2. Add `--local-build-options "--enable-plugin-ibm"` to the options you
   pass to the `kickstart.sh` script for the install.

### IBM i (AS/400) Monitoring Additional Setup

1. **Download IBM i Access Client Solutions** from IBM:
   - Go to [IBM i Access Client Solutions](https://www.ibm.com/support/pages/ibm-i-access-client-solutions)
   - Download the appropriate package for Linux
   - Extract the package

2. **Install the ODBC driver:**
   ```bash
   # Navigate to the extracted directory
   cd /path/to/ibm-iaccess

   # Install the ODBC driver (example for 64-bit)
   sudo ./install_acs_64.sh
   ```

3. **Configure ODBC DSN** in `/etc/odbc.ini`:
   ```ini
   [AS400_PROD]
   Description = IBM i Production System
   Driver = IBM i Access ODBC Driver
   System = your.as400.hostname
   UserID = your_username
   Password = your_password
   Naming = 0
   DefaultLibraries = QSYS2,QSYS
   Database =
   ConnectionType = 0
   CommitMode = 2
   ExtendedDynamic = 1
   DefaultPkgLibrary = QGPL
   DefaultPackage = A/DEFAULT(IBM),2,0,1,0,512
   AllowDataCompression = 1
   LibraryView = 0
   AllowUnsupportedChar = 0
   ForceTranslation = 0
   Trace = 0
   ```

#### IBM DB2 Monitoring Additional Setup

1. **Download IBM Data Server Driver Package** from IBM:
   - Visit [IBM Data Server Driver Downloads](https://www.ibm.com/support/pages/download-initial-version-115-clients-and-drivers)
   - Download "IBM Data Server Driver Package (DS Driver)" for Linux
   - Choose the appropriate architecture (AMD64/x86-64)

2. **Install the driver:**
   ```bash
   # Extract the downloaded file
   tar -xzf ibm_data_server_driver_package_linuxx64.tar.gz
   cd dsdriver

   # Install
   sudo ./installDSDriver
   ```

3. **Configure ODBC DSN** in `/etc/odbc.ini`:
   ```ini
   [DB2_PROD]
   Description = DB2 Production Database
   Driver = /opt/ibm/dsdriver/lib/libdb2o.so
   Database = SAMPLE
   Hostname = your.db2.hostname
   Port = 50000
   Protocol = TCPIP
   UID = your_username
   PWD = your_password
   ```

## Configuration

Configuration files are located in `/etc/netdata/ibm.d/`. Each collector has its own configuration file:

### IBM i (AS/400) Configuration

Edit `/etc/netdata/ibm.d/as400.conf`:

```yaml
jobs:
  - name: production_as400
    dsn: AS400_PROD              # ODBC DSN name from /etc/odbc.ini
    update_every: 10              # Collection frequency in seconds
    collect_active_jobs: yes      # Enable job monitoring
    max_active_jobs: 100         # Limit number of jobs tracked
    reset_statistics: no         # Reset cumulative counters
```

### IBM DB2 Configuration

Edit `/etc/netdata/ibm.d/db2.conf`:

```yaml
jobs:
  - name: production_db2
    dsn: DB2_PROD                # ODBC DSN name
    update_every: 10
    database: SAMPLE             # Database to monitor
    collect_tablespaces: yes     # Enable tablespace monitoring
    collect_bufferpools: yes     # Enable buffer pool monitoring
    collect_locks: yes           # Enable lock monitoring
```

### IBM MQ Configuration

Edit `/etc/netdata/ibm.d/mq.conf`:

```yaml
jobs:
  - name: production_mq
    hostname: mq.example.com
    port: 1414
    channel: SYSTEM.DEF.SVRCONN
    queue_manager: QM1
    username: mqadmin            # Optional
    password: secret             # Optional
    update_every: 5
    collect_queues: yes
    collect_channels: yes
    collect_topics: yes
```

### WebSphere Configuration

Edit `/etc/netdata/ibm.d/websphere_pmi.conf`:

```yaml
jobs:
  - name: production_was
    hostname: was.example.com
    port: 9043                   # SOAP connector port
    username: wasadmin
    password: secret
    update_every: 10
    servlet_url: /wasPerfTool/servlet/perfservlet
    cell: Cell01
    node: Node01
    server: server1
```

## Verifying Your Setup

### Test ODBC Connectivity

```bash
# List configured DSNs
odbcinst -q -s

# Test connection to IBM i
isql -v AS400_PROD username password

# Test connection to DB2
isql -v DB2_PROD username password
```

### Test the Plugin

```bash
# Test specific collector in debug mode
sudo /usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m as400

# Run with dump mode to see collected metrics
script -c 'sudo /usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m as400 --dump=10s --dump-summary 2>&1' /dev/null
```

### Check Netdata Logs

```bash
# View plugin logs
tail -f /var/log/netdata/error.log | grep ibm.d

# Check collector status in Netdata
curl http://localhost:19999/api/v1/info | jq '.collectors'
```

## Troubleshooting

### Common Issues

#### ODBC Driver Not Found
**Error:** `Can't open lib 'IBM i Access ODBC Driver' : file not found`

**Solution:** Verify driver installation and check `/etc/odbcinst.ini`:
```bash
odbcinst -q -d
```

#### Connection Refused
**Error:** `SQL30081N A communication error has been detected`

**Solution:**
1. Verify network connectivity: `ping your.ibm.system`
2. Check firewall rules for required ports
3. Verify credentials and system names

#### Missing Libraries
**Error:** `error while loading shared libraries: libodbc.so.2`

**Solution:**
```bash
# Find and install missing library
sudo ldconfig -p | grep odbc
sudo apt-get install libodbc2  # or equivalent for your distro
```

#### Permission Denied
**Error:** `Permission denied accessing /etc/netdata/ibm.d/`

**Solution:**
```bash
# Fix permissions
sudo chown -R netdata:netdata /etc/netdata/ibm.d/
sudo chmod 640 /etc/netdata/ibm.d/*.conf
```

### Performance Tuning

For large IBM i systems with many jobs:
```yaml
jobs:
  - name: tuned_as400
    dsn: AS400_PROD
    update_every: 30            # Reduce frequency
    max_active_jobs: 50         # Limit job collection
    connection_timeout: 30       # Increase timeout
    query_timeout: 25           # Increase query timeout
```

For high-volume MQ systems:
```yaml
jobs:
  - name: tuned_mq
    # ... connection details ...
    update_every: 15            # Balance load vs granularity
    queue_filter: "APP.*"       # Monitor specific queues
    channel_filter: "SYSTEM.*"  # Exclude system channels
```

## Security Considerations

1. **Use dedicated monitoring accounts** with minimal required permissions
2. **Encrypt passwords** in configuration files:
   ```bash
   # Store credentials in environment variables
   export IBM_AS400_PASSWORD='secret'
   ```
   Then reference in config:
   ```yaml
   password: ${IBM_AS400_PASSWORD}
   ```

3. **Restrict configuration file access:**
   ```bash
   chmod 600 /etc/netdata/ibm.d/*.conf
   chown netdata:netdata /etc/netdata/ibm.d/*.conf
   ```

4. **Use SSL/TLS connections** where supported by configuring ODBC with SSL

## Metrics and Alerts

The plugin automatically creates alerts for critical conditions:

- **CPU utilization** > 80% sustained
- **Disk space** < 10% free
- **Message queue depth** > threshold
- **Lock wait time** > 30 seconds
- **Connection pool exhaustion**

View active alerts:
```bash
curl http://localhost:19999/api/v1/alarms | jq '.alarms'
```

## Getting Help

- **Documentation:** Full metrics reference available in each module's metadata.yaml
- **IBM i Comparison:** [Detailed comparison with other IBM i monitoring tools](modules/as400/IBMi-MONITORING.md)
- **MQ Comparison:** [DevOps evaluation guide for MQ monitoring solutions](modules/mq/MQ-MONITORING.md)
- **Community:** [Netdata Community Forums](https://community.netdata.cloud)
- **Issues:** [GitHub Issues](https://github.com/netdata/netdata/issues)
- **Developer Guide:** See [AGENTS.md](AGENTS.md) for contributing

## License

The IBM monitoring plugin is part of Netdata and is released under the GPL v3+ license. IBM client libraries are subject to IBM's licensing terms.
