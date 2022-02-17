<!--
title: "NGINX monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/nginx/README.md
sidebar_label: "NGINX"
-->

# NGINX monitoring with Netdata

Monitors one or more NGINX servers depending on configuration. Servers can be either local or remote.

## Requirements

-   nginx with configured 'ngx_http_stub_status_module'
-   'location /stub_status'

Example nginx configuration can be found in 'python.d/nginx.conf'

It produces following charts:

1.  **Active Connections**

    -   active

2.  **Requests** in requests/s

    -   requests

3.  **Active Connections by Status**

    -   reading
    -   writing
    -   waiting

4.  **Connections Rate** in connections/s

    -   accepts
    -   handled

## Configuration

Edit the `python.d/nginx.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/nginx.conf
```

Needs only `url` to server's `stub_status`.

Here is an example for local server:

```yaml
update_every : 10
priority     : 90100

local:
  url     : 'http://localhost/stub_status'
```

Without configuration, module attempts to connect to `http://localhost/stub_status`

---


