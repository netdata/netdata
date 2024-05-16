### Understand the alert

This alert is related to VerneMQ, a high-performance MQTT message broker. It monitors the number of unexpected PUBCOMP (publish complete) packets received in the last minute. If you receive this alert, it means there's an issue with the MQTT message flow between clients and the broker, which might lead to data inconsistencies.

### What are PUBCOMP packets?

In MQTT, the PUBCOMP packet is used when QoS (Quality of Service) 2 is applied. It's the fourth and final packet in the four-packet flow to ensure that messages are delivered exactly once. An unexpected PUBCOMP packet means that the client or the broker received a PUBCOMP packet that it didn't expect in the message flow, which can cause issues in processing the message correctly.

### Troubleshoot the alert

1. Inspect the VerneMQ logs: Check the VerneMQ logs for any error messages or unusual activity that could indicate a problem with the message flow. By default, VerneMQ logs are located in `/var/log/vernemq/`, but this might be different for your system.
   
   ```
   sudo tail -f /var/log/vernemq/console.log
   sudo tail -f /var/log/vernemq/error.log
   ```

2. Identify problematic clients: Inspect the MQTT client logs to identify which clients are causing the unexpected PUBCOMP packets. Some MQTT client libraries provide logging features, while others might require debugging or setting a higher log level.

3. Check QoS settings: Ensure that the clients and the MQTT broker have the same QoS settings to avoid inconsistencies in the four-packet flow.

4. Monitor the VerneMQ metrics: Use Netdata or other monitoring tools to keep an eye on MQTT message flows and observe any anomalies that require further investigation.

5. Update client libraries and VerneMQ: Ensure that all MQTT client libraries and the VerneMQ server are up-to-date to avoid any incompatibilities or bugs that could lead to unexpected behavior.

### Useful resources

1. [VerneMQ Documentation](https://docs.vernemq.com/)
2. [MQTT Specification - MQTT Control Packets](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901046)
