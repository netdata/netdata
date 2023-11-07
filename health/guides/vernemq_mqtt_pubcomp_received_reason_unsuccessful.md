### Understand the alert

This alert indicates that the VerneMQ broker has received an increased number of unsuccessful MQTT v5 PUBCOMP (Publish Complete) packets in the last minute. The PUBCOMP packet is the fourth and final packet in the QoS 2 publish flow. It means that there are issues in the MQTT message delivery process at Quality of Service (QoS) level 2, which could lead to message loss or duplicated messages.

### What does an unsuccessful PUBCOMP mean?

An unsuccessful PUBCOMP occurs when the recipient of a PUBLISH message (subscriber) acknowledges reception but encounters a problem while processing the message. The PUBCOMP packet contains a Reason Code, indicating the outcome of processing the PUBLISH message. In a successful case, the code would be 0x00 (Success); otherwise, it would be one of the following: 0x80 (Unspecified Error), 0x83 (Implementation Specific Error), 0x87 (Not Authorized), 0xD0 (Packet Identifier in Use), or 0xD2 (Packet Identifier Not Found).

### Troubleshoot the alert

1. Check the VerneMQ error logs: VerneMQ logs can provide valuable information on encountered errors or any misconfiguration that leads to unsuccessful PUBCOMP messages. Generally, their location is `/var/log/vernemq/console.log`, `/var/log/vernemq/error.log`, and `/var/log/vernemq/crash.log`.

2. Review MQTT clients' logs: Inspect the logs of the MQTT clients that are publishing or subscribing to the messages on the VerneMQ broker. This may help you identify specific clients causing the problem or any pattern associated with unsuccessful PUBCOMP messages.

3. Verify the Quality of Service (QoS) level: Check if the QoS level for PUBCOMP packets is set to 2, as required. If necessary, adjust the settings for the MQTT clients to match the expected QoS level.

4. Investigate authorization and access control: If the Reason Code is related to authorization (0x87), verify that the MQTT clients involved have the correct permissions to publish and subscribe to the topics in question. Make sure that the VerneMQ Access Control List (ACL) or external authentication mechanisms are correctly configured.

5. Monitor network connectivity: Unsuccessful PUBCOMP messages could be due to network issues between the MQTT clients and the VerneMQ broker. Monitor and analyze network latency or packet loss between clients and the VerneMQ server to identify any potential issues.

### Useful resources

1. [VerneMQ Documentation](https://vernemq.com/docs/)
2. [MQTT v5 Specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html)
3. [Troubleshooting VerneMQ](https://vernemq.com/docs/guide/introduction/troubleshooting/)
4. [VerneMQ ACL Configuration](https://vernemq.com/docs/configuration/acl.html)