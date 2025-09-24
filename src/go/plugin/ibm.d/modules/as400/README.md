# IBM i (AS/400) collector

## Overview

Monitors IBM i (AS/400) systems using SQL services and CL commands to
expose CPU, memory, storage, job, and subsystem activity.


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
| update_every | Data collection frequency | `1` | no | 1 | - |
| Vnode | Vnode | `` | no | - | - |
| DSN | Data Source Name (DSN) for connection | `` | no | - | - |
| Timeout | Connection timeout duration in seconds | `<no value>` | no | - | - |
| MaxDbConns | Maximum number of db conns to monitor | `1` | no | - | - |
| MaxDbLifeTime | Maximum number of db life time to monitor | `<no value>` | no | - | - |
| Hostname | Server hostname or IP address | `` | no | - | - |
| Port | Server port number | `8471` | no | 1 | 65535 |
| Username | Username for authentication | `` | no | - | - |
| Password | Password for authentication | `` | no | - | - |
| Database | Database | `*SYSBAS` | no | - | - |
| ConnectionType | Connection type | `odbc` | no | - | - |
| ODBCDriver | O d b c driver | `IBM i Access ODBC Driver` | no | - | - |
| UseSSL | Enable SSL/TLS encrypted connection | `false` | no | - | - |
| MaxDisks | Maximum number of disks to monitor | `100` | no | - | - |
| MaxSubsystems | Maximum number of subsystems to monitor | `100` | no | - | - |
| MaxJobQueues | Maximum number of job queues to monitor | `100` | no | - | - |
| MaxActiveJobs | Maximum number of active jobs to monitor | `100` | no | - | - |
| DiskSelector | Pattern to filter disk (wildcards supported) | `` | no | - | - |
| SubsystemSelector | Pattern to filter subsystem (wildcards supported) | `` | no | - | - |
| JobQueueSelector | Pattern to filter job queue (wildcards supported) | `` | no | - | - |

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
