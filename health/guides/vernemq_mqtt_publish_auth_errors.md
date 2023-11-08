### Understand the alert

This alert is triggered when the Netdata Agent detects a spike in unauthorized MQTT v3/v5 `PUBLISH` attempts in the last minute on your VerneMQ broker. If you receive this alert, it means that there might be clients attempting to publish messages without the proper authentication, which could indicate a misconfiguration or potential security risk.

### What are MQTT and VerneMQ?

MQTT (Message Queuing Telemetry Transport) is a lightweight, publish-subscribe protocol designed for low-bandwidth, high-latency, or unreliable networks. VerneMQ is a high-performance, distributed MQTT broker that supports a wide range of industry standards and can handle millions of clients.

### Troubleshoot the alert

1. Verify the clients' credentials

   To check if the clients are using the correct credentials while connecting and publishing to the VerneMQ broker, inspect their log files or debug messages to find authentication-related issues.

2. Review VerneMQ broker configuration

   Ensure that the VerneMQ configuration allows for proper authentication of clients. Verify that the correct authentication plugins and settings are enabled. The configuration file is usually located at `/etc/vernemq/vernemq.conf`. For more information on VerneMQ config, please refer to [VerneMQ documentation](https://vernemq.com/docs/configuration/index.html).

3. Analyze VerneMQ logs

   Inspect the VerneMQ logs to identify unauthorized attempts and assess any potential risks. The logs typically reside in the `/var/log/vernemq` directory, and you can tail the logs using the following command:

   ```
   tail -f /var/log/vernemq/console.log
   ```

4. Configure firewall rules

   If you find unauthorized or suspicious IP addresses attempting to connect to your VerneMQ broker, consider blocking those addresses using firewall rules to prevent unauthorized access. 

### Useful resources

1. [VerneMQ documentation](https://vernemq.com/docs/index.html)
2. [Getting started with MQTT](https://mqtt.org/getting-started/)
3. [MQTT Security Fundamentals](https://www.hivemq.com/mqtt-security-fundamentals/)
4. [VerneMQ configuration options](https://vernemq.com/docs/configuration/)