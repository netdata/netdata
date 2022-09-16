<!--
title: "Icecast monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/icecast/README.md
sidebar_label: "Icecast"
-->

# Icecast monitoring with Netdata

Monitors the number of listeners for active sources.

## Requirements

-   icecast version >= 2.4.0

It produces the following charts:

1.  **Listeners** in listeners

-   source number

## Configuration

Edit the `python.d/icecast.conf` configuration file using `edit-config` from the Netdata [config
directory](/docs/configure/nodes.md), which is typically at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/icecast.conf
```

Needs only `url` to server's `/status-json.xsl`

Here is an example for remote server:

```yaml
remote:
  url      : 'http://1.2.3.4:8443/status-json.xsl'
```

Without configuration, module attempts to connect to `http://localhost:8443/status-json.xsl`

---


