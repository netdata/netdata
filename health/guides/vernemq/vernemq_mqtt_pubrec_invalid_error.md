### Understand the alert

This alert is triggered when the Netdata Agent monitors an unexpected increase in the number of VerneMQ v3 MQTT `PUBREC` packets received during the last minute. VerneMQ is an MQTT broker that is essential for message distribution in IoT applications. MQTT v3 is one of the protocol versions used by the MQTT brokers.

### What does an invalid PUBREC packet mean?

`PUBREC` is a control packet in the MQTT protocol that acknowledges receipt of a `PUBLISH` packet. This packet is used during Quality of Service (QoS) level 2 message delivery, ensuring that the message is received exactly once. An invalid `PUBREC` packet means that VerneMQ has received a `PUBREC` packet that contains incorrect, unexpected, or duplicate data.

### Troubleshoot the alert

- Check VerneMQ logs

  Investigate the VerneMQ logs to see if there are any error messages or warnings related to the processing of `PUBREC` packets. The logs can be found in `/var/log/vernemq/console.log` or `/usr/local/var/log/vernemq/console.log`. Look for any entries with specific error messages mentioning `PUBREC`.

- Check MQTT Clients

  Monitor the MQTT clients that are connected to the VerneMQ broker to identify which clients are sending invalid `PUBREC` packets. Check the logs or monitoring systems of those clients to understand the root cause of the problem. They might be experiencing issues or bugs causing them to send incorrect `PUBREC` packets.

- Check the MQTT topics

  Monitor the MQTT topics with high levels of QoS 2 message delivery and determine if a specific topic is causing the spike in invalid `PUBREC` packets.

- Upgrade or fix MQTT Clients

  If the issue arises from specific client implementations, consider upgrading the MQTT client libraries, fixing any configuration issues or reporting the bug to the appropriate development teams.

- Review VerneMQ configuration

  Verify that the VerneMQ broker configuration is set up correctly and that MQTT v3 protocol is enabled. If necessary, adjust the configuration to better handle the volume of QoS 2 messages being processed.

### Useful resources

1. [VerneMQ documentation](https://vernemq.com/docs/index.html)
2. [MQTT v3.1.1 specification](http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html)
