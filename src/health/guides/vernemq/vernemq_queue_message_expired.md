### Understand the alert

This alert is related to VerneMQ, a scalable and open-source MQTT broker. The `vernemq_queue_message_expired` alert indicates that there is a high number of expired messages that could not be delivered in the last minute.

### What does message expiration mean?

In MQTT, messages are kept in queues until they are delivered to their respective subscribers. Sometimes, messages might have a specific lifespan given by the Time to Live (TTL) attribute, and if they are not delivered within this time, they expire.

Expired messages are removed from the queue and are not delivered to subscribers. This usually means that clients are unable to process the incoming messages fast enough, putting the VerneMQ system under stress.

### Troubleshoot the alert

1. **Check VerneMQ status**: Use the `vernemq` command along with the `vmq-admin` tool to monitor the status of your VerneMQ broker:

   ```
   sudo vmq-admin cluster show
   ```

   Analyze the output to make sure that the cluster is up and running without issues.

2. **Check the message rate and throughput**: You can use the `vmq-admin metrics show` command to display key metrics related to your VerneMQ cluster:

   ```
   sudo vmq-admin metrics show
   ```

   Analyze the output and identify any sudden increase in the message rate or unusual rate of message expiration.

3. **Identify slow or malfunctioning clients**: VerneMQ provides a command to list all clients connected to the cluster. You can use the following command to identify slow or malfunctioning clients:

   ```
   sudo vmq-admin session show
   ```

   Check the output for clients who have a high amount of queue delay, low queued messages, or are not receiving messages properly.

4. **Optimize client connections**: Increasing the message TTL or decreasing the message rate can help decrease the number of expired messages. Adjust the client settings accordingly, ensuring they match the application requirements.

5. **Ensure proper resource allocation**: Check whether the VerneMQ broker has enough resources by monitoring CPU, memory, and disk usage using tools like `top`1, `vmstat`, or `iotop`.

6. **Check VerneMQ logs**: VerneMQ logs can provide valuable insight into the underlying issue. Check the logs for any relevant error messages or warnings:

   ```
   sudo tail -f /var/log/vernemq/console.log
   sudo tail -f /var/log/vernemq/error.log
   ```

7. **Monitor Netdata charts**: Monitor Netdata's VerneMQ dashboard to gain more insight into the behavior of your MQTT broker over time. Look for spikes in the number of expired messages, slow message delivery, or increasing message queues.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
