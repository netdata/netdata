### Understand the alert

The `elasticsearch_cluster_health_status_yellow` alert triggers when the Elasticsearch cluster's health status is `yellow` for longer than 10 minutes. This may indicate potential issues in the cluster, like unassigned or missing replicas. The alert class is `Errors`, and the type is `SearchEngine`.

### What does the health status mean?

In Elasticsearch, cluster health status can be one of three colors:

- Green: All primary shards and replicas are active and properly assigned to each index.
- Yellow: All primary shards are active, but one or more replicas are unassigned or missing.
- Red: One or more primary shards are unassigned or missing.

### Troubleshoot the alert

1. Check the Elasticsearch cluster health.

You can check the health of the Elasticsearch cluster using the `/_cluster/health` API endpoint:

```
curl -XGET 'http://localhost:9200/_cluster/health?pretty'
```

2. Identify the unassigned or missing replicas.

You can check for any unassigned or missing shards using the `/_cat/shards` API endpoint:

```
curl -XGET 'http://localhost:9200/_cat/shards?v&h=index,shard,prirep,state'
```

3. Check Elasticsearch logs for any errors or warnings:

```
sudo journalctl --unit elasticsearch
```

4. Check disk space on all Elasticsearch nodes. Insufficient disk space may lead to unassigned or missing replicas:

```
df -h
```

5. Ensure Elasticsearch is properly configured.

Check the `elasticsearch.yml` configuration file on all nodes for any misconfigurations or errors:

```
sudo nano /etc/elasticsearch/elasticsearch.yml
```

6. Review the Elasticsearch documentation on [Cluster-Level Shard Allocation and Routing Settings](https://www.elastic.co/guide/en/elasticsearch/reference/current/allocation-awareness.html) to understand how to properly assign and balance shards.

### Useful resources

1. [Elasticsearch Cluster Health](https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-health.html)
2. [Elasticsearch Shards](https://www.elastic.co/guide/en/elasticsearch/reference/current/cat-shards.html)
3. [Allocation Awareness in Elasticsearch](https://www.elastic.co/guide/en/elasticsearch/reference/current/allocation-awareness.html)