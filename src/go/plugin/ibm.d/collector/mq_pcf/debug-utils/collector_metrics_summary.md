# Netdata MQ PCF Collector - Metrics Collection Summary

## Queue Metrics Currently Collected

### From MQCMD_INQUIRE_Q (Non-Destructive ✅)
| Metric | Chart Name | Collection Status |
|--------|------------|-------------------|
| Current Queue Depth | `queue_<name>_depth` | ✅ Implemented |
| Max Queue Depth | `queue_<name>_max_depth` | ✅ Implemented |
| Depth High Limit | `queue_<name>_depth_high_limit` | ✅ Implemented |
| Depth Low Limit | `queue_<name>_depth_low_limit` | ✅ Implemented |
| Inhibit Get Status | `queue_<name>_inhibit_get` | ✅ Implemented |
| Inhibit Put Status | `queue_<name>_inhibit_put` | ✅ Implemented |
| Default Priority | `queue_<name>_def_priority` | ✅ Implemented |
| Default Persistence | `queue_<name>_def_persistence` | ✅ Implemented |

### From MQCMD_INQUIRE_Q_STATUS (Non-Destructive ✅)
| Metric | Chart Name | Collection Status |
|--------|------------|-------------------|
| Open Input Count | `queue_<name>_open_input_count` | ✅ Implemented |
| Open Output Count | `queue_<name>_open_output_count` | ✅ Implemented |

### Metrics We Want But Cannot Get (Non-Destructively)
| Metric | Why Not Available | Impact |
|--------|-------------------|---------|
| Messages Enqueued | Only in MQCMD_RESET_Q_STATS (destructive) | Cannot track message rates |
| Messages Dequeued | Only in MQCMD_RESET_Q_STATS (destructive) | Cannot track message rates |
| Peak Queue Depth | Only in MQCMD_RESET_Q_STATS (destructive) | Cannot track historical peaks |
| Queue Time Stats | Only in MQCMD_RESET_Q_STATS (destructive) | Cannot track message latency |

## Channel Metrics Currently Collected

### From MQCMD_INQUIRE_CHANNEL_STATUS (Non-Destructive ✅)
| Metric | Chart Name | Collection Status |
|--------|------------|-------------------|
| Messages Sent | `channel_<name>_messages_sent` | ✅ Implemented (active channels only) |
| Bytes Sent | `channel_<name>_bytes_sent` | ✅ Implemented (active channels only) |
| Bytes Received | `channel_<name>_bytes_received` | ✅ Implemented (active channels only) |
| Batches Completed | `channel_<name>_batches_completed` | ✅ Implemented (active channels only) |
| Buffers Sent | `channel_<name>_buffers_sent` | ✅ Implemented (active channels only) |
| Buffers Received | `channel_<name>_buffers_received` | ✅ Implemented (active channels only) |

### From MQCMD_INQUIRE_CHANNEL (Non-Destructive ✅)
| Metric | Chart Name | Collection Status |
|--------|------------|-------------------|
| Batch Size | `channel_<name>_batch_size_config` | ✅ Implemented |
| Max Message Length | `channel_<name>_max_msg_length_config` | ✅ Implemented |
| Heartbeat Interval | `channel_<name>_heartbeat_interval_config` | ✅ Implemented |
| Network Priority | `channel_<name>_network_priority_config` | ✅ Implemented |

## Queue Manager Metrics Currently Collected

### From MQCMD_INQUIRE_Q_MGR_STATUS (Non-Destructive ✅)
| Metric | Chart Name | Collection Status |
|--------|------------|-------------------|
| Connection Count | `queue_manager_connections` | ✅ Implemented |
| Chinit Status | `queue_manager_chinit_status` | ✅ Implemented |
| Command Server Status | `queue_manager_command_server_status` | ✅ Implemented |

## Metrics Gap Analysis

### Critical Missing Metrics (Due to MQ API Limitations)
1. **Message Rates** - Cannot calculate messages/second without counters
2. **Queue Performance** - Cannot measure message processing latency
3. **Historical Peaks** - Cannot track maximum queue depths over time
4. **Throughput Trends** - Cannot show message flow patterns

### Workarounds for Users
1. **For Message Rates**:
   - Enable MQ accounting and use MQ Explorer
   - Use application-level metrics
   - Monitor queue depth changes (approximate only)

2. **For Performance Metrics**:
   - Use MQ event messages
   - Implement application-level timing
   - Use MQ trace (with caution - impacts performance)

3. **For Historical Data**:
   - Use a single dedicated monitoring tool that performs RESET_Q_STATS
   - Store peak values in external database
   - Use MQ's built-in statistics collection

## Collection Frequency Recommendations

| Metric Type | Recommended Interval | Reason |
|-------------|---------------------|---------|
| Queue Depths | 5-10 seconds | Real-time monitoring |
| Queue Status | 30-60 seconds | Less frequent changes |
| Channel Status | 10-30 seconds | Active channels only |
| Queue Manager | 60 seconds | Stable metrics |

## Performance Considerations

1. **MQCMD_INQUIRE_Q** - Fast, can handle 1000s of queues
2. **MQCMD_INQUIRE_Q_STATUS** - Slower, fails if queue not accessed
3. **MQCMD_INQUIRE_CHANNEL_STATUS** - Fails for inactive channels
4. **Avoid MQCMD_RESET_Q_STATS** - Destructive to other monitoring

## Future Enhancement Possibilities

1. **Optional Destructive Mode** - Allow RESET_Q_STATS with big warnings
2. **Event Message Integration** - Parse MQ event messages for rates
3. **Accounting Record Parser** - Read MQ accounting data
4. **Derived Metrics** - Calculate approximate rates from depth changes
5. **Cache Queue Status** - Reduce failed INQUIRE_Q_STATUS calls