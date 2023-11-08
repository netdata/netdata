### Understand the alert

This alert calculates the average number of packets received by the network interface `${label:device}` over the period of one minute. If you receive this alert, it means that the rate of received packets has significantly increased, which could indicate a potential network bottleneck or an increased network workload.

### What does received packets rate mean?

`Received packets rate` represents the speed at which packets are arriving at the network interface of your machine. A packet is a single unit of data transmitted over the network. A high rate of received packets indicates that your network is under significant workload as it processes incoming data.

### Troubleshoot the alert

1. Monitor your network traffic

   Use the `iftop` command to get a real-time report of bandwidth usage on your network interfaces:
   ```
   sudo iftop -i ${label:device}
   ```
   If you don't have `iftop` installed, install it using your package manager.

2. Identify the top consumers of network bandwidth

   Inspect the output of `iftop` to identify if any IP addresses or hosts are using an unusual amount of bandwidth. This can help you pinpoint any sudden surges in network traffic caused by specific services or applications.

3. Check for possible network congestion

   Determine if the high received packets rate is caused by network congestion. Network congestion occurs when the volume of data being transmitted exceeds the available capacity of the network. You can use `ping` or `traceroute` commands to check for latency and packet loss.

4. Examine your application logs

   Investigate your application logs to identify any unusual activity or network spikes. This can provide valuable information about potential issues, such as a sudden increase in incoming client connections, improperly optimized application configurations, or the presence of malicious traffic.

5. Optimize your network configuration

   Review your networking configurations to ensure they are optimized for the current workload. Check for any misconfigurations or resource limitations that might be causing the high received packets rate. You might consider increasing the maximum number of open file descriptors, changing your network driver settings, or adjusting your network buffer sizes.

### Useful resources

1. [Iftop Guide – Monitor Network Bandwidth](https://www.tecmint.com/iftop-linux-network-bandwidth-monitoring-tool/)
