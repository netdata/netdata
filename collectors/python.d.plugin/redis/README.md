<!--
title: "Redis monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/redis/README.md
sidebar_label: "Redis"
-->

# Redis monitoring with Netdata

Monitors database status. It reads server response to `INFO` command.

Following charts are drawn:

1.  **Operations** per second

    -   operations

2.  **Hit rate** in percent

    -   rate

3.  **Memory utilization** in kilobytes

    -   total
    -   lua

4.  **Database keys**

    -   lines are creates dynamically based on how many databases are there

5.  **Clients**

    -   connected
    -   blocked

6.  **Slaves**

    -   connected

## Configuration

Edit the `python.d/redis.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/redis.conf
```

```yaml
socket:
  name     : 'local'
  socket   : '/var/lib/redis/redis.sock'

localhost:
  name     : 'local'
  host     : 'localhost'
  port     : 6379
```

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:6379`.

---


