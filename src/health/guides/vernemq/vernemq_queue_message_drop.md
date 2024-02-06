### Understand the alert

This alert monitors the number of dropped messages in VerneMQ due to full message queues within the last minute. If you receive this alert, it means that message queues are full and VerneMQ is dropping messages. This can be a result of slow consumers, slow VerneMQ performance, or fast publishers.

### Troubleshoot the alert

1. Check the message queue length and performance metrics of VerneMQ

   Monitor the current message queue length for each topic by using the command:

   ```
   vmq-admin metrics show | grep queue | sort | uniq -c
   ```

   You can also monitor VerneMQ performance metrics like CPU utilization, memory usage, and network I/O by using the `top` command:

   ```
   top
   ```

2. Identify slow consumers, slow VerneMQ, or fast publishers

   Analyze the message flow and performance data to determine if the issue is caused by slow consumers, slow VerneMQ performance, or fast publishers.

   - Slow Consumers: If you identify slow consumers, consider optimizing their processing capabilities or scaling them to handle more load.
   - Slow VerneMQ: If VerneMQ itself is slow, consider optimizing its configuration, increasing resources, or scaling the nodes in the cluster.
   - Fast Publishers: If fast publishers are causing the issue, consider rate-limiting them or breaking their input into smaller chunks.

3. Increase the queue length or adjust max_online_messages

   If increasing the capacity of your infrastructure is not a viable solution, consider increasing the queue length or adjusting the `max_online_messages` value in VerneMQ. This can help mitigate the issue of dropped messages due to full queues.

   Update the VerneMQ configuration file (`vernemq.conf`) to set the desired `max_online_messages` value:

   ```
   max_online_messages=<your_desired_value>
   ```

   Then, restart VerneMQ to apply the changes:

   ```
   sudo service vernemq restart
   ```

4. Monitor the situation

   Continue to monitor the message queue length and VerneMQ performance metrics after making changes, to ensure that the issue is resolved or mitigated.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
2. [Understanding and Monitoring VerneMQ Metrics](https://docs.vernemq.com/monitoring/introduction)
3. [VerneMQ Configuration Guide](https://docs.vernemq.com/configuration/introduction)