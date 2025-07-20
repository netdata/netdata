# IBM MQ collector

## Overview

Monitors IBM MQ queue managers, queues, channels, and topics
using the PCF (Programmable Command Format) protocol.


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
| mq.queues.overview | monitored, excluded, model, unauthorized, unknown, failed | queues |
| mq.channels.overview | monitored, excluded, unauthorized, failed | channels |
| mq.topics.overview | monitored, excluded, unauthorized, failed | topics |



### Per channel

These metrics refer to individual channel instances.

Labels:

| Label | Description |
|:------|:------------|
| channel | Channel identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mq.channel.status | inactive, binding, starting, running, stopping, retrying, stopped, requesting, paused, disconnected, initializing, switching | status |
| mq.channel.messages | messages | messages/s |
| mq.channel.bytes | bytes | bytes/s |
| mq.channel.batches | batches | batches/s |
| mq.channel.batch_config | batch_size, batch_interval | value |
| mq.channel.intervals | disc_interval, hb_interval, keep_alive_interval | seconds |
| mq.channel.retry_config | short_retry, long_retry | count |
| mq.channel.max_msg_length | max_msg_length | bytes |
| mq.channel.conversation_config | sharing_conversations, network_priority | value |

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
| mq.queue.messages | enqueued, dequeued | messages/s |
| mq.queue.connections | input, output | connections |
| mq.queue.high_depth | high_depth | messages |
| mq.queue.oldest_msg_age | oldest_msg_age | seconds |
| mq.queue.uncommitted_msgs | uncommitted | messages |
| mq.queue.last_activity | since_last_get, since_last_put | seconds |
| mq.queue.inhibit_status | inhibit_get, inhibit_put | status |
| mq.queue.priority | def_priority | priority |
| mq.queue.triggers | trigger_depth, trigger_type | messages |
| mq.queue.backout_threshold | backout_threshold | retries |
| mq.queue.max_msg_length | max_msg_length | bytes |

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
| QueueManager | IBM MQ Queue Manager name to connect to | `<no value>` | yes | - | - |
| Channel | IBM MQ channel name for connection | `<no value>` | yes | - | - |
| Host | IBM MQ server hostname or IP address | `<no value>` | yes | - | - |
| Port | IBM MQ server port number | `<no value>` | yes | 1 | 65535 |
| User | Username for IBM MQ authentication | `<no value>` | yes | - | - |
| Password | Password for IBM MQ authentication | `<no value>` | yes | - | - |
| CollectQueues | Enable collection of queue metrics | `<no value>` | yes | - | - |
| CollectChannels | Enable collection of channel metrics | `<no value>` | yes | - | - |
| CollectTopics | Enable collection of topic metrics | `<no value>` | yes | - | - |
| CollectSystemQueues | Enable collection of system queue metrics | `<no value>` | yes | - | - |
| CollectSystemChannels | Enable collection of system channel metrics | `<no value>` | yes | - | - |
| CollectChannelConfig | Enable collection of channel configuration metrics | `<no value>` | yes | - | - |
| CollectQueueConfig | Enable collection of queue configuration metrics | `<no value>` | yes | - | - |
| QueueSelector | Pattern to filter queues (wildcards supported) | `<no value>` | yes | - | - |
| ChannelSelector | Pattern to filter channels (wildcards supported) | `<no value>` | yes | - | - |
| CollectResetQueueStats | Enable collection of queue statistics (destructive operation) | `<no value>` | yes | - | - |

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
