# rabbitmq

This module monitors [RabbitMQ](https://www.rabbitmq.com/) performance and health metrics.

Following charts are drawn:

1.  **Queued Messages**

    -   ready
    -   unacknowledged

2.  **Message Rates**

    -   ack
    -   redelivered
    -   deliver
    -   publish

3.  **Global Counts**

    -   channels
    -   consumers
    -   connections
    -   queues
    -   exchanges

4.  **File Descriptors**

    -   used descriptors

5.  **Socket Descriptors**

    -   used descriptors

6.  **Erlang processes**

    -   used processes

7.  **Erlang run queue**

    -   Erlang run queue

8.  **Memory**

    -   free memory in megabytes

9.  **Disk Space**

    -   free disk space in gigabytes


Per Vhost charts:

1.  **Vhost Messages**

    -   ack
    -   confirm
    -   deliver
    -   get
    -   get_no_ack
    -   publish
    -   redeliver
    -   return_unroutable

## configuration

```yaml
socket:
  name     : 'local'
  host     : '127.0.0.1'
  port     :  15672
  user     : 'guest'
  pass     : 'guest'
```

When no configuration file is found, module tries to connect to: `localhost:15672`.

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Frabbitmq%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
