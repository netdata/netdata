# vernemq_mqtt_unsubscribe_error

**Messaging | VerneMQ**

Publish/Subscribe is a messaging pattern that aims to decouple the sending (Publisher) and
receiving (Subscriber) party. A real world example could be a sport mobile app that shows you
up-to-date information of a particular football game you're interested in. In this case you are the
subscriber, as you express interest in this specific game. On the other side sits the publisher,
which is an online reporter that feeds a system with the actual match data. This system, which is
often referred as the message broker brings the two parties together by sending the new data to all
interested subscribers.
<sup>[1](https://vernemq.com/intro/mqtt-primer/publish-subscribe.html) <sup>

An UNSUBSCRIBE packet is sent by the Client to the Server, to unsubscribe from topics. The Netdata
Agent monitors the number of failed v3/v5 UNSUBSCRIBE operations in the last minute.

MQTT v5 protocol provides detailed UNSUBSCRIBE reasons codes as opposed to MQTT v3. You can find the
detailed response codes which was sent by a client or a server and their descriptions in the
official documentation of MQTT in
the [MQTT v5 docs, UNSUBSCRIBE Reason Code](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901194)


<details>
<summary>References and sources</summary>

1. [Pub/Sub process explained on VerneMQ site](https://vernemq.com/intro/mqtt-primer/publish-subscribe.html)
2. [MQTT v3 docs UNSUBSCRIBE request](http://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html#_Toc398718072)
3. [MQTT v5 docs UNSUBSCRIBE request](https://docs.oasis-open.org/mqtt/mqtt/v5.0/os/mqtt-v5.0-os.html#_Toc3901179)

</details>

### Troubleshooting Section

<details>
<summary>General approach</summary>

Open the alerts Dashboard and locate the chart of this alert (`mqtt_unsubscribe_error`). Inspect the
error log of your VerneMQ cluster in the timestamp of this alert.

</details>


