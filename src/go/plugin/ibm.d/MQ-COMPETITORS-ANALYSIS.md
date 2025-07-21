# IBM MQ Monitoring: Netdata vs Competitors Analysis

This document compares our mq_pcf collector with leading IBM MQ monitoring solutions.

## Current Implementation Status

**As of latest updates:**
- **Total Metrics**: 1043+ time-series for mq-test instance
- **Collection Pattern**: Efficient 3N batching (3 requests per queue)
- **New Features Implemented**:
  - ✅ OldestMessageAge metric for all queues
  - ✅ UncommittedMessages metric tracking in-flight transactions
  - ✅ LastActivity metrics showing time since last GET/PUT operations
  - ✅ Split configuration contexts for better organization
  - ✅ AttributeValue type with NotCollected sentinel
  - ✅ Per-attribute validity tracking (no fake data)
  - ✅ Extended to channels for consistent handling
  - ✅ Type-safe metric collection API
  - ✅ Queue Manager Connection Count metric
  - ✅ Queue Manager Uptime metric
  - ✅ Enhanced Listener monitoring (status, backlog, uptime)
  - ✅ Transparency pattern for all collectors
  - ✅ Shared time parsing utilities
  - ✅ **NEW: Depth Percentage calculation**
  - ✅ **NEW: Average Queue Time (MQIACF_Q_TIME_INDICATOR)**
  - ✅ **NEW: Service Interval and Retention Interval metrics**
  - ✅ **NEW: Message Persistence configuration metric**
  - ✅ **NEW: Additional queue attributes collected (scope, usage, etc.)**
  - ✅ **NEW: Time since last message metric for topics**
  - ✅ **NEW: MQI statistics collection for comprehensive monitoring**
  - ✅ **NEW: Reorganized MQI statistics under queues family**
  - ✅ **NEW: Improved PCF command logging with descriptive names**
  - ✅ **NEW: Generic patterns for maintainable PCF protocol code**
  
**Metric Breakdown**:
- **Queue Metrics**: 30+ contexts organized into 6 families:
  - **queues/activity**: depth, depth_percentage, messages, connections, high_depth, uncommitted_msgs, last_activity
  - **queues/performance**: oldest_msg_age, avg_queue_time, service_interval
  - **queues/configuration**: inhibit_status, priority, message_persistence, retention_interval
  - **queues/limits**: triggers, backout_threshold, max_msg_length
  - **queues/behavior**: queue_scope, queue_usage, msg_delivery_sequence, harden_get_backout
  - **queues/statistics**: MQI operation statistics (get/put/browse bytes, failed ops, expired msgs, purges)
- **Channel Metrics**: 3 contexts (status, messages, bytes) with batch configuration
- **Topic Metrics**: 4 contexts (publishers, subscribers, messages, time_since_last_message)
- **Queue Manager**: Status, connection count, uptime, and overview metrics
- **Listener Metrics**: 3 contexts (status, backlog, uptime) with IP/port labels
- **Resolution**: Per-second (1s) - unmatched by competitors

## Competitors Analyzed
1. **Prometheus IBM MQ Exporter** (used with Grafana)
2. **Dynatrace IBM MQ Extension 2.0**
3. **Datadog IBM MQ Integration**
4. **Zabbix/Spectroman IBM MQ Monitoring** (open source)
5. **SignalFx/OpenTelemetry IBM MQ Monitoring** (Splunk)

## Key Differentiator: Resolution

| Solution | Collection Resolution | Notes |
|----------|---------------------|-------|
| **Netdata** | **Per-second (1s)** | Real-time, high-resolution monitoring |
| Prometheus | 15s-60s typical | Configurable, but higher resolution impacts storage |
| Dynatrace | 1-5 minutes | Varies by metric type |
| Datadog | 15s minimum | Higher resolutions cost extra |
| Zabbix | 30s-5m typical | Configurable per item |
| SignalFx/OTel | 60s typical | Configurable, uses OpenTelemetry |

**Netdata's per-second resolution enables detection of transient issues that other solutions miss.**

## Legend
- ✅ = Implemented and verified
- ❌ = Not implemented
- ❓ = Unknown/unclear from documentation
- ⚠️ = Claimed in documentation but NOT found in source code (fake metric)

## General Collection Requirements

### PCF (Programmable Command Format)
Most solutions use PCF for metrics collection, which requires:
- **Command Queue**: Access to SYSTEM.ADMIN.COMMAND.QUEUE
- **Reply Queue**: SYSTEM.DEFAULT.MODEL.QUEUE or custom reply queue
- **Authentication**: Valid MQ user credentials
- **Network**: Port 1414 (default) or custom channel port

### Minimum Permissions
- **+connect** to queue manager
- **+inq** (inquire) on objects being monitored
- **+put** on SYSTEM.ADMIN.COMMAND.QUEUE
- **+get** on reply queue (usually dynamic)
- **+chg** (change) for RESET_Q_STATS command

### Monitoring Levels
Many metrics require specific monitoring levels set on MQ objects:
- **MONQ(MEDIUM/HIGH)**: For queue statistics
- **MONCHL(MEDIUM/HIGH)**: For channel statistics
- **MONACLS**: For accounting and statistics

### Alternative Methods
- **REST API**: MQ 9.0.4+ provides REST interface (requires different setup)
- **runmqsc**: Command-line interface (used by Zabbix)
- **JMX**: Java Management Extensions (alternative approach)

## Metrics Comparison

### Queue Manager Level Metrics
**Cardinality**: Per-Queue Manager (1 per queue manager)  
**Collection Requirements**:
- **PCF Access**: MQCMD_INQUIRE_Q_MGR command
- **Permissions**: +inq authority on queue manager
- **Transport**: Client or Bindings mode
- **Special Requirements**: Resource metrics (CPU/Memory) require MQ Resource Monitoring REST API (v9.0.4+)

| Metric | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|--------|---------|------------|-----------|---------|---------|----------|-------|
| Status (running/stopped) | ✅ | ✅ | ✅ | ✅* | ✅ | ✅ | Basic operational status |
| Connection count | ✅ | ✅ | ✅ | ⚠️ | ✅ | ✅ | Current connections |
| Active listeners | ✅ | ✅ | ✅ | ⚠️ | ❌ | ✅ | Listener status |
| Uptime | ✅ | ✅ | ✅ | ⚠️ | ❌ | ❌ | QMgr uptime |
| CPU usage | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Requires resource monitoring |
| Memory usage | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Requires resource monitoring |
| Log usage/performance | ❌ | ✅ | ❓ | ⚠️ | ✅ | ✅ | Transaction log metrics |
| File system usage | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Disk space metrics |
| Commit/rollback counts | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Transaction metrics |
| Expired message count | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Message expiry tracking |
| Pub/sub throughput | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Published bytes/messages |
| Distribution lists | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | QMgr dist list support |
| Max message list | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Max msg length on QMgr |

### Queue Level Metrics
**Cardinality**: Per-Queue (potentially hundreds or thousands)  
**Collection Requirements**:
- **PCF Access**: MQCMD_INQUIRE_Q and MQCMD_INQUIRE_Q_STATUS commands
- **Permissions**: +inq authority on queues (or SYSTEM.ADMIN.COMMAND.QUEUE)
- **Queue Statistics**: Many metrics require MONQ(MEDIUM) or MONQ(HIGH) on queues
- **Reset Statistics**: Enqueue/dequeue rates require MQCMD_RESET_Q_STATS (+chg authority)
- **Transport**: Client or Bindings mode

| Metric | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|--------|---------|------------|-----------|---------|---------|----------|-------|
| Current depth | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Messages in queue (MQIA_CURRENT_Q_DEPTH) |
| Depth percentage | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | % of max depth (calculated) |
| Max depth | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Configuration limit (MQIA_MAX_Q_DEPTH) |
| Enqueue count/rate | ✅³ | ✅ | ✅* | ✅* | ❌ | ✅ | Via RESET_Q_STATS (MQIA_MSG_ENQ_COUNT) |
| Dequeue count/rate | ✅³ | ✅ | ✅* | ✅* | ❌ | ✅ | Via RESET_Q_STATS (MQIA_MSG_DEQ_COUNT) |
| Open input/output count | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Active connections (MQIA_OPEN_INPUT/OUTPUT_COUNT) |
| Oldest message age | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Queue time tracking (MQIACF_OLDEST_MSG_AGE) |
| Average queue time | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | MQIACF_Q_TIME_INDICATOR |
| Last GET/PUT time | ✅ | ❓ | ✅ | ✅ | ✅ | ❌ | As "time since" metric (MQCACF_LAST_GET/PUT_DATE/TIME) |
| Expired messages | ✅² | ✅ | ❓ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| Get/Put bytes | ✅² | ✅ | ❓ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| Browse count/bytes | ✅² | ❌ | ❌ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| Get/Put fail counts | ✅² | ❌ | ❌ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| Non-queued messages | ✅² | ❌ | ❌ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| Purge count | ✅² | ❌ | ❌ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| MQOPEN/CLOSE/INQ/SET counts | ❌² | ✅ | ❓ | ❌ | ❌ | ❌ | Via statistics queue |
| Short/Long time indicators | ❌² | ❓ | ✅ | ❌ | ❌ | ✅ | Via statistics queue |
| High/Low depth events | ✅ | ❓ | ✅ | ✅ | ❌ | ✅ | Threshold monitoring |
| Inhibit status | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Get/Put inhibited |
| Backout threshold | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Poison msg handling |
| Trigger settings | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Depth, type config |
| Service interval | ✅ | ❌ | ❌ | ✅ | ❌ | ✅ | MQIA_Q_SERVICE_INTERVAL |
| Retention interval | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MQIA_RETENTION_INTERVAL |
| High queue depth (stats) | ✅ | ❌ | ❌ | ✅ | ❌ | ✅ | Peak depth via RESET |
| Min/Max depth (stats) | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Via STATISTICS.QUEUE |
| Time since reset | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Stats window tracking |
| Uncommitted messages | ✅ | ❌ | ❌ | ✅ | ✅ | ✅ | Transaction pending |
| Harden get backout | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MQIA_HARDEN_GET_BACKOUT |
| Message delivery sequence | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MQIA_MSG_DELIVERY_SEQUENCE |
| Queue scope | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MQIA_SCOPE |
| Queue type | ✅ | ❓ | ❓ | ✅ | ✅ | ✅ | Local/Alias/Remote/etc |
| Queue usage | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MQIA_USAGE |
| Default persistence | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MQIA_DEF_PERSISTENCE |
| Default priority | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Message priority |
| Queue file size | ❌⁴ | ❌ | ❌ | ❌ | ❌ | ✅ | Filesystem metric |
| Max queue file size | ❌⁴ | ❌ | ❌ | ❌ | ❌ | ✅ | Filesystem metric |

**Legend:**
- ❌¹ = Available via PCF, not yet implemented (easy to add)
- ❌² = Requires SYSTEM.ADMIN.STATISTICS.QUEUE consumer (different architecture)
- ✅³ = Implemented but disabled by default (destructive operation - user must enable)
- ❌⁴ = Not available via MQ APIs (filesystem metrics)

### Channel Level Metrics
**Cardinality**: Per-Channel (typically tens to hundreds)  
**Collection Requirements**:
- **PCF Access**: MQCMD_INQUIRE_CHANNEL and MQCMD_INQUIRE_CHANNEL_STATUS commands
- **Permissions**: +inq authority on channels
- **Channel Statistics**: Requires MONCHL(MEDIUM) or MONCHL(HIGH) on channels
- **Transport**: Client or Bindings mode
- **Active Channels**: Some metrics only available for running channels

| Metric | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|--------|---------|------------|-----------|---------|---------|----------|-------|
| Status | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | All status states (inactive/binding/starting/running/stopping/retrying/stopped/requesting/paused/disconnected/initializing/switching) |
| Connection status | ❌¹ | ❌ | ❌ | ✅ | ❌ | ❌ | Connection state |
| Active connections | ❌¹ | ❌ | ❌ | ❌ | ❌ | ❌ | Per channel instances |
| Message rate | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | Messages/second (MQIACH_MSGS) |
| Byte rate | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | Bytes/second (MQIACH_BYTES_SENT) |
| Buffers sent/received | ❌¹ | ❌ | ❌ | ✅ | ❌ | ✅ | Buffer counts (MQIACH_BUFFERS_SENT/RCVD) |
| Batch metrics | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Batch size/rate (MQIACH_BATCHES) |
| Full/incomplete batches | ❌² | ❌ | ❌ | ✅ | ❌ | ❌ | Batch efficiency (via stats queue) |
| Average batch size | ❌² | ❌ | ❌ | ✅ | ❌ | ❌ | Batch statistics (via stats queue) |
| Put retries | ❌² | ❌ | ❌ | ✅ | ❌ | ✅ | Message retry count (via stats queue) |
| Current messages | ❌¹ | ❌ | ❌ | ✅ | ❌ | ❌ | In-doubt messages (MQIACH_CURRENT_MSGS) |
| Configuration | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Timeouts, retries, batch size/interval |
| XMITQ time | ❌¹ | ✅ | ❓ | ❌ | ❌ | ❌ | Transmission queue wait |
| MCA status | ❌¹ | ❌ | ❌ | ✅ | ❌ | ❌ | Agent status (MQIACH_MCA_STATUS) |
| In-doubt status | ❌¹ | ❌ | ❌ | ✅ | ❌ | ❌ | Transaction state (MQIACH_INDOUBT_STATUS) |
| SSL key resets | ❌¹ | ❌ | ❌ | ✅ | ❌ | ❌ | Security resets (MQIACH_SSL_KEY_RESETS) |
| NPM speed | ❌¹ | ❌ | ❌ | ✅ | ❌ | ❌ | Non-persistent msg speed (MQIACH_NPM_SPEED) |
| Message retry settings | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | MR count/interval (MQIACH_MR_COUNT/INTERVAL) |
| Max sharing conversations | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ | Max conversations (MQIACH_SHARING_CONVERSATIONS) |
| Current sharing conversations | ❌¹ | ❌ | ❌ | ❌ | ❌ | ✅ | Current conversations |

### Topic Level Metrics
**Cardinality**: Per-Topic (variable, typically less than queues)  
**Collection Requirements**:
- **PCF Access**: MQCMD_INQUIRE_TOPIC and MQCMD_INQUIRE_TOPIC_STATUS commands
- **Permissions**: +inq authority on topics
- **Transport**: Client or Bindings mode
- **Pub/Sub**: Requires publish/subscribe to be enabled

| Metric | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|--------|---------|------------|-----------|---------|---------|----------|-------|
| Publisher count | ✅ | ✅ | ✅ | ⚠️ | ❌ | ✅ | Active publishers |
| Subscriber count | ✅ | ✅ | ✅ | ⚠️ | ❌ | ✅ | Active subscribers |
| Message count | ✅ | ✅ | ✅ | ⚠️ | ❌ | ✅ | Published messages |
| Time since last message | ✅ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Activity tracking |

### Subscription Level Metrics
**Cardinality**: Per-Subscription (potentially very high)  
**Collection Requirements**:
- **PCF Access**: MQCMD_INQUIRE_SUB and MQCMD_INQUIRE_SUB_STATUS commands
- **Permissions**: +inq authority on subscriptions
- **Transport**: Client or Bindings mode
- **Note**: Can have very high cardinality in pub/sub heavy environments

| Metric | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|--------|---------|------------|-----------|---------|---------|----------|-------|
| Message backlog | ❌ | ✅ | ❓ | ❌ | ❌ | ❌ | Undelivered messages |
| Last message time | ❌ | ✅ | ❓ | ❌ | ❌ | ❌ | Last delivery timestamp |
| Connection status | ❌ | ✅ | ❓ | ❌ | ❌ | ❌ | Active/inactive |

### Listener Level Metrics
**Cardinality**: Per-Listener (typically 1-5 per queue manager)  
**Collection Requirements**:
- **PCF Access**: MQCMD_INQUIRE_LISTENER and MQCMD_INQUIRE_LISTENER_STATUS commands
- **Permissions**: +inq authority on listeners
- **Transport**: Client or Bindings mode

| Metric | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|--------|---------|------------|-----------|---------|---------|----------|-------|
| Status | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | All 5 states: stopped/starting/running/stopping/retrying |
| Port | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ | Listening port (as label) |
| Backlog | ✅ | ❓ | ❓ | ❌ | ❌ | ❌ | Connection backlog limit |
| IP Address | ✅ | ❓ | ❓ | ❌ | ❌ | ❌ | Bind address (as label) |
| Uptime | ✅ | ❓ | ❓ | ❌ | ❌ | ❌ | Time since listener started |

### Platform & Features

| Feature | Netdata | Prometheus | Dynatrace | Datadog | Zabbix | SignalFx | Notes |
|---------|---------|------------|-----------|---------|---------|----------|-------|
| **Platforms** |
| Linux | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | |
| Windows | remotely | ✅ | ✅ | ✅ | remotely | ✅ | |
| MacOS | remotely | ❓ | ❓ | ✅ | remotely | ❓ | |
| AIX | remotely | ✅ | ✅ | ❌ | remotely | ✅ | |
| z/OS | remotely | ❓ | ✅ | ❌ | remotely | ❓ | |
| **Deployment** |
| Local monitoring | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | On MQ host |
| Remote monitoring | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | From remote host |
| Auto-discovery | ✅ | ❓ | ✅ | ✅ | ✅ | ❌ | Find queue managers |
| Queue auto-discovery | ✅ | ❓ | ✅ | ✅ | ✅ | ✅ | Dynamic queue discovery |
| **Advanced Features** |
| Credential vault | ❌ | ❓ | ✅ | ✅ | ❌ | ❌ | Secure credential storage |
| Transaction tracing | ❌ | ❓ | ✅ | ❌ | ❌ | ❌ | Message flow tracking |
| Bulk collection | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Performance optimization |
| DLQ event generation | ❌ | ❓ | ✅ | ❌ | ❌ | ❌ | Dead letter queue alerts |
| Queue manager aliasing | ❌ | ❓ | ✅ | ❌ | ❌ | ❌ | Rename for clarity |
| Listener monitoring | ❌ | ❓ | ✅ | ❌ | ❌ | ✅ | Connection endpoints |
| Queue monitoring levels | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Requires MONQ(MEDIUM) |
| Convert endianness | ✅ | ❓ | ❓ | ✅ | ❌ | ❓ | Automatic via MQ library |
| Service checks | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Connection health |
| Event queue monitoring | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | Auth/config events |
| SSL/TLS support | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | Encrypted connections |

## Key Findings

### SignalFx/OpenTelemetry's Approach
1. **Java-Based with PCF**
   - Uses IBM MQ Java client libraries
   - PCF-based collection (like Netdata)
   - Requires Java 11+ runtime
   
2. **OpenTelemetry Integration**
   - Native OTEL metric format
   - Standard observability pipeline
   - Event queue monitoring for authority/config events
   
3. **Comprehensive Metrics**
   - Queue time metrics (oldest message age, onqtime)
   - Queue file size monitoring
   - Event-based metrics (queue depth events, auth failures)
   - Listener status monitoring
   - Detailed channel metrics including buffers
   
4. **Enterprise Features**
   - SSL/TLS support with cipher suite configuration
   - Binding and Client transport modes
   - Remote monitoring capability
   - Event queue processing

### Zabbix/Spectroman's Approach
1. **Shell Script Based**
   - Uses runmqsc commands wrapped in bash scripts
   - Direct MQ console access via sudo
   - Flexible but requires elevated privileges
   
2. **Dynamic Template Creation**
   - Auto-creates Zabbix templates per queue manager
   - Uses Zabbix API for dynamic configuration
   - Queue and channel discovery via LLD
   
3. **Basic Metrics Coverage**
   - Queue depth monitoring (CURDEPTH)
   - Queue status (QSTATUS)
   - Channel status monitoring
   - Error log monitoring
   - Connection counting
   
4. **Limitations**
   - No remote monitoring capability
   - Limited metric coverage compared to PCF
   - Requires sudo access for mqm user
   - Lower resolution (30s-5m typical)

### Datadog's Unique Strengths (Verified from Source Code)
1. **Extensive Queue Metrics**
   - Most comprehensive queue statistics (browse, fail counts, purge, etc.)
   - Detailed configuration metrics (trigger, service interval, retention)
   - Queue performance statistics (min/max depth, time since reset)
   - Uses MQCMD_INQUIRE_Q, MQCMD_INQUIRE_Q_STATUS, and optionally MQCMD_RESET_Q_STATS

2. **Channel Statistics**
   - Detailed batch efficiency metrics
   - Buffer-level statistics  
   - Connection and retry metrics
   - Optional connection-level metrics (high cardinality)

3. **Platform Features**
   - Explicit endianness conversion option (`convert_endianness: true` for AIX/IBM i)
   - Process matching for failover scenarios
   - Auto-discovery with MQ patterns, regex patterns, or both
   - Optional statistics collection from SYSTEM.ADMIN.STATISTICS.QUEUE

4. **Implementation Details**
   - Uses pymqi (Python MQ library)
   - Minimum collection interval: 15 seconds
   - Supports custom channel status mapping
   - Queue timezone handling for last_put/get_time calculations

### Netdata's Unique Advantages
1. **Per-Second Resolution**
   - Real-time monitoring unmatched by competitors
   - Detect transient spikes and issues others miss
   - Critical for high-frequency trading and real-time systems
   
2. **Distributed Architecture**
   - No central bottleneck
   - Scales with infrastructure
   - Local collection reduces network overhead

3. **Auto-Discovery via net_listeners**
   - go.d.plugin framework includes local-listeners auto-discovery
   - Can detect services by port/process patterns
   - Note: IBM MQ auto-discovery not currently configured but framework supports it

4. **Built-in Service Checks**
   - Successful metric collection indicates service health
   - No separate health check needed - if collecting, it's working

5. **Comprehensive MQI Statistics**
   - Complete MQI operation tracking (opens, closes, inquires, sets)
   - Get/Put/Browse operation bytes and counts
   - Failed operation tracking
   - Message expiry and lifecycle monitoring
   - Organized under intuitive queue families

6. **Advanced Framework Features**
   - Generic PCF patterns for maintainable code
   - Descriptive MQCMD command logging 
   - Type-safe metric collection API
   - Automatic chart lifecycle management

### Common Gaps in Netdata
1. **Queue Time Metrics** (All major competitors have these)
   - Oldest message age
   - Average queue time
   - Last GET/PUT timestamps

2. **Resource Metrics** (Prometheus has these)
   - CPU, Memory, Disk usage
   - Connection counts
   - Transaction logs
   - Requires MQ Resource Monitoring API

3. **Extended Statistics** (Datadog excels here)
   - Browse operations
   - Failed operation counts
   - Purge counts
   - Non-queued messages

### Platform Limitations
- Collector runs on Linux-only (CGO dependency)
- Can monitor MQ on ANY platform remotely (Windows, AIX, z/OS, etc.)
- Competitors can run their collectors on multiple platforms
- No MacOS support for running collector (Datadog has it)

### Missing Features vs Competition
- Credential vault (Dynatrace, Datadog)
- Bulk collection optimization (Dynatrace, Datadog)

## Implementation Recommendations

### Phase 1: PCF-Based Metrics ✅ COMPLETED
All PCF-available queue metrics have been implemented:
- ✅ Depth percentage calculation
- ✅ Average queue time (MQIACF_Q_TIME_INDICATOR)
- ✅ Service interval (MQIA_Q_SERVICE_INTERVAL)
- ✅ Retention interval (MQIA_RETENTION_INTERVAL)
- ✅ Message persistence (MQIA_DEF_PERSISTENCE)
- ✅ Queue scope (MQIA_SCOPE)
- ✅ Queue usage (MQIA_USAGE)
- ✅ Message delivery sequence (MQIA_MSG_DELIVERY_SEQUENCE)
- ✅ Harden get backout (MQIA_HARDEN_GET_BACKOUT)

### Phase 2: Extended Statistics Queue Metrics ✅ COMPLETED
Requires consuming SYSTEM.ADMIN.STATISTICS.QUEUE messages:
- ✅ Get/Put bytes (MQIAMO64_GET_BYTES, MQIAMO64_PUT_BYTES)
- ✅ Failed operation counts (MQIAMO_GETS_FAILED, MQIAMO_PUTS_FAILED)
- ✅ Browse statistics (MQIAMO_BROWSES, MQIAMO64_BROWSE_BYTES)
- ✅ Purge counts (MQIAMO_MSGS_PURGED)
- ✅ Non-queued messages (MQIAMO_MSGS_NOT_QUEUED)
- ✅ Min/Max depth statistics (MQIAMO_Q_MIN_DEPTH, MQIAMO_Q_MAX_DEPTH)
- ✅ Average queue time split by persistence (MQIAMO64_AVG_Q_TIME)
- ✅ Message lifecycle metrics (MQIAMO_MSGS_EXPIRED)
- ✅ Channel statistics (messages, bytes, batches, put retries)
- ✅ MQOPEN/CLOSE/INQ/SET counts (MQIAMO_OPENS, MQIAMO_CLOSES, etc.)
- ✅ **NEW: MQI statistics reorganized under queues family for better organization**
- ✅ **NEW: Time since last message metric for topics (MQCACF_LAST_PUB_TIME)**

### Phase 3: Code Quality and Maintainability Improvements ✅ COMPLETED
1. **Framework Improvements**
   - ✅ Generic patterns for PCF protocol maintainability
   - ✅ Improved error handling and command logging
   - ✅ Reduced code duplication through abstraction
   - ✅ Enhanced transparency in collection operations

2. **Developer Experience**
   - ✅ Descriptive MQCMD command names instead of numbers
   - ✅ Reduced log verbosity for normal operations
   - ✅ Better debugging and troubleshooting capabilities

### Phase 4: Platform and Advanced Features
1. **Platform Support**
   - Windows support (remove CGO dependency)
   - Consider pure Go implementation

2. **Resource Monitoring** (Requires REST API)
   - CPU/Memory/Disk metrics
   - REST API collector for MQ 9.0.4+

### Medium Priority (Competitive Features)
1. **Auto-discovery**
   - Queue manager discovery (process matching)
   - Dynamic queue discovery with patterns
   - Reduces configuration overhead

2. **Channel Extended Metrics**
   - Buffer statistics
   - Batch efficiency metrics
   - Connection instances

3. **Service Health Checks**
   - Connection validation
   - Queue manager availability
   - Channel health status

### Low Priority (Advanced Features)
1. **Resource Monitoring** (Requires different API)
   - CPU/Memory/Disk metrics
   - REST API collector for MQ 9.0.4+

2. **Bulk Collection Mode**
   - Performance optimization for large environments
   - Wildcard queries

3. **Advanced Features**
   - Transaction tracing
   - Credential vault integration
   - Endianness conversion

## Technical Notes
- Queue statistics require MONQ(MEDIUM) or higher on MQ server
- Many advanced metrics require MQCMD_INQUIRE_Q_STATUS with specific parameters
- Resource metrics require MQ Resource Monitoring API (not available via PCF)
- Consider creating separate collectors:
  - Enhanced PCF collector for queue/channel statistics
  - REST API collector for resource metrics (MQ 9.0.4+)
  - Platform-specific collectors for wider OS support
- Metrics marked with * indicate data is collected but not yet exposed as a metric

## Analysis Notes from Source Code Review

### Datadog Implementation (Verified)
**Note**: Metrics marked with * indicate features found in documentation but not in the open-source code

After reviewing Datadog's open-source code, we confirmed:
- Uses **pymqi** Python library with PCF commands
- Collects only 2 queue manager metrics: dist_lists and max_msg_list
- Has explicit `convert_endianness` option for AIX/IBM i
- Supports optional statistics collection from SYSTEM.ADMIN.STATISTICS.QUEUE
- Implements queue depth percentage calculation
- Minimum collection interval: 15 seconds (no per-second support)
- Reset queue stats are optional (requires +chg permission)
- Process matching for failover scenarios
- No topic/subscription metrics collection found in code

### Datadog: Documentation vs Reality
Based on source code analysis, these metrics appear in documentation but NOT in code:
1. **Queue Manager Level**:
   - Status (only via process matching, not PCF)
   - All resource metrics (CPU, memory, logs, file system)
   - All transaction metrics (commits, rollbacks)
   - Message expiry and pub/sub throughput
   
2. **Channel Level**:
   - Active connections per channel
   - Connection status (only has conn_status string in status metrics)
   
3. **Topic/Subscription Level**:
   - NO topic or subscription collection at all
   
4. **Actually Collects**:
   - Only 2 QMgr metrics: dist_lists, max_msg_list
   - Comprehensive queue metrics (depths, rates, configs)
   - Channel status and statistics
   - Optional statistics from SYSTEM.ADMIN.STATISTICS.QUEUE

### Netdata Implementation Details

**Queue Status Metrics Approach**:
- All status metrics use **AttributeValue** pattern with NotCollected sentinel
- Never sends fake zeros when attributes missing from PCF response
- Maintains data integrity: "missing data = data"

**LastActivity Implementation**:
- Calculates **time since** last GET/PUT operations (like Datadog)
- Converts MQ date/time format (YYYYMMDD/HHMMSSSS) to seconds
- Handles empty strings gracefully (no activity recorded)
- Different from SignalFx which tracks average message residence time (onqtime)

**Status Metric Collection**:
- Single MQCMD_INQUIRE_Q_STATUS per queue (part of 3N pattern)
- Collects: OldestMsgAge, UncommittedMsgs, OpenInput/OutputCount, LastGet/PutDate/Time
- HasStatusMetrics flag doesn't guarantee all attributes present
- Each attribute tracked individually with AttributeValue

### Key Takeaways
1. **Netdata's per-second resolution remains unique** - All competitors limited to 15s+
2. **Most "extensive" metric lists are theoretical** - Actual implementations often more limited
3. **Endianness is handled differently**:
   - Netdata/Prometheus: Automatic via MQ library
   - Datadog: Explicit configuration option
   - Others: Unknown/manual
4. **Statistics collection varies**:
   - Some use RESET_Q_STATS (affects queue statistics)
   - Others use SYSTEM.ADMIN.STATISTICS.QUEUE (non-intrusive)
5. **Cardinality control approaches**:
   - Pattern matching (MQ wildcards)
   - Regex filtering
   - Manual queue lists
   - Auto-discovery with limits