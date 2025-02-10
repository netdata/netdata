
# Nodes Ephemerality in Netdata

## Overview

In distributed monitoring environments, maintaining a reliable and consistent observability system is crucial. Netdata v2.23 introduces significant improvements to how ephemeral nodes are managed, ensuring a better balance between alerting consistency and flexibility for transient infrastructure.

Previously, ephemeral nodes were defined as "nodes that are forgotten a day after they last disconnect." This approach sometimes led to unexpected inconsistencies in monitoring, particularly for users operating highly dynamic environments. With v2.23, ephemeral nodes are now defined as "nodes that are expected to disconnect without alerts being raised."

This change serves three key objectives:

1.  **Stronger Monitoring Consistency for Permanent Nodes**: By ensuring that only permanent nodes trigger disconnection alerts, users can focus on real operational issues without being overwhelmed by alert noise.

2.  **Enhanced Flexibility for Transient Environments**: Users managing auto-scaling cloud instances, containers, and other volatile infrastructure can now configure nodes as ephemeral, preventing unnecessary alerts and making monitoring more effective.

3.  **Automated Cleanup for Ephemeral Nodes**: Netdata provides an automated way for the monitoring system to clean up itself by "forgetting" ephemeral nodes after a defined period. By default, the retention period is determined by the parent nodes' data retention settings. However, given that Netdata's tiered storage may provide retention for months or years, users may configure a shorter expiration time for ephemeral nodes.

By introducing these changes, Netdata significantly enhances its ability to monitor itself as a mesh-like distributed observability system, ensuring that alerts reflect actual system health rather than expected, routine disconnections. Additionally, the automatic cleanup feature prevents stale ephemeral nodes from accumulating in the monitoring system, keeping dashboards clean and up-to-date.

## Understanding Ephemeral Nodes

When it comes to ephemerality, Netdata supports 2 types of nodes:

-   **Ephemeral Nodes**: nodes that are expected to disconnect and/or reconnect frequently, or nodes that are expected to shut down or vanish at any point in time. Such nodes may be:

    -   Auto-scaling cloud instances.
    -   Containers and VMs that are created and destroyed dynamically.
    -   IoT devices with intermittent connectivity.
    -   Development/test environments where nodes frequently restart.
-   **Permanent Nodes**: nodes that are expected to always be online, and disconnections are a strong indication of some kind of failure that operations teams should be aware of.


## Configuring Ephemeral Nodes

By default, all nodes in Netdata are **permanent**. Users can mark nodes as ephemeral like this:

At the `netdata.conf` of the ephemeral node, set:

```ini
[global]
   is ephemeral node = yes

```

And restart the node. This ephemerality flag is propagated to Netdata Parents and Netdata Cloud via the `_is_ephemeral` host label (boolean: true/false).

## Netdata Parents Alerts

Netdata v2.23 introduces two alerts for **permanent** nodes:

-   `streaming_never_connected`: Counts the number of **permanent** nodes never connected to a Netdata Parent (since its last restart) and transitions to WARNING when this number is non-zero.
-   `streaming_disconnected`: Counts the number of **permanent** nodes that have been connected but are now disconnected from the Netdata Parent, and transitions to WARNING when this number is non-zero.

To identify the exact nodes that trigger these alarms, use the `Netdata-streaming` function under the `Top` tab of the dashboard. This Netdata Function presents a list (a table) of all nodes known to a Netdata Parent and provides detailed state information for the lifecycle of the node, including database status, ingestion status, streaming status, health and alerts status, and more.

In this table, red lines indicate a problem during ingestion, and yellow lines indicate a problem during (re)streaming. Colored lines are only related to **permanent** nodes. Filter the table using `Ephemerality` by selecting `permanent` and use the table columns `InStatus`, `InReason`, and `InAge` to understand the ingestion issue at hand. Similarly, for (re)streaming to another Netdata Parent, use `OutStatus`, `OutReason`, and `OutAge`.

### How to Mark Archived Nodes as Ephemeral

In case there are **permanent** nodes that are no longer available, in order to clear the alerts, the following command must be run on each of the Netdata Parents having these alerts raised:

```sh
netdatacli mark-stale-nodes-ephemeral ALL_NODES

```

This command instructs Netdata to mark as **ephemeral** all the nodes not currently online.

Keep in mind that nodes will be marked again as **permanent** if they reconnect and they have not been configured in their `netdata.conf` to be **ephemeral**. So, marking them at the parents is only useful for nodes that are not expected to connect again.

## Netdata Cloud Alerts

Before Netdata v2.23, Netdata Cloud was sending node unreachable notifications for all nodes, independently of their ephemerality.

Since Netdata v2.23, Netdata Cloud is sending node unreachable notifications only for **permanent** nodes.

## Automatically "Forgetting" Ephemeral Nodes

Netdata versions prior to v2.23 were automatically "forgetting" ephemeral nodes if they disconnected for more than 1 day. In Netdata v2.23+, this feature is now **disabled** by default.

To enable it again, set this in `netdata.conf` of the Netdata Parents that are expected to "forget" the ephemeral nodes:

```ini
[db]
   cleanup ephemeral hosts after = 1d

```

The above instructs the Netdata Parent to automatically "forget" ephemeral nodes 1 day after they disconnect. When a node is "forgotten," its data is no longer available for queries, and when all parents reporting the node to Netdata Cloud "forget" it, Netdata Cloud automatically deletes the node.