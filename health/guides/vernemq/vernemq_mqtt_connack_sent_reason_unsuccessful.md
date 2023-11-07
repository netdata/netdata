# vernemq_mqtt_connack_sent_reason_unsuccessful

**Messaging | VerneMQ**

In the MQTT protocol, the CONNACK packet is the packet sent by the server in response to a CONNECT
attempt from a client. The first packet sent from the server to the client must be a CONNACK packet.
If the client does not receive a CONNACK packet from the server within a reasonable amount of time,
the client should close the Network Connection. The "reasonable" amount of time depends on the type
of application and the communications infrastructure.

For various scenarios, there are specific CONNACK responses for both MQTT v3 and v5 . You can find
the detailed response codes and descriptions for each protocol in the official documentation of
MQTT,
for [v3 (subsection 3.2.2.3 Connect Return code)](http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html#_Toc398718035)
and
for [v5 (subsection 3.2.2.2 Connect Reason Code)](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901074)

MQTT v5 supports a wider variety of negative acknowledgements (unsuccessful CONNACK packets ), which
makes it a lot easier for both the client and the VerneMQ admin to understand what's happening.

The Netdata Agent monitors the number of sent unsuccessful v3/v5 CONNACK packets over the last
minute. This alert is raised into warning when your VerneMQ server sends more than 5 unsuccessful
packets in the last minute.

<details>
<summary>References and Sources</summary>

1. [MQTT v3 docs, CONNACK description](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html#_Toc398718033)
2. [MQTT v5 docs, CONNACK description](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901074)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

Open the alerts Dashboard, and locate the chart of this alert (`mqtt_connack_sent_reason`). Inspect
which CONNACK packets (by reason) triggered this alert. As soon as you inspect the reason (by
consulting the subsections: _connect reason code_ which we mentioned above for your protocol ), you
will have to examine
your [logs](https://docs.vernemq.com/configuring-vernemq/logging#console-logging) to check which
client(s) are raising these issues. These kinds of issues appear in the warning log level, so you
may have to set your log level appropriately.

```
root@netdata # cat /var/log/vernemq/console.log | grep "due to <keywords: error> "
```

</details>
