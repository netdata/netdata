# MQ PCF Debug Utilities

This directory contains debug utilities for analyzing IBM MQ PCF (Programmable Command Format) responses. These tools help understand what attributes and metrics are available from MQ at different versions and configurations.

## Purpose

These utilities were created to:
1. Debug missing runtime metrics (MSG_ENQ_COUNT, MSG_DEQ_COUNT) in the Netdata MQ collector
2. Analyze all 63 attributes returned by MQCMD_INQUIRE_Q and determine which ones to collect
3. Understand channel attribute availability across different MQ versions
4. Test queue statistics collection using MQCMD_RESET_Q_STATS
5. Document the actual PCF response structure for future reference

## Available Utilities

### dump_queue_attrs
Dumps all attributes returned by MQCMD_INQUIRE_Q for a specific queue.

```bash
./dump_queue_attrs <queue_manager> <queue_name> [host] [port] [channel] [user] [password]

# Example:
./dump_queue_attrs QM1 TEST.QUEUE1.943 localhost 1414 SYSTEM.DEF.SVRCONN app passw0rd
```

### dump_channel_attrs
Dumps all attributes returned by MQCMD_INQUIRE_CHANNEL for a specific channel.

```bash
./dump_channel_attrs <queue_manager> <channel_name> [host] [port] [channel] [user] [password]

# Example:
./dump_channel_attrs QM1 SYSTEM.DEF.SVRCONN localhost 1414 SYSTEM.DEF.SVRCONN app passw0rd
```

### test_queue_stats
Tests MQCMD_RESET_Q_STATS to retrieve and reset queue statistics.

```bash
./test_queue_stats <queue_manager> <queue_name> [host] [port] [channel] [user] [password]

# Example:
./test_queue_stats QM1 TEST.QUEUE1.943 localhost 1414 SYSTEM.DEF.SVRCONN app passw0rd
```

### test_queue_status
Tests MQCMD_INQUIRE_Q_STATUS to retrieve runtime queue status without resetting statistics.

```bash
./test_queue_status <queue_manager> <queue_name> [host] [port] [channel] [user] [password]

# Example:
./test_queue_status QM1 TEST.QUEUE1.943 localhost 1414 SYSTEM.DEF.SVRCONN app passw0rd
```

### test_queue_stats_attrs
Tests MQCMD_RESET_Q_STATS with specific attribute requests (experimental).

```bash
./test_queue_stats_attrs <queue_manager> <queue_name> [host] [port] [channel] [user] [password]

# Example:
./test_queue_stats_attrs QM1 TEST.QUEUE1.943 localhost 1414 SYSTEM.DEF.SVRCONN app passw0rd
```

## Building

Use the provided Makefile:

```bash
# Build all utilities
make

# Build specific utility
make dump_queue_attrs

# Clean
make clean
```

## Key Findings

### Queue Attributes (MQCMD_INQUIRE_Q)
- Returns 63 attributes total for a queue
- Does NOT include runtime metrics like MSG_ENQ_COUNT or MSG_DEQ_COUNT
- Includes configuration (max depth, triggers, persistence)
- Includes current state (depth, inhibit status, open counts)
- Available attributes are consistent across MQ versions

### Queue Statistics (MQCMD_RESET_Q_STATS)
- Returns MSG_ENQ_COUNT and MSG_DEQ_COUNT
- Also returns HIGH_Q_DEPTH (peak depth since reset)
- Returns timing statistics (Q_TIME_AVG, Q_TIME_MIN, Q_TIME_MAX)
- Requires STATQ to be enabled (ALTER QMGR STATQ(ON))
- **CRITICAL: Statistics are reset after retrieval - this is destructive!**
- Not suitable for monitoring as it interferes with other tools

### Queue Status (MQCMD_INQUIRE_Q_STATUS)
- Returns current runtime status without resetting counters
- Includes CURRENT_Q_DEPTH, OPEN_INPUT_COUNT, OPEN_OUTPUT_COUNT
- Returns LAST_GET_DATE/TIME and LAST_PUT_DATE/TIME
- Does NOT return MSG_ENQ_COUNT or MSG_DEQ_COUNT
- Non-destructive alternative to RESET_Q_STATS for some metrics

### Channel Attributes (MQCMD_INQUIRE_CHANNEL)
- Returns extensive configuration and runtime data
- Includes batch settings, timeouts, retry configuration
- Runtime metrics (messages, bytes, batches) only available for active channels
- MQRCCF_CHANNEL_NOT_ACTIVE (3065) returned for inactive channels

## Invalid PCF Constants Found

The following constants were found in the original code but don't exist in IBM MQ:
- `MQIACF_Q_ATTRS = 1002` - Not a valid PCF constant
- `MQIACF_CHANNEL_ATTRS = 1015` - Not a valid PCF constant

These were causing MQRC_OBJECT_OPEN_ERROR (2067) for every queue/channel query.

## Test Environments Used

These utilities were tested against:
1. MQ 9.4.3 (Latest) - localhost:1414
2. MQ 9.2.4 LTS - localhost:1415  
3. MQ 9.1.5 - localhost:1416

All versions returned consistent attribute sets.

## Usage in Collector Development

1. Run utilities against target MQ versions to verify attribute availability
2. Save output to `.out` files for reference (e.g., `dump_queue_attrs.out`)
3. Use findings to update collector to use all available attributes
4. Test queue statistics collection feasibility for runtime metrics

## MQ Runtime Metrics Limitations

### The Challenge
IBM MQ separates queue metrics into different categories:
1. **Configuration & State** - Available via MQCMD_INQUIRE_Q (non-destructive)
2. **Runtime Counters** - Only available via MQCMD_RESET_Q_STATS (destructive)
3. **Current Status** - Available via MQCMD_INQUIRE_Q_STATUS (non-destructive)

### Monitoring Implications
For continuous monitoring, we cannot use MQCMD_RESET_Q_STATS because:
- It resets counters to zero after reading them
- This breaks other monitoring tools that rely on these counters
- Multiple monitoring tools cannot coexist if they all reset counters

### What We Can Monitor
Using non-destructive commands only:
- Current queue depth
- Queue configuration (max depth, inhibit status, etc.)
- Open input/output counts (applications reading/writing)
- Last GET/PUT timestamps
- Peak queue depth (from INQUIRE_Q if statistics enabled)

### What We Cannot Monitor (without side effects)
- Message enqueue count (total messages put)
- Message dequeue count (total messages gotten)
- Average queue time
- Reset-based statistics

This is a fundamental limitation of the MQ monitoring API design.

## Notes

- These are development tools, not for production use
- Credentials are passed as command line arguments (not secure for production)
- Tools create temporary reply queues that are automatically cleaned up
- Output shows both attribute constants and their numeric values for debugging