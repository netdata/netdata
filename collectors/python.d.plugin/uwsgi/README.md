# uwsgi

Module monitor uwsgi performance metrics.

<https://uwsgi-docs.readthedocs.io/en/latest/StatsServer.html>

lines are creates dynamically based on how many workers are there

Following charts are drawn:

1.  **Requests**

    -   requests per second
    -   transmitted data
    -   average request time

2.  **Memory**

    -   rss
    -   vsz

3.  **Exceptions**
4.  **Harakiris**
5.  **Respawns**

## configuration

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fuwsgi%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
