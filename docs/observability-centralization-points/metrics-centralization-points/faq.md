# FAQ on Metrics Centralization Points

## How much can a Netdata Parent node scale?

Netdata Parents generally scale well. According [to our tests](https://blog.netdata.cloud/netdata-vs-prometheus-performance-analysis/) Netdata Parents scale better than Prometheus for the same workload: -35% CPU utilization, -49% Memory Consumption, -12% Network Bandwidth, -98% Disk I/O, -75% Disk footprint.

For more information, Check [Sizing Netdata Parents](/docs/observability-centralization-points/metrics-centralization-points/sizing-netdata-parents.md).

## If I set up a parents cluster, will I be able to have more Child nodes stream to them?

No. When you set up an active-active cluster, even if child nodes connect randomly to one or the other, all the parent nodes receive all the metrics of all the child nodes. So, all of them do all the work.

## How much retention do the child nodes need?

Child nodes need to have only the retention required to connect to another Parent if one fails or stops for maintenance.

- If you have a cluster of parents, 5 to 10 minutes in `alloc` mode is usually enough.
- If you have only one parent, it would be better to run the child nodes with `dbengine` so that they will have enough retention to back-fill the parent node if it stops for maintenance.

## Does streaming between child nodes and parents support encryption?

Yes. You can configure your parent nodes to enable TLS at their web server and configure the child nodes to connect with TLS to it. The streaming connection is also compressed, on top of TLS.

## Can I have an HTTP proxy between parent and child nodes?

No. The streaming protocol works on the same port as the internal web server of Netdata Agents, but the protocol is not HTTP-friendly and cannot be understood by HTTP proxy servers.

## Should I load balance multiple parents with a TCP load balancer?

Although this can be done and for streaming between child and parent nodes it could work, we recommend not doing it. It can lead to several kinds of problems.

It is better to configure all the parent nodes directly in the child nodes `stream.conf`. The child nodes will do everything in their power to find a parent node to connect, and they will never give up.

## When I have multiple parents for the same children, will I receive alert notifications from all of them?

If all parents are configured to run health checks and trigger alerts, yes.

We recommend using Netdata Cloud to avoid receiving duplicate alert notifications. Netdata Cloud deduplicates alert notifications so that you will receive them only once.

## When I have only Parents connected to Netdata Cloud, will I be able to use the Functions feature on my child nodes?

Yes. Function requests will be received by the Parents and forwarded to the Child via their streaming connection. Function requests are propagated between parents, so this will work even if multiple levels of Netdata Parents are involved.

## If I have a cluster of parents and get one out for maintenance for a few hours, will it have missing data when it returns online?

Check [Restoring a Netdata Parent after maintenance](/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md).

## I have a cluster of parents. Which one is used by Netdata Cloud?

When there are multiple data sources for the same node, Netdata Cloud follows this strategy:

1. Netdata Cloud prefers Netdata Agents having `live` data.
2. For time-series queries, when multiple Netdata Agents have the retention required to answer the query, Netdata Cloud prefers the one that is further away from production systems.
3. For Functions, Netdata Cloud prefers Netdata Agents that are closer to the production systems.

## Is there a way to balance child nodes to the parent nodes of a cluster?

Yes. When configuring the Parents at the Children `stream.conf`, configure them in different order. Children get connected to the first Parent they find available, so if the order given to them is different, they will spread the connections to the Parents available.

## Is there a way to get notified when a child gets disconnected?

It depends on the ephemerality setting of each Netdata Child.

1. **Permanent nodes**: These are nodes that should be available permanently and if they disconnect, an alert should be triggered to notify you. By default, all nodes are considered permanent (not ephemeral).

2. **Ephemeral nodes**: These are nodes that are ephemeral by nature, and they may shut down at any point in time without any impact on the services you run.

To set the ephemeral flag on a node, edit its netdata.conf and in the `[global]` section set `is ephemeral node = yes`. This setting is propagated to parent nodes and Netdata Cloud.

A parent node tracks connections and disconnections. When a node is marked as ephemeral and stops connecting for more than 24 hours, the parent will delete it from its memory and local administration, and tell Cloud that it is no longer live nor stale. Data for the node can no longer be accessed, but if the node connects again later, the node will be "revived", and previous data becomes available again.

A node can be forced into this "forgotten" state with the Netdata CLI tool on the parent the node is connected to (if still connected) or one of the parent Agents it was previously connected to. The state will be propagated _upwards_ and _sideways_ in case of an HA setup.

```
netdatacli remove-stale-node <node_id | machine_guid | hostname | ALL_NODES>
```

When using Netdata Cloud (via a parent or directly), and a permanent node gets disconnected, Netdata Cloud sends node disconnection notifications.
