### Understand the alert

This alert is related to VerneMQ, a high-performance MQTT broker. It monitors the number of unexpected v3/v5 PUBACK packets received in the last minute. If you receive this alert, it means that there are more PUBACK packets received than expected, which could indicate an issue with your MQTT broker or your MQTT client application(s).

### What are PUBACK packets?

In MQTT (Message Queuing Telemetry Transport) protocol, PUBACK packets are acknowledgement packets sent by the MQTT broker to confirm the receipt of a PUBLISH message with QoS (Quality of Service) level 1. The MQTT client will wait for this acknowledgment packet before it can continue with the next transaction.

### Troubleshoot the alert

1. Check VerneMQ logs for any unusual events, errors, or issues that could be related to the PUBACK packets. The VerneMQ logs can be found in `/var/log/vernemq` by default, or any custom location defined in the configuration file.

   ```
   sudo tail -f /var/log/vernemq/console.log
   ```

2. Investigate your MQTT client application(s) to ensure they are handling the PUBLISH messages correctly and not causing duplicate or unexpected PUBACK packets. You can use an MQTT client library that supports QoS level 1 to eliminate the possibility of custom code not following the MQTT protocol properly.

3. Monitor your MQTT broker and client application(s) for any network connectivity issues that could cause unexpected PUBACK packets. You can use tools like `ping` and `traceroute` to check the network connectivity between the MQTT broker and client application(s).

4. Analyze the load and performance of your MQTT broker using the various metrics provided by VerneMQ. You can access the VerneMQ status and metrics using the `vmq-admin` command:

   ```
   sudo vmq-admin metrics show
   ```

   Look for any unusual spikes or bottlenecks that could cause unexpected PUBACK packets in the output.

5. If none of the above steps resolve the issue, consider reaching out to the VerneMQ community or opening a GitHub issue to seek further assistance.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
2. [Understanding MQTT QoS Levels](https://www.hivemq.com/blog/mqtt-essentials-part-6-mqtt-quality-of-service-levels/)
