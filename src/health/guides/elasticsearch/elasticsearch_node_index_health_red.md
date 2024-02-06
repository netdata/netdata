### Understand the alert

This alert is triggered when the health status of an Elasticsearch node index turns `red`. If you receive this alert, it means that at least one primary shard and its replicas are not allocated to any node, and the data in the index is potentially at risk.

### What does a red index health status mean?

In Elasticsearch, the index health status can be green, yellow, or red:

- Green: All primary and replica shards are allocated and active.
- Yellow: All primary shards are active, but not all replicas are allocated due to the lack of available nodes.
- Red: At least one primary shard and its replicas are not allocated, which means the cluster can't serve all the incoming data, and data loss is possible.

### Troubleshoot the alert

1. Check the cluster health

   Use the Elasticsearch `_cluster/health` endpoint to check the health status of your cluster:
   ```
   curl -X GET "localhost:9200/_cluster/health?pretty"
   ```

2. Identify the unassigned shards

    Use the Elasticsearch `_cat/shards` endpoint to view the status of all shards in your cluster:
    ```
    curl -X GET "localhost:9200/_cat/shards?h=index,shard,prirep,state,unassigned.reason&pretty"
    ```

3. Check Elasticsearch logs

    Examine the Elasticsearch logs for any error messages or alerts related to shard allocation. The log file is usually located at `/var/log/elasticsearch/`.

4. Resolve shard allocation issues

    Depending on the cause of the unassigned shards, you may need to perform actions such as:
    
    - Add more nodes to the cluster to distribute the load evenly.
    - Reallocate shards manually using the Elasticsearch `_cluster/reroute` API.
    - Adjust shard allocation settings in the Elasticsearch `elasticsearch.yml` configuration file.

5. Recheck the cluster health

    After addressing the issues found in the previous steps, use the `_cluster/health` endpoint again to check if the health status of the affected index has improved.

### Useful resources

1. [Elasticsearch: Cluster Health](https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-health.html)
2. [Elasticsearch: Shards and Replicas](https://www.elastic.co/guide/en/elasticsearch/reference/current/_basic_concepts.html#shards-and-replicas)
3. [Elasticsearch: Shard Allocation and Cluster-Level Settings](https://www.elastic.co/guide/en/elasticsearch/reference/current/shards-allocation.html)