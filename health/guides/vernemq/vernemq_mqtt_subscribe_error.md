# vernemq_mqtt_subscribe_error

**Messaging | VerneMQ**

The SUBSCRIBE packet is sent from the client to the server to create one or more subscriptions. Each
subscription registers a Client’s interest in one or more Topics. The server sends PUBLISH packets
to the Client in order to forward Application Messages that were published to Topics that match
these subscriptions. The SUBSCRIBE packet also specifies (for each subscription) the maximum QoS
with which the server can send Application Messages to the Client. The Netdata Agent monitors the
number of failed v3/v5 SUBSCRIBE operations in the last minute.

<details>
<summary>MQTT basic concepts and more</summary>

Basic concepts in every MQTT
architecture <sup>[1](https://learn.sparkfun.com/tutorials/introduction-to-mqtt/all) </sup>

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
      delivery. This is often refered to as fire and forget.
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

1. [MQTT basic concepts](https://learn.sparkfun.com/tutorials/introduction-to-mqtt/all)
2. [MQTT v5 docs SUBSCRIBE packet](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901161)


</details>

### Troubleshooting Section

These kinds of errors can appear in cases of a network partition in your cluster (aka netsplit)

<details>
<summary>Check connectivity between nodes</summary>

You must ensure that the connectivity between your cluster nodes is valid. As soon as the partition
is healed, and connectivity reestablished, the VerneMQ nodes replicate the latest changes made to
the subscription data. This includes all the changes 'accidentally' made during the Window of
Uncertainty. Using Dotted Version Vectors VerneMQ ensures that convergence regarding subscription
data and retained messages is eventually reached.

</details>

