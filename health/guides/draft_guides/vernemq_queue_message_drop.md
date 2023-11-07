# vernemq_queue_message_drop

**Messaging | VerneMQ**

The Netdata Agent monitors the number of dropped messages due to full queues in the last minute. This alert indicates
that message queues are full and VerneMQ is dropping messages. This can be caused when consumers or VerneMQ are too
slow, or publishers are too fast. To address this issue, try to increase the queue length, adjust the
`max_online_messages` value.

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)
