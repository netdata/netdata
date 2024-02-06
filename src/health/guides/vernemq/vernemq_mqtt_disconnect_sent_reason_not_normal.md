### Understand the alert

This alert indicates that VerneMQ, a high-performance, distributed MQTT message broker, is sending an abnormal number of v5 DISCONNECT packets in the last minute. This may signify an issue in the MQTT messaging system and impact the functioning of IoT devices or other MQTT clients connected to VerneMQ.

### What does an abnormal v5 DISCONNECT packet mean?

In MQTT v5, the DISCONNECT packet is sent by a client or server to indicate the end of a session. A "not normal" DISCONNECT packet, generally refers to a DISCONNECT packet sent with a reason code other than "Normal Disconnection" (0x00). These reason codes might include:

- Protocol errors
- Invalid DISCONNECT payloads
- Authorization or authentication violations
- Exceeded keep-alive timers
- Server/connection errors
- User-triggered disconnects

A high number of not normal DISCONNECT packets, might indicate an issue in your MQTT infrastructure, misconfigured clients, or security breaches.

### Troubleshoot the alert

1. **Inspect VerneMQ logs**: VerneMQ logs can provide detailed information about connections, disconnections, and possible issues. Check the VerneMQ logs for errors and information about unusual disconnects.

   ```
   cat /var/log/vernemq/console.log
   cat /var/log/vernemq/error.log
   ```

2. **Monitor VerneMQ status**: Use the `vmq-admin` command-line tool to monitor VerneMQ and view its runtime status. Check the number of connected clients, subscriptions, and sessions.

   ```
   sudo vmq-admin cluster show
   sudo vmq-admin session show
   sudo vmq-admin listener show
   ```

3. **Check clients and configurations**: Review client configurations for potential errors, like incorrect authentication credentials, misconfigured keep-alive timers, or invalid packet formats. If possible, isolate problematic clients and test their behavior.

4. **Consider resource limitations**: If your VerneMQ instance is reaching resource limitations (CPU, memory, network), it might automatically terminate some connections to maintain performance. Monitor system resources using the `top` command or tools like Netdata.

5. **Evaluate security**: If the issue persists, consider checking the security of your MQTT infrastructure. Investigate possible cyber threats, such as a DDoS attack or unauthorized clients attempting to connect.

### Useful resources

1. [VerneMQ Documentation](https://docs.vernemq.com/)
2. [MQTT v5 Specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html)
3. [Debugging MQTT Connections](https://www.hivemq.com/blog/mqtt-essentials-part-9-last-will-and-testament/)