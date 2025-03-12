# Nodes Ephemerality in Netdata

## Overview

Netdata v2.3.0 changes how ephemeral nodes are defined and managed in distributed monitoring environments. This update enhances monitoring reliability while providing flexibility for dynamic infrastructure management.

**Key Changes**:

Netdata now defines ephemeral nodes as "nodes that are expected to disconnect without raising alerts," replacing the previous definition of forgotten nodes after one day of disconnection. This change provides three major benefits:

1. **Improved Permanent Node Monitoring**: Disconnection alerts are triggered only for permanent nodes, reducing alert noise and helping teams focus on genuine operational issues.
2. **Better Support for Dynamic Infrastructure**: Organizations using auto-scaling cloud instances, containers, and other dynamic resources can now designate nodes as ephemeral, preventing unnecessary alerts.
3. **Automated Node Management**: The system automatically removes ephemeral nodes based on configurable retention periods, maintaining clean and relevant monitoring dashboards.

## Node Types

Netdata supports two types of nodes:

| Type      | Description                                          | Common Examples                                                                                                                                                             |
|-----------|------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Ephemeral | Nodes expected to disconnect or reconnect frequently | • Auto-scaling cloud instances<br/>• Dynamic containers and VMs<br/>• IoT devices with intermittent connectivity<br/>• Development/test environments with frequent restarts |
| Permanent | Nodes expected to maintain continuous connectivity   | • Production servers<br/>• Core infrastructure nodes<br/>• Critical monitoring systems<br/>• Stable database servers                                                        |

> **Note**: Disconnections in permanent nodes indicate potential system failures requiring immediate attention.

## Setting Up Ephemeral Nodes

By default, Netdata treats all nodes as permanent. To mark a node as ephemeral:

1. Open `netdata.conf` on the target node
2. Add the following configuration:
   ```ini
   [global]
     is ephemeral node = yes
   ```
3. Restart the node

This configuration sets the `_is_ephemeral` host label which propagates to Netdata Parents and Netdata Cloud.

## Alerts: Parent Node Alerts

Netdata v2.3.0 adds [two alerts](https://github.com/netdata/netdata/blob/master/src/health/health.d/streaming.conf) specifically for permanent nodes:

| Alert                     | Triggers                                                      |
|---------------------------|---------------------------------------------------------------|
| streaming_never_connected | When permanent nodes have never connected to a Netdata Parent |
| streaming_disconnected    | When previously connected permanent nodes disconnect          |

## Monitoring Child Node Status

To investigate alert:

1. Navigate to the `Top` tab in your dashboard
2. Select the `Netdata-streaming` function
3. Review the detailed node status table:
    - Red lines: Node connection problems (when nodes attempt to connect to this Parent)
    - Yellow lines: Restreaming issues (when this Parent attempts to stream data to other Parent nodes)
    - Color highlighting applies only to permanent nodes
    - Filter by `Ephemerality` to focus on permanent nodes
    - Use `InStatus`, `InReason`, and `InAge` columns to analyze node connections to the parent node
    - Use `OutStatus`, `OutReason`, and `OutAge` columns to analyze this Parent's restreaming to other Parent nodes

## Managing Archived Nodes

To clear alerts for permanently offline nodes:

```bash
netdatacli mark-stale-nodes-ephemeral <node_id | machine_guid | hostname | ALL_NODES>
```

> **Note**: Nodes will revert to permanent status if they reconnect unless configured as ephemeral in their `netdata.conf`.

## Cloud Integration

Starting with v2.3.0, Netdata Cloud sends node-unreachable notifications **exclusively for permanent nodes**, improving alert relevance.

## Automatic Ephemeral Nodes Cleanup

The automatic removal of disconnected ephemeral nodes is disabled by default in v2.3.0+. To enable this feature:

1. Edit the `netdata.conf` file on Netdata Parent nodes
2. Add the following configuration:

   ```ini
   [db]
     cleanup ephemeral hosts after = 1d
   ```
3. Restart the node

This setting removes ephemeral nodes from queries 24 hours after disconnection. When all parent nodes remove a node, Netdata Cloud automatically deletes it too.
