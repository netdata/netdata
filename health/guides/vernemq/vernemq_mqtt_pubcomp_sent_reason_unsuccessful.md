### Understand the alert

This alert indicates that the number of unsuccessful v5 PUBCOMP (Publish Complete) packets sent within the last minute has increased. VerneMQ is an MQTT broker, which plays a crucial role in managing and processing the message flow between MQTT clients. If you receive this alert, it implies that there are issues in the message flow, which might affect the communication between MQTT clients and the broker.

### What does PUBCOMP mean?

In MQTT protocol, PUBCOMP is the fourth and final packet in the Quality of Service (QoS) 2 protocol exchange. The flow consists of PUBLISH, PUBREC (Publish Received), PUBREL (Publish Release), and PUBCOMP packets. PUBCOMP is sent by the receiver (MQTT client or broker) to confirm that it has received and processed the PUBREL packet. Unsuccessful PUBCOMP packets indicate that the receiver was not able to process the message properly.

### Troubleshoot the alert

- Check VerneMQ logs for errors or warnings

  VerneMQ logs can provide valuable information about issues with the message flow. Locate the log file (usually at `/var/log/vernemq/console.log`) and inspect it for any error messages or warnings related to the PUBCOMP packet or its predecessors (PUBLISH, PUBREC, PUBREL) in the QoS 2 flow.

- Identify problematic MQTT clients

  Analyze the logs to identify the MQTT clients that are frequently involved in unsuccessful PUBCOMP packets exchange. These clients might have connection or configuration issues that lead to unsuccessful PUBCOMP packets.

- Validate MQTT clients configurations

  Ensure that the MQTT clients involved in unsuccessful PUBCOMP packets have valid configurations and that they are compatible with the broker (VerneMQ). Check parameters such as QoS level, protocol version, authentication, etc.

- Monitor VerneMQ metrics

  Use Netdata or other monitoring tools to observe VerneMQ metrics and identify unusual patterns in the broker's performance. Increased load on the broker, high memory or CPU usage, slow response times, or network hiccups might contribute to unsuccessful PUBCOMP packets.

- Ensure proper MQTT payload size

  Unsuccessful PUBCOMP packets can be caused by oversized payload or incorrect Message ID. Verify that the payload size respects the Maximum Transmission Unit (MTU) and that the Message ID follows the MQTT protocol specifications.

### Useful resources

1. [VerneMQ - Troubleshooting](https://vernemq.com/docs/troubleshooting/)
2. [MQTT Protocol Specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html)
3. [VerneMQ - Monitoring](https://vernemq.com/docs/monitoring/)