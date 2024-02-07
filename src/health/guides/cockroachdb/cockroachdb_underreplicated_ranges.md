### Understand the alert

This alert is related to CockroachDB, a scalable and distributed SQL database. When you receive this alert, it means that there are under-replicated ranges in your database cluster. Under-replicated ranges can impact the availability and fault tolerance of your database, leading to potential data loss or unavailability in case of node failures.

### What are under-replicated ranges?

In a CockroachDB cluster, data is split into small chunks called ranges. These ranges are then replicated across multiple nodes to ensure fault tolerance and high availability. The desired replication factor determines the number of replicas for each range.

When a range has fewer replicas than the desired replication factor, it is considered as "under-replicated". This situation can occur if nodes are unavailable or if the cluster is in the process of recovering from failures.

### Troubleshoot the alert

1. Access the CockroachDB Admin UI

   Access the Admin UI by navigating to the URL `http://<any-node-ip>:8080` on any of your cluster nodes.

2. Check the 'Replication Status' in the dashboard

   In the Admin UI, check the 'Under-replicated Ranges' metric on the main 'Dashboard' or 'Metrics' page.

3. Inspect the logs of your CockroachDB nodes

   Look for any error messages or issues that could be causing under-replication. For example, you may see errors related to node failures or network issues.

4. Check cluster health and capacity

   Make sure that all nodes in the cluster are running and healthy. You can do this by running the command `cockroach node status`. Consider adding more nodes or increasing the capacity if your nodes are overworked.

5. Verify replication factor configuration

   Check your cluster's replication factor configuration to ensure it is set to an appropriate value. The default replication factor is 3, which can tolerate one failure. You can view and change it using the [`zone configurations`](https://www.cockroachlabs.com/docs/stable/configure-replication-zones.html).

6. Consider decommissioning problematic nodes

   If specific nodes are causing under-replication, consider decommissioning them to allow the cluster to automatically rebalance the ranges. Follow the [decommissioning guide](https://www.cockroachlabs.com/docs/stable/remove-nodes.html) in the CockroachDB documentation.

### Useful resources

1. [CockroachDB: Troubleshoot Under-replicated and Unavailable Ranges](https://www.cockroachlabs.com/docs/stable/cluster-setup-troubleshooting.html#db-console-shows-under-replicated-unavailable-ranges)
2. [CockroachDB: Configuring Replication Zones](https://www.cockroachlabs.com/docs/stable/configure-replication-zones.html)
3. [CockroachDB: Decommission a Node](https://www.cockroachlabs.com/docs/stable/remove-nodes.html)