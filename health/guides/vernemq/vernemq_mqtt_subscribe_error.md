### Understand the alert

This alert is related to `VerneMQ`, the open-source, distributed MQTT message broker. If you receive this alert, it means that the number of failed v3/v5 `SUBSCRIBE` operations has increased in the last minute.

### What do v3 and v5 SUBSCRIBE operations mean?

MQTT v3 and v5 are different versions of the MQTT protocol, used for the Internet of Things (IoT) devices and their communication. The `SUBSCRIBE` operation allows a client (device) to subscribe to a specific topic and receive messages published under that topic.

### Troubleshoot the alert

- Check the VerneMQ logs

1. Identify the location of the VerneMQ logs. The default location is `/var/log/vernemq`. If you have changed the default location, you can find it in the `vernemq.conf` file by looking for `log.console.file` and `log.error.file`.

   ```
   grep log.console.file /etc/vernemq/vernemq.conf
   grep log.error.file /etc/vernemq/vernemq.conf
   ```

2. Analyze the logs for any errors or issues related to the `SUBSCRIBE` operation:

   ```
   tail -f /path/to/vernemq/logs
   ```

- Check the system resources

1. Check the available resources (RAM and CPU) on your system:

   ```
   top
   ```

2. If you find that the system resources are low, consider adding more resources or stopping unnecessary processes/applications.

- Check the client-side logs

1. Most MQTT clients (e.g., Mosquitto, Paho, MQTT.js) provide their logs to help you identify any issues related to the `SUBSCRIBE` operation.

2. Analyze the client logs for errors in connecting, subscribing, or receiving messages from the MQTT broker.

- Analyze the topics and subscriptions

1. Verify if there are any invalid, restricted, or forbidden topics in your MQTT broker.

2. Check the ACLs (Access Control Lists) and client authentication settings in your VerneMQ `vernemq.conf` file.

   ```
   grep -E '^(allow_anonymous|vmq_acl.acl_file|vmq_passwd.password_file)' /etc/vernemq/vernemq.conf
   ```

3. Ensure the `ACLs` and authentication configuration are correct and allow the clients to subscribe to the required topics.

### Useful resources

1. [VerneMQ Administration](https://vernemq.com/docs/administration/)
2. [VerneMQ Configuration](https://vernemq.com/docs/configuration/)
3. [VerneMQ Logging](https://vernemq.com/docs/guide/internals.html#logging)