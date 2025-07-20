# IBM MQ Monitoring: Netdata vs Competitors Analysis

This document compares our mq_pcf collector with leading IBM MQ monitoring solutions.

## Current Implementation Status

**As of latest updates:**
- **Total Metrics**: 1043+ time-series for mq-test instance (up from 1042)
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
  - ✅ Active Listener Status monitoring
  
**Metric Breakdown**:
- **Queue Metrics**: 12 contexts (depth, messages, connections, high_depth, oldest_msg_age, uncommitted_msgs, last_activity, inhibit_status, priority, triggers, backout_threshold, max_msg_length)
- **Channel Metrics**: 3 contexts (status, messages, bytes) with batch configuration
- **Topic Metrics**: 3 contexts (publishers, subscribers, messages)
- **Queue Manager**: Status, connection count, and overview metrics
- **Listener Metrics**: 2 contexts (status, port)
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
- ✅* = Partially implemented or via indirect method

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
| Current depth | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Messages in queue |
| Depth percentage | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | % of max depth |
| Max depth | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Configuration limit |
| Enqueue count/rate | ✅* | ✅ | ✅* | ✅* | ❌ | ✅ | *Requires RESET_Q_STATS |
| Dequeue count/rate | ✅* | ✅ | ✅* | ✅* | ❌ | ✅ | *Requires RESET_Q_STATS |
| Open input/output count | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Active connections |
| Oldest message age | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Queue time tracking |
| Average queue time | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | Message residence time |
| Last GET/PUT time | ✅* | ❓ | ✅ | ✅ | ✅ | ❌ | Operation timestamps |
| Expired messages | ❌ | ✅ | ❓ | ✅ | ❌ | ❌ | Per-queue expiry |
| Get/Put bytes | ❌ | ✅ | ❓ | ✅ | ❌ | ❌ | Data volume metrics |
| Browse count/bytes | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Non-destructive reads |
| Get/Put fail counts | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Failed operations |
| Non-queued messages | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Direct transfers |
| Purge count | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Purged messages |
| MQOPEN/CLOSE/INQ/SET counts | ❌ | ✅ | ❓ | ❌ | ❌ | ❌ | API operation counts |
| Short/Long time indicators | ❌ | ❓ | ✅ | ❌ | ❌ | ✅ | Queue time distribution |
| High/Low depth events | ✅ | ❓ | ✅ | ✅ | ❌ | ✅ | Threshold monitoring |
| Inhibit status | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Get/Put inhibited |
| Backout threshold | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Poison msg handling |
| Trigger settings | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Trigger configuration |
| Service interval | ❌ | ❌ | ❌ | ✅ | ❌ | ✅ | Target service time |
| Retention interval | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Queue lifetime |
| High queue depth (stats) | ✅* | ❌ | ❌ | ✅ | ❌ | ✅ | Peak depth in interval |
| Min/Max depth (stats) | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Depth range in interval |
| Time since reset | ✅* | ❌ | ❌ | ✅ | ❌ | ❌ | Stats window |
| Uncommitted messages | ✅ | ❌ | ❌ | ✅ | ✅ | ✅ | Transaction pending |
| Harden get backout | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Backout persistence |
| Message delivery sequence | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | FIFO/Priority |
| Queue scope | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Local/Cell |
| Queue type | ✅ | ❓ | ❓ | ✅ | ✅ | ✅ | Local/Alias/Remote |
| Queue usage | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Normal/Transmission |
| Default persistence | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Message persistence |
| Default priority | ✅ | ❌ | ❌ | ✅ | ❌ | ❌ | Message priority |
| Queue file size | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | Current file size |
| Max queue file size | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | Maximum file size |

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
| Status | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Running/stopped/retrying |
| Connection status | ❌ | ❌ | ❌ | ✅* | ❌ | ❌ | Connection state |
| Active connections | ❌ | ❌ | ❌ | ⚠️ | ❌ | ❌ | Per channel instances |
| Message rate | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | Messages/second |
| Byte rate | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | Bytes/second |
| Buffers sent/received | ❌ | ❌ | ❌ | ✅ | ❌ | ✅ | Buffer counts |
| Batch metrics | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Batch size/rate |
| Full/incomplete batches | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Batch efficiency |
| Average batch size | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Batch statistics |
| Put retries | ❌ | ❌ | ❌ | ✅ | ❌ | ✅ | Message retry count |
| Current messages | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | In-doubt messages |
| Configuration | ✅ | ❓ | ✅ | ✅ | ❌ | ❌ | Timeouts, retries, etc |
| XMITQ time | ❌ | ✅ | ❓ | ❌ | ❌ | ❌ | Transmission queue wait |
| MCA status | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Agent status |
| In-doubt status | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Transaction state |
| SSL key resets | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Security resets |
| NPM speed | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | Non-persistent msg speed |
| Message retry settings | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ | MR count/interval |
| Max sharing conversations | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | Max conversations |
| Current sharing conversations | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | Current conversations |

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
| Time since last message | ❌ | ✅ | ❓ | ⚠️ | ❌ | ❌ | Activity tracking |

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
| Status | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ | Running/stopped |
| Port | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ | Listening port |
| Backlog | ❌ | ❓ | ❓ | ❌ | ❌ | ❌ | Connection backlog |

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

### High Priority (Core Gaps)
1. **Queue Time Metrics** - Mostly implemented
   - ✅ Oldest message age (MQIA_MSGAGE) - IMPLEMENTED
   - ✅ Last GET/PUT timestamps - IMPLEMENTED (as time since last activity)
   - ❌ Average queue time
   - Available via MQCMD_INQUIRE_Q_STATUS

2. **Extended Queue Statistics**
   - Get/Put bytes (not just counts)
   - Failed operation counts
   - Browse statistics
   - Available via MQCMD_INQUIRE_Q_STATUS with MQIACF_Q_STATISTICS

3. **Platform Support**
   - Windows support (remove CGO dependency)
   - Consider pure Go implementation

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