### Understand the alert

This alert calculates the average time between the reception of client `GET` requests and their subsequent responses in a `Riak KV` cluster over the last hour. If you receive this alert, it means that the average `GET` request latency in your Riak database has increased.

### What does mean latency mean?

Mean latency measures the average time taken between the start of a request and its completion, indicating the efficiency of the Riak system in processing `GET` requests. High mean latency implies slower processing times, which can negatively impact your application's performance.

### Troubleshoot the alert

- Check the system resources

1. High latency might be related to resource bottlenecks on your Riak nodes. Check CPU, memory, and disk usage using `top` or `htop` tools.
   ```
   top
   ```
   or
   ```
   htop
   ```
   
2. If you find any resource constraint, consider scaling your Riak cluster or optimize resource usage by tuning the application configurations.

- Investigate network issues

1. Networking problems between the Riak nodes or the client and the nodes could cause increased latency. Check for network performance issues using `ping` or `traceroute`.

   ```
   ping node_ip_address
   ```
   or
   ```
   traceroute node_ip_address
   ```

2. Investigate any anomalies or network congestion and address them accordingly.

- Analyze Riak KV configurations

1. Check Riak configuration settings, like read/write parameters and anti-entropy settings, for any misconfigurations.

2. Re-evaluate and optimize settings for performance based on your application requirements.

- Monitor application performance

1. Analyze your application's request patterns and workload. High request rates or large amounts of data being fetched can cause increased latency.

2. Optimize your application workload to reduce latency and distribute requests uniformly across the Riak nodes.

### Useful resources

1. [Riak KV documentation](https://riak.com/posts/technical/official-riak-kv-documentation-2.2/)
