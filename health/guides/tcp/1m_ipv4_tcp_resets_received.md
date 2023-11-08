### Understand the alert

This alert, `1m_ipv4_tcp_resets_received`, calculates the average number of TCP RESETS received (`AttemptFails`) over the last minute on your system. If you receive this alert, it means that there is an increase in the number of TCP RESETS, which might indicate a problem with your networked applications or servers.

### What does TCP RESET mean?

`TCP RESET` is a signal that is sent from one connection end to the other when an ongoing connection is immediately terminated without an orderly close. This usually happens when a networked application encounters an issue, such as an incorrect connection request, invalid data packet, or a closed port.

### Troubleshoot the alert

1. Identify the top consumers of TCP RESETS:

   You can use the `ss` utility to list the TCP sockets and their states:
   
   ```
   sudo ss -tan
   ```

   Look for the `State` column to see which sockets have a `CLOSE-WAIT`, `FIN-WAIT`, `TIME-WAIT`, or `LAST-ACK` status. These states usually have a high number of TCP RESETS.

2. Check the logs of the concerned applications:

   If you have identified the problematic applications or servers, inspect their logs for any error messages, warnings, or unusual activity related to network connection issues.

3. Inspect the system logs:

   Check the system logs, such as `/var/log/syslog` on Linux or `/var/log/system.log` on FreeBSD, for any network-related issues. This could help you find possible reasons for the increased number of TCP RESETS.

4. Monitor and diagnose network issues:

   Use tools like `tcpdump`, `wireshark`, or `iftop` to capture packets and observe network traffic. This can help you identify patterns that may be causing the increased number of TCP RESETS.

5. Check for resource constraints:

   Ensure that your system's resources, such as CPU, memory, and disk space, are not under heavy load or reaching their limits. High resource usage could cause networked applications to behave unexpectedly, resulting in an increased number of TCP RESETS.

### Useful resources

1. [ss Utility - Investigate Network Connections & Sockets](https://www.binarytides.com/linux-ss-command/)
2. [Wireshark - A Network Protocol Analyzer](https://www.wireshark.org/)
3. [Monitoring Network Traffic with iftop](https://www.tecmint.com/iftop-linux-network-bandwidth-monitoring-tool/)
