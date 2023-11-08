### Understand the alert

This alert checks the ratio of the average number of received packets for a network interface over the last 10 seconds, compared to the rate over the last minute. If the rate of received packets increases significantly over a short period, it may indicate a packet storm, which can impact network performance and connectivity.

### What is a packet storm?

A packet storm is a sudden increase in network traffic due to a large number of packets being sent simultaneously. This can cause network congestion, packet loss, and increased latency, leading to a degradation of network performance and potential loss of connectivity.

### Troubleshoot the alert

1. Identify the affected interface and examine its traffic patterns:

Use `iftop` or a similar monitoring tool to view real-time network traffic on the affected interface.

```
sudo iftop -i <interface_name>
```

Replace `<interface_name>` with the name of the network interface experiencing the packet storm (e.g., eth0).

2. Check for possible packet flood sources:

Examine logs, firewall rules, and traffic patterns for evidence of a Denial of Service (DoS) attack or a misconfigured application. Use tools like `tcpdump` or `wireshark` to capture network packets and analyze traffic.

3. Limit or block unwanted traffic:

Apply traffic shaping or Quality of Service (QoS) policies, firewall rules, or Intrusion Prevention System (IPS) to limit or block the sources of unwanted traffic.

4. Monitor network performance:

Continuously monitor network performance to ensure the issue is resolved and prevent future packet storms. Use monitoring tools like Netdata to keep track of network performance metrics.

### Useful resources

1. [iftop: Linux Network Bandwidth Monitoring Tool](https://www.tecmint.com/iftop-linux-network-bandwidth-monitoring-tool/)
2. [tcpdump: A powerful command-line packet analyzer](https://www.tcpdump.org/)
3. [Wireshark: A network protocol analyzer for UNIX and Windows](https://www.wireshark.org/)
