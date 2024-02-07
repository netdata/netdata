### Understand the alert

This alert is related to the percentage of used IPv4 TCP connections. If you receive this alert, it means that your system has high TCP connections utilization, and you might be approaching the limit of maximum connections.

### What does high IPv4 TCP connections utilization mean?

When the number of IPv4 TCP connections gets too high, the system's ability to establish new connections decreases. This is because there are limitations due to resources such as memory or system settings. High utilization could lead to connection-related issues or service interruptions.

### Troubleshoot the alert

1. Check current TCP connections:

   To see the current number of TCP connections, you can use the `ss` or `netstat` command:

   ```
   ss -t | grep ESTAB | wc -l
   ```

   or

   ```
   netstat -ant | grep ESTABLISHED | wc -l
   ```

2. Identify connections with high usage:

   To list the connections with their state (e.g., ESTABLISHED, LISTEN), use the following command:

   ```
   ss -tan
   ```

   Look for connections with a high number of ESTABLISHED connections, as these may be contributing to the high utilization.

3. Inspect running processes to identify potential culprits:

   You can use the `lsof` command to list all open files and the processes that are using them:

   ```
   sudo lsof -iTCP
   ```

   Look for processes with a high number of open files, as these are likely responsible for the increased TCP connections utilization.

4. Take action:

   Once you have identified the processes contributing to high TCP connections utilization, you can take appropriate action. This may involve optimizing the application, adjusting system settings, or optimizing hardware resources.

### Useful resources

1. [Linux lsof command tutorial](https://www.howtoforge.com/linux-lsof-command/)
