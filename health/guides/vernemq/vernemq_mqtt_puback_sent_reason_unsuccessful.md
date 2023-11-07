# vernemq_mqtt_puback_sent_reason_unsuccessful

**Messaging | VerneMQ**

A PUBACK packet is the response to a PUBLISH packet with QoS 1. The Netdata Agent monitors the
number of sent unsuccessful v5 PUBACK packets in the last minute. For various scenarios, there are
specific PUBACK responses for MQTT protocol v5. You can find the detailed response codes which were
sent by a client or a server and their descriptions in the official documentation of MQTT in
the [MQTT v5 docs, PUBACK Reason Code](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901124)
. The Client or Server sending the PUBACK packet must always use one of the PUBACK Reason Codes.

<details>
<summary>See more about QoS 1 </summary>

The Quality of Service (QoS) level is an agreement between the sender of a message and the receiver
of a message that defines the guarantee of delivery for a specific message. In QoS 1, a client will
receive a confirmation message from the broker upon receipt. If the expected confirmation is not
received within a certain time frame, the client has to retry the message. A message received by a
client must be acknowledged on time as well, otherwise the broker will re-deliver the
message. <sup>[1](https://vernemq.com/intro/mqtt-primer/quality_of_service.html) </sup>

</details>

<details>
<summary>References and sources</summary>

1. [Quality of service explained, VerneMQ docs](https://vernemq.com/intro/mqtt-primer/quality_of_service.html)
2. [MQTT v5 docs, PUBACK description](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901100)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

Open the alerts Dashboard and locate the chart of this alert (`mqtt_puback_sent_reason`). Identify
which PUBACK packets (by reason) triggered this alert. Inspect the reason why you server sent those
responses by consulting the subsection _PUBACK Reason
Code  <sup>[1](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901124)_ </sup>
mentioned above.

</details>
