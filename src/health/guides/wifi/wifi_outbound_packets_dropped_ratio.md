### Understand the alert

The `wifi_outbound_packets_dropped_ratio` alert indicates that a significant number of packets were dropped on the way to transmission over the last 10 minutes. This could be due to a lack of resources or other issues with the network interface.

### What does dropped packets mean?

Dropped packets refer to data packets that are discarded by a network interface instead of being transmitted through the network. This can occur for various reasons such as hardware failures, lack of resources (e.g., memory, processing power), or network congestion.

### Troubleshoot the alert

1. Check interface statistics

Use the `ifconfig` command to view information about your network interfaces, including their packet drop rates. Look for the dropped packets count in the TX (transmit) section.

```bash
ifconfig <interface_name>
```

Replace `<interface_name>` with the name of the network interface you are investigating, such as `wlan0` for a wireless interface.

2. Check system logs

System logs can provide valuable information about any potential issues. Check the logs for any errors or warnings related to the network interface or driver.

For example, use `dmesg` command to display kernel messages:

```bash
dmesg | grep -i "<interface_name>"
```

Replace `<interface_name>` with the name of the network interface you are investigating.

3. Check for hardware issues

Inspect the network interface for any signs of hardware failure or malfunction. This may include damaged cables, loose connections, or issues with other networking equipment (e.g. switches, routers).

4. Monitor network congestion

High packet drop rates can be caused by network congestion. Monitor network usage and performance using tools such as `iftop`, `nload`, or `vnstat`. Identify and address any traffic bottlenecks or excessive usage.

5. Update network drivers

Outdated or faulty network drivers may cause packet drop issues. Check for driver updates and install any available updates following the manufacturer's instructions.

6. Optimize network settings

You can adjust network settings, like buffers or queues, to mitigate dropped packets. Consult your operating system or network device documentation for specific recommendations on adjusting these settings.

### Useful resources

1. [ifconfig command in Linux](https://www.geeksforgeeks.org/ifconfig-command-in-linux-with-examples/)
2. [nload – Monitor Network Traffic and Bandwidth Usage in Real Time](https://www.tecmint.com/nload-monitor-linux-network-traffic-bandwidth-usage/)