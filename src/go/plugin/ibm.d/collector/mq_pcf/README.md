# IBM MQ PCF collector

## Overview

The IBM MQ PCF (Programmable Command Format) collector monitors IBM MQ queue managers, queues, channels, and topics using native IBM MQ PCF commands. This collector provides comprehensive monitoring of MQ infrastructure with real-time metrics collection.

## Collected metrics

This collector gathers metrics from:

- **Queue Manager**: Status and monitoring overview statistics
- **Queues**: Current depth, configuration limits, inhibit status, activity metrics, message rates (when available)
- **Channels**: Status, message rates, data transfer rates, batch rates, configuration parameters
- **Topics**: Publishers, subscribers, message rates (optional)

All metrics include proper labeling for multi-instance environments and support dynamic discovery of MQ objects. Each chart includes version labels (`mq_version`, `mq_edition`) for filtering and grouping by MQ version/edition.

## Requirements

- IBM MQ client libraries version 8.0 or newer
- IBM MQ queue manager running and accessible
- Appropriate MQ permissions for monitoring user
- Server connection channel configured on queue manager

## Configuration

### Basic Configuration

```yaml
jobs:
  - name: local_qm
    queue_manager: 'QM1'
    host: 'localhost'
    port: 1414
    collect_queues: true
    collect_channels: true
```

### Remote Queue Manager

```yaml
jobs:
  - name: production_qm
    queue_manager: 'PROD.QM'
    channel: 'NETDATA.SVRCONN'
    host: 'mqserver.example.com'
    port: 1414
    user: 'netdata'
    password: 'monitoring_password'
    collect_queues: true
    collect_channels: true
    collect_topics: true
```

### Filtered Monitoring

```yaml
jobs:
  - name: app_monitoring
    queue_manager: 'APP.QM'
    host: 'localhost'
    port: 1414
    queue_selector: '^APP\.'     # Monitor only queues starting with APP.
    channel_selector: '^TO\.'    # Monitor only channels starting with TO.
```

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `update_every` | `5` | Data collection frequency in seconds |
| `queue_manager` | *required* | Name of the IBM MQ queue manager |
| `channel` | `SYSTEM.DEF.SVRCONN` | Server connection channel name |
| `host` | `localhost` | Queue manager hostname or IP |
| `port` | `1414` | Queue manager listener port |
| `user` | | Username for authentication (optional) |
| `password` | | Password for authentication (optional) |
| `collect_queues` | `true` | Enable queue statistics collection |
| `collect_channels` | `true` | Enable channel statistics collection |
| `collect_topics` | `false` | Enable topic statistics collection |
| `collect_system_queues` | `true` | Enable system queue statistics collection (SYSTEM.* queues) |
| `collect_system_channels` | `true` | Enable system channel statistics collection (SYSTEM.* channels) |
| `collect_channel_config` | `false` | Enable static channel configuration collection (timeouts, limits, etc) |
| `queue_selector` | | Regular expression to filter queue names |
| `channel_selector` | | Regular expression to filter channel names |
| `collect_reset_queue_stats` | `false` | **DESTRUCTIVE**: Enable message counter collection using MQCMD_RESET_Q_STATS |

## Implementation Details

This collector uses IBM MQ's Programmable Command Format (PCF) interface to:

1. **Connect** to queue managers using MQCONN
2. **Send PCF commands** via SYSTEM.ADMIN.COMMAND.QUEUE
3. **Receive responses** from SYSTEM.ADMIN.COMMAND.REPLY.MODEL
4. **Parse PCF messages** to extract metrics
5. **Manage connections** with automatic reconnection

### PCF Commands Used

- `MQCMD_INQUIRE_Q_MGR_STATUS` - Queue manager status
- `MQCMD_INQUIRE_Q` - Queue discovery and configuration
- `MQCMD_INQUIRE_Q_STATUS` - Queue runtime statistics  
- `MQCMD_INQUIRE_CHANNEL` - Channel discovery and configuration
- `MQCMD_INQUIRE_CHANNEL_STATUS` - Channel status and throughput
- `MQCMD_INQUIRE_TOPIC_STATUS` - Topic activity (when enabled)
- `MQCMD_RESET_Q_STATS` - Message counters (optional, destructive)

## Permissions Required

The monitoring user must have:

- **CONNECT** authority to the queue manager
- **INQUIRE** authority on monitored queues, channels, and topics
- **DISPLAY** authority for PCF commands
- Access to **SYSTEM.ADMIN.COMMAND.QUEUE** (output)
- Access to **SYSTEM.ADMIN.COMMAND.REPLY.MODEL** (input)

### Setting Up Permissions

```mqsc
# Connect to queue manager
runmqsc QM1

# Grant basic authorities
SETMQAUT -m QM1 -t qmgr -p netdata +connect +inq +dsp

# Grant queue inquire authority  
SETMQAUT -m QM1 -t queue -n '**' -p netdata +inq +dsp

# Grant channel inquire authority
SETMQAUT -m QM1 -t channel -n '**' -p netdata +inq +dsp

# Grant access to admin queues
SETMQAUT -m QM1 -t queue -n 'SYSTEM.ADMIN.COMMAND.QUEUE' -p netdata +put
SETMQAUT -m QM1 -t queue -n 'SYSTEM.ADMIN.COMMAND.REPLY.MODEL' -p netdata +get
```

## Troubleshooting

### Connection Issues

1. **Verify queue manager is running**:
   ```bash
   dspmq
   ```

2. **Check listener status**:
   ```mqsc
   runmqsc QM1
   DIS LISTENER(*)
   ```

3. **Verify channel configuration**:
   ```mqsc
   runmqsc QM1
   DIS CHL(SYSTEM.DEF.SVRCONN)
   ```

### Permission Problems

1. **Check user authorities**:
   ```mqsc
   runmqsc QM1
   DIS AUTHREC PRINCIPAL(netdata)
   ```

2. **Test PCF access**:
   ```bash
   # Use amqsreq or similar tools to test PCF commands
   ```

### Performance Considerations

- **Disable topic collection** if not needed (`collect_topics: false`)
- **Use selectors** to limit monitored objects
- **Increase update_every** to reduce polling frequency
- **Monitor collector logs** for performance warnings

## Chart Organization

Charts are organized into families:

- **queue_manager**: Queue manager level metrics
- **overview**: Monitoring status overview
- **queues/local**: Local queue metrics with `queue` label
- **queues/alias**: Alias queue metrics with `queue` label
- **queues/remote**: Remote queue metrics with `queue` label
- **queues/model**: Model queue metrics with `queue` label
- **channels/status**: Channel status metrics with `channel` label
- **channels/activity**: Channel activity metrics with `channel` label
- **channels/config**: Channel configuration metrics with `channel` label (when enabled)
- **topics**: Per-topic metrics with `topic` label (when enabled)

Each dynamic instance (queue/channel/topic) gets its own set of charts with appropriate labeling for filtering and grouping in the Netdata dashboard.

## Chart Labels

All charts include these labels for filtering and analysis:
- **mq_version**: IBM MQ version (e.g., "9.3.0.4")
- **mq_edition**: IBM MQ edition (e.g., "Advanced", "Standard", "Express")
- **queue/channel/topic**: Instance name for dynamic charts

These labels enable powerful filtering in Netdata Cloud dashboards, such as:
- View metrics for specific MQ versions: `mq_version="9.3.0.4"`
- Compare performance across editions: `mq_edition="Advanced"`
- Group multiple queue managers by version during upgrades

## Runtime Metrics Limitations

### Queue Manager CPU/Memory/Log Metrics

**Note**: Queue Manager CPU usage, memory usage, and log usage metrics are **not available** through standard PCF commands. These metrics require IBM MQ resource monitoring to be enabled, which is outside the scope of PCF monitoring. The PCF interface provides operational metrics but not system resource utilization.

### IBM MQ Design Constraints

IBM MQ separates queue metrics into different PCF commands with different behaviors:

1. **MQCMD_INQUIRE_Q** - Returns 63 attributes including configuration and current state
   - Non-destructive (safe for monitoring)
   - Does NOT include message counters (MSG_ENQ_COUNT, MSG_DEQ_COUNT)
   - Provides current depth, max depth, inhibit status, etc.

2. **MQCMD_INQUIRE_Q_STATUS** - Returns current runtime status
   - Non-destructive (safe for monitoring)
   - Provides open input/output counts, last GET/PUT timestamps
   - Does NOT include message counters

3. **MQCMD_RESET_Q_STATS** - Returns message counters and statistics
   - **DESTRUCTIVE** - resets all counters to zero after reading
   - Provides MSG_ENQ_COUNT, MSG_DEQ_COUNT, timing statistics
   - Breaks other monitoring tools that rely on these counters

### What This Collector Monitors

Due to these constraints, this collector provides:
- ✅ Current queue depth
- ✅ Queue configuration (max depth, high/low limits)
- ✅ Queue state (inhibit put/get status)
- ✅ Open handle counts (applications reading/writing)
- ✅ Channel throughput (for active channels)
- ⚠️ Total messages enqueued (requires enabling destructive mode)
- ⚠️ Total messages dequeued (requires enabling destructive mode)
- ⚠️ Queue timing statistics (requires destructive reset - not implemented)

### Workarounds

If you need message rate metrics:
1. **Enable destructive mode** in this collector (see section below) - if Netdata is your only monitoring tool
2. Enable MQ's built-in monitoring and use MQ Explorer
3. Use a single monitoring tool that performs RESET_Q_STATS
4. Calculate rates from queue depth changes (approximate only)
5. Use MQ event messages or accounting records

This is a fundamental limitation of IBM MQ's monitoring API design, not a limitation of this collector.

## Destructive Statistics Collection (Optional)

### Overview

The collector supports an **optional destructive mode** that enables collection of message counters (enqueued/dequeued) using `MQCMD_RESET_Q_STATS`. This mode is **disabled by default** because it has significant implications.

### What It Does

When `collect_reset_queue_stats: true` is set:
- Collects MSG_ENQ_COUNT (total messages put)
- Collects MSG_DEQ_COUNT (total messages gotten)
- Collects HIGH_Q_DEPTH (peak depth since last reset)
- **RESETS ALL COUNTERS TO ZERO** after reading them

### WARNING: Impact on Other Tools

**This feature WILL BREAK other monitoring tools** that rely on the same statistics:
- IBM MQ Explorer statistics will show zero
- Other monitoring solutions will lose their counters
- Multiple tools cannot coexist when using RESET_Q_STATS
- There is NO way to read these counters without resetting them

### When to Use This Feature

Only enable `collect_reset_queue_stats: true` when:
1. **Netdata is the ONLY monitoring tool** for your MQ infrastructure
2. You need accurate message rate metrics
3. You understand and accept the destructive nature
4. No other tools or scripts use MQ statistics

### Configuration Example

```yaml
jobs:
  # DESTRUCTIVE monitoring - only for exclusive Netdata use
  - name: exclusive_monitoring
    queue_manager: 'PROD.QM'
    host: 'localhost'
    port: 1414
    collect_reset_queue_stats: true  # WARNING: Breaks other tools!
```

### What You Get

With destructive mode enabled:
- ✅ Accurate messages/second rates
- ✅ Total message counts
- ✅ Peak queue depths
- ✅ Time since last reset
- ❌ Compatibility with other monitoring tools

### Logs and Warnings

When enabled, the collector will log prominent warnings:
```
DESTRUCTIVE statistics collection is ENABLED!
Queue message counters will be RESET TO ZERO after each collection!
This WILL BREAK other monitoring tools using the same statistics!
Only use this if Netdata is the ONLY monitoring tool for MQ!
```