# vernemq_mqtt_disconnect_sent_reason_not_normal

**Messaging | VerneMQ**

_The DISCONNECT packet is the final MQTT Control Packet sent from the Client or the Server. It
indicates the reason why the Network Connection is being closed. The Client or Server may send a
DISCONNECT packet before closing the Network
Connection. <sup>[1](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205) </sup>_

The Netdata Agent monitors the number of sent _not normal_ v5 DISCONNECT packets over the last
minute. This alert is raised into warning when your VerneMQ server sends more than 5 DISCONNECT
packets over the last minute.

For various scenarios, there are specific DISCONNECT responses for MQTT protocol v5. You can find
the detailed response codes which were sent by the Server and their descriptions in the official
documentation of MQTT in
the [MQTT v5 docs, subsection 3.14.2.1: Disconnect reason code](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205)

<details>
<summary>References and sources</summary>

1. [MQTT v5 docs DISCONNECT notification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

Open the alerts Dashboard and locate the chart of this alert (`mqtt_disconnect_sent_reason`).
Inspect which DISCONNECT packets (by reason) triggered this alert. Inspect the reason why your
server sent those responses by consulting the subsection _Disconnect reason
code <sup>[1](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205) </sup>_
mentioned above.

For example, your server may respond to a client with `QoS not supported`. In that case, the client must
change the QoS settings.

</details>



