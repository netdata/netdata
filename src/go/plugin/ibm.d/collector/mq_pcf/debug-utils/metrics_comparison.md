# IBM MQ PCF Metrics Availability Comparison

## Queue Metrics Comparison

| Metric | MQCMD_INQUIRE_Q | MQCMD_INQUIRE_Q_STATUS | MQCMD_RESET_Q_STATS | Notes |
|--------|-----------------|------------------------|---------------------|-------|
| **Queue Identification** |
| Queue Name | ✅ MQCA_Q_NAME | ✅ MQCA_Q_NAME | ✅ MQCA_Q_NAME | Always available |
| Queue Type | ✅ MQIA_Q_TYPE | ❌ | ❌ | Local, alias, remote, etc. |
| Base Queue Name | ✅ MQCA_BASE_Q_NAME | ❌ | ❌ | For alias queues |
| **Current State** |
| Current Depth | ✅ MQIA_CURRENT_Q_DEPTH | ✅ MQIA_CURRENT_Q_DEPTH | ❌ | Real-time depth |
| Max Queue Depth | ✅ MQIA_MAX_Q_DEPTH | ❌ | ❌ | Configuration limit |
| Depth High Limit | ✅ MQIA_Q_DEPTH_HIGH_LIMIT | ❌ | ❌ | Alert threshold |
| Depth Low Limit | ✅ MQIA_Q_DEPTH_LOW_LIMIT | ❌ | ❌ | Alert threshold |
| **Message Counters** |
| Messages Enqueued | ❌ | ❌ | ✅ MQIA_MSG_ENQ_COUNT | **Destructive reset!** |
| Messages Dequeued | ❌ | ❌ | ✅ MQIA_MSG_DEQ_COUNT | **Destructive reset!** |
| High Queue Depth | ❌ | ❌ | ✅ MQIA_HIGH_Q_DEPTH | Peak since reset |
| Time Since Reset | ❌ | ❌ | ✅ MQIA_TIME_SINCE_RESET | Seconds |
| **Open Handles** |
| Open Input Count | ❌ | ✅ MQIA_OPEN_INPUT_COUNT | ❌ | Apps reading |
| Open Output Count | ❌ | ✅ MQIA_OPEN_OUTPUT_COUNT | ❌ | Apps writing |
| **Queue Status** |
| Inhibit Get | ✅ MQIA_INHIBIT_GET | ❌ | ❌ | Get operations blocked |
| Inhibit Put | ✅ MQIA_INHIBIT_PUT | ❌ | ❌ | Put operations blocked |
| **Timestamps** |
| Creation Date | ✅ MQCA_CREATION_DATE | ❌ | ❌ | Queue creation |
| Creation Time | ✅ MQCA_CREATION_TIME | ❌ | ❌ | Queue creation |
| Alteration Date | ✅ MQCA_ALTERATION_DATE | ❌ | ❌ | Last modified |
| Alteration Time | ✅ MQCA_ALTERATION_TIME | ❌ | ❌ | Last modified |
| Last Get Date | ❌ | ✅ MQCACF_LAST_GET_DATE | ❌ | Last message get |
| Last Get Time | ❌ | ✅ MQCACF_LAST_GET_TIME | ❌ | Last message get |
| Last Put Date | ❌ | ✅ MQCACF_LAST_PUT_DATE | ❌ | Last message put |
| Last Put Time | ❌ | ✅ MQCACF_LAST_PUT_TIME | ❌ | Last message put |
| **Configuration** |
| Default Priority | ✅ MQIA_DEF_PRIORITY | ❌ | ❌ | Default msg priority |
| Default Persistence | ✅ MQIA_DEF_PERSISTENCE | ❌ | ❌ | Persistent/non-persistent |
| Max Message Length | ✅ MQIA_MAX_MSG_LENGTH | ❌ | ❌ | Max msg size |
| Shareability | ✅ MQIA_SHAREABILITY | ❌ | ❌ | Exclusive/shared |
| Usage | ✅ MQIA_USAGE | ❌ | ❌ | Normal/transmission |
| **Monitoring Settings** |
| Monitoring Queue | ✅ MQIA_MONITORING_Q | ❌ | ❌ | OFF/LOW/MEDIUM/HIGH |
| Statistics Queue | ✅ MQIA_STATISTICS_Q | ❌ | ❌ | OFF/ON |
| Accounting Queue | ✅ MQIA_ACCOUNTING_Q | ❌ | ❌ | OFF/ON |

## Summary by Command

### MQCMD_INQUIRE_Q
- **Purpose**: Get queue configuration and current state
- **Destructive**: No ✅
- **Returns**: 63 attributes total
- **Best for**: Configuration, limits, current depth, queue state
- **Missing**: Runtime counters (enq/deq), open handles, timestamps

### MQCMD_INQUIRE_Q_STATUS  
- **Purpose**: Get current runtime status
- **Destructive**: No ✅
- **Returns**: ~15 attributes
- **Best for**: Open handles, last put/get times, current depth
- **Missing**: Configuration, runtime counters
- **Note**: Returns data only if queue has been accessed

### MQCMD_RESET_Q_STATS
- **Purpose**: Get and reset statistics counters
- **Destructive**: Yes ❌ (resets counters!)
- **Returns**: 5 attributes
- **Best for**: Message counts, peak depth, time since reset
- **Missing**: Everything else
- **WARNING**: Breaks other monitoring tools!

## Recommended Monitoring Strategy

### What We Can Monitor (Non-Destructive)
Using MQCMD_INQUIRE_Q + MQCMD_INQUIRE_Q_STATUS:
- ✅ Current queue depth
- ✅ Queue configuration (max depth, limits, inhibit status)
- ✅ Open input/output counts (applications connected)
- ✅ Last GET/PUT timestamps
- ✅ Queue type and relationships
- ✅ Creation/alteration dates
- ✅ All configuration parameters

### What We Cannot Monitor (Without Side Effects)
- ❌ Total messages enqueued (MSG_ENQ_COUNT)
- ❌ Total messages dequeued (MSG_DEQ_COUNT)  
- ❌ Peak queue depth since last reset
- ❌ Time since statistics reset
- ❌ Queue time statistics (avg/min/max)

### Alternative Data Sources
1. **MQ Event Messages** - Can provide enqueue/dequeue events
2. **MQ Accounting Records** - Detailed message flow data
3. **Application-level Metrics** - Count messages in the application
4. **MQ Trace** - Detailed but performance-impacting

## Channel Metrics (For Reference)

| Metric | MQCMD_INQUIRE_CHANNEL | MQCMD_INQUIRE_CHANNEL_STATUS |
|--------|----------------------|------------------------------|
| Channel Name | ✅ | ✅ |
| Channel Type | ✅ | ✅ |
| Connection Name | ✅ | ✅ |
| Current Messages | ❌ | ✅ (if active) |
| Bytes Sent/Received | ❌ | ✅ (if active) |
| Batches Completed | ❌ | ✅ (if active) |
| Configuration | ✅ (all) | ❌ |

**Note**: Channel runtime metrics are only available for ACTIVE channels. Inactive channels return MQRCCF_CHANNEL_NOT_ACTIVE (3065).