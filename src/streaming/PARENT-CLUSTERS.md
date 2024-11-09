## Notes on Netdata Active-Active Parent Clusters

### Streaming Connection Overview

Each Netdata Child has `[stream].destination` configuration in stream.conf to define the parents it should stream to.

Netdata Children connect to one parent at a time. When multiple parents are defined in `[stream].destination`, Netdata Children will select one of them to connect to. If this connection is not possible for any reason, they will connect to the next, until they find a working one. When the whole list of Netdata Parents has been tried without a successful connection, Netdata Children will restart trying the list from the beginning.

Once Netdata Parents have their children connected, they now become children themselves, to propagate the data they receive to their Netdata Parents (grandparents).

### Active-Active Parent Clusters Overview

Active-active parent clusters are configured with circular propagation of the data. So parent A has parent B as grandparent, and parent B has parent A as grandparent.

This setup allows both parents to have the same data for all their children and the children are free to connect to any of the two parents.

This method works for any number of 2+ parents, as long as all the parents have all the others as grandparents.

### Replication Overview

When a child Netdata Child connects to a Netdata Parent, they first enter a negotiation phase. The child announces the metrics it wants to stream, including the retention it has for them. The parent checks its own database for missing information. If there are samples missing, it first asks the children to replicate past data, before streaming fresh data.

This replication happens per instance (group of metrics). So, while some metrics are replicated, others are streaming fresh data.

Replication is only triggered when fresh data are collected. Children nodes do not announce archived metrics they may have. Archived metrics will be replicated only when they start being collected again.

Only `tier0` (high-resolution) samples are replicated (higher tiers are always derived from `tier0`), this means that the retention of `tier0` on children is critical in ensuring the parents will not have gaps in their databases.

## Challenges

### A new parent in an active-active cluster

Adding a new parent to the cluster is a challenge for 2 reasons:

#### How to get all the data of the existing parent replicated to the new one?

The running parents have both fresh and archived metrics. However replication will propagate only the currently collected ones, ignoring the archived ones. This means that the new parent will be missing metrics of stopped containers, disconnected disks or network interfaces, etc.

The only viable solution today, is to copy the database of another parent to the new one. By copying the files in `/var/cache/netdata` to the new parent and then starting it up, all the existing data will copied to the new server. This copy of all files can be done while the existing parent runs (hot-copy), but it requires using `rsync` to copy the dbengine files (these files are append-only, so it is safe to copy them while they are used) and `sqlite3` to dump the SQLite3 databases while they are hot.

There is still a (smaller) window for missing data in the new parent, especially on systems with significant ephemerality. Repeating the copy a couple of times and starting the parent immediately after, can minimize it.

####  How to make sure than children do not connect to the new parent before it finishes replicating old data?

When adding a new parent in a cluster, it is important not to reconfigure the children to use it, until the new parent has finished replicating old data.

Network disruptions while a new parent is synchronizing can lead to missing data in that parent. For example, if a child connects to the new parent before this parent has finished replicating with the others, the parent will ask from the child to replicate a much larger duration, which the child may not have. This will leave a gaps in the new parent's database.

In Netdata 2.1+ (or 2.0 nightly) we added a balancing feature in Netdata Children. This balancing allows the children to query their parents for their retention, before connecting to them, and to prefer the parents having more recent data. However, the children will still connect to the first available parent (starting from the one having most recent data), which means that in case of network disruptions they may still end up connecting prematurely to the new parent.

The only viable solution is to have a parent join a cluster without offering this parent to the children. Once the new parent finishes replicating, then the children can be reconfigured to use the new parent too.

### Resources on the Nodes of a Cluster

CPU and memory resources consumption on parents, depend on 3 activities:

#### Ingestion rate

All nodes in a cluster will always ingest all the metrics from all children streaming to any of them.

So, the compute resources required on all nodes in a cluster is balanced and equal.

#### **Machine learning**

This is the most CPU intensive task in a cluster, and it also has a noticeable memory impact.

Since Netdata 1.45, anomaly information embedded in the samples is propagated across parent nodes, so that even with machine learning disabled, parents can still provide anomaly information for all metrics, using anomaly detection performed at the child.

However, prior to Netdata 2.1, when machine learning was enabled on parents, all nodes in a cluster had to train their own machine learning data. So, significant compute resources were spend on all parents on a cluster, for training machine learning for all the children (of all the parents), multiplying the computational resources required to run machine learning.

With the release of Netdata 2.1, the first Netdata Agent (child or parent) that trains machine learning data for the metrics of a node, propagates this machine learning data forward (to the next nodes in a chain), reducing the computational resources on parent clusters significantly.

With this feature, users have the option:

1. Train machine learning at the edge (children), so that vertical scalability on the parents can be maximized.
2. Train machine learning at the parents, but this time only the first parent (who will receive the direct connection from the child) will train ML for this child. The other parents will receive a copy of it from the parent that is training it.

Practically, the resources for machine learning, prior to Netdata 2.1 were `N` where `N` is the number of nodes in a cluster. Starting with Netdata 2.1 they are `1` (so just one of the parents spends the resources for machine learning).

#### Restreaming rate

Propagating samples to another parent requires significant resources too. This includes formatting the messages, compressing the traffic and sending it.

Generally, the best vertical scalability on a parent can be achieved with machine learning and grandparent streaming disabled (standalone, no clustering, no grandparent).

When parents are in a cluster the restreaming resources required will be `N - 1` where `N` is the number of parents in the cluster. So, the grandparent of each child node, will not use any restreaming resources.

### Balancing Parent

Netdata v2.1 introduces a balancing algorithm for child nodes.

#### Initial balancing

Before connecting to a parent, child nodes execute an API query to each candidate parent to receive information about their retention on the parents.

The goal is to prefer parent nodes that have the most latest data. However, since parents will always have slight differences in retention time, children consider parents equal, if they have less than 2 minutes of difference in retention.

When parents are considered equal, each child picks a parent randomly.

This logic can be influenced by network disruptions. So, on every disconnection and depending on the reason of disconnection, children block the same parent for a few seconds (and some randomness), to avoid bombarding it with requests. This logic may interfere with the best parent selection.

#### Rebalancing after cluster node changes

TBD
