# vernemq_mqtt_pubcomp_sent_reason_unsuccessful

**Messaging | VerneMQ**

The PUBCOMP packet is the response to a PUBREL packet. It is the fourth and final packet of the QoS
2 protocol exchange. The Netdata Agent monitors the number of sent unsuccessful v5 PUBCOMP packets
in the last minute.

<details>
<summary>MQTT basic concepts and more</summary>

Basic concepts in every MQTT
architecture <sup>[1](https://learn.sparkfun.com/tutorials/introduction-to-mqtt/all) </sup>:

- _Broker_ - The broker is the server that distributes the information to the interested clients
  connected to the server.
- _Client_ - The device that connects to broker to send or receive information.
- _Topic_ - The name that the message is about. Clients publish, subscribe, or do both to a topic.
- _Publish_ - Clients that send information to the broker to distribute to interested clients based
  on the topic name.
- _Subscribe_ - Clients tell the broker which topic(s) they're interested in. When a client
  subscribes to a topic, any message published to the broker is distributed to the subscribers of
  that topic. Clients can also unsubscribe to stop receiving messages from the broker about that
  topic.
- _QoS_ - Quality of Service. Each connection can specify a quality of service to the broker with an
  integer value ranging from 0-2. The QoS does not affect the handling of the TCP data
  transmissions, only between the MQTT clients. Note: In the examples later on, we'll only be using
  QoS 0.

    - _QoS 0_ specifies at most once, or once and only once without requiring an acknowledgment of
      delivery. This is often referred to as fire and forget.
    - _QoS 1_ specifies at least once. The message is sent multiple times until an acknowledgment is
      received, known otherwise as acknowledged delivery.
    - _QoS 2_ specifies exactly once. The sender and receiver clients use a two level handshake to
      ensure only one copy of the message is received, known as assured delivery.

- _VerneMQ WebSockets_ - WebSocket is a computer communications protocol, providing full-duplex
  communication channels over a single TCP connection. VerneMQ supports the WebSocket protocol out
  of the box. To be able to open a WebSocket connection to VerneMQ, you have to configure a
  WebSocket listener or Secure WebSocket listener in the `vernemq.conf`. See more in the official
  documentation in
  the [how to configure WebSocket](https://docs.vernemq.com/configuring-vernemq/websockets)
  section

</details>

<details>
<summary>References and sources</summary>

1. [Introduction to MQTT](https://learn.sparkfun.com/tutorials/introduction-to-mqtt/all)
2. [MQTT v5 docs, PUBCOMP reason codes](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901154)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

Open the alerts Dashboard and locate the chart of this alert (`mqtt_pubcomp_sent_reason`). Inspect
which PUBCOMP packets (by reason) triggered this alert. Inspect the reason why your server sent
those responses by consulting the subsection _OPERATION REASON
CODE_ <sup>[2](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901154) </sup>
which we mentioned above.

</details>

