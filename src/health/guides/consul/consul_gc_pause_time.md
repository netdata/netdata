### Understand the alert

This alert calculates the time spent in stop-the-world garbage collection (GC) pauses on a Consul server node within a one-minute interval. Consul is a distributed service mesh software providing service discovery, configuration, and segmentation functionality. If you receive this alert, it means that the Consul server is experiencing an increased amount of time in GC pauses, which may lead to performance degradation of your service mesh.

### What are garbage collection pauses?

Garbage collection (GC) in Consul is a mechanism to clean up unused memory resources and improve the overall system performance. During a GC pause, all running processes in Consul server are stopped to allow the garbage collection process to complete. If the duration of GC pauses is too high, it indicates that the Consul server might be under memory pressure, which can affect the overall performance of the system.

### Troubleshoot the alert

1. **Check the Consul server logs**: Examine the Consul server's logs for any errors or warnings related to memory pressure, increased heap usage, or GC pauses. You can typically find the logs in `/var/log/consul`.

2. **Monitor Consul server metrics**: Check the Consul server's memory usage, heap usage and GC pause metrics using or Netdata. This can help you identify the cause of increased GC pause time.

3. **Optimize Consul server configuration**: Ensure that your Consul server is properly configured based on your system resources and workload. Review and adjust the [Consul server configuration parameters](https://www.consul.io/docs/agent/options) as needed.

4. **Reduce memory pressure**: If you have identified memory pressure as the root cause, consider adding more memory resources to your Consul server or adjusting the Consul server's memory limits.

5. **Update Consul server**: Make sure that your Consul server is running the latest version, which can include optimizations and performance improvements.

### Useful resources

- [Consul Server Configuration Parameters](https://www.consul.io/docs/agent/options)
