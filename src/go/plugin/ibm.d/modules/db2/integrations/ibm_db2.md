# IBM DB2 collector

## Overview

Monitors IBM DB2 databases using system catalog views and MON_GET_* table
functions to expose connections, locking, buffer pool efficiency, tablespace
capacity, and workload performance metrics.

Detailed charts are opt-in per object family through include/exclude lists.
Defaults focus on engine activity (system connections, core buffer pools,
catalog tablespaces). Matching uses glob patterns that can target schema or
application names, with include rules taking precedence over excludes.

When the number of matching objects exceeds the configured `max_*` limits,
the collector publishes deterministic top-N per-instance charts, aggregates
the remainder under `group="__other__"`, and logs a throttled warning so you
can refine selectors before cardinality runs away. Group charts (by schema,
application prefix, or buffer pool family) are always emitted so high-level
visibility is preserved even when individual instances are trimmed.


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per IBM DB2 instance


These metrics refer to the entire monitored IBM DB2 instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.service_health | connection, database | status |
| db2.connections | total, active, executing, idle, max_allowed | connections |
| db2.locking | waits, timeouts, escalations | events/s |
| db2.deadlocks | deadlocks | deadlocks/s |
| db2.lock_details | active, waiting_agents, memory_pages | locks |
| db2.lock_wait_time | wait_time | milliseconds |
| db2.sorting | sorts, overflows | sorts/s |
| db2.row_activity | read, returned, modified | rows/s |
| db2.bufferpool_hit_ratio | hits, misses | percentage |
| db2.bufferpool_data_hit_ratio | hits, misses | percentage |
| db2.bufferpool_index_hit_ratio | hits, misses | percentage |
| db2.bufferpool_xda_hit_ratio | hits, misses | percentage |
| db2.bufferpool_column_hit_ratio | hits, misses | percentage |
| db2.bufferpool_reads | logical, physical | reads/s |
| db2.bufferpool_data_reads | logical, physical | reads/s |
| db2.bufferpool_index_reads | logical, physical | reads/s |
| db2.bufferpool_xda_reads | logical, physical | reads/s |
| db2.bufferpool_column_reads | logical, physical | reads/s |
| db2.bufferpool_writes | writes | writes/s |
| db2.log_space | used, available | bytes |
| db2.log_utilization | utilization | percentage |
| db2.log_io | reads, writes | operations/s |
| db2.log_operations | commits, rollbacks, reads, writes | operations/s |
| db2.log_timing | avg_commit, avg_read, avg_write | milliseconds |
| db2.log_buffer_events | buffer_full | events/s |
| db2.long_running_queries | total, warning, critical | queries |
| db2.backup_status | status | status |
| db2.backup_age | full, incremental | hours |
| db2.federation_connections | active, idle | connections |
| db2.federation_operations | rows_read, selects, waits | operations/s |
| db2.database_status | active, inactive | status |
| db2.database_count | active, inactive | databases |
| db2.cpu_usage | user, system, idle, iowait | percentage |
| db2.active_connections | active, total | connections |
| db2.memory_usage | database, instance, bufferpool, shared_sort | MiB |
| db2.sql_statements | selects, modifications | statements/s |
| db2.transaction_activity | committed, aborted | transactions/s |
| db2.time_spent | direct_read, direct_write, pool_read, pool_write | milliseconds |



### Per bufferpool

These metrics refer to individual bufferpool instances.

Labels:

| Label | Description |
|:------|:------------|
| bufferpool | Bufferpool identifier |
| page_size | Page_size identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.bufferpool_instance_hit_ratio | overall | percentage |
| db2.bufferpool_instance_detailed_hit_ratio | data, index, xda, column | percentage |
| db2.bufferpool_instance_reads | logical, physical | reads/s |
| db2.bufferpool_instance_data_reads | logical, physical | reads/s |
| db2.bufferpool_instance_index_reads | logical, physical | reads/s |
| db2.bufferpool_instance_pages | used, total | pages |
| db2.bufferpool_instance_writes | writes | writes/s |

### Per bufferpoolgroup

These metrics refer to individual bufferpoolgroup instances.

Labels:

| Label | Description |
|:------|:------------|
| group | Group identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.bufferpool_group_hit_ratio | overall | percentage |
| db2.bufferpool_group_detailed_hit_ratio | data, index, xda, column | percentage |
| db2.bufferpool_group_reads | logical, physical | reads/s |
| db2.bufferpool_group_data_reads | logical, physical | reads/s |
| db2.bufferpool_group_index_reads | logical, physical | reads/s |
| db2.bufferpool_group_pages | used, total | pages |
| db2.bufferpool_group_writes | writes | writes/s |

### Per connection

These metrics refer to individual connection instances.

Labels:

| Label | Description |
|:------|:------------|
| application_id | Application_id identifier |
| application_name | Application_name identifier |
| client_hostname | Client_hostname identifier |
| client_ip | Client_ip identifier |
| client_user | Client_user identifier |
| state | State identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.connection_state | state | state |
| db2.connection_activity | read, written | rows/s |
| db2.connection_wait_time | lock, log_disk, log_buffer, pool_read, pool_write, direct_read, direct_write, fcm_recv, fcm_send | milliseconds |
| db2.connection_processing_time | routine, compile, section, commit, rollback | milliseconds |

### Per connectiongroup

These metrics refer to individual connectiongroup instances.

Labels:

| Label | Description |
|:------|:------------|
| group | Group identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.connection_group.count | count | connections |
| db2.connection_group.state | state | state |
| db2.connection_group.activity | read, written | rows/s |
| db2.connection_group.wait_time | lock, log_disk, log_buffer, pool_read, pool_write, direct_read, direct_write, fcm_recv, fcm_send | milliseconds |
| db2.connection_group.processing_time | routine, compile, section, commit, rollback | milliseconds |

### Per database

These metrics refer to individual database instances.

Labels:

| Label | Description |
|:------|:------------|
| database | Database identifier |
| status | Status identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.database_instance_status | status | status |
| db2.database_applications | applications | applications |

### Per index

These metrics refer to individual index instances.

Labels:

| Label | Description |
|:------|:------------|
| index | Index identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.index_usage | index, full | scans/s |

### Per indexgroup

These metrics refer to individual indexgroup instances.

Labels:

| Label | Description |
|:------|:------------|
| group | Group identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.index_group_usage | index, full | scans/s |

### Per memorypool

These metrics refer to individual memorypool instances.

Labels:

| Label | Description |
|:------|:------------|
| pool_type | Pool_type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.memory_pool_usage | used | bytes |
| db2.memory_pool_hwm | hwm | bytes |

### Per memoryset

These metrics refer to individual memoryset instances.

Labels:

| Label | Description |
|:------|:------------|
| host | Host identifier |
| database | Database identifier |
| set_type | Set_type identifier |
| member | Member identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.memory_set_usage | used | bytes |
| db2.memory_set_committed | committed | bytes |
| db2.memory_set_high_water_mark | hwm | bytes |
| db2.memory_set_additional_committed | additional | bytes |
| db2.memory_set_percent_used_hwm | used_hwm | percentage |

### Per prefetcher

These metrics refer to individual prefetcher instances.

Labels:

| Label | Description |
|:------|:------------|
| bufferpool | Bufferpool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.prefetcher_prefetch_ratio | ratio | percentage |
| db2.prefetcher_cleaner_ratio | ratio | percentage |
| db2.prefetcher_physical_reads | reads | reads/s |
| db2.prefetcher_async_reads | reads | reads/s |
| db2.prefetcher_wait_time | wait_time | milliseconds |
| db2.prefetcher_unread_pages | unread | pages/s |

### Per table

These metrics refer to individual table instances.

Labels:

| Label | Description |
|:------|:------------|
| table | Table identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.table_size | data, index, long_obj | bytes |
| db2.table_activity | read, written | rows/s |

### Per tablegroup

These metrics refer to individual tablegroup instances.

Labels:

| Label | Description |
|:------|:------------|
| group | Group identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.table_group_size | data, index, long_obj | bytes |
| db2.table_group_activity | read, written | rows/s |

### Per tableio

These metrics refer to individual tableio instances.

Labels:

| Label | Description |
|:------|:------------|
| table | Table identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.table_io_scans | scans | scans/s |
| db2.table_io_rows | read | rows/s |
| db2.table_io_activity | inserts, updates, deletes | operations/s |
| db2.table_io_overflow | overflow | accesses/s |

### Per tablespace

These metrics refer to individual tablespace instances.

Labels:

| Label | Description |
|:------|:------------|
| tablespace | Tablespace identifier |
| type | Type identifier |
| content_type | Content_type identifier |
| state | State identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.tablespace_usage | used | percentage |
| db2.tablespace_size | used, free | bytes |
| db2.tablespace_usable_size | total, usable | bytes |
| db2.tablespace_state | state | state |

### Per tablespacegroup

These metrics refer to individual tablespacegroup instances.

Labels:

| Label | Description |
|:------|:------------|
| group | Group identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| db2.tablespace_group_usage | used | percentage |
| db2.tablespace_group_size | used, free | bytes |
| db2.tablespace_group_usable_size | total, usable | bytes |
| db2.tablespace_group_state | state | state |


## Configuration

### File

The configuration file name for this integration is `ibm.d/db2.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/db2.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| update_every | Data collection frequency | `5` | no | 1 | - |
| Vnode | Vnode allows binding the collector to a virtual node. | `` | no | - | - |
| DSN | DSN provides a full DB2 connection string when manual control is required. | `` | no | - | - |
| Timeout | Timeout controls how long DB2 RPCs may run before cancellation. | `2000000000` | no | - | - |
| MaxDbConns | MaxDbConns limits the connection pool size. | `1` | no | - | - |
| MaxDbLifeTime | MaxDbLifeTime forces pooled connections to be recycled after the specified duration. | `600000000000` | no | - | - |
| CollectDatabaseMetrics | CollectDatabaseMetrics toggles high-level database status metrics. | `auto` | no | - | - |
| CollectBufferpoolMetrics | CollectBufferpoolMetrics toggles buffer pool efficiency metrics. | `auto` | no | - | - |
| CollectTablespaceMetrics | CollectTablespaceMetrics toggles tablespace capacity metrics. | `auto` | no | - | - |
| CollectConnectionMetrics | CollectConnectionMetrics toggles per-connection activity metrics. | `auto` | no | - | - |
| CollectLockMetrics | CollectLockMetrics toggles lock contention metrics. | `auto` | no | - | - |
| CollectTableMetrics | CollectTableMetrics toggles table-level size and row metrics. | `auto` | no | - | - |
| CollectIndexMetrics | CollectIndexMetrics toggles index usage metrics. | `auto` | no | - | - |
| MaxDatabases | MaxDatabases caps the number of databases charted. | `10` | no | - | - |
| MaxBufferpools | MaxBufferpools caps the number of buffer pools charted. | `20` | no | - | - |
| MaxTablespaces | MaxTablespaces caps the number of tablespaces charted. | `50` | no | - | - |
| MaxConnections | MaxConnections caps the number of connection instances charted. | `50` | no | - | - |
| MaxTables | MaxTables caps the number of tables charted. | `25` | no | - | - |
| MaxIndexes | MaxIndexes caps the number of indexes charted. | `50` | no | - | - |
| BackupHistoryDays | BackupHistoryDays controls how many days of backup history are retrieved. | `30` | no | - | - |
| CollectMemoryMetrics | CollectMemoryMetrics enables memory pool statistics. | `true` | no | - | - |
| CollectWaitMetrics | CollectWaitMetrics enables wait time statistics (locks, logs, I/O). | `true` | no | - | - |
| CollectTableIOMetrics | CollectTableIOMetrics enables table I/O statistics when available. | `true` | no | - | - |
| CollectDatabasesMatching | CollectDatabasesMatching filters databases by name using glob patterns. | `` | no | - | - |
| IncludeConnections | IncludeConnections filters monitored connections by application ID or application name (wildcards supported). | `[db2sysc* db2agent* db2hadr* db2acd* db2bmgr*]` | no | - | - |
| ExcludeConnections | ExcludeConnections excludes connections after inclusion matching. | `[*TEMP*]` | no | - | - |
| IncludeBufferpools | IncludeBufferpools filters buffer pools by name. | `[IBMDEFAULTBP IBMSYSTEMBP* IBMHADRBP*]` | no | - | - |
| ExcludeBufferpools | ExcludeBufferpools excludes buffer pools after inclusion. | `nil` | no | - | - |
| IncludeTablespaces | IncludeTablespaces filters tablespaces by name. | `[SYSCATSPACE TEMPSPACE* SYSTOOLSPACE]` | no | - | - |
| ExcludeTablespaces | ExcludeTablespaces excludes tablespaces after inclusion. | `[TEMPSPACE2]` | no | - | - |
| IncludeTables | IncludeTables filters tables by schema/name. | `nil` | no | - | - |
| ExcludeTables | ExcludeTables excludes tables after inclusion. | `nil` | no | - | - |
| IncludeIndexes | IncludeIndexes filters indexes by schema/name. | `nil` | no | - | - |
| ExcludeIndexes | ExcludeIndexes excludes indexes after inclusion. | `nil` | no | - | - |

### Examples

#### Basic configuration

IBM DB2 monitoring with default settings.

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

To troubleshoot issues with the `db2` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m db2
```

## Getting Logs

If you're encountering problems with the `db2` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep db2
```

### For non-systemd systems

```bash
sudo grep db2 /var/log/netdata/error.log
sudo grep db2 /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep db2
```
