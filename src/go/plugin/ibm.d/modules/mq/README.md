# IBM MQ collector

## Overview

Monitors IBM MQ queue managers, queues, channels, and topics
using the PCF (Programmable Command Format) protocol.

By default the collector tracks the critical system queues `SYSTEM.DEAD.LETTER.QUEUE`,
`SYSTEM.ADMIN.COMMAND.QUEUE`, and `SYSTEM.ADMIN.STATISTICS.QUEUE`. All other queues are
opt-in via the `include_queues` list, with `exclude_queues` removing noisy patterns such as
`SYSTEM.*` or `AMQ.*`. Include patterns take precedence over excludes so you can safely
monitor individual system queues while dropping the broader wildcard.

Per-queue charts are bounded by `max_queues` (default 50). When more queues are discovered,
the collector exports the busiest ones individually, rolls the remainder into an
aggregated `__other__` dimension, and logs a throttled warning listing the overflowed
groups. Parallel queue-group charts summarise depth, traffic, and backlog per naming
prefix (first two dot-separated segments, collapsing all `SYSTEM.*` queues together), so
high-level visibility is never lost even when detailed charts are trimmed.


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per IBM MQ instance


These metrics refer to the entire monitored IBM MQ instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.qmgr.status | status | status |
| mq.qmgr.connection_count | connections | connections |
| mq.qmgr.uptime | uptime | seconds |
| mq.queues.overview | monitored, excluded, invisible, failed | queues |
| mq.channels.overview | monitored, excluded, invisible, failed | channels |
| mq.topics.overview | monitored, excluded, invisible, failed | topics |
| mq.listeners.overview | monitored, excluded, invisible, failed | listeners |

These metrics refer to the entire monitored IBM MQ instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.qmgr.cpu_usage | user, system | percentage |
| mq.qmgr.memory_usage | total | bytes |
| mq.qmgr.ram_usage | total | bytes |
| mq.qmgr.log_utilization | used | percentage |
| mq.qmgr.log_file_size | size | bytes |
| mq.qmgr.log_write_rate | rate | bytes/s |



### Per channel

These metrics refer to individual channel instances.

Labels:

| Label | Description |
|:------|:------------|
| channel | Channel identifier |
| type | Type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.channel.status | inactive, binding, starting, running, stopping, retrying, stopped, requesting, paused, disconnected, initializing, switching | status |
| mq.channel.messages | messages | messages/s |
| mq.channel.bytes | bytes | bytes/s |
| mq.channel.batches | batches | batches/s |
| mq.channel.batch_size | batch_size | messages |
| mq.channel.batch_interval | batch_interval | milliseconds |
| mq.channel.intervals | disc_interval, hb_interval, keep_alive_interval | seconds |
| mq.channel.short_retry_count | short_retry | retries |
| mq.channel.long_retry_interval | long_retry | seconds |
| mq.channel.max_msg_length | max_msg_length | bytes |
| mq.channel.sharing_conversations | sharing_conversations | conversations |
| mq.channel.network_priority | network_priority | priority |
| mq.channel.buffer_counts | sent, received | buffers |
| mq.channel.current_messages | current | messages |
| mq.channel.xmitq_time | xmitq_time | milliseconds |
| mq.channel.mca_status | mca_status | status |
| mq.channel.indoubt_status | indoubt_status | status |
| mq.channel.ssl_key_resets | ssl_key_resets | resets |
| mq.channel.npm_speed | npm_speed | speed |
| mq.channel.current_sharing_convs | current_sharing | conversations |

### Per channelstatistics

These metrics refer to individual channelstatistics instances.

Labels:

| Label | Description |
|:------|:------------|
| channel | Channel identifier |
| type | Type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.channel_stats.messages | messages | messages/s |
| mq.channel_stats.bytes | bytes | bytes/s |
| mq.channel_stats.batches | full_batches, incomplete_batches | batches/s |
| mq.channel_stats.batch_size | avg_batch_size | messages |
| mq.channel_stats.put_retries | put_retries | retries/s |

### Per listener

These metrics refer to individual listener instances.

Labels:

| Label | Description |
|:------|:------------|
| listener | Listener identifier |
| port | Port identifier |
| ip_address | Ip_address identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.listener.status | stopped, starting, running, stopping, retrying | status |
| mq.listener.backlog | backlog | connections |
| mq.listener.uptime | uptime | seconds |

### Per mqistatistics

These metrics refer to individual mqistatistics instances.

Labels:

| Label | Description |
|:------|:------------|
| queue_manager | Queue_manager identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.mqi_stats.opens | opens_total, opens_failed | operations/s |
| mq.mqi_stats.closes | closes_total, closes_failed | operations/s |
| mq.mqi_stats.inqs | inqs_total, inqs_failed | operations/s |
| mq.mqi_stats.sets | sets_total, sets_failed | operations/s |

### Per queue

These metrics refer to individual queue instances.

Labels:

| Label | Description |
|:------|:------------|
| queue | Queue identifier |
| type | Type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.queue.depth | current, max | messages |
| mq.queue.depth_percentage | percentage | percentage |
| mq.queue.messages | enqueued, dequeued | messages/s |
| mq.queue.connections | input, output | connections |
| mq.queue.high_depth | high_depth | messages |
| mq.queue.uncommitted_msgs | uncommitted | messages |
| mq.queue.file_size | current, max | bytes |
| mq.queue.last_activity | since_last_get, since_last_put | seconds |
| mq.queue.oldest_msg_age | oldest_msg_age | seconds |
| mq.queue.time_indicators | short_period, long_period | microseconds |
| mq.queue.service_interval | service_interval | milliseconds |
| mq.queue.inhibit_status | inhibit_get, inhibit_put | status |
| mq.queue.priority | def_priority | priority |
| mq.queue.message_persistence | persistent, non_persistent | boolean |
| mq.queue.retention_interval | retention_interval | hours |
| mq.queue.triggers | trigger_depth, trigger_type | messages |
| mq.queue.backout_threshold | backout_threshold | retries |
| mq.queue.max_msg_length | max_msg_length | bytes |
| mq.queue.scope | queue_manager, cell | boolean |
| mq.queue.usage | normal, transmission | boolean |
| mq.queue.msg_delivery_sequence | priority, fifo | boolean |
| mq.queue.harden_get_backout | enabled, disabled | boolean |

### Per queuegroup

These metrics refer to individual queuegroup instances.

Labels:

| Label | Description |
|:------|:------------|
| group | Group identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.queue_group.depth | current, max | messages |
| mq.queue_group.depth_percentage | percentage | percentage |
| mq.queue_group.messages | enqueued, dequeued | messages/s |
| mq.queue_group.connections | input, output | connections |
| mq.queue_group.uncommitted_msgs | uncommitted | messages |
| mq.queue_group.file_size | current, max | bytes |
| mq.queue_group.oldest_msg_age | oldest_msg_age | seconds |

### Per queuestatistics

These metrics refer to individual queuestatistics instances.

Labels:

| Label | Description |
|:------|:------------|
| queue | Queue identifier |
| type | Type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.queue_stats.depth_min_max | min_depth, max_depth | messages |
| mq.queue_stats.avg_queue_time | non_persistent, persistent | microseconds |
| mq.queue_stats.time_indicators | short_period, long_period | microseconds |
| mq.queue_stats.operations | puts_non_persistent, puts_persistent, gets_non_persistent, gets_persistent, put1s, browses | operations/s |
| mq.queue_stats.bytes | put_bytes_non_persistent, put_bytes_persistent, get_bytes_non_persistent, get_bytes_persistent, browse_bytes | bytes/s |
| mq.queue_stats.failures | puts_failed, put1s_failed, gets_failed, browses_failed | failures/s |
| mq.queue_stats.message_lifecycle | expired, purged, not_queued | messages/s |

### Per subscription

These metrics refer to individual subscription instances.

Labels:

| Label | Description |
|:------|:------------|
| subscription | Subscription identifier |
| topic | Topic identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.subscription.messages | pending | messages |
| mq.subscription.last_message_age | age | seconds |

### Per topic

These metrics refer to individual topic instances.

Labels:

| Label | Description |
|:------|:------------|
| topic | Topic identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.topic.publishers | publishers | publishers |
| mq.topic.subscribers | subscribers | subscribers |
| mq.topic.messages | messages | messages/s |
| mq.topic.time_since_last_message | time_since_last_msg | seconds |


## Configuration

### File

The configuration file name for this integration is `ibm.d/mq.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/mq.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| update_every | Data collection frequency | `10` | no | 1 | - |
| QueueManager | IBM MQ Queue Manager name to connect to | `QM1` | no | - | - |
| Channel | IBM MQ channel name for connection | `SYSTEM.DEF.SVRCONN` | no | - | - |
| Host | IBM MQ server hostname or IP address | `localhost` | no | - | - |
| Port | IBM MQ server port number | `1414` | no | 1 | 65535 |
| User | Username for IBM MQ authentication | `` | no | - | - |
| Password | Password for IBM MQ authentication | `` | no | - | - |
| CollectQueues | Enable collection of queue metrics | `true` | no | - | - |
| CollectChannels | Enable collection of channel metrics | `true` | no | - | - |
| CollectTopics | Enable collection of topic metrics | `true` | no | - | - |
| CollectListeners | Enable collection of listener metrics | `true` | no | - | - |
| CollectSubscriptions | Enable collection of subscription metrics | `true` | no | - | - |
| CollectSystemQueues | Enable collection of system queue metrics (SYSTEM.* queues provide critical infrastructure visibility) | `true` | no | - | - |
| CollectSystemChannels | Enable collection of system channel metrics (SYSTEM.* channels show clustering and administrative health) | `true` | no | - | - |
| CollectSystemTopics | Enable collection of system topic metrics (SYSTEM.* topics show internal messaging patterns) | `true` | no | - | - |
| CollectSystemListeners | Enable collection of system listener metrics (SYSTEM.* listeners show internal connectivity) | `true` | no | - | - |
| CollectChannelConfig | Enable collection of channel configuration metrics | `true` | no | - | - |
| CollectQueueConfig | Enable collection of queue configuration metrics | `true` | no | - | - |
| IncludeQueues | Patterns to include queues (wildcards supported). Empty means include everything. | `[SYSTEM.DEAD.LETTER.QUEUE SYSTEM.ADMIN.COMMAND.QUEUE SYSTEM.ADMIN.STATISTICS.QUEUE]` | no | - | - |
| ExcludeQueues | Patterns to exclude queues after inclusion (wildcards supported). | `[SYSTEM.* AMQ.*]` | no | - | - |
| ChannelSelector | Pattern to filter channels (wildcards supported) | `` | no | - | - |
| TopicSelector | Pattern to filter topics (wildcards supported) | `` | no | - | - |
| ListenerSelector | Pattern to filter listeners (wildcards supported) | `` | no | - | - |
| SubscriptionSelector | Pattern to filter subscriptions (wildcards supported) | `` | no | - | - |
| MaxQueues | Maximum number of queues to collect (0 = no limit) | `50` | no | - | - |
| MaxChannels | Maximum number of channels to collect (0 = no limit) | `100` | no | - | - |
| MaxTopics | Maximum number of topics to collect (0 = no limit) | `100` | no | - | - |
| MaxListeners | Maximum number of listeners to collect (0 = no limit) | `100` | no | - | - |
| CollectResetQueueStats | Enable collection of queue statistics (destructive operation) | `false` | no | - | - |
| CollectStatisticsQueue | Enable collection of statistics queue metrics (SYSTEM.ADMIN.STATISTICS.QUEUE provides advanced metrics like min/max depth) | `false` | no | - | - |
| CollectSysTopics | Enable collection of $SYS topic metrics (provides Queue Manager CPU, memory, and log utilization) | `false` | no | - | - |
| StatisticsInterval | Statistics collection interval in seconds (auto-detected STATINT overwrites this value) | `60` | no | - | - |
| SysTopicInterval | $SYS topic collection interval in seconds (user override for customized MQ configurations) | `10` | no | - | - |

### Examples

#### Basic configuration

IBM MQ monitoring with default settings.

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

To troubleshoot issues with the `mq` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m mq
```

## Getting Logs

If you're encountering problems with the `mq` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep mq
```

### For non-systemd systems

```bash
sudo grep mq /var/log/netdata/error.log
sudo grep mq /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep mq
```
