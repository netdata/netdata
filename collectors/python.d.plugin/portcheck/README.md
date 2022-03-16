<!--
title: "TCP endpoint monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/portcheck/README.md
sidebar_label: "TCP endpoints"
-->

# TCP endpoint monitoring with Netdata

Monitors TCP endpoint availability and response time.

Following charts are drawn per host:

1.  **Latency** ms

    -   Time required to connect to a TCP port.
    Displays latency in 0.1 ms resolution. If the connection failed, the value is missing.

2.  **Status** boolean

    -   Connection successful
    -   Could not create socket: possible DNS problems
    -   Connection refused: port not listening or blocked
    -   Connection timed out: host or port unreachable

## Configuration

Edit the `python.d/portcheck.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/portcheck.conf
```

```yaml
server:
  host: 'dns or ip'     # required
  port: 22              # required
  timeout: 1            # optional
  update_every: 1       # optional
```

### notes

-   The error chart is intended for alarms, badges or for access via API.
-   A system/service/firewall might block Netdata's access if a portscan or
    similar is detected.
-   Currently, the accuracy of the latency is low and should be used as reference only.

---


