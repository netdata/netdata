### Understand the alert

This alert is raised when the number of unhandled messages in the last minute, monitored by the Netdata Agent, is too high. It indicates that many messages were not delivered due to connections with `clean_session=true` in a VerneMQ messaging system.

### What does clean_session=true mean?

In MQTT, `clean_session=true` means that the client doesn't want to store any session state on the broker for the duration of its connection. When the session is terminated, all subscriptions and messages are deleted. The broker won't store any messages or send any missed messages once the client reconnects.

### What are VerneMQ unhandled messages?

Unhandled messages are messages that cannot be delivered to subscribers due to connection issues, protocol limitations, or session configurations. These messages are often related to clients' settings for `clean_session=true`, which means they don't store any session state on the broker.

### Troubleshoot the alert

- Identify clients causing unhandled messages

  One way to find the clients causing unhandled messages is by analyzing the VerneMQ log files. Look for warning or error messages related to undelivered messages or clean sessions. The log files are typically located in `/var/log/vernemq/`.
  
- Check clients' clean_session settings

  Review your MQTT clients' configurations to verify if they have `clean_session=true`. Consider changing the setting to `clean_session=false` if you want the broker to store session state and send missed messages upon reconnection.

- Monitor VerneMQ statistics

  Use the following command to see an overview of the VerneMQ statistics:

  ```
  vmq-admin metrics show
  ```

  Look for metrics related to dropped or unhandled messages, such as `gauge.queue_message_unhandled`.

- Examine your system resources

  High unhandled message rates can also be a result of insufficient system resources. Check your system resources (CPU, memory, disk usage) and consider upgrading if necessary.

### Useful resources

1. [VerneMQ - An MQTT Broker](https://vernemq.com/)
2. [VerneMQ Documentation: Monitoring & Metrics](https://docs.vernemq.com/monitoring/)
3. [Understanding MQTT Clean Sessions, Queuing, Retained Messages and QoS](https://www.hivemq.com/blog/mqtt-essentials-part-7-persistent-session-queuing-messages/)