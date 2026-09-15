# eBPF Plugin Functions

The eBPF plugin provides dynamic function calls for real-time network monitoring.

## Socket Collector: network-protocols Function

**Purpose**: Retrieve TCP/UDP socket statistics (IPv4 + IPv6 combined, per-cycle deltas)

**Handler**: `SocketCollector.handleNetworkProtocolsFunction()`

**Response Format**: JSON table with columns:
- Transport (TCP/UDP)
- IP Family (IPv4+IPv6)
- Received (segments/datagrams per second)
- Sent (segments/datagrams per second)
- Errors (failures/rx errors per second)
- Active/Passive Connections (opens/s)
- Retransmitted Segments (TCP only, segments/s)
- Plus additional statistics (see full schema below)

**Example**:
```json
{
  "status": 200,
  "type": "table",
  "update_every": 1,
  "data": [
    ["TCP", "IPv4+IPv6", 100, 200, 5, 10, 0, 2, 0, 300, 1, 0],
    ["UDP", "IPv4+IPv6", 50, 75, 2, 0, 0, 1, 0, 0, 0, 0]
  ],
  "columns": {
    "Transport": {...},
    "Family": {...},
    "Received": {...},
    ...
  },
  "charts": {...}
}
```

### Integration with Agent Framework

Functions are integrated into the agent orchestration framework via:

1. **Function Store**: `socketFunctionStore` holds the latest metrics from each collection cycle
2. **Update in Collect()**: Each collection cycle updates the function store with new data
3. **Handler Method**: `handleNetworkProtocolsFunction()` serves function requests
4. **Automatic Routing**: The agent's `composition.ProcessIngress` routes incoming function calls to the appropriate handler

### Data Flow

```
Collection Cycle
       ↓
SocketCollector.Collect()
       ↓
SocketCollector.state.Update(snapshot) computes deltas
       ↓
socketGlobalPublish metrics written to MetricStore
       ↓
socketFunctionStore updated with socketGlobalPublish
       ↓
Function Request (FUNCTION uid timeout "network-protocols" ...)
       ↓
Agent ProcessIngress routes to SocketCollector.handleNetworkProtocolsFunction()
       ↓
Retrieves latest socketGlobalPublish from fnStore
       ↓
Calls buildNetworkProtocolsJSON() to format table response
       ↓
Returns JSON via netdataapi.FUNCRESULT
```

### Implementation Details

**State Management**:
- `socketGlobalState` tracks kernel counter snapshots
- `socketDelta()` computes per-cycle deltas from monotonic counters
- Handles counter resets and wraps safely

**Metrics**:
- TCP: cleanup_rbuf (recv), sendmsg (sent), retransmits, connects, close calls
- UDP: recvmsg (recv), sendmsg (sent)
- Includes error counters and byte counts for all paths

**Rate Conversion**:
- Per-interval deltas converted to per-second rates in table response
- Uses rounding to avoid floor-to-zero on low-traffic hosts

## Future Function Extensions

Other collectors (cachestat, dcstat, fd) could similarly expose functions:

- **cachestat-stats**: Page cache hit ratio, dirty page count
- **dcstat-stats**: Directory cache statistics
- **fd-stats**: File descriptor open/close rates

Implementation would follow the same pattern:
1. Add fnStore to collector struct
2. Update fnStore in Collect()
3. Implement handler method
4. Declare function in collectorapi interface

## Function Lifecycle

1. **Init()**: Create and initialize fnStore
2. **Collect()**: Update fnStore with latest metrics
3. **Function Call**: Agent routes call to handler method
4. **Response**: Handler returns JSON response
5. **Cleanup()**: Resources cleaned up by agent framework

## Agent ProcessIngress

The agent's `composition.ProcessIngress` automatically:
- Reads stdin for FUNCTION calls
- Routes them to the appropriate collector
- Handles payload parsing
- Returns responses via stdout
- Manages timeout and error handling

No manual stdin dispatch is needed—the framework handles all protocol details.
