### Understand the alert

This alert monitors the number of received `unsuccessful v5 PUBREL` packets in the last minute in the VerneMQ MQTT broker. If you receive this alert, it means that there were unsuccessful PUBREL attempts in VerneMQ, which might indicate an issue during the message delivery process.

### What are MQTT and PUBREL?

MQTT (Message Queuing Telemetry Transport) is a lightweight, low-code and low-latency messaging protocol that works with a subscription-based system. It utilizes a broker, like VerneMQ, to facilitate communication.

A `PUBREL` packet is the third one in a QoS-2 (Quality of Service level 2) message flow. QoS-2 is the highest available level in MQTT and strives to provide once-and-only-once message delivery to subscribers. The `PUBREL` packet is sent by the publisher to acknowledge its receipt of a `PUBREC` packet and signal that it is OK to release the message.

An unsuccessful `PUBREL` packet indicates that the message release process encountered issues and may not have been completed as expected.

### Troubleshoot the alert

1. Check the VerneMQ broker logs for any unusual messages:

   ```
   sudo journalctl -u vernemq
   ```

   Look for errors or warnings that might be related to the unsuccessful `PUBREL` packets.

2. Examine the configuration files of VerneMQ:

   ```
   cat /etc/vernemq/vernemq.conf
   ```

   Check if there are any misconfigurations or unsupported features that could cause issues with QoS-2 message flow. Refer to the [VerneMQ Documentation](https://docs.vernemq.com/configuration/introduction) for correct configurations.

3. Analyze the clients' logs, which can be publishers or subscribers, for any errors or issues related to MQTT connections and QoS levels. Make sure the clients are using the correct QoS levels and are following the MQTT protocol.

4. Monitor VerneMQ's RAM, CPU, and file descriptor usage to determine if the broker's performance is degraded. Resolve any performance bottlenecks or resource constraints to prevent further unsuccessful `PUBREL` packets.

5. For in-depth analysis, enable VerneMQ's debug logs by setting `log.console.level` to `debug` in its configuration file and restarting the service. Be cautious, as this might generate large amounts of log data.

6. If the issue persists, consider reaching out to the VerneMQ support channels, such as their [GitHub](https://github.com/vernemq/vernemq) repository.

### Useful resources

1. [VerneMQ Documentation](https://docs.vernemq.com/)
2. [MQTT Essentials](https://www.hivemq.com/mqtt-essentials/)
3. [Understanding MQTT QoS Levels - Part 1](https://www.hivemq.com/blog/mqtt-essentials-part-6-mqtt-quality-of-service-levels/)
