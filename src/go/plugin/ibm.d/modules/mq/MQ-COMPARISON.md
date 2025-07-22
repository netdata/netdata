# IBM MQ Monitoring Solutions: DevOps Evaluation Guide

This guide evaluates IBM MQ monitoring capabilities across major monitoring platforms from a DevOps perspective. The analysis focuses on operational requirements, technology choices, and practical deployment considerations.

## Monitoring Solutions Evaluated

1. **Netdata** - Real-time performance monitoring
2. **Datadog** - Cloud-based infrastructure monitoring
3. **Dynatrace** - Application performance management
4. **Splunk** - Data platform with monitoring capabilities
5. **Grafana** - Visualization platform (using Prometheus exporter)
6. **Zabbix** - Open source enterprise monitoring
7. **Nagios** - Infrastructure monitoring and alerting system
8. **CheckMK** - Infrastructure monitoring and alerting platform

### Solution Abbreviations
For table readability, the following abbreviations are used:
- **ND** = Netdata
- **DD** = Datadog
- **DT** = Dynatrace
- **SP** = Splunk
- **GR** = Grafana
- **ZB** = Zabbix
- **NG** = Nagios
- **CM** = CheckMK

## Queue Manager Level Monitoring

### Basic Operational Status
**Cardinality**: 1 metric set per queue manager
**Technology**: PCF MQCMD_INQUIRE_Q_MGR or process monitoring
**Authority Required**: +connect +inq on queue manager
**Configuration**: None
**Value**: Fundamental availability monitoring

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Status | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | MQIA_Q_MGR_STATUS |
| Connection count | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | MQIA_CONNECTION_COUNT |
| Command level | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_COMMAND_LEVEL |
| Platform | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_PLATFORM |
| Uptime | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | Calculated |
| Active channels | ❌ | ✅¹ | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | From channel count |
| Active listeners | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIA_ACTIVE_LISTENERS |
| Publish to sub msgs | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |
| Expired msg count | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |

¹Via channel.channels metric

### Resource Utilization
**Cardinality**: 1 metric set per queue manager
**Technology**: $SYS topic subscriptions (Grafana/Prometheus) or REST API
**Authority Required**: +sub on $SYS topics or REST API access
**Configuration**: Monitor topics must be enabled (MONINT > 0)
**Value**: Capacity planning, performance troubleshooting

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | Collection Method |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------------|
| CPU usage | ✅* | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |
| Memory usage | ✅* | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |
| Log write latency | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |
| File system usage | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |
| Log bytes used | ✅* | ❌ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | See log utilization |
| Transaction counts | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | $SYS topics |

*Framework implemented, requires MQ 9.0+ with MONINT configured and attribute ID mapping

### Queue Manager Log Utilization
**Cardinality**: 1 metric set per queue manager
**Technology**: PCF commands
**Authority Required**: +inq on queue manager
**Configuration**: None
**Value**: Log space monitoring and capacity planning

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | Description |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------|
| Log utilization % | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | 6 metrics for log usage |

### Advanced Queue Manager Attributes
**Cardinality**: 1 metric set per queue manager
**Technology**: PCF MQCMD_INQUIRE_Q_MGR
**Authority Required**: +connect +inq on queue manager
**Configuration**: None
**Value**: Configuration tracking and compliance

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Distribution lists | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_DIST_LISTS |
| Max message length | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_MAX_MSG_LENGTH |
| Max channels | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | MQIA_MAX_CHANNELS |
| Active channels | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | MQIA_ACTIVE_CHANNELS |

## Queue Level Monitoring

### Queue Configuration Attributes
**Cardinality**: 1 metric set per queue (potentially thousands)
**Technology**: PCF MQCMD_INQUIRE_Q
**Authority Required**: +inq on queues
**Configuration**: None
**Value**: Configuration compliance, capacity planning

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Max depth | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | MQIA_MAX_Q_DEPTH |
| Inhibit get/put | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_INHIBIT_GET/PUT |
| Backout threshold | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_BACKOUT_THRESHOLD |
| Trigger settings | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_TRIGGER_* |
| Service interval | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | MQIA_Q_SERVICE_INTERVAL |
| Input open option | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_DEF_INPUT_OPEN_OPTION |
| Depth event config | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_Q_DEPTH_*_EVENT |

### Queue Runtime Status
**Cardinality**: 1 metric set per queue
**Technology**: PCF MQCMD_INQUIRE_Q_STATUS
**Authority Required**: +inq on queues
**Configuration**: None
**Value**: Real-time operational monitoring

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Current depth | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | MQIA_CURRENT_Q_DEPTH |
| Depth percentage | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Calculated |
| Open input count | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | MQIA_OPEN_INPUT_COUNT |
| Open output count | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | MQIA_OPEN_OUTPUT_COUNT |
| Oldest message age | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | MQIACF_OLDEST_MSG_AGE |
| Queue file size | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | MQIACF_CUR_Q_FILE_SIZE² |
| Uncommitted msgs | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | MQIACF_UNCOMMITTED_MSGS |
| Last GET time | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ | ❌ | ✅ | Time since last GET |
| Last PUT time | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ | ❌ | ✅ | Time since last PUT |

²Requires MQ 9.1.5+

### Queue Activity Rates (Destructive Operation)
**Cardinality**: 1 metric set per queue
**Technology**: PCF MQCMD_RESET_Q_STATS
**Authority Required**: +chg on queues
**Configuration**: MONQ(MEDIUM) or MONQ(HIGH)
**Value**: Throughput monitoring
**Warning**: Resets counters after reading

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Enqueue rate | ✅³ | ✅³ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | MQIA_MSG_ENQ_COUNT |
| Dequeue rate | ✅³ | ✅³ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ | MQIA_MSG_DEQ_COUNT |
| High depth | ✅³ | ✅³ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | MQIA_HIGH_Q_DEPTH |

³Disabled by default due to destructive nature

### Queue MQI Statistics (Non-Intrusive)
**Cardinality**: 1 metric set per queue
**Technology**: SYSTEM.ADMIN.STATISTICS.QUEUE subscription
**Authority Required**: +get on SYSTEM.ADMIN.STATISTICS.QUEUE
**Configuration**: STATQ(ON) on queue manager
**Value**: Detailed application behavior analysis

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | Description |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------|
| Get operations | ✅ | ✅⁴ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQGET count/bytes |
| Put operations | ✅ | ✅⁴ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQPUT count/bytes |
| Put1 operations | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQPUT1 count |
| Get/Put failures | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Failed operations |
| Browse operations | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Browse count/bytes |
| Open/Close ops | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQOPEN/MQCLOSE |
| Expired messages | ✅ | ✅⁴ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Expiration count |
| Purged messages | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Purge count |
| Non-queued msgs | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Direct transfers |
| Min/Max depth | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Queue depth range |
| Avg queue time | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | Message latency |

⁴Optional feature, disabled by default (collect_statistics_metrics)

### Dead Letter Queue Handling
**Cardinality**: Backout attributes per queue
**Technology**: MQCMD_INQUIRE_Q (configuration attributes)
**Authority Required**: +inq on queues
**Configuration**: Backout requeue name must be configured
**Value**: Message poison prevention and failure handling
**Note**: No solution provides special DLQ monitoring beyond standard queue metrics

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|----|----|----|----|----|----|----|----|--------------|
| Backout threshold | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_BACKOUT_THRESHOLD |
| Harden get backout | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIA_HARDEN_GET_BACKOUT |
| MQDLH parsing | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Reason code extraction |
| DLQ auto-detection | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Special queue handling |
| Source queue tracking | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | From MQDLH header |

## Channel Level Monitoring

### Channel Configuration
**Cardinality**: 1 metric set per channel (tens to hundreds)
**Technology**: PCF MQCMD_INQUIRE_CHANNEL
**Authority Required**: +inq on channels
**Configuration**: None
**Value**: Configuration compliance and tuning

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Batch size | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_BATCH_SIZE |
| Batch interval | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_BATCH_INTERVAL |
| Heartbeat interval | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_HB_INTERVAL |
| Max message length | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_MAX_MSG_LENGTH |
| Retry settings | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_*_RETRY |
| NPM speed | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_NPM_SPEED |
| Sharing conversations | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_SHARING_CONVERSATIONS |

### Channel Runtime Status
**Cardinality**: 1 metric set per running channel
**Technology**: PCF MQCMD_INQUIRE_CHANNEL_STATUS
**Authority Required**: +inq on channels
**Configuration**: None for basic; MONCHL(MEDIUM/HIGH) for detailed
**Value**: Connection health and performance monitoring

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Status | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | MQIACH_CHANNEL_STATUS |
| Status summary | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | Aggregated by status |
| Messages | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | MQIACH_MSGS |
| Bytes sent/rcvd | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | MQIACH_BYTES_* |
| Batches | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIACH_BATCHES |
| Current convs | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | Sharing conversations |
| SSL key resets | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_SSL_KEY_RESETS |
| Connection status | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_CONNS |
| Active connections | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Channel instances |
| Channel instances | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | 3 instance metrics |
| In-doubt status | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | MQIACH_INDOUBT_STATUS |
| XMITQ time short | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIACH_XMITQ_TIME_SHORT |
| XMITQ time long | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIACH_XMITQ_TIME_LONG |
| Network time | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | Channel timing metrics |
| Exit time | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | Channel timing metrics |
| Total time | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | Channel timing metrics |

### Channel Statistics (Non-Intrusive)
**Cardinality**: 1 metric set per channel
**Technology**: SYSTEM.ADMIN.STATISTICS.QUEUE subscription
**Authority Required**: +get on SYSTEM.ADMIN.STATISTICS.QUEUE
**Configuration**: STATQ(ON) and STATCHL(MEDIUM/HIGH)
**Value**: Detailed performance analysis

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | Description |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------|
| Messages/Bytes | ✅ | ✅⁴ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Transfer metrics |
| Put retries | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Retry counts |
| Batch metrics | ✅ | ✅⁴ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | Full/partial batches |

⁴Optional feature, disabled by default

## Topic and Subscription Monitoring

### Topic Status
**Cardinality**: 1 metric set per topic
**Technology**: PCF MQCMD_INQUIRE_TOPIC_STATUS
**Authority Required**: +inq on topics
**Configuration**: Pub/Sub enabled
**Value**: Pub/Sub health monitoring

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Publisher count | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIA_PUB_COUNT |
| Subscriber count | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIA_SUB_COUNT |
| Published msgs | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Via status |

⚠️ Documentation claims support but no implementation found

### Subscription Status
**Cardinality**: 1 metric set per subscription (can be very high)
**Technology**: PCF MQCMD_INQUIRE_SUB_STATUS
**Authority Required**: +inq on subscriptions
**Configuration**: Pub/Sub enabled
**Value**: Subscription health and message backlog monitoring

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | PCF Constant |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|--------------|
| Message count | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQIACF_MESSAGE_COUNT |
| Last message time | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | MQCACF_LAST_MSG_TIME |

⚠️ Documentation claims support but no implementation found

## Event Monitoring

### Event Queue Subscriptions
**Cardinality**: Events as they occur
**Technology**: Subscribe to SYSTEM.ADMIN.*.EVENT queues
**Authority Required**: +get on event queues
**Configuration**: Enable specific event types on queue manager
**Value**: Real-time alerting for security and operational events

| Event Type | ND | DD | DT | SP | GR | ZB | NG | CM | Event Queue |
|------------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------|
| Authority failures | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | QMGR.EVENT |
| Queue depth events | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | QMGR.EVENT |
| Channel events | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | CHANNEL.EVENT |
| Performance events | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | PERFM.EVENT |

## Listener Monitoring

### Listener Status
**Cardinality**: 1 metric set per listener (typically 1-5)
**Technology**: PCF MQCMD_INQUIRE_LISTENER_STATUS
**Authority Required**: +inq on listeners
**Configuration**: None
**Value**: Network endpoint availability

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|
| Status | ✅ | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Port/IP | ✅ | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| Backlog | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

## z/OS Specific Monitoring

### z/OS Usage Metrics
**Cardinality**: 1 metric set per queue manager
**Technology**: z/OS specific PCF commands
**Authority Required**: +inq on queue manager
**Configuration**: z/OS platform only
**Value**: z/OS resource usage and performance

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM | Description |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------|
| z/OS CPU usage | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | 8 z/OS metrics |
| z/OS memory usage | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | z/OS specific |
| z/OS paging | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | z/OS specific |

## Cluster Monitoring

### Cluster Queue Manager Status
**Cardinality**: 1 metric set per cluster queue manager
**Technology**: PCF MQCMD_INQUIRE_CLUSTER_Q_MGR
**Authority Required**: +inq on cluster
**Configuration**: Cluster must be configured
**Value**: Cluster health and topology monitoring

| Metric | ND | DD | DT | SP | GR | ZB | NG | CM |
|--------|---------|---------|-----------|--------|---------|--------|--------|---------|
| Cluster suspend state | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| Cluster QM status | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| Cluster QM availability | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |

## Auto-Discovery and Configuration Management

### MQ Server Discovery and Instance Management
**Technology**: Various (network discovery, configuration files, API calls)  
**Value**: Automated setup and dynamic environment adaptation

| Capability | ND | DD | DT | SP | GR | ZB | NG | CM | Description |
|------------|----|----|----|----|----|----|----|----|-------------|
| Auto-discover MQ servers | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | Network discovery of MQ installations |
| Monitor all auto-discovered Queue Managers | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Monitor all QMs found via discovery |
| Monitor specific Queue Managers | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Target specific QM instances |
| Monitor all auto-discovered Queues | ✅ | ✅⁶ | ❌ | ✅ | ✅ | ✅⁷ | ❌ | ✅ | Auto-discover and monitor all queues |
| Monitor specific Queues | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Target specific queue instances |
| Monitor all auto-discovered Channels | ✅ | ✅⁶ | ❌ | ✅ | ✅ | ✅⁷ | ❌ | ✅ | Auto-discover and monitor all channels |
| Monitor specific Channels | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Target specific channel instances |
| Monitor all auto-discovered Listeners | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Auto-discover and monitor all listeners |
| Monitor specific Listeners | ✅ | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | Target specific listener instances |
| Monitor all auto-discovered Topics | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Auto-discover and monitor all topics |
| Monitor specific Topics | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Target specific topic instances |
| Monitor all auto-discovered Subscriptions | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Auto-discover and monitor all subscriptions |
| Monitor specific Subscriptions | ✅ | ⚠️ | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | Target specific subscription instances |

⁶Datadog: `auto_discover_queues` with `queue_patterns`/`queue_regex` filtering  
⁷Zabbix: Custom auto-discovery templates (e.g., spectroman/ibm-mq-monitoring)

### Configuration Management Approaches

**Netdata**: Network discovery of MQ servers, but requires explicit configuration per queue manager. Supports regex-based filtering for all object types within configured queue managers.

**Datadog**: Manual server configuration, automatic queue/channel discovery with `auto_discover_queues`, `queue_patterns`, and `queue_regex` for performance optimization. No topic/subscription auto-discovery.

**Dynatrace**: Automatic IBM MQ process detection via OneAgent, bulk inquiry options, regex-based queue manager filtering. Manual configuration for queue/channel specifics.

**Splunk**: Manual configuration with IBM WebSphere MQ Modular Input. Supports multiple queues/channels per data input with regex filtering.

**Grafana/Prometheus**: Manual server configuration, regex-based filtering for `<queues>`, `<channels>`, `<topics>`, `<subscriptions>` in mq_prometheus exporter configuration.

**Zabbix**: Manual setup with third-party auto-discovery templates (spectroman/ibm-mq-monitoring) that dynamically create templates for discovered queue managers and objects.

**Nagios**: Manual configuration per check, supports multiple queues per check with configuration files. Advanced plugins support regex filtering for queue discovery.

**CheckMK**: Uses `dspmq` command output for queue manager discovery. Manual configuration for queue-specific monitoring. Limited auto-discovery capabilities.

## Collection Characteristics

### Update Frequency
| Solution | Minimum | Typical | Maximum | Architecture |
|----------|---------|---------|---------|--------------|
| ND | 1s | 1s | 60s | Edge (per-node) |
| DD | 15s | 15-30s | 300s | Centralized agent |
| DT | 60s | 60s | 300s | Centralized agent |
| SP | 10s | 60s | 300s | Centralized |
| GR | 15s | 30-60s | 300s | Pull-based |
| ZB | 30s | 60-300s | 3600s | Centralized |
| NG | 60s | 300s | 3600s | Centralized |
| CM | 60s | 60-300s | 3600s | Centralized |

### Platform Support
| Solution | Collector Platform | Remote Support | Architecture |
|----------|-------------------|----------------|--------------|
| ND | Linux, Windows, macOS | ✅ Client mode | CGO-based |
| DD | Linux, Windows, macOS | ✅ Client mode | Python pymqi |
| DT | Cross-platform | ✅ OneAgent | Proprietary |
| SP | Cross-platform | ✅ Client mode | Java-based |
| GR | Linux, Windows, AIX | ✅ Client mode | Go client |
| ZB | Linux only | ❌ Local only | Shell scripts |
| NG | Linux, Unix, Windows | ✅ Client mode | Shell/Perl scripts |
| CM | Linux, Solaris, AIX, HPUX | ✅ Client mode | password-less root ssh |

## Key Technical Differentiators

**Netdata**: Highest resolution (1s), complete MQI statistics implementation, Linux-only collector

**Datadog**: Python-based flexibility, endianness handling for AIX/IBM i, significant gap between documentation and implementation

**Dynatrace**: Commercial APM integration, z/OS specific metrics, channel timing analysis, topology mapping for transaction tracing

**Splunk**: Event queue monitoring capability, Java-based cross-platform support

**Grafana/Prometheus**: $SYS topic subscriptions for resource metrics, multiple export formats, comprehensive z/OS support, cluster monitoring

**Zabbix**: Local monitoring only, requires sudo access, minimal metric coverage

**Nagios**: Basic health checks focused on availability monitoring, simple alerting model with command-based checks. Supports queue manager status (qmmon), queue depth (qdmon), channel status (chlmon), message age (mamon), port monitoring (portmon/portcon), process monitoring (brkmon/cmdmon), and log monitoring (fdcmon/refmon) functions

**CheckMK**: Command-line tool based monitoring using native MQ utilities (dspmq, runmqsc). Supports queue manager availability monitoring, basic queue metrics (depth, message age, last GET/PUT times, open counts, queue time), and channel status monitoring with CRITICAL alerts for STOPPED channels. Limited to metrics available through standard MQ command-line tools.

## Service Health Checks

**Technology**: Various (connection tests, PCF queries, process checks)
**Value**: Proactive alerting and availability monitoring

| Check Type | ND | DD | DT | SP | GR | ZB | NG | CM | Description |
|------------|---------|---------|-----------|--------|---------|--------|--------|---------|-------------|
| Connection check | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Can connect to QM |
| Queue manager check | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | QM responding |
| Queue availability | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Queue accessible |
| Channel health | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Channel status check |
| Channel status alerts | ❌ | ✅⁵ | ✅ | ✅ | ✅ | ❌ | ✅ | ✅ | Status-based alerts |

⁵Datadog provides specific service checks with WARNING/CRITICAL based on channel state

## Operational Considerations

1. **Authority Requirements**: All solutions require similar MQ authorities. Event monitoring requires additional permissions. Datadog requires +chg for reset statistics.

2. **Configuration Impact**: Statistics collection (STATQ) has minimal performance impact. Reset operations are destructive.

3. **Cardinality Management**: Queue and subscription metrics can create high cardinality. Plan retention accordingly.

4. **Platform Limitations**: Only Dynatrace, Splunk, and Grafana offer true cross-platform collectors.

5. **Resolution vs Volume**: Higher resolution (Netdata) provides better anomaly detection but increases storage requirements.

6. **Endianness**: Datadog provides convert_endianness option for AIX/IBM i platforms.

7. **$SYS Topics**: Grafana/Prometheus requires MONINT > 0 on queue manager to enable $SYS topic publications for resource metrics.
