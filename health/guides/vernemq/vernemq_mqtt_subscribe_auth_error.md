### Understand the alert

This alert indicates that there have been unauthorized MQTT (Message Queuing Telemetry Transport) v3/v5 SUBSCRIBE attempts in the last minute. This could mean that there are clients trying to subscribe to topics without proper authentication or authorization in your VerneMQ broker.

### What does unauthorized subscribe mean?

In the MQTT protocol, clients can subscribe to topics to receive messages published by other clients to the broker. An unauthorized subscribe occurs when a client tries to subscribe to a topic but does not have the required permissions or has not provided valid credentials.

### Troubleshoot the alert

1. Check the VerneMQ logs for unauthorized subscribe attempts:

   The first step in troubleshooting this issue is to check the VerneMQ logs to identify the source of the unauthorized attempts. Look for log messages related to authentication or authorization errors in the log files (`/var/log/vernemq/console.log` or `/var/log/vernemq/error.log`).

   Example log message:
   ```
   date time [warning] <client_id>@<client_IP> MQTT SUBSCRIBE authorization failure for user "<username>", topic "<topic_name>"
   ```

2. Verify client authentication and authorization configuration:

   Check the client configurations to ensure they have the correct credentials (username and password) and are authorized to subscribe to the intended topics. Remember that topic permissions are case-sensitive and might have wildcards. Update the client configurations if necessary and restart the MQTT clients.

3. Review the VerneMQ broker configurations:

   Verify the authentication and authorization plugins or settings in the VerneMQ broker (`/etc/vernemq/vernemq.conf` or `/etc/vernemq/vmq.acl` for access control). Make sure the settings are correctly configured to allow the clients to subscribe to the intended topics. Update the configurations if necessary and restart the VerneMQ broker.

4. Monitor the unauthorized subscribe attempts using the Netdata dashboard or configuration file:

   Continue monitoring the unauthorized subscribe attempts using the Netdata dashboard or by configuring the alert thresholds in the Netdata configuration file. This will help you track the issue and ensure that the problem has been resolved.

### Useful resources

1. [VerneMQ documentation](https://vernemq.com/docs/)
2. [MQTT v3.1.1 specification](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html)
3. [MQTT v5.0 specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html)
4. [Understanding MQTT topic permissions and wildcards](http://www.steves-internet-guide.com/understanding-mqtt-topics/)