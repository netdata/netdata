<!--
title: "uWSGI monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/uwsgi/README.md"
sidebar_label: "uWSGI"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
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
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

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


