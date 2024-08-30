### Understand the alert

This alert indicates that there are unavailable ranges in your CockroachDB cluster. Unavailable ranges occur when a majority of a range's replicas are on nodes that are unavailable. This can cause the entire range to be unable to process queries.

### Troubleshoot the alert

1. Check for dead or unavailable nodes

   Use the `./cockroach node status` command to list the status of all nodes in your cluster. Look for nodes that are marked as dead or unavailable and try to bring them back online.

   ```
   ./cockroach node status --certs-dir=<your_cert_directory>
   ```

2. Inspect the logs

   CockroachDB logs can provide valuable information about issues that may be affecting your cluster. Check the logs for errors or warnings related to unavailable ranges using `grep`:

   ```
   grep -i 'unavailable range' /path/to/cockroachdb/logs
   ```

3. Check replication factor

   Make sure your cluster's replication factor is set to an appropriate value. A higher replication factor can help tolerate node failures and prevent unavailable ranges. You can check the replication factor by running the following SQL query:

   ```
   SHOW CLUSTER SETTING kv.range_replicas;
   ```

   To set the replication factor, run the following SQL command:

   ```
   SET CLUSTER SETTING kv.range_replicas=<desired_replication_factor>;
   ```

4. Investigate and resolve network issues

   Network issues can cause nodes to become unavailable and lead to unavailable ranges. Check the status of your network and any firewalls, load balancers, or other network components that may be affecting connectivity between nodes.

5. Monitor and manage hardware resources

   Insufficient hardware resources, such as CPU, memory, or disk space, can cause nodes to become unavailable. Monitor your nodes' resource usage and ensure that they have adequate resources to handle the workload.

6. Consider rebalancing the cluster

   Rebalancing the cluster can help distribute the load more evenly across nodes and reduce the number of unavailable ranges. See the [CockroachDB documentation](https://www.cockroachlabs.com/docs/stable/demo-replication-and-rebalancing.html) for more information on manual rebalancing.

### Useful resources

1. [CockroachDB troubleshooting guide](https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges)
