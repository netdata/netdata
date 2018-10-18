# uwsgi

Module monitor uwsgi performance metrics.

https://uwsgi-docs.readthedocs.io/en/latest/StatsServer.html

lines are creates dynamically based on how many workers are there

Following charts are drawn:

1. **Requests**
 * requests per second
 * transmitted data
 * average request time

2. **Memory**
 * rss
 * vsz

3. **Exceptions**
4. **Harakiris**
5. **Respawns**

### configuration

```yaml
socket:
  name     : 'local'
  socket   : '/tmp/stats.socket'

localhost:
  name     : 'local'
  host     : 'localhost'
  port     : 1717
```

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:1717`.
