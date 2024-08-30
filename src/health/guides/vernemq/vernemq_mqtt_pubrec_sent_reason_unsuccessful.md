### Understand the alert

This alert monitors the number of sent unsuccessful v5 PUBREC packets in the last minute in the VerneMQ MQTT broker. If you receive this alert, it means that there is an issue with successfully acknowledging receipt of PUBLISH packets in the MQTT system.

### What does PUBREC mean?

In the MQTT protocol, when a client sends a PUBLISH message with Quality of Service (QoS) level 2, it expects an acknowledgment from the server in the form of a PUBREC (Publish Received) message. This confirms the successful receipt of the PUBLISH message by the server. If a PUBREC message is marked as unsuccessful, it indicates a problem with the message acknowledgment process.

### Troubleshoot the alert

1. Check VerneMQ log files for any errors or warnings related to unsuccessful PUBREC messages. VerneMQ logs can be found in `/var/log/vernemq` (by default) or the directory specified in your configuration file.

   ```
   sudo tail -f /var/log/vernemq/console.log
   sudo tail -f /var/log/vernemq/error.log
   ```

2. Verify if any clients are having issues with the MQTT connection, such as intermittent network problems or misconfigured settings. Check the client logs for any issues and take appropriate action.

3. Review the MQTT QoS settings for the clients in the system. If possible, consider lowering the QoS level to 1 or 0, which uses less resources and bandwidth. QoS level 2 might not be necessary for some use cases.

4. Inspect the VerneMQ system and environment for resource bottlenecks or other performance issues. Use tools like `top`, `htop`, `vmstat`, or `iotop` to monitor system resources and identify any potential problems.

5. If the issue persists, consider seeking support from the VerneMQ community or the software vendor for further assistance.

### Useful resources

1. [VerneMQ Documentation](https://docs.vernemq.com/)
2. [MQTT Essentials – All Core MQTT Concepts explained](https://www.hivemq.com/mqtt-essentials/)
3. [Understanding QoS Levels in MQTT](https://www.hivemq.com/blog/mqtt-essentials-part-6-mqtt-quality-of-service-levels/)