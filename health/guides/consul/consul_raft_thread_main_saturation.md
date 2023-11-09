### Understand the alert

This alert triggers when the main Raft goroutine's saturation percentage reaches a certain threshold. If you receive this alert, it means that your Consul server is experiencing high utilization of the main Raft goroutine.

### What is Consul?

Consul is a service discovery, configuration, and orchestration solution developed by HashiCorp. It is used in microservice architectures and distributed systems to make services aware and discoverable by other services. Raft is a consensus-based algorithm used for maintaining the state of the Consul servers.

### What is the main Raft goroutine?

The main Raft goroutine is responsible for carrying out consensus-related tasks in the Consul server. It ensures the consistency and reliability of the server's state. High saturation of this goroutine can lead to performance issues in the Consul server cluster.

### Troubleshoot the alert

1. Verify the current status of the Consul server.
   Check the health status and logs of the Consul server using the following command:
   ```
   consul monitor
   ```

2. Monitor Raft metrics.
   Use the Consul telemetry feature to collect and analyze Raft performance metrics. Consult the [Consul official documentation](https://www.consul.io/docs/agent/telemetry) on setting up telemetry.

3. Review the server's resources.
   Confirm whether the server hosting the Consul service has enough resources (CPU, memory, and disk space) to handle the current load. Upgrade the server resources or adjust the Consul configurations accordingly.

4. Inspect the Consul server's log files.
   Analyze the log files to identify any errors or issues that could be affecting the performance of the main Raft goroutine.

5. Monitor network latency between Consul servers.
   High network latency can affect the performance of the Raft algorithm. Use monitoring tools like `ping` or `traceroute` to measure the latency between the Consul servers.

6. Check for disruptions in the Consul cluster.
   Investigate possible disruptions caused by external factors, such as server failures, network partitioning or misconfigurations in the cluster.

### Useful resources

1. [Consul: Service Mesh for Microservices Networking](https://www.consul.io/)
2. [Consul Documentation](https://www.consul.io/docs)
3. [Consul Telemetry](https://www.consul.io/docs/agent/telemetry)
4. [Understanding Raft Consensus Algorithm](https://raft.github.io/)
