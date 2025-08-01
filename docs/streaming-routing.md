# Netdata Streaming Routing

## What is Streaming Routing?

Streaming routing determines how Netdata child nodes connect to parent nodes in a distributed monitoring setup. When multiple parents are available, the routing algorithm automatically selects the best destination and handles failover.

:::note 
This feature requires configuring streaming in `netdata.conf`. See [Streaming Configuration](/src/streaming/README.md) for setup instructions.
:::

## How Parent Selection Works

When a child node initiates or loses connection, it selects a parent, using this priority:

1. **Check data completeness** - Prefer parents that already have this node's historical data
2. **Load balance** - Randomly choose among equally suitable parents
3. **Failover** - If connection fails, try the next parent in the list

```
Child Node
    │
    ├─→ Parent A (has historical data) ✓ Selected
    ├─→ Parent B (has historical data)
    └─→ Parent C (no historical data)
```

## Multi-Tier Architecture

In larger deployments, you can create multiple tiers:

```
Ingest Layer ──→ Proxy Layer ──→ Storage Layer
```

This setup distributes the workload:
- **Ingest Layer**: Children collect metrics
- **Proxy Layer**: Parents forward data without heavy processing
- **Storage Layer**: Store data and run analytics

## Query Routing

Netdata Cloud optimizes query performance by routing requests intelligently:

- **Historical queries** → Sent to the furthest parent (better resources)
- **Live queries** → Sent to the closest node (lower latency)

## Machine Learning Workload

The ML workload distribution depends on your configuration:

| Child ML Setting | Impact                                                           |
|------------------|------------------------------------------------------------------|
| Disabled         | Parent must train models from scratch (high CPU usage)           |
| Enabled          | Child trains models locally and sends results (lower parent CPU) |

## Key Behaviors

### Network vs. Data Priority

Unlike traditional networking, Netdata prioritizes **data integrity** over network efficiency:

- May connect to geographically distant parents if they have better data
- Ensures no metrics are lost during failures
- Automatically rebalances load after recovery

### Failover Example

```
Normal operation:
Child → Parent A (primary)

During Parent A outage:
Child → Parent B (failover)

After recovery:
Child may stay with Parent B if it now has complete data
```

### Connection Behavior

- **Reconnection interval**: 1 second (configurable via `reconnect delay seconds`)
- **Connection timeout**: 60 seconds default
- **Persistent connections**: Child maintains connection until failure
- **No automatic rebalancing**: Child won't switch parents unless current connection fails
