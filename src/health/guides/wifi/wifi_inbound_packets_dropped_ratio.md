### Understand the alert

This alert calculates the ratio of inbound dropped packets for a specific network interface over the last 10 minutes. If you receive this alert, it means that your WiFi network interface dropped a significant number of packets, which could be due to lack of resources or unsupported protocol.

### What does "inbound dropped packets" mean?

In the context of networking, "inbound dropped packets" means that packets were received by the network interface but were not processed. This can happen due to various reasons, including:

1. Insufficient resources (e.g., CPU, memory) to handle the packet.
2. Unsupported protocol.
3. Network congestion, leading to packets being dropped.
4. Hardware or configuration issues.

### Troubleshoot the alert

- Check the system resource utilization

Using the `top` command, check the resource utilization (CPU, memory, and I/O) in your system. High resource usage might indicate that your system is struggling to process the incoming packets.

```
top
```

- Inspect network configuration and hardware

1. Check if there are any hardware issues or misconfigurations in your WiFi adapter or network interface. Refer to your hardware's documentation or manufacturer's support for troubleshooting steps.
 
2. Make sure your network device drivers are up-to-date.

- Monitor network traffic

Use the `iftop` command to monitor network traffic on your interface. High network traffic can cause congestion, leading to dropped packets. If you don't have it installed, follow the [installation instructions](https://www.tecmint.com/iftop-linux-network-bandwidth-monitoring-tool/).

```
sudo iftop -i <interface_name>
```

- Investigate network protocols

Inbound dropped packets may be caused by unsupported network protocols. Use the `tcpdump` command to examine network traffic for any abnormalities or unknown protocols.

```
sudo tcpdump -i <interface_name>
```

### Useful resources

1. [Top 20 Netstat Command Examples in Linux](https://www.tecmint.com/20-netstat-commands-for-linux-network-management/)
2. [iftop command in Linux to monitor network traffic](https://www.tecmint.com/iftop-linux-network-bandwidth-monitoring-tool/)

Remember to replace `<interface_name>` with the actual name of the WiFi network interface causing the alert.