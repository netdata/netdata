<!--
title: "Traefik monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/traefik/README.md
sidebar_label: "Traefik"
-->

# Traefik monitoring with Netdata

Uses the `health` API to provide statistics.

It produces:

1.  **Responses** by statuses

    -   success (1xx, 2xx, 304)
    -   error (5xx)
    -   redirect (3xx except 304)
    -   bad (4xx)
    -   other (all other responses)

2.  **Responses** by codes

    -   2xx (successful)
    -   5xx (internal server errors)
    -   3xx (redirect)
    -   4xx (bad)
    -   1xx (informational)
    -   other (non-standart responses)

3.  **Detailed Response Codes** requests/s (number of responses for each response code family individually)

4.  **Requests**/s

    -   request statistics

5.  **Total response time**

    -   sum of all response time

6.  **Average response time**

7.  **Average response time per iteration**

8.  **Uptime**

    -   Traefik server uptime

## Configuration

Edit the `python.d/traefik.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/traefik.conf
```

Needs only `url` to server's `health`

Here is an example for local server:

```yaml
update_every : 1
priority     : 60000

local:
  url     : 'http://localhost:8080/health'
```

Without configuration, module attempts to connect to `http://localhost:8080/health`.

---


