<!--
title: "Apache monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/apache/README.md
sidebar_label: "Apache"
-->

# Apache monitoring with Netdata

Monitors one or more Apache servers depending on configuration.

## Requirements

-   apache with enabled `mod_status`

It produces the following charts:

1.  **Requests** in requests/s

    -   requests

2.  **Connections**

    -   connections

3.  **Async Connections**

    -   keepalive
    -   closing
    -   writing

4.  **Bandwidth** in kilobytes/s

    -   sent

5.  **Workers**

    -   idle
    -   busy

6.  **Lifetime Avg. Requests/s** in requests/s

    -   requests_sec

7.  **Lifetime Avg. Bandwidth/s** in kilobytes/s

    -   size_sec

8.  **Lifetime Avg. Response Size** in bytes/request

    -   size_req

## Configuration

Edit the `python.d/apache.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/apache.conf
```

Needs only `url` to server's `server-status?auto`

Example for two servers:

```yaml
update_every : 10
priority     : 90100

local:
  url      : 'http://localhost/server-status?auto'

remote:
  url          : 'http://www.apache.org/server-status?auto'
  update_every : 5
```

Without configuration, module attempts to connect to `http://localhost/server-status?auto`

---


