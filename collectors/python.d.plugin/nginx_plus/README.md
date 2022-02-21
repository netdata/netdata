<!--
title: "NGINX Plus monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/nginx_plus/README.md
sidebar_label: "NGINX Plus"
-->

# NGINX Plus monitoring with Netdata

Monitors one or more NGINX Plus servers depending on configuration. Servers can be either local or remote.

Example nginx_plus configuration can be found in 'python.d/nginx_plus.conf'

It produces following charts:

1.  **Requests total** in requests/s

    -   total

2.  **Requests current** in requests

    -   current

3.  **Connection Statistics** in connections/s

    -   accepted
    -   dropped

4.  **Workers Statistics** in workers

    -   idle
    -   active

5.  **SSL Handshakes** in handshakes/s

    -   successful
    -   failed

6.  **SSL Session Reuses** in sessions/s

    -   reused

7.  **SSL Memory Usage** in percent

    -   usage

8.  **Processes** in processes

    -   respawned

For every server zone:

1.  **Processing** in requests

-   processing

2.  **Requests** in requests/s

    -   requests

3.  **Responses** in requests/s

    -   1xx
    -   2xx
    -   3xx
    -   4xx
    -   5xx

4.  **Traffic** in kilobits/s

    -   received
    -   sent

For every upstream:

1.  **Peers Requests** in requests/s

    -   peer name (dimension per peer)

2.  **All Peers Responses** in responses/s

    -   1xx
    -   2xx
    -   3xx
    -   4xx
    -   5xx

3.  **Peer Responses** in requests/s (for every peer)

    -   1xx
    -   2xx
    -   3xx
    -   4xx
    -   5xx

4.  **Peers Connections** in active

    -   peer name (dimension per peer)

5.  **Peers Connections Usage** in percent

    -   peer name (dimension per peer)

6.  **All Peers Traffic** in KB

    -   received
    -   sent

7.  **Peer Traffic** in KB/s (for every peer)

    -   received
    -   sent

8.  **Peer Timings** in ms (for every peer)

    -   header
    -   response

9.  **Memory Usage** in percent

    -   usage

10. **Peers Status** in state

    -   peer name (dimension per peer)

11. **Peers Total Downtime** in seconds

    -   peer name (dimension per peer)

For every cache:

1.  **Traffic** in KB

    -   served
    -   written
    -   bypass

2.  **Memory Usage** in percent

    -   usage

## Configuration

Edit the `python.d/nginx_plus.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/nginx_plus.conf
```

Needs only `url` to server's `status`.

Here is an example for a local server:

```yaml
local:
  url     : 'http://localhost/status'
```

Without configuration, module fail to start.

---


