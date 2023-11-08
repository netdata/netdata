### Understand the alert

This alert triggers when there is a `leadership transition` in the `Consul` service mesh. If you receive this alert, it means that server `${label:node_name}` in datacenter `${label:datacenter}` has become the new leader.

### What does consul_raft_leadership_transitions mean?

Consul is a service mesh solution that provides service discovery, configuration, and segmentation functionality. It uses the Raft consensus algorithm to maintain a consistent data state across the cluster. A leadership transition occurs when the current leader node loses its leadership status and a different node takes over.

### What causes leadership transitions?

Leadership transitions in Consul can be caused by various reasons, such as:

1. Network communication issues between the nodes.
2. High resource utilization on the leader node, causing it to miss heartbeat messages.
3. Nodes crashing or being intentionally shut down.
4. A forced leadership transition triggered by an operator.

Frequent leadership transitions may lead to service disruptions, increased latency, and reduced availability. Therefore, it's essential to identify and resolve the root cause promptly.

### Troubleshoot the alert

1. Check the Consul logs for indications of network issues or node failures:

   ```
   journalctl -u consul.service
   ```
   Alternatively, you can check the Consul log file, which is usually located at `/var/log/consul/consul.log`.

2. Inspect the health and status of the Consul cluster using the `consul members` command:

   ```
   consul members
   ```
   This command lists all cluster members and their roles, including the new leader node.

3. Determine if there's high resource usage on the affected nodes by monitoring CPU, memory, and disk usage:

   ```
   top
   ```

4. Examine network connectivity between nodes using tools like `ping`, `traceroute`, or `mtr`.

5. If the transitions are forced by operators, review the changes made and their impact on the cluster.

6. Consider increasing the heartbeat timeout configuration to allow the leader more time to respond, especially if high resource usage is causing frequent leadership transitions.

7. Review Consul's documentation on [consensus and leadership](https://developer.hashicorp.com/consul/docs/architecture/consensus) and [operation and maintenance](https://developer.hashicorp.com/consul/docs/guides) to gain insights into best practices and ways to mitigate leadership transitions.

### Useful resources

1. [Consul: Service Mesh Overview](https://www.consul.io/docs/intro)
2. [Consul: Understanding Consensus and Leadership](https://developer.hashicorp.com/consul/docs/architecture/consensus)
3. [Consul: Installation, Configuration, and Maintenance](https://developer.hashicorp.com/consul/docs/guides)
