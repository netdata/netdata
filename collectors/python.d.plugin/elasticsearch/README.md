# elasticsearch

This module monitors Elasticsearch performance and health metrics.

It produces:

1. **Search performance** charts:
 * Number of queries, fetches
 * Time spent on queries, fetches
 * Query and fetch latency

2. **Indexing performance** charts:
 * Number of documents indexed, index refreshes, flushes
 * Time spent on indexing, refreshing, flushing
 * Indexing and flushing latency

3. **Memory usage and garbace collection** charts:
 * JVM heap currently in use, committed
 * Count of garbage collections
 * Time spent on garbage collections

4. **Host metrics** charts:
 * Available file descriptors in percent
 * Opened HTTP connections
 * Cluster communication transport metrics

5. **Queues and rejections** charts:
 * Number of queued/rejected threads in thread pool

6. **Fielddata cache** charts:
 * Fielddata cache size
 * Fielddata evictions and circuit breaker tripped count

7. **Cluster health API** charts:
 * Cluster status
 * Nodes and tasks statistics
 * Shards statistics

8. **Cluster stats API** charts:
 * Nodes statistics
 * Query cache statistics
 * Docs statistics
 * Store statistics
 * Indices and shards statistics

### configuration

Sample:

```yaml
local:
  host               : 'ipaddress'    # Elasticsearch server ip address or hostname
  port               : 'port'         # Port on which elasticsearch listens
  cluster_health     :  True/False    # Calls to cluster health elasticsearch API. Enabled by default.
  cluster_stats      :  True/False    # Calls to cluster stats elasticsearch API. Enabled by default.
```

If no configuration is given, module will fail to run.

---
