### Understand the alert

This alert is related to VerneMQ, an MQTT message broker. If you receive this alert, it means that an increasing number of unsuccessful v5 PUBACK packets have been sent in the last minute.

### What does "unsuccessful v5 PUBACK" mean?

In the MQTT protocol, when a client sends a Publish message with a Quality of Service (QoS) level 1, the message broker sends a PUBACK packet to acknowledge receipt of the message. However, MQTT v5 has added a reason code field in the PUBACK packet, allowing brokers to report any issues or errors that occurred during message delivery. An "unsuccessful v5 PUBACK" refers to a PUBACK packet that reports a delivery problem or issue.

### Troubleshoot the alert

1. Check VerneMQ logs for possible errors or warnings: VerneMQ logs can provide valuable insights into the broker's runtime behavior, including connection issues or problems with authentication/authorization. Look for errors or warnings in the logs that could indicate the cause of the unsuccessful PUBACK packets.

   ```
   sudo journalctl -u vernemq
   ```

2. Verify client connections: Connection issues can be a possible cause of unsuccessful PUBACK packets. Use the `vmq-admin session show` command to view the client connections, and check for any abnormal behavior (e.g., frequent disconnects and reconnects).

   ```
   sudo vmq-admin session show
   ```

3. Check MQTT client logs: Review the logs from the devices that connect to your VerneMQ broker instance to verify if they encounter any issues or errors when sending messages.

4. Monitor the broker's resources usage: High system load or insufficient resources may affect VerneMQ's performance and prevent it from processing PUBACK packets as expected. Use monitoring tools like `top` and `iotop` to observe CPU and I/O usage, and assess whether the broker has enough resources to handle the MQTT traffic.

5. Update VerneMQ configuration: Double-check your VerneMQ settings for any misconfiguration related to QoS, message storage, or security policies that could prevent PUBACK packets from being sent or processed successfully.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
2. [MQTT Version 5 Features](https://www.hivemq.com/blog/mqtt-5-foundational-changes-in-the-protocol/)
