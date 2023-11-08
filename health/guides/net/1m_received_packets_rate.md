### Understand the alert

1m_received_packets_rate alert indicates the average number of packets received by the network interface on your system over the last minute. If you receive this alert, it signifies higher than usual network traffic incoming.

### What do received packets mean?

A received packet is a unit of data that is transmitted through the network interface to your system. Higher received packets rate means an increase in incoming network traffic to your system. It could be due to legitimate usage or could signal a potential issue such as a network misconfiguration, an attack, or a system malfunction.

### Troubleshoot the alert

1. Analyze the network throughput: Use the `nload` or `iftop` command to check the incoming traffic on your system's network interfaces. These commands display the current network traffic and will help you monitor the incoming data.

   ```
   sudo nload <network_interface>  // or
   sudo iftop -i <network_interface>
   ```

   Replace `<network_interface>` with your network interface (e.g., eth0).

2. Check for specific processes consuming unusually high network bandwidth: Use the `netstat` command combined with `grep` to filter the results and find processes with high network traffic.

   ```
   sudo netstat -tunap | grep <network_interface>
   ```

   Replace `<network_interface>` with your network interface (e.g., eth0).

3. Identify host-consuming bandwidth: After identifying the processes consuming a high network, you can trace back their respective hosts. Use the `tcpdump` command to capture live network traffic and analyze it for specific IP addresses causing the high packets rate.

   ```
   sudo tcpdump -n -i <network_interface> -c 100
   ```

   Replace `<network_interface>` with your network interface (e.g., eth0).

4. Mitigate the issue: Depending on the root cause, apply appropriate remedial actions. This may include:
   - Adjusting application/service configuration to reduce network traffic
   - Updating firewall rules to block undesired sources/IPs
   - Ensuring network devices are appropriately configured
   - Addressing system overload issues that hamper network performance

### Useful resources

1. [nload - Monitor Linux Network Traffic and Bandwidth Usage in Real Time](https://www.tecmint.com/nload-monitor-linux-network-traffic-bandwidth-usage/)
2. [An Introduction to the ss Command](http://www.binarytides.com/linux-ss-command/)
