### Understand the alert

This alert calculates the average latency (`ping round-trip time`) to a network host (${label:host}) over the last 10 seconds. If you receive this alert, it means there might be issues with your network connectivity or host responsiveness.

### What does latency mean?

Latency is the time it takes for a packet of data to travel from the sender to the receiver, and back from the receiver to the sender. In this case, we're measuring the latency using the `ping` command, which sends an ICMP echo request to the host and then waits for the ICMP echo reply.

### Troubleshoot the alert

1. Double-check the network connection:

   Verify the network connectivity between your system and the target host. Check if the host is accessible via other tools such as `traceroute` or `mtr`.

   ```
   traceroute ${label:host}
   mtr ${label:host}
   ```

2. Check for packet loss:

   Packet loss can make latency appear higher than it actually is. Use the `ping` command to check for packet loss:

   ```
   ping -c 10 ${label:host}
   ```

   Look for the percentage of packet loss in the output.

3. Investigate the host:

   If no packet loss is detected and the network connection is stable, the problem might be related to the host itself. Check the host for overloaded resources, such as high CPU usage, disk I/O, or network traffic.

4. Check DNS resolution:

   If the alert's `${label:host}` is a domain name, make sure that DNS resolution is working properly:

   ```
   nslookup ${label:host}
   ```

5. Verify firewall and routing:

   Check if any firewall rules or routing policies might be affecting the network traffic between your system and the target host.

### Useful resources

1. [Using Ping and Traceroute to troubleshoot network connectivity](https://support.cloudflare.com/hc/en-us/articles/200169336-Using-Ping-and-Traceroute-to-troubleshoot-network-connectivity)
