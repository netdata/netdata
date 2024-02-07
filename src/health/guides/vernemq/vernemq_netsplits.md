### Understand the alert

This alert indicates that your VerneMQ cluster has experienced a netsplit (split-brain) situation within the last minute. This can lead to inconsistencies in the cluster, and you need to troubleshoot the problem to maintain proper cluster operation.

### What is a netsplit?

In distributed systems, a netsplit occurs when a cluster of nodes loses connectivity to one or more nodes due to a network failure, leaving the cluster to operate in a degraded state. In the context of VerneMQ, a netsplit can lead to inconsistencies in the subscription data and retained messages.

### Troubleshoot the alert

- Confirm the alert issue

  Review the VerneMQ logs to check for any signs of network partitioning or netsplits.

- Check connectivity between nodes

  Ensure that the network connectivity between your cluster nodes is restored. You can use tools like `ping` and `traceroute` to verify network connectivity.

- Inspect node status

  Use the `vmq-admin cluster show` command to inspect the current status of the nodes in the VerneMQ cluster, and check for any disconnected nodes:

  ```
  vmq-admin cluster show
  ```

- Reestablish connections and heal partitions

  If a node is disconnected, reconnect it using the `vmq-admin cluster join` command:

  ```
  vmq-admin cluster join discovery-node=IP_ADDRESS_OF_ANOTHER_NODE
  ```

  As soon as the partition is healed, and connectivity is reestablished, the VerneMQ nodes will replicate the latest changes made to the subscription data.

- Ensure node connectivity remains active

  Monitor the cluster and network to maintain consistent connectivity between the nodes. Set up monitoring tools and consider using an auto-healing or auto-scaling framework to help maintain node connectivity.

### Useful resources

1. [VerneMQ Clustering Guide: Netsplits](https://docs.vernemq.com/v/master/vernemq-clustering/netsplits)
2. [VerneMQ Documentation](https://docs.vernemq.com/)
