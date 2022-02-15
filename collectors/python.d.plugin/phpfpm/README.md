<!--
title: "PHP-FPM monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/phpfpm/README.md
sidebar_label: "PHP-FPM"
-->

# PHP-FPM monitoring with Netdata

Monitors one or more PHP-FPM instances depending on configuration.

## Requirements

-   `PHP-FPM` with [enabled `status` page](https://easyengine.io/tutorials/php/fpm-status-page/)
-   access to `status` page via web server

## Charts

It produces following charts:

-   Active Connections in `connections`
-   Requests in `requests/s`
-   Performance in `status`
-   Requests Duration Among All Idle Processes in `milliseconds`
-   Last Request CPU Usage Among All Idle Processes in `percentage`
-   Last Request Memory Usage Among All Idle Processes in `KB`

## Configuration

Edit the `python.d/phpfpm.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/phpfpm.conf
```

Needs only `url` to server's `status`. Here is an example for local and remote instances:

```yaml
local:
  url     : 'http://localhost/status?full&json'

remote:
  url     : 'http://203.0.113.10/status?full&json'
```

Without configuration, module attempts to connect to `http://localhost/status`

---


