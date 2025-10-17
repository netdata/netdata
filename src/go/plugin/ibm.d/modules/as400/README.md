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

**CPU Collection Methods:**

The collector uses a hybrid approach for CPU utilization metrics to handle IBM i 7.4+ where
`AVERAGE_CPU_*` columns were deprecated:

1. **Primary Method - TOTAL_CPU_TIME**: Uses the monotonic `TOTAL_CPU_TIME` counter from
   `QSYS2.SYSTEM_STATUS()` to calculate CPU utilization via delta-based calculation. This is
   the most accurate method but requires `*JOBCTL` special authority. TOTAL_CPU_TIME is a
   cumulative counter in nanoseconds representing CPU-seconds consumed, naturally in per-core
   scale.

2. **Fallback Method - ELAPSED_CPU_USED**: If `*JOBCTL` authority is not available, falls back
   to `ELAPSED_CPU_USED` with automatic reset detection. This method tracks when IBM i statistics
   are reset (either manually or via `reset_statistics` configuration) and re-establishes a
   baseline after detecting resets. The values are already in per-core scale.

3. **Legacy Method - AVERAGE_CPU_UTILIZATION**: For IBM i versions before 7.4, uses the now-
   deprecated `AVERAGE_CPU_UTILIZATION` column, which IBM reports in the same per-core scale.

The collector automatically selects the appropriate method based on available permissions and
logs which method is being used.

**CPU Metric Scale:**

CPU utilization is reported using the "100% = 1 CPU core" semantic. This means:
- 100% indicates one CPU core is fully utilized
- 400% indicates four CPU cores are fully utilized
- Values are limited to 100% × ConfiguredCPUs, matching the partition's configured capacity

For shared LPARs, the metrics show absolute CPU consumption in per-core scale, not relative to
entitled capacity. For example, a shared LPAR entitled to 0.20 cores can show 150% utilization
when bursting above entitlement.

**Statistics Reset Behavior:**

The `reset_statistics` configuration option controls whether the collector resets IBM i system
statistics on each query via `SYSTEM_STATUS(RESET_STATISTICS=>'YES')`. When enabled:

- System-level statistics (CPU, memory pools, etc.) are reset after each collection cycle
- Matches legacy behavior but clears global statistics that other tools may rely on
- The ELAPSED_CPU_USED fallback method will detect and handle these resets automatically
- **Caution**: Enabling this affects all users and applications on the IBM i system

Default: `false` (statistics are not reset, using `RESET_STATISTICS=>'NO'`)

**Cardinality Management:**

To prevent performance issues from excessive metric creation, the collector enforces cardinality
limits on per-instance metrics (disks, subsystems, job queues, message queues, output queues,
active jobs, network interfaces, HTTP servers).

**How Limits Work:**
- The collector counts instances before collecting metrics
- If count exceeds the configured `max_*` limit, **collection is skipped entirely** for that category
- The collector logs a warning: `"[category] count (X) exceeds limit (Y), skipping collection"`
- No metrics are collected for that category until you adjust the configuration

**Configuration Options:**

Use **both** limit and selector options together to manage high-cardinality environments:

| Option | Purpose | Default |
|--------|---------|---------|
| `max_disks` | Maximum disk units to monitor | 100 |
| `max_subsystems` | Maximum subsystems to monitor | 100 |
| `max_job_queues` | Maximum job queues to monitor | 100 |
| `max_message_queues` | Maximum message queues to monitor | 100 |
| `max_output_queues` | Maximum output queues to monitor | 100 |
| `max_active_jobs` | Maximum active jobs to monitor | 100 |
| `collect_disks_matching` | Glob pattern to filter disks (e.g., `"001* 002*"`) | `""` (match all) |
| `collect_subsystems_matching` | Glob pattern to filter subsystems (e.g., `"QINTER QBATCH"`) | `""` (match all) |
| `collect_job_queues_matching` | Glob pattern to filter job queues (e.g., `"QSYS/*"`) | `""` (match all) |

**Example Workflow:**

1. System has 500 disks, collector skips disk metrics (exceeds default limit of 100)
2. Check logs: `"disk count (500) exceeds limit (100), skipping per-disk metrics"`
3. Two options:
   - **Option A**: Increase limit: `max_disks: 500` (collects all 500 disks)
   - **Option B**: Use selector: `collect_disks_matching: "00[1-5]*"` (cherry-pick specific disks)

**Best Practices:**
- Use selectors to monitor only business-critical objects in large environments
- Set limits based on your Netdata server's capacity (each instance = multiple charts)
- Start with defaults and adjust based on actual usage patterns

**IBM i 7.2–7.3 Behavior Note (Message Queues):**

IBM i 7.4 introduced a message-queue table function that returns only the live backlog. On
7.2–7.3 systems we fall back to the `QSYS2.MESSAGE_QUEUE_INFO` view, which includes *all*
recorded messages (even those already processed/cleared from the queue). Aggregations—especially
`MAX(SEVERITY)`—therefore reflect the historical log, not just the outstanding backlog. This
behaviour is inherent to the IBM SQL service and can lead to higher-than-expected max severity
values on pre-7.4 systems.

Network interface metrics have a fixed internal limit of 50 instances, and HTTP server metrics are capped at 200 instances; these limits are currently not configurable.


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
| netdata.plugin_ibm.as400_query_latency | analyze_plan_cache, count_active_jobs, count_disks, count_http_servers, count_job_queues, count_message_queues, count_network_interfaces, count_output_queues, count_subsystems, detect_ibmi_version_primary, detect_ibmi_version_fallback, disk_instances, disk_instances_enhanced, disk_status, http_server_info, job_info, job_queues, memory_pools, message_queue_aggregates, network_connections, network_interfaces, output_queue_info, plan_cache_summary, serial_number, system_activity, system_model, system_status, temp_storage_named, temp_storage_total, technology_refresh_level, top_active_jobs, other | ms |

These metrics refer to the entire monitored IBM i (AS/400) instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| as400.cpu_utilization | utilization | percentage |
| as400.cpu_utilization_entitled | utilization | percentage |
| as400.cpu_configuration | configured | cpus |
| as400.cpu_capacity | capacity | percentage |
| as400.total_jobs | total | jobs |
| as400.active_jobs_by_type | batch, interactive, active | jobs |
| as400.job_queue_length | waiting | jobs |
| as400.main_storage_size | total | bytes |
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
| CollectActiveJobs | CollectActiveJobs toggles collection of detailed per-job metrics. | `auto` | no | - | - |
| CollectHTTPServerMetrics | CollectHTTPServerMetrics toggles collection of IBM HTTP Server statistics. | `auto` | no | - | - |
| CollectPlanCacheMetrics | CollectPlanCacheMetrics toggles collection of plan cache analysis metrics. | `auto` | no | - | - |
| MaxDisks | MaxDisks caps how many disk units may be charted. | `100` | no | - | - |
| MaxSubsystems | MaxSubsystems caps how many subsystems may be charted. | `100` | no | - | - |
| MaxActiveJobs | MaxActiveJobs caps how many active jobs may be charted. | `100` | no | - | - |
| DiskSelector | DiskSelector filters disk units by name using glob-style patterns. | `` | no | - | - |
| SubsystemSelector | SubsystemSelector filters subsystems by name using glob-style patterns. | `` | no | - | - |

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
