### Understand the alert

This alert indicates that the number of received unsuccessful v5 `PUBREC` packets in the last minute is higher than expected. VerneMQ is an open-source MQTT broker. MQTT is a lightweight messaging protocol for small sensors and mobile devices optimized for high-latency or unreliable networks. `PUBREC` is an MQTT packet that is part of the quality of service 2 (QoS 2) message flow for MQTT publish/subscribe model. An unsuccessful `PUBREC` could mean that there are issues with the MQTT messages being processed by the MQTT broker.

### What does PUBREC mean?

`PUBREC` stands for "Publish Received." In MQTT, it is part of the QoS 2 message flow to ensure end-to-end delivery of a message between clients (publishers) and subscribers connected to an MQTT broker. When a client sends a `PUBLISH` message with QoS 2, the broker acknowledges the receipt with a `PUBREC` message.

### Troubleshoot the alert

To address this alert and identify the root cause, follow these steps:

1. **Check the VerneMQ log files**: Inspect the VerneMQ log files to find any issues or errors related to the processing of MQTT messages. Look for messages related to `PUBREC` or QoS 2 issues. The logs are typically located at `/var/log/vernemq/console.log`or `/var/log/vernemq/error.log`.

2. **Monitor the VerneMQ metrics**: Check VerneMQ metrics using tools like `vmq-admin` to get insights into the broker's performance and message statistics. The command `vmq-admin metrics show` provides various metrics, including the number of received `PUBREC` and the number of unsuccessful `PUBREC` messages.

3. **Verify the publisher's configuration**: Check the configuration of the MQTT clients (publishers) that are sending the QoS 2 messages to ensure a proper message flow. It's crucial to confirm that the clients are using the correct version of MQTT and adhere to the limitations set by MQTT v5, like the packet size or the maximum topic aliases used.

4. **Identify unsupported features**: Some MQTT brokers may not support all MQTT v5 features. Verify that the publisher's MQTT library supports MQTT v5 features in use, such as user properties or message expiration interval, and that it is compatible with VerneMQ.

5. **Analyze network conditions**: Unreliable network conditions or high traffic load may cause unsuccessful MQTT messages. Evaluate the network and identify any issues causing packet loss or latency. Often, improving the network conditions, migrating the broker/server to a stronger network, or adjusting the user's connection settings can help with such issues.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
2. [MQTT v5 Specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/cs02/mqtt-v5.0-cs02.html)
