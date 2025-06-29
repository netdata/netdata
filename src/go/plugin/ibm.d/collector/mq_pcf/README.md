# IBM MQ PCF collector

## Overview

The IBM MQ PCF (Programmable Command Format) collector monitors IBM MQ queue managers, queues, channels, and topics using native IBM MQ PCF commands. This collector provides comprehensive monitoring of MQ infrastructure with real-time metrics collection.

## Collected metrics

This collector gathers metrics from:

- **Queue Manager**: Status, CPU usage, memory usage, log usage
- **Queues**: Current depth, message rates (enqueue/dequeue), oldest message age  
- **Channels**: Status, message rates, data transfer rates, batch rates
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
| `queue_selector` | | Regular expression to filter queue names |
| `channel_selector` | | Regular expression to filter channel names |

## Implementation Details

This collector uses IBM MQ's Programmable Command Format (PCF) interface to:

1. **Connect** to queue managers using MQCONN
2. **Send PCF commands** via SYSTEM.ADMIN.COMMAND.QUEUE
3. **Receive responses** from SYSTEM.ADMIN.COMMAND.REPLY.MODEL
4. **Parse PCF messages** to extract metrics
5. **Manage connections** with automatic reconnection

### PCF Commands Used

- `MQCMD_INQUIRE_Q_MGR_STATUS` - Queue manager status and performance
- `MQCMD_INQUIRE_Q` - Queue discovery
- `MQCMD_INQUIRE_Q_STATUS` - Queue depth and statistics  
- `MQCMD_INQUIRE_CHANNEL` - Channel discovery
- `MQCMD_INQUIRE_CHANNEL_STATUS` - Channel status and throughput
- `MQCMD_INQUIRE_TOPIC_STATUS` - Topic activity (when enabled)

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
- **queues**: Per-queue metrics with `queue` label
- **channels**: Per-channel metrics with `channel` label  
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