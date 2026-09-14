# eBPF Plugin Functions

The eBPF plugin provides dynamic function calls for real-time network monitoring.

## Socket Collector: network-protocols Function

**Purpose**: Retrieve current network protocol statistics (TCP/UDP, IPv4/IPv6)

**Handler**: `SocketCollector.handleNetworkProtocolsFunction()`

**Response Format**: JSON with current socket metrics
```json
{
  "ipv4_send": 12345,
  "ipv4_recv": 54321,
  "ipv6_send": 1000,
  "ipv6_recv": 2000
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
Update socketFunctionStore
       ↓
Agent ProcessIngress receives FUNCTION call
       ↓
Routes to SocketCollector.handleNetworkProtocolsFunction()
       ↓
Returns JSON response
```

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
