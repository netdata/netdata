### Understand the alert

This alert is related to VerneMQ, which is an MQTT broker. The Netdata Agent calculates the average VerneMQ's scheduler utilization over the last 10 minutes. If you receive this alert, it means your VerneMQ scheduler's utilization is high, which may indicate performance issues or resource constraints.

### What does scheduler utilization mean?

VerneMQ uses schedulers to manage its tasks and processes. In this context, scheduler utilization represents the degree to which the VerneMQ schedulers are being used. High scheduler utilization may cause delays in processing tasks, leading to performance degradation and possibly affecting the proper functioning of the MQTT broker.

### Troubleshoot the alert

- Verify the VerneMQ scheduler utilization

1. To check the scheduler utilization, you can use the `vmq-admin` command like this:

   ```
   vmq-admin metrics show | grep scheduler
   ```

   This command will display the scheduler utilization percentage.

- Analyze the VerneMQ MQTT traffic

1. To analyze the MQTT traffic, use the `vmq-admin` `session` and `client` subcommands. These can give you insights into the current subscription and client status:

   ```
   vmq-admin session show
   vmq-admin client show
   ```

   This can help you identify if there is any abnormal activity or an increase in the number of clients or subscriptions that may be affecting the scheduler's performance.

- Evaluate VerneMQ system resources

1. Assess CPU and memory usage of the VerneMQ process using the `top` or `htop` commands:

   ```
   top -p $(pgrep -f vernemq)
   ```

   This will show you the CPU and memory usage for the VerneMQ process. If the process is consuming too many resources, it might be affecting the scheduler's utilization.

2. Evaluate the system's available resources (CPU, memory, and I/O) using commands like `vmstat`, `free`, and `iostat`.

   ```
   vmstat
   free
   iostat
   ```

   These commands can help you understand if your system's resources are nearing their limits or if there are any bottlenecks affecting the overall performance.

3. Check the VerneMQ logs for any errors or warnings. The default location for VerneMQ logs is `/var/log/vernemq`. Look for messages that may indicate issues affecting the scheduler's performance.

- Optimize VerneMQ performance or adjust resources

1. If the MQTT traffic is high or has increased recently, consider scaling up your VerneMQ instance by adding more resources (CPU or memory) or by distributing the load across multiple nodes.

2. If your system resources are limited, consider optimizing your VerneMQ configuration to improve performance. Some example options include adjusting the `max_online_messages`, `max_inflight_messages`, or `queue_deliver_mode`.

3. If the alert persists even after evaluating and making changes to the above steps, consult the VerneMQ documentation or community for further assistance.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
2. [VerneMQAdministration Guide](https://vernemq.com/docs/administration/)
3. [VerneMQ Configuration Guide](https://vernemq.com/docs/configuration/)