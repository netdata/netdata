# elasticsearch

This module monitors [Elasticsearch](https://www.elastic.co/products/elasticsearch) performance and health metrics.

It produces:

1.  **Search performance** charts:

    -   Number of queries, fetches
    -   Time spent on queries, fetches
    -   Query and fetch latency

2.  **Indexing performance** charts:

    -   Number of documents indexed, index refreshes, flushes
    -   Time spent on indexing, refreshing, flushing
    -   Indexing and flushing latency

3.  **Memory usage and garbace collection** charts:

    -   JVM heap currently in use, committed
    -   Count of garbage collections
    -   Time spent on garbage collections

4.  **Host metrics** charts:

    -   Available file descriptors in percent
    -   Opened HTTP connections
    -   Cluster communication transport metrics

5.  **Queues and rejections** charts:

    -   Number of queued/rejected threads in thread pool

6.  **Fielddata cache** charts:

    -   Fielddata cache size
    -   Fielddata evictions and circuit breaker tripped count

7.  **Cluster health API** charts:

    -   Cluster status
    -   Nodes and tasks statistics
    -   Shards statistics

8.  **Cluster stats API** charts:

    -   Nodes statistics
    -   Query cache statistics
    -   Docs statistics
    -   Store statistics
    -   Indices and shards statistics

9.  **Indices** charts (per index statistics, disabled by default):

    -   Docs count
    -   Store size
    -   Num of replicas
    -   Health status

## configuration

Edit the `python.d/elasticsearch.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different, if different
sudo ./edit-config python.d/elasticsearch.conf
```

Sample:

```yaml
local:
  host               : 'ipaddress'    # Elasticsearch server ip address or hostname.
  port               : 'port'         # Port on which elasticsearch listens.
  node_status        :  yes/no        # Get metrics from "/_nodes/_local/stats". Enabled by default.
  cluster_health     :  yes/no        # Get metrics from "/_cluster/health". Enabled by default.
  cluster_stats      :  yes/no        # Get metrics from "'/_cluster/stats". Enabled by default.
  indices_stats      :  yes/no        # Get metrics from "/_cat/indices". Disabled by default.
```

If no configuration is given, module will try to connect to `http://127.0.0.1:9200`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Felasticsearch%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
