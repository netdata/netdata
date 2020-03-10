<!--
---
title: "Squid monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/squid/README.md
---
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

Edit the `python.d/squid.conf` configuration file using `edit-config` from the your agent's [config
directory](../../../docs/step-by-step/step-04.md#find-your-netdataconf-file), which is typically at `/etc/netdata`.

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fsquid%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
