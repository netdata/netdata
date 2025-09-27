# IBM i (AS/400) collector

## Overview

Monitors IBM i (AS/400) systems using SQL services and CL commands to
expose CPU, memory, storage, job, and subsystem activity.

**Dependencies:**
- unixODBC 2.3+ with IBM i Access ODBC driver
- IBM i 7.2 or later with SQL services enabled

**Required Libraries:**
- libodbc.so (provided by unixODBC)
- IBM i Access Client Solutions


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per IBM i (AS/400) instance


These metrics refer to the entire monitored IBM i (AS/400) instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.cpu_utilization | utilization | percentage |
| as400.cpu_configuration | configured | cpus |
| as400.cpu_capacity | capacity | percentage |
| as400.total_jobs | total | jobs |
| as400.active_jobs_by_type | batch, interactive, active | jobs |
| as400.job_queue_length | waiting | jobs |
| as400.main_storage_size | total | KiB |
| as400.temporary_storage | current, maximum | MiB |
| as400.memory_pool_usage | machine, base, interactive, spool | bytes |
| as400.memory_pool_defined | machine, base | bytes |
| as400.memory_pool_reserved | machine, base | bytes |
| as400.memory_pool_threads | machine, base | threads |
| as400.memory_pool_max_threads | machine, base | threads |
| as400.disk_busy_average | busy | percentage |
| as400.system_asp_usage | used | percentage |
| as400.system_asp_storage | total | MiB |
| as400.total_auxiliary_storage | total | MiB |
| as400.system_threads | active, per_processor | threads |
| as400.network_connections | remote, total | connections |
| as400.network_connection_states | listen, close_wait | connections |
| as400.temp_storage_total | current, peak | bytes |
| as400.system_activity_cpu_rate | average | percentage |
| as400.system_activity_cpu_utilization | average, minimum, maximum | percentage |



### Per activejob

These metrics refer to individual activejob instances.

Labels:

| Label | Description |
|:------|:------------|
| job_name | Job_name identifier |
| job_status | Job_status identifier |
| subsystem | Subsystem identifier |
| job_type | Job_type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.activejob_cpu | cpu | percentage |
| as400.activejob_resources | temp_storage | MiB |
| as400.activejob_time | cpu_time, total_time | seconds |
| as400.activejob_activity | disk_io, interactive_transactions | operations/s |
| as400.activejob_threads | threads | threads |

### Per disk

These metrics refer to individual disk instances.

Labels:

| Label | Description |
|:------|:------------|
| disk_unit | Disk_unit identifier |
| disk_type | Disk_type identifier |
| disk_model | Disk_model identifier |
| hardware_status | Hardware_status identifier |
| disk_serial_number | Disk_serial_number identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.disk_busy | busy | percentage |
| as400.disk_io_requests | read, write | requests/s |
| as400.disk_space_usage | used | percentage |
| as400.disk_capacity | available, used | gigabytes |
| as400.disk_blocks | read, write | blocks/s |
| as400.disk_ssd_health | life_remaining | percentage |
| as400.disk_ssd_age | power_on_days | days |

### Per httpserver

These metrics refer to individual httpserver instances.

Labels:

| Label | Description |
|:------|:------------|
| server | Server identifier |
| function | Function identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.http_server_connections | normal, ssl | connections |
| as400.http_server_threads | active, idle | threads |
| as400.http_server_requests | requests, responses, rejected | requests/s |
| as400.http_server_bytes | received, sent | bytes/s |

### Per jobqueue

These metrics refer to individual jobqueue instances.

Labels:

| Label | Description |
|:------|:------------|
| job_queue | Job_queue identifier |
| library | Library identifier |
| status | Status identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.jobqueue_length | jobs | jobs |

### Per messagequeue

These metrics refer to individual messagequeue instances.

Labels:

| Label | Description |
|:------|:------------|
| library | Library identifier |
| queue | Queue identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.message_queue_messages | total, informational, inquiry, diagnostic, escape, notify, sender_copy | messages |
| as400.message_queue_severity | max | severity |

### Per networkinterface

These metrics refer to individual networkinterface instances.

Labels:

| Label | Description |
|:------|:------------|
| interface | Interface identifier |
| interface_type | Interface_type identifier |
| connection_type | Connection_type identifier |
| internet_address | Internet_address identifier |
| network_address | Network_address identifier |
| subnet_mask | Subnet_mask identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.network_interface_status | active | status |
| as400.network_interface_mtu | mtu | bytes |

### Per outputqueue

These metrics refer to individual outputqueue instances.

Labels:

| Label | Description |
|:------|:------------|
| library | Library identifier |
| queue | Queue identifier |
| status | Status identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.output_queue_files | files | files |
| as400.output_queue_writers | writers | writers |
| as400.output_queue_status | released | state |

### Per plancache

These metrics refer to individual plancache instances.

Labels:

| Label | Description |
|:------|:------------|
| metric | Metric identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.plan_cache_summary | value | value |

### Per subsystem

These metrics refer to individual subsystem instances.

Labels:

| Label | Description |
|:------|:------------|
| subsystem | Subsystem identifier |
| library | Library identifier |
| status | Status identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.subsystem_jobs | active, maximum | jobs |

### Per tempstoragebucket

These metrics refer to individual tempstoragebucket instances.

Labels:

| Label | Description |
|:------|:------------|
| bucket | Bucket identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.temp_storage_bucket | current, peak | bytes |


## Configuration

### File

The configuration file name for this integration is `ibm.d/as400.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/as400.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| update_every | Data collection frequency | `10` | no | 1 | - |
| Vnode | Vnode allows binding the collector to a virtual node. | `` | no | - | - |
| DSN | DSN provides a full IBM i ODBC connection string if manual override is needed. | `` | no | - | - |
| Timeout | Timeout controls how long to wait for SQL statements and RPCs. | `2000000000` | no | - | - |
| MaxDbConns | MaxDbConns restricts the maximum number of open ODBC connections. | `1` | no | - | - |
| MaxDbLifeTime | MaxDbLifeTime limits how long a pooled connection may live before being recycled. | `600000000000` | no | - | - |
| Hostname | Hostname is the remote IBM i host to monitor. | `` | no | - | - |
| Port | Port is the TCP port for the IBM i Access ODBC server. | `8471` | no | 1 | 65535 |
| Username | Username supplies the credentials used for authentication. | `` | no | - | - |
| Password | Password supplies the password used for authentication. | `` | no | - | - |
| Database | Database selects the IBM i database (library) to use when building the DSN. | `*SYSBAS` | no | - | - |
| ConnectionType | ConnectionType selects how the collector connects (currently only "odbc"). | `odbc` | no | - | - |
| ODBCDriver | ODBCDriver specifies the driver name registered on the host. | `IBM i Access ODBC Driver` | no | - | - |
| UseSSL | UseSSL enables TLS for the ODBC connection when supported by the driver. | `false` | no | - | - |
| ResetStatistics | ResetStatistics toggles destructive SQL services that reset system statistics on each query. | `false` | no | - | - |
| CollectDiskMetrics | CollectDiskMetrics toggles collection of disk unit statistics. | `auto` | no | - | - |
| CollectSubsystemMetrics | CollectSubsystemMetrics toggles collection of subsystem activity metrics. | `auto` | no | - | - |
| CollectJobQueueMetrics | CollectJobQueueMetrics toggles collection of job queue backlog metrics. | `auto` | no | - | - |
| CollectActiveJobs | CollectActiveJobs toggles collection of detailed per-job metrics. | `auto` | no | - | - |
| CollectHTTPServerMetrics | CollectHTTPServerMetrics toggles collection of IBM HTTP Server statistics. | `auto` | no | - | - |
| CollectMessageQueueMetrics | CollectMessageQueueMetrics toggles collection of IBM i message queue metrics. | `auto` | no | - | - |
| CollectOutputQueueMetrics | CollectOutputQueueMetrics toggles collection of IBM i output queue metrics. | `auto` | no | - | - |
| CollectPlanCacheMetrics | CollectPlanCacheMetrics toggles collection of plan cache analysis metrics. | `auto` | no | - | - |
| MaxDisks | MaxDisks caps how many disk units may be charted. | `100` | no | - | - |
| MaxSubsystems | MaxSubsystems caps how many subsystems may be charted. | `100` | no | - | - |
| MaxJobQueues | MaxJobQueues caps how many job queues may be charted. | `100` | no | - | - |
| MaxMessageQueues | MaxMessageQueues caps how many message queues may be charted. | `100` | no | - | - |
| MaxOutputQueues | MaxOutputQueues caps how many output queues may be charted. | `100` | no | - | - |
| MaxActiveJobs | MaxActiveJobs caps how many active jobs may be charted. | `100` | no | - | - |
| DiskSelector | DiskSelector filters disk units by name using glob-style patterns. | `` | no | - | - |
| SubsystemSelector | SubsystemSelector filters subsystems by name using glob-style patterns. | `` | no | - | - |
| JobQueueSelector | JobQueueSelector filters job queues by name using glob-style patterns. | `` | no | - | - |

### Examples

#### Basic configuration

IBM i (AS/400) monitoring with default settings.

<details>
<summary>Config</summary>

```yaml
jobs:
  - name: local
    endpoint: dummy://localhost
```

</details>

## Troubleshooting

### Debug Mode

To troubleshoot issues with the `as400` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m as400
```

## Getting Logs

If you're encountering problems with the `as400` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep as400
```

### For non-systemd systems

```bash
sudo grep as400 /var/log/netdata/error.log
sudo grep as400 /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep as400
```
