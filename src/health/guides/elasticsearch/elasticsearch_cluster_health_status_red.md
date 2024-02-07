### Understand the alert

This alert is triggered when the Elasticsearch cluster health status turns `RED`. If you receive this alert, it means that there is a problem that needs immediate attention, such as data loss or one or more primary and replica shards are not allocated to the cluster.

### Elasticsearch Cluster Health Status

Elasticsearch cluster health status provides an indication of the cluster's overall health, based on the state of its shards. The status can be `green`, `yellow`, or `red`:

- `Green`: All primary and replica shards are allocated.
- `Yellow`: All primary shards are allocated, but some replica shards are not.
- `Red`: One or more primary shards are not allocated, leading to data loss.

### Troubleshoot the alert

1. Check the Elasticsearch cluster health using the `_cat` API:

```
curl -XGET 'http://localhost:9200/_cat/health?v'
```

Examine the output to understand the current health status, the number of nodes and shards, and any unassigned shards.

2. To get more details on the unassigned shards, use the `_cat/shards` API:

```
curl -XGET 'http://localhost:9200/_cat/shards?v'
```

Look for shards with the status `UNASSIGNED`.

3. Identify the root cause of the issue, such as:

   - A node has left the cluster or failed, causing the primary shard to become unassigned.
   - Insufficient disk space is available, preventing shards from being allocated.
   - Cluster settings or shard allocation settings are misconfigured.

4. Take appropriate action based on the root cause:

   - Ensure all Elasticsearch nodes are running and connected to the cluster.
   - Add more nodes or increase disk space as needed.
   - Review and correct cluster and shard allocation settings.

5. Monitor the health status as the cluster recovers:

```
curl -XGET 'http://localhost:9200/_cat/health?v'
```

If the health status turns `YELLOW` or `GREEN`, the cluster is no longer in the `RED` state.

### Useful resources

1. [Elasticsearch Cluster Health](https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-health.html)
2. [Fixing Elasticsearch Cluster Health Status "RED"](https://www.elastic.co/guide/en/elasticsearch/guide/current/_cluster_health.html)
3. [Elasticsearch Shard Allocation](https://www.elastic.co/guide/en/elasticsearch/reference/current/shards-allocation.html)