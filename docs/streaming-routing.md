# Netdata Streaming Routing

Streaming routing controls how Netdata child nodes connect to parent nodes when multiple parents are available. It handles three key operations: initial parent selection, connection management, and failover.

:::info Prerequisites

This feature requires configuring streaming in `netdata.conf`. See [Streaming Configuration](/src/streaming/README.md) for setup instructions.

:::

## How Streaming Routing Works

### 1. Initial Parent Selection

When a child node starts, it selects a parent from the configured list using this algorithm:

```mermaid
graph LR
    A[Start] --> B{Has data?}
    B -->|Yes| C[Parents with data]
    B -->|No| D[All parents]
    C --> E[Random select]
    D --> E
    E --> F[Connect]
```

**Example:**

```ini
# In child's stream.conf
[stream]
    enabled = yes
    destination = parent-a:19999 parent-b:19999 parent-c:19999
    api key = YOUR_API_KEY
```

With this configuration:

```
Child Node startup:
    │
    ├─→ Parent A (has historical data) ✓ Selected (random between A & B)
    ├─→ Parent B (has historical data) 
    └─→ Parent C (no historical data)   ← Lower priority
```

### 2. Connection Management

Once connected, the child maintains a persistent connection:

- **Connection timeout**: 60 seconds (default)
- **Keepalive**: Continuous streaming maintains connection
- **No automatic rebalancing**: Child stays connected until failure
- **Data integrity**: All metrics buffered during brief disconnections

:::warning Important

Children do not automatically reconnect to their original parent after failover. This prevents connection flapping but requires manual intervention for load redistribution.

:::

### 3. Failover and Reconnection

When the active connection fails:

```mermaid
graph LR
    A[Failed] --> B[Wait 5-X sec]
    B --> C{Next?}
    C -->|Yes| D[Try next]
    C -->|No| E[Restart list]
    E --> D
    D --> F{OK?}
    F -->|No| B
    F -->|Yes| G[Stream]
```

:::note Reconnection Timing

The wait time is randomized between 5 seconds and your configured `reconnect delay` value (default: 5). This prevents thundering herd when multiple children reconnect simultaneously.

:::

**Failover Example:**

```
Normal:     Child → Parent A
            
Failure:    Child ✗ Parent A (connection lost)
            Child → Parent B (immediate failover)
            
Recovery:   Parent A comes back online
            Child → Parent B (stays connected - no automatic switch)
```

## Key Routing Behaviors

| Behavior                   | Description                                                              | Impact                                                    |
|----------------------------|--------------------------------------------------------------------------|-----------------------------------------------------------|
| **Data-Driven Selection**  | Prioritizes parents with complete historical data over network proximity | Child may connect to distant parent if it has better data |
| **Sticky Connections**     | No automatic rebalancing after failover                                  | Requires manual intervention to redistribute load         |
| **Round-Robin Failover**   | Tries parents in configuration order, cycles through entire list         | No parent blacklisting; failed parents are retried        |
| **Connection Persistence** | Maintains connection until failure occurs                                | Prevents unnecessary reconnections and data gaps          |
| **No Health Checks**       | Doesn't proactively test parent availability                             | Discovers failures only when connection breaks            |
| **Randomized Delays**      | Reconnection waits random time (5s to configured maximum)                | Prevents thundering herd during mass reconnections        |

## Configuration Reference

### Essential Parameters

```ini
[stream]
    # Streaming targets (space-separated list)
    destination = parent1:19999 parent2:19999 parent3:19999
    
    # Reconnection delay - randomized between 5 and this value (seconds)
    # Default: 5, Minimum: 5
    reconnect delay = 5
    
    # Initial connection timeout
    timeout seconds = 60
```

:::tip Performance Tip

List parents in order of preference. While selection is random among equals, the order matters during failover - the child will try parents in the listed sequence.

:::

### Multi-Tier Setup

For larger deployments:

```
Child Nodes ──→ Parent Proxies ──→ Ultimate Parents
                 (forward only)      (store & analyze)
```

Configure intermediate parents as proxies to distribute load without storage overhead.

## Monitoring Streaming Status

### Check Connection Status

#### Using the UI

The **Netdata Streaming** function (under the "Functions" tab) provides:

- Comprehensive overview of all streaming connections
- Status, replication completion time, and connection details
- Works on both parent and child nodes:
    - **On child**: Shows outgoing connections
    - **On parent**: Shows incoming connections (InHops = 1 for direct children, >1 for proxied connections)

#### Viewing Logs

```bash
# Check journal for streaming-related messages
journalctl _SYSTEMD_INVOCATION_ID="$(systemctl show --value --property=InvocationID netdata)" --namespace=netdata --grep stream
```

### Verify Parent Connectivity

```bash
# Test each parent
nc -zv parent-a 19999
nc -zv parent-b 19999
```

:::note Troubleshooting

If a child connects to an unexpected parent, check the data retention on all parents. The child prefers parents that already have its historical data.

:::

## Common Scenarios

| Scenario          | What Happens                            | Why                      |
|-------------------|-----------------------------------------|--------------------------|
| Parent A fails    | Child switches to Parent B              | Automatic failover       |
| All parents fail  | Child cycles through list every second  | Continuous retry         |
| Parent A recovers | Child stays on Parent B                 | No automatic rebalancing |
| New child starts  | Randomly selects from parents with data | Load distribution        |

:::caution Maintenance Planning

When taking a parent offline for maintenance, its children will failover to other parents and won't automatically return. Plan capacity accordingly.

:::

## Best Practices

1. **List parents in priority order** - First parent is preferred if all equal
2. **Configure at least 3 parents** - Ensures availability during maintenance
3. **Monitor parent data completeness** - Affects routing decisions
4. **Plan maintenance carefully** - Children won't automatically return
