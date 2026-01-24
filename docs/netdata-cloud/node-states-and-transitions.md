# Node States

Netdata provides dashboards at multiple levels of your infrastructure. Each level displays node states based on what it can observe. This page explains what each state means, when transitions happen, and how to configure the behavior.

## Dashboard Levels

Netdata's distributed architecture provides three observation points:

| Dashboard | What You See | How to Access |
|-----------|--------------|---------------|
| **Agent** | Local node only | `http://node-ip:19999` |
| **Parent** | All nodes streaming to this Parent | `http://parent-ip:19999` |
| **Netdata Cloud** | All nodes claimed to your Space | `https://app.netdata.cloud` |

**How data flows:**

```
Agent → (streaming) → Parent → (ACLK) → Netdata Cloud
Agent → (ACLK) → Netdata Cloud  (standalone, no Parent)
```

Node states reflect this flow: if a link breaks, states change based on where data is still available.

## States on Netdata Cloud

| State | Meaning |
|-------|---------|
| **Live** | Node is connected to Netdata Cloud (directly or via Parents) and providing live metrics |
| **Stale** | Node disconnected, but a Parent connected to Netdata Cloud has its historical data |
| **Offline** | Node is disconnected and no data is available |
| **Unseen** | Node was claimed but has never connected |

### Stale vs Offline

The difference is **data availability**:

| Scenario | State | Can Query Data? |
|----------|-------|-----------------|
| Child disconnected, Parent connected to Cloud | **Stale** | Yes, via Parent |
| Standalone Agent disconnected | **Offline** | No |
| Child disconnected, all Parents disconnected from Cloud | **Offline** | No |

Stale nodes remain queryable because the Parent serves as a data cache. This is why you cannot delete Stale nodes from the UI—they still have accessible data.

## States on Parent Dashboards

Parents display nodes that stream (or have streamed) to them:

| State | Meaning |
|-------|---------|
| **Live** | Node is actively streaming metrics |
| **Stale** | Node stopped streaming, historical data retained |

Parents don't show Offline or Unseen states. When a node's retention expires or cleanup runs, it disappears from the Parent's view entirely.

## State Mapping: Parent → Cloud

When a Parent connects to Netdata Cloud, it reports the state of all its children:

| Parent Sees | Cloud Shows | Why |
|-------------|-------------|-----|
| Live | **Live** | Data flowing through Parent |
| Stale | **Stale** | Parent connected to Cloud has historical data |
| (removed) | **Offline** | No data source available |

**High-availability setups (recommended):**
With two Parents (Child → P1 → P2), children stream to one Parent, which replicates to the other. Both Parents connect to Cloud.

If the child connects to Cloud only via Parents:

| Event | Result | Why |
|-------|--------|-----|
| P1 disconnects from Cloud | No change (Live) | P1 still runs, replicates to P2, P2 reports to Cloud |
| P1 stops | Brief Stale, then Live | Child fails over to P2 |
| P2 disconnects from Cloud | No change (Live) | P1 still connected to Cloud |
| P2 stops | No change (Live) | Child still streams to P1, P1 reports to Cloud |
| Both disconnect from Cloud | **Offline** | No Parent can report to Cloud |

If the child also connects directly to Cloud, it remains Live regardless of Parent status.

**Single Parent setups:**
When the only Parent disconnects from Cloud, all its children become **Offline** because Cloud can no longer query their data.

**When a Parent reconnects:**
Children with retained data appear as **Stale** (or **Live** if actively streaming).

## Transition Timings

### Detection Speed

**Netdata Cloud detects agent/parent disconnection:**

| Event | Detection Time | Mechanism |
|-------|----------------|-----------|
| Agent or Parent loses Cloud connection | **~60 seconds** | MQTT keepalive (60s interval) |
| UI reflects state change | **1-2 minutes** | Cloud processing + UI refresh |

**Parent detects child disconnection:**

| Event | Detection Time | Mechanism |
|-------|----------------|-----------|
| Child shuts down gracefully | **Immediate** | Socket close detected |
| Child crashes or network drops | **~60 seconds** | TCP keepalive probes (30s idle + 3×10s probes) |
| Child silently stops sending data | **10 minutes** | Idle activity timeout |

These timings are hardcoded and not user-configurable.

### Standalone Agent Transitions

A standalone Agent connects directly to Cloud without a Parent.

| Event | From | To | Timing |
|-------|------|-----|--------|
| Agent starts, connects to Cloud | Unseen/Offline | **Live** | Immediate on connection |
| Agent stops or loses network | Live | **Offline** | Immediate to ~60 seconds |
| Agent restarts | Offline | **Live** | Immediate on reconnection |

**No Stale state**: Standalone agents go directly to Offline because there's no Parent holding their data.

### Child Node Transitions

A child streams metrics to a Parent, which connects to Cloud.

| Event | From | To | Timing |
|-------|------|-----|--------|
| Child connects to Parent | Unseen/Offline | **Live** | Immediate |
| Child stops streaming | Live | **Stale** | Immediate to ~60 seconds (see Detection Speed) |
| Child restarts streaming | Stale | **Live** | Immediate |
| All Parents go offline | Live/Stale | **Offline** | Immediate to ~60 seconds |
| Parent reconnects (child still down) | Offline | **Stale** | Immediate (if data retained) |

### First Connection

| Event | From | To | Timing |
|-------|------|-----|--------|
| Node claimed to Space | - | **Unseen** | Immediate |
| Node connects for first time | Unseen | **Live** | Immediate on connection |

## Automatic Cleanup

### Netdata Cloud Cleanup

Cloud automatically removes nodes that remain Offline or Unseen:

| Node Type | Cleanup After | Notes |
|-----------|---------------|-------|
| Standalone agents (0 hops) | **7 days** | Direct Cloud connection |
| Child nodes (1+ hops) | **48 hours** | Connected via Parent |
| Unseen nodes | **48 hours** | Claimed but never connected |

**Stale nodes are never automatically removed.** They have queryable data via their Parent.

These thresholds are managed by Netdata Cloud infrastructure and are not user-configurable.

## Configuration Options

### Ephemeral Nodes

For dynamic infrastructure (auto-scaling groups, containers, spot instances), mark nodes as ephemeral:

```ini
# On the child node's netdata.conf
[global]
    is ephemeral node = yes
```

**Effects:**
- No disconnection alerts for this node
- Node label `_is_ephemeral=true` propagates to Parents and Cloud

### Marking Existing Nodes as Ephemeral

To mark already-offline nodes as ephemeral (clears alerts, keeps data queryable):

```bash
netdatacli mark-stale-nodes-ephemeral <node-id | hostname | ALL_NODES>
```

### Removing Nodes

To force-remove a Stale node:

```bash
netdatacli remove-stale-node <node-id | hostname | ALL_NODES>
```

This sends an offline signal to Cloud. The node transitions to Offline and becomes eligible for cleanup (or immediate UI deletion).

See [Remove Node](/docs/learn/remove-node.md) for detailed instructions.

### Connection Hops

Check how a node connects to Cloud:

| Hops | Meaning |
|------|---------|
| 0 | Direct Cloud connection (standalone) |
| 1 | Connected via one Parent |
| 2+ | Connected via chained Parents |

View hops in Netdata Cloud by clicking the node info button.

Nodes with more hops have more potential failure points, but also benefit from Parent data caching (Stale state instead of Offline).

## Troubleshooting

### Log Filtering with MESSAGE_ID

Netdata logs include MESSAGE_IDs for filtering specific events. Use `journalctl` to view relevant logs:

```bash
# Cloud connection events (ACLK)
journalctl -u netdata MESSAGE_ID=acb33cb9-5778-476b-aac7-02eb7e4e151d

# Streaming from children (on Parent)
journalctl -u netdata MESSAGE_ID=ed4cdb8f-1beb-4ad3-b57c-b3cae2d162fa

# Streaming to parent (on Child)
journalctl -u netdata MESSAGE_ID=6e2e3839-0676-4896-8b64-6045dbf28d66
```

### Node shows Stale, expected Live

**Cause:** Node stopped streaming to its Parent.

**Check:**
1. Is the node's Netdata Agent running? `systemctl status netdata`
2. Can the node reach the Parent? Check network/firewall
3. Check streaming config: `cat /etc/netdata/stream.conf`
4. Check agent logs: `journalctl -u netdata | grep -i stream`

### Node shows Offline, expected Stale

**Cause:** Either it's a standalone Agent, or all its Parents are disconnected from Cloud.

**Check:**
1. Is this a standalone Agent or does it stream to a Parent?
2. If streaming: Is the Parent online and connected to Cloud?
3. Check Parent's dashboard—does it show the child?

### Node shows Unseen

**Cause:** Node was claimed but never successfully connected to Cloud.

**Check:**
1. Is the Netdata Agent running?
2. Can the agent reach `app.netdata.cloud`? Check firewall/proxy
3. Is the claiming token correct?
4. Check agent logs: `journalctl -u netdata | grep -i aclk`

### All children went Offline simultaneously

**Cause:** All Parents lost their Cloud connection. With HA setups (two Parents), this only happens if both disconnect.

**Check:**
1. Are the Parents online? `systemctl status netdata`
2. Can they reach Cloud? Check network
3. Check Parent logs: `journalctl -u netdata | grep -i aclk`

### Can't delete node from UI

**Cause:** Node is Stale (has data via Parent). UI prevents deletion to protect queryable data.

**Solution:** Use CLI to remove:
```bash
netdatacli remove-stale-node <node-id>
```

### Node reappears after deletion

**Cause:** Agent is still running and configured to reconnect.

**Solution:**
1. Stop the agent: `systemctl stop netdata`
2. Remove claim: `rm /var/lib/netdata/cloud.d/claimed_id`
3. Clear environment variables if set

## See Also

- [Node Types and Lifecycle](/docs/nodes-ephemerality.md) - Ephemeral vs permanent nodes
- [Remove Node](/docs/learn/remove-node.md) - Detailed removal instructions
- [Streaming Configuration](/docs/observability-centralization-points/metrics-centralization-points/README.md) - Parent-child setup
