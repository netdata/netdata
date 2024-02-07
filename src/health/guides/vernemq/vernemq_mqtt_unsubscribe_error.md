### Understand the alert

This alert monitors the number of failed v3/v5 `UNSUBSCRIBE` operations in VerneMQ in the last minute. If you receive this alert, it means that there is a significant number of failed `UNSUBSCRIBE` operations, which may impact the MQTT messaging on your system.

### What is VerneMQ?

VerneMQ is a high-performance, distributed MQTT message broker. It provides scalable and reliable communication for Internet of Things (IoT) systems and applications.

### What is an MQTT UNSUBSCRIBE operation?

An `UNSUBSCRIBE` operation in MQTT protocol is a request sent by a client to the server to remove one or more topics from the subscription list. It allows clients to stop receiving messages for particular topics.

### Troubleshoot the alert

1. Check VerneMQ logs for any error messages or indications of issues with the `UNSUBSCRIBE` operation:

   ```
   sudo journalctl -u vernemq
   ```

   Alternatively, you may find the logs in `/var/log/vernemq/` directory, if using the default configuration:

   ```
   cat /var/log/vernemq/console.log
   cat /var/log/vernemq/error.log
   ```

2. Review the VerneMQ configuration to ensure it is properly set up. The default configuration file is located at `/etc/vernemq/vernemq.conf`. Make sure that the settings are correct, especially those related to the MQTT protocol version and the supported QoS levels.

3. Monitor the VerneMQ metrics using the `vmq-admin metrics show` command. This will provide you with an overview of the broker's performance and help you identify any abnormal metrics that could be related to the failed `UNSUBSCRIBE` operations:

   ```
   sudo vmq-admin metrics show
   ```

   Pay attention to the `mqtt.unsubscribe_error` metric, which indicates the number of failed `UNSUBSCRIBE` operations.

4. Check the MQTT clients that are sending the `UNSUBSCRIBE` requests. It is possible that the client itself is misconfigured or has some faulty logic in its communication with the MQTT broker. Review the client's logs and configuration to identify any issues.

