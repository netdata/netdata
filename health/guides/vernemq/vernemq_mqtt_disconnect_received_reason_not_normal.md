# vernemq_mqtt_disconnect_received_reason_not_normal

**Messaging | VerneMQ**

_The DISCONNECT packet is the final MQTT Control Packet sent from the Client or the Server. It
indicates the reason why the Network Connection is being closed. The Client or Server may send a
DISCONNECT packet before closing the Network Connection. If the Network Connection is closed without
the Client first sending a DISCONNECT packet with Reason Code 0x00 (Normal disconnection) and the
Connection has a Will Message, the Will Message is
published. <sup>[1](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205) </sup>_

The Netadata agent monitors the number of received not normal v5 DISCONNECT packets over the last
minute. This alert is raised into warning when your VerneMQ server receive more than 5 DISCONNECT
packets over the last minute.

For various scenarios, there are specific DISCONNECT responses for MQTT protocol v5. You can find
the detailed response codes which were sent by a client and their descriptions in the official
documentation of MQTT in
the [MQTT v5 docs, subsection 3.14.2.1: Disconnect reason code](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205)

<details>
<summary>References and sources</summary>

1. [MQTT v5 docs DISCONNECT notification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

Open the alerts Dashboard and locate the chart of this alert (`mqtt_disconnect_received_reason`).
Inspect which DISCONNECT packets (by reason) triggered this alert. You can clarify why your server
received those responses from a client by consulting the subsection _Disconnect reason
code <sup>[1](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901205) </sup>_
which we mentioned above.

For example: your server may receive some DISCONNECT packets with the reason: "Disconnect with Will
Message." This is not abnormal except in the case in which the network connection is closed
abruptly. This may indicate problems in the connectivity with your clients.

</details>





