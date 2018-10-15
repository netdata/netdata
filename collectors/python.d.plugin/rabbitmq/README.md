# rabbitmq

Module monitor rabbitmq performance and health metrics.

Following charts are drawn:

1. **Queued Messages**
 * ready
 * unacknowledged

2. **Message Rates**
 * ack
 * redelivered
 * deliver
 * publish

3. **Global Counts**
 * channels
 * consumers
 * connections
 * queues
 * exchanges

4. **File Descriptors**
 * used descriptors

5. **Socket Descriptors**
 * used descriptors

6. **Erlang processes**
 * used processes

7. **Erlang run queue**
 * Erlang run queue

8. **Memory**
 * free memory in megabytes

9. **Disk Space**
 * free disk space in gigabytes

### configuration

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
