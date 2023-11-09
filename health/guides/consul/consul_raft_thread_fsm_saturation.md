### Understand the alert

This alert monitors the `consul_raft_thread_fsm_saturation` metric, which represents the saturation of the `FSM Raft` goroutine in Consul, a service mesh. If you receive this alert, it indicates that the Raft goroutine on a specific Consul server is becoming saturated.

### What is Consul?

Consul is a distributed service mesh that provides a full-featured control plane with service discovery, configuration, and segmentation functionalities. It enables organizations to build and operate large-scale, dynamic, and resilient systems. The Raft FSM goroutine is responsible for executing finite state machine (FSM) operations on the Consul servers.

### What does FSM Raft goroutine saturation mean?

Saturation of the FSM Raft goroutine means that it is spending more time executing operations, which may cause delays in Consul's ability to process requests and manage the overall service mesh. High saturation levels can lead to performance issues, increased latency, or even downtime for your Consul deployment.

### Troubleshoot the alert

1. Identify the Consul server and datacenter with the high Raft goroutine saturation:

   The alert has labels `label:node_name` and `label:datacenter`, indicating the affected Consul server and its respective datacenter.

2. Examine Consul server logs:

   Check the logs of the affected Consul server for any error messages or indications of high resource usage. This can provide valuable information on the cause of the saturation.

3. Monitor Consul cluster performance:

   Use Consul's built-in monitoring tools to keep an eye on your Consul cluster's health and performance. For instance, you may monitor Raft metrics via the Consul `/v1/agent/metrics` API endpoint.

4. Scale your Consul infrastructure:

   If the increased saturation is due to high demand, scaling your Consul infrastructure by adding more servers or increasing the resources available to existing servers can help mitigate the issue.

5. Review and optimize Consul configuration:

   Review your Consul configuration and make any necessary optimizations to ensure the best performance. For instance, you could adjust the [Raft read and write timeouts](https://www.consul.io/docs/agent/options).

6. Investigate and resolve any underlying issues causing the saturation:

   Look for any factors contributing to the increased load on the FSM Raft goroutine and address those issues. This may involve reviewing application workloads, network latency, or hardware limitations.

### Useful resources

1. [Consul Telemetry](https://www.consul.io/docs/agent/telemetry)
2. [Consul Configuration - Raft](https://www.consul.io/docs/agent/options#raft)
