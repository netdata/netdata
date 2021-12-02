<!--
title: "uWSGI monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/uwsgi/README.md
sidebar_label: "uWSGI"
-->

# uWSGI monitoring with Netdata

Monitors performance metrics exposed by [`Stats Server`](https://uwsgi-docs.readthedocs.io/en/latest/StatsServer.html).


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

## Configuration

Edit the `python.d/uwsgi.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/uwsgi.conf
```

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
