### Understand the alert

This alert is triggered when the number of not normal v5 DISCONNECT packets received by VerneMQ in the last minute is above a certain threshold. This indicates that there is an issue with MQTT clients connecting to your VerneMQ MQTT broker that requires attention.

### What does not normal mean?

In the context of this alert, "not normal" refers to v5 DISCONNECT packets that were received with a reason code other than "normal disconnection", as specified in the MQTT v5 protocol. Normal disconnection refers to clients disconnecting gracefully without any issues.

### Troubleshoot the alert

1. Inspect VerneMQ logs

   Check the VerneMQ logs for any relevant information about the MQTT clients that are experiencing not normal disconnects. This can provide important context to identify the root cause of the issue.

   ```
   sudo journalctl -u vernemq
   ```

2. Check the MQTT clients

   Investigate the MQTT clients that are experiencing not normal disconnects. This may involve inspecting client logs or usage patterns, as well as verifying that the clients are using the correct MQTT version (v5) and have the appropriate configurations.

3. Monitor VerneMQ metrics

   Use the VerneMQ metrics to monitor the broker's performance and identify any sudden spikes in abnormal disconnects or other relevant metrics.

   To view the VerneMQ metrics, access the VerneMQ admin interface, usually available at `http://<your_vernemq_address>:8888/metrics`.

4. Review network conditions

   Verify that there are no networking issues between the MQTT clients and the VerneMQ MQTT broker, as these issues could cause MQTT clients to disconnect unexpectedly.

5. Review VerneMQ configuration

   Review your VerneMQ configuration to ensure it is correctly set up to handle the expected MQTT client load and usage patterns.

### Useful resources

1. [VerneMQ documentation](https://vernemq.com/docs/)
2. [MQTT v5 specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html)
