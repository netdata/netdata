### Understand the alert

This alert checks if a particular TCP service on a specified host and port is reachable. If the average percentage of successful checks within the last minute is below 75%, it triggers an alert indicating the TCP service is not functioning properly.

### Troubleshoot the alert

- Verify if the problem is network-related or service-related

  1. Check if the host and port are correct and the service is configured to listen on that specific port.

  2. Use `ping` or `traceroute` to diagnose the connectivity issues between your machine and the host.

  3. Use `telnet` or `nc` to check if the specific port on the host is reachable. For example, `telnet example.com port_number` or `nc example.com port_number`.

  4. Check the network configuration, firewall settings, and routing rules on both the local machine and the target host.

- Check if the TCP service is running and functioning properly

  1. Check the service logs for any errors or issues that may prevent it from working correctly.

  2. Restart the service and monitor its behavior.

  3. Investigate if there are any recent changes in the service configuration or updates that may cause the issue.

  4. Monitor system resources such as CPU, memory, and disk usage to ensure they are not causing any performance bottlenecks.

- Optimize the service configuration

  1. Review the service's performance-related configurations and fine-tune them, if necessary.

  2. Check if there are any optimizations or best practices that can be applied to boost the service performance and reliability.

