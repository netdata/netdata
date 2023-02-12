<!--
title: "Squid monitoring with Netdata"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/squid/README.md"
sidebar_label: "Squid"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/Webapps"
-->

# Squid monitoring with Netdata

Monitors one or more squid instances depending on configuration.

It produces following charts:

1.  **Client Bandwidth** in kilobits/s

    -   in
    -   out
    -   hits

2.  **Client Requests** in requests/s

    -   requests
    -   hits
    -   errors

3.  **Server Bandwidth** in kilobits/s

    -   in
    -   out

4.  **Server Requests** in requests/s

    -   requests
    -   errors

## Configuration

Edit the `python.d/squid.conf` configuration file using `edit-config` from the Netdata [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/squid.conf
```

```yaml
priority     : 50000

local:
  request : 'cache_object://localhost:3128/counters'
  host    : 'localhost'
  port    : 3128
```

Without any configuration module will try to autodetect where squid presents its `counters` data

---


