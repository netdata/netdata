# Netdata Parents (Streaming and Replication)

## What are they and why do we need them?

A “Parent” is a Netdata Agent, like the ones we install on all our systems, but is configured as a central node that receives, stores and processes metrics data from other Netdata “Child” nodes in our infrastructure.

Netdata Parents are flexible. You can have one big active-active cluster of Netdata Parents, or you can spread a lot of independent Parents across the infrastructure.

This “distributed still centralized” setup provides a lot of benefits. Let’s see them:

## Infrastructure-Level Dashboards: All Nodes in One Dashboard

A Parent node receives and aggregates metrics data from all child nodes that push metrics to it, presenting all of them on a single, centralized dashboard.

Metrics streaming between Netdata nodes is real-time and low-latency, so that the Parent can provide the same resolution and detail its children provide.

Each chart on the Parent’s dashboard is automatically turned into a multi-node chart, allowing instant aggregation of the data across the entire dashboard. This is transparent and automatic for all kinds of charts, even application-specific ones. For example, when you have 2 PostgreSQL servers in your infrastructure, the parent will present one set of charts for PostgreSQL and these charts will include data from both servers.

## Increased Data Retention: Store More, Learn More

Netdata’s database (`dbengine`), supports multiple tiers of variable resolution for storing metrics’ samples. Tier 0 is the high-resolution one and usually stores per second data. Tier 1 is the middle resolution one, downsampling data to per minute. Tier 2 is the low-resolution one, downsampling data to per hour. With this setup, a default Netdata setup is usually able to maintain 2-3 days of high resolution and up to a year of low-resolution data, all in less than 1 GB of disk space.

In many cases, however, organizations require a lot more retention than this. A Netdata Parent can be configured to have weeks or even months of high-resolution data and several years of low-resolution data for all its Child nodes, by allowing the Netdata database to grow to hundreds of GiBs or even several TiBs.

## Monitoring Ephemeral Nodes: No Node Left Behind

Production systems are often ephemeral by nature. In containerized and orchestrated environments, like Kubernetes, nodes may come and go due to scaling policies, maintenance tasks, or as part of regular operations.

Netdata Parents come to the rescue in such scenarios. They can continuously receive metrics from ephemeral nodes during their lifecycle. As these nodes are removed or replaced, the Parent retains their performance history, essentially archiving the life of each node.

The Netdata dashboards on the Parents automatically bring into the charts data from archived nodes when users pan the dashboard to the time-window these nodes were alive. This means that no data is lost and visibility is maintained across the entire lifespan of every node, regardless of its ephemeral nature.

## Unified Alerts Management: Silence the Noise

Each Netdata Agent is able to run health checks, trigger alerts and send notifications on its own. However, in a large-scale infrastructure with numerous nodes, each capable of generating alerts, managing these notifications can quickly become a challenge. Duplicate alerts and non-centralized management can lead to unnecessary noise, causing alert fatigue and possibly overlooking critical warnings.

Netdata Parents provide a solution to this problem. By configuring a Parent node to handle all alerts and health checks, and disabling health monitoring on the Child nodes, you centralize your alerts management, meaning that all alerts are now generated from a single place, reducing noise and ensuring that each unique issue only triggers a single notification.

In addition to making alert management more straightforward, this setup also allows for more refined control over your alert configurations. Instead of managing alert settings across multiple nodes, you can handle all configurations in one place, ensuring consistency and ease of management.

## Offloading Production Systems: Prioritize Performance

In a production environment, every bit of system resources is crucial. Minimizing the overhead due to monitoring and observability is vital to ensure optimal system performance. Although the Netdata Agent is designed to be lightweight and efficient, using a Netdata Parent can allow the Netdata Agents on your production systems to focus on the absolutely necessary for collecting metrics and pushing them to their Parent.

On your production systems, by configuring the Netdata Agents to use the `alloc` database mode with 5-10 minutes of retention time and disabling health monitoring and Machine Learning (ML) processing, you significantly reduce the system resources consumed by the monitoring system.

Netdata, with the `alloc` database mode, doesn't touch the disk at all (apart from logging - which can also be disabled). This approach eliminates any potential disk I/O impact from Netdata on your production applications, which could be particularly beneficial in I/O-sensitive environments.

## Fault Tolerance and Redundancy: Ensure Continuous Monitoring

Netdata Agents stream metrics to one Netdata Parent at a time. But more than one Parent can be configured on each child. The first available at any given time is used.

Similarly, Netdata Parents can be configured to stream/proxy the data they receive to another Netdata Parent. And they can support multiple Parents too, one of which will be used at any given time.

Configuration allows setting up a circular streaming setup. Parent A streams to Parent B and Parent B streams to Parent A. Child nodes are configured to stream to any of Parents A and B and they will automatically fall back and switch parents as necessary.

With the replication feature (enabled by default), all nodes replicate missing data on their Parent, before streaming live metrics, filling up any gap the Parent may have. 

The same setup can work for 2 or even more parents, to form an active-active multi-node cluster. Child nodes can connect to any of the parent nodes available and the parent nodes will automatically replicate and stream metrics to each other.

The setup is optimized even for wide-area connections between child nodes and parents, or for cases where the bandwidth between child nodes and parents has a cost associated with it. At any given time each child node sends its data only once. The parents then replicate and stream this data to each other.

## Security and Isolation: Protect Your Production Systems

Parent nodes can be set up in your organization's Demilitarized Zone (DMZ), acting as a protective barrier or application firewall, shielding your production Netdata agents from the outside world.

With Netdata Parents configured, the Netdata Agents running on your production systems need only one connection to these parents. They don’t need to run data queries, they will never send alert notifications, or even connect to Netdata Cloud.

Especially for Netdata Cloud, when the Parent node is connected to Netdata Cloud, it registers its Child nodes to it and can serve all functions required by the Cloud on behalf of the Child nodes. So, although only the parent is connected to Netdata Cloud, there is no difference in the user features you enjoy on Netdata Cloud in regard to your production systems. They will all be there.

## FAQ about Netdata Parents

### How much can a Parent node scale?

For about 1 million real-time metrics, with a default configuration:

- collected and streamed to it per second,
- stored in 3 database tiers (high, mid, low resolution),
- with ML training and anomaly detection running,
- health for alerts and notifications

And about 2 TiB of storage for metrics, you will need about 5-8 CPU cores and 32GiB of RAM. On such a setup you can have:

- 15 days of high resolution metrics
- 3 months of mid resolution metrics
- 1 year of low resolution metrics 

For such a setup, we recommend a 16 CPU cores system so that there is spare capacity for queries. More RAM and faster disks will give faster queries.

So, depending on the number of metrics per node you have and the size of your Parents, you may be able to aggregate 200 to 500 nodes per Parent.

### If I set up 2 active-active parents, will I be able to have more Child nodes stream to them?

No. When you set up an active-active cluster, even if child nodes connect randomly to one or the other, all the parent nodes receive all the metrics of all the child nodes. So, all of them do all the work.

There is a feature we currently work on, to allow Parent nodes to detect that they receive ML information with the streamed metric data (they receive it already but they ignore it), to prevent them from training their own ML models and running anomaly detection again for the child node. But this is not ready yet.

### How much retention do the child nodes need?

Child nodes need to have only the retention required in order to connect to another Parent if one fails or stops for maintenance.

- If you have an active-active cluster of parents, 5 to 10 minutes in `alloc` mode is enough.
- If you have only 1 parent, it would be better to run the child nodes with `dbengine` so that they will have enough retention to backfill the parent nodes if it stops for a few hours for maintenance.

### Does streaming between child nodes and parents support encryption?

Yes. You can configure your parent nodes to enable TLS and configure the child nodes to connect with TLS to it. The streaming connection is also compressed with LZ4 and this works even on top of TLS.

### Can I have an HTTP proxy between parent and child nodes?

No. The streaming protocol works on the same port as the internal web server of Netdata Agents, but the protocol is not HTTP-friendly and cannot be understood by HTTP proxy servers.

### Should I load balance the parents with a TCP load balancer?

Although this can be done and for streaming between child and parent nodes it could work, we recommend not doing it. It can lead to several kinds of problems.

It is better to configure all the parent nodes directly in the child nodes `stream.conf`. The child nodes will do everything in their power to find a parent node to connect and they will never give up.

### When I have an active-active cluster of parents, will I receive alert notifications from both of them?

If both are configured to run health checks and trigger alerts, yes.

We recommend using Netdata Cloud to avoid receiving duplicate alert notifications. Netdata Cloud deduplicates alert notifications so that you will receive them only once. On top of that, you can control silencing and routing directly from the Netdata Cloud UI.

### When I have only Parents connected to Netdata Cloud, will I be able to use the Functions feature on my child nodes?

Yes.

Functions is a feature of data collection plugins to expose functions that can be run from the dashboard to view more detailed information about a data collection. For example, apps.plugin exposes the processes function that returns a list of all the processes running, together with information about their CPU utilization, memory consumption, disk I/O operations, bandwidth, and a lot more.

When a parent receives a Function request, it forwards it to the plugin that exposes it. If the plugin is available over a streaming connection, the parent will forward the request to the socket it receives metrics from. This process will be repeated even if many parents are chained in order to reach the child.

### If I have a set of 2 active-active parents and get one out for maintenance for a few hours, will it have missing data when it returns back online?

There are 2 reasons you may have gaps in your data after you bring it back online:

1. Replication does not replicate metrics that are not actively collected. So, when the parent comes back, if there are samples that this parent does not have, for metrics that are not currently being collected, these samples will not be propagated to that parent. [We are working to fix this issue](https://github.com/netdata/netdata/issues/15198).
2. If the parent has been offline for a long time and the child nodes run in db mode `alloc`, you need to plan how you will bring this parent back online. Child nodes in this mode do not have enough retention to backfill the parent and if they connect to it before the other parent, you will end up with missing information on that parent.

The simplest way to solve this is to block at the firewall all connections to port 19999 from child nodes, but allow connections from the other parent nodes. Once replication finishes for all nodes, you can unblock the connections from child nodes to it.

### I got a parent out of maintenance but it replicates (backfills) missing data slowly. Can I speed it up?

Yes, there is a setting on `netdata.conf` under section `[db]` called `replication threads`. The default value is 1.

Usually, each thread is able to replicate about 2-5 million samples per second. We suggest setting this to 5 threads for all parents. Generally do not use too many threads because you are risking congesting the disks and/or the CPU cores available. Keep in mind that the sending parent needs this setting.

There is no need to increase this number on child nodes. Each node has one replication sender, so when hundreds of nodes are replicating to a parent, there are already a lot of senders pushing metrics to it.

### I have multiple active-active parents. Which one is used by Netdata Cloud for queries?

When you have multiple parents available, the one that is further away from the child node is used by Netdata Cloud, unless it does not have the data required.

This works like this: The child has `hops = 0`. Each parent receiving metrics for this child increases the `hops` by 1. So the first parent will have `hops = 1`, the second parent will have `hops = 2` and so on.

Netdata Cloud knows the retention of each parent. So, when it needs data from this child, it first checks the available retention each parent has for it and then it uses the parent with the higher `hops`. If no parent is available and the child node is directly connected to Netdata Cloud, it uses the child.

### Is there a way to balance child nodes to the parent nodes of an active-active cluster?

If you have 2 parent nodes A and B, you can configure them on half the child nodes as A, B, and the other half as B, A. The child nodes will connect to the first available (left to right). If both A and B are online, half of the child nodes will connect to A and the other half to B.

Keep in mind, however, that if you restart a parent, all the child nodes that were connected to it will automatically reconnect to the other parent. Once this happens, the child nodes will stay connected to it.

### Is there a way to get notified when a child gets disconnected?

There are 2 kinds of production nodes:
1. **Permanent nodes**  
   These are nodes that should be available permanently and if they disconnect an alert should be triggered to notify you.  
   By default, all nodes are considered permanent (not ephemeral).
2. **Ephemeral nodes**  
   These are nodes that are ephemeral by nature and they may shutdown at any point in time without any impact on the services you run.

To set the ephemeral flag on a node, edit its `netdata.conf` and in the `[health]` section set is `ephemeral = yes`. This setting is propagated to parent nodes and Netdata Cloud.

When using Netdata Cloud (via a parent or directly) and a permanent node gets disconnected, Netdata Cloud sends node disconnection notifications.
