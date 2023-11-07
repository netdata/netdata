# vernemq_mqtt_puback_unexpected

**Messaging | VerneMQ**

A PUBACK packet is the response to a PUBLISH packet with QoS 1. The Netdata Agent monitors the
number of received unexpected v3/v5 PUBACK packets in the last minute.

MQTT v5 protocol provides detailed PUBACK reasons codes as opposed to MQTT v3. You can find the
detailed response codes which were sent by a client or a server and their descriptions in the
official documentation of MQTT in
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
3. [MQTT v3 docs, PUBACK description](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html#_Toc398718043)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

This alert monitors the PUBACK packets for both v3 and v5 MQTT protocol. In case you didn't receive
any other alerts (`vernemq_mqtt_puback_received_reason_unsuccessful`
, `vernemq_mqtt_puback_sent_unsuccessful`) (in which you can consult their troubleshooting
sections), that means that the unexpected PUBACK packets was came(sent) from(to) clients which are
using the MQTT v3 protocol. In that case you can inspect your MQTT server access log for further
investigation.


</details>

