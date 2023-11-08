### Understand the alert

This alert indicates that your system is experiencing high IPv4 TCP socket utilization, specifically orphaned sockets. Orphaned connections are those not attached to any user file handle. When these connections exceed the limit, they are reset immediately. The warning state is triggered when the percentage of used orphan IPv4 TCP sockets exceeds 25%, and the critical state is triggered when the value exceeds 50%.

### Troubleshoot the alert

- Check the current orphan socket usage

To check the number of orphan sockets in your system, run the following command:

   ```
   cat /proc/sys/net/ipv4/tcp_max_orphans
   ```

- Identify the processes causing high orphan socket usage

To identify the processes causing high orphan socket usage, you can use the `ss` command:

   ```
   sudo ss -tan state time-wait state close-wait
   ```

   Look for connections with a large number of orphan sockets and investigate the related processes.

- Increase the orphan socket limit

If you need to increase the orphan socket limit to accommodate legitimate connections, you can update the value in the `/proc/sys/net/ipv4/tcp_max_orphans` file. Replace `{DESIRED_AMOUNT}` with the new limit:

   ```
   echo {DESIRED_AMOUNT} > /proc/sys/net/ipv4/tcp_max_orphans
   ```

   Consider the kernel's penalty factor for orphan sockets (usually 2x or 4x) when determining the appropriate limit.

   **Note**: Be cautious when making system changes and ensure you understand the implications of updating these settings.

- Review and optimize application behavior

Investigate the applications generating a high number of orphan sockets and consider optimizing their behavior. This may involve updating application settings or code to better manage network connections.

- Monitor your system

Keep an eye on your system's orphan socket usage, particularly during peak hours. Adjust the limit as needed to accommodate legitimate connections.

### Useful resources

1. [Network Sockets](https://en.wikipedia.org/wiki/Network_socket)
2. [Linux-admins.com - Troubleshooting Out of Socket Memory](http://www.linux-admins.net/2013/01/troubleshooting-out-of-socket-memory.html)